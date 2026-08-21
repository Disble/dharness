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
	"os"
	"path/filepath"
	"strings"
	"time"

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

// Verifier is the extra obligation a step takes on when its Apply hands the
// outcome to something dharness does not control.
//
// Apply returning nil says one thing: the work it started did not error. For
// a step that writes a file itself, that is the whole answer — the bytes are
// there or Write failed. For a step that shells out, it is a proxy for the
// answer, and §09 is exactly about not accepting a proxy when the direct
// signal is available: `lefthook install` exits 0 whether or not it installed
// the hook dharness needs, and the direct signal — a pre-commit hook that
// carries the gate — is one os.ReadFile away.
//
// A step implements this when its postcondition is checkable and its Apply
// cannot check it. run asks after a successful Apply, and a postcondition
// that does not hold is a failure, not an applied step with a caveat.
//
// It is deliberately not part of Step. Most steps have nothing to add: their
// Apply already proves what they claim, and a Verify returning nil on every
// one of them would be eleven implementations of "trust me".
type Verifier interface {
	// Verify reports why this step's postcondition does not hold, or nil
	// when it does. It runs after Apply and reads the repository, exactly
	// like Satisfied — never the transcript, and never the exit code it is
	// there to double-check.
	Verify(p project.Project) error
}

// verified answers a step's postcondition, or nil for a step that has none
// to answer. Split out so run's own body stays one status decision per
// branch rather than an interface assertion inline.
func verified(step Step, p project.Project) error {
	verifier, ok := step.(Verifier)
	if !ok {
		return nil
	}
	return verifier.Verify(p)
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

// Run derives the plan, applies what it can, and reports one result per
// Plan() step plus every note read before the first byte changes — the
// structured value RunSync assembles into a report.Report and renders
// twice (design.md Decision 8). It is Plan()'s own thin wrapper over run,
// the same shape Apply is over applySteps(Pending(p), p, stdout): unlike
// Apply, Run has to build a StepResult even for a step that is already
// satisfied or delegated (defect 1's rule, extended to every status), so it
// cannot reuse Pending or applySteps, which both exist to skip exactly
// those steps.
func Run(p project.Project) ([]report.StepResult, []report.Note, error) {
	return run(Plan(), p)
}

// run is Run's own implementation, over an explicit plan rather than a call
// to Plan() buried inside it — the same reason applySteps takes steps
// []Step rather than calling Plan() itself: a stub plan lets a test drive
// exactly the branch it needs (satisfied, delegated, or a failure at a
// chosen index) without depending on the real registry's eleven steps.
//
// Status is decided in the order Pending/applySteps have always used:
// Satisfied first, so an already-satisfied step is never even asked
// Delegated, then Delegated, then Apply — preserving today's semantics and
// keeping boundariesOwnerStep's fallback constants unreachable in the
// product (design.md Decision 8, change #1; guarded by
// TestBoundariesFallbackConstantsStayByteIdentical).
//
// That order puts one obligation on every step: a step that delegates must
// not also answer Satisfied. Both answering true is not a harmless overlap
// here — Delegated is never asked, so the reason is computed and discarded,
// and a step that refused to act is filed under "Already in place". The
// apply path tolerated it, because there Satisfied selects and Delegated
// skips; this one does not.
func run(plan []Step, p project.Project) (steps []report.StepResult, notes []report.Note, err error) {
	notes = collectNotes(p)

	writer := &Writer{}
	results := make([]report.StepResult, 0, len(plan))

	for i, step := range plan {
		n := i + 1

		if step.Satisfied(p) {
			results = append(results, report.StepResult{
				N:        n,
				ID:       step.ID(),
				Status:   report.Satisfied,
				Evidence: satisfiedEvidence(p, step),
			})
			continue
		}

		if why, ok := step.Delegated(p); ok {
			results = append(results, delegatedResult(p, n, step, why))
			continue
		}

		start := time.Now()
		var sink bytes.Buffer
		before := len(writer.touched)
		facts, applyErr := step.Apply(p, writer, &sink)
		after := len(writer.touched)

		// The postcondition is asked here rather than inside Apply because
		// it has to hold for the step, not for the command the step ran:
		// asking Apply to check itself is asking the same proxy twice. The
		// clock covers it — verifying is part of what the step cost.
		if applyErr == nil {
			applyErr = verified(step, p)
		}
		ms := time.Since(start).Milliseconds()

		if applyErr != nil {
			return retractAndReport(results, plan, i, step, ms, applyErr, writer)
		}

		results = append(results, report.StepResult{
			N:          n,
			ID:         step.ID(),
			Status:     report.Applied,
			MS:         ms,
			Transcript: sink.String(),
			Installed:  facts.Installed,
			Wrote:      writer.Changed(p.Root, before, after),
		})
	}

	return results, notes, nil
}

// delegatedResult builds one delegated step's report entry.
// boundariesOwnerStep is the one case step-delegation's added requirement
// covers: when there is a real collision, the same structured Collision
// values Collisions(p) computes for the report replace why on the entry,
// rather than sitting beside it unused — keeping Why and Collisions
// mutually exclusive on any StepResult this package actually builds
// (design.md Decision 4), not merely on what the renderer happens to
// prefer when both are present.
func delegatedResult(p project.Project, n int, step Step, why string) report.StepResult {
	result := report.StepResult{N: n, ID: step.ID(), Status: report.Delegated, Why: why}

	if _, isBoundaries := step.(boundariesOwnerStep); isBoundaries {
		if collisions := Collisions(p); len(collisions) > 0 {
			result.Collisions = collisions
			result.Why = ""
		}
	}

	return result
}

// satisfiedEvidence names the fact that satisfied step, for a report's
// reader who was never shown the unsatisfied case (design.md Decision 8:
// "satisfied with its Describe/detection evidence").
//
// Describe's own text is fix instructions for the *unsatisfied* case — most
// often actively wrong read as evidence that nothing needs doing (gap 4,
// from the team lead's measured run: legacyLintConfigStep's satisfied row
// showed "Make .eslintrc.json parse, or delete it..." as if that were a
// fact about why the step was already done). Every step whose Satisfied
// condition is a single checkable fact states that fact directly here,
// rather than reusing Describe. Only the branches Satisfied itself cannot
// distinguish from one another with a cheap, side-effect-free check keep
// the firstLine(Describe(p)) fallback in the default case below.
func satisfiedEvidence(p project.Project, step Step) string {
	switch step.(type) {
	case boundariesOwnerStep:
		return "no colliding key declared"

	case fallowExtendsStep:
		if !p.HasSource() {
			return firstLine(step.Describe(p))
		}
		return "extends → " + ownedFrom(p, p.Source, ownedFallow)

	case lefthookExtendsStep:
		if hookManager(p) != managerLefthook {
			return "no lefthook config"
		}
		return "extends → " + ownedFrom(p, p.Root, ownedLefthook)

	case legacyLintConfigStep:
		if !p.HasSource() {
			return firstLine(step.Describe(p))
		}
		if _, err := os.Stat(filepath.Join(p.Source, legacyLintConfig)); errors.Is(err, os.ErrNotExist) {
			return "not present"
		}
		return "parses"

	case mcpStep:
		return mcpConfig + " declares fallow"

	case hookInstallStep:
		switch hookManager(p) {
		case managerHusky:
			return huskyHook
		default:
			return gateHookPath(p)
		}

	case agentSkillStep:
		for _, candidate := range skillLocations {
			if _, err := os.Stat(filepath.Join(p.Root, candidate)); err == nil {
				return filepath.ToSlash(candidate) + " present"
			}
		}
		return firstLine(step.Describe(p))

	case architectureStep:
		return "boundaries declared"

	case ownedFilesStep:
		return "owned files match"

	case eslintExtendsStep:
		return eslintExtendsSatisfiedEvidence(p, step)

	default:
		return firstLine(step.Describe(p))
	}
}

// eslintExtendsSatisfiedEvidence names what a satisfied eslintExtendsStep
// actually converged on.
//
// It used to mirror every one of Satisfied's branches, because Satisfied
// answered true for the delegating shapes too. It no longer does — a shape
// that delegates is not satisfied — so the only states that reach here are
// a repository with no JS project, which has no config to describe, and a
// spliced config whose regions already match byte for byte. The removed
// branches ("TypeScript config present", "config shape not recognised" and
// the rest) were not extra detail: they were this function reporting, under
// an "Already in place" heading, the reasons a step had been refused.
func eslintExtendsSatisfiedEvidence(p project.Project, step Step) string {
	if !p.HasSource() {
		return firstLine(step.Describe(p))
	}
	return "spliced regions match"
}

// gateHookPath names the pre-commit hook this repository actually uses,
// relative to its root so a report names a place rather than a machine.
//
// It answers ".git/hooks/pre-commit" for a repository that has not moved its
// hooks, which is what this evidence line always said — and the real
// directory for one that has, which it used to get wrong. The join is the
// fallback for a repository git will not answer for; a satisfied step that
// got here read the hook from somewhere, so naming the default is better
// than naming nothing.
func gateHookPath(p project.Project) string {
	hooks, err := project.HooksDir(p.Root)
	if err != nil {
		return filepath.ToSlash(filepath.Join(".git", "hooks", "pre-commit"))
	}
	if rel, err := filepath.Rel(p.Root, hooks); err == nil {
		hooks = rel
	}
	return filepath.ToSlash(filepath.Join(hooks, "pre-commit"))
}

// firstLine takes a satisfied step's evidence from its own Describe(p): the
// detection fact a reader already trusts, since it is the same sentence
// Describe always renders. Only the first line is kept — Describe's own
// text runs to several lines of fix instructions for most steps, and the
// satisfied block's aligned rows have one line per step; a multi-line
// Evidence value would break that alignment, not merely look untidy.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// retractAndReport undoes what this run wrote, marks the failed step
// failed, every earlier-applied step retracted, and every step this run
// never reached not-reached, then returns the failure. It is the
// explicit-retraction obligation project-sync's added requirement states —
// a status already printed is retracted by name, not merely contradicted
// by omission — and defect 1's rule applied to the failure path: every step
// in plan still carries exactly one status, even the ones this run never
// attempted.
//
// The returned error keeps only the step name and cause: the narrative —
// which step is retracted, which are not-reached — belongs to the report
// RunSync builds from steps, not to the error string a second time (design.md
// Decision 8, change #3: "the report states the narrative, the error states
// the cause, and neither repeats the other").
func retractAndReport(results []report.StepResult, plan []Step, failedAt int, failedStep Step, ms int64, applyErr error, writer *Writer) ([]report.StepResult, []report.Note, error) {
	if undoErr := writer.Undo(); undoErr != nil {
		return nil, nil, errors.Join(
			fmt.Errorf("%s failed and the repository could not be restored: %w", failedStep.ID(), applyErr),
			undoErr,
		)
	}

	for i := range results {
		if results[i].Status == report.Applied {
			results[i].Status = report.Retracted
		}
	}

	results = append(results, report.StepResult{
		N:      failedAt + 1,
		ID:     failedStep.ID(),
		Status: report.Failed,
		MS:     ms,
		Error:  applyErr.Error(),
	})

	for i := failedAt + 1; i < len(plan); i++ {
		results = append(results, report.StepResult{
			N:      i + 1,
			ID:     plan[i].ID(),
			Status: report.NotReached,
		})
	}

	return results, nil, fmt.Errorf("%s failed: %w", failedStep.ID(), applyErr)
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
