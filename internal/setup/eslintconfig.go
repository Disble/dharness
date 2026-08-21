package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Disble/dharness/internal/jsconfig"
	"github.com/Disble/dharness/internal/preset"
	"github.com/Disble/dharness/internal/project"
)

// projectEslintModule reports which dialect dharness must write for this
// project: the one the project's own flat config is already written in.
//
// It is derived on every call rather than recorded (§07). A project that has
// no flat config yet gets ESM, because that is what wireEslintExtends is
// about to write for it — the two answers have to agree or the config
// dharness creates cannot load the file dharness owns.
func projectEslintModule(p project.Project) jsconfig.Module {
	path := eslintFlatConfig(p.Source)
	if path == "" {
		return jsconfig.ESM
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return jsconfig.ESM
	}
	return jsconfig.ModuleOf(raw)
}

// ownedEslintConfig renders .dharness/eslint.config.js: dharness writes this
// file, and it exports a factory rather than a config array because the
// packages it names are installed beside the project's package.json, and a
// bare specifier resolves from the file that writes it — which, in a split
// layout, is not this directory (design decision 3).
//
// Both the factory's parameter list and the array it returns are built from
// the same layers slice, so the destructuring and the spread cannot
// disagree — a structural property, not a convention. An empty slice renders
// a plugin-only factory, valid JavaScript with no dangling spread.
//
// The returned array includes each distinct expression once. Two presets
// matching the same repository contribute react-doctor's framework-
// independent rules each, and rendering the identical element twice reads as
// a bug in a file people open. First occurrence wins, which is the only
// ordering safe to assert: dropping a later copy could only change behaviour
// if something between the two turned a rule off that the copy turned back
// on, and dharness never needs one config object to do that.
func ownedEslintConfig(p project.Project, layers []preset.Layer, module jsconfig.Module) string {
	params := append([]string{"plugin"}, layerBindings(layers)...)

	export := "export default function"
	if module == jsconfig.CommonJS {
		export = "module.exports = function"
	}

	var body strings.Builder
	body.WriteString("// dharness writes this file. It exports a factory rather than a config\n")
	body.WriteString("// array because the packages it names are installed beside the project's\n")
	body.WriteString("// package.json, and a bare specifier resolves from the file that writes it\n")
	body.WriteString("// — which, in a split layout, is not this directory.\n")
	fmt.Fprintf(&body, "%s dharnessLayer({ %s }) {\n", export, strings.Join(params, ", "))
	if module == jsconfig.CommonJS && len(layers) > 0 {
		body.WriteString(commonJSInterop)
	}
	body.WriteString("  return [\n")
	// The ignore leads, and carries nothing but `ignores`, which is what
	// makes ESLint read it as a global ignore rather than as configuration
	// for the files it matches.
	if pattern := ownedIgnorePattern(p); pattern != "" {
		fmt.Fprintf(&body, "    { ignores: [%q] },\n", pattern)
	}
	included := map[string]bool{}
	for _, layer := range layers {
		expression := layerExpression(layer, module)
		if included[expression] {
			continue
		}
		included[expression] = true
		fmt.Fprintf(&body, "    %s,\n", expression)
	}
	body.WriteString("    {\n")
	body.WriteString("      plugins: { dharness: plugin },\n")
	body.WriteString("      rules: {\n")
	for _, id := range sortedRuleIDs() {
		fmt.Fprintf(&body, "        %q: %q,\n", id, DefaultSeverity(p, id))
	}
	body.WriteString("      },\n")
	body.WriteString("    },\n")
	body.WriteString("  ];\n")
	body.WriteString("}\n")
	return body.String()
}

// ownedIgnorePattern is the glob that keeps ESLint out of the directory
// dharness owns, or "" when the question does not arise.
//
// It exists because dharness's own rules fired on dharness's own generated
// file: `dharness/require-jsdoc` on the `export default function
// dharnessLayer(...)` ownedEslintConfig writes, in a file whose header says
// every edit to it is lost on the next sync. The rule was unsatisfiable by
// the only person it fired at.
//
// The pattern is relative to the project's ESLint config, which is what
// ESLint resolves `ignores` against. In a split layout the owned directory
// sits above that config — outside ESLint's base path, so nothing there is
// linted and a pattern climbing out with "../" would be rejected rather than
// helpful. Nothing is emitted there.
func ownedIgnorePattern(p project.Project) string {
	if !p.HasSource() {
		return project.Dir + "/**"
	}
	rel, err := filepath.Rel(p.Source, filepath.Join(p.Root, project.Dir))
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	return filepath.ToSlash(rel) + "/**"
}

// commonJSInteropName is the helper every CommonJS-rendered layer is read
// through, and commonJSInterop is the helper itself.
//
// A CommonJS require() of an ES module returns the module *namespace*, not
// its default export: the config sits under .default, and reaching for
// .configs on the namespace yields undefined. Reached by `import` the same
// package is already the default export, so this is a difference between the
// two dialects and not between packages.
//
// The check is Symbol.toStringTag, because that is the only thing that tells
// the two apart at runtime. Measured against the packages the registry
// actually names: eslint-plugin-react-doctor 0.9.12 required from CommonJS
// comes back tagged "Module" with its configs under .default, while
// eslint-config-expo/flat 57.0.1 is a plain object with no tag. A looser
// check — "does it have .default" — would unwrap a CommonJS module that
// happens to export a key called default, which is a different thing.
//
// It lives in the file dharness owns rather than in the project's own
// config, which only ever gains a reference (§03).
const (
	commonJSInteropName = "dharnessDefault"

	commonJSInterop = "  // require() of an ES module yields the namespace, whose default export\n" +
		"  // sits under .default; a real CommonJS module is already the value.\n" +
		"  const " + commonJSInteropName + " = (m) =>\n" +
		"    m && m[Symbol.toStringTag] === \"Module\" && \"default\" in m ? m.default : m;\n"
)

// layerExpression renders one layer as the element it contributes to the
// returned array: the binding, the property path the config sits at inside
// it, and the spread that a config array needs and a single config object
// must not have.
//
// A segment that is a JavaScript identifier is dotted; anything else is
// subscripted as a quoted string, which is how react-doctor's own
// documentation spells configs["react-native"]. %q is what makes the
// subscript safe — the segments come from this repository's own registry,
// but rendering them unescaped would make a future one a code-injection
// site in a file every adopting project imports.
func layerExpression(layer preset.Layer, module jsconfig.Module) string {
	var b strings.Builder
	if layer.Spread {
		b.WriteString("...")
	}
	if module == jsconfig.CommonJS {
		fmt.Fprintf(&b, "%s(%s)", commonJSInteropName, layer.Binding)
	} else {
		b.WriteString(layer.Binding)
	}
	for _, segment := range layer.Accessor {
		if bindingPattern.MatchString(segment) {
			fmt.Fprintf(&b, ".%s", segment)
			continue
		}
		fmt.Fprintf(&b, "[%q]", segment)
	}
	return b.String()
}

// bindingPattern is the JavaScript identifier grammar, used here to choose
// between dot and subscript notation. preset.Manifest.Validate holds its own
// copy for a different question — whether an authored binding is legal at
// all — and the two are deliberately not shared: this one is a rendering
// choice that never rejects anything, and collapsing them would let a
// tightening of the authoring rule silently change generated syntax.
var bindingPattern = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

// layerModules lists the distinct modules the layers name, one entry per
// binding, in first-seen order.
//
// One module can carry several layers — every framework preset reads a
// different config off eslint-plugin-react-doctor — and it is imported once
// and destructured once for all of them. Two import declarations binding one
// identifier, or a duplicated name in a destructuring pattern, are both
// SyntaxErrors, so this is a correctness rule and not tidiness. The dedupe is
// safe because a binding maps to exactly one package, which
// TestNoBindingNamesTwoPackages holds the registry to.
//
// The three places that spell the binding list — the import region, the
// factory's parameter list and the factory call — all read it from here, so
// they cannot disagree (design decision 3).
func layerModules(layers []preset.Layer) []preset.Layer {
	var modules []preset.Layer
	seen := map[string]bool{}
	for _, layer := range layers {
		if seen[layer.Binding] {
			continue
		}
		seen[layer.Binding] = true
		modules = append(modules, layer)
	}
	return modules
}

// layerBindings is layerModules' binding names alone.
func layerBindings(layers []preset.Layer) []string {
	modules := layerModules(layers)
	bindings := make([]string, 0, len(modules))
	for _, module := range modules {
		bindings = append(bindings, module.Binding)
	}
	return bindings
}

// sortedRuleIDs is RuleIDs() sorted alphabetically, made explicit here since
// the owned config is JavaScript rather than JSON — there is no map-marshal
// order to inherit.
func sortedRuleIDs() []string {
	ids := RuleIDs()
	sort.Strings(ids)
	return ids
}

// eslintImportRegion renders the marked import region: the plugin's own
// import, one per contributed layer, and the import of the owned factory
// itself — computed from dir the same way ownedFrom computes fallow.jsonc's
// reference (files.go's stated reason for both to exist), so the reference
// resolves from wherever this file actually lives.
func eslintImportRegion(p project.Project, dir string, layers []preset.Layer, module jsconfig.Module, indent, eol string) string {
	declare := func(binding, specifier string) string {
		if module == jsconfig.CommonJS {
			return fmt.Sprintf("const %s = require(%q);", binding, specifier)
		}
		return fmt.Sprintf("import %s from %q;", binding, specifier)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s%s%s", indent, eslintImportBegin, eol)
	fmt.Fprintf(&b, "%s%s%s", indent, declare("dharnessPlugin", RulesPackage), eol)
	for _, layer := range layerModules(layers) {
		fmt.Fprintf(&b, "%s%s%s", indent, declare(layer.Binding, layer.Package), eol)
	}
	fmt.Fprintf(&b, "%s%s%s", indent, declare("dharnessLayer", moduleSpecifier(ownedFrom(p, dir, ownedEslintName(module)))), eol)
	fmt.Fprintf(&b, "%s%s%s", indent, eslintImportEnd, eol)
	return b.String()
}

// moduleSpecifier turns a relative path into one Node's resolver accepts.
//
// ownedFrom returns filepath.Rel's answer, which for a project config
// sitting beside .dharness/ is ".dharness/eslint.config.js". fallow's and
// lefthook's `extends` read that as the relative path it obviously is;
// Node does not. An ES module's relative specifier must begin with "./" or
// "../", and anything else is a *bare* specifier — a package name. So that
// path resolves to a package called ".dharness", is not found, and ESLint
// fails at startup rather than anywhere Go can see.
//
// A path that already ascends begins with "../" and is left alone.
func moduleSpecifier(path string) string {
	if strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../") {
		return path
	}
	return "./" + path
}

// eslintLayerRegion renders the marked spread element: a call into the
// owned factory, passing the plugin under its fixed "plugin" parameter name
// and every contributed layer under its own binding — the same list
// eslintImportRegion imports and ownedEslintConfig destructures, so none of
// the three can disagree (design decision 3).
func eslintLayerRegion(layers []preset.Layer, indent, eol string) string {
	args := append([]string{"plugin: dharnessPlugin"}, layerBindings(layers)...)

	var b strings.Builder
	fmt.Fprintf(&b, "%s%s%s", indent, eslintLayerBegin, eol)
	fmt.Fprintf(&b, "%s...dharnessLayer({ %s }),%s", indent, strings.Join(args, ", "), eol)
	fmt.Fprintf(&b, "%s%s%s", indent, eslintLayerEnd, eol)
	return b.String()
}

// contributedLayers drops the layers the project's own ESLint config already
// pulls in, leaving what dharness actually adds.
//
// It exists because recognising create-next-app's shape turned a delegated
// step into an edited file, and the framework's scaffolder spreads
// eslint-config-next/core-web-vitals and eslint-config-next/typescript
// itself. The nextjs preset contributes the same two, so the wired result
// imported each under a second name and spread it twice — the whole
// framework config applied twice, in a file dharness had just written into.
//
// The fix stays inside what dharness owns. Deleting the project's spreads
// would be the obvious alternative and it is the wrong one: those bytes are
// outside the marked regions, and §03 makes the marked regions the only
// bytes this tool touches. What dharness decides is what dharness
// contributes, so that is what changes.
//
// The comparison is on the specifier, never the package. eslint-config-next
// and eslint-config-next/core-web-vitals are different configs behind
// different subpath exports — the bare one is the weaker base — so a project
// spreading one has not already got the other.
//
// dharness's own marked import region is excluded before the question is
// asked. It imports every layer dharness contributes, so counting it would
// drop every layer on the second run and re-add them on the third.
func contributedLayers(layers []preset.Layer, projectConfig []byte) []preset.Layer {
	present := map[string]bool{}
	for _, specifier := range jsconfig.Imports(withoutDharnessImports(projectConfig)) {
		present[specifier] = true
	}

	kept := make([]preset.Layer, 0, len(layers))
	for _, layer := range layers {
		if present[layer.Package] {
			continue
		}
		kept = append(kept, layer)
	}
	return kept
}

// withoutDharnessImports returns src with dharness's own import region cut
// out, or src unchanged when the region is absent or malformed — the two
// cases where there is nothing to exclude and nothing safe to guess at.
func withoutDharnessImports(src []byte) []byte {
	from, to, state := markerRegion(string(src), eslintImportBegin, eslintImportEnd)
	if state != markersPresent {
		return src
	}
	out := make([]byte, 0, len(src)-(to-from))
	out = append(out, src[:from]...)
	return append(out, src[to:]...)
}

// projectContributedLayers is contributedLayers over the project's own
// config read from disk, for the callers that render the owned factory
// rather than splice — they answer the same question from the same file and
// must not disagree about it.
func projectContributedLayers(p project.Project, layers []preset.Layer) []preset.Layer {
	path := eslintFlatConfig(p.Source)
	if path == "" {
		return layers
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return layers
	}
	return contributedLayers(layers, raw)
}
