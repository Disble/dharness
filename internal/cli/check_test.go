package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/runner"
)

// record captures what would have been invoked, so the gate's order and flags
// are asserted without spawning anything.
type record struct {
	commands []runner.Command
	fail     map[string]error
	emit     string
}

func (r *record) run(cmd runner.Command, stdout, _ io.Writer) error {
	r.commands = append(r.commands, cmd)
	if r.emit != "" {
		_, _ = io.WriteString(stdout, r.emit)
	}
	return r.fail[toolOf(cmd)]
}

func toolOf(cmd runner.Command) string { return cmd.String() }

func stub(t *testing.T, staged string) (*record, string) {
	t.Helper()

	root := t.TempDir()
	t.Cleanup(project.SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		return []byte(staged), nil
	}))

	previous := workingDirectory
	workingDirectory = func() (string, error) { return root, nil }
	t.Cleanup(func() { workingDirectory = previous })

	captured := &record{fail: map[string]error{}}
	t.Cleanup(runner.SetForTest(captured.run))
	return captured, root
}

// mutable turns the stub root into a project Stryker can actually drive:
// vitest declared, and its runner plugin present in node_modules.
func mutable(t *testing.T, root string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"devDependencies":{"vitest":"^4.0.0"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "node_modules", "@stryker-mutator", "vitest-runner"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestCheckRunsReactDoctorBeforeFallow(t *testing.T) {
	captured, _ := stub(t, "src/a.ts\n")

	if err := RunCheck(nil, io.Discard); err != nil {
		t.Fatalf("RunCheck() = %v, want nil", err)
	}

	if len(captured.commands) != 2 {
		t.Fatalf("ran %d commands, want 2: %v", len(captured.commands), captured.commands)
	}
	if got := toolOf(captured.commands[0]); got != "react-doctor" {
		t.Errorf("first command = %q, want react-doctor", got)
	}
	if got := toolOf(captured.commands[1]); got != "fallow" {
		t.Errorf("second command = %q, want fallow", got)
	}
}

// The cheap tool failing has to skip the expensive one; that short circuit is
// most of what makes a gate that runs on every commit affordable.
func TestCheckStopsAtTheFirstFailure(t *testing.T) {
	captured, _ := stub(t, "src/a.ts\n")
	captured.fail["react-doctor"] = &runner.ExitError{Command: "react-doctor", Code: 1}

	err := RunCheck(nil, io.Discard)

	var exitErr *runner.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("RunCheck() = %v, want ExitError", err)
	}
	if len(captured.commands) != 1 {
		t.Errorf("ran %d commands after a failure, want 1", len(captured.commands))
	}
}

// Most commits touch nothing these tools can read. Spawning them anyway would
// pay the whole startup cost to be told there was nothing to do.
func TestCheckSkipsEverythingWhenNoSourceFileIsStaged(t *testing.T) {
	captured, _ := stub(t, "README.md\npackage-lock.json\n")

	var out bytes.Buffer
	if err := RunCheck(nil, &out); err != nil {
		t.Fatalf("RunCheck() = %v, want nil", err)
	}

	if len(captured.commands) != 0 {
		t.Errorf("ran %v, want no commands", captured.commands)
	}
	if !strings.Contains(out.String(), "nothing to check") {
		t.Errorf("output did not explain the skip: %q", out.String())
	}
}

// These flags are the product: every one of them reaches the network or turns
// the exit code into something other than the verdict.
func TestCheckKeepsReactDoctorOffTheNetworkAndBlockingOnErrors(t *testing.T) {
	captured, _ := stub(t, "src/a.ts\n")

	if err := RunCheck(nil, io.Discard); err != nil {
		t.Fatalf("RunCheck() = %v", err)
	}

	args := strings.Join(captured.commands[0].Args, " ")
	for _, flag := range []string{"--staged", "--no-dead-code", "--no-score", "--no-supply-chain"} {
		if !strings.Contains(args, flag) {
			t.Errorf("react-doctor invoked without %q: %s", flag, args)
		}
	}
}

func TestCheckFailsWithoutAGitIndex(t *testing.T) {
	t.Cleanup(project.SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		return nil, os.ErrNotExist
	}))
	previous := workingDirectory
	workingDirectory = func() (string, error) { return t.TempDir(), nil }
	t.Cleanup(func() { workingDirectory = previous })

	if err := RunCheck(nil, io.Discard); err == nil {
		t.Fatal("RunCheck() = nil, want an error when the index cannot be read")
	}
}

func TestMutateWithoutPathsExplainsWhatItNeeds(t *testing.T) {
	_, _ = stub(t, "")

	err := RunMutate(nil, io.Discard)

	if !errors.Is(err, ErrNoMutatePaths) {
		t.Fatalf("RunMutate() = %v, want ErrNoMutatePaths", err)
	}
}

// --incremental keeps the accumulated report and --force reruns this scope
// anyway. Dropping either one silently changes what the review means.
func TestMutatePairsIncrementalWithForceAndBoundsConcurrency(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)

	_ = RunMutate([]string{"src/a.ts", "src/b.ts"}, io.Discard)

	args := strings.Join(captured.commands[0].Args, " ")
	for _, want := range []string{"--mutate src/a.ts", "--mutate src/b.ts", "--incremental", "--force", "--concurrency 2"} {
		if !strings.Contains(args, want) {
			t.Errorf("stryker invoked without %q: %s", want, args)
		}
	}
}

func TestMutateDryRunMeasuresWithoutMutating(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)

	captured.emit = "16:52:00 INFO DryRunExecutor Initial test run succeeded. Ran 3 tests in 0 seconds.\n"

	if err := RunMutate([]string{"src/a.ts", "--dry-run"}, io.Discard); err != nil {
		t.Fatalf("RunMutate() = %v", err)
	}

	args := strings.Join(captured.commands[0].Args, " ")
	if !strings.Contains(args, "--dryRunOnly") {
		t.Errorf("--dry-run did not reach Stryker: %s", args)
	}
	if strings.Contains(args, "--force") {
		t.Errorf("--dry-run must not mutate: %s", args)
	}

	// Without the measurement recorded, sync has no terminal state and asks for
	// it on every run, forever.
	measured := project.Describe(root).ReadEvidence().ScopedMutation
	if measured == nil {
		t.Fatal("the measurement was not recorded")
	}
	if measured.RelatedTests != 3 || measured.MeasuredPath != "src/a.ts" {
		t.Errorf("recorded %+v, want 3 tests for src/a.ts", measured)
	}
}

func writeReport(t *testing.T, root, contents string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "reports", "mutation", "mutation.json"), contents)
}

// Stryker prints surviving mutants and exits 0. If dharness reads the report
// and stays quiet too, the command reports success on tests that would not
// notice the code breaking.
func TestMutateFailsOnSurvivors(t *testing.T) {
	_, root := stub(t, "")
	mutable(t, root)
	writeReport(t, root, `{"files":{"src/a.ts":{"mutants":[
		{"status":"Killed","mutatorName":"BooleanLiteral","location":{"start":{"line":1}}},
		{"status":"Survived","mutatorName":"EqualityOperator","location":{"start":{"line":7}}},
		{"status":"Timeout","mutatorName":"ArithmeticOperator","location":{"start":{"line":9}}},
		{"status":"NoCoverage","mutatorName":"StringLiteral","location":{"start":{"line":11}}}
	]}}}`)

	var out bytes.Buffer
	err := RunMutate([]string{"src/a.ts"}, &out)

	var survivors *SurvivorsError
	if !errors.As(err, &survivors) {
		t.Fatalf("RunMutate() = %v, want SurvivorsError", err)
	}
	// A timeout is a detection, and an uncovered mutant is a coverage gap.
	// Neither is a test that failed to notice a change it did observe.
	if len(survivors.Survivors) != 1 {
		t.Fatalf("reported %d survivors, want 1: %+v", len(survivors.Survivors), survivors.Survivors)
	}
	if !strings.Contains(out.String(), "src/a.ts:7 EqualityOperator") {
		t.Errorf("output does not locate the survivor:\n%s", out.String())
	}
}

// A run that wrote no report measured nothing, and silence is not a pass.
func TestMutateFailsWhenThereIsNoReportToRead(t *testing.T) {
	_, root := stub(t, "")
	mutable(t, root)

	if err := RunMutate([]string{"src/a.ts"}, io.Discard); err == nil {
		t.Fatal("RunMutate() = nil with no report; a missing verdict is not a pass")
	}
}

// Nobody writes their flags before their paths. The standard flag package stops
// parsing at the first positional, which would silently turn a flag into a path.
func TestMutateAcceptsFlagsAfterPaths(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)

	_ = RunMutate([]string{"src/a.ts", "--concurrency", "4", "src/b.ts"}, io.Discard)

	args := strings.Join(captured.commands[0].Args, " ")
	for _, want := range []string{"--mutate src/a.ts", "--mutate src/b.ts", "--concurrency 4"} {
		if !strings.Contains(args, want) {
			t.Errorf("stryker invoked without %q: %s", want, args)
		}
	}
	if strings.Contains(args, "--mutate --concurrency") {
		t.Errorf("a flag was consumed as a path: %s", args)
	}
}

// Stryker exposes no way to fail on survivors from the command line: --break
// and --thresholds.break were both rejected as unknown options. The json
// reporter is what makes a verdict possible at all, and it runs at low
// priority because it is the one thing here that can saturate a machine.
func TestMutateAsksForTheReportItNeedsToJudge(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)

	_ = RunMutate([]string{"src/a.ts"}, io.Discard)

	args := strings.Join(captured.commands[0].Args, " ")
	if !strings.Contains(args, "--reporters clear-text,json") {
		t.Errorf("without the json reporter there is no verdict to read: %s", args)
	}
	if strings.Contains(args, "--break") {
		t.Errorf("--break does not exist in Stryker and was rejected when tried: %s", args)
	}
	if !captured.commands[0].LowPriority {
		t.Error("mutation ran at normal priority; it is the one command that can freeze the machine")
	}
}

// A project that configured Stryker chose its thresholds and reporters on
// purpose; overruling them from a default would be dharness deciding something
// that is not its business.
func TestMutateDefersTheRunnerToAProjectThatConfiguredStryker(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)
	writeFile(t, filepath.Join(root, "stryker.config.mjs"), "export default {}")

	_ = RunMutate([]string{"src/a.ts"}, io.Discard)

	args := strings.Join(captured.commands[0].Args, " ")
	if strings.Contains(args, "--testRunner") {
		t.Errorf("dharness overruled the project's own runner: %s", args)
	}
	if !strings.Contains(args, "--incremental") {
		t.Errorf("dharness dropped what it does own: %s", args)
	}
}

// Whoever reads the gate output — a person, or the model that ran the commit —
// has to be able to tell which tool produced what, and has to learn that the
// run stopped early rather than that everything else passed.
func TestCheckAttributesOutputAndSaysWhatItSkipped(t *testing.T) {
	captured, _ := stub(t, "src/a.ts\n")
	captured.fail["react-doctor"] = &runner.ExitError{Command: "react-doctor", Code: 1}

	var out bytes.Buffer
	if err := RunCheck(nil, &out); err == nil {
		t.Fatal("RunCheck() = nil, want the tool failure")
	}

	text := out.String()
	if !strings.Contains(text, "── react-doctor ──") {
		t.Errorf("output does not say which tool ran:\n%s", text)
	}
	if !strings.Contains(text, "fallow did not run") {
		t.Errorf("output does not say the gate stopped early:\n%s", text)
	}
	if strings.Contains(text, "── fallow ──") {
		t.Errorf("output announces a tool that never ran:\n%s", text)
	}
}

// A locally installed tool resolves to an absolute path. Reporting that path as
// the thing that failed makes the reader work out which tool it was.
func TestFailureNamesTheToolNotItsResolvedPath(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(binDir, binaryName("fallow")), "")

	command := project.Describe(root).Resolve("fallow").Command(root, "audit")

	if command.String() != "fallow" {
		t.Errorf("command reports %q, want fallow", command.String())
	}
	if !strings.Contains(command.Name, "node_modules") {
		t.Errorf("command would not execute the installed copy: %q", command.Name)
	}
}

// dharness wraps adoption, configuration and the gates. Every other question a
// failure raises belongs to the tool that raised it, so the failure output has
// to hand the reader a way in — in the form this project actually runs.
func TestFailureHandsOffToTheToolsOwnHelp(t *testing.T) {
	captured, root := stub(t, "src/a.ts\n")
	writeFile(t, filepath.Join(root, "bun.lock"), "")
	captured.fail["react-doctor"] = &runner.ExitError{Command: "react-doctor", Code: 1}

	var out bytes.Buffer
	if err := RunCheck(nil, &out); err == nil {
		t.Fatal("RunCheck() = nil, want the tool failure")
	}

	text := out.String()
	if !strings.Contains(text, "bunx react-doctor@latest --help") {
		t.Errorf("failure does not hand off to the tool, in this project's own form:\n%s", text)
	}
	if strings.Contains(text, "npx ") {
		t.Errorf("failure points a bun project at npx:\n%s", text)
	}
}

// A repository with no commits still runs react-doctor, which only needs the
// index, and skips fallow, which needs something to compare against. Failing
// the first commit on a tool error would land exactly one step after adoption.
func TestCheckSkipsFallowUntilThereIsHistory(t *testing.T) {
	captured, _ := stub(t, "src/a.ts\n")

	t.Cleanup(project.SetGitOutputForTest(func(_ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "rev-parse" {
			return nil, errors.New("fatal: Needed a single revision")
		}
		return []byte("src/a.ts\n"), nil
	}))

	var out bytes.Buffer
	if err := RunCheck(nil, &out); err != nil {
		t.Fatalf("RunCheck() = %v, want nil", err)
	}

	if len(captured.commands) != 1 {
		t.Fatalf("ran %d commands, want only react-doctor: %v", len(captured.commands), captured.commands)
	}
	if got := toolOf(captured.commands[0]); got != "react-doctor" {
		t.Errorf("ran %q, want react-doctor", got)
	}
	if !strings.Contains(out.String(), "no commits yet") {
		t.Errorf("the gate skipped fallow without saying why:\n%s", out.String())
	}
}
