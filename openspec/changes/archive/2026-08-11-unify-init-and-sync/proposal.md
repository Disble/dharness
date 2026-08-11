# Proposal: unify-init-and-sync

Specification of record: `docs/flujo-implementado.md` (sections "Adopción y
actualización — dentro de `dharness sync`", "Qué es el plan", "3. Entregar", "Qué
hace reaparecer un paso", and the decision table "Situación / Qué hace sync /
¿Detiene?"). Exploration: `openspec/changes/unify-init-and-sync/exploration.md`,
Engram `sdd/unify-init-and-sync/explore`.

## Intent

A repository that already configured fallow gets its entire adoption aborted and
rolled back. `extendsStep.Apply` returns a hard error when the project's own
`.fallowrc.json` or `lefthook.yml` already has content; that error is
indistinguishable from a destructive failure, so `Writer.Undo` reverts the whole
run. The block fires precisely when the project is best configured — the case
that produced §20.

The cause is that the recipient of a step is a compile-time fact. `Delegated` is
a second interface implemented by exactly one type, resolved by type assertion.
Nothing can say "dharness could do this here, but not in this repository", so the
one piece of work dharness actually hands to the agent cannot appear in the plan.

Two commands make it worse. `sync` only prints and never applies; `init` applies
and then prints `ArchitecturePrompt` unconditionally (`internal/cli/init.go:66`),
so a satisfied step never disappears — a direct §15 violation. The tool meant to
say "what stopped being true" cannot say it about delegated work.

## Scope

### In scope

1. **Step contract.** `Delegated(p project.Project) (why string, ok bool)`
   becomes a method on every `Step`, answered during Prepare, executing nothing —
   the same purity guarantee as `Satisfied`. `Apply` runs only when `ok == false`.
2. **Command merge.** `init` and `sync` become one command, `sync`.
   `internal/cli/init.go` is deleted; `RunSync` absorbs its behaviour;
   `internal/app/app.go` and `internal/app/help.go` follow.
3. **`ArchitecturePrompt` becomes a real step** — satisfied when
   `.dharness/fallow.jsonc` already declares `boundaries`, delegated otherwise
   (Intención: no detection possible). It stops printing unconditionally (§15) and
   starts appearing in the plan.
4. **`hookInstallStep.Satisfied` stops returning `true` when no hook manager
   answers.** It becomes an open decision handed to the agent.
5. **The documentation this change makes false is corrected in the same change**,
   in both artifacts. See "Documentation" below.

Item 5 is the documentation update, specified in its own section below. It is in
scope, not a separate pass.

### Out of scope — each a named follow-up change, not dropped work

Listed in the order they should be sequenced.

1. `writer-undo-completeness`: `Writer.Undo` leaks the `.dharness/` directory and
   the `.gitignore` that `EnsureDir` writes outside the `Writer`. **First**,
   ahead of the version step, because this change's own report leans on a
   rollback guarantee that is knowingly broader than what `Writer.Undo`
   delivers: item 1 makes a real `Apply` error the sole surviving `Undo` path,
   and that path still does not remove directories created by `os.MkdirAll`
   inside `Write`, nor the `.gitignore` written outside the `Writer` by
   `project.Project.EnsureDir` (`internal/project/evidence.go:75-88`).
2. `owned-files-version-step`: version stamping plus the `fallow.jsonc` merge
   algorithm. Needs its own design pass; getting it wrong silently deletes
   agent-authored `boundaries`. Depends on item 1.
3. `install-step-declared-deps`: `installStep` asks `node_modules` instead of the
   dependencies declared in `package.json`. Local to `installStep`; reuses the
   parsing already in `internal/project/detect.go:160-163`.

**Constraint on the merged command's report text, arising from follow-up 1.**
Until `writer-undo-completeness` lands, the rollback wording in `sync`'s output
MUST NOT claim that everything written was undone. It states what was undone and
stays silent on the rest. This hedge is deliberate and has an expiry: it exists
because `Writer.Undo` under-delivers today, and the wording should be tightened
in the same change that fixes it — not rediscovered as an unexplained vagueness.

## Capabilities

### New capabilities

- `project-sync`: one command that derives the plan from the repository on every
  run, applies dharness's own steps, and hands the rest to the agent.
- `step-delegation`: the per-project recipient decision — who can do this step in
  *this* repository — answered without executing anything.

### Modified capabilities

None. `openspec/specs/` does not exist yet in this repository.

## Behavioural contract of the merged command

Derived from the specification, not invented.

| Situation | `sync` does | Stops? |
|---|---|---|
| No git repository, or no JS project | No plan is possible; stops before writing anything | **Yes** |
| A step dharness was executing fails | Undoes what was written, reports everything as failed, no partial successes | **Yes** |
| `.fallowrc.json` already exists with its own config | Writes `.dharness/fallow.jsonc` anyway and hands the agent the missing `extends` line | No |
| The project file is code, not data (`doctor.config.ts`) | Does not merge; describes the change and continues | No |
| No hook manager answers | Records the gate as an open decision and continues | No |
| react-doctor skill | Always delegated: its installer writes five things and no flag asks for one | No |
| Architecture (`boundaries`) not declared | Delegated as Intención — instructions and no options | No |

Three phases, two stopping points (Figure 1). Prepare writes nothing, so it
cannot fail. Re-running repeats nothing: a step already done is not in the plan;
a step someone undid by hand reappears on its own.

## Documentation — in this change, not a later pass

In this repository the design document is the specification and travels with the
code. Leaving "Qué existe hoy" saying Fusión does not exist, after this change
makes it exist, is the drift that has already bitten this project once: the
published artifact of `design-principles.md` still carries an old §03 that the
`.md` amended twice. A document ahead of the code in one place and behind it in
another, with only one of the two stated, is worse than either alone.

### What this change makes false

| Location | What becomes false | What it becomes |
|---|---|---|
| "Qué existe hoy", Fusión row, Estado | "**No existe.** Es el hueco por el que el comando abortaba: no había prompt que entregar, así que la única salida era fallar" | Exists **for the `extends` case only**. The row's "Dónde se usa" cell names two cases and this change delivers one, so the row is qualified, not flipped |
| "Qué existe hoy", Fusión row, Dónde se usa | Nothing, but the `doctor.config.ts` case it names is **not** delivered here | Verify at apply time whether `doctorConfigStep` handles a code-not-data config; if it does not, the row must say so instead of implying Fusión is complete |
| "Qué existe hoy", Conducción row, Estado | "Existe como motivo y descripción, no como prompt" — **conditionally** | Depends on the report shape `sdd-design` fixes. Item 1 pairs `Delegated`'s `why` with `Describe(p)`; whether that is "a prompt" is decided by the report, so re-check and write what is true. Do not pre-assert it here |
| "Qué existe hoy", Intención row, Estado | "El único prompt real que hay" | False regardless: Fusión becomes a second real prompt |
| Header, line 11 | "Cinco comandos sobre tres librerías" — true today (`init`, `sync`, `check`, `mutate`, `version`) | Four. The list on line 16 already reads `sync · check · mutate · version` and needs no change |

Verified against `internal/app/app.go:31-48`, which dispatches all five today.

### Two artifacts, not one

Standing convention: every change to a design document goes to the file **and**
to its published artifact in the same pass.

- `docs/flujo-implementado.md`
- the published artifact:
  `https://claude.ai/code/artifact/6033beba-ca70-4163-92fd-97de7ed8663e`

The republish is not a code diff and does not count toward the review budget, but
it is not optional and MUST appear as its own task. Updating it requires passing
that exact URL — without it a new artifact is minted and the link is lost.

## Principles that accept this change

- **§15** — one command, re-run, says what stopped being true. Item 3 is the
  literal case §15 came from: a step that printed always, contradicting the rule
  that satisfied steps disappear.
- **§20** — a block is only the irrecoverable. Item 1 stops conflating "cannot
  run here" with "failed after writing bytes". Only a real `Apply` error still
  triggers `Writer.Undo`.
- **§21** — the client is the agent. `Delegated`'s `why` plus the already
  project-aware `Describe(p)` are the prompt. No new type, no step aimed at
  someone dharness cannot see.

The rule in `CLAUDE.md` holds: no CLI exposes a command for "who can do this step
in this repository", so the work belongs here, and the part needing judgment goes
to the agent as a prompt.

## The test reversal, stated explicitly

`TestGateStepIsSatisfiedWhenNoManagerAnswers`
(`internal/setup/setup_test.go:311`) defends the current behaviour on the grounds
that the step "must not block everything else in the plan". That concern is
removed by the new contract: a delegated step blocks nothing — it is recorded and
the run continues. The decision table in `docs/flujo-implementado.md` already
specifies the open-decision behaviour. The test is reversed deliberately, with
its rationale replaced, never silently deleted.

## Three honest statements

1. After this change `docs/flujo-implementado.md` is **still ahead of the code**
   on the owned-files-version step (Figure 2's `P5B` row, Figure 3's amber merge
   branch, and the "Archivos propios al día" row). That gap is deliberate and
   deferred, not overlooked.
2. The document is ahead of the code in a **second** place, found during the
   spec phase and verified in `internal/project/discover.go:61-67`: the left-hand
   red block of Figure 1, "no hay repositorio git y un proyecto JS → bloqueo
   real", does not exist. `Discover` returns `At(dir, dir), nil` outside a
   repository rather than erroring, and its comment states the policy that
   detection is not where that is raised. `RunSync` does not raise it either —
   with `Source == Root == dir`, `HasSource()` is true and the command reports a
   plan for an ordinary directory. Today's behaviour is not "fails outside a
   repository"; it is "answers as though any directory were one", which is worse
   than failing. Unlike statement 1, this gap is **closed by this change**: the
   stop is in scope because Figure 1 is the specification. `sdd-design` decides
   where it is enforced and whether "no JS project" is the same stop or the
   existing graceful `noSourceMessage` path — the figure draws them as one box,
   the code treats them as two states.
3. `internal/cli/init.go` has **no tests today**. Merging untested behaviour into
   the tested `RunSync` (`internal/cli/sync_test.go`, 4 tests) raises real risk.
   The tests for that behaviour are part of this change's work, not an
   afterthought. `TestSyncWritesNothing` becomes categorically false and must be
   replaced, not patched.

## Affected areas

| Area | Impact | Description |
|---|---|---|
| `internal/setup/setup.go` | Modified | `Step` gains `Delegated(p)`; `Delegated` interface and type assertion removed; `Apply`/`Pending` loops updated |
| `internal/setup/steps.go` | Modified | `Delegated(p)` on all step types; real logic in `extendsStep` and `hookInstallStep`; new architecture step |
| `internal/setup/files.go` | Modified | `wireFallowExtends`/`wireLefthookExtends` stop returning hard errors |
| `internal/setup/prompt.go` | Modified | `ArchitecturePrompt` becomes step text |
| `internal/cli/sync.go` | Modified | Absorbs `RunInit`'s body |
| `internal/cli/init.go` | Removed | 68 lines |
| `internal/app/app.go`, `internal/app/help.go` | Modified | Dispatch, `UnknownCommandError`, COMMANDS section |
| `internal/cli/sync_test.go`, `internal/setup/setup_test.go` | Modified | Central invariant reverses; new contract tests |

## Changed-line forecast

~370–420 changed lines: ~350–400 of Go and tests, plus ~10–20 for the
`docs/flujo-implementado.md` correction. The artifact republish adds none — it is
not a diff.

**400-line budget risk: High.** Raised from Medium when the documentation entered
scope: the top of the range now crosses the budget instead of approaching it. The
estimate is a forecast from reading rather than a measured diff, and strict TDD
with mutation coverage on the reversed `default` branch could run higher still.

Chain seam if the real diff exceeds 400 lines:

- **Slice 1** — step contract (item 1) plus the `extendsStep` delegation fix.
- **Slice 2** — command merge (item 2) plus the architecture step (item 3) plus
  the `hookInstallStep` fix (item 4).

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Diff exceeds the 400-line budget | High | Split at the declared seam; the documentation correction rides with slice 2 |
| Artifact republish forgotten, or republished without the existing URL | Medium | Its own task, carrying the exact URL; a new URL loses the link |
| Untested `RunInit` behaviour lost in the merge | Medium | Write the missing `RunSync` tests before merging behaviour; TDD is strict here |
| `Satisfied` substring check couples to `fallow.jsonc`'s literal text | Low | Follows the existing `extendsWired` precedent; a JSONC parser is a dependency and the product is stdlib-only |
| Reversed test read as an accident | Low | Reversal is stated in this proposal and must be justified in the replacement test |

## Rollback plan

The change is one branch with no persisted state, no migration, and no on-disk
format change. Revert the merge commit: `init` and `sync` return, `Delegated`
returns to a type assertion. Nothing written by a previous version needs
undoing — the plan is re-derived from the repository on every run, so an older
binary simply re-derives the older plan.

## Dependencies

None. The product stays stdlib-only; no dependency is added, including a JSONC
parser. `go test ./...`, `go vet ./...`, `gofmt -l .` and
`go run ./tools/mutationstaged` are the whole verification surface.

## Success criteria

- [ ] `dharness init` no longer exists; `dharness sync` applies, reports, and
      delegates in one run.
- [ ] A repository with a pre-existing, non-empty `.fallowrc.json` completes
      `sync` with no rollback, and the missing `extends` line appears as a
      delegated step.
- [ ] With `boundaries` already declared in `.dharness/fallow.jsonc`, the
      architecture step does not appear in the output.
- [ ] With no hook manager present, the gate appears as an open decision and the
      remaining steps still run.
- [ ] `Apply` is never called for a step whose `Delegated(p)` returns `ok == true`.
- [ ] Every row named in "What this change makes false" is corrected in
      `docs/flujo-implementado.md`, and the same content is republished to the
      existing artifact URL.
- [ ] `go test ./...`, `go vet ./...`, `gofmt -l .` and
      `go run ./tools/mutationstaged` all clean.
