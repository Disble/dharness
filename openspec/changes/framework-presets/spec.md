# Spec: framework-presets

Source of record for behaviour: `openspec/changes/framework-presets/proposal.md`
(all four numbered decisions, the "Verified after the proposal was written"
section, and the success criteria), `openspec/changes/framework-presets/exploration.md`
(current-state citations). Principles cited by number:
`docs/design-principles.md` §03, §05, §07, §09, §15, §17, §20, §21. Proposal:
Engram `sdd/framework-presets/proposal`,
`openspec/changes/framework-presets/proposal.md`.

`openspec/specs/` is empty. All three capabilities below are new; every
requirement is `ADDED`.

---

## ADDED Requirements — Capability: `framework-presets`

Detection with evidence, versioned manifests, a registry of per-framework
packages, and composition of more than one preset's contributions by scope.

### Requirement: One package per framework, each exposing Identity, `Detect`, and a versioned `Manifest`

`internal/preset/` MUST hold one package per framework — `wails`, `expo`,
`nextjs`, `generic` — plus a registry that selects among them through one
switch in a factory. Each package MUST expose an identity, a
`Detect(p project.Project) (matched bool, evidence string)` function, and a
`Manifest` of schema `dharness.preset/v1`. No table keyed by framework ID MAY
replace the per-package split: collapsing per-fact evidence into one
undifferentiated struct is the rejected approach recorded in
`exploration.md`'s Approach 2.

#### Scenario: the registry selects a preset through one switch

- **GIVEN** a resolved `project.Project`
- **WHEN** the registry is asked which presets apply
- **THEN** selection runs through one factory switch over the four package
  identities, and each matched package's own `Detect` decides whether it
  contributes — no cross-package coupling decides for another package

#### Scenario: a framework matching none of the four presets gets `generic`

- **GIVEN** a project whose tree matches no `wails`, `expo`, or `nextjs`
  signal
- **WHEN** the registry resolves presets for it
- **THEN** `generic` is the sole preset returned, and it contributes no keys
  and no facts — it exists to make today's behaviour one preset among
  several, not to be replaced by an empty case

### Requirement: `Detect` reports evidence beside its match, never a bare boolean

Every preset's `Detect` MUST return the observable that justified a match
alongside the match itself. A preset MUST NOT report `matched == true` with
an empty evidence string.

#### Scenario: Wails detection evidence names the file that matched

- **GIVEN** a repository whose root contains `wails.json`
- **WHEN** the Wails preset's `Detect` runs
- **THEN** it returns `matched == true` and an evidence string naming
  `wails.json` as the observable

#### Scenario: no match returns no evidence

- **GIVEN** a repository with no `wails.json` at the root
- **WHEN** the Wails preset's `Detect` runs against it
- **THEN** it returns `matched == false`, and the registry does not include
  Wails among the presets contributing to this project

### Requirement: every manifest fact carries an evidence string naming the observable that justifies it

A `Manifest` fact MUST be shaped `{value, because}`. `because` MUST name the
specific observable — a file, a key inside that file, or a documented
framework default — that justifies `value`. A fact whose `because` names
nothing checkable (e.g. a bare "this is how the framework usually works") is
not a valid manifest entry, per §17: the verdict comes from what was
observed, never from prose asserting a general truth.

#### Scenario: the Wails ignore-pattern fact names the key and its fallback

- **GIVEN** the Wails preset's manifest fact for the generated frontend
  directory
- **WHEN** its evidence is inspected
- **THEN** `because` names both `wailsjsdir` (the key read from `wails.json`)
  and the documented default (`frontend`) used when the key is absent — not
  the path alone, because the path is derived from either source and the
  evidence must say which

#### Scenario: a fact with no checkable observable is rejected at review, not written

- **GIVEN** a candidate manifest fact whose `because` is prose with no named
  file, key, or documented default behind it
- **WHEN** the manifest is authored
- **THEN** that fact does not ship — every fact in every preset's manifest
  names a checkable observable, with none deferred to "generally true of the
  framework"

### Requirement: a preset fact is a default the repository can override, never an assertion (§07)

dharness MUST NOT assert that a preset-implied directory or condition exists
in this repository. A fact contributed by a preset MUST be written as
something inert where the condition it describes is false, and MUST be
re-derived on every run rather than recorded once and trusted thereafter.

#### Scenario: an ignore pattern for a directory that does not exist is inert, not wrong

- **GIVEN** a Wails-matched project where `wailsjsdir` resolves to a
  directory that does not (yet) exist on disk
- **WHEN** `sync` writes the contributed ignore pattern into
  `.dharness/fallow.jsonc`
- **THEN** the pattern is written anyway, and fallow ignoring a
  non-existent path is a no-op, not a reported error — the preset never
  required the directory to exist to justify writing the pattern

#### Scenario: nothing about a preset fact is persisted between runs

- **GIVEN** a preset's manifest for a matched project
- **WHEN** `sync` runs twice in a row with nothing else changed
- **THEN** the second run derives the same facts from the same detection,
  reading no record of "this preset already contributed X" — there is no
  such record to read (§07)

### Requirement: precedence is fixed and total — project declaration > detection > preset > global default

Where more than one source could answer the same question, dharness MUST
resolve it in this order, with no rung skipped and no rung reordered:

1. **Project declaration** — the project's own config already states an
   answer. dharness MUST NOT override it.
2. **Detection** — an observable fact about this repository's tree (e.g.
   barrel files present) that is not framework-specific.
3. **Preset** — a framework-carried default (e.g. Wails' `wailsjsdir`
   fallback), used only where detection has nothing narrower to say.
4. **Global default** — what dharness writes when nothing above answered
   (e.g. `DefaultThresholds()`, `error` severity for a rule with no other
   answer).

A rung MUST only be consulted when every rung above it did not answer.

#### Scenario: project declaration wins over a matched preset

- **GIVEN** a Wails-matched project whose own `.fallowrc.json` already
  declares `ignorePatterns` (any value)
- **WHEN** `sync` derives what to write
- **THEN** the preset's contributed ignore pattern is still written into
  `.dharness/fallow.jsonc` (per `owned-config-contribution`'s write-anyway
  requirement) and reported as colliding, but the project's own declared
  value is what fallow actually honours — dharness does not edit the
  project's file to force its own value to win

#### Scenario: detection wins over a preset default for folder-ownership

- **GIVEN** a Next.js-matched project whose tree has `index.ts` barrels
- **WHEN** `sync` derives the severity for `folder-ownership`
- **THEN** the barrel-presence detection answers `error`, and no preset
  carries or overrides a `folder-ownership` opinion — decision 2 keeps this
  rung out of every preset's manifest entirely

#### Scenario: preset wins over the global default when nothing more specific answers

- **GIVEN** a Wails-matched project with no project declaration for the
  ignore pattern and no detection question applicable to it
- **WHEN** `sync` derives what to write
- **THEN** the preset's fallback value (`frontend`, per Wails' own
  documented default) is what gets written — the global default (an empty
  commented object) is never reached because the preset rung answered first

#### Scenario: global default is reached only when nothing else answers

- **GIVEN** a `generic`-matched project (no preset facts, no relevant
  detection)
- **WHEN** `sync` derives `.dharness/fallow.jsonc`'s contents
- **THEN** the global default applies unchanged — the empty commented
  object, exactly as today

### Requirement: `generic` reproduces today's behaviour exactly

For a project matching no framework preset, `Plan()`'s returned steps (IDs,
`Describe` text, `Satisfied`/`Delegated` outcomes) and every file `sync`
writes MUST be byte-identical to the behaviour before this change. This MUST
be measured against a golden fixture captured before any preset beyond
`generic` is implemented (proposal slice 1), not against a description of
intended behaviour.

#### Scenario: the golden pin exists before any real preset

- **GIVEN** `internal/setup/setup_test.go` has no `Plan()`-output golden
  today (verified: neither it nor `steps_root_test.go` snapshots the full
  plan)
- **WHEN** slice 1 of this change lands
- **THEN** a golden fixture capturing `Plan()`'s step IDs and every written
  file's bytes for a `generic`-matched project exists, and it is captured
  from behaviour that predates the registry — not authored by hand to match
  a design intention

#### Scenario: `generic` matches the golden byte-for-byte

- **GIVEN** the slice 1 golden fixture and a `generic`-matched project
- **WHEN** `sync` runs against it after the registry and all four presets
  exist
- **THEN** `.dharness/fallow.jsonc`, `.dharness/lefthook.yml`, and
  `.dharness/rules.json` are byte-identical to the golden, and `Plan()`'s
  step IDs, `Describe` strings, and `Satisfied`/`Delegated` outcomes are
  unchanged from the golden's recorded values

### Requirement: multi-preset composition by scope — Root-scope and Source-scope signals both contribute in one repository

The registry MUST return contributions from more than one matched preset in
a single repository, keyed by which of `Root`/`Source` (the split
`project.Discover` already makes) each preset's signal belongs to. A
Root-scope signal (`wails.json`, at the repository root) and a Source-scope
signal (a `package.json` dependency, at the resolved Source directory) MUST
be able to match simultaneously and both contribute.

#### Scenario: a Wails root with a Next.js source contributes from both presets

- **GIVEN** a repository whose root has `wails.json` and whose resolved
  Source directory has `next` declared as a `package.json` dependency
- **WHEN** the registry resolves presets for this project
- **THEN** both the Wails preset (matched at Root scope) and the Next.js
  preset (matched at Source scope) contribute to the manifest handed to
  `ownedFilesStep`, and neither preset's absence is reported — success
  criterion: "A Wails root with a Next.js source receives contributions from
  both presets, resolved by scope"

#### Scenario: two presets contributing the same key is a defined outcome, not a crash

- **GIVEN** a hypothetical repository where two matched presets each
  contribute a value for the same key
- **WHEN** the registry composes their contributions
- **THEN** the composition follows the same fixed precedence stated above
  (project > detection > preset > global default) with a documented
  tie-break between presets at the same rung — this is not left implicit for
  an implementer to guess

### Requirement: multi-framework monorepos remain out of scope, inheriting the existing ambiguity failure

Where `project.Discover` cannot resolve a single Source because more than
one independent JS project exists and the caller's directory does not settle
it, `AmbiguousSourceError` MUST still be returned before any preset
resolution runs. Preset resolution MUST NOT attempt to guess among multiple
Source candidates, and MUST NOT be reached until `Discover` has already
resolved to exactly one Source.

#### Scenario: an ambiguous repository fails closed before presets run

- **GIVEN** a repository with two independent JS projects and a caller
  standing outside both
- **WHEN** `sync` runs
- **THEN** `Discover` returns `AmbiguousSourceError` naming both candidates,
  and no preset's `Detect` is called — this is unchanged from today's
  behaviour, per decision 3

---

## ADDED Requirements — Capability: `owned-config-contribution`

What dharness writes into `.dharness/fallow.jsonc`, the only file it owns
outright (§03), and how a key a matched preset contributes but the project
also declares is detected, reported, and resolved.

### Requirement: `ownedFilesStep.Apply` writes the matched presets' contributed keys instead of an empty object

`ownedFilesStep.Apply` MUST write the union of the matched presets'
contributed manifest keys into `.dharness/fallow.jsonc`, composed per the
precedence and scope rules in `framework-presets`. Where no preset
contributes a key (a `generic`-matched project), the file MUST remain the
existing empty commented object.

#### Scenario: a Wails-matched project's owned file is no longer empty

- **GIVEN** a project matched by the Wails preset
- **WHEN** `ownedFilesStep.Apply` runs
- **THEN** `.dharness/fallow.jsonc` contains the ignore pattern the Wails
  manifest contributes — success criterion: "In a Wails project,
  `.dharness/fallow.jsonc` is no longer an empty commented object"

#### Scenario: a generic-matched project's owned file stays empty

- **GIVEN** a project matched only by `generic`
- **WHEN** `ownedFilesStep.Apply` runs
- **THEN** `.dharness/fallow.jsonc` is written as the same empty commented
  object as before this change, with the `boundaries` comment block intact
  and undisturbed

### Requirement: `declaresBoundaries` generalises into `declaredKeys`, over the same quoted-key test

The collision check MUST test the project's own `.fallowrc.json` (or
equivalent) for the **quoted** form of each candidate key (`"ignorePatterns"`,
not the bare word), for the same reason `declaresBoundaries` already does:
these files are JSONC, a config correctly pointing at dharness may carry the
bare word inside an explanatory comment, and a bare substring test would
false-positive on that comment. `declaredKeys(path string, candidates []string) []string`
MUST return the subset of `candidates` found in their quoted form in the
file at `path`. No JSON or JSONC parser MUST be introduced to answer this
question — dharness's own stdlib-only constraint and the honest limit
`declaresBoundaries` already documents (a comment that quotes the key
false-positives) both carry over unchanged.

#### Scenario: `declaredKeys` finds a quoted key

- **GIVEN** `.fallowrc.json` containing `"ignorePatterns": ["wailsjs/**"]`
- **WHEN** `declaredKeys` is called with `["ignorePatterns", "boundaries"]`
  as candidates
- **THEN** it returns `["ignorePatterns"]` — `boundaries` is absent from the
  file and is not returned

#### Scenario: a bare word inside a comment does not count as declared

- **GIVEN** `.fallowrc.json` containing the comment
  `// Architecture boundaries live in the file dharness owns` and no quoted
  `"boundaries"` key anywhere in the file
- **WHEN** `declaredKeys` is called with `["boundaries"]` as a candidate
- **THEN** it returns an empty slice — the bare word inside the comment does
  not match the quoted-key test, matching `declaresBoundaries`'s existing
  behaviour for the same file

#### Scenario: `declaredKeys` is the one mechanism behind both the boundaries check and the generalised collision check

- **GIVEN** the widened `boundariesOwnerStep` and the new contributed-key
  collision logic
- **WHEN** either checks whether the project's config declares a key
- **THEN** both call through `declaredKeys`, and `declaresBoundaries` either
  becomes a call to `declaredKeys(path, []string{"boundaries"})` or is
  retired in its favour — no second, divergent textual test is introduced

### Requirement: `boundariesOwnerStep` widens from one key to the set of keys the matched presets contribute

The step's `Satisfied` MUST be unsatisfied exactly when the collision set —
the matched presets' contributed keys intersected with `declaredKeys` on the
project's own config — is non-empty. `Describe`/`Delegated` MUST name every
colliding key and both values (the preset's contributed value and the
project's declared value), not only `boundaries`.

#### Scenario: `ignorePatterns` collides in the motivating repository

- **GIVEN** a Wails-matched project whose own `.fallowrc.json` already
  declares `"ignorePatterns": ["wailsjs/**"]`, and the Wails preset
  contributes an `ignorePatterns` fact
- **WHEN** `sync` derives the plan
- **THEN** the widened step is unsatisfied, `Delegated` returns `ok == true`
  naming `ignorePatterns` with both the preset's contributed value and the
  project's own declared value shown, and the run continues — success
  criterion: "In a project whose own `.fallowrc.json` declares a key a
  matched preset contributes, `sync` names that key, shows both values, and
  the run continues without rollback"

#### Scenario: no collision leaves the step satisfied

- **GIVEN** a Wails-matched project whose own `.fallowrc.json` declares no
  key any matched preset contributes
- **WHEN** `sync` derives the plan
- **THEN** the widened step's `Satisfied` returns `true`, and the step is
  absent from the plan (§15)

#### Scenario: `boundaries` alone still collides, unchanged from today

- **GIVEN** a project whose own `.fallowrc.json` declares `"boundaries"`
  (the pre-existing single-key case)
- **WHEN** `sync` derives the plan
- **THEN** the widened step behaves exactly as `boundariesOwnerStep` does
  today for this one key — `boundaries` is a fixed member of the candidate
  set regardless of which preset matched, because the architecture block is
  written by `ownedFilesStep` for every project, not only preset-matched
  ones

### Requirement: dharness writes the contributed key into its own file anyway; it never suppresses the write to avoid a collision

`ownedFilesStep.Apply` MUST write every matched preset's contributed key
into `.dharness/fallow.jsonc` regardless of whether the collision check
finds the project also declaring it. The write MUST NOT be conditioned on
the collision step's outcome, and MUST NOT be skipped, commented out, or
replaced with a placeholder because the project already has its own value.

#### Scenario: the write happens even though the value will not take effect

- **GIVEN** the motivating repository, where the project's own
  `.fallowrc.json` declares `ignorePatterns` and `extends` will discard the
  preset's value entirely once fallow resolves the child config
- **WHEN** `ownedFilesStep.Apply` runs
- **THEN** `.dharness/fallow.jsonc` still contains the preset's
  `ignorePatterns` value — the write is unconditional, because suppressing
  it would make dharness's own file misstate what it recommends and would
  erase the signal the moment the project later removed its declared key

### Requirement: the intersection emptying makes the collision step disappear, with nothing recorded (§07, §15)

Once the project's config no longer declares a colliding key — whether by
the agent moving dharness's values into the project's key, or by removing
the project's key and keeping dharness's — the collision step MUST become
satisfied and disappear from the plan on the next `sync`, with no file or
record persisted to remember that a collision previously existed.

#### Scenario: resolving the collision either way clears it on the next run

- **GIVEN** the `ignorePatterns` collision from the scenario above, resolved
  by the agent deleting the project's own `ignorePatterns` key
- **WHEN** `sync` runs again
- **THEN** the collision step is satisfied and absent from the plan —
  `declaredKeys` re-reads the project's file fresh, with no prior-collision
  record consulted or written — success criterion: "Resolving the collision
  either way makes the step disappear on the next `sync`, with nothing
  recorded"

#### Scenario: re-declaring the key afterward brings the step back

- **GIVEN** a resolved collision (step absent) and a project that later
  re-adds the same key to its own `.fallowrc.json`
- **WHEN** `sync` runs again
- **THEN** the collision step reappears, unsatisfied, exactly as if it had
  never been resolved — `Satisfied` is re-derived every run, never cached

### Requirement: a project config that is code, not data, cannot be checked for declared keys — that case describes and continues

Where the project's relevant configuration is executable code
(`doctor.config.ts`, ESLint flat config) rather than a data file
`declaredKeys` can read textually, dharness MUST NOT attempt to parse or
execute it to learn declared keys. The affected step MUST describe that its
keys could not be checked and continue, matching the existing precedent for
`doctor.config.ts` (`legacyLintConfigStep`'s own handling of the analogous
case) rather than stopping the run.

#### Scenario: a TypeScript doctor config is described and skipped, not parsed

- **GIVEN** a project whose only doctor configuration is `doctor.config.ts`
  (code, not `doctor.config.json`)
- **WHEN** the collision check considers whether this project declares a
  contributed key
- **THEN** it does not attempt to read or parse `doctor.config.ts` for keys,
  reports (where applicable) that the check could not run against a code
  config, and the run continues past it without stopping or rolling back

---

## ADDED Requirements — Capability: `rule-severity-derivation`

Deriving a rule's first-write default from the tree rather than from a
global constant, and the honest limit on when that derivation applies.

### Requirement: `offByDefault` is removed as a global constant; `DefaultSeverity` takes the project

`internal/setup/plugin.go`'s package-level `offByDefault` map MUST be
removed. `DefaultSeverity` MUST change signature from `DefaultSeverity(rule
string) string` to `DefaultSeverity(p project.Project, rule string) string`.
Its contract MUST remain otherwise unchanged: it answers only for a rule the
project has not chosen a severity for itself (§05), and its sole call site
remains `doctorConfigStep.Apply`'s `if _, chosen := config.Rules[id];
!chosen` branch.

#### Scenario: the global map no longer exists

- **GIVEN** `internal/setup/plugin.go` after this change
- **WHEN** its symbols are inspected
- **THEN** no package-level `offByDefault` variable exists, and
  `DefaultSeverity` requires a `project.Project` argument to compile

#### Scenario: `DefaultSeverity` still answers only where the project chose nothing

- **GIVEN** a project's `doctor.config.json` already declares a severity for
  `dharness/folder-ownership`
- **WHEN** `doctorConfigStep.Apply` runs
- **THEN** `DefaultSeverity` is never called for that rule id — the existing
  `!chosen` guard is unchanged, so a project's own choice is never
  overwritten (§05)

### Requirement: `folder-ownership`'s default severity is derived from barrel presence in the tree, asked of git

Whether the resolved Source tree publishes barrel files (`index.ts` or
`index.tsx`) MUST decide `folder-ownership`'s first-write default:
`"error"` where at least one barrel is present, `"off"` where none is. This
question MUST be asked of git (matching the `Discover` precedent of asking
the tool rather than walking the tree), not answered by a framework preset —
per decision 2, barrel presence is a direct, observable-from-the-tree signal
and no preset MAY carry or override an opinion about it (§09).

#### Scenario: a barrel-publishing tree gets `error`

- **GIVEN** a project whose Source tree contains at least one `index.ts` or
  `index.tsx` barrel file, tracked by git
- **WHEN** `doctorConfigStep.Apply` runs for the first time (no severity
  chosen yet)
- **THEN** `dharness/folder-ownership` is written `"error"` — success
  criterion: "`folder-ownership` is written `error` in a barrel-publishing
  tree and `off` in one without"

#### Scenario: a tree with no barrels gets `off`

- **GIVEN** a project whose Source tree contains no `index.ts`/`index.tsx`
  barrel file
- **WHEN** `doctorConfigStep.Apply` runs for the first time
- **THEN** `dharness/folder-ownership` is written `"off"`

#### Scenario: barrel presence is asked of git, not a directory walk

- **GIVEN** the function that answers barrel presence for a project
- **WHEN** it is implemented
- **THEN** it queries git (e.g. `git ls-files` against an `index.ts`/
  `index.tsx` pathspec, matching `Discover`'s `sourceCandidates` precedent),
  not `filepath.Walk` or an unbounded directory read

#### Scenario: no preset contributes or overrides folder-ownership

- **GIVEN** any of the four preset manifests (`wails`, `expo`, `nextjs`,
  `generic`)
- **WHEN** their contributed facts are inspected
- **THEN** none of them names `folder-ownership` or any rule severity —
  decision 2 keeps this a detection answer exclusively, never a preset
  decision

### Requirement: the derived default is a first-write default only; a project that adds barrels later is not switched on by a subsequent `sync`

`doctorConfigStep.Satisfied` MUST continue to return `true` as soon as
`RulesPackage` appears in `plugins` — this is unchanged by this capability.
Because severities are therefore written once, at first adoption, dharness
MUST NOT widen `Satisfied` to re-check severities against current barrel
presence on a later `sync`. This limit MUST be stated as a known, permanent
property of the design, not treated as a bug to fix later: dharness cannot
distinguish "the project chose `error`" from "dharness wrote `error`", so
re-deriving on a later run would risk overwriting a deliberate project
choice (§05).

#### Scenario: adding barrels after adoption does not retroactively flip the severity

- **GIVEN** a project adopted by dharness before it had any barrel files
  (`folder-ownership` written `"off"` at first adoption) and later given
  `index.ts` barrels
- **WHEN** `sync` runs again
- **THEN** `doctorConfigStep.Satisfied` is already `true` (the package is
  already in `plugins`), so `Apply` does not run again and
  `folder-ownership` stays `"off"` — this is the stated limit, not a defect

#### Scenario: the limit is documented at the point a reader would otherwise assume re-derivation

- **GIVEN** the comment block that currently records `offByDefault`'s
  rationale and measurement (`plugin.go:74-88`)
- **WHEN** this change lands
- **THEN** the comment is rewritten, not deleted — the eight-non-actionable-
  finding measurement stays, its conclusion changes from "therefore off
  everywhere" to "therefore off where the tree has no barrels", and the
  first-write-only limit is stated in the same place a reader would
  otherwise assume re-derivation happens on every run

---

## Explicit non-requirements (out of scope for this spec)

Named in the proposal as deliberately deferred, and not specified here:

- Reducing the number of delegated steps in `Plan()` — measured and
  rejected; the framing is "the owned file stops being empty", not a step
  count (decision 1).
- A preset deciding `boundaries` zones — zones encode intent and stay with
  the agent (§21); a preset may seed the `architectureStep` prompt, which is
  a future change, not this one.
- Merging preset keys into the project's own config files — dharness writes
  only its own file and, where applicable, the existing `extends` line
  (§03).
- Pushing this knowledge upstream into fallow or react-doctor — checked per
  `CLAUDE.md`'s first rule; not actionable, no command or flag answers it.
- Presets beyond the four (`wails`, `expo`, `nextjs`, `generic`) — four is
  the bounded first slice, not a ceiling.
- A JSONC parser, a YAML parser, or a framework SDK — the product stays
  stdlib-only; `declaredKeys`'s textual test is the mechanism, by design
  (decision 4).
- Multi-framework monorepo resolution beyond the existing
  `AmbiguousSourceError` fail-closed behaviour (decision 3).

## Notes on testability

Every scenario above is phrased so it maps directly to a Go test:
`Detect(p)` / manifest field inspection / `declaredKeys(path, candidates)` /
`Satisfied(p)` / `Delegated(p)` on constructed `project.Project` fixtures and
fixture config files, inspecting return values and written file bytes before/
after `setup.Apply` or `cli.RunSync` — no assertion reads report prose to
decide pass/fail, per `openspec/config.yaml`'s `specs` rule and design
principles §11/§17 (the verdict comes from exit codes and JSON, never from
prose or a model; applied to tests as: assert on `bool`/`error`/file bytes,
not on substrings of a report meant for a human or agent reader).
