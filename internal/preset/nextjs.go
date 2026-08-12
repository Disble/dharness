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

// eslintConfigNextPackage is what nextjsESLintDocs names — published and
// versioned by Next.js itself (vercel/next.js, packages/eslint-config-next),
// not a version dharness invents.
const eslintConfigNextPackage = "eslint-config-next"

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
			Layers: []Layer{
				{
					Package: eslintConfigNextPackage,
					Binding: "dharnessNext",
					Because: nextjsESLintDocs + `: "Next.js provides an ESLint configuration package, ` +
						"`eslint-config-next`" + `" — published and versioned by Next.js itself, not a ` +
						`version dharness invents`,
				},
			},
		},
	}, true
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
