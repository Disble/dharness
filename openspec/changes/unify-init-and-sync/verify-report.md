# Verification Report: unify-init-and-sync

**Mode**: full (proposal, spec, design, tasks all present)
**Verdict**: PASS WITH WARNINGS

## Completeness

41 of 42 tasks are checked [x]. Task 2.7 is [~], pre-accepted by the orchestrator for its named extends-logic scope (survivors outside that scope are documented, not silently dropped). All other checked tasks were inspected against the actual code and are true, not just ticked -- see Correctness below.

## Commands run (literal output)

### go build ./...
```
EXIT:0
```
(no output -- clean)

### go vet ./...
```
EXIT:0
```
(no output -- clean)

### gofmt -l .
```
EXIT:0
```
(no output -- clean, no unformatted files)

### go test ./...
```
?   	github.com/Disble/dharness/cmd/dharness	[no test files]
ok  	github.com/Disble/dharness/internal/app	(cached)
ok  	github.com/Disble/dharness/internal/cli	(cached)
ok  	github.com/Disble/dharness/internal/project	(cached)
ok  	github.com/Disble/dharness/internal/runner	(cached)
ok  	github.com/Disble/dharness/internal/setup	(cached)
ok  	github.com/Disble/dharness/internal/testsupport/mutation	(cached)
ok  	github.com/Disble/dharness/internal/tool	(cached)
ok  	github.com/Disble/dharness/tools/mutationstaged	(cached)
EXIT:0
```

### bash scripts/verify-gate.sh
```
verify-gate: the hook refused a broken file, as it must.
EXIT:0
```

Mutation testing was not re-run by this verification (out of budget for this pass; apply-progress reports Slice 1 at 0.60 with all extends-logic survivors killed, and Slice 2 at 0.86, both cross-checked against docs/mutation-testing.md's documented byte-offset over-approximation for pre-existing untested code -- not independently re-executed here).

## Spec compliance matrix

Capability project-sync:

| Requirement | Scenario | Test | Status |
|---|---|---|---|
| Command is named sync only | init is unknown | internal/app/app_test.go (TestRunArgsInitIsUnknown) | PASS |
| | error does not name init | internal/app/app_test.go | PASS |
| | sync performs what init used to | TestSyncAppliesAndDelegatesInOneRun | PASS |
| Prepare executes nothing, cannot fail | plan derivation writes nothing | Pending/Satisfied/Delegated are read-only -- source inspection confirms no write/exec calls | PASS |
| | Delegated(p) never calls Apply | source inspection, applySteps (setup.go:83-114) | PASS |
| Exactly two stopping conditions | no repo / no JS project stops before writing | TestSyncStopsOutsideAGitRepository, TestSyncStopsBeforeWritingWithoutAJSProject | PASS |
| | Apply failure leads to full rollback, no partial success | TestApplyRollbackNamesWhatWasUndoneNotEverything, TestSyncRollbackNamesWhatItRestoredAndNothingMore | PASS |
| | every other situation continues | TestSyncCompletesWhenTheProjectAlreadyConfiguredFallow, TestGateStepIsAnOpenDecisionWhenNoManagerAnswers | PASS |
| Re-running repeats nothing, state derived | satisfied step disappears | TestArchitectureStepDisappearsOnceBoundariesAreDeclared | PASS |
| | undoing by hand reappears | no dedicated test, but follows structurally: Satisfied(p) is the sole read on every call, no persisted state exists in the package | WARNING (no dedicated test) |
| | no on-disk record of run history | source inspection -- setup package has no state read besides project.Project fields | PASS |
| Rollback wording does not overclaim | rollback avoids stronger claim | TestApplyRollbackNamesWhatWasUndoneNotEverything | PASS |

Capability step-delegation:

| Requirement | Scenario | Test | Status |
|---|---|---|---|
| Delegated is a Step method, not a type assertion | on the Step interface | source inspection, setup.go:23-42 -- no Delegated interface remains | PASS |
| | Apply never called when Delegated true | TestApplySkipsEveryDelegatedStep | PASS |
| Project file with content is delegated, not an error | .fallowrc.json exists with content | TestFallowExtendsIsDelegatedWhenTheProjectOwnsTheConfig | PASS |
| | lefthook.yml exists with content | lefthook equivalent test in setup_test.go (task 2.2) | PASS |
| | file absent, writes it | TestExtendsStepsWriteTheirFileWhenTheProjectHasNone | PASS |
| No hook manager is an open decision, not silent success | no manager responds | TestGateStepIsAnOpenDecisionWhenNoManagerAnswers | PASS |
| | old test reversed with new rationale, old comment removed | confirmed -- TestGateStepIsSatisfiedWhenNoManagerAnswers no longer exists; grep for its defending language returns zero hits in internal/setup | PASS |
| Architecture step satisfied by boundaries, disappears once satisfied (Section 15) | boundaries not yet declared | architectureStep.Delegated always true when unsatisfied -- inspected, no dedicated negative test but trivial | PASS |
| | boundaries already declared | TestArchitectureStepDisappearsOnceBoundariesAreDeclared | PASS |
| | re-running after agent declares boundaries | same test covers the disappearance directly (state re-derived per call, no persisted state to reset) | PASS |

## Correctness -- the 8 specifically-flagged items

1. Prepare executes nothing. Confirmed by source inspection of every Satisfied/Delegated method in internal/setup/steps.go: none writes a file, calls runner.Run, or can return an error from Delegated. applySteps (setup.go:87-114) is the only place Apply is invoked, gated by Delegated(p)'s ok return. PASS.
2. Only two stopping conditions. internal/cli/sync.go: !p.InRepository returns a fmt.Errorf (exit 1 via app.ExitCode's default branch), !p.HasSource() prints noSourceMessage and returns nil (exit 0). No other early return exists in RunSync outside setup.Apply's own failure path, which is the second named stopping condition, not a third one. Confirmed both exit codes: TestSyncStopsOutsideAGitRepository asserts a non-nil error (exit 1 per app.ExitCode); TestSyncStopsBeforeWritingWithoutAJSProject asserts err == nil (exit 0). PASS.
3. extendsStep's replacement no longer errors on existing content, rollback not triggered. TestFallowExtendsIsDelegatedWhenTheProjectOwnsTheConfig writes a non-empty .fallowrc.json, asserts Delegated returns ok=true, then runs applySteps and asserts the original file bytes are untouched and no error returned. wireFallowExtends/wireLefthookExtends (files.go) no longer have an error-return branch for this case, per design Decision 4. PASS -- the bug is closed.
4. Architecture step disappears when satisfied (Section 15). architectureStep{} is appended in Plan() (setup.go:58); Satisfied is a substring check on .dharness/fallow.jsonc for "boundaries" (steps.go:432-435); RunSync's report loop iterates setup.Pending(p) a second time after applying, which excludes any satisfied step by construction (no unconditional print anywhere in sync.go). TestArchitectureStepDisappearsOnceBoundariesAreDeclared confirms. PASS.
5. hookInstallStep's reversed test carries its new rationale, old comment gone. TestGateStepIsAnOpenDecisionWhenNoManagerAnswers asserts both !Satisfied and Delegated(ok=true, why!=""). The old test name and its "must not block everything else" defending comment are gone entirely (grep across internal/setup returns zero hits); the new rationale lives in steps.go:345-347 and steps.go:365-367. PASS.
6. Rollback wording does not claim everything was undone. setup.go's non-Join error string reads: "Every file this run wrote was put back as it was found; directories it created are not removed." -- with an inline comment naming writer-undo-completeness as the expiry. TestApplyRollbackNamesWhatWasUndoneNotEverything asserts the error does NOT contain "everything this run wrote was undone" or "the repository was fully restored", and DOES contain "put back". The hedge is intact, not strengthened. PASS.
7. internal/cli/init.go is gone, no init references survive. Confirmed: file does not exist (ls errors "No such file or directory"). app.go's dispatcher has no "init" case; UnknownCommandError.Error() reads "expected sync, check, mutate or version" -- no init substring. help.go's COMMANDS section lists only sync, check, mutate, version. PASS.
8. Documentation matches reality. docs/flujo-implementado.md line 11: "Cuatro comandos sobre tres librerias" (confirmed -- not "Cinco"). The Fusion row (line 363) states the doctor.config.ts case explicitly is NOT resolved: "El caso doctor.config.ts -- codigo, no datos -- no esta resuelto: doctorConfigStep solo lee y fusiona doctor.config.json; un .ts no se detecta ni se describe." This matches steps.go's doctorConfigStep.Apply, which only ever opens doctorConfig (the .json constant) -- never a .ts file. PASS -- the doc does not overclaim Fusion completeness.

## Design coherence

All six architecture decisions (1, 2, 3, 4, 5, 6, 6bis) in design.md are reflected in the code as described:
- Decision 1 (report shape) -- RunSync's three regions (header, Applying:, ## Left to you: blocks) match exactly, including the removed "Run `dharness init`..." trailing pointer.
- Decision 2 (no Kind field/enum) -- confirmed absent from the Step interface and all implementations.
- Decision 3 (rollback wording) -- see item 6 above.
- Decision 4 (extendsStep split) -- confirmed: fallowExtendsStep and lefthookExtendsStep exist as separate types with the exact per-state table from design.md.
- Decision 5 (hookInstallStep) -- see item 5 above; Describe's managerNone branch is present (steps.go:358-362).
- Decision 6 (architectureStep) -- see item 4 above.
- Decision 6bis (repository hard stop) -- Project.InRepository field exists (detect.go), set by inRepository() helper on Discover's three success branches (discover.go:79-90), left at zero value on the swallow branch (discover.go:66), checked in RunSync immediately after Discover (sync.go:36-42) with the exact error text from the design doc.

No design deviations found.

## Issues

### CRITICAL
None.

### WARNING
1. Task 9.3 (artifact republish) cannot be independently verified. tasks.md marks it [x] with the note "republished by the orchestrator to the same URL; .md and artifact are in sync." The Engram apply-progress artifact (id 7986) explicitly states this task was blocked for the apply executor ("no tool available to this executor can publish to a claude.ai artifact... left [ ] with the reason recorded inline") and that a later actor (the orchestrator) was expected to complete it. This verifier has no tool capable of checking that the claude.ai artifact at the stated URL actually reflects the current docs/flujo-implementado.md content. Per the team lead's explicit instruction, task 9.3 is treated as accepted/done here -- but it remains factually unverifiable from this session and should be spot-checked by a human with access to that URL before archive.
2. "Undoing a step by hand makes it reappear" has no dedicated test. The spec's project-sync capability includes this scenario explicitly. It is true by construction (no persisted state anywhere in setup -- confirmed by source inspection), but no test exercises the specific "delete the file, re-run, step reappears" flow end-to-end. Not CRITICAL because the mechanism (pure re-derivation on every call) is structurally guaranteed and covered indirectly by TestArchitectureStepDisappearsOnceBoundariesAreDeclared's inverse case, but the spec's explicit GWT scenario has no 1:1 covering test.
3. Line budget overran materially and was reported, not silently absorbed. Slice 2 measured 613 changed lines against a 400-line hard budget and a ~270-315 slice forecast (apply-progress artifact 7986). This is disclosed honestly in both the Engram artifact and openspec/apply-progress.md, with a stated reason (test fixtures the command merge required, RunInit having zero prior test coverage) and an explicit statement that splitting further would be organizational, not a scope reduction. Flagging per the review workload guard since this is a real budget breach, even though it was handled transparently and per the team lead's "report it, don't split unilaterally" instruction.
4. Mutation testing not independently re-run in this verification pass. Reported scores (Slice 1: 0.60 with all extends-logic survivors killed; Slice 2: 0.86) come from the apply-progress artifact, not from a fresh run in this session. Task instructions permitted skipping mutation if budget is tight; this verifier prioritized the build/vet/gofmt/test/gate suite and the spec/task correctness audit instead.

### SUGGESTION
1. Task 2.7's [~] marker and its note are internally consistent with the apply-progress detail (extends-logic survivors killed, remainder is documented cross-file over-approximation per docs/mutation-testing.md); no action needed, noted only for completeness of the audit trail.

## Design principles cited

- Section 15 (a satisfied step disappears from the report) -- architecture step compliance confirmed above.
- Section 17 (verdict comes from exit codes/JSON, never prose) -- spec's own "Notes on testability" section explicitly requires tests to assert on bool/error/file-existence, not report prose, except the rollback-wording requirement (which is specifically about text). Confirmed the test suite follows this: TestApplyRollbackNamesWhatWasUndoneNotEverything is the one prose-assertion test, and it is the one requirement that is specifically about wording.
- Section 20 (a block is only the irrecoverable) -- Decision 3/4 changes confirmed: extendsStep's content-exists case no longer triggers Writer.Undo.
- Section 21 (the client is the agent) -- Delegated's why + Describe(p) confirmed as the prompt shape for all three delegated-step kinds (Fusion/Conduccion/Intencion) with no Kind type introduced.

## Final verdict

PASS WITH WARNINGS. All CRITICAL checks pass: build, vet, gofmt, tests, and the gate script all succeed; the eight specifically-flagged risk items are all verified fixed in the code with passing tests; docs/flujo-implementado.md accurately reflects what was and was not delivered. The four WARNING items are disclosure/completeness gaps already surfaced by the apply team, not defects hidden from the team lead -- most notably the unverifiable artifact republish (item 1) and the material line-budget overrun (item 3), both of which were reported honestly rather than concealed.
