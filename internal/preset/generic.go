package preset

import (
	"fmt"

	"github.com/Disble/dharness/internal/project"
)

// generic is the preset every project that matches nothing else falls back
// to. It exists so "today's behaviour" is one preset among several rather
// than a special case reachable only through absence.
//
// Its manifest was empty when it was introduced, which is what made the
// golden pin's byte-identity claim provable through the whole registry
// refactor: no framework, no contribution, no change. It is where dharness's
// framework-independent opinions belong, so the first of them landing here is
// the mechanism working rather than the claim breaking — and the frozen
// golden is what made that change impossible to ship unnoticed.
type generic struct{}

func (generic) ID() string   { return "generic" }
func (generic) Scope() Scope { return Root }

// dharness's cross-cutting opinion about how fallow should look for
// duplication, expressed with fallow's own keys rather than with anything
// dharness invented — this is configuration, not a feature.
//
// Each of the three departs from a fallow default, verified against its
// schema. A key that only restated a default would be noise every adopting
// repository has to maintain and re-read whenever fallow adds a rule, which
// is why the defensive nine-rule pin that once sat beside this was dropped:
// if a default changes, that shows up in fallow's changelog, not in a file
// of ours that says nothing.
//
//	mode            fallow: "mild"  → dharness: "semantic"
//	minOccurrences  fallow: 2       → dharness: 3
//	threshold       fallow: 0.0     → dharness: 3
//
// Semantic compares structure rather than text, so it still finds the
// copy-paste that has since diverged — which is the copy-paste that survives
// review. Two occurrences is usually coincidence and three is a pattern, so
// raising the floor is what keeps the report about code worth refactoring.
//
// Three per cent is borrowed from the SonarQube quality gate this repository
// already answers to, and the number is only half the story: the two tools
// measure different things by different algorithms, so this is a companion
// signal rather than the same gate expressed twice. It is worth having anyway
// — a project that never asks the question gets no answer from either.
//
// It travels as a manifest fact rather than sitting in the owned file's
// skeleton, and that placement is load-bearing: fallow's `extends` replaces a
// key instead of merging it, so a project declaring its own `duplicates`
// silently discards dharness's. Only a contributed key joins
// boundariesOwnerStep's candidate set, and only a candidate gets reported
// rather than vanishing.
const (
	duplicationMode        = "semantic"
	duplicationOccurrences = 3
	duplicationCeiling     = 3
)

// Detect always matches. There is nothing to read, and nothing it could
// fail to find: generic is the answer once nothing else has one, and it is
// where dharness's framework-independent opinions live.
func (generic) Detect(project.Project) (Match, bool) {
	return Match{
		ID:       "generic",
		Scope:    Root,
		Evidence: "no framework signal matched",
		Manifest: Manifest{
			Schema: Schema,
			Facts: []Fact{{
				Key: "duplicates",
				Value: map[string]any{
					"mode":           duplicationMode,
					"minOccurrences": duplicationOccurrences,
					"threshold":      duplicationCeiling,
				},
				Because: fmt.Sprintf(
					"dharness compares structure rather than text (mode %q), reports a clone group only from %d occurrences, and caps duplication at %d%%. fallow's own defaults are mild, 2 and no limit, so all three are departures rather than restatements: structural matching still finds the copy-paste that has since diverged, two occurrences is usually coincidence where three is a pattern, and the ceiling is borrowed from the SonarQube gate this toolchain already answers to — different algorithms, so they corroborate rather than repeat each other",
					duplicationMode, duplicationOccurrences, duplicationCeiling),
			}},
		},
	}, true
}
