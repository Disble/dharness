package preset

import (
	"strings"
	"testing"
)

func TestGenericAlwaysMatches(t *testing.T) {
	match, matched := generic{}.Detect(fixtureProject())

	if !matched {
		t.Fatal("generic{}.Detect() matched == false, want true")
	}
	if match.Evidence != "no framework signal matched" {
		t.Errorf("Evidence = %q, want %q", match.Evidence, "no framework signal matched")
	}
	// generic carries dharness's cross-cutting opinions, so its manifest is
	// no longer empty. What it must never carry is anything framework-shaped:
	// it matches every project, including ones it knows nothing about.
	for _, fact := range match.Manifest.Facts {
		if fact.Key == "ignorePatterns" {
			t.Errorf("generic contributes %q, which is a framework's answer and not a universal one", fact.Key)
		}
	}
	if len(match.Manifest.Seeds) != 0 {
		t.Errorf("Manifest.Seeds = %v, want empty — generic knows no framework to describe", match.Manifest.Seeds)
	}
}

func TestGenericScopeIsRoot(t *testing.T) {
	if (generic{}).Scope() != Root {
		t.Error("generic{}.Scope() != Root")
	}
}

func TestGenericManifestValidates(t *testing.T) {
	match, _ := generic{}.Detect(fixtureProject())
	if err := match.Manifest.Validate(); err != nil {
		t.Errorf("generic's manifest fails Validate(): %v", err)
	}
}

// generic carries dharness's cross-cutting opinions — the ones that hold for
// every project regardless of framework — and the duplication ceiling is the
// first of them.
//
// It goes through the manifest rather than into the owned file's skeleton for
// one reason: fallow's `extends` merges this object field by field, so a
// project declaring only `mode` silently keeps dharness's `minOccurrences`
// and `threshold` — the half-override survives rather than being discarded
// whole. Only a contributed key reaches boundariesOwnerStep's candidate set,
// and only a candidate gets reported instead of vanishing.
func TestGenericCarriesTheDuplicationCeiling(t *testing.T) {
	match, matched := generic{}.Detect(fixtureProject())
	if !matched {
		t.Fatal("generic{}.Detect() matched == false; generic always matches")
	}

	var found *Fact
	for i, fact := range match.Manifest.Facts {
		if fact.Key == "duplicates" {
			found = &match.Manifest.Facts[i]
		}
	}
	if found == nil {
		t.Fatalf("generic contributes no duplicates ceiling: %+v", match.Manifest.Facts)
	}

	value, ok := found.Value.(map[string]any)
	if !ok {
		t.Fatalf("duplicates value = %T, want an object carrying fallow's own keys", found.Value)
	}

	// Each of the three departs from a fallow default, verified against its
	// schema: mode is "mild", minOccurrences is 2, threshold is 0.0 (no
	// limit). A key that only restated a default would be noise to maintain
	// in every adopting repository — the reason the nine-rule pin this
	// originally shipped beside was dropped.
	for _, want := range []struct {
		key   string
		value any
	}{
		{"mode", "semantic"},
		{"minOccurrences", 3},
		{"threshold", 3},
	} {
		if value[want.key] != want.value {
			t.Errorf("%s = %v, want %v", want.key, value[want.key], want.value)
		}
	}

	// The evidence must say the two measurements are not the same one, or the
	// number reads as a ported gate rather than a companion signal.
	for _, expected := range []string{"semantic", "3", "different"} {
		if !strings.Contains(found.Because, expected) {
			t.Errorf("Because = %q, want it to carry %q", found.Because, expected)
		}
	}

	// Two limits, both of them ways the three values can be misread, so the
	// comment has to carry them next to the values it explains: the occurrence
	// floor decides which groups are reported and never the percentage the
	// ceiling measures, and a structural match is still only a shape match, so
	// a reported group is a candidate to read rather than a verdict.
	for _, expected := range []string{
		"which groups are reported",
		"not the percentage the ceiling measures",
		"a candidate to read rather than a verdict",
	} {
		if !strings.Contains(found.Because, expected) {
			t.Errorf("Because = %q, want it to carry %q", found.Because, expected)
		}
	}

	// The floor's own reason belongs in the comment as well, and as the
	// floor's intent for the report: fallow documents minOccurrences as the
	// minimum number of occurrences before a clone group is reported, so
	// "two is coincidence, three is a pattern" says which groups are worth
	// reading — it is not a claim about what the ceiling measures. It sits
	// next to the two limits above for that reason.
	const floorIntent = "two occurrences is usually coincidence where three is a pattern"
	if !strings.Contains(found.Because, floorIntent) {
		t.Errorf("Because = %q, want it to carry the floor's intent, %q", found.Because, floorIntent)
	}

	if err := match.Manifest.Validate(); err != nil {
		t.Errorf("generic's manifest fails Validate(): %v", err)
	}
}
