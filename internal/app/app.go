// Package app wires the command line to the commands.
//
// Run is the entry point; RunArgs is the same thing with its arguments and
// output injected, which is what makes the whole surface testable without
// touching os.Args or the terminal.
package app

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Disble/dharness/internal/cli"
	"github.com/Disble/dharness/internal/runner"
)

// Version is set from main via ldflags at build time.
var Version = "dev"

func Run() error {
	return RunArgs(os.Args[1:], os.Stdout)
}

func RunArgs(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		printHelp(stdout, Version)
		return nil
	}

	switch args[0] {
	case "help", "--help", "-h":
		printHelp(stdout, Version)
		return nil
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "dharness %s\n", Version)
		return nil
	case "sync":
		return cli.RunSync(args[1:], stdout)
	case "check":
		return cli.RunCheck(args[1:], stdout)
	case "mutate":
		return cli.RunMutate(args[1:], stdout)
	default:
		return &UnknownCommandError{Command: args[0]}
	}
}

// UnknownCommandError names the commands that do exist, because a mistyped
// command is the one moment a user is guaranteed to be reading the error.
type UnknownCommandError struct {
	Command string
}

func (e *UnknownCommandError) Error() string {
	return fmt.Sprintf("unknown command %q; expected sync, check, mutate or version", e.Command)
}

// ExitCode maps an error to a process status.
//
// A wrapped tool's own exit code is propagated unchanged: a gate that reports
// its own status instead of the tool's turns a failed check into a green commit
// whenever the two disagree.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *runner.ExitError
	if errors.As(err, &exitErr) && exitErr.Code != 0 {
		return exitErr.Code
	}
	return 1
}
