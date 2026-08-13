# Backlog: the `mutate` command

Observations about `dharness mutate` — `internal/cli/mutate.go` and the Stryker
invocation in `internal/tool` — worth acting on but not worth derailing the
change that found them.

Same rule as `mutation-wrapper.md`: each entry records what was measured, not
what was guessed. These two arrive from outside, reported by a frontend project
that tried to replace its own pre-commit mutation step with `dharness mutate`.
What is measured *here* is this repository's command shape; the failure itself
was observed there. Both entries say which is which.

Nothing here is a promise.

---

## 1. Stryker cannot resolve `typescript` from the transient environment

**Reported 2026-08-13.**

    ERROR Stryker  Cannot find package 'typescript' imported from
    C:\Users\User\AppData\Local\Temp\bunx-1070947736-@stryker-mutator\
      vitest-runner@latest\node_modules\@stryker-mutator\core\dist\src\
      sandbox\ts-config-preprocessor.js
    dharness: stryker exited with code 1

`typescript` 6.0.2 is in that project's devDependencies. The package is not
missing — it is missing from where Stryker looks.

What this repository contributes to the failure, read from its own code:
`transientPackages` (`internal/tool/command.go:68`) builds the bun route as
`bunx --package @stryker-mutator/core@latest --package
@stryker-mutator/vitest-runner@latest stryker`, and `StrykerCommand`
(`internal/tool/tool.go:50`) sets `Dir` to the JS project. So the working
directory is the project and the code is not: bun unpacks both packages into a
temporary directory outside it, and `ts-config-preprocessor.js` imports
`typescript` from its own location, where the project's `node_modules` is not on
the resolution path.

This is the failure class dharness already knows about. `StrykerCommand`
refuses Yarn PnP outright (`internal/tool/tool.go:57-59`) because "the remote
runner cannot resolve the project's test dependencies", and §03 records two
standing exceptions to remote execution — ESLint and fallow's `effective` — both
taken for the same reason: a transient environment cannot resolve what the
project installed. This is that reason again, in a configuration dharness
accepts rather than refuses.

Why nothing here caught it: **the repository contains no `tsconfig.json`
anywhere** — `rg tsconfig` over the whole tree, fixtures included, returns
nothing. Stryker runs the TSConfig preprocessor only when a project has one, so
every test dharness has exercises the path where it does not exist.

Candidates, none of them chosen and none of them costed:

- add `--package typescript` to the transient set, which is a shape
  `transientPackages` already has, but it makes dharness pin a version of the
  project's own compiler;
- resolve Stryker locally when the project installed it, the ESLint exception a
  third time;
- make the project's `node_modules` reachable from the transient environment.

What would settle it: the scratch fixture `CLAUDE.md` already prescribes — built
outside this repository, never pointed at it — given a `tsconfig.json` and a real
`typescript` devDependency, with `dharness mutate` run against it. Until that
reproduces, the mechanism above is the reporter's reading plus this repository's
command shape — not a measurement.

## 2. `mutate` takes paths; the hook it would replace takes line ranges

**Reported 2026-08-13**, same migration, measured there against one real staged
file:

| | their existing hook | `dharness mutate` |
| --- | --- | --- |
| Scope | the diff's line range (`@@ -122 +122 @@`) | the whole file (145 lines) |
| Incremental | yes, cached in `.git/dlinter/` | not exposed |
| Partial staging | refused explicitly | does not apply |

So swapping one for the other is not moving an invocation. It changes what gets
mutated on every commit, from the lines the author added to the entire file.

`RunMutate` (`internal/cli/mutate.go:53`) takes `<path...>`, and `mutate`
(`internal/tool/tool.go:274`) emits one `--mutate <path>` per argument.

**Stryker already accepts the range.** Its `--mutate` documents
`file:startLine[:startColumn]-endLine[:endColumn]`, and `--mutate src/app.js:5-7`
appears in its own incremental documentation. By this repository's first rule,
that makes the ask not a feature to build but an argument to not break.

**Measured 2026-08-13**, against `scopePaths` (`internal/cli/mutate.go:162`)
under Windows path semantics, with the command run from `D:\repo` and the JS
project at `D:\repo\frontend`:

    frontend/src/a.ts        →  src/a.ts
    frontend/src/a.ts:12-40  →  src/a.ts:12-40
    frontend/src/a.ts:1:3-1:5 → src/a.ts:1:3-1:5

The colon survives `filepath.Join` and `filepath.Rel` untouched, and nothing on
the way to the command line stats the path — there is no `os.Stat` in
`mutate.go`, and `parseInterspersed` treats the argument as an opaque string. So
the range syntax may already work end to end and nobody has tried, because entry
1 stops the run before it reaches Stryker.

What would settle it: the same scratch fixture, `dharness mutate src/a.ts:5-7`,
and its mutant count read against the whole-file run — read off the binary, not
off a test.

Two things that would not settle, and they are the real question:

1. **Incrementality across commits.** `StrykerMutate` already passes
   `--incremental` and `--incrementalFile`, but nothing in dharness handles
   partial staging, which the hook it would replace refuses outright.
2. **Whether this command belongs in a pre-commit hook at all.** `RunMutate`'s
   own doc comment says mutation testing "belongs after the green step and
   before the refactor — invoked when a unit of work is finished, never on every
   commit." The reporter wants it on every commit and is asking for the scoping
   that would make that affordable. Accepting ranges without answering this
   ships the syntax for a use the command was designed against.

The disagreement is the entry. The syntax is the easy half.
