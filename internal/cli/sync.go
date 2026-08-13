package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/report"
	"github.com/Disble/dharness/internal/runner"
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
//
// setup.Run builds one report.Report — the single analysis both the human
// view and --format json render (design.md Decision 8). Neither writer
// reads the repository a second time or recomputes anything; the flag only
// picks which of the two already-built renderings reaches stdout.
func RunSync(args []string, stdout io.Writer) error {
	flags := newFlagSet("sync", stdout, "Set this project up: apply what dharness can, then hand the rest to the\nagent. Derived from the repository as it is right now, so re-running it is\nsafe and reports drift.")
	format := flags.String("format", "human", "output format: human or json")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if helpRequested(args) {
		return nil
	}
	if *format != "human" && *format != "json" {
		return fmt.Errorf("unknown --format %q; expected human or json", *format)
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
	if !p.HasSource() {
		fmt.Fprintln(stdout, noSourceMessage(p))
		return nil
	}

	start := time.Now()
	steps, notes, runErr := setup.Run(p)
	ms := time.Since(start).Milliseconds()

	rpt := report.Report{
		Root:    p.Root,
		Source:  p.SourceRel(),
		Summary: summarizeSteps(steps, ms),
		Steps:   steps,
		Notes:   notes,
		Exit:    runner.ExitCode(runErr),
	}
	if measured := p.ReadEvidence().ScopedMutation; measured != nil {
		rpt.Evidence = &report.Evidence{RelatedTests: measured.RelatedTests, MeasuredPath: measured.MeasuredPath}
	}
	if retracted := retractedStepNames(steps); len(retracted) > 0 {
		rpt.Rollback = &report.Rollback{Retracted: retracted}
	}

	if *format == "json" {
		if err := report.WriteJSON(stdout, rpt); err != nil {
			return err
		}
	} else if err := report.WriteHuman(stdout, rpt); err != nil {
		return err
	}

	return runErr
}

// summarizeSteps counts steps by status — the same counts both renderings
// read from Report.Summary, computed once here rather than by either
// writer (design.md Property 2: the verdict, and every count that feeds
// it, is assigned, never recomputed downstream).
func summarizeSteps(steps []report.StepResult, ms int64) report.Summary {
	s := report.Summary{Steps: len(steps), MS: ms}
	for _, step := range steps {
		switch step.Status {
		case report.Applied:
			s.Applied++
		case report.Delegated:
			s.Delegated++
		case report.Satisfied:
			s.Satisfied++
		case report.Failed:
			s.Failed++
		case report.Retracted:
			s.Retracted++
		}
	}
	return s
}

// retractedStepNames names every step setup.Run marked retracted, which is
// what Report.Rollback is built from — the explicit-retraction obligation
// project-sync's added requirement states, carried by the report rather
// than by the error a second time (design.md Decision 8, change #3).
func retractedStepNames(steps []report.StepResult) []string {
	var names []string
	for _, step := range steps {
		if step.Status == report.Retracted {
			names = append(names, step.ID)
		}
	}
	return names
}
