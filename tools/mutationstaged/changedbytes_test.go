package main

import (
	"reflect"
	"testing"
)

const stagedDiff = `diff --git a/internal/pkg/thing.go b/internal/pkg/thing.go
--- a/internal/pkg/thing.go
+++ b/internal/pkg/thing.go
@@ -3,0 +4,2 @@ package pkg
+// added
+var added = 1
@@ -20,1 +22,1 @@ func other() {
-old := 1
+replaced := 2
`

func TestParseChangedLineRangesReadsEveryHunk(t *testing.T) {
	t.Parallel()

	got, err := parseChangedLineRanges(stagedDiff)
	if err != nil {
		t.Fatalf("parse changed ranges: %v", err)
	}
	want := map[string][]lineRange{
		"internal/pkg/thing.go": {{first: 4, last: 5}, {first: 22, last: 22}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ranges = %#v, want %#v", got, want)
	}
}

func TestParseChangedLineRangesCoversPureDeletionJoin(t *testing.T) {
	t.Parallel()

	diff := "+++ b/a.go\n@@ -9,2 +8,0 @@\n-old\n-lines\n"
	got, err := parseChangedLineRanges(diff)
	if err != nil {
		t.Fatalf("parse changed ranges: %v", err)
	}
	want := map[string][]lineRange{"a.go": {{first: 8, last: 9}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ranges = %#v, want %#v", got, want)
	}
}

func TestParseChangedLineRangesRejectsHunkWithoutFile(t *testing.T) {
	t.Parallel()

	if _, err := parseChangedLineRanges("@@ -1 +1 @@\n"); err == nil {
		t.Fatal("expected hunk without file header to fail")
	}
}

func TestLineRangesToOffsetsUseIndexBytes(t *testing.T) {
	t.Parallel()

	content := []byte("package p\n\nvar answer = 1\n")
	got := lineRangesToOffsets(content, []lineRange{{first: 3, last: 3}})
	want := []offsetRange{{start: 11, end: len(content)}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("offsets = %#v, want %#v", got, want)
	}
}

func TestLineRangesToOffsetsClampAndDropOutOfBounds(t *testing.T) {
	t.Parallel()

	content := []byte("package p\n")
	if got := lineRangesToOffsets(content, []lineRange{{first: 1, last: 99}}); !reflect.DeepEqual(got, []offsetRange{{0, len(content)}}) {
		t.Fatalf("clamped offsets = %#v", got)
	}
	if got := lineRangesToOffsets(content, []lineRange{{first: 50, last: 60}}); len(got) != 0 {
		t.Fatalf("past-end offsets = %#v, want none", got)
	}
}

func TestMergeAndEncodeOffsetRanges(t *testing.T) {
	t.Parallel()

	got := mergeOffsetRanges([]offsetRange{{40, 50}, {0, 10}, {8, 20}, {20, 30}})
	want := []offsetRange{{0, 30}, {40, 50}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merged = %#v, want %#v", got, want)
	}
	if encoded := encodeOffsetRanges(got); encoded != "0-30,40-50" {
		t.Fatalf("encoded = %q", encoded)
	}
}
