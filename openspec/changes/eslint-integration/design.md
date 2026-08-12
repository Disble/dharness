# Design: eslint-integration

Inputs: `openspec/changes/eslint-integration/proposal.md` (authoritative on
scope, including the orchestrator correction inside "The refusals"),
`spec.md` (four capabilities, every requirement),
`exploration.md` (current-state citations and "The parser, settled by the
orchestrator"), `docs/design-principles.md`, `CLAUDE.md`'s two rules,
`openspec/changes/framework-presets/design.md` (house style, and the
`internal/preset` / `internal/setup` boundary this design copies wholesale).

Read alongside: `internal/setup/owned.go`, which already ships the
delimited-region technique for a file dharness owns. This design borrows its
grammar and rejects its failure policy — see Decision 4.

**Three findings correct the proposal.** Each is load-bearing, each is
argued from code in this repository rather than from preference, and none of
them is a re-opening of a settled decision:

1. `.dharness/eslint.config.js` would be **gitignored** in every repository,
   and permanently gitignored in every already-adopted one
   (`internal/project/evidence.go:22-31`, `EnsureDir`'s write-if-absent at
   `evidence.go:81-86`). Decision 2.
2. A `.dharness/eslint.config.js` that imports `dharness-eslint-plugin` by
   bare specifier **cannot resolve it in a split layout**, which is the
   motivating repository's own shape. Decision 3.
3. `doctorConfigStep` cannot survive losing `RulesPackage`: that string is
   its entire `Satisfied` probe (`steps.go:458-476`), so the step would
   become permanently unsatisfiable — a step with no reachable satisfied
   state, which §15 refuses and which `UncheckableConfigNote` already
   refused once in this codebase. Decision 8.

## Technical Approach

`internal/jsconfig` answers one question about a JavaScript source: **where
does dharness's layer belong in this file, or why does it belong nowhere.**
It returns byte offsets and a refusal string. It holds no dharness spelling,
no marker text, no framework names, and no writer. Its answer has the exact
shape `Step.Delegated` already has — `(value, why string, ok bool)` — so
`eslintExtendsStep.Delegated` is a translation rather than a judgement.

`internal/setup` owns everything that is a dharness spelling: the two marker
pairs, the rendered region text, the file dharness writes, and the decision
to write at all. The compiler enforces the split: `internal/jsconfig`
imports neither `internal/setup` nor `internal/preset`, and only
`internal/setup` imports `internal/jsconfig`. That single-importer property
is not tidiness — it is what makes the cgo fallback recorded in the proposal
a one-package rewrite instead of a change to the product.

Four properties fall out and are load-bearing everywhere below:

1. **The verdict is bytes and offsets.** A refusal is a named string, a
   splice is `src[:at] + region + src[at:]`, and the guard is a re-parse of
   the candidate bytes. Never prose, never a model (§11, §17, `CLAUDE.md`'s
   second rule).
2. **Nothing is recorded.** Every run re-parses and re-derives whether the
   two regions are present and current. A hand-removed splice reappears
   (§07, §15).
3. **Each file goes where the thing that resolves it lives.** `.fallowrc.json`
   is written to `p.Source` and `lefthook.yml` to `p.Root` for that reason
   already (`files.go:197-216`). Decision 3 applies the same rule to a bare
   module specifier, which is the one thing `.dharness/` cannot resolve.
4. **The write happens last.** The re-parse guard is a *precondition of the
   write*, not a postcondition of it, so the broken file never exists on
   disk at all. Decision 6.

## Architecture Decisions

### Decision 1 — `internal/jsconfig` returns an anchor and a refusal, in `Delegated`'s own shape

**Choice.** One exported function, one exported struct, one exported error-free
contract:

```go
// Package jsconfig answers one question about a JavaScript source: where
// does an extra array element belong, or why does it belong nowhere. It
// holds no dharness spelling — no marker text, no file names, no framework
// names — and it never writes.
package jsconfig

// Anchor is a place in src, in byte offsets into the exact bytes Analyze
// was given. Every field is an offset or a literal read out of the source,
// so a caller splices with slice arithmetic and nothing else.
type Anchor struct {
    // ImportAt is where an import statement may be inserted: the start of
    // the line after the last top-level import declaration, or the start of
    // the file (after a BOM and after a "use strict"-style prologue) when
    // there is none.
    ImportAt int

    // LayerAt is where the array element belongs, per Decision 5's position
    // rule, already moved back over any contiguous comment run.
    LayerAt int

    // Indent is the leading whitespace of the anchor line, copied verbatim,
    // so the inserted element sits with its neighbours.
    Indent string

    // LineEnding is "\r\n" or "\n", read off the anchor line rather than
    // chosen. A file that mixes them keeps whatever that one line used.
    LineEnding string
}

// Analyze reports where dharness's layer belongs in src, or why it belongs
// nowhere. ok == false always carries a non-empty why; ok == true always
// carries a complete Anchor. Never both, never neither.
//
// It takes the source and nothing else. There is no framework parameter:
// dharness's layer is always the array's first element (Decision 5), so
// the position needs no knowledge of what the project imported.
func Analyze(src []byte) (a Anchor, why string, ok bool)

// Splice returns src with region inserted at at. It is separate from
// Analyze because it is the whole destructive operation and it is worth
// being able to state, in one line, that it is an insert: the result is
// src[:at] + region + src[at:] and nothing else. Decision 9's byte-surgery
// test asserts exactly that identity.
func Splice(src []byte, at int, region string) []byte
```

**Why the answer is a refusal string and not a typed error.** Every refusal
in the spec's matrix ends in the same place: `eslintExtendsStep.Delegated`
returns it as `why` and the agent reads it. A typed error would be
translated at exactly one call site into exactly one string, which is a seam
carrying nothing — the same argument that rejected the where/how split in
framework-presets Decision 1. `error` is reserved for what it means
everywhere else in this repository: the operation could not be performed.
`Analyze` performs; a refusal is a successful answer.

**Rejected: an `Elements` count on the anchor, for the candidate guard to
assert `N+1`.** It was in an earlier draft of this design and it is cut for
two reasons, the second of which is the important one.

It is *redundant*: the candidate guard already asserts no ERROR node
anywhere in the default export and exactly one well-formed region of each
marker kind, and `Splice`'s three-part byte identity is separately tested.
A count adds no case those three miss.

It is also *wrong on the replace path*. Decision 6 promises that a config
spliced before a preset started contributing a layer converges on the next
run — markers present, bytes different, so `Apply` replaces rather than
inserts. On a replace the element count is unchanged, so a constant `N+1`
assertion fails a correct write. The count would have to become a variable
the caller sets from the path it took, which is a guard that only ever
confirms what the caller already decided. Cutting it also removes the
tree-sitter subtlety it forced — comments are named array elements in this
grammar, so any count would have had to exclude them.

**Rejected: returning the tree, or a node handle.** That is the tree-sitter
type escaping the package, and the single-importer property with it. The
fallback in the proposal — cgo and the official bindings — survives only
because nothing outside `internal/jsconfig` names a tree-sitter type.

**Rejected: `Analyze(path string)`.** Reading the file is `internal/setup`'s
job; it already has the bytes, and it needs them again for the splice. A
package that reads files has I/O policy, and this one has none.

### Decision 2 — `.dharness/eslint.config.js` is declared shared, and an already-adopted allow list is repaired by appending

**The problem, which neither the proposal nor the spec surfaces.**
`internal/project/evidence.go:22-31` writes an allow list into
`.dharness/.gitignore`:

```
*
!.gitignore
!lefthook.yml
!fallow.jsonc
!rules.json
!evidence.json
```

A new owned file is ignored by default — that is the shape's whole point.
So `.dharness/eslint.config.js` would be untracked, the project's
`eslint.config.js` would import a file no other clone has, and lint would
fail everywhere except the machine that ran `sync`. Worse:
`EnsureDir` writes the ignore file **only when it is absent**
(`evidence.go:81-86`), and `TestOwnedDirectoryKeepsAnExistingIgnoreFile`
pins that. Every repository already adopted keeps the five-entry list
forever.

**Choice, in two parts.**

1. `dirIgnore` gains `!eslint.config.js`, and
   `internal/project/evidence_test.go:73`'s shared list gains it too.
2. `ownedFilesStep` gains one obligation: **the allow list must name every
   owned file, and a missing entry is appended.** Appended, not rewritten —
   the no-clobber test stays green, and the operation is the same class as
   `appendHuskyGate` (`files.go:154-167`), which is the precedent for adding
   one line to a file dharness does not rewrite.

```go
// ensureShared appends an allow-list entry for name when .dharness/.gitignore
// does not already name it. Append rather than rewrite: the ignore file is
// written once, at adoption, and a repository adopted before this change
// keeps a list that predates every file added since. Rewriting it would
// discard whatever the project added to it, which is the one thing
// TestOwnedDirectoryKeepsAnExistingIgnoreFile exists to forbid.
func ensureShared(p project.Project, w *Writer, name string) error
```

`ownedFilesStep.Satisfied` gains the same clause, so the repair is derived
and re-derived: delete the line by hand and the step reappears (§15).

**Why this is not a migration.** There is no migration mechanism in this
product and this does not introduce one. It is the same check as every other
step: read the repository, compare, converge.

**Rejected: naming the file in the project's root `.gitignore` instead.**
`dirIgnore`'s comment states the reason it exists — the directory declares
its own contents rather than asking the project's `.gitignore` to know about
dharness (§03).

**Rejected: leaving it and documenting it.** A file the project imports and
git does not carry is exactly the "written where it cannot fire" failure
this whole change exists to end.

### Decision 3 — the owned config is a factory, because a bare specifier resolves from the importing file and `.dharness/` is not where the packages are

**The problem.** In the motivating repository `Root` holds `wails.json` and
`Source` is `frontend/`; `node_modules` exists only under `Source`
(`PackageStateFiles`, `LocalBinary` both join from `p.Source`). Node resolves
a bare specifier by walking **up** from the importing module. A
`Root/.dharness/eslint.config.js` containing
`import plugin from "dharness-eslint-plugin"` searches
`Root/.dharness/node_modules`, then `Root/node_modules`, then above the
repository. `Root/frontend/node_modules` is never consulted. The owned config
cannot load the plugin it exists to configure.

**Choice: the bare specifiers live in the project's own config, and the owned
file exports a factory over them.**

`.dharness/eslint.config.js`, written by dharness:

```js
// dharness writes this file. It exports a factory rather than a config
// array because the packages it names are installed beside the project's
// package.json, and a bare specifier resolves from the file that writes it
// — which, in a split layout, is not this directory.
export default function dharnessLayer({ plugin, dharnessNext }) {
  return [
    ...dharnessNext,
    { plugins: { dharness: plugin }, rules: { /* the six severities */ } },
  ];
}
```

The project's `eslint.config.js`, in the two marked regions:

```js
// dharness:eslint-import begin — rewritten by `dharness sync`; edits here are lost.
import dharnessPlugin from "dharness-eslint-plugin";
import dharnessNext from "eslint-config-next";
import dharnessLayer from "../.dharness/eslint.config.js";
// dharness:eslint-import end
```

```js
  // dharness:eslint-layer begin — rewritten by `dharness sync`; edits here are lost.
  ...dharnessLayer({ plugin: dharnessPlugin, dharnessNext }),
  // dharness:eslint-layer end
```

The relative specifier is `ownedFrom(p, dir, "eslint.config.js")`, the
existing helper, computed from the directory of the config being edited —
which is `files.go:179-195`'s entire stated reason to exist.

**Both regions are generated from one list** — the matched presets' layer
bindings plus `plugin` — so the destructuring in the owned file and the call
in the project's file cannot disagree. That is a structural property, not a
convention.

**Why not the three alternatives.**

| Alternative | Rejected because |
|---|---|
| Bare specifier in the owned file, accept split-layout breakage | The motivating repository *is* split. A config that resolves on the author's machine and not in CI is the "severity written where it cannot fire" failure this change exists to end |
| A relative path into `frontend/node_modules/dharness-eslint-plugin/...` | Needs the package's entry file, which is `exports`/`main` in *its* package.json — dharness would be re-implementing module resolution, and Yarn PnP has no `node_modules` at all |
| A second owned directory under `p.Source` | Two `.dharness/` directories, two ignore files, and §03's single owned location gone, to avoid one function parameter |

**This is reasoned from Node's resolution algorithm, not measured.**
`CLAUDE.md`'s second rule applies to dharness's own verdicts, and §17 to
claims about other people's tools. **Slice 2 does not merge without the
probe**: a split fixture with the plugin installed under `frontend/`, an
owned config importing it by bare specifier, and `node --input-type=module`
resolving it — the failure recorded in `docs/learning-log.md` as a dated
line, or this decision retracted if it does not reproduce.

**Consequence, stated rather than discovered later.** Naming the file
`eslint.config.js` while it exports a factory means `eslint --config
.dharness/eslint.config.js` would hand ESLint a function that expects an
argument. ESLint does not auto-discover it — flat config searches from the
cwd upward, and the gate runs in `p.Source`, so `Root/.dharness/` is never
on that path. The name is kept because the spec pins it, `dirIgnore` names
it, and the goldens carry it; the file's own first comment says what it is.

### Decision 4 — the marker pairs share `owned.go`'s grammar and none of its code

**Choice.** Two new pairs, in `internal/setup/eslintregion.go`:

```go
const (
    eslintImportBegin = "// dharness:eslint-import begin — rewritten by `dharness sync`; edits here are lost."
    eslintImportEnd   = "// dharness:eslint-import end"
    eslintLayerBegin  = "// dharness:eslint-layer begin — rewritten by `dharness sync`; edits here are lost."
    eslintLayerEnd    = "// dharness:eslint-layer end"
)
```

**Same grammar, different names, and the code is not shared.** The wording
is `presetBegin`'s wording (`owned.go:19-22`) so a reader who has seen one
recognises the other without documentation. The names differ because each
region must be individually addressable — the spec's requirement, and the
reason a re-run replaces exactly its own bytes.

**Why `regionBounds` is not reused.** `regionBounds` answers
`(begin, end, ok)` and its caller `replaceRegion` treats `!ok` as *insert
fresh* (`owned.go:138-147`). For a file dharness owns, guessing is correct:
dharness wrote every byte. For the project's file, a begin without an end is
exactly the half-written region the spec requires a **refusal** for. One
helper with two failure policies is where a corruption bug lands. So:

```go
// markerState is a three-way answer, not a bool, because absent and
// malformed lead to opposite actions: absent means insert, malformed means
// refuse and delegate. regionBounds in owned.go collapses both into
// "insert", which is correct for a file dharness wrote every byte of and
// is exactly the guess that corrupts a file it did not.
type markerState int

const (
    markersAbsent markerState = iota
    markersPresent
    markersMalformed
)

func markerRegion(raw, begin, end string) (from, to int, state markerState)
```

Malformed is: a begin with no end, an end with no begin, an end before its
begin, or more than one of either marker.

**These four strings ship into every adopting repository and cannot be
changed cheaply afterwards.** They are fixed in slice 3a's review, before
any repository has them, exactly as framework-presets' open question fixed
`dharness:presets` in its slice 2.

**Line comments, not block comments.** `//` is what the JavaScript ecosystem
writes, tree-sitter reports it as a named element with its own byte range
(the probe), and — unlike `/* */` — a lost end delimiter cannot swallow the
code after it.

### Decision 5 — the import table answers the `defineConfig` identity, and dharness's layer is always the array's first element

**The mechanism.** `Analyze` walks the top-level import declarations once and
builds `binding → module specifier`. It is used for exactly one question, and
it is the only place in this design where a name becomes a module:

- **`defineConfig`**: the default export is a call expression; the callee is
  a bare identifier; that identifier is in the table; and its specifier is
  exactly `"eslint/config"`. Any other callee shape — a member expression
  like `tseslint.config`, an identifier absent from the table, or an
  identifier whose specifier is anything else — delegates. The spec's
  lookalike scenario is answered by the table, never by the name.

**Why recognition is by specifier and never by name.** §09: a direct signal
exists — the import declaration says which module the binding came from — so
no proxy is invented over it. §17: a name match is an assumption about the
project; an import specifier is a fact the file states.

**Cut: the framework floor.** An earlier draft gave `Analyze` a
`frameworkModules []string` parameter, recognised spread elements whose
specifier was in that list, and walked the array to land dharness's layer
just past the last one. It is removed entirely, along with the parameter, the
spread recognition and the walk. Three reasons, in the order they were found:

1. **It has no measured effect.** ESLint flat config merges last-wins *per
   rule*. `eslint-config-next` and `eslint-config-expo` declare no
   `dharness/*` rule, so wherever they sit relative to dharness's layer,
   neither can override anything dharness writes. The floor bought ordering
   between two layers that do not intersect. The spec says as much already —
   the floor is "a bonus this rule provides, never a requirement it depends
   on".
2. **Index 0 satisfies the invariant unconditionally, and more strongly.**
   §05 needs `dharness < projectCustom`. First-element placement puts
   dharness's layer below *everything* the project wrote, so the invariant
   holds by construction rather than by a walk that could false-positive.
   Nothing the project authored can be shadowed by dharness, because
   dharness is underneath all of it.
3. **It duplicated Decision 3.** The walk placed dharness's layer after the
   project's own `...next`, while the owned factory spreads `next` *inside*
   that layer — two mechanisms positioning the same framework config, and the
   draft never noticed they overlapped. Removing the walk leaves one.

It also removes the only coupling from `internal/jsconfig` to what
`internal/preset` computes, which keeps the cgo fallback a one-package
rewrite rather than a two-package one.

**The position rule, concretely.**

1. Find the array literal: the default export's own array, or the first
   argument of a recognised `defineConfig` call.
2. `LayerAt` is the start of the line of its first non-comment named child.
3. Move `LayerAt` back over the contiguous comment run immediately preceding
   that element: the earliest byte of the run, then to the start of its line.
   Contiguity is measured by there being no blank line between a comment's
   end and the next element's start, so a comment separated by a blank line
   is a section header and stays above dharness's region.
4. Where the array is empty, `LayerAt` is the offset just past `[`, and the
   inserted region carries its own line break.

**dharness never lands after a project element**, and with the walk gone that
is no longer an argument — it is the rule itself.

### Decision 6 — the re-parse is a precondition of the write, and `Undo` is the backstop for the rest of the run

**Choice.** `eslintExtendsStep.Apply` never writes bytes that have not
already passed their own check:

```
read the project's config
  → marker scan of src  ──→ malformed : unreachable, Delegated answered it
                        ├─→ absent    : the insert path
                        └─→ present   : the replace path

  insert path
      → Analyze(src)                        refusal here is unreachable:
                                            Delegated already answered it
      → candidate := Splice(Splice(src, LayerAt, layerRegion), ImportAt, importRegion)
          (the later offset is spliced first, so it does not shift —
           ImportAt < LayerAt always, so the layer splice is applied first)

  replace path
      → candidate := replace(replace(src, layerFrom, layerTo, layerRegion),
                                          importFrom, importTo, importRegion)
          (same rule, same reason: the layer region is the later one, so it
           goes first; Analyze is not called, because the bounds come from
           the marker scan rather than from the position rule)

  both paths
  → Analyze(candidate)  +  marker scan of candidate
      assert: no ERROR node anywhere in the default export
      assert: exactly one region of each marker kind, both well-formed
  → w.Write(path, candidate)               ← the only write, and it is last
```

**Why the replace path is written down here rather than left implicit.**
`Satisfied` is a byte comparison, so *markers present with different bytes*
is a reachable, expected state — it is exactly the convergence case below.
An `Apply` that only knew how to insert would splice a second pair of
regions into a file that already had one, the candidate's own marker scan
would reject it as malformed, `Apply` would return an error, `applySteps`
would unwind, and the step would fail on every subsequent run until a human
deleted the region by hand. The two paths are separate because the bounds
come from different places: the insert path asks the position rule where the
region *belongs*, the replace path asks the marker scan where it *is*.

**Why this is stronger than the proposal's ordering and still satisfies the
spec.** The spec requires a re-parse of "the resulting bytes"; the resulting
bytes are the candidate, which exists in memory. Verifying there means the
unparseable file **never exists on disk** — no window for a watcher, an
editor, a concurrent `eslint --watch`, or a crash between the write and the
check. The spec's scenario asks that `Apply` return an error, that the
original bytes be intact, and that `sync` report the step failed; all three
hold, and the third leg holds trivially rather than by a successful restore.

`Writer.Undo` is not made redundant. `applySteps` (`setup.go:89-116`) runs
several steps before this one — `installStep` has already touched
`package.json` and a lockfile — and an error from `Apply` still unwinds all
of it in reverse. What changes is that the file this design is afraid of is
not among the things that need unwinding.

**Rejected: running ESLint to validate the splice.** A Node process during
`sync`, and a config can legitimately fail to load for reasons the splice did
not cause — a missing peer dependency, a plugin not installed yet. The
structural re-parse is the verdict (§11, §17, `CLAUDE.md`'s second rule).
The proposal already rejected it; it is restated here because it is the
obvious thing to reach for when the guard someday reports a false negative.

**Idempotency, and why `Satisfied` is a byte comparison.**
`eslintExtendsStep.Satisfied` is true when both regions are present,
well-formed, and their bytes equal what this run would render. That is
`ownedFilesStep.Satisfied`'s rule (`steps.go:114-118`) applied to a file
dharness does not own, and it buys two things at once: a second `sync` is a
no-op (`Apply` is never called, so the file is byte-identical by
construction, not by care), and a config spliced before a preset started
contributing a layer **converges** on the next run instead of drifting.

### Decision 7 — `Manifest` gains `Layers`, and `Validate` gains two rules

**Choice.**

```go
// Layer is a config layer one framework publishes about itself: a package
// to install, and the binding .dharness/eslint.config.js receives it under.
// It is the third contribution kind because it is neither a fallow config
// key (Fact) nor prompt text (Seed) — it is a dependency plus a name in
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
    // SyntaxError. Measured against Node: same module twice under one name
    // fails to compile, same module twice under two names loads and
    // contributes its entries twice, which is inert.
    Binding string

    // Because names the observable, exactly as Fact.Because does.
    Because string
}

type Manifest struct {
    Schema string
    Facts  []Fact
    Seeds  []Seed
    Layers []Layer
}

// Layers enumerates every layer contributed across matches, in match order
// — the same Root-then-Source, registry order Resolve returns — so the
// generated import block and factory signature are byte-stable across runs.
func Layers(matches []Match) []Layer
```

`Validate` gains, alongside the rules it already applies:

| Rule | Why it is checkable, and why here |
|---|---|
| `Because` is non-empty | The existing evidence rule (§17), unchanged |
| `Package` carries no `@` after position 0 | The spec forbids pinning a version. `@scope/name` is legal, `name@1.2.3` is not. A string test, not a policy |
| `Binding` matches `[A-Za-z_$][A-Za-z0-9_$]*` | It is emitted as an identifier in two places. An authoring bug caught at build time, exactly as `json.Marshal` on `Fact.Value` already is |
| `Binding` has the `dharness` prefix | Collision-proof by construction. dharness writes imports into a file that already has imports, and two declarations of one identifier in an ES module is a SyntaxError — a config ESLint cannot load. A prefix test is three lines and needs no lookup against the project |

**Cut: a JS reserved-word check on `Binding`.** The regex stays; the keyword
table does not. It would be a ~40-entry list in Go, maintained against a
language that adds keywords, policing two registry entries dharness writes
and a registry test dharness also writes. Producing the bug it catches means
an author naming a binding `class` or `default` on purpose. The `Package`
rule survives the same question for the opposite reason: it is three lines,
and it encodes a requirement the spec actually states.

Validation stays where framework-presets Decision 2 put it: a test that walks
the registry, never a user's run. A malformed manifest is a dharness
authoring bug, and failing someone's `sync` for it blocks on something
recoverable (§20).

**No schema bump.** `dharness.preset/v1` is unchanged: the manifest never
leaves the binary, so the field is additive. That is the spec's requirement
and it is also just true — `Schema` is compared by `Validate` and by nothing
else.

**`integrationPackages` becomes preset-aware, and nothing else changes.**

```go
// integrationPackages lists the packages dharness adds to this project: the
// rules plugin every project gets, plus whatever the matched presets
// contribute. It takes a project now because the answer depends on
// detection — the same re-derive-at-the-call-site rule framework-presets
// decision 4 fixed, and for the same reason: a memoised value would be
// recorded state (§07).
func integrationPackages(p project.Project) []string {
    packages := []string{RulesPackage}
    for _, layer := range preset.Layers(preset.Resolve(p)) {
        packages = append(packages, layer.Package)
    }
    return dedupe(packages)
}
```

`installStep.Apply`'s rollback needs nothing new: it already snapshots
`PackageStateFiles()` and compensates with `RemovePackages` over **the exact
same slice** it installed (`steps.go:69-86`). A preset-contributed package is
in that slice, so it is in the removal. The seam generalises because it was
written over a value rather than a constant.

### Decision 8 — `doctorConfigStep` is removed from `Plan()`, because a step whose satisfied state is unreachable is not a step

**The mechanism, not a preference.** `doctorConfigStep.Satisfied` is exactly
"`RulesPackage` appears in `doctor.config.json`'s `plugins`"
(`steps.go:458-476`). The spec requires that dharness stop writing that
declaration. A step that must not write the only thing that satisfies it is
**unsatisfiable forever**: it sits in every `sync` report, in every plan
region of every golden, and nothing the project can do clears it. §15 says a
step must be able to reappear; its corollary is that a satisfied step
disappears, and this one never can. `UncheckableConfigNote`
(`steps.go:358-379`) already made this exact argument once in this
repository — "a step with no completion state is not pending work" — and it
is the argument here.

The step's other contribution, the six severities, moves to
`.dharness/eslint.config.js` by the spec's first requirement. Nothing is
left. So:

- `doctorConfigStep` and `doctorConfigFile` are deleted; `Plan()` goes from
  eleven steps to ten and then back to eleven when `eslintExtendsStep` joins
  in slice 3a.
- `doctor.config.json` leaves the generic golden's `== tree ==` entirely — a
  generic project no longer receives one.
- `legacyLintConfigStep` **stays**. It is about `.eslintrc.json`, a different
  file and a different measured failure, and react-doctor still runs.
- `RulesPackage` is still installed. The plugin package did not stop being
  needed; its consumer changed from react-doctor to ESLint.

**The first-write limit dissolves, and that is the point.** `DefaultSeverity`
is unchanged as a function — barrel-derived `folder-ownership`, `error`
otherwise — but the limit recorded around it in `plugin.go:94-100` was
never a property of the function. It was a property of writing into
`doctor.config.json`, a file the project also edits, where dharness cannot
tell "the project chose `off`" from "dharness wrote `off`" (§05). In
`.dharness/eslint.config.js` there is nothing to tell apart: dharness wrote
every byte, the file is rewritten every run, and a project that disagrees
disagrees in **its own layer, which comes last and wins**. So a project that
grows barrels later does get `folder-ownership` switched on by a later
`sync`, and §05 is satisfied structurally instead of by declining to
re-derive.

`plugin.go`'s comment block is rewritten rather than dropped, as
framework-presets Decision 6 did for `offByDefault`: the measurement (eight
non-actionable findings on one repository) stays, and the stated limit is
replaced with the reason it no longer applies.

**Residue is reported, never removed.** An already-adopted
`doctor.config.json` keeps its `dharness/*` block and its `plugins` entry.
dharness cannot distinguish its own past write from the project's (§05, the
same limit `plugin.go` already records), those entries are inert under the
gate's `--staged` invocation, and machine-removing them is not on the table.
The report is a note beside the plan, not a step — `UncheckableConfigNote`'s
exact shape, and for its exact reason: there is no state the project reaches
that makes dharness stop saying it, so it is not pending work.

### Decision 9 — the gate stage carries its own command, and the staged paths are re-based

**Two things block an ESLint stage today, and neither is in the proposal.**

1. `RunCheck`'s loop builds every command with
   `tool.RemoteLatest(...)` (`check.go:86`). The stage list cannot express a
   locally-resolved binary, which is precisely the recorded §03 exception
   this change needs.
2. `p.StagedSourceFiles()` returns **repository-relative** paths
   (`git.go:89-107`), while the stages run with `Dir: p.Source`. react-doctor
   asks git itself, so it never mattered. ESLint takes explicit paths, so in
   a split layout it would be handed `frontend/src/a.ts` while running in
   `frontend/` and report every file as missing — or worse, silently lint
   nothing.

**Choice.** `stage` carries its command instead of its arguments:

```go
// stage is one wrapped tool, the command that runs it, and the command that
// points at its help. It carries commands rather than arguments because the
// stages no longer resolve the same way: react-doctor and fallow run
// through the remote executor, ESLint runs the project's own binary
// (§03's recorded exception — a flat config imports the project's plugins,
// and a transient environment cannot resolve them).
type stage struct {
    tool    string
    command runner.Command
    help    runner.Command
}
```

with `remoteStage(p, name, args...)` and `localStage(p, name, path, args...)`
building the two shapes. The loop loses its `RemoteLatest` call and gains
`stage.command`; `pointer(...)` takes `stage.help`.

The invocation, in `internal/tool`:

```go
// ESLintStaged lints exactly the staged files, by explicit path.
//
// No --cache. It writes .eslintcache into the project's tree, which §03
// would then have to account for as project-owned or dharness-owned, and it
// is stale-prone across branches — a cache keyed on a file's content and
// the config's mtime reports a clean file after a branch switch that
// changed neither. The staged file list is the larger win and it is already
// available. Caching is a measured optimisation, deferred, and this comment
// is here so it is not re-added as an obvious improvement.
//
// The paths arrive relative to p.Source, not to the repository: the command
// runs where the package manager installed ESLint, and git reports paths
// from the repository root.
func ESLintStaged(files []string) []string
```

and the re-basing, beside the existing scoping in `git.go`:

```go
// StagedSourceFilesFromSource is StagedSourceFiles with the source prefix
// removed, for a tool that runs in p.Source and takes explicit paths.
// StagedSourceFiles already filtered to that prefix, so this only strips it.
func (p Project) StagedSourceFilesFromSource() ([]string, error)
```

**The skip.** `p.LocalBinary("eslint") == ""` → the stage is not built at
all, and a deferred line names it and why, in `HasCommits`'s exact shape
(`check.go:76-78`). Not a failure, not a block (§13, §16, §20). dharness
does not install ESLint at gate time; that is `eslintExtendsStep`'s question
at `sync` time, and the two are independent by construction — one asks about
a file, the other about a binary.

**Placement is not decided here, and saying so is the decision.** The spec
requires a measurement and this repository has already paid once for
asserting an ordering it had not measured. What is fixed here is the
mechanism and the provisional position:

- **Provisionally last**, after `fallow dupes`. Last is the only position
  that cannot make a currently-enforced check slower to reach, so it is the
  conservative default while the number is unknown.
- **Slice 4 does not merge without the measurement**: wall-clock for each
  stage over the same explicit staged file list on the reference repository,
  three runs, median, recorded as a dated line in `docs/learning-log.md`
  naming the numbers. If ESLint's floor is below react-doctor's, it moves to
  first, per §12 — and the commit that moves it cites the line.

### Decision 10 — the generic golden changes twice, in two named slices, and the splice is not why

**Settling the spec's open question.** The spec requires that the *splice
sub-mechanism* contribute nothing to the generic fixture's tree. Verified
against the fixture and `renderGolden`: it does not — `genericConventionalProject`
and `genericSplitProject` write only `package.json` and a lockfile
(`golden_test.go:229-256`), so there is no ESLint config, so nothing is
spliced.

**But the step does write, and the fixture does change.** The write-if-absent
path fires for exactly these fixtures — no flat config, no `.eslintrc.*` —
so `eslint.config.js` appears in `== tree ==`, in `p.Source`. That is the
same thing `.fallowrc.json` already does in that fixture
(`generic-conventional.txt:216-220`), by the same `wireFallowExtends`
precedent, and it is correct rather than a leak.

`renderGolden` walks every step in `Plan()` (`golden_test.go:140-150`), so
any change to the plan changes the fixture, and
`TestGenericMechanismHasNoUpdatePath` forbids wiring `-update` into it. That
is Decision 7 of framework-presets working, not failing. The edits, per
slice:

| Slice | `== plan ==` | `== tree ==` |
|---|---|---|
| 2a | unchanged — no step is added or removed | `.dharness/eslint.config.js` added, complete with its severities; `.dharness/.gitignore` gains `!eslint.config.js`. **The three framework fixtures take the same tree delta**, regenerated with `-update` |
| 2b | `doctorConfigStep`'s row removed (eleven steps → ten) | `doctor.config.json` removed entirely |
| 3a | `eslintExtendsStep`'s row added (ten → eleven) | `eslint.config.js` added at `<root>` (conventional) / `<source>` (split) |
| 3b | unchanged | unchanged — the splice never fires on a fixture with no config |
| 5 | unchanged | unchanged for generic; the three framework fixtures regenerate with `-update` |

Each is a hand-authored, reviewed diff in the slice that causes it. The
proposal assigned the plan-region edit to 3a alone; **2b needs one too**,
because removing a step is a plan change. That correction is Decision 8's
direct consequence.

**Why 2a carries a tree edit and 2b does not carry the file.** `renderGolden`
renders the plan and then calls `Apply` (`golden_test.go:140-153`), so a file
lands in `== tree ==` in the slice that wires its write into
`ownedFilesStep.Apply` — not in a later one. 2a is that slice by its own
definition, and it has to be: a 2a that built the render and repair functions
without wiring them would have no observable behaviour at all, which is the
dead-code failure that merged slices 1a and 1b back together.

**And it lands in every fixture, not only the two under scrutiny.**
`renderGolden` runs the whole plan for each fixture in the suite, so wiring a
write in `ownedFilesStep.Apply` changes `nextjs.txt`, `wails.txt` and
`wails-nextjs.txt` in the same slice as the generic pair. Those three have an
`-update` path — framework-presets Decision 7 made them living fixtures — so
they regenerate rather than being hand-edited, and only the generic pair is
authored by hand. Measured during 2a: all five gained the identical
23-line block and nothing else. A reviewer expecting a two-file diff should
expect five.

So the severities are rendered into the owned file in **2a**, and for the
span between 2a and 2b they exist in both `.dharness/eslint.config.js` and
`doctor.config.json`. That is not the two-homes drift this repository keeps
finding: both are rendered from `DefaultSeverity`, which stays the single
source, and 2b deletes the second home. 2b is then purely a removal —
`doctorConfigStep` out of `Plan()`, `doctor.config.json` off disk,
`plugin.go`'s first-write limit retired.

### Decision 11 — the CRLF and BOM fixtures are constructed in Go, and `.gitattributes` is not touched

Stated as a decision because getting it wrong is silent.
`.gitattributes:4` is `* text=auto eol=lf`, added because gofmt reports a
CRLF file as unformatted. A committed CRLF `.js` fixture would be
**normalised to LF on checkout**, so the CRLF test would pass while testing
nothing — and on a Windows checkout with `autocrlf` it would pass for a
third, different reason.

So the CRLF and BOM cases are built from the LF fixture at test time:

```go
crlf := bytes.ReplaceAll(lf, []byte("\n"), []byte("\r\n"))
bom  := append([]byte("\xef\xbb\xbf"), lf...)
```

Only LF fixtures are committed. **Rejected: a `-text` exception for
`internal/jsconfig/testdata/`.** It would work, and it puts a rule in
`.gitattributes` whose violation is invisible — nobody reviewing a new
fixture checks whether the path still matches the pattern. Constructing the
bytes makes the property local to the test that asserts it.

## Data Flow

    project.Discover(dir) ──→ Project{Root, Source, …}
              │
              ├─ sync ──→ preset.Resolve(p) ──→ []Match
              │              │
              │              ├─→ preset.Layers(matches) ──→ []Layer
              │              │        │
              │              │        ├─→ integrationPackages(p) ──→ installStep
              │              │        │
              │              │        ├─→ ownedEslintConfig(p, layers) ──→ .dharness/eslint.config.js
              │              │        │      └─ factory over {plugin, …bindings}
              │              │        │      └─ DefaultSeverity(p, rule) per rule
              │              │        │
              │              └─→ importRegion / layerRegion(layers)
              │                          │
              │                          ↓
              │              eslintExtendsStep.Satisfied / .Delegated / .Apply
              │                          │
              │                  read the project's config
              │                          │
              │                          ↓
              │              jsconfig.Analyze(src)
              │                  ├─ import table: binding → specifier
              │                  ├─ default export: array | defineConfig | other
              │                  ├─ ERROR node anywhere → why, ok=false
              │                  └─ Anchor{ImportAt, LayerAt, Indent, LineEnding}
              │                          │
              │                  markerRegion(raw, …) ──→ absent | present | malformed
              │                          │
              │                  malformed ──→ delegate, name the pair
              │                  present   ──→ Satisfied iff the bytes equal the render;
              │                                otherwise replace ×2 at the scanned bounds
              │                  absent    ──→ jsconfig.Splice ×2 at the anchor
              │                                       │
              │                                       ↓
              │                                    candidate
              │                                       │
              │                                jsconfig.Analyze(candidate)
              │                                  + marker scan
              │                                       │
              │                          fails ──→ error → applySteps → Writer.Undo
              │                          passes ─→ w.Write(path, candidate)   ← only write
              │
              └─ check ──→ p.LocalBinary("eslint")
                             │
                     ""  ──→ stage not built; named in the output (§16), never a block (§20)
                     path ─→ localStage(p, "eslint", path,
                                 tool.ESLintStaged(p.StagedSourceFilesFromSource()))

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/jsconfig/jsconfig.go` | New | `Anchor`, `Analyze`, `Splice`; the import table, the default-export shapes, the position rule, the comment-run scan |
| `internal/jsconfig/jsconfig_test.go` | New | The refusal matrix, the position rule, byte-surgery identity, CRLF/BOM |
| `internal/jsconfig/testdata/*.js` | New | LF-only inputs; CRLF and BOM variants constructed in Go (Decision 11 below) |
| `go.mod`, `go.sum` | Modify | `github.com/odvcencio/gotreesitter`, the product's first dependency |
| `AGENTS.md` | Modify | The dependency recorded as a stated deviation |
| `internal/project/evidence.go` | Modify | `dirIgnore` gains `!eslint.config.js` (Decision 2) |
| `internal/project/evidence_test.go` | Modify | The shared-list assertion gains the same entry |
| `internal/project/git.go` | Modify | `StagedSourceFilesFromSource` (Decision 9) |
| `internal/setup/eslintconfig.go` | New | The owned factory's rendering, the two region renderers, the binding list both are built from |
| `internal/setup/eslintregion.go` | New | The four marker constants, `markerState`, `markerRegion` |
| `internal/setup/files.go` | Modify | `eslintFlatConfig`/`eslintLegacyConfig` name detection; `ensureShared` |
| `internal/setup/steps.go` | Modify | `eslintExtendsStep`; `doctorConfigStep` and `doctorConfigFile` deleted; `integrationPackages(p)`; `ownedFilesStep` writes the owned ESLint config and repairs the allow list |
| `internal/setup/setup.go` | Modify | `Plan()`: one step out (2b), one step in (3a) |
| `internal/setup/plugin.go` | Modify | The severity comment block rewritten; the first-write limit retired with its reason (Decision 8) |
| `internal/preset/preset.go` | Modify | `Layer`, `Manifest.Layers`, `Layers()`, two `Validate` rules |
| `internal/preset/nextjs.go`, `expo.go` | Modify | `eslint-config-next` / `eslint-config-expo` layers with evidence |
| `internal/cli/check.go` | Modify | `stage` carries commands; `remoteStage`/`localStage`; the ESLint stage and its skip |
| `internal/tool/tool.go` | Modify | `ESLint` constant, `ESLintStaged` |
| `internal/setup/golden_test.go` + `testdata/golden/` | Modify | Generic fixtures hand-edited per Decision 10's table; framework fixtures via `-update` |
| `docs/design-principles.md` | Modify | §03's project-local resolution exception, recorded beside the 10 August 2026 amendment |
| `docs/learning-log.md` | Modify | Decision 3's split-layout resolution probe; Decision 9's placement measurement |

`.gitattributes` is **not** touched — see Decision 11.

## Interfaces / Contracts

```go
// internal/jsconfig
type Anchor struct {
    ImportAt   int
    LayerAt    int
    Indent     string
    LineEnding string
}

func Analyze(src []byte) (Anchor, string, bool)
func Splice(src []byte, at int, region string) []byte

// internal/preset
type Layer struct {
    Package string
    Binding string
    Because string
}

type Manifest struct {
    Schema string
    Facts  []Fact
    Seeds  []Seed
    Layers []Layer
}

func Layers(matches []Match) []Layer

// internal/project
func (p Project) StagedSourceFilesFromSource() ([]string, error)

// internal/setup
type eslintExtendsStep struct{}

func integrationPackages(p project.Project) []string
func ensureShared(p project.Project, w *Writer, name string) error
func markerRegion(raw, begin, end string) (from, to int, state markerState)
func ownedEslintConfig(p project.Project, layers []preset.Layer) string
func eslintImportRegion(p project.Project, dir string, layers []preset.Layer, indent, eol string) string
func eslintLayerRegion(layers []preset.Layer, indent, eol string) string

// internal/tool
const ESLint = "eslint"
func ESLintStaged(files []string) []string
```

## Testing Strategy

**Three existing seams carry all of it; this change invents one helper and no
seam.**

1. **`runner.Run`'s package variable** already keeps `installStep` from
   shelling out under the golden test (`golden_test.go:135`), and now keeps
   the ESLint stage from launching Node under `check`'s tests.
2. **`project.SetGitOutputForTest`** already answers `rev-parse`, `ls-files`
   and `diff --cached`. The re-based staged list (Decision 9) is a table of
   stub responses plus a `SourceRel` — no `git init`, no index.
3. **The filesystem is not seamed, deliberately.** `git.go`'s stated policy:
   *"a temp directory is a real tree."* Every splice fixture is a real file
   in `t.TempDir()`.

**How the splice is testable without a repository.** The two operations that
matter are pure functions of bytes: `Analyze(src, modules)` and
`Splice(src, at, region)`. Neither reads a file, neither needs a project, and
neither needs git. `eslintExtendsStep` is the thin layer above them, and its
own tests construct a `project.At(tmp, tmp)` with a hand-written config —
exactly the shape `setup_test.go` already uses. `runner.Run` is stubbed for
the one step that shells out.

**How a fixture proves byte-surgery rather than a correct-looking result.**
This is the assertion, and it is the reason `Splice` is a separate exported
function:

```go
got := Splice(src, at, region)
// The output IS the input with one contiguous run inserted — not a
// re-render that happens to look the same. A pretty-printer that produced
// identical-looking output fails all three of these.
if len(got) != len(src)+len(region)                  { t.Error(...) }
if !bytes.Equal(got[:at], src[:at])                  { t.Error(...) }
if !bytes.Equal(got[at+len(region):], src[at:])      { t.Error(...) }
```

A golden `.expected.js` alone would pass for a formatter that reproduced the
file; these three do not. The `.expected.js` fixtures stay, for readability
of the diff, but they are the secondary assertion.

New tests, one per contract this design asserts:

| Test | Pins |
|---|---|
| `TestSpliceInsertsAndChangesNothingElse` | Decision 9 of testing above: the three-part byte identity |
| `TestAnalyzeRefusesEveryRefusalMatrixCell` | `.ts` is not jsconfig's case; the rest — unrecognised call, `ERROR` node, non-array export — each with its own `why` |
| `TestDefineConfigResolvesByImportSpecifier` | Decision 5: `"eslint/config"` splices, a project-local `defineConfig` of the same name delegates |
| `TestDefineConfigWithAnErrorNodeArgumentRefuses` | All three conditions, not two of three |
| `TestLayerLandsAtTheFirstElement` | Decision 5: the position rule, including an array whose first element is a framework spread |
| `TestCommentRunPrecedingTheAnchorIsNotOrphaned` | The comment-run scan, and the blank-line boundary |
| `TestPresentMarkersWithStaleBytesAreReplacedNotDuplicated` | Decision 6's replace path: one region of each kind after a second `sync` that renders differently |
| `TestCRLFIsMatchedNotNormalised` / `TestBOMSurvives` | Constructed in Go, per Decision 11 |
| `TestMarkerRegionDistinguishesAbsentFromMalformed` | Decision 4's three-way answer, all four malformed shapes |
| `TestSpliceGuardRollsBackAnUnparseableResult` | Decision 6, with `Splice` stubbed via a constructed adversarial region |
| `TestSecondSyncWritesNothing` | `Satisfied` true, `Apply` not called, bytes equal |
| `TestOwnedEslintConfigIsDeclaredShared` | Decision 2, on a fresh `.dharness/` |
| `TestExistingAllowListGainsTheMissingEntry` | Decision 2's append path, on a five-entry list |
| `TestAllowListRepairKeepsWhatTheProjectAdded` | The no-clobber property `EnsureDir`'s test already pins |
| `TestLayerValidateRejectsAPinnedVersion` / `…AnInvalidBinding` | Decision 7's two new rules |
| `TestInstallIncludesPresetContributedPackages` | `integrationPackages(p)` over a Next.js fixture |
| `TestEslintStageIsSkippedWithoutABinary` | §13/§20: skipped, named, exit unaffected |
| `TestEslintStagePathsAreRelativeToSource` | Decision 9's re-basing, on a split fixture |

**Mutation coverage, per `tools/mutationstaged`.** Four branches are easy to
leave unkilled and must be asserted positively:

- the `< ImportAt`/`LayerAt` splice ordering — a fixture where reversing the
  two splices shifts the second offset and produces a broken file;
- `markerState`'s malformed arm — assert the **refusal**, not merely that
  nothing was written;
- the insert/replace branch on `markerState` — a fixture with present markers
  and stale bytes must end with exactly one region of each kind, or the arm
  collapsing to *always insert* survives;
- the comment-run's blank-line boundary — a comment separated by a blank line
  must stay *above* the region, or dropping the boundary check survives.

**Platform.** Every path assertion through `filepath.Join`; every rendered
path through `filepath.ToSlash`. `renderGolden` already normalises `\r\n` to
`\n` when capturing the tree (`golden_test.go:162`), so a written
`eslint.config.js` is stable across platforms in the fixture — and that is
also why the CRLF assertions live in `internal/jsconfig`'s tests, where they
compare raw bytes, and not in the golden.

## Threat Matrix

| Boundary | Applicability | Design response | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | **N/A** — nothing here classifies a file as executable or not |
| Git repository selection | **Applicable** — the gate's staged list is repository-relative while the stage runs in `Source` | `StagedSourceFilesFromSource` strips the prefix `StagedSourceFiles` already filtered on | `TestEslintStagePathsAreRelativeToSource`, on a split fixture |
| Commit state | **Applicable** — an owned file that git does not carry is invisible to every other clone | Decision 2: `dirIgnore` names it, and an existing allow list is repaired by appending | `TestOwnedEslintConfigIsDeclaredShared`, `TestExistingAllowListGainsTheMissingEntry` |
| Push state | **N/A** — no remote interaction |
| PR commands | **N/A** — no VCS automation |
| Subprocess execution | **Applicable** — `check` gains one process per run, and only when a binary is resolvable | Skipped entirely with no binary; never `RemoteLatest`, so no network fetch (§03's recorded exception) | `TestEslintStageIsSkippedWithoutABinary`; `runner.Run` stubbed everywhere else |
| **Destructive write to a file dharness does not own** | **Applicable, and the reason Decisions 4 and 6 exist** — this is the project's whole lint and often its pre-commit | Refuse on anything not fully understood; splice only between two distinct marker pairs; verify the candidate **before** writing; `Writer` snapshots the file either way | `TestSpliceGuardRollsBackAnUnparseableResult`, `TestMarkerRegionDistinguishesAbsentFromMalformed`, the byte-identity triple |
| Command-line length | **Applicable, newly** — ESLint takes explicit paths, and Windows caps a command line at 32,767 characters | Accepted for now: the list is the staged set, and a commit staging enough source files to exceed it is outside every measurement this repository has. Named here so a report of "ESLint did not run on a huge commit" is diagnosed rather than rediscovered | None. Chunking is a measured optimisation, deferred with `--cache` |
| Third-party parser correctness | **Applicable** — a partial tree from an error-tolerant parser | Any `ERROR` node covering the default export refuses the whole file; the guard re-parses the candidate with the same parser, so a parser that accepts what it should not still cannot produce an element count that is wrong | `TestAnalyzeRefusesEveryRefusalMatrixCell` |

## Line Forecast

| Slice | Content | ~lines | Change from the proposal |
|---|---|---|---|
| 1 | `internal/jsconfig` entire: parse, import table, default-export shapes, refusals, `Anchor`, `Splice`, the comment-run scan, CRLF/BOM, the byte-surgery tests, the dependency, the `AGENTS.md` deviation | ~420 | **One slice, as the proposal had it** — see below |
| 2a | The owned factory config with its severities, `ownedFilesStep` writing it, `dirIgnore` + `ensureShared` + the allow-list repair, the generic goldens' first hand-edit (**tree only**) | ~250 | **New scope.** Decision 2 is not in the proposal at all |
| 2b | `doctorConfigStep` deleted; `doctor.config.json` off disk; `plugin.go`'s comment rewritten and its first-write limit retired; the generic goldens' second hand-edit (**plan and tree**) | ~200 | The proposal's slice 2, minus what moved to 2a, plus the plan-region edit Decision 8 forces |
| 3a | `eslintExtendsStep`: write-if-absent, the full refusal matrix, `Delegated`; the generic goldens' third hand-edit | ~300 | Unchanged |
| 3b | The insert and replace paths, both marker pairs, the re-parse precondition, idempotency | ~420 | Up from ~400: the replace path is written down now (Decision 6) |
| 4 | The gate stage: `stage` carrying commands, `ESLintStaged`, `StagedSourceFilesFromSource`, the skip, the placement measurement | ~260 | **Up from ~200.** The `stage` generalisation and the path re-basing are not in the proposal |
| 5 | `Layer`, `Manifest.Layers`, two `Validate` rules, `integrationPackages(p)`, the Next.js and Expo layers, framework goldens via `-update` | ~260 | Down from ~300: the reserved-word table is cut (Decision 7) |

Realistic total **~1,900–2,150 changed lines** against a 400-line budget —
down roughly 150–200 from this design's first draft, entirely from the three
cuts recorded in Decisions 5 and 7. The proposal's estimate was ~1,800 across
six slices; **seven slices**, and here is where its plan needs revising, with
the reason in each case rather than silently:

1. **Slice 1 stays one slice.** An earlier draft split it into 1a (*what the
   file is*) and 1b (*how the bytes move*), on the grounds that ~570 lines do
   not fit one 400-line review. That was budget arithmetic dressed as risk
   isolation, and it failed its own test: 1a alone is a package with no
   caller, no step and no golden change — dead code that cannot be reviewed
   for whether it does its job. With Decision 5's floor walk and the
   `Elements` guard both cut, what remains is ~420, and the split's original
   motivation goes with them. Contrast 2a/2b, which **is** risk isolation and
   stays split: 2a carries a correctness fix that 2b depends on.
2. **Slice 2 splits into 2a/2b**, and 2a carries scope the proposal does not
   have. Decision 2 is a correctness fix without which slice 2 ships a file
   git does not carry.
3. **Slice 2b needs a `== plan ==` hand-edit**, which the proposal assigned
   to 3a alone. Removing `doctorConfigStep` is a plan change.
4. **Slice 4 grows.** The `stage` struct cannot express a locally-resolved
   binary today, and the staged paths are repository-relative. Both are
   prerequisites, not extras.
5. **Slice 4 remains independently mergeable and can still move ahead of 3b**
   — it touches `internal/cli`, `internal/tool` and `internal/project` and
   nothing else in this change does. If 3b overruns, 4 goes first with no
   reordering.
6. **Slice 5's `Layer` is a dependency of 2a's rendering.** The owned
   factory's parameter list comes from `preset.Layers`. Two orderings work:
   land the `Layer` type and an empty `Layers` in 2a (~30 lines) and the
   Next.js/Expo layers in 5, or move 5 before 2a. The first is recommended —
   it keeps the framework goldens' `-update` regeneration in one slice.

## Open Questions

- [x] **Decision 3's split-layout resolution failure — measured, and it
      reproduces.** A fixture with the plugin installed only under `Source/`
      and the owned config at `Root/.dharness/` fails with
      `ERR_MODULE_NOT_FOUND`; the byte-identical import from `Source/`
      resolves. The factory shape stands. The motivating repository is worse
      than the plain case: its root carries no `package.json` and an **empty**
      `node_modules/`, so the resolution walk has a stop that looks valid and
      holds nothing. Recorded in `docs/learning-log.md`, 12 August 2026.
- [x] **Where the owned ESLint config lives — settled, and it keeps the
      factory.** The alternative considered was writing it into `p.Source`
      instead, where a bare specifier resolves and the owned file could
      collapse to a plain array (~300–400 lines lighter, Decision 3 gone with
      it). **Rejected, against code rather than preference.** `ownedFrom`
      builds every owned path as `filepath.Join(p.Root, project.Dir, name)`
      (`files.go:188-189`): *every* dharness-owned file lives in
      `Root/.dharness/`, without exception. What `wireFallowExtends` writes
      into `p.Source` is not an owned file — it is the **project's**
      `.fallowrc.json` carrying a one-line pointer at the owned one
      (`files.go:201-206`). There is no precedent for an owned file outside
      `.dharness/`, and inventing one would put it outside what `EnsureDir`
      manages and what Decision 2's allow list covers. Decision 3's split —
      bare specifiers in the project's config, injection into the owned file —
      *is* the shipped pattern, applied to module resolution.
- [x] **The four marker strings — settled in slice 3a, as this document
      required.** The spelling proposed in Decision 4 was adopted verbatim:
      `presetBegin`'s exact grammar, so a reader who has seen one region
      recognises the other with no new documentation. They ship into every
      adopting repository and cannot be changed cheaply afterwards, which is
      why the question was raised — and the answer was that a string reasoned
      through once does not benefit from being reopened at implementation
      time. Framework-presets' equivalent question stays open there.
- [x] **The gate stage's placement — measured in slice 4, and the measurement
      reversed it.** Provisional was *last*; ESLint measured **cheapest of the
      four**: 1008 ms median against react-doctor's 2959, fallow audit's 2102
      and fallow dupes' 1398 (three runs each on a reference project, recorded
      in `docs/learning-log.md`, 12 August 2026). It runs **first**. Local
      resolution is most of the reason — it skips the package-manager round
      trip the remote-executed stages pay every run. This is exactly what the
      merge condition existed to catch: reasoning about ESLint's cost profile
      in isolation would have left it last.

      **A second question the same measurement raised, deliberately left
      open**: fallow measured cheaper than react-doctor on that project too,
      which contradicts the existing order read literally. That order was
      argued from *scaling* — `--staged` bounds react-doctor to the diff,
      fallow's repository-graph build gives it a floor — and a five-file
      reference project has a graph too small to test the claim. Wall-clock on
      a toy repository is the wrong instrument, so the order stands until
      something measures the right thing.
- [ ] **The call-expression distribution stays unmeasured**, as the proposal
      and spec both record. Nothing in this design depends on the number:
      `defineConfig` splices, everything else delegates, and both paths ship
      in 3a/3b regardless.
- [x] **`Layer.Binding` collisions across two matched presets — closed in
      slice 5.** `TestNoBindingIsContributedTwice` ships, and the shipped
      bindings are `dharnessNext` and `dharnessExpo`. The namespacing rule
      that arrived for a different reason — a bare binding collides with the
      *project's* own import and produces a SyntaxError — makes the
      cross-preset case harder to reach as a side effect, since every binding
      now carries the same prefix and differs only in its tail. The original
      note follows, unchanged, because its argument is what the test asserts:
- [ ] **`Layer.Binding` collisions across two matched presets** are not
      possible today — `nextjs` and `expo` contribute one each, with different
      bindings, and both are Source scope so a repository matching both is a
      project depending on `next` and `expo` simultaneously. A registry test
      (`TestNoBindingIsContributedTwice`) asserts the branch is unreachable,
      in `TestNoScalarKeyIsContributedTwice`'s exact shape and for its exact
      reason: a future fifth preset should degrade visibly rather than emit a
      duplicate `const`.

## Key Learnings

1. A step whose satisfying condition dharness stops writing becomes
   permanently unsatisfiable, so it must be deleted rather than emptied.
2. Node resolves a bare module specifier by walking upward from the importing
   file, never from the project root.
3. The owned directory's gitignore is an allow list written once at adoption,
   so a newly owned file stays untracked in every already-adopted repository.
4. Tree-sitter reports JavaScript comments as named array elements, so the
   position rule walks *non-comment* named children and then steps back over
   the comment run immediately above its target.
5. Verifying spliced bytes before writing them removes the window in which an
   unparseable config exists on disk at all.
6. A guard whose expected value depends on which branch the caller took only
   restates the caller's decision. What needed writing down was the branch —
   the replace path — not an assertion over it.
7. Ordering two config layers is only worth machinery when the layers
   intersect. Two layers that declare disjoint rule sets merge to the same
   result in either order, so the cheapest correct position is the one that
   needs no analysis to find.
