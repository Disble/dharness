# Proposal: framework-presets

Exploration: `openspec/changes/framework-presets/exploration.md`, Engram
`sdd/framework-presets/explore`. Read its **Orchestrator correction** section
first — it overturns the exploration's own conclusion about `extends`, and this
proposal is built on the correction, not on what it corrects.

## Intent

dharness refuses to know what kind of project it is in, and the cost is
concrete: `ownedFilesStep.Apply` (`internal/setup/steps.go:117-139`) writes
`.dharness/fallow.jsonc` as a **permanently empty commented object**. dharness
already runs that step unconditionally, already owns that file outright, and
writes nothing into it that describes this repository. Everything a framework
would let it say goes unsaid, so the answer is handed to the agent by default
rather than by judgement.

The framing is therefore **the owned file stops being empty** — not a reduction
in delegated steps. Walking all seven currently-delegated steps against what a
preset could answer, none of them flips to dharness-executed: `agentSkillStep`
and `architectureStep` are Conducción and Intención (§21), `boundariesOwnerStep`
and `legacyLintConfigStep` inspect the *project's own* files, and the three
`extends`/`hookInstall` steps turn on preconditions that are not framework
facts. The plan stays eleven steps. The value is a new dimension inside one of
them.

A preset carries **facts and decisions**. Facts are observable framework
properties (generated directories, entry patterns, source layout). Decisions are
opinionated configuration for that framework — which rules, which thresholds,
which suggested boundaries. Facts alone would leave dharness opining on the
harness and never on the code, which is the "configuración descafeinada"
this change exists to end.

## The hard constraint, and why writing the file is not enough

Measured against fallow 3.14.0: **`extends` replaces a key, it does not merge
it.** A parent declaring `ignoreDependencies` or `boundaries` is honoured until
the child declares its own; from then on the parent's value is discarded whole,
with no error and with the `extends` line still reading as correct.

The motivating repository's own `.fallowrc.json` already declares
`"ignorePatterns": ["wailsjs/**"]`. A preset writing that key into the file
dharness owns would therefore do **nothing, silently, in the exact repository
the example came from**. Writing into the owned file is necessary and not
sufficient.

`boundariesOwnerStep` (`steps.go:231-260`) already gives one key the correct
treatment: write it in dharness's file *and* report that the project declares
the same key, because only one is in effect and the configuration does not say
which. This change generalises that from one key to N.

## Scope

### In scope

1. **Preset registry with per-framework packages.** One package per framework
   (`wails`, `expo`, `nextjs`, `generic`) exposing Identity,
   `Detect(p) (matched bool, evidence string)`, and a versioned `Manifest`. One
   switch in a factory. `generic` is not filler: it makes today's behaviour one
   preset among several rather than something replaced, so a project matching no
   framework behaves **exactly** as it does today.
2. **Versioned manifest carrying evidence beside each claim.** Schema
   `dharness.preset/v1`. Every fact is `{value, because}`, where `because` names
   the observable that justifies it — the claim about `wailsjs/` is believed
   *because `wails.json` exists and Wails documents that path as generated*. A
   baked-in fact goes stale between binary releases; evidence makes a stale claim
   auditable instead of silent.
3. **Preset facts are defaults the repository overrides, never assertions
   (§07).** dharness never asserts that `wailsjs/` exists. It writes an ignore
   pattern that is inert where the directory is absent. Precedence, stated once
   and settling a whole class of questions:
   **project declaration > detection > preset > global default.**
4. **`ownedFilesStep` writes the preset's contributed keys** into
   `.dharness/fallow.jsonc`, which is the only file dharness owns outright and
   therefore the only place preset content can land at all (§03).
5. **Generalised contributed-key collision reporting.**
   `boundariesOwnerStep` widens from `boundaries` to the set of keys the matched
   presets contribute. Unsatisfied when that set intersects the top-level keys of
   the project's own `.fallowrc.json`; delegated, naming each colliding key and
   both values; the run continues (§20).
6. **Multi-preset composition by scope.** Wails signals at Root scope
   (`wails.json`), Next.js and Expo at Source scope (`package.json`
   dependency). The registry returns contributions from more than one preset,
   keyed by which of `Root`/`Source` each belongs to. The existing Root/Source
   split in `Discover` is the seam; nothing new is invented to hold it.
7. **`folder-ownership` reclassified from preset-decided to detection-derived.**
   See the decision below.
8. **A golden pin over `Plan()` output before any of the above.** None exists
   today (checked `setup_test.go`, `steps_root_test.go`), so "generic reproduces
   current behaviour exactly" has no mechanism to lean on. One is built first.

### Out of scope — named, not dropped

| Deferred | Why |
|---|---|
| Multi-framework monorepos | Already failing closed; see decision 3 |
| Reducing the number of delegated steps | Measured: none flips. The framing was wrong, not the target |
| A preset deciding `boundaries` zones | Zones encode intent; §21 keeps that with the agent. A preset may *seed* the prompt |
| Merging preset keys into the project's own config files | §03: dharness writes its own file and adds one line to the project's |
| Pushing this knowledge upstream into fallow or react-doctor | Checked per `CLAUDE.md`'s first rule: no command or flag answers it, so it is not actionable |
| Presets beyond the four | Four is the bounded first slice, not the ceiling |
| A JSONC parser | The product is stdlib-only. The design is built so none is needed — see decision 4 |

## The four decisions

### 1. The goal is reframed, not the step count

Stated above. `Plan()` returns eleven steps before and after. The success
criterion is what `.dharness/fallow.jsonc` contains, plus what the collision step
reports — never a smaller plan. Recorded because the brief's original framing
does not survive its own evidence, and re-deriving that later would cost the
same walk again.

### 2. `folder-ownership`'s default is detection-derived, not preset-carried

Whether a project publishes through barrels is **observable from the tree** —
do `index.ts`/`index.tsx` barrels exist? — so it is a detection answer, not a
framework fact. A Next.js project may or may not use barrels; the framework does
not decide it. Putting it in a preset would encode a guess where a direct signal
exists (§09).

Concretely:

- `offByDefault` (`internal/setup/plugin.go:89-91`), a package-level map with one
  entry, **is removed as a global constant.**
- `DefaultSeverity(rule string)` becomes `DefaultSeverity(p project.Project, rule string)`.
  Its contract is unchanged: it still answers only for *a rule the project has not
  chosen a severity for itself* (§05). The single call site is
  `doctorConfigStep.Apply` (`steps.go:312`), which already writes only into the
  `else` of `if _, chosen := config.Rules[id]; !chosen`.
- Barrel presence is asked of git, matching the `Discover` precedent of asking
  the tool rather than walking the tree.
- The comment block at `plugin.go:74-88` is the recorded rationale for the
  current default and its measurement (eight non-actionable findings on one
  repository). It is rewritten, not deleted: the measurement stays, the
  conclusion "therefore off everywhere" becomes "therefore off where the tree has
  no barrels".

**Honest limit, found while verifying this.** `doctorConfigStep.Satisfied`
returns true as soon as `RulesPackage` appears in `plugins`, so severities are
written **once**, at first adoption. A project that adds barrels later will not
have the rule switched on by a subsequent `sync`. Widening `Satisfied` to
re-check severities is rejected: dharness cannot distinguish "the project chose
`error`" from "dharness wrote `error`", so re-deriving would overwrite a
deliberate choice (§05). The derived value is therefore a **first-write default
only**, and that limit is stated rather than discovered later.

### 3. Multi-framework monorepos are explicitly deferred, because they already fail closed

`AmbiguousSourceError` (`internal/project/discover.go:17-27, 90`) already refuses
to guess between multiple independent JS projects and names the candidates,
unless the directory the caller is standing in settles it. A repository with two
frameworks in two independent JS projects therefore **cannot reach a plan today**
without the caller answering which one they mean — and by then exactly one
Source is resolved, so exactly one Source-scope preset applies.

Presets change nothing about that resolution and inherit it whole. Deferring
costs nothing and adds no new failure mode.

**This is not the same question as item 6.** Single-repo multi-preset
composition — Wails at Root scope with Next.js at Source scope — is one Root and
one Source, is in scope, and is the case the reference adapter pattern does not
cover, because its agents are mutually exclusive and frameworks are not.

### 4. How a preset-contributed key survives a project that declares it

**The check.** The manifest declares the keys the preset contributes. The
collision set is the keys that also appear at the top level of the project's own
`.fallowrc.json`.

> **Orchestrator correction.** The proposal originally specified parsing that
> file with `encoding/json` for its top-level key names. **That does not work,
> and it fails on the file this whole change is motivated by.** `.fallowrc.json`
> is JSONC: the reference repository's own copy opens with
> `// This file is JSONC: fallow reads these comments; generic JSON tools may
> not.` on line 2, and `json.Unmarshal` returns
> `Expecting property name enclosed in double quotes: line 2 column 3`.
>
> The precedent already in the codebase is the correct mechanism and exists for
> exactly this reason. `declaresBoundaries` (`internal/setup/files.go`) tests for
> the **quoted key** — `"boundaries"` with its quotes — rather than the bare
> word, and its comment records why: a config correctly pointing at dharness
> carries the sentence "Architecture boundaries live in the file dharness owns"
> in a comment, and a bare substring answers yes to that sentence. Verified: the
> quoted form matches zero times in that file while the bare word matches once.
>
> Generalise `declaresBoundaries(path)` into `declaredKeys(path, candidates)`
> over the same quoted-key test. `extendsWired` and `architectureStep` already
> use textual tests on these files, so this is the third instance of an
> established pattern rather than a new compromise.

This is what keeps the design stdlib-only, and it is the reason the "no JSONC
parser" line under Dependencies holds: dharness never parses its own JSONC file
to learn what it contributed — the manifest already says so — and it never
parses the project's either. Where the project's config is code rather than
data, the keys cannot be read at all; that case describes and continues,
following the precedent already set for `doctor.config.ts`.

The verdict is decided structurally from the file's bytes, never from prose and
never from a model (§17, and `CLAUDE.md`'s second rule). The honest limit of a
textual test is that a comment which quotes the key would false-positive; the
existing comment already names that case and calls it a file written to defeat
the check rather than an accident.

**The response.** dharness writes the contributed key into its own file **anyway**,
and reports the collision. It does not suppress the write. Suppressing would make
dharness's file misstate what it recommends, and would erase the signal the
moment the project later removed its own key.

**The exit.** The step is delegated and resolves the way
`boundariesOwnerStep` already resolves: the agent moves dharness's values into the
project's key, or removes the project's key and keeps dharness's. Either answer
is valid; having both is not. The intersection empties, the step disappears
(§15), and nothing is recorded to make that stick — it is re-derived every run
(§07).

**Applied to the motivating repository:** `ignorePatterns` collides. The preset
alone would have done nothing there; the collision step is what makes the change
actually land in the repository the example came from. That is the honest
statement of where the value is.

## Verified after the proposal was written

The proposal was written without a way to check whether Wails can relocate
`wailsjs/`, and correctly refused to assert an answer. **The orchestrator has
since verified it against Wails' own source, and the answer makes the override
path load-bearing on day one rather than hypothetical.**

`wails.json` carries a `wailsjsdir` key —
`WailsJSDir string \`json:"wailsjsdir"\`` in `v2/internal/project/project.go`,
also exposed as the `-wailsjsdir` flag ("Directory to generate the Wails JS
modules"). Its default is set in `v2/pkg/commands/build/base.go`:

```go
if options.WailsJSDir == "" {
    options.WailsJSDir = filepath.Join(cwd, "frontend")
}
```

And relocating it is not a fringe case: Wails' own SvelteKit guide instructs
users to set `"wailsjsdir": "./frontend/src/lib"`.

Three consequences for the design:

1. **Item 3 is load-bearing, not a safeguard.** A whole framework combination
   ships with the directory moved, on official advice. A preset that asserted
   `wailsjs/` would be wrong for those projects on day one.
2. **The Wails preset has a direct signal and must use it (§09).** Read
   `wailsjsdir` from `wails.json`; fall back to the documented default only when
   the key is absent. Inventing a proxy where a direct signal exists is the
   mistake this repository has already made and recorded.
3. **The evidence string for that fact is now writable**, and should name both
   the key and the fallback rather than the path alone.

The risk row for this item is resolved and should be struck by `sdd-design`.

## Capabilities

### New capabilities

- `framework-presets`: detection with evidence, versioned manifests, the
  registry, and composition by Root/Source scope.
- `owned-config-contribution`: what dharness writes into the file it owns, and
  how a contributed key the project also declares is detected and reported.
- `rule-severity-derivation`: deriving a rule's first-write default from the
  tree rather than from a global constant.

### Modified capabilities

None. `openspec/specs/` does not exist in this repository.

## Affected areas

| Area | Impact | Description |
|---|---|---|
| `internal/preset/` (new) | New | Registry, `Manifest`, `Detect`, scope; one package per framework |
| `internal/project/detect.go` | Modified | `detectFramework` joins `detectPackageManager`/`detectTestRunner`; barrel detection for decision 2 |
| `internal/project/discover.go` | Modified | Root-scope signal (`wails.json`) alongside the Source-scope lockfile signal |
| `internal/setup/steps.go:117-139` | Modified | `ownedFilesStep.Apply` writes contributed keys instead of an empty object |
| `internal/setup/steps.go:231-260` | Modified | `boundariesOwnerStep` generalises from one key to N |
| `internal/setup/steps.go:312` | Modified | `DefaultSeverity` call site gains the project |
| `internal/setup/plugin.go:57-101` | Modified | `offByDefault` removed; `DefaultThresholds`/`DefaultSeverity` parameterised |
| `internal/project/evidence.go:22-31` | Modified | `dirIgnore` is an allow list; any new committed file dharness owns must be named there or it is silently ignored |
| `internal/setup/setup_test.go` | Modified | New golden pin over `Plan()`; `generic` must reproduce it byte-for-byte |

## Slice plan

`auto-chain` / `stacked-to-main`, 400-line budget. **400-line budget risk: High**
— the whole change is well over 1,000 lines, so chaining is certain, not
contingent. Order is load-bearing.

| Slice | Content | Rough lines | Why here |
|---|---|---|---|
| 1 | Golden pin over `Plan()` and step output | ~150–250 | The safety net for every later slice. Nothing else can be proven safe first |
| 2 | Registry, `Manifest`, `Detect`, `generic` preset only | ~250–350 | Behaviour must come out byte-identical against slice 1's pin. Pure structure, zero framework knowledge |
| 3 | Contributed-key collision step (generalises `boundariesOwnerStep`) | ~200–300 | **Before any real preset.** Writing preset keys without the collision report is precisely the silent no-op this change exists to prevent |
| 4 | `folder-ownership` reclassification and `DefaultSeverity` parameterisation | ~150–200 | Self-contained; independent of 2–3; can move earlier if 3 overruns |
| 5 | Wails, Expo, Next.js manifests and scope composition | ~300–450 | Split per framework if the diff exceeds budget; each preset is independently mergeable once 2 and 3 exist |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| A preset key silently no-ops because the project declares it | **Certain, in the motivating repo** | Slice 3 lands before slice 5; the collision set is checked structurally, not by reading prose |
| `generic` does not reproduce today's behaviour exactly | Medium | Slice 1's golden pin exists solely to make this falsifiable before any preset is written |
| A baked-in framework fact goes stale between releases | Medium | Evidence beside each claim; facts are overridable defaults, never assertions (§07) |
| Wails' output path assumption turns out wrong | Unknown — unverified | Nothing asserts it; the override path handles it either way. Do not remove this row until it is verified |
| Scope composition returns conflicting values from two presets | Medium | Precedence is fixed and stated: project > detection > preset > global. Two presets contributing one key is a design question for `sdd-design`, not left implicit |
| Collision step becomes a permanent nag | Low | It is delegated with two valid resolutions and disappears when the intersection empties (§15) |
| A new committed preset file is silently gitignored | Low | `dirIgnore` is an allow list by design; any new owned file must be named in it |
| Scope creep into monorepo support | Medium | Deferred explicitly, on an existing failure mode that already covers it |

## Principles that accept this change

- **§01 / §02** — checked first: no fallow or react-doctor command answers "what
  framework is this and what should it ignore". The work belongs here, and the
  part needing judgement (zones, collisions) goes to the agent as a prompt.
- **§03** — preset content lands only in `.dharness/fallow.jsonc` and
  `.dharness/rules.json`. The project's files gain nothing but the existing
  `extends` line.
- **§05** — `DefaultSeverity` still answers only where the project chose nothing.
- **§07** — preset facts are defaults the repository overrides; the collision set
  is re-derived every run and never recorded.
- **§09** — barrel presence is a direct signal, so no proxy heuristic is invented
  for it.
- **§15** — the collision step disappears once resolved; there is no separate
  install and maintenance path.
- **§17 / §20 / §21** — the verdict is JSON key presence; a collision lowers a
  rung rather than blocking; the prompt goes to the agent and the ladder ends
  there.

## Rollback plan

One chain of branches, no persisted state, no migration, no on-disk format
change beyond content inside files dharness already owns and rewrites every run.
Revert the merge commits in reverse order: `.dharness/fallow.jsonc` returns to
the empty commented object on the next `sync`, `offByDefault` returns, and an
older binary simply re-derives the older plan (§07). Nothing written by a newer
version needs undoing by hand.

## Dependencies

None. The product stays stdlib-only — no JSONC parser, no YAML parser, no
framework SDK. ooze remains the sole development dependency and stays confined to
staged Go mutation tooling. `go test ./...`, `go vet ./...`, `gofmt -l .` and
`go run ./tools/mutationstaged` are the whole verification surface.

## Success criteria

- [ ] In a project matching no framework, `Plan()` output and every written file
      are byte-identical to today, proven by the slice 1 golden.
- [ ] In a Wails project, `.dharness/fallow.jsonc` is no longer an empty
      commented object.
- [ ] In a project whose own `.fallowrc.json` declares a key a matched preset
      contributes, `sync` names that key, shows both values, and the run
      continues without rollback.
- [ ] Resolving the collision either way makes the step disappear on the next
      `sync`, with nothing recorded.
- [ ] Every manifest fact carries an evidence string naming the observable that
      justifies it.
- [ ] `folder-ownership` is written `error` in a barrel-publishing tree and `off`
      in one without, and neither overwrites a severity the project chose.
- [ ] A Wails root with a Next.js source receives contributions from both
      presets, resolved by scope.
- [ ] `go test ./...`, `go vet ./...`, `gofmt -l .` and
      `go run ./tools/mutationstaged` all clean on every slice.
