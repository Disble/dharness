package cli

import (
	"fmt"
	"io"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/setup"
)

// RunInit performs what this project needs, and ends by handing over the one
// decision no command can make.
//
// It reads the same plan `sync` reports, and every step is guarded by the same
// derivation, so running it twice does the work once. If any step fails, every
// file written before it is restored and nothing is reported as having
// succeeded — a run that rolled back left the repository as it found it.
func RunInit(args []string, stdout io.Writer) error {
	flags := newFlagSet("init", stdout, "Set this project up, then hand the architecture analysis to the agent.")
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
	if len(pending) > 0 {
		fmt.Fprintln(stdout, "Applying:")
		if err := setup.Apply(p, stdout); err != nil {
			return err
		}
		fmt.Fprintln(stdout)
	}

	// What is left after applying is what no command performs. It is listed
	// with the reason, because "ask a person" without a reason is a shrug.
	for _, step := range setup.Pending(p) {
		delegated, ok := step.(setup.Delegated)
		if !ok {
			continue
		}
		fmt.Fprintf(stdout, "## Left to you: %s\n\n", step.ID())
		fmt.Fprintf(stdout, "dharness cannot run this: %s\n\n%s\n\n", delegated.Why(), step.Describe(p))
	}

	fmt.Fprint(stdout, setup.ArchitecturePrompt(p))
	return nil
}
