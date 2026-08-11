# Archive Report: unify-init-and-sync

**Change**: `unify-init-and-sync`  
**Archived**: 2026-08-11  
**Status**: CLOSED — Merged to main (PR #4), released as v1.0.0  

## Artifact Traceability

All SDD artifacts retrieved from Engram (`project: dharness`):

| Artifact | Observation ID | Topic Key | Persisted |
|----------|---|---|---|
| Proposal | #7980 | `sdd/unify-init-and-sync/proposal` | Engram |
| Spec | #7982 | `sdd/unify-init-and-sync/spec` | Engram, `openspec/specs/setup/spec.md` (synced) |
| Design | #7984 | `sdd/unify-init-and-sync/design` | Engram |
| Tasks | #7985 | `sdd/unify-init-and-sync/tasks` | Engram |
| Verify-Report | #7988 | `sdd/unify-init-and-sync/verify-report` | Engram |

## Final State — Authority Hierarchy

### Rank 1: Orchestrator's Final-State Facts

Per the team lead's message at close:

1. **Merged to main** in PR #4 with `--merge`, preserving three commits:
   - `558cac7` docs(design)
   - `565d1b4` chore(sdd)
   - `af5f353` feat(cli)!

2. **Released as v1.0.0** on 2026-08-11 to GitHub (https://github.com/Disble/dharness/releases/tag/v1.0.0)
   - Bumped to 1.0.0 (not 0.2.0) because breaking change before 1.0 goes straight to major.

3. **Verification**: `sdd-verify` returned **PASS WITH WARNINGS** (0 CRITICAL, 4 WARNING, 1 SUGGESTION)
   - No critical blockers to archive.
   - Warnings addressed and reported separately.

4. **Verification Commands Run** (per verify-report #7988):
   - `go build ./...` → EXIT:0
   - `go vet ./...` → EXIT:0
   - `gofmt -l .` → EXIT:0 (no unformatted files)
   - `go test ./...` → all packages ok, EXIT:0
   - `bash scripts/verify-gate.sh` → "the hook refused a broken file, as it must", EXIT:0
   - Mutation testing: Slice 2 score 0.86 (min 0.80)

### Rank 2: Task Completion Gate

**Status**: PASS

- Total tasks: 42
- Completed (`[x]`): 41 tasks + 1 pre-accepted (`[~]`)
- Task 2.7 (mutation testing): pre-accepted per orchestrator scope
  - Extends-logic survivors killed; overall score limited by pre-existing survivors in init.go/sync.go, documented in mutation-testing.md.
- Task 9.3 (artifact republish): completed by orchestrator (marked `[x]` with note that republish was done to exact URL)

### Rank 3: Follow-up Work

Three follow-ups were recorded at close. Per orchestrator message:

1. **`install-step-declared-deps`** — **CLOSED**, shipped in v1.0.2 (commit `3afbe65`)
   - `installStep` stopped reading `node_modules` and now runs the package manager's own idempotent install, reading the exit code as the verdict.

2. **`writer-undo-completeness`** — **OPEN**
   - `Writer.Undo` does not remove directories it created and never sees the `.gitignore` written outside it by `EnsureDir`.
   - Failure message is deliberately hedged.
   - Constraint: rollback wording in `sync`'s output must not claim everything written was undone until this lands.

3. **`owned-files-version-step`** — **OPEN**
   - Version stamping and the `fallow.jsonc` merge algorithm; needs its own design pass.
   - Risk: getting it wrong silently deletes agent-authored `boundaries`.

## Specs Synced

| Domain | Action | Capabilities | Details |
|--------|--------|---|---|
| `setup` | Created | `project-sync` (5 requirements), `step-delegation` (4 requirements) | New capabilities; `openspec/specs/` was empty. Spec copied from `openspec/changes/unify-init-and-sync/spec.md` to `openspec/specs/setup/spec.md` (byte-identical). |

The spec declares two new capabilities:
- **`project-sync`**: One command that derives the plan from the repository on every run, applies dharness's own steps, and hands the rest to the agent.
- **`step-delegation`**: The per-project recipient decision — who can do this step in *this* repository — answered without executing anything.

Both capabilities are fully specified with GWT scenarios and testability notes. See `openspec/specs/setup/spec.md`.

## Archive Contents

✅ Moved to `openspec/changes/archive/2026-08-11-unify-init-and-sync/`:
- `proposal.md` (12 KB)
- `spec.md` (14 KB)
- `design.md` (18 KB)
- `tasks.md` (7 KB)
- `exploration.md` (4 KB)
- `apply-progress.md` (3 KB)
- `verify-report.md` (5 KB)

## Key Design Decisions Implemented

All seven architecture decisions from `design.md` (#7984) confirmed present in code:

1. **Report shape**: Three regions (header, Applying:, ## Left to you:)
2. **No prompt types**: Fusión/Conducción/Intención stay in the `why` string
3. **Rollback wording**: Hedged to avoid overclaim pending `writer-undo-completeness`
4. **`extendsStep` splits**: Separate `fallowExtendsStep` + `lefthookExtendsStep` per recipient
5. **`hookInstallStep` fix**: Returns delegated (open decision) when no hook manager answers; §20 rationale
6. **`architectureStep`**: Satisfied by declared `boundaries`, disappears once satisfied (§15)
7. **Repository hard stop (6bis)**: Stops before writing when not in a git repository; exit 1 with clear message

## Documentation Corrections

All false statements in `docs/flujo-implementado.md` were corrected in the change:

| What became false | What it became | Status |
|---|---|---|
| Fusión: "No existe" | "Exists **for the `extends` case only**" | ✅ Qualified |
| Fusión: "Dónde se usa" (doctor.config.ts case) | "Not delivered in this change" | ✅ Verified via code inspection |
| Conducción: "Existe como motivo y descripción" | Confirmed as actual Decision 1 report shape | ✅ Verified |
| Intención: "El único prompt real" | False; Fusión is now a second real prompt | ✅ Corrected |
| Header: "Cinco comandos" | "Cuatro comandos" (`init` removed) | ✅ Updated |
| Figure 1: Two stop states | Separated (no repo vs. no JS project with different exit codes per Decision 6bis) | ✅ Added |

Artifact republished to https://claude.ai/code/artifact/6033beba-ca70-4163-92fd-97de7ed8663e (by orchestrator).

## Field Discoveries

Two important limits discovered in the field after shipping:

1. **fallow's `extends` REPLACES a key rather than merging it** (measured against fallow 3.14.0)
   - This change wrote the architecture into `.dharness/fallow.jsonc` and had the project's config point at it by reference.
   - That mechanism silently stops working the moment the project declares its own `boundaries`.
   - `boundariesOwnerStep` was added in v1.0.2 to report exactly that.
   - **Worth recording**: a limit discovered in the field, not a defect of the change.

2. **`folder-ownership` now ships `off`** (v1.0.2)
   - It requires a split module to publish an `index.ts`, which a project that bans barrel files cannot satisfy.
   - Eight permanent findings measured on one such repository.
   - Shipping `off` by default was the pragmatic resolution.

## Verification Summary

**Verdict: PASS WITH WARNINGS** (per verify-report #7988)

- 0 CRITICAL issues
- 4 WARNING issues
  1. Task 9.3 republish unverifiable from verifier's tool access (but orchestrator confirms done)
  2. Scenario "undoing a step by hand makes it reappear" not directly exercised (true by construction, no persisted state)
  3. Line budget overrun: Slice 2 measured 613 changed lines vs. 400-line budget (disclosed honestly, not concealed)
  4. Mutation testing not independently re-run this session (relied on apply-progress scores)
- 1 SUGGESTION

- **Design coherence**: All 7 architecture decisions confirmed present in code

## SDD Cycle Status

✅ **COMPLETE**

1. ✅ Proposal (Revision 2, documentation and budget risk addressed)
2. ✅ Spec (Two new capabilities defined with GWT scenarios, synced to main specs)
3. ✅ Design (Seven decisions documented and implemented)
4. ✅ Tasks (42 tasks, 41 complete, 1 pre-accepted; all work shipped)
5. ✅ Apply (Both slices shipped; all key changes in v1.0.0)
6. ✅ Verify (PASS WITH WARNINGS, no blockers)
7. ✅ Archive (Specs synced, change moved to archive folder, report written)

## Deliverables Summary

| Item | Status | Location |
|------|--------|----------|
| Merged to main | ✅ | PR #4, three commits preserved |
| Released | ✅ | v1.0.0 on 2026-08-11 |
| Spec synced to main | ✅ | `openspec/specs/setup/spec.md` |
| Change archived | ✅ | `openspec/changes/archive/2026-08-11-unify-init-and-sync/` |
| Tests | ✅ | All pass, 0.86 mutation score |
| Gate | ✅ | Refuses broken files as it must |
| Documentation | ✅ | Corrected and published |

---

**Archive created**: 2026-08-11  
**Orchestrator**: team-lead  
**Executor**: sdd-archive (Claude Opus 5)  
**Session ID**: dharness-archive-20260811
