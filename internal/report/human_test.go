package report

import (
	"bytes"
	"encoding/json"
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
	// Checked word by word rather than as one contiguous substring: the
	// evidence column wraps within the fixed report width (gap 4), so a
	// long evidence value is not guaranteed to survive on one line — only
	// every word of it is.
	for _, word := range strings.Fields(r.Steps[2].Evidence) {
		if !strings.Contains(out, word) {
			t.Errorf("output missing evidence word %q from %q:\n%s", word, r.Steps[2].Evidence, out)
		}
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

// TestWriteHumanCollisionBlockShowsBothSidesAndResolutions pins gap 1 from
// the team lead's measured run: the collision block must carry both sides'
// value and location, which one measurement says runs, and the lettered
// resolutions the JSON's Resolutions slice already carries — not the bare
// "`key` needs a decision" placeholder. Asserted on presence of each fact,
// per spec.md's own testability note, never on exact spacing.
func TestWriteHumanCollisionBlockShowsBothSidesAndResolutions(t *testing.T) {
	ours := jsonRaw(`{"minOccurrences":3,"mode":"semantic","threshold":3}`)
	theirs := jsonRaw(`{"minOccurrences":2,"mode":"exact","threshold":5}`)
	effective := "theirs"
	r := Report{
		Steps: []StepResult{
			{
				ID:     "resolve the keys this project and dharness both declare",
				Status: Delegated,
				Collisions: []Collision{
					{
						ID:          "sync:collision/duplicates",
						Key:         "duplicates",
						Ours:        Declared{Path: ".dharness/fallow.jsonc", Line: 8, Value: &ours},
						Theirs:      Declared{Path: "frontend/.fallowrc.json", Line: 12, Value: &theirs},
						Effective:   &effective,
						Resolutions: []string{"delete-theirs", "move-into-ours"},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"sync:collision/duplicates",
		".dharness/fallow.jsonc",
		"8",
		"frontend/.fallowrc.json",
		"12",
		`"minOccurrences":3`,
		`"minOccurrences":2`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("collision block missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "runs") {
		t.Errorf("collision block does not say which side runs:\n%s", out)
	}
	flat := strings.ToLower(out)
	if !strings.Contains(flat, "delete") || !strings.Contains(flat, "frontend/.fallowrc.json") {
		t.Errorf("collision block does not render the delete-theirs resolution naming the project's file:\n%s", out)
	}
	if !strings.Contains(flat, "move") || !strings.Contains(out, ".dharness/fallow.jsonc") {
		t.Errorf("collision block does not render the move-into-ours resolution naming the owned file:\n%s", out)
	}
}

// TestWriteHumanCollisionValueWrapsWithinReportWidth pins the first defect
// found live during 4.22's follow-up verification: a colliding value can be
// hundreds of characters long — fallow's own resolved config for one key
// measured 345 characters against a real project, far wider than any other
// line the report prints — and an unwrapped value destroyed the whole
// block's layout. The fixed report width (wrapWidth) must bound every
// rendered line, including this one, exactly as it already bounds
// Why/Evidence/Reason.
func TestWriteHumanCollisionValueWrapsWithinReportWidth(t *testing.T) {
	// The exact value fallow's `--format json` returned for one colliding
	// key in the team lead's own reproduction — compact, with no internal
	// whitespace at all, so a plain word-wrap (which can only break at a
	// space) cannot bound it without help.
	theirs := jsonRaw(`{"enabled":true,"mode":"weak","near":false,"minTokens":50,"minLines":5,"minOccurrences":2,"threshold":5.0,"ignore":[],"ignoredClones":[],"ignoreDefaults":true,"skipLocal":false,"crossLanguage":false,"ignoreImports":true,"normalization":{},"minCorpusSizeForShingleFilter":1024,"minCorpusSizeForTokenCache":5000}`)
	if n := utf8.RuneCountInString(string(theirs)); n <= wrapWidth {
		t.Fatalf("fixture sanity check failed: value is %d runes, want more than wrapWidth (%d)", n, wrapWidth)
	}
	// dharness's own side is deliberately left unmeasured, so narrowing
	// cannot apply and this value reaches the renderer whole. Narrowing
	// (narrowToDifferences) would legitimately drop most of these keys —
	// they are fallow's defaults — and this test is not about that: it is
	// about a long value being bounded by the width and surviving the
	// wrap intact. Those are separate rules and they get separate
	// fixtures, so neither can silently start passing because the other
	// changed.
	r := Report{
		Steps: []StepResult{
			{
				ID:     "resolve the keys this project and dharness both declare",
				Status: Delegated,
				Collisions: []Collision{{
					ID:     "sync:collision/duplicates",
					Key:    "duplicates",
					Ours:   Declared{Path: ".dharness/fallow.jsonc", Line: 4},
					Theirs: Declared{Path: "frontend/.fallowrc.json", Line: 2, Value: &theirs},
				}},
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()

	for _, line := range strings.Split(out, "\n") {
		if n := utf8.RuneCountInString(line); n > wrapWidth {
			t.Errorf("line exceeds the fixed report width of %d runes (%d): %q", wrapWidth, n, line)
		}
	}
	if !strings.Contains(out, "minCorpusSizeForTokenCache") {
		t.Errorf("wrapped value lost content:\n%s", out)
	}
}

// TestEffectiveMarkStaysAttachedWhenTheValueWraps triangulates
// TestWriteHumanCollisionValueWrapsWithinReportWidth with the case that
// exercises the mark: "this one runs" must still appear exactly once, and
// it must read as attached to the value it marks — the value's own last
// wrapped line — rather than floating on a line of its own that a reader
// could mistake for marking the whole side. It must also still respect
// wrapWidth with the mark appended, not only without it.
func TestEffectiveMarkStaysAttachedWhenTheValueWraps(t *testing.T) {
	theirs := jsonRaw(`{"enabled":true,"mode":"weak","near":false,"minTokens":50,"minLines":5,"minOccurrences":2,"threshold":5.0,"ignore":[],"ignoredClones":[],"ignoreDefaults":true,"skipLocal":false,"crossLanguage":false,"ignoreImports":true,"normalization":{},"minCorpusSizeForShingleFilter":1024,"minCorpusSizeForTokenCache":5000}`)
	// Left unmeasured on dharness's side on purpose, so narrowing cannot
	// apply and the mark has a genuinely long, whole value to stay
	// attached to. Which keys narrowing keeps is a different rule with
	// its own test.
	effective := "theirs"
	r := Report{
		Steps: []StepResult{
			{
				ID:     "resolve the keys this project and dharness both declare",
				Status: Delegated,
				Collisions: []Collision{{
					ID:        "sync:collision/duplicates",
					Key:       "duplicates",
					Ours:      Declared{Path: ".dharness/fallow.jsonc"},
					Theirs:    Declared{Path: "frontend/.fallowrc.json", Value: &theirs},
					Effective: &effective,
				}},
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()

	if got := strings.Count(out, "this one runs"); got != 1 {
		t.Fatalf(`strings.Count(output, "this one runs") = %d, want exactly 1:%s`, got, out)
	}

	var markLine string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "this one runs") {
			markLine = line
			break
		}
	}
	if markLine == "" {
		t.Fatalf("mark line not found:\n%s", out)
	}
	// The value ends in "5000}" — if the mark is attached to the value's
	// own last wrapped line rather than floating on a line by itself, that
	// tail of the value and the mark appear together on the same line.
	if !strings.Contains(markLine, "5000}") {
		t.Errorf("mark is not attached to the value's last wrapped line, which should end \"5000}\":\n%s", markLine)
	}

	for _, line := range strings.Split(out, "\n") {
		if n := utf8.RuneCountInString(line); n > wrapWidth {
			t.Errorf("line exceeds the fixed report width of %d runes (%d) even with the mark attached: %q", wrapWidth, n, line)
		}
	}
}

// TestEffectiveMarkNarrowsWrapWidthToLeaveRoomForItself is a mutation guard
// for writeDeclaredSide's own width-narrowing branch (available -=
// utf8.RuneCountInString(mark)): the fixture in
// TestEffectiveMarkStaysAttachedWhenTheValueWraps never lands its last
// hard-split chunk close enough to the un-narrowed width to expose a mutant
// that skips the narrowing, so this pins the boundary directly. The value's
// length is chosen as an exact multiple of (wrapWidth - declaredSideIndent)
// — the width wrap would use if the mark were wrongly ignored — so an
// un-narrowed wrap produces a full-width final chunk that overflows the
// moment the mark is appended to it; the correctly narrowed width never
// produces a final chunk wide enough for that to happen, by construction.
func TestEffectiveMarkNarrowsWrapWidthToLeaveRoomForItself(t *testing.T) {
	const unnarrowedAvailable = wrapWidth - declaredSideIndent // 55
	value := strings.Repeat("a", unnarrowedAvailable*2)        // 110, a clean multiple
	theirs := jsonRaw(value)
	ours := jsonRaw(`{"minOccurrences":3}`)
	effective := "theirs"
	r := Report{
		Steps: []StepResult{
			{
				ID:     "resolve",
				Status: Delegated,
				Collisions: []Collision{{
					ID:        "sync:collision/duplicates",
					Key:       "duplicates",
					Ours:      Declared{Path: ".dharness/fallow.jsonc", Value: &ours},
					Theirs:    Declared{Path: "frontend/.fallowrc.json", Value: &theirs},
					Effective: &effective,
				}},
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()

	for _, line := range strings.Split(out, "\n") {
		if n := utf8.RuneCountInString(line); n > wrapWidth {
			t.Errorf("line exceeds the fixed report width of %d runes (%d): the mark's own width was not reserved for: %q", wrapWidth, n, line)
		}
	}
}

// TestWriteHumanLabelsTheProjectSideAsResolvedWhenMeasured pins the second
// defect found live during 4.22's follow-up verification: setup.Collisions
// always populates Theirs.Value from fallow's `--format json` output — the
// fully resolved config, every default filled in — never the text the
// project actually wrote. A real run measured this concretely: dharness's
// own declared value carried 3 fields; fallow's resolved value for the same
// key carried 15. Printed side by side with no distinction, a reader
// believes the two sides are directly comparable and cannot find the
// difference — the one thing this block exists to show. The project side
// must say, once, that it is showing fallow's resolved value rather than
// the declared text.
func TestWriteHumanLabelsTheProjectSideAsResolvedWhenMeasured(t *testing.T) {
	ours := jsonRaw(`{"minOccurrences":3,"mode":"semantic","threshold":3}`)
	theirs := jsonRaw(`{"minOccurrences":2,"mode":"weak","threshold":5,"enabled":true,"near":false}`)
	r := Report{
		Steps: []StepResult{
			{
				ID:     "resolve the keys this project and dharness both declare",
				Status: Delegated,
				Collisions: []Collision{{
					ID:     "sync:collision/duplicates",
					Key:    "duplicates",
					Ours:   Declared{Path: ".dharness/fallow.jsonc", Value: &ours},
					Theirs: Declared{Path: "frontend/.fallowrc.json", Value: &theirs},
				}},
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()

	lines := strings.Split(out, "\n")
	var oursBlock, theirsBlock string
	for i, line := range lines {
		if strings.Contains(line, "dharness ") && i+1 < len(lines) {
			oursBlock = line + "\n" + lines[i+1]
		}
		if strings.Contains(line, "project ") && i+1 < len(lines) {
			theirsBlock = line + "\n" + lines[i+1]
		}
	}
	if oursBlock == "" || theirsBlock == "" {
		t.Fatalf("could not locate both sides in output:\n%s", out)
	}
	if !strings.Contains(theirsBlock, "resolved") {
		t.Errorf("project side does not say its value is fallow's resolved value:\n%s", theirsBlock)
	}
	if strings.Contains(oursBlock, "resolved") {
		t.Errorf("dharness's own literally-declared side is wrongly labeled resolved too:\n%s", oursBlock)
	}
}

// TestWriteHumanOmitsResolvedLabelWhenTheirsValueIsAbsent is
// TestWriteHumanLabelsTheProjectSideAsResolvedWhenMeasured's discriminating
// twin: when the resolve measurement never succeeded (Theirs.Value == nil),
// there is no resolved value to label — the side already states
// collisionValueUnavailable, and labeling it resolved too would claim a
// measurement that never happened.
func TestWriteHumanOmitsResolvedLabelWhenTheirsValueIsAbsent(t *testing.T) {
	ours := jsonRaw(`{"minOccurrences":3}`)
	r := Report{
		Steps: []StepResult{
			{
				ID:     "resolve the keys this project and dharness both declare",
				Status: Delegated,
				Collisions: []Collision{{
					ID:     "sync:collision/duplicates",
					Key:    "duplicates",
					Ours:   Declared{Path: ".dharness/fallow.jsonc", Value: &ours},
					Theirs: Declared{Path: "frontend/.fallowrc.json"},
				}},
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()

	if strings.Contains(out, "resolved") {
		t.Errorf("output labels a side resolved despite no measured value being present:\n%s", out)
	}
}

// TestWrapHardSplitsAWordWithNoBreakPoint pins wrap's fallback for a "word"
// (a run of non-whitespace, by strings.Fields' own definition) that alone
// already exceeds width: a real collision value is exactly this shape —
// fallow's `--format json` output is compact, with no internal whitespace
// at all, so the whole value is one unbreakable word. Word-wrapping alone
// cannot bound such a line; "the bound, not the break points" (this file's
// own rule for wrap) requires carving it even with nowhere natural to cut.
func TestWrapHardSplitsAWordWithNoBreakPoint(t *testing.T) {
	cases := []struct {
		name      string
		word      string
		width     int
		wantLines int
	}{
		// An exact multiple is the boundary a mutant on the split loop's
		// `>` (e.g. loosened to `>=`) disagrees with: it would take one
		// extra, empty final chunk after the last real one lands exactly
		// on width, growing this case to 3 lines instead of 2 — content
		// and per-line width alone cannot tell them apart, since an empty
		// trailing line satisfies both.
		{"word exactly twice the width", strings.Repeat("a", 20), 10, 2},
		{"word not an exact width multiple", strings.Repeat("b", 25), 10, 3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines := wrap(tc.word, tc.width, 0)

			if len(lines) != tc.wantLines {
				t.Errorf("wrap(%q, %d, 0) = %d lines %v, want %d lines", tc.word, tc.width, len(lines), lines, tc.wantLines)
			}

			for _, line := range lines {
				if n := utf8.RuneCountInString(line); n > tc.width {
					t.Errorf("wrap(%q, %d, 0) produced line %q of %d runes, want <= %d",
						tc.word, tc.width, line, n, tc.width)
				}
			}

			var rejoined string
			for _, line := range lines {
				rejoined += line
			}
			if rejoined != tc.word {
				t.Errorf("wrap(%q, %d, 0) lost or altered content when hard-splitting: got %q, want %q", tc.word, tc.width, rejoined, tc.word)
			}
		})
	}
}

// TestWriteDeclaredSideValueStartsAtTheDocumentedIndent is a mutation guard
// for declaredSideIndent: the value line's own leading-space count is
// checked against a literal, not the declaredSideIndent constant itself — a
// mutation only changes the constant in production code, and comparing
// against the same (also-mutated) symbol here would make the assertion
// agree with the mutant by construction, the same self-reference risk
// TestWriteClosingBlockRuleSpansWrapWidth already avoids for wrapWidth. A
// mutant changing 15 to any other number shifts the value out of the column
// the location line above it starts a value at.
func TestWriteDeclaredSideValueStartsAtTheDocumentedIndent(t *testing.T) {
	const wantIndent = 15
	value := jsonRaw(`"X"`)
	r := Report{
		Steps: []StepResult{
			{
				ID:     "resolve",
				Status: Delegated,
				Collisions: []Collision{{
					ID:   "sync:collision/duplicates",
					Key:  "duplicates",
					Ours: Declared{Path: ".dharness/fallow.jsonc", Value: &value},
				}},
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()

	var valueLine string
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == `"X"` {
			valueLine = line
			break
		}
	}
	if valueLine == "" {
		t.Fatalf("value line not found:\n%s", out)
	}
	leading := len(valueLine) - len(strings.TrimLeft(valueLine, " "))
	if leading != wantIndent {
		t.Errorf("value line indent = %d, want exactly %d: %q", leading, wantIndent, valueLine)
	}
}

// TestWriteHumanCollisionValueContinuationLinesHaveNoExtraHangingIndent is a
// mutation guard for wrap's own indent argument in writeDeclaredSide (0, not
// a hanging indent): this block already adds its own fixed
// declaredSideIndent prefix externally to every line, so wrap must not add
// a second hanging indent on top of it — mirroring
// TestWriteHumanSatisfiedEvidenceContinuationLinesHaveNoExtraHangingIndent
// for the satisfied block's own evidence. The fixture uses actual
// whitespace-separated words (unlike a real compact-JSON collision value)
// deliberately: the hard-split path a spaceless value takes never consults
// wrap's indent argument at all, so only a value with real word boundaries
// can tell indent 0 apart from indent 1 here.
func TestWriteHumanCollisionValueContinuationLinesHaveNoExtraHangingIndent(t *testing.T) {
	const wantIndent = 15
	value := strings.TrimSpace(strings.Repeat("measured ", 20))
	theirs := jsonRaw(value)
	r := Report{
		Steps: []StepResult{
			{
				ID:     "resolve",
				Status: Delegated,
				Collisions: []Collision{{
					ID:     "sync:collision/duplicates",
					Key:    "duplicates",
					Theirs: Declared{Path: "frontend/.fallowrc.json", Value: &theirs},
				}},
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()

	var valueLines []string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		onlyMeasured := true
		for _, field := range fields {
			if field != "measured" {
				onlyMeasured = false
				break
			}
		}
		if onlyMeasured {
			valueLines = append(valueLines, line)
		}
	}
	if len(valueLines) < 2 {
		t.Fatalf("expected the value to wrap into at least 2 lines, got %d:\n%s", len(valueLines), out)
	}
	for _, line := range valueLines {
		leading := len(line) - len(strings.TrimLeft(line, " "))
		if leading != wantIndent {
			t.Errorf("value continuation line carries a hanging indent on top of the fixed prefix: got %d leading spaces, want exactly %d: %q", leading, wantIndent, line)
		}
	}
}

func jsonRaw(s string) json.RawMessage { return json.RawMessage(s) }

// TestWriteHumanAppliedTranscriptFramesEveryLine pins gap 2 from the measured
// run: a multi-line subprocess transcript must carry the gutter glyph on
// every line, not only its first — the unframed-leak defect 5 exists to
// kill, only half fixed when a single prefix is applied to the whole blob.
func TestWriteHumanAppliedTranscriptFramesEveryLine(t *testing.T) {
	r := Report{
		Steps: []StepResult{
			{
				ID:         "install what this project is missing",
				Status:     Applied,
				Transcript: "npm warn ancient lockfile\nnpm warn ancient lockfile The package-lock.json file was created with...\nnpm warn ancient lockfile so supplemental metadata must be fetched...",
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()

	for _, line := range strings.Split(r.Steps[0].Transcript, "\n") {
		framed := glyphGutter + " " + line
		if !strings.Contains(out, framed) {
			t.Errorf("transcript line not framed under its own gutter glyph: %q\n%s", line, out)
		}
	}
}

// TestWriteHumanSatisfiedGlyphIsNotTheSubprocessGutter pins gap 5: the
// "Already in place" block's own glyph must not collide with the gutter
// glyph the legend assigns to subprocess output.
func TestWriteHumanSatisfiedGlyphIsNotTheSubprocessGutter(t *testing.T) {
	r := Report{Steps: []StepResult{{ID: "already done", Status: Satisfied, Evidence: "found"}}}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()

	var satisfiedLine string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "already done") {
			satisfiedLine = line
			break
		}
	}
	if satisfiedLine == "" {
		t.Fatalf("satisfied step line not found:\n%s", out)
	}
	if strings.Contains(satisfiedLine, glyphGutter) {
		t.Errorf("satisfied row uses the subprocess gutter glyph %q, which the legend reserves for subprocess output: %q", glyphGutter, satisfiedLine)
	}
	if !strings.Contains(satisfiedLine, glyphSatisfied) {
		t.Errorf("satisfied row does not carry glyphSatisfied %q: %q", glyphSatisfied, satisfiedLine)
	}
}

// TestWriteHumanStepRowsCarryNumbering pins gap 6: every per-step row states
// its position out of the plan's total step count — the JSON already carries
// StepResult.N, and the human view rendered nothing from it.
func TestWriteHumanStepRowsCarryNumbering(t *testing.T) {
	r := Report{
		Summary: Summary{Steps: 11},
		Steps: []StepResult{
			{N: 1, ID: "install what this project is missing", Status: Applied},
			{N: 3, ID: "already wired", Status: Satisfied, Evidence: "found"},
			{N: 4, ID: "resolve the keys this project and dharness both declare", Status: Delegated, Why: "two owners"},
		},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()

	for _, want := range []string{"1/11", "3/11", "4/11"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing step numbering %q:\n%s", want, out)
		}
	}
}

// TestWriteHumanHeaderBlockPrecedesSummary pins gap 7: the report opens with
// the project context (js project/package manager/test runner/presets/owned
// files) before the summary line, matching fallow's own provenance-header
// convention this change is modelled on.
func TestWriteHumanHeaderBlockPrecedesSummary(t *testing.T) {
	r := Report{
		Version:        "1.2.0",
		Root:           `D:\dev\disble\autoreas-sp\autoreas-bridge`,
		Source:         "frontend",
		PackageManager: "bun",
		TestRunner:     "vitest",
		OwnedDir:       ".dharness",
		Summary:        Summary{Steps: 1},
		Steps:          []StepResult{{N: 1, ID: "x", Status: Satisfied, Evidence: "y"}},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()

	for _, want := range []string{"1.2.0", r.Root, "bun", "vitest", ".dharness"} {
		if !strings.Contains(out, want) {
			t.Errorf("header block missing %q:\n%s", want, out)
		}
	}

	headerAt := strings.Index(out, "1.2.0")
	summaryAt := strings.Index(out, glyphSummary)
	if headerAt < 0 || summaryAt < 0 {
		t.Fatalf("header or summary marker not found:\n%s", out)
	}
	if headerAt >= summaryAt {
		t.Errorf("header block at byte %d does not precede the summary marker at byte %d:\n%s", headerAt, summaryAt, out)
	}
}

// TestWriteHumanHeaderBlockAbsentWithNoRoot guards the many existing tests
// that construct a bare Report{} to test one block in isolation: with no
// Root, there is no project to state a header for, so none renders.
func TestWriteHumanHeaderBlockAbsentWithNoRoot(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteHuman(&buf, Report{}); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	if strings.Contains(buf.String(), "js project") {
		t.Errorf("header block rendered with no Root set:\n%s", buf.String())
	}
}

// TestWriteHumanAppliedBlockEndsWithLegend pins gap 8: the glyph legend must
// actually appear somewhere in the rendered output, naming every glyph the
// Applied block can use.
func TestWriteHumanAppliedBlockEndsWithLegend(t *testing.T) {
	r := Report{Steps: []StepResult{{ID: "install", Status: Applied}}}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "legend") {
		t.Fatalf("no legend line in output:\n%s", out)
	}
	for _, glyph := range []string{changeGlyph(Created), changeGlyph(Modified), changeGlyph(Unchanged), glyphGutter} {
		if !strings.Contains(out, glyph) {
			t.Errorf("legend missing glyph %q:\n%s", glyph, out)
		}
	}
}

// TestClosingBlockNextNamesTheCollisionHandle pins gap 9: when the delegated
// step carries a collision, the closing block's next pointer must name the
// collision's own addressable handle, not the step's prose heading — the
// same distinction Collision.ID exists to make (design.md Decision 1).
func TestClosingBlockNextNamesTheCollisionHandle(t *testing.T) {
	r := Report{
		Steps: []StepResult{
			{
				ID:         "resolve the keys this project and dharness both declare",
				Status:     Delegated,
				Collisions: []Collision{{ID: "sync:collision/duplicates", Key: "duplicates"}},
			},
		},
	}

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
	if !strings.Contains(closing, "sync:collision/duplicates") {
		t.Errorf("closing block's next pointer does not name the collision handle:\n%s", closing)
	}
	if strings.Contains(closing, r.Steps[0].ID) {
		t.Errorf("closing block's next pointer names the step's prose heading instead of the collision handle:\n%s", closing)
	}
}

// TestWriteHumanSatisfiedEvidenceWraps pins the second fault in gap 4: long
// evidence text must wrap within the fixed report width, never run past it
// unbounded — a mutant that drops the wrap call would leave every returned
// line's rune count unbounded, which the loop below directly checks.
func TestWriteHumanSatisfiedEvidenceWraps(t *testing.T) {
	longEvidence := strings.TrimSpace(strings.Repeat("measured ", 20))
	r := Report{Steps: []StepResult{{ID: "x", Status: Satisfied, Evidence: longEvidence}}}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()

	for _, line := range strings.Split(out, "\n") {
		if n := utf8.RuneCountInString(line); n > wrapWidth {
			t.Errorf("line exceeds the fixed report width of %d runes (%d): %q", wrapWidth, n, line)
		}
	}
	for _, word := range strings.Fields(longEvidence) {
		if !strings.Contains(out, word) {
			t.Errorf("wrapped evidence lost word %q:\n%s", word, out)
		}
	}
}

// TestWriteHumanSatisfiedEvidenceWrapWidthAccountsForTheIndent is a
// mutation guard for the wrap-width computation (wrapWidth-9): 61 runes is
// exactly wrapWidth(70) minus the block's own 9-space indent, so this two-
// word value fits on one line only if the wrap width is computed against
// that same indent — a mutant using a different constant (e.g. wrapWidth-10)
// would force it onto two.
func TestWriteHumanSatisfiedEvidenceWrapWidthAccountsForTheIndent(t *testing.T) {
	evidence := strings.Repeat("a", 30) + " " + strings.Repeat("b", 30)
	if n := utf8.RuneCountInString(evidence); n != 61 {
		t.Fatalf("fixture sanity check failed: evidence is %d runes, want 61", n)
	}
	r := Report{Steps: []StepResult{{ID: "x", Status: Satisfied, Evidence: evidence}}}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	if !strings.Contains(buf.String(), evidence) {
		t.Errorf("a 61-rune evidence value (wrapWidth-9) wrapped onto two lines instead of staying on one:\n%s", buf.String())
	}
}

// TestWriteHumanSatisfiedEvidenceContinuationLinesHaveNoExtraHangingIndent
// is a mutation guard for wrap()'s own indent argument in the satisfied
// block (0, not 3 as the delegated/notes blocks use — this block already
// adds its own fixed 9-space prefix externally to every line, so wrap()
// must not add a second hanging indent on top of it).
func TestWriteHumanSatisfiedEvidenceContinuationLinesHaveNoExtraHangingIndent(t *testing.T) {
	longEvidence := strings.TrimSpace(strings.Repeat("measured ", 20))
	r := Report{Steps: []StepResult{{ID: "x", Status: Satisfied, Evidence: longEvidence}}}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}

	var evidenceLines []string
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.Contains(line, "measured") {
			evidenceLines = append(evidenceLines, line)
		}
	}
	if len(evidenceLines) < 2 {
		t.Fatalf("expected the evidence to wrap into at least 2 lines, got %d:\n%s", len(evidenceLines), buf.String())
	}
	for _, line := range evidenceLines {
		leading := len(line) - len(strings.TrimLeft(line, " "))
		if leading != 9 {
			t.Errorf("evidence line has %d leading spaces, want exactly 9 (no extra hanging indent beyond the block's own gutter): %q", leading, line)
		}
	}
}

// assertAppliedOrFailedBlockAligned drives one label/ID/time fixture through
// WriteHuman and asserts (a) each row's own ID appears only on a line that
// also carries that row's own label — catching a swapped row[0]/row[1]
// *content* reference — and (b) the elapsed-time column starts at the same
// offset for both rows — catching a w[0]/w[1]/w[2] *width* index
// confusion, provided the fixture's own column lengths make that
// confusion visible (see the two callers below for why one fixture cannot
// cover both directions at once).
func assertAppliedOrFailedBlockAligned(t *testing.T, status Status, steps, n2 int, ids [2]string) {
	t.Helper()

	r := Report{
		Summary: Summary{Steps: steps},
		Steps: []StepResult{
			{N: 1, ID: ids[0], Status: status, MS: 100},
			{N: n2, ID: ids[1], Status: status, MS: 200},
		},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()
	lines := strings.Split(out, "\n")

	for _, step := range r.Steps {
		label := stepLabel(step.N, r.Summary.Steps)
		found := false
		for _, line := range lines {
			if !strings.Contains(line, step.ID) {
				continue
			}
			found = true
			if !strings.Contains(line, label) {
				t.Errorf("line %q carries ID %q but not its own label %q — label/ID content may be swapped between rows", line, step.ID, label)
			}
		}
		if !found {
			t.Errorf("ID %q not found in output:\n%s", step.ID, out)
		}
	}

	offsetOf := func(elapsed string) int {
		for _, line := range lines {
			if strings.HasSuffix(line, elapsed) {
				return strings.LastIndex(line, elapsed)
			}
		}
		t.Fatalf("elapsed time %q not found in output:\n%s", elapsed, out)
		return -1
	}
	first, second := offsetOf("0.10s"), offsetOf("0.20s")
	if first != second {
		t.Errorf("elapsed-time columns misaligned: %d vs %d\n%s", first, second, out)
	}
}

// TestWriteHumanAppliedAndFailedBlocksPairEachLabelWithItsOwnID is a
// mutation guard for the label/ID/time three-column layout both blocks
// share (w[0] label, w[1] ID, w[2] fixed-width time). Padding to any fixed
// per-block width normalizes cross-row alignment regardless of *which*
// column's width is used, as long as every row's own content still fits
// under it — so the only way to expose a width-index confusion is a row
// whose own content would overflow the *wrong* column's width while the
// other row's does not. A single two-row fixture cannot expose every
// direction of that confusion at once (row 2's much longer label would
// itself absorb an accidentally-narrow ID width, and vice versa), so this
// runs two fixtures: one with identical label lengths (isolating the ID
// column's own width), one with identical ID lengths (isolating the label
// column's own width).
func TestWriteHumanAppliedAndFailedBlocksPairEachLabelWithItsOwnID(t *testing.T) {
	cases := []struct {
		name   string
		status Status
	}{
		{"applied", Applied},
		{"failed", Failed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("differing ID lengths, identical label lengths", func(t *testing.T) {
				// N=1 and N=2 against Steps=9 produce equal-length labels
				// ("1/9", "2/9", 3 runes each), isolating the ID column:
				// id[1] (31 runes) far exceeds the label width, so
				// mistakenly padding it to the label's or the time
				// column's width would overflow only this row.
				assertAppliedOrFailedBlockAligned(t, tc.status, 9, 2, [2]string{"zqx", "very-long-identifier-name-here"})
			})
			t.Run("differing label lengths, identical ID lengths", func(t *testing.T) {
				// N=1 and N=2222222 against Steps=9999999 produce very
				// different label lengths (9 vs 15 runes), both exceeding
				// the equal-length IDs (5 runes) — the same asymmetry,
				// isolating the label column instead.
				assertAppliedOrFailedBlockAligned(t, tc.status, 9999999, 2222222, [2]string{"abcde", "fghij"})
			})
		})
	}
}

// TestResolutionLetterNamesPositionsAlphabetically is a mutation guard for
// resolutionLetter's arithmetic: the nth resolution is the nth letter, not
// merely "some" letter — asserted for both positions Collisions ever
// produces (a, b), since a mutant flipping + to - still returns a valid
// rune for i=0 (both give 'a') and only disagrees from position 1 onward.
func TestResolutionLetterNamesPositionsAlphabetically(t *testing.T) {
	cases := []struct {
		i    int
		want string
	}{
		{0, "a"},
		{1, "b"},
	}
	for _, tc := range cases {
		if got := resolutionLetter(tc.i); got != tc.want {
			t.Errorf("resolutionLetter(%d) = %q, want %q", tc.i, got, tc.want)
		}
	}
}

// TestWriteHumanHeaderBlockPresetsLine is a mutation guard for the
// len(r.Presets) > 0 branch: a Report with matched presets must name them,
// and a Report with none must say so explicitly ("none matched") — the two
// outcomes a mutant collapsing this comparison to >= 0, <= 0, > 1 or > -1
// would each get wrong for at least one of these two cases.
func TestWriteHumanHeaderBlockPresetsLine(t *testing.T) {
	base := Report{Root: "/repo", Summary: Summary{Steps: 1}, Steps: []StepResult{{N: 1, ID: "x", Status: Satisfied, Evidence: "y"}}}

	t.Run("matched presets are named", func(t *testing.T) {
		r := base
		r.Presets = []string{"nextjs"}
		var buf bytes.Buffer
		if err := WriteHuman(&buf, r); err != nil {
			t.Fatalf("WriteHuman() = %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "nextjs") {
			t.Errorf("header block does not name the matched preset:\n%s", out)
		}
		if strings.Contains(out, "none matched") {
			t.Errorf("header block says none matched despite Presets being non-empty:\n%s", out)
		}
	})

	t.Run("no presets says so explicitly", func(t *testing.T) {
		var buf bytes.Buffer
		if err := WriteHuman(&buf, base); err != nil {
			t.Fatalf("WriteHuman() = %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "none matched") {
			t.Errorf("header block does not say none matched with an empty Presets:\n%s", out)
		}
	})
}

// TestWriteHumanHeaderBlockJSProjectNamesTheSourceWhenSet is a mutation
// guard for the jsProject == "" branch: a Report whose Source is set must
// name that directory, never the "repository root" fallback that branch
// exists for only when Source is empty — a mutant inverting the comparison
// would swap the two.
func TestWriteHumanHeaderBlockJSProjectNamesTheSourceWhenSet(t *testing.T) {
	r := Report{Root: "/repo", Source: "frontend", Summary: Summary{Steps: 0}}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "frontend") {
		t.Errorf("header block does not name the js project directory:\n%s", out)
	}
	if strings.Contains(out, "repository root") {
		t.Errorf("header block says repository root despite Source being set:\n%s", out)
	}
}

// TestWriteCollisionOmitsResolutionsWhenEmpty is a mutation guard for the
// len(c.Resolutions) > 0 branch in writeCollision: an empty Resolutions
// slice (never produced by setup.Collisions today, but a value the renderer
// must still handle correctly) must not print the "Pick one owner:" prompt
// with nothing to choose from.
func TestWriteCollisionOmitsResolutionsWhenEmpty(t *testing.T) {
	r := Report{
		Steps: []StepResult{
			{
				ID:         "resolve",
				Status:     Delegated,
				Collisions: []Collision{{ID: "sync:collision/duplicates", Key: "duplicates"}},
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "Pick one owner") {
		t.Errorf("output prompts to pick an owner with zero Resolutions to choose from:\n%s", out)
	}
}

// TestWriteCollisionShowsResolutionsWhenExactlyOne is
// TestWriteCollisionOmitsResolutionsWhenEmpty's discriminating twin, and a
// mutation guard for len(c.Resolutions) > 0 in the other direction: a
// single resolution (len == 1) is still > 0, so the "Pick one owner:"
// prompt must still render — a mutant collapsing the comparison to > 1
// would agree with the empty case here but disagree on this one, since 2
// resolutions (every other test's fixture) satisfies both > 0 and > 1
// identically and cannot tell them apart on its own.
func TestWriteCollisionShowsResolutionsWhenExactlyOne(t *testing.T) {
	r := Report{
		Steps: []StepResult{
			{
				ID:     "resolve",
				Status: Delegated,
				Collisions: []Collision{{
					ID:          "sync:collision/duplicates",
					Key:         "duplicates",
					Resolutions: []string{"delete-theirs"},
				}},
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Pick one owner") {
		t.Errorf("output omits the prompt with exactly one resolution to choose from:\n%s", out)
	}
}

// TestWriteDeclaredSideOmitsTheLineWhenAbsent is a mutation guard for the
// d.Line > 0 branch: Declared.Line's own documented sentinel (0, "not
// found"/"not measured") must render as a bare path, never ":0" — the
// absent-line case Line's own json tag (omitempty) already treats as
// absent.
func TestWriteDeclaredSideOmitsTheLineWhenAbsent(t *testing.T) {
	ours := jsonRaw(`"x"`)
	r := Report{
		Steps: []StepResult{
			{
				ID:     "resolve",
				Status: Delegated,
				Collisions: []Collision{{
					ID:   "sync:collision/duplicates",
					Key:  "duplicates",
					Ours: Declared{Path: ".dharness/fallow.jsonc", Line: 0, Value: &ours},
				}},
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()
	if strings.Contains(out, ".dharness/fallow.jsonc:0") {
		t.Errorf("output renders a fabricated line number for an absent Line:\n%s", out)
	}
	if !strings.Contains(out, ".dharness/fallow.jsonc") {
		t.Errorf("output does not render the bare path at all:\n%s", out)
	}
}

// TestWriteDeclaredSideShowsLineOne is
// TestWriteDeclaredSideOmitsTheLineWhenAbsent's discriminating twin: Line ==
// 1 is the smallest real (non-sentinel) line number, so it must render —
// a mutant collapsing d.Line > 0 to d.Line > 1 would agree with the Line ==
// 0 case here but wrongly omit this one, since every other test's fixture
// uses Line values (8, 12) that satisfy both > 0 and > 1 identically.
func TestWriteDeclaredSideShowsLineOne(t *testing.T) {
	ours := jsonRaw(`"x"`)
	r := Report{
		Steps: []StepResult{
			{
				ID:     "resolve",
				Status: Delegated,
				Collisions: []Collision{{
					ID:   "sync:collision/duplicates",
					Key:  "duplicates",
					Ours: Declared{Path: ".dharness/fallow.jsonc", Line: 1, Value: &ours},
				}},
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, ".dharness/fallow.jsonc:1") {
		t.Errorf("output does not render line 1:\n%s", out)
	}
}

// TestEffectiveMarkAppearsOnExactlyOneSide is a mutation guard for
// effectiveMark's own comparison: with Effective measured as "theirs", the
// "this one runs" mark must appear on the theirs side and nowhere else — a
// mutant inverting the comparison, or replacing it with an unconditional
// true, would either move the mark to the wrong side or mark both.
func TestEffectiveMarkAppearsOnExactlyOneSide(t *testing.T) {
	ours := jsonRaw(`{"a":1}`)
	theirs := jsonRaw(`{"a":2}`)
	effective := "theirs"
	c := Collision{
		ID:        "sync:collision/duplicates",
		Key:       "duplicates",
		Ours:      Declared{Path: ".dharness/fallow.jsonc", Value: &ours},
		Theirs:    Declared{Path: "frontend/.fallowrc.json", Value: &theirs},
		Effective: &effective,
	}
	r := Report{Steps: []StepResult{{ID: "resolve", Status: Delegated, Collisions: []Collision{c}}}}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()

	if got := strings.Count(out, "this one runs"); got != 1 {
		t.Fatalf(`strings.Count(output, "this one runs") = %d, want exactly 1:%s`, got, out)
	}

	lines := strings.Split(out, "\n")
	var oursLine, theirsLine string
	for i, line := range lines {
		if strings.Contains(line, "dharness ") && i+1 < len(lines) {
			oursLine = line + "\n" + lines[i+1]
		}
		if strings.Contains(line, "project ") && i+1 < len(lines) {
			theirsLine = line + "\n" + lines[i+1]
		}
	}
	if strings.Contains(oursLine, "this one runs") {
		t.Errorf("the mark appears on the ours side despite Effective == %q:\n%s", effective, oursLine)
	}
	if !strings.Contains(theirsLine, "this one runs") {
		t.Errorf("the mark is missing from the theirs side, where Effective == %q:\n%s", effective, theirsLine)
	}
}

// TestWhyAndCollisionsAreMutuallyExclusiveInRendering pins design.md
// Decision 4's "seam that makes a second renderer impossible, rather than
// absent": a StepResult carrying both a non-empty Why and a non-empty
// Collisions slice — an adversarial value no production caller in this
// package builds, but the renderer must still be correct against it — must
// render the collision block and never the Why text. A mutant collapsing
// this to "always print Why" dies here.
func TestWhyAndCollisionsAreMutuallyExclusiveInRendering(t *testing.T) {
	r := Report{
		Steps: []StepResult{
			{
				ID:         "resolve the keys this project and dharness both declare",
				Status:     Delegated,
				Why:        "this prose must never reach the output",
				Collisions: []Collision{{Key: "duplicates"}},
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "duplicates") {
		t.Errorf("output missing the collision block:\n%s", out)
	}
	if strings.Contains(out, r.Steps[0].Why) {
		t.Errorf("output renders Why even though Collisions is non-empty; the two must be mutually exclusive:\n%s", out)
	}
}

// TestFailureVariantRendersEveryNonTerminalStatus pins the six-value status
// enumeration's failure-variant scenarios together: a report whose steps
// carry failed, retracted and not-reached is rendered with one line per
// step, the retracted step is named in the closing block as included in
// the rollback, and the tally sums to the plan's length.
func TestFailureVariantRendersEveryNonTerminalStatus(t *testing.T) {
	r := Report{
		Summary: Summary{Steps: 4, Failed: 1, Retracted: 1},
		Steps: []StepResult{
			{N: 1, ID: "install what this project is missing", Status: Retracted},
			{N: 2, ID: "write the files dharness owns", Status: Failed, Error: "permission denied"},
			{N: 3, ID: "point .fallowrc.json at the file dharness owns", Status: NotReached},
			{N: 4, ID: "wire the gate into git", Status: NotReached},
		},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()

	for _, step := range r.Steps {
		if !strings.Contains(out, step.ID) {
			t.Errorf("output missing step %q, want every non-terminal status rendered with its own line:\n%s", step.ID, out)
		}
	}

	ruleAt := strings.LastIndex(out, strings.Repeat(glyphRule, wrapWidth))
	if ruleAt < 0 {
		t.Fatalf("closing separator not found in output:\n%s", out)
	}
	closing := out[ruleAt:]
	if !strings.Contains(closing, r.Steps[0].ID) {
		t.Errorf("closing block does not name the retracted step as included in the rollback:\n%s", closing)
	}

	tally := r.Summary.Failed + r.Summary.Retracted + len(stepsWithStatus(r.Steps, NotReached))
	if tally != len(r.Steps) {
		t.Errorf("tally = %d, want it to sum to the plan's length %d", tally, len(r.Steps))
	}
}

// TestClosingBlockNamesTheDelegatedStepAsNext pins "a run with delegated
// work names a next step": the closing block adds its own mention of the
// delegated step's identifier, distinct from the earlier per-step "Left to
// you" detail already asserted elsewhere.
func TestClosingBlockNamesTheDelegatedStepAsNext(t *testing.T) {
	r := Report{
		Steps: []StepResult{
			{ID: "resolve the keys this project and dharness both declare", Status: Delegated, Why: "two owners"},
		},
	}

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
	if !strings.Contains(closing, r.Steps[0].ID) {
		t.Errorf("closing block does not name the delegated step as next:\n%s", closing)
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

// TestWriteHumanEvidenceIndentIsFixedAcrossDifferentIDLengths supersedes
// the column-alignment rule this test used to pin: aligning evidence into a
// column next to the ID left almost no room for it whenever any row in the
// block had a long ID (this product's step IDs are full sentences, not
// short slugs — measured live against a real project, where even a short
// fact like "owned files match" wrapped needlessly because of an unrelated
// row's long ID). Evidence now renders on its own line, at the same fixed
// indent regardless of the ID's own length — a stronger, simpler
// invariant, checked here the same structural way the old column rule was.
func TestWriteHumanEvidenceIndentIsFixedAcrossDifferentIDLengths(t *testing.T) {
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
			if strings.TrimSpace(line) == evidence {
				offsets = append(offsets, strings.Index(line, evidence))
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("evidence %q not found on its own line:\n%s", evidence, out)
		}
	}

	if offsets[0] != offsets[1] {
		t.Errorf("evidence indent differs across rows of different ID length: %v\n%s", offsets, out)
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
			marker: " ! 0/0   resolve",
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

// TestWriteHumanCollisionNarrowsToDifferingKeys pins the one thing the
// collision block exists to do: let a reader see how the two values differ.
//
// Measured against a real project, dharness's side carried 3 keys and
// fallow's resolved side carried 15, twelve of which were identical
// defaults. Printed whole, the difference — one key, one word — was buried
// in nine wrapped lines of JSON split mid-token. The keys that agree carry
// no information for a decision about the keys that do not.
func TestWriteHumanCollisionNarrowsToDifferingKeys(t *testing.T) {
	ours := jsonRaw(`{"mode":"semantic","minOccurrences":3,"threshold":3}`)
	theirs := jsonRaw(`{"mode":"weak","minOccurrences":3,"threshold":3,"near":false}`)
	r := Report{
		Steps: []StepResult{{
			ID:     "resolve the keys this project and dharness both declare",
			Status: Delegated,
			Collisions: []Collision{{
				ID:     "sync:collision/duplicates",
				Key:    "duplicates",
				Ours:   Declared{Path: ".dharness/fallow.jsonc", Value: &ours},
				Theirs: Declared{Path: "frontend/.fallowrc.json", Value: &theirs},
			}},
		}},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()

	// The key that disagrees survives, with both of its values.
	for _, want := range []string{`"mode"`, `"semantic"`, `"weak"`} {
		if !strings.Contains(out, want) {
			t.Errorf("output dropped %s, which is what the reader has to decide about:\n%s", want, out)
		}
	}

	// Keys that agree are gone from both sides, and so is a key only the
	// resolved side carries: `near` is one of fallow's own defaults, not
	// something the project chose and not something dharness disagrees
	// with. Counting it as a difference is what made the first version of
	// this rule fall back to the whole value on the very case it was
	// written for — measured live, dharness declared 3 keys against
	// fallow's 16, and all three shared keys differed, so "differs on
	// both sides" alone hid nothing at all.
	for _, gone := range []string{"minOccurrences", "threshold", "near"} {
		if strings.Contains(out, gone) {
			t.Errorf("output still carries %q, which carries no decision:\n%s", gone, out)
		}
	}

	// And it says how many it left out, rather than silently shortening.
	if !strings.Contains(out, "3 identical") {
		t.Errorf("output hides 3 keys without saying so:\n%s", out)
	}
}

// TestWriteHumanCollisionKeepsWholeValuesWhenItCannotNarrowHonestly pins the
// fallback. Narrowing is only meaningful between two JSON objects; a side
// that was never measured, or that is not an object, has no keys to compare,
// and shortening the other side against nothing would hide information while
// claiming a comparison happened.
func TestWriteHumanCollisionKeepsWholeValuesWhenItCannotNarrowHonestly(t *testing.T) {
	cases := map[string]struct{ ours, theirs *json.RawMessage }{
		"the project's side was never measured": {ptrRaw(`{"mode":"semantic","threshold":3}`), nil},
		"dharness's side was never measured":    {nil, ptrRaw(`{"mode":"weak","threshold":3}`)},
		"a side is not an object":               {ptrRaw(`{"mode":"semantic","threshold":3}`), ptrRaw(`"weak"`)},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := Report{Steps: []StepResult{{
				ID:     "resolve the keys this project and dharness both declare",
				Status: Delegated,
				Collisions: []Collision{{
					ID:     "sync:collision/duplicates",
					Key:    "duplicates",
					Ours:   Declared{Path: ".dharness/fallow.jsonc", Value: tc.ours},
					Theirs: Declared{Path: "frontend/.fallowrc.json", Value: tc.theirs},
				}},
			}}}

			var buf bytes.Buffer
			if err := WriteHuman(&buf, r); err != nil {
				t.Fatalf("WriteHuman() = %v", err)
			}
			out := buf.String()

			// The measured side keeps every key it arrived with.
			if tc.ours != nil && !strings.Contains(out, "threshold") {
				t.Errorf("a value that could not be compared was shortened anyway:\n%s", out)
			}
			if strings.Contains(out, "hidden") {
				t.Errorf("claimed it hid keys when it never compared two objects:\n%s", out)
			}
		})
	}
}

// TestWriteHumanCollisionSaysNothingWhenNoKeyIsIdentical pins the other edge
// of the same rule: the note reports work that happened, so two objects that
// share no identical key must not carry a note saying zero were hidden.
func TestWriteHumanCollisionSaysNothingWhenNoKeyIsIdentical(t *testing.T) {
	ours := jsonRaw(`{"mode":"semantic"}`)
	theirs := jsonRaw(`{"mode":"weak"}`)
	r := Report{Steps: []StepResult{{
		ID:     "resolve the keys this project and dharness both declare",
		Status: Delegated,
		Collisions: []Collision{{
			ID:     "sync:collision/duplicates",
			Key:    "duplicates",
			Ours:   Declared{Path: ".dharness/fallow.jsonc", Value: &ours},
			Theirs: Declared{Path: "frontend/.fallowrc.json", Value: &theirs},
		}},
	}}}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	if out := buf.String(); strings.Contains(out, "hidden") {
		t.Errorf("printed a hidden-key note when nothing was hidden:\n%s", out)
	}
}

// ptrRaw is jsonRaw's addressable form, for table cases that need a nil side.
func ptrRaw(s string) *json.RawMessage {
	raw := jsonRaw(s)
	return &raw
}

// TestWriteHumanDelegatedWhyStaysWithinReportWidth pins the width of the
// delegated block's reason text, which nothing pinned before.
//
// Found live, not by the suite: three delegated steps rendered lines of 71
// to 73 runes in a report whose every other block holds 70. The cause was
// that wrap was handed the full width while the caller then added its own
// three-space prefix, so the bound was applied to the text and not to the
// line that reaches the reader. The suite was green throughout, because no
// test measured this block's lines at all.
func TestWriteHumanDelegatedWhyStaysWithinReportWidth(t *testing.T) {
	// A real reason, long enough to wrap several times: this is
	// hookInstallStep's own text, the step whose rendering was measured
	// over width.
	why := "nothing answers: there is no lefthook config, no .husky/ and no " +
		"lefthook binary. Choosing a hook manager is a decision this project " +
		"has not made, and not a default dharness gets to pick."

	r := Report{
		Summary: Summary{Steps: 1, Delegated: 1},
		Steps: []StepResult{{
			N: 1, ID: "wire the gate into git", Status: Delegated, Why: why,
		}},
	}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()

	for _, line := range strings.Split(out, "\n") {
		if n := utf8.RuneCountInString(line); n > wrapWidth {
			t.Errorf("delegated reason line exceeds the report width of %d runes (%d): %q", wrapWidth, n, line)
		}
	}

	// The bound must come from wrapping, never from dropping text: every
	// word of the reason has to survive somewhere in the block.
	for _, word := range strings.Fields(why) {
		if !strings.Contains(out, word) {
			t.Errorf("wrapping the reason lost %q:\n%s", word, out)
		}
	}
}

// TestWriteHumanCollisionKeyOrderIsStableAcrossRuns pins determinism.
// narrowToDifferences walks two Go maps, and Go randomises map iteration
// order deliberately, so without an explicit sort the same collision would
// render its keys in a different order on every run — a report that cannot
// be diffed against itself, and a golden that could never be frozen.
// Rendering the same report many times must produce one answer.
func TestWriteHumanCollisionKeyOrderIsStableAcrossRuns(t *testing.T) {
	ours := jsonRaw(`{"alpha":1,"beta":2,"gamma":3,"delta":4,"epsilon":5,"same":0}`)
	theirs := jsonRaw(`{"alpha":9,"beta":8,"gamma":7,"delta":6,"epsilon":5,"same":0}`)
	r := Report{Steps: []StepResult{{
		ID:     "resolve the keys this project and dharness both declare",
		Status: Delegated,
		Collisions: []Collision{{
			ID:     "sync:collision/duplicates",
			Key:    "duplicates",
			Ours:   Declared{Path: ".dharness/fallow.jsonc", Value: &ours},
			Theirs: Declared{Path: "frontend/.fallowrc.json", Value: &theirs},
		}},
	}}}

	render := func() string {
		var buf bytes.Buffer
		if err := WriteHuman(&buf, r); err != nil {
			t.Fatalf("WriteHuman() = %v", err)
		}
		return buf.String()
	}

	first := render()
	for i := 0; i < 24; i++ {
		if got := render(); got != first {
			t.Fatalf("render %d differs from the first — key order is not deterministic:\n%s\n---\n%s", i, first, got)
			return
		}
	}

	// And the order is the sorted one, not merely a stable accident.
	alpha := strings.Index(first, `"alpha"`)
	beta := strings.Index(first, `"beta"`)
	gamma := strings.Index(first, `"gamma"`)
	if alpha < 0 || beta < 0 || gamma < 0 || !(alpha < beta && beta < gamma) {
		t.Errorf("differing keys are not rendered in sorted order (alpha=%d beta=%d gamma=%d):\n%s", alpha, beta, gamma, first)
	}
}

// TestWriteHumanCollisionComparesValuesNotTheirFormatting pins that two
// sides are compared as JSON rather than as text. dharness writes its own
// files indented; fallow's `--format json` is compact with no internal
// whitespace at all. The same value from those two sources differs byte for
// byte and agrees completely, and a text comparison would report every key
// as a disagreement — the report would then show fifteen "differences" and
// hide none of them.
func TestWriteHumanCollisionComparesValuesNotTheirFormatting(t *testing.T) {
	ours := jsonRaw(`{ "mode" : "semantic" , "threshold" : 3 }`)
	theirs := jsonRaw(`{"mode":"weak","threshold":3}`)
	r := Report{Steps: []StepResult{{
		ID:     "resolve the keys this project and dharness both declare",
		Status: Delegated,
		Collisions: []Collision{{
			ID:     "sync:collision/duplicates",
			Key:    "duplicates",
			Ours:   Declared{Path: ".dharness/fallow.jsonc", Value: &ours},
			Theirs: Declared{Path: "frontend/.fallowrc.json", Value: &theirs},
		}},
	}}}

	var buf bytes.Buffer
	if err := WriteHuman(&buf, r); err != nil {
		t.Fatalf("WriteHuman() = %v", err)
	}
	out := buf.String()

	if strings.Contains(out, "threshold") {
		t.Errorf("threshold is 3 on both sides and differs only in spacing, so it carries no decision:\n%s", out)
	}
	if !strings.Contains(out, "1 identical") {
		t.Errorf("the whitespace-only difference was not recognised as agreement:\n%s", out)
	}
}
