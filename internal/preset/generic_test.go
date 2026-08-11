package preset

import "testing"

func TestGenericAlwaysMatches(t *testing.T) {
	match, matched := generic{}.Detect(fixtureProject())

	if !matched {
		t.Fatal("generic{}.Detect() matched == false, want true")
	}
	if match.Evidence != "no framework signal matched" {
		t.Errorf("Evidence = %q, want %q", match.Evidence, "no framework signal matched")
	}
	if len(match.Manifest.Facts) != 0 {
		t.Errorf("Manifest.Facts = %v, want empty — generic contributes nothing", match.Manifest.Facts)
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
