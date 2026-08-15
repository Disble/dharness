package setup

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/jsconfig"
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
		{Package: "eslint-config-next", Binding: "dharnessNext", Spread: true, Because: "published by Next.js"},
	}

	got := ownedEslintConfig(p, layers, jsconfig.ESM)

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

	got := ownedEslintConfig(p, nil, jsconfig.ESM)

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

	got := ownedEslintConfig(p, nil, jsconfig.ESM)

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

	got := ownedEslintConfig(p, nil, jsconfig.ESM)

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

	got := eslintImportRegion(p, p.Source, nil, jsconfig.ESM, "", "\n")

	want := eslintImportBegin + "\n" +
		"import dharnessPlugin from \"dharness-eslint-plugin\";\n" +
		"import dharnessLayer from \"./.dharness/eslint.config.js\";\n" +
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

	got := eslintImportRegion(p, p.Source, layers, jsconfig.ESM, "", "\n")

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

	got := eslintImportRegion(p, p.Source, nil, jsconfig.ESM, "", "\n")

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

// TestLayerExpressionRendersEveryDocumentedForm pins the four shapes the
// frameworks' own documentation actually uses. v1's renderer could spell
// only the first of them, which is why three of these were unreachable
// before Layer gained Accessor and Spread.
func TestLayerExpressionRendersEveryDocumentedForm(t *testing.T) {
	cases := []struct {
		name  string
		layer preset.Layer
		want  string
	}{
		{
			// Next.js: `import nextVitals from "eslint-config-next/core-web-vitals"` then `...nextVitals`.
			name:  "array spread",
			layer: preset.Layer{Binding: "dharnessNext", Spread: true},
			want:  "...dharnessNext",
		},
		{
			// Expo: `const expoConfig = require("eslint-config-expo/flat")` then `expoConfig,` — one object.
			name:  "single object",
			layer: preset.Layer{Binding: "dharnessExpo"},
			want:  "dharnessExpo",
		},
		{
			// react-doctor: `reactDoctor.configs.next`.
			name:  "identifier accessor",
			layer: preset.Layer{Binding: "dharnessReactDoctor", Accessor: []string{"configs", "next"}},
			want:  `dharnessReactDoctor.configs.next`,
		},
		{
			// react-doctor: `reactDoctor.configs["react-native"]` — a hyphen is not an identifier.
			name:  "subscripted accessor",
			layer: preset.Layer{Binding: "dharnessReactDoctor", Accessor: []string{"configs", "react-native"}},
			want:  `dharnessReactDoctor.configs["react-native"]`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := layerExpression(c.layer, jsconfig.ESM); got != c.want {
				t.Errorf("layerExpression() = %q, want %q", got, c.want)
			}
		})
	}
}

// TestLayerExpressionSpreadsAnAccessedArray triangulates the two axes as
// independent: an accessor that resolves to a config array is both accessed
// and spread. typescript-eslint's recommendedTypeChecked is this shape.
func TestLayerExpressionSpreadsAnAccessedArray(t *testing.T) {
	layer := preset.Layer{Binding: "dharnessTs", Accessor: []string{"configs", "recommended"}, Spread: true}
	if got, want := layerExpression(layer, jsconfig.ESM), "...dharnessTs.configs.recommended"; got != want {
		t.Errorf("layerExpression() = %q, want %q", got, want)
	}
}

// TestOneModuleIsImportedOnceForSeveralLayers pins what makes the
// react-doctor mapping possible at all: every framework preset reads a
// different config off one module, so nextjs and expo both contribute a
// layer bound to dharnessReactDoctor. Emitting that import twice is a
// SyntaxError — an ES module cannot bind one identifier in two import
// declarations — so the region must dedupe it while keeping both
// expressions.
func TestOneModuleIsImportedOnceForSeveralLayers(t *testing.T) {
	p := project.At(t.TempDir(), t.TempDir())
	layers := []preset.Layer{
		{Package: "eslint-plugin-react-doctor", Binding: "dharnessReactDoctor", Accessor: []string{"configs", "next"}, Because: "documented"},
		{Package: "eslint-plugin-react-doctor", Binding: "dharnessReactDoctor", Accessor: []string{"configs", "react-native"}, Because: "documented"},
	}

	got := eslintImportRegion(p, t.TempDir(), layers, jsconfig.ESM, "", "\n")

	if n := strings.Count(got, `import dharnessReactDoctor from "eslint-plugin-react-doctor";`); n != 1 {
		t.Errorf("eslintImportRegion() emitted the import %d times, want exactly 1:\n%s", n, got)
	}
}

// TestOneModuleIsDestructuredOnceForSeveralLayers is the same rule over the
// factory's parameter list: a duplicated parameter name in a destructuring
// pattern is a SyntaxError in strict mode, which every ES module is.
func TestOneModuleIsDestructuredOnceForSeveralLayers(t *testing.T) {
	p := project.At(t.TempDir(), t.TempDir())
	layers := []preset.Layer{
		{Package: "eslint-plugin-react-doctor", Binding: "dharnessReactDoctor", Accessor: []string{"configs", "next"}, Because: "documented"},
		{Package: "eslint-plugin-react-doctor", Binding: "dharnessReactDoctor", Accessor: []string{"configs", "react-native"}, Because: "documented"},
	}

	got := ownedEslintConfig(p, layers, jsconfig.ESM)

	if !strings.Contains(got, "export default function dharnessLayer({ plugin, dharnessReactDoctor }) {") {
		t.Errorf("ownedEslintConfig() did not dedupe the repeated binding in its parameter list:\n%s", got)
	}
	for _, want := range []string{"dharnessReactDoctor.configs.next,", `dharnessReactDoctor.configs["react-native"],`} {
		if !strings.Contains(got, want) {
			t.Errorf("ownedEslintConfig() is missing %s — deduping the binding must not drop an expression:\n%s", want, got)
		}
	}
}

// TestLayerRegionPassesEachBindingOnce is the third place the same binding
// list is spelled: the call site. A repeated key in an object literal is
// legal JavaScript but passes the module twice for no reason, and the three
// renderings must agree or the factory receives something it did not
// destructure.
func TestLayerRegionPassesEachBindingOnce(t *testing.T) {
	layers := []preset.Layer{
		{Package: "eslint-plugin-react-doctor", Binding: "dharnessReactDoctor", Accessor: []string{"configs", "next"}, Because: "documented"},
		{Package: "eslint-plugin-react-doctor", Binding: "dharnessReactDoctor", Accessor: []string{"configs", "react-native"}, Because: "documented"},
	}

	got := eslintLayerRegion(layers, "", "\n")

	if !strings.Contains(got, "...dharnessLayer({ plugin: dharnessPlugin, dharnessReactDoctor }),") {
		t.Errorf("eslintLayerRegion() did not dedupe the repeated binding:\n%s", got)
	}
}

// TestOwnedImportSpecifierIsRelative pins what Node's resolver requires and
// a path does not carry on its own. ownedFrom returns filepath.Rel's answer,
// and for a project whose config sits beside .dharness/ that is
// ".dharness/eslint.config.js" — which Node reads as a *bare* specifier,
// because an ES module's relative specifier has to begin with "./" or
// "../". It resolves as a package named ".dharness", is not found, and
// ESLint fails to load the config at startup.
//
// fallow's and lefthook's own `extends` take the same path happily, which is
// why one renderer producing it for all three went unnoticed: only the
// JavaScript one is resolved by Node.
func TestOwnedImportSpecifierIsRelative(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root}

	got := eslintImportRegion(p, p.Source, nil, jsconfig.ESM, "", "\n")

	if !strings.Contains(got, `from "./.dharness/eslint.config.js";`) {
		t.Errorf("eslintImportRegion() does not emit a relative specifier Node can resolve:\n%s", got)
	}
}

// TestOwnedImportSpecifierKeepsAnAscendingPath triangulates the rule above:
// a split layout already begins with "../", which is a relative specifier,
// and prefixing it again would climb one directory too few.
func TestOwnedImportSpecifierKeepsAnAscendingPath(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: filepath.Join(root, "frontend")}

	got := eslintImportRegion(p, p.Source, nil, jsconfig.ESM, "", "\n")

	if !strings.Contains(got, `from "../.dharness/eslint.config.js";`) {
		t.Errorf("eslintImportRegion() rewrote an already-relative ascending path:\n%s", got)
	}
}

// TestImportRegionRendersCommonJSDeclarations pins the dialect half of flat
// config support: `npx expo lint` generates a CommonJS eslint.config.js, and
// an `import` statement spliced into one is a SyntaxError.
func TestImportRegionRendersCommonJSDeclarations(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root}
	layers := []preset.Layer{{Package: "eslint-config-expo/flat", Binding: "dharnessExpo", Because: "documented"}}

	got := eslintImportRegion(p, p.Source, layers, jsconfig.CommonJS, "", "\n")

	for _, want := range []string{
		`const dharnessPlugin = require("dharness-eslint-plugin");`,
		`const dharnessExpo = require("eslint-config-expo/flat");`,
		`const dharnessLayer = require("./.dharness/eslint.config.js");`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("eslintImportRegion() is missing %s:\n%s", want, got)
		}
	}
	// The marker text itself contains "eslint-import", so the check is for a
	// declaration rather than for the word.
	if strings.Contains(got, "\nimport ") {
		t.Errorf("eslintImportRegion() emitted an ESM import into a CommonJS config:\n%s", got)
	}
}

// TestOwnedConfigIsWrittenInTheProjectsOwnDialect pins the other side of the
// same requirement. The project's config loads .dharness/eslint.config.js by
// the mechanism its own dialect gives it, so the owned file has to export
// the way that mechanism reads: a CommonJS config calling require() on a
// file that says `export default` gets a function it cannot call.
func TestOwnedConfigIsWrittenInTheProjectsOwnDialect(t *testing.T) {
	p := project.At(t.TempDir(), t.TempDir())

	esm := ownedEslintConfig(p, nil, jsconfig.ESM)
	if !strings.Contains(esm, "export default function dharnessLayer({ plugin }) {") {
		t.Errorf("ownedEslintConfig() ESM form = %q", esm)
	}

	cjs := ownedEslintConfig(p, nil, jsconfig.CommonJS)
	if !strings.Contains(cjs, "module.exports = function dharnessLayer({ plugin }) {") {
		t.Errorf("ownedEslintConfig() CommonJS form = %q", cjs)
	}
	if strings.Contains(cjs, "export default") {
		t.Errorf("ownedEslintConfig() CommonJS form still carries an ESM export:\n%s", cjs)
	}
}

// TestOneConfigIsIncludedOnce pins the deduplication the shared
// recommended layer forces. Both framework presets contribute react-doctor's
// framework-independent rules, so a repository matching two of them offers
// the identical expression twice, and rendering it twice reads as a bug in a
// file people open.
//
// First occurrence wins, which is the only ordering that is safe to assert:
// dropping a later copy can only matter if something between the two turned
// a rule off that the copy would turn back on, and dharness never needs the
// same config object to do that.
func TestOneConfigIsIncludedOnce(t *testing.T) {
	p := project.At(t.TempDir(), t.TempDir())
	recommended := preset.Layer{Package: "eslint-plugin-react-doctor", Binding: "dharnessReactDoctor", Accessor: []string{"configs", "recommended"}, Because: "documented"}
	layers := []preset.Layer{
		recommended,
		{Package: "eslint-plugin-react-doctor", Binding: "dharnessReactDoctor", Accessor: []string{"configs", "next"}, Because: "documented"},
		recommended,
		{Package: "eslint-plugin-react-doctor", Binding: "dharnessReactDoctor", Accessor: []string{"configs", "react-native"}, Because: "documented"},
	}

	got := ownedEslintConfig(p, layers, jsconfig.ESM)

	if n := strings.Count(got, "dharnessReactDoctor.configs.recommended,"); n != 1 {
		t.Errorf("ownedEslintConfig() included the shared config %d times, want 1:\n%s", n, got)
	}
	for _, want := range []string{"dharnessReactDoctor.configs.next,", `dharnessReactDoctor.configs["react-native"],`} {
		if !strings.Contains(got, want) {
			t.Errorf("ownedEslintConfig() dropped %s while deduping:\n%s", want, got)
		}
	}
}

// TestDedupeKeepsTheFirstOccurrence triangulates the rule above on position:
// the surviving copy is the earlier one, so every later config still layers
// on top of it.
func TestDedupeKeepsTheFirstOccurrence(t *testing.T) {
	p := project.At(t.TempDir(), t.TempDir())
	shared := preset.Layer{Package: "x", Binding: "dharnessX", Because: "documented"}
	layers := []preset.Layer{shared, {Package: "y", Binding: "dharnessY", Because: "documented"}, shared}

	got := ownedEslintConfig(p, layers, jsconfig.ESM)

	if strings.Index(got, "dharnessX,") > strings.Index(got, "dharnessY,") {
		t.Errorf("ownedEslintConfig() kept the later copy, not the first:\n%s", got)
	}
}

// TestCommonJSUnwrapsAnESModuleNamespace pins the interop the Expo sandbox
// found, which no Go test could have: `require()` of an ES module returns
// the module *namespace*, not its default export, so
// `require("eslint-plugin-react-doctor").configs` is undefined and ESLint
// dies with "Cannot read properties of undefined". The same package reached
// through `import` is already the default export.
//
// Measured against eslint-plugin-react-doctor 0.9.12 and
// eslint-config-expo/flat 57.0.1: the first comes back with
// Symbol.toStringTag === "Module" and its configs under .default, the second
// is a plain CommonJS object with no tag at all. Unwrapping only a tagged
// namespace is what keeps both correct.
func TestCommonJSUnwrapsAnESModuleNamespace(t *testing.T) {
	p := project.At(t.TempDir(), t.TempDir())
	layers := []preset.Layer{
		{Package: "eslint-config-expo/flat", Binding: "dharnessExpo", Because: "documented"},
		{Package: "eslint-plugin-react-doctor", Binding: "dharnessReactDoctor", Accessor: []string{"configs", "recommended"}, Because: "documented"},
	}

	got := ownedEslintConfig(p, layers, jsconfig.CommonJS)

	if !strings.Contains(got, "const dharnessDefault = (m) =>") {
		t.Errorf("ownedEslintConfig() CommonJS form carries no interop helper:\n%s", got)
	}
	for _, want := range []string{
		"dharnessDefault(dharnessExpo),",
		"dharnessDefault(dharnessReactDoctor).configs.recommended,",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ownedEslintConfig() is missing %s:\n%s", want, got)
		}
	}
}

// TestESMDoesNotCarryTheInteropHelper triangulates the case above: a default
// import is never a module namespace, so the helper can never fire there and
// emitting it would be noise in the file every ESM project opens.
func TestESMDoesNotCarryTheInteropHelper(t *testing.T) {
	p := project.At(t.TempDir(), t.TempDir())
	layers := []preset.Layer{{Package: "eslint-plugin-react-doctor", Binding: "dharnessReactDoctor", Accessor: []string{"configs", "next"}, Because: "documented"}}

	got := ownedEslintConfig(p, layers, jsconfig.ESM)

	if strings.Contains(got, "dharnessDefault") {
		t.Errorf("ownedEslintConfig() ESM form carries the CommonJS interop helper:\n%s", got)
	}
	if !strings.Contains(got, "dharnessReactDoctor.configs.next,") {
		t.Errorf("ownedEslintConfig() ESM form does not read the config directly:\n%s", got)
	}
}
