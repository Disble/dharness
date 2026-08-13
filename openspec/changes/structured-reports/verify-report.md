```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:63cc10bf9c042c9403b5cc29d3dc0bd33aab31b107b089658bda4bc44952cf00
verdict: fail
blockers: 1
critical_findings: 3
requirements: 21/22
scenarios: 41/42
test_command: go test ./...
test_exit_code: 0
test_output_hash: sha256:11202d3a6135482d8689facc50b9c07bf0e1d3322dccaa761aca9aadedb07add
build_command: go build ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: structured-reports
**Version**: spec.md, amended (six-value Status; split collision-identity requirement; measured six-line golden figure)
**Mode**: Strict TDD

Verified at commit 8eb0e2a on feat/structured-reports (HEAD; working tree clean, nothing staged/unstaged). All 76 tasks in tasks.md are checked, none pending, so full spec/design/task verification applies.

### Completeness
| Metric | Value |
|--------|-------|
| Tasks total | 76 |
| Tasks complete | 76 |
| Tasks incomplete | 0 |

### Build & Tests Execution
**Build**: Passed
```text
go build ./...   -> exit 0, empty output
```

**Tests**: all packages passed
```text
go test ./...    -> exit 0
ok  internal/app, internal/cli, internal/jsconfig, internal/preset,
    internal/project, internal/report, internal/runner, internal/setup,
    internal/testsupport/mutation, internal/tool, tools/mutationstaged
```

**go vet ./...**: clean (exit 0)
**gofmt -l .**: clean, no files listed (exit 0)

**Golden fixtures** (git diff main..HEAD -- internal/setup/testdata/golden/): exactly 6 lines changed, 6 files, all at line 26 -- expo.txt, generic-conventional.txt, generic-split.txt, nextjs.txt, wails-nextjs.txt, wails.txt, each replacing "resolve the two architectures this project declares" with "resolve the keys this project and dharness both declare". Matches design.md Decision 6 and spec.md's amended non-requirement exactly. TestGenericMechanismHasNoUpdatePath passes in the full suite run.

**boundariesFallbackDescribe/boundariesFallbackWhy** (internal/setup/steps.go): byte-compared main vs HEAD -- identical text (only line numbers shifted, 506->727 and 516->737). The "six lines is the complete golden impact" claim holds.

**Mutation gate -- independently re-measured, not read from prose.** apply-progress.md's own record (slice 1: 0.98; slice 4 report group: 1.00; slice 4 setup/steps group: 0.97; slice 3: 1.00) predates the current HEAD. The most recent commit, 8eb0e2a ("fix(report): narrow a collision to the keys that carry a decision"), touches internal/report/human.go/human_test.go and has no mutation record anywhere in tasks.md, apply-progress.md, or Engram. I re-measured it directly: git reset --soft HEAD~1 (restaging exactly 8eb0e2a's diff), ran go run ./tools/mutationstaged, then git reset --soft 8eb0e2a to restore HEAD exactly (verified clean afterward, no data lost -- standard --soft semantics, working tree untouched throughout). Result:

```
- Total:       40
- Killed:      32
- Survived:     8
Score:     0.80 (minimum: 0.80)
```

This exactly matches the count the team lead's brief stated (0.80, 8 survivors) -- confirmed independently rather than taken on trust. Two of the eight survivors are inside narrowToDifferences itself (the hidden == 0 boundary and its 0-vs--1 return sentinel); none of the 8 are documented as proven-equivalent the way slice 1's two remaining survivors were. This is the weakest score in the whole change, sitting exactly on the floor with no margin, over new logic that (see CRITICAL-1 below) is also unspecced.

### Spec Compliance Matrix
21 of 22 requirements verified compliant against a real build-and-run, not just passing tests (per the team lead's explicit standard). One requirement fails.

| Requirement | Scenario | Test / Evidence | Result |
|---|---|---|---|
| Every step carries exactly one status | 11-step plan -> 11 results | TestRunReturnsAStepResultForEveryPlanStep; live run showed 11/11 rows across Applied/Left to you/Already in place | COMPLIANT |
| Satisfied step carries evidence | fallow-extends etc. | TestSatisfiedStepCarriesEvidenceNotBareStatus; live: step 8 -> ".mcp.json declares fallow" | COMPLIANT |
| Outcome printed after Apply returns | timed step line | Live: step 1 glyph+duration together (1.98s) | COMPLIANT |
| Summary before per-step detail | both views | TestWriteHumanRendersEverySummaryFirst; live: summary line precedes all blocks | COMPLIANT |
| JSON summary counts match human | both views | TestSyncFormatJSONAndHumanAgreeOnSummaryCounts; live: JSON summary == human summary line | COMPLIANT |
| --format json = same one analysis | parseable JSON only on stdout | TestSyncFormatJSONEmitsParseableJSONAndNothingElse; live: json.Valid clean, no banner text | COMPLIANT |
| JSON status derives from exit code only | delegated-work-still-0; failure case | sync.go:76 Exit: runner.ExitCode(runErr) -- assigned, not computed from Summary.Failed; TestSyncExitFieldMatchesRunnerExitCode | COMPLIANT |
| Closing block: tally + exit + next | delegated work names next | Live: closing tally line + next pointer present | COMPLIANT |
| Failure variant retracts + not-reached | rollback | TestFailureRetractsEarlierStepsAndMarksRemainingNotReached; retractAndReport (setup.go:335) -- no false "no earlier step" sentence on this path | COMPLIANT |
| Scoped-mutation evidence survives | present-when-non-nil | TestScopedMutationEvidenceSurvivesBothViewsRegardless; sync.go:78-80 ungated on left==0 | COMPLIANT |
| Residue notes listed in full, no --show hint | -- | TestWriteHumanResidueNoteListsEveryEntryAndNoFlagRef; rg for --show finds no production occurrence | COMPLIANT (see WARNING-2 on defect 13's ordering aspect) |
| No report persisted to file | --format json writes only stdout | TestNoReportFileIsPersisted; live: file search in throwaway repo found nothing | COMPLIANT |
| Step.Apply sink, never real stdout | install/hook steps | TestApplyWritesOnlyToTheGivenSink; rg for os.Stdout/os.Stderr in steps.go -> nothing inside an Apply body | COMPLIANT |
| Structured Facts (Installed) | install outcome | Live JSON: "installed": ["dharness-eslint-plugin@0.3.0"] | COMPLIANT |
| File attribution partitioned per step | two steps, independent sets | TestPerStepFileAttributionIsPartitioned; live: step file lists disjoint | COMPLIANT |
| Created/modified/unchanged classification | byte-identical rewrite | TestChangedClassifiesCreatedModifiedUnchanged + mutation-guard test | COMPLIANT |
| Collision computed once, rendered from one value | key renders once, both views | TestWriteHumanCollisionKeyRendersExactlyOnce, TestCollisionsComputesEachKeyOnce, TestDelegatedCollisionMatchesTheComputedReportValue; live: duplicates appears once | COMPLIANT |
| ID() makes no architecture claim; entry names actual key(s) | -- | TestBoundariesOwnerStepIDMakesNoArchitectureClaim; live: new ID string + key named in collision block | COMPLIANT |
| effective local-binary-only, no remote fallback | zero-binary / exit-3 short-circuit | TestResolvedConfigShortCircuitsOnNoLocalBinary, ...OnExit3; rg for bunx/npx/pnpm-dlx/yarn-dlx in internal/setup -> nothing; check.go's remoteStage untouched | COMPLIANT |
| effective absent, never fabricated | unparsable / missing key | TestResolvedConfigAbsenceHasOneShape | COMPLIANT |
| A colliding value is reported whole or not at all | multi-line value shown whole when measured | TestWriteHumanCollisionValueWrapsWithinReportWidth was edited in commit 8eb0e2a to leave Ours.Value nil specifically so narrowToDifferences cannot engage. Live binary run: the project's resolved value was narrowed from 16 keys to 3, with 13 silently dropped. | FAILING -- see CRITICAL-1 |
| Delegated collision hands back structured Collision, not prose | -- | TestDelegatedCollisionMatchesTheComputedReportValue | COMPLIANT |
| Status line retracted by name on rollback | -- | TestFailureVariantRendersEveryNonTerminalStatus; writeFailureTally names retracted steps | COMPLIANT |

**Compliance summary**: 21/22 requirements compliant, 41/42 scenarios compliant (both scenarios under the failing requirement checked; one fails).

### The thirteen defects -- verified individually against the live binary
Reproduced per the team lead's own recipe: throwaway git repo, tracked frontend/package.json + frontend/package-lock.json, .gitignore with node_modules/, frontend/.fallowrc.json = { "duplicates": { "minOccurrences": 2, "mode": "weak", "threshold": 5 } }, npm i -D fallow (real fallow 3.15.0 installed locally). Built dharness.exe from HEAD and ran sync / sync --format json twice (first apply pass, then a satisfied-state pass).

| # | Defect | Status | Evidence |
|---|---|---|---|
| 1 | Satisfied steps invisible | Closed | Live: "Already in place (5)" block lists steps 2,5,6,7,8 with evidence |
| 2 | Applying: prints intention before outcome | Closed | Live: step 1 shows glyph+duration together, printed after Apply returns |
| 3 | Rollback contradicts itself | Closed on the sync path | retractAndReport (setup.go:335) -- no false claim; TestFailureVariantRendersEveryNonTerminalStatus passes. Note: the old, pre-existing applySteps/Apply helper (setup.go:393-434) still literally contains the sentence "No earlier step is reported as having succeeded" at line 423 -- but per design.md Decision 8 this old path is deliberately kept, unmodified, with its only caller being renderGolden in golden_test.go. Confirmed via grep: internal/cli/sync.go calls only setup.Run, never setup.Apply. Not user-reachable; not a regression. |
| 4 | Writer records touched paths, report names none | Closed | Live: file change lines listed under each applied step |
| 5 | Sink leak to real os.Stdout | Closed | rg for os.Stdout/os.Stderr in internal/setup/steps.go -> no match inside an Apply body |
| 6 | Collision key rendered twice by two renderers | Closed | Collisions/renderCollisions consolidation; live: duplicates appears once |
| 7 | "Two architectures" false claim | Closed | ID() now "resolve the keys this project and dharness both declare"; golden fixtures updated accordingly |
| 8 | Project value truncated to a fragment | Partially reopened in a new form | The naive single-line-scan fragment is gone (value now sourced from fallow config --format json). But as of commit 8eb0e2a, the shown value is a different kind of incomplete: silently-narrowed to differing keys only -- see CRITICAL-1. The original defect (an accidental truncation) is fixed; a new, deliberate-but-unspecced truncation replaces it. |
| 9 | Paths unresolvable (no directory shown) | Closed | Live: both sides show directory-qualified paths with line numbers |
| 10 | No final tally / exit code / next command | Closed | Live: closing tally with exit code + next pointer present |
| 11 | No machine-readable output | Closed | Live: sync --format json parses (json.Valid), agrees with human view |
| 12 | Inconsistent heading levels, Applied has none | Closed | Live: uniform "-- Applied (N) --" style headings on every block |
| 13 | Residue block ends on justification, not action | Not clearly closed | See WARNING-2 below |

### Issues Found

**CRITICAL**

1. narrowToDifferences (commit 8eb0e2a, internal/report/human.go) violates spec.md's own MUST, is unauthorized by design.md, untracked by tasks.md, and undocumented in apply-progress.md.
   spec.md, capability config-collision: "a colliding value is reported whole or not at all -- never a truncated fragment... The dharness-owned value for a colliding key MUST be reported whole... Where the resolved value cannot be measured, the report states that the value could not be shown in full -- it MUST NOT fall back to the truncated textual fragment." Scenario: "GIVEN a project's fallow config declaring a multi-line object value for a colliding key, and effective successfully measured, WHEN the collision is rendered in either view, THEN the full value object appears."
   Live-observed in the throwaway repo: dharness's own 3-key value rendered whole (all 3 keys differed from the project's, so nothing was hidden on that side by luck), but the project's fully-measured, successfully-resolved 16-key value was narrowed to 3 keys with a "13 identical key(s) hidden" note -- the full value object did not appear, contradicting the scenario directly.
   Design.md documents nine decisions; none authorizes narrowing a measured, successfully-resolved value. tasks.md's last entry is task 4.22's follow-up (defects 1-2, wrapping + labeling); there is no task for "narrow to differing keys." apply-progress.md's slice 4 section -- the most recent artifact on disk -- predates this commit entirely (last touched at commit 149e981; the intervening e857c5e/8f113ca/87bdbf6 are reflected, 8eb0e2a is not).
   The pre-existing pinned test for this exact scenario, TestWriteHumanCollisionValueWrapsWithinReportWidth, was edited in this same commit to set Ours.Value to nil -- specifically so narrowing cannot engage -- rather than being reconciled with the new behavior. The suite is green because the scenario that would catch this was edited out of the way, not because the requirement holds.
   This needs a decision from the team lead/user: either spec.md is amended (via a design decision, the same process every other deviation in this change went through) to permit narrowing with disclosed rationale, or the narrowing is reverted/gated to preserve the MUST.

2. Mutation score for the same commit is exactly the floor (0.80), with 8 unchased survivors, and no record of the run anywhere.
   Independently re-measured (see Build & Tests section above): 32/40 killed, 8 survived, score 0.80/0.80 -- no margin. Two survivors are inside narrowToDifferences itself. Unlike every other survivor in this change (slice 1's 2 remaining survivors, slice 4's 1 in installedWithVersions), none of these 8 are documented as proven-equivalent mutants with a written reason, per the mutation-tdd skill's own rule ("disable only a proven equivalent mutant... with a written reason"). This is the weakest-tested code in the entire change, and it is also the code implementing CRITICAL-1's unspecced behavior.

3. Task-honesty gap: real, committed production code with no SDD paper trail. Commit 8eb0e2a changed internal/report/human.go (+126/-13) and its test file (+274 lines) -- a real behavior change and a real test-narrowing edit -- with no corresponding task added to tasks.md, no design decision, and no apply-progress entry. tasks.md's own task 4.22 follow-up explicitly disclosed the delegated-block width defect (71-73 runes) as "flagged for a follow-up task" -- that follow-up task was never created; the fix simply landed in an undocumented commit alongside an unrelated, larger, unspecced feature (the narrowing).

**WARNING**

1. The collision block's hidden-key note is factually inaccurate. narrowToDifferences increments the same hidden counter for two different reasons -- keys with an identical value on both sides, and keys present only in the resolved/theirs side (fallow's own defaults dharness never declared) -- then always renders "note: N identical key(s) hidden". Live-observed: all 13 hidden keys in the throwaway repo were fallow defaults absent from dharness's declaration entirely; zero were actually identical-value matches, yet the note read "13 identical key(s) hidden". This directly contradicts design.md's own stated principle: "There is no field in this model that a reader may treat as an opaque token exempt from being true" and the repository's recurring-failure theme (docs/learning-log.md) of prose asserting what the code does not do.

2. Defect 13's "ends on action, not justification" aspect is not clearly closed. writeNotesBlock renders Kind, then Entries, then Reason last. For the residue note, Reason (internal/setup/steps.go:875) is pure justification prose ending "...(measured against react-doctor 0.5.7)." with no closing action statement. The listed-in-full and no-flag-reference halves of defect 13 are fixed and tested; the ordering half (justification-last vs. action-last) has neither implementation nor test, and spec.md itself does not literally mandate an order -- so this is not a spec violation, but the defect list is not fully closed on this specific point.

3. One of the 8 unchased mutation survivors is a real off-by-one risk, not just a decorative literal: wrap(step.Why, wrapWidth-3, 3) mutated to wrapWidth-4 and survived, because the covering test only asserts "no line exceeds 70 runes," not that the bound is exactly tight. A wrong-by-one width constant here would go undetected.

**SUGGESTION**

1. firstDelegatedID (closing block's next pointer) picks the first delegated step in Plan() order, not preferentially the collision. Live-observed with 5 delegated steps: next pointed to "point .fallowrc.json at the file dharness owns" (step 3) rather than the collision (step 4), even though the collision is the richer, most-addressable item with its own stable Collision.ID. Not a spec violation (the requirement only asks for "a delegated step's identifier"), but worth a design note since the approved target-report.md example never had more than one delegated step to disambiguate this.

2. The hard-split rendering of a long compact-JSON value still breaks mid-token for a human reader (e.g. a word split across two lines) even after narrowing, since a narrowed-but-still-long value (51 runes against ~37 available once the effective mark reserves space) still exceeds the width. This is a disclosed, accepted tradeoff from task 4.22's follow-up, not new -- noted for completeness only.

### Design Coherence
| Decision | Followed? | Notes |
|---|---|---|
| 1 -- model/Status/ExitCode move | Yes | runner.ExitCode, app.ExitCode forwarder, six-value Status verbatim |
| 2 -- Facts/sink signature | Yes | All 11 Apply implementations widened; sink confirmed live |
| 3 -- Writer.Changed | Yes | Byte comparison, root-relative paths, directory included |
| 4 -- collision computed once | Yes | Collisions/renderCollisions; live: one render |
| 5 -- fallow measurement, two-outcome | Yes | Local-only, exit-3 short-circuit confirmed by test + code |
| 6 -- ID() heading, six golden lines | Yes | Exactly 6 lines, 6 files, line 26; fallback constants byte-identical |
| 7 -- alignment/wrap/glyphs, 70-column | Yes, with one gap | Live: every line <= 70 runes except the header's absolute-path line (disclosed exception); one unchased mutation survivor on the delegated-block width bound (WARNING-3) |
| 8 -- setup.Run/RunSync split | Yes | Confirmed via sync.go reading |
| 9 -- four-slice plan, ~920-1,300 lines | Not re-verified | Out of scope for a line-count audit at this phase; not required for correctness |
| Undocumented: narrowToDifferences | No decision covers it | See CRITICAL-1 |

### Verdict
**FAIL**
One CRITICAL spec violation (an unspecced, undocumented narrowing of the collision block's "reported whole" guarantee, landed in the branch's most recent commit with no design/task/apply-progress trail and a mutation score sitting exactly on the floor) blocks archive. Everything else in the change -- all 76 tasks, all four slices' original scope, twelve of the thirteen defects fully closed, the thirteenth partially reopened in a new shape by the same commit -- is genuinely solid, build/vet/test/gofmt clean, and independently confirmed against a real built binary rather than the test suite alone.

### Recommendation
Route back to sdd-apply (or a direct fix) to resolve CRITICAL-1 through one of: (a) revert narrowToDifferences, keeping writeDeclaredSide's prior whole-value + wrap/hard-split behavior from e857c5e, or (b) get the narrowing explicitly approved via a design decision that amends spec.md's "reported whole" MUST with disclosed rationale -- the same process every other deviation in this change went through. Either path should also fix WARNING-1's mislabeled note text, chase the 8 unchased survivors (or document any that are genuinely equivalent, with proof, per the mutation-tdd rule), and add the missing task/apply-progress entries. Re-verify afterward before archiving.
