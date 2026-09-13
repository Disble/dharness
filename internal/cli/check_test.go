package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/runner"
	"github.com/Disble/dharness/internal/tool"
)

// record captures what would have been invoked, so the gate's order and flags
// are asserted without spawning anything.
type record struct {
	commands []runner.Command
	fail     map[string]error
	emit     string

	// writeReport and reportPath mimic Stryker's own side effect: writing its
	// json report the moment a real mutation run actually executes, never on
	// a dry run. RunMutate deletes whatever report sat at that path before Stryker
	// runs, so a report seeded on disk before RunMutate is called (the old
	// writeReport-then-call pattern) would already be gone by the time
	// reportSurvivors opens the file — a test that wants RunMutate to judge
	// a report now has to make the stub produce it at run time.
	writeReport string
	reportPath  string
}

func (r *record) run(cmd runner.Command, stdout, _ io.Writer) error {
	r.commands = append(r.commands, cmd)
	if r.emit != "" {
		_, _ = io.WriteString(stdout, r.emit)
	}
	if r.writeReport != "" && cmd.Label == "stryker" && !slices.Contains(cmd.Args, "--dryRunOnly") {
		if err := writeMutationReport(r.reportPath, r.writeReport); err != nil {
			return err
		}
	}
	return r.fail[toolOf(cmd)]
}

// writeMutationReport creates the report file the same way Stryker's own
// json reporter would, directories included.
func writeMutationReport(path, contents string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(contents), 0o600)
}

// stubWritesReportOnRun arranges for the stub Stryker invocation to write
// contents to root's default mutation report the moment it actually runs a
// real mutation.
func stubWritesReportOnRun(captured *record, root, contents string) {
	captured.writeReport = contents
	captured.reportPath = filepath.Join(root, "reports", "mutation", "mutation.json")
}

func toolOf(cmd runner.Command) string { return cmd.String() }

// gitStub answers the three questions the commands ask git, which is what a
// conventional single-directory project looks like from the outside: the given
// root is the repository, its lockfile makes it the JS project too, and the
// staged list is whatever the test supplied.
//
// Tests write their staged paths newline-separated because that is readable;
// git's real -z output is NUL-separated, so the stub does that conversion in
// the one place rather than in every test.
func gitStub(root, staged string) func(string, ...string) ([]byte, error) {
	return func(_ string, args ...string) ([]byte, error) {
		switch {
		case len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel":
			return []byte(root + "\n"), nil
		case len(args) >= 1 && args[0] == "ls-files":
			return []byte("package-lock.json\x00"), nil
		case isStagedDiff(args):
			return stagedDiff(staged), nil
		default:
			return []byte(strings.ReplaceAll(staged, "\n", "\x00")), nil
		}
	}
}

// isStagedDiff recognises StagedDiff's own request rather than any diff, so
// the stub cannot answer a question the production code stopped asking. The
// leading -c is what separates it from the --name-only list request.
func isStagedDiff(args []string) bool {
	return len(args) >= 3 && args[0] == "-c" && args[2] == "diff" && !slices.Contains(args, "--name-only")
}

// stagedDiff renders the staged paths as the unified diff git would produce
// for them.
//
// The stub used to answer this with the NUL-separated file list, because it
// answered everything that way. That matters now: the gate feeds this to
// fallow on stdin as the audit's scope, and an empty or malformed diff is the
// one input that makes audit exit 0 over an unexamined change. A test double
// that returns the wrong shape here would agree with the suite and disagree
// with git.
func stagedDiff(staged string) []byte {
	var diff bytes.Buffer
	for path := range strings.SplitSeq(strings.TrimSpace(staged), "\n") {
		if path == "" {
			continue
		}
		fmt.Fprintf(&diff, "diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n@@ -1 +1 @@\n+export const staged = 1;\n",
			path, path, path, path)
	}
	return diff.Bytes()
}

func stub(t *testing.T, staged string) (*record, string) {
	t.Helper()

	root := t.TempDir()
	t.Cleanup(project.SetGitOutputForTest(gitStub(root, staged)))

	previous := workingDirectory
	workingDirectory = func() (string, error) { return root, nil }
	t.Cleanup(func() { workingDirectory = previous })

	captured := &record{fail: map[string]error{}}
	t.Cleanup(runner.SetForTest(captured.run))
	return captured, root
}

// mutable turns the stub root into a project Stryker can actually drive.
//
// The binary under node_modules/.bin is part of that now. dharness runs the
// copy the project installed, so a root without one is a project mutate
// refuses rather than a project it drives remotely.
func mutable(t *testing.T, root string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"devDependencies":{"vitest":"^4.0.0"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package-lock.json"), []byte(`{"lockfileVersion":3}`), 0o600); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(binDir, binaryName("stryker")), "")
}

// commandFor finds one captured invocation by label.
//
// Mutation runs two commands now — the install that keeps Stryker at @latest,
// then Stryker itself — and an index would pin that order rather than the
// behaviour each test is about.
func commandFor(t *testing.T, captured *record, label string) runner.Command {
	t.Helper()

	for _, command := range captured.commands {
		if command.Label == label {
			return command
		}
	}
	t.Fatalf("no %s command among %+v", label, captured.commands)
	return runner.Command{}
}

// strykerArgs is the flat argument string the mutation invocation carried.
func strykerArgs(t *testing.T, captured *record) string {
	t.Helper()
	return strings.Join(commandFor(t, captured, "stryker").Args, " ")
}

func TestCheckRunsReactDoctorBeforeFallow(t *testing.T) {
	captured, _ := stub(t, "src/a.ts\n")

	if err := RunCheck(nil, io.Discard); err != nil {
		t.Fatalf("RunCheck() = %v, want nil", err)
	}

	if len(captured.commands) != 3 {
		t.Fatalf("ran %d commands, want 3: %v", len(captured.commands), captured.commands)
	}
	if got := toolOf(captured.commands[0]); got != "react-doctor" {
		t.Errorf("first command = %q, want react-doctor", got)
	}
	// Both fallow stages follow: audit answers the staged-change question —
	// its base and its diff both come from the index — and dupes the
	// whole-repository one. react-doctor stays first because --staged scopes
	// it to the diff, so its cost tracks the change.
	//
	// Each is named after the subcommand it runs rather than after the binary.
	// Two stages called "fallow" made the gate print "fallow failed, so fallow
	// did not run", a sentence about an event that did not happen.
	for i, want := range map[int]string{1: fallowAuditStage, 2: fallowDupesStage} {
		if got := toolOf(captured.commands[i]); got != want {
			t.Errorf("command %d = %q, want %q", i, got, want)
		}
	}
}

func TestCheckRunsRemoteLatestEvenWhenWrappedToolsAreInstalled(t *testing.T) {
	captured, root := stub(t, "src/a.ts\n")
	binDir := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"react-doctor", "fallow"} {
		writeFile(t, filepath.Join(binDir, binaryName(name)), "")
	}

	if err := RunCheck(nil, io.Discard); err != nil {
		t.Fatalf("RunCheck() = %v", err)
	}

	want := []runner.Command{
		{Label: "react-doctor", Name: "npx", Args: append([]string{"--yes", "react-doctor@latest"}, tool.ReactDoctorStaged()...), Dir: root},
		{Label: fallowAuditStage, Name: "npx", Args: append([]string{"--yes", "fallow@latest"}, tool.FallowAudit()...), Dir: root},
		{Label: fallowDupesStage, Name: "npx", Args: []string{"--yes", "fallow@latest", "dupes"}, Dir: root},
	}
	if len(captured.commands) != len(want) {
		t.Fatalf("ran %d commands, want %d: %+v", len(captured.commands), len(want), captured.commands)
	}
	for i := range want {
		got := captured.commands[i]
		if got.Label != want[i].Label || got.Name != want[i].Name || got.Dir != want[i].Dir || !slices.Equal(got.Args, want[i].Args) {
			t.Errorf("command %d = %s %v in %s, want %s %v in %s", i, got.Name, got.Args, got.Dir, want[i].Name, want[i].Args, want[i].Dir)
		}
	}
}

func TestRemoteExecutionFailureDoesNotFallBackToTheProjectCopy(t *testing.T) {
	captured, root := stub(t, "src/a.ts\n")
	binDir := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(binDir, binaryName("react-doctor")), "")
	remoteErr := &runner.ExitError{Command: "react-doctor", Code: 1}
	captured.fail["react-doctor"] = remoteErr

	err := RunCheck(nil, io.Discard)

	if !errors.Is(err, remoteErr) {
		t.Fatalf("RunCheck() = %v, want the remote failure", err)
	}
	if len(captured.commands) != 1 {
		t.Fatalf("remote failure ran %d commands, want no local retry: %+v", len(captured.commands), captured.commands)
	}
	if captured.commands[0].Name != "npx" {
		t.Errorf("failed command used %q, want npx", captured.commands[0].Name)
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

// Stryker runs from the project's own node_modules, and the inverse of this
// test used to pin the opposite. Measured on 2026-08-13: Core unpacked into a
// bunx temporary directory imports typescript from its own location, finds
// nothing, and dies with ERR_MODULE_NOT_FOUND before the first mutant in any
// project holding a tsconfig.json.
func TestStrykerRunsTheBinaryTheProjectInstalled(t *testing.T) {
	captured, root := stub(t, "")
	binDir := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(binDir, binaryName("stryker"))
	writeFile(t, binary, "")

	p := project.Project{Root: root, Source: root, PackageManager: "npm"}
	if err := runStryker(binary, p, project.StrykerSelection{TestRunner: "vitest"}, []string{"run"}, io.Discard); err != nil {
		t.Fatalf("runStryker() = %v", err)
	}

	if len(captured.commands) != 1 {
		t.Fatalf("ran %d commands, want 1: %+v", len(captured.commands), captured.commands)
	}
	want := runner.Command{Label: "stryker", Name: binary, Args: []string{"run", "--appendPlugins", "@stryker-mutator/vitest-runner"}, Dir: root, LowPriority: true}
	if got := captured.commands[0]; got.Label != want.Label || got.Name != want.Name || got.Dir != want.Dir || got.LowPriority != want.LowPriority || !slices.Equal(got.Args, want.Args) {
		t.Errorf("runStryker command = %s %v in %s (low priority %t), want %s %v in %s (low priority %t)", got.Name, got.Args, got.Dir, got.LowPriority, want.Name, want.Args, want.Dir, want.LowPriority)
	}
}

// --incremental keeps the accumulated report and --force reruns this scope
// anyway. Dropping either one silently changes what the review means.
//
// Both paths land in one --mutate argument, joined by a comma: Stryker
// 9.6.1's own splitter ignores every value but the last on a repeated flag
// (stryker-cli.js:11-14,114), so two --mutate flags would silently mutate
// only src/b.ts.
func TestMutatePairsIncrementalWithForceAndBoundsConcurrency(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)

	_ = RunMutate([]string{"src/a.ts", "src/b.ts"}, io.Discard)

	args := strykerArgs(t, captured)
	for _, want := range []string{"--mutate src/a.ts,src/b.ts", "--incremental", "--force", "--concurrency 2"} {
		if !strings.Contains(args, want) {
			t.Errorf("stryker invoked without %q: %s", want, args)
		}
	}
	if strings.Count(args, "--mutate") != 1 {
		t.Errorf("stryker was given more than one --mutate flag, only the last of which it would honour: %s", args)
	}
}

// TestMutateFreshMeasuresOnlyWhatWasNamed pins --fresh against the defect that
// produced it.
//
// Measured on a bun/vitest fixture: with a warm incremental file, naming one
// file printed a table of three and reported `All files ... 4 survived` two
// lines above `Every mutant was caught`. The table is Stryker's own clear-text
// reporter over a cumulative report, and no Stryker option scopes it to
// --mutate — `clearTextReporter.skipFull` skips fully covered files, not
// out-of-scope ones. Running without the cache is what scopes it: the same
// invocation minus --incremental printed one row, two mutants, agreeing with
// the verdict.
//
// The incremental file is not passed either, so a --fresh run cannot rewrite
// the accumulated results it is choosing to ignore.
func TestMutateFreshMeasuresOnlyWhatWasNamed(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)

	_ = RunMutate([]string{"--fresh", "src/a.ts"}, io.Discard)

	args := strykerArgs(t, captured)
	if !strings.Contains(args, "--mutate src/a.ts") {
		t.Fatalf("stryker did not mutate the named path: %s", args)
	}
	if strings.Contains(args, "--incremental") {
		t.Errorf("--fresh still read the cache: %s", args)
	}
	if strings.Contains(args, "--incrementalFile") {
		t.Errorf("--fresh still named the cache file, so the run could rewrite it: %s", args)
	}
	if !strings.Contains(args, "--force") {
		t.Errorf("--fresh dropped --force, which is what reruns this scope: %s", args)
	}
}

func TestMutateDryRunMeasuresWithoutMutating(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)

	captured.emit = "16:52:00 INFO DryRunExecutor Initial test run succeeded. Ran 3 tests in 0 seconds.\n"

	if err := RunMutate([]string{"src/a.ts", "--dry-run"}, io.Discard); err != nil {
		t.Fatalf("RunMutate() = %v", err)
	}

	args := strykerArgs(t, captured)
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

// TestMutateNamesTheRowsItDidNotAskFor is the reader-facing half of the same
// defect. The table above dharness's verdict is Stryker's, printed from a
// cumulative report, and on a clean file with a warm cache it reported four
// survivors two lines above "Every mutant was caught". Both sentences were
// true of different things.
//
// dharness does not rewrite that table (§03). It states what the table is,
// names the files that are not this run's subject, and says where the results
// it is reading from live — a location dharness chose, under .git/, which is
// the last place anybody looks.
func TestMutateNamesTheRowsItDidNotAskFor(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)
	stubWritesReportOnRun(captured, root, `{"files":{
		"src/a.ts":{"mutants":[{"status":"Killed","mutatorName":"BooleanLiteral","location":{"start":{"line":1}}}]},
		"src/old.ts":{"mutants":[{"status":"Survived","mutatorName":"EqualityOperator","location":{"start":{"line":3}}}]}}}`)

	var out bytes.Buffer
	if err := RunMutate([]string{"src/a.ts"}, &out); err != nil {
		t.Fatalf("RunMutate() = %v; the named file has no survivors", err)
	}

	got := out.String()
	if !strings.Contains(got, "src/old.ts") {
		t.Errorf("the run did not name the file it never asked about:\n%s", got)
	}
	if !strings.Contains(got, "--fresh") {
		t.Errorf("the run did not name the flag that measures only what was named:\n%s", got)
	}
	if !strings.Contains(got, "stryker-incremental.json") {
		t.Errorf("the run did not say where the results it reused live:\n%s", got)
	}
	if strings.Contains(got, `\stryker-incremental.json`) {
		t.Errorf("the path was printed with backslashes, unlike every other path this tool prints:\n%s", got)
	}
}

// TestMutateSaysNothingWhenTheReportMatchesTheRun keeps that note off a run
// with nothing to explain — the common case, and the one that would turn the
// explanation into noise.
func TestMutateSaysNothingWhenTheReportMatchesTheRun(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)
	stubWritesReportOnRun(captured, root, `{"files":{"src/a.ts":{"mutants":[
		{"status":"Killed","mutatorName":"BooleanLiteral","location":{"start":{"line":1}}}]}}}`)

	var out bytes.Buffer
	if err := RunMutate([]string{"src/a.ts"}, &out); err != nil {
		t.Fatalf("RunMutate() = %v", err)
	}
	if strings.Contains(out.String(), "--fresh") {
		t.Errorf("explained a mismatch that did not happen:\n%s", out.String())
	}
}

// A timeout is a detection: the mutant hung the suite, which is what
// mutation testing exists to notice. NoCoverage is the opposite failure — no
// test ever ran the mutated line at all — and it counts the same as
// Survived.
func TestMutateFailsOnSurvivors(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)
	stubWritesReportOnRun(captured, root, `{"files":{"src/a.ts":{"mutants":[
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
	if len(survivors.Survivors) != 2 {
		t.Fatalf("reported %d survivors, want 2: %+v", len(survivors.Survivors), survivors.Survivors)
	}
	if !strings.Contains(out.String(), "src/a.ts:7 EqualityOperator") {
		t.Errorf("output does not locate the survivor:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "src/a.ts:11 StringLiteral (no test ran it)") {
		t.Errorf("output does not locate the mutant no test ever ran:\n%s", out.String())
	}
}

// TestMutateFailsWhenOnlyNoCoverageMutantsRemainInScope reproduces the
// defect measured against dharness 1.7.6 directly: a range with 11
// Killed and 8 NoCoverage mutants — an exported function no test calls —
// exited 0 and printed "Every mutant was caught: these tests notice this
// code breaking." Also measured: the same untested function reports
// Survived when no other in-scope mutant is hit and NoCoverage when one is,
// so the two statuses have to share one verdict rather than depend on what
// else happens to be in scope alongside it.
func TestMutateFailsWhenOnlyNoCoverageMutantsRemainInScope(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)

	var mutants []string
	for line := 1; line <= 11; line++ {
		mutants = append(mutants, fmt.Sprintf(`{"status":"Killed","mutatorName":"BooleanLiteral","location":{"start":{"line":%d}}}`, line))
	}
	for line := 12; line <= 19; line++ {
		mutants = append(mutants, fmt.Sprintf(`{"status":"NoCoverage","mutatorName":"StringLiteral","location":{"start":{"line":%d}}}`, line))
	}
	report := fmt.Sprintf(`{"files":{"src/a.ts":{"mutants":[%s]}}}`, strings.Join(mutants, ","))
	stubWritesReportOnRun(captured, root, report)

	var out bytes.Buffer
	err := RunMutate([]string{"src/a.ts"}, &out)

	var survivors *SurvivorsError
	if !errors.As(err, &survivors) {
		t.Fatalf("RunMutate() = %v, want SurvivorsError", err)
	}
	if len(survivors.Survivors) != 8 {
		t.Fatalf("reported %d survivors, want 8: %+v", len(survivors.Survivors), survivors.Survivors)
	}
	if strings.Contains(out.String(), "Every mutant was caught") {
		t.Errorf("output claims every mutant was caught while 8 were never even run:\n%s", out.String())
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

// TestMutateDoesNotJudgeAReportLeftByAnEarlierRun pins the removal. A run
// with allowEmpty and a scope no test imports exits 0 and writes no report,
// yet mutation.json still holds whatever the PREVIOUS run left there — and
// reading that file would judge this run by a verdict it never produced. The
// report has to be removed before Stryker runs, so a run that writes nothing
// leaves nothing to read.
func TestMutateDoesNotJudgeAReportLeftByAnEarlierRun(t *testing.T) {
	_, root := stub(t, "")
	mutable(t, root)
	// A leftover report from an earlier, unrelated run: every mutant Killed,
	// which would read as a clean pass if dharness judged it.
	writeReport(t, root, `{"files":{"src/old.ts":{"mutants":[
		{"status":"Killed","mutatorName":"BooleanLiteral","location":{"start":{"line":1}}}]}}}`)

	// The stub does not write anything when Stryker "runs" this time.

	if err := RunMutate([]string{"src/a.ts"}, io.Discard); err == nil {
		t.Fatal("RunMutate() = nil, want an error: a leftover report from an earlier run must not be read as this run's verdict")
	}
}

// TestMutateRemovesAStaleReportBeforeRunning proves the removal itself does
// not misfire when there is something on disk to remove: a stale report left
// at the default path must not block a run that goes on to write and judge
// its own fresh report.
func TestMutateRemovesAStaleReportBeforeRunning(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)
	writeReport(t, root, `{"files":{"src/old.ts":{"mutants":[
		{"status":"Survived","mutatorName":"BooleanLiteral","location":{"start":{"line":1}}}]}}}`)
	stubWritesReportOnRun(captured, root, `{"files":{"src/a.ts":{"mutants":[
		{"status":"Killed","mutatorName":"BooleanLiteral","location":{"start":{"line":1}}}]}}}`)

	if err := RunMutate([]string{"src/a.ts"}, io.Discard); err != nil {
		t.Fatalf("RunMutate() = %v, want nil: removing a stale report must not itself fail the run", err)
	}
}

// TestMutatePrintsIgnoredMutantsAndStillPasses pins the ignored listing.
// Measured: `// Stryker disable next-line all: reason` produces
// "status":"Ignored","statusReason":"reason" in the JSON report, and
// Stryker's own clear-text reporter never prints either one — an author who
// marked a mutant equivalent otherwise has no way to see dharness agrees.
func TestMutatePrintsIgnoredMutantsAndStillPasses(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)
	stubWritesReportOnRun(captured, root, `{"files":{"src/a.ts":{"mutants":[
		{"status":"Killed","mutatorName":"BooleanLiteral","location":{"start":{"line":1}}},
		{"status":"Ignored","mutatorName":"ArrayDeclaration","statusReason":"equivalent: order does not matter here","location":{"start":{"line":5}}}
	]}}}`)

	var out bytes.Buffer
	if err := RunMutate([]string{"src/a.ts"}, &out); err != nil {
		t.Fatalf("RunMutate() = %v; an ignored mutant does not fail the run", err)
	}

	got := out.String()
	if !strings.Contains(got, "src/a.ts:5 ArrayDeclaration — equivalent: order does not matter here") {
		t.Errorf("output does not name the ignored mutant and its reason:\n%s", got)
	}
}

// TestMutateIgnoredHeadingDoesNotClaimEquivalence pins the heading's claim:
// Stryker also emits Ignored with statusReason "Static mutant (and
// "ignoreStatic" was enabled)" (core mutant-test-planner.js:84-86), which is
// not an author marking anything equivalent. The heading says only that
// Stryker skipped it, and leaves the reason to explain why.
func TestMutateIgnoredHeadingDoesNotClaimEquivalence(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)
	stubWritesReportOnRun(captured, root, `{"files":{"src/a.ts":{"mutants":[
		{"status":"Killed","mutatorName":"BooleanLiteral","location":{"start":{"line":1}}},
		{"status":"Ignored","mutatorName":"ArrayDeclaration","statusReason":"Static mutant (and \"ignoreStatic\" was enabled)","location":{"start":{"line":5}}}
	]}}}`)

	var out bytes.Buffer
	if err := RunMutate([]string{"src/a.ts"}, &out); err != nil {
		t.Fatalf("RunMutate() = %v; an ignored mutant does not fail the run", err)
	}

	got := out.String()
	if !strings.Contains(got, "1 mutant(s) were skipped by Stryker:") {
		t.Errorf("output does not carry the accurate heading:\n%s", got)
	}
	if strings.Contains(got, "marked equivalent") {
		t.Errorf("a static-mutant skip is not an equivalence claim:\n%s", got)
	}
}

// TestMutateSaysNothingAboutIgnoredMutantsWhenThereAreNone keeps the note
// off the common case: a report with nothing Ignored in scope must not print
// a note claiming mutants were marked equivalent and skipped.
func TestMutateSaysNothingAboutIgnoredMutantsWhenThereAreNone(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)
	stubWritesReportOnRun(captured, root, `{"files":{"src/a.ts":{"mutants":[
		{"status":"Killed","mutatorName":"BooleanLiteral","location":{"start":{"line":1}}}
	]}}}`)

	var out bytes.Buffer
	if err := RunMutate([]string{"src/a.ts"}, &out); err != nil {
		t.Fatalf("RunMutate() = %v", err)
	}
	if strings.Contains(out.String(), "marked equivalent") {
		t.Errorf("explained an ignored mutant that does not exist:\n%s", out.String())
	}
}

// TestMutateFailureHintsAtTheDisableDirective pins the survivor hint: a
// survivor's own mutator name goes straight into the directive that would
// mark it equivalent, so nobody has to look up Stryker's own syntax by hand.
// Measured: next-line does not reach a dependency-array argument at all (the
// mutant still Survived), while the range form — disable before, restore
// after, both naming the mutator — does (Ignored), so the hint has to show that
// form rather than next-line.
func TestMutateFailureHintsAtTheDisableDirective(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)
	stubWritesReportOnRun(captured, root, `{"files":{"src/a.ts":{"mutants":[
		{"status":"Survived","mutatorName":"EqualityOperator","location":{"start":{"line":7}}}
	]}}}`)

	var out bytes.Buffer
	err := RunMutate([]string{"src/a.ts"}, &out)

	var survivors *SurvivorsError
	if !errors.As(err, &survivors) {
		t.Fatalf("RunMutate() = %v, want SurvivorsError", err)
	}

	got := out.String()
	if !strings.Contains(got, "// Stryker disable EqualityOperator:") {
		t.Errorf("output does not hint at the disable directive:\n%s", got)
	}
	if !strings.Contains(got, "// Stryker restore EqualityOperator") {
		t.Errorf("output does not hint at the restore line:\n%s", got)
	}
}

// TestMutateNoCoverageHintNamesTheMissingTestNotADisableDirective pins the
// NoCoverage hint: a NoCoverage mutant was never run by any test, so wrapping it in
// `// Stryker disable` cannot be the fix — the fix is a test that calls the
// line at all.
func TestMutateNoCoverageHintNamesTheMissingTestNotADisableDirective(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)
	stubWritesReportOnRun(captured, root, `{"files":{"src/a.ts":{"mutants":[
		{"status":"NoCoverage","mutatorName":"StringLiteral","location":{"start":{"line":11}}}
	]}}}`)

	var out bytes.Buffer
	err := RunMutate([]string{"src/a.ts"}, &out)

	var survivors *SurvivorsError
	if !errors.As(err, &survivors) {
		t.Fatalf("RunMutate() = %v, want SurvivorsError", err)
	}

	got := out.String()
	if strings.Contains(got, "Stryker disable") {
		t.Errorf("a NoCoverage mutant was never run, so the disable-directive hint does not apply:\n%s", got)
	}
	if !strings.Contains(got, "test") {
		t.Errorf("output does not name a missing test as the fix:\n%s", got)
	}
}

// TestMutateDisableHintOnlyUnderSurvivedEntries pins the mixed-shape half of
// the same fix: a report holding both a Survived and a NoCoverage mutant must
// print the disable-directive hint only under the entry a test actually ran
// and missed.
func TestMutateDisableHintOnlyUnderSurvivedEntries(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)
	stubWritesReportOnRun(captured, root, `{"files":{"src/a.ts":{"mutants":[
		{"status":"Survived","mutatorName":"EqualityOperator","location":{"start":{"line":7}}},
		{"status":"NoCoverage","mutatorName":"StringLiteral","location":{"start":{"line":11}}}
	]}}}`)

	var out bytes.Buffer
	err := RunMutate([]string{"src/a.ts"}, &out)

	var survivors *SurvivorsError
	if !errors.As(err, &survivors) {
		t.Fatalf("RunMutate() = %v, want SurvivorsError", err)
	}

	got := out.String()
	if strings.Count(got, "Stryker disable") != 1 {
		t.Errorf("want exactly one disable hint, for the Survived entry only:\n%s", got)
	}
	survivedIdx := strings.Index(got, "src/a.ts:7 EqualityOperator")
	hintIdx := strings.Index(got, "Stryker disable")
	noCoverageIdx := strings.Index(got, "src/a.ts:11 StringLiteral")
	if survivedIdx == -1 || hintIdx == -1 || noCoverageIdx == -1 || !(survivedIdx < hintIdx && hintIdx < noCoverageIdx) {
		t.Errorf("the disable hint is not positioned under the Survived entry:\n%s", got)
	}
}

// TestMutateHeadingIsAccurateForPureAndMixedShapes pins the survivor heading:
// "N mutant(s) survived" was measured wrong when every mutant was
// NoCoverage — none of them survived a test, they were never run under one.
func TestMutateHeadingIsAccurateForPureAndMixedShapes(t *testing.T) {
	cases := []struct {
		name   string
		report string
		want   string
		avoid  []string
	}{
		{
			name: "pure NoCoverage",
			report: `{"files":{"src/a.ts":{"mutants":[
				{"status":"NoCoverage","mutatorName":"StringLiteral","location":{"start":{"line":11}}},
				{"status":"NoCoverage","mutatorName":"StringLiteral","location":{"start":{"line":12}}}
			]}}}`,
			want:  "2 mutant(s) never ran under a test",
			avoid: []string{"survived"},
		},
		{
			name: "mixed",
			report: `{"files":{"src/a.ts":{"mutants":[
				{"status":"Survived","mutatorName":"EqualityOperator","location":{"start":{"line":7}}},
				{"status":"NoCoverage","mutatorName":"StringLiteral","location":{"start":{"line":11}}}
			]}}}`,
			want: "2 mutant(s) not caught: 1 survived, 1 never ran under a test",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			captured, root := stub(t, "")
			mutable(t, root)
			stubWritesReportOnRun(captured, root, tc.report)

			var out bytes.Buffer
			err := RunMutate([]string{"src/a.ts"}, &out)

			var survivors *SurvivorsError
			if !errors.As(err, &survivors) {
				t.Fatalf("RunMutate() = %v, want SurvivorsError", err)
			}

			got := out.String()
			if !strings.Contains(got, tc.want) {
				t.Errorf("heading %q not found in:\n%s", tc.want, got)
			}
			for _, avoid := range tc.avoid {
				if strings.Contains(strings.ToLower(got), avoid) {
					t.Errorf("output should not contain %q for this shape:\n%s", avoid, got)
				}
			}
		})
	}
}

// TestSurvivorsErrorMessageNamesWhichShapeFailed pins SurvivorsError.Error()
// directly: nothing else in this suite calls Error() rather than just
// asserting errors.As, so the three branches it switches on had no coverage
// of their own text.
func TestSurvivorsErrorMessageNamesWhichShapeFailed(t *testing.T) {
	cases := []struct {
		name      string
		survivors []tool.Survivor
		want      string
	}{
		{
			name:      "pure Survived",
			survivors: []tool.Survivor{{Status: "Survived"}, {Status: "Survived"}},
			want:      "2 mutant(s) survived: a test would not have noticed this code breaking",
		},
		{
			name:      "pure NoCoverage",
			survivors: []tool.Survivor{{Status: "NoCoverage"}, {Status: "NoCoverage"}, {Status: "NoCoverage"}},
			want:      "3 mutant(s) never ran under a test: nothing would have noticed this code breaking",
		},
		{
			name:      "mixed",
			survivors: []tool.Survivor{{Status: "Survived"}, {Status: "NoCoverage"}},
			want:      "2 mutant(s) not caught: 1 survived, 1 never ran under a test",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := &SurvivorsError{Survivors: tc.survivors}
			if got := err.Error(); got != tc.want {
				t.Errorf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestMutateHeadingExactTextForPureSurvived pins the pure-Survived shape of
// survivorsHeading with an exact match rather than a substring: a mutant that
// forces the "not caught: N survived, 0 never ran" branch instead of the
// plain "survived" one still contains the word "survived" anywhere in the
// output, so only an exact match on the heading line catches it.
func TestMutateHeadingExactTextForPureSurvived(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)
	stubWritesReportOnRun(captured, root, `{"files":{"src/a.ts":{"mutants":[
		{"status":"Survived","mutatorName":"EqualityOperator","location":{"start":{"line":7}}},
		{"status":"Survived","mutatorName":"BooleanLiteral","location":{"start":{"line":9}}}
	]}}}`)

	var out bytes.Buffer
	err := RunMutate([]string{"src/a.ts"}, &out)

	var survivors *SurvivorsError
	if !errors.As(err, &survivors) {
		t.Fatalf("RunMutate() = %v, want SurvivorsError", err)
	}
	if !strings.Contains(out.String(), "2 mutant(s) survived — a test would not have noticed:") {
		t.Errorf("exact heading text missing for a pure-Survived report:\n%s", out.String())
	}
}

// TestMutatePrintsEveryEntryEvenAfterANoCoverageOne pins the NoCoverage
// branch's `continue`: a report with a NoCoverage mutant sorted before a
// Survived one must still print the Survived entry and its disable hint. A
// `break` in that branch's place would look identical whenever the NoCoverage
// entry happens to be last, which every other test in this file has it be.
func TestMutatePrintsEveryEntryEvenAfterANoCoverageOne(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)
	stubWritesReportOnRun(captured, root, `{"files":{"src/a.ts":{"mutants":[
		{"status":"NoCoverage","mutatorName":"StringLiteral","location":{"start":{"line":5}}},
		{"status":"Survived","mutatorName":"EqualityOperator","location":{"start":{"line":10}}}
	]}}}`)

	var out bytes.Buffer
	err := RunMutate([]string{"src/a.ts"}, &out)

	var survivors *SurvivorsError
	if !errors.As(err, &survivors) {
		t.Fatalf("RunMutate() = %v, want SurvivorsError", err)
	}

	got := out.String()
	if !strings.Contains(got, "src/a.ts:10 EqualityOperator") {
		t.Errorf("printing stopped after the earlier NoCoverage entry instead of continuing to the next one:\n%s", got)
	}
	if !strings.Contains(got, "Stryker disable EqualityOperator") {
		t.Errorf("the Survived entry after a NoCoverage one lost its disable hint:\n%s", got)
	}
}

// TestMutateReadsTheReportWhereTheConfigSendsIt pins the configured path.
// --jsonReporter.fileName does not exist as a CLI flag, so a project that
// customised where Stryker's json reporter writes can only be respected by
// reading its own JSON config, exactly like testRunner already is.
func TestMutateReadsTheReportWhereTheConfigSendsIt(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)
	writeFile(t, filepath.Join(root, "stryker.config.json"), `{"testRunner":"vitest","jsonReporter":{"fileName":"custom/report.json"}}`)
	captured.writeReport = `{"files":{"src/a.ts":{"mutants":[
		{"status":"Survived","mutatorName":"EqualityOperator","location":{"start":{"line":7}}}]}}}`
	captured.reportPath = filepath.Join(root, "custom", "report.json")

	var out bytes.Buffer
	err := RunMutate([]string{"src/a.ts"}, &out)

	var survivors *SurvivorsError
	if !errors.As(err, &survivors) {
		t.Fatalf("RunMutate() = %v, want SurvivorsError read from the configured report path", err)
	}
	if !strings.Contains(out.String(), "src/a.ts:7 EqualityOperator") {
		t.Errorf("output does not locate the survivor from the configured report path:\n%s", out.String())
	}
}

// TestResolveReportPathNeverResolvesToSourceItself pins the default path
// directly. project leaves ReportPath empty rather than defaulting it to
// tool.MutationReportPath itself, so resolveReportPath is the only place left
// that applies the default — and filepath.Join(source, "") is Source itself,
// which the caller then passes to os.Remove.
func TestResolveReportPathNeverResolvesToSourceItself(t *testing.T) {
	source := filepath.Join("some", "project")

	got := resolveReportPath(source, "")

	if got == source {
		t.Fatalf("resolveReportPath(%q, \"\") = %q, must not equal Source itself", source, got)
	}
	if !strings.Contains(got, "mutation") {
		t.Errorf("resolveReportPath(%q, \"\") = %q, want the default mutation report path", source, got)
	}
}

// TestResolveReportPathUsesAnAbsoluteConfiguredPathAsIs pins the second half:
// filepath.Join does not special-case an absolute second argument, so an
// absolute jsonReporter.fileName has to be recognised and returned unchanged
// rather than nested under Source into a path neither side meant.
func TestResolveReportPathUsesAnAbsoluteConfiguredPathAsIs(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "custom-report.json")

	got := resolveReportPath(filepath.Join("some", "source"), abs)

	if got != abs {
		t.Errorf("resolveReportPath() = %q, want the absolute path unchanged: %q", got, abs)
	}
}

// TestMutateResolvesAnAbsoluteConfiguredReportPathAsIs exercises the same fix
// end to end: a project whose stryker config points jsonReporter.fileName at
// an absolute path must have its report read from exactly that path, not one
// nested under the project source.
func TestMutateResolvesAnAbsoluteConfiguredReportPathAsIs(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)
	elsewhere := filepath.Join(t.TempDir(), "report.json")
	writeFile(t, filepath.Join(root, "stryker.config.json"),
		fmt.Sprintf(`{"testRunner":"vitest","jsonReporter":{"fileName":%q}}`, filepath.ToSlash(elsewhere)))
	captured.writeReport = `{"files":{"src/a.ts":{"mutants":[
		{"status":"Survived","mutatorName":"EqualityOperator","location":{"start":{"line":7}}}]}}}`
	captured.reportPath = elsewhere

	var out bytes.Buffer
	err := RunMutate([]string{"src/a.ts"}, &out)

	var survivors *SurvivorsError
	if !errors.As(err, &survivors) {
		t.Fatalf("RunMutate() = %v, want SurvivorsError read from the absolute report path", err)
	}
	if !strings.Contains(out.String(), "src/a.ts:7 EqualityOperator") {
		t.Errorf("output does not locate the survivor from the absolute report path:\n%s", out.String())
	}
}

// declaresStryker makes the project name Stryker itself, pinned exactly, which
// is what a repository that cares about a reproducible verdict actually does.
func declaresStryker(t *testing.T, root string) {
	t.Helper()

	manifest := `{"devDependencies":{"vitest":"^4.0.0","@stryker-mutator/core":"9.6.1","@stryker-mutator/vitest-runner":"9.6.1"}}`
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
}

// §05: a declared version is a decision, and an exact pin is the most
// deliberate one there is. Measured against a real repository on 2026-08-13,
// installing at @latest rewrote "9.6.1" into "^9.6.1" — and in a mutation
// engine a minor arriving on its own moves the verdict over untouched code.
func TestMutateKeepsTheStrykerVersionTheProjectDeclared(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)
	declaresStryker(t, root)

	_ = RunMutate([]string{"src/a.ts"}, io.Discard)

	install := commandFor(t, captured, "npm")
	if !slices.Equal(install.Args, []string{"install"}) {
		t.Errorf("install = %v, want a bare install that names no version", install.Args)
	}
	for _, arg := range install.Args {
		if strings.Contains(arg, "@latest") {
			t.Errorf("dharness rewrote a version the project pinned: %v", install.Args)
		}
	}
}

// The escape hatch, and it says what it costs: the project's declared version
// is rewritten, on purpose, because someone asked.
func TestMutateUpgradeRewritesTheDeclaredVersionOnPurpose(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)
	declaresStryker(t, root)

	_ = RunMutate([]string{"src/a.ts", "--upgrade"}, io.Discard)

	install := strings.Join(commandFor(t, captured, "npm").Args, " ")
	for _, want := range []string{"--save-dev", "@stryker-mutator/core@latest", "@stryker-mutator/vitest-runner@latest"} {
		if !strings.Contains(install, want) {
			t.Errorf("--upgrade did not ask for %s: %s", want, install)
		}
	}
}

// Nothing declared is nothing to overwrite, so the newest is the right answer.
func TestMutateAddsStrykerAtLatestWhenTheProjectHasNone(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)

	_ = RunMutate([]string{"src/a.ts"}, io.Discard)

	install := strings.Join(commandFor(t, captured, "npm").Args, " ")
	if !strings.Contains(install, "@stryker-mutator/core@latest") {
		t.Errorf("an undeclared Stryker was not added at @latest: %s", install)
	}
}

// A project that cannot be given a Stryker is told so, and told what to run.
//
// The refusal exists because the alternative was measured and is worse: the
// transient executor route reaches Stryker but not the project's typescript, so
// falling back to it would answer a missing install with a Node stack trace
// naming a path inside a temporary directory.
func TestMutateRefusesWhenStrykerCannotBeInstalled(t *testing.T) {
	cases := map[string]func(t *testing.T, captured *record, root string){
		"the install fails": func(_ *testing.T, captured *record, _ string) {
			captured.fail["npm"] = &runner.ExitError{Command: "npm", Code: 1}
		},
		"no binary appears": func(t *testing.T, _ *record, root string) {
			if err := os.Remove(filepath.Join(root, "node_modules", ".bin", binaryName("stryker"))); err != nil {
				t.Fatal(err)
			}
		},
	}

	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			captured, root := stub(t, "")
			mutable(t, root)
			breakIt(t, captured, root)

			err := RunMutate([]string{"src/a.ts"}, io.Discard)

			var unavailable *StrykerUnavailableError
			if !errors.As(err, &unavailable) {
				t.Fatalf("RunMutate() = %v, want StrykerUnavailableError", err)
			}
			for _, evidence := range []string{
				"npm install --save-dev",
				"@stryker-mutator/core@latest",
				"@stryker-mutator/vitest-runner@latest",
				"nothing was mutated",
			} {
				if !strings.Contains(err.Error(), evidence) {
					t.Errorf("refusal omits %q: %s", evidence, err)
				}
			}
			if got := commandFor(t, captured, "npm"); len(got.Args) == 0 {
				t.Error("the install was never attempted")
			}
			for _, command := range captured.commands {
				if command.Label == "stryker" {
					t.Errorf("Stryker ran without being installable: %+v", command)
				}
			}
		})
	}
}

// Nobody writes their flags before their paths. The standard flag package stops
// parsing at the first positional, which would silently turn a flag into a path.
func TestMutateAcceptsFlagsAfterPaths(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)

	_ = RunMutate([]string{"src/a.ts", "--concurrency", "4", "src/b.ts"}, io.Discard)

	args := strykerArgs(t, captured)
	for _, want := range []string{"--mutate src/a.ts,src/b.ts", "--concurrency 4"} {
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

	args := strykerArgs(t, captured)
	if !strings.Contains(args, "--reporters clear-text,json") {
		t.Errorf("without the json reporter there is no verdict to read: %s", args)
	}
	if strings.Contains(args, "--break") {
		t.Errorf("--break does not exist in Stryker and was rejected when tried: %s", args)
	}
	if !commandFor(t, captured, "stryker").LowPriority {
		t.Error("mutation ran at normal priority; it is the one command that can freeze the machine")
	}
}

// A project that configured Stryker chose its thresholds and reporters on
// purpose; overruling them from a default would be dharness deciding something
// that is not its business.
func TestMutateUsesTheConfiguredRunnerWithoutOverridingIt(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)
	writeFile(t, filepath.Join(root, "stryker.config.json"), `{"testRunner":"jest","appendPlugins":["custom-plugin"]}`)

	// The stub does not write a mutation report; reaching that expected error
	// proves the command passed the runner-selection boundary.
	_ = RunMutate([]string{"src/a.ts"}, io.Discard)

	installed := strings.Join(commandFor(t, captured, "npm").Args, " ")
	if !strings.Contains(installed, "@stryker-mutator/jest-runner@latest") {
		t.Errorf("configured runner was not installed with Core: %s", installed)
	}

	args := strykerArgs(t, captured)
	if strings.Contains(args, "--testRunner") {
		t.Errorf("dharness overruled the project's own runner: %s", args)
	}
	if !strings.Contains(args, "--appendPlugins custom-plugin,@stryker-mutator/jest-runner") {
		t.Errorf("configured appendPlugins were not preserved: %s", args)
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
	// Both skipped stages are named individually. "fallow did not run" was
	// ambiguous while two stages shared that name, and whoever reads the gate
	// to decide what to run next cannot act on an ambiguous name.
	if !strings.Contains(text, fallowAuditStage+" and "+fallowDupesStage+" did not run") {
		t.Errorf("output does not name both skipped stages:\n%s", text)
	}
	if strings.Contains(text, "── fallow") {
		t.Errorf("output announces a tool that never ran:\n%s", text)
	}
}

// A failing stage names its successor, never itself.
//
// Both fallow stages were called "fallow", so an audit failure printed "fallow
// failed, so fallow did not run" — a sentence about an event that did not
// occur, aimed at exactly the reader who has to decide what to do next.
func TestAFailingStageNamesItsSuccessorNotItself(t *testing.T) {
	captured, _ := stub(t, "src/a.ts\n")
	captured.fail[fallowAuditStage] = &runner.ExitError{Command: fallowAuditStage, Code: 1}

	var out bytes.Buffer
	if err := RunCheck(nil, &out); err == nil {
		t.Fatal("RunCheck() = nil, want the audit failure")
	}

	text := out.String()
	want := fallowAuditStage + " failed, so " + fallowDupesStage + " did not run"
	if !strings.Contains(text, want) {
		t.Errorf("output does not contain %q:\n%s", want, text)
	}
	if strings.Contains(text, "fallow failed, so fallow did not run") {
		t.Errorf("a stage reported that it blocked itself:\n%s", text)
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
	writeFile(t, filepath.Join(binDir, binaryName("lefthook")), "")

	p := project.Describe(root)
	command := tool.Installed("lefthook", p.LocalBinary("lefthook"), root, "install")

	if command.String() != "lefthook" {
		t.Errorf("command reports %q, want lefthook", command.String())
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
	captured, root := stub(t, "src/a.ts\n")

	// Only the HEAD lookup fails: a repository with no commits still has a
	// toplevel and an index, and answering everything with an error would be
	// testing a repository that does not exist rather than one with no history.
	answer := gitStub(root, "src/a.ts\n")
	t.Cleanup(project.SetGitOutputForTest(func(dir string, args ...string) ([]byte, error) {
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--verify" {
			return nil, errors.New("fatal: Needed a single revision")
		}
		return answer(dir, args...)
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

// ESLint is the one wrapped tool this gate resolves locally rather than
// through the remote executor, and it measured cheapest of the four stages
// (docs/learning-log.md, 12 August 2026), so it runs first when installed.
func TestCheckRunsEslintFirstWhenInstalled(t *testing.T) {
	captured, root := stub(t, "src/a.ts\nsrc/b.tsx\n")
	binDir := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(binDir, binaryName("eslint")), "")

	if err := RunCheck(nil, io.Discard); err != nil {
		t.Fatalf("RunCheck() = %v", err)
	}

	if len(captured.commands) != 4 {
		t.Fatalf("ran %d commands, want 4 (eslint, react-doctor, fallow audit, fallow dupes): %+v", len(captured.commands), captured.commands)
	}

	first := captured.commands[0]
	if first.Label != "eslint" {
		t.Errorf("first command = %q, want eslint", first.Label)
	}
	if !strings.Contains(first.Name, "node_modules") {
		t.Errorf("eslint resolved through %q, want the locally installed copy, never the remote executor", first.Name)
	}
	if !slices.Equal(first.Args, []string{"--no-warn-ignored", "src/a.ts", "src/b.tsx"}) {
		t.Errorf("eslint args = %v, want the ignored-file flag then exactly the staged files", first.Args)
	}
	for _, arg := range first.Args {
		if arg == "--cache" {
			t.Error("eslint invoked with --cache, which this change explicitly rejects")
		}
	}

	if got := toolOf(captured.commands[1]); got != "react-doctor" {
		t.Errorf("second command = %q, want react-doctor", got)
	}
}

// A split layout stages paths relative to the repository root, but the
// stage runs with Dir: p.Source and ESLint takes explicit paths — so a
// project split from its repository still has to hand ESLint paths it can
// resolve from where it runs, not from the repository's own root.
func TestCheckRebasesEslintPathsInASplitLayout(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "frontend")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(source, "package-lock.json"), "{}")

	captured := &record{fail: map[string]error{}}
	t.Cleanup(runner.SetForTest(captured.run))
	t.Cleanup(project.SetGitOutputForTest(func(_ string, args ...string) ([]byte, error) {
		switch {
		case len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel":
			return []byte(root + "\n"), nil
		case len(args) >= 1 && args[0] == "ls-files":
			return []byte("frontend/package-lock.json\x00"), nil
		default:
			return []byte("frontend/src/a.ts\x00"), nil
		}
	}))
	previous := workingDirectory
	workingDirectory = func() (string, error) { return source, nil }
	t.Cleanup(func() { workingDirectory = previous })

	binDir := filepath.Join(source, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(binDir, binaryName("eslint")), "")

	if err := RunCheck(nil, io.Discard); err != nil {
		t.Fatalf("RunCheck() = %v", err)
	}

	if len(captured.commands) == 0 {
		t.Fatal("no commands ran")
	}
	first := captured.commands[0]
	if first.Label != "eslint" {
		t.Fatalf("first command = %q, want eslint: %+v", first.Label, captured.commands)
	}
	if !slices.Equal(first.Args, []string{"--no-warn-ignored", "src/a.ts"}) {
		t.Errorf("eslint args = %v, want [src/a.ts] rebased from frontend/src/a.ts", first.Args)
	}
	if first.Dir != source {
		t.Errorf("eslint Dir = %q, want %q", first.Dir, source)
	}
}

// Most projects have not installed ESLint yet. The gate still runs
// react-doctor and fallow, and it has to say why ESLint did not join them
// rather than leaving a silent gap — the same shape HasCommits already uses
// for a repository with no history.
func TestEslintStageIsSkippedWithoutABinary(t *testing.T) {
	captured, _ := stub(t, "src/a.ts\n")

	var out bytes.Buffer
	if err := RunCheck(nil, &out); err != nil {
		t.Fatalf("RunCheck() = %v", err)
	}

	for _, cmd := range captured.commands {
		if toolOf(cmd) == "eslint" {
			t.Fatalf("eslint ran without a local install: %+v", captured.commands)
		}
	}
	if !strings.Contains(out.String(), "eslint did not run") {
		t.Errorf("output does not say ESLint was skipped:\n%s", out.String())
	}
}

// ESLint runs first now, so its failure has to cut every stage behind it —
// the same short-circuit react-doctor's own failure already proved.
func TestEslintFailureStopsBeforeReactDoctor(t *testing.T) {
	captured, root := stub(t, "src/a.ts\n")
	binDir := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(binDir, binaryName("eslint")), "")
	captured.fail["eslint"] = &runner.ExitError{Command: "eslint", Code: 1}

	err := RunCheck(nil, io.Discard)

	var exitErr *runner.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("RunCheck() = %v, want ExitError", err)
	}
	if len(captured.commands) != 1 {
		t.Errorf("ran %d commands after eslint failed, want 1: %+v", len(captured.commands), captured.commands)
	}
}

// The duplication ceiling dharness writes is only a ceiling if something
// enforces it, and audit does not: measured against fallow 3.14.0, a
// repository at 80% duplication with `threshold: 3` set passes `audit` with
// exit 0 and fails `dupes` with exit 1. audit's verdict is scoped to what the
// changeset introduces; the percentage is a whole-repository question and
// `dupes` is the command that asks it.
//
// So the gate runs it as its own stage. Without this, dharness would ship a
// number into every project's config that reads like a gate and gates
// nothing — the failure this repository spends most of its rules avoiding.
func TestCheckEnforcesTheDuplicationCeiling(t *testing.T) {
	captured, _ := stub(t, "src/a.ts\n")

	if err := RunCheck(nil, io.Discard); err != nil {
		t.Fatalf("RunCheck() = %v, want nil", err)
	}

	if len(captured.commands) != 3 {
		t.Fatalf("ran %d commands, want 3: %v", len(captured.commands), captured.commands)
	}

	last := captured.commands[2]
	if got := toolOf(last); got != fallowDupesStage {
		t.Errorf("third command = %q, want %q", got, fallowDupesStage)
	}
	if !slices.Contains(last.Args, "dupes") {
		t.Errorf("the third stage is not dupes: %+v", last)
	}

	// No --threshold on the command line: the value belongs in the config
	// dharness owns, where the project can see it, argue with it and override
	// it. A flag here would be a number nobody can find.
	for _, arg := range last.Args {
		if strings.HasPrefix(arg, "--threshold") {
			t.Errorf("dupes carries a --threshold flag; the ceiling belongs in the config, not the invocation: %+v", last)
		}
	}
}

// The audit stage carries the staged diff on stdin, and it has to be the diff
// git produced rather than anything reconstructed here.
//
// Measured against fallow 3.16.0, this is the flag that keeps the gate on the
// index: with --changed-since HEAD alone, an audit reported a finding in a
// file that was only in the working tree and had never been staged.
func TestCheckFeedsFallowTheStagedDiffOnStdin(t *testing.T) {
	captured, _ := stub(t, "src/a.ts\n")

	if err := RunCheck(nil, io.Discard); err != nil {
		t.Fatalf("RunCheck() = %v, want nil", err)
	}

	audit := captured.commands[1]
	if !slices.Contains(audit.Args, "audit") {
		t.Fatalf("second command is not the audit stage: %+v", audit)
	}
	if audit.Stdin == nil {
		t.Fatal("the audit stage carries no stdin; --diff-stdin would read end-of-file and admit nothing, which exits 0 over an unaudited change")
	}

	fed, err := io.ReadAll(audit.Stdin)
	if err != nil {
		t.Fatalf("reading the audit's stdin = %v", err)
	}
	if !bytes.Equal(fed, stagedDiff("src/a.ts\n")) {
		t.Errorf("the audit was fed %q, want the staged diff %q", fed, stagedDiff("src/a.ts\n"))
	}
}

// dupes is the whole-repository question and must not be handed the staged
// diff, or the ceiling would only ever measure the change.
func TestCheckLeavesDupesUnscoped(t *testing.T) {
	captured, _ := stub(t, "src/a.ts\n")

	if err := RunCheck(nil, io.Discard); err != nil {
		t.Fatalf("RunCheck() = %v, want nil", err)
	}

	dupes := captured.commands[2]
	if !slices.Contains(dupes.Args, "dupes") {
		t.Fatalf("third command is not the dupes stage: %+v", dupes)
	}
	if dupes.Stdin != nil {
		t.Error("the dupes stage carries stdin; the duplication ceiling is a whole-repository wall, not a scoped question")
	}
}

// An empty diff is the one input that turns audit green without auditing
// anything: measured against fallow 3.16.0, an empty --diff-file over a
// genuinely bad staged file exited 0, while the same file with the real diff
// exited 1. The gate refuses rather than reports a pass it did not earn.
func TestCheckRefusesAnEmptyStagedDiff(t *testing.T) {
	captured, root := stub(t, "src/a.ts\n")
	t.Cleanup(project.SetGitOutputForTest(func(dir string, args ...string) ([]byte, error) {
		if isStagedDiff(args) {
			return nil, nil
		}
		return gitStub(root, "src/a.ts\n")(dir, args...)
	}))

	err := RunCheck(nil, io.Discard)

	if err == nil {
		t.Fatal("RunCheck() = nil for an empty staged diff, want a refusal: an empty scope passes without auditing anything")
	}
	if !strings.Contains(err.Error(), "empty diff") {
		t.Errorf("RunCheck() = %v, want an error naming the empty diff", err)
	}
	for _, command := range captured.commands {
		if slices.Contains(command.Args, "audit") {
			t.Error("the audit stage ran with an empty scope; it would exit 0 over an unaudited change")
		}
	}
}

// TestCheckSaysWhyTheWaitIsNotAnalysis states, in the gate's own output, the
// cost of a decision the gate makes on the user's behalf.
//
// Measured on a five-file create-next-app: a commit touching one file takes
// ~23s, of which the tools' own self-reported analysis is ~4.6s. The rest is
// the package manager resolving react-doctor@latest and fallow@latest against
// the registry, three separate times, on every commit. That resolution is
// deliberate (§03) — the gate runs the current release and pins nothing — but
// a person watching a 23-second `git commit` with no explanation reads it as
// the analysis being slow, and reaches for --no-verify.
//
// It prints before the first stage, because the whole point is to be readable
// while the wait is happening rather than after it.
func TestCheckSaysWhyTheWaitIsNotAnalysis(t *testing.T) {
	var out bytes.Buffer
	stub(t, "src/a.ts\n")

	if err := RunCheck(nil, &out); err != nil {
		t.Fatalf("RunCheck() = %v", err)
	}

	got := out.String()
	flat := normalizeSpace(got)
	if !strings.Contains(flat, "@latest") {
		t.Errorf("the gate does not name the resolution it pays for:\n%s", got)
	}
	if !strings.Contains(flat, "registry") {
		t.Errorf("the gate does not say where the wait goes:\n%s", got)
	}
	if latest, first := strings.Index(got, "@latest"), strings.Index(got, "──"); latest > first {
		t.Errorf("the note prints after the first stage began, too late to read while waiting:\n%s", got)
	}
}

// TestCheckSaysNothingWhenThereIsNothingToCheck keeps the note off the fast
// path: a docs-only commit exits in 0.17s and is not taxed, and a line
// explaining a wait that did not happen is the noise this gate already has
// too much of.
func TestCheckSaysNothingWhenThereIsNothingToCheck(t *testing.T) {
	var out bytes.Buffer
	stub(t, "")

	if err := RunCheck(nil, &out); err != nil {
		t.Fatalf("RunCheck() = %v", err)
	}

	if strings.Contains(out.String(), "@latest") {
		t.Errorf("the gate explained a wait it never had:\n%s", out.String())
	}
}

// TestGateSeparatesAConfigErrorFromLintFindings pins the distinction the
// gate's own output could not make: ESLint exit 2 is a config that never
// loaded, so no staged file was linted and the fix is not in the change;
// exit 1 is the gate working. The note reads the exit code and nothing else.
func TestGateSeparatesAConfigErrorFromLintFindings(t *testing.T) {
	// Built the way the gate builds it, not by hand: the first version of
	// this test constructed a stage the product never produces, and passed
	// against a note the gate could not print.
	root := t.TempDir()
	eslint := localStage(project.Project{Root: root, Source: root}, tool.ESLint, filepath.Join(root, "node_modules", ".bin", "eslint"))

	configError := eslintConfigErrorNote(eslint, &runner.ExitError{Command: tool.ESLint, Code: 2})
	if !strings.Contains(configError, "configuration error") {
		t.Errorf("eslintConfigErrorNote() on exit 2 = %q, want it to name a configuration error", configError)
	}
	if !strings.Contains(configError, "dharness sync") {
		t.Errorf("eslintConfigErrorNote() on exit 2 = %q, want it to name the command that checks the same thing", configError)
	}

	if findings := eslintConfigErrorNote(eslint, &runner.ExitError{Command: tool.ESLint, Code: 1}); findings != "" {
		t.Errorf("eslintConfigErrorNote() on exit 1 = %q, want nothing: that is the gate working", findings)
	}

	other := stage{command: runner.Command{Name: tool.Fallow}}
	if note := eslintConfigErrorNote(other, &runner.ExitError{Command: tool.Fallow, Code: 2}); note != "" {
		t.Errorf("eslintConfigErrorNote() = %q for a stage that is not ESLint, want nothing", note)
	}
}
