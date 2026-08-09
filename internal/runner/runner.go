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

	err := process.Run()
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
