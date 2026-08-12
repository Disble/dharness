package report

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestWidthsIsTheMaxPerColumnMeasuredInRunes pins design.md's "column rule,
// not spacing" testing-strategy row: widths measures runes, not bytes, so a
// multi-byte glyph column and a single-byte column of the same visual width
// report the same width. Never asserts on a rendered line's spacing.
func TestWidthsIsTheMaxPerColumnMeasuredInRunes(t *testing.T) {
	rows := [][]string{
		{"──", "a"},
		{"xx", "bb"},
	}

	got := widths(rows)
	want := []int{2, 2}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("widths(%v) = %v, want %v (── is 2 runes/6 bytes; a byte-based bug would report 6)", rows, got, want)
	}
}

// TestWidthsMaxComparisonSurvivesASecondWiderRow kills the mutant that
// drops or inverts the max comparison (design.md Mutation coverage table,
// 4th named branch): a two-row table whose second row is wider in every
// column must report the second row's measurement.
func TestWidthsMaxComparisonSurvivesASecondWiderRow(t *testing.T) {
	rows := [][]string{
		{"a", "bb"},
		{"ccc", "dddd"},
	}

	got := widths(rows)
	want := []int{3, 4}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("widths(%v) = %v, want %v", rows, got, want)
	}
}

// TestWidthsColumnOfEmptyCellsReportsZero pins a newly-seen column's
// starting width to zero: a column every row leaves empty must report width
// 0, not a nonzero placeholder that never gets measured down. A mutant that
// seeds a newly-grown column above zero survives every other widths case
// here, because any nonempty cell overwrites a too-low seed regardless —
// only an always-empty column ever observes the seed itself.
func TestWidthsColumnOfEmptyCellsReportsZero(t *testing.T) {
	rows := [][]string{
		{"a", ""},
		{"bb", ""},
	}

	got := widths(rows)
	want := []int{2, 0}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("widths(%v) = %v, want %v", rows, got, want)
	}
}

// TestWrapNeverExceedsWidthAndPreservesWords pins "the bound, not the break
// points": no returned line exceeds width, and the words rejoined from the
// output equal the input's words, in order. The "candidate exactly at
// width" case is a boundary: a mutant on <= vs < dies there, because a
// wrongly-strict comparison would split a candidate that fits exactly.
func TestWrapNeverExceedsWidthAndPreservesWords(t *testing.T) {
	cases := []struct {
		name      string
		text      string
		width     int
		indent    int
		wantLines int // -1 means "do not assert on line count"
	}{
		{"short text fits one line", "one two three", 20, 0, 1},
		{"wraps across several lines", "alpha beta gamma delta epsilon zeta eta theta", 12, 2, -1},
		{"candidate exactly at width fits without wrapping", "aaa bbbbbb", 10, 0, 1},
		{"hanging indent narrows continuation lines", "install what this project is missing here", 15, 4, -1},
		{"empty text produces no lines", "", 20, 0, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines := wrap(tc.text, tc.width, tc.indent)

			for _, line := range lines {
				if n := utf8.RuneCountInString(line); n > tc.width {
					t.Errorf("wrap(%q, %d, %d) produced line %q of %d runes, want <= %d",
						tc.text, tc.width, tc.indent, line, n, tc.width)
				}
			}

			var got []string
			for _, line := range lines {
				got = append(got, strings.Fields(line)...)
			}
			want := strings.Fields(tc.text)
			if len(got) != 0 || len(want) != 0 {
				if !reflect.DeepEqual(got, want) {
					t.Errorf("wrap(%q, %d, %d) words = %v, want %v", tc.text, tc.width, tc.indent, got, want)
				}
			}

			if tc.wantLines >= 0 && len(lines) != tc.wantLines {
				t.Errorf("wrap(%q, %d, %d) = %d lines %v, want %d lines",
					tc.text, tc.width, tc.indent, len(lines), lines, tc.wantLines)
			}
		})
	}
}

// TestWriteHumanRendersEverySummaryFirst pins "the summary is rendered
// before per-step detail" (both the applied and satisfied scenarios): every
// step's ID appears in the output, and the summary marker appears before
// the first per-step detail block's marker. It also pins every fact each
// block carries for its step — elapsed time, transcript, every installed
// package and every touched file (not merely the first of each, which a
// range-break defect would leave silently dropped), and a delegated step's
// reason — so a per-step field silently going unrendered fails here rather
// than surviving as an untested branch.
func TestWriteHumanRendersEverySummaryFirst(t *testing.T) {
	r := Report{
		Summary: Summary{Steps: 3, Applied: 1, Delegated: 1, Satisfied: 1},
		Steps: []StepResult{
			{
				ID:         "install what this project is missing",
				Status:     Applied,
				MS:         5560,
				Transcript: "bun add dharness-eslint-plugin",
				Installed:  []string{"dharness-eslint-plugin@0.3.0", "dharness-shared-config@1.0.0"},
				Wrote: []FileChange{
					{Path: "frontend/package.json", Kind: Modified},
					{Path: "frontend/bun.lock", Kind: Modified},
				},
			},
			{ID: "resolve the keys this project and dharness both declare", Status: Delegated, Why: "two owners"},
			{ID: "point .fallowrc.json at the file dharness owns", Status: Satisfied, Evidence: "extends → .dharness/fallow.jsonc"},
		},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()

	for _, step := range r.Steps {
		if !strings.Contains(out, step.ID) {
			t.Errorf("output missing step ID %q:\n%s", step.ID, out)
		}
	}

	// The expected text is a hardcoded literal, not formatMS(5560): calling
	// the function under test to build its own expectation would make this
	// assertion pass even if formatMS's own arithmetic were wrong, since
	// both sides would be wrong the same way.
	if !strings.Contains(out, "5.56s") {
		t.Errorf("output missing the applied step's elapsed time %q:\n%s", "5.56s", out)
	}
	if !strings.Contains(out, r.Steps[0].Transcript) {
		t.Errorf("output missing the applied step's transcript:\n%s", out)
	}
	for _, installed := range r.Steps[0].Installed {
		if !strings.Contains(out, installed) {
			t.Errorf("output missing installed package %q (every entry must render, not only the first):\n%s", installed, out)
		}
	}
	for _, change := range r.Steps[0].Wrote {
		if !strings.Contains(out, change.Path) {
			t.Errorf("output missing touched file %q (every entry must render, not only the first):\n%s", change.Path, out)
		}
	}
	if !strings.Contains(out, r.Steps[1].Why) {
		t.Errorf("output missing the delegated step's reason %q:\n%s", r.Steps[1].Why, out)
	}
	if !strings.Contains(out, r.Steps[2].Evidence) {
		t.Errorf("output missing the satisfied step's evidence %q:\n%s", r.Steps[2].Evidence, out)
	}

	summaryAt := strings.Index(out, glyphSummary)
	firstDetailAt := strings.Index(out, "Applied")
	if summaryAt < 0 {
		t.Fatalf("summary marker %q not found in output:\n%s", glyphSummary, out)
	}
	if firstDetailAt < 0 {
		t.Fatalf("first detail block marker %q not found in output:\n%s", "Applied", out)
	}
	if summaryAt >= firstDetailAt {
		t.Errorf("summary marker at byte %d, detail block marker at byte %d — summary must come first:\n%s",
			summaryAt, firstDetailAt, out)
	}
}

// TestWriteHumanCollisionKeyRendersExactlyOnce pins config-collision's "one
// colliding key renders exactly once in the human view" scenario at the
// renderer layer — a mutant reviving a second, independent walk over the
// colliding key would print it twice.
func TestWriteHumanCollisionKeyRendersExactlyOnce(t *testing.T) {
	r := Report{
		Summary: Summary{Steps: 1, Delegated: 1},
		Steps: []StepResult{
			{
				ID:         "resolve the keys this project and dharness both declare",
				Status:     Delegated,
				Collisions: []Collision{{Key: "duplicates"}},
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()

	if got := strings.Count(out, "duplicates"); got != 1 {
		t.Errorf("strings.Count(output, %q) = %d, want 1:\n%s", "duplicates", got, out)
	}
}

// TestWriteHumanResidueNoteListsEveryEntryAndNoFlagRef pins "notes list
// their entries in full and never reference an unimplemented flag" — the
// resolved question round rejected a --show residue flag. The reason text is
// long enough to wrap into several lines, so every word of it must survive
// the wrap — a range-break defect over the wrapped lines would silently
// drop everything after the first line.
func TestWriteHumanResidueNoteListsEveryEntryAndNoFlagRef(t *testing.T) {
	entries := []string{"dharness-eslint-plugin", "dharness/max-file-lines"}
	reason := "react-doctor runs with --staged and plugin rules simply do not fire " +
		"under that flag, which is why these entries stay inert rather than actionable"
	r := Report{
		Summary: Summary{Steps: 0},
		Notes: []Note{
			{
				Kind:    "residue",
				Path:    "frontend/doctor.config.json",
				Entries: entries,
				Reason:  reason,
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()

	for _, entry := range entries {
		if !strings.Contains(out, entry) {
			t.Errorf("output missing residue entry %q:\n%s", entry, out)
		}
	}
	for _, word := range strings.Fields(reason) {
		if !strings.Contains(out, word) {
			t.Errorf("output missing reason word %q — the wrapped reason must survive in full, not just its first line:\n%s", word, out)
		}
	}
	if strings.Contains(out, "--show") {
		t.Errorf("output references an unimplemented flag:\n%s", out)
	}
}

// TestWriteHumanOmitsEmptyBlockHeadings pins that a block with zero steps in
// its status is omitted entirely rather than printed with an empty body —
// an absent block being ambiguous with an empty one is exactly the kind of
// silent gap this report exists to end.
func TestWriteHumanOmitsEmptyBlockHeadings(t *testing.T) {
	cases := []struct {
		name    string
		steps   []StepResult
		absent  []string
		present string
	}{
		{
			name:    "only an applied step is present",
			steps:   []StepResult{{ID: "install", Status: Applied}},
			absent:  []string{"Left to you", "Already in place"},
			present: "Applied",
		},
		{
			name:    "only a delegated step is present",
			steps:   []StepResult{{ID: "resolve", Status: Delegated, Why: "needs a human"}},
			absent:  []string{"Applied", "Already in place"},
			present: "Left to you",
		},
		{
			name:    "only a satisfied step is present",
			steps:   []StepResult{{ID: "already done", Status: Satisfied, Evidence: "found"}},
			absent:  []string{"Applied", "Left to you"},
			present: "Already in place",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Report{Steps: tc.steps}

			var buf bytes.Buffer
			if err := WriteHuman(&buf, r); err != nil {
				t.Fatalf("WriteHuman() = %v", err)
			}
			out := buf.String()

			if !strings.Contains(out, tc.present) {
				t.Errorf("output missing the one block that should render, %q:\n%s", tc.present, out)
			}
			for _, heading := range tc.absent {
				if strings.Contains(out, heading) {
					t.Errorf("output renders an empty block heading %q instead of omitting it:\n%s", heading, out)
				}
			}
			if strings.Contains(out, "Notes") {
				t.Errorf("output renders a Notes heading with zero notes:\n%s", out)
			}
		})
	}
}

// TestWriteClosingBlockGlyphReflectsFailure pins the closing block's glyph
// choice to the actual failure count, not a hardcoded success mark: a run
// with a failed step must not close on the same glyph as a clean run.
func TestWriteClosingBlockGlyphReflectsFailure(t *testing.T) {
	cases := []struct {
		name       string
		failed     int
		wantGlyph  string
		otherGlyph string
	}{
		{"a clean run closes on the applied glyph", 0, glyphApplied, glyphFailed},
		{"a run with a failure closes on the failed glyph", 1, glyphFailed, glyphApplied},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Report{Summary: Summary{Failed: tc.failed}}

			var buf bytes.Buffer
			if err := WriteHuman(&buf, r); err != nil {
				t.Fatalf("WriteHuman() = %v", err)
			}
			out := buf.String()

			ruleAt := strings.LastIndex(out, strings.Repeat(glyphRule, wrapWidth))
			if ruleAt < 0 {
				t.Fatalf("closing separator not found in output:\n%s", out)
			}
			closing := out[ruleAt:]
			if !strings.Contains(closing, tc.wantGlyph) {
				t.Errorf("closing block missing glyph %q for %d failed step(s):\n%s", tc.wantGlyph, tc.failed, closing)
			}
			if strings.Contains(closing, tc.otherGlyph) {
				t.Errorf("closing block carries the wrong glyph %q for %d failed step(s):\n%s", tc.otherGlyph, tc.failed, closing)
			}
		})
	}
}

// TestWriteClosingBlockRuleSpansWrapWidth pins the closing separator to the
// same fixed 70-column width design.md Decision 7 sets for wrapping. The
// expected width is the hardcoded literal 70, not the wrapWidth constant:
// building the expectation from the same constant the production code reads
// would pass even if that constant's value were wrong, since both sides
// would move together.
func TestWriteClosingBlockRuleSpansWrapWidth(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteHuman(&buf, Report{}); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}

	// The line's exact rune count is asserted, not mere containment: a rule
	// one rune too long still *contains* a 70-rune run of the same
	// repeated character, so a substring check alone cannot tell 70 from
	// 71 when every rune on the line is identical.
	const wantWidth = 70
	var ruleLine string
	found := false
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.HasPrefix(line, glyphRule) {
			ruleLine = line
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no closing separator line found in output:\n%s", buf.String())
	}
	if n := utf8.RuneCountInString(ruleLine); n != wantWidth {
		t.Errorf("closing separator is %d runes wide, want exactly %d:\n%s", n, wantWidth, buf.String())
	}
}

// TestWriteHumanAlignsEvidenceColumnAcrossDifferentIDLengths pins the
// column rule at the renderer layer: two satisfied steps whose IDs differ
// in length must still have their evidence values start at the same
// column, because widths() reports the max ID length and %-*s pads every
// row to it. A mutant swapping which column's width feeds the padding
// would misalign this pair of rows without exceeding either row's own
// column count, which is why the check is a structural offset comparison,
// not a fixed spacing string.
func TestWriteHumanAlignsEvidenceColumnAcrossDifferentIDLengths(t *testing.T) {
	r := Report{
		Steps: []StepResult{
			{ID: "x", Status: Satisfied, Evidence: "P"},
			{ID: "xxxxx", Status: Satisfied, Evidence: "Q"},
		},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()

	lines := strings.Split(out, "\n")
	var offsets []int
	for _, evidence := range []string{"P", "Q"} {
		found := false
		for _, line := range lines {
			if strings.HasSuffix(line, evidence) && strings.Contains(line, "x") {
				offsets = append(offsets, strings.LastIndex(line, evidence))
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("evidence %q not found on its own satisfied-step line:\n%s", evidence, out)
		}
	}

	if offsets[0] != offsets[1] {
		t.Errorf("evidence columns misaligned across rows of different ID length: %v\n%s", offsets, out)
	}
}

// TestWriteHumanAlignsElapsedTimeColumnAcrossDifferentIDLengths mirrors
// TestWriteHumanAlignsEvidenceColumnAcrossDifferentIDLengths for the applied
// block: the elapsed-time column must align across rows whose IDs differ in
// length, which only holds when the ID column is padded to the max *ID*
// length — a mutant padding to the max elapsed-time width instead would
// still align these two rows if neither ID exceeded that (short) width, so
// the longer ID here is deliberately longer than the fixed 5-rune
// "D.DDs" elapsed-time text.
func TestWriteHumanAlignsElapsedTimeColumnAcrossDifferentIDLengths(t *testing.T) {
	r := Report{
		Steps: []StepResult{
			{ID: "x", Status: Applied, MS: 100},
			{ID: "xxxxxxxxxx", Status: Applied, MS: 200},
		},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()
	lines := strings.Split(out, "\n")

	offsetOf := func(elapsed string) int {
		for _, line := range lines {
			if strings.HasSuffix(line, elapsed) {
				return strings.LastIndex(line, elapsed)
			}
		}
		t.Fatalf("elapsed time %q not found on its own line in output:\n%s", elapsed, out)
		return -1
	}

	first := offsetOf("0.10s")
	second := offsetOf("0.20s")
	if first != second {
		t.Errorf("elapsed-time columns misaligned across rows of different ID length: %d vs %d\n%s", first, second, out)
	}
}

// TestWriteHumanWrappedContinuationLinesCarryTheConfiguredIndent pins the
// hanging indent wrap() is given at both call sites in the delegated and
// notes blocks: a continuation line must be indented deeper than the first
// line by exactly the configured amount, which the block's own literal
// leading spaces alone cannot show because they apply to every line
// equally — only wrap()'s own indent parameter distinguishes first lines
// from continuations.
func TestWriteHumanWrappedContinuationLinesCarryTheConfiguredIndent(t *testing.T) {
	longText := strings.TrimSpace(strings.Repeat("alpha ", 20))

	cases := []struct {
		name   string
		report Report
		marker string
	}{
		{
			name:   "delegated block's why text",
			report: Report{Steps: []StepResult{{ID: "resolve", Status: Delegated, Why: longText}}},
			marker: " ! resolve",
		},
		{
			name:   "notes block's reason text",
			report: Report{Notes: []Note{{Kind: "residue", Reason: longText}}},
			marker: " i residue",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteHuman(&buf, tc.report); err != nil {
				t.Fatalf("WriteHuman() = %v", err)
			}

			var wrapped []string
			inBlock := false
			for _, line := range strings.Split(buf.String(), "\n") {
				switch {
				case line == tc.marker:
					inBlock = true
				case inBlock && strings.TrimSpace(line) == "":
					inBlock = false
				case inBlock:
					wrapped = append(wrapped, line)
				}
			}

			if len(wrapped) < 2 {
				t.Fatalf("expected the wrapped text to span at least 2 lines, got %d:\n%s", len(wrapped), buf.String())
			}

			leading := func(s string) int { return len(s) - len(strings.TrimLeft(s, " ")) }
			first, second := leading(wrapped[0]), leading(wrapped[1])
			if delta := second - first; delta != 3 {
				t.Errorf("continuation line indent delta = %d, want 3 (first line %d leading spaces, second %d):\n%v",
					delta, first, second, wrapped)
			}
		})
	}
}
