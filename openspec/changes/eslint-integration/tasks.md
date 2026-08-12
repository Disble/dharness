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

- [ ] 2b.1 Delete `doctorConfigStep`/`doctorConfigFile` from `internal/setup/steps.go`; `Plan()` in `setup.go` drops the step (eleven → ten). Obs: `go build ./...`; no remaining `RulesPackage`-under-`plugins` write.
- [ ] 2b.2 Rewrite `internal/setup/plugin.go`'s comment block: keep the eight-non-actionable-findings measurement, replace the first-write-only limit with why it no longer applies (Decision 8). Obs: `go vet ./...` clean.
- [ ] 2b.3 **Hand-authored golden edit — the second** (`testdata/golden/generic-conventional.txt`, `generic-split.txt`): `== plan ==` `doctorConfigStep`'s row removed (eleven steps → ten); `== tree ==` `doctor.config.json` removed. Nothing else — `.dharness/eslint.config.js` and its allow-list entry already landed in 2a.8. Never via `-update` — `TestGenericMechanismHasNoUpdatePath` (`internal/setup/golden_test.go:100`) forbids it. Obs: `go test ./internal/setup/...` green; that test itself unmodified and still passing. Note: the three framework fixtures (`nextjs.txt`, `wails.txt`, `wails-nextjs.txt`) take **this slice's own delta** — they lose `doctor.config.json` and the same plan row — and regenerate with `-update`. `renderGolden` runs the plan for every fixture, so five files change here too, not two.

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

- [ ] 3a.1 `internal/setup/eslintregion.go` (new): the four marker constants, `markerState` (absent/present/malformed), `markerRegion(raw, begin, end) (from, to int, state)`. Obs: `TestMarkerRegionDistinguishesAbsentFromMalformed` — all four malformed shapes.
- [ ] 3a.2 `internal/setup/files.go`: detect `eslint.config.js`/`.mjs`/`.cjs` vs `.ts`/`.mts`/`.cts` vs `.eslintrc.*`-only vs absent (`eslintFlatConfig`/`eslintLegacyConfig`). Obs: one fixture per case.
- [ ] 3a.3 `eslintExtendsStep.Delegated(p)`: `ok == true` only for `.ts`/`.mts`/`.cts`, an unrecognised call, an ERROR node, or a malformed marker pair. Obs: table-driven test over every `eslint-config-splice` refusal-matrix cell in spec.md.
- [ ] 3a.4 `eslintExtendsStep.Apply` write-if-absent path, matching `wireFallowExtends`'s shape. Obs: spec scenario "a project with no ESLint config gets one written".
- [ ] 3a.5 Legacy `.eslintrc.*`-only delegation, matching `legacyLintConfigStep`'s wording. Obs: spec scenario "a project with only `.eslintrc.json` delegates".
- [ ] 3a.6 **Hand-authored golden edit — the third** — `== plan ==`: `eslintExtendsStep`'s row added (ten → eleven); `== tree ==`: `eslint.config.js` added at `<root>`/`<source>`. Never via `-update`. Obs: `go test ./internal/setup/...` green; `TestGenericMechanismHasNoUpdatePath` unmodified.

## Slice 3b — insert/replace paths, re-parse guard (~420 lines, exceeds 400-line budget)

Capability: `eslint-config-splice` (convergence-by-replacement, byte survival
outside markers, verify-then-rollback, idempotency). Why here: the only slice
with a destructive edit — it is the entire subject of its own review, and
further splitting it would only hide that fact.

- [ ] 3b.1 Insert path: `Analyze(src)` for the anchor; splice the layer region then the import region (later offset first — `ImportAt < LayerAt` always). Obs: `TestSpliceInsertsAndChangesNothingElse` exercised through the step.
- [ ] 3b.2 Replace path: `markerState == present` and bytes differ → rewrite each region in place at the marker-scan bounds, never a second insertion. Obs: `TestPresentMarkersWithStaleBytesAreReplacedNotDuplicated`.
- [ ] 3b.3 In-transaction re-parse guard on both paths: `Analyze(candidate)` + marker scan assert no ERROR node, exactly one region of each kind, correct element-count delta; failure → error → `Writer.Undo`. Obs: `TestSpliceGuardRollsBackAnUnparseableResult` — original bytes restored, step reported failed.
- [ ] 3b.4 Idempotency. Obs: `TestSecondSyncWritesNothing` — `Satisfied` true, `Apply` not called, bytes equal.
- [ ] 3b.5 CRLF/BOM preserved end-to-end through the step. Obs: 1.9's constructed bytes replayed through `eslintExtendsStep.Apply`.
- [ ] 3b.6 Mutation guard: insert/replace branch on `markerState` — present-and-stale markers must end with exactly one region of each kind, or the "always insert" collapse survives. Obs: killed mutant in `go run ./tools/mutationstaged`.

## Slice 4 — gate stage (~260 lines)

Capability: `eslint-gate-stage` (all requirements). Why here: independent of
3b — touches `internal/cli`, `internal/tool`, `internal/project` only; can
move ahead of 3b if it overruns.

**MERGE CONDITION — gates this slice.** The placement measurement: wall-clock
per stage over the same explicit staged file list on the reference
repository, three runs, median, recorded as a dated `docs/learning-log.md`
line. **OPEN QUESTION — closes here.** Placement stays provisional (last,
after `fallow dupes`) until this measurement; do not close by reasoning about
ESLint's cost profile in isolation.

- [ ] 4.1 `internal/cli/check.go`: `stage{tool, command runner.Command, help runner.Command}` replaces `stage{tool, args}`; `remoteStage`/`localStage` build the two shapes; loop drops `RemoteLatest`, gains `stage.command`. Obs: existing `internal/cli/check_test.go` react-doctor/fallow cases pass with the new shape.
- [ ] 4.2 `internal/project/git.go`: `StagedSourceFilesFromSource()` strips the prefix `StagedSourceFiles` already filtered on. Obs: `TestEslintStagePathsAreRelativeToSource` on a split fixture.
- [ ] 4.3 `internal/tool/tool.go`: `const ESLint = "eslint"`, `func ESLintStaged(files []string) []string`, no `--cache`. Obs: argument-slice assertion — `--cache` absent.
- [ ] 4.4 Stage resolves via `p.LocalBinary("eslint")`; absent → not built, named in output, exit unaffected. Obs: `TestEslintStageIsSkippedWithoutABinary`.
- [ ] 4.5 Run the placement measurement (merge condition above); place the stage per its result, citing the learning-log line in the commit.
- [ ] 4.6 `docs/design-principles.md`: record the ESLint local-resolution exception beside the existing 10 August 2026 §03 amendment (`docs/design-principles.md:80`). Obs: spec scenario "the exception is named where the general rule is recorded".

## Slice 5 — preset layer contribution (~260 lines)

Capability: `preset-layer-contribution` (remaining requirements). Why here:
framework goldens regenerate via `-update` here, never for the generic
fixture.

**OPEN QUESTION — closes here.** `Layer.Binding` collisions across two
matched presets are unreachable today; add `TestNoBindingIsContributedTwice`
(`TestNoScalarKeyIsContributedTwice`'s shape) so a future fifth preset
degrades visibly instead of emitting a duplicate `const`.

- [ ] 5.1 `Manifest.Validate`: `Package` carries no `@` after position 0; `Binding` matches `[A-Za-z_$][A-Za-z0-9_$]*` and carries the `dharness` prefix (no reserved-word table — Decision 7 cut). Obs: `TestLayerValidateRejectsAPinnedVersion` (design's Testing Strategy table); `TestLayerValidateRejectsAnInvalidBinding` and `TestBareBindingIsRejectedAtBuildTime` — names derived from spec.md's scenario titles, not cited verbatim in design.md's Testing Strategy table.
- [ ] 5.2 `internal/preset/nextjs.go`: `Layer{Package: "eslint-config-next", Binding: "dharnessNext", Because: <published/versioned by Next.js itself>}`. Obs: registry `Validate` test passes; `TestInstallIncludesPresetContributedPackages` (Next.js fixture).
- [ ] 5.3 `internal/preset/expo.go`: `Layer{Package: "eslint-config-expo", Binding: "dharnessExpo", Because: <published/versioned by Expo itself>}`. Obs: same registry test, Expo fixture.
- [ ] 5.4 `TestNoBindingIsContributedTwice` (open question above).
- [ ] 5.5 `internal/setup/steps.go`: `integrationPackages(p)` takes a project, appends `preset.Layers(preset.Resolve(p))` packages to the fixed `RulesPackage` set, deduped. Obs: `TestInstallIncludesPresetContributedPackages` (design's Testing Strategy table); `TestFailedInstallRollsBackOnlyWhatThisRunAdded` (derived from spec.md's scenario title, not cited verbatim in design.md) — no new rollback mechanism.
- [ ] 5.6 Regenerate the framework golden fixtures (Next.js, Expo, and the existing third) via `-update` — never the generic fixture. Obs: `go test ./internal/setup/... -update`, diff reviewed, generic fixture byte-identical to 3a's.

---

## Before every commit

`go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l .`,
`go run ./tools/mutationstaged` (floor 0.80). Slice 1 additionally requires
`CGO_ENABLED=0 go build ./...` for all six release targets, re-verified
whenever `internal/jsconfig` changes.
