# Exploration: unify-init-and-sync

Source of record: Engram observation #7979, topic key `sdd/unify-init-and-sync/explore`.
This file is the OpenSpec half of the hybrid artifact store. The exploring agent
had no write access; the orchestrator materialised it and corrected one factual
claim (noted inline).

Specification for this change: `docs/flujo-implementado.md`, as rewritten on
2026-08-10, plus `docs/design-principles.md` §15, §20 and §21.

## Current state

Two commands exist (`internal/cli/init.go`, `internal/cli/sync.go`), both
dispatched from `internal/app/app.go`.

- `RunSync` reads `setup.Pending(p)` and only prints. It never calls
  `setup.Apply`.
- `RunInit` calls `setup.Apply`, re-derives `setup.Pending(p)` to print what is
  left, and ends by printing `setup.ArchitecturePrompt(p)` **unconditionally**
  (`internal/cli/init.go:66`).

`Step` (`internal/setup/setup.go:23`) is `ID / Describe / Satisfied / Apply`.
`Delegated` (`setup.go:43`) is a second interface adding `Why() string`,
satisfied by exactly one type, `agentSkillStep`. Delegation is therefore a
compile-time fact, resolved by `step.(setup.Delegated)` type assertion in both
commands, and never decided per project.

`setup.Apply` (`setup.go:81`) skips delegated types, applies the rest in order,
and on the first `Apply` error calls `writer.Undo()` and returns.

### A live bug this change fixes as a side effect

`extendsStep.Apply` calls `wireFallowExtends` / `wireLefthookExtends`
(`internal/setup/files.go:131-155`). When the project's own `.fallowrc.json` or
`lefthook.yml` already exists with content, those functions return a hard
`error` rather than writing. That error is indistinguishable from any other
`Apply` failure, so it triggers `writer.Undo()` and rolls back the entire run,
including steps that had already succeeded.

This is the exact scenario `docs/design-principles.md` §20 was written from.
(Correction to the exploring agent's wording: §20 does not claim the bug is
fixed — its `Salió de` clause records the case that produced the principle, per
that document's convention. The principle is new; the bug is still live. Both
statements are true and neither contradicts the other.)

`extendsStep` is therefore the load-bearing example of "who can do it" being a
per-project fact: file absent means dharness writes it, file present with
content means only a human-readable line can be handed over.

### A second defect found while reading

`TestGateStepIsSatisfiedWhenNoManagerAnswers` (`internal/setup/setup_test.go:311`)
pins `hookInstallStep.Satisfied` returning `true` when no hook manager answers
(`steps.go:277-281`, `default: return true`), with a comment defending it. This
contradicts the decision table in `docs/flujo-implementado.md`, which says the
case must be recorded as an open decision, not silently satisfied.

Verified by the orchestrator: the test exists as described and does defend the
behaviour.

### Other observations

- `.dharness/.gitignore` is written by `project.Project.EnsureDir`
  (`internal/project/evidence.go:75-88`) with a direct `os.WriteFile`, bypassing
  `Writer`. `Writer.Undo` (`internal/setup/writer.go:86-106`) never sees it and
  never removes directories created by `os.MkdirAll` inside `Write`.
- `installStep.Satisfied` / `missing()` ask `node_modules/<pkg>` via `installed()`
  (`files.go:66-72`), never the declared dependencies in `package.json`. A fresh
  clone has no `node_modules`; under Yarn PnP it never appears.
  `internal/project/detect.go:160-163` already parses the declared dependencies.
- No version-stamping mechanism exists in `internal/setup` or `internal/project`.
  `ownedFilesStep.Satisfied` only checks the three owned files' existence, so
  once they exist the step is permanently satisfied and `Apply` never runs again.
  The "merge, do not overwrite" requirement for `fallow.jsonc` is therefore not
  reachable today; it becomes load-bearing only once a version check exists.
- `internal/cli/init.go` has **no dedicated test file**. Verified by the
  orchestrator. All of `RunInit`'s behaviour is currently unverified.

## Q1 — The step contract

Minimal shape: replace the static `Delegated` type assertion with a per-project
decision the step itself makes, evaluated without executing anything.

```go
type Step interface {
    ID() string
    Describe(p project.Project) string
    Satisfied(p project.Project) bool
    Delegated(p project.Project) (why string, ok bool) // pure, like Satisfied
    Apply(p project.Project, w *Writer) error          // only when ok == false
}
```

This matches Figure 2 of `docs/flujo-implementado.md`: "por cada paso, ¿quién
puede hacerlo?" is answered during Prepare, which writes nothing and therefore
cannot fail. It makes the recipient a property of the plan rather than something
discovered mid-`Apply` through an error — which is what `extendsStep` does today
and what causes the rollback bug.

What it unlocks:

- **§20** — a step that cannot run stops being conflated with a step that failed
  destructively. Only a real `Apply` error still triggers `Writer.Undo`.
- **§21** — `Delegated`'s `why` plus the already project-aware `Describe(p)` are
  the prompt. No new type is required.
- **The three prompt kinds** map onto *why* `Delegated` returns true, not onto a
  new field. Fusión is `extendsStep`; Conducción is `agentSkillStep`; Intención
  is the future boundaries step. The contract stays uniform.

Alternative considered and rejected: a sentinel error type
(`*setup.NeedsAgent{Why string}`) returned from `Apply`. Smaller diff, but it
breaks the Prepare/Apply purity split — discovering delegation would require
calling something `Apply`-shaped, which the specification forbids.

## Q2 — Cost of the merge

| Area | Estimate | Note |
|---|---|---|
| `internal/cli/sync.go` absorbs `RunInit`'s body | ~70–90 | Report must show applied, delegated, and the architecture handoff |
| `internal/cli/init.go` deleted | 68 removed | Whole file |
| `internal/app/app.go` dispatch + `UnknownCommandError` | ~5–10 | One case, one string |
| `internal/app/help.go` COMMANDS section | ~15–20 | Two entries collapse into one |
| `internal/cli/sync_test.go` | ~60–100 | `TestSyncWritesNothing` becomes categorically false and must be replaced; the other three need new assertions |
| `internal/setup/setup.go` contract + loop | ~20–30 | |
| `internal/setup/steps.go` — `Delegated(p)` on 7 types | ~40–70 | Five trivial; `extendsStep` and `hookInstallStep` carry real logic |
| `internal/setup/files.go` wiring functions | ~20–30 | Behaviour change, not just signature |
| New tests for the delegation contract | ~60–100 | Strict TDD, RED→GREEN→MUTATE |

**Q1 + Q2 alone: ~290–370 changed lines**, against a 400-line review budget.

## Q3 — Architecture prompt as a step

Recommended **in scope**. `Satisfied(p)` checks whether `.dharness/fallow.jsonc`
contains the literal substring `boundaries`, following the precedent of
`extendsWired` (`files.go:106-109`), which deliberately looks for a reference
rather than parsing a foreign file. No JSONC parser exists in this repo and
adding one is not justified by a single check. `Delegated(p)` always returns
true when unsatisfied — Intención, where no detection is possible.

Reason to include now: this step is why `RunInit` violates §15 today (the prompt
prints unconditionally) and why the old `sync` never mentioned it at all. Both
are symptoms of the problem this change exists to fix. Estimate: ~30–50 lines.

## Q4 — Owned-files-version step

**Deferred to a separate chained change.** No version infrastructure exists.
Where the marker lives differs per file, and `fallow.jsonc` is only partially
dharness-owned once the agent has written `boundaries` into it, so the merge
cannot be "regenerate and reappend" — it needs an explicit algorithm and its own
design pass. Getting it wrong silently deletes agent-authored architecture.
Folding it in would also push the total past 400 lines with no slack.

Depends on Q1: the version step is naturally a `Step`, and whether it can itself
be delegated ("dharness upgraded but could not safely merge `fallow.jsonc`") is a
real question for the new contract.

## Q5 — Scope boundary

| Defect | Separable | Why |
|---|---|---|
| `installStep` asks `node_modules` instead of declared deps | **Yes** | Local to `installStep` / `installed()` / `missing()`; reuses the parsing already in `detect.go:160-163`; touches nothing this change touches |
| `Writer.Undo` leaks the directory and `.gitignore` | **Yes** | Pure `Writer` / `EnsureDir` plumbing; the affected rollback path predates this change. Worth prioritising, since it undermines the rollback guarantee this change's report relies on |
| `hookInstallStep` silently satisfied when no manager answers | **No** | Second driving case for the very mechanism Q1 introduces. Once Q1 lands the fix is ~10–15 lines: flip the `default` branch and add a `Delegated` reason. The decision table in `docs/flujo-implementado.md` already specifies this behaviour |

## Approaches

1. **Everything in one change** — coherent story, but over budget and mixes a
   well-precedented merge with genuinely novel merge/version design. Rejected.
2. **Q1 + Q2 + Q3 + the `hookInstallStep` fix; defer Q4 and the other two Q5
   defects** — ~350–400 lines including tests. Ships a coherent "one command,
   re-run, says what stopped being true" story and fixes the two defects that are
   direct symptoms of the mechanism being introduced. **Recommended.**
3. **Q1 + Q2 only** — smallest slice (~290–370), but ships the mechanism without
   applying it to the second case it was designed for, and leaves the §15
   violation unresolved.

## Recommendation

Approach 2. Q1's contract change is justified by having at least two real
per-project delegation cases: `extendsStep`, already required by the merge since
it causes a live rollback bug, and `hookInstallStep`, the second documented case.
Q3 is nearly free once Q1 exists and closes the §15 violation this change's own
premise names.

Flag `400-line budget risk: Medium` to `sdd-tasks`. Natural chain seam if the
real diff exceeds budget: slice 1 is the Q1 contract plus the `extendsStep` fix;
slice 2 is the command merge plus Q3 plus the `hookInstallStep` fix.

## Risks

- The line estimate is a forecast from reading, not a measured diff. Strict TDD
  with mutation coverage on the reversed `default` branch could run higher.
- Reversing `TestGateStepIsSatisfiedWhenNoManagerAnswers` is a deliberate design
  reversal. Its comment defends the current behaviour on the grounds that the
  step "must not block everything else in the plan" — a concern the new contract
  removes, because a delegated step blocks nothing. The reversal must be explicit,
  not a silent deletion.
- After this change `docs/flujo-implementado.md` will still be ahead of the code
  on Q4. The proposal must say so rather than leave it implicit.

## Ready for proposal

Yes. The open question — whether the `hookInstallStep` fix is in scope — is
resolved by the specification itself: the decision table in
`docs/flujo-implementado.md` already requires the open-decision behaviour.
