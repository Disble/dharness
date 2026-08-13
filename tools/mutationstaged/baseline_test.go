package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// recordedRunner delegates process output to the real runner so Git still
// materializes a sandbox, and intercepts executions so no ooze run is paid for.
type recordedRunner struct {
	inner processRunner
	runs  []commandSpec
	fail  func(commandSpec) bool
}

func (runner *recordedRunner) Output(dir, name string, args ...string) ([]byte, error) {
	return runner.inner.Output(dir, name, args...)
}

func (runner *recordedRunner) Run(spec commandSpec) error {
	runner.runs = append(runner.runs, spec)
	if runner.fail != nil && runner.fail(spec) {
		return fmt.Errorf("simulated non-zero exit")
	}
	return nil
}

func commandLine(spec commandSpec) string {
	return strings.Join(append([]string{spec.Name}, spec.Args...), " ")
}

func environmentValue(env []string, key string) string {
	for _, entry := range env {
		if value, found := strings.CutPrefix(entry, key+"="); found {
			return value
		}
	}
	return ""
}

func stagedCalcFixture(t *testing.T) string {
	t.Helper()
	root := newGitFixture(t)
	writeFixtureFile(t, root, "internal/calc/calc.go", "package calc\nfunc Add(a, b int) int { return a - b }\n")
	gitFixture(t, root, "add", "internal/calc/calc.go")
	return root
}

func TestRunRefusesToScoreWhenTheBaselineSuiteFails(t *testing.T) {
	root := stagedCalcFixture(t)
	runner := &recordedRunner{inner: osProcessRunner{}, fail: func(commandSpec) bool { return true }}
	tool := &tool{cwd: root, runner: runner, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}

	err := tool.run(false)
	if err == nil || !strings.Contains(err.Error(), "unmutated code") {
		t.Fatalf("run error = %v, want a refusal naming the red baseline", err)
	}
	if len(runner.runs) != 1 {
		t.Fatalf("executed %d process(es), want only the baseline: a red suite kills every mutant, so ooze must never be released", len(runner.runs))
	}
}

func TestBaselineRunsOozesOwnCommandInTheSandboxOozeWillMutate(t *testing.T) {
	root := stagedCalcFixture(t)
	runner := &recordedRunner{inner: osProcessRunner{}}
	tool := &tool{cwd: root, runner: runner, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}

	if err := tool.run(false); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(runner.runs) != 2 {
		t.Fatalf("executed %d process(es), want the baseline and then ooze", len(runner.runs))
	}

	baseline, ooze := runner.runs[0], runner.runs[1]
	want := buildTestCommand([]string{"internal/calc/calc.go"})
	if got := commandLine(baseline); got != want {
		t.Fatalf("baseline command = %q, want ooze's own test command %q", got, want)
	}
	if sandbox := environmentValue(ooze.Env, envRepositoryDir); baseline.Dir != sandbox {
		t.Fatalf("baseline ran in %q, want the sandbox ooze mutates (%q)", baseline.Dir, sandbox)
	}
}

func TestPassingBaselineIsReportedAsEvidence(t *testing.T) {
	root := stagedCalcFixture(t)
	var stdout bytes.Buffer
	tool := &tool{cwd: root, runner: &recordedRunner{inner: osProcessRunner{}}, stdout: &stdout, stderr: &bytes.Buffer{}}

	if err := tool.run(false); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "baseline suite") {
		t.Fatalf("output does not record the baseline check:\n%s", stdout.String())
	}
}

func TestDryRunDoesNotPayForABaseline(t *testing.T) {
	root := stagedCalcFixture(t)
	runner := &recordedRunner{inner: osProcessRunner{}, fail: func(commandSpec) bool { return true }}
	tool := &tool{cwd: root, runner: runner, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}

	if err := tool.run(true); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("dry run executed %d process(es), want none: -dry exists to cost nothing", len(runner.runs))
	}
}
