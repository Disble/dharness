# Spec: setup

Standing behaviour of `dharness sync` and the step contract behind it. This is
the document later changes delta against — not a change proposal. It entered
the repository as the delta spec of `unify-init-and-sync`
(`openspec/changes/archive/2026-08-11-unify-init-and-sync/spec.md`), archived
on 2026-08-11 and released as v1.0.0, and its requirements are stated here as
what holds rather than as what that change added.

Source of record for behaviour: `docs/flujo-implementado.md` ("Adopción y
actualización — dentro de `dharness sync`", "Qué es el plan", "3. Entregar",
"Qué hace reaparecer un paso", and the decision table "Situación / Qué hace
sync / ¿Detiene?"). Principles are cited by number from
`docs/design-principles.md`.

> **Known limit, measured after this shipped.** fallow's `extends` replaces a
> key rather than merging it, so the architecture written into
> `.dharness/fallow.jsonc` is silently discarded the moment a project declares
> its own `boundaries`. `boundariesOwnerStep` reports that case as of v1.0.2;
> the requirements below describe the reference mechanism, not a guarantee that
> it is the one in effect.

---

## Capability: `project-sync`

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
  — e.g. `.fallowrc.json` already has content, the project's ESLint config is
  a `.ts`/`.mts`/`.cts` file and so is code rather than data, no hook manager
  answers, the react-doctor skill, `boundaries` not declared
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

## Capability: `step-delegation`

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

### Requirement: a step whose target project file is already understood may edit it directly — a narrower exception than delegate-on-presence

`eslintExtendsStep` MUST NOT follow the delegate-on-presence pattern the
existing requirement above fixes for `.fallowrc.json` and `lefthook.yml`. Where
`eslint.config.js`/`.mjs`/`.cjs` already exists and satisfies every
condition of `eslint-config-splice`'s write path (array-literal or
recognised `defineConfig` default export, no `ERROR` node, no malformed
marker pair), `Delegated(p)` MUST return `ok == false` and `Apply` splices
directly. `Delegated(p)` MUST return `ok == true` only for the cases
`eslint-config-splice` names as refusals — a `.ts`/`.mts`/`.cts` config, an
unrecognised call expression, an `ERROR` node, or a malformed marker pair.
This is scoped to `eslintExtendsStep` alone; it does not generalise the
existing requirement above, and it does not change delegate-on-presence
for any other step.

#### Scenario: a splice-eligible config is edited directly, not delegated

- **GIVEN** `eslint.config.js` with an array-literal default export and no
  `ERROR` node
- **WHEN** `sync` runs
- **THEN** `eslintExtendsStep.Delegated(p)` returns `ok == false`, and
  `Apply` splices — unlike `extendsStep`'s existing behaviour for
  `.fallowrc.json`, presence of the file does not by itself cause delegation

#### Scenario: an unreadable config still delegates, exactly as the general pattern would predict

- **GIVEN** `eslint.config.js` whose default export is `tseslint.config(...)`
- **WHEN** `sync` runs
- **THEN** `eslintExtendsStep.Delegated(p)` returns `ok == true`, matching
  the outcome delegate-on-presence would have given, even though the
  mechanism that produced it is narrower (understanding, not mere presence)

#### Scenario: the exception does not spread to `.fallowrc.json` or `lefthook.yml`

- **GIVEN** the existing requirement above
- **WHEN** this change ships
- **THEN** `extendsStep`'s behaviour for `.fallowrc.json` and `lefthook.yml`
  is unchanged — presence alone still delegates for those two files, and no
  test for this change asserts otherwise

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

## Capability: `eslint-config-splice`

Parsing an existing `eslint.config.js`, deciding whether the edit is safe,
the marker regions, the refusal matrix, idempotency, and
verify-then-rollback. Everything here concerns `.js`/`.mjs`/`.cjs` configs
only; a `.ts`/`.mts`/`.cts` config is its own requirement below and always
delegates.

### Requirement: `.dharness/eslint.config.js` is the single home for the six rule severities

dharness MUST write the six `dharness/*` rule severities into
`.dharness/eslint.config.js` and nowhere else. No step writes `dharness/*`
severities into `doctor.config.json`, or declares `RulesPackage` under
`plugins` in that file: the step that did was deleted rather than emptied,
because its satisfying condition was exactly that declaration and a step
with no reachable satisfied state is not a step. `.dharness/rules.json`
thresholds are unaffected by this requirement — they are read by the plugin
at rule-execution time, not written as part of a severity map.
`DefaultSeverity`'s existing contract (its barrel-derived `folder-ownership`
default and the recorded first-write-only limit, per the `framework-presets`
spec) is unchanged; this requirement only moves *where* the six severities
are written, not how any of them is derived.

#### Scenario: severities land only in the owned ESLint config

- **GIVEN** a project adopted for the first time under this change
- **WHEN** `sync` writes `.dharness/eslint.config.js`
- **THEN** the six `dharness/*` severities appear there, `doctor.config.json`
  carries neither a `dharness/*` severity nor a `RulesPackage` plugin
  declaration, and `.dharness/rules.json`'s thresholds are unchanged by this
  write

#### Scenario: residue in an already-adopted repository is reported, never removed

- **GIVEN** a repository adopted before this change, whose
  `doctor.config.json` still carries `dharness/*` severities and
  `RulesPackage` from the prior mechanism
- **WHEN** `sync` runs again under this change
- **THEN** dharness does not edit or delete that residue — it cannot
  distinguish its own earlier write from a value the project itself later
  set (§05) — and `sync`'s output names the residue as inert under the
  gate's `--staged` invocation, never silently

### Requirement: no ESLint config at all is written whole, matching the extends-wiring precedent

Where a project has no `eslint.config.js`/`.mjs`/`.cjs`/`.ts`/`.mts`/`.cts`
and no legacy `.eslintrc.*`, `eslintExtendsStep` MUST write a complete
`eslint.config.js` that imports and spreads `.dharness/eslint.config.js`,
following the same write-if-absent shape `wireFallowExtends` and
`wireLefthookExtends` already use for `.fallowrc.json` and `lefthook.yml`.

#### Scenario: a project with no ESLint config gets one written

- **GIVEN** a project with no ESLint configuration file of any kind
- **WHEN** `sync` runs
- **THEN** `eslintExtendsStep.Delegated(p)` returns `ok == false`, and
  `Apply` writes a new `eslint.config.js` that imports
  `.dharness/eslint.config.js` and spreads it into a default-exported array

### Requirement: a `.ts`/`.mts`/`.cts` config always delegates

Where the only ESLint configuration file present has a `.ts`, `.mts`, or
`.cts` extension, dharness MUST delegate, never parse or edit it. This
follows the same reasoning §03's amendment already applies to
`.js`/`.mjs`/`.cjs` Stryker configs: a second grammar plus typed helpers
whose semantics belong to the project.

#### Scenario: a TypeScript ESLint config is left untouched

- **GIVEN** a project whose only ESLint configuration is `eslint.config.ts`
- **WHEN** `sync` runs
- **THEN** `eslintExtendsStep.Delegated(p)` returns `ok == true`, no parse of
  the file is attempted, and no bytes of it are written

### Requirement: a legacy `.eslintrc.*`-only project delegates, matching `legacyLintConfigStep`'s shape

Where a project has `.eslintrc.*` and no flat config
(`eslint.config.js`/`.mjs`/`.cjs`/`.ts`/`.mts`/`.cts`), dharness MUST
delegate in the same shape `legacyLintConfigStep` already uses for the
analogous doctor-config case, naming that migration to flat config is the
project's decision. dharness MUST NOT attempt to compose an ESM spread
against an eslintrc file.

#### Scenario: a project with only `.eslintrc.json` delegates

- **GIVEN** a project whose only ESLint configuration is `.eslintrc.json`,
  no flat config present
- **WHEN** `sync` runs
- **THEN** the step delegates, naming the eslintrc-only shape and that
  migrating to a flat config is the project's decision, matching
  `legacyLintConfigStep`'s existing wording pattern

### Requirement: the splice targets an array-literal default export inside two distinct, individually-addressable marker pairs

Where an `eslint.config.js`/`.mjs`/`.cjs` exists, its default export is an
array literal, and the parse carries no `ERROR` node, dharness MUST splice
by inserting two separately-marked regions: an import statement marked
`dharness:eslint-import begin`/`end`, and a spread element marked
`dharness:eslint-layer begin`/`end`. Each marker pair MUST be distinct — not
one pair reused twice — so a re-run can replace exactly its own bytes
without touching the other region.

#### Scenario: a plain array-literal config is spliced

- **GIVEN** `eslint.config.js` whose default export is `[...]` (an array
  literal) with no `ERROR` node in the parse
- **WHEN** `sync` runs
- **THEN** the file gains an import marked with the `dharness:eslint-import`
  pair and a spread element marked with the `dharness:eslint-layer` pair,
  each individually delimited

#### Scenario: a `.cjs` config using CommonJS rather than ESM delegates

- **GIVEN** an `eslint.config.cjs` whose config is assigned with
  `module.exports = [...]` and which carries no `export default` at all
- **WHEN** dharness inspects it
- **THEN** it delegates, naming the absent default export — the extension
  admits the file to this requirement, but only `export default` is a shape
  the splice recognises

The extension list above names which files dharness *looks at*; it is not a
claim about the module system inside them. `.cjs` in particular may hold
CommonJS, where the config is a `module.exports` assignment and there is no
`export default` to anchor to. That refuses through the existing
no-default-export path rather than a rule of its own, and this scenario
exists so the behaviour is stated rather than inferred from the absence of a
rule. Splicing an `import` statement into a CommonJS module would be wrong
twice over — the wrong module system, and a file that then fails to load.

#### Scenario: the two marker pairs are never merged into one

- **GIVEN** a spliced config
- **WHEN** its marker regions are inspected
- **THEN** exactly one `dharness:eslint-import` region and one
  `dharness:eslint-layer` region exist, at different locations, never a
  single shared marker doing both jobs

### Requirement: the insertion position is the array's first element, always, and never lands after a project element

The spread element MUST be inserted at index 0 of the config array, before
every element the project wrote. There is no recognition step and no
condition: the position does not depend on what the project imported.

The load-bearing invariant is that dharness's layer precedes the project's
own custom entries (§05), and first-element placement satisfies it
unconditionally — nothing the project authored can be shadowed by dharness,
because dharness is underneath all of it.

**An earlier revision of this requirement** made the position conditional:
insert after the last array element that was a spread of a recognised
framework config binding, otherwise at index 0. It is **withdrawn**, and the
recognition it needed is withdrawn with it. ESLint flat config merges
last-wins *per rule*, and the framework configs dharness contributes declare
no `dharness/*` rule, so their position relative to dharness's layer has no
observable effect on any rule's resolved severity. The conditional rule
bought ordering between two layers that do not intersect, and it was the only
thing requiring the config parser to know framework package names.

#### Scenario: a framework spread is preceded, not followed

- **GIVEN** `eslint.config.js` whose array begins
  `[...nextConfig, { rules: { ... } }]`
- **WHEN** the splice runs
- **THEN** the `dharness:eslint-layer` spread is the array's first element,
  before `...nextConfig`, and both of the project's original elements keep
  their order behind it

#### Scenario: a project's own element as the first entry is still preceded

- **GIVEN** `eslint.config.js` whose array's first element is a project's
  own custom rule object
- **WHEN** the splice runs
- **THEN** the `dharness:eslint-layer` spread is inserted as the array's
  first element — before the project's own rule, never after it

### Requirement: comments preceding the insertion anchor survive attached to what they describe

The insertion offset MUST be the start of the line of the earliest byte in
the contiguous comment run immediately preceding the anchor element, so a
project's comment stays attached to the element it was written to describe
rather than landing after dharness's inserted spread.

#### Scenario: a comment above the anchor element is not orphaned

- **GIVEN** an array element preceded by a comment on its own line,
  immediately above it with no blank line between
- **WHEN** the splice inserts before that element
- **THEN** dharness's inserted spread lands before the comment, not between
  the comment and the element it describes

### Requirement: `defineConfig([...])` imported from `"eslint/config"` is spliced, under three required conditions, all of them

Where the default export is a call expression, dharness MUST splice it
exactly when all three hold, and MUST delegate if any fails: the callee
resolves to an identifier imported from the module specifier
`"eslint/config"`; the call's first argument is an array literal; and the
parse of that first argument carries no `ERROR` node. The spread is
inserted into that array argument by the same position and comment rules as
the plain-array case. This is the documented shape ESLint's own basic
example leads with, not a convention dharness invents.

#### Scenario: the documented `defineConfig` shape is spliced

- **GIVEN** `eslint.config.js` containing
  `import { defineConfig } from "eslint/config";` and
  `export default defineConfig([{ rules: { semi: "error" } }]);`, with no
  `ERROR` node
- **WHEN** `sync` runs
- **THEN** dharness splices the `dharness:eslint-layer` spread into the
  array passed to `defineConfig`, following the same position and comment
  rules as a bare array-literal export

#### Scenario: an `ERROR` node inside the `defineConfig` argument still refuses

- **GIVEN** a `defineConfig([...])` call whose first argument's parse
  contains an `ERROR` node
- **WHEN** `sync` runs
- **THEN** dharness delegates rather than splicing, even though the callee
  and import both match — all three conditions must hold, not two of three

### Requirement: any other call expression delegates — a lookalike is not the documented shape

Where the default export is a call expression that does not satisfy every
condition of the `defineConfig` requirement above — `defineConfig` imported
from any module other than `"eslint/config"`, `tseslint.config(...)`,
`withNuxt(...)`, a locally-defined helper, or any other call — dharness MUST
delegate. Whether such a call returns an array cannot be known without
executing project code (§17, §21), and nothing published documents what
these calls return.

#### Scenario: `tseslint.config(...)` delegates

- **GIVEN** `eslint.config.js` whose default export is `tseslint.config(...)`
- **WHEN** `sync` runs
- **THEN** dharness delegates without attempting to splice, naming that the
  call's return shape is not known without executing project code

#### Scenario: `defineConfig` imported from another module delegates

- **GIVEN** `eslint.config.js` with a locally-defined function also named
  `defineConfig`, imported from a project-local path rather than
  `"eslint/config"`, used as `export default defineConfig([...])`
- **WHEN** `sync` runs
- **THEN** dharness delegates — the identifier name alone does not satisfy
  the requirement; the import specifier must resolve to `"eslint/config"`

### Requirement: an `ERROR` node covering the default export refuses the whole file

Where the parse of the default export contains any `ERROR` node, dharness
MUST refuse and delegate, regardless of which other conditions above would
otherwise be satisfied. A partial tree from an error-tolerant parser is not
permission to edit a file dharness did not fully understand.

#### Scenario: a syntactically broken config is not edited

- **GIVEN** `eslint.config.js` whose default export contains a syntax error
  the parser reports as an `ERROR` node
- **WHEN** `sync` runs
- **THEN** dharness delegates, and no bytes of the file are written

### Requirement: a malformed marker pair refuses rather than guesses

Where a marker begin is present without its matching end, or an end appears
before its matching begin, dharness MUST refuse and delegate rather than
attempt to infer the intended region. Guessing at a half-written region is
how a config gets corrupted.

#### Scenario: a begin marker with no matching end refuses

- **GIVEN** `eslint.config.js` containing a `dharness:eslint-layer begin`
  comment with no corresponding `end` comment anywhere in the file
- **WHEN** `sync` runs
- **THEN** dharness delegates, naming the malformed marker pair, and does
  not attempt to locate or replace a region

#### Scenario: an end marker appearing before its begin refuses

- **GIVEN** `eslint.config.js` containing a `dharness:eslint-import end`
  comment that appears earlier in the file than any
  `dharness:eslint-import begin` comment
- **WHEN** `sync` runs
- **THEN** dharness delegates, naming the malformed marker pair, and no
  bytes are written

### Requirement: two `sync` runs over an already-spliced config produce byte-identical output

Running `sync` a second time over a config already carrying both marker
regions MUST leave the file byte-identical to its state after the first
run, and `eslintExtendsStep.Satisfied` MUST already be `true` on that second
run.

#### Scenario: idempotency holds across repeated runs

- **GIVEN** an already-spliced `eslint.config.js`
- **WHEN** `sync` runs again with nothing else changed
- **THEN** the file's bytes are unchanged, `eslintExtendsStep.Satisfied(p)`
  returns `true`, and `Apply` is not called

#### Scenario: the spliced array gains exactly one element, never more

- **GIVEN** a config with an original array of N elements before any splice
- **WHEN** two `sync` runs complete
- **THEN** parsing the result finds exactly one region of each marker kind
  and exactly N+1 elements in the default-export array — not N+2

### Requirement: a config whose marked regions are stale converges by replacement, never by a second insertion

Where both marker regions are present and well-formed but their bytes differ
from what the current run renders — a preset began contributing a layer, a
severity changed — dharness MUST rewrite each region **in place, at the
bounds the marker scan reports**, and MUST NOT insert a second pair of
regions at the position rule's anchor.

This is a reachable, expected state rather than an error: `Satisfied` is a
byte comparison, so *present but different* is exactly what a changed
contribution looks like on the next run. An implementation that could only
insert would produce a file carrying two of each region, which the marker
scan is required to reject as malformed — leaving the step failed on every
subsequent run until a human deleted a region by hand.

#### Scenario: a newly contributed layer converges on the next run

- **GIVEN** an `eslint.config.js` spliced by an earlier run, whose marked
  regions do not name a framework layer
- **AND** a project state in which a preset now contributes one
- **WHEN** `sync` runs
- **THEN** both marked regions carry the new render, exactly one region of
  each marker kind exists, the array's element count is unchanged, and every
  byte outside the two regions is untouched

### Requirement: everything outside the marked regions — including CRLF and BOM — survives byte-for-byte

The splice MUST be byte-surgical: bytes outside the two marked regions MUST
be unchanged from the input, and the inserted region MUST adopt the line
ending observed on the anchor line rather than a fixed convention. A BOM
present at the start of the file MUST be preserved.

#### Scenario: CRLF line endings are matched, not normalised

- **GIVEN** `eslint.config.js` using CRLF line endings throughout
- **WHEN** the splice inserts the two marked regions
- **THEN** the inserted lines use CRLF, matching the anchor line's ending,
  and every pre-existing line elsewhere in the file keeps its original CRLF
  bytes

#### Scenario: a byte-order mark survives the splice

- **GIVEN** `eslint.config.js` beginning with a UTF-8 BOM
- **WHEN** the splice runs
- **THEN** the output file still begins with the same BOM bytes

### Requirement: the edit is verified inside the transaction — re-parse, or `Undo` runs

After splicing, dharness MUST re-parse the resulting bytes with the same
parser and assert: no `ERROR` node, exactly one region of each marker kind,
and an array with exactly one more element than before the splice. Any
failure of this assertion MUST cause `Apply` to return an error, which
`applySteps` turns into `Writer.Undo` and a byte-exact restore — never a
written file left in a state that failed its own check.

#### Scenario: an edit that would leave the config unparseable is rolled back

- **GIVEN** a splice that, applied, would produce an `ERROR` node in the
  result (a constructed adversarial case)
- **WHEN** `Apply` runs the in-transaction re-parse
- **THEN** `Apply` returns an error, `Writer.Undo` restores the original
  bytes exactly, and `sync` reports the step as failed, not done

#### Scenario: ESLint itself is never run to validate the splice

- **GIVEN** a completed splice
- **WHEN** dharness decides whether the edit succeeded
- **THEN** the verdict comes from the structural re-parse alone — no Node
  process is launched to load the resulting config, per §11/§17 and
  `CLAUDE.md`'s second rule

### Requirement: the splice contributes nothing to the golden tree for a project with no ESLint config

Where the golden fixture project has no ESLint configuration file, the
splice sub-mechanism MUST NOT write anything: it is satisfied trivially,
because there is no existing array to splice into, and the `== tree ==`
region of `internal/setup/golden_test.go`'s generic fixture is unaffected
by it.

#### Scenario: the generic golden fixture's tree is untouched by the splice logic

- **GIVEN** the golden fixture project with no ESLint configuration
- **WHEN** `sync` runs against it
- **THEN** the splice logic contributes no bytes to the resulting tree, and
  any change the golden fixture requires under this proposal
  (`.dharness/eslint.config.js` appearing, the severity block leaving
  `doctor.config.json`) comes from a different step, hand-edited into the
  fixture for exactly the slice that introduces it — never regenerated via
  `-update`, per `TestGenericMechanismHasNoUpdatePath`

### Requirement: the parser dependency builds without cgo on every release target

`github.com/odvcencio/gotreesitter` MUST compile with `CGO_ENABLED=0` for
all six of dharness's release targets, and this MUST be recorded as a
deviation in `AGENTS.md` rather than left implicit — the product's first
dependency, deliberately accepted.

#### Scenario: the six release targets build with cgo disabled

- **GIVEN** the dependency added by this change
- **WHEN** `CGO_ENABLED=0 go build ./...` runs for each of the six release
  targets
- **THEN** every target builds successfully, and `.goreleaser.yml` requires
  no change to accommodate the dependency

---

## Capability: `eslint-gate-stage`

Where ESLint runs inside `dharness check`, how it resolves the binary, and
when it does not run at all.

### Requirement: placement follows measured cost, not an asserted position

The ESLint stage's position among the gate's stages MUST be set by
measuring its cost against the other stages (§12: cheapest first, first
failure cuts the rest), not by asserting an ordering that was not measured.
The measurement and its result MUST be recorded, not merely assumed from
ESLint's cost profile in isolation — this repository has already paid once
for asserting an ordering it had not measured.

#### Scenario: the recorded placement cites a measurement, not an assumption

- **GIVEN** the gate's stage ordering after this change ships
- **WHEN** the ordering is inspected against the design record or
  `docs/learning-log.md`
- **THEN** the ESLint stage's position is justified by a comparison against
  the other stages' measured cost over an explicit staged file list, not by
  an unmeasured claim that it is cheaper or more expensive than react-doctor

### Requirement: ESLint resolves through the project's local binary, never a remote executor — a recorded exception

The ESLint stage MUST resolve through `p.LocalBinary("eslint")`, the same
seam `hookManager` already uses for lefthook, and MUST NOT use the
remote-executor resolution (`eslint@latest`) that `docs/design-principles.md`'s
10 August 2026 amendment to §03 otherwise fixes for react-doctor, fallow,
and Stryker. This is a deliberate, recorded exception: an ESLint flat config
imports the project's own plugins and framework configs, which a transient
environment cannot resolve — the same failure shape suspected behind
react-doctor's `--staged` plugin drop.

#### Scenario: the gate runs the project's own installed ESLint

- **GIVEN** a project with ESLint installed locally and resolvable through
  its package manager
- **WHEN** the gate's ESLint stage runs
- **THEN** it invokes the binary `p.LocalBinary("eslint")` resolves, not a
  remote `eslint@latest` execution

#### Scenario: the exception is named where the general rule is recorded

- **GIVEN** `docs/design-principles.md`'s §03 10 August 2026 amendment,
  which fixes remote-executor resolution for react-doctor, fallow, and
  Stryker
- **WHEN** this change lands
- **THEN** the exception for ESLint's project-local resolution is recorded
  beside that amendment, not left as an unstated inconsistency

### Requirement: no `--cache` in this change

The ESLint invocation MUST NOT pass `--cache`. Caching would write
`.eslintcache` into the project's tree — a file §03 would then have to
account for as project-owned or dharness-owned — and is stale-prone across
branches. This is deferred as a measured optimisation, not adopted now.

#### Scenario: the invocation carries no cache flag

- **GIVEN** the ESLint stage's invocation in `internal/tool/tool.go`
- **WHEN** its arguments are inspected
- **THEN** `--cache` is absent, and no `.eslintcache` file is written by
  this stage

### Requirement: no ESLint installed means the stage does not run — named, never a block

Where a project has no ESLint resolvable through `p.LocalBinary("eslint")`,
the gate MUST skip the ESLint stage entirely, name that it was skipped and
why in its output (§16), and MUST NOT treat the absence as a failure or
block the gate (§20). dharness does not install ESLint into a project at
gate time — installation is the project's toolchain choice, made (if at
all) through `eslintExtendsStep` at `sync` time.

#### Scenario: a project with no ESLint skips the stage without failing

- **GIVEN** a project with no ESLint binary resolvable
- **WHEN** `dharness check` runs
- **THEN** the ESLint stage is skipped, the output names that it was
  skipped and why, and the gate's overall exit code is unaffected by this
  absence

### Requirement: the setup step and the gate stage answer different questions, independently

`eslintExtendsStep` (at `sync` time) MUST key its decision on whether an
ESLint config file exists. The gate's ESLint stage (at `check` time) MUST
key its decision on whether an ESLint binary is resolvable. These are two
independent questions, and neither answer determines the other.

#### Scenario: a config exists but no binary is installed

- **GIVEN** a project whose `eslint.config.js` was spliced by `sync`, but
  whose `node_modules` does not (yet) contain ESLint
- **WHEN** `dharness check` runs
- **THEN** the gate's ESLint stage is skipped on the binary question alone,
  independent of the config file's presence

---

## Capability: `preset-layer-contribution`

A preset contributing an installable package and a config layer — a
contribution kind `Manifest` does not have today (`Manifest` is
`{Schema, Facts, Seeds}`, `preset.go:117-121`).

### Requirement: `Manifest` gains a third contribution kind — package, binding, and `Because` — additively

`Manifest` MUST gain a third contribution kind alongside `Facts` and
`Seeds`, carrying the package to install, the import binding to wire into
`.dharness/eslint.config.js`, and its `Because`. This addition MUST NOT
require a schema bump: the manifest never leaves the binary, so the field
is additive to the existing `dharness.preset/v1` schema.

#### Scenario: the new kind ships without a schema version change

- **GIVEN** `Manifest`'s schema identifier before and after this change
- **WHEN** the third contribution kind is added
- **THEN** the schema identifier is unchanged, because the manifest is an
  in-binary structure, never serialised across a version boundary

#### Scenario: a preset with no package contribution still validates

- **GIVEN** a preset manifest (e.g. `wails`) that contributes only `Facts`,
  no package
- **WHEN** the manifest is validated
- **THEN** it validates successfully with the new field left empty — the
  third kind is additive, not mandatory for every preset

### Requirement: an emitted import binding is namespaced, so it cannot collide with one the project already declared

Every identifier dharness writes into the project's import region MUST be
namespaced to dharness — it MUST NOT be the bare, obvious name for the
package it imports. `eslint-config-next` is imported as `dharnessNext`,
never as `next`.

This is a correctness requirement, not a style one. Two `import` declarations
binding the same identifier in one ES module are a **SyntaxError**
(`Identifier 'next' has already been declared`), so a project whose own
config already writes `import next from "eslint-config-next"` would receive a
config ESLint cannot load at all. Measured against Node: identical module,
identical binding fails to compile; identical module under two different
bindings loads and simply contributes its entries twice, which is inert.

Neither existing guard covers this. A duplicate import is valid *syntax* —
the collision is a scope rule, not a grammar rule — so the candidate re-parse
finds no `ERROR` node; and the guard inspects the default export, while the
collision is in the import declarations above it.

#### Scenario: the project already imports the package dharness contributes

- **GIVEN** an `eslint.config.js` containing `import next from "eslint-config-next"`
- **AND** a matched preset contributing that same package
- **WHEN** the splice runs
- **THEN** the emitted import region binds a namespaced identifier distinct
  from `next`, and the resulting file loads

#### Scenario: a bare binding is rejected at build time

- **GIVEN** a registry entry whose binding is the bare package name
- **WHEN** the registry validation test runs
- **THEN** it fails, naming the entry — an authoring bug caught before it can
  reach any repository

### Requirement: `Validate` requires evidence for the new contribution kind, exactly as it already requires for `Facts` and `Seeds`

A package contribution's `Because` MUST name a checkable observable, the
same evidence-required rule `Validate` already applies to `Facts`, per §17:
a package contribution justified only by "this is what the framework
usually needs" is not a valid manifest entry.

#### Scenario: a package contribution with no checkable `Because` is rejected at validation

- **GIVEN** a candidate package contribution whose `Because` names no
  specific published fact (e.g. no version, no documented framework
  requirement)
- **WHEN** `Validate` runs against the manifest
- **THEN** validation fails, matching the existing rejection `Validate`
  already applies to an unevidenced `Facts` entry

### Requirement: `integrationPackages()` becomes preset-aware, and rollback removes exactly what that run added

`integrationPackages()` (`steps.go:88-92`) MUST include the matched
presets' contributed packages alongside the existing fixed set.
`installStep.Apply`'s existing snapshot-and-compensate mechanism
(`PackageStateFiles()` before install, exact removal of what that run added
on failure) MUST cover preset-contributed packages, with no new rollback
path introduced — the existing seam already generalises.

#### Scenario: a Next.js-matched project's install includes `eslint-config-next`

- **GIVEN** a project matched by the Next.js preset
- **WHEN** `installStep.Apply` runs
- **THEN** `eslint-config-next` is among the packages installed, contributed
  by the preset's manifest rather than a fixed list

#### Scenario: a failed install rolls back only what this run added

- **GIVEN** a Next.js-matched project where `installStep.Apply` fails
  partway through installing the preset-contributed package
- **WHEN** rollback runs
- **THEN** exactly the packages this run added are removed, using the
  existing `PackageStateFiles()` snapshot-and-compensate mechanism, with no
  new mechanism introduced for preset-contributed packages

### Requirement: Next.js and Expo contribute their own published lint config packages, versioned by the frameworks themselves

The Next.js preset MUST contribute `eslint-config-next` and the Expo preset
MUST contribute `eslint-config-expo` as installable packages with an import
binding layered into `.dharness/eslint.config.js`, ahead of dharness's own
layer, per the fixed `...frameworkRecommended, ...dharness, ...projectCustom`
order. dharness MUST NOT pin or invent a version for either package —
installing what the framework itself publishes and versions is not dharness
inventing a convention.

#### Scenario: a Next.js project's owned config layers the framework's own package first

- **GIVEN** a project matched by the Next.js preset
- **WHEN** `.dharness/eslint.config.js` is written
- **THEN** it layers `eslint-config-next`'s recommended config ahead of
  dharness's own rules, matching the fixed layering order

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

The following are non-requirements for `eslint-config-splice` and related changes:

- Running ESLint to validate the splice — rejected; the in-transaction
  re-parse is the verdict (§11, §17).
- Recording which projects have been spliced, or a progress marker of any
  kind — every run re-derives whether the marker regions are present
  (§07, §15).
- An opt-in flag or confirmation step gating the first splice in a
  repository — the marked region plus byte-exact rollback is the assumed
  sufficient safeguard; revisit only if that assumption changes.
- Machine removal of `dharness/*` severity residue from an already-adopted
  `doctor.config.json` — dharness cannot distinguish its own past write
  from the project's own (§05); reported, never removed.
- `--cache` for the ESLint gate stage — deferred as a measured optimisation.
- Sampling or measuring the real-world distribution of call-expression vs.
  array-literal `eslint.config.js` shapes — the delegation answer holds
  regardless of the distribution; the risk stays open until measured, but no
  requirement here depends on the number.
- Presets beyond `nextjs` and `expo` contributing a package layer — `wails`
  and `generic` contribute no ESLint package under this change; adding one
  for either is a future change.
- A JS/TS bundler-based or interpreter-based approach to reading
  `eslint.config.js` — rejected in exploration (esbuild and typescript-go
  are not importable parser libraries; goja and otto lack the needed
  ESM/position support).

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
