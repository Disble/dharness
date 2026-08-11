# Design: framework-presets

Inputs: `openspec/changes/framework-presets/proposal.md` (authoritative on
scope, including its two orchestrator corrections), `exploration.md`,
`docs/design-principles.md`, `CLAUDE.md`'s two rules,
`openspec/changes/unify-init-and-sync/design.md` (house style and the
`extendsStep` split this design leans on).

Read alongside: the reference architecture named in the exploration is an
upstream Go project that fans one command out over sixteen coding agents. Its
shape is one package-level `Adapter` per agent, a registry that selects exactly
one of them, and a per-adapter description that splits *where* files go from
*how* they are written. This design borrows the registry and the
per-implementation manifest, and rejects the where/how split — see Decision 1.
Its adapters are mutually exclusive; presets are not.

## Technical Approach

A preset is **detection plus a manifest of facts, each carrying its own
evidence**. Detection runs against a resolved `project.Project`; the registry
returns every match rather than one, ordered Root scope first. `internal/setup`
renders the matched facts into a **marked region** inside
`.dharness/fallow.jsonc` — the one file dharness owns — and rewrites only that
region, so the architecture the agent writes into the same file survives every
run. `boundariesOwnerStep` widens from the single key `boundaries` to that
region's keys plus `boundaries`, tested against the project's own config with
the quoted-key test `declaresBoundaries` already uses.

Nothing is recorded. The matches, the region's bytes and the collision set are
all re-derived on every run (§07).

Three properties fall out of that sentence and are load-bearing everywhere
below:

1. **`internal/preset` says what is true; `internal/setup` says what gets
   written.** The preset packages hold no file paths, no JSONC, and no writer.
2. **A preset never asserts that a path exists.** It contributes an ignore
   pattern that is inert where the directory is absent (§07).
3. **The verdict is bytes.** Key presence, region equality, exit codes — never
   prose, never a model (§17, `CLAUDE.md`'s second rule).

## Architecture Decisions

### Decision 1 — `Preset` is three methods, and the where/how split is rejected

**Choice.**

```go
// Preset is one framework's answer to "what is true about a project like
// this, and how do I know". It reports facts; it never writes.
type Preset interface {
    // ID names the preset in a report and in a fact's evidence.
    ID() string

    // Scope says which of the two directories this preset's signal lives in.
    // Answered before Detect so a Source-scope preset is never asked about a
    // repository with no JS project.
    Scope() Scope

    // Detect reports whether this repository is such a project, and if so
    // everything the preset contributes. Pure: it reads, it never writes.
    Detect(p project.Project) (Match, bool)
}

type Match struct {
    ID       string
    Scope    Scope
    Evidence string   // why this preset matched at all
    Manifest Manifest // what it contributes, with per-fact evidence
}
```

**Why the manifest comes out of `Detect` rather than sitting beside it as a
constant.** The Wails manifest is not knowable without reading the project:
`wails.json` carries `wailsjsdir`, and the ignore pattern is computed from it
(Decision 8). A `Manifest()` method separate from `Detect` would read
`wails.json` twice and give two chances to disagree about one file. One read,
one answer.

**Why the where/how split is rejected.** The reference architecture separates
the paths an adapter targets from the strategy that writes them, because its
sixteen agents write different formats into different trees. Here every preset
contributes into exactly one file, in exactly one format, through exactly one
writer that `internal/setup` already owns. A strategy dimension with one
strategy is a seam that carries nothing, and the moment it exists a preset gains
the ability to write — which would give `.dharness/fallow.jsonc` a second owner
and break §03. The split is not discarded, it is **relocated to the package
boundary**: `internal/preset` is the *what*, `internal/setup` is the *where and
how*, and the compiler enforces it because `internal/preset` imports no writer.

**Rejected: `Describe`/`Apply` on `Preset`.** That is `Step`'s interface, and a
preset is not a step. Presets contribute a dimension *inside* `ownedFilesStep`
(proposal decision 1); giving them the step verbs would invite an eleven-step
plan that grows one entry per framework.

**Rejected: one package per framework.** The exploration proposed
`internal/preset/wails` and friends. Each would need `Match` and `Manifest` from
`internal/preset`, and `internal/preset`'s registry would need each of them —
an import cycle. The fix is one package, **one file per framework**
(`wails.go`, `nextjs.go`, `expo.go`, `generic.go`), which keeps the auditability
property the exploration actually wanted and is the shape `internal/tool`
already uses for CLI knowledge.

### Decision 2 — The manifest is an ordered slice of facts, not a struct and not a map

**Choice.**

```go
const Schema = "dharness.preset/v1"

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

type Manifest struct {
    Schema string
    Facts  []Fact
}
```

**Why not a typed struct with one Go field per fallow key.** The keys are
fallow's, not dharness's. A struct means a Go field plus a json tag per key,
and — worse — the collision step needs to *enumerate the contributed key names*,
which a struct can only answer through reflection or a hand-maintained parallel
list that will drift. Enumeration is the whole mechanism of proposal decision 4.

**Why an ordered slice and not `map[string]Fact`.** Go map iteration order is
randomised. The region rendered into `.dharness/fallow.jsonc` must be
byte-stable across runs or every `sync` produces a diff and the golden pin
(Decision 7) is worthless. Declaration order is also the order a reader wants:
the author's order.

**What makes an evidence string valid, and where that is enforced.** Three
rules: non-empty; it names an observable (a filename, a key, a documented
default) rather than restating the value; and it never contains the quoted key
`"boundaries"` — see Decision 8's guard. Enforcement is **`Manifest.Validate()
error`, called by a test that walks the whole registry against every fixture,
never by the user's run.** A missing evidence string is a dharness authoring
bug; failing a user's `sync` for it would block on something recoverable (§20).
`Validate` also attempts `json.Marshal` on every `Value`, which is how an
unencodable value is caught at build time rather than at write time.

### Decision 3 — Composition unions lists; a scalar collision between two presets is a build error, not a runtime resolution

**What the registry returns.**

```go
// Resolve returns every preset that matches, Root scope before Source scope,
// registry order within a scope. It never returns nil: generic always matches.
func Resolve(p project.Project) []Match

// resolve is Resolve over an explicit registry, split out for the same reason
// applySteps is split out of Apply: the composition rule can be tested against
// stub presets without depending on the real four.
func resolve(p project.Project, presets []Preset) []Match
```

A Source-scope preset is skipped outright when `!p.HasSource()` — one guard in
`resolve`, not four copies inside four `Detect` methods.

**Two presets contributing the same key — the case the proposal left open.**
Precedence *between* rungs is settled (project > detection > preset > global).
Within the preset rung:

| Value kind | Rule |
|---|---|
| List | **Union**, in resolve order, duplicates removed by the existing `dedupe`. Each contributed element keeps the evidence of the preset that contributed it |
| Scalar (string, number, bool, object) | **Root scope wins.** The losing value and both evidence strings are rendered as a comment beside the winner |

**Why union is right and not a compromise.** Every key the four presets touch is
a list: `ignorePatterns` and `ignoreDependencies` — `entryPoints` was struck by
the orchestrator, see Open Questions. A Wails root with
a Next.js source wants `wailsjs/**` *and* `.next/**`; picking one would be the
silent no-op this whole change exists to end. Union is the meaning of the key,
not a merge policy invented on top of it.

**Why a scalar collision is a comment and never a prompt.** The
project-versus-preset collision is the *project's* to resolve, so it is
delegated (§21) and gets a prompt. A preset-versus-preset collision is
**dharness's own authoring problem** — the user created nothing and can resolve
nothing — and handing it to them would be work returned to the user (§18). It
is written down where an auditor sees it, and a registry test
(`TestNoScalarKeyIsContributedTwice`) asserts the branch is unreachable across
the four presets shipped. The runtime rule exists so a future fifth preset
degrades visibly instead of randomly; the test is what keeps it from being
routine.

### Decision 4 — Nothing is added to `project.Project`, and the import direction is forced

**Choice.** `internal/preset` imports `internal/project`. `internal/project`
imports `internal/preset` **never**. There is no `Preset` field on `Project` and
no resolved value threaded through `setup.Step`. `internal/setup` calls
`preset.Resolve(p)` at each call site that needs it.

**What decides it is not taste.** `Detect(p project.Project)` means
`internal/preset` depends on `internal/project`. A `Presets []Match` field on
`Project` would make `internal/project` depend on `internal/preset`. That is an
import cycle; Go refuses to build it. The only way to have the field is to move
`Match` and `Manifest` into `internal/project`, which would put framework
knowledge in the package whose doc comment says detection is *"deliberately
shallow: lockfile names, package.json dependencies"*.

**Why re-deriving at each call site is not a cost worth avoiding.** `Resolve`
reads at most two small files (`wails.json`, `package.json`). Every `Satisfied`
in `internal/setup` already stats or reads files, several of them per run. A
memoised value would be recorded state, which §07 exists to refuse, and it would
have to be invalidated by nothing in particular.

**Rejected: a second parameter on every `Step` method.** Eleven steps and five
methods each, changed so three call sites can avoid two file reads.

**Call sites (three):** `ownedFilesStep.Satisfied` and `.Apply` (the region),
`boundariesOwnerStep.Satisfied`/`.Describe`/`.Delegated` (the candidate keys).
`DefaultSeverity` is *not* one of them — Decision 6 is detection-derived, and
keeping it out of the preset rung is the whole point of proposal decision 2.

### Decision 5 — `declaredKeys(path, candidates)`

**Choice.** `declaresBoundaries` is replaced, not wrapped:

```go
// declaredKeys reports which of candidates the config at path declares.
//
// It tests the quoted key rather than the bare word, for the reason
// declaresBoundaries did: these files are JSONC, and a config correctly
// pointing at dharness carries sentences like "Architecture boundaries live in
// the file dharness owns" in a comment. A bare substring answers yes to that
// sentence and would make every wired project report a conflict it does not
// have. Verified on the motivating repository: `"boundaries"` matches zero
// times there while the bare word matches once.
//
// A file that cannot be read declares nothing. Results come back in candidate
// order so the prompt reads the same way twice.
func declaredKeys(path string, candidates []string) []string
```

Body is the same shape as before: read once, `strings.Contains(raw, `"`+key+`"`)`
per candidate, collect. No parser — the product is stdlib-only, and
`encoding/json` is not an option here at all: `.fallowrc.json` is JSONC, the
motivating repository's copy opens with a comment on line 2, and
`json.Unmarshal` returns `Expecting property name enclosed in double quotes:
line 2 column 3` on it. This is the third instance of an established textual
test, after `declaresBoundaries` and `extendsWired`.

**Which files it is asked about.** `.fallowrc.json` **and `.fallowrc.jsonc`** —
both are in fallow's own `fallowConfigFiles` list and both spell keys with
quotes. `fallow.toml` is excluded deliberately: TOML keys are bare, so the
quoted test answers "declares nothing" for a file that may declare everything.
Where the config is TOML, the step describes and continues, following the
precedent already set for `doctor.config.ts`. Today's code hard-codes
`.fallowrc.json`; adding the `.jsonc` spelling is a correctness fix that comes
free with the generalisation.

**The two accepted failure modes, stated rather than discovered later.**

| Mode | Consequence | Why accepted |
|---|---|---|
| A comment that quotes the key | False positive: a collision reported that does not exist | The existing comment already names it: a file written to defeat the check rather than an accident. The step is delegated and the agent can see the file |
| The key nested inside another object | False positive | fallow's keys are top-level; a nested key of the same name is itself worth looking at |
| A config written as TOML or as code | False negative: no collision reported | The keys cannot be read at all without becoming an interpreter for someone else's configuration |

Neither false positive can lose data: the step is delegated with two valid
resolutions and nothing is written on its behalf (§20).

### Decision 6 — `DefaultSeverity(p, rule)` and the barrel probe

**Choice.** `offByDefault` is deleted as a package-level map. The comment block
above it is rewritten, not dropped: the measurement stays (eight non-actionable
findings on one repository), the conclusion changes from *therefore off
everywhere* to *therefore off where the tree has no barrels*.

```go
// internal/setup
func DefaultSeverity(p project.Project, rule string) string {
    switch strings.TrimPrefix(rule, RulesPrefix+"/") {
    case "folder-ownership":
        if p.PublishesBarrels() {
            return "error"
        }
        return "off"
    }
    return "error"
}

// internal/project — asked of git, like Discover, rather than by walking.
//
// A method rather than a Project field: the probe costs a subprocess and one
// command in five needs the answer. Discover runs for check and mutate too,
// and neither asks about barrels (§12, §13).
func (p Project) PublishesBarrels() bool
```

**The exact invocation:**

```
git -C <p.Source> ls-files -z -- "*/index.ts" "*/index.tsx"
```

run through the existing `gitOutput` seam. Three things that recipe buys, all of
them the reason `Discover` asks git rather than walking:

- **`node_modules` and every generated tree are excluded for free**, because
  git's index already excludes what `.gitignore` excludes. A walk would need a
  skip list and a depth bound, and would count `frontend/wailsjs/…/index.ts`.
- **Running in `p.Source` scopes the pathspec to the JS project** without
  computing a prefix. A Wails repository's Go half cannot answer for the
  frontend.
- **The seam already exists.** `project.SetGitOutputForTest` makes this testable
  with no repository, which is the whole of Decision 9 for this feature.

**The leading `*/` is deliberate.** A single `index.ts` at the source root is a
package entry point, not evidence of barrel publishing; `*/index.ts` requires at
least one directory component. One match is enough — a count threshold would be
an invented heuristic where a direct signal exists (§09), and it is the exact
mistake this repository already made and recorded when it reached for a
dead-code percentage.

**When git cannot answer** — `!p.InRepository`, `!p.HasSource()`, or a non-nil
error from `gitOutput` — the answer is `false`, which is today's behaviour and
today's severity. A failed probe never argues a rule *on*. Note the deliberate
consequence: `ls-files` reads the index, so a barrel that exists on disk and has
never been staged does not count. That is right — an unstaged file is not yet
this project's published architecture — and it is recorded here so it is not
rediscovered as a bug.

**The first-write limit is inherited, not introduced.** `doctorConfigStep.
Satisfied` returns true as soon as `RulesPackage` appears in `plugins`, so
severities are written once, at adoption. A project that adds barrels later does
not get the rule switched on by a later `sync`. Widening `Satisfied` to re-check
severities stays rejected: dharness cannot tell "the project chose `error`" from
"dharness wrote `error`", and re-deriving would overwrite a deliberate choice
(§05). Derived, first-write, stated.

### Decision 7 — The golden pin: what is snapshotted, and how a legitimate change is told from a regression

**What is snapshotted.** Not `Plan()`'s IDs — a pin over the ID list would let
every change to what a step *writes* through untouched. One text file per
fixture, holding two regions:

```
== plan ==
1  install what this project is missing
   satisfied=false delegated=false
   describe |
     <Describe(p), verbatim, indented>
2  write the files dharness owns
   satisfied=false delegated=false
   ...

== tree ==
.dharness/fallow.jsonc
---
{
  // dharness writes this file; ...
}
---
.dharness/rules.json
---
{ ... }
---
```

- **Plan region:** every step in `Plan()` order, with `ID()`, `Satisfied(p)`,
  `Delegated(p)`'s `ok` and `why`, and `Describe(p)` verbatim. That is the whole
  report surface — `internal/cli` renders nothing the plan region does not
  contain.
- **Tree region:** every file existing under the fixture root after
  `setup.Apply`, paths sorted, `filepath.ToSlash`, contents verbatim between
  `---` fences.

**Format rules, because the pin is worthless if it is flaky.** Paths relative to
the fixture root and slash-separated; LF newlines written explicitly, never the
platform's; the fixture root and source substituted as `<root>`/`<source>`
wherever they appear in text; `runner.Run` stubbed so no install runs;
`gitOutput` stubbed so the barrel probe and `Discover` answer from the fixture.
Deterministic ordering everywhere — which is why Decision 2 chose a slice over a
map.

**Fixtures.** Slice 1 creates `generic-conventional` (Root == Source, no
framework) and `generic-split` (Root != Source, the Wails-shaped layout with no
`wails.json`). Slice 5 adds `wails`, `nextjs`, `wails-nextjs`.

**Telling a legitimate change from a regression — the mechanism, not a
convention.** Two categories with two different rules, and the difference is
enforced by having two tests rather than by asking anyone to be careful:

| Category | Test | Update path |
|---|---|---|
| `generic-*` | `TestGenericGoldenIsUnchanged` — plain comparison against the checked-in file | **None.** No `-update` flag reaches these files. Changing one means editing it by hand |
| framework fixtures | `TestFrameworkGoldens` | `go test ./internal/setup -run TestFrameworkGoldens -update` regenerates |

The generic goldens are the frozen baseline for this whole change: any diff in
them during slices 2–5 is a regression by definition, and the absence of an
`-update` path is what makes accepting one a deliberate, reviewable act rather
than a re-run. The framework goldens are living: their diffs are expected, and
the commit that produces one states which manifest fact changed and why.

### Decision 8 — The managed region, and why the owned file is not rewritten whole

**The problem, which the proposal does not surface.** `ownedFilesStep.Apply`
writes `.dharness/fallow.jsonc` wholesale, and `Satisfied` is true as soon as
the three owned files exist. Both facts have to change for preset content to
land — and the moment `Apply` can run a second time, it overwrites the
`boundaries` block **the agent writes into that same file**
(`architectureStep.Satisfied` is exactly "the owned fallow file contains
`boundaries`"). The file dharness owns is co-owned in practice, and rewriting it
would destroy the one thing in it that no detection can reproduce.

**Choice: dharness rewrites a delimited region and nothing else.**

```jsonc
{
  // dharness:presets begin — rewritten by `dharness sync`; edits here are lost.
  // wails: wails.json declares no "wailsjsdir", and Wails documents frontend/
  // as its default, so its generated bindings land in frontend/wailsjs.
  "ignorePatterns": ["wailsjs/**"],
  // dharness:presets end

  "boundaries": { ... }        // written by the agent; never touched
}
```

- `Satisfied` gains: the region's bytes equal what the current matches render.
  Byte equality of the **region only**, so drift converges and nothing else in
  the file is compared.
- `Apply` writes `lefthook.yml` and `rules.json` wholesale as it does today —
  dharness owns those with no co-author. For `fallow.jsonc`: absent, write the
  existing skeleton with the region inside it; present, replace the bytes
  between the two markers and leave every other byte alone. Markers gone,
  insert the region immediately after the first `{`, which is a byte dharness
  itself wrote.
- **When the matched facts are empty, no region is written at all.** `generic`
  contributes nothing, so a non-framework project's file is byte-for-byte
  today's skeleton and the plan is byte-for-byte today's plan. That is the
  success criterion, and it is satisfied structurally rather than by care.

**Why this and not the three alternatives.**

| Alternative | Rejected because |
|---|---|
| Rewrite the file whole | Destroys the agent's `boundaries` on every manifest change. The one thing in the file detection cannot rebuild |
| A second owned file (`fallow.presets.jsonc`) plus a two-entry `extends` | Needs a new committed file named in `dirIgnore`, changes the `extends` line every existing adopter already wrote, and rests on **unmeasured** fallow behaviour: how two parents compose is not the single-parent replacement this change measured |
| Delegate the merge into dharness's own file to the agent | It is not a merge with anyone: dharness wrote both halves of that file's structure. Delegating would also give `ownedFilesStep` two recipients, which is the exact shape `unify-init-and-sync` split `extendsStep` to avoid — and splitting here would change `Plan()`'s output for a generic project, breaking the byte-identity criterion |

Region replacement between two literal markers parses nothing. It is the same
class of textual operation as `appendHuskyGate`, applied to a file dharness owns
outright rather than one it borrows.

**Two guards, both testable.** `Manifest.Validate` rejects the key
`boundaries` — no preset may ever contribute it (§21: zones encode intent) — and
a test asserts `architectureStep.Satisfied` stays false on a Wails fixture whose
agent block has not been written. Together they keep the region and the
architecture from ever reading as each other.

**Plan stays eleven steps.** Proposal decision 1 holds, and it holds by
construction rather than by restraint.

### Decision 9 — The Wails manifest, worked, because it is the one that computes

The other three presets are a dependency test and a literal; Wails is the one
whose facts are read from the project, so its shape is fixed here and slice 5
fills the rest in.

```go
// Detect: p.Root/wails.json exists.
// wailsjsdir: read with encoding/json — wails.json is plain JSON written by
// Wails' own tooling, not JSONC. On any read or decode failure, fall back to
// the documented default and say so in the evidence. A malformed wails.json is
// the project's problem, never a failed sync.
```

The generated directory is `<wailsjsdir>/wailsjs`, and fallow resolves
`ignorePatterns` relative to the config that declares them — which lives in
`p.Source`. So the contributed value is
`filepath.Rel(p.Source, filepath.Join(p.Root, wailsJSDir, "wailsjs")) + "/**"`,
slash-separated. In the motivating repository, where `wailsjsdir` is absent and
`Source` is `frontend/`, that computes exactly `wailsjs/**` — the pattern that
repository wrote by hand, which is the check that this is the right derivation.

Evidence names the key **and** the fallback, per the proposal's third
consequence:

- key present: ``wails.json sets "wailsjsdir": "./frontend/src/lib", and Wails
  generates its bindings under <wailsjsdir>/wailsjs``
- key absent: ``wails.json declares no "wailsjsdir"; Wails defaults it to
  frontend/, so the bindings land in frontend/wailsjs``

The proposal's risk row *"Wails' output path assumption turns out wrong —
Unknown, unverified"* is **struck**: a direct signal exists and the preset reads
it (§09).

Signals for the other three, fixed here and populated in slice 5: `nextjs` —
Source scope, `next` in `package.json` dependencies; `expo` — Source scope,
`expo` in dependencies; `generic` — Root scope, always matches, evidence *"no
framework signal matched"*, **manifest empty and required by test to stay
empty**. `generic` is not filler and it is not free of obligations: it is what
makes "today's behaviour" one preset among several, and its empty manifest is
what makes the byte-identity criterion provable.

## Data Flow

    project.Discover(dir) ──→ Project{Root, Source, …}
              │
              ├──→ preset.Resolve(p) ──→ registry: wails, nextjs, expo, generic
              │         │                   each: Scope() then Detect(p)
              │         │                   Source-scope skipped if !HasSource()
              │         ↓
              │      []Match  (Root scope first, registry order within)
              │         │
              │         ├──→ setup.presetRegion(matches) ──→ rendered JSONC
              │         │       └─ union lists, scalar: Root wins + comment
              │         │       └─ empty matches → empty region → today's bytes
              │         │
              │         └──→ preset.Keys(matches) ──→ contributed key names
              │                          │
              │                          ↓
              │        boundariesOwnerStep: declaredKeys(
              │            p.Source/.fallowrc.json{,c},
              │            {"boundaries"} ∪ keys)
              │                          │
              │            empty ──→ satisfied, nothing printed (§15)
              │            non-empty ──→ delegated, one prompt naming each
              │                          key and both values (§20, §21)
              │
              └──→ p.PublishesBarrels() ──→ git ls-files in Source
                        └──→ setup.DefaultSeverity(p, rule) → "error" | "off"

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/preset/preset.go` | New | `Preset`, `Match`, `Manifest`, `Fact`, `Scope`, `Schema`, `Validate`, `Resolve`/`resolve`, `Keys` |
| `internal/preset/generic.go` | New | Always matches, empty manifest (slice 2) |
| `internal/preset/wails.go` | New | Root scope, `wails.json`, `wailsjsdir` (slice 5) |
| `internal/preset/nextjs.go` | New | Source scope, `next` dependency (slice 5) |
| `internal/preset/expo.go` | New | Source scope, `expo` dependency (slice 5) |
| `internal/preset/preset_test.go` | New | Composition against stub presets; `Validate` over the real registry |
| `internal/project/git.go` | Modify | `PublishesBarrels` and its recipe, beside the other git questions |
| `internal/setup/plugin.go` | Modify | `offByDefault` deleted; `DefaultSeverity(p, rule)`; the rationale comment rewritten, not removed |
| `internal/setup/files.go` | Modify | `declaresBoundaries` → `declaredKeys(path, candidates)`; the `.fallowrc.jsonc` spelling joins it |
| `internal/setup/owned.go` | New | Region markers, `presetRegion(matches)`, `replaceRegion(existing, region)` |
| `internal/setup/steps.go` | Modify | `ownedFilesStep.Satisfied`/`.Apply` (Decision 8); `boundariesOwnerStep` generalised; the `DefaultSeverity` call site gains `p` |
| `internal/setup/golden_test.go` | New | Decision 7: the two tests and the renderer |
| `internal/setup/testdata/golden/*.txt` | New | `generic-conventional`, `generic-split`, then the framework fixtures |
| `internal/setup/setup_test.go` | Modify | Severity tests gain a project; new contract tests below |
| `docs/design-principles.md` | Modify | Only if a case here produces a principle; none is proposed |
| `docs/learning-log.md` | Modify | One dated line for the JSONC/`json.Unmarshal` measurement and one for the region decision |

`internal/project/evidence.go`'s `dirIgnore` is **not** touched: every byte this
change writes lands inside `.dharness/fallow.jsonc`, which the allow list
already names. The proposal's risk row about a silently gitignored owned file
stays inert, and it stays on the list because Decision 8's rejected alternative
is the thing that would have triggered it.

## Interfaces / Contracts

```go
// internal/preset
type Scope int
const (
    Root Scope = iota // signal lives in project.Project.Root
    Source            // signal lives in project.Project.Source
)

const Schema = "dharness.preset/v1"

type Preset interface {
    ID() string
    Scope() Scope
    Detect(p project.Project) (Match, bool)
}

type Match struct {
    ID       string
    Scope    Scope
    Evidence string
    Manifest Manifest
}

type Fact struct {
    Key     string
    Value   any
    Because string
}

type Manifest struct {
    Schema string
    Facts  []Fact
}

// Validate reports an authoring error: an empty Because, an unencodable
// Value, a Schema other than dharness.preset/v1, or the reserved key
// "boundaries". Called by tests over the registry, never by a run.
func (m Manifest) Validate() error

func Resolve(p project.Project) []Match
func Keys(matches []Match) []string

// internal/project
func (p Project) PublishesBarrels() bool

// internal/setup
func DefaultSeverity(p project.Project, rule string) string
func declaredKeys(path string, candidates []string) []string
func presetRegion(matches []preset.Match) string
```

## Testing Strategy

**Two existing seams carry all of it; this change invents none.**

1. **`project.SetGitOutputForTest`** answers `rev-parse --show-toplevel`,
   `ls-files` for lockfiles, and now `ls-files -- */index.ts */index.tsx`. Every
   barrel case — barrels present, absent, git errors, not in a repository, no
   source — is a table of stub responses. No `git init` anywhere.
2. **`runner.Run`'s package variable** keeps `installStep` from shelling out
   under the golden test, which applies the whole plan.

**Preset detection uses the real filesystem, deliberately.** `git.go`'s comment
states the repository's policy: *"git is seamed and the filesystem is not: a
temp directory is a real tree."* Detection reads `wails.json` and
`package.json`; a fixture writes them into `t.TempDir()`. **Rejected: an
`fs.FS` parameter on `Detect`** — it would seam the one thing the project has
decided not to seam, and it would put a filesystem abstraction in the package
whose whole claim is that it holds no I/O policy.

**Composition is tested against stub presets, not the real four.** `resolve` is
split out of `Resolve` for exactly the reason `applySteps` is split out of
`Apply`: the rule under test is the ordering and union rule, and binding it to
whatever `wails.go` currently contributes would make an unrelated manifest edit
fail an unrelated test.

New tests, one per contract this design asserts:

| Test | Pins |
|---|---|
| `TestGenericGoldenIsUnchanged` | Decision 7's frozen baseline; no `-update` path |
| `TestFrameworkGoldens` | The living fixtures |
| `TestEveryFactCarriesEvidence` | Decision 2's validity rule over the whole registry |
| `TestNoPresetContributesBoundaries` | Decision 8's reserved key |
| `TestNoScalarKeyIsContributedTwice` | Decision 3's unreachable branch stays unreachable |
| `TestListKeysUnionAcrossScopes` | Wails + Next.js both reach `ignorePatterns`, in Root-then-Source order |
| `TestSourceScopePresetsAreSkippedWithoutASource` | `resolve`'s single guard |
| `TestRegionIsAbsentWhenNothingIsContributed` | The byte-identity claim for `generic` |
| `TestApplyPreservesBoundariesOutsideTheRegion` | Decision 8's whole reason to exist |
| `TestRegionIsReinsertedWhenTheMarkersAreRemoved` | §15: the step reappears |
| `TestDeclaredKeysIgnoresACommentedKey` | Decision 5's quoted-key test, on a JSONC fixture that mentions the bare word |
| `TestCollisionNamesEveryContributedKeyTheProjectDeclares` | Proposal decision 4's response |
| `TestFolderOwnershipIsErrorWhereBarrelsExist` / `…IsOffWithout` | Decision 6, both directions |
| `TestBarrelProbeAnswersOffWhenGitFails` | The failed-probe default |

**Mutation coverage, per `tools/mutationstaged`.** Two branches are easy to
leave unkilled and must be asserted positively: `resolve`'s Source-scope guard
(assert the *absence* of the Source match, not merely a shorter slice), and
`PublishesBarrels`'s `*/index.ts` prefix (a fixture with a root-level `index.ts`
and nothing else must answer `false`, or dropping the `*/` survives).

**Platform.** Every path assertion goes through `filepath.Join`; every rendered
path through `filepath.ToSlash`. The golden renderer writes LF explicitly.

## Threat Matrix

| Boundary | Applicability | Design response | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | **N/A** — nothing here classifies a file as executable or not |
| Git repository selection | **Applicable** — the barrel probe runs in `p.Source`, and `Root != Source` changes both its scope and the computed Wails pattern | Probe runs in `Source`; returns `false` when `!InRepository`; Wails pattern computed with `filepath.Rel(p.Source, …)` | `TestBarrelProbeAnswersOffWhenGitFails`; a split-layout Wails fixture whose pattern is `wailsjs/**`, not `frontend/wailsjs/**` |
| Commit state | **Applicable, newly** — `sync` reads the index for the first time, through `ls-files` | Deliberate and recorded: an unstaged barrel does not count, because it is not yet the project's published architecture | A fixture whose `ls-files` stub returns nothing while the tree holds an `index.ts` answers `off` |
| Push state | **N/A** — no remote interaction |
| PR commands | **N/A** — no VCS automation |
| Subprocess execution | **Applicable** — `sync` gains one `git` invocation per run | One call, in one method, behind the existing seam; never reached when `!InRepository` or `!HasSource()` | The golden test stubs `gitOutput` and `runner.Run`; a test asserts no probe runs outside a repository |
| Destructive write to a co-owned file | **Applicable, and the reason Decision 8 exists** — the agent's `boundaries` shares a file with dharness's region | Only the bytes between two markers are replaced; `Writer` snapshots the file for undo either way | `TestApplyPreservesBoundariesOutsideTheRegion` |

## Line Forecast

| Source | Lines |
|---|---|
| Proposal's slice estimates | ~1,050–1,550 |
| Decision 8 — region render, replace, reinsert, plus its four tests | +~110 |
| Decision 7 — the golden renderer is a real component, not an assertion | +~70 |
| Decision 5 — the `.fallowrc.jsonc` spelling and its test | +~15 |

Realistic total **~1,250–1,750 changed lines** against a 400-line budget, so the
proposal's five slices hold and chaining is certain. One ordering change:
Decision 8's region machinery belongs to **slice 2**, not slice 5 — it is what
makes `generic`'s empty contribution provably byte-identical, and writing it
alongside the first real preset would mean the golden never sees the
no-region path in isolation. Slice 2's estimate rises to ~350–450 and is the
first candidate to split (region machinery, then registry).

Slice 4 (`DefaultSeverity` and the barrel probe) touches `internal/project` and
`internal/setup/plugin.go` and nothing else in this change does. It remains
independently mergeable and can move ahead of slice 3 without reordering
anything.

## Open Questions

- [x] **`entryPoints` is struck. Closed by the orchestrator, measured.** The
      design correctly refused to assert which keys are dharness's to contribute
      and asked for the first-rule check before slice 5. The check was run:
      `fallow dead-code -f json` on a real project reports
      `"entry_points":{"total":188,"sources":{"manual_entry":3,"package.json":7,"plugin":178}}`
      — 178 of 188 discovered by fallow's own framework plugins, of which it
      ships 123. `CLAUDE.md` line 13 already records this same lesson from a
      previous occurrence: "The configuration check turned out to be
      `entry_points`, which fallow already reports." No preset contributes
      `entry`. The manifest shape is unaffected; only the key list is.
- [ ] The `dharness:presets` marker text is fixed in slice 2 and appears in
      every adopting repository from then on. Changing it later means an
      orphaned region in every repository that ran an older binary. Settle the
      exact spelling in slice 2's review, not afterwards.
- [ ] `Fact.Value any` is validated by a test, not by the type system. If a
      fifth preset ever wants a nested object value, re-check whether
      `json.Marshal`'s map-key ordering still gives the golden a stable byte
      sequence for it.
