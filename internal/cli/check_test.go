package cli

import (
	"bytes"
	"errors"
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
}

func (r *record) run(cmd runner.Command, stdout, _ io.Writer) error {
	r.commands = append(r.commands, cmd)
	if r.emit != "" {
		_, _ = io.WriteString(stdout, r.emit)
	}
	return r.fail[toolOf(cmd)]
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
		default:
			return []byte(strings.ReplaceAll(staged, "\n", "\x00")), nil
		}
	}
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
	// Both fallow stages follow: audit answers the changeset-scoped question
	// and dupes the whole-repository one. react-doctor stays first because
	// --staged scopes it to the diff, so its cost tracks the change.
	for _, i := range []int{1, 2} {
		if got := toolOf(captured.commands[i]); got != "fallow" {
			t.Errorf("command %d = %q, want fallow", i, got)
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
		{Label: "fallow", Name: "npx", Args: []string{"--yes", "fallow@latest", "audit"}, Dir: root},
		{Label: "fallow", Name: "npx", Args: []string{"--yes", "fallow@latest", "dupes"}, Dir: root},
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
func TestMutatePairsIncrementalWithForceAndBoundsConcurrency(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)

	_ = RunMutate([]string{"src/a.ts", "src/b.ts"}, io.Discard)

	args := strykerArgs(t, captured)
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
	if !slices.Equal(first.Args, []string{"src/a.ts", "src/b.tsx"}) {
		t.Errorf("eslint args = %v, want exactly the staged files", first.Args)
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
	if !slices.Equal(first.Args, []string{"src/a.ts"}) {
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
	if got := toolOf(last); got != "fallow" {
		t.Errorf("third command = %q, want fallow", got)
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
