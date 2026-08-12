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
