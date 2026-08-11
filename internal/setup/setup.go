// Package setup holds the one description of what a project needs, and the two
// verbs that act on it.
//
// sync reports the steps that are not satisfied; init applies them. They read
// the same plan, so neither can drift from the other: a step added here shows
// up in both, and a step whose Satisfied changes stops being reported and
// stops being applied at the same moment.
package setup

import (
	"errors"
	"fmt"
	"io"

	"github.com/Disble/dharness/internal/project"
)

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
	// It runs only when Delegated(p) returned ok == false.
	Apply(p project.Project, w *Writer) error
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
		legacyLintConfigStep{},
		doctorConfigStep{},
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
	return applySteps(Pending(p), p, stdout)
}

// applySteps runs Apply on every step in steps whose Delegated(p) answers
// ok == false, in order. Split out from Apply so the loop's own contract —
// a delegated step is never touched — can be tested against a stub step
// without depending on setup.Plan().
func applySteps(steps []Step, p project.Project, stdout io.Writer) error {
	writer := &Writer{}

	for _, step := range steps {
		if _, ok := step.Delegated(p); ok {
			continue
		}

		fmt.Fprintf(stdout, "  %s\n", step.ID())
		if err := step.Apply(p, writer); err != nil {
			if undoErr := writer.Undo(); undoErr != nil {
				return errors.Join(
					fmt.Errorf("%s failed and the repository could not be restored: %w", step.ID(), err),
					undoErr,
				)
			}
			// The hedge is deliberate: Writer.Undo restores files it snapshotted
			// and does not remove directories created by os.MkdirAll, nor the
			// .gitignore written outside the Writer by
			// project.Project.EnsureDir. Tighten this sentence to "everything
			// this run wrote was undone" in `writer-undo-completeness`.
			return fmt.Errorf(
				"%s failed. Every file this run wrote was put back as it was found; directories it created are not removed. No earlier step is reported as having succeeded: %w",
				step.ID(), err)
		}
	}
	return nil
}
