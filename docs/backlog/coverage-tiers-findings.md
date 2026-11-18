# Coverage tiers — findings from the first full adoption

Report prepared for the dharness maintainers from a downstream logbook. Fifth in the set
(`dharness-sync-findings.md`, `dharness-check-findings.md`, `dharness-mutate-findings.md`,
`dharness-mutate-staged-findings.md`). Everything here concerns the coverage tiers:
the declaration, the native Vitest gate, and the distributed skill.

## Environment

| | |
|---|---|
| dharness | 1.14.0 |
| project | `d-canvas` — Electron + React, bun, Vitest 5.0.3, `@vitest/coverage-v8` 5.0.3 |
| scale | 34 features, 174 overrides, 319 files: 62 core, 58 important, 199 infrastructure |
| result | gate green: 744 tests (567 before adoption), every file at target, about 10 s |
| observed | 2026-11-14; source logbook `d-canvas/docs/logbooks/coverage-tiers.md` |

## Summary

**The gate did its job.** Adoption started at 59 files under target and ended with every
core file at 100% and every important file at 80% or more, through tests the gate forced
into screens that had none. `dharness policy` resolved the declaration on the first try
and `sync` spliced the config cleanly.

The findings are about classification, not enforcement. The declaration can express the
tree the owner wanted, but only by listing every role file by its exact path (C2), and a
role file added later silently inherits a tier nobody chose (C3). The rest are documentation
gaps, one typing gap, and two proposals that this file corrects before anyone builds them.

Each finding below was checked against the source at 1.14.0. Where the report's proposed
fix does not survive that check, the correction says why.

## Status (2026-11-18)

| Finding | Resolution |
|---|---|
| C12 | Fixed in `875d9ec`: `sync` writes `.dharness/coverage-tiers.d.mts` |
| C2, C3 | Fixed: top-level `roles` in `coverage-tiers.json` (exact override → role → directory → default) |
| C1, C5, C8, C9, C10, C11, C13 | Skill: three-level classification, test type per tier, per-file targets, html reporter, branch choice |
| C4 | Documented: one feature per folder, named with a shared prefix; `paths: []` not built |
| C6 | Documented: adopt with tests written; `--coverage.changed` for progress only; the gate stays complete |
| C7 | Documented: pre-test steps belong in the Vitest config's `globalSetup` |
| unverified | The text reporter hiding fully covered files was not checked against Vitest 5; the skill does not claim it |

---

## C1 — nothing says to rank features before classifying files

**Severity: medium. Confirmed.** `docs/coverage-tiers.md` and
`internal/setup/assets/coverage-tiers/SKILL.md` describe the tiers and the declaration's
mechanics; neither says how to decide a tier. The owner rejected two classifications
(by architecture layer, then by file role inside one feature) before the working one:
a tree ranked at three levels.

1. **Features against each other**, with three questions: what depends on it; how visible
   is a failure; is there another path.
2. **Modules inside a feature.** A module that protects a core rule stays core inside an
   important feature (the store that encrypts MCP secrets); wiring inside a core feature
   drops to infrastructure (the agent's process entry).
3. **Files inside a module, by role.** Logic, helpers and sub-components take the module's
   tier; types, constants, barrels and type-only contracts are infrastructure.

**Fix:** put the procedure in the skill, which is where an agent classifying a repository
reads. This is judgment, not execution, so it belongs to the agent as a prompt
(CLAUDE.md, rule 3).

### Source material for the skill text (added 2026-11-18)

The owner supplied the 100/80/0 reference the tiers come from, and a test-type table:

| Tier | Reference definition | Example files |
|---|---|---|
| core 100% | Critical business logic | `moneyCalculations.ts`, `cartOperations.ts` |
| important 80% | Features the user sees | `ShoppingCart.tsx`, `PaymentForm.tsx` |
| infrastructure 0% | Validates itself: TypeScript checks types, constants are static | `types/interfaces.ts`, `config/constants.ts` |

| Test type | Speed | Use for |
|---|---|---|
| Unit | < 10 ms | Pure functions, business logic, calculations, validations |
| Integration | 100–500 ms | User flows, interactions between components, shared state; real DOM |
| E2E | 1–10 s | Critical business flows, cross-browser validation, regressions |

**How it fits the three levels.** The reference covers levels 2 and 3, not level 1. Its
example has one product feature (a shop), so ranking features never comes up. Applied to
a whole repository on its own, it reproduces d-canvas's first, rejected classification
("security rules core, screens important, wiring infrastructure"). Inside one feature it
is exactly what the adopter ended up doing: the connection is core, its Settings screen
important; MCP is important, its secrets store core. So the skill text is: rank features
first (level 1, from this report), then apply the reference inside each feature.

**What the reference adds that the report lacked.**

- *Infrastructure is "validates itself".* That is the criterion, and a file suffix is
  only a proxy for it. A `.constants.ts` that computes values is not static; a
  `protocol.ts` holding only types is. This is the test a C2 suffix rule has to
  approximate, and the reason its `reason` field matters.
- *Each tier has a test type that reaches it.* Core logic is unit-test territory: pure,
  fast, deterministic, so 100% is cheap. Important code is integration territory: real
  DOM, user flows, some branches costly to reach, so 80%. d-canvas's canvas work shows
  both: React Flow screens needed DOM stubs to be tested at all, and a node-with-no-size
  branch stayed uncovered at no cost.
- *E2E tests do not count toward the gate.* They run in a browser outside Vitest, so code
  that only an E2E test reaches reads as 0%. A critical flow still gets its E2E test; the
  core logic inside it must also be reachable by a unit test.

That last point answers part of C10. When core logic can only be reached through a
component or a process boundary, extract it into a function a unit test can call, rather
than forcing the branch through an integration test.

## C2 — a role cannot be targeted, so every role file is listed by path

**Severity: medium. Confirmed.** 164 of the 174 overrides only say that a `.types.ts`,
`.constants.ts` or `index.ts` is infrastructure. `tiers.Rule` (`internal/tiers/manifest.go`)
has `path`, `tier` and `reason`, and `matches` accepts an exact path or a directory prefix
ending in `/`. Nothing matches by suffix.

**Correction to the proposed fix.** The report offers two fixes. Only the first survives:

- *A suffix rule* (`{"suffix": ".types.ts", "tier": "infrastructure", "reason": "…"}`) — sound.
  It needs a precedence decision against exact paths and directory prefixes, stated the
  way the current one is ("exact override wins over the most specific directory").
- *Role files default to infrastructure when the role-file rules are on* — **does not hold.**
  `RoleSuffixes` (`internal/setup/plugin.go:92`) is `.types.ts`, `.constants.ts`,
  `.helpers.ts`, `.schema.ts`. Helpers and schemas carry logic: the report's own level 3
  says helpers take the module's tier. And `index.ts` and `protocol.ts`, which the report
  needed, are not in the list. Coupling the two would demote real logic and still miss
  barrels.

## C3 — a role file added later silently inherits its feature's tier

**Severity: medium. Confirmed, follows from C2.** A new `.types.ts` under a core feature
is declared core without anyone deciding so. It passes because a types-only file has
nothing to measure, so the gate stays green while the declaration stops saying what the
owner decided. Seen on the first feature after adoption: one module brought a barrel and
two constants files, each needing an override by hand.

**Fix:** the C2 suffix rule. Until then, a reviewer checks every new role file by hand.

## C4 — a feature has one `path`, a product feature crosses layers

**Severity: low. Confirmed.** `Feature.Path` is a single string. The connection lives in
`src/main/settings/`, `src/main/disble/` and `src/renderer/settings/`; the adopter split
it into `connection/store`, `connection/client`, `connection/screen`.

**Open:** `paths: []` against documenting the slash-id convention. `paths` touches the
overlap check (features may not overlap) and every consumer of `Feature.Path`; the
convention costs one paragraph. Not decided here.

## C5 — features do not nest; overrides do, and the docs do not say so

**Severity: low. Confirmed.** Overlapping features are rejected, so a loose file in a parent
folder (`src/main/card-host.ts`, `ipc-args.ts`, `window.ts`, `protocol.ts`) needs a feature
of its own, while overrides nest to any depth. **Fix:** one paragraph in the docs: the
feature is the root of a branch, and the tree below it lives in the overrides. Pairs with C1.

## C6 — adopting a feature without its tests blocks every commit, including the tests

**Severity: medium. Confirmed.** Once `sync` wires the job, the hook reads it from the
working tree, so no commit passes until the whole adopted unit meets target — including
the commits that add the missing tests. The adopter committed the provider with the job set
aside, wrote tests, and committed the wiring last. fallow boundaries avoid this by reporting
existing violations without blocking; coverage has no equivalent.

**The runner already has a command for this.** Vitest has no baseline that compares against
earlier numbers, but since 4.1 it ships `coverage.changed` (`--coverage.changed`,
`--coverage.changed=<commit/branch>`): every test still runs, and coverage is collected and
judged only for files changed since a commit, or for staged and unstaged changes when set
to `true`. Under CLAUDE.md's first rule, that is what dharness should run, rather than build a
baseline. It is stricter than the report asked for: a touched file has to meet its full
target, not just avoid getting worse.

**What blocks it today (from reading the code; not run).** The runtime's
`AdoptedSourceReporter` requires every core and important file to appear in the report,
so that an exclusion can never turn an unmeasured file into a pass. Under
`--coverage.changed`, every untouched adopted file would be missing, and the run would
refuse. Supporting it means narrowing that check to the changed adopted files in that mode,
without reopening the hole it exists to close.

**Trade-off.** A changed-only gate lets an untouched file sit under target indefinitely, so
the complete run still has to happen somewhere (pre-push or CI). **Fix now:** the docs
sentence: adopt a feature with its tests written, or narrow `enforcedPrefixes` to the
feature being adopted. **Next:** decide whether the hook runs `--coverage.changed` and the
complete run moves to a later stage.

## C7 — the gate skips the project's own pre-test step

**Severity: low. Confirmed.** The generated job runs `vitest run --coverage --config
.dharness/vitest.coverage.mjs` directly. d-canvas's `test` script is
`bun run build:card-css && vitest run`; the CSS step writes a git-ignored file the tests
import, and without it 3 test files fail in the gate, so a fresh clone fails the hook.

**Fix:** a sentence in the docs. Vitest already runs work before tests (`globalSetup`),
so the place for a pre-test build is the project's Vitest config, where the gate and every
other run both pick it up. dharness should not learn to read `test` scripts for this.

## C8 — "per file, four metrics" is only in the runtime section

**Severity: low. Confirmed.** The tier table in `docs/coverage-tiers.md` lists one
percentage per tier and reads like a feature average. "Per-file thresholds for statements,
branches, functions and lines" appears forty lines later. The skill's table says it for
core only ("No statement, branch, function or line"). **Fix:** say it in both tables.

## C9 — a structural test executes nothing

**Severity: low. Confirmed as a guidance gap.** An IPC guard test that reads source text to
prove no channel bypasses `guardedHandle` left `ipc-guard.ts` at 0%. **Fix:** one line in
the skill: coverage counts executed code, not code a test reasons about.

## C10 — 100% branches reaches branches nobody designed

**Severity: low. Guidance gap.** Two answers appeared during adoption: pin the impossible
state with a test (`id ?? ''` reachable only through a hand-edited `mcp-servers.json`), or
reshape the code so the branch disappears (five fallbacks became `!` assertions naming the
invariant; one throttle guard became a timer restructure kept honest by existing tests).
Both are defensible. **Fix:** the skill names the choice — pin the branch when the behavior
is wanted, otherwise remove it and record why — so the next adopter does not rediscover it.

## C11 / C13 — finding the remaining gaps

**Severity: low. Partly confirmed; the proposed command is not recommended.** Every one of
five test writers wrote the same script over `coverage-final.json` to list unhit statements,
functions and branches, and several were confused by v8 counting a missing `else` as a
branch with no source location. The report proposes either richer gate output or a
`dharness coverage gaps <file>` command.

Vitest owns the threshold verdict and its reporters ("Vitest calculates the metrics and owns
threshold failure"). Its native `html` and `lcov` reporters already mark unhit branches at
their lines, and `--coverage.reporter=<name>` selects one per run. A dharness command that
reparses `coverage-final.json` fails the first question in CLAUDE.md: a command exists.

**Fix:** point at the native reporter in the skill, and add the v8 `else` detail there.
**Unverified:** the logbook says Vitest's text reporter hid fully covered files; d-canvas
sets no reporter, so that is Vitest's default behavior, not something dharness configured.
Check against Vitest 5 before writing it down.

## C12 — the spliced import breaks a strict typecheck

**Severity: medium. Confirmed.** `internal/setup/assets/coverage-tiers/` ships
`runtime.mjs` and `SKILL.md`, no declaration file. A project that typechecks its Vitest
config under `strict` fails with TS7016 on `import { withCoverageTiers } from
"./.dharness/coverage-tiers.mjs"`. The adopter added `"allowJs": true` to
`tsconfig.node.json`, which widens their typecheck to fix our import.

**Correction to the proposed fix.** Of "ship a `.d.mts`, or JSDoc types in it", only the
first works: without `allowJs` TypeScript never reads the `.mjs`, so JSDoc inside it cannot
reach the compiler. **Fix:** embed and write `.dharness/coverage-tiers.d.mts` beside the
runtime, as managed bytes like the runtime itself.

---

## Not dharness's to fix

- **The test writers could not see the whole tree.** One reported `agent-host.ts` (0%) as a
  tier problem; it was already infrastructure, and `dharness policy` said so. The adopter's
  own fix is right: hand writers the resolved policy for their folders.
- **React Flow under happy-dom** needed size, `getBoundingClientRect` and `ResizeObserver`
  stubs. Real, and the project's.

## Suggested order

C12 (a typecheck failure every strict adopter hits), then C2+C3 together, then the
documentation set C1, C5, C6, C7, C8, C9, C10, C11 in one pass. C4 needs a decision first.
