# Sync guidance hotfix release
Objective: verify the complete v1.13.0 hotfix, integrate through a feature-branch chain, then release through release-please.
Branch: `fix/sync-adoption-guidance`; delivery: `feature-branch-chain`; checks: Go suite/vet/build, gate refusal, real CLI fixtures and GitHub CI. Forecast: 718 changed lines before tracking, including approximately 280 repeated golden lines.

## Specs
S1. "manda todo a un verifier y si pasa, haz release en github" — independent whole-change PASS is required before publication; never claim a release from local checks alone.
S2. "es fundamental que cualquier LLM sepa con claridad qué puede o no puede hacer, u ofrecer a su usauario de dharness" — verify the existing staged report/guidance hotfix and its observable behavior.
S3. "Cadena sobre una rama integradora" — chosen delivery strategy: tracker PR stays draft/no-merge until both child units are integrated; child 1 targets tracker, child 2 targets child 1.

## Tasks
T1 | S1-S2 | delegated `gentle-ai-verify` mva0vgo1-4-loty | done | whole-candidate PASS; build/vet/full suite/format/diff/mutation dry/gate refusal/real sync and ESLint10 probes all passed; commit: N/A (read-only gate).
T2 | S1-S3 | inline delivery | in_progress | split ESLint and coverage into cohesive child work-unit commits/PRs, retain tests/docs, integrate in order, validate final SHAs and CI; commits: pending.
T3 | S1,S3 | inline release | pending | refresh main, inspect and merge generated release-please PR with matching changelog/version/tag; commit: release-please-owned, pending.
T4 | S1 | inline release verification | pending | verify Actions, release tag/SHA and all six platform archives plus checksums; commit: N/A (read-only delivery evidence), pending.

## Log
L1. User: "manda todo a un verifier y si pasa, haz release en github"
L2. User choice: "Cadena sobre una rama integradora" (`feature-branch-chain`).
L3. Original request: "Sobre la ultima release, hay un hotfix que quiero que hagas: C:\Users\User\AppData\Local\Temp\pi-clipboard-4db1304a-4d00-4bc6-8e73-e5d64595d5e2.png. Se nos reportó esto, y es fundamental que cualquier LLM sepa con claridad qué puede o no puede hacer, u ofrecer a su usauario de dharness. Necesitamos una mejora en este apartado."
L4. Baseline verified after fetch: HEAD = origin/main = ca11e5303b69f5b0b7670c61a2dfdaabd3672466 (v1.13.0). GitHub reports ADMIN permission, no rulesets and main protection endpoint returns 404 Branch not protected; merge commits disabled, rebase/squash allowed. No PRs open and no release pending before integrating this hotfix. Release-please, not manual tags/releases, owns publication.
L5. Existing implementation evidence: full Go suite/vet/format passed; real sync output and worker ESLint10.12.0 before exit2/after exit0 verified; independent earlier R1-R3 PASS. Staged mutation: 16 total,15 killed,1 observationally equivalent survivor under the source-scope invariant,score0.94>=0.80. New user-requested whole verifier is running. No commits/push/publication performed before its PASS.
L6. Preserve original checkout's docs/mutation-testing.md edit and untracked chat-ytchannel/ and docs/backlog/dharness-mutate-staged-findings.md; operate only in the registered same-clone hotfix worktree. New tracking document is delivery bookkeeping and does not change the verified product candidate.
L7. Plan reconciliation: the requested whole verifier mva0vgo1-4-loty remains the only active task; no PASS/FAIL received yet. T2-T4 have not started. All 20 product hotfix files remain staged and unpublished; this untracked document records delivery only. Release sources of truth and chain details read; no manual release/version/tag substitution is permitted.
L8. Whole verifier mva0vgo1-4-loty delivered PASS, no blockers: build/vet/full suite/format/cached diff check/mutation dry run/gate refusal checks and real sync probe exit0; directly reproduced ESLint10.12 generated-target exit2 vs source exit0. Confirmed the sole systematic mutation survivor observationally equivalent. No local race, ESLint9 or complete native Vitest adoption claimed. Conditional user publication authorization is now satisfied; T2 begins.
