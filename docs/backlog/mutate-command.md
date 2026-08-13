# Backlog: the `mutate` command

Observations about `dharness mutate` — `internal/cli/mutate.go` and the Stryker
invocation in `internal/tool` — worth acting on but not worth derailing the
change that found them.

Same rule as `mutation-wrapper.md`: each entry records what was measured, not
what was guessed. Nothing here is a promise.

---

## Closed on 2026-08-13

Both entries this file opened with came from a frontend project trying to
replace its pre-commit mutation step with `dharness mutate`. Both were
reproduced against a scratch fixture, fixed, and verified by running the binary.
The mechanisms live where they are enforced now, not here: `docs/learning-log.md`
for what was measured, §03 of `docs/design-principles.md` for the rule that
changed, and the doc comments on `tool.StrykerPackages` and `tool.SurvivorsInScope`.

**1. Stryker could not resolve `typescript`.** Reproduced exactly — Core running
from a bunx temporary directory imports the compiler from its own location, so
any project with a `tsconfig.json` died before its first mutant. Stryker now
resolves from the project, installed at `@latest` on every run, and a project
that cannot be given one is refused with the install command named rather than
handed a Node stack trace. The transient route is gone, and with it a second
defect it was hiding: bun installs only the last `--package`, so the environment
built for "Core and its runner together" never held Core at the requested
version at all.

**2. `mutate` took paths, not line ranges.** It takes both now —
`src/thing.ts:12-40`, Stryker's own syntax, passed through verbatim including
columns. This was never a feature to build, only an argument to stop reshaping.

**3. A scoped run had an unscoped verdict**, which neither entry predicted and
only running found. `--incremental` makes Stryker's report cumulative by design;
dharness judged all of it. A run scoped to `src/a.ts:5-7` instrumented five
mutants and then failed on a survivor at line 10 from an earlier whole-file run.
The verdict now covers exactly what the run asked about, and the fix is
falsifiable in both directions: `:9-14` still fails on that same line 10, `:1-3`
and `:5-7` pass.

---

## Open

## 1. Whether this command belongs in a pre-commit hook

The migration that opened this file wanted `mutate` on every commit, and the
scoping it asked for is now there. The disagreement it exposed is not.

`RunMutate`'s own doc comment says mutation testing "belongs after the green step
and before the refactor — invoked when a unit of work is finished, never on every
commit", and the install added on 2026-08-13 leans on that sentence: it is paid
once per finished unit, and it writes to `package.json` and the lockfile when a
new Stryker ships. On every commit that is a different bargain.

Two things stay unmeasured, and they are the ones that decide it:

- **Partial staging.** The hook `mutate` would replace refuses it explicitly.
  dharness does not handle it at all — it mutates what the working tree holds,
  which for a partially staged file is not what is being committed.
- **What a commit-sized run actually costs.** `--dry-run` exists to answer this
  and nobody has pointed it at the case: the number that matters is how many
  tests the runner considers related to a diff-sized range, not to a file.

Until both are measured, the honest position is that ranges make the command
*affordable* to run on a commit, not *correct* to run there.
