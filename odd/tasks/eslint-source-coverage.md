# ESLint source coverage and sync remediation

## Objective
Prevent dharness's ESLint rules from silently missing recognized source extensions, and help consuming agents repair their integration during the next `dharness sync`.

## Problem and rationale
An owned flat-config block without `files` depends on matchers supplied by the consumer. A TypeScript-only matcher leaves TSX unconfigured; ESLint can exit zero and `--no-warn-ignored` suppresses the warning. `--print-config` proves configuration resolution, not parsing. Make ESLint own the actual verdict instead of implementing another matcher or interpreting human diagnostics.

## Authorized scope and constraints
- Work only in dharness on `fix/eslint-source-coverage`; do not inspect or modify dhub.
- Change owned configuration and invocation/reporting seams only. Preserve consumer parsers, global ignores, framework configs, thresholds, and deliberate overrides.
- No new product/module dependency, no new wrapped CLI, no forced parser installation.
- Sync reports actionable remediation to the consuming agent; it never edits consumer source/parser configuration and never rolls back valid wiring merely because consumer code has lint findings.
- Use native ESLint evidence; a fatal diagnostic does not by itself prove a missing parser.
- Observe RED -> GREEN -> MUTATE -> REFACTOR where applicable. Run the built binary outside this repository for observable output checks.
- Conventional work-unit commits are part of approved ODD; no push, PR, merge, or release is authorized.
- Keep writes single-threaded and preserve pre-existing unrelated changes: `docs/mutation-testing.md`, `chat-ytchannel/`, `docs/backlog/dharness-mutate-staged-findings.md`.
- Design principles: 01/09 native signals; 03/05 ownership; 11 native verdicts; 15 rediscovery; 20/21 actionable agent handoff.
- RDD: off, clone-local and global. Ordinary risk-based independent verification applies.

## Tasks
- [ ] **T1 — Make the owned ESLint layer match recognized source extensions.** Status: in progress (independent verifier `murjj8k7-6-ab0z` checking staged candidate).
  - Declare explicit source matchers without adding parser/language options; retain separate global-ignore objects.
  - Verify recognized extension semantics, including TSX, consumer parser preservation, deliberate global ignores, and split layouts/config dialects where applicable.
  - Add regression scenarios and update existing golden files through their supported update path; include user-facing documentation with the behavior.
  - Checks: observed focused RED/GREEN; real ESLint in a throwaway project (rules fire with proper parser; parser gap fails rather than silently passing); focused manual mutation where applicable; staged `ditto staged`; Go format/build/vet/test; independent verification when assessment requires it.
  - Writer evidence: RED observed for omitted TSX matchers, missing API, and byte-pinned matcher; GREEN on setup/project packages and five real ESLint cases; three manual mutations killed and restored. `mutation-tdd` skill unavailable; existing protocol used.
  - Runtime evidence: writer's five ESLint cases passed; independent checks running. Parent spot-check `go test ./internal/project -run TestIsSourceFileFoldsTheExtensionCase -count=1` passed. Commit: pending. Rollback: matcher generation and its tests/docs only.
- [ ] **T1b — Preserve the real-ESLint regression contract as a reviewable work unit.** Status: pending.
  - Commit the opt-in native regression separately (244 authored lines), after T1's production/unit/golden/docs slice (264 lines); no tests/docs are removed to meet the budget.
  - Checks: independent real-ESLint cases, offline skip contract, full suite. Staged mutation is N/A for a test-only work unit with no product branches; T1 production mutation remains mandatory.
  - Runtime evidence: pending. Commit: pending. Rollback: opt-in regression test only.
- [ ] **T2 — Surface sync integration diagnostics and corrective handoff.** Status: pending.
  - Reuse existing local ESLint resolution and bounded per-extension probes; keep process arguments in `internal/tool`.
  - Add real parsing/lint evidence distinct from configuration-load verification and actionable instructions in the existing sync report/prompt path.
  - Preserve adoption ladder: recoverable diagnostics are pending corrective work, not rollback of valid owned wiring or an assertion that lint passed.
  - Do not add a whole-repository audit to every commit; ordinary gate remains staged.
  - Checks: focused RED/GREEN and defensive-path mutations; healthy/broken/unavailable ESLint scenarios; same sync after repair no longer requests the satisfied correction; bounded real ESLint plus built dharness runtime output in a scratch Git project; staged `ditto staged`; Go format/build/vet/test; independent verification when assessment requires it.
  - Runtime evidence: pending. Commit: pending. Rollback: bounded diagnostics invocation and sync handoff only; T1 stays independently useful.

## Acceptance criteria
- Supported source extensions cannot be silently outside the owned rule block solely because the consumer omitted a matcher.
- Missing parser support produces ESLint's real diagnostics/nonzero lint verdict; adequate existing parsers are unchanged.
- Explicit global ignores remain effective and are not presented as accidental parser gaps.
- On upgrade, plain `sync` regenerates the owned layer and provides evidence-backed corrective instructions without modifying repo-owned parser or source files.
- Second sync after correction derives current state rather than retaining a migration flag.
- No policy to forbid deliberate consumer overrides is introduced; mutable configuration remains a stated limit.

## Evidence and progress
- Approved by user: "dale, aprobado con odd".
- Starting commit: `f2a97b6`; branch created before feature writes.
- Disposable ESLint fixture prepared and import-checked: `C:/Users/User/AppData/Local/Temp/dharness-eslint-fixture.aa3uau`; ESLint 10.11.0, typescript-eslint 8.71.0, dharness-eslint-plugin 0.3.0, TypeScript alias @typescript/typescript6 6.0.2. Set `DHARNESS_ESLINT_FIXTURE` for opt-in checks. This is dependency preparation, not behavior verification.
- T1 independent checks: gofmt clean; build/vet/full offline tests passed; native contract five cases passed and default skip explicit. `ditto staged --dry` and `ditto staged` exit 0: 7 generated, 5 scored/killed, 0 survivors, 2 non-compiling Integer Decrement mutants excluded; score 1.00.
- Built dharness plain sync in scratch exit 0 (5 applied, 3 delegated, 3 satisfied, 0 failed); eight owned matchers present and consumer parser/global ignores preserved. Normal TSX with actual JSX proof pending final verifier report.
- Gate failure-path script not run: it writes/stages/resets checkout probes and was unsafe with pre-existing work. Gate code is unchanged; the normal commit hook will run. This is an explicit skipped check, not a passed gate proof.
- Worker suspicion that a literal `.ts` filename falls outside the glob was refuted by actual ESLint `dot:true` resolution (.ts and .TS covered).
- Exploration completed: owned factory and seven golden fixtures identified; sync needs a post-wiring diagnostic note, not a rollback-causing verification failure.
- T1 authored result: 508 lines, split into production/unit/golden/docs (264) and reusable native contract (244), preserving complete behavior tests in the first slice.
- Native ASSESS could not assess the ambient diff because pre-existing untracked `chat-ytchannel/` is an unrelated nested Git repo. Do not alter that directory/ignore configuration; treat assessment as high risk and require independent verification.
- T1 external-test decision: offline default Go suite; an opt-in real-ESLint fixture prepared outside the repository is mandatory release evidence, not an automatic network install inside tests.

## Next step
Verify T1 independently, including full Go checks, the native regression and staged mutation; commit T1 and the separate T1b native contract, recording both identities before starting T2.
