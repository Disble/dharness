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

	// Apply performs the step, recording what it touched so it can be undone.
	Apply(p project.Project, w *Writer) error
}

// Delegated is a step dharness cannot perform, with the reason it cannot.
//
// The distinction is not a shrug: a step is delegated only when no command
// exists that does exactly it, and the reason names what the available command
// would do instead.
type Delegated interface {
	Step
	Why() string
}

// Plan is everything dharness knows how to check, in the order it must happen.
//
// Installing precedes writing configuration that names the installed packages,
// and wiring the gate precedes asking a hook manager to install it.
func Plan() []Step {
	return []Step{
		installStep{},
		ownedFilesStep{},
		extendsStep{},
		doctorConfigStep{},
		mcpStep{},
		hookInstallStep{},
		agentSkillStep{},
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
	writer := &Writer{}

	for _, step := range Pending(p) {
		if _, delegated := step.(Delegated); delegated {
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
			return fmt.Errorf("%s failed; every earlier step was undone: %w", step.ID(), err)
		}
	}
	return nil
}
