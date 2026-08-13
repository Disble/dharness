# Proposal: structured-reports

Exploration: `openspec/changes/structured-reports/explore.md`.
Approved format: `openspec/changes/structured-reports/target-report.md` — read it
first. It is the settled design reference and the source of the thirteen defects
this change closes. **Do not propose alternative formats.**

## Intent

**`dharness sync`'s report cannot be handed to the agent that ran the commit.**
It was run against a real adopted repository and it does not say what succeeded,
what was attempted, what was skipped, or what was delegated. Of eleven steps, a
real run printed three lines and one delegated block; the other seven were an
ambiguous absence indistinguishable from "already done", "not applicable" and
"never reached".

The cause is structural, not cosmetic. **The report is a transcript of the apply
loop, not a result model.** `RunSync` (`internal/cli/sync.go:19`) prints as it
executes: `applySteps` (`internal/setup/setup.go:97`) writes the step ID *before*
calling `Apply`, so every line states an intention and none states an outcome.
Nothing intermediate is built, so there is nothing to render twice — which is why
there is no machine-readable output on any command, in a repository whose own
doctrine is that the verdict comes from exit codes and JSON.

§16 mandates the fix: the output has two readers, a person and the model that ran
the commit. The approved report is that principle applied.

## Decisions taken before this proposal — recorded, not re-opened

1. **The format is the approved target report.** No alternatives, no style menu.
2. **`sync` exits 0 whenever it completed its own work**, including with delegated
   work remaining. Delegated work is a correct outcome; `dharness check` is the
   gate, `sync` is not. Non-zero stays reserved for `sync` failing and rolling
   back. *Verified to need zero code change* — `RunSync` never returns non-nil
   when `left > 0`, and no hook calls `sync` (`internal/setup/steps.go:34` wires
   only `dharness check`). This change pins the position; it does not alter it.
3. **Scope is `sync`.** `check` and `mutate` gaining `--format json` is a named
   follow-up, designed for and not built here.

## What changes

| # | Change | Why it is here |
|---|---|---|
| 1 | **New `internal/report` package** — `Report`, `Summary`, `StepResult`, `FileChange`, `Collision`, `Note`; one analysis, two renderings (human + JSON) | Not inside `internal/setup`: `check` and `mutate` have nothing to do with adoption, and importing a setup package for a generic report shape inverts the dependency |
| 2 | **`Step.Apply` gains an output sink and a structured outcome** | `Apply(p, w *Writer) error` (`internal/setup/setup.go:41`) has no output channel, so `installStep.Apply` (`steps.go:80`, `:82`) and `hookInstallStep.Apply` (`steps.go:863`) write to the process stdout, bypassing the writer `applySteps` was handed. A sink frames the subprocess transcript; a structured outcome carries facts a sink cannot produce (`installed: [...]`) |
| 3 | **Per-step file attribution from `Writer`** | `Writer.touched` (`internal/setup/writer.go:18-27`) already records path, `existed`, pre-write bytes and mode for every file — but per run, not per step. `applySteps` records `len(touched)` before and after each `Apply`; the slice only appends, so `touched[before:after]` is exactly that step's set. Classification (`created`/`modified`/`unchanged`) stays inside `internal/setup` because `snapshot` is unexported |
| 4 | **One structured `Collision`, replacing two prose renderers** | `describeBoundaries` (`steps.go:528-540`) and `delegateBoundaries` (`steps.go:552-564`) each iterate the colliding keys independently and render near-identical prose, so one key prints twice. Deduplicating strings would leave the second renderer free to drift again |
| 5 | **`effective` is measured** — `fallow config --path` probes, `--format json` resolves, local binary first | See below; the cost and the exit contract are measured. `boundariesOwnerStep.ID()` (`steps.go:468`) is also corrected: the collision is one key with two values, not "two architectures" |
| 6 | **`RunSync` rewrite** — build the `Report`, then render it; `--format json` emits the same analysis; the failure variant explicitly retracts ticks already printed | Today the rollback message says *"No earlier step is reported as having succeeded"* while stdout has just reported them (`setup.go:110-112`) |
| 7 | **Coordinated `sync_test.go` rewrite** | 16 test functions; 8 lines assert on literal headings (`"Applying:"`, `"## Left to you"`, `"## Not checked"`, `"## Assumed"`, `"## Residue"`, `"Nothing to do"`) that all change shape |

Two details that must not be dropped in the rewrite: the measured-evidence line
(`p.ReadEvidence().ScopedMutation`, `sync.go:131-134`) is §08 evidence and needs a
home in the model; and the approved report's `use --show residue to list them`
names a flag that does not exist today — see the question round.

## The finding the approach is built on

`fallow config --format json` **already resolves `extends` and prints the final
config as clean JSON on stdout** (the `loaded config:` preamble goes to stderr;
26 top-level keys; measured against the reference project). `--path` alone prints
the config path and **exits 3 when no config file exists**.

This is `CLAUDE.md`'s first rule — *if the CLI already does it, dharness does not
do it* — and it **deletes work rather than adding it**:

- **No JSONC value parser.** `declaredValue`/`declaredLine`
  (`internal/setup/files.go:130-143`) is a deliberately textual single-line scan
  that cannot show a multi-line value and returns the fragment `"duplicates": {`
  (defect 8). The honest fix is not a better parser; it is asking fallow.
- **`effective` becomes measured, not inferred.** It is the highest-value field in
  the JSON twin — *I know there are two values, which one runs?* — and §09/§17
  forbid inventing a proxy signal or an independently computed verdict. If fallow
  cannot answer, the field is **absent**, never guessed.

### The cost is measured, and it decides the invocation

Three runs each, against the reference project. Real runs, not estimates.

| Route | Cost |
|---|---|
| local `node_modules/.bin/fallow` | 347 / 358 / 349 ms |
| remote `bunx fallow@latest` — the route `check.go` uses for fallow today | 3900 ms cold, 1022 ms warm |

Remote is ~3× warm and ~11× cold against local, so **`sync` prefers the project's
local binary and treats the remote executor as a fallback whose cost is stated
wherever it can be reached.** The mechanism already exists: `internal/cli/check.go:77-82`
resolves ESLint through `p.LocalBinary(tool.ESLint)` and `localStage`, and names
the miss explicitly when there is no local install. `p.LocalBinary(tool.Fallow)`
is available and the reference project has `node_modules/.bin/fallow`. Reuse that
pattern; do not invent a second one.

**This is a departure from §03's 10-August amendment**, which resolves all three
CLIs from the remote executor and puts `node_modules/.bin` out of resolution.
ESLint is already its first recorded exception (§03, 12 August). This change adds
a second, for a new invocation on the `sync` path only — the gate's fallow stage
(`check.go:102-103`) keeps remote resolution untouched. §12's own amendment is
the warrant: ascending cost means *measured*, not assumed.

### The exit contract, measured

| | config file present | no config file |
|---|---|---|
| `fallow config --format json` | exit 0 | **exit 0**, prints effective defaults |
| `fallow config --path` | exit 0 | **exit 3** |

`--format json` never fails on a zero-config project — it returns defaults — so
it can never be used to detect absence. `--path` can, for the price of the
cheaper call. The order follows: **probe with `--path`, and resolve with
`--format json` only when a config actually exists.** Exit 3 means no fallow
config at all, so no collision is possible and the entire analysis is skipped
(§13). Roughly 350 ms is the honest cost line, paid only by a project that has a
collision to resolve.

When fallow is absent entirely — no local binary, and the remote route
unavailable or refused — the collision block degrades to what dharness can still
say on its own and **never fabricates `effective`**. An unmeasured `effective` is
worse than an absent one, because being measured is the whole value of the field.

## Explicitly not in scope

- **`check` and `mutate` `--format json`** — a named follow-up. The model is
  designed with them in view: `check`'s stage outcome is dharness's own
  orchestration data and transfers directly, `tool.Survivor`
  (`internal/tool/report.go:23`) is already JSON-shaped.
- **Refactoring `renderGolden` to share the model.** It is test-only scaffolding
  (`internal/setup/golden_test.go:133`), called only from `golden_test.go:42` and
  `:74`, and `internal/cli` references it nowhere. It stays a separate consumer of
  `Plan()`/`Step`. Sharing the model would force `-update` on four fixtures and
  hand edits on two frozen ones, which `TestGenericMechanismHasNoUpdatePath`
  (`golden_test.go:101`) exists to prevent. **Zero golden bytes move.**
- **Re-parsing wrapped tools' per-finding detail** (§01/§09).
- **Persisting any report to a file** (§07/§08). No `.dharness/report.json` may
  become de facto tracked state.
- **Any new runtime dependency.** The product is stdlib-only (`AGENTS.md`); no
  table, colour or JSON library. `encoding/json` and `fmt` are the whole toolkit.
- **`sync`'s exit codes** and its two stopping conditions.

## Capabilities

### New capabilities

- `sync-report`: the result model, the human rendering, the JSON twin under
  `--format json`, the summary/tally/exit-statement block, and the failure variant
  that retracts ticks already printed.
- `step-outcome`: `Step.Apply`'s output sink and structured outcome, plus per-step
  file attribution and its `created`/`modified`/`unchanged` classification.
- `config-collision`: the collision as one computed fact, with `effective`
  measured from `fallow config --format json` behind a `--path` probe, resolved
  local-binary-first, and **absent** when it cannot be measured.

### Modified capabilities

- `project-sync` (`openspec/specs/setup/spec.md:25`): the requirement at line 153
  ("the rollback wording does not overclaim") gains the retraction obligation — a
  status line already printed MUST be explicitly withdrawn by the closing block,
  not merely contradicted. The exit-0-with-delegated-work position is pinned here;
  no existing requirement contradicts it.
- `step-delegation` (`spec.md:174`): a delegated step MUST hand back its reason as
  structured data the report renders, rather than pre-rendered prose, so one
  collision cannot be rendered twice from two places.

## Affected areas

| Area | Impact | Description |
|---|---|---|
| `internal/report/` (new) | New | Types, JSON marshalling, human renderer, tests |
| `internal/setup/setup.go:41,89-116` | Modified | `Step.Apply` sink and outcome; per-step `touched` partition; error path feeds the failure variant |
| `internal/setup/writer.go:18-27` | Modified | Path classification exposed as a public result type |
| `internal/setup/steps.go` | Modified | 11 `Apply` sites write to the sink; `installStep`/`hookInstallStep` stop touching process stdout; `boundariesOwnerStep` returns a `Collision` and its `ID()` names the key |
| `internal/setup/files.go:130-143` | Modified | Textual value scan superseded for the collision path |
| `internal/tool` | Modified | `fallow config --path` and `--format json` syntax. Every product invocation lives here (`AGENTS.md`) |
| `internal/project.LocalBinary` | Reused | `p.LocalBinary(tool.Fallow)`, the seam `check.go:77-82` already uses for ESLint |
| `internal/cli/sync.go:19` | Modified | Rewritten to build then render; `--format json` |
| `internal/cli/sync_test.go` | Modified | Coordinated rewrite, 16 test functions |
| `internal/cli/check.go` | **Unchanged** | The gate's fallow stage keeps remote resolution |
| `internal/setup/testdata/golden/` | **Unchanged** | Verified decoupled |
| `docs/design-principles.md` | Modified | §03: the second recorded local-resolution exception, with its measurement |
| `docs/learning-log.md` | Modified | One dated line for the `fallow config` cost and exit contract (P12) |

## Slice plan

Session budget 800 changed lines (`additions + deletions`), delivery
`ask-on-risk` with the user away — chained slices apply automatically.

**Decision needed before apply: No.**
**Chained PRs recommended: Yes.**
**400-line budget risk: High** — the exploration forecasts ~560–1050 lines total,
~580–1080 once slice 3 carries the local-first resolution path. Past 800 as a
single slice, and past 400 for either half.

| Slice | Content | ~lines | Why the seam is here |
|---|---|---|---|
| 1 | `internal/report` package: types, JSON, human renderer, tests | 200–350 | Pure addition with no consumer. Reviewable against the approved report alone |
| 2 | `Step.Apply` sink + structured outcome + per-step `Writer` attribution | 80–150 | Mechanical across 11 sites; lands before anything reads it |
| 3 | `boundariesOwnerStep` → structured `Collision`; `--path` probe, `--format json` resolve, local-first with a stated remote fallback | 100–180 | The only slice that adds a subprocess and the only one touching §03. It is the whole subject of its own review |
| 4 | `RunSync` rewrite, `--format json`, failure variant, `sync_test.go` rewrite | 200–400 | The only slice that changes observable output; its test rewrite is inseparable from it |

Slices 1–2 and 3–4 each sit inside the 800-line budget. Slice 4 is the only one a
revert has to reach to restore today's output.

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| `fallow config` slows every `sync` | **Low — measured** | ~350 ms on the local binary, and only after `--path` has proved a config exists. A zero-config project pays one cheap probe and nothing else (§13) |
| The remote fallback is reached on the `sync` path | Med | 3900 ms cold for one field. It is entered only once a collision is already proved, and the report states what it cost. Never silent |
| fallow absent entirely — no local binary, remote unavailable | Certain in some projects | The collision block degrades to what dharness knows on its own; `effective` is absent, never fabricated (§09/§17). A missing measurement is not a failed `sync` (§20) |
| Local-first resolution departs from §03's remote-executor amendment | Certain — by design | Recorded as a second exception beside ESLint's, with the measurement, in `docs/design-principles.md`. The gate's fallow stage is untouched |
| Box-drawing glyphs (`─ ✓ ✗ ■ │`) mangle on a legacy Windows console | **Unknown — unmeasured** | Windows is first-class (`AGENTS.md`). Measure at design time; recorded fallback is an ASCII glyph set. Do not remove this row until it is measured |
| `Writer` gains a sink field while `Writer` already means "file writer with rollback" | Med | Name the seam at design time; the alternative (widening `Apply`'s signature) ripples through 10 steps, a test stub and ~40 call sites |
| The report drifts into persisted state | Low | §07/§08: fresh per-run computation, stdout only, no file |
| Rendering code is mutation-hostile — spacing and format mutants survive | Med | P09: tests own behaviour scenarios (a step's status is reported, a path is attributed), never string trivia. Strengthen the owning scenario before adding a test |
| A `status` field grows into a heuristic verdict | Low | §11/§17: `status` derives strictly from the exit codes `ExitCode()` (`internal/app/app.go:64`) already maps. Pin it in the spec |
| Slice 4's test rewrite hides a behaviour regression | Med | Golden fixtures are untouched and stay green throughout; the rewrite may change assertions on the report's shape and MUST NOT relax assertions on what `sync` does |

## Rollback plan

Revert the merge commits in reverse order. There is no persisted state, no
migration, and no fixture to regenerate — all six goldens are byte-identical
throughout. Reverting slice 4 alone restores today's output while leaving the
model in place, because nothing else consumes it. An older binary re-derives the
older plan from the repository (§07).

## Dependencies

- **None new.** Stdlib only; ooze stays dev-only in `tools/mutationstaged`.
- `fallow` for the `effective` measurement only: the project's local
  `node_modules/.bin/fallow` preferred, the remote executor as a stated fallback,
  and absence degrading rather than failing.

## Success criteria

- [ ] A `sync` run over an adopted repository accounts for **every** step in
      `Plan()` with a status; no step is an ambiguous absence (defect 1).
- [ ] Every file the run touched is named with its directory and its kind —
      created, modified, unchanged (defects 4, 9).
- [ ] `bun add` output appears attributed under its step, and nothing is written
      to the process stdout behind the writer `applySteps` was handed (defect 5).
- [ ] One colliding key produces exactly one rendered collision, in both views,
      from one computed value (defects 6, 7).
- [ ] The project's colliding value is reported whole, never as the fragment
      `"duplicates": {`, and `effective` is either measured from
      `fallow config --format json` or absent (defect 8).
- [ ] A project with no fallow config runs `fallow config --path`, reads exit 3,
      and never runs the JSON resolve.
- [ ] With fallow unavailable, `effective` is absent from both views, no value is
      fabricated, and `sync` still exits 0.
- [ ] `sync` resolves fallow through `p.LocalBinary(tool.Fallow)` first, and the
      gate's own fallow stage still resolves remotely — unchanged.
- [ ] `dharness sync --format json` emits valid JSON on stdout from the same
      analysis as the human view, and exits 0 with delegated work remaining
      (defect 11).
- [ ] On a step failure the closing block explicitly retracts the ticks already
      printed, and no line claims restoration beyond what `Writer.Undo` covers
      (defect 3; `spec.md:153`).
- [ ] All six golden fixtures are byte-identical and
      `TestGenericMechanismHasNoUpdatePath` still passes.
- [ ] `go test ./...`, `go vet ./...`, `gofmt -l .` and
      `go run ./tools/mutationstaged` are clean on every slice.

## Proposal question round

Asked here because a phase executor cannot reach the user directly, and the user
is away. The proposal is complete under the stated assumptions; these would
sharpen it. **None of these re-opens the format, the exit code, or the scope.**

The invocation cost and the exit contract were **measured after the first draft
and are now closed**, not questions: local ~350 ms, remote 3900 ms cold, `--path`
exits 3 on a zero-config project and `--format json` never does.

1. **The remote fallback on the `sync` path.** With no local fallow, the remote
   route costs 3900 ms cold for one field. *Assumed: it is taken* — the `--path`
   probe has already proved a config exists and a collision was found, so the
   cost buys the report's highest-value field, and the report states what it
   cost. If a four-second `sync` is unacceptable, the alternative is to skip the
   measurement and say `effective` could not be measured.
2. **The TOML blind spot.** `fallow config` reads all four config formats, which
   is what `UncheckableConfigNote` exists to work around. Does it retire that
   note? *Assumed: not in this change* — retiring it is a separate claim that
   needs its own verification, and this change is already at budget.
3. **`--show residue`.** The approved report prints
   `note: 7 entries hidden (use --show residue to list them)`, and that flag does
   not exist. *Assumed: the flag lands with slice 4*, because printing a flag that
   does not exist is exactly the failure §16 and `AGENTS.md`'s "errors name the
   tool and the fix" forbid. The alternative is to state the count and list
   nothing.

## Question round — resolved

Answered by the orchestrator rather than escalated: the user is away, none of the
three changes what the product does, and each is answerable from the repository.
All three are recorded here as binding for every downstream phase.

### 1. The remote fallback on the `sync` path — **rejected**

The proposal assumed the remote route is taken when no local fallow exists. It is
not. **The `effective` measurement resolves through `p.LocalBinary(tool.Fallow)`
only. There is no remote fallback on the `sync` path.** With no local binary,
`effective` is absent and the report says it could not be measured — the same
degraded path the proposal already defines for fallow being absent entirely.

Three reasons, in order of weight:

1. **It reaches the network.** `internal/tool/tool.go:101-103` already records the
   rule this repository plays by: `--no-score` and `--no-supply-chain` are forced
   off precisely because *"both reach the network, which a gate that runs on every
   commit must not do."* `sync` is not the gate, but `bunx fallow@latest` fetches a
   package, and reaching the network to decorate one field of a report is the same
   trade that comment refuses. Offline, it does not degrade — it stalls.
2. **The cost is not worth the field.** 3900 ms cold for one value, on a command a
   person runs and waits on. The degraded report is already correct and already
   says what it does not know.
3. **It shrinks slice 3** and removes its only network dependency, leaving one
   resolution path to test instead of two.

This also simplifies the §03 departure: the new exception is "the `sync` path
resolves fallow locally **or not at all**", never "locally, then remotely".

### 2. The TOML blind spot — **not in this change**, assumption upheld

`fallow config` does read all four config formats, so it very likely retires
`UncheckableConfigNote`'s blind spot. That is a separate claim needing its own
verification, and this change is already at budget. Record it as a follow-up;
do not let it grow slice 3.

### 3. `--show residue` — **dropped; list the entries instead**

Do not add the flag, and do not print a reference to it.

The residue set is **bounded and small by construction**: it is `RulesPackage`
plus `RuleIDs()`, a fixed list dharness itself defines. It cannot grow with the
project. The collapse-plus-flag pattern was borrowed from fallow, where it guards
against 177 skipped files — a bound that does not exist here. Applied to seven
fixed entries it adds a flag, a code path and a test to hide something that fits
on seven lines.

**The `## Notes` block lists the residue entries in full and prints no flag
hint.** This removes scope and removes an unimplemented reference in the same
edit. `target-report.md` is a design reference, not a byte contract; this is
exactly the kind of adjustment that clause allows.

### Note on the unmeasured glyph risk

The risk table lists box-drawing glyphs (`─ ✓ ✗ ■ │`) on a legacy Windows console
as unmeasured. Partial evidence already exists and belongs in the design phase's
input: fallow emits `──`, `■`, `●`, `✓` and `✗` from a Rust binary, and its output
was captured rendering correctly in this user's Windows terminal during the
exploration. That covers the modern terminal, not `cmd.exe` under code page 437.
Keep the row; measure the legacy console at design time before committing to the
glyph set, and keep the recorded ASCII fallback.
