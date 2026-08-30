package preset

import (
	"path/filepath"
	"strings"
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

// TestSchemaConstant pins the manifest shape presets contribute. v2 is the
// first bump: Layer gained Accessor and Spread, because the forms the
// frameworks actually document are not all "default export, spread" —
// eslint-plugin-react-doctor's presets are read off `configs`, and each one
// is a single config object rather than an array to spread.
func TestSchemaConstant(t *testing.T) {
	if Schema != "dharness.preset/v2" {
		t.Errorf("Schema = %q, want %q", Schema, "dharness.preset/v2")
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

// TestLayerValidateRejectsAPinnedVersion pins the first of Decision 7's two
// new rules: an "@" anywhere after position 0 pins a version, which the spec
// forbids — dharness installs what the framework itself publishes and
// versions. "@scope/name" keeps its own leading "@" legal.
func TestLayerValidateRejectsAPinnedVersion(t *testing.T) {
	m := Manifest{Schema: Schema, Layers: []Layer{{Package: "eslint-config-next@14.0.0", Binding: "dharnessNext", Because: "test"}}}
	if err := m.Validate(); err == nil {
		t.Error("Validate() = nil for a pinned package version, want an error")
	}
}

// TestScopedPackageWithNoVersionValidates triangulates the rule above: the
// leading "@" of a scoped package name is not itself a pin.
func TestScopedPackageWithNoVersionValidates(t *testing.T) {
	m := Manifest{Schema: Schema, Layers: []Layer{{Package: "@org/eslint-config", Binding: "dharnessOrg", Because: "test"}}}
	if err := m.Validate(); err != nil {
		t.Errorf("Validate() = %v for a scoped, unpinned package, want nil", err)
	}
}

// TestSingleCharacterPackageNamePinIsCaught pins the exact boundary
// "no @ after position 0" draws: a one-character package name pinned
// immediately at position 1 ("x@1.2.3") must still be caught. A check that
// starts scanning one byte too late (position 2 instead of 1) would miss
// exactly this case while still catching every longer package name.
func TestSingleCharacterPackageNamePinIsCaught(t *testing.T) {
	m := Manifest{Schema: Schema, Layers: []Layer{{Package: "x@1.2.3", Binding: "dharnessX", Because: "test"}}}
	if err := m.Validate(); err == nil {
		t.Error("Validate() = nil for a single-character package name pinned at position 1, want an error")
	}
}

// TestLayerValidateRejectsAnInvalidBinding pins Decision 7's second rule: a
// binding that is not a valid JavaScript identifier would produce a config
// that does not parse — an authoring bug caught at build time, exactly as
// json.Marshal on Fact.Value already is.
func TestLayerValidateRejectsAnInvalidBinding(t *testing.T) {
	m := Manifest{Schema: Schema, Layers: []Layer{{Package: "eslint-config-next", Binding: "dharness-next", Because: "test"}}}
	if err := m.Validate(); err == nil {
		t.Error("Validate() = nil for a binding that is not a valid identifier, want an error")
	}
}

// TestBareBindingIsRejectedAtBuildTime is spec.md's own scenario title: a
// registry entry whose binding is the bare package name — a valid
// identifier, but not namespaced to dharness — fails Validate() rather than
// reaching a repository, where it would collide with a project's own import
// of the same package under the same name (a SyntaxError).
func TestBareBindingIsRejectedAtBuildTime(t *testing.T) {
	m := Manifest{Schema: Schema, Layers: []Layer{{Package: "eslint-config-next", Binding: "next", Because: "test"}}}
	if err := m.Validate(); err == nil {
		t.Error("Validate() = nil for a bare, non-namespaced binding, want an error")
	}
}

// TestLayerValidateRequiresEvidence is Layer's own form of
// TestEveryFactCarriesEvidence — the same evidence-required rule (§17),
// over the third contribution kind.
func TestLayerValidateRequiresEvidence(t *testing.T) {
	m := Manifest{Schema: Schema, Layers: []Layer{{Package: "eslint-config-next", Binding: "dharnessNext", Because: ""}}}
	if err := m.Validate(); err == nil {
		t.Error("Validate() = nil for a layer with no evidence, want an error")
	}
}

// TestNoBindingNamesTwoPackages is TestNoScalarKeyIsContributedTwice's
// (internal/setup/owned_test.go) counterpart for Layer.Binding: no rule picks
// a winner when two matches bind one identifier to two different packages,
// and the generated import region would emit two import declarations under
// one identifier — a SyntaxError.
//
// A binding repeating for the *same* package is a different case and is
// legal: eslint-plugin-react-doctor is one module every framework preset
// reads a different preset off, so nextjs and expo both name
// dharnessReactDoctor. The import region dedupes it to one declaration
// (eslintImportRegion) while each layer keeps its own expression. The rule
// this test pins is therefore Binding -> Package being a function, not
// Binding being unique.
func TestNoBindingNamesTwoPackages(t *testing.T) {
	root := t.TempDir()
	writeWailsFixtureFile(t, root, "package.json", `{"dependencies":{"next":"^14.0.0","expo":"~51.0.0"}}`)

	matches := Resolve(project.At(root, root))
	layers := Layers(matches)
	if len(layers) == 0 {
		t.Fatal("Layers() returned nothing to check")
	}

	seen := map[string]string{}
	for _, layer := range layers {
		if pkg, ok := seen[layer.Binding]; ok && pkg != layer.Package {
			t.Fatalf("Layers() bound %q to both %q and %q — two import declarations under one identifier is a SyntaxError", layer.Binding, pkg, layer.Package)
		}
		seen[layer.Binding] = layer.Package
	}
}

// TestLayerValidateRejectsAnEmptyAccessorSegment pins the one authoring bug
// an accessor can carry that the renderer cannot recover from: a segment
// that names no property renders `x.` or `x[""]`, neither of which reads the
// preset the layer meant to name.
func TestLayerValidateRejectsAnEmptyAccessorSegment(t *testing.T) {
	m := Manifest{Schema: Schema, Layers: []Layer{{
		Package:  "eslint-plugin-react-doctor",
		Binding:  "dharnessReactDoctor",
		Accessor: []string{"configs", ""},
		Because:  "test",
	}}}
	if err := m.Validate(); err == nil {
		t.Error("Validate() = nil for an accessor with an empty segment, want an error")
	}
}

// TestLayerAccessorWithNonIdentifierSegmentValidates triangulates the rule
// above. "react-native" is not a JavaScript identifier and must still be
// accepted: the renderer subscripts it (configs["react-native"]) rather than
// dotting it, which is exactly the spelling react-doctor's own documentation
// uses. Rejecting it would refuse the preset Expo projects need.
func TestLayerAccessorWithNonIdentifierSegmentValidates(t *testing.T) {
	m := Manifest{Schema: Schema, Layers: []Layer{{
		Package:  "eslint-plugin-react-doctor",
		Binding:  "dharnessReactDoctor",
		Accessor: []string{"configs", "react-native"},
		Because:  "test",
	}}}
	if err := m.Validate(); err != nil {
		t.Errorf("Validate() = %v for a hyphenated accessor segment, want nil", err)
	}
}

// TestInstallNameStripsTheSubpath pins the split every framework flat config
// forced: ESLint imports "eslint-config-next/core-web-vitals" and npm
// installs "eslint-config-next". Installing the specifier verbatim asks the
// registry for a package that does not exist.
func TestInstallNameStripsTheSubpath(t *testing.T) {
	cases := []struct {
		name, pkg, want string
	}{
		{"unscoped subpath", "eslint-config-next/core-web-vitals", "eslint-config-next"},
		{"unscoped deep subpath", "eslint-config-expo/flat/extra", "eslint-config-expo"},
		{"unscoped bare", "eslint-plugin-react-doctor", "eslint-plugin-react-doctor"},
		{"scoped subpath", "@org/eslint-config/flat", "@org/eslint-config"},
		{"scoped bare", "@org/eslint-config", "@org/eslint-config"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := (Layer{Package: c.pkg}).InstallName(); got != c.want {
				t.Errorf("Layer{Package: %q}.InstallName() = %q, want %q", c.pkg, got, c.want)
			}
		})
	}
}

// TestInstallNameOfALoneScopeIsUnchanged pins the boundary the scoped branch
// draws. "@org" alone is not a package name npm would accept, but splitting
// it into a two-segment name is not something this function can invent
// either — it returns what it was given so the malformed name reaches the
// registry error rather than being silently reshaped into a different one.
func TestInstallNameOfALoneScopeIsUnchanged(t *testing.T) {
	if got := (Layer{Package: "@org"}).InstallName(); got != "@org" {
		t.Errorf("InstallName() = %q, want %q unchanged", got, "@org")
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

// wantLayer is the shape assertLayerContribution compares against: every
// field that reaches generated JavaScript. Because is checked for being
// non-empty rather than for its text, which is prose that will be re-quoted
// whenever the upstream page is re-read.
type wantLayer struct {
	pkg       string
	binding   string
	accessor  []string
	spread    bool
	registers string
}

// assertLayerContribution pins the whole Layer contract for one framework
// preset: the exact ordered list of layers, each one's package, binding,
// accessor and spread, a non-empty Because on all of them, and a manifest
// that validates.
//
// The framework presets differ by a dependency and by their layer list and
// by nothing else, so writing the assertions out per preset meant a rule
// tightened in one could silently not apply to the other. Order is asserted
// because flat config resolves rules last-wins: the sequence is a decision,
// not a listing.
func assertLayerContribution(t *testing.T, detect func(project.Project) (Match, bool), dependency string, want []wantLayer) {
	t.Helper()

	root := t.TempDir()
	writeWailsFixtureFile(t, root, "package.json", dependency)

	match, _ := detect(project.At(root, root))
	if len(match.Manifest.Layers) != len(want) {
		t.Fatalf("Manifest.Layers = %+v, want %d layers", match.Manifest.Layers, len(want))
	}

	for i, w := range want {
		layer := match.Manifest.Layers[i]
		if layer.Package != w.pkg {
			t.Errorf("Layers[%d].Package = %q, want %q", i, layer.Package, w.pkg)
		}
		if layer.Binding != w.binding {
			t.Errorf("Layers[%d].Binding = %q, want the namespaced %q", i, layer.Binding, w.binding)
		}
		if strings.Join(layer.Accessor, ".") != strings.Join(w.accessor, ".") {
			t.Errorf("Layers[%d].Accessor = %q, want %q", i, layer.Accessor, w.accessor)
		}
		if layer.Spread != w.spread {
			t.Errorf("Layers[%d].Spread = %v, want %v", i, layer.Spread, w.spread)
		}
		if layer.Registers != w.registers {
			t.Errorf("Layers[%d].Registers = %q, want %q", i, layer.Registers, w.registers)
		}
		if layer.Because == "" {
			t.Errorf("Layers[%d].Because is empty, want a checkable observable", i)
		}
	}

	if err := match.Manifest.Validate(); err != nil {
		t.Errorf("manifest fails Validate(): %v", err)
	}
}
