// Package preset says what is true about a project like this, and how it
// knows. internal/setup says what gets written; this package holds no file
// paths, no JSONC, and no writer (design decision 1).
package preset

import (
	"encoding/json"
	"fmt"

	"github.com/Disble/dharness/internal/project"
)

// Schema versions the manifest shape a preset contributes. It is checked by
// Manifest.Validate, not by a run: a schema mismatch is an authoring bug,
// caught by the registry test that walks every preset before it ships.
const Schema = "dharness.preset/v1"

// Scope says which of project.Project's two directories a preset's signal
// lives in. It is answered before Detect so a Source-scope preset is never
// asked about a repository with no JS project (resolve's Source guard).
type Scope int

const (
	// Root scope signals live in project.Project.Root — a repository-level
	// file like wails.json.
	Root Scope = iota

	// Source scope signals live in project.Project.Source — a package.json
	// dependency, present only where a JS project exists at all.
	Source
)

// Preset is one framework's answer to "what is true about a project like
// this, and how do I know". It reports facts; it never writes.
type Preset interface {
	// ID names the preset in a report and in a fact's evidence.
	ID() string

	// Scope says which directory this preset's signal lives in.
	Scope() Scope

	// Detect reports whether this repository is such a project, and if so
	// everything the preset contributes. Pure: it reads, it never writes.
	Detect(p project.Project) (Match, bool)
}

// Match is what one matched preset contributes.
type Match struct {
	ID       string
	Scope    Scope
	Evidence string
	Manifest Manifest
}

// Fact is one thing a preset contributes, and the observable that justifies
// it. Because is not documentation: it is rendered into the file dharness
// writes, so a claim that has gone stale is visible in the repository rather
// than only in this binary's source.
type Fact struct {
	// Key is fallow's key name, spelled as fallow spells it.
	Key string

	// Value is anything encoding/json can render. Slices and string-keyed
	// maps encode deterministically, which the golden pin depends on.
	Value any

	// Because names the observable: a file, a key inside it, a documented
	// default. Never a justification of the design.
	Because string
}

// Manifest is an ordered set of facts. A slice, not a map: Go's map
// iteration order is randomised, and the region rendered into
// .dharness/fallow.jsonc must be byte-stable across runs or every sync
// produces a diff (the golden pin depends on it).
type Manifest struct {
	Schema string
	Facts  []Fact
}

// boundariesKey is reserved. Zones encode intent, and no preset may ever
// speak for it — the guard that keeps the region and architectureStep's
// block from ever reading as each other (design decision 8).
const boundariesKey = "boundaries"

// Validate reports an authoring error: an empty Because, an unencodable
// Value, a Schema other than Schema, or the reserved key "boundaries".
// Called by a test that walks the whole registry, never by a user's run —
// an authoring bug is recoverable, and failing sync for it would block on
// something dharness itself broke.
func (m Manifest) Validate() error {
	if m.Schema != Schema {
		return fmt.Errorf("manifest schema %q, want %q", m.Schema, Schema)
	}
	for _, fact := range m.Facts {
		if fact.Because == "" {
			return fmt.Errorf("fact %q carries no evidence", fact.Key)
		}
		if fact.Key == boundariesKey {
			return fmt.Errorf("fact %q is reserved: zones encode intent, and no preset may contribute it", boundariesKey)
		}
		if _, err := json.Marshal(fact.Value); err != nil {
			return fmt.Errorf("fact %q's value does not encode: %w", fact.Key, err)
		}
	}
	return nil
}

// registry is the presets Resolve consults. Slice 2 registers only generic;
// slice 5's factory widens this to wails, nextjs, expo.
var registry = []Preset{generic{}}

// Resolve returns every preset that matches p, Root scope before Source
// scope, registry order within a scope. It never returns nil: generic
// always matches.
func Resolve(p project.Project) []Match {
	return resolve(p, registry)
}

// resolve is Resolve over an explicit registry, split out for the same
// reason applySteps is split out of Apply: the ordering and scope-guard
// rule is tested against stub presets without depending on the real ones.
func resolve(p project.Project, presets []Preset) []Match {
	var rootMatches, sourceMatches []Match
	for _, preset := range presets {
		if preset.Scope() == Source && !p.HasSource() {
			continue
		}
		match, ok := preset.Detect(p)
		if !ok {
			continue
		}
		if preset.Scope() == Root {
			rootMatches = append(rootMatches, match)
		} else {
			sourceMatches = append(sourceMatches, match)
		}
	}
	return append(rootMatches, sourceMatches...)
}

// Keys enumerates the contributed key names across matches, in first-seen
// (Root-then-Source, registry) order. The collision step (slice 3) and the
// region renderer both need the key list without re-deriving it from every
// manifest's facts.
func Keys(matches []Match) []string {
	var keys []string
	seen := map[string]bool{}
	for _, match := range matches {
		for _, fact := range match.Manifest.Facts {
			if !seen[fact.Key] {
				seen[fact.Key] = true
				keys = append(keys, fact.Key)
			}
		}
	}
	return keys
}
