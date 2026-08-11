package setup

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Disble/dharness/internal/preset"
)

// presetBegin and presetEnd delimit the region ownedFilesStep.Apply rewrites
// inside .dharness/fallow.jsonc on every sync. Everything outside them —
// most of all the `boundaries` block the agent writes below — is never
// touched (design decision 8).
//
// The wording is meant to be read, not just matched: whoever opens this file
// by hand can tell, without documentation, that the region is machine-
// written and that an edit inside it is lost on the next sync.
const (
	presetBegin = "// dharness:presets begin — rewritten by `dharness sync`; edits here are lost."
	presetEnd   = "// dharness:presets end"
)

// architectureSkeleton is what dharness writes for a project no preset
// contributes to — today's fallow.jsonc, unchanged. It doubles as the base a
// region is inserted into the first time a preset does contribute, so a
// project that starts generic and later matches a framework gets the region
// added rather than the file rewritten around it.
const architectureSkeleton = "{\n  // dharness writes this file; the architecture below is decided by analysis,\n  // not by detection. Declare `boundaries` here rather than in the project's\n  // own fallow config: `extends` replaces this key, it does not merge it.\n  //\n  // See `dharness sync`.\n}\n"

// presetRegion renders the union of matches' contributed facts as the JSONC
// text that goes between the two markers, or "" when nothing is contributed
// — the byte-identity path a generic-matched project depends on.
//
// Composition lives here, in internal/setup, rather than in internal/preset:
// what a fact means once rendered — union a list, comment a losing scalar —
// is how the region is written, and internal/preset holds no writer or
// rendering policy (design decision 1, and the Data Flow section of
// design.md which routes composition through setup.presetRegion).
func presetRegion(matches []preset.Match) string {
	facts := composeFacts(matches)
	if len(facts) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("  " + presetBegin + "\n")
	for _, fact := range facts {
		for _, comment := range fact.comments {
			b.WriteString("  // " + comment + "\n")
		}
		raw, err := json.Marshal(fact.value)
		if err != nil {
			// Manifest.Validate already rejects an unencodable value in a
			// registry test, so a run never reaches this branch — an
			// authoring bug, not a state a project can cause.
			panic(fmt.Sprintf("preset fact %q does not encode: %v", fact.key, err))
		}
		fmt.Fprintf(&b, "  %q: %s,\n", fact.key, raw)
	}
	b.WriteString("  " + presetEnd + "\n")
	return b.String()
}

// composedFact is one key's resolved contribution after the union/collision
// rule runs, plus the comment lines that justify it.
type composedFact struct {
	key      string
	value    any
	comments []string
}

// composeFacts applies design decision 3's rule per key: a list unions
// across every contributor in resolve order, duplicates removed by the
// existing dedupe helper; a scalar is won by the first contributor — Root
// scope, since matches arrives Root-before-Source from preset.Resolve — and
// every losing contributor is rendered as a comment beside the winner.
func composeFacts(matches []preset.Match) []composedFact {
	type contribution struct {
		presetID string
		fact     preset.Fact
	}

	contributions := map[string][]contribution{}
	order := preset.Keys(matches)
	for _, match := range matches {
		for _, fact := range match.Manifest.Facts {
			contributions[fact.Key] = append(contributions[fact.Key], contribution{match.ID, fact})
		}
	}

	facts := make([]composedFact, 0, len(order))
	for _, key := range order {
		contribs := contributions[key]
		first := contribs[0].fact

		if list, ok := first.Value.([]string); ok {
			_ = list
			var union []string
			var comments []string
			for _, c := range contribs {
				elements, _ := c.fact.Value.([]string)
				union = append(union, elements...)
				comments = append(comments, fmt.Sprintf("%s: %s", c.presetID, c.fact.Because))
			}
			facts = append(facts, composedFact{key: key, value: dedupe(union), comments: comments})
			continue
		}

		winner := contribs[0]
		comments := []string{fmt.Sprintf("%s: %s", winner.presetID, winner.fact.Because)}
		for _, c := range contribs[1:] {
			comments = append(comments, fmt.Sprintf(
				"(overridden by %s) %s: %s = %v", winner.presetID, c.presetID, c.fact.Because, c.fact.Value))
		}
		facts = append(facts, composedFact{key: key, value: winner.fact.Value, comments: comments})
	}
	return facts
}

// replaceRegion returns existing with the preset region replaced by region.
//
//   - region == "" — existing is returned byte-for-byte unchanged. This is
//     the path a generic-matched project depends on: no preset contributes
//     anything, so the file this run writes is identical to the file a run
//     with no region machinery at all would have written.
//   - the markers are present in existing — only the bytes between them
//     change; every other byte, most of all the boundaries block the agent
//     writes below them, is untouched.
//   - the markers are absent and region != "" — region is inserted
//     immediately after the first "{", the byte dharness itself always
//     writes first, whatever comes after it in the wild.
func replaceRegion(existing, region string) string {
	if region == "" {
		return existing
	}

	if begin, end, ok := regionBounds(existing); ok {
		return existing[:begin] + region + existing[end:]
	}

	brace := strings.Index(existing, "{")
	if brace == -1 {
		return existing
	}
	insertAt := brace + 1
	return existing[:insertAt] + "\n" + region + existing[insertAt:]
}

// regionBounds locates the byte range of a previously-written region,
// including its own start-of-line and trailing newline, so replaceRegion can
// splice it out cleanly and regionBytes can compare it as a whole.
func regionBounds(raw string) (begin, end int, ok bool) {
	beginIdx := strings.Index(raw, presetBegin)
	endIdx := strings.Index(raw, presetEnd)
	if beginIdx == -1 || endIdx == -1 || endIdx < beginIdx {
		return 0, 0, false
	}

	lineStart := strings.LastIndex(raw[:beginIdx], "\n") + 1
	lineEnd := endIdx + len(presetEnd)
	if nl := strings.Index(raw[lineEnd:], "\n"); nl != -1 {
		lineEnd += nl + 1
	} else {
		lineEnd = len(raw)
	}
	return lineStart, lineEnd, true
}

// regionBytes returns the region currently written in raw, or "" when the
// markers are absent — the other half of ownedFilesStep.Satisfied's
// byte-comparison.
func regionBytes(raw string) string {
	begin, end, ok := regionBounds(raw)
	if !ok {
		return ""
	}
	return raw[begin:end]
}
