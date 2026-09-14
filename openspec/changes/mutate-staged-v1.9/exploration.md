# Exploration: mutate-staged-v1.9

## Current State

`HEAD` is v1.7.6 (`d96043d`); v1.8.0 is available read-only at `origin/main` (`95059fc`). The shipped staged flow scopes index additions, snapshots index bytes, classifies types-only files, runs the slow Vitest-wide `vitest list` guard, invokes local Stryker, and decides the scoped result from Stryker JSON. v1.9 retains the snapshot, classification, scoped Stryker run, and JSON verdict while adding Stryker MSP discovery, actionable related-test failures, and phase timing.

The design is supported by principles 01, 05, 09, 11, 12, 13, 16, and 17: use Stryker's direct discovery API for membership; preserve project configuration; derive verdicts from exit codes and JSON; perform cheaper early exits; and keep output actionable. The proposal must amend principle 01 because its current premise says every Stryker option is a flag, while v1.9 relies on the documented MSP/configuration surface.

## Affected Areas

- `internal/cli/mutate_staged.go` at `origin/main` — orders the staged phases, renders skips and verdicts, and replaces `GuardVitestSuite`.
- `internal/staged/guard.go` at `origin/main` — current Vitest-wide listing guard; successor owns related-test execution and structured result parsing.
- `internal/staged/` — owns staged scopes, type classification, snapshot isolation, and the membership client/outcome model.
- `internal/runner/runner.go` at `origin/main` — current cancellation kills only the direct process; MSP requires lifecycle ownership for descendants.
- `internal/tool/tool.go` at `origin/main` — all product command construction remains here, including Stryker MSP invocation framing.
- `internal/cli/mutate_staged_test.go`, `internal/staged/*_test.go`, `internal/runner/*_test.go` at `origin/main` — behavior, framing, process lifecycle, and real-repository coverage.
- `docs/design-principles.md`, `docs/flujo-implementado.md`, `docs/learning-log.md`, and their required mirrors — document the direct-tool delegation, phase order, and measured findings.

## Evidence Availability

- `docs/handoff-mutate-staged-v1.9.md` — owner-approved P1–P3 design, v1.8 map, work units, measurements, test contract, and known residuals.
- `D:\dev\disble\dharness-handoff-v1.9\golden-mutate-membership.tsv` — durable membership oracle for WU-B/C.
- `D:\dev\disble\dharness-handoff-v1.9\msp-probe.mjs` and `msp-*.log` — existing MSP protocol and lifecycle evidence.
- `D:\dev\disble\dharness-handoff-v1.9\h-p2*.{json,log,sh}` and `h-p3-*.log` — related-test and timing evidence.
- Engram `#9540` — measured stdin-EOF orphan behavior; it also records that H-MSP0b, J2, and H-TK remain unmeasured.

No measurement was executed during this exploration.

## Approaches

1. **Use Stryker MSP discovery and related-test commands** — retain current scope/snapshot/classify behavior, then ask Stryker which staged runtime ranges it will mutate and run the selected runner's related-test command.
   - Pros: Direct membership signal; removes the brittle test-file heuristic and broad Vitest list cost; gives a named zero-related-test verdict before Stryker.
   - Cons: Requires a strict JSON-RPC stdio client and process-tree lifecycle control.
   - Effort: High.

2. **Extend the existing local filters and Vitest listing guard** — add more exclusions and improve its error message.
   - Pros: Small code change.
   - Cons: Repeats Stryker's matching logic, remains incomplete across project configurations, and retains the measured ~61-second preflight.
   - Effort: Medium.

## Recommendation

Proceed with approach 1 in the planned WU-A through WU-E sequence. Complete H-MSP0b, J2, and H-TK in scratch fixtures before product coding, using the handoff's pre-registered predictions and kill conditions. Keep the implementation boundary in `internal/tool` for product command construction, with staged behavior and parsing held in `internal/staged` and orchestration in `internal/cli`.

## Risks

- H-MSP0b may show that an npm `.cmd` shim contaminates or corrupts JSON-RPC stdout; direct Node invocation would then be required.
- J2 may invalidate the planned Jest list-only branch; the measured fallback runs Jest and reads structured totals.
- H-TK has no measurement yet. MSP servers survive stdin EOF and direct-process killing leaves descendants alive, so release work cannot claim safe lifecycle handling until the Windows tree-kill probe passes or its fallback and crash gap are documented.
- The local checkout has foreign modifications (`docs/mutation-testing.md`, `chat-ytchannel/`) and is twelve commits behind `origin/main`; exploration preserved them. A future implementation must use a clean, user-authorized worktree based on v1.8.0.
- Current OpenSpec config is stale about ooze and its mutation command. `AGENTS.md` is authoritative; planning must avoid copying the stale config claims.

## Ready for Proposal

Yes. The owner-approved scope, design, evidence locators, and review-sized work units are sufficient for a proposal. The proposal and design must make the three scratch measurements blocking prerequisites for product coding and record their fallback decisions. No additional human product decision is currently required; publishing, pushing the bridge branch, and Stryker upstream filing remain outside the granted authorization.
