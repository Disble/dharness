package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	mutation "github.com/Disble/dharness/internal/testsupport/mutation"
)

const (
	envIgnorePattern = "DHARNESS_MUTATION_IGNORE"
	envTestCommand   = "DHARNESS_MUTATION_TEST_CMD"
	envThreshold     = "DHARNESS_MUTATION_THRESHOLD"
	envRepositoryDir = "DHARNESS_MUTATION_ROOT"
	envScope         = "DHARNESS_MUTATION_SCOPE"
	defaultThreshold = "0.80"
	harnessPackage   = "./internal/testsupport/mutation/"
	harnessTimeout   = "10m"
)

type tool struct {
	cwd    string
	runner processRunner
	stdout io.Writer
	stderr io.Writer
}

type scopePlan struct {
	encoded string
	derived bool
	reason  string
	stats   mutation.ScopeStats
}

func main() {
	dry := flag.Bool("dry", false, "show staged mutation scope without executing mutants")
	flag.Parse()
	cwd, err := os.Getwd()
	if err == nil {
		err = newTool(cwd, os.Stdout, os.Stderr).run(*dry)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newTool(cwd string, stdout, stderr io.Writer) *tool {
	return &tool{cwd: cwd, runner: osProcessRunner{}, stdout: stdout, stderr: stderr}
}

func (tool *tool) run(dry bool) error {
	rootOutput, err := tool.git("rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("resolve Git repository root: %w", err)
	}
	root := filepath.Clean(strings.TrimSpace(string(rootOutput)))
	tool.cwd = root

	stagedOutput, err := tool.git("diff", "--cached", "--name-only", "--diff-filter=ACMR", "-z")
	if err != nil {
		return fmt.Errorf("list staged files: %w", err)
	}
	staged := selectMutableGoSources(splitNUL(stagedOutput))
	if len(staged) == 0 {
		fmt.Fprintln(tool.stdout, "go mutation: no staged production Go files; ooze was not started.")
		return nil
	}
	if err := tool.rejectPartiallyStaged(staged); err != nil {
		return err
	}

	trackedOutput, err := tool.git("ls-files", "-z", "--", "*.go")
	if err != nil {
		return fmt.Errorf("list tracked Go files: %w", err)
	}
	tracked := normalizeTrackedPaths(splitNUL(trackedOutput))
	ignore := buildIgnorePattern(tracked, staged)
	testCommand := buildTestCommand(staged)
	plan, err := tool.computeScope(staged)
	if err != nil {
		return err
	}

	fmt.Fprintln(tool.stdout, describeSelection(staged))
	fmt.Fprintf(tool.stdout, "  mutable files    : %s\n", strings.Join(staged, " "))
	fmt.Fprintf(tool.stdout, "  test command     : %s\n", testCommand)
	fmt.Fprintf(tool.stdout, "  excluded files   : %d\n", len(tracked)-len(staged))
	fmt.Fprintf(tool.stdout, "  line scope       : %s\n", describeScope(plan))
	fmt.Fprintf(tool.stdout, "  candidate mutants: %d (kept nodes %d, dropped nodes %d)\n",
		plan.stats.Candidates, plan.stats.Kept, plan.stats.Dropped)
	if dry {
		return nil
	}
	if plan.stats.Candidates == 0 {
		if plan.derived {
			return fmt.Errorf("go mutation: staged line scope matched no ooze mutation nodes; refusing a zero-execution run (kept %d, dropped %d)", plan.stats.Kept, plan.stats.Dropped)
		}
		return fmt.Errorf("go mutation: selected staged files contain no ooze mutation candidates; refusing a zero-execution run")
	}

	sandbox, cleanup, err := materializeIndex(tool.runner, root)
	if err != nil {
		return err
	}
	defer cleanup()
	return tool.runOoze(root, sandbox, ignore, testCommand, plan.encoded)
}

func (tool *tool) rejectPartiallyStaged(staged []string) error {
	for _, file := range staged {
		output, err := tool.git("diff", "--name-only", "-z", "--", file)
		if err != nil {
			return fmt.Errorf("check partial staging for %s: %w", file, err)
		}
		if len(output) > 0 {
			return fmt.Errorf("go mutation: partial staging is unsupported for %s; stage or discard its remaining worktree changes", file)
		}
	}
	return nil
}

func (tool *tool) computeScope(staged []string) (scopePlan, error) {
	allOffsets := []offsetRange{}
	plan := scopePlan{derived: true}
	for _, file := range staged {
		diff, err := tool.git("-c", "core.quotePath=false", "diff", "--cached", "--no-ext-diff", "--no-renames", "-U0", "--", file)
		if err != nil {
			return scopePlan{}, fmt.Errorf("read staged diff for %s: %w", file, err)
		}
		changed, parseErr := parseChangedLineRangesForFile(string(diff), file)
		if parseErr != nil {
			return tool.wholeFileScope(staged, "a staged diff could not be parsed; failing open to whole files")
		}
		content, err := tool.git("show", ":"+file)
		if err != nil {
			return scopePlan{}, fmt.Errorf("read staged content of %s: %w", file, err)
		}
		lines, found := changed[file]
		if !found {
			return tool.wholeFileScope(staged, "a staged file had no derivable diff range; failing open to whole files")
		}
		offsets := mergeOffsetRanges(lineRangesToOffsets(content, lines))
		if len(offsets) == 0 {
			return tool.wholeFileScope(staged, "a staged line range did not map to index bytes; failing open to whole files")
		}
		stats, err := mutation.AnalyzeSource(file, content, mutationRanges(offsets))
		if err != nil {
			return scopePlan{}, err
		}
		plan.stats = addStats(plan.stats, stats)
		allOffsets = append(allOffsets, offsets...)
	}
	plan.encoded = encodeOffsetRanges(mergeOffsetRanges(allOffsets))
	return plan, nil
}

func (tool *tool) wholeFileScope(staged []string, reason string) (scopePlan, error) {
	plan := scopePlan{reason: reason}
	for _, file := range staged {
		content, err := tool.git("show", ":"+file)
		if err != nil {
			return scopePlan{}, fmt.Errorf("read staged content of %s: %w", file, err)
		}
		stats, err := mutation.AnalyzeSource(file, content, nil)
		if err != nil {
			return scopePlan{}, err
		}
		plan.stats = addStats(plan.stats, stats)
	}
	return plan, nil
}

func mutationRanges(ranges []offsetRange) mutation.OffsetRanges {
	converted := make(mutation.OffsetRanges, 0, len(ranges))
	for _, span := range ranges {
		converted = append(converted, mutation.OffsetRange{Start: span.start, End: span.end})
	}
	return converted
}

func addStats(left, right mutation.ScopeStats) mutation.ScopeStats {
	return mutation.ScopeStats{
		Candidates: left.Candidates + right.Candidates,
		Kept:       left.Kept + right.Kept,
		Dropped:    left.Dropped + right.Dropped,
	}
}

func describeScope(plan scopePlan) string {
	if !plan.derived {
		return plan.reason
	}
	return fmt.Sprintf("%d staged byte range(s)", strings.Count(plan.encoded, ",")+1)
}

func (tool *tool) runOoze(root, sandbox, ignore, testCommand, scope string) error {
	threshold := os.Getenv(envThreshold)
	if threshold == "" {
		threshold = defaultThreshold
	}
	err := tool.runner.Run(commandSpec{
		Dir: root, Name: "go",
		Args:   []string{"test", "-v", "-tags=mutation", "-count=1", "-timeout=" + harnessTimeout, harnessPackage},
		Env:    append(os.Environ(), envIgnorePattern+"="+ignore, envTestCommand+"="+testCommand, envThreshold+"="+threshold, envRepositoryDir+"="+sandbox, envScope+"="+scope),
		Stdout: tool.stdout, Stderr: tool.stderr,
	})
	if err != nil {
		return fmt.Errorf("go mutation: ooze failed or mutation score was below %s: %w", threshold, err)
	}
	return nil
}

func (tool *tool) git(args ...string) ([]byte, error) {
	return tool.runner.Output(tool.cwd, "git", args...)
}

func splitNUL(output []byte) []string {
	trimmed := strings.TrimSuffix(string(output), "\x00")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\x00")
}
