package setup

import (
	"fmt"
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
