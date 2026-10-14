package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/Disble/dharness/internal/tiers"
)

// RunPolicy is the read-only facade used by native coverage configuration.
// It is intentionally not a new manual step or a test-runner wrapper.
func RunPolicy(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("policy", flag.ContinueOnError)
	flags.SetOutput(stdout)
	root := flags.String("root", "", "repository root containing coverage-tiers.json")
	flags.Usage = func() {
		fmt.Fprintln(stdout, "Read-only coverage-tier resolution (coverage integration helper).")
		fmt.Fprintln(stdout, "Usage: dharness policy --root <repository-root>")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if *root == "" {
		return fmt.Errorf("policy: --root is required by the coverage integration")
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("policy: unexpected positional arguments; use --root <repository-root>")
	}
	resolution, err := tiers.Resolve(*root)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(stdout).Encode(resolution); err != nil {
		return fmt.Errorf("policy: write resolved JSON: %w", err)
	}
	return nil
}
