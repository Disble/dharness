# Apply progress: unify-init-and-sync — Slice 1

## Status: partial — needs a team-lead decision before settling

Phases 1–3 (step contract, extends split, rollback wording) are implemented,
TDD'd (RED confirmed for every behavioural task before GREEN), and green on
`go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`. Phase 4's
build/vet/fmt/test checks (4.1) are clean. Task 2.7's mutation check is
**partially** satisfied — see below.

## What changed

- `internal/setup/setup.go` — `Delegated(p) (why string, ok bool)` joins the
  `Step` interface; the old `Delegated` interface and its type assertion are
  deleted. `Apply` is now a thin wrapper around a new unexported
  `applySteps(steps []Step, p, stdout)`, which is what makes
  `TestApplySkipsEveryDelegatedStep` testable against a stub step without
  depending on `setup.Plan()`. Rollback wording rewritten per Decision 3, with
  the expiry comment naming `writer-undo-completeness`.
- `internal/setup/steps.go` — trivial `Delegated` (`"", false`) added to
  `installStep`, `ownedFilesStep`, `doctorConfigStep`, `mcpStep`,
  `hookInstallStep` (Decision 5 lands in a later slice — kept `false` so
  behaviour is unchanged), and — pre-split — `extendsStep` itself (removed in
  the same phase once split). `extendsStep` replaced by `fallowExtendsStep`
  and `lefthookExtendsStep` per Decision 4's state table; `wireFallowExtends`
  / `wireLefthookExtends` in `files.go` lost their error-return branch
  entirely, since the "config already exists" case is now answered by
  `Delegated` before `Apply` runs.
- `internal/cli/init.go`, `internal/cli/sync.go` — **necessary, minimal,
  out-of-slice touch.** Both used the now-deleted `setup.Delegated` type
  assertion (`step.(setup.Delegated)`); this could not compile once the
  interface method replaced it. Fixed mechanically to `step.Delegated(p)`
  with byte-identical report text — no command-merge logic touched, no
  behaviour change. Flagging this explicitly since the brief said "do not
  touch internal/cli"; it was unavoidable for Phase 1's own compile.

## Mutation gate (task 2.7) — not fully green, and why

`go run ./tools/mutationstaged` staged run score: **0.60** (24/40 killed),
below the tool's 0.80 threshold. Breakdown:

- **All extends-logic survivors are killed.** `fallowExtendsStep.Satisfied`,
  `lefthookExtendsStep.Satisfied`, and (incidentally, same short-circuit
  shape) `installStep.Satisfied` each had a `!p.HasSource() || …` /
  `hookManager(p) != managerLefthook || …` short-circuit gap; three new
  focused tests close them (`TestFallowExtendsSatisfiedWhenTheProjectHasNoSource`,
  `TestLefthookExtendsSatisfiedWhenLefthookIsNotTheHookManager`,
  `TestInstallStepSatisfiedWhenTheProjectHasNoSource`).
- **The remaining 16 survivors are pre-existing, untested code in
  `internal/cli/init.go` (6), `internal/cli/sync.go` (4), and
  `appendHuskyGate` in `internal/setup/files.go` (5)** — none of it touched
  by this slice's actual logic. `docs/mutation-testing.md` names this
  exactly: *"ooze's `Virus.Incubate` API does not identify the source file.
  Ranges from multiple staged files are therefore unioned for the runtime
  filter. This can retain an extra mutant when equal byte offsets occur in
  different files."* Because `init.go`/`sync.go` had to be staged for the
  build to compile (see above), their entire (pre-existing, currently
  zero-coverage) bodies became fair game for coincidental byte-offset overlap
  with the genuinely new code in `steps.go`/`setup.go`/`files.go`.
  `init.go` has no test file at all today, and it is deleted outright in
  Slice 2 (task 8.12) — writing full coverage for it now is speculative work
  against code that does not survive the next slice.

**This mutation gate is repository development tooling, not the commit
hook** (`docs/mutation-testing.md`: *"it is deliberately absent from the
commit hook"*), so it does not block `git commit`. It does block task 2.7 as
written.

## Line budget — over slice 1's own forecast

Current staged diff: **362 changed lines** (309 insertions + 53 deletions),
against slice 1's own ~200–230 forecast and the acquired attempt's
`max-changed-lines: 230`. Breakdown:

| File | +/- |
|---|---|
| `internal/setup/setup_test.go` | +176 |
| `internal/setup/steps.go` | +111/-  (net, incl. deletions) |
| `internal/setup/setup.go` | +40 |
| `internal/setup/files.go` | +27/- |
| `internal/cli/init.go` | +4/-4 |
| `internal/cli/sync.go` | +4/-4 |

The overage is almost entirely `setup_test.go` (176 lines): 8 new focused
tests across Phases 1–3, all directly testing new behaviour, none testing
`internal/cli` or `appendHuskyGate`. Closing the remaining 16 mutation
survivors would add an `internal/cli/init_test.go` from scratch plus
`appendHuskyGate` coverage — realistically another 100–150+ lines, pushing
well past 230 and eating into the 400-line whole-PR budget for a file
(`init.go`) that Slice 2 deletes.

## Recommendation to the team lead

Do not chase the remaining 16 survivors in this slice. They are not gaps in
Slice 1's actual logic; they are coverage debt in code this slice only had to
touch mechanically to keep compiling, in a file (`init.go`) already scheduled
for deletion next slice, and in `appendHuskyGate`, an unrelated function nowhere
in Decision 3/4's scope. Options, in order of preference:

1. **Accept task 2.7 as satisfied for the extends logic** (its literal scope)
   and record the residual score/survivor count as a known, documented
   tooling artifact — cite `docs/mutation-testing.md`'s own over-approximation
   note — rather than writing throwaway `init.go` tests.
2. If a clean mutation gate is a hard requirement for this PR, defer it to
   Slice 2, where `init.go` is deleted and `sync_test.go`/husky coverage can
   be added once, against code that survives.
3. If neither is acceptable, this needs its own follow-up task, since closing
   it here pushes the diff to ~500+ lines — bigger than Slice 2's own forecast.

## Verification evidence

- `go build ./...` — clean.
- `go vet ./...` — clean.
- `gofmt -l .` — clean.
- `go test ./...` — all packages `ok`.
- `go run ./tools/mutationstaged` — 24/40 killed, score 0.60 (below 0.80);
  all extends-logic survivors killed, remainder is cross-file over-approximation
  as documented above.

## Tasks completed

Phases 1, 2 (except the mutation-score sub-item, partially satisfied), 3, 4
in `openspec/changes/unify-init-and-sync/tasks.md`, marked `[x]` (2.7 marked
`[~]`).

---

# Apply progress: unify-init-and-sync — Slice 2

## Status: done, except one task this executor cannot perform

Phases 5–10 (`hookInstallStep` fix, `architectureStep`, the repository hard
stop, the command merge, documentation, verification) are implemented, TDD'd
(RED confirmed for every behavioural task before GREEN), and green on
`go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`. Mutation score
is 0.86 (109/127 killed), above the 0.80 threshold and up from Slice 1's 0.60.
`bash scripts/verify-gate.sh` confirms the gate still refuses.

Task 9.3 (republishing `docs/flujo-implementado.md` to the existing artifact URL) was blocked FOR THE APPLY EXECUTOR — no artifact-publish tool exists in that role. **It was completed afterwards by the orchestrator**, which does have that capability: the corrected rows, the four-command header and the Figure 1 exit-code note were ported from the .md into the artifact source and republished to the same URL, so the file and the artifact are back in sync. This paragraph originally read as unresolved and was corrected once the republish landed.

## What changed

- **`internal/setup/steps.go` — `hookInstallStep` (Decision 5).** `Satisfied`'s
  `default` case now returns `false` instead of `true`: no hook manager
  answering is an open decision, not silent success. `Delegated` answers per
  manager — lefthook/husky both `ok=false` (dharness installs), no manager
  `ok=true` with a reason. `Describe` gained the `managerNone` branch naming
  both managers and the gate check. `TestGateStepIsSatisfiedWhenNoManagerAnswers`
  was reversed into `TestGateStepIsAnOpenDecisionWhenNoManagerAnswers`, per the
  proposal's explicit instruction to reverse rather than silently delete.
- **`internal/setup/steps.go`, `internal/setup/prompt.go`, `internal/setup/setup.go`
  — `architectureStep` (Decision 6).** New step, last in `Plan()`. `Satisfied`
  is a substring check for `boundaries` in `.dharness/fallow.jsonc`, following
  the `extendsWired` precedent — no JSONC parser, stdlib-only. `Delegated`
  always `ok=true` (Intención: no detection is possible).
  `ArchitecturePrompt` lost its `## Left to you:` heading and opening
  paragraph — both moved into `Delegated`'s `why`; the rest is
  `architectureStep.Describe(p)`. Verified during 6.3: `doctorConfigStep`
  reads only `doctor.config.json` and never inspects a `.ts` file, so the
  code-not-data case from the proposal's open question is **not** delivered
  by this change — recorded in the docs correction, not silently implied.
- **`internal/project/detect.go`, `internal/project/discover.go` — the
  repository hard stop (Decision 6bis).** `Project` gains `InRepository bool`.
  `Discover` sets it true on all three success branches (via a small
  `inRepository` helper so `At`, used elsewhere, is unaffected); the swallow
  branch (not a git repository) leaves it at its zero value, unchanged.
- **`internal/cli/sync.go` — the stop, and the command merge (Decision 1).**
  `RunSync` checks `!p.InRepository` immediately after `Discover`, before the
  header, and returns the Decision 6bis error verbatim if so. `RunSync` now
  absorbs `RunInit`'s entire body: prints `Applying:` (only if at least one
  pending step is not delegated — a `hasApplicable` helper guards this so the
  header never prints over an empty block), calls `setup.Apply(p, stdout)`
  (which already prints each applied step's `ID()` itself), then renders one
  `## Left to you: <ID>` block per delegated step with `why` and
  `Describe(p)`. The old trailing `Run \`dharness init\`...` pointer line is
  gone — there is no other command to point at.
- **`internal/cli/init.go` — deleted** (68 lines). `internal/app/app.go` drops
  the `"init"` case and `UnknownCommandError`'s message; `internal/app/help.go`
  collapses the two `init`/`sync` COMMANDS entries into one.
- **`internal/cli/sync_test.go`.** Added a `gitProject(t, root, lockfiles...)`
  fixture (writes the lockfiles and stubs `project.SetGitOutputForTest` so
  `Discover` treats `root` as a repository) and a `stubRunner(t)` helper,
  because `RunSync` now applies and shells out. `TestSyncWritesNothing` was
  **deleted** (categorically false once `sync` applies); its before/after
  `tree()` byte-comparison migrated into the new
  `TestSyncStopsOutsideAGitRepository`. `TestSyncSpeaksTheProjectsOwnPackageManager`
  was rewritten to assert on the `runner.SetForTest`-captured command instead
  of report text, since a non-delegated step's command is applied silently now
  rather than described. `TestSyncSaysWhyTheDelegatedStepIsDelegated` and
  `TestSyncReachesATerminalAnswer` were extended per the design's testing
  strategy table. Six new tests cover the merged report shape, the
  already-configured-fallow case, delegated-step safety, the no-JS-project
  exit-0 path, and Decision 3's rollback wording through `RunSync` itself
  (not just through `setup.Apply` directly).
- **`internal/project/discover_test.go`.** Two new tests pin `InRepository`:
  true on both success branches (one JS project, no JS project), false outside
  a repository.
- **`internal/app/app_test.go`.** Extended the unknown-command test to assert
  the message no longer contains `init`; added
  `TestRunArgsInitIsUnknown` confirming `init` now dispatches as an unknown
  command like any other typo.
- **`docs/flujo-implementado.md`** (untracked in git before this change — see
  note below). Corrected per the proposal's "What this change makes false"
  table: header line 11 "Cinco comandos" → "Cuatro comandos"; the Fusión row
  qualified to the `extends` case only, with the `.ts` non-delivery from 6.3
  stated explicitly; the Conducción row states the actual Decision 1 report
  shape (`## Left to you: <paso>` / `dharness cannot run this: <motivo>`);
  the Intención row no longer claims to be the only real prompt. Added a note
  under Figure 1 explaining that its left-hand red box draws two states (no
  repository, no JS project) as one outcome but two different exit codes.
- **`docs/learning-log.md`.** One dated line appended per task 9.4.

## Line budget — measured, and materially over both the slice forecast and the hard budget

Measured via `git diff --stat` scoped to exactly the files this slice touched,
against the index state Slice 1 left staged (so Slice 1's own lines are
excluded):

| File | +/- |
|---|---|
| `internal/cli/sync_test.go` | +246 (net, incl. deletions) |
| `internal/setup/setup_test.go` | +69 |
| `internal/setup/steps.go` | +61 |
| `internal/cli/sync.go` | +61 |
| `internal/project/discover_test.go` | +45 |
| `internal/cli/init.go` | −68 (deletion) |
| `internal/app/app_test.go` | +18 |
| `internal/app/help.go` | +11 |
| `internal/project/discover.go` | +14 |
| `internal/setup/prompt.go` | +9 |
| `internal/project/detect.go` | +5 |
| `internal/app/app.go` | +4 |
| `internal/setup/setup.go` | +1 |
| `docs/learning-log.md` | +1 |

**Total: 472 insertions + 141 deletions = 613 changed lines.** This is
materially over the slice's own ~270–315 forecast and over the 400-line hard
budget the runtime attempt was acquired against. The design's own fallback —
cutting Decision 6bis into a third slice, since it touches only
`internal/project` — would not have closed the gap by itself: Decision 6bis's
own footprint (`detect.go` + `discover.go` + `discover_test.go` + the
`RunSync` check + one new sync test) is roughly 90 of the 613 lines. The bulk
is the test suite the command merge itself requires: `sync_test.go` alone is
246 of the 613, driven by the `gitProject`/`stubRunner` fixtures every
existing test now needs (Decision 6bis makes `InRepository` production-real,
so a bare `t.TempDir()` can no longer reach the plan without a git stub) plus
six new tests covering behaviour `RunInit` had zero tests for before this
change. Splitting further at this point would be a commit-organization
exercise over already-implemented, already-green code, not a scope reduction
— reported here rather than acted on unilaterally, per the team lead's
instruction.

`docs/flujo-implementado.md` (581 lines) is **not counted** in the 613: it was
never tracked by git before this session (`git ls-files` finds no history for
it), so the whole file appears as a new addition in any diff regardless of how
much of it this slice actually touched (five sections: the header count, the
Figure 1 note, and the three "Qué existe hoy" table cells).

## Verification evidence

- `go build ./...` — clean.
- `go vet ./...` — clean.
- `gofmt -l .` — clean.
- `go test ./...` — all packages `ok` (literal output in the executor's final
  report).
- `go run ./tools/mutationstaged -dry` then real — 127 candidate mutants over
  the 9 staged production files (`app.go`, `help.go`, `sync.go`, `detect.go`,
  `discover.go`, `files.go`, `prompt.go`, `setup.go`, `steps.go`), **109
  killed, 18 survived, score 0.86** (minimum 0.80). Up from Slice 1's 0.60.
  Survivors: 6 in `app.go` (Integer Decrement/Increment — help/version string
  paths this slice did not add behavioural tests for), 1 in `sync.go`
  (Comparison Invert), 3 in `detect.go` (Comparison Replace ×2, Integer
  Increment), 7 in `files.go` (Arithmetic Assignment, Comparison Invert ×3,
  Comparison Replace ×3 — the same `appendHuskyGate`/comparison surface Slice
  1's notes already named as pre-existing, undertested code this change does
  not own), 1 in `setup.go` (Loop Break). Not chased, per the team lead's
  explicit instruction, since the score is above threshold.
- `bash scripts/verify-gate.sh` — `verify-gate: the hook refused a broken
  file, as it must.`

## Tasks completed

Phases 5–10 in `openspec/changes/unify-init-and-sync/tasks.md`, marked `[x]`,
except 9.3 (blocked for this executor, later completed by the orchestrator;
left `[ ]` with the reason recorded inline).
