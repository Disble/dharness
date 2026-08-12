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

// sortedRuleIDs is RuleIDs() in the same alphabetical order doctorConfigStep
// already renders its rules in — that ordering falls out there from
// json.Marshal on a Go map, which this function reproduces explicitly since
// the owned config is JavaScript, not JSON.
func sortedRuleIDs() []string {
	ids := RuleIDs()
	sort.Strings(ids)
	return ids
}
