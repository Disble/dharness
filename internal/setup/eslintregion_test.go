package setup

import "testing"

// TestMarkerRegionDistinguishesAbsentFromMalformed pins design decision 4's
// three-way answer: absent and malformed are not the same outcome, and all
// four malformed shapes must be reported as malformed rather than guessed
// at — regionBounds in owned.go collapses both into "insert", which is
// wrong here because this marker pair lives in a file dharness did not
// write every byte of.
func TestMarkerRegionDistinguishesAbsentFromMalformed(t *testing.T) {
	begin, end := eslintLayerBegin, eslintLayerEnd

	t.Run("absent", func(t *testing.T) {
		_, _, state := markerRegion("export default [];\n", begin, end)
		if state != markersAbsent {
			t.Errorf("markerRegion() state = %v, want markersAbsent", state)
		}
	})

	t.Run("present", func(t *testing.T) {
		raw := begin + "\n" + end + "\n"
		_, _, state := markerRegion(raw, begin, end)
		if state != markersPresent {
			t.Errorf("markerRegion() state = %v, want markersPresent", state)
		}
	})

	malformed := map[string]string{
		"begin with no matching end": begin + "\n",
		"end with no matching begin": end + "\n",
		"end before its begin":       end + "\n" + begin + "\n",
		"more than one begin":        begin + "\n" + begin + "\n" + end + "\n",
		"more than one end":          begin + "\n" + end + "\n" + end + "\n",
	}
	for name, raw := range malformed {
		t.Run(name, func(t *testing.T) {
			_, _, state := markerRegion(raw, begin, end)
			if state != markersMalformed {
				t.Errorf("markerRegion(%q) state = %v, want markersMalformed", raw, state)
			}
		})
	}
}

// TestMarkerRegionBoundsIncludeTheWholeLine pins the present-path bounds:
// from is the start of the begin marker's own line, to is just past the end
// marker's trailing newline — the same shape regionBounds already
// establishes in owned.go, so a caller can splice the region out cleanly.
func TestMarkerRegionBoundsIncludeTheWholeLine(t *testing.T) {
	raw := "before\n  " + eslintLayerBegin + "\n  ...x,\n  " + eslintLayerEnd + "\nafter\n"
	from, to, state := markerRegion(raw, eslintLayerBegin, eslintLayerEnd)
	if state != markersPresent {
		t.Fatalf("markerRegion() state = %v, want markersPresent", state)
	}
	want := "  " + eslintLayerBegin + "\n  ...x,\n  " + eslintLayerEnd + "\n"
	if got := raw[from:to]; got != want {
		t.Errorf("markerRegion() region = %q, want %q", got, want)
	}
}

// TestMarkerRegionMalformedBoundsAreZero pins that a malformed answer never
// carries a stale byte range a careless caller could splice against.
func TestMarkerRegionMalformedBoundsAreZero(t *testing.T) {
	from, to, state := markerRegion(eslintLayerBegin+"\n", eslintLayerBegin, eslintLayerEnd)
	if state != markersMalformed {
		t.Fatalf("markerRegion() state = %v, want markersMalformed", state)
	}
	if from != 0 || to != 0 {
		t.Errorf("markerRegion() = (%d, %d), want (0, 0) on a malformed answer", from, to)
	}
}

// TestRegionIndentReadsTheLeadingWhitespaceAtFrom pins that the replace
// path reads the indent off the region's own line rather than assuming a
// fixed convention — the same property jsconfig's indentOfLine gives the
// insert path, reimplemented here on a string rather than shared code
// (design decision 4: same grammar, no shared code with owned.go or
// jsconfig). The whitespace-runs-to-the-end case pins the loop's exact
// boundary: a relaxed "<=" comparison reads raw[len(raw)] out of bounds.
func TestRegionIndentReadsTheLeadingWhitespaceAtFrom(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		from int
		want string
	}{
		{"no indentation", "export default", 0, ""},
		{"two-space indent", "  ...x,", 0, "  "},
		{"tab indent", "\t...x,", 0, "\t"},
		{"whitespace runs to the end of raw", "   ", 0, "   "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := regionIndent(c.raw, c.from); got != c.want {
				t.Errorf("regionIndent(%q, %d) = %q, want %q", c.raw, c.from, got, c.want)
			}
		})
	}
}

// TestRegionLineEndingReadsTheRegionsOwnEnding pins the CRLF/LF read exactly
// where jsconfig's lineEndingAt does, including the nl == 0 boundary where a
// relaxed comparison would read one byte before the start of raw, the
// nl == 1 boundary the ">" comparison depends on rather than ">=", and the
// "no further newline" case with a stray "\r" further back in raw, which a
// mutated "i == -1" check must not mistake for a CRLF two bytes away.
func TestRegionLineEndingReadsTheRegionsOwnEnding(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		from int
		want string
	}{
		{"LF", "line\nmore", 0, "\n"},
		{"CRLF", "line\r\nmore", 0, "\r\n"},
		{"newline is the very first byte", "\nmore", 0, "\n"},
		{"no further newline defaults to LF", "line", 0, "\n"},
		{"CRLF where from points directly at the newline", "x\r\ny", 2, "\r\n"},
		{"CRLF at nl == 1", "\r\nabc", 1, "\r\n"},
		{"no further newline past a stray CR earlier in raw", "ab\rcd", 4, "\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := regionLineEnding(c.raw, c.from); got != c.want {
				t.Errorf("regionLineEnding(%q, %d) = %q, want %q", c.raw, c.from, got, c.want)
			}
		})
	}
}

// TestReplaceRangeIsTheThreePartByteIdentity pins replaceRange's own
// byte-surgery identity, the same shape jsconfig.Splice's test asserts for
// an insert: the output is exactly the bytes before from, then region, then
// the bytes from to onward — not a re-render that happens to look the same.
func TestReplaceRangeIsTheThreePartByteIdentity(t *testing.T) {
	src := []byte("before[STALE]after")
	from, to := 6, 13 // "[STALE]"
	region := "[FRESH]"

	got := replaceRange(src, from, to, region)

	want := "before[FRESH]after"
	if string(got) != want {
		t.Errorf("replaceRange() = %q, want %q", got, want)
	}
}
