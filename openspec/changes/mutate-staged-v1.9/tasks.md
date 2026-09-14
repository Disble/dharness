# Implementation Tasks: mutate-staged-v1.9

## Phase 0 — Preconditions and slice discipline

- [ ] 0.1 Confirm the closed probe inputs before product edits: H-MSP0b requires the project-local Stryker npm shim on Windows, J2 requires Jest list-only JSON-array mode, and H-TK requires suspended-start Job Object ownership with no `taskkill` fallback. Keep P1-P3 only; do not add MSP `mutationTest`, new Stryker semantics, bridge work, publication, or upstream work.
- [ ] 0.2 For every code slice below, use strict TDD in this order: add the smallest behavior-level test and observe RED; implement the minimum GREEN change; run the focused alternate/negative cases; load and apply the mutation-TDD decision table after GREEN; manually delete each defensive/error/cancellation guard that its owning test claims to cover; refactor only while focused tests remain green. Stage only that slice and end it with the listed `ditto staged` command before beginning the next slice.
- [ ] 0.3 Deliver each completed slice as its own review unit. Auto-chain is pre-authorized; split or stop before a slice exceeds 400 changed lines, including tests and documentation. Re-run its evidence after any rebase.

## Phase 1 — Observable phase state (slice A)

- [ ] 1.1 **A — Implement the fixed five-phase record and deterministic orchestration seam** (forecast 220-320 lines; low budget risk). In `internal/cli`, add the fake-clock seam and one deferred renderer created before preflight returns. It must print exactly one stdout record, in this fixed order: `snapshot`, `classify`, `discover`, `related`, `stryker`; render completed, failed, and not-started states exactly as specified; flatten only the rendered error reason to one line; preserve the underlying returned error.
  - RED: cover all-not-started after prefix filtering, all-types-only, no-retained-range, zero-related verdict, each operational phase failure, scoped survivor verdict, and full success. Each row asserts exactly one ordered five-label record and no later invocation after early return.
  - GREEN/TRIANGULATE: prove completed-negative verdicts (zero related and surviving mutant) are completed phases rather than instrumentation failures; prove multiline errors and deterministic durations; retain the existing snapshot and scoped-verdict behavior.
  - MUTATE: manually delete the deferred render/failure transition guards and prove the owning focused tests fail; then run `ditto staged --exclude-prefix tools/ --threshold 0.80 --test-command "go test -short -count=1 ./internal/cli/"`.

## Phase 2 — MSP transport and process ownership

- [ ] 2.1 **B1 — Add a strict, sequential Content-Length JSON-RPC codec** (forecast 280-380 lines; medium budget risk). In `internal/staged`, implement in-memory framed request/response handling for MSP: one exact ASCII length header, required separator, decimal byte length, split/coalesced reads, JSON-RPC `2.0`, monotonically correlated IDs, and separate stderr draining. Treat invalid framing, invalid JSON, mismatched IDs, protocol errors, and early EOF as failures rather than empty discovery.
  - RED: demonstrate every accepted framing boundary and reject stray stdout, malformed header spelling, duplicate/missing/invalid/overflowing lengths, truncated body, invalid JSON/version, wrong ID, error response, and early server exit.
  - GREEN/TRIANGULATE: prove stderr never reaches the parser and a valid empty discovery remains distinguishable from transport success.
  - MUTATE: manually delete framing/correlation/error guards and prove focused tests fail; then run `ditto staged --exclude-prefix tools/ --threshold 0.80 --test-command "go test -short -count=1 ./internal/staged/"`.

- [ ] 2.2 **B2.1 — Add the managed-process contract and Unix tree lifecycle** (forecast 180-300 lines; low budget risk after the B2 split). In `internal/runner`, retain one-shot `Run` unchanged and add the long-lived process API used only by MSP: stdin/stdout/stderr, wait, and idempotent close. On Unix, create a new process group and kill/reap the negative group ID on normal return, protocol failure, and cancellation; an exited leader must not suppress group cleanup. Combine cleanup failure with the primary error.
  - RED: use a helper that creates descendants and show that normal close, protocol error, cancellation, and an already-exited leader initially leave a process-tree lifecycle gap.
  - GREEN/TRIANGULATE: prove all owned Unix descendants are gone and reaped, cancellation unblocks I/O, and cleanup failure cannot become discovery success.
  - MUTATE: manually delete group-kill, wait, and primary-error-combination guards and prove focused tests fail; then run `ditto staged --exclude-prefix tools/ --threshold 0.80 --test-command "go test -short -count=1 ./internal/runner/"`.

- [ ] 2.3 **B2.2 — Add Windows Job Object ownership** (forecast 220-340 lines; medium budget risk after the B2 split). In Windows build-tagged `internal/runner` code, implement Job Object ownership with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`: suspended launch, job creation/configuration, assignment before user code runs, resume, and close/reap. Fail startup if any ownership step fails; do not add direct-Node or `taskkill` fallbacks.
  - RED: on Windows, use a controlled helper to show the child cannot run before assignment and descendants survive without Job Object ownership.
  - GREEN/TRIANGULATE: prove job close removes the shim, Stryker child, and proxy descendants on success, protocol failure, cancellation, and owner crash control; inspect product code to prove no `taskkill` path exists.
  - MUTATE: manually delete assignment/resume/close failure guards and prove focused Windows tests fail; then run `ditto staged --exclude-prefix tools/ --threshold 0.80 --test-command "go test -short -count=1 ./internal/runner/"`.

## Phase 3 — Discovery and retained staged scope

- [ ] 3.1 **C1 — Implement the MSP configure/discover client** (forecast 280-380 lines; medium budget risk). Add the `internal/staged` client over the B1 codec and B2 managed process. Build `stryker serve stdio` only through `internal/tool`, run it in `snapshotSource`, configure it from snapshot JSON (or its snapshot default resolution), then issue one ranged discovery request and, only when needed, one path-only request. Close and reap on success, protocol failure, early exit, and cancellation.
  - RED: cover request ordering, snapshot-relative slash paths/range coordinates, configured config-file selection, early exit, malformed/incomplete responses, protocol errors, and cancellation cleanup.
  - GREEN/TRIANGULATE: run a controlled process for configure plus discovery; where local Stryker exists, exercise the real local shim and permit a skip only for explicit local-tool absence. Prove no discovery failure can start mutation.
  - MUTATE: manually delete request-validation, cancellation, and cleanup guards and prove focused tests fail; then run `ditto staged --exclude-prefix tools/ --threshold 0.80 --test-command "go test -short -count=1 ./internal/staged/"`.

- [ ] 3.2 **C2 — Classify membership per staged range and retire the local test-name heuristic** (forecast 300-400 lines; high-edge budget risk). Preserve Git range/rename/CRLF/partial-stage checks, declaration/source filters, and pre-classification `--exclude-prefix`; remove only `.test.`, `.spec.`, and `__tests__` membership guessing. In `internal/staged`, retain only original ranges represented by ranged MSP mutants, path-discover only files with no retained range, and produce sorted snapshot-relative outcomes: `in the set, 0 mutants in the staged lines: <path>` for a path-only hit and `outside Stryker's mutate set: <path>` for both-form omission. Never explain the latter as configured exclusion or intrinsic zero-mutant.
  - RED: cover a multi-range file with partial retention, a partially retained file that never path-discovers, path-only hit, both-form omission, prefix exclusion before both discovery forms, and test-shaped paths reaching discovery.
  - GREEN/TRIANGULATE: preserve rename pairing, index ranges, Source-not-Root, and partial-staging refusals; prove no retained range exits 0 before related or mutation and all outcomes are stable/sorted.
  - MUTATE: manually delete range-intersection, no-path-discovery, prefix, and ambiguity-wording guards and prove focused tests fail; then run `ditto staged --exclude-prefix tools/ --threshold 0.80 --test-command "go test -short -count=1 ./internal/staged/"`.

- [ ] 3.3 **C3 — Wire discovery into snapshot-scoped CLI orchestration and final scope assembly** (forecast 300-400 lines; high-edge budget risk). In `internal/cli`, re-read `StrykerSelection` from the index snapshot and use it for MSP configuration, runner selection, optional Vitest config, generated mutation configuration, and final Stryker invocation. Run classification before discovery; announce dropped types-only files; generate final mutation scope from retained runtime ranges plus classifier-dropped self-check ranges only; preserve existing JSON range-scoped verdict rules.
  - RED: cover snapshot-versus-working-tree config disagreement, all-types-only early exit, retained/discarded final config composition, classifier disagreement, classifier agreement excluded from tally, and outside-range `Survived`/`NoCoverage` not affecting the verdict.
  - GREEN/TRIANGULATE: prove retained in-range `Survived`/`NoCoverage` fail, `Killed`/`Timeout`/`Ignored` pass, classifications unavailable drops nothing, and working-tree-only bytes cannot alter membership or verdict.
  - MUTATE: manually delete snapshot-authority, classifier-disagreement, and range-scope guards and prove focused tests fail; then run `ditto staged --exclude-prefix tools/ --threshold 0.80 --test-command "go test -short -count=1 ./internal/cli/ ./internal/staged/"`.

## Phase 4 — Related-test reachability (slice D)

- [ ] 4.1 **D — Add runner-owned related-test commands, structured parsers, and CLI gates** (forecast 320-400 lines; high-edge budget risk). In `internal/tool`, construct exactly one related command over unique sorted retained files: Vitest `vitest related <files> --run --passWithNoTests --reporter=json --outputFile <json-output> [--config <vitest-config>]`, or J2-confirmed Jest `jest --findRelatedTests <files> --listTests --json`. In `internal/staged`, parse only Vitest's non-negative numeric `numTotalTests` file or Jest's JSON array. In `internal/cli`, retire the repository-wide Vitest-list guard; make non-zero runner exits and malformed structured output fail before mutation, zero aggregate exit 1 with the exact actionable message, and positive aggregate proceed without per-file reachability claims.
  - RED: pin command tokens/order and the absence of a Jest fallback; cover Vitest zero/positive/missing/wrong-typed/malformed/unreadable JSON and non-zero exit; cover Jest empty/non-empty/non-array/malformed JSON and non-zero exit.
  - GREEN/TRIANGULATE: prove only zero may name every retained file in `no test reaches: <files>; the fix is a test that imports them`; prove positive results make no individual reachable/unreachable claim and invoke Stryker exactly once after the one related command.
  - MUTATE: manually delete JSON validation, zero-total, non-zero-exit, and no-follow-on-mutation guards and prove focused tests fail; then run `ditto staged --exclude-prefix tools/ --threshold 0.80 --test-command "go test -short -count=1 ./internal/tool/ ./internal/staged/ ./internal/cli/"`.

## Phase 5 — Documentation and observable readback

- [ ] 5.1 **E1 — Update the user and maintainer documentation** (forecast 250-380 lines; medium budget risk). Update `README.md`, `docs/flujo-implementado.md`, `docs/design-principles.md`, `docs/learning-log.md` (append-only, dated entry), and applicable mutation backlog entries. Describe the snapshot → classify → discover → related → stryker flow, honest membership outcomes, aggregate related-test boundary, preserved scoped JSON verdict, platform lifecycle boundary, and phase-cost boundary. Amend principle 01 so it recognizes Stryker MSP discovery as Stryker's effective-membership mechanism and no longer claims every Stryker option is a flag.
  - Documentation check: compare every example against the binding output grammar; include all five phase labels and the exact outside-set, in-set-zero, and no-test-reaches strings. Keep `docs/mutation-testing.md`, bridge files, and `chat-ytchannel/` unchanged.

- [ ] 5.2 **E2 — Synchronize artifact mirrors and capture observable binary evidence** (forecast 200-350 lines; medium budget risk). Update the artifact mirrors for `docs/flujo-implementado.md` and `docs/design-principles.md` without changing their authoritative source relationship. Build the binary outside this repository and run it only in scratch JavaScript projects with Source not equal to Root. Read and record output for outside-set, in-set-zero, no-related, types-only, failed phase, full timing record, retained-range survivor, snapshot-config disagreement, and process-cleanup cases.
  - Documentation/readback check: ensure examples and mirrors use the five labels in order, never overstate both-form omission or positive related aggregates, and show the preserved scoped verdict. Keep evidence with the implementation handoff; do not run `dharness` against this checkout.

## Phase 6 — Requirement traceability and chained delivery

- [ ] 6.1 Before requesting final review, verify this traceability matrix against the completed slice evidence: preserved staged behavior and snapshot-scoped configuration → A/C2/C3; deterministic membership and honest empty outcomes → C1/C2/C3; related-test reachability → D; classifier disagreement safety and scoped verdict → C3; MSP framing/lifecycle → B1/B2.1/B2.2/C1; phase output → A; documentation deliverables → E1/E2. Resolve any unrepresented SHALL or scenario in its owning slice rather than adding an unbudgeted catch-all task.
- [ ] 6.2 Deliver A, B1, B2.1, B2.2, C1, C2, C3, D, E1, and E2 as chained review units in that order. Re-estimate changed lines before each review, split any slice that reaches 400 lines, and attach focused RED/GREEN/manual-guard/`ditto staged` evidence to each code unit plus the E2 observable-output receipt.

## Review Workload Forecast

- Chained PRs recommended: **Yes**.
- 400-line budget risk: **High** — B2 is deliberately split into Unix/common and Windows work; C2, C3, and D sit at the budget edge and must split before review if their forecast grows.
- Estimated changed lines: **2,500-3,400** including tests and documentation.
- Decision needed before apply: **No** — the prerequisite probes are closed and auto-chained delivery is pre-authorized; implementation remains limited to P1-P3.
