package staged

import (
	"errors"
	"testing"
)

// TestParseHunkRangesReadsMultipleHunksAndFiles pins the ordinary shape: a
// -U0 diff with more than one hunk in a file, and more than one file, each
// range attached to the file whose own "+++ b/" line preceded it.
func TestParseHunkRangesReadsMultipleHunksAndFiles(t *testing.T) {
	diff := []byte(`diff --git a/src/a.ts b/src/a.ts
--- a/src/a.ts
+++ b/src/a.ts
@@ -5,0 +6,2 @@ function a() {
+line6
+line7
@@ -20,0 +23 @@ function b() {
+line23
diff --git a/src/b.ts b/src/b.ts
--- a/src/b.ts
+++ b/src/b.ts
@@ -1,0 +2 @@
+line2
`)

	ranges, err := parseHunkRanges(diff)
	if err != nil {
		t.Fatalf("parseHunkRanges() = %v", err)
	}

	wantA := []lineRange{{start: 6, end: 7}, {start: 23, end: 23}}
	if got := ranges["src/a.ts"]; !equalRanges(got, wantA) {
		t.Errorf("ranges[src/a.ts] = %v, want %v", got, wantA)
	}
	wantB := []lineRange{{start: 2, end: 2}}
	if got := ranges["src/b.ts"]; !equalRanges(got, wantB) {
		t.Errorf("ranges[src/b.ts] = %v, want %v", got, wantB)
	}
}

// TestParseHunkRangesSkipsAZeroCount pins "count 0 contributes nothing": a
// hunk whose new side is explicitly ",0" is a pure deletion at that point in
// the file, and must not become a range asking dharness to mutate a line that
// was never added.
func TestParseHunkRangesSkipsAZeroCount(t *testing.T) {
	diff := []byte(`diff --git a/src/a.ts b/src/a.ts
--- a/src/a.ts
+++ b/src/a.ts
@@ -2 +1,0 @@ l1
-l2
`)

	ranges, err := parseHunkRanges(diff)
	if err != nil {
		t.Fatalf("parseHunkRanges() = %v", err)
	}
	if len(ranges["src/a.ts"]) != 0 {
		t.Errorf("ranges[src/a.ts] = %v, want none: the hunk added nothing", ranges["src/a.ts"])
	}
}

// TestParseHunkRangesIsEmptyForADeletionOnlyDiff covers the whole-diff shape
// of the same case: a file with only removed lines produces no file entry
// worth mutating, and no error either — deleting code is not a parsing
// failure.
func TestParseHunkRangesIsEmptyForADeletionOnlyDiff(t *testing.T) {
	diff := []byte(`diff --git a/src/a.ts b/src/a.ts
--- a/src/a.ts
+++ b/src/a.ts
@@ -2 +1,0 @@ l1
-l2
@@ -5 +3,0 @@ l4
-l5
`)

	ranges, err := parseHunkRanges(diff)
	if err != nil {
		t.Fatalf("parseHunkRanges() = %v", err)
	}
	if len(ranges["src/a.ts"]) != 0 {
		t.Errorf("ranges[src/a.ts] = %v, want none", ranges["src/a.ts"])
	}
}

// TestParseHunkRangesContinuesScanningAfterAZeroCountHunk pins the scan loop
// itself: skipping a zero-count hunk must not stop the scan, or every hunk
// and every file after it in the same diff would silently disappear.
func TestParseHunkRangesContinuesScanningAfterAZeroCountHunk(t *testing.T) {
	diff := []byte(`diff --git a/src/a.ts b/src/a.ts
--- a/src/a.ts
+++ b/src/a.ts
@@ -2 +1,0 @@ l1
-l2
@@ -5,0 +5 @@ l4
+l5
`)

	ranges, err := parseHunkRanges(diff)
	if err != nil {
		t.Fatalf("parseHunkRanges() = %v", err)
	}
	want := []lineRange{{start: 5, end: 5}}
	if got := ranges["src/a.ts"]; !equalRanges(got, want) {
		t.Errorf("ranges[src/a.ts] = %v, want %v: the hunk after the zero-count one was never reached", got, want)
	}
}

// TestParseNumstatAddedReadsAPlainEntry pins the common shape: <added> TAB
// <deleted> TAB <path>, no rename involved.
func TestParseNumstatAddedReadsAPlainEntry(t *testing.T) {
	numstat := []byte("3\t1\tsrc/a.ts\x00")

	added, err := parseNumstatAdded(numstat)
	if err != nil {
		t.Fatalf("parseNumstatAdded() = %v", err)
	}
	if added["src/a.ts"] != 3 {
		t.Errorf("added[src/a.ts] = %d, want 3", added["src/a.ts"])
	}
}

// TestParseNumstatAddedReadsARename pins the shape measured against a real
// repository: a rename's own numstat record carries an EMPTY inline path
// field, followed by the old and new paths as their own NUL-terminated
// fields — "1\t0\t\x00old.txt\x00new.txt\x00" — rather than the "old => new"
// arrow notation numstat prints without -z.
func TestParseNumstatAddedReadsARename(t *testing.T) {
	numstat := []byte("1\t0\t\x00old.txt\x00new.txt\x00")

	added, err := parseNumstatAdded(numstat)
	if err != nil {
		t.Fatalf("parseNumstatAdded() = %v", err)
	}
	if added["new.txt"] != 1 {
		t.Errorf("added[new.txt] = %d, want 1: keyed by the destination path, the same one parseHunkRanges uses", added["new.txt"])
	}
	if _, ok := added["old.txt"]; ok {
		t.Errorf("added[old.txt] present, want only the destination path counted")
	}
}

// TestParseNumstatAddedSkipsABinaryEntry pins the "-" shape numstat reports
// for a binary file: there is nothing to cross-check a binary file against,
// since it never produces a hunk either. The binary entry comes first and a
// real entry follows, so skipping it has to continue the scan rather than
// end it.
func TestParseNumstatAddedSkipsABinaryEntry(t *testing.T) {
	numstat := []byte("-\t-\tsrc/image.png\x00" + "2\t0\tsrc/a.ts\x00")

	added, err := parseNumstatAdded(numstat)
	if err != nil {
		t.Fatalf("parseNumstatAdded() = %v", err)
	}
	if _, ok := added["src/image.png"]; ok {
		t.Errorf("added[src/image.png] present, want a binary entry skipped entirely")
	}
	if added["src/a.ts"] != 2 {
		t.Errorf("added[src/a.ts] = %d, want 2: the entry after the skipped binary one was never reached", added["src/a.ts"])
	}
}

// TestParseNumstatAddedChecksTheAddedCountNotTheDeletedCount pins which field
// gates the binary skip: a real file can carry "-" as one count only in a
// contrived record, but the two fields are not interchangeable — only the
// added count decides whether a record is skipped.
func TestParseNumstatAddedChecksTheAddedCountNotTheDeletedCount(t *testing.T) {
	added, err := parseNumstatAdded([]byte("-\t3\tsrc/a.ts\x00" + "3\t-\tsrc/b.ts\x00"))
	if err != nil {
		t.Fatalf("parseNumstatAdded() = %v", err)
	}
	if _, ok := added["src/a.ts"]; ok {
		t.Errorf("added[src/a.ts] present, want skipped: its added count is \"-\"")
	}
	if added["src/b.ts"] != 3 {
		t.Errorf("added[src/b.ts] = %d, want 3: its added count is a real number, its deleted count being \"-\" does not matter", added["src/b.ts"])
	}
}

// TestParseNumstatAddedFailsOnATruncatedRenameRecord pins the exact boundary
// of the rename look-ahead: a rename record with its old path but no new path
// after it is one field short, and has to be refused rather than read past
// the end of the field slice or silently misattributed.
func TestParseNumstatAddedFailsOnATruncatedRenameRecord(t *testing.T) {
	_, err := parseNumstatAdded([]byte("1\t0\t\x00old.txt\x00"))
	if err == nil {
		t.Fatal("parseNumstatAdded() = nil, want an error: the rename's new path is missing")
	}
}

// TestParseNumstatAddedContinuesAfterARename pins the loop-index arithmetic
// directly: consuming a rename's two extra fields must ADD to the index the
// loop was already at, not reset it — a preceding record puts that index
// above zero, where "i += 2" and "assign the index to 2" first disagree.
func TestParseNumstatAddedContinuesAfterARename(t *testing.T) {
	numstat := []byte("5\t0\tsrc/pre.ts\x00" + "1\t0\t\x00old.txt\x00new.txt\x00" + "2\t0\tsrc/b.ts\x00")

	added, err := parseNumstatAdded(numstat)
	if err != nil {
		t.Fatalf("parseNumstatAdded() = %v", err)
	}
	if added["src/pre.ts"] != 5 {
		t.Errorf("added[src/pre.ts] = %d, want 5", added["src/pre.ts"])
	}
	if added["new.txt"] != 1 {
		t.Errorf("added[new.txt] = %d, want 1", added["new.txt"])
	}
	if added["src/b.ts"] != 2 {
		t.Errorf("added[src/b.ts] = %d, want 2: the record after the rename was not reached correctly", added["src/b.ts"])
	}
}

// TestNumstatFieldsDistinguishesEmptyFromOneByte pins the exact boundary of
// the trailing-terminator trim: an input that is nothing but the terminator
// must report no fields at all, one byte away from an input holding exactly
// one field.
func TestNumstatFieldsDistinguishesEmptyFromOneByte(t *testing.T) {
	if got := numstatFields([]byte{0}); got != nil {
		t.Errorf("numstatFields([]byte{0}) = %v, want nil", got)
	}
	if got := numstatFields([]byte("a")); len(got) != 1 || got[0] != "a" {
		t.Errorf(`numstatFields([]byte("a")) = %v, want ["a"]`, got)
	}
}

// TestScopeFailsClosedWhenNumstatDisagreesWithTheParsedHunks pins the safety
// net directly: a numstat record claiming added lines for a file the hunk
// parser found no range for is a parsing gap, not a file with nothing to
// scope, and must refuse rather than silently return an empty scope.
func TestScopeFailsClosedWhenNumstatDisagreesWithTheParsedHunks(t *testing.T) {
	defer SetGitOutputForTest(func(_ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[len(args)-1] == "--diff-filter=ACMR" {
			// A diff with no recognisable hunk at all for src/a.ts, as if the
			// parser had a gap for whatever shape produced this file's hunks.
			return []byte("diff --git a/src/a.ts b/src/a.ts\n--- a/src/a.ts\n+++ b/src/a.ts\n"), nil
		}
		// Exactly one added line: the boundary between "gained lines" and
		// "gained none", not two, which would also pass a boundary shifted by
		// one in either direction.
		return []byte("1\t0\tsrc/a.ts\x00"), nil
	})()

	_, err := Scope(t.TempDir(), nil)

	var unparsed *UnparsedAddedLinesError
	if !errors.As(err, &unparsed) {
		t.Fatalf("Scope() = %v, want UnparsedAddedLinesError", err)
	}
	if unparsed.Path != "src/a.ts" {
		t.Errorf("UnparsedAddedLinesError.Path = %q, want %q", unparsed.Path, "src/a.ts")
	}
}

// TestScopeFailsClosedWhenAQuotedPathDefeatsTheDiffParser pins the refusal
// for a C-quoted path: git C-quotes a whole pathname — wrapping it in double quotes
// and backslash-escaping what is inside — for a double quote, a tab, or any
// other control character, and it does this regardless of
// core.quotePath=false: that setting only ever suppresses quoting for
// non-ASCII UTF-8 bytes. A hunk header for such a path reads
// `+++ "b/src/a\"b.ts"` rather than diffNewPath's own literal `+++ b/`, so it
// never matches, and the hunk that follows is attributed to whatever path
// preceded it in the diff instead. numstat's own -z output is unaffected —
// documented to print pathnames as bare bytes, NUL-terminated, with no
// quoting at all when -z accompanies --numstat — so the cross-check between
// the two sees the quoted file gained a line no range was ever recorded for,
// and refuses with UnparsedAddedLinesError exactly as it does for any other
// parsing gap.
func TestScopeFailsClosedWhenAQuotedPathDefeatsTheDiffParser(t *testing.T) {
	defer SetGitOutputForTest(func(_ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[len(args)-1] == "--diff-filter=ACMR" {
			return []byte(`diff --git a/src/first.ts b/src/first.ts
--- a/src/first.ts
+++ b/src/first.ts
@@ -0,0 +1 @@
+export const first = 1;
diff --git "a/src/a\"b.ts" "b/src/a\"b.ts"
--- "a/src/a\"b.ts"
+++ "b/src/a\"b.ts"
@@ -0,0 +1 @@
+export const a = 1;
`), nil
		}
		return []byte("1\t0\tsrc/first.ts\x00" + "1\t0\tsrc/a\"b.ts\x00"), nil
	})()

	_, err := Scope(t.TempDir(), nil)

	var unparsed *UnparsedAddedLinesError
	if !errors.As(err, &unparsed) {
		t.Fatalf("Scope() = %v, want UnparsedAddedLinesError", err)
	}
	if unparsed.Path != `src/a"b.ts` {
		t.Errorf("UnparsedAddedLinesError.Path = %q, want %q", unparsed.Path, `src/a"b.ts`)
	}
}

// TestScopeSkipsAnExcludedFileWithoutStoppingTheScan pins Scope's own
// filtering loop: an excluded file is sorted before an included one here
// specifically so skipping it cannot be mistaken for stopping — Scope sorts
// paths before iterating, and "AAA.md" sorts before "src/a.ts".
func TestScopeSkipsAnExcludedFileWithoutStoppingTheScan(t *testing.T) {
	defer SetGitOutputForTest(func(_ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[len(args)-1] == "--diff-filter=ACMR" {
			return []byte(`diff --git a/AAA.md b/AAA.md
--- a/AAA.md
+++ b/AAA.md
@@ -0,0 +1 @@
+not source
diff --git a/src/a.ts b/src/a.ts
--- a/src/a.ts
+++ b/src/a.ts
@@ -0,0 +1 @@
+export const a = 1;
`), nil
		}
		return []byte("1\t0\tAAA.md\x00" + "1\t0\tsrc/a.ts\x00"), nil
	})()
	defer stubNoUnstagedChanges()()

	scopes, err := Scope(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("Scope() = %v", err)
	}
	if len(scopes) != 1 || scopes[0].Argument() != "src/a.ts:1-1" {
		t.Errorf("Scope() = %v, want exactly [src/a.ts:1-1]: skipping AAA.md must not stop the scan before src/a.ts", scopes)
	}
}

// stubNoUnstagedChanges answers checkFullyStaged's own probe with "nothing
// unstaged", so a Scope-level test can exercise its own concern (filtering,
// the fail-closed cross-check) without also having to fake a real partial-
// staging answer for every path it names.
func stubNoUnstagedChanges() func() {
	previous := gitOutput
	gitOutput = func(dir string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "diff" && args[1] == "--name-only" {
			return nil, nil
		}
		return previous(dir, args...)
	}
	return func() { gitOutput = previous }
}

// TestCheckFullyStagedRefusesTheShortestNonEmptyAnswer pins the exact
// boundary checkFullyStaged reads: git's own answer is either fully empty
// (nothing unstaged) or names at least one path, and there is no shape of
// "a little unstaged" in between for a boundary elsewhere to hide behind.
func TestCheckFullyStagedRefusesTheShortestNonEmptyAnswer(t *testing.T) {
	defer SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		return []byte("x"), nil
	})()

	err := checkFullyStaged(t.TempDir(), []string{"src/a.ts"})

	var partial *PartiallyStagedError
	if !errors.As(err, &partial) {
		t.Fatalf("checkFullyStaged() = %v, want PartiallyStagedError", err)
	}
}

// TestCheckFullyStagedPassesOnAnEmptyAnswer is the other side of the same
// boundary: git reporting nothing unstaged must not be refused.
func TestCheckFullyStagedPassesOnAnEmptyAnswer(t *testing.T) {
	defer SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		return nil, nil
	})()

	if err := checkFullyStaged(t.TempDir(), []string{"src/a.ts"}); err != nil {
		t.Errorf("checkFullyStaged() = %v, want nil", err)
	}
}

// TestSplitNULDropsEmptyFieldsAndKeepsScanning pins splitNUL directly: empty
// fields between real ones are dropped, and dropping one must not stop the
// scan before the fields that follow it.
func TestSplitNULDropsEmptyFieldsAndKeepsScanning(t *testing.T) {
	got := splitNUL([]byte("\x00a\x00\x00b\x00"))
	want := []string{"a", "b"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("splitNUL() = %v, want %v", got, want)
	}
}

// TestIncludedFiltersByExtensionTestNameAndExcludePrefix pins the filtering
// contract directly: project.IsSourceFile, the three test-file conventions,
// and a caller-supplied exclude prefix.
func TestIncludedFiltersByExtensionTestNameAndExcludePrefix(t *testing.T) {
	cases := []struct {
		path            string
		excludePrefixes []string
		want            bool
	}{
		{"src/a.ts", nil, true},
		{"src/a.md", nil, false},
		{"src/a.test.ts", nil, false},
		{"src/a.spec.ts", nil, false},
		{"src/__tests__/a.ts", nil, false},
		{"tools/generated/a.ts", []string{"tools/"}, false},
		{"src/a.ts", []string{"tools/"}, true},
		// project.IsSourceFile does not distinguish a declaration file
		// from an ordinary one — filepath.Ext("env.d.ts") is ".ts", exactly
		// what an ordinary source file's own extension is — so included()
		// has to exclude the three declaration suffixes itself. A
		// declaration file can carry no runtime code, and entering scope
		// would disengage the types-only classifier for the whole commit: tsc emits
		// no output for a .d.ts either, which reads identically to "the
		// classifier could not run".
		{"src/env.d.ts", nil, false},
		{"src/env.d.mts", nil, false},
		{"src/env.d.cts", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			if got := included(tc.path, tc.excludePrefixes); got != tc.want {
				t.Errorf("included(%q, %v) = %v, want %v", tc.path, tc.excludePrefixes, got, tc.want)
			}
		})
	}
}

func equalRanges(got, want []lineRange) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
