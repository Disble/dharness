# Proposal: eslint-integration

Exploration: `openspec/changes/eslint-integration/exploration.md`, Engram
`sdd/eslint-integration/explore`. Read its **The parser, settled by the
orchestrator** section: it is in the file and not in the Engram copy, and it
closes the change's central open question.

## Intent

**dharness's commit gate does not run dharness's own lint rules.** Measured
against react-doctor 0.5.7, plugin installed and resolvable, `maxFileLines: 1` —
a limit no file can satisfy:

| invocation | plugin rules | exit |
|---|---|---|
| the gate's flags without `--staged` | `max-file-lines` and `require-jsdoc` fire | **1** |
| the gate's flags with `--staged` | "No issues found" | **0** |

react-doctor's own native rules do run under `--staged`, so what the flag drops
is the plugins declared in `doctor.config.json`. **The mechanism is
unconfirmed** — a temp-directory plus unresolvable-plugin message appeared twice
and did not reproduce. Nothing downstream may assert the cause.

So `installStep` installs `dharness-eslint-plugin`, `doctorConfigStep` writes six
severities, and the gate enforces none of them. A severity written where it
cannot fire is worse than absent, because it reads as coverage.

## The departure — stated precisely, because the brief's framing was wrong

The change was framed as breaking §03. It is not. §03 was amended twice
(9 and 10 August 2026) to cover exactly "dharness writes its own file, the
project's gains one reference", and `fallowExtendsStep` / `lefthookExtendsStep`
are that pattern.

The real departure is narrower. Those two steps **only write a project file when
it does not exist yet**: the instant the target exists, `Delegated(p)` returns
true (`steps.go:188-198`, `224-234`) and the merge becomes the agent's. Virtually
every ESLint 9 project already has an `eslint.config.js`, so applying that
precedent literally would delegate nearly every time. **Splicing into a file the
project wrote has no precedent in this codebase** — the delimited-region
technique in `owned.go` (`presetBegin`/`presetEnd`) is reserved for files
dharness itself created.

The existing spec requirement (`openspec/specs/setup/spec.md:202`) names
`.fallowrc.json` and `lefthook.yml` explicitly and does not generalise, so this
change **adds** a requirement about a third file rather than overturning one.

## Decisions taken before this proposal — implemented, not re-opened

1. dharness couples to ESLint and splices into the project's `eslint.config.js`.
   ESLint ships with the frameworks and is already in many projects' pre-commit;
   standing beside it would duplicate a pass the project already runs.
2. The layering: `...frameworkRecommended, ...dharness, ...projectCustom`.
   dharness strengthens the framework's defaults and never resurrects a rule the
   project deliberately disabled (§05).
3. The parser is `github.com/odvcencio/gotreesitter` — pure Go, `CGO_ENABLED=0`,
   byte ranges per node, embedded grammars, verified against the reference
   repository's real `eslint.config.js`. Official tree-sitter bindings need cgo
   and would cost the single-binary cross-compile.
4. **The first product dependency is accepted**, deliberately. dharness has been
   stdlib-only (`AGENTS.md`, `openspec/config.yaml:5-8`); ooze stays dev-only.
   Fallback if the library stalls: cgo with the official bindings, paying the
   cross-compile. Recorded as a deviation, not slipped in.
5. `eslint-config-next` 16.3.0 and `eslint-config-expo` 57.0.1 are published by
   the frameworks themselves and versioned with them, so installing them is not
   dharness inventing a convention.

## The six things this proposal resolves

### 1. The splice

dharness owns `.dharness/eslint.config.js` (§03). The project's config gains two
**separately-marked** regions, each replaced whole on re-run:

```js
// dharness:eslint-import begin — rewritten by `dharness sync`; edits here are lost.
import dharness from "./.dharness/eslint.config.js";
// dharness:eslint-import end
```

```js
  // dharness:eslint-layer begin — rewritten by `dharness sync`; edits here are lost.
  ...dharness,
  // dharness:eslint-layer end
```

Two distinct marker names, not one pair used twice: each region is individually
addressable, so a re-run replaces exactly its own bytes and nothing else. The
path is computed the way `ownedFrom` computes it, from the declaring directory.

**Position rule.** Insert at **index 0**, always. The load-bearing invariant
is `dharness < projectCustom` (§05), and the array's first element satisfies
it unconditionally — nothing the project wrote can be shadowed by dharness,
because dharness sits underneath all of it. An earlier revision made the
position conditional on recognising a framework spread and inserting after
it; that is withdrawn, because flat config merges last-wins *per rule* and
the framework configs dharness contributes declare no `dharness/*` rule, so
the relative order has no observable effect.

**Comments survive.** The probe showed comments are array elements with their own
byte ranges. The insertion offset is the start of the line of the earliest byte
in the contiguous comment run preceding the anchor element, so a comment stays
attached to what it describes.

**Idempotency proof.** Two `sync` runs produce byte-identical output; the second
run's `Satisfied` is already true. Test: parse the result and assert exactly one
region of each marker kind, and that the default-export array has exactly one
more element than the input.

### 2. The refusals

| Case | Answer | Why |
|---|---|---|
| `.js`/`.mjs`/`.cjs`, default export is an array literal, no `ERROR` node | **write** (splice) | the tree is understood; byte ranges make the edit surgical |
| No ESLint config at all | **write** the whole file | the `wireFallowExtends` precedent exactly: write-if-absent |
| `eslint.config.ts`/`.mts`/`.cts` | **delegate** | a second grammar plus typed helpers whose semantics are the project's. Same reasoning §03's amendment already applies to `.js`/`.mjs`/`.cjs` Stryker configs |
| Default export is a call — `tseslint.config(...)`, `withNuxt(...)`, anything not named below | **delegate** | whether it returns an array cannot be known without executing project code (§17, §21) |
| Default export is `defineConfig([...])` where `defineConfig` is imported from `"eslint/config"` and its first argument is an array literal | **write** (splice into that array) | see the orchestrator correction below |
| Legacy `.eslintrc.*` only | **delegate**, in `legacyLintConfigStep`'s exact shape | eslintrc cannot spread an ESM module in either direction; migrating is the project's decision |
| The parse yields any `ERROR` node covering the default export | **refuse, delegate** | **a partial tree from an error-tolerant parser is not permission to edit a file dharness did not understand** |
| A marker pair is present but malformed (missing end, end before begin) | **refuse, delegate** | guessing at a half-written region is how a config gets corrupted |

CRLF, BOM and encoding are preserved: the splice is byte-surgical and adopts the
line ending observed on the anchor line.

> **Orchestrator correction — `defineConfig` is the documented shape, and
> refusing it would refuse almost everyone.**
>
> The risk table below flags "ESLint's own config is a call expression in most
> real projects" as unmeasured. It has since been measured, and it is worse
> than a risk: **ESLint's own documentation gives the call as the recommended
> form.** Its basic example, verbatim:
>
> ```js
> // eslint.config.js
> import { defineConfig } from "eslint/config";
>
> export default defineConfig([
>   { rules: { semi: "error", "prefer-const": "error" } },
> ]);
> ```
>
> With "a call always delegates", slice 3b — the splice, the only destructive
> edit, the entire justification for taking a parser dependency — would apply
> to a shrinking minority of projects while dharness delegated on the shape
> ESLint tells people to write.
>
> The rule is therefore narrower than "no calls". `defineConfig` from
> `"eslint/config"` is a **documented identity over an array**, and its first
> argument in the documented form is an array literal whose byte range
> tree-sitter reports directly. Recognising that one binding is the same class
> of knowledge as knowing `wailsjsdir`'s default: a fact the tool publishes
> about itself, not an assumption about the project.
>
> Three conditions, all required, or it delegates: the callee resolves to an
> identifier imported from `"eslint/config"`; the call has an array literal as
> its first argument; and the parse of that argument carries no `ERROR` node.
> Any other call — `tseslint.config`, `withNuxt`, a locally defined helper, or
> `defineConfig` imported from anywhere else — still delegates, because
> nothing published says what it returns.
>
> The spec must pin both halves: that the documented shape is spliced, and
> that a lookalike from another module is refused.

### 3. The six rules get one home

**`.dharness/eslint.config.js` becomes the single home for the six severities.**
`doctorConfigStep` stops writing `dharness/*` severities and stops declaring
`RulesPackage` under `plugins` in `doctor.config.json`. `.dharness/rules.json`
thresholds are unchanged — they are read by the plugin, not by a severity map.
`DefaultSeverity` keeps its contract, including the barrel-derived
`folder-ownership` default and its recorded first-write-only limit.

**Residue in already-adopted repositories is left as found.** `doctor.config.json`
is the project's file (§03), and dharness cannot tell "dharness wrote error" from
"the project wrote error" — the same limit already recorded at `plugin.go:94-100`.
Those entries are inert under the gate's `--staged` invocation. This is reported,
never machine-removed.

### 4. The gate

- **Placement is decided by measurement in `sdd-design`, not asserted here.**
  §12 fixes the rule (cheapest first, first failure cuts the rest); the working
  hypothesis is ESLint before react-doctor, because ESLint over an explicit
  staged file list has the lowest floor. This repository has already paid for
  asserting an ordering it had not measured.
- **Resolution is the project's, not the remote executor's** — a deliberate,
  recorded exception to §03's 10-August amendment. An ESLint flat config is code
  that imports the project's own plugins and framework configs; a transient
  `eslint@latest` environment cannot resolve them, which is the very failure
  shape suspected behind react-doctor's `--staged` plugin drop. The seam is
  `p.LocalBinary("eslint")`, as `hookManager` already uses for lefthook.
- **No `--cache` in this change.** It writes `.eslintcache` into the project's
  tree, which §03 would have to account for, and it is stale-prone across
  branches. The staged file list is the larger win and it is already available
  (`check.go:52`). Caching is a measured optimisation, deferred.
- **No ESLint installed → the stage does not run** (§13), named in the output
  (§16), never a block (§20). dharness does not install ESLint: that is the
  project's toolchain choice.
- The setup step keys on the **config file**; the gate stage keys on the
  **binary**. Two different questions, answered separately.
- react-doctor stays, for its native rules.

### 5. A preset contributes a package and a layer

`Manifest` today is `{Schema, Facts, Seeds}` (`preset.go:117-121`) and `Fact` is a
fallow config key. Add a third contribution kind carrying the package to install,
the binding to import, and its `Because`. No schema bump: the manifest never
leaves the binary, so the field is additive; `Validate` gains the same
evidence-required rule the other two kinds already have.

The seam already exists and already rolls back correctly:
`integrationPackages()` (`steps.go:88-92`) becomes preset-aware, and
`installStep.Apply` already snapshots `PackageStateFiles()` and compensates by
removing exactly what that run added.

### 6. Blast radius

A malformed edit breaks the project's whole lint and often its pre-commit.

- `Writer` snapshots contents and mode before the first byte changes, and `Undo`
  restores in reverse — but **only on error**. A run that *succeeds* and leaves an
  unparseable config is the real hazard.
- **The added guard: re-parse inside the transaction.** After splicing, parse the
  resulting bytes with the same parser and assert no `ERROR` node, exactly one
  region of each marker kind, and an array with exactly one more element. Any
  failure returns an error from `Apply`, which `applySteps` already turns into
  `Undo` and a byte-exact restore. This converts "the edit broke the project"
  into an ordinary step failure the existing machinery handles.
- **Rejected: running ESLint to validate the splice.** It costs a Node process
  during `sync`, and a config can legitimately fail to load for reasons the
  splice did not cause. The structural re-parse is the verdict — from bytes, not
  from prose and not from a model (§11, §17, `CLAUDE.md`'s second rule).
- If the gate later meets an unparseable config, ESLint's own non-zero exit
  propagates untouched (§11) with its `--help` pointer (§16). No repair attempt.
- dharness never deletes a project element. It inserts, and on re-run replaces
  only bytes strictly between its own markers.

## The golden constraint cannot hold as briefed — and must not

The brief requires `internal/setup/golden_test.go`'s generic golden to stay
byte-identical. **Verified: it cannot.** `renderGolden` (`golden_test.go:132-150`)
walks *every* step in `Plan()`, satisfied or not, emitting ID, `Satisfied`,
`Delegated` and `Describe` for each. Adding a step necessarily changes the
fixture, and `TestGenericMechanismHasNoUpdatePath` (`golden_test.go:96-118`)
forbids wiring `-update` into it.

That is the mechanism working, not failing. Decision 7 removed the update path
precisely so a generic-golden change is **hand-authored and reviewed as its own
diff** rather than regenerated. What does hold, and is the constraint worth
enforcing:

- The `== tree ==` region for a generic project is **unchanged by the splice
  step**: with no ESLint config, the step is satisfied and writes nothing.
- The `== tree ==` region **does** gain `.dharness/eslint.config.js` in the slice
  that writes it, and `doctor.config.json` loses the severity block in that same
  slice.
- Each slice hand-edits the fixture for exactly its own change, so every PR is
  self-contained and green.

## Capabilities

### New capabilities

- `eslint-config-splice`: parsing, the position rule, the marker regions, the
  refusal matrix, idempotency, and verify-then-rollback.
- `eslint-gate-stage`: where ESLint runs, how it resolves, and when it does not
  run at all.
- `preset-layer-contribution`: a preset contributing an installable package and a
  config layer.

### Modified capabilities

- `step-delegation` (`openspec/specs/setup/spec.md:174`): gains a requirement for
  a step that edits an existing project file. The existing requirement at line
  202 names `.fallowrc.json` and `lefthook.yml` and is not contradicted.

## Affected areas

| Area | Impact | Description |
|---|---|---|
| `internal/jsconfig/` (new) | New | tree-sitter parse, element ranges, array-literal and `ERROR` detection |
| `go.mod`, `go.sum`, `AGENTS.md` | Modified | First product dependency, recorded as a deviation |
| `internal/setup/steps.go` | Modified | New `eslintExtendsStep`; `doctorConfigStep` stops writing severities; `integrationPackages()` preset-aware |
| `internal/setup/setup.go:48-62` | Modified | `Plan()` gains one step |
| `internal/setup/files.go` | Modified | Config-name detection and the splice writers, beside `wireFallowExtends` |
| `internal/setup/plugin.go` | Modified | Severity map moves to the owned ESLint config |
| `internal/preset/preset.go:117-154` | Modified | Third contribution kind, plus `Validate` |
| `internal/preset/nextjs.go`, `expo.go` | Modified | `eslint-config-next` / `eslint-config-expo` layers |
| `internal/cli/check.go:61-78` | Modified | ESLint stage, placement measured |
| `internal/tool/tool.go` | Modified | ESLint invocation; project-resolved, not `RemoteLatest` |
| `internal/setup/golden_test.go` + `testdata/golden/` | Modified | Generic fixture hand-edited per slice; framework fixtures via `-update` |
| `docs/design-principles.md`, `docs/learning-log.md` | Modified | §03's resolution exception; the measured `--staged` plugin drop |

## Slice plan

`auto-chain` / `stacked-to-main`, 400-line budget.
**Decision needed before apply: No.**
**Chained PRs recommended: Yes.**
**400-line budget risk: High** — the whole change is well past 1,200 lines.

| Slice | Content | ~lines | Why here |
|---|---|---|---|
| 1 | `internal/jsconfig` + the dependency + the `AGENTS.md` deviation | ~350 | Read-only, no step, no golden change. The dependency lands alone so it is reviewable alone |
| 2 | `.dharness/eslint.config.js` written; severities leave `doctor.config.json` | ~250 | Single home before anything imports it. Generic golden `== tree ==` hand-edit |
| 3a | `eslintExtendsStep`: write-if-absent, plus the full refusal matrix | ~300 | Every delegation path lands before any edit to an existing file. Generic golden `== plan ==` hand-edit |
| 3b | The splice into an existing array, markers, re-parse guard, idempotency | ~400 | The only slice with a destructive edit; it is the entire subject of its own review |
| 4 | Gate stage: `tool` invocation, placement measurement, absent-ESLint skip | ~200 | Independent of 3b; can move earlier if 3b overruns |
| 5 | Preset layer contribution; Next.js and Expo layers | ~300 | Framework goldens regenerate with `-update` |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| A splice leaves the project's lint unparseable | Med | Slice 3b's in-transaction re-parse turns it into an ordinary step failure with byte-exact `Undo` |
| The `--staged` plugin-drop mechanism is never confirmed | **Certain today** | Nothing asserts the cause; the outcome is measured and reproducible, and the fix does not depend on the mechanism |
| `gotreesitter` stalls (single-author reimplementation) | Med | `internal/jsconfig` is the only importer; fallback is cgo with the official bindings, at the cost of cross-compilation |
| The position rule puts dharness after a project override | Low | Default is index 0; recognition of framework bindings only ever moves it later, never past a project element |
| ESLint's own config is a call expression in most real projects | **Unknown — unmeasured** | Slice 3a delegates it correctly either way. Do not remove this row until the shape distribution is sampled |
| Two homes for one severity persist in adopted repositories | Med | Stated and reported, not machine-removed; dharness cannot distinguish its own past write (§05) |
| Generic golden hand-edit is wrong | Low | The test itself is the check — a wrong edit fails it |
| ESLint stage doubles the gate's cost | Med | §12/§13: measured placement, explicit staged file list, no process when nothing is staged |

## Principles that accept this change

- **§01 / §02** — checked first: no react-doctor or fallow command runs a plugin
  rule under `--staged`, and none composes a flat config. The judgement parts
  (`.ts`, call-expression exports, eslintrc migration) go to the agent as prompts.
- **§03** — dharness's rules live in `.dharness/eslint.config.js`. The project's
  file gains one import and one spread, both inside marked regions. The
  resolution exception for ESLint is recorded against the 10-August amendment.
- **§05** — the project's layer is always last; a disabled rule never returns.
- **§07 / §15** — nothing is recorded. Every run re-derives whether the regions
  are present, so a hand-removed splice reappears.
- **§09** — the parser reports byte ranges directly; no textual heuristic is
  invented over a signal the tree already gives.
- **§11 / §17** — the verdict is ESLint's exit code and the parser's structure,
  never prose and never a model.
- **§12 / §13** — placement by measured cost; no process when nothing is staged.
- **§20 / §21** — every unreadable config lowers a rung and continues; the prompt
  goes to the agent, and the ladder ends there.

## Rollback plan

Revert the merge commits in reverse order. No persisted state, no migration.
`.dharness/eslint.config.js` disappears; the marked regions in the project's
`eslint.config.js` are the only bytes dharness added to a file it does not own,
and they are removable by deleting exactly what lies between each marker pair —
which is why the markers are distinct and self-describing. An older binary
re-derives the older plan (§07). The dependency leaves with slice 1's revert.

## Dependencies

- `github.com/odvcencio/gotreesitter` — the product's first, deliberate. Must
  build with `CGO_ENABLED=0` on all six release targets before slice 1 merges.
- ooze stays dev-only, confined to `tools/mutationstaged`.

## Success criteria

- [ ] In a project with `maxFileLines: 1`, `dharness check` exits non-zero on a
      staged file that violates it. This is the whole point and it fails today.
- [ ] Running `sync` twice on the same `eslint.config.js` produces byte-identical
      output, and the second run reports the step satisfied.
- [ ] A config whose default export is a call expression, a `.ts` config, an
      eslintrc-only project, and a config with an `ERROR` node each delegate with
      a named reason, and none is edited.
- [ ] A splice that would leave the config unparseable is rolled back byte-for-
      byte, and `sync` reports the step as failed rather than done.
- [ ] Comments the project wrote in its config array survive the splice in place.
- [ ] A generic project's written tree is unchanged by the splice step.
- [ ] `CGO_ENABLED=0 go build ./...` succeeds for all six release targets.
- [ ] `go test ./...`, `go vet ./...`, `gofmt -l .` and
      `go run ./tools/mutationstaged` clean on every slice.

## Proposal question round

Asked here because a phase executor cannot reach the user directly. The proposal
is complete under the stated assumptions; these would sharpen it.

1. **Splice consent.** Should the first slice that edits an existing
   `eslint.config.js` require an explicit opt-in (a flag, or a delegated step the
   agent resolves) for its first run in a repository, or is the marked region
   plus byte-exact rollback sufficient? Assumed: sufficient.
2. **Severity residue.** Is leaving the inert `dharness/*` block in already-
   adopted `doctor.config.json` files acceptable, or should `sync` report it once
   as a delegated cleanup? Assumed: left silent, since it cannot fire.
3. **Call-expression configs.** `defineConfig(...)` is the shape ESLint's own
   docs now lead with. If it turns out to be the majority shape in real projects,
   does this change still pay for itself with delegation as its answer there, or
   does the scope need to grow to cover it? Assumed: delegation is enough for the
   first slice; the distribution is unmeasured.
4. **ESLint absent.** When a project has an `eslint.config.js` but no ESLint in
   `node_modules`, should dharness still splice? Assumed: yes — the config is
   evidence of intent, and the gate stage independently skips.
