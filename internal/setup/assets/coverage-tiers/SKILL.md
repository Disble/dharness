---
name: coverage-tiers
description: Apply this project's feature coverage tiers and read the coverage gate's verdict.
---

# Coverage tiers

This repository declares its coverage tiers in `coverage-tiers.json`. The native
gate enforces them once the integration is wired and its prerequisites are present;
a successful `dharness sync` is not proof that coverage passed. This skill is guidance, not the enforcement: a project whose
agents never load it gets the same verdict from the same gate. Read it before
classifying source or changing a tier, not after a gate failure.

## The three tiers

| Tier | Native coverage target | What it means |
| --- | ---: | --- |
| `core` | 100% | Critical business logic: the behavior the feature exists to deliver. No statement, branch, function or line here may go unexecuted. |
| `important` | 80% | Behavior the user sees that matters to the feature but is not its purpose. |
| `infrastructure` | 0% | Code that validates itself: types the compiler checks, static constants, generated code and wiring. Excluded from the coverage target and from mutation scope. |

Targets apply **per file**, to each of statements, branches, functions and
lines. They are not a feature average: one core file at 99% branches fails the
gate even when the rest of its feature is at 100%.

These are coverage targets, not mutation-score thresholds. Mutation keeps the
verdict it already has; declaring `infrastructure` removes those paths from the
ranges `dharness mutate` would otherwise hand the engine.

## The declaration

`coverage-tiers.json` lives at the repository root, is versioned with the code,
and is the only switch: without it nothing is adopted, and `dharness sync`
writes no coverage wiring and no guidance. `enforcedPrefixes` bounds the adopted
source. Every file under a prefix must belong to a declared feature, so declare
narrow prefixes rather than a directory whose legacy cannot be fixed now.

Each feature carries an `id`, a `path`, a `defaultTier` and a `reason`.
`overrides` name one exact file or one directory prefix inside that feature,
each with its own `tier` and `reason`. The optional top-level `roles` classify
files by name in every feature: each has a `suffix`, a `tier` and a `reason`.

```json
"roles": [
  {"suffix": ".types.ts", "tier": "infrastructure", "reason": "Types the compiler checks"},
  {"suffix": "index.ts", "tier": "infrastructure", "reason": "Barrels only re-export"}
]
```

A suffix matches at a dot boundary: `.types.ts` matches `cart.types.ts`, and
`index.ts` matches `index.ts` and `cart.index.ts` but not `reindex.ts`. When
several suffixes match, the longest wins.

For each file, the first of these that matches decides its tier:

1. An exact-file override.
2. A role.
3. The most specific directory override.
4. The feature's `defaultTier`.

A role beats a directory override, so a types file inside a core directory is
still infrastructure; an exact override beats a role, so a `.constants.ts`
that computes its values can still be declared core. Conflicting or overlapping
declarations are rejected and never resolved by array order.

An adopted feature enters complete: its older files are covered under the same
targets, and a file is never exempt because it predates adoption. Generated
files, dependencies and test files are not business source for these targets.

Classification is a decision about the product, not a way to make a run pass.
When a file's tier is wrong, change the declaration deliberately and say why in
the `reason`; do not silently reassign a tier to clear a failure.

## Classifying: a tree with three levels

Tiers are a tree, not a list of file kinds. Classifying files by kind across a
whole repository ("logic is core, screens are important") skips the decision
that matters most: which features the product cannot work without.

1. **Rank the features against each other.** For each pair, ask which is more
   core:
   - **What depends on it?** Without the API connection there are no dashboards;
     a "New" button only adds one dashboard.
   - **How visible is a failure?** A leaked key or a stale number makes no noise;
     a broken button shows on the first click. An invisible failure ranks higher.
   - **Is there another path?** If users or an agent can reach the same outcome
     another way, the feature has a backup and ranks lower.

   Each feature's rank is its `defaultTier`.
2. **Inside each feature, classify its modules.** Critical business logic is
   core; behavior the user sees is important. A module that protects a core
   rule stays core inside an important feature (the store that encrypts secrets
   in an important settings feature); a screen or wiring inside a core feature
   drops to important or infrastructure. These are directory overrides.
3. **Inside each module, classify files by role.** Logic, helpers and
   sub-components take the module's tier. Code that validates itself is
   infrastructure: type declarations, static constants, barrels that only
   re-export. Declare these once as `roles`, not one override per file, so a
   role file added later is classified on purpose instead of inheriting its
   feature's tier. A file's suffix only approximates "validates itself": a
   constants file that computes its values needs an exact override.

Features cannot nest: overlapping feature paths are rejected. The feature is the
root of one branch, and everything below it lives in its overrides, which nest
to any depth. A loose file in a parent folder needs a feature of its own. A
product feature that spans several folders (a store in `src/main/`, a screen in
`src/renderer/`) is declared as one feature per folder; name them with a shared
prefix, such as `connection/store` and `connection/screen`, so they still read
as one.

### Which test reaches each tier

The gate counts code that tests **execute**, inside Vitest:

| Tier | Test that reaches it | Why the target fits |
| --- | --- | --- |
| `core` | Unit: pure functions, calculations, validations; isolated and deterministic | Every branch is cheap to reach, so 100% is affordable |
| `important` | Integration: user flows and components against a DOM | Some branches cost more than they protect, so 80% |
| outside the gate | End-to-end: real browser, network and backend | Runs outside Vitest and adds no coverage |

A critical flow still deserves its end-to-end test, but the core logic inside it
also needs a unit test, or it reads as 0%. A test that reads source text instead
of running it (for example, one that scans files to prove every IPC channel goes
through a guard) proves its point and still leaves the code it reads uncovered.
When core logic can only be reached through a component or a process boundary,
extract it into a function a unit test can call.

## What the gate does

`dharness sync` points the project's own Vitest config at
`.dharness/vitest.coverage.mjs`, a checked bridge to the managed runtime, and
adds one coverage job to the local git hook. The runtime asks dharness's
resolver (the read-only `dharness policy --root .`) for the classification and
projects it into native `include` entries and per-file thresholds for
statements, branches, functions and lines. Vitest calculates the metrics and
Vitest owns the failure. A required adopted source that is missing from the run
is an error, not a silently absent file, so an exclusion cannot turn an
unmeasured `core` or `important` file into a pass.

## What the agent still owns

- Agree on business-impact tiers with the project; do not infer permission to
  adopt or weaken its policy from an absent declaration or a failing test.
- Declare existing repository-relative paths only. Leave not-yet-created
  directories outside `enforcedPrefixes` until they exist, then extend adoption.
- Install missing local Vitest and compatible coverage-provider dependencies
  with the project's package manager, preserving versions already declared.
  Supply the project's own Vitest/Vite config, test environment and tests.
  `dharness sync` does not install coverage dependencies or write those tests.
- Adopt a feature once its tests are written. From the first `dharness sync`,
  the hook checks the whole adopted unit on every commit, including the commit
  that would add the missing tests. Adopt one feature at a time, or narrow
  `enforcedPrefixes` to the feature being adopted. While writing those tests,
  `vitest run --coverage --coverage.changed` (Vitest 4.1+) shows progress on
  the files you changed; the gate itself always checks the complete unit.
- The gate runs Vitest directly, not the project's `test` script. Work that
  script does first (generating a file the tests import, for example) belongs
  in the Vitest config's `globalSetup`, where every run picks it up; otherwise
  a fresh clone fails the gate until that step has run.
- Run the native coverage command after wiring. If needed, offer package scripts
  such as `test:coverage`, integration into the existing `check` script, and CI
  wiring separately: sync does not create them or run the coverage tests.
- Native coverage integration supports Vitest only, not Jest or Go coverage.

## Running the complete verification locally

Run the project's own wired invocation from its JS project root — the one the
gate file emits. For a Bun project whose JS source is the repository root:

    bunx --no-install vitest run --coverage --config .dharness/vitest.coverage.mjs

An npm project uses `npx --no-install`; a split layout uses the relative bridge
path the gate emits. Read `.dharness/lefthook.yml` (or the husky hook) for this
project's exact line instead of maintaining a second invocation; the hook runs
the same command. Ordinary focused runs stay coverage-free unless you pass
`--coverage` yourself.

## One Vitest testing gate

There is exactly one complete Vitest testing gate: the managed coverage-tier
invocation `vitest run --coverage --config <bridge>` that the hook runs
(`.dharness/lefthook.yml`, or the husky hook). `dharness sync` wires that job
and, where the hook already carries commands, reads the active pre-commit
commands and the local package scripts they reach. A manager's own `exec`/`x`
prefix is read as the command it runs, so `pnpm exec vitest run` is the same
complete run. Each invocation is judged on
its own, so a compound line such as `dharness check && bun run test` is read as
two commands and the second one still counts:

| What sync finds | What sync does |
|---|---|
| No Vitest, only focused/watch or selection runs, or a suite in another root or under another config | Integrates automatically, preserving every byte the hook already declares |
| A reachable complete run of the same suite, including through `cd <managed root>` or a job `root:` naming it | Leaves the merge to you (quick path below); it never deletes it |
| A command that cannot be resolved (a wrapper script such as `bun scripts/check.mjs`, a missing script, a cycle, a dynamic path, an unsupported flag) | Reports it as uncertain — unknown is not confirmed Vitest — and leaves it to you |
| A hook line that runs the check beside your own command | Leaves those bytes untouched, writes nothing for the tiers, and reports the concrete line to you; chaining coverage onto it would delete your command |
| An unused script, or the managed coverage job itself | Ignores it; neither is a conflict |

Quick path:

1. Keep the managed gate; remove the duplicate full-suite invocation from the hook or the script it reaches.
2. Leave unrelated checks, focused or watch runs, and alternate configs untouched.
3. Re-run `dharness sync` to confirm the reference is wired and the gate is one.

Checklist:

- [ ] Only the managed job runs the complete suite with coverage.
- [ ] No consumer command or script was deleted by the tool — every removal is yours.
- [ ] `git diff` on the hook shows the additive reference plus your own edit, nothing rewritten.

## When the gate fails

1. Read the failure. Vitest reports the file, the metric and the threshold that
   was not met; a missing-file error names the declared source it could not
   find.
2. Decide whether the cause is a classification error or a genuine coverage
   gap. The first is fixed in `coverage-tiers.json`, with the reason recorded;
   the second is fixed by testing the behavior the tier describes.
3. Never lower a tier, exclude a path or add an exclusion to make a run green.
   The declaration is the project's own statement about its business impact, and
   weakening it silently is the failure this policy exists to prevent.
4. Record what you ran and what it covered. A coverage pass is not proof that
   every mutation was detected, and `dharness mutate --staged` checks changed
   ranges only.

### Finding the last uncovered lines and branches

Vitest's text report shows percentages. To see exactly what is unhit, run the
gate's own command with `--coverage.reporter=html` and open
`coverage/index.html`: it marks every unexecuted statement, function and branch
at its line. Do not write a script over `coverage-final.json` for this. An `if`
with no `else` still has two branches, and the untaken implicit `else` may be
reported without a line of its own.

### A branch nobody designed

At 100% branches, core code exposes branches that only an impossible state can
reach: a `?? fallback` for a value that is always present, a defensive
`.catch(() => {})` a library makes unreachable. Choose one, and say which in the
commit:

- **The behavior is wanted:** keep the branch and pin it with a test that forces
  that state.
- **The state cannot happen:** remove the branch, replacing a fallback with an
  assertion that names the invariant, and let the existing tests prove the
  change.

Never add an exclusion comment or lower the tier to skip the choice.

## Boundaries

- The gate is the enforcement. This skill explains it; it does not replace it.
- The Go resolver is the single classification authority. This skill never
  classifies a file and is never consulted to decide a tier.
- Existing stronger consumer thresholds are preserved; a conflicting weaker
  setting refuses instead of being overwritten.
- Removing the declaration while its managed files remain is a conflict, not a
  silent uninstall: the gate fails closed until adoption is removed
  deliberately.
