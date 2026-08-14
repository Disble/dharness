package preset

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Disble/dharness/internal/project"
)

// nextjsDependency is the package.json key Next.js's own installer writes.
const nextjsDependency = "next"

// nextjsDocs is the page these seeds are quoted from, read directly against
// Next.js's own documentation for this change (not carried over from a
// local checkout — see the framework-presets task list, Slice 5, Phase 17).
const nextjsDocs = "https://nextjs.org/docs/app/getting-started/project-structure"

// nextjsESLintDocs is the page the Layer's Because cites — a different page
// from nextjsDocs above, read directly against Next.js's own documentation
// for this change: "Next.js provides an ESLint configuration package,
// `eslint-config-next`", the package this preset contributes.
const nextjsESLintDocs = "https://nextjs.org/docs/app/api-reference/config/eslint"

// eslintConfigNextPackage is the flat config Next.js recommends, named by
// nextjsESLintDocs — published and versioned by Next.js itself
// (vercel/next.js, packages/eslint-config-next), not a version dharness
// invents.
//
// It is the core-web-vitals subpath, not the bare package. The first version
// of this preset contributed "eslint-config-next", which is the base config;
// Next.js's own page calls core-web-vitals "Recommended for most projects"
// and every flat-config example on it imports that subpath. Contributing the
// base config shipped a weaker rule set than the framework recommends while
// citing the page that says so.
const eslintConfigNextPackage = "eslint-config-next/core-web-vitals"

// eslintConfigNextTypeScriptPackage is the TypeScript companion the same
// page documents: "Adds TypeScript-specific linting rules from
// typescript-eslint. Use this alongside the base or core-web-vitals config."
// It is contributed only when the project is TypeScript, read off tsconfig.json.
const eslintConfigNextTypeScriptPackage = "eslint-config-next/typescript"

// reactDoctorPluginPackage is the ESLint plugin react-doctor publishes so its
// rules run inside ESLint.
//
// dharness needs it for a reason particular to this toolchain rather than a
// general preference. react-doctor's CLI adopts an existing lint config only
// when that config is JSON, and dharness writes flat config JavaScript — so
// the policy dharness composes is not the policy the react-doctor CLI phase
// of the gate evaluates. react-doctor's own documented answer is the inverse
// direction: install the plugin and put its presets inside ESLint. That
// makes the flat config the single place the rules live.
const reactDoctorPluginPackage = "eslint-plugin-react-doctor"

// reactDoctorPluginDocs is the page every react-doctor Layer's Because cites.
const reactDoctorPluginDocs = "https://www.react.doctor/docs/configuration/eslint-and-oxlint-plugins"

// reactDoctorBinding is the one identifier the plugin is imported under, no
// matter how many presets read a config off it. Several framework presets
// contribute a layer bound to it — nextjs reads configs.next, expo reads
// configs["react-native"] — and internal/setup emits one import and one
// destructured parameter for all of them, because two import declarations
// binding one identifier in an ES module is a SyntaxError.
const reactDoctorBinding = "dharnessReactDoctor"

// nextjs is the Next.js preset: Source scope, because "next" is declared
// where the JS project lives, not at the repository root.
//
// It contributes no ignorePatterns. fallow honours gitignore, and .next/ is
// gitignored by every Next.js starter — measured: an orphan file inside a
// gitignored .next/ is not reported by fallow at all, while the same file in
// a tracked directory is reported as unused. A pattern here would
// re-implement what the CLI already does, which the first rule of this
// repository's CLAUDE.md forbids. A preset earns an ignore pattern only for
// generated code the project commits — wails.go's wailsjs/** is that case;
// Next.js's own .next/ is not.
type nextjs struct{}

func (nextjs) ID() string   { return "nextjs" }
func (nextjs) Scope() Scope { return Source }

// Detect answers "does this project depend on next" from package.json's
// dependencies/devDependencies alone. What it contributes is two seeds —
// Next.js's own documented structure — never a fact: Next.js is explicit
// that it takes no position on anything beyond routing, so asserting more
// would invent a convention the framework itself disclaims. The one Layer
// it does contribute is different in kind: eslint-config-next is an
// installable package Next.js itself publishes and versions, not a
// convention dharness is guessing at.
func (nextjs) Detect(p project.Project) (Match, bool) {
	path := filepath.Join(p.Source, "package.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return Match{}, false
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if json.Unmarshal(raw, &pkg) != nil {
		return Match{}, false
	}
	if !declaresDependency(pkg.Dependencies, pkg.DevDependencies, nextjsDependency) {
		return Match{}, false
	}

	display := filepath.ToSlash(path)
	return Match{
		ID:       "nextjs",
		Scope:    Source,
		Evidence: fmt.Sprintf("%s declares %q", display, nextjsDependency),
		Manifest: Manifest{
			Schema: Schema,
			Seeds: []Seed{
				{
					Text: "Next.js documents four top-level folders: app/ (App Router) or " +
						"pages/ (Pages Router) for routing, public/ for static assets served " +
						"as-is, and an optional src/ that wraps the rest. Routes are a " +
						"delivery shell around the domain, not domain modules themselves.",
					Because: nextjsDocs + " — Next.js's own documented top-level project structure",
				},
				{
					Text: "Beyond routing, Next.js takes no position on how the rest of the " +
						"source is organized — read the real seams off this tree rather than " +
						"assuming a components/ or lib/ convention.",
					Because: nextjsDocs + `: "Next.js is unopinionated about how you organize and ` +
						`colocate your project files," and its own components/lib example ` +
						`folders "have no special framework significance."`,
				},
			},
			Layers: nextjsLayers(p),
		},
	}, true
}

// nextjsLayers is what a Next.js project's ESLint config is composed of:
// the framework's own recommended flat config, its TypeScript companion when
// the project is TypeScript, and react-doctor's Next.js preset.
//
// The order is the order they are rendered into the config array, and flat
// config resolves rules last-wins, so it is a decision rather than a listing:
// the framework's own rules come first and react-doctor layers on top,
// because react-doctor is the specialist this toolchain runs the gate on.
func nextjsLayers(p project.Project) []Layer {
	layers := []Layer{
		{
			Package: eslintConfigNextPackage,
			Binding: "dharnessNext",
			Spread:  true,
			Because: nextjsESLintDocs + `: "eslint-config-next/core-web-vitals: Includes everything ` +
				`from the base config, plus upgrades rules that impact Core Web Vitals from warnings ` +
				`to errors. Recommended for most projects." — published and versioned by Next.js ` +
				`itself, not a version dharness invents`,
		},
	}

	if declaresTypeScript(p.Source) {
		layers = append(layers, Layer{
			Package: eslintConfigNextTypeScriptPackage,
			Binding: "dharnessNextTypeScript",
			Spread:  true,
			Because: nextjsESLintDocs + `: "eslint-config-next/typescript: Adds TypeScript-specific ` +
				`linting rules from typescript-eslint. Use this alongside the base or core-web-vitals ` +
				`config." — contributed because this project declares a tsconfig.json`,
		})
	}

	return append(layers, reactDoctorRecommended, Layer{
		Package:  reactDoctorPluginPackage,
		Binding:  reactDoctorBinding,
		Accessor: []string{"configs", "next"},
		Because: reactDoctorPluginDocs + `: react-doctor publishes a "next" preset in its own ESLint ` +
			`plugin. dharness runs react-doctor in the gate, and react-doctor's CLI adopts an existing ` +
			`lint config only when that config is JSON — so a flat config's rules reach it through this ` +
			`plugin or not at all`,
	})
}

// reactDoctorRecommended is react-doctor's framework-independent rule set,
// which every framework preset contributes alongside its own.
//
// It is a separate layer because the framework presets do not include it.
// Measured against eslint-plugin-react-doctor 0.9.12 rather than read off
// the documentation, which does not say either way: `recommended` carries
// 581 rules, `next` carries 25 and `react-native` 40, and the overlap
// between recommended and either of them is zero. A preset that contributed
// only its framework config would ship 25 rules out of 606 and read as if it
// had wired react-doctor up.
//
// The framework preset is rendered after it, so a framework-specific rule
// wins where flat config resolves last-wins. Today that ordering decides
// nothing, because the two sets are disjoint; a future react-doctor release
// that overlaps them is what would make it matter, and this is where to look
// when it does.
var reactDoctorRecommended = Layer{
	Package:  reactDoctorPluginPackage,
	Binding:  reactDoctorBinding,
	Accessor: []string{"configs", "recommended"},
	Because: reactDoctorPluginDocs + `: "` + "`recommended`" + ` contains the framework-independent ` +
		`rules" — the framework presets add to it rather than including it (measured against ` +
		`eslint-plugin-react-doctor 0.9.12: 581 rules in recommended, none of them in next or ` +
		`react-native)`,
}

// tsconfigFile is the file that answers "is this project TypeScript". It is
// the direct signal (§09): TypeScript's own compiler locates a project by
// this file, so its presence is what "TypeScript project" means rather than
// a proxy like a .ts file somewhere in the tree.
const tsconfigFile = "tsconfig.json"

// declaresTypeScript reports whether source holds a tsconfig.json. A
// malformed one still counts: the question is whether the project chose
// TypeScript, and it did — parsing it would be answering a different one,
// and tsconfig.json is JSONC besides, which encoding/json cannot read.
func declaresTypeScript(source string) bool {
	if source == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(source, tsconfigFile))
	return err == nil && !info.IsDir()
}

// declaresDependency reports whether name is declared in either dependency
// map — shared by nextjs and expo, the only two presets whose signal is a
// package.json dependency rather than a file's own presence.
func declaresDependency(deps, devDeps map[string]string, name string) bool {
	if _, ok := deps[name]; ok {
		return true
	}
	_, ok := devDeps[name]
	return ok
}
