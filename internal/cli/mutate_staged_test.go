package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/runner"
	"github.com/Disble/dharness/internal/staged"
	"github.com/Disble/dharness/internal/tool"
)

// The tests in this file drive a real repository, the same way
// internal/staged's own real_repo_test.go does: --staged's whole job is
// reading git's own staged diff and materialising git's own index, and no
// synthetic stub can honestly answer for either.

var ambientGitPinningForMutateStaged = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR",
	"GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_PREFIX",
}

// isolateFromAmbientGitForMutateStaged clears the pinning this process may
// have inherited — this suite can run inside dharness's own `git commit` or
// `git rebase --exec`, both of which export GIT_DIR and GIT_INDEX_FILE
// naming dharness itself — and restores it when the test ends.
func isolateFromAmbientGitForMutateStaged(t *testing.T) {
	t.Helper()
	for _, name := range ambientGitPinningForMutateStaged {
		inherited, ok := os.LookupEnv(name)
		if !ok {
			continue
		}
		t.Setenv(name, inherited)
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
}

func gitRunStaged(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{
		"-c", "user.name=dharness test",
		"-c", "user.email=test@dharness.invalid",
		"-c", "commit.gpgsign=false",
	}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
}

// newStagedRepo builds a real, isolated repository whose JS project is the
// repository root itself, with an already-committed package.json/lockfile
// and a stryker shim under node_modules/.bin — the minimum a project needs
// for mutate --staged to find a local Stryker to run. It does not install a
// runner fake: each test wires the exact one it needs.
func newStagedRepo(t *testing.T) (root string, captured *record) {
	t.Helper()
	isolateFromAmbientGitForMutateStaged(t)

	root = t.TempDir()
	gitRunStaged(t, root, "init", "--quiet", "--initial-branch=main", ".")
	writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	writeFile(t, filepath.Join(root, "package-lock.json"), `{"lockfileVersion":3}`)
	gitRunStaged(t, root, "add", "-A")
	gitRunStaged(t, root, "commit", "--quiet", "-m", "init")

	binDir := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(binDir, binaryName("stryker")), "")

	previous := workingDirectory
	workingDirectory = func() (string, error) { return root, nil }
	t.Cleanup(func() { workingDirectory = previous })

	return root, &record{fail: map[string]error{}}
}

// stageFile writes contents at path (relative to root) and stages it.
func stageFile(t *testing.T, root, path, contents string) {
	t.Helper()
	writeFile(t, filepath.Join(root, path), contents)
	gitRunStaged(t, root, "add", "--", path)
}

// stagedRunner replaces runner.Run for a --staged test. It writes tsc's own
// compiled output for every path named in tscOutputs, mirroring the real
// classifier's --outDir/--rootDir contract, and — when report is non-empty —
// writes Stryker's own json report the moment a real mutation run executes,
// at exactly the path the generated config's own jsonReporter.fileName names.
//
// cmd.Dir is the snapshot's own copy of source for a staged run, which is
// only known once Snapshot() has actually materialised it. The fileName
// itself is read from the generated config rather than assumed, the same way
// the real Stryker would resolve it, because --staged always overrides
// jsonReporter.fileName to a per-run absolute path (see
// writeStagedStrykerConfig) — a stub that wrote to a fixed relative path
// under cmd.Dir would no longer be exercising what dharness actually reads.
func stagedRunner(captured *record, tscOutputs map[string]string, report string) func(runner.Command, io.Writer, io.Writer) error {
	return func(cmd runner.Command, stdout, stderr io.Writer) error {
		if cmd.Label == "tsc" {
			outDir := flagValueCLI(cmd.Args, "--outDir")
			for rel, content := range tscOutputs {
				if findArgWithSuffix(cmd.Args, filepath.FromSlash(rel)) == "" {
					continue
				}
				out := filepath.Join(outDir, strings.TrimSuffix(rel, ".ts")+".js")
				if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(out, []byte(content), 0o644); err != nil {
					return err
				}
			}
		}
		if report != "" && cmd.Label == "stryker" && !argsContain(cmd.Args, "--dryRunOnly") {
			fileName, err := jsonReporterFileNameIn(cmd.Dir)
			if err != nil {
				return err
			}
			if err := writeMutationReport(fileName, report); err != nil {
				return err
			}
		}
		return captured.run(cmd, stdout, stderr)
	}
}

// jsonReporterFileNameIn reads the jsonReporter.fileName a staged run's
// generated config names, from the config file sitting in dir — the same
// file Stryker itself would read to decide where to write its report.
func jsonReporterFileNameIn(dir string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(dir, stagedStrykerConfig))
	if err != nil {
		return "", err
	}
	var config struct {
		JSONReporter struct {
			FileName string `json:"fileName"`
		} `json:"jsonReporter"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return "", err
	}
	return config.JSONReporter.FileName, nil
}

func flagValueCLI(args []string, name string) string {
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// findArgWithSuffix returns the first argument ending in suffix (with a path
// separator immediately before it, so "src/a.ts" cannot match
// "src/other_a.ts"), or "".
func findArgWithSuffix(args []string, suffix string) string {
	needle := string(filepath.Separator) + suffix
	for _, arg := range args {
		if strings.HasSuffix(arg, needle) {
			return arg
		}
	}
	return ""
}

// TestMutateStagedRefusesAPositionalPath pins the refusal of a named path:
// --staged mutates exactly what the staged change justifies, so a named
// path is a contradiction rather than a narrowing.
func TestMutateStagedRefusesAPositionalPath(t *testing.T) {
	root, captured := newStagedRepo(t)
	t.Cleanup(runner.SetForTest(captured.run))
	stageFile(t, root, "src/a.js", "export const a = 1;\n")

	err := RunMutate([]string{"--staged", "src/a.js"}, io.Discard)

	if err == nil {
		t.Fatal("RunMutate() = nil, want a refusal")
	}
	if len(captured.commands) != 0 {
		t.Errorf("commands = %+v, want none: the refusal must fire before anything runs", captured.commands)
	}
}

// TestMutateStagedRefusesDryRun pins the refusal of --dry-run: it measures
// the initial test count and writes no report; --staged always needs a real
// report to self-check the classifier against.
func TestMutateStagedRefusesDryRun(t *testing.T) {
	root, captured := newStagedRepo(t)
	t.Cleanup(runner.SetForTest(captured.run))
	stageFile(t, root, "src/a.js", "export const a = 1;\n")

	err := RunMutate([]string{"--staged", "--dry-run"}, io.Discard)

	if err == nil {
		t.Fatal("RunMutate() = nil, want a refusal")
	}
	if len(captured.commands) != 0 {
		t.Errorf("commands = %+v, want none", captured.commands)
	}
}

// TestMutateStagedRefusesUpgrade pins the refusal of --upgrade: --staged never
// installs Stryker, at gate time or otherwise, so --upgrade has nothing to
// act on.
func TestMutateStagedRefusesUpgrade(t *testing.T) {
	root, captured := newStagedRepo(t)
	t.Cleanup(runner.SetForTest(captured.run))
	stageFile(t, root, "src/a.js", "export const a = 1;\n")

	err := RunMutate([]string{"--staged", "--upgrade"}, io.Discard)

	if err == nil {
		t.Fatal("RunMutate() = nil, want a refusal")
	}
	if len(captured.commands) != 0 {
		t.Errorf("commands = %+v, want none", captured.commands)
	}
}

// TestMutateStagedAcceptsFreshAsANoOp pins the one flag that is not refused:
// --fresh is accepted, and --staged never reads or writes an incremental
// file regardless, so it changes nothing.
func TestMutateStagedAcceptsFreshAsANoOp(t *testing.T) {
	root, captured := newStagedRepo(t)
	stageFile(t, root, "src/a.js", "export const a = 1;\n")
	t.Cleanup(runner.SetForTest(stagedRunner(captured, nil, minimalStagedReport("src/a.js"))))

	if err := RunMutate([]string{"--staged", "--fresh"}, io.Discard); err != nil {
		t.Fatalf("RunMutate() = %v, want nil", err)
	}
}

// TestMutateStagedWithNothingStagedRunsNoCommandAtAll pins the early exit:
// no staged scope means no snapshot, no classifier, no guard and no
// Stryker — dharness says so and stops.
func TestMutateStagedWithNothingStagedRunsNoCommandAtAll(t *testing.T) {
	_, captured := newStagedRepo(t)
	t.Cleanup(runner.SetForTest(captured.run))

	var out strings.Builder
	if err := RunMutate([]string{"--staged"}, &out); err != nil {
		t.Fatalf("RunMutate() = %v, want nil", err)
	}
	if !strings.Contains(out.String(), "nothing staged to mutate") {
		t.Errorf("output = %q, want it to say nothing is staged", out.String())
	}
	if len(captured.commands) != 0 {
		t.Errorf("commands = %+v, want none", captured.commands)
	}
}

// TestMutateStagedRefusesAMissingLocalStryker pins the refusal to install:
// --staged never installs, so a project without a local Stryker is
// refused with the same StrykerUnavailableError the ordinary path raises
// when installation itself fails, naming the install command.
func TestMutateStagedRefusesAMissingLocalStryker(t *testing.T) {
	root, captured := newStagedRepo(t)
	if err := os.Remove(filepath.Join(root, "node_modules", ".bin", binaryName("stryker"))); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runner.SetForTest(captured.run))
	stageFile(t, root, "src/a.js", "export const a = 1;\n")

	err := RunMutate([]string{"--staged"}, io.Discard)

	var unavailable *StrykerUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("RunMutate() = %v, want StrykerUnavailableError", err)
	}
	if len(captured.commands) != 0 {
		t.Errorf("commands = %+v, want none: --staged must never install", captured.commands)
	}
}

// minimalStagedReport is the smallest Stryker JSON report shape
// tool.Survivors can read, naming path with no mutants at all — "every
// mutant was caught" is the easiest verdict to arrange for tests that are
// not about the verdict itself.
func minimalStagedReport(path string) string {
	return `{"schemaVersion":"1.0","files":{"` + path + `":{"language":"typescript","mutants":[]}}}`
}

// TestMutateStagedSkipsStrykerWhenEveryCandidateIsTypesOnly pins an early exit: a
// scope made entirely of files the compiler erases to nothing never reaches
// Stryker at all — the cheapest way to run a check nothing needs is not to
// run it (design-principles.md §13).
func TestMutateStagedSkipsStrykerWhenEveryCandidateIsTypesOnly(t *testing.T) {
	root, captured := newStagedRepo(t)
	writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName("tsc")), "")
	stageFile(t, root, "src/types.ts", "export type A = string;\n")
	t.Cleanup(runner.SetForTest(stagedRunner(captured, map[string]string{"src/types.ts": ""}, "")))

	var out strings.Builder
	if err := RunMutate([]string{"--staged"}, &out); err != nil {
		t.Fatalf("RunMutate() = %v, want nil", err)
	}
	if !strings.Contains(out.String(), "types-only") || !strings.Contains(out.String(), "src/types.ts") {
		t.Errorf("output = %q, want it to name the dropped file", out.String())
	}
	if commandExists(captured, "stryker") {
		t.Errorf("commands = %+v, want no stryker command: nothing survived classification", captured.commands)
	}
}

func commandExists(captured *record, label string) bool {
	for _, cmd := range captured.commands {
		if cmd.Label == label {
			return true
		}
	}
	return false
}

// TestMutateStagedRefusesWhenTheVitestGuardFails pins the guard: a suite that
// cannot load must refuse before Stryker ever runs, exactly as it would for
// a missing Stryker.
func TestMutateStagedRefusesWhenTheVitestGuardFails(t *testing.T) {
	root, captured := newStagedRepo(t)
	writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName("vitest")), "")
	stageFile(t, root, "src/a.js", "export const a = 1;\n")
	captured.fail["vitest"] = &runner.ExitError{Command: "vitest", Code: 1}
	t.Cleanup(runner.SetForTest(captured.run))

	err := RunMutate([]string{"--staged"}, io.Discard)

	var suite *staged.VitestSuiteError
	if !errors.As(err, &suite) {
		t.Fatalf("RunMutate() = %v, want VitestSuiteError", err)
	}
	if commandExists(captured, "stryker") {
		t.Errorf("commands = %+v, want no stryker command: the guard must refuse first", captured.commands)
	}
}

// TestMutateStagedDetectsAClassifierDisagreement pins the self-check: a file
// the classifier believed compiled to nothing, but that Stryker's own report shows
// carrying at least one mutant anyway, is a classifier defect on the exact
// bytes this run mutated, and must fail loudly rather than silently pass.
func TestMutateStagedDetectsAClassifierDisagreement(t *testing.T) {
	root, captured := newStagedRepo(t)
	writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName("tsc")), "")
	stageFile(t, root, "src/a.js", "export const a = 1;\n")
	stageFile(t, root, "src/types.ts", "export type A = string;\n")

	// The report disagrees with the classifier: src/types.ts, believed
	// empty, shows up carrying a killed mutant anyway.
	report := `{"schemaVersion":"1.0","files":{` +
		`"src/a.js":{"language":"javascript","mutants":[]},` +
		`"src/types.ts":{"language":"typescript","mutants":[{"id":"1","mutatorName":"BooleanLiteral","status":"Killed","location":{"start":{"line":1,"column":1},"end":{"line":1,"column":2}}}]}` +
		`}}`
	t.Cleanup(runner.SetForTest(stagedRunner(captured, map[string]string{"src/types.ts": ""}, report)))

	err := RunMutate([]string{"--staged"}, io.Discard)

	var disagreement *ClassifierDisagreementError
	if !errors.As(err, &disagreement) {
		t.Fatalf("RunMutate() = %v, want ClassifierDisagreementError", err)
	}
	if len(disagreement.Files) != 1 || disagreement.Files[0] != "src/types.ts" {
		t.Errorf("ClassifierDisagreementError.Files = %v, want [src/types.ts]", disagreement.Files)
	}
}

// TestMutateStagedFailsOnAnInScopeSurvivor pins the verdict itself: a
// survivor in a kept file fails the run, the same as the ordinary path.
func TestMutateStagedFailsOnAnInScopeSurvivor(t *testing.T) {
	root, captured := newStagedRepo(t)
	stageFile(t, root, "src/a.js", "export const a = 1;\n")
	report := `{"schemaVersion":"1.0","files":{"src/a.js":{"language":"javascript","mutants":[` +
		`{"id":"1","mutatorName":"BooleanLiteral","status":"Survived","location":{"start":{"line":1,"column":1},"end":{"line":1,"column":2}}}` +
		`]}}}`
	t.Cleanup(runner.SetForTest(stagedRunner(captured, nil, report)))

	err := RunMutate([]string{"--staged"}, io.Discard)

	var survivors *SurvivorsError
	if !errors.As(err, &survivors) {
		t.Fatalf("RunMutate() = %v, want SurvivorsError", err)
	}
}

// TestMutateStagedPassesWhenEveryMutantIsCaught is the other side of the
// verdict: a clean report must not fail the run.
func TestMutateStagedPassesWhenEveryMutantIsCaught(t *testing.T) {
	root, captured := newStagedRepo(t)
	stageFile(t, root, "src/a.js", "export const a = 1;\n")
	t.Cleanup(runner.SetForTest(stagedRunner(captured, nil, minimalStagedReport("src/a.js"))))

	var out strings.Builder
	if err := RunMutate([]string{"--staged"}, &out); err != nil {
		t.Fatalf("RunMutate() = %v, want nil", err)
	}
	if strings.Contains(out.String(), "skipped by Stryker") {
		t.Errorf("output = %q, want no ignored-mutant heading: the report names none", out.String())
	}
}

// TestMutateStagedSaysSoWhenTheStagedRangesGeneratedNoMutant pins the no-op
// that looked like a pass. A kept file can legitimately carry no mutant — an
// enum, a side-effect import — and so can a range Stryker matched nothing in,
// and both used to print "Every mutant was caught" over nothing at all. It
// still exits 0; it no longer claims anything was caught.
func TestMutateStagedSaysSoWhenTheStagedRangesGeneratedNoMutant(t *testing.T) {
	root, captured := newStagedRepo(t)
	stageFile(t, root, "src/a.js", "export const a = 1;\n")
	t.Cleanup(runner.SetForTest(stagedRunner(captured, nil, minimalStagedReport("src/a.js"))))

	var out strings.Builder
	if err := RunMutate([]string{"--staged"}, &out); err != nil {
		t.Fatalf("RunMutate() = %v, want nil", err)
	}
	if strings.Contains(out.String(), "Every mutant was caught") {
		t.Errorf("output = %q, want no claim that anything was caught: no mutant was generated", out.String())
	}
	for _, want := range []string{
		"1 file(s), 1 range(s), 0 in-scope mutant(s): 0 killed, 0 survived, 0 no coverage, 0 timeout, 0 errors, 0 ignored",
		"no mutants were generated in the staged ranges",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output = %q, want it to contain %q", out.String(), want)
		}
	}
}

// TestMutateStagedSaysSoWhenEveryMutantWasIgnored pins the case Total()==0
// does not catch: a range whose only mutants are Ignored — measured on a
// real consumer at 110 files, each `export const valueNNN = 'vNNN';`, all
// static and therefore Ignored under the project's own `ignoreStatic: true`
// — has 110 in-scope mutants, not zero, so it never reached the "no mutants
// were generated" branch. It also has zero survivors, which used to be read
// as a pass: dharness printed "Every mutant was caught" over a report where
// nothing was ever run under a test at all. It still exits 0; it no longer
// claims anything was caught.
func TestMutateStagedSaysSoWhenEveryMutantWasIgnored(t *testing.T) {
	root, captured := newStagedRepo(t)
	stageFile(t, root, "src/a.js", "export const a = 1;\n")
	report := `{"schemaVersion":"1.0","files":{"src/a.js":{"language":"javascript","mutants":[` +
		`{"id":"1","mutatorName":"StringLiteral","status":"Ignored","statusReason":"Static mutant (and \"ignoreStatic\" was enabled)","location":{"start":{"line":1,"column":1},"end":{"line":1,"column":2}}}]}}}`
	t.Cleanup(runner.SetForTest(stagedRunner(captured, nil, report)))

	var out strings.Builder
	if err := RunMutate([]string{"--staged"}, &out); err != nil {
		t.Fatalf("RunMutate() = %v, want nil", err)
	}
	if strings.Contains(out.String(), "Every mutant was caught") {
		t.Errorf("output = %q, want no claim that anything was caught: nothing was ever tested", out.String())
	}
	if !strings.Contains(out.String(), "no mutant was tested: every in-scope mutant was skipped by Stryker (see the reasons above)") {
		t.Errorf("output = %q, want the skipped-only verdict", out.String())
	}
}

// TestMutateStagedPrintsTheMeasuredCountsBesideEveryVerdict pins the counts
// line on both sides of the verdict: counted inside the kept ranges only, and
// printed whether the run passes or fails.
func TestMutateStagedPrintsTheMeasuredCountsBesideEveryVerdict(t *testing.T) {
	mutants := func(statuses ...string) string {
		var entries []string
		for i, status := range statuses {
			entries = append(entries, `{"id":"`+strconv.Itoa(i)+`","mutatorName":"BooleanLiteral","status":"`+status+`","location":{"start":{"line":1,"column":1},"end":{"line":1,"column":2}}}`)
		}
		return strings.Join(entries, ",")
	}
	cases := []struct {
		name     string
		statuses []string
		counts   string
		verdict  string
		fails    bool
	}{
		{"a pass", []string{"Killed", "Killed", "Timeout"}, "2 file(s), 2 range(s), 3 in-scope mutant(s): 2 killed, 0 survived, 0 no coverage, 1 timeout, 0 errors, 0 ignored", "Every mutant was caught", false},
		{"a pass alongside an ignored mutant", []string{"Killed", "Ignored"}, "2 file(s), 2 range(s), 2 in-scope mutant(s): 1 killed, 0 survived, 0 no coverage, 0 timeout, 0 errors, 1 ignored", "Every mutant was caught", false},
		{"a pass with equal killed and timeout counts", []string{"Killed", "Timeout"}, "2 file(s), 2 range(s), 2 in-scope mutant(s): 1 killed, 0 survived, 0 no coverage, 1 timeout, 0 errors, 0 ignored", "Every mutant was caught", false},
		{"a failure", []string{"Killed", "Survived", "NoCoverage", "Ignored", "RuntimeError"}, "2 file(s), 2 range(s), 5 in-scope mutant(s): 1 killed, 1 survived, 1 no coverage, 0 timeout, 1 errors, 1 ignored", "not caught", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, captured := newStagedRepo(t)
			stageFile(t, root, "src/a.js", "export const a = 1;\n")
			stageFile(t, root, "src/b.js", "export const b = 1;\n")
			report := `{"schemaVersion":"1.0","files":{` +
				`"src/a.js":{"language":"javascript","mutants":[` + mutants(tc.statuses...) + `]},` +
				`"src/b.js":{"language":"javascript","mutants":[]}}}`
			t.Cleanup(runner.SetForTest(stagedRunner(captured, nil, report)))

			var out strings.Builder
			err := RunMutate([]string{"--staged"}, &out)

			if (err != nil) != tc.fails {
				t.Fatalf("RunMutate() = %v, want failure %v", err, tc.fails)
			}
			if !strings.Contains(out.String(), tc.counts) || !strings.Contains(out.String(), tc.verdict) {
				t.Errorf("output = %q, want %q and %q", out.String(), tc.counts, tc.verdict)
			}
		})
	}
}

// TestMutateStagedReadsTheStrykerConfigItRunsAgainst pins where the config is
// read from. Stryker runs in the snapshot, so it reads the snapshot's copy of
// stryker.config.json — the one being committed. An unstaged edit to the
// working tree's copy moves nothing Stryker does, and must move nothing
// dharness does either: here it carries a key only the committed config set.
//
// jsonReporter.fileName is deliberately not the signal here any more — see
// TestMutateStagedJudgesOnlyItsOwnPerRunReportPath and
// TestMutateStagedOverridesJSONReporterFileNameButKeepsOtherKeys for that:
// --staged always overrides it to its own per-run path, committed or not.
func TestMutateStagedReadsTheStrykerConfigItRunsAgainst(t *testing.T) {
	root, captured := newStagedRepo(t)
	writeFile(t, filepath.Join(root, "stryker.config.json"), `{"testRunner":"vitest","pinTag":"committed"}`)
	gitRunStaged(t, root, "add", "stryker.config.json")
	gitRunStaged(t, root, "commit", "--quiet", "-m", "stryker config")
	writeFile(t, filepath.Join(root, "stryker.config.json"), `{"testRunner":"vitest","pinTag":"unstaged"}`)
	stageFile(t, root, "src/a.js", "export const a = 1;\n")

	var generated string
	t.Cleanup(runner.SetForTest(generatedConfigAtRun(captured, nil, minimalStagedReport("src/a.js"), &generated)))

	var out strings.Builder
	if err := RunMutate([]string{"--staged"}, &out); err != nil {
		t.Fatalf("RunMutate() = %v, want nil", err)
	}
	if !strings.Contains(generated, `"pinTag":"committed"`) {
		t.Errorf("generated config = %q, want the committed pinTag carried through", generated)
	}
	if strings.Contains(generated, "unstaged") {
		t.Errorf("generated config = %q, want the unstaged edit ignored", generated)
	}
}

// TestMutateStagedAcceptsConcurrencyOfOne pins the exact boundary: 1 is the
// smallest valid worker count and must proceed rather than being refused.
func TestMutateStagedAcceptsConcurrencyOfOne(t *testing.T) {
	root, captured := newStagedRepo(t)
	stageFile(t, root, "src/a.js", "export const a = 1;\n")
	t.Cleanup(runner.SetForTest(stagedRunner(captured, nil, minimalStagedReport("src/a.js"))))

	if err := RunMutate([]string{"--staged", "--concurrency", "1"}, io.Discard); err != nil {
		t.Fatalf("RunMutate() = %v, want nil", err)
	}
}

// TestMutateStagedRefusesZeroConcurrency pins the other side of the same
// boundary: 0 is not a positive number of workers and must be refused
// before anything runs.
func TestMutateStagedRefusesZeroConcurrency(t *testing.T) {
	root, captured := newStagedRepo(t)
	t.Cleanup(runner.SetForTest(captured.run))
	stageFile(t, root, "src/a.js", "export const a = 1;\n")

	err := RunMutate([]string{"--staged", "--concurrency", "0"}, io.Discard)

	if err == nil {
		t.Fatal("RunMutate() = nil, want a refusal")
	}
	if len(captured.commands) != 0 {
		t.Errorf("commands = %+v, want none", captured.commands)
	}
}

// TestMutateStagedKeepsEverythingAndPrintsANoticeWhenTheClassifierCannotRun
// pins the "unavailable" half of the classifier's contract, threaded through
// --staged: no local tsc at all means nothing gets dropped, the run still
// reaches Stryker, and the notice explaining why is relayed to the reader.
func TestMutateStagedKeepsEverythingAndPrintsANoticeWhenTheClassifierCannotRun(t *testing.T) {
	root, captured := newStagedRepo(t)
	// Deliberately no node_modules/.bin/tsc.
	stageFile(t, root, "src/a.ts", "export const a = 1;\n")
	t.Cleanup(runner.SetForTest(stagedRunner(captured, nil, minimalStagedReport("src/a.ts"))))

	var out strings.Builder
	if err := RunMutate([]string{"--staged"}, &out); err != nil {
		t.Fatalf("RunMutate() = %v, want nil", err)
	}
	if !strings.Contains(out.String(), "classifier unavailable") {
		t.Errorf("output = %q, want the classifier-unavailable notice", out.String())
	}
	if !commandExists(captured, "stryker") {
		t.Error("want Stryker still run: an unavailable classifier keeps every candidate, it never drops on its own")
	}
}

// TestMutateStagedPrintsIgnoredMutantsAndStillPasses pins the ignored-in-
// scope reporting the staged path owns independently of the ordinary one:
// an Ignored mutant with its own reason must be relayed, and doing so must
// not fail the run.
func TestMutateStagedPrintsIgnoredMutantsAndStillPasses(t *testing.T) {
	root, captured := newStagedRepo(t)
	stageFile(t, root, "src/a.js", "export const a = 1;\n")
	report := `{"schemaVersion":"1.0","files":{"src/a.js":{"language":"javascript","mutants":[` +
		`{"id":"1","mutatorName":"BooleanLiteral","status":"Ignored","statusReason":"equivalent","location":{"start":{"line":1,"column":1},"end":{"line":1,"column":2}}}` +
		`]}}}`
	t.Cleanup(runner.SetForTest(stagedRunner(captured, nil, report)))

	var out strings.Builder
	if err := RunMutate([]string{"--staged"}, &out); err != nil {
		t.Fatalf("RunMutate() = %v, want nil", err)
	}
	if !strings.Contains(out.String(), "skipped by Stryker") || !strings.Contains(out.String(), "equivalent") {
		t.Errorf("output = %q, want the ignored mutant and its reason reported", out.String())
	}
}

// TestMutateStagedPrintsEveryEntryEvenAfterANoCoverageOne pins the survivor
// printing loop's own continuation: a NoCoverage entry prints its own note
// and moves on, it must not stop the scan before a later entry — the same
// shape the ordinary path's own printer guards, owned independently here.
func TestMutateStagedPrintsEveryEntryEvenAfterANoCoverageOne(t *testing.T) {
	root, captured := newStagedRepo(t)
	// Two lines, so the staged scope covers both — a one-line file would
	// scope to just line 1 and silently filter the second mutant out of
	// SurvivorsInScope before the printing loop this test is about ever runs.
	stageFile(t, root, "src/a.js", "export const a = 1;\nexport const b = 2;\n")
	report := `{"schemaVersion":"1.0","files":{"src/a.js":{"language":"javascript","mutants":[` +
		`{"id":"1","mutatorName":"StringLiteral","status":"NoCoverage","location":{"start":{"line":1,"column":1},"end":{"line":1,"column":2}}},` +
		`{"id":"2","mutatorName":"BooleanLiteral","status":"Survived","location":{"start":{"line":2,"column":1},"end":{"line":2,"column":2}}}` +
		`]}}}`
	t.Cleanup(runner.SetForTest(stagedRunner(captured, nil, report)))

	var out strings.Builder
	err := RunMutate([]string{"--staged"}, &out)

	var survivors *SurvivorsError
	if !errors.As(err, &survivors) {
		t.Fatalf("RunMutate() = %v, want SurvivorsError", err)
	}
	if !strings.Contains(out.String(), "No test executes this line") {
		t.Errorf("output = %q, want the NoCoverage note attached to the first (StringLiteral) entry", out.String())
	}
	// Exact rather than a bare "Stryker disable" substring: the hint
	// interpolates the survivor's own mutator name, so this fails if the
	// hint attaches to the wrong entry — e.g. if the NoCoverage check
	// inverted and the two entries swapped which message each one gets.
	if !strings.Contains(out.String(), "Stryker disable BooleanLiteral") {
		t.Errorf("output = %q, want the disable hint attached to the survivor AFTER the NoCoverage entry, by name", out.String())
	}
}

// generatedConfigAtRun captures, at the moment Stryker runs, the config file
// the staged invocation names — the snapshot holding it is removed once the
// run returns, so this is the only moment its contents can be read.
func generatedConfigAtRun(captured *record, tscOutputs map[string]string, report string, generated *string) func(runner.Command, io.Writer, io.Writer) error {
	run := stagedRunner(captured, tscOutputs, report)
	return func(cmd runner.Command, stdout, stderr io.Writer) error {
		if cmd.Label == "stryker" && argsContain(cmd.Args, "dharness-staged.stryker.config.json") {
			contents, err := os.ReadFile(filepath.Join(cmd.Dir, "dharness-staged.stryker.config.json"))
			if err != nil {
				return err
			}
			*generated = string(contents)
		}
		return run(cmd, stdout, stderr)
	}
}

// TestMutateStagedPassesTheInPlaceFlagAndBothScopes pins the exact
// invocation shape: Stryker receives --inPlace,
// and the scope names every scoped path — kept and dropped alike, so the
// self-check has something to compare against.
//
// The scope travels in a generated config file, never as --mutate. On Windows
// stryker.cmd runs through cmd.exe, whose 8191-character command line refuses
// a long comma-joined --mutate outright ("The command line is too long."), and
// a real project's last 300 commits held staged scopes of up to 212 ranges and
// 15764 characters. Stryker's config-file mutate key takes the same
// file:start-end entries.
func TestMutateStagedPassesTheInPlaceFlagAndBothScopes(t *testing.T) {
	root, captured := newStagedRepo(t)
	writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName("tsc")), "")
	stageFile(t, root, "src/a.js", "export const a = 1;\n")
	stageFile(t, root, "src/types.ts", "export type A = string;\n")
	report := `{"schemaVersion":"1.0","files":{` +
		`"src/a.js":{"language":"javascript","mutants":[]},` +
		`"src/types.ts":{"language":"typescript","mutants":[]}` +
		`}}`
	var generated string
	t.Cleanup(runner.SetForTest(generatedConfigAtRun(captured, map[string]string{"src/types.ts": ""}, report, &generated)))

	if err := RunMutate([]string{"--staged"}, io.Discard); err != nil {
		t.Fatalf("RunMutate() = %v, want nil", err)
	}

	stryker := commandFor(t, captured, "stryker")
	if !argsContain(stryker.Args, "--inPlace") {
		t.Errorf("stryker args = %v, want --inPlace", stryker.Args)
	}
	if argsContain(stryker.Args, "--mutate") {
		t.Errorf("stryker args = %v, want no --mutate: the scope travels in the config file", stryker.Args)
	}
	if !argsContain(stryker.Args, "dharness-staged.stryker.config.json") {
		t.Errorf("stryker args = %v, want the generated config named", stryker.Args)
	}
	// Exact rather than Contains for the scope: with no project config the
	// generated file holds only the scope and this run's own report path, in
	// order, kept and dropped alike.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(generated), &fields); err != nil {
		t.Fatalf("generated config %q is not a JSON object: %v", generated, err)
	}
	if len(fields) != 2 {
		t.Errorf("generated config = %s, want exactly mutate and jsonReporter", generated)
	}
	wantMutate := `["src/a.js:1-1","src/types.ts:1-1"]`
	if string(fields["mutate"]) != wantMutate {
		t.Errorf("generated mutate = %s, want %s", fields["mutate"], wantMutate)
	}
	assertOwnAbsoluteReportPath(t, fields["jsonReporter"])
}

// TestMutateStagedCarriesTheProjectConfigIntoTheGeneratedOne pins what the
// generated config keeps. Naming a config file replaces the project's own for
// that run, so every key the project set has to arrive unchanged — each value
// byte for byte, whitespace included — and only mutate is dharness's. A
// bracketed path keeps the escaping Stryker's glob matching needs, because the
// config's mutate entries go through the same matcher --mutate did.
func TestMutateStagedCarriesTheProjectConfigIntoTheGeneratedOne(t *testing.T) {
	root, captured := newStagedRepo(t)
	projectConfig := `{
  "testRunner": "vitest",
  "vitest": { "configFile": "vitest.unit.config.ts" },
  "plugins": [ "@stryker-mutator/vitest-runner", "./local-plugin.js" ],
  "mutate": ["src/**/*.js"]
}`
	writeFile(t, filepath.Join(root, "stryker.config.json"), projectConfig)
	gitRunStaged(t, root, "add", "stryker.config.json")
	gitRunStaged(t, root, "commit", "--quiet", "-m", "stryker config")
	stageFile(t, root, "src/[id]/a.js", "export const a = 1;\n")
	var generated string
	t.Cleanup(runner.SetForTest(generatedConfigAtRun(captured, nil, minimalStagedReport("src/[id]/a.js"), &generated)))

	if err := RunMutate([]string{"--staged"}, io.Discard); err != nil {
		t.Fatalf("RunMutate() = %v, want nil", err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(generated), &fields); err != nil {
		t.Fatalf("generated config %q is not a JSON object: %v", generated, err)
	}
	want := map[string]string{
		"testRunner": `"vitest"`,
		"vitest":     `{ "configFile": "vitest.unit.config.ts" }`,
		"plugins":    `[ "@stryker-mutator/vitest-runner", "./local-plugin.js" ]`,
		"mutate":     `["src/[[]id[]]/a.js:1-1"]`,
	}
	// +1: jsonReporter, which every generated config carries, project config
	// or not — see assertOwnAbsoluteReportPath.
	if len(fields) != len(want)+1 {
		t.Errorf("generated config = %s, want exactly the keys %v plus jsonReporter", generated, want)
	}
	for key, value := range want {
		if string(fields[key]) != value {
			t.Errorf("generated %s = %s, want %s", key, fields[key], value)
		}
	}
	assertOwnAbsoluteReportPath(t, fields["jsonReporter"])
}

// assertOwnAbsoluteReportPath asserts a generated config's jsonReporter names
// an absolute path under a dh-report- temp directory — the per-run location
// --staged always owns, never the project's configured (or default) one; see
// writeStagedStrykerConfig.
func assertOwnAbsoluteReportPath(t *testing.T, jsonReporter json.RawMessage) {
	t.Helper()
	var reporter struct {
		FileName string `json:"fileName"`
	}
	if err := json.Unmarshal(jsonReporter, &reporter); err != nil {
		t.Fatalf("generated jsonReporter = %s, not an object: %v", jsonReporter, err)
	}
	if !filepath.IsAbs(reporter.FileName) {
		t.Errorf("generated jsonReporter.fileName = %q, want an absolute per-run path", reporter.FileName)
	}
	if !strings.HasPrefix(filepath.Base(filepath.Dir(reporter.FileName)), "dh-report-") {
		t.Errorf("generated jsonReporter.fileName = %q, want it under a dh-report- temp directory this run owns", reporter.FileName)
	}
}

// TestMutateStagedOverridesJSONReporterFileNameButKeepsOtherKeys pins the
// concurrency fix at the config level: the generated jsonReporter always
// names this run's own reportPath, replacing whatever the project
// configured, while any other key inside jsonReporter survives byte for
// byte — the same discipline the top-level merge already applies to mutate.
func TestMutateStagedOverridesJSONReporterFileNameButKeepsOtherKeys(t *testing.T) {
	source := t.TempDir()
	writeFile(t, filepath.Join(source, "stryker.config.json"),
		`{"testRunner":"vitest","jsonReporter":{"fileName":"reports/project.json","otherOption":true}}`)
	reportPath := filepath.Join(t.TempDir(), "dh-report-x", "mutation.json")

	configFile, err := writeStagedStrykerConfig(source, "stryker.config.json", []string{"src/a.js:1-1"}, reportPath)
	if err != nil {
		t.Fatalf("writeStagedStrykerConfig() = %v, want nil", err)
	}

	raw, err := os.ReadFile(filepath.Join(source, configFile))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("generated config %s is not a JSON object: %v", raw, err)
	}
	var reporter struct {
		FileName    string `json:"fileName"`
		OtherOption bool   `json:"otherOption"`
	}
	if err := json.Unmarshal(fields["jsonReporter"], &reporter); err != nil {
		t.Fatalf("generated jsonReporter = %s, not an object: %v", fields["jsonReporter"], err)
	}
	if reporter.FileName != reportPath {
		t.Errorf("generated jsonReporter.fileName = %q, want %q: the run's own report path, overriding the project's", reporter.FileName, reportPath)
	}
	if !reporter.OtherOption {
		t.Error("generated jsonReporter.otherOption = false, want true: preserved from the project's config")
	}
}

// TestMutateStagedJudgesOnlyItsOwnPerRunReportPath pins the concurrency fix
// end to end. --staged's snapshot links every gitignored entry — a
// reports/ directory among them — back to the real project rather than
// copying it, so a report at the project's own configured jsonReporter path
// can be the exact file a concurrent plain `dharness mutate` just wrote in
// that same checkout. Even when such a report sits there, with a verdict
// that would fail this run, --staged never opens it: only the report at its
// own generated, per-run path decides anything.
func TestMutateStagedJudgesOnlyItsOwnPerRunReportPath(t *testing.T) {
	root, captured := newStagedRepo(t)
	writeFile(t, filepath.Join(root, "stryker.config.json"), `{"testRunner":"vitest","jsonReporter":{"fileName":"reports/project.json"}}`)
	gitRunStaged(t, root, "add", "stryker.config.json")
	gitRunStaged(t, root, "commit", "--quiet", "-m", "stryker config")
	stageFile(t, root, "src/a.js", "export const a = 1;\n")

	// A concurrent plain run's own verdict, landing exactly where the
	// project's config points — a Survived mutant that would fail this run
	// if it were ever read.
	survivedElsewhere := `{"schemaVersion":"1.0","files":{"src/a.js":{"language":"javascript","mutants":[` +
		`{"id":"1","mutatorName":"BooleanLiteral","status":"Survived","location":{"start":{"line":1,"column":1},"end":{"line":1,"column":2}}}]}}}`
	// This run's own verdict, at its own path: a Killed mutant, so success
	// here can only mean the per-run report was the one read.
	caughtHere := `{"schemaVersion":"1.0","files":{"src/a.js":{"language":"javascript","mutants":[` +
		`{"id":"1","mutatorName":"BooleanLiteral","status":"Killed","location":{"start":{"line":1,"column":1},"end":{"line":1,"column":2}}}]}}}`

	t.Cleanup(runner.SetForTest(func(cmd runner.Command, stdout, stderr io.Writer) error {
		if cmd.Label == "stryker" {
			if err := writeMutationReport(filepath.Join(cmd.Dir, "reports", "project.json"), survivedElsewhere); err != nil {
				return err
			}
			// The real Stryker's own side effect: it writes exactly where
			// its own (generated, overridden) config's jsonReporter.fileName
			// points.
			fileName, err := jsonReporterFileNameIn(cmd.Dir)
			if err != nil {
				return err
			}
			if err := writeMutationReport(fileName, caughtHere); err != nil {
				return err
			}
		}
		return captured.run(cmd, stdout, stderr)
	}))

	var out strings.Builder
	if err := RunMutate([]string{"--staged"}, &out); err != nil {
		t.Fatalf("RunMutate() = %v, want the verdict read from the run's own report, not reports/project.json", err)
	}
	if !strings.Contains(out.String(), "Every mutant was caught") {
		t.Errorf("output = %q, want the verdict from the per-run report, not the Survived one at reports/project.json", out.String())
	}
}

// TestWriteStagedStrykerConfigRefusesAConfigItCannotExtend pins the refusal
// when the project's config cannot carry a mutate key: dharness never falls
// back to a config without the project's keys, which would run Stryker on
// settings nobody chose, and the error names the file to fix.
func TestWriteStagedStrykerConfigRefusesAConfigItCannotExtend(t *testing.T) {
	cases := []struct{ name, contents string }{
		{"invalid JSON", `{`},
		{"null rather than an object", `null`},
		{"jsonReporter set to a scalar rather than an object", `{"jsonReporter":5}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := t.TempDir()
			writeFile(t, filepath.Join(source, ".stryker.conf.json"), tc.contents)

			_, err := writeStagedStrykerConfig(source, ".stryker.conf.json", []string{"src/a.js:1-1"}, filepath.Join(t.TempDir(), "mutation.json"))

			if err == nil || !strings.Contains(err.Error(), ".stryker.conf.json") {
				t.Fatalf("writeStagedStrykerConfig() = %v, want an error naming .stryker.conf.json", err)
			}
		})
	}
}

// tempDirForThisTest points os.TempDir at a fresh directory for the rest of
// the test, so whatever a staged run materialises there can be counted once
// it returns.
func tempDirForThisTest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"TMP", "TEMP", "TMPDIR"} {
		t.Setenv(name, dir)
	}
	return dir
}

// entriesIn names what dir holds, for asserting a run left nothing behind.
func entriesIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// TestMutateStagedInterruptedDuringTheGuardCleansUpOnlyOnceItsChildReturned
// pins the order an interrupt has to follow. The guard's `vitest list` runs
// for about 70 seconds on a real project, inside the snapshot. When the
// interrupt arrives the child is still exiting, and until it has, the snapshot
// is its working directory: removing it then fails on Windows, and a cleanup
// that had already spent itself on that failure left the whole copy behind.
func TestMutateStagedInterruptedDuringTheGuardCleansUpOnlyOnceItsChildReturned(t *testing.T) {
	root, captured := newStagedRepo(t)
	writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName("vitest")), "")
	stageFile(t, root, "src/a.js", "export const a = 1;\n")
	temp := tempDirForThisTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var guardContext context.Context
	snapshotOutlivedTheChild := false
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, stdout, stderr io.Writer) error {
		if cmd.Label != "vitest" {
			return captured.run(cmd, stdout, stderr)
		}
		captured.commands = append(captured.commands, cmd)
		guardContext = cmd.Context
		cancel()
		// Still exiting: long enough for anything racing the child to act.
		time.Sleep(200 * time.Millisecond)
		_, err := os.Stat(cmd.Dir)
		snapshotOutlivedTheChild = err == nil
		return &runner.ExitError{Command: "vitest", Code: 1}
	}))

	err := mutateStaged(ctx, 2, nil, io.Discard, newPhaseRecord())

	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("mutateStaged() = %v, want it to report the interruption rather than a suite that failed to load", err)
	}
	if guardContext == nil || guardContext.Err() == nil {
		t.Error("vitest list ran without the run's context, so an interrupt could not have killed it")
	}
	if !snapshotOutlivedTheChild {
		t.Error("the snapshot was removed while the guard's child was still running in it")
	}
	if commandExists(captured, "stryker") {
		t.Errorf("commands = %+v, want no stryker command after an interrupt", captured.commands)
	}
	if left := entriesIn(t, temp); len(left) != 0 {
		t.Errorf("the run left %v behind, want its snapshot removed once the child had returned", left)
	}
}

// TestMutateStagedInterruptedDuringTheClassifierRunsNothingAfterIt pins the
// check between steps: a child that exits cleanly just as the interrupt
// arrives must not let the run carry on to the next command.
func TestMutateStagedInterruptedDuringTheClassifierRunsNothingAfterIt(t *testing.T) {
	root, captured := newStagedRepo(t)
	writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName("tsc")), "")
	writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName("vitest")), "")
	stageFile(t, root, "src/a.ts", "export const a = 1;\n")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var tscContext context.Context
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, stdout, stderr io.Writer) error {
		if cmd.Label == "tsc" {
			tscContext = cmd.Context
			cancel()
		}
		return captured.run(cmd, stdout, stderr)
	}))

	err := mutateStaged(ctx, 2, nil, io.Discard, newPhaseRecord())

	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("mutateStaged() = %v, want the interruption", err)
	}
	if tscContext == nil || tscContext.Err() == nil {
		t.Error("tsc ran without the run's context, so an interrupt could not have killed it")
	}
	if commandExists(captured, "vitest") || commandExists(captured, "stryker") {
		t.Errorf("commands = %+v, want nothing after the interrupted classifier", captured.commands)
	}
}

// TestMutateStagedInterruptedBeforeTheSnapshotNeverMaterialisesOne pins the
// check before Snapshot: a run already interrupted does not pay for checking
// the whole index out only to delete it again.
func TestMutateStagedInterruptedBeforeTheSnapshotNeverMaterialisesOne(t *testing.T) {
	root, captured := newStagedRepo(t)
	stageFile(t, root, "src/a.js", "export const a = 1;\n")
	t.Cleanup(runner.SetForTest(captured.run))
	var probes []string
	t.Cleanup(staged.SetGitOutputForTest(func(dir string, args ...string) ([]byte, error) {
		probes = append(probes, strings.Join(args, " "))
		return project.GitOutput(dir, args...)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := mutateStaged(ctx, 2, nil, io.Discard, newPhaseRecord())

	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("mutateStaged() = %v, want the interruption", err)
	}
	for _, probe := range probes {
		if strings.Contains(probe, "checkout-index") {
			t.Errorf("git %s ran, want no snapshot materialised for a run already interrupted", probe)
		}
	}
	if len(captured.commands) != 0 {
		t.Errorf("commands = %+v, want none", captured.commands)
	}
}

// TestMutateStagedInterruptedDuringTheSnapshotStartsNoChild pins the check
// after Snapshot: its git probes take no context, so an interrupt that lands
// while the index is being checked out is only seen once they return, and the
// run must stop there rather than hand the fresh snapshot to tsc.
func TestMutateStagedInterruptedDuringTheSnapshotStartsNoChild(t *testing.T) {
	root, captured := newStagedRepo(t)
	writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName("tsc")), "")
	stageFile(t, root, "src/a.ts", "export const a = 1;\n")
	t.Cleanup(runner.SetForTest(captured.run))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	t.Cleanup(staged.SetGitOutputForTest(func(dir string, args ...string) ([]byte, error) {
		if slices.Contains(args, "checkout-index") {
			cancel()
		}
		return project.GitOutput(dir, args...)
	}))

	err := mutateStaged(ctx, 2, nil, io.Discard, newPhaseRecord())

	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("mutateStaged() = %v, want the interruption", err)
	}
	if len(captured.commands) != 0 {
		t.Errorf("commands = %+v, want none after an interrupt during the snapshot", captured.commands)
	}
}

// TestMutateStagedInterruptedDuringStrykerReportsTheInterruptNotAVerdict pins
// the last step: Stryker killed mid-run exits non-zero, and neither its help
// pointer nor a verdict read from a half-written report is an answer to an
// interrupt.
func TestMutateStagedInterruptedDuringStrykerReportsTheInterruptNotAVerdict(t *testing.T) {
	root, captured := newStagedRepo(t)
	stageFile(t, root, "src/a.js", "export const a = 1;\n")
	temp := tempDirForThisTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, stdout, stderr io.Writer) error {
		captured.commands = append(captured.commands, cmd)
		if cmd.Label == "stryker" {
			cancel()
			return &runner.ExitError{Command: "stryker", Code: 1}
		}
		return nil
	}))

	var out strings.Builder
	err := mutateStaged(ctx, 2, nil, &out, newPhaseRecord())

	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("mutateStaged() = %v, want the interruption", err)
	}
	if strings.Contains(out.String(), "ask the tool") {
		t.Errorf("output = %q, want no pointer to Stryker's help for a run that was interrupted", out.String())
	}
	if left := entriesIn(t, temp); len(left) != 0 {
		t.Errorf("the run left %v behind", left)
	}
}

// TestMutateStagedReportsAChildTheConsoleInterruptEndedBeforeItsOwnHandler pins
// the race a real interrupt runs. A console interrupt reaches the child and
// dharness at the same moment, and the child can be dead before dharness's
// handler has cancelled the context. Measured through `taskkill /PID` mid-run:
// once the run reported "stryker exited with code 3221225786", and every time
// it printed a pointer to Stryker's help. The child's own ending says what
// happened, so the context being still live must not turn it into a failure.
func TestMutateStagedReportsAChildTheConsoleInterruptEndedBeforeItsOwnHandler(t *testing.T) {
	interruptedExit := func(tool string) error {
		return &runner.ExitError{Command: tool, Code: 3221225786, Interrupted: true}
	}
	cases := []struct {
		name, endedBy string
	}{
		{"during the guard", "vitest"},
		{"during stryker", "stryker"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, captured := newStagedRepo(t)
			writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName("vitest")), "")
			stageFile(t, root, "src/a.js", "export const a = 1;\n")
			captured.fail[tc.endedBy] = interruptedExit(tc.endedBy)
			t.Cleanup(runner.SetForTest(captured.run))

			var out strings.Builder
			err := mutateStaged(context.Background(), 2, nil, &out, newPhaseRecord())

			if !errors.Is(err, ErrInterrupted) {
				t.Fatalf("mutateStaged() = %v, want the interruption, not %s's failure", err, tc.endedBy)
			}
			if strings.Contains(out.String(), "ask the tool") {
				t.Errorf("output = %q, want no pointer to Stryker's help for an interrupted run", out.String())
			}
		})
	}
}

func argsContain(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}

// TestUniqueScopedFilesContinuesPastADuplicateToLaterFiles pins the
// dedup loop's own continue: a second range over an already-seen file must
// not stop the scan before a different file that follows it.
func TestUniqueScopedFilesContinuesPastADuplicateToLaterFiles(t *testing.T) {
	scopes := []tool.MutationScope{
		tool.ParseMutationScope("src/a.ts:1-1"),
		tool.ParseMutationScope("src/a.ts:2-2"), // a second range, same file
		tool.ParseMutationScope("src/b.ts:1-1"),
	}

	got := uniqueScopedFiles(scopes)

	want := []string{"src/a.ts", "src/b.ts"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("uniqueScopedFiles() = %v, want %v", got, want)
	}
}

// TestClearReportTolerantOfAMissingReport pins the ordinary case: nothing to
// clear is not a failure.
func TestClearReportTolerantOfAMissingReport(t *testing.T) {
	if err := clearReport(filepath.Join(t.TempDir(), "missing.json")); err != nil {
		t.Errorf("clearReport() = %v, want nil", err)
	}
}

// TestClearReportRemovesAnExistingReport pins the ordinary success case
// directly, so a mutant that turns a clean removal into a reported failure
// (or the reverse) is caught regardless of what the missing-file case alone
// would show.
func TestClearReportRemovesAnExistingReport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mutation.json")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := clearReport(path); err != nil {
		t.Fatalf("clearReport() = %v, want nil", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("want the report actually removed")
	}
}

// TestClearReportRefusesARealRemovalError pins the fail-closed half: a
// removal failure that is NOT "does not exist" must be reported rather than
// swallowed the same way a missing file is.
func TestClearReportRefusesARealRemovalError(t *testing.T) {
	dir := t.TempDir()
	// A non-empty directory at the report's own path: os.Remove refuses to
	// remove it, and the failure is neither IsNotExist nor anything this
	// function is entitled to treat as "already gone".
	path := filepath.Join(dir, "mutation.json")
	if err := os.MkdirAll(filepath.Join(path, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := clearReport(path); err == nil {
		t.Fatal("clearReport() = nil, want an error: the removal failed for a reason other than \"does not exist\"")
	}
}
