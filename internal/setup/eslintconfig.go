package setup

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Disble/dharness/internal/preset"
	"github.com/Disble/dharness/internal/project"
)

// ownedEslintConfig renders .dharness/eslint.config.js: dharness writes this
// file, and it exports a factory rather than a config array because the
// packages it names are installed beside the project's package.json, and a
// bare specifier resolves from the file that writes it — which, in a split
// layout, is not this directory (design decision 3).
//
// Both the factory's parameter list and the array it returns are built from
// the same layers slice, so the destructuring and the spread cannot
// disagree — a structural property, not a convention. layers is empty until
// slice 5's presets start contributing one; an empty slice renders a
// plugin-only factory, valid JavaScript with no dangling spread.
func ownedEslintConfig(p project.Project, layers []preset.Layer) string {
	params := []string{"plugin"}
	for _, layer := range layers {
		params = append(params, layer.Binding)
	}

	var body strings.Builder
	body.WriteString("// dharness writes this file. It exports a factory rather than a config\n")
	body.WriteString("// array because the packages it names are installed beside the project's\n")
	body.WriteString("// package.json, and a bare specifier resolves from the file that writes it\n")
	body.WriteString("// — which, in a split layout, is not this directory.\n")
	fmt.Fprintf(&body, "export default function dharnessLayer({ %s }) {\n", strings.Join(params, ", "))
	body.WriteString("  return [\n")
	for _, layer := range layers {
		fmt.Fprintf(&body, "    ...%s,\n", layer.Binding)
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
func eslintImportRegion(p project.Project, dir string, layers []preset.Layer, indent, eol string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s%s%s", indent, eslintImportBegin, eol)
	fmt.Fprintf(&b, "%simport dharnessPlugin from %q;%s", indent, RulesPackage, eol)
	for _, layer := range layers {
		fmt.Fprintf(&b, "%simport %s from %q;%s", indent, layer.Binding, layer.Package, eol)
	}
	fmt.Fprintf(&b, "%simport dharnessLayer from %q;%s", indent, ownedFrom(p, dir, ownedEslint), eol)
	fmt.Fprintf(&b, "%s%s%s", indent, eslintImportEnd, eol)
	return b.String()
}

// eslintLayerRegion renders the marked spread element: a call into the
// owned factory, passing the plugin under its fixed "plugin" parameter name
// and every contributed layer under its own binding — the same list
// eslintImportRegion imports and ownedEslintConfig destructures, so none of
// the three can disagree (design decision 3).
func eslintLayerRegion(layers []preset.Layer, indent, eol string) string {
	args := []string{"plugin: dharnessPlugin"}
	for _, layer := range layers {
		args = append(args, layer.Binding)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s%s%s", indent, eslintLayerBegin, eol)
	fmt.Fprintf(&b, "%s...dharnessLayer({ %s }),%s", indent, strings.Join(args, ", "), eol)
	fmt.Fprintf(&b, "%s%s%s", indent, eslintLayerEnd, eol)
	return b.String()
}
