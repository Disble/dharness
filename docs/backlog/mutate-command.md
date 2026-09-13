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

## Closed on 2026-09-13

**1. Whether this command belongs in a pre-commit hook.** Closed by `--staged`
(SDD `mutate-staged`), which answers both points this entry left unmeasured.

- **Partial staging** is refused explicitly now: `internal/staged.Scope` cross-
  checks every file it is about to scope against `git diff --name-only` and
  fails closed with `PartiallyStagedError` the moment one carries changes
  beyond what is indexed. This is the same refusal the pre-commit hook this
  entry compared against already had, now built in rather than left to the
  caller.
- **What a commit-sized run costs** is no longer approximated from
  `--dry-run`'s file-level count: `--staged` scopes to the exact line ranges a
  staged change added, the same ranges `mutate <path>:<range>` already
  supported, and never installs Stryker — a missing local binary is refused
  rather than paid for mid-commit.

It still never installs at gate time — `internal/cli/check.go`'s own
declining-to-install stays true of `--staged` too, for the identical reason:
a command that can run unattended, potentially behind a Ctrl-C, must not be
the moment `package.json` and the lockfile change. `docs/design-principles.md`
§03's "`mutate` no es el gate" is amended rather than reopened by this: a
staged run can now sit IN the gate without becoming the gate, because it still
never installs and still runs only what a finished, staged change justifies.

## Open

## 1. `evidence.json` has one slot and is committed

**Reported 2026-08-13** by the frontend project running v1.4.0, and confirmed
in the code the same day.

`Evidence.ScopedMutation` is a single `*ScopedMutation` holding one
`MeasuredPath` (`internal/project/evidence.go:55-63`), and
`RecordScopedMutation` replaces it outright on every `mutate --dry-run`. The
file is deliberately committed — `.dharness/.gitignore` whitelists it with
`!evidence.json`, and the golden fixtures pin that.

Measured there: `use-history-table.ts` was measured, then
`download-folder.helpers.ts`, and only the second survived.

So a tracked file is rewritten by whoever measured last. Two authors measuring
different files produce a diff on every run and a conflict on every merge, over
a file whose whole purpose (§08) is to be evidence nobody has to re-derive.

The shape of the fix is not obvious and that is why this is an entry rather
than a patch. Keying the record by path makes it a map that grows without
bound and never expires — a measurement for a file that has since changed is
worse than no measurement, because §08's own rule is that a reader must be able
to tell whether it still describes the code in front of them. Recording only
what `sync` needs to reach a terminal state may mean one slot is right and the
defect is that `mutate --dry-run` writes to it at all.
