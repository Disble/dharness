// Package runner executes external commands.
//
// Everything dharness does is spawn other programs, so this is the one place
// that touches os/exec. Run is a package variable rather than an interface
// because the whole surface is a single function: tests replace it and observe
// what would have been invoked, without spawning anything.
package runner

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
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

	// LowPriority asks the operating system to schedule this process behind
	// whatever the machine is already doing.
	//
	// It is set for mutation testing, which is the one thing here that can
	// saturate a machine for minutes. Capping worker count is not the same
	// remedy: fewer workers still compete at the same priority, while a lower
	// priority yields the moment something else wants the CPU. The run takes
	// slightly longer in wall clock and the machine stays usable throughout.
	LowPriority bool
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
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("%s exited with code %d", e.Command, e.Code)
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

// Run executes cmd, streaming its output to the given writers.
var Run = execute

func execute(cmd Command, stdout, stderr io.Writer) error {
	name, args := platformize(cmd.Name, cmd.Args)

	process := exec.Command(name, args...)
	process.Dir = cmd.Dir
	process.Stdout = stdout
	process.Stderr = stderr

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

	err := process.Wait()
	if err == nil {
		return nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return &ExitError{Command: cmd.String(), Code: exitErr.ExitCode()}
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
