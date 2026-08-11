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
// one reason: fallow's `extends` replaces a key rather than merging it, so a
// project declaring its own `duplicates` silently discards dharness's. Only a
// contributed key reaches boundariesOwnerStep's candidate set, and only a
// candidate gets reported instead of vanishing.
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
		t.Fatalf("duplicates value = %T, want an object carrying fallow's own threshold key", found.Value)
	}
	if value["threshold"] != 3 {
		t.Errorf("threshold = %v, want 3", value["threshold"])
	}

	// The evidence must say the two measurements are not the same one, or the
	// number reads as a ported gate rather than a companion signal.
	for _, expected := range []string{"3", "different"} {
		if !strings.Contains(found.Because, expected) {
			t.Errorf("Because = %q, want it to carry %q", found.Because, expected)
		}
	}
	if err := match.Manifest.Validate(); err != nil {
		t.Errorf("generic's manifest fails Validate(): %v", err)
	}
}
