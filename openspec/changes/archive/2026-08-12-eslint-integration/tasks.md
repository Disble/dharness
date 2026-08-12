# Tasks: eslint-integration

Inputs: `spec.md` (four capabilities), `design.md` (eleven decisions, the
authoritative Line Forecast slice plan), `proposal.md` (scope, slice plan),
`exploration.md`. TDD: each task's named observable is the RED test, written
before its implementation and made GREEN; `go run ./tools/mutationstaged`
(MUTATE, floor 0.80) closes every slice per P09.

**Word-budget note.** This artifact exceeds the sdd-tasks skill's 530-word
guideline. The orchestrator's brief mandates a file-plus-observable per task,
all three golden hand-edits (2a, 2b, 3a), both gating merge conditions, and
all five open questions — a change of this size (seven slices,
~1,900–2,150 lines) cannot carry that detail under 530 words. Completeness of
the mandated content is kept; brevity is sacrificed deliberately, not by
omission.

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | ~1,900–2,150 (design's Line Forecast) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR1(slice 1) → PR2(2a) → PR3(2b) → PR4(3a) → PR5(3b) → PR6(4) → PR7(5) |
| Delivery strategy | auto-chain (proposal.md's Slice plan) |
| Chain strategy | stacked-to-main |

```text
Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High
```

Slices 1 (~420) and 3b (~420) individually exceed the 400-line review budget
even alone; flagged here rather than silently re-split — Decision 1's "slice 1
stays one slice" argument (a split package with no caller is not reviewable
for behaviour) and slice 3b's status as "the entire subject of its own
review" both make further splitting worse than the overrun.

### Suggested Work Units

| Unit | Goal | PR | Focused test | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 1 | `internal/jsconfig`: parse, refusals, position rule, byte-surgery | PR1 | `go test ./internal/jsconfig/...` | N/A — pure functions over `[]byte`, no project needed | Revert PR1; dependency leaves with it |
| 2a | Owned factory config with its severities, `ownedFilesStep` writing it, allow-list repair; golden hand-edit 1 (tree only) | PR2 | `go test ./internal/setup/...` | golden diff review | Revert PR2 |
| 2b | Pure removal: `doctorConfigStep` deleted, `doctor.config.json` off disk; golden hand-edit 2 (plan+tree) | PR3 | `go test ./internal/setup/...` | golden diff review | Revert PR3 |
| 3a | `eslintExtendsStep` write-if-absent + refusal matrix; golden hand-edit 3 | PR4 | `go test ./internal/setup/...` | golden diff review | Revert PR4 |
| 3b | Splice insert/replace, re-parse guard, idempotency | PR5 | `go test ./internal/setup/... -run EslintExtends` | adversarial fixture in `t.TempDir()` | Revert PR5; markers self-describing, hand-removable |
| 4 | Gate stage: local resolution, re-based paths, placement | PR6 | `go test ./internal/cli/... ./internal/project/... ./internal/tool/...` | `runner.Run` stub + `SetGitOutputForTest` | Revert PR6; independent of 3b |
| 5 | Preset layer contribution: Next.js/Expo, install rollback | PR7 | `go test ./internal/preset/... ./internal/setup/...` | framework goldens via `-update` | Revert PR7 |

---

## Slice 1 — `internal/jsconfig` (~420 lines, exceeds 400-line budget)

Capability: `eslint-config-splice` (parser dependency requirement). Why here:
read-only, no step, no golden change — the dependency is reviewable alone.

- [x] 1.1 `go.mod`/`go.sum`: add `github.com/odvcencio/gotreesitter`. Obs: `go build ./...` and `CGO_ENABLED=0 go build ./...` succeed for all six release targets.
- [x] 1.2 `AGENTS.md`: record the dependency as a stated deviation from stdlib-only. Obs: Deviations section names it and the `CGO_ENABLED=0` constraint.
- [x] 1.3 `internal/jsconfig/jsconfig.go`: `Anchor{ImportAt, LayerAt, Indent, LineEnding}`, `Analyze(src []byte) (Anchor, string, bool)`, `Splice(src []byte, at int, region string) []byte`. No framework param (Decision 5 cut), no `Elements` field (Decision 1 cut). Obs: package imports neither `internal/setup` nor `internal/preset`.
- [x] 1.4 Import table: binding → module specifier. Obs: `TestDefineConfigResolvesByImportSpecifier`.
- [x] 1.5 Default-export shapes: array literal / `defineConfig([...])` from `"eslint/config"` / other call / ERROR node. Obs: `TestAnalyzeRefusesEveryRefusalMatrixCell`, `TestDefineConfigWithAnErrorNodeArgumentRefuses`.
- [x] 1.6 Position rule: index 0 always, no framework-spread recognition. Obs: `TestLayerLandsAtTheFirstElement` (including a framework-spread first element).
- [x] 1.7 Comment-run scan, blank-line boundary. Obs: `TestCommentRunPrecedingTheAnchorIsNotOrphaned`.
- [x] 1.8 `Splice` byte-surgery identity. Obs: `TestSpliceInsertsAndChangesNothingElse`.
- [x] 1.9 CRLF/BOM built in Go from the committed LF fixture (Decision 11); `.gitattributes` untouched. Obs: `TestCRLFIsMatchedNotNormalised`, `TestBOMSurvives`.
- [x] 1.10 Mutation guard: `ImportAt`/`LayerAt` ordering fixture (later offset spliced first). Obs: killed mutant in `go run ./tools/mutationstaged`, not merely covered.

## Slice 2a — owned factory config + allow-list repair (~250+30 lines, new scope)

Capability: `preset-layer-contribution` (Layer type only) and
`eslint-config-splice` (severity home, mechanism). Why here: Decision 2 is a
correctness fix absent from the proposal — without it the owned file is
gitignored forever in every already-adopted repository.

**MERGE CONDITION — SATISFIED, 12 August 2026.** Decision 3's split-layout
resolution probe ran and reproduces: the owned config at `Root/.dharness/`
importing the plugin by bare specifier fails `ERR_MODULE_NOT_FOUND` while the
byte-identical import from `frontend/` resolves. The factory shape stands and
this slice is unblocked. Recorded in `docs/learning-log.md`.

- [x] 2a.1 Run the probe; record the dated result in `docs/learning-log.md`.
- [x] 2a.2 `internal/preset/preset.go`: `Layer{Package, Binding, Because}`, `Manifest.Layers []Layer`, `func Layers(matches []Match) []Layer` returning empty (real contributions land in slice 5, per design's recommended ordering). Obs: `dharness.preset/v1` schema string unchanged; package builds.
- [x] 2a.3 `internal/setup/eslintconfig.go` (new): `ownedEslintConfig(p, layers)` renders the factory over `DefaultSeverity` per rule. Obs: unit test on a constructed `layers` slice — rendered parameter list matches the binding list byte-for-byte.
- [x] 2a.4 `internal/project/evidence.go`: `dirIgnore` gains `!eslint.config.js`; `evidence_test.go`'s shared-list assertion gains it. Obs: `TestOwnedEslintConfigIsDeclaredShared`.
- [x] 2a.5 `internal/setup/files.go`: `ensureShared(p, w, name)` appends a missing entry, never rewrites. Obs: `TestExistingAllowListGainsTheMissingEntry`, `TestAllowListRepairKeepsWhatTheProjectAdded`.
- [x] 2a.6 `ownedFilesStep.Satisfied` gains the repair clause. Obs: entry removed by hand → `Satisfied == false`.
- [x] 2a.7 Wire `ownedFilesStep.Apply` to call `ownedEslintConfig` + `ensureShared`. Obs: a fresh `.dharness/` after `Apply` holds `eslint.config.js` and an allow list naming it.
- [x] 2a.8 **Hand-authored golden edit — the first, and it is `== tree ==` only** (`testdata/golden/generic-conventional.txt`, `generic-split.txt`): `.dharness/eslint.config.js` added complete with its severities; `.dharness/.gitignore` gains `!eslint.config.js`. `== plan ==` is **unchanged** — this slice adds and removes no step. Never via `-update` — `TestGenericMechanismHasNoUpdatePath` (`internal/setup/golden_test.go:100`) forbids it. Obs: `go test ./internal/setup/...` green; that test itself unmodified and still passing. Note: the three framework fixtures (`nextjs.txt`, `wails.txt`, `wails-nextjs.txt`) take the identical tree delta and regenerate with `-update` — `renderGolden` runs the plan for every fixture, so five files change here, not two.

Note for 2a's review, so it does not get "fixed": between 2a and 2b the six
severities exist in **both** `.dharness/eslint.config.js` and
`doctor.config.json`. That is deliberate and is not the two-homes drift this
repository keeps finding — both render from `DefaultSeverity`, which stays the
single source, and 2b deletes the second home.

Why the golden edit is 2a's and not 2b's: `renderGolden` renders the plan and
then calls `Apply` (`internal/setup/golden_test.go:140-153`), so a file lands
in `== tree ==` in whichever slice wires its write. A 2a that built the render
and repair functions without wiring them would have no observable behaviour at
all — the dead-code failure that merged slices 1a and 1b back together.

## Slice 2b — severities move; `doctorConfigStep` deleted (~200 lines)

Capability: `eslint-config-splice` (severity-home requirement). Why here: risk
isolation from 2a — 2a is a correctness fix 2b depends on, so the split stays.

- [x] 2b.1 Delete `doctorConfigStep`/`doctorConfigFile` from `internal/setup/steps.go`; `Plan()` in `setup.go` drops the step (eleven → ten). Obs: `go build ./...`; no remaining `RulesPackage`-under-`plugins` write.
- [x] 2b.2 Rewrite `internal/setup/plugin.go`'s comment block: keep the eight-non-actionable-findings measurement, replace the first-write-only limit with why it no longer applies (Decision 8). Obs: `go vet ./...` clean.
- [x] 2b.3 **Hand-authored golden edit — the second** (`testdata/golden/generic-conventional.txt`, `generic-split.txt`): `== plan ==` `doctorConfigStep`'s row removed (eleven steps → ten); `== tree ==` `doctor.config.json` removed. Nothing else — `.dharness/eslint.config.js` and its allow-list entry already landed in 2a.8. Never via `-update` — `TestGenericMechanismHasNoUpdatePath` (`internal/setup/golden_test.go:100`) forbids it. Obs: `go test ./internal/setup/...` green; that test itself unmodified and still passing. Note: the three framework fixtures (`nextjs.txt`, `wails.txt`, `wails-nextjs.txt`) take **this slice's own delta** — they lose `doctor.config.json` and the same plan row — and regenerate with `-update`. `renderGolden` runs the plan for every fixture, so five files change here too, not two.
- [x] 2b.4 **Arrived from the verify pass, 12 August 2026.** The "removed" half of spec.md's residue scenario shipped in 2b.1–2b.3; the "reported" half — `sync`'s output names an already-adopted repository's `doctor.config.json` residue as inert under the gate's `--staged` invocation, never silently — was never planned or built. `internal/setup/steps.go`: `EslintResidueNote(p)`, `UncheckableConfigNote`'s exact shape (note beside the plan, not a step; "" when there is nothing to report), detecting `RulesPackage`/any `RuleIDs()` entry via `declaredKeys` over `doctor.config.json`. Wired into `RunSync` (`internal/cli/sync.go`) under its own `## Residue` heading — neither "Not checked" (a blind spot) nor "Assumed" (a guessed default) fits, since dharness reads the file fine and knows exactly what it holds. Computed in `RunSync`, not `Plan()`, so no golden fixture is touched. Obs: spec scenario "residue in an already-adopted repository is reported, never removed", asserted directly — `TestSyncReportsEslintResidueInAnAlreadyAdoptedRepository` (report names the residue and the `--staged` reason; file bytes unchanged after `sync`), `TestSyncSaysNothingAboutEslintResidueWhenThereIsNone` (silent with no `doctor.config.json`).

## Slice 3a — `eslintExtendsStep` write-if-absent + refusal matrix (~300 lines)

Capability: `eslint-config-splice` (write-if-absent, refusal matrix, distinct
marker pairs) and the MODIFIED `step-delegation` requirement. Why here: every
delegation path lands before any edit to an existing file.

**OPEN QUESTION — closes here.** The four marker strings ship into every
adopting repository and cannot be changed cheaply afterwards. Settle the exact
spelling in this slice's review (Decision 4), not afterwards.

**Noted, not closed here.** The call-expression vs. array-literal distribution
of real `eslint.config.js` shapes stays unmeasured; this slice's answer
(`defineConfig` splices, everything else delegates) holds regardless of the
number, so no task here depends on it.

- [x] 3a.1 `internal/setup/eslintregion.go` (new): the four marker constants, `markerState` (absent/present/malformed), `markerRegion(raw, begin, end) (from, to int, state)`. Obs: `TestMarkerRegionDistinguishesAbsentFromMalformed` — all four malformed shapes.
- [x] 3a.2 `internal/setup/files.go`: detect `eslint.config.js`/`.mjs`/`.cjs` vs `.ts`/`.mts`/`.cts` vs `.eslintrc.*`-only vs absent (`eslintFlatConfig`/`eslintLegacyConfig`). Obs: one fixture per case.
- [x] 3a.3 `eslintExtendsStep.Delegated(p)`: `ok == true` only for `.ts`/`.mts`/`.cts`, an unrecognised call, an ERROR node, or a malformed marker pair. Obs: table-driven test over every `eslint-config-splice` refusal-matrix cell in spec.md.
- [x] 3a.4 `eslintExtendsStep.Apply` write-if-absent path, matching `wireFallowExtends`'s shape. Obs: spec scenario "a project with no ESLint config gets one written".
- [x] 3a.5 Legacy `.eslintrc.*`-only delegation, matching `legacyLintConfigStep`'s wording. Obs: spec scenario "a project with only `.eslintrc.json` delegates".
- [x] 3a.6 `internal/setup/prompt.go`: `ArchitecturePrompt` still sends the reader to `doctor.config.json` to turn the barrel rule on (`prompt.go:87`, via `doctorPath`). Slice 2b deleted that file, so the advice names something dharness no longer writes — and `TestArchitecturePromptSaysHowToTurnOnTheBarrelRule` currently pins the wrong string, which is a test defending a lie. Point it at the project's own `eslint.config.js` layer, which this slice is the first to make real, and retire `doctorPath`/`doctorConfig` with it. Obs: that test asserts the new destination; `rg doctorConfig internal/` returns nothing outside the golden fixtures' history.
- [x] 3a.7 **Hand-authored golden edit — the third** — `== plan ==`: `eslintExtendsStep`'s row added (ten → eleven); `== tree ==`: `eslint.config.js` added at `<root>`/`<source>`. Never via `-update`. Obs: `go test ./internal/setup/...` green; `TestGenericMechanismHasNoUpdatePath` unmodified.

## Slice 3b — insert/replace paths, re-parse guard (~420 lines, exceeds 400-line budget)

Capability: `eslint-config-splice` (convergence-by-replacement, byte survival
outside markers, verify-then-rollback, idempotency). Why here: the only slice
with a destructive edit — it is the entire subject of its own review, and
further splitting it would only hide that fact.

- [x] 3b.1 Insert path: `Analyze(src)` for the anchor; splice the layer region then the import region (later offset first — `ImportAt < LayerAt` always). Obs: `TestSpliceInsertsAndChangesNothingElse` exercised through the step.
- [x] 3b.2 Replace path: `markerState == present` and bytes differ → rewrite each region in place at the marker-scan bounds, never a second insertion. Obs: `TestPresentMarkersWithStaleBytesAreReplacedNotDuplicated`.
- [x] 3b.3 In-transaction re-parse guard on both paths: `Analyze(candidate)` + marker scan assert no ERROR node and exactly one well-formed region of each kind; failure → error → `Writer.Undo`. **No element-count assertion** — design.md Decision 1 cuts it as redundant and, on the replace path, wrong: the count is unchanged there, so a constant `N+1` fails a correct write. An earlier draft of this line said "correct element-count delta"; that phrasing predated the cut and was never true. Obs: `TestSpliceGuardRollsBackAnUnparseableResult` — original bytes restored, step reported failed.
- [x] 3b.4 Idempotency. Obs: `TestSecondSyncWritesNothing` — `Satisfied` true, `Apply` not called, bytes equal.
- [x] 3b.5 CRLF/BOM preserved end-to-end through the step. Obs: 1.9's constructed bytes replayed through `eslintExtendsStep.Apply`.
- [x] 3b.6 Mutation guard: insert/replace branch on `markerState` — present-and-stale markers must end with exactly one region of each kind, or the "always insert" collapse survives. Obs: killed mutant in `go run ./tools/mutationstaged`.

## Slice 4 — gate stage (~260 lines)

Capability: `eslint-gate-stage` (all requirements). Why here: independent of
3b — touches `internal/cli`, `internal/tool`, `internal/project` only; can
move ahead of 3b if it overruns.

**MERGE CONDITION — SATISFIED, 12 August 2026.** The placement measurement ran:
wall-clock per stage over the same explicit staged file list (5 files) on a
reference project, three runs, median, `docs/learning-log.md`. **OPEN
QUESTION — closed here, against the provisional answer.** ESLint measured
1008 ms — cheaper than fallow dupes (1398 ms), fallow audit (2102 ms), and
react-doctor (2959 ms), the reverse of the "provisional last" assumption. It
runs first in the gate, not last; react-doctor and fallow keep the relative
order they already had.

- [x] 4.1 `internal/cli/check.go`: `stage{tool, command runner.Command, help runner.Command}` replaces `stage{tool, args}`; `remoteStage`/`localStage` build the two shapes; loop drops `RemoteLatest`, gains `stage.command`. Obs: existing `internal/cli/check_test.go` react-doctor/fallow cases pass with the new shape.
- [x] 4.2 `internal/project/git.go`: `StagedSourceFilesFromSource()` strips the prefix `StagedSourceFiles` already filtered on. Obs: `TestEslintStagePathsAreRelativeToSource` on a split fixture.
- [x] 4.3 `internal/tool/tool.go`: `const ESLint = "eslint"`, `func ESLintStaged(files []string) []string`, no `--cache`. Obs: argument-slice assertion — `--cache` absent.
- [x] 4.4 Stage resolves via `p.LocalBinary("eslint")`; absent → not built, named in output, exit unaffected. Obs: `TestEslintStageIsSkippedWithoutABinary`.
- [x] 4.5 Run the placement measurement (merge condition above); place the stage per its result, citing the learning-log line in the commit. **Result: ESLint measured cheapest of the four (1008 ms median), not most expensive — it now runs first, not last as the provisional placement assumed.** `docs/learning-log.md`, 12 August 2026.
- [x] 4.6 `docs/design-principles.md`: record the ESLint local-resolution exception beside the existing 10 August 2026 §03 amendment (`docs/design-principles.md:80`). Obs: spec scenario "the exception is named where the general rule is recorded".

## Slice 5 — preset layer contribution (~260 lines)

Capability: `preset-layer-contribution` (remaining requirements). Why here:
framework goldens regenerate via `-update` here, never for the generic
fixture.

**OPEN QUESTION — closes here.** `Layer.Binding` collisions across two
matched presets are unreachable today; add `TestNoBindingIsContributedTwice`
(`TestNoScalarKeyIsContributedTwice`'s shape) so a future fifth preset
degrades visibly instead of emitting a duplicate `const`.

- [x] 5.1 `Manifest.Validate`: `Package` carries no `@` after position 0; `Binding` matches `[A-Za-z_$][A-Za-z0-9_$]*` and carries the `dharness` prefix (no reserved-word table — Decision 7 cut). Obs: `TestLayerValidateRejectsAPinnedVersion` (design's Testing Strategy table); `TestLayerValidateRejectsAnInvalidBinding` and `TestBareBindingIsRejectedAtBuildTime` — names derived from spec.md's scenario titles, not cited verbatim in design.md's Testing Strategy table.
- [x] 5.2 `internal/preset/nextjs.go`: `Layer{Package: "eslint-config-next", Binding: "dharnessNext", Because: <published/versioned by Next.js itself>}`. Obs: registry `Validate` test passes; `TestInstallIncludesPresetContributedPackages` (Next.js fixture).
- [x] 5.3 `internal/preset/expo.go`: `Layer{Package: "eslint-config-expo", Binding: "dharnessExpo", Because: <published/versioned by Expo itself>}`. Obs: same registry test, Expo fixture.
- [x] 5.4 `TestNoBindingIsContributedTwice` (open question above).
- [x] 5.5 `internal/setup/steps.go`: `integrationPackages(p)` takes a project, appends `preset.Layers(preset.Resolve(p))` packages to the fixed `RulesPackage` set, deduped. Obs: `TestInstallIncludesPresetContributedPackages` (design's Testing Strategy table); `TestFailedInstallRollsBackOnlyWhatThisRunAdded` (derived from spec.md's scenario title, not cited verbatim in design.md) — no new rollback mechanism.
- [x] 5.6 Regenerate the framework golden fixtures (Next.js, Expo, and the existing third) via `-update` — never the generic fixture. Obs: `go test ./internal/setup/... -update`, diff reviewed, generic fixture byte-identical to 3a's.

---

## Before every commit

`go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l .`,
`go run ./tools/mutationstaged` (floor 0.80). Slice 1 additionally requires
`CGO_ENABLED=0 go build ./...` for all six release targets, re-verified
whenever `internal/jsconfig` changes.
