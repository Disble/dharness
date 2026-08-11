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

// duplicationCeiling is dharness's one cross-cutting opinion about fallow:
// a ceiling on duplicated code, expressed with fallow's own key rather than
// with anything dharness invented — its schema documents
// `duplicates.threshold` as "Maximum allowed duplication percentage (0 = no
// limit)", so this is configuration, not a feature.
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
const duplicationCeiling = 3

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
				Key:   "duplicates",
				Value: map[string]any{"threshold": duplicationCeiling},
				Because: fmt.Sprintf(
					"dharness sets a %d%% duplication ceiling for every project, borrowed from the SonarQube gate this toolchain already answers to; the two measure different things by different algorithms, so they corroborate rather than repeat each other",
					duplicationCeiling),
			}},
		},
	}, true
}
