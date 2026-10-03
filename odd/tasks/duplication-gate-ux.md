# Duplication gate UX

## Objective
Make duplication failures explainable for the next release without changing fallow's verdict or project policy.

## Problem and rationale
A dhub investigation required cross-agent research to distinguish a whole-tree duplication ceiling, structural matching, reported-group filtering, effective configuration, and execution errors. dharness should add minimal context and native read-only diagnostic pointers, not interpret findings.

## Scope
- `check`: whole-tree absolute ceiling context on duplication failures, native read-only configuration/report pointers, and a verified distinction between fallow runtime/configuration failure and findings.
- Generated fallow configuration: refine the existing duplication rationale beside the values.
- Tests and append-only learning evidence alongside the behavior.

## Constraints and non-goals
- Worktree: `D:/dev/disble/dharness-worktrees/duplication-gate-ux`, branch `feat/duplication-gate-ux`, base `main` at `f2a97b6`.
- Leave the main workspace and its other agent entirely alone; no further coordination with that agent.
- User explicitly requests NO commits; no commit, push, PR, merge, or release. Temporary staging solely for required mutation checks is confined to this isolated worktree and must be restored afterward. Work-unit commit closure remains pending until separately authorized.
- No changes to sync prompts, collision handling, mode, minOccurrences, threshold, baselines, exclusions, or automatic ignoredClones.
- Preserve upstream output and exit code, existing execution ordering, no extra process runs in the gate, and manager-aware invocation ownership in `internal/tool`.
- Do not claim the actual ceiling always comes from the preset: project overrides can apply.
- Do not promote fallow 3.30.0's observed minOccurrences behavior to a universal guarantee.
- No new dependencies; Windows remains first-class. Never run dharness against either source checkout.
- Design anchors: docs/design-principles.md §§01,03,05,09,11,16,17.

## Tasks
- [x] UX-01: Add bounded check failure context and native diagnostic pointers, plus validated fallow error-vs-findings classification. Use test-first behavior scenarios; preserve commands, errors, verdicts, and successful output.
- [x] UX-02: Refine existing generated duplication rationale without moving/changing values or adding sync model instructions. Verify generated-output contracts and regenerate affected fixture expectations through the generator where possible.
- [x] UX-03 (done, see Verification evidence below): Independently verify tests/build/vet/formatting and run the built binary in scratch Git projects on Windows to inspect actual failure/configuration output and unchanged exit codes. Perform proportionate mutation checks and record all unavailable checks.

## Acceptance and checks
- A duplication failure identifies whole-tree scope and absolute ceiling without inferring provenance/value or labeling false positives.
- Native diagnostic commands use the project's detected executor; --help is retained where useful; no diagnostic subprocess executes automatically.
- Known fallow execution/configuration error is not mislabeled as measured duplication; unknown errors stay honest and original errors propagate.
- Successful gates and unrelated stage failures receive no duplication-specific explanation.
- Existing raw upstream output, stage order, skipped-stage messages, and invocation arguments remain intact.
- Generated comments explain semantic recall/precision and the limited meaning of reported-group filtering; numeric values remain semantic/3/3.
- RED and GREEN are observed for applicable deterministic behavior tests. Passive comments do not need artificial RED; output-contract checks still run.
- Focused tests, `go test ./...`, `go vet ./...`, `go build ./...`, formatting, mutation evidence, and scratch-binary inspection are recorded separately.
- Systematic `ditto staged` requires temporary staging of owned changes in this isolated worktree, then restoration of the initial index. Do not commit. Record mutant totals, survivors, and all refusals; manual checks never replace systematic evidence.

## Progress and evidence
- [x] UX-01 (check context, native diagnostic pointers, error-versus-verdict note): five tracked files (`internal/cli/check.go`, `internal/cli/check_test.go`, `internal/cli/flags.go`, `internal/tool/tool.go`, `internal/tool/tool_test.go`). Focused CLI/tool tests green, `gofmt` clean.
- [x] UX-02 (generated rationale): `internal/preset/generic.go` doc comment and `Because`, `internal/preset/generic_test.go`, seven `internal/setup/testdata/golden/*.txt` comment lines, `docs/learning-log.md`. The two generic goldens were hand-edited because `TestGenericGoldenIsUnchanged` deliberately has no `-update` path; the five framework goldens were regenerated with `-update`. Values unchanged: semantic/3/3.
- Incident: one correction writer timed out with a bash call in flight, cause unknown and its transcript unavailable. The worktree was verified terminal and quiescent (no `go.exe`, 36 minutes of frozen mtimes, empty index) before resuming.
- Native ASSESS returned unassessable because the parent-owned untracked ODD file requires a scope declaration; RDD is off, so independent verification was required conservatively. No review authority was started.
- Commits: the earlier hold was reversed and commits were authorized on 2026-10-02. This branch now carries `c9d41ab` (feat(check): explain fallow's duplication ceiling when the gate fails) for UX-01 and `4923774` (docs(preset): state both limits of the duplication floor) for UX-02, each passing the pre-commit gate against its own materialised index; this work-unit document is recorded by a third commit. Nothing is pushed: push, pull request and release remain the user's decisions.

## Verification evidence (independent, UX-03)
- `gofmt -l .`, `go vet ./...` and `go build ./...` clean; `go test -count=1 -timeout=900s ./...` passed all ten packages with no `-update` used anywhere.
- Observable output confirmed by running the built binary in throwaway git repositories outside both checkouts: a whole-tree duplication failure exited 1 and printed fallow's raw output followed by the new note naming the detected manager's read-only `config --path`, `config --format json` and `dupes --format json`, with the `--help` pointer retained; the passing repository exited 0 without the note; a non-dupes failure (`fallow audit` exiting 2 on an invalid `duplicates.mode`) carried no dupes note. The same fixture reproduced the original confusion — `hid 1 clone group below minOccurrences` and `No code duplication found` beside `Duplication (98.5%) exceeds threshold (3.0%)`.
- Mutation `ditto staged` over the staged changed files: 6 of 6 mutatable mutants killed, score 1.00. The index was restored to empty with `git read-tree HEAD`.
- Two manual guard deletions, each proven in-tree with `git diff --quiet` returning 1 and restored from a sha256-verified byte backup: removing the `s.label != fallowDupesStage` early return failed `TestDuplicationContextStaysOffPassingGatesAndOtherFailures`; removing `exit.Code != fallowErrorCode` failed `TestUnrecognizedFallowExitCodeStaysUnclassified`.

## Accepted gaps
- The dupes-stage exit-2 note is not observable through the binary: `check` runs `fallow audit` first and an invalid `duplicates` value fails there, so no single-config fixture reaches `dupes` with exit 2. It is covered only by the unit test and the second manual deletion.
- One generated mutant never compiled and was excluded from the mutation score, so that expression stays unmeasured.

## Next step
The change is implemented and independently verified but intentionally uncommitted. The user decides the commits and integration; the collision-prose drift below is a separate decision.
## Merge readiness (checked after the peer closed their side)
- Patch of the 15 tracked files: `D:/dev/disble/dharness-worktrees/duplication-gate-ux.patch`, 522 lines, sha256 7a7f3327b6382c1fc5dcc2f10979801a29d995100278bec74faa6735d0548b7e. It was taken before the commits and matches their combined diff; the commits are now the integration source and the patch stays frozen on disk as a second copy. The task document is not in the patch; from the third commit onward it is part of the branch.
- Applied with `git apply --3way` onto the peer's tip `549578e` in a temporary detached worktree of the same clone: 14 of the 15 files applied cleanly, including all seven goldens, which confirms the two golden regions are disjoint without hand-merging. Exactly one file conflicts: `docs/learning-log.md`, because both sides appended dated lines after the same 2026-09-16 anchor. Resolution keeps both, theirs first and mine last, matching the append-only rule. The temporary worktree was removed and pruned; the peer's checkout was never touched.
- Golden rule for the combined tree: regenerate the five framework goldens with `TestFrameworkGoldens -update`, then re-derive the two generic goldens by hand, because `TestGenericGoldenIsUnchanged` has no `-update` path by design and a second test pins that the source never wires the flag. Running the full suite is what judges them.
- Combined verification (suite, `ditto staged` over the merged tree, real binary for both output routes) is deliberately not done yet: per-side mutation scores do not cover the other side's code, and the user has not authorized an integration tree.

## Open items outside this unit
- The collision prose in `internal/setup/steps.go` and the remaining sentence in `internal/preset/generic.go` still assert that fallow's `extends` replaces a key rather than merging it, sourced to a fallow 3.14.0 measurement. The versioned 3.30.0 schema describes objects merging field by field, which would make that whole-key conflict explanation overstate a conflict for a project that declares only some fields. Not changed here: collision handling was explicitly out of this unit's scope. The user decides whether to pursue it.
- dharness does not pin fallow (`@latest`), so the preset values and the note wording are measured against whatever the gate resolves at run time.
- User decisions after the peer closed their side: my side stays as verified but uncommitted, the worktree is left ready to be merged and integrated (no integration tree is created by me), and the collision-prose drift is opened as a separate bounded read-only diagnostic.
