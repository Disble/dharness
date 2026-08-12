// Package setup holds the one description of what a project needs, and the two
// verbs that act on it.
//
// sync reports the steps that are not satisfied; init applies them. They read
// the same plan, so neither can drift from the other: a step added here shows
// up in both, and a step whose Satisfied changes stops being reported and
// stops being applied at the same moment.
package setup

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/report"
)

// Facts are what a step knows and a transcript cannot say. A sink of raw
// bytes cannot answer "what package"; only the step that built the install
// command can, and it already has the list.
type Facts struct {
	// Installed names the package specs this step asked the manager to add
	// — exactly the slice it passed to tool.InstallPackages, never anything
	// read back out of the subprocess's output (§01, §09).
	Installed []string
}

// Step is one thing a project needs.
//
// Satisfied is what makes both verbs safe to repeat: state is derived from the
// repository every time rather than recorded, so a step undone by hand comes
// back and a step already done is skipped.
type Step interface {
	// ID names the step for a report.
	ID() string

	// Describe says what is missing and, where there is one, the command that
	// would fix it.
	Describe(p project.Project) string

	// Satisfied reports whether the repository already meets this step.
	Satisfied(p project.Project) bool

	// Delegated reports whether this repository leaves the step to the agent,
	// and why. Pure, like Satisfied: it is answered during Prepare, which
	// writes nothing and therefore cannot fail.
	Delegated(p project.Project) (why string, ok bool)

	// Apply performs the step, recording what it touched so it can be undone.
	// It runs only when Delegated(p) returned ok == false. Everything it
	// produces goes to out, never to the process's own stdout, so applySteps
	// can frame it under this step. A non-nil error means Facts is not read.
	Apply(p project.Project, w *Writer, out io.Writer) (Facts, error)
}

// Plan is everything dharness knows how to check, in the order it must happen.
//
// Installing precedes writing configuration that names the installed packages,
// and wiring the gate precedes asking a hook manager to install it.
func Plan() []Step {
	return []Step{
		installStep{},
		ownedFilesStep{},
		fallowExtendsStep{},
		boundariesOwnerStep{},
		lefthookExtendsStep{},
		eslintExtendsStep{},
		legacyLintConfigStep{},
		mcpStep{},
		hookInstallStep{},
		agentSkillStep{},
		architectureStep{},
	}
}

// Pending returns the steps this project does not yet satisfy.
func Pending(p project.Project) []Step {
	var pending []Step
	for _, step := range Plan() {
		if !step.Satisfied(p) {
			pending = append(pending, step)
		}
	}
	return pending
}

// Apply performs every pending step that dharness can perform.
//
// On failure everything written is undone and the error names the step. There
// is no partial success to report afterwards: a run that rolled back left the
// repository as it found it, and saying "four of six worked" would describe a
// state that no longer exists.
func Apply(p project.Project, stdout io.Writer) error {
	_, err := applySteps(Pending(p), p, stdout)
	return err
}

// stepOutcome is what applySteps learned about one applied step: the
// structured Facts Apply returned, and the files it touched — partitioned
// from the run's shared Writer by the before/after mark taken around this
// one call, per the file-attribution requirement. Held for a future caller
// to fold into a report.StepResult; Apply's compat wrapper above discards
// it, since its only remaining caller (renderGolden) never reads it.
type stepOutcome struct {
	id    string
	facts Facts
	wrote []report.FileChange
}

// applySteps runs Apply on every step in steps whose Delegated(p) answers
// ok == false, in order. Split out from Apply so the loop's own contract —
// a delegated step is never touched — can be tested against a stub step
// without depending on setup.Plan().
//
// Each step's Apply writes to a bytes.Buffer of its own rather than directly
// to stdout, so nothing it produces can reach the process's own stream
// unframed ahead of the report that will eventually own that framing
// (step-outcome's sink requirement, defect 5). This slice copies that
// buffer straight onto stdout immediately after Apply returns, preserving
// today's interleaving and byte content — Decision 9's own invariant that
// slice 2 changes nothing a user sees, until slice 4 rewrites RunSync to
// render a report instead of streaming live.
func applySteps(steps []Step, p project.Project, stdout io.Writer) ([]stepOutcome, error) {
	writer := &Writer{}
	var outcomes []stepOutcome

	for _, step := range steps {
		if _, ok := step.Delegated(p); ok {
			continue
		}

		fmt.Fprintf(stdout, "  %s\n", step.ID())

		var sink bytes.Buffer
		before := len(writer.touched)
		facts, err := step.Apply(p, writer, &sink)
		after := len(writer.touched)
		stdout.Write(sink.Bytes())

		if err != nil {
			if undoErr := writer.Undo(); undoErr != nil {
				return nil, errors.Join(
					fmt.Errorf("%s failed and the repository could not be restored: %w", step.ID(), err),
					undoErr,
				)
			}
			// The hedge is deliberate: Writer.Undo restores files it snapshotted
			// and does not remove directories created by os.MkdirAll, nor the
			// .gitignore written outside the Writer by
			// project.Project.EnsureDir. Tighten this sentence to "everything
			// this run wrote was undone" in `writer-undo-completeness`.
			return nil, fmt.Errorf(
				"%s failed. Every file this run wrote was put back as it was found; directories it created are not removed. No earlier step is reported as having succeeded: %w",
				step.ID(), err)
		}

		outcomes = append(outcomes, stepOutcome{
			id:    step.ID(),
			facts: facts,
			wrote: writer.Changed(p.Root, before, after),
		})
	}
	return outcomes, nil
}
