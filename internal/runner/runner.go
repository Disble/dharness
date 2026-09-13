// Package runner executes external commands.
//
// Everything dharness does is spawn other programs, so this is the one place
// that touches os/exec. Run is a package variable rather than an interface
// because the whole surface is a single function: tests replace it and observe
// what would have been invoked, without spawning anything.
package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// Command is one external invocation.
//
// Label is what the tool is called; Name is whatever has to be executed to
// reach it. They differ on purpose: a locally installed tool resolves to an
// absolute path, and reporting "C:\repo\node_modules\.bin\fallow.cmd failed"
// makes whoever reads the gate output — a person or the model that ran it —
// work out which of the three tools that was.
type Command struct {
	Label string
	Name  string
	Args  []string
	Dir   string

	// Stdin is what the process reads on its standard input, or nil for
	// nothing.
	//
	// It exists for one caller — fallow audit's --diff-stdin, which takes the
	// staged diff that scopes the audit. A reader rather than a temporary
	// file because the alternative was measured against this repository's own
	// history: --tempDirName carries the comment that Stryker's sandbox
	// "cleanTempDir only runs on a successful exit by default, so a failed run
	// leaves a copy of the project inside the repository". A gate fails often
	// by design, so the path that forgets to clean up is the common one. A
	// pipe leaves nothing because there is nothing to leave.
	Stdin io.Reader

	// LowPriority asks the operating system to schedule this process behind
	// whatever the machine is already doing.
	//
	// It is set for mutation testing, which is the one thing here that can
	// saturate a machine for minutes. Capping worker count is not the same
	// remedy: fewer workers still compete at the same priority, while a lower
	// priority yields the moment something else wants the CPU. The run takes
	// slightly longer in wall clock and the machine stays usable throughout.
	LowPriority bool

	// Context, when set, kills the running process the moment it is done —
	// left nil for every ordinary call, which starts no watcher goroutine
	// and behaves exactly as before.
	//
	// It exists for a staged mutation run: Stryker can run for minutes
	// against a throwaway snapshot, and a Ctrl-C that only stops dharness
	// itself would leave that child process still reading from — and the
	// snapshot's own cleanup racing to delete — a directory the process is
	// still using. Cancelling the Context kills the process first, so Run
	// returns and the caller's own cleanup runs after there is nothing left
	// reading from what it removes.
	Context context.Context
}

func (c Command) String() string {
	if c.Label != "" {
		return c.Label
	}
	return c.Name
}

// ExitError reports a command that ran and reported failure. The code is
// carried so dharness can exit with the same one: a gate that swallows a
// tool's exit code turns a red run into a green commit.
type ExitError struct {
	Command string
	Code    int

	// Interrupted records that a console interrupt ended the process rather
	// than the process failing on its own. See the Interrupted function.
	Interrupted bool
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("%s exited with code %d", e.Command, e.Code)
}

// Interrupted reports whether err is a process a console interrupt ended —
// Ctrl-C, or the console closing — under the default handling of it.
//
// It exists because such an interrupt reaches every process attached to the
// console at the same moment, the caller included, and the child can be dead
// before the caller's own handler has cancelled anything. Measured through a
// real `taskkill /PID` mid-run: Stryker came back with 3221225786 while the
// run's context was still live, and the run reported a Stryker failure beside
// a pointer to Stryker's help. How the process ended does not race.
func Interrupted(err error) bool {
	var exit *ExitError
	return errors.As(err, &exit) && exit.Interrupted
}

// StartError reports a command that could never be started — the binary was
// missing, or the shell refused it. It is distinct from ExitError because the
// remedy is different: one means the code is bad, the other means the setup is.
type StartError struct {
	Command string
	Cause   error
}

func (e *StartError) Unwrap() error { return e.Cause }

func (e *StartError) Error() string {
	return fmt.Sprintf("could not run %s: %v", e.Command, e.Cause)
}

// invocation is what os/exec is finally asked to run, after the platform has
// had its say about how a command has to be reached.
//
// CmdLine exists because os/exec's own quoting is not always right: a Windows
// shim is re-parsed by cmd.exe, which needs a line built to different rules.
// It is empty for every ordinary command, and when it is set it wins — the
// Args are not consulted, so platformize leaves them unset rather than
// recording a list that never runs.
type invocation struct {
	Name    string
	Args    []string
	CmdLine string

	// Err reports a command that must not be started at all, because the
	// platform cannot hand it its arguments unaltered.
	Err error
}

// ErrUndeliverableArgument reports an argument that cannot reach a tool
// unchanged, so the run is refused instead of made.
//
// It is a refusal rather than a best effort on purpose. Every other failure
// here ends in a verdict somebody acts on, and a tool asked about the wrong
// path answers confidently about the wrong path — a green gate over an
// unexamined file, or a red one naming a file that was never there.
var ErrUndeliverableArgument = errors.New("argument cannot be delivered unaltered")

// Run executes cmd, streaming its output to the given writers.
var Run = execute

// cancelledWaitDelay bounds how long Run waits, once a Command with a Context
// has exited, for output pipes something else still holds. See execute.
const cancelledWaitDelay = 2 * time.Second

func execute(cmd Command, stdout, stderr io.Writer) error {
	target := platformize(cmd.Name, cmd.Args)
	if target.Err != nil {
		return &StartError{Command: cmd.String(), Cause: target.Err}
	}

	process := exec.Command(target.Name, target.Args...)
	if target.CmdLine != "" {
		applyCmdLine(process, target.CmdLine)
	}
	process.Dir = cmd.Dir
	process.Env = Environ()
	process.Stdout = stdout
	process.Stderr = stderr
	// Left nil when the caller supplied nothing: os/exec then connects the
	// process to the null device, so a tool that reads stdin sees EOF rather
	// than blocking on a terminal the gate does not own.
	process.Stdin = cmd.Stdin

	// Priority is applied on both sides of Start because the two platforms
	// offer it at different moments: Windows sets a creation flag, POSIX
	// renices a process that already exists.
	if cmd.LowPriority {
		beforeStart(process)
	}
	if err := process.Start(); err != nil {
		return &StartError{Command: cmd.String(), Cause: err}
	}
	if cmd.LowPriority {
		afterStart(process.Process.Pid)
	}

	if cmd.Context != nil {
		// Kill reaches the process started here and nothing it started. On
		// Windows that process is cmd.exe running a .cmd shim, and the node
		// process behind it keeps the output pipe open, so Wait would read
		// until that grandchild exits on its own. Measured through an
		// npm-style shim whose node script sleeps 30 seconds: cancelled after
		// 1 second, Run returned after 30.10; with this delay, after 3.01, and
		// the grandchild died writing to the closed pipe within a further 1.5.
		// What this does not do is end the grandchild. One whose output is a
		// file rather than a pipe pins nothing and runs on: a console
		// interrupt reaches it directly, a signal sent to dharness alone does
		// not.
		process.WaitDelay = cancelledWaitDelay
		stopWatching := make(chan struct{})
		defer close(stopWatching)
		go func() {
			select {
			case <-cmd.Context.Done():
				_ = process.Process.Kill()
			case <-stopWatching:
			}
		}()
	}

	err := process.Wait()
	if err == nil {
		return nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return &ExitError{Command: cmd.String(), Code: exitErr.ExitCode(), Interrupted: endedByConsoleInterrupt(exitErr.ProcessState)}
	}
	return &StartError{Command: cmd.String(), Cause: err}
}

// SetForTest replaces Run and returns a restore function.
func SetForTest(replacement func(Command, io.Writer, io.Writer) error) func() {
	previous := Run
	Run = replacement
	return func() { Run = previous }
}

// ExitCode maps an error to a process status.
//
// A wrapped tool's own exit code is propagated unchanged: a gate that reports
// its own status instead of the tool's turns a failed check into a green commit
// whenever the two disagree. It lives here rather than in internal/app because
// its whole body depends on ExitError and nothing else, and internal/cli needs
// to reach it without the import cycle internal/app's own dependency on
// internal/cli would create.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) && exitErr.Code != 0 {
		return exitErr.Code
	}
	return 1
}
