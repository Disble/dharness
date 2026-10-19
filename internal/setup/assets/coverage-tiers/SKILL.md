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
| `core` | 100% | The behavior the feature exists to deliver. No statement, branch, function or line here may go unexecuted. |
| `important` | 80% | Behavior that matters to the feature but is not its purpose. |
| `infrastructure` | 0% | Generated code, type declarations, constants and wiring. Excluded from the coverage target and from mutation scope. |

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
each with its own `tier` and `reason`. An exact file match wins over the most
specific directory prefix; otherwise the feature's default applies. Conflicting
or overlapping declarations are rejected and never resolved by array order.

An adopted feature enters complete: its older files are covered under the same
targets, and a file is never exempt because it predates adoption. Generated
files, dependencies and test files are not business source for these targets.

Classification is a decision about the product, not a way to make a run pass.
When a file's tier is wrong, change the declaration deliberately and say why in
the `reason`; do not silently reassign a tier to clear a failure.

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

## Boundaries

- The gate is the enforcement. This skill explains it; it does not replace it.
- The Go resolver is the single classification authority. This skill never
  classifies a file and is never consulted to decide a tier.
- Existing stronger consumer thresholds are preserved; a conflicting weaker
  setting refuses instead of being overwritten.
- Removing the declaration while its managed files remain is a conflict, not a
  silent uninstall: the gate fails closed until adoption is removed
  deliberately.
