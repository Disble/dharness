package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/runner"
	"github.com/Disble/dharness/internal/tool"
)

// RunCheck is the commit gate.
//
// react-doctor runs first because --staged scopes it to the change, so its cost
// tracks the diff rather than the repository. fallow runs second because audit
// limits what it reports to changed files but still builds the repository graph,
// which gives it a higher floor. Cheapest first is not a style preference: a
// failure in the first skips the second entirely, and that is where most of the
// saving in a gate that runs on every commit comes from.
//
// One dependency this gate cannot enforce: react-doctor adopts the project's
// ESLint or oxlint JSON configuration when one exists, so a broken lint config
// is read as a broken config in silence. Lint is not a dharness step, so
// ordering it before this one is the hook's responsibility.
func RunCheck(args []string, stdout io.Writer) error {
	flags := newFlagSet("check", stdout, "Run the commit gate: react-doctor, then fallow.")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if helpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected check argument %q", flags.Arg(0))
	}

	dir, err := workingDirectory()
	if err != nil {
		return err
	}

	p, err := project.Discover(dir)
	if err != nil {
		return err
	}
	if !p.HasSource() {
		fmt.Fprintln(stdout, noSourceMessage(p))
		return nil
	}

	staged, err := p.StagedSourceFiles()
	if err != nil {
		return err
	}
	if len(staged) == 0 {
		fmt.Fprintln(stdout, "no staged source files, nothing to check")
		return nil
	}

	stages := []stage{{tool.ReactDoctor, tool.ReactDoctorStaged()}}

	// fallow compares the change against a base, and a repository with no
	// commits has none. Its own error names the fix, but there is nothing to
	// fix: this is the first commit, which is exactly when adoption ends. The
	// cheapest way to run something is not to run it when it cannot answer.
	if project.HasCommits(p.Root) {
		stages = append(stages, stage{tool.Fallow, tool.FallowAudit()})
	} else {
		defer fmt.Fprintf(stdout, "\n%s did not run: this repository has no commits yet, so there is\nno base to compare against. It runs from the next commit on.\n", tool.Fallow)
	}

	for index, stage := range stages {
		// Two tools writing into one stream with nothing between them leaves
		// whoever reads the gate — a person, or the model that ran it — to work
		// out where one report ends and the next begins.
		fmt.Fprintf(stdout, "\n── %s ──\n", stage.tool)

		command := p.Resolve(stage.tool).Command(p.Source, stage.args...)
		if err := runner.Run(command, stdout, stdout); err != nil {
			if skipped := stages[index+1:]; len(skipped) > 0 {
				fmt.Fprintf(stdout, "\n%s failed, so %s did not run. There may be more to fix behind it.\n",
					stage.tool, names(skipped))
			}
			fmt.Fprint(stdout, pointer(p, stage.tool))
			return err
		}
	}

	return nil
}

// stage is one wrapped tool and the arguments dharness gives it.
type stage struct {
	tool string
	args []string
}

func names(stages []stage) string {
	list := make([]string, 0, len(stages))
	for _, stage := range stages {
		list = append(list, stage.tool)
	}
	return strings.Join(list, " and ")
}
