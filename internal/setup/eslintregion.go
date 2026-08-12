package setup

import "strings"

// The two marker pairs eslintExtendsStep writes into a project's own
// eslint.config.js: one for the import declarations, one for the spread
// element inside the default-exported array. Same grammar as owned.go's
// presetBegin/presetEnd — a reader who has seen one recognises the other
// without documentation — but individually named and individually
// addressable, so a re-run can replace exactly its own region without
// touching the other one.
//
// These four strings ship into every adopting repository and cannot be
// changed cheaply afterwards (design decision 4). Settled here, in this
// slice's review.
const (
	eslintImportBegin = "// dharness:eslint-import begin — rewritten by `dharness sync`; edits here are lost."
	eslintImportEnd   = "// dharness:eslint-import end"
	eslintLayerBegin  = "// dharness:eslint-layer begin — rewritten by `dharness sync`; edits here are lost."
	eslintLayerEnd    = "// dharness:eslint-layer end"
)

// markerState is a three-way answer, not a bool, because absent and
// malformed lead to opposite actions: absent means insert, malformed means
// refuse and delegate. regionBounds in owned.go collapses both into
// "insert", which is correct for a file dharness wrote every byte of and is
// exactly the guess that corrupts a file it did not.
type markerState int

const (
	markersAbsent markerState = iota
	markersPresent
	markersMalformed
)

// markerRegion locates one marker pair's byte range in raw, including its
// own start-of-line and trailing newline — the same bounds regionBounds
// computes in owned.go, but with a third outcome instead of a bool.
//
// Malformed is: a begin with no matching end, an end with no matching
// begin, an end before its begin, or more than one of either marker. Any of
// these is a half-written region, and dharness refuses to guess at one
// rather than risk replacing or inserting into the wrong bytes. A malformed
// answer's bounds are always (0, 0) — callers must branch on state, never
// on whether from/to look non-zero.
func markerRegion(raw, begin, end string) (from, to int, state markerState) {
	beginCount := strings.Count(raw, begin)
	endCount := strings.Count(raw, end)

	if beginCount == 0 && endCount == 0 {
		return 0, 0, markersAbsent
	}
	if beginCount != 1 || endCount != 1 {
		return 0, 0, markersMalformed
	}

	beginIdx := strings.Index(raw, begin)
	endIdx := strings.Index(raw, end)
	if endIdx < beginIdx {
		return 0, 0, markersMalformed
	}

	lineStart := strings.LastIndex(raw[:beginIdx], "\n") + 1
	lineEnd := endIdx + len(end)
	if nl := strings.Index(raw[lineEnd:], "\n"); nl != -1 {
		lineEnd += nl + 1
	} else {
		lineEnd = len(raw)
	}
	return lineStart, lineEnd, markersPresent
}

// regionIndent returns the leading spaces and tabs on the line starting at
// from — markerRegion's own from is already the start of that line, so a
// replace re-renders at the indent the earlier run wrote, without needing to
// re-run jsconfig.Analyze's position rule to rediscover it.
func regionIndent(raw string, from int) string {
	i := from
	for i < len(raw) && (raw[i] == ' ' || raw[i] == '\t') {
		i++
	}
	return raw[from:i]
}

// regionLineEnding reports "\r\n" or "\n", read off the region's own first
// line break — the convention the earlier run wrote, preserved rather than
// reselected.
func regionLineEnding(raw string, from int) string {
	i := strings.IndexByte(raw[from:], '\n')
	if i == -1 {
		return "\n"
	}
	nl := from + i
	if nl > 0 && raw[nl-1] == '\r' {
		return "\r\n"
	}
	return "\n"
}

// replaceRange returns src with the bytes between from and to replaced by
// region. owned.go's replaceRegion does the same job for a file dharness
// owns outright, at bounds a single fixed marker pair locates; this is the
// equivalent for the project's own file, where two independent regions each
// carry their own bounds from a marker scan (design decision 4: same
// grammar, no shared code).
func replaceRange(src []byte, from, to int, region string) []byte {
	out := make([]byte, 0, len(src)-(to-from)+len(region))
	out = append(out, src[:from]...)
	out = append(out, region...)
	out = append(out, src[to:]...)
	return out
}
