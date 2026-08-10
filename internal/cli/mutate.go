package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/runner"
	"github.com/Disble/dharness/internal/tool"
)

// defaultConcurrency is deliberately below the core count Stryker would pick.
//
// The project's own measurements showed eight workers finishing slower than the
// default on a small scope, and this runs on a machine that has to stay usable
// while it does.
const defaultConcurrency = 2

// ErrNoMutatePaths reports a run with nothing to mutate.
var ErrNoMutatePaths = errors.New("mutate needs at least one path, for example: dharness mutate src/thing.ts")

// SurvivorsError reports mutants that no test noticed.
//
// It exists because Stryker will not report this itself: its exit code says
// nothing about survivors unless a threshold is set in a config file, and there
// is no command line equivalent. Verified by running it — a run with six killed
// mutants and one survivor exited 0.
type SurvivorsError struct {
	Survivors []tool.Survivor
}

func (e *SurvivorsError) Error() string {
	return fmt.Sprintf("%d mutant(s) survived: a test would not have noticed this code breaking", len(e.Survivors))
}

// RunMutate runs mutation testing over exactly the paths it is given.
//
// This is a command, not a gate. Mutation testing answers whether a test would
// notice the code breaking, which only means anything once the tests are green,
// so it belongs after the green step and before the refactor — invoked when a
// unit of work is finished, never on every commit.
//
// --dry-run answers a different question with the same machinery: how many
// tests a scoped run would actually execute. Stryker does not derive that from
// the paths, the test runner derives it from the import graph, and barrel files
// inflate that graph until every test counts as related.
func RunMutate(args []string, stdout io.Writer) error {
	flags := newFlagSet("mutate <path...>", stdout, "Run mutation testing over the given files. Use it once their tests are green.")
	concurrency := flags.Int("concurrency", defaultConcurrency, "Stryker workers")
	dryRun := flags.Bool("dry-run", false, "measure how many tests a scoped run executes, without mutating anything")
	paths, err := parseInterspersed(flags, args)
	if err != nil {
		return err
	}
	if helpRequested(args) {
		return nil
	}

	if len(paths) == 0 {
		return ErrNoMutatePaths
	}
	if *concurrency < 1 {
		return fmt.Errorf("--concurrency needs a positive number, got %d", *concurrency)
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
		return fmt.Errorf("%s", noSourceMessage(p))
	}

	paths, err = scopePaths(p, dir, paths)
	if err != nil {
		return err
	}

	testRunner, err := p.StrykerRunner()
	if err != nil {
		return err
	}

	if *dryRun {
		fmt.Fprintln(stdout, "Running the initial test run only, to count the tests the runner considers related.")

		// The count only exists in the output: --dryRunOnly writes no report.
		var transcript bytes.Buffer
		if err := runStryker(p, p.Source, tool.StrykerDryRun(paths, testRunner, *concurrency), io.MultiWriter(stdout, &transcript)); err != nil {
			return err
		}
		return recordMeasurement(p, transcript.String(), paths[0], stdout)
	}

	incremental, err := p.StatePath("stryker-incremental.json")
	if err != nil {
		return err
	}
	// A corrupt incremental report makes Stryker fail in a way that reads like
	// a problem with the code under test. Discarding it costs one full run.
	project.DiscardIfUnreadable(incremental, json.Valid)

	sandbox, err := p.EnsureDir("stryker-tmp")
	if err != nil {
		return err
	}
	// Cleaned before as well as after: a previous run killed mid-flight leaves
	// a copy of the project behind, and Stryker will not reuse it.
	if err := runner.RemoveSandbox(sandbox); err != nil {
		return fmt.Errorf("clear the previous mutation sandbox: %w", err)
	}
	defer func() { _ = runner.RemoveSandbox(sandbox) }()

	if err := runStryker(p, p.Source, tool.StrykerMutate(paths, testRunner, incremental, sandbox, *concurrency), stdout); err != nil {
		return err
	}
	return reportSurvivors(p.Source, stdout)
}

// PathOutsideSourceError reports a path Stryker could never have mutated.
//
// It names both directories because the mistake it catches is a reasonable
// one: in a split layout the path that is correct to type and the path Stryker
// receives are different, and a tool that silently mutated nothing would look
// like a suite with no survivors.
type PathOutsideSourceError struct {
	Path   string
	Source string
}

func (e *PathOutsideSourceError) Error() string {
	return fmt.Sprintf("%s is outside this project's JS source (%s), and Stryker only mutates what is inside it", e.Path, e.Source)
}

// scopePaths re-expresses the paths a person typed the way Stryker will read
// them.
//
// Paths are typed relative to the directory the command was run from, and
// Stryker runs in the JS project, which in a split layout is a different
// directory. `dharness mutate frontend/src/a.ts` from the repository root would
// otherwise reach Stryker as `frontend/src/a.ts` interpreted from inside
// frontend/ — a path that does not exist, mutated with no complaint.
//
// A path outside the JS project is refused rather than clamped, because there
// is no correct path to guess at and mutating nothing is indistinguishable from
// mutating something that survived nothing.
func scopePaths(p project.Project, dir string, paths []string) ([]string, error) {
	scoped := make([]string, 0, len(paths))
	for _, given := range paths {
		absolute := given
		if !filepath.IsAbs(absolute) {
			absolute = filepath.Join(dir, absolute)
		}

		rel, err := filepath.Rel(p.Source, absolute)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, &PathOutsideSourceError{Path: given, Source: p.Source}
		}
		scoped = append(scoped, filepath.ToSlash(rel))
	}
	return scoped, nil
}

func runStryker(p project.Project, dir string, args []string, stdout io.Writer) error {
	command := p.Resolve(tool.Stryker).Command(dir, args...)
	command.LowPriority = true

	if err := runner.Run(command, stdout, stdout); err != nil {
		fmt.Fprint(stdout, pointer(p, tool.Stryker))
		return err
	}
	return nil
}

// recordMeasurement persists what the dry run cost, so sync can stop asking.
//
// Without it sync has no terminal state: every step it prints is derived from
// the tree, and this one is not derivable at all — it takes a full initial test
// run to learn. A repository that never records it is asked forever.
func recordMeasurement(p project.Project, transcript, path string, stdout io.Writer) error {
	related, err := tool.RelatedTests(transcript)
	if err != nil {
		return err
	}
	if err := p.RecordScopedMutation(path, related); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "\n%d test(s) ran for %s. Recorded in %s/, which is meant to be committed.\n",
		related, path, project.Dir)
	return nil
}

// reportSurvivors turns Stryker's report into the exit code it does not
// produce. A missing report is not a pass: it means the run said nothing.
func reportSurvivors(dir string, stdout io.Writer) error {
	path := filepath.Join(dir, tool.MutationReportPath)

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("mutation ran but wrote no report at %s, so its verdict cannot be read: %w", tool.MutationReportPath, err)
	}
	defer func() { _ = file.Close() }()

	survivors, err := tool.Survivors(file)
	if err != nil {
		return err
	}
	if len(survivors) == 0 {
		fmt.Fprintln(stdout, "\nEvery mutant was caught: these tests notice this code breaking.")
		return nil
	}

	fmt.Fprintf(stdout, "\n%d mutant(s) survived — a test would not have noticed:\n\n", len(survivors))
	for _, survivor := range survivors {
		fmt.Fprintf(stdout, "  %s\n", survivor)
	}
	return &SurvivorsError{Survivors: survivors}
}
