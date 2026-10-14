# Adopt coverage tiers one feature at a time

Declare business-impact tiers in the consuming repository's root `coverage-tiers.json`. dharness resolves the declaration; the consumer decides which behavior is CORE. Adoption does not classify all legacy code at once, but an adopted feature includes its old files too.

| Tier | Native test coverage target |
|---|---:|
| CORE | 100% |
| IMPORTANT | 80% |
| INFRASTRUCTURE | 0% |

These are coverage targets, not mutation-score thresholds. Mutation retains its existing verdict; infrastructure is excluded from its effective scope. New equivalence/waiver evidence machinery is outside this delivery.

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

Choose enforced prefixes deliberately: every source there must belong to a declared feature. Sources outside them remain unadopted and keep existing behavior. Do not declare `src/features/` if it contains legacy features you are not yet adopting; start with narrower prefixes. An adopted feature is never exempted because some of its files predate adoption. Generated files, dependencies and test files are not business source to be covered by these targets.

## Single resolution authority

The integration uses the read-only JSON facade `dharness policy --root <repository-root>`. It returns canonical source classifications and coverage targets; it does not start a test runner or write configuration. This is an integration detail, not another manual step before tests.

An absent manifest means no adoption unless a managed coverage integration already requires that manifest. A malformed manifest is a failure, never an empty policy. The resolver measures the complete declared source universe, including unexecuted files, without using a code-age cutoff.

## Existing mutation workflows

`dharness mutate <paths...>` filters declared infrastructure before installation, dry-run measurement, sandbox creation or cache access. Eligible CORE, IMPORTANT and unadopted paths retain their original ranges, including column suffixes. An all-infrastructure request succeeds with `no runtime source to mutate after coverage-tier exclusions` and starts no mutation tool.

Filtering changes scope, not verdict: eligible `Survived` and `NoCoverage` mutants still fail. Existing cumulative-report notes can name excluded files outside this run's scope; they do not make those files eligible. A malformed policy refuses the run instead of silently retaining every path.

## Boundaries

- A tier declaration is not proof that its business classification is correct; that risk belongs to the project.
- Complete adopted-unit verification is different from `mutate --staged`, which checks changed ranges only.
- Native coverage integration is Vitest-first. Jest coverage is backlog; existing Jest workflows are unchanged.
- The skill is guidance, not the gate. Enforcement must work even when an agent never reads it.
- No external skill installer, Vercel publication, new wrapped CLI or new module dependency is required.

## Delivery status

The ODD task document `odd/tasks/coverage-tiers.md` records which integration units and checks have actually completed. Do not infer that coverage/mutation/skill wiring is complete solely because the manifest resolver exists.
