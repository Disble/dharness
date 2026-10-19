# Adopt coverage tiers one feature at a time

Declare business-impact tiers in the consuming repository's root `coverage-tiers.json`. dharness resolves the declaration; the consumer decides which behavior is CORE. Adoption does not classify all legacy code at once, but an adopted feature includes its old files too.

| Tier | Native test coverage target |
|---|---:|
| CORE | 100% |
| IMPORTANT | 80% |
| INFRASTRUCTURE | 0% |

These are coverage targets, not mutation-score thresholds. Mutation retains its existing verdict; infrastructure is excluded from its effective scope. New equivalence/waiver evidence machinery is outside this delivery.

## When sync offers adoption

Without `coverage-tiers.json`, `dharness sync` puts the native coverage step under **Left to you**, in both the human report and the JSON `delegated` status. It means **optional adoption is undecided**, not that coverage is enforced. The report contains the adoption procedure before the tier skill is installed. Offer the choice to the user; do not invent a business classification or enable it silently. Deferring adoption leaves coverage wiring unchanged and the choice reappears on the next sync.

| Owner | Action |
|---|---|
| Agent, with the project's policy agreed | Inspect existing source/tests; declare narrowly scoped features and their business-impact tiers; validate with `dharness policy --root .`; run `dharness sync`. |
| Agent, when prerequisites are missing | Install compatible local Vitest/provider packages with the project's package manager, preserving existing versions; supply the native config, test environment and tests. |
| dharness sync | Write the managed runtime/bridge, splice the native config, wire the hook coverage job, and distribute the tier skill and references in existing instruction files. |
| Not automated by sync | Execute tests, write missing tests, install coverage dependencies, add `test:coverage` or integrate an existing `check` script, or create CI workflows. Offer these separately when needed. |

Run the exact native coverage command from the generated hook in the JS project root to verify the complete adopted unit. A successful sync is not a coverage pass. Native integration is Vitest-only: do not promise Jest or Go coverage wiring.

## Declaration

```json
{
  "version": 1,
  "enforcedPrefixes": ["src/features/"],
  "features": [{
    "id": "checkout",
    "path": "src/features/checkout/",
    "defaultTier": "core",
    "reason": "Checkout decisions and validation",
    "overrides": [
      {"path": "src/features/checkout/ui/", "tier": "important", "reason": "Presentation"},
      {"path": "src/features/checkout/types.ts", "tier": "infrastructure", "reason": "Type declarations"}
    ]
  }]
}
```

Paths are repository-relative and use forward slashes. A trailing slash declares a directory prefix; otherwise a rule names one exact file. Exact override wins over the most specific directory override; otherwise the feature's default applies. Conflicting declarations and overlapping features are rejected, not resolved by array order.

Version 1 resolves dharness-recognized JavaScript/TypeScript source paths. Native coverage integration is Vitest-only, not a Go coverage runner.

Choose enforced prefixes deliberately: every source there must belong to a declared feature. Sources outside them remain unadopted and keep existing behavior. Declare existing paths only: leave planned directories such as `src/backend/` out until they exist, then extend the declaration deliberately. Do not declare `src/features/` if it contains legacy features you are not yet adopting; start with narrower prefixes. An adopted feature is never exempted because some of its files predate adoption. Generated files, dependencies and test files are not business source to be covered by these targets.

## Single resolution authority

The integration uses the read-only JSON facade `dharness policy --root <repository-root>`. It returns canonical source classifications and coverage targets; it does not start a test runner or write configuration. This is an integration detail, not another manual step before tests.

An absent manifest means no adoption unless a managed coverage integration already requires that manifest. A malformed manifest is a failure, never an empty policy. The resolver measures the complete declared source universe, including unexecuted files, without using a code-age cutoff.

## Native Vitest coverage in the existing gate

The project must already have a locally installed Vitest, its coverage provider and a native Vitest/Vite config. `dharness sync` does not download either runner or provider for this integration. The project's Node runtime must be able to load the installed public `vitest/node` API; the measured native combination is Node 24.21.0, Vitest 5.0.3 and `@vitest/coverage-v8` 5.0.3.

With a valid declaration, run the existing `dharness sync`. It:

- copies the embedded runtime to `.dharness/coverage-tiers.mjs`;
- adds managed import/export regions around the config's **raw exported value**, without evaluating it or rewriting a shared initializer;
- writes `.dharness/vitest.coverage.mjs`, a checked bridge to that consumer config;
- adds a native coverage invocation to the existing local gate, not a new test-launch command or CI workflow.

For a Bun project with its JS source at the repository root, the generated native invocation is:

```sh
bunx --no-install vitest run --coverage --config .dharness/vitest.coverage.mjs
```

The generated `.dharness/*.mjs` files can be staged alongside project source; separate commits are not required. The ESLint stage excludes the directory dharness owns from its explicit targets because ESLint 10 would otherwise resolve its generated config factory as a standalone config. Project `.mjs` files remain lint targets; react-doctor, fallow and mutation scope are unchanged.

A split JS root uses a relative bridge path and the existing hook's working-directory mechanism. Husky always uses POSIX-shell quoting, including on Windows; the emitted Windows lefthook command uses the host command interpreter. Lefthook retains its own scheduling policy: this integration does not assume that a commands map stops at the first failure.

The existing `dharness check` remains the staged tool gate. The configured local hook additionally invokes the project's native Vitest for **the complete adopted unit**. An existing full-verification workflow may consume the same checked bridge; no CI files are created or managed.

The runtime asks the Go resolver on each enabled coverage run and projects its targets into native `include` and per-file thresholds for statements, branches, functions and lines. Vitest calculates the metrics and owns threshold failure. A native coverage reporter also refuses a missing required adopted source: an exclusion must not turn an unmeasured CORE/IMPORTANT file into a green run. Existing include scope and reporters remain, stronger consumer thresholds are preserved, and conflicting weaker settings refuse instead of being overwritten.

Ordinary focused/watch runs remain ordinary when coverage is off. The installed Vitest parser owns CLI precedence: an explicit `--coverage=false` or `--coverage.enabled=false` overrides a config that enabled coverage. A config that enables coverage without a CLI override still activates enforcement. No second flag grammar or source classifier is introduced.

Ambiguous exports, missing prerequisites and conflicting managed regions are reported for correction rather than guessed. The declaration or any managed footprint keeps the coverage job active even when preparation refuses: an unwired config, missing bridge or removed manifest must fail, not silently drop enforcement. To stop adoption, remove the declaration **and** its managed integration deliberately; deleting only the manifest is a conflict.

Without a declaration or managed footprint, no coverage job or files are added. `sync` offers the native coverage adoption under **Left to you**; the dependent skill-distribution step explicitly reports itself as inactive until adoption. It still repairs drift in the owned gate file; consumer-owned content outside managed regions is retained.

## One complete gate and routine conflicts

There is exactly one complete Vitest testing gate: the managed coverage-tier
invocation above. Where the hook already carries commands, `dharness sync`
reads the active pre-commit commands (the lefthook pre-commit section, or the
husky hook lines) and the local `package.json` scripts they reach — never an
unused script, never the managed job itself, never CI. Each invocation is
judged on its own, so `dharness check && bun run test` is analyzed as two
commands and the second one still counts. A manager's own `exec`/`x` prefix is
read as the command it runs, so `pnpm exec vitest run` is the same complete
run, while a launcher executing a file (`node scripts/check.mjs`, `bun
scripts/check.mjs`) is a wrapper whose runner cannot be determined: it stays
uncertain.

| What sync finds | What sync does |
|---|---|
| No Vitest, only focused/watch runs, a selection flag, or a suite in another root or under another config | Adds the hook reference automatically, preserving every byte already declared |
| A reachable complete run of the same suite, including through `cd <managed root>` or a job `root:` that names it | Hands precise reconciliation to the consuming agent: keep the managed gate, remove the duplicate |
| A command that cannot be resolved (a wrapper script, a missing script, a cycle, a dynamic path, an unsupported flag) | Reports it as uncertain — unknown is not confirmed Vitest — for the agent to clarify |
| An `extends` key, an explicit `---` document, a flow mapping, or another shape the additive merge cannot take | Names that shape and asks for the reference by hand instead of rewriting the file |

Identity is decided where it can be read plainly: `cd frontend` and `root:
frontend/` name the managed source when they resolve there, a different
root or an existing different `--config` names another suite, and an identity
that cannot be established is handed over rather than assumed safe.

Scoped runs stay: a focused file filter, `--watch`, `--changed`, an alternate
config and a suite in another root are not the managed gate's complete run, so
they are left alone and the reference is still added around them. A complete
run may still carry resource flags such as `--maxWorkers=2`; they do not
narrow which tests execute. dharness never deletes a consumer command or
rewrites a package script; until one complete gate remains, the wiring step
stays with the agent. The coverage step follows the same rule: it chains the
coverage job only onto a gate line dharness itself rendered, so a hook line
that already runs the check beside the consumer's own command is left byte for
byte and the conflict is reported to the agent with nothing written for the
tiers.

## Agent guidance, distributed by the same binary

The procedure lives once, in `internal/setup/assets/coverage-tiers/SKILL.md`, embedded with `go:embed` and shipped in the binary: nothing is downloaded at sync time and no external skill installer is invoked. `dharness sync` writes that body, unchanged, to two client locations under the repository root: `.claude/skills/coverage-tiers/SKILL.md` and `.agents/skills/coverage-tiers/SKILL.md`. They are copies rather than links, so they need no symlink privilege on Windows, and they are managed bytes: a hand edit is repaired on the next run, and a run with the bytes already correct writes nothing.

The same step adds a bounded region to the instruction files the project already keeps. `AGENTS.md` and `CLAUDE.md` each gain (or have replaced) one marker-delimited block naming the tiers, the declaration and the skill path their own client reads. A file that does not exist is never created; bytes outside the region are never rewritten; and a partial or duplicated region is refused with the fix named instead of repaired by guesswork. The region is located with the same line-bounded marker grammar the ESLint and coverage-config splices already use.

This guidance follows the declaration, not the wiring: a repository whose `coverage-tiers.json` exists gets the skill even while the native integration is still being prepared, and a repository with no declaration gets nothing. Removing the declaration while the managed copies remain is a conflict, not a silent uninstall.

The skill and the regions are guidance, not enforcement. The gate decides, so an agent that never loads the skill still meets the same verdict; the copies exist to explain the policy to whoever is doing the work.

## Existing mutation workflows

`dharness mutate <paths...>` filters declared infrastructure before installation, dry-run measurement, sandbox creation or cache access. Eligible CORE, IMPORTANT and unadopted paths retain their original ranges, including column suffixes. An all-infrastructure request succeeds with `no runtime source to mutate after coverage-tier exclusions` and starts no mutation tool.

`dharness mutate --staged` resolves the manifest and adopted source from its index snapshot, not unstaged working-tree edits. Its existing local Stryker prerequisites remain; the tier filter then runs before technical classification, membership discovery and related tests. Excluded infrastructure cannot re-enter the dropped-file self-check.

Filtering changes scope, not verdict: eligible `Survived` and `NoCoverage` mutants still fail. Existing cumulative-report notes can name excluded files outside this run's scope; they do not make those files eligible. A malformed policy refuses the run instead of silently retaining every path.

## Boundaries

- A tier declaration is not proof that its business classification is correct; that risk belongs to the project.
- Complete adopted-unit verification is different from `mutate --staged`, which checks changed ranges only.
- Native coverage integration is Vitest-first. Jest coverage is backlog; existing Jest workflows are unchanged.
- Coverage belongs to the project's existing complete verification workflow and local gate; ordinary focused/watch runs do not gain mandatory coverage.
- Existing CI may consume the same native runner configuration. This adoption does not create or manage CI workflows, select a CI provider, or add a coverage-launch command.
- The skill is guidance, not the gate. Enforcement works even when an agent never reads it, and the gate's verdict does not depend on any client loading the skill.
- No external skill installer, Vercel publication, new wrapped CLI or new module dependency is required.

## Delivery status

The ODD task document `odd/tasks/coverage-tiers.md` records which integration units and checks have actually completed. Do not infer that coverage/mutation/skill wiring is complete solely because the manifest resolver exists.
