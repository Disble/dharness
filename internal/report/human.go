package report

import (
	"fmt"
	"io"
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
	return append(lines, linePrefix+line)
}

// WriteHuman renders r as the report a person reads: one summary line, then
// per-step detail grouped by outcome, then a closing tally. Every block
// after the summary is built from the same Report value the JSON twin
// renders — there is no second computation here.
func WriteHuman(w io.Writer, r Report) error {
	var b strings.Builder

	writeSummaryLine(&b, r)
	writeAppliedBlock(&b, r)
	writeDelegatedBlock(&b, r)
	writeSatisfiedBlock(&b, r)
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
		rows[i] = []string{step.ID, formatMS(step.MS)}
	}
	w := widths(rows)
	for i, row := range rows {
		fmt.Fprintf(b, " %s %-*s  %s\n", glyphApplied, w[0], row[0], row[1])
		if steps[i].Transcript != "" {
			fmt.Fprintf(b, "         %s %s\n", glyphGutter, steps[i].Transcript)
		}
		for _, installed := range steps[i].Installed {
			fmt.Fprintf(b, "         %s installed %s\n", glyphGutter, installed)
		}
		for _, change := range steps[i].Wrote {
			fmt.Fprintf(b, "         %s %s\n", changeGlyph(change.Kind), change.Path)
		}
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
		fmt.Fprintf(b, " ! %s\n", step.ID)
		switch {
		case len(step.Collisions) > 0:
			for _, collision := range step.Collisions {
				fmt.Fprintf(b, "   `%s` needs a decision\n", collision.Key)
			}
		case step.Why != "":
			for _, line := range wrap(step.Why, wrapWidth, 3) {
				fmt.Fprintf(b, "   %s\n", line)
			}
		}
	}
	b.WriteString("\n")
}

func writeSatisfiedBlock(b *strings.Builder, r Report) {
	steps := stepsWithStatus(r.Steps, Satisfied)
	if len(steps) == 0 {
		return
	}

	fmt.Fprintf(b, "%s Already in place (%d) %s\n\n", sectionRule, len(steps), sectionRule)
	rows := make([][]string, len(steps))
	for i, step := range steps {
		rows[i] = []string{step.ID, step.Evidence}
	}
	w := widths(rows)
	for _, row := range rows {
		fmt.Fprintf(b, " %s %-*s  %s\n", glyphGutter, w[0], row[0], row[1])
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

func writeClosingBlock(b *strings.Builder, r Report) {
	fmt.Fprintf(b, "%s\n", strings.Repeat(glyphRule, wrapWidth))
	glyph := glyphApplied
	if r.Summary.Failed > 0 {
		glyph = glyphFailed
	}
	fmt.Fprintf(b, "%s %d applied %s %d delegated %s %d satisfied %s %d failed   %s  exit %d\n",
		glyph, r.Summary.Applied, glyphSeparator, r.Summary.Delegated, glyphSeparator,
		r.Summary.Satisfied, glyphSeparator, r.Summary.Failed,
		formatMS(r.Summary.MS), r.Exit)
}
