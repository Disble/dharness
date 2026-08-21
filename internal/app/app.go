// Package app wires the command line to the commands.
//
// Run is the entry point; RunArgs is the same thing with its arguments and
// output injected, which is what makes the whole surface testable without
// touching os.Args or the terminal.
package app

import (
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
	cli.Version = Version

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

// setupVerbs are the words people reach for on a project that has never seen
// dharness. None of them is a command — init was merged into sync (Decision
// 1) and the others never existed — and each of them means sync.
//
// The list is deliberately short and exact. A fuzzy match would start
// answering for typos of real commands too, and "did you mean sync" is
// unhelpful when what someone typed was `chekc`.
var setupVerbs = map[string]bool{
	"init":      true,
	"setup":     true,
	"bootstrap": true,
	"install":   true,
}

func (e *UnknownCommandError) Error() string {
	if setupVerbs[e.Command] {
		return fmt.Sprintf(
			"unknown command %q; to set this project up run `dharness sync`, which is safe to re-run. Expected sync, check, mutate or version",
			e.Command)
	}
	return fmt.Sprintf("unknown command %q; expected sync, check, mutate or version", e.Command)
}

// ExitCode maps an error to a process status. It forwards to
// runner.ExitCode, which owns the implementation now that internal/cli needs
// to reach it too — internal/app cannot be that shared home, since it is the
// package that imports internal/cli, not the other way around.
func ExitCode(err error) int {
	return runner.ExitCode(err)
}
