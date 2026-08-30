package setup

import (
	"fmt"
	"io"
	"os"
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
		"import dharnessLayer from \"./.dharness/eslint.config.mjs\";\n" +
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

	if !strings.Contains(got, "import dharnessLayer from \"../.dharness/eslint.config.mjs\";\n") {
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
			// react-doctor: a preset read off `configs` is one config object,
			// included as-is rather than spread.
			name:  "single object",
			layer: preset.Layer{Binding: "dharnessReactDoctor"},
			want:  "dharnessReactDoctor",
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
// ".dharness/eslint.config.mjs" — which Node reads as a *bare* specifier,
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

	if !strings.Contains(got, `from "./.dharness/eslint.config.mjs";`) {
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

	if !strings.Contains(got, `from "../.dharness/eslint.config.mjs";`) {
		t.Errorf("eslintImportRegion() rewrote an already-relative ascending path:\n%s", got)
	}
}

// TestImportRegionRendersCommonJSDeclarations pins the dialect half of flat
// config support: `npx expo lint` generates a CommonJS eslint.config.js, and
// an `import` statement spliced into one is a SyntaxError.
func TestImportRegionRendersCommonJSDeclarations(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root}
	layers := []preset.Layer{{Package: "eslint-config-expo/flat.js", Binding: "dharnessExpo", Because: "documented"}}

	got := eslintImportRegion(p, p.Source, layers, jsconfig.CommonJS, "", "\n")

	for _, want := range []string{
		`const dharnessPlugin = require("dharness-eslint-plugin");`,
		`const dharnessExpo = require("eslint-config-expo/flat.js");`,
		`const dharnessLayer = require("./.dharness/eslint.config.cjs");`,
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
// same requirement. The project's config loads .dharness/eslint.config.cjs by
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
// eslint-config-expo/flat.js 57.0.2: the first comes back with
// Symbol.toStringTag === "Module" and its configs under .default, the second
// is a plain CommonJS config array with no tag at all. Unwrapping only a
// tagged namespace is what keeps both correct.
func TestCommonJSUnwrapsAnESModuleNamespace(t *testing.T) {
	p := project.At(t.TempDir(), t.TempDir())
	layers := []preset.Layer{
		{Package: "eslint-config-expo/flat.js", Binding: "dharnessExpo", Because: "documented"},
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

// TestOwnedEslintConfigIgnoresTheDirectoryDharnessOwns is F4: the first
// error ESLint reported once the layer was finally wired came from
// dharness's own generated file — `dharness/require-jsdoc` firing on the
// `export default function dharnessLayer(...)` this very function writes.
//
// The file's own header says dharness rewrites it, so any JSDoc a user adds
// is gone on the next sync. A rule nobody can satisfy is worse than no rule:
// the layer ignores the directory it owns instead. That also covers whatever
// else lands in .dharness/ later, which emitting one JSDoc comment would
// not.
//
// It is the array's first element and carries nothing but `ignores`, which
// is what makes ESLint read it as a global ignore rather than as
// configuration for matched files.
func TestOwnedEslintConfigIgnoresTheDirectoryDharnessOwns(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root}

	got := ownedEslintConfig(p, nil, jsconfig.ESM)

	want := fmt.Sprintf("    { ignores: [%q] },\n", project.Dir+"/**")
	if !strings.Contains(got, want) {
		t.Errorf("ownedEslintConfig() does not ignore the directory it owns, want %s:\n%s", want, got)
	}
	if !strings.Contains(got, "return [\n"+want) {
		t.Errorf("the ignore is not the array's first element:\n%s", got)
	}
}

// TestOwnedEslintConfigOmitsTheIgnoreOutsideTheEslintBasePath is the split
// layout: the project's ESLint config lives with package.json and the owned
// directory sits at the repository root, so the pattern would have to climb
// out of ESLint's base path — which ESLint does not accept, and does not
// need, because nothing above that path is linted in the first place.
func TestOwnedEslintConfigOmitsTheIgnoreOutsideTheEslintBasePath(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "frontend")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	p := project.Project{Root: root, Source: source}

	got := ownedEslintConfig(p, nil, jsconfig.ESM)

	if strings.Contains(got, "ignores:") {
		t.Errorf("ownedEslintConfig() emitted an ignore pattern that climbs out of the base path:\n%s", got)
	}
}

// TestOwnedEslintFileNameCarriesItsDialect is F5/C4: the generated file was
// always called eslint.config.js, whatever was written inside it.
//
// In a Next.js project — package.json with no "type", which is the norm —
// that means ESM syntax in a .js file, and Node prints
// MODULE_TYPELESS_PACKAGE_JSON on every ESLint run, which for an adopted
// project is every gated commit. Node's own remedy, adding "type": "module"
// to package.json, is a far larger change than the warning warrants and is
// wrong for Next.js.
//
// The extension answers instead, and it also closes a case that was broken
// rather than noisy: module.exports in a .js file inside a package.json that
// *does* declare "type": "module" is a SyntaxError, not a warning.
func TestOwnedEslintFileNameCarriesItsDialect(t *testing.T) {
	if got := ownedEslintName(jsconfig.ESM); got != "eslint.config.mjs" {
		t.Errorf("ownedEslintName(ESM) = %q, want eslint.config.mjs", got)
	}
	if got := ownedEslintName(jsconfig.CommonJS); got != "eslint.config.cjs" {
		t.Errorf("ownedEslintName(CommonJS) = %q, want eslint.config.cjs", got)
	}
}

// TestOwnedFilesStepRemovesTheLegacyOwnedEslintConfig covers the upgrade: a
// project adopted before F5 carries .dharness/eslint.config.js, and the
// reference in its own config is rewritten to the new name by the marked
// region. Leaving the old file behind would commit a config nothing imports.
func TestOwnedFilesStepRemovesTheLegacyOwnedEslintConfig(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root}
	legacy := filepath.Join(root, project.Dir, ownedEslintLegacyName)
	if err := os.MkdirAll(filepath.Join(root, project.Dir), 0o755); err != nil {
		t.Fatal(err)
	}
	writeStepFixtureFile(t, filepath.Join(root, project.Dir), ownedEslintLegacyName, "// written by an older dharness\n")

	if _, err := (ownedFilesStep{}).Apply(p, &Writer{}, io.Discard); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Errorf("Apply() left %s behind; nothing imports it any more", ownedEslintLegacyName)
	}
	if _, err := os.Stat(filepath.Join(root, project.Dir, ownedEslintName(jsconfig.ESM))); err != nil {
		t.Errorf("Apply() did not write the dialect-named file: %v", err)
	}
}

// TestOwnedFilesStepRestoresTheLegacyFileOnRollback keeps the removal inside
// the same transaction as every other write: a run that fails later put the
// repository back as it found it, and a deleted file is not an exception to
// that.
func TestOwnedFilesStepRestoresTheLegacyFileOnRollback(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root}
	legacy := filepath.Join(root, project.Dir, ownedEslintLegacyName)
	if err := os.MkdirAll(filepath.Join(root, project.Dir), 0o755); err != nil {
		t.Fatal(err)
	}
	writeStepFixtureFile(t, filepath.Join(root, project.Dir), ownedEslintLegacyName, "// written by an older dharness\n")

	w := &Writer{}
	if _, err := (ownedFilesStep{}).Apply(p, w, io.Discard); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if err := w.Undo(); err != nil {
		t.Fatalf("Undo() = %v", err)
	}

	raw, err := os.ReadFile(legacy)
	if err != nil {
		t.Fatalf("Undo() did not restore the legacy file: %v", err)
	}
	if string(raw) != "// written by an older dharness\n" {
		t.Errorf("Undo() restored %q, want the bytes it found", raw)
	}
}

// TestContributedLayersDropsWhatTheProjectAlreadySpreads is the duplication
// the splice created once jsconfig learned create-next-app's shape.
//
// The scaffolder's config spreads eslint-config-next/core-web-vitals and
// eslint-config-next/typescript itself. The nextjs preset contributes the
// same two packages, so the wired result imported each one twice under two
// names and spread both — the framework's whole config applied twice, in a
// file dharness had just edited.
//
// dharness contributes what is missing, not what is already there. That
// keeps the fix inside what dharness owns: nothing of the project's is
// rewritten, and the marked regions stay the only bytes this tool touches.
func TestContributedLayersDropsWhatTheProjectAlreadySpreads(t *testing.T) {
	config := []byte(`import { defineConfig } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";

const eslintConfig = defineConfig([...nextVitals]);
export default eslintConfig;
`)
	layers := []preset.Layer{
		{Package: "eslint-config-next/core-web-vitals", Binding: "dharnessNext", Spread: true},
		{Package: "eslint-plugin-react-doctor", Binding: "dharnessReactDoctor"},
	}

	got := contributedLayers(layers, config)

	if len(got) != 1 || got[0].Package != "eslint-plugin-react-doctor" {
		t.Errorf("contributedLayers() = %v, want only the layer the project does not already have", got)
	}
}

// TestContributedLayersIgnoresDharnessOwnImportRegion is what keeps the
// filter from oscillating. dharness's own marked region imports every layer
// it contributes, so reading the file back without excluding that region
// would drop every layer on the second run and re-add them on the third.
func TestContributedLayersIgnoresDharnessOwnImportRegion(t *testing.T) {
	config := []byte(eslintImportBegin + `
import dharnessPlugin from "dharness-eslint-plugin";
import dharnessNext from "eslint-config-next/core-web-vitals";
` + eslintImportEnd + `

export default [];
`)
	layers := []preset.Layer{{Package: "eslint-config-next/core-web-vitals", Binding: "dharnessNext", Spread: true}}

	if got := contributedLayers(layers, config); len(got) != 1 {
		t.Errorf("contributedLayers() = %v, want the layer kept: dharness's own import is not the project's", got)
	}
}

// TestContributedLayersKeepsADifferentSubpath pins the comparison at the
// specifier rather than the package. eslint-config-next and
// eslint-config-next/core-web-vitals are different configs — the bare one is
// the weaker base — so a project spreading one has not already got the
// other.
func TestContributedLayersKeepsADifferentSubpath(t *testing.T) {
	config := []byte("import next from \"eslint-config-next\";\nexport default [...next];\n")
	layers := []preset.Layer{{Package: "eslint-config-next/core-web-vitals", Binding: "dharnessNext", Spread: true}}

	if got := contributedLayers(layers, config); len(got) != 1 {
		t.Errorf("contributedLayers() = %v, want the subpath kept", got)
	}
}

// TestContributedLayersFoldsTheJSExtension is the other half of the subpath
// rule, and the reason contributedLayers survived the Expo specifier gaining
// ".js". dharness contributes "eslint-config-expo/flat.js" because Node ESM
// refuses the bare subpath as a directory import; `npx expo lint` generates
// `require("eslint-config-expo/flat")`, and CommonJS resolves both spellings
// to the same file — measured on 57.0.2, the two requires return the
// identical instance.
//
// Compared literally, the project would stop looking like it already had the
// config and the whole thing would come back a second time in a file that
// already spreads it.
func TestContributedLayersFoldsTheJSExtension(t *testing.T) {
	config := []byte("const expoConfig = require('eslint-config-expo/flat');\nmodule.exports = [expoConfig];\n")
	layers := []preset.Layer{{Package: "eslint-config-expo/flat.js", Binding: "dharnessExpo", Spread: true}}

	if got := contributedLayers(layers, config); len(got) != 0 {
		t.Errorf("contributedLayers() = %v, want the layer dropped: require() resolves both spellings to one file", got)
	}
}

// TestContributedLayersFoldsNothingButJS holds the fold to the one extension
// it is true for. require() appends ".js" to a specifier written without
// one, so those two spellings are provably one module; it does not append
// ".mjs", so "X/foo" and "X/foo.mjs" are different files and folding them
// would drop a layer the project has not got.
func TestContributedLayersFoldsNothingButJS(t *testing.T) {
	config := []byte("import base from \"eslint-config-example/flat\";\nexport default [...base];\n")
	layers := []preset.Layer{{Package: "eslint-config-example/flat.mjs", Binding: "dharnessExample", Spread: true}}

	if got := contributedLayers(layers, config); len(got) != 1 {
		t.Errorf("contributedLayers() = %v, want the layer kept: .mjs is not what require() would have resolved", got)
	}
}
