package preset

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Disble/dharness/internal/project"
)

// expoDependency is the package.json key Expo's own installer writes.
const expoDependency = "expo"

// expo is the Expo preset: Source scope, "expo" declared in package.json.
//
// Detection only, empty manifest — no facts, no seeds. Expo's file-based-
// routing documentation could not be verified against Expo's own docs
// during this change (the page returned 404), and CLAUDE.md's second rule —
// the verdict comes from evidence, never from prose read into the model —
// applies just as hard to a seed as to a rule: a claim this binary cannot
// check must not ship as if it were checked. An empty seed is honest; an
// invented one is exactly what this repository has already corrected more
// than once. This preset stays detection-only until a future change
// verifies a real fact against Expo's own documentation and adds it —
// recorded here so the emptiness reads as a decision, not an oversight.
//
// It contributes no ignorePatterns for the same reason nextjs does not:
// .expo/ is gitignored by every Expo starter, and fallow already honours
// gitignore — a pattern here would re-implement what the CLI already does.
type expo struct{}

func (expo) ID() string   { return "expo" }
func (expo) Scope() Scope { return Source }

func (expo) Detect(p project.Project) (Match, bool) {
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
	if !declaresDependency(pkg.Dependencies, pkg.DevDependencies, expoDependency) {
		return Match{}, false
	}

	display := filepath.ToSlash(path)
	return Match{
		ID:       "expo",
		Scope:    Source,
		Evidence: fmt.Sprintf("%s declares %q", display, expoDependency),
		Manifest: Manifest{Schema: Schema},
	}, true
}
