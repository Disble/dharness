package staged

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/runner"
)

// fakeTsc replaces runner.Run with a stand-in that behaves like tsc's own
// --outDir/--rootDir contract for exactly the paths named in contents: for
// each staged, scope-relative path present in cmd.Args (as its absolute
// snapshotSource-joined form), it writes the given content at the same
// location tsc itself would — outDir joined with compiledOutputName(path).
// Any invocation naming none of those paths writes nothing and still
// succeeds, which is what a real tsc run over unrelated files would do.
func fakeTsc(t *testing.T, snapshotSource string, contents map[string]string) func() {
	t.Helper()
	return runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		outDir := flagValue(cmd.Args, "--outDir")
		if outDir == "" {
			t.Fatalf("fake tsc invoked without --outDir: %v", cmd.Args)
		}
		for rel, content := range contents {
			if !containsArg(cmd.Args, filepath.Join(snapshotSource, filepath.FromSlash(rel))) {
				continue
			}
			out := filepath.Join(outDir, filepath.FromSlash(compiledOutputName(rel)))
			if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(out, []byte(content), 0o644); err != nil {
				return err
			}
		}
		return nil
	})
}

// fakeTscExitCode replaces runner.Run with a stand-in that always reports the
// given non-zero exit, as runner.ExitError does for a tool that ran and
// failed — never a StartError, which would mean the binary could not be
// reached at all.
func fakeTscExitCode(code int) func() {
	return runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		return &runner.ExitError{Command: cmd.String(), Code: code}
	})
}

// fakeTscNeverCalled replaces runner.Run with a stand-in that fails the test
// the moment it is invoked, for asserting a call that must never happen.
func fakeTscNeverCalled(t *testing.T) func() {
	t.Helper()
	return runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		t.Fatalf("tsc invoked, want no invocation at all: %v", cmd.Args)
		return nil
	})
}

func flagValue(args []string, name string) string {
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func containsArg(args []string, value string) bool {
	for _, arg := range args {
		if arg == value {
			return true
		}
	}
	return false
}

// TestClassifyDropsAnEmptyOutput pins the plain empty-output shape: a
// types-only file — declare enum, a types-only namespace, export type *,
// declare global — compiles to nothing at all under --isolatedModules.
func TestClassifyDropsAnEmptyOutput(t *testing.T) {
	source := t.TempDir()
	defer fakeTsc(t, source, map[string]string{"src/types.ts": ""})()

	result, err := Classify(context.Background(), "tsc", source, []string{"src/types.ts"})
	if err != nil {
		t.Fatalf("Classify() = %v", err)
	}
	if len(result.Kept) != 0 {
		t.Errorf("Classify().Kept = %v, want none", result.Kept)
	}
	if len(result.Dropped) != 1 || result.Dropped[0] != "src/types.ts" {
		t.Errorf("Classify().Dropped = %v, want [src/types.ts]", result.Dropped)
	}
	if result.Notice != "" {
		t.Errorf("Classify().Notice = %q, want none: the classifier ran to a verdict", result.Notice)
	}
}

// TestClassifyDropsTheBareModuleMarker pins the second empty shape:
// isolatedModules forces a file with no other emitted statement into a bare
// `export {};`, which is exactly as empty as "" for dharness's own purposes.
func TestClassifyDropsTheBareModuleMarker(t *testing.T) {
	source := t.TempDir()
	defer fakeTsc(t, source, map[string]string{"src/types.ts": "export {};\n"})()

	result, err := Classify(context.Background(), "tsc", source, []string{"src/types.ts"})
	if err != nil {
		t.Fatalf("Classify() = %v", err)
	}
	if len(result.Dropped) != 1 || result.Dropped[0] != "src/types.ts" {
		t.Errorf("Classify().Dropped = %v, want [src/types.ts]", result.Dropped)
	}
}

// TestClassifyKeepsNonEmptyOutput is the other side: a file whose compiled
// output carries a real statement must never be dropped.
func TestClassifyKeepsNonEmptyOutput(t *testing.T) {
	source := t.TempDir()
	defer fakeTsc(t, source, map[string]string{"src/a.ts": "export const a = 1;\n"})()

	result, err := Classify(context.Background(), "tsc", source, []string{"src/a.ts"})
	if err != nil {
		t.Fatalf("Classify() = %v", err)
	}
	if len(result.Dropped) != 0 {
		t.Errorf("Classify().Dropped = %v, want none", result.Dropped)
	}
	if len(result.Kept) != 1 || result.Kept[0] != "src/a.ts" {
		t.Errorf("Classify().Kept = %v, want [src/a.ts]", result.Kept)
	}
}

// TestClassifyMixesKeptAndDroppedInTheGivenFiles pins that the per-file
// decision is independent across a scope carrying both shapes, and that
// neither file's own outcome leaks onto the other.
func TestClassifyMixesKeptAndDroppedInTheGivenFiles(t *testing.T) {
	source := t.TempDir()
	defer fakeTsc(t, source, map[string]string{
		"src/a.ts":     "export const a = 1;\n",
		"src/types.ts": "export {};\n",
	})()

	result, err := Classify(context.Background(), "tsc", source, []string{"src/a.ts", "src/types.ts"})
	if err != nil {
		t.Fatalf("Classify() = %v", err)
	}
	if len(result.Kept) != 1 || result.Kept[0] != "src/a.ts" {
		t.Errorf("Classify().Kept = %v, want [src/a.ts]", result.Kept)
	}
	if len(result.Dropped) != 1 || result.Dropped[0] != "src/types.ts" {
		t.Errorf("Classify().Dropped = %v, want [src/types.ts]", result.Dropped)
	}
}

// TestClassifyContinuesPastAnExcludedFileToLaterCandidates pins the
// exclusion loop's own continue: skipping a pass-through file (non-TS or a
// declaration file) must not stop the scan before a real candidate that
// follows it in the same call.
func TestClassifyContinuesPastAnExcludedFileToLaterCandidates(t *testing.T) {
	source := t.TempDir()
	defer fakeTsc(t, source, map[string]string{"src/b.ts": "export const b = 1;\n"})()

	result, err := Classify(context.Background(), "tsc", source, []string{"src/a.js", "src/b.ts"})
	if err != nil {
		t.Fatalf("Classify() = %v", err)
	}
	if len(result.Kept) != 2 {
		t.Fatalf("Classify().Kept = %v, want both src/a.js (passed through) and src/b.ts (real code): skipping src/a.js must not stop the scan", result.Kept)
	}
}

// TestClassifyContinuesPastADroppedFileToLaterKeptFiles pins the decision
// loop's own continue: dropping one candidate must not stop the scan before
// a later candidate that still carries real code.
func TestClassifyContinuesPastADroppedFileToLaterKeptFiles(t *testing.T) {
	source := t.TempDir()
	defer fakeTsc(t, source, map[string]string{
		"src/types.ts": "export {};\n",
		"src/a.ts":     "export const a = 1;\n",
	})()

	result, err := Classify(context.Background(), "tsc", source, []string{"src/types.ts", "src/a.ts"})
	if err != nil {
		t.Fatalf("Classify() = %v", err)
	}
	if len(result.Dropped) != 1 || result.Dropped[0] != "src/types.ts" {
		t.Errorf("Classify().Dropped = %v, want [src/types.ts]", result.Dropped)
	}
	if len(result.Kept) != 1 || result.Kept[0] != "src/a.ts" {
		t.Fatalf("Classify().Kept = %v, want [src/a.ts]: dropping src/types.ts must not stop the scan before it", result.Kept)
	}
}

// TestClassifyDropsNothingWhenTscCannotStart pins the exit-code fallback
// directly: a command that never started at all — the binary could not be
// reached, runner.StartError rather than runner.ExitError — has no exit code
// to report, and the notice must still name a stable, deliberate value
// rather than whatever a zero-valued int happens to be.
func TestClassifyDropsNothingWhenTscCannotStart(t *testing.T) {
	source := t.TempDir()
	defer runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		return &runner.StartError{Command: cmd.String(), Cause: os.ErrNotExist}
	})()

	result, err := Classify(context.Background(), "tsc", source, []string{"src/a.ts"})
	if err != nil {
		t.Fatalf("Classify() = %v", err)
	}
	if len(result.Kept) != 1 || result.Kept[0] != "src/a.ts" {
		t.Errorf("Classify().Kept = %v, want [src/a.ts]", result.Kept)
	}
	if !strings.Contains(result.Notice, "tsc exited -1") {
		t.Errorf("Classify().Notice = %q, want it to report the fallback exit code -1", result.Notice)
	}
}

// TestClassifyDropsNothingOnANonZeroExit pins the fail-safe half: tsc running
// and failing answers no question about which files are types-only, so every
// candidate is kept and the loud line says why nothing was dropped.
func TestClassifyDropsNothingOnANonZeroExit(t *testing.T) {
	source := t.TempDir()
	defer fakeTscExitCode(1)()

	result, err := Classify(context.Background(), "tsc", source, []string{"src/a.ts", "src/types.ts"})
	if err != nil {
		t.Fatalf("Classify() = %v", err)
	}
	if len(result.Dropped) != 0 {
		t.Errorf("Classify().Dropped = %v, want none: tsc failed to run", result.Dropped)
	}
	if len(result.Kept) != 2 {
		t.Errorf("Classify().Kept = %v, want both candidates kept", result.Kept)
	}
	if !strings.Contains(result.Notice, "tsc exited 1") {
		t.Errorf("Classify().Notice = %q, want it to name the exit code", result.Notice)
	}
}

// TestClassifyDropsNothingOnAMissingOutput pins the other fail-safe half: tsc
// exiting 0 but never writing one candidate's output — measured as a real
// shape, not a hypothetical — is exactly as unusable a
// verdict as a non-zero exit, and must fail the same way rather than
// dropping every file it did produce output for.
func TestClassifyDropsNothingOnAMissingOutput(t *testing.T) {
	source := t.TempDir()
	// Only src/a.ts gets an output; src/types.ts's is never written.
	defer fakeTsc(t, source, map[string]string{"src/a.ts": "export const a = 1;\n"})()

	result, err := Classify(context.Background(), "tsc", source, []string{"src/a.ts", "src/types.ts"})
	if err != nil {
		t.Fatalf("Classify() = %v", err)
	}
	if len(result.Dropped) != 0 {
		t.Errorf("Classify().Dropped = %v, want none: one output never arrived", result.Dropped)
	}
	if len(result.Kept) != 2 {
		t.Errorf("Classify().Kept = %v, want both candidates kept", result.Kept)
	}
	if !strings.Contains(result.Notice, "src/types.ts") {
		t.Errorf("Classify().Notice = %q, want it to name the missing output", result.Notice)
	}
}

// TestClassifyNeverClassifiesANonTSFile pins the extension boundary: a
// .js/.jsx/.mjs/.cjs file is passed through untouched, and tsc is never even
// asked about it — there is nothing here for it to classify.
func TestClassifyNeverClassifiesANonTSFile(t *testing.T) {
	defer fakeTscNeverCalled(t)()

	result, err := Classify(context.Background(), "tsc", t.TempDir(), []string{"src/a.js"})
	if err != nil {
		t.Fatalf("Classify() = %v", err)
	}
	if len(result.Kept) != 1 || result.Kept[0] != "src/a.js" {
		t.Errorf("Classify().Kept = %v, want [src/a.js]", result.Kept)
	}
	if len(result.Dropped) != 0 || result.Notice != "" {
		t.Errorf("Classify() = %+v, want a plain pass-through", result)
	}
}

// TestClassifyNeverClassifiesADeclarationFile pins the same boundary Scope
// draws, defensively, a second time here: a declaration file is passed
// through untouched even if one reaches Classify directly.
func TestClassifyNeverClassifiesADeclarationFile(t *testing.T) {
	defer fakeTscNeverCalled(t)()

	result, err := Classify(context.Background(), "tsc", t.TempDir(), []string{"src/env.d.ts"})
	if err != nil {
		t.Fatalf("Classify() = %v", err)
	}
	if len(result.Kept) != 1 || result.Kept[0] != "src/env.d.ts" {
		t.Errorf("Classify().Kept = %v, want [src/env.d.ts]", result.Kept)
	}
}

// TestClassifyUnavailableWithNoLocalBinary pins the "no tsc at all" shape: an
// empty binary path means the classifier cannot be asked anything, so every
// TypeScript candidate is kept and the notice says why.
func TestClassifyUnavailableWithNoLocalBinary(t *testing.T) {
	defer fakeTscNeverCalled(t)()

	result, err := Classify(context.Background(), "", t.TempDir(), []string{"src/a.ts"})
	if err != nil {
		t.Fatalf("Classify() = %v", err)
	}
	if len(result.Kept) != 1 || result.Kept[0] != "src/a.ts" {
		t.Errorf("Classify().Kept = %v, want [src/a.ts]", result.Kept)
	}
	if result.Notice == "" {
		t.Error("Classify().Notice = \"\", want an explanation: there is no local tsc to ask")
	}
}

// TestClassifyBuildsTheDocumentedFlagSequence pins the exact tsc invocation:
// every flag Classify documents, in order, followed by --rootDir and --outDir,
// followed by the absolute snapshot paths of the candidates, in the order
// they were given.
func TestClassifyBuildsTheDocumentedFlagSequence(t *testing.T) {
	source := t.TempDir()
	var captured []string
	defer runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		captured = append([]string{}, cmd.Args...)
		return nil
	})()

	if _, err := Classify(context.Background(), "tsc", source, []string{"src/a.ts", "src/b.ts"}); err != nil {
		t.Fatalf("Classify() = %v", err)
	}

	if len(captured) < 2 {
		t.Fatalf("captured args too short: %v", captured)
	}
	filePaths := captured[len(captured)-2:]
	wantFiles := []string{filepath.Join(source, "src", "a.ts"), filepath.Join(source, "src", "b.ts")}
	for i, want := range wantFiles {
		if filePaths[i] != want {
			t.Errorf("captured file arg[%d] = %q, want %q", i, filePaths[i], want)
		}
	}

	flags := captured[:len(captured)-2]
	want := []string{
		"--noCheck", "--isolatedModules", "--noResolve", "--skipLibCheck",
		"--removeComments", "--module", "esnext", "--target", "esnext",
		"--jsx", "preserve", "--declaration", "false", "--sourceMap", "false",
		"--rootDir", source, "--outDir", flagValue(captured, "--outDir"),
	}
	if len(flags) != len(want) {
		t.Fatalf("captured flags = %v, want %v", flags, want)
	}
	for i := range want {
		if flags[i] != want[i] {
			t.Errorf("flags[%d] = %q, want %q (full: %v)", i, flags[i], want[i], flags)
		}
	}
}

// TestClassifyRunsFromAFreshCwdOutsideTheProjectOrSnapshot pins the TS5112
// avoidance directly: tsc errors when files are named on the command line
// and a tsconfig.json sits in the cwd or any ancestor, and the snapshot
// carries the project's own tracked tsconfig.json — so the invocation's own
// working directory must be a fresh, empty temp directory that is neither
// the snapshot nor anywhere under it.
func TestClassifyRunsFromAFreshCwdOutsideTheProjectOrSnapshot(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "tsconfig.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	var dir string
	var existedAndWasADir bool
	defer runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		dir = cmd.Dir
		// Checked from inside the fake, before Classify's own deferred
		// cleanup removes it: the directory only needs to exist for the
		// duration of the real tsc invocation it is standing in for.
		if info, err := os.Stat(cmd.Dir); err == nil {
			existedAndWasADir = info.IsDir()
		}
		return nil
	})()

	if _, err := Classify(context.Background(), "tsc", source, []string{"src/a.ts"}); err != nil {
		t.Fatalf("Classify() = %v", err)
	}

	if dir == "" {
		t.Fatal("tsc ran with no cwd recorded")
	}
	if !existedAndWasADir {
		t.Errorf("cmd.Dir = %q was not a real, existing directory at invocation time", dir)
	}
	rel, err := filepath.Rel(source, dir)
	if err == nil && !strings.HasPrefix(rel, "..") {
		t.Errorf("cmd.Dir = %q, want a directory outside the snapshot %q (rel = %q)", dir, source, rel)
	}
}

// candidatesUnderALongDirectory returns count scope-relative .ts paths deep
// enough that together they are far past what one cmd.exe line carries, and a
// fake tsc content map giving each one real runtime code.
func candidatesUnderALongDirectory(count int) ([]string, map[string]string) {
	dir := "src/" + strings.Repeat("a-deliberately-long-directory/", 4)
	files := make([]string, 0, count)
	contents := make(map[string]string, count)
	for i := range count {
		rel := fmt.Sprintf("%smodule-%03d.ts", dir, i)
		files = append(files, rel)
		contents[rel] = "export const a = 1;\n"
	}
	return files, contents
}

// candidateArgs is the part of a tsc invocation naming candidates: every
// argument under source, which excludes --rootDir's own value (source itself).
func candidateArgs(args []string, source string) []string {
	var candidates []string
	for _, arg := range args {
		if strings.HasPrefix(arg, source+string(filepath.Separator)) {
			candidates = append(candidates, arg)
		}
	}
	return candidates
}

// TestClassifySplitsALargeChangeAcrossInvocationsTheShellAccepts pins the
// command-line cap. tsc is reached through its npm .cmd shim, so cmd.exe
// re-parses the whole line, and cmd.exe refuses one over 8191 characters.
// Measured with this exact invocation on a 150-file change: one line of
// roughly 26,000 characters, "The command line is too long.", exit 1, and the
// classifier unavailable for the whole commit. Every candidate must still be
// named exactly once, and a verdict must still be reached.
func TestClassifySplitsALargeChangeAcrossInvocationsTheShellAccepts(t *testing.T) {
	source := t.TempDir()
	files, contents := candidatesUnderALongDirectory(80)
	var invocations [][]string
	defer fakeTsc(t, source, contents)()
	writeOutputs := runner.Run
	defer runner.SetForTest(func(cmd runner.Command, stdout, stderr io.Writer) error {
		invocations = append(invocations, candidateArgs(cmd.Args, source))
		return writeOutputs(cmd, stdout, stderr)
	})()

	result, err := Classify(context.Background(), "tsc", source, files)
	if err != nil {
		t.Fatalf("Classify() = %v", err)
	}

	if len(invocations) < 2 {
		t.Fatalf("tsc ran %d time(s), want the candidates split across several invocations", len(invocations))
	}
	named := map[string]int{}
	for i, invocation := range invocations {
		size := 0
		for _, path := range invocation {
			size += len(path) + 3
			named[path]++
		}
		if size > 6000 {
			t.Errorf("invocation %d carries %d characters of quoted candidate paths, want at most 6000", i, size)
		}
	}
	for _, rel := range files {
		if abs := filepath.Join(source, filepath.FromSlash(rel)); named[abs] != 1 {
			t.Errorf("%s was named %d time(s), want exactly once", rel, named[abs])
		}
	}
	if result.Notice != "" || len(result.Kept) != len(files) {
		t.Errorf("Classify() = %d kept, notice %q; want all %d kept by a verdict", len(result.Kept), result.Notice, len(files))
	}
}

// TestClassifyDropsNothingWhenALaterInvocationFails pins that splitting the
// change does not split the verdict: the first invocation's files compile to
// nothing, the second invocation fails, and nothing may be dropped — a run
// that could not classify every candidate answers the question for none.
func TestClassifyDropsNothingWhenALaterInvocationFails(t *testing.T) {
	source := t.TempDir()
	files, contents := candidatesUnderALongDirectory(80)
	for rel := range contents {
		contents[rel] = "export {};\n"
	}
	calls := 0
	defer fakeTsc(t, source, contents)()
	writeOutputs := runner.Run
	defer runner.SetForTest(func(cmd runner.Command, stdout, stderr io.Writer) error {
		calls++
		if calls > 1 {
			return &runner.ExitError{Command: cmd.String(), Code: 1}
		}
		return writeOutputs(cmd, stdout, stderr)
	})()

	result, err := Classify(context.Background(), "tsc", source, files)
	if err != nil {
		t.Fatalf("Classify() = %v", err)
	}
	if calls < 2 {
		t.Fatalf("tsc ran %d time(s), want a second invocation to fail", calls)
	}
	if len(result.Dropped) != 0 || len(result.Kept) != len(files) {
		t.Errorf("Classify() dropped %d and kept %d, want nothing dropped and all %d kept", len(result.Dropped), len(result.Kept), len(files))
	}
	if !strings.Contains(result.Notice, "tsc exited 1") {
		t.Errorf("Classify().Notice = %q, want it to name the failed invocation's exit code", result.Notice)
	}
}

// TestTscBatchesFillTheBudgetAndNeverEmitAnEmptyBatch pins the batching rule
// itself: a quoted path costs its length plus three characters, a batch may
// use the budget exactly, one character more starts the next, and a path
// longer than the whole budget is still named — alone, never after an empty
// invocation.
func TestTscBatchesFillTheBudgetAndNeverEmitAnEmptyBatch(t *testing.T) {
	path := func(cost int) string { return strings.Repeat("p", cost-3) }
	cases := []struct {
		name  string
		costs []int
		want  []int
	}{
		{"exactly the budget stays in one invocation", []int{3000, 3000}, []int{2}},
		{"one character over starts a second", []int{3000, 3001}, []int{1, 1}},
		{"an overlong path runs alone", []int{7000}, []int{1}},
		{"an overlong path does not strand the next", []int{7000, 10}, []int{1, 1}},
		{"a later batch also fills to exactly the budget", []int{7000, 3000, 3000}, []int{1, 2}},
		{"a later batch also splits one character over", []int{7000, 3000, 3001}, []int{1, 1, 1}},
	}
	for _, tc := range cases {
		var paths []string
		for _, cost := range tc.costs {
			paths = append(paths, path(cost))
		}
		batches := tscBatches(paths)
		var got []int
		for _, batch := range batches {
			got = append(got, len(batch))
		}
		if len(got) != len(tc.want) {
			t.Errorf("%s: batch sizes = %v, want %v", tc.name, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s: batch sizes = %v, want %v", tc.name, got, tc.want)
				break
			}
		}
	}
}

// TestCompiledOutputNameSwapsTheDocumentedExtensions pins the exact mapping
// measured against a real 518-file frontend with --jsx preserve: .ts and
// .mts and .cts are the ordinary compiled shapes, and .tsx becomes .jsx
// rather than .js because preserve leaves JSX syntax in the emitted file.
func TestCompiledOutputNameSwapsTheDocumentedExtensions(t *testing.T) {
	cases := map[string]string{
		"src/a.ts":    "src/a.js",
		"src/a.tsx":   "src/a.jsx",
		"src/a.mts":   "src/a.mjs",
		"src/a.cts":   "src/a.cjs",
		"src/a.other": "src/a.other",
	}
	for in, want := range cases {
		if got := compiledOutputName(in); got != want {
			t.Errorf("compiledOutputName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestIsTypeScriptFileMatchesOnlyTheFourExtensions pins the boundary
// isTypeScriptFile draws, independent of isDeclarationFile — a declaration
// file also ends in one of these four extensions, and Classify relies on
// checking isDeclarationFile first rather than on this function excluding it.
func TestIsTypeScriptFileMatchesOnlyTheFourExtensions(t *testing.T) {
	cases := map[string]bool{
		"src/a.ts":   true,
		"src/a.tsx":  true,
		"src/a.mts":  true,
		"src/a.cts":  true,
		"src/a.js":   false,
		"src/a.jsx":  false,
		"src/a.mjs":  false,
		"src/a.cjs":  false,
		"src/a.d.ts": true,
		"src/a.md":   false,
	}
	for path, want := range cases {
		if got := isTypeScriptFile(path); got != want {
			t.Errorf("isTypeScriptFile(%q) = %v, want %v", path, got, want)
		}
	}
}
