# Spec: unify-init-and-sync

Source of record for behaviour: `docs/flujo-implementado.md` ("Adopción y
actualización — dentro de `dharness sync`", "Qué es el plan", "3. Entregar",
"Qué hace reaparecer un paso", and the decision table "Situación / Qué hace
sync / ¿Detiene?"). Principles cited by number: `docs/design-principles.md`
§15, §20, §21. Proposal: Engram `sdd/unify-init-and-sync/proposal` (#7980),
`openspec/changes/unify-init-and-sync/proposal.md`.

`openspec/specs/` is empty. Both capabilities below are new; every requirement
is `ADDED`.

---

## ADDED Requirements — Capability: `project-sync`

One command that derives the plan from the repository on every run, applies
dharness's own steps, and hands the rest to the agent.

### Requirement: The command is named `sync`, and only `sync`

`init` MUST NOT exist as a command. The single merged command MUST be
dispatched as `sync`. An unknown command's error message MUST list only the
commands that exist and MUST NOT name `init`.

#### Scenario: `dharness init` is unknown

- **GIVEN** the dispatcher in `internal/app/app.go`
- **WHEN** it is invoked with `init` as the first argument
- **THEN** it returns an `UnknownCommandError` (or equivalent), not a call into
  a `RunInit`-shaped function

#### Scenario: the unknown-command error does not name `init`

- **GIVEN** a command that matches no case in the dispatcher, e.g. `bogus`
- **WHEN** `RunArgs` builds the error message
- **THEN** the message MUST enumerate exactly the commands that exist (`sync`,
  `check`, `mutate`, `version`, plus `help`) and MUST NOT contain the
  substring `init`

#### Scenario: `dharness sync` performs what `init` used to perform

- **GIVEN** a repository with pending steps dharness can apply
- **WHEN** `dharness sync` runs
- **THEN** it applies every pending step whose `Delegated(p)` returns
  `ok == false`, in the order `setup.Plan()` declares, before reporting
  anything

### Requirement: Prepare executes nothing and therefore cannot fail

Deriving the plan MUST be a pure read of the repository. No step's
`Satisfied` or `Delegated` method may write a file, run a process, or install
a package. Preparing the plan MUST NOT return an error caused by any step's
own state; the only errors preparation can surface are ones that predate the
plan entirely (no git repository, no JS project — see the next requirement).

#### Scenario: deriving the plan writes nothing

- **GIVEN** any project.Project value, adopted or not
- **WHEN** `setup.Pending(p)` is called, which in turn calls `Satisfied(p)`
  and `Delegated(p)` for every step in `setup.Plan()`
- **THEN** the repository's working tree is byte-for-byte unchanged
  afterwards, and `Pending` returns a `[]Step` without invoking `Apply` on
  any of them

#### Scenario: `Delegated(p)` never calls `Apply`

- **GIVEN** any `Step` implementation
- **WHEN** its `Delegated(p project.Project) (why string, ok bool)` method
  runs
- **THEN** it MUST NOT call the step's own `Apply`, MUST NOT run an external
  process, and MUST NOT write to disk

### Requirement: Exactly two stopping conditions, and no others

`sync` MUST stop before doing any work when there is no git repository or no
JS project to adopt. `sync` MUST stop and undo what it wrote when a step
dharness was executing fails inside `Apply`. No other situation MUST stop the
command; every other situation MUST end with that step recorded in the plan
(applied, or delegated) and the run continuing to the next step.

#### Scenario: no git repository, or no JS project — stop before writing

- **GIVEN** a directory that is not inside a git repository, or is inside one
  with no JS project to adopt (no lockfile found)
- **WHEN** `dharness sync` runs
- **THEN** it reports that no plan is possible and returns without calling
  `setup.Apply` and without creating or modifying any file

#### Scenario: a step dharness is executing fails — full rollback, no partial success

- **GIVEN** a plan with at least two steps dharness can apply, the first of
  which succeeds and writes a file
- **WHEN** the second step's `Apply` returns an error
- **THEN** `sync` undoes everything written in this run (via `Writer.Undo`),
  reports every step attempted in this `Apply` pass as failed — including the
  one that had already succeeded — and MUST NOT report any step as having
  partially or fully succeeded

#### Scenario: every other situation continues and ends in the plan

- **GIVEN** any situation from the decision table that is not "no git
  repository or no JS project" and not "a step dharness was executing fails"
  — e.g. `.fallowrc.json` already has content, `doctor.config.ts` is code not
  data, no hook manager answers, the react-doctor skill, `boundaries` not
  declared
- **WHEN** `dharness sync` runs
- **THEN** the run does not stop: the step appears in the plan (delegated,
  with its reason), and every subsequent step in `setup.Plan()` is still
  evaluated and, where applicable, applied

### Requirement: Re-running repeats nothing; state is derived, never recorded

The plan MUST be derived from the repository on every invocation. No file or
record of "which steps ran" MUST be persisted or consulted to decide what is
pending. A step already satisfied MUST be absent from the next run's plan. A
step whose result is undone by hand (a written file deleted, a config edited
back) MUST reappear on the next run without any action to "reset" state.

#### Scenario: a satisfied step disappears from the plan

- **GIVEN** a step is satisfied after a prior `sync` run
- **WHEN** `sync` runs again immediately, nothing else having changed
- **THEN** that step is absent from `setup.Pending(p)` and from the printed
  report

#### Scenario: undoing a step by hand makes it reappear

- **GIVEN** a step was satisfied by a previous `sync` run
- **WHEN** the file or configuration that satisfied it is deleted or reverted
  outside of dharness, and `sync` runs again
- **THEN** `setup.Pending(p)` includes that step again, with no prior state
  to reconcile — `Satisfied(p)` alone decides it

#### Scenario: no on-disk record of run history exists

- **GIVEN** the `setup` package's public surface (`Plan`, `Pending`, `Apply`)
- **WHEN** it is asked whether a step is outstanding
- **THEN** the answer comes only from reading the current repository state
  (`Satisfied(p)`, `Delegated(p)`), never from a file that records which
  steps a previous run performed

### Requirement: The rollback wording does not overclaim, pending `writer-undo-completeness`

Until the follow-up change `writer-undo-completeness` lands, `sync`'s output
after a rollback MUST NOT state or imply that everything written during the
run was undone. It MUST state what was undone and MUST stay silent about
anything `Writer.Undo` does not cover (directories created by `os.MkdirAll`
inside `Write`, and `.gitignore` written by `project.Project.EnsureDir`
outside the `Writer`).

#### Scenario: rollback report avoids the stronger, false claim

- **GIVEN** a step failure that triggers `Writer.Undo`, in a run where
  `ownedFilesStep` had already run `EnsureDir` (which writes
  `.dharness/.gitignore` outside the `Writer`)
- **WHEN** `sync` prints its rollback report
- **THEN** the report MUST NOT contain a claim equivalent to "everything
  written was undone" or "the repository was fully restored"; it MUST
  describe only what `Writer.Undo` actually reverted

---

## ADDED Requirements — Capability: `step-delegation`

The per-project recipient decision — who can do this step in *this*
repository — answered without executing anything.

### Requirement: `Delegated` is a method every `Step` answers, not a type assertion

Every `Step` MUST expose `Delegated(p project.Project) (why string, ok bool)`
as part of the `Step` interface itself. The recipient of a step MUST be
decided per project, at Prepare time, not by a compile-time type assertion
against a second interface. `Apply` MUST run only when `Delegated(p)` returns
`ok == false`.

#### Scenario: `Delegated` is on the `Step` interface

- **GIVEN** the `setup.Step` interface
- **WHEN** any type is used as a `Step` (passed to `setup.Plan()`)
- **THEN** it MUST implement `Delegated(p project.Project) (string, bool)` as
  part of satisfying `Step`; no separate `Delegated` interface with a type
  assertion is used to make this decision

#### Scenario: `Apply` is never called when `Delegated(p)` is true

- **GIVEN** any project and any step in `setup.Plan()`
- **WHEN** `setup.Apply(p, stdout)` runs
- **THEN** for every step where `Delegated(p)` returns `ok == true`, that
  step's `Apply` method is never invoked

### Requirement: a step whose project file already has content is delegated, not an error

When the project's own `.fallowrc.json` or `lefthook.yml` already exists with
content, `extendsStep` MUST treat that as delegated work, not as an `Apply`
failure. Writing dharness's own file (`.dharness/fallow.jsonc` or
`.dharness/lefthook.yml`) MUST still happen; only the missing `extends` line
in the project's own file is handed to the agent. This MUST NOT trigger
`Writer.Undo`.

#### Scenario: `.fallowrc.json` already exists with its own configuration

- **GIVEN** a project whose `.fallowrc.json` exists and has content that is
  not the dharness `extends` reference
- **WHEN** `sync` runs
- **THEN** `extendsStep.Delegated(p)` returns `ok == true` with a reason
  naming the missing `extends` line, `.dharness/fallow.jsonc` is written
  (fallow-side, unaffected by this project file), and the run does not stop,
  undo, or report a failure

#### Scenario: `lefthook.yml` already exists with its own configuration

- **GIVEN** a project whose `lefthook.yml` exists and has content that is
  not the dharness `extends` reference, and lefthook is the answering hook
  manager
- **WHEN** `sync` runs
- **THEN** `extendsStep.Delegated(p)` returns `ok == true` for the lefthook
  half of the step, naming the missing `extends` line, and no rollback
  occurs

#### Scenario: the project file does not exist yet

- **GIVEN** a project whose `.fallowrc.json` (or `lefthook.yml`) does not
  exist
- **WHEN** `sync` runs
- **THEN** `extendsStep.Delegated(p)` returns `ok == false` for that file,
  and `Apply` writes the file itself with the `extends` reference, as it
  does today

### Requirement: no hook manager answering is an open decision, not silent success

`hookInstallStep.Satisfied(p)` MUST NOT return `true` when no hook manager
answers. `hookInstallStep.Delegated(p)` MUST return `ok == true` in that
case, with a reason naming that choosing a hook manager is not a default
dharness picks. The run MUST continue past it without stopping.

#### Scenario: no hook manager responds

- **GIVEN** a project with no `lefthook.yml`/`.lefthook.yml` variant, no
  `.husky` directory, and no local `lefthook` binary
- **WHEN** `sync` derives the plan
- **THEN** `hookInstallStep.Satisfied(p)` returns `false`,
  `hookInstallStep.Delegated(p)` returns `ok == true` with a reason, the step
  appears in the report as an open decision, and every other step in
  `setup.Plan()` still runs

#### Scenario: `TestGateStepIsSatisfiedWhenNoManagerAnswers` is reversed, not deleted

- **GIVEN** the existing test at `internal/setup/setup_test.go:311` asserting
  `Satisfied == true` when no manager answers
- **WHEN** this change lands
- **THEN** the test is replaced by one asserting `Satisfied == false` and
  `Delegated(p)` returning `ok == true`, with a comment stating the new
  rationale (a delegated step blocks nothing, so satisfying it artificially
  is no longer needed to keep the plan moving)

### Requirement: the architecture step is satisfied by a declared `boundaries` block, and disappears once satisfied (§15)

A step for the architecture prompt MUST exist in `setup.Plan()`. It MUST be
satisfied when `.dharness/fallow.jsonc` already contains `boundaries`
declared, and delegated (Intención — instructions, no options) otherwise. It
MUST stop being printed unconditionally: once satisfied, it MUST be absent
from both the `Pending` report and the applied/delegated report, in every
run, not just the run in which it became satisfied.

#### Scenario: `boundaries` not yet declared

- **GIVEN** `.dharness/fallow.jsonc` exists without a `boundaries` block (or
  does not exist)
- **WHEN** `sync` runs
- **THEN** the architecture step appears in the plan, `Delegated(p)` returns
  `ok == true`, and its prompt text carries instructions with no options
  (matching `setup.ArchitecturePrompt`'s existing content: what to find out,
  where to write it, how to check it)

#### Scenario: `boundaries` already declared

- **GIVEN** `.dharness/fallow.jsonc` contains the substring `boundaries`
  inside a declared block
- **WHEN** `sync` runs
- **THEN** the architecture step's `Satisfied(p)` returns `true`, and the
  step is absent from the plan and from the report — no unconditional print,
  matching §15

#### Scenario: re-running after the agent declares boundaries makes the step disappear

- **GIVEN** a first `sync` run where the architecture step was delegated and
  printed
- **WHEN** the agent writes `boundaries` into `.dharness/fallow.jsonc` and
  `sync` runs again
- **THEN** the architecture step no longer appears in the second run's
  output — it does not require any acknowledgement or record beyond the file
  itself

---

## Explicit non-requirements (out of scope for this spec)

These are named in the proposal as separate follow-up changes and are
deliberately **not** specified here:

- The owned-files-version step and the `fallow.jsonc` merge algorithm
  (`owned-files-version-step`).
- `installStep` reading declared dependencies from `package.json` instead of
  `node_modules` (`install-step-declared-deps`).
- `Writer.Undo` completeness — removing directories created by
  `os.MkdirAll` inside `Write`, and covering `.gitignore` written by
  `project.Project.EnsureDir` outside the `Writer` (`writer-undo-completeness`).

## Notes on testability

Every scenario above is phrased so it maps directly to a Go test:
`Satisfied(p)` / `Delegated(p)` on a constructed `project.Project` fixture,
inspecting return values and the file system before/after `setup.Apply` or
`cli.RunSync`, with no assertion that reads report prose to decide pass/fail
— per `openspec/config.yaml`'s `specs` rule and design principles §11/§17
(the verdict comes from exit codes and JSON, never from prose or a model;
applied here to tests as: assert on `bool`/`error`/file-existence, not on
substrings of a report meant for a human or agent reader, except where the
requirement is specifically about that report's wording, as in the rollback
wording requirement above).
