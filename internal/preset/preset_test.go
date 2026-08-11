package preset

import (
	"testing"

	"github.com/Disble/dharness/internal/project"
)

// fixtureProject is a project.Project with no framework signal, useful
// anywhere a test needs one without caring about its shape.
func fixtureProject() project.Project {
	return project.At("", "")
}

func TestSchemaConstant(t *testing.T) {
	if Schema != "dharness.preset/v1" {
		t.Errorf("Schema = %q, want %q", Schema, "dharness.preset/v1")
	}
}

func TestEveryFactCarriesEvidence(t *testing.T) {
	m := Manifest{Schema: Schema, Facts: []Fact{{Key: "ignorePatterns", Value: []string{"wailsjs/**"}, Because: ""}}}
	if err := m.Validate(); err == nil {
		t.Error("Validate() = nil for a fact with no evidence, want an error")
	}
}

func TestNoPresetContributesBoundaries(t *testing.T) {
	m := Manifest{Schema: Schema, Facts: []Fact{{Key: "boundaries", Value: "x", Because: "the observable"}}}
	if err := m.Validate(); err == nil {
		t.Error("Validate() = nil for a manifest contributing \"boundaries\", want an error")
	}
}

// stubPreset lets the composition rule (registry order, scope, Source guard)
// be tested without depending on the real presets — the same reason
// applySteps is split out of Apply.
type stubPreset struct {
	id      string
	scope   Scope
	matches bool
	match   Match
}

func (s stubPreset) ID() string   { return s.id }
func (s stubPreset) Scope() Scope { return s.scope }
func (s stubPreset) Detect(project.Project) (Match, bool) {
	return s.match, s.matches
}

func TestResolveAlwaysReturnsAtLeastGeneric(t *testing.T) {
	p := project.At(t.TempDir(), "")
	got := resolve(p, []Preset{generic{}})
	if len(got) == 0 {
		t.Fatal("resolve() = empty, want at least generic")
	}
}

func TestSourceScopePresetsAreSkippedWithoutASource(t *testing.T) {
	p := project.At(t.TempDir(), "")
	stub := stubPreset{id: "always-matches", scope: Source, matches: true, match: Match{ID: "always-matches", Scope: Source}}

	got := resolve(p, []Preset{stub})
	for _, m := range got {
		if m.ID == "always-matches" {
			t.Fatal("resolve() returned a Source-scope match for a project with no Source")
		}
	}
}

// TestUnmatchedSourceGuardDoesNotStopLaterPresets pins the guard as a
// per-preset "skip this one" rather than a loop-ending condition: an earlier
// Source-scope preset with no Source to match against must not prevent a
// later Root-scope preset from being evaluated.
func TestUnmatchedSourceGuardDoesNotStopLaterPresets(t *testing.T) {
	p := project.At(t.TempDir(), "")
	sourceOnly := stubPreset{id: "source-only", scope: Source, matches: true, match: Match{ID: "source-only", Scope: Source}}
	rootPreset := stubPreset{id: "root-preset", scope: Root, matches: true, match: Match{ID: "root-preset", Scope: Root}}

	got := resolve(p, []Preset{sourceOnly, rootPreset})
	if !containsMatchID(got, "root-preset") {
		t.Fatalf("resolve() = %v, want root-preset present even though an earlier preset was skipped", got)
	}
}

// TestNonMatchingPresetDoesNotStopLaterPresets is the same pin for the other
// per-preset skip: a preset whose Detect returns matched == false must not
// stop a later preset that does match from being evaluated.
func TestNonMatchingPresetDoesNotStopLaterPresets(t *testing.T) {
	p := fixtureProject()
	noMatch := stubPreset{id: "no-match", scope: Root, matches: false}
	matchStub := stubPreset{id: "matches", scope: Root, matches: true, match: Match{ID: "matches", Scope: Root}}

	got := resolve(p, []Preset{noMatch, matchStub})
	if !containsMatchID(got, "matches") {
		t.Fatalf("resolve() = %v, want matches present even though an earlier preset did not match", got)
	}
}

func containsMatchID(matches []Match, id string) bool {
	for _, m := range matches {
		if m.ID == id {
			return true
		}
	}
	return false
}

// TestManifestValidateRejectsAnUnencodableValue pins the branch json.Marshal
// exists to catch: a fact whose Value cannot be encoded fails at build time
// (a registry test), never at write time.
func TestManifestValidateRejectsAnUnencodableValue(t *testing.T) {
	m := Manifest{Schema: Schema, Facts: []Fact{{Key: "k", Value: make(chan int), Because: "test"}}}
	if err := m.Validate(); err == nil {
		t.Error("Validate() = nil for an unencodable value, want an error")
	}
}

func TestResolveOrdersRootBeforeSource(t *testing.T) {
	root := t.TempDir()
	p := project.At(root, root)
	source := stubPreset{id: "source-preset", scope: Source, matches: true, match: Match{ID: "source-preset", Scope: Source}}
	rootPreset := stubPreset{id: "root-preset", scope: Root, matches: true, match: Match{ID: "root-preset", Scope: Root}}

	// Registered Source-first, to prove the ordering comes from Resolve, not
	// from registration order.
	got := resolve(p, []Preset{source, rootPreset})
	if len(got) != 2 || got[0].ID != "root-preset" || got[1].ID != "source-preset" {
		t.Fatalf("resolve() = %v, want root-preset before source-preset", got)
	}
}
