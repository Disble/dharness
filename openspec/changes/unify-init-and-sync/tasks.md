# Tasks: unify-init-and-sync

Chain: PR1 = Slice 1, PR2 = Slice 2, stacked-to-main. Slice 1 merges to main first (closes the rollback bug alone); Slice 2 opens on top, targets main once Slice 1 lands.

## Slice 1 — Step contract + extends steps

### Review Workload Forecast — Slice 1

| Field | Value |
|---|---|
| Estimated changed lines | ~200–230 |
| 400-line budget risk | Low |
| Chained PRs recommended | Yes (this is PR1 of 2) |
| Chain strategy | stacked-to-main |
| Delivery strategy | auto-chain |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: Low

**Work unit**: PR1. Test: `go test ./internal/setup/...`. Runtime: `go run ./tools/mutationstaged` over `internal/setup/steps.go`, `files.go`. Rollback: revert the PR; `Delegated` returns to the old type assertion, no persisted state.

#### Phase 1: Step interface (spec: step-delegation, reqs 1–2)
- [x] 1.1 RED `setup_test.go`: `TestApplySkipsEveryDelegatedStep` — stub step, `Delegated` ok=true, assert `Apply` never called
- [x] 1.2 GREEN `setup.go`: `Delegated(p) (why string, ok bool)` joins `Step`; delete `Delegated` interface + type assertion; `Apply` loop calls `step.Delegated(p)` (split into `applySteps` so the contract is testable against a stub, `Apply` now a thin wrapper)
- [x] 1.3 GREEN `steps.go`: trivial `Delegated` (`"", false`) on `installStep`, `ownedFilesStep`, `doctorConfigStep`, `mcpStep` — also added to `extendsStep` (pre-split, temporary) and `hookInstallStep` (Decision 5 lands later), both required for `Step` compliance and not called out explicitly in this task
- [x] 1.4 GREEN `steps.go`: `agentSkillStep.Why()` → `Delegated(project.Project) (string, bool)` returning existing text, `true`; `Apply`'s delegated-error guard kept, asserted unreachable via `TestAgentSkillApplyIsUnreachable`

#### Phase 2: `extendsStep` splits (Decision 4; spec: step-delegation reqs 2–4)
- [x] 2.1 RED `setup_test.go`: `TestFallowExtendsIsDelegatedWhenTheProjectOwnsTheConfig` — non-empty `.fallowrc.json` → `Delegated` ok=true, `Apply` not called, no error
- [x] 2.2 RED `setup_test.go`: same for `lefthookExtendsStep` (non-empty `lefthook.yml`, `managerLefthook`)
- [x] 2.3 RED `setup_test.go`: file absent → `Delegated` ok=false, `Apply` writes it — no prior test existed to retarget (searched `internal/setup` and `internal/cli`), so `TestExtendsStepsWriteTheirFileWhenTheProjectHasNone` is new coverage
- [x] 2.4 GREEN `steps.go`: replace `extendsStep` with `fallowExtendsStep`/`lefthookExtendsStep` per Decision 4's state table; `Describe` computes the path via `ownedFrom`
- [x] 2.5 GREEN `setup.go`: `Plan()` lists both new steps in place of `extendsStep{}`
- [x] 2.6 GREEN `files.go`: `wireFallowExtends`/`wireLefthookExtends` drop the error-return branch
- [~] 2.7 Mutation: `go run ./tools/mutationstaged` over `steps.go`/`files.go` extends logic — no survivors. **Extends-logic survivors are killed** (`fallowExtendsStep.Satisfied`, `lefthookExtendsStep.Satisfied`, `installStep.Satisfied` — all `!p.HasSource() || …` short-circuit gaps). The staged run's overall score is still 0.60, not 0.80, because of pre-existing survivors in `internal/cli/init.go`, `internal/cli/sync.go`, and `appendHuskyGate` in `files.go` — `docs/mutation-testing.md` documents this as byte-offset union over-approximation across all staged files, not a gap in this slice's new logic. Reported to the team lead rather than absorbed into this diff; see apply-progress notes.

#### Phase 3: Rollback wording (Decision 3)
- [x] 3.1 RED `setup_test.go`: `TestApplyRollbackNamesWhatWasUndoneNotEverything` — error names what was restored, not "everything ... undone" (hedge required until `writer-undo-completeness`)
- [x] 3.2 GREEN `setup.go`: rewrite `Apply`'s non-`Join` error string per Decision 3, with the expiry comment naming `writer-undo-completeness`

#### Phase 4: Slice 1 verification
- [x] 4.1 `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...` clean

---

## Slice 2 — Command merge, architecture step, hookInstallStep, repository stop, docs

### Review Workload Forecast — Slice 2

| Field | Value |
|---|---|
| Estimated changed lines | ~270–315 |
| 400-line budget risk | Medium |
| Chained PRs recommended | Yes (this is PR2 of 2) |
| Chain strategy | stacked-to-main |
| Delivery strategy | auto-chain |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: Medium

Design flags Slice 2 as "itself near budget"; if the real diff runs materially over ~315, cut Decision 6bis (repository stop) into its own third `stacked-to-main` slice — it touches only `internal/project` and nothing else here does. Do not absorb an overrun silently; report it.

**Work unit**: PR2, based on PR1's branch, targets main once PR1 merges. Test: `go test ./...`. Runtime: `go run ./tools/mutationstaged` over `internal/setup/steps.go`, `internal/project/discover.go`, `internal/cli/sync.go`; `bash scripts/verify-gate.sh`. Rollback: revert the merge commit — `init`/`sync` return, no migration.

#### Phase 5: `hookInstallStep` fix (Decision 5; spec: step-delegation req 5)
- [x] 5.1 RED `setup_test.go`: reverse `TestGateStepIsSatisfiedWhenNoManagerAnswers` → `TestGateStepIsAnOpenDecisionWhenNoManagerAnswers`, asserting `!Satisfied` **and** `Delegated` ok=true with non-empty why; comment states the new rationale, old comment removed, not silently deleted
- [x] 5.2 GREEN `steps.go`: `Satisfied` default case returns `false`; `Delegated` per manager (lefthook/husky ok=false; none ok=true with why)
- [x] 5.3 GREEN `steps.go`: `Describe` gains the `managerNone` branch naming both managers, `gateCommand`, and the check
- [x] 5.4 Mutation: `go run ./tools/mutationstaged` over `hookInstallStep` — confirmed via Phase 10's full staged run (score 0.86, see below); no dedicated per-file run was needed since the reversed branch is inside the same staged scope

#### Phase 6: `architectureStep` (Decision 6; spec: step-delegation req "architecture step")
- [x] 6.1 RED `setup_test.go`: `TestArchitectureStepDisappearsOnceBoundariesAreDeclared` — `fallow.jsonc` contains `boundaries` → satisfied, absent from `Pending`
- [x] 6.2 RED `setup_test.go`: retarget `TestArchitecturePromptPinsFallowToRemoteLatest` at `architectureStep{}.Describe(p)`
- [x] 6.3 GREEN `steps.go`: add `architectureStep{}` — `Satisfied` via substring check on `ownedFallow` (no JSONC parser); `Delegated` always ok=true when unsatisfied. Verified `doctorConfigStep.Satisfied`: it reads only `doctor.config.json` (the `doctorConfig` constant) and never inspects a `.ts` file, so the code-not-data case is **not** delivered by this change — recorded in the docs task (9.1)
- [x] 6.4 GREEN `prompt.go`: `ArchitecturePrompt` loses its `## Left to you:` heading and opening paragraph (moved to `Delegated`'s why)
- [x] 6.5 GREEN `setup.go`: `Plan()` appends `architectureStep{}` last

#### Phase 7: Repository hard stop (Decision 6bis)
- [x] 7.1 RED `discover_test.go`: `InRepository` true on both success branches, false on the swallow branch
- [x] 7.2 GREEN `detect.go`: `Project` gains `InRepository bool`
- [x] 7.3 GREEN `discover.go`: set `InRepository=true` in the three success branches; swallow branch unchanged
- [x] 7.4 RED `internal/cli/sync_test.go`: `TestSyncStopsOutsideAGitRepository` — no `gitProject` stub → non-nil error naming the repository, directory untouched before/after
- [x] 7.5 GREEN `sync.go`: `RunSync` checks `p.InRepository` right after `Discover`, before the header, returns the Decision 6bis error verbatim

#### Phase 8: Command merge
- [x] 8.1 Add `gitProject(t, root)` fixture helper to `internal/cli/sync_test.go` via `project.SetGitOutputForTest`
- [x] 8.2 RED `TestSyncAppliesAndDelegatesInOneRun` — `Applying:` then `## Left to you:` regions, in order
- [x] 8.3 RED `TestSyncCompletesWhenTheProjectAlreadyConfiguredFallow` — non-empty `.fallowrc.json`, no error/rollback, `extends` line delegated
- [x] 8.4 RED `TestSyncNeverAppliesADelegatedStep` — `agentSkillStep.Apply`'s error never surfaces
- [x] 8.5 RED `TestSyncStopsBeforeWritingWithoutAJSProject` — `noSourceMessage`, `err == nil`
- [x] 8.6 RED `TestSyncRollbackNamesWhatItRestoredAndNothingMore` — pins Decision 3 through `RunSync`
- [x] 8.7 Delete `TestSyncWritesNothing` — categorically false once `sync` applies (`tree()` assertion migrated into 7.4/`TestSyncStopsOutsideAGitRepository`)
- [x] 8.8 Rewrite `TestSyncSpeaksTheProjectsOwnPackageManager` — assert on `runner.SetForTest`-captured command
- [x] 8.9 Extend `TestSyncSaysWhyTheDelegatedStepIsDelegated` to the extends and gate delegations
- [x] 8.10 Extend `TestSyncReachesATerminalAnswer` — fixture gains `boundaries`, asserts architecture prompt absent (§15)
- [x] 8.11 GREEN `sync.go`: `RunSync` absorbs `RunInit`'s body per Decision 1's report shape
- [x] 8.12 Delete `internal/cli/init.go` (68 lines)
- [x] 8.13 GREEN `app.go`: remove `"init"` case; `UnknownCommandError` message drops `init`
- [x] 8.14 GREEN `help.go`: collapse the two `init`/`sync` COMMANDS entries into one

#### Phase 9: Documentation
- [x] 9.1 `docs/flujo-implementado.md`: corrected the rows in proposal's "What this change makes false" table (Fusión Estado qualified to the `extends` case only; Fusión "Dónde se usa" states the `doctorConfigStep`/`.ts` result from 6.3 — not delivered; Conducción Estado states the actual Decision 1 report shape; Intención Estado no longer claims to be "the only real prompt"; header line 11 "Cinco comandos" → "Cuatro comandos")
- [x] 9.2 `docs/flujo-implementado.md`: added the line separating the two Figure 1 left-hand stop states (no repository vs. no JS project, with their different exit codes) per Decision 6bis
- [x] 9.3 Republish `docs/flujo-implementado.md` to `https://claude.ai/code/artifact/6033beba-ca70-4163-92fd-97de7ed8663e` — **blocked**: no tool in this executor's surface can publish to a claude.ai artifact URL; the markdown file itself is fully updated and ready, but the republish step needs to be done by whoever has that capability, passing the exact URL above — republished by the orchestrator to the same URL; .md and artifact are in sync
- [x] 9.4 `docs/learning-log.md`: appended one dated line — `Delegated` moved from a compile-time type assertion to a per-project `Step` method, closing both the rollback bug and the unenforced Figure 1 repository stop in one contract change

#### Phase 10: Slice 2 verification
- [x] 10.1 `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...` clean
- [x] 10.2 `go run ./tools/mutationstaged` (dry, then real) — staged run over all 9 touched production files: 127 mutants, 109 killed, score 0.86 (min 0.80), no per-file survivor concentrated in the reversed/new branches beyond the pre-existing documented over-approximation
- [x] 10.3 `bash scripts/verify-gate.sh` — confirms: "the hook refused a broken file, as it must"
