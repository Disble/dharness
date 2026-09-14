# Proposal: Make staged mutation scope and test reachability explicit

## Intent

`dharness mutate --staged` currently admits files outside Stryker's effective `mutate` set and reports an opaque Stryker failure when no related test reaches changed runtime code. v1.9 uses Stryker's own discovery signal, names zero-related-test failures before mutation, and exposes phase cost.

## Scope

### In Scope
- Preserve index snapshotting, types-only classification, scoped `stryker run`, and JSON-derived verdicts.
- Use Stryker MSP `discover` to retain only staged runtime ranges in the configured mutate set; retire the local test-file heuristic while retaining `--exclude-prefix`.
- Replace the Vitest-wide listing guard with one runner-specific related-test check and emit the five-phase timing line.
- Add strict JSON-RPC framing and tree lifecycle handling for the MSP server; document the phase flow and amend design principle 01's invalid flag-only premise.
- Complete H-MSP0b, J2, and H-TK in scratch fixtures before product code, applying their pre-registered fallback rules.

### Out of Scope
- MSP `mutationTest`, new Stryker configuration semantics, and changes to scoped verdict status rules.
- Bridge adoption, pushing to `dev`, Stryker upstream filing, publication, and any foreign staged changes.

## Capabilities

### New Capabilities
- `staged-mutation`: Deterministic staged mutation membership, related-test reachability, lifecycle, timing, and actionable output.

### Modified Capabilities
None.

## Approach

Follow P01/P02 by asking Stryker MSP for effective membership, P05 by honoring project `mutate` configuration, P09/P11/P17 by deriving outcomes from protocol data, exit codes, and JSON, and P12/P13/P16 through measured ordering, early exits, and actionable output. Keep command construction in `internal/tool`, protocol and runner behavior in `internal/staged`/`internal/runner`, and orchestration in `internal/cli`. Implement in reviewable WU-A–WU-E slices under the 400-line budget.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `internal/cli/mutate_staged.go` | Modified | Phase orchestration, output, and timing. |
| `internal/staged/` | Modified | MSP client, membership outcomes, related-test guard. |
| `internal/runner/runner.go` | Modified | MSP process-tree cleanup. |
| `internal/tool/tool.go` | Modified | Stryker invocation framing. |
| `internal/*/*_test.go` | Modified | Protocol, lifecycle, and staged behavior coverage. |
| `docs/design-principles.md` | Modified | Correct P01 premise; record evidence. |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Windows shim corrupts MSP frames | Medium | H-MSP0b; fall back to direct Node invocation. |
| Jest listing differs from prediction | Medium | J2; run Jest and read structured totals. |
| MSP descendants survive exit | High | H-TK; use tree kill or document the fallback crash gap. |

## Rollback Plan

Revert the v1.9 work-unit commits, restoring the v1.8 staged flow. Do not alter the current checkout, foreign documentation, bridge worktree, or remote state; implement later in a clean isolated worktree from v1.8.

## Dependencies

- Successful scratch-fixture evidence for H-MSP0b, J2, and H-TK before product coding.
- Existing local Stryker installation and the durable handoff evidence.

## Success Criteria

- [ ] Outside-set, zero-mutant, types-only, and no-related-test outcomes are named before mutation.
- [ ] A no-related-test result exits 1 without invoking Stryker mutation.
- [ ] Every MSP server exit removes its process tree and every run prints completed phase timings.
- [ ] Focused tests, staged mutation evidence, full gate, and a scratch binary-output read pass.
