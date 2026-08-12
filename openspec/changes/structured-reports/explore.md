# Exploration — `structured-reports`

Investigation only. No product code was written during this phase.

## Why this change exists

`dharness sync` was run against a real adopted repository and the report could not
be handed to an agent picking up the work. It does not say what succeeded, what
was attempted, what was skipped, or what was delegated.

The root cause is structural, not cosmetic: **the report is a transcript of the
apply loop, not a result model.** `RunSync` (`internal/cli/sync.go:19`) prints as
it executes and builds no intermediate structure.

A target report has already been designed and approved. This change makes it
real. The format is settled; what follows is mechanics and cost.

## Decisions already closed (not open questions)

1. **The format is the approved target report.** No alternatives, no style menu.
   The measured output of ESLint, fallow, and react-doctor is source material for
   that design, not a set of candidates.
2. **`sync` exits 0 whenever it completed its own work**, including when delegated
   work remains. Delegated work is a correct outcome of `sync`, not a failure.
   `sync` is not a gate — `dharness check` is. Non-zero stays reserved for `sync`
   actually failing and rolling back. Counts travel in the JSON twin.
   *Verified to require zero code change:* `RunSync` already never returns non-nil
   when `left > 0`; the only non-nil returns are "not a git repository" and an
   Apply failure. No hook calls `dharness sync` — the generated
   `.dharness/lefthook.yml` (`internal/setup/steps.go:34`) only ever wires
   `dharness check`. This position has no existing consumer to reconcile against.
3. **Scope is `sync`.** It is the command that was demonstrated and the one that is
   broken. `check` and `mutate` are recorded as a follow-up change (see "Deferred").

## Verified findings

### The golden freeze does not block this change

`renderGolden` (`internal/setup/golden_test.go:133`) is test-only scaffolding for
`internal/setup`'s own `Step` contract. Verified by call-site search: its only
callers are `golden_test.go:42` and `golden_test.go:74`. `internal/cli` references
it nowhere, and `RunSync` never calls it.

`sync_test.go` asserts on `RunSync`'s stdout with `strings.Contains` only. There is
no byte-frozen fixture for the CLI report at all.

**Consequence: redesigning `sync`'s output costs zero golden bytes.** All six
fixtures stay untouched.

The corollary is a constraint on how the work is done: build the new report model
as a fresh computation that `RunSync`'s renderer consumes. Do **not** refactor
`renderGolden` to consume the shared model in order to remove duplication — that
would require `-update` on the four framework fixtures and hand edits on the two
frozen generic ones, which `TestGenericMechanismHasNoUpdatePath`
(`internal/setup/golden_test.go:101`) exists to prevent. `renderGolden` stays a
separate, stable consumer of `Plan()`/`Step`.

### `fallow config` already answers the collision question

This is the finding that changes the design most, and it is a direct application of
`CLAUDE.md`'s first rule: *if the CLI already does it, dharness does not do it.*

`fallow config --format json` walks up from the project root, **resolves `extends`**,
and prints the final config as JSON. Measured against the reference project:

- stdout is clean, valid JSON — 26 top-level keys. The `loaded config: <path>`
  preamble goes to **stderr**, so stdout needs no stripping.
- `duplicates` came back fully resolved:
  `{"mode":"semantic","minOccurrences":3,"threshold":3}`
- `fallow config --path` prints only the config file path, and exits **3** when no
  config file exists.
- fallow's own help states `.fallowrc.json` accepts JSONC (comments, trailing
  commas), and that precedence is first-match-wins per directory in the order
  `.fallowrc.json` > `.fallowrc.jsonc` > `fallow.toml` > `.fallow.toml`.

This deletes work rather than adding it:

- **No JSONC value parser is needed.** `declaredValue`/`declaredLine`
  (`internal/setup/files.go:130-143`) is a deliberately textual single-line scan,
  JSONC-safe by design but unable to show a multi-line value — it returns the
  fragment `"duplicates": {`. The honest fix is not a better parser; it is asking
  fallow, which already resolved it.
- **`effective` becomes measured, not inferred.** The approved JSON twin's
  highest-value field answers "I know there are two values, which one runs?"
  fallow answers that directly.
- **`fallow.toml` stops being a blind spot.** `UncheckableConfigNote` exists
  because TOML keys are bare and the quoted-key test cannot read them
  (framework-presets design decision 5). `fallow config` reads all four formats.
  Whether this retires the blind spot entirely is for design to settle.

Open item for design: `fallow config` is an extra subprocess invocation on the
`sync` path. Cost it before adopting it unconditionally, and decide the behaviour
when fallow is absent or exits 3.

### The `Step.Apply` sink gap

`Apply(p project.Project, w *Writer) error` (`internal/setup/setup.go:41`) has no
output channel. Verified sites writing directly to the process stdout, bypassing
the `stdout` that `applySteps` was handed:

- `internal/setup/steps.go:80` — `installStep.Apply`
- `internal/setup/steps.go:82` — the rollback compensation inside `installStep`
- `internal/setup/steps.go:863` — `hookInstallStep.Apply`

Two mechanisms are needed, and they answer different questions:

- **A sink** captures the raw subprocess transcript honestly (so `bun add` output
  can be indented and attributed under its step instead of leaking unframed).
  Cheapest form: a field on `*Writer` defaulting to `io.Discard`. Flag the naming
  collision — `Writer` currently means "file writer with rollback".
  The alternative, widening the interface signature, ripples through 10 concrete
  steps plus a test stub and roughly 40 call sites — mechanical, not hard.
- **A structured outcome** carries the facts a step already knows. A sink alone
  cannot produce the approved report's `installed: ["dharness-eslint-plugin@0.3.0"]`
  or per-step `wrote: [...]` — those need the step to hand back facts, not bytes.

This fits the codebase's grain: `Delegated` already returns `(string, bool)` rather
than a bare boolean. This codebase already prefers value-plus-explanation.

### `Writer` already tracks every path

`Writer.touched []snapshot` (`internal/setup/writer.go:18-27`) records `path`,
`existed`, the pre-write `data`, and `mode` for every file touched — but across the
whole run, not partitioned per step.

Smallest honest partition: `applySteps` records `len(writer.touched)` before and
after each `step.Apply`. The slice only ever grows by append, so
`touched[before:after]` is exactly that step's set.

Classification: `!existed` → created. `existed` → one `os.ReadFile` at report-build
time compared against the stored pre-write `data` → modified or unchanged.
(`Writer` stores pre-write bytes only, never post-write.)

`snapshot` is unexported with unexported fields, so the classification belongs
inside `internal/setup`, returning a public result type.

### The duplicate render is a modelling problem, not a string problem

`describeBoundaries` (`internal/setup/steps.go:528-540`) and `delegateBoundaries`
(`internal/setup/steps.go:552-564`) each independently iterate the colliding keys
and render key + dharness's value + the project's value into prose. With one key,
the report prints it twice in near-identical wording.

The fix is to compute the collision **once** as a structured fact and render it
into the human view and the JSON from that single value. Deduplicating strings
would leave the second renderer able to drift again.

Related and in the same code: `boundariesOwnerStep.ID()` is the fixed constant
`"resolve the two architectures this project declares"`
(`internal/setup/steps.go:468`). The real collision was on `duplicates`, not
`boundaries` — one key with two values, not two architectures. `collidingKeys` was
generalised; the ID was not. This is the prose-asserts-what-code-does-not failure
mode already recorded in `docs/learning-log.md`.

### The result model

Shape required by the approved report and its JSON twin:

- `Report{ Version, Root, Source, Summary, Steps []StepResult, Notes []Note }`
- `Summary{ Steps, Applied, Delegated, Satisfied, Failed, MS }`
- `StepResult{ N, ID, Status, MS, Wrote []FileChange, Installed []string,
  Collision *Collision, Evidence string }`
  with `Status` one of applied / delegated / satisfied / failed / not-reached
- `FileChange{ Path, Kind }` — created / modified / unchanged
- `Collision{ Key, Ours, Theirs, Effective, Resolutions []string }`
- `Note{ Kind, Path, Entries []string, Actionable bool, Reason string }`

**Placement: a new `internal/report` package**, not inside `internal/setup`.
`check` and `mutate` have nothing to do with project adoption; making them import
a package about setup to obtain a generic report shape inverts the dependency.
`internal/report` is imported by `internal/setup` (to populate `StepResult` from a
`Step`) and by the `internal/cli` commands (to render). `internal/tool.Survivor`
stays where it is.

### Test cost

`internal/cli/sync_test.go` has 16 test functions; 8 lines assert on today's literal
headings (`"Applying:"`, `"## Left to you"`, `"## Not checked"`, `"## Assumed"`,
`"## Residue"`, `"Nothing to do"`). Every one of those strings changes shape under
the approved report. This is a coordinated rewrite, not incidental touch-ups —
though smaller than first estimated.

## Line forecast against the 800-line budget

| Area | Estimate |
|---|---|
| `internal/report` package — types, JSON marshal, human renderer, tests | 200–350 |
| `Step` sink + `Writer` attribution across 11 implementation sites | 80–150 |
| `boundariesOwnerStep` → structured `Collision`, fed by `fallow config` | 80–150 |
| `RunSync` rewrite to build and render `Report` | 100–200 |
| `sync_test.go` coordinated rewrite | 100–200 |
| Golden fixtures | **0** (verified decoupled) |
| **Total** | **~560–1050** |

Plausibly over budget as a single slice. Chained PRs are expected; the natural
seam is model-and-plumbing first, then the `RunSync` rewrite with its test rewrite.

## Deferred to a follow-up change

`check` and `mutate` gaining `--format json`. Recorded now so the model is designed
with them in view, but not built here.

- **`check` transfers only at the envelope layer.** `internal/cli/check.go:124`
  pipes each tool's own human report straight through
  (`runner.Run(stage.command, stdout, stdout)`); nothing is parsed. The stage
  outcome — name, status, duration, exit code, what was skipped — is dharness's own
  orchestration data and transfers directly. Per-finding detail does **not**:
  ESLint, fallow, and react-doctor each already emit their own `--format json`, and
  re-parsing that into a dharness schema is dharness reimplementing the CLI.
- **`mutate` transfers cleanly and is the cheapest of the three.** `tool.Survivor`
  (`internal/tool/report.go:23`) is already an exported, JSON-shaped struct parsed
  from Stryker's own JSON report (`tool.Survivors`, `report.go:60`). It is rendered
  with a `fmt.Fprintf` loop today (`internal/cli/mutate.go:234-237`) rather than
  marshalled. No new parsing — only a JSON writer over data that already exists.

## Principles

No principle in `docs/design-principles.md` blocks this change.

- **§16 mandates it.** Output has two readers, a person and the model that ran the
  commit. The approved report is that principle applied; the current output is not.
- **§11/§17 constrain the design.** Any `status`/`verdict` field in the JSON must be
  derived strictly from the same exit codes `ExitCode()` (`internal/app/app.go:64`)
  already maps — never an independently computed "looks passing" judgement. State
  this in the design so no heuristic verdict field is added later.
- **§07/§08 keep the report ephemeral.** It stays a fresh per-run computation
  written to stdout, never persisted to a state file. No `.dharness/report.json`
  may become de facto tracked state.
- **§01/§09 are the live risk**, in implementation rather than in the design:
  hand-rolling JSONC parsing, or re-parsing wrapped tools' findings for `check`,
  would violate them. The `fallow config` finding above is the mitigation.

## Backlog captured along the way

- `internal/setup/steps.go:645`, `:700` and `internal/setup/setup_test.go:926` cite
  "measured against react-doctor 0.5.7" while `internal/tool/tool.go:92` cites
  "0.9.11" for the same binary. Internal comments, not printed output — a one-line
  fix for whoever is next in that file.
