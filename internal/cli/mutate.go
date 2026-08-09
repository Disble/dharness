package cli

import (
	"errors"
	"fmt"
	"io"

	"dharness/internal/project"
	"dharness/internal/runner"
	"dharness/internal/tool"
)

// defaultConcurrency is deliberately below the core count Stryker would pick.
//
// The project's own measurements showed eight workers finishing slower than the
// default on a small scope, and this runs on a machine that has to stay usable
// while it does.
const defaultConcurrency = 2

// ErrNoMutatePaths reports a run with nothing to mutate.
var ErrNoMutatePaths = errors.New("mutate needs at least one path, for example: dharness mutate src/thing.ts")

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
// inflate that graph until every test counts as related. Measuring it costs one
// initial test run and mutates nothing.
func RunMutate(args []string, stdout io.Writer) error {
	flags := newFlagSet("mutate <path...>", stdout, "Run mutation testing over the given files. Use it once their tests are green.")
	concurrency := flags.Int("concurrency", defaultConcurrency, "Stryker workers")
	dryRun := flags.Bool("dry-run", false, "measure how many tests a scoped run executes, without mutating anything")
	breakAt := flags.Int("break", tool.AnySurvivorFails, "fail below this mutation score; 0 defers to the project's own Stryker configuration")
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

	p := project.Describe(dir)
	testRunner, err := p.StrykerRunner()
	if err != nil {
		return err
	}

	// A project that configured Stryker chose its own thresholds and reporters,
	// so dharness leaves both alone, on the same rule that governs testRunner.
	threshold := *breakAt
	if p.HasStrykerConfig() {
		threshold = 0
	}

	strykerArgs := tool.StrykerMutate(paths, testRunner, *concurrency, threshold)
	if *dryRun {
		fmt.Fprintln(stdout, "Running the initial test run only, to count the tests the runner considers related.")
		strykerArgs = tool.StrykerDryRun(paths, testRunner)
	}

	if err := runner.Run(p.Resolve(tool.Stryker).Command(dir, strykerArgs...), stdout, stdout); err != nil {
		fmt.Fprint(stdout, pointer(p, tool.Stryker))
		return err
	}
	return nil
}
