package cli

import (
	"fmt"
	"io"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/setup"
)

// RunSync sets a project up: applies every step dharness can perform, then
// hands the rest to the agent with the reason it could not be run here.
//
// It derives its plan from the repository on every invocation rather than
// from a record of what a previous run did. That is what makes it safe to run
// at any time and useful long after adoption: a hook rewritten, a package
// removed, a runner swapped, a `boundaries` block written by hand — each one
// makes its step reappear or disappear on its own.
func RunSync(args []string, stdout io.Writer) error {
	flags := newFlagSet("sync", stdout, "Set this project up: apply what dharness can, then hand the rest to the\nagent. Derived from the repository as it is right now, so re-running it is\nsafe and reports drift.")
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
	p, err := project.Discover(dir)
	if err != nil {
		return err
	}
	if !p.InRepository {
		return fmt.Errorf(
			"%s is not inside a git repository. dharness owns a commit gate, so there is\n"+
				"nothing for adoption to attach to: no .git/hooks to install the hook into and\n"+
				"nothing to commit .dharness/ to. Run it from inside a repository.",
			dir)
	}

	fmt.Fprintf(stdout, "# dharness in %s\n\n", p.Root)
	if !p.HasSource() {
		fmt.Fprintln(stdout, noSourceMessage(p))
		return nil
	}
	if rel := p.SourceRel(); rel != "" {
		fmt.Fprintf(stdout, "JS project: %s/ — the repository root keeps the hook.\n", rel)
	}
	fmt.Fprintf(stdout, "Package manager: %s. Test runner: %s.\n\n", orNotDetected(p.PackageManager), orNotDetected(p.TestRunner))

	// Read before applying, because it is a property of the repository as
	// found. Adoption writes a .fallowrc.json of its own, which would make the
	// blind spot look like it had been resolved by the very run that could not
	// see past it.
	uncheckable := setup.UncheckableConfigNote(p)

	// Same reasoning, for a matched preset that could not read its own
	// configuration: it still contributed a default (see Match.Uncertain), so
	// this is what it guessed from, not a step nothing can clear.
	uncertain := setup.UncertainPresetNote(p)

	// Read before applying for the same reason as uncheckable above, though
	// nothing here writes doctor.config.json any more: consistency, not a
	// dependency. A repository adopted before this version may still carry
	// six dharness/* severities and RulesPackage in that file, which dharness
	// leaves exactly as found (§05) but does not stay quiet about.
	residue := setup.EslintResidueNote(p)

	if pending := setup.Pending(p); hasApplicable(pending, p) {
		fmt.Fprintln(stdout, "Applying:")
		if err := setup.Apply(p, stdout); err != nil {
			return err
		}
		fmt.Fprintln(stdout)
	}

	// What is left after applying is what no command performs here. It is
	// listed with the reason, because "ask a person" without a reason is a
	// shrug. Nothing is printed for a step that is now satisfied (§15): the
	// loop reads setup.Pending(p) again, which excludes it on its own.
	//
	// The terminal answer is decided here rather than before applying, because
	// what a re-run wants to know is what is still outstanding, not what was
	// outstanding a moment ago. installStep is never satisfied in a JS project
	// — it defers to the package manager instead of guessing — so a check made
	// before applying would never find a project with nothing pending.
	left := 0
	for _, step := range setup.Pending(p) {
		why, ok := step.Delegated(p)
		if !ok {
			continue
		}
		left++
		fmt.Fprintf(stdout, "## Left to you: %s\n\n", step.ID())
		fmt.Fprintf(stdout, "dharness cannot run this: %s\n\n%s\n\n", why, step.Describe(p))
	}

	// Printed beside the plan rather than in it. A config dharness cannot read
	// textually is a blind spot with no resolution the project can reach, so
	// it is not pending work — but staying quiet about it would report a check
	// that never ran as a check that passed.
	if uncheckable != "" {
		fmt.Fprintf(stdout, "## Not checked\n\n%s\n\n", uncheckable)
	}

	// A separate heading, not a second "Not checked", because it answers a
	// different question. The block above says a check did not run; this one
	// says a check did run and used a documented default because the project's
	// own answer could not be read. Two sections sharing one title read as one
	// section repeated.
	if uncertain != "" {
		fmt.Fprintf(stdout, "## Assumed\n\n%s\n\n", uncertain)
	}

	// A third heading, because this is neither of the other two: "Not
	// checked" says a check did not run, and "Assumed" says a check ran on a
	// default because the project's own answer could not be read. This one
	// says the opposite of both — dharness read the file just fine and knows
	// exactly what is in it. It is not pending work either (§15): there is no
	// state the project reaches that clears it, since dharness will never
	// remove what it finds.
	if residue != "" {
		fmt.Fprintf(stdout, "## Residue\n\n%s\n\n", residue)
	}

	if left == 0 {
		fmt.Fprintln(stdout, "Nothing to do: everything this project needs is in place.")
		if measured := p.ReadEvidence().ScopedMutation; measured != nil {
			fmt.Fprintf(stdout, "Scoped mutation ran %d test(s) for %s when it was measured.\n",
				measured.RelatedTests, measured.MeasuredPath)
		}
	}

	return nil
}

// hasApplicable reports whether at least one pending step is dharness's to
// run. Without this check the "Applying:" header would print ahead of a run
// where every pending step is delegated, claiming work that never happens.
func hasApplicable(pending []setup.Step, p project.Project) bool {
	for _, step := range pending {
		if _, ok := step.Delegated(p); !ok {
			return true
		}
	}
	return false
}
