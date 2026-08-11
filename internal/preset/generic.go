package preset

import "github.com/Disble/dharness/internal/project"

// generic is the preset every project that matches nothing else falls back
// to. It exists so "today's behaviour" is one preset among several rather
// than a special case reachable only through absence, and its empty
// manifest is what makes the golden pin's byte-identity claim provable
// (design decision 9).
type generic struct{}

func (generic) ID() string   { return "generic" }
func (generic) Scope() Scope { return Root }

// Detect always matches. There is nothing to read, and nothing it could
// fail to find: generic is the answer once nothing else has one.
func (generic) Detect(project.Project) (Match, bool) {
	return Match{
		ID:       "generic",
		Scope:    Root,
		Evidence: "no framework signal matched",
		Manifest: Manifest{Schema: Schema},
	}, true
}
