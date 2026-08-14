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
	"regexp"
	"strings"

	"github.com/Disble/dharness/internal/project"
)

// Schema versions the manifest shape a preset contributes. It is checked by
// Manifest.Validate, not by a run: a schema mismatch is an authoring bug,
// caught by the registry test that walks every preset before it ships.
//
// v2 bumped it. v1's Layer could express exactly one form — a default export
// spread into the config array — and that turned out to be the minority of
// what the frameworks document about themselves: eslint-config-expo/flat is
// a single config object included directly, and eslint-plugin-react-doctor's
// presets are read off a `configs` property. Layer gained Accessor and
// Spread rather than the presets rendering their own JavaScript, which would
// have moved code generation into the package that holds no writer.
const Schema = "dharness.preset/v2"

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

// Layer is a config layer one framework publishes about itself: a package to
// install, and the binding .dharness/eslint.config.js receives it under. It
// is the third contribution kind because it is neither a fallow config key
// (Fact) nor prompt text (Seed) — it is a dependency plus a name in
// generated code.
type Layer struct {
	// Package is the package to install, unpinned. dharness installs what
	// the framework publishes and versions; a version here would be
	// dharness inventing a convention.
	Package string

	// Binding is the identifier the package is imported under, in both the
	// project's import region and the owned factory's parameter list. It is
	// written into code dharness generates, so an invalid identifier
	// produces a config that does not parse.
	//
	// It is namespaced — "dharnessNext", never "next" — and that is a
	// correctness rule rather than a naming convention. dharness writes its
	// import into a file the project also writes imports into, and two
	// import declarations binding one identifier in an ES module are a
	// SyntaxError.
	//
	// Two layers may share a Binding only when they share a Package: one
	// module, one import declaration, several presets read off it. Binding
	// is therefore a function of Package, not a unique key.
	Binding string

	// Accessor is the property path the config sits at inside the imported
	// module, or nil when the module's own default export is the config.
	// {"configs", "next"} renders as `.configs.next`; a segment that is not
	// a JavaScript identifier is subscripted instead, so {"configs",
	// "react-native"} renders as `.configs["react-native"]` — the spelling
	// react-doctor's own documentation uses.
	//
	// It is a path rather than a string of JavaScript because this package
	// holds no JS syntax: the segments are names, and internal/setup decides
	// how to spell them.
	Accessor []string

	// Spread reports whether the value is a config array, which flat config
	// requires spread into the exported array, or a single config object,
	// which is included as-is.
	//
	// There is no safe default. eslint-config-next/core-web-vitals is an
	// array and eslint-config-expo/flat is one object; each framework
	// documents its own answer and the preset carries it rather than the
	// renderer guessing from a shape it cannot see at build time.
	Spread bool

	// Because names the observable, exactly as Fact.Because does.
	Because string
}

// InstallName is the package to install for this layer: Package with any
// subpath removed. The two diverge because the frameworks publish their flat
// configs behind subpath exports — "eslint-config-next/core-web-vitals" is
// what ESLint imports and "eslint-config-next" is what npm installs — and a
// preset that carried both as separate fields could state them inconsistently.
//
// The split follows npm's own package-name rule rather than one dharness
// invented (§09): a scoped name keeps two segments, an unscoped name keeps
// one, and everything after that is the subpath.
func (l Layer) InstallName() string {
	segments := strings.Split(l.Package, "/")
	if strings.HasPrefix(l.Package, "@") {
		if len(segments) < 2 {
			return l.Package
		}
		return segments[0] + "/" + segments[1]
	}
	return segments[0]
}

// Manifest is an ordered set of facts, seeds and layers. A slice, not a map:
// Go's map iteration order is randomised, and the region rendered into
// .dharness/fallow.jsonc must be byte-stable across runs or every sync
// produces a diff (the golden pin depends on it). The same ordering
// requirement applies to Seeds, rendered into ArchitecturePrompt, and to
// Layers, rendered into .dharness/eslint.config.js's factory signature.
type Manifest struct {
	Schema string
	Facts  []Fact
	Seeds  []Seed
	Layers []Layer
}

// boundariesKey is reserved. Zones encode intent, and no preset may ever
// speak for it — the guard that keeps the region and architectureStep's
// block from ever reading as each other (design decision 8).
const boundariesKey = "boundaries"

// dharnessBindingPrefix is what makes an emitted import binding
// collision-proof by construction (Layer.Binding's doc comment, design
// decision 7): dharness writes its import into a file the project also
// writes imports into, and two import declarations under one identifier in
// an ES module are a SyntaxError.
const dharnessBindingPrefix = "dharness"

// bindingPattern is the exact grammar an import binding and a destructured
// factory parameter both require. It is checked, not the JS reserved-word
// list — that table was cut as a ~40-entry list policing two registry
// entries dharness writes itself (design decision 7).
var bindingPattern = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

// packagePinnedVersion reports whether pkg carries a version pin — an "@"
// anywhere after position 0. "@scope/name" keeps its own leading "@" legal;
// "name@1.2.3" and "@scope/name@1.2.3" do not.
func packagePinnedVersion(pkg string) bool {
	if pkg == "" {
		return false
	}
	return strings.Contains(pkg[1:], "@")
}

// Validate reports an authoring error: an empty Because, an unencodable
// Value, a Schema other than Schema, the reserved key "boundaries", a
// pinned Layer.Package, or a Layer.Binding that is not a namespaced JS
// identifier. Called by a test that walks the whole registry, never by a
// user's run — an authoring bug is recoverable, and failing sync for it
// would block on something dharness itself broke.
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
	for _, layer := range m.Layers {
		if layer.Because == "" {
			return fmt.Errorf("layer %q carries no evidence", layer.Package)
		}
		if packagePinnedVersion(layer.Package) {
			return fmt.Errorf("layer package %q is pinned to a version; dharness installs what the framework itself publishes and versions", layer.Package)
		}
		if !bindingPattern.MatchString(layer.Binding) {
			return fmt.Errorf("layer binding %q is not a valid JavaScript identifier", layer.Binding)
		}
		if !strings.HasPrefix(layer.Binding, dharnessBindingPrefix) {
			return fmt.Errorf("layer binding %q is not namespaced to dharness — two import declarations under one identifier in an ES module is a SyntaxError", layer.Binding)
		}
		for i, segment := range layer.Accessor {
			if segment == "" {
				return fmt.Errorf("layer %q's accessor segment %d is empty; it names no property", layer.Package, i)
			}
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

// Layers enumerates every layer contributed across matches, in match order —
// the same Root-then-Source, registry order Resolve returns — so the
// generated import block and factory signature are byte-stable across runs.
func Layers(matches []Match) []Layer {
	var layers []Layer
	for _, match := range matches {
		layers = append(layers, match.Manifest.Layers...)
	}
	return layers
}
