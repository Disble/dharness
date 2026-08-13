package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"
)

// The glyph set, in one place, because the fallback is a swap of this
// block and nothing else. ── already ships on the gate path
// (internal/cli/check.go:122) and is pinned by a test, so `sync` and
// `check` read as the same product.
const (
	glyphRule      = "─"
	glyphSummary   = "■"
	glyphGutter    = "│"
	glyphApplied   = "✓"
	glyphFailed    = "✗"
	glyphSeparator = "·"

	// glyphSatisfied marks an "Already in place" row. It is its own
	// constant, distinct from glyphGutter, because the legend the Applied
	// block prints assigns the gutter glyph to subprocess output alone —
	// reusing it for a satisfied step's row collided the two meanings on
	// the same character (measured gap: the team lead's real run rendered
	// both under │).
	glyphSatisfied = "="
)

// wrapWidth is fixed rather than queried from the terminal (design.md
// Decision 7): querying it needs a syscall this stdlib-only product does
// not carry, and it would make the output depend on the reader's window —
// untestable, and useless in the agent transcript that is the second
// reader.
const wrapWidth = 70

// widths returns the column width each row must be padded to, measured in
// runes rather than bytes because ── and the status glyphs are multi-byte
// and %-*s pads by byte count.
//
// It is a separate pure function so alignment is tested as numbers. A test
// that asserts on rendered spacing pins layout; a test that asserts
// widths([]row{...}) == []int{...} pins the rule, and a mutant that drops
// the max comparison dies in it.
//
// Two mutants over this loop are proven equivalent and left disabled by no
// annotation this tooling exposes, recorded here instead (mutation-tdd:
// disable only a proven equivalent mutant, at the narrowest location, with
// a written reason):
//   - `n > result[column]` mutated to `n >=`: on a tie, the mutant assigns
//     result[column] = n, which already equals result[column] — the same
//     final value either way, for every possible n.
//   - the newly-grown column's seed, `append(result, 0)`, mutated to -1:
//     no rune count is ever negative, so `n > -1` is always true and the
//     seed is immediately overwritten by the first real measurement,
//     converging to the same value seed 0 would have left unwritten.
func widths(rows [][]string) []int {
	var result []int
	for _, row := range rows {
		for column, cell := range row {
			for len(result) <= column {
				result = append(result, 0)
			}
			if n := utf8.RuneCountInString(cell); n > result[column] {
				result[column] = n
			}
		}
	}
	return result
}

// wrap breaks text into lines of at most width runes, never splitting a
// word. Every line after the first is prefixed with indent spaces — a
// hanging indent taken from the block's own gutter — and that prefix
// counts against width, since the returned lines are printed as-is.
func wrap(text string, width, indent int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	prefix := strings.Repeat(" ", indent)
	linePrefix := ""
	line := words[0]
	var lines []string

	for _, word := range words[1:] {
		candidate := linePrefix + line + " " + word
		if utf8.RuneCountInString(candidate) <= width {
			line += " " + word
			continue
		}
		lines = append(lines, linePrefix+line)
		line = word
		linePrefix = prefix
	}
	lines = append(lines, linePrefix+line)

	return hardSplitOverlongLines(lines, width)
}

// hardSplitOverlongLines is wrap's fallback for a "word" (a run of
// non-whitespace, by strings.Fields' own definition) that alone already
// exceeds width: it has no space to break at, so the greedy word-wrap above
// returns it whole, wider than width, on a line of its own. It can only
// ever be placed alone — any candidate joining it to anything else is
// already over width before the join is even tried — so there is nothing
// else sharing that line to disturb by cutting it. Measured live against a
// real collision value: fallow's `--format json` output is compact with no
// internal whitespace at all, so the whole value is one unbreakable word by
// this definition. Rather than special-case that inside the greedy loop,
// every already-assembled line is checked once, here, after wrapping: a
// still-too-long line is carved into width-rune pieces with no separator —
// "the bound, not the break points" (this function's own long-standing
// rule) applies even when there is no break point at all.
func hardSplitOverlongLines(lines []string, width int) []string {
	var out []string
	for _, line := range lines {
		runes := []rune(line)
		for len(runes) > width {
			out = append(out, string(runes[:width]))
			runes = runes[width:]
		}
		out = append(out, string(runes))
	}
	return out
}

// WriteHuman renders r as the report a person reads: one summary line, then
// per-step detail grouped by outcome, then a closing tally. Every block
// after the summary is built from the same Report value the JSON twin
// renders — there is no second computation here.
func WriteHuman(w io.Writer, r Report) error {
	var b strings.Builder

	writeHeaderBlock(&b, r)
	writeSummaryLine(&b, r)
	writeAppliedBlock(&b, r)
	writeFailedBlock(&b, r)
	writeRetractedBlock(&b, r)
	writeDelegatedBlock(&b, r)
	writeSatisfiedBlock(&b, r)
	writeNotReachedBlock(&b, r)
	writeNotesBlock(&b, r)
	writeClosingBlock(&b, r)

	_, err := io.WriteString(w, b.String())
	return err
}

func formatMS(ms int64) string {
	return fmt.Sprintf("%.2fs", float64(ms)/1000)
}

func stepsWithStatus(steps []StepResult, status Status) []StepResult {
	var matched []StepResult
	for _, step := range steps {
		if step.Status == status {
			matched = append(matched, step)
		}
	}
	return matched
}

// writeHeaderBlock states the project context every per-step line silently
// assumed, before any per-step detail (defect 12, gap 7): which directory
// holds the JS project, which package manager and test runner were
// detected, which presets matched, and where dharness's own files live.
// Omitted entirely when Root is empty — a Report built to exercise one
// block in isolation carries no project to describe, the same "nothing to
// say" rule writeNotesBlock already follows for an empty Notes slice.
//
// This is a deliberately simplified single-column layout, not
// target-report.md's two-column grid: target-report.md is a design
// reference, not a byte contract, and the fact this block exists to carry
// (defect 12) is the project context, not its exact grid position. The
// approved report's "read" line (which config files this run opened) is not
// reproduced here — there is no tracked concept of "files read this run" in
// the model yet, and inventing one to fill a header line would be exactly
// the fabrication §09 forbids.
func writeHeaderBlock(b *strings.Builder, r Report) {
	if r.Root == "" {
		return
	}

	fmt.Fprintf(b, "dharness %s %s sync %s %s\n\n", r.Version, glyphSeparator, glyphSeparator, r.Root)

	jsProject := r.Source
	if jsProject == "" {
		jsProject = "repository root"
	}
	presets := "none matched"
	if len(r.Presets) > 0 {
		presets = strings.Join(r.Presets, ", ")
	}

	fmt.Fprintf(b, "  js project       %s\n", jsProject)
	fmt.Fprintf(b, "  package manager  %s\n", orNone(r.PackageManager))
	fmt.Fprintf(b, "  test runner      %s\n", orNone(r.TestRunner))
	fmt.Fprintf(b, "  presets          %s\n", presets)
	if r.OwnedDir != "" {
		fmt.Fprintf(b, "  owned files      %s/\n", r.OwnedDir)
	}
	b.WriteString("\n")
}

// orNone reports "none detected" for a field detection found nothing for,
// rather than an empty gap in the header line that reads as a rendering bug
// instead of an honestly measured absence.
func orNone(s string) string {
	if s == "" {
		return "none detected"
	}
	return s
}

// stepLabel is the "n/total" position gap 6 asks for: the JSON twin already
// carries StepResult.N, and the human view rendered nothing from it.
func stepLabel(n, total int) string {
	return fmt.Sprintf("%d/%d", n, total)
}

func writeSummaryLine(b *strings.Builder, r Report) {
	fmt.Fprintf(b, "%s %d steps %s %d applied %s %d delegated %s %d satisfied %s %d failed   %s\n\n",
		glyphSummary, r.Summary.Steps,
		glyphSeparator, r.Summary.Applied,
		glyphSeparator, r.Summary.Delegated,
		glyphSeparator, r.Summary.Satisfied,
		glyphSeparator, r.Summary.Failed,
		formatMS(r.Summary.MS))
}

func changeGlyph(k Kind) string {
	switch k {
	case Created:
		return "+"
	case Modified:
		return "~"
	case Unchanged:
		return "="
	default:
		return "?"
	}
}

// sectionRule is the two-rune section marker (── already ships on the gate
// path, internal/cli/check.go:122). It is built from two glyphRule
// occurrences rather than strings.Repeat(glyphRule, N), because a decorative
// repeat count is a literal nothing observes, and an untested literal is a
// mutant with nothing to kill it (mutation-tdd: prefer production
// simplification over a test that pins decoration).
const sectionRule = glyphRule + glyphRule

func writeAppliedBlock(b *strings.Builder, r Report) {
	steps := stepsWithStatus(r.Steps, Applied)
	if len(steps) == 0 {
		return
	}

	fmt.Fprintf(b, "%s Applied (%d) %s\n\n", sectionRule, len(steps), sectionRule)
	rows := make([][]string, len(steps))
	for i, step := range steps {
		rows[i] = []string{stepLabel(step.N, r.Summary.Steps), step.ID, formatMS(step.MS)}
	}
	w := widths(rows)
	for i, row := range rows {
		fmt.Fprintf(b, " %s %-*s %-*s  %s\n", glyphApplied, w[0], row[0], w[1], row[1], row[2])
		// Every line of a step's captured subprocess output is framed under
		// its own gutter glyph, not only the transcript's first line — a
		// single prefix over the whole blob left everything after the first
		// line unframed (gap 2, defect 5 only half fixed).
		if transcript := strings.TrimRight(steps[i].Transcript, "\n"); transcript != "" {
			for _, line := range strings.Split(transcript, "\n") {
				fmt.Fprintf(b, "         %s %s\n", glyphGutter, line)
			}
		}
		for _, installed := range steps[i].Installed {
			fmt.Fprintf(b, "         %s installed %s\n", glyphGutter, installed)
		}
		for _, change := range steps[i].Wrote {
			fmt.Fprintf(b, "         %s %s\n", changeGlyph(change.Kind), change.Path)
		}
	}
	// The legend names every glyph this block itself uses (gap 8):
	// created/modified/unchanged for a touched file, and the gutter for
	// captured subprocess output — printed once here since this is the one
	// block where all four can appear.
	fmt.Fprintf(b, "\n         legend  %s created   %s modified   %s unchanged   %s subprocess\n",
		changeGlyph(Created), changeGlyph(Modified), changeGlyph(Unchanged), glyphGutter)
	b.WriteString("\n")
}

// writeFailedBlock renders the one step whose Apply failed this run — the
// step a failure variant's report is built around. Its own row shape mirrors
// writeAppliedBlock's (it did run, and for as long as it took), with the
// captured cause printed beneath it when there is one.
func writeFailedBlock(b *strings.Builder, r Report) {
	steps := stepsWithStatus(r.Steps, Failed)
	if len(steps) == 0 {
		return
	}

	fmt.Fprintf(b, "%s Failed (%d) %s\n\n", sectionRule, len(steps), sectionRule)
	rows := make([][]string, len(steps))
	for i, step := range steps {
		rows[i] = []string{stepLabel(step.N, r.Summary.Steps), step.ID, formatMS(step.MS)}
	}
	w := widths(rows)
	for i, row := range rows {
		fmt.Fprintf(b, " %s %-*s %-*s  %s\n", glyphFailed, w[0], row[0], w[1], row[1], row[2])
		if steps[i].Error != "" {
			fmt.Fprintf(b, "         %s\n", steps[i].Error)
		}
	}
	b.WriteString("\n")
}

// writeRetractedBlock renders every step this run had already applied and
// reported before a later step failed — the explicit-retraction obligation
// project-sync's added requirement states: a status already printed is
// retracted by name here, not merely contradicted by omission (defect 3).
func writeRetractedBlock(b *strings.Builder, r Report) {
	steps := stepsWithStatus(r.Steps, Retracted)
	if len(steps) == 0 {
		return
	}

	fmt.Fprintf(b, "%s Retracted (%d) %s\n\n", sectionRule, len(steps), sectionRule)
	for _, step := range steps {
		fmt.Fprintf(b, " %s %s   %s   rolled back\n", glyphGutter, stepLabel(step.N, r.Summary.Steps), step.ID)
	}
	b.WriteString("\n")
}

// writeNotReachedBlock renders every step in Plan() this run never
// attempted because it stopped on an earlier failure — reported by name,
// not silently dropped, which is defect 1's ambiguous-absence failure
// applied to the failure path.
func writeNotReachedBlock(b *strings.Builder, r Report) {
	steps := stepsWithStatus(r.Steps, NotReached)
	if len(steps) == 0 {
		return
	}

	fmt.Fprintf(b, "%s Not reached (%d) %s\n\n", sectionRule, len(steps), sectionRule)
	for _, step := range steps {
		fmt.Fprintf(b, " %s %s   %s\n", glyphSeparator, stepLabel(step.N, r.Summary.Steps), step.ID)
	}
	b.WriteString("\n")
}

func writeDelegatedBlock(b *strings.Builder, r Report) {
	steps := stepsWithStatus(r.Steps, Delegated)
	if len(steps) == 0 {
		return
	}

	fmt.Fprintf(b, "%s Left to you (%d) %s\n\n", sectionRule, len(steps), sectionRule)
	for _, step := range steps {
		fmt.Fprintf(b, " ! %s   %s\n", stepLabel(step.N, r.Summary.Steps), step.ID)
		switch {
		case len(step.Collisions) > 0:
			for _, collision := range step.Collisions {
				writeCollision(b, collision)
			}
		case step.Why != "":
			// The 3 subtracted here is the same 3 the Fprintf below adds:
			// wrap is asked for the width that is actually left once this
			// block's own prefix is accounted for. Passing wrapWidth whole
			// and then prefixing produced lines of wrapWidth+3 — measured
			// live at 71 to 73 runes across three delegated steps, in a
			// report whose every other block holds 70.
			for _, line := range wrap(step.Why, wrapWidth-3, 3) {
				fmt.Fprintf(b, "   %s\n", line)
			}
		}
	}
	b.WriteString("\n")
}

// collisionValueUnavailable is what a colliding value's own side states when
// it could not be measured — never the truncated textual fragment defect 8
// used to show, and never a guessed value (§09/§17, the same
// absent-versus-fabricated rule config-collision's own requirement states
// for effective).
const collisionValueUnavailable = "could not be shown"

// writeCollision renders one colliding key's full structured fact (gap 1):
// dharness's own value and where it lives, the project's value and where it
// lives, which one a measurement says actually runs, and the lettered ways
// to resolve it — everything the JSON twin already carried that the human
// view rendered nothing of beyond the bare key name.
func writeCollision(b *strings.Builder, c Collision) {
	fmt.Fprintf(b, "\n   `%s` has two owners        id  %s\n\n", c.Key, c.ID)

	ours, theirs, hidden := narrowToDifferences(c.Ours.Value, c.Theirs.Value)
	writeDeclaredSide(b, "dharness", c.Ours, ours, effectiveMark(c, "ours"), false)
	writeDeclaredSide(b, "project", c.Theirs, theirs, effectiveMark(c, "theirs"), true)
	if hidden > 0 {
		// Two different reasons put a key in this set — it holds the same
		// value on both sides, or only the resolved side declares it at
		// all — so the note names both rather than calling every hidden
		// key identical. Measured live, all 13 keys hidden from a real
		// fallow value were defaults dharness never declared, and a note
		// reading "13 identical" was false about every one of them.
		fmt.Fprintf(b, "%snote: %d key(s) hidden — same on both sides, or defaults only fallow sets\n",
			strings.Repeat(" ", declaredSideIndent), hidden)
	}
	b.WriteString("\n")

	if len(c.Resolutions) > 0 {
		b.WriteString("   Pick one owner:\n")
		for i, slug := range c.Resolutions {
			fmt.Fprintf(b, "     %s  %s\n", resolutionLetter(i), collisionResolutionText(slug, c))
		}
		b.WriteString("\n")
	}

	b.WriteString("   Then re-run `dharness sync` to confirm.\n")
}

// declaredSideIndent is the column every value line starts at: 5 leading
// spaces + an 8-rune label field + 2 spaces (fmt.Sprintf("     %-8s  ",
// label)) — the same column the location line above it already starts its
// own value at. wrap's own indent parameter stays 0 at this call site,
// exactly like writeSatisfiedBlock's evidence: this function prints that
// fixed prefix itself, on every line, so wrap must not add a second hanging
// indent on top of it.
const declaredSideIndent = 15

// resolvedNote marks the project side of a collision as fallow's resolved
// value rather than the text the project actually wrote, whenever it was
// actually measured (setup.Collisions only ever populates Theirs.Value from
// `fallow config --format json` — the fully resolved config, every default
// filled in, never the declared text). A run measured against a real
// project made the gap concrete: dharness's own declared value carried 3
// fields, fallow's resolved value for the same key carried 15. Printed side
// by side with no distinction, a reader expects the two blobs to be
// directly comparable and cannot find the difference — the one thing this
// block exists to show. Narrowing the resolved side to only the keys that
// differ from Ours, the way fallow's own dupes report hides unchanged
// clones (`note: hid N clone groups`), was considered and rejected here: it
// reads better, but it is a second comparison feature with its own tests,
// not a one-line fix to an unlabeled value that is already correct.
//
// Kept to one short word, deliberately, appended to the location line
// rather than the (already wrapped) value line: a paragraph-length
// explanation would itself need wrapping and start competing with the
// value for the reader's attention. It is not itself wrapped — every path
// this block has ever printed, measured live, stays well inside the
// remaining budget, and the location line was never bounded before this
// note existed either; a genuinely pathological path is a pre-existing,
// separate gap this fix does not take on.
const resolvedNote = "  (fallow-resolved)"

// writeDeclaredSide renders one side of a collision — its location (path,
// and line when known, plus resolvedNote when this side's value is
// fallow's measured, fully resolved config rather than declared text) and
// its value, or collisionValueUnavailable when it could not be measured,
// plus mark when a measurement says this is the side that runs.
func writeDeclaredSide(b *strings.Builder, label string, d Declared, value, mark string, resolved bool) {
	location := d.Path
	if d.Line > 0 {
		location = fmt.Sprintf("%s:%d", d.Path, d.Line)
	}
	note := ""
	if resolved && d.Value != nil {
		note = resolvedNote
	}
	fmt.Fprintf(b, "     %-8s  %s%s\n", label, location, note)

	// A colliding value can be hundreds of characters long — fallow's own
	// resolved config for one key measured 345 characters live against a
	// real project, the widest line the whole report ever produced, and it
	// broke the block's layout unwrapped. It is wrapped here exactly like
	// every other wrapped field (wrap, wrapWidth), at declaredSideIndent —
	// the same column the location line above it already starts a value
	// at.
	//
	// The effective mark, when present, is appended to the LAST wrapped
	// line rather than printed on a line of its own: a separate line would
	// read as marking the whole side rather than the specific value it
	// measures, and the existing rendering test (and every real reading of
	// this block) expects it directly attached to what it marks. To keep
	// that line within wrapWidth with the mark attached, the value is
	// wrapped inside a width narrowed by the mark's own length whenever a
	// mark is present — a small, uniform tightening applied to every line
	// rather than a special case for only the last one. available is never
	// non-positive for any real caller: wrapWidth (70) minus
	// declaredSideIndent (15) minus the mark's own fixed length (18) still
	// leaves room to spare.
	available := wrapWidth - declaredSideIndent
	if mark != "" {
		available -= utf8.RuneCountInString(mark)
	}
	valueIndent := strings.Repeat(" ", declaredSideIndent)
	lines := wrap(value, available, 0)
	for i, line := range lines {
		suffix := ""
		if i == len(lines)-1 {
			suffix = mark
		}
		fmt.Fprintf(b, "%s%s%s\n", valueIndent, line, suffix)
	}
}

// narrowToDifferences renders each side of a collision carrying only the
// keys the two sides disagree on, and reports how many identical keys it
// left out. It falls back to the whole value — returning 0 hidden — for
// anything it cannot narrow honestly: a side that was never measured, a
// value that is not a JSON object, or two objects that share no key at all.
//
// This is what the block is for. Measured against a real project,
// dharness's side carried 3 keys and fallow's resolved side carried 15,
// twelve of them identical defaults; the single word that differed was
// buried in nine wrapped lines. The keys that agree carry no information
// for a decision about the keys that do not, and hiding them is the same
// move fallow makes in its own dupes report (`note: hid 8 clone groups
// below minOccurrences=3`) — shorten, then say what was shortened.
//
// Only the human view narrows. The JSON twin keeps both values whole,
// because a machine reading `theirs` wants the value that runs, not a
// diff computed for a person.
func narrowToDifferences(ours, theirs *json.RawMessage) (string, string, int) {
	whole := func(raw *json.RawMessage) string {
		if raw == nil {
			return collisionValueUnavailable
		}
		return string(*raw)
	}

	o, oOK := objectFields(ours)
	t, tOK := objectFields(theirs)
	if !oOK || !tOK {
		return whole(ours), whole(theirs), 0
	}

	var keys []string
	for key := range o {
		keys = append(keys, key)
	}
	for key := range t {
		if _, both := o[key]; !both {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	var keptOurs, keptTheirs []string
	hidden := 0
	for _, key := range keys {
		ov, inOurs := o[key]
		tv, inTheirs := t[key]
		if inOurs && inTheirs && ov == tv {
			hidden++
			continue
		}
		// A key only the resolved side carries is one of fallow's own
		// defaults, not something the project chose and not something
		// dharness disagrees with. Measured live: dharness declared 3
		// keys, fallow's resolved value carried 15, and the 12 extras
		// were every default fallow fills in. Counting them as
		// differences is what made the first narrowing rule fall back to
		// the whole value on the one case it was written for.
		if !inOurs {
			hidden++
			continue
		}
		if inOurs {
			keptOurs = append(keptOurs, fmt.Sprintf("%q:%s", key, ov))
		}
		if inTheirs {
			keptTheirs = append(keptTheirs, fmt.Sprintf("%q:%s", key, tv))
		}
	}
	if hidden == 0 {
		return whole(ours), whole(theirs), 0
	}
	return "{" + strings.Join(keptOurs, ",") + "}",
		"{" + strings.Join(keptTheirs, ",") + "}",
		hidden
}

// objectFields decodes one side into its top-level keys with each value
// left as the text it arrived as, so two sides are compared as JSON rather
// than as Go values dharness does not own the types for.
func objectFields(raw *json.RawMessage) (map[string]string, bool) {
	if raw == nil {
		return nil, false
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(*raw, &decoded); err != nil {
		return nil, false
	}
	fields := map[string]string{}
	for key, value := range decoded {
		var buf bytes.Buffer
		if err := json.Compact(&buf, value); err != nil {
			return nil, false
		}
		fields[key] = buf.String()
	}
	return fields, true
}

// effectiveMark names which side a measurement says actually runs, or ""
// when effective was never measured — an absent effective must never imply
// either side (config-collision's own "absent, never fabricated" rule).
func effectiveMark(c Collision, side string) string {
	if c.Effective != nil && *c.Effective == side {
		return "   ← this one runs"
	}
	return ""
}

// resolutionLetter names the nth resolution a, b, c... — there are never
// more than two today (delete-theirs, move-into-ours), but the letters are
// derived from position rather than hardcoded so a third resolution would
// not need this function rewritten.
func resolutionLetter(i int) string {
	return string(rune('a' + i))
}

// collisionResolutionText renders one resolution slug as the sentence a
// person acts on, naming the actual paths this collision found — not a
// generic instruction that could belong to any collision.
func collisionResolutionText(slug string, c Collision) string {
	switch slug {
	case "delete-theirs":
		return fmt.Sprintf("delete `%s` from %s", c.Key, c.Theirs.Path)
	case "move-into-ours":
		return fmt.Sprintf("move your value into %s, delete the copy", c.Ours.Path)
	default:
		return slug
	}
}

func writeSatisfiedBlock(b *strings.Builder, r Report) {
	steps := stepsWithStatus(r.Steps, Satisfied)
	if len(steps) == 0 {
		return
	}

	fmt.Fprintf(b, "%s Already in place (%d) %s\n\n", sectionRule, len(steps), sectionRule)
	// Evidence renders on its own indented line beneath the step's ID rather
	// than in a column aligned across every row: this product's step IDs are
	// full sentences, and a real run measured what that column-alignment
	// forces — a long ID anywhere in the block leaves almost no room for
	// evidence in every row, so even a short fact wraps needlessly (gap 4's
	// wrapping requirement, satisfied here without the side effect). The
	// fixed 9-space indent matches the Applied block's own sub-detail lines
	// (Transcript/Installed/Wrote), so both blocks read the same way.
	for _, step := range steps {
		fmt.Fprintf(b, " %s %s   %s\n", glyphSatisfied, stepLabel(step.N, r.Summary.Steps), step.ID)
		for _, line := range wrap(step.Evidence, wrapWidth-9, 0) {
			fmt.Fprintf(b, "         %s\n", line)
		}
	}
	b.WriteString("\n")
}

func writeNotesBlock(b *strings.Builder, r Report) {
	if len(r.Notes) == 0 {
		return
	}

	fmt.Fprintf(b, "%s Notes (%d) %s\n\n", sectionRule, len(r.Notes), sectionRule)
	for _, note := range r.Notes {
		fmt.Fprintf(b, " i %s\n", note.Kind)
		for _, entry := range note.Entries {
			fmt.Fprintf(b, "   %s %s\n", glyphSeparator, entry)
		}
		for _, line := range wrap(note.Reason, wrapWidth, 3) {
			fmt.Fprintf(b, "   %s\n", line)
		}
	}
	b.WriteString("\n")
}

// writeClosingBlock ends the report with a block distinct from the summary
// line at the top, carrying the same counts, the elapsed time and the exit
// code — plus, on a clean run, a next pointer when work remains, and on a
// failed run, the explicit retraction project-sync's added requirement
// states. Evidence measured for this run, when present, is carried in
// either case: it is not gated on whether anything is left (design.md
// Decision 8, change #2).
func writeClosingBlock(b *strings.Builder, r Report) {
	fmt.Fprintf(b, "%s\n", strings.Repeat(glyphRule, wrapWidth))

	if r.Summary.Failed > 0 {
		writeFailureTally(b, r)
	} else {
		writeSuccessTally(b, r)
	}

	if r.Evidence != nil {
		fmt.Fprintf(b, "\n  measured  %d test(s) related to %s\n", r.Evidence.RelatedTests, r.Evidence.MeasuredPath)
	}
}

func writeSuccessTally(b *strings.Builder, r Report) {
	fmt.Fprintf(b, "%s %d applied %s %d delegated %s %d satisfied %s %d failed   %s  exit %d\n",
		glyphApplied, r.Summary.Applied, glyphSeparator, r.Summary.Delegated, glyphSeparator,
		r.Summary.Satisfied, glyphSeparator, r.Summary.Failed,
		formatMS(r.Summary.MS), r.Exit)

	if next := firstDelegatedID(r.Steps); next != "" {
		fmt.Fprintf(b, "\n  next  %s\n", next)
	}
}

// writeFailureTally renders the failure variant's closing line. It names
// every retracted step explicitly — a status already printed is retracted
// by name, not merely contradicted by omission (project-sync's added
// requirement) — and it never claims more than Writer.Undo actually
// covers: no directory removal, no "everything restored", only the
// retraction itself and the measured tally.
func writeFailureTally(b *strings.Builder, r Report) {
	retracted := stepsWithStatus(r.Steps, Retracted)
	names := make([]string, len(retracted))
	for i, step := range retracted {
		names[i] = step.ID
	}

	msg := "rolled back — nothing was applied"
	if len(names) > 0 {
		msg += ", including " + strings.Join(names, ", ")
	}
	fmt.Fprintf(b, "%s %s   %s  exit %d\n", glyphFailed, msg, formatMS(r.Summary.MS), r.Exit)
	fmt.Fprintf(b, "\n  %d failed %s %d retracted %s %d not reached\n",
		r.Summary.Failed, glyphSeparator, r.Summary.Retracted, glyphSeparator, len(stepsWithStatus(r.Steps, NotReached)))
}

// firstDelegatedID names the first delegated step in plan order, or "" when
// none remain. The closing block's next pointer, distinct from the earlier
// per-step "Left to you" detail: a run with delegated work names it again
// here so the closing tally is a complete answer on its own.
//
// A step carrying a collision names the collision's own addressable handle
// (Collision.ID, e.g. "sync:collision/duplicates") rather than the step's
// prose heading (gap 9) — the same distinction design.md Decision 1 draws
// between StepResult.ID (a sentence) and Collision.ID (the one thing this
// report points at).
func firstDelegatedID(steps []StepResult) string {
	for _, step := range steps {
		if step.Status == Delegated {
			if len(step.Collisions) > 0 {
				return step.Collisions[0].ID
			}
			return step.ID
		}
	}
	return ""
}
