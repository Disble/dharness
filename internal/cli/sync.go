package cli

import (
	"fmt"
	"io"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/setup"
)

// RunSync reports what this project still needs, and writes nothing.
//
// It reads the same plan `init` applies, so the two can never disagree: a step
// reported here is a step that would be performed there, and a step that stops
// being reported has stopped being outstanding for both.
//
// Every check derives its answer from the repository rather than from a record
// of what was done. That is what makes it safe to run at any time and useful
// long after adoption: a hook rewritten, a package removed, a runner swapped —
// each one makes its step reappear on its own.
func RunSync(args []string, stdout io.Writer) error {
	flags := newFlagSet("sync", stdout, "Report what this project still needs. Writes nothing; safe at any time.")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if helpRequested(args) {
		return nil
	}

	dir, err := workingDirectory()
	if err != nil {
		return err
	}
	p := project.Describe(dir)

	fmt.Fprintf(stdout, "# dharness in %s\n\n", p.Root)
	fmt.Fprintf(stdout, "Package manager: %s. Test runner: %s.\n\n", p.PackageManager, orNotDetected(p.TestRunner))

	pending := setup.Pending(p)
	if len(pending) == 0 {
		fmt.Fprintln(stdout, "Nothing to do: everything this project needs is in place.")
		if measured := p.ReadEvidence().ScopedMutation; measured != nil {
			fmt.Fprintf(stdout, "Scoped mutation ran %d test(s) for %s when it was measured.\n",
				measured.RelatedTests, measured.MeasuredPath)
		}
		return nil
	}

	for index, step := range pending {
		fmt.Fprintf(stdout, "## %d. %s\n\n", index+1, step.ID())
		if delegated, ok := step.(setup.Delegated); ok {
			fmt.Fprintf(stdout, "dharness cannot run this: %s\n\n", delegated.Why())
		}
		fmt.Fprintf(stdout, "%s\n\n", step.Describe(p))
	}

	fmt.Fprintln(stdout, "Run `dharness init` to apply everything above that has a command behind it.")
	return nil
}
