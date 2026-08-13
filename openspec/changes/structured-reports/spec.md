# Spec: structured-reports

Source of record for behaviour: `openspec/changes/structured-reports/proposal.md`
(all sections, and the `## Question round — resolved` section, which is binding
and overrides the proposal body wherever the two disagree),
`openspec/changes/structured-reports/target-report.md` (the approved report
shape, its failure variant, its JSON twin, and the thirteen defects it
replaces), `openspec/changes/structured-reports/explore.md` (current-state
citations and the result-model shape). Principles cited by number:
`docs/design-principles.md` §01, §05, §07, §08, §09, §11, §12, §13, §16, §17,
§20, §21. Proposal: Engram `sdd/structured-reports/proposal`,
`openspec/changes/structured-reports/proposal.md`.

`openspec/specs/setup/spec.md` already exists. Three capabilities below
(`sync-report`, `step-outcome`, `config-collision`) are new; every requirement
in them is `ADDED`. Two capabilities already exist —
`project-sync` (`openspec/specs/setup/spec.md:25`) and `step-delegation`
(`openspec/specs/setup/spec.md:174`) — and the requirements added to them
below extend each capability without overturning any existing requirement.

The reference shapes named below (`Report`, `Summary`, `StepResult`,
`FileChange`, `Collision`, `Note`) come from `explore.md`'s "The result model"
section. They are a design reference for these requirements, not a byte
contract: field names may be adjusted at design time as long as every fact
named below is carried by the model and reaches both renderings.

---

## ADDED Requirements — Capability: `sync-report`

The result model `RunSync` builds once per run and renders twice: a human
view on stdout, and (under `--format json`) the same analysis as JSON on
stdout. Replaces the transcript `RunSync` (`internal/cli/sync.go:19`) prints
today, which builds no intermediate structure and states intentions before
outcomes are known.

### Requirement: every step in `Plan()` carries exactly one status, and none is an ambiguous absence

The report MUST account for every step `setup.Plan()` declares with exactly
one of `applied`, `delegated`, `satisfied`, `failed`, `not-reached`, or
`retracted`. No step may be silently absent from the report the way
`setup.Pending` today filters satisfied steps out with nothing printed in
their place (defect 1). A step reported `satisfied` MUST carry the evidence
that satisfied it (the `Describe`/detection fact — e.g.
`extends → .dharness/fallow.jsonc`), not a bare status word with nothing
behind it.

**`retracted` added after `sdd-design` Decision 1**, which found the original
five-value set unable to account for every step in the one case this change
exists to fix. The approved report's own failure tally reads
`1 failed · 0 applied · 9 not reached` — ten of eleven steps. The eleventh is
the step that succeeded, was printed, and was then undone by `Writer.Undo`.
It is not `applied` (its work was reversed), not `failed` (it did not fail),
and not `not-reached` (it ran). `retracted` is that state, and
`Summary.Retracted` counts it, so the tally sums to the plan's own length.
This is the machine-readable half of the retraction obligation the
`project-sync` capability states in prose below.

#### Scenario: an eleven-step plan produces eleven step results

- **GIVEN** `setup.Plan()`'s eleven steps, in a project where three are
  applied, one is delegated, and seven are already satisfied
- **WHEN** `RunSync` builds its report
- **THEN** the report's step list has exactly eleven entries, one per
  `Plan()` step, and every entry's status is one of the five defined values —
  none is missing and none carries an empty or unrecognised status

#### Scenario: a satisfied step carries its evidence, not just its status word

- **GIVEN** a step already satisfied before this run (e.g. `fallowExtendsStep`,
  satisfied because `.fallowrc.json` already contains `extends →
  .dharness/fallow.jsonc`)
- **WHEN** the report is built
- **THEN** that step's entry carries a non-empty evidence value naming what
  made it satisfied, not merely `status: "satisfied"` with no supporting fact

### Requirement: a step's line states an outcome, printed after `Apply` returns

The report MUST NOT print a step's identity before its `Apply` outcome is
known. Today `applySteps` (`internal/setup/setup.go:97`) writes the step ID
immediately before calling `Apply`, so the printed line states an intention
and carries no status glyph and no elapsed time (defect 2). The rewritten
flow MUST print (or build the report entry for) a step only once its
`Apply`/`Satisfied`/`Delegated` result is known, carrying the outcome glyph
and duration alongside it.

#### Scenario: a step's printed line carries its result, not merely its name

- **GIVEN** a step that applies successfully in 5.56 seconds
- **WHEN** the human report renders that step
- **THEN** the rendered line for that step carries its outcome (applied) and
  its measured duration together with its identity — never the identity alone,
  printed ahead of knowing whether `Apply` would succeed

### Requirement: the summary is rendered before per-step detail, in both views

Matching fallow's own convention (`■ Metrics: ...` before `── Dead Code
──`), the human view MUST render one summary line — counting steps, applied,
delegated, satisfied, failed, and elapsed time — before any per-step detail
block. The JSON twin's `summary` object carries the identical counts.

#### Scenario: the summary line precedes every step block in the human view

- **GIVEN** a completed `sync` run with a non-empty step list
- **WHEN** the human report is rendered
- **THEN** the summary line's text appears in the output before the text of
  any individual step's detail block

#### Scenario: the JSON summary's counts match the human summary's counts

- **GIVEN** one completed `sync` run
- **WHEN** both the human view and `--format json` are rendered from that
  run's `Report`
- **THEN** `summary.steps`, `summary.applied`, `summary.delegated`,
  `summary.satisfied`, and `summary.failed` in the JSON equal the
  corresponding counts printed in the human summary line

### Requirement: `--format json` renders the same single analysis as the human view, never a second computation

`dharness sync --format json` MUST emit valid JSON on stdout, decoded from
the same `Report` value the human rendering consumes — built once per run,
not recomputed for the flag. This is the machine-readable output §11/§16
require and no command has today (defect 11).

#### Scenario: `--format json` emits parseable JSON on stdout

- **GIVEN** any completed `sync` run
- **WHEN** it is invoked with `--format json`
- **THEN** stdout is valid JSON decodable by `encoding/json` into the
  documented `Report` shape, and no non-JSON text (a progress line, a banner)
  precedes or follows it on stdout

#### Scenario: the human view and the JSON twin agree because they share one analysis

- **GIVEN** a run whose plan mixes applied, delegated, satisfied, and one
  collision
- **WHEN** `RunSync` is invoked once for the human view and once with
  `--format json` against the identical repository state
- **THEN** every step's status, and the collision's `effective` field, agree
  between the two renderings — because both are produced by rendering the
  same computed `Report`, not by two independent passes over the repository

### Requirement: the JSON envelope's status agrees with the exit code, and is never independently computed

Whatever field in the JSON answers "did this run succeed" MUST derive
strictly from the same decision `app.ExitCode(err)` (`internal/app/app.go:64`)
makes for the process exit code — never a verdict computed separately from
step counts (e.g. `failed == 0`). Delegated work remaining is a correct
outcome of `sync`, not a failure: per the existing `project-sync` requirement
"Exactly two stopping conditions, and no others"
(`openspec/specs/setup/spec.md:84`), `sync` exits 0 whenever it completed its
own work, including with delegated work left. This requirement pins that
position for the JSON output specifically, so introducing `--format json`
does not grow a second, competing notion of "pass".

#### Scenario: delegated work remaining still reports the exit-0 outcome

- **GIVEN** a run that applies what it can and leaves one step delegated,
  with no step failing
- **WHEN** `--format json` renders the run
- **THEN** the JSON's status-bearing field is exactly what `app.ExitCode`
  would compute for `RunSync`'s (nil) return — the same value a step-failure
  run's field would differ from — and no separate rule keyed on
  `summary.delegated > 0` overrides it

#### Scenario: a step failure's JSON status matches the process's own exit code

- **GIVEN** a run where a step's `Apply` fails and `Writer.Undo` runs
- **WHEN** `--format json` renders the run
- **THEN** the JSON's status-bearing field equals `app.ExitCode` applied to
  the same error `RunSync` returned — not a value computed by inspecting
  `summary.failed` independently of that error

### Requirement: the closing block carries the tally and the exit statement

The human view MUST end with a block distinct from the summary line at the
top, carrying the same counts, the elapsed time, and the exit code, plus a
`next` pointer when work remains and a `gate` pointer naming what `dharness
check` will run. This replaces today's report, which has no final tally, no
exit-code statement, and no next command (defect 10).

#### Scenario: the closing block states the exit code that matches the process's own

- **GIVEN** any completed `sync` run
- **WHEN** the human report's closing block is rendered
- **THEN** the exit code it states is the same value `app.ExitCode` computes
  for the error `RunSync` returns for that run

#### Scenario: a run with delegated work names a next step

- **GIVEN** a run that leaves at least one step delegated
- **WHEN** the closing block is rendered
- **THEN** it names the delegated step's identifier as the next thing to
  resolve, distinct from and in addition to the per-step "Left to you" detail
  printed earlier

### Requirement: the failure variant explicitly retracts steps already reported, and accounts for every step not reached

On a step failure, the report MUST NOT leave a status line printed for an
earlier, since-rolled-back step standing as if uncontradicted. It MUST
explicitly mark that step's status as retracted/failed-with-the-run (not
"applied"), and every step in `Plan()` that was never attempted because the
run stopped MUST be reported with status `not-reached` — not silently
dropped, which is the same ambiguous-absence failure defect 1 names for the
success path.

#### Scenario: a step that had succeeded is not left standing as applied

- **GIVEN** step 1 applies successfully and step 2's `Apply` then fails,
  triggering `Writer.Undo`
- **WHEN** the report is built for this run
- **THEN** step 1's entry in the report does not carry status `applied` —
  it is reported in a way that names the run's overall rollback, and no
  reader of the report structure can conclude step 1's work survived

#### Scenario: steps after the failure are reported `not-reached`, not omitted

- **GIVEN** a plan of eleven steps where step 2 fails and nine steps after it
  in `Plan()` order were never attempted
- **WHEN** the report is built
- **THEN** those nine steps each appear with status `not-reached`, and the
  report's step count still equals eleven — none of them is simply absent

### Requirement: measured evidence keeps its place in the model

The scoped-mutation evidence line (`p.ReadEvidence().ScopedMutation`,
`internal/cli/sync.go:131-134` — §08 evidence: a number that cost something
to measure and cannot be re-derived for free) MUST be carried by the report
model and rendered in both views when present, not dropped by the rewrite.

#### Scenario: scoped-mutation evidence survives into the rendered report

- **GIVEN** a run where `p.ReadEvidence().ScopedMutation` is non-nil and every
  step is satisfied
- **WHEN** the human report and `--format json` are both rendered
- **THEN** the measured related-test count and the path it was measured
  against both appear somewhere in the human view and somewhere in the JSON —
  neither rendering drops the fact the pre-rewrite output stated

### Requirement: notes list their entries in full and never reference an unimplemented flag

Per the resolved question round, the residue set (`RulesPackage` plus
`RuleIDs()` — a fixed, bounded list dharness itself defines) MUST be listed
in full inside the `## Notes` block. The report MUST NOT print a hint
pointing at a `--show residue` flag or any other flag that does not exist.

#### Scenario: residue entries are listed, not summarised behind a flag

- **GIVEN** a repository whose `doctor.config.json` still carries residue
  entries from a prior mechanism
- **WHEN** the human report renders the residue note
- **THEN** every entry in the residue set appears listed in the output, and
  no text in that block names a command-line flag that `dharness sync` does
  not accept

### Requirement: no report is persisted to a file

Consistent with §07/§08, the report MUST be a fresh, in-memory computation
per run. `sync` MUST NOT write a report to disk under any flag — no
`.dharness/report.json` or equivalent may become de facto tracked state.

#### Scenario: `--format json` writes only to stdout

- **GIVEN** a repository before and after a `dharness sync --format json` run
- **WHEN** the set of files present in `.dharness/` and the repository root is
  compared before and after
- **THEN** no new file holding a copy of the report exists afterwards — the
  only files that differ are the ones the applied steps themselves own

---

## ADDED Requirements — Capability: `step-outcome`

`Step.Apply`'s output channel and the structured facts a transcript alone
cannot produce, plus the per-step file attribution the `Writer` already
tracks but does not yet partition.

### Requirement: `Step.Apply` writes subprocess and informational output through a sink, never the process's real stdout

`installStep.Apply` (`internal/setup/steps.go:80`, its rollback compensation
at `:82`) and `hookInstallStep.Apply` (`internal/setup/steps.go:863`) MUST
NOT write to `os.Stdout`/`os.Stderr` directly. Every byte a step's `Apply`
produces MUST flow through the sink `applySteps` controls, so it can be
framed and attributed under that step in the report rather than leaking
unframed ahead of it (defect 5).

#### Scenario: `bun add`'s output is captured, not leaked to the real process stdout

- **GIVEN** `installStep.Apply` running an install that prints package-manager
  output
- **WHEN** `applySteps` runs this step with a sink that is not the process's
  real `os.Stdout`
- **THEN** none of that subprocess output appears on the process's actual
  stdout — all of it is observable only through the sink `Apply` was given

#### Scenario: the hook-manager install's output is likewise captured

- **GIVEN** `hookInstallStep.Apply` running `lefthook install` (or the husky
  equivalent)
- **WHEN** `applySteps` runs this step
- **THEN** the invocation's output reaches only the sink `Apply` was given,
  matching `installStep`'s behaviour, not `os.Stdout` directly

### Requirement: a step's structured outcome carries facts a transcript alone cannot produce

Beyond a captured transcript, `Apply` MUST be able to hand back structured
facts the report needs — such as the packages installed
(`installed: ["dharness-eslint-plugin@0.3.0"]`) — because a sink of raw bytes
cannot itself answer "what package, what version" without the report
re-parsing subprocess output, which §01/§09 forbid.

#### Scenario: an install's structured outcome names the package and version

- **GIVEN** `installStep.Apply` completing a package install successfully
- **WHEN** the report is built for that step
- **THEN** its entry carries the installed package name(s) as structured
  data, not derived by scanning the captured transcript text for a package
  name at render time

### Requirement: file attribution is partitioned per step, not reported only for the whole run

`Writer.touched` (`internal/setup/writer.go:18`) already records every file a
run changes, but as one slice for the entire `Apply` pass. The report MUST
attribute each touched file to the specific step that touched it: recording
`len(writer.touched)` immediately before and after each step's `Apply` call
and taking that step's slice as exactly `touched[before:after]` (the slice
only ever grows by append within one run).

#### Scenario: two steps that both write files are attributed independently

- **GIVEN** a run where step A writes one file and step B (running later in
  the same `Apply` pass) writes two different files
- **WHEN** the report is built
- **THEN** step A's entry names exactly the one file it wrote and step B's
  entry names exactly the two files it wrote — neither entry names a file the
  other step touched

#### Scenario: a file's directory is part of what is named

- **GIVEN** a step that writes `.dharness/eslint.config.js`
- **WHEN** that step's file attribution is rendered
- **THEN** the path recorded includes its directory, resolving defect 9 (a
  path reported without saying which directory it is in)

### Requirement: a touched file classifies as created, modified, or unchanged

For every file a step's `Apply` touches, the report MUST classify it: a file
that did not exist before this run (`!snapshot.existed`) is `created`; a file
that existed is `modified` when its post-write bytes differ from the
snapshot's stored pre-write bytes, and `unchanged` when they are identical —
read from disk at report-build time, since `Writer` stores pre-write bytes
only, never post-write.

#### Scenario: a newly written file classifies as created

- **GIVEN** a step that writes a file which did not exist before this run
- **WHEN** the report classifies that file
- **THEN** its kind is `created`

#### Scenario: a rewritten file with different bytes classifies as modified

- **GIVEN** a step that rewrites a pre-existing file, and the file's contents
  after the write differ from its contents before
- **WHEN** the report classifies that file
- **THEN** its kind is `modified`

#### Scenario: a file rewritten to identical bytes classifies as unchanged, not modified

- **GIVEN** a step whose `Apply` calls `Writer.Write` on a file whose contents
  after the write are byte-identical to its contents before (an idempotent
  re-run, e.g. `.dharness/lefthook.yml` already correct)
- **WHEN** the report classifies that file
- **THEN** its kind is `unchanged`, never `modified` — the classification
  compares bytes, not whether `Write` was called

---

## ADDED Requirements — Capability: `config-collision`

The colliding-key fact `boundariesOwnerStep` reports, computed once as a
structured value and rendered from that one value into both views, with
`effective` measured through the project's local fallow binary or reported
absent.

### Requirement: a collision is computed once and rendered from that one value in both views

`describeBoundaries` (`internal/setup/steps.go:528`) and `delegateBoundaries`
(`internal/setup/steps.go:552`) each independently iterate the colliding keys
today and render near-identical prose, so one key prints twice (defects 6,
7). The rewrite MUST compute each colliding key's collision fact exactly
once per run and render both the human "Left to you" block and the JSON
`Collision` entry from that same computed value — never from two
independently-walked renderers.

#### Scenario: one colliding key renders exactly once in the human view

- **GIVEN** a project whose fallow config declares exactly one colliding key
  (`duplicates`)
- **WHEN** the human report is rendered
- **THEN** the string `duplicates` appears in the collision block naming the
  key exactly once, not twice from two separate prose blocks

#### Scenario: one colliding key renders exactly once in the JSON twin

- **GIVEN** the same project as above
- **WHEN** `--format json` is rendered
- **THEN** the JSON's `steps` array carries exactly one entry whose
  `collision.key` (or equivalent) equals `duplicates` — not two entries or one
  entry with the key duplicated inside it

#### Scenario: two colliding keys render in the same order in both views

- **GIVEN** a project whose fallow config and a matched preset together
  produce two colliding keys
- **WHEN** both the human view and the JSON twin are rendered from the same
  run
- **THEN** both list the two keys in the same order, because both are
  rendering the same computed slice of `Collision` values, not two
  independent walks that could disagree on order

### Requirement: nothing asserts "two architectures", and the rendered collision names the keys actually found

Amended after `sdd-design` Decision 6 proved the original single requirement
unsatisfiable as written: it put the MUST on `boundariesOwnerStep.ID()`, and
`ID()` takes no `project.Project` (`internal/setup/setup.go:25`), so the
colliding key is not reachable from it. The obligation splits in two.

**(a)** `boundariesOwnerStep.ID()` (`internal/setup/steps.go:467`) MUST NOT
assert that this project declares two architectures. That sentence is false
for one key with two values, and false again for a project with no collision
at all — which is the common case, and the case this change starts rendering
(defect 1 makes satisfied steps visible, so the string moves from never
printed to printed on nearly every run).

**(b)** The collision's **rendered entry and its addressable handle** MUST
name the colliding key(s) actually found. The handle is carried on the
computed collision value, not on `ID()`.

#### Scenario: a single-key collision names the key in the rendered entry

- **GIVEN** a collision on the key `duplicates` alone
- **WHEN** the collision entry is rendered
- **THEN** it names `duplicates`, and neither view asserts that two
  architectures are in conflict when only one key collided

#### Scenario: the step identity makes no architecture claim even with no collision

- **GIVEN** a project with no colliding key at all, where the step is
  satisfied and therefore appears in the "already in place" block
- **WHEN** its identity is rendered
- **THEN** it does not assert that the project declares two architectures

### Requirement: `effective` resolves through the project's local fallow binary only — no remote fallback on the `sync` path

Per the resolved question round, the `effective` measurement MUST resolve
exclusively through `p.LocalBinary(tool.Fallow)`
(`internal/project/detect.go:190`), the same seam `check.go:77` already uses
for ESLint. There MUST be no remote-executor fallback (`bunx fallow@latest`
or equivalent) on the `sync` path: with no local binary, `effective` is
absent — the same degraded path as fallow being unreachable for any other
reason. `internal/cli/check.go`'s own fallow stage keeps its existing remote
resolution untouched; this requirement governs the `sync` path's new
invocation only.

#### Scenario: a project with a local fallow binary measures `effective`

- **GIVEN** a project with `node_modules/.bin/fallow` present and a genuine
  collision detected
- **WHEN** `sync` resolves `effective` for that collision
- **THEN** the binary invoked is the one `p.LocalBinary(tool.Fallow)` returns,
  and no remote-executor command (`bunx`, `npx`, `pnpm dlx`, `yarn dlx`) is
  ever constructed for this measurement

#### Scenario: a project with no local fallow binary never reaches the network for `effective`

- **GIVEN** a project with a genuine collision detected and no
  `node_modules/.bin/fallow` (or platform equivalent)
- **WHEN** `sync` resolves `effective` for that collision
- **THEN** no subprocess is spawned for the measurement at all, `effective`
  is absent from both views, and `sync` still exits 0

#### Scenario: the gate's own fallow stage is unaffected

- **GIVEN** `internal/cli/check.go`'s fallow stage, which resolves remotely
  today
- **WHEN** this change ships
- **THEN** `check.go`'s fallow invocation (`tool.FallowAudit`,
  `tool.FallowDupes`) is unchanged — still resolved through
  `remoteStage`/the remote executor, not `p.LocalBinary(tool.Fallow)`

### Requirement: the exit-3 probe short-circuits the whole measurement, never just the resolve call

The measurement MUST probe first with `fallow config --path` and inspect its
exit code before ever invoking `fallow config --format json`. An exit code of
3 means no fallow config exists at all; in that case the `--format json`
call MUST NOT run, and the collision analysis for `effective` is skipped
entirely for this run (§13: the cheapest way to run something is not to run
it when it cannot answer).

#### Scenario: a zero-config project's probe exits 3 and the resolve call never runs

- **GIVEN** a project (or a constructed fixture for the measurement function)
  where `fallow config --path`, run against the resolved local binary, exits
  with code 3
- **WHEN** the measurement runs
- **THEN** `fallow config --format json` is never invoked, and the function
  reports `effective` as absent for that run

#### Scenario: a project with a config present probes successfully and resolves

- **GIVEN** a project where `fallow config --path` exits 0 (a config file
  exists)
- **WHEN** the measurement runs
- **THEN** `fallow config --format json` is invoked exactly once afterwards,
  and its resolved value for the colliding key becomes the source for
  `effective`

### Requirement: `effective` is absent, never fabricated, whenever it cannot be measured

Whenever the measurement cannot produce an answer — no local binary, a
`--path` exit code of 3, a non-zero exit from `--format json`, output that
does not parse as JSON, or the colliding key absent from the resolved
output — the report MUST carry `effective` as absent (a nil/omitted value),
never a guessed or inferred value, and `sync` MUST still exit 0 for this
reason alone. An unmeasured `effective` is worse than an absent one, because
being measured is the entire value of the field (§09/§17).

#### Scenario: an unparsable resolve response leaves `effective` absent

- **GIVEN** a `fallow config --format json` invocation that exits 0 but whose
  stdout is not valid JSON (a constructed adversarial case)
- **WHEN** the measurement runs
- **THEN** `effective` is absent in the report, `sync` still exits 0, and no
  fallback heuristic (declared-key order, file precedence assumed from
  `extends` direction) substitutes for the missing measurement

#### Scenario: absence of `effective` does not block or fail `sync`

- **GIVEN** any run in which `effective` ends up absent
- **WHEN** `sync` completes
- **THEN** the run's exit code is governed only by the existing two stopping
  conditions (`openspec/specs/setup/spec.md:84`) — a missing `effective`
  measurement is never treated as a step failure

### Requirement: a colliding value is reported whole or not at all — never a truncated fragment

The dharness-owned value for a colliding key MUST be reported whole: it is
already known to dharness as the value it wrote (or is about to write) into
`.dharness/fallow.jsonc`, requiring no textual re-scan. The project-declared
value, once a collision is confirmed, MUST be sourced from
`fallow config --format json`'s resolved output for that key rather than
from `declaredValue`/`declaredLine`'s single-line textual scan
(`internal/setup/files.go:130-143`), because that scan cannot show a
multi-line value and today returns the fragment `"duplicates": {` (defect 8).
Where the resolved value cannot be measured, the report states that the
value could not be shown in full — it MUST NOT fall back to the truncated
textual fragment.

**AMENDED after live verification.** As first written this requirement said
"reported whole" of *both* views, and `sdd-verify` correctly raised the
resulting rendering as a spec violation. The amendment is recorded here
rather than the code being reverted, because the original wording aimed at
defect 8 — a *fragment*, produced by a single-line textual scan, that a
reader cannot tell is partial — and it over-reached into forbidding a
deliberate, labelled, counted selection, which is a different thing.

What the live run showed: dharness declared 3 keys, fallow's resolved value
carried 16, and the three shared keys all differed. Rendered whole, the one
word the reader had to decide about was buried in nine wrapped lines of
compact JSON split mid-token. A value nobody can read is not a value
reported.

So the rule splits by audience, and both halves are MUSTs:

- **The JSON twin MUST carry both values whole**, always. The machine
  reading `theirs` wants the value that runs, not a diff computed for
  someone else's eyes.
- **The human view MAY omit keys that carry no decision** — keys holding the
  same value on both sides, and keys only the resolved side declares (those
  are fallow's own defaults, which the project never wrote and dharness does
  not disagree with). When it omits any, it MUST say how many and why, in
  the same rendering. Silent shortening remains forbidden, and so does the
  truncated fragment the original requirement was written against.

#### Scenario: a multi-line colliding value is shown whole in the JSON twin

- **GIVEN** a project's fallow config declaring a multi-line object value for
  a colliding key, and `effective` successfully measured
- **WHEN** the collision is rendered
- **THEN** `--format json` carries the full value object for both sides — not
  the fragment a single-line scan of the opening line would have produced,
  and not the human view's narrowed selection

#### Scenario: the human view omits only what carries no decision, and says so

- **GIVEN** the same collision, where some keys hold identical values on both
  sides and others are declared only by the resolved side
- **WHEN** the collision is rendered for a person
- **THEN** the keys that disagree appear with both of their values, the rest
  are omitted, and the rendering states how many were omitted and that they
  were either the same on both sides or defaults only fallow sets

#### Scenario: nothing is omitted when the two sides cannot be compared

- **GIVEN** a collision where either side was never measured, or a value that
  is not a JSON object
- **WHEN** the collision is rendered for a person
- **THEN** the measured side is rendered whole and no omission is claimed —
  shortening one side against nothing would hide information while asserting
  a comparison happened

#### Scenario: an unmeasured value is stated as unavailable, not silently truncated

- **GIVEN** a collision detected but `effective` cannot be measured (no local
  fallow binary)
- **WHEN** the collision is rendered
- **THEN** the report states that the project's declared value could not be
  shown, rather than falling back to a partial textual scan that would print
  an opening-brace fragment

---

## MODIFIED Requirements — Capability: `project-sync`

Adds one requirement to the existing capability at
`openspec/specs/setup/spec.md:25`. The existing requirement at line 153
("The rollback wording does not overclaim") is unchanged and continues to
govern what `Writer.Undo` does and does not cover; the requirement below adds
the retraction obligation on top of it.

### ADDED Requirement: a status line already printed is explicitly retracted, not merely contradicted, on rollback

Where a step succeeded and was reported before a later step's failure
triggers `Writer.Undo`, the closing block MUST explicitly retract that
earlier step's reported success — naming it — rather than only avoiding a
new claim of success. Today's message ("No earlier step is reported as
having succeeded", `internal/setup/setup.go:110-112`) is false the moment
stdout has already reported an earlier step's line; contradiction by
omission is not enough (defect 3).

#### Scenario: the closing block names the step whose success is retracted

- **GIVEN** step 1 succeeds and is reported, then step 2's `Apply` fails and
  `Writer.Undo` restores everything this run wrote
- **WHEN** `sync` prints its rollback report
- **THEN** the closing block explicitly names step 1 as included in the
  rollback — e.g. stating that nothing was applied "including step 1" — and
  does not merely stay silent about step 1 while asserting a general "no step
  succeeded" claim

#### Scenario: the retraction does not overclaim beyond what `Writer.Undo` covers

- **GIVEN** the same rollback, in a run where an earlier step had already run
  `EnsureDir` (creating a directory `Writer.Undo` does not remove)
- **WHEN** the retraction is rendered
- **THEN** it explicitly retracts the step's reported success without also
  claiming the directory was removed — matching the existing requirement at
  `openspec/specs/setup/spec.md:153` that the rollback wording does not
  overclaim beyond `Writer.Undo`'s actual coverage

---

## MODIFIED Requirements — Capability: `step-delegation`

Adds one requirement to the existing capability at
`openspec/specs/setup/spec.md:174`. No existing requirement in this
capability is altered; `Delegated`'s existing signature and its role as a
`Step`-interface method (the requirement at line 179) are unchanged for every
step other than the case named below.

### ADDED Requirement: a delegated collision hands back structured data the report renders, not pre-rendered prose

`boundariesOwnerStep`'s delegation MUST expose its collision as the same
structured fact its report entry is built from — the one `Collision` value
computed under `config-collision` above — rather than a pre-rendered prose
string built by a second, independent renderer. This is what makes the
`config-collision` requirement "computed once, rendered from that one value
in both views" achievable: if `Delegated` still returned finished prose
built by its own walk of the colliding keys, that walk would be free to
drift from the one the report's JSON rendering uses, reproducing defects 6
and 7 in a new form. This requirement is scoped to the collision case; it
does not change `Delegated`'s general `(why string, ok bool)` contract for
any other step, and does not reopen the requirement at
`openspec/specs/setup/spec.md:179` that `Delegated` is answered per-project
without a type assertion.

#### Scenario: the report's collision rendering and the delegated reason cannot drift apart

- **GIVEN** a run with one colliding key
- **WHEN** the report is built from `boundariesOwnerStep`'s delegation
- **THEN** the `Collision` value backing the human "Left to you" block and
  the one backing the JSON `collision` entry are the same computed value —
  not two independently-built representations of the same key that a future
  change could edit out of sync with each other

#### Scenario: every other step's `Delegated` reason stays a plain string

- **GIVEN** any step other than `boundariesOwnerStep` that delegates (e.g.
  `hookInstallStep` when no hook manager answers)
- **WHEN** its `Delegated(p)` is called
- **THEN** it still returns `(why string, ok bool)` exactly as the existing
  requirement at `openspec/specs/setup/spec.md:179` describes — this change
  does not widen every step's delegation into structured data, only the one
  case that needs it to avoid a duplicate renderer

---

## Explicit non-requirements (out of scope for this spec)

- **`check` and `mutate` gaining `--format json`.** A named follow-up; the
  model above is designed with them in view but not built for them here.
- **A remote-executor fallback for the `effective` measurement.** Rejected by
  the resolved question round: `sync` resolves fallow locally or not at all,
  never remotely. `internal/cli/check.go`'s own fallow stage is unaffected.
- **A `--show residue` flag.** Rejected by the resolved question round; the
  `## Notes` block lists residue entries in full instead.
- **Retiring `UncheckableConfigNote`'s TOML blind spot.** Whether
  `fallow config`'s reading of all four config formats retires this note is a
  separate, unverified claim, deferred to a follow-up.
- **Refactoring `renderGolden` (`internal/setup/golden_test.go:133`) to
  consume the shared `Report` model.** It stays a separate, stable consumer
  of `Plan()`/`Step`, per `TestGenericMechanismHasNoUpdatePath`
  (`internal/setup/golden_test.go:101`).

  **Correction, measured.** This clause originally read "Zero golden bytes
  move under this change." That was false, and `sdd-design` Decision 6 proved
  it: `renderGolden` prints `step.ID()` for every step
  (`internal/setup/golden_test.go:145`), and all six fixtures carry
  `resolve the two architectures this project declares` at **line 26**.
  Amending `ID()` under requirement (a) above therefore moves **six lines,
  one per fixture**, all in slice 3 — four regenerated with
  `go test ./internal/setup -run TestFrameworkGoldens -update`, two
  hand-edited, exactly as `eslint-integration` Decision 10 already did four
  times.

  No fixture gains an update path, and
  `TestGenericMechanismHasNoUpdatePath` stays green: it greps
  `golden_test.go`'s own source for `flag.Bool("update"` inside
  `TestGenericGoldenIsUnchanged`'s body, and editing a `.txt` fixture does
  not touch that source. Verified by reading the test.

  Those six lines are the complete golden impact of this change. The
  fixtures render a generic project, where `collidingKeys` returns empty and
  both collision renderers reach only their fallback constants
  (`internal/setup/steps.go:506-516`), so the whole collision restructure
  runs in branches no fixture executes. **The two fallback constants MUST
  stay byte-identical** for that to remain true.
- **Re-parsing wrapped tools' per-finding detail** for the report (§01/§09).
- **Persisting any report to a file** (§07/§08) — covered above as a
  requirement, restated here because it bounds scope as much as it states
  behaviour.
- **Any new runtime dependency.** `encoding/json` and `fmt` are the whole
  toolkit; the product stays stdlib-only.
- **Changing `sync`'s exit codes or its two stopping conditions.** This
  change pins the existing exit-0-with-delegated-work position; it does not
  alter it.

## Notes on testability

Every scenario above is phrased to map directly to a Go test: constructing a
`Report`/`StepResult`/`Collision`/`Note` value and asserting on its fields;
`applySteps`/`RunSync` against a stubbed `runner.Run` and a constructed
`project.Project`, asserting on the built `Report` and on
`app.ExitCode(err)`; `json.Unmarshal` round-trips for the `--format json`
path; byte-for-byte file-system assertions for the created/modified/unchanged
classification and for "no report file is persisted". Per §11/§17 and the
existing testability note in `openspec/specs/setup/spec.md:1021`, no
assertion reads report prose to decide pass/fail, except where a requirement
is specifically about that prose's content (the retraction wording, the
residue note, the "reported whole, never truncated" value) — and even those
assert on the presence or absence of specific facts (a key, a full value
object, an absent flag reference), not on exact spacing or heading text,
which is the mutation-hostile trap this repository has already paid for once
(`docs/learning-log.md`). A rendering test that pins layout without pinning
the underlying fact is the wrong shape of test for this change: it would
report false confidence — the layout could shuffle while the two-renderer
duplicate-key defect, or a fabricated `effective`, survived unnoticed.
