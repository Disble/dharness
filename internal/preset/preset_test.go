package preset

import (
	"path/filepath"
	"testing"

	"github.com/Disble/dharness/internal/project"
)

// fixtureProject is a project.Project with no framework signal, useful
// anywhere a test needs one without caring about its shape.
func fixtureProject() project.Project {
	return project.At("", "")
}

// TestMatchCarriesUncertain pins the field the orchestrator's design review
// added to Match: a preset that recognises its signal but cannot read its
// configuration still matches, and names what it could not read here rather
// than answering false and reading as "not this framework".
func TestMatchCarriesUncertain(t *testing.T) {
	m := Match{Uncertain: "wails.json exists but does not parse"}
	if m.Uncertain != "wails.json exists but does not parse" {
		t.Errorf("Match.Uncertain = %q, want it to round-trip", m.Uncertain)
	}
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

// TestEverySeedCarriesEvidence is Seed's own form of TestEveryFactCarriesEvidence
// — the same validity rule, over the other half of Manifest.
func TestEverySeedCarriesEvidence(t *testing.T) {
	m := Manifest{Schema: Schema, Seeds: []Seed{{Text: "a structural fact", Because: ""}}}
	if err := m.Validate(); err == nil {
		t.Error("Validate() = nil for a seed with no evidence, want an error")
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

// TestWailsRootWithNextjsSourceContributesFromBoth is task 18.1, the real
// scenario the whole composition rule exists for: a repository root that is
// a Wails project, whose JS half depends on Next.js. Resolve must return
// both matches through the real registry, not stubs — this is what proves
// wails and nextjs are actually registered together, not merely compilable
// together.
func TestWailsRootWithNextjsSourceContributesFromBoth(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "frontend")
	writeWailsFixtureFile(t, root, "wails.json", "{}\n")
	writeWailsFixtureFile(t, source, "package.json", `{"dependencies":{"next":"^14.0.0"}}`)

	p := project.At(root, source)
	matches := Resolve(p)

	if !containsMatchID(matches, "wails") {
		t.Fatalf("Resolve() = %v, want a wails match", matches)
	}
	if !containsMatchID(matches, "nextjs") {
		t.Fatalf("Resolve() = %v, want a nextjs match", matches)
	}
	if containsMatchID(matches, "expo") {
		t.Fatalf("Resolve() = %v, want no expo match — nothing in the fixture declares it", matches)
	}
	// generic always matches too — a real signal short-circuits nothing
	// (resolve's own documented rule) — and it contributes dharness's
	// cross-cutting duplication ceiling, so it is a third contributor here
	// rather than an inert one.

	keys := Keys(matches)
	if !containsKey(keys, "ignorePatterns") {
		t.Errorf("Keys() = %v, want wails' ignorePatterns in the union", keys)
	}
	if !containsKey(keys, "duplicates") {
		t.Errorf("Keys() = %v, want generic's cross-cutting duplicates ceiling in the union", keys)
	}
	if seeds := Seeds(matches); len(seeds) == 0 {
		t.Error("Seeds() is empty, want nextjs's documented structure to reach the union")
	}
}

// TestRealRegistryFactsAndSeedsValidate is task 19.1's registry-wide form:
// every preset that can match a representative fixture must produce a
// manifest that passes Validate() — not stub matches, the real four.
func TestRealRegistryFactsAndSeedsValidate(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "frontend")
	writeWailsFixtureFile(t, root, "wails.json", `{"wailsjsdir": "./frontend/src/lib"}`)
	writeWailsFixtureFile(t, source, "package.json", `{"dependencies":{"next":"^14.0.0","expo":"~51.0.0"}}`)

	matches := Resolve(project.At(root, source))
	if len(matches) == 0 {
		t.Fatal("Resolve() returned no matches to validate")
	}
	for _, match := range matches {
		if err := match.Manifest.Validate(); err != nil {
			t.Errorf("preset %q's manifest fails Validate(): %v", match.ID, err)
		}
	}
}

// TestLayersEnumeratesAcrossMatchesInOrder pins Layers' contract: it unions
// every match's Layer contributions in the same Root-then-Source, registry
// order Resolve already returns, so the generated import block and factory
// signature are byte-stable across runs (design decision 7).
func TestLayersEnumeratesAcrossMatchesInOrder(t *testing.T) {
	rootMatch := Match{ID: "root-preset", Scope: Root, Manifest: Manifest{
		Schema: Schema,
		Layers: []Layer{{Package: "eslint-config-root", Binding: "dharnessRoot", Because: "root.json declares it"}},
	}}
	sourceMatch := Match{ID: "source-preset", Scope: Source, Manifest: Manifest{
		Schema: Schema,
		Layers: []Layer{{Package: "eslint-config-source", Binding: "dharnessSource", Because: "package.json dependency"}},
	}}

	got := Layers([]Match{rootMatch, sourceMatch})
	if len(got) != 2 || got[0].Binding != "dharnessRoot" || got[1].Binding != "dharnessSource" {
		t.Errorf("Layers() = %+v, want root's layer before source's, in match order", got)
	}
}

// TestLayersIsEmptyWhenNoMatchContributesOne triangulates the union case
// above: a manifest with no Layers must not contribute a zero-value entry.
func TestLayersIsEmptyWhenNoMatchContributesOne(t *testing.T) {
	m := Match{ID: "generic", Scope: Root, Manifest: Manifest{Schema: Schema}}
	if got := Layers([]Match{m}); len(got) != 0 {
		t.Errorf("Layers() = %+v, want empty for a manifest with no layers", got)
	}
}

func containsKey(keys []string, want string) bool {
	for _, key := range keys {
		if key == want {
			return true
		}
	}
	return false
}
