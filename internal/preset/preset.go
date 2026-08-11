// Package preset says what is true about a project like this, and how it
// knows. internal/setup says what gets written; this package holds no file
// paths, no JSONC, and no writer (design decision 1).
//
// No preset here contributes a rule severity or a threshold. Confirmed with
// the user: the eslint plugin's rules are framework-agnostic. Five of the six
// rules are guardrails on generated code — a file-length ceiling, jsdoc
// requirements, barrel purity — and nothing about a three-thousand-line file
// changes because the framework around it is Next.js instead of Wails. The
// sixth, folder-ownership, already left this rung entirely: it is derived
// from whether the tree publishes barrels (DefaultSeverity,
// internal/setup/plugin.go), which is observable from git rather than
// assumed from whichever preset happened to match. A future preset that
// wants a rule or a threshold to vary needs a measured case for it, the same
// discipline CLAUDE.md already applies everywhere else in this repository —
// not an assumption that a framework implies a coding convention.
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

	// Uncertain names what this preset could not read, and is empty when it
	// read everything it needed. A preset that recognises a project but
	// cannot read its configuration still matches and still contributes its
	// documented defaults — saying "not this framework" because a file
	// would not parse is the silent no-op this change exists to end.
	Uncertain string
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

// Seed is a structural fact a preset's own framework documents about itself
// — not a decision. ArchitecturePrompt renders it as "this is what the
// framework documents; confirm or correct it against the tree", never as
// "these are your zones": §21 keeps zones with the agent, read from the code
// and the person who wrote it, not detected. A preset that decided zones
// instead would be doing architectureStep's job, which Manifest.Validate's
// reserved "boundaries" key already refuses for Facts — Seed carries no key
// at all, so it cannot collide with that guard or with anything fallow
// reads.
type Seed struct {
	// Text is the structural fact, in the framework's own documented terms.
	Text string

	// Because names the observable that justifies Text — a documentation
	// page, a quoted sentence from it — the same discipline as Fact.Because
	// and for the same reason: a convention that changes upstream must be
	// visible in the repository, not only in this binary.
	Because string
}

// Manifest is an ordered set of facts and seeds. A slice, not a map: Go's
// map iteration order is randomised, and the region rendered into
// .dharness/fallow.jsonc must be byte-stable across runs or every sync
// produces a diff (the golden pin depends on it). The same ordering
// requirement applies to Seeds, rendered into ArchitecturePrompt.
type Manifest struct {
	Schema string
	Facts  []Fact
	Seeds  []Seed
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
	for _, seed := range m.Seeds {
		if seed.Because == "" {
			return fmt.Errorf("seed %q carries no evidence", seed.Text)
		}
	}
	return nil
}

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

// Seeds enumerates every seed contributed across matches, in match order
// (the same Root-then-Source, registry order Resolve already returns) — so
// ArchitecturePrompt renders them in a stable sequence and a project's
// prompt does not reorder itself between two runs.
func Seeds(matches []Match) []Seed {
	var seeds []Seed
	for _, match := range matches {
		seeds = append(seeds, match.Manifest.Seeds...)
	}
	return seeds
}
