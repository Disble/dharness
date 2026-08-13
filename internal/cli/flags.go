// Package cli holds one function per command. Each takes its arguments and the
// writer to report on, and returns an error rather than an exit code: deciding
// how a failure becomes a status belongs to the top level, not to a command.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/runner"
)

// newFlagSet builds a subcommand flag set that reports on the caller's writer
// instead of stderr, so command output stays assertable in tests.
func newFlagSet(name string, out io.Writer, description string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(out)
	flags.Usage = func() {
		fmt.Fprintf(out, "Usage: dharness %s\n%s\n\nFlags:\n", name, description)
		flags.PrintDefaults()
	}
	return flags
}

// parseInterspersed parses flags that appear anywhere among the positional
// arguments, and returns those arguments.
//
// The standard flag package stops parsing at the first non-flag, which would
// silently turn `dharness mutate src/a.ts --dry-run` into a run over two paths
// named "src/a.ts" and "--dry-run". Nobody writes their flags first, so this
// consumes one positional at a time and resumes parsing after it.
func parseInterspersed(flags *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	rest := args
	for len(rest) > 0 {
		if err := parseFlags(flags, rest); err != nil {
			return nil, err
		}
		rest = flags.Args()
		if len(rest) == 0 {
			break
		}
		positional = append(positional, rest[0])
		rest = rest[1:]
	}
	return positional, nil
}

func parseFlags(flags *flag.FlagSet, args []string) error {
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	return nil
}

func helpRequested(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "help" {
			return true
		}
	}
	return false
}

// workingDirectory is swappable so commands can be tested against a fixture.
var workingDirectory = os.Getwd

// pointer tells the reader where to go next when the gate says no.
//
// dharness wraps three things — adoption, configuration and the gates — and
// deliberately nothing else. Every tool here has its own surface for the next
// question: why a rule fired, what a finding means, which rules exist. Wrapping
// any of it would mean tracking their flags forever, so the failure output
// names the tool's own help instead and stops there.
//
// The pointer uses the remote form on purpose. These CLIs are meant to be run
// at their latest version, that is how they are used in practice, and an
// exploratory question is exactly where that costs nothing.
func pointer(command runner.Command) string {
	invocation := strings.Join(append([]string{command.Name}, command.Args...), " ")
	return fmt.Sprintf(
		"\ndharness wraps the gate, not %s itself. For anything beyond pass or fail —\nwhy a finding fired, what it means, which rules exist — ask the tool:\n\n    %s\n",
		command.String(), invocation,
	)
}

// noSourceMessage explains the one state in which dharness has nothing to say.
//
// Every tool dharness wraps analyses JavaScript, so a repository with no place
// a package manager installs is not a repository dharness can gate. Saying so
// is the honest answer; the alternative — the old default of "npm" — reported a
// Go module as an npm project and offered to install into it.
func noSourceMessage(p project.Project) string {
	return fmt.Sprintf(
		"No JS project found in %s: nothing there holds a lockfile, so there is no\nplace a package manager installs and nothing for the wrapped tools to analyse.\n\nIf the project is there but has never been installed, install it first — the\nlockfile is what dharness looks for.",
		p.Root,
	)
}
