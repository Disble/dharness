package setup

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/preset"
	"github.com/Disble/dharness/internal/project"
)

// TestOwnedEslintConfigParamListMatchesBindingsByteForByte pins the
// structural property design decision 3 states: the factory's destructuring
// and its returned array are built from the same layers slice, so a
// binding contributed by a preset shows up as both a parameter and a
// spread, spelled identically.
func TestOwnedEslintConfigParamListMatchesBindingsByteForByte(t *testing.T) {
	p := project.At(t.TempDir(), t.TempDir())
	layers := []preset.Layer{
		{Package: "eslint-config-next", Binding: "dharnessNext", Because: "published by Next.js"},
	}

	got := ownedEslintConfig(p, layers)

	if !strings.Contains(got, "export default function dharnessLayer({ plugin, dharnessNext }) {") {
		t.Errorf("ownedEslintConfig() parameter list does not match the binding list byte-for-byte:\n%s", got)
	}
	if !strings.Contains(got, "...dharnessNext,") {
		t.Errorf("ownedEslintConfig() does not spread the contributed layer:\n%s", got)
	}
}

// TestOwnedEslintConfigWithNoLayersHasOnlyThePluginParam triangulates the
// case above with a different input: real contributions land in slice 5, so
// preset.Layers(preset.Resolve(p)) returns empty today, and the factory must
// still be valid JavaScript with no dangling comma or empty destructure.
func TestOwnedEslintConfigWithNoLayersHasOnlyThePluginParam(t *testing.T) {
	p := project.At(t.TempDir(), t.TempDir())

	got := ownedEslintConfig(p, nil)

	if !strings.Contains(got, "export default function dharnessLayer({ plugin }) {") {
		t.Errorf("ownedEslintConfig() = %q, want the plugin-only parameter list with no layers", got)
	}
	if strings.Contains(got, "...") {
		t.Errorf("ownedEslintConfig() spreads something with no layers contributed:\n%s", got)
	}
}

// TestOwnedEslintConfigRendersEverySeverityFromDefaultSeverity proves the
// six rules are rendered from the same DefaultSeverity function
// doctorConfigStep used, not a second, divergent copy of the values.
func TestOwnedEslintConfigRendersEverySeverityFromDefaultSeverity(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(project.SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		return []byte("components/index.ts"), nil
	}))
	p := project.Project{Root: root, Source: root, InRepository: true}

	got := ownedEslintConfig(p, nil)

	for _, id := range RuleIDs() {
		want := fmt.Sprintf("%q: %q,", id, DefaultSeverity(p, id))
		if !strings.Contains(got, want) {
			t.Errorf("ownedEslintConfig() is missing %s, want it rendered from DefaultSeverity:\n%s", want, got)
		}
	}
}

// TestOwnedEslintConfigNamesTheDharnessPluginBinding pins the fixed,
// unnamespaced "plugin" parameter design decision 3 gives the rules
// plugin itself — every project gets it, unconditionally, so it carries no
// binding name of its own the way a preset-contributed layer does.
func TestOwnedEslintConfigNamesTheDharnessPluginBinding(t *testing.T) {
	p := project.At(t.TempDir(), t.TempDir())

	got := ownedEslintConfig(p, nil)

	if !strings.Contains(got, "plugins: { dharness: plugin }") {
		t.Errorf("ownedEslintConfig() = %q, want the plugin bound under the dharness key", got)
	}
}

// TestEslintImportRegionRendersThePluginAndTheOwnedFactoryImport pins the
// no-layers case: the marker pair, the plugin's own import and the owned
// factory's import, computed via ownedFrom the same way fallow.jsonc's
// reference is (files.go's stated reason for both to exist).
func TestEslintImportRegionRendersThePluginAndTheOwnedFactoryImport(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root}

	got := eslintImportRegion(p, p.Source, nil, "", "\n")

	want := eslintImportBegin + "\n" +
		"import dharnessPlugin from \"dharness-eslint-plugin\";\n" +
		"import dharnessLayer from \".dharness/eslint.config.js\";\n" +
		eslintImportEnd + "\n"
	if got != want {
		t.Errorf("eslintImportRegion() = %q, want %q", got, want)
	}
}

// TestEslintImportRegionRendersOneImportPerLayer triangulates the case
// above with a contributed layer: the binding is both imported and later
// passed to the factory call, spelled identically (design decision 3).
func TestEslintImportRegionRendersOneImportPerLayer(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root}
	layers := []preset.Layer{{Package: "eslint-config-next", Binding: "dharnessNext", Because: "published by Next.js"}}

	got := eslintImportRegion(p, p.Source, layers, "", "\n")

	if !strings.Contains(got, "import dharnessNext from \"eslint-config-next\";\n") {
		t.Errorf("eslintImportRegion() does not import the contributed layer:\n%s", got)
	}
}

// TestEslintImportRegionResolvesFromASplitLayout pins the split-layout
// shape ownedFrom already gives .fallowrc.json: the reference climbs out of
// Source to reach Root/.dharness/, exactly the same relative path a project
// config would need to resolve the owned factory.
func TestEslintImportRegionResolvesFromASplitLayout(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "frontend")
	p := project.Project{Root: root, Source: source}

	got := eslintImportRegion(p, p.Source, nil, "", "\n")

	if !strings.Contains(got, "import dharnessLayer from \"../.dharness/eslint.config.js\";\n") {
		t.Errorf("eslintImportRegion() does not resolve from the split layout:\n%s", got)
	}
}

// TestEslintLayerRegionCallsTheFactoryWithEveryBinding pins the spread
// element's own shape: the marker pair, the plugin passed under its fixed
// "plugin" parameter name, and every contributed layer passed under its own
// binding — shorthand, since the local name already matches.
func TestEslintLayerRegionCallsTheFactoryWithEveryBinding(t *testing.T) {
	layers := []preset.Layer{{Package: "eslint-config-next", Binding: "dharnessNext", Because: "published by Next.js"}}

	got := eslintLayerRegion(layers, "  ", "\n")

	want := "  " + eslintLayerBegin + "\n" +
		"  ...dharnessLayer({ plugin: dharnessPlugin, dharnessNext }),\n" +
		"  " + eslintLayerEnd + "\n"
	if got != want {
		t.Errorf("eslintLayerRegion() = %q, want %q", got, want)
	}
}

// TestEslintLayerRegionWithNoLayersCallsWithOnlyThePlugin triangulates the
// case above with no contributed layers — the shape generic and Wails
// projects render today.
func TestEslintLayerRegionWithNoLayersCallsWithOnlyThePlugin(t *testing.T) {
	got := eslintLayerRegion(nil, "  ", "\n")

	if !strings.Contains(got, "...dharnessLayer({ plugin: dharnessPlugin }),\n") {
		t.Errorf("eslintLayerRegion() = %q, want the plugin-only call with no layers", got)
	}
}
