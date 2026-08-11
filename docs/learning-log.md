# Learning Log (Vitácora)

A human-readable record of _why_ things in this repo are the way they are:
decisions taken and non-obvious problems solved, so future sessions inherit the
reasoning instead of rediscovering it.

This is a memory aid, **not** a substitute for deterministic guards. Whenever a
lesson can be enforced (linter, test, `go vet`, pre-commit gate), do that too —
this file only explains the _why_, it never replaces the _how_.

## How to append

- One lesson per line, kept to a single short sentence.
- Format: `- [YYYY-MM-DD]: text`
- Newest entries at the bottom. Never rewrite past entries; add a new line if
  something changes.

## Entries

- [2026-08-09]: A published binary is authority over its own documentation — react-doctor 0.9.11 rejects `--blocking`, which its own `--help` examples still advertise.
- [2026-08-09]: `npx <tool>` with no version can resolve a stale cached release: react-doctor came back as 0.2.1 while `@latest` gave 0.9.11.
- [2026-08-09]: Measure before optimising — full project detection costs 0.114 ms, so caching it would only buy a stale-cache failure mode.
- [2026-08-09]: Seam what cannot be controlled (process execution, git, cwd, build info) and use a temp directory instead of faking the filesystem.
- [2026-08-09]: Go's `flag` package stops parsing at the first positional argument, so flags written after paths are silently consumed as paths.
- [2026-08-09]: Windows cannot spawn an npm `.cmd` shim directly; it has to go through `cmd.exe`.
- [2026-08-09]: Reporting a resolved binary path instead of the tool's name makes every failure message a small puzzle.
- [2026-08-09]: StrykerJS has no `--since` — that flag belongs to Stryker.NET, and the only incremental mechanism is `--incremental` plus `--force`.
- [2026-08-09]: Stryker's `mutate` accepts `file:startLine-endLine`, but a range cannot be combined with a glob in the same entry, so the list is always computed.
- [2026-08-09]: Stryker exits 0 with surviving mutants unless `break` is set; its default is documented as "never let your build fail".
- [2026-08-09]: Stryker's `json` reporter writes a file while react-doctor's `--json` writes to stdout — structured output is not one mechanism.
- [2026-08-09]: Stryker aborts when the initial test run fails, so mutation only answers a question worth asking once the tests are green.
- [2026-08-09]: Stryker derives which tests to run from the import graph, so barrel files can inflate "related tests" until the scoping disappears.
- [2026-08-09]: Stryker's vitest runner ignores `coverageAnalysis`, always uses `perTest`, and forces single-threaded execution.
- [2026-08-09]: More workers made Stryker slower on a small scope (3.85 s with eight against 3.59 s with the default), so capping concurrency costs nothing.
- [2026-08-09]: `fallow audit` already exits 1 on a fail verdict and already gates on new findings only (`--gate new-only`).
- [2026-08-09]: `fallow audit` with no base flag resolves a sensible local base without a remote, while `--changed-since` resolves a commit range that cannot see the index.
- [2026-08-09]: fallow rejects an unknown config key by listing every valid one — the entry-point key is `entry`, not `entries`.
- [2026-08-09]: fallow reports its resolved entry points directly, so the heuristic invented to detect a misconfigured graph was never needed.
- [2026-08-09]: In fallow boundaries a type-only import still counts as a violation, while an import of an external package does not.
- [2026-08-09]: `fallow list --boundaries` prints the expanded zones with file counts and warns when a zone matches nothing — that is the configuration check.
- [2026-08-09]: gremlins produced 94%, 0% and 0% efficacy across three identical runs, so it cannot stand behind a gate.
- [2026-08-09]: Mermaid in a published artifact needs `theme: "base"` with explicit `themeVariables`, and a `<br/>` inside a label eats the surrounding space.
- [2026-08-09]: StrykerJS exposes no way to fail on survivors from the command line — neither `--break` nor `--thresholds.break` exists, so the verdict has to come from reading its report.
- [2026-08-09]: `--dryRunOnly` completes without writing any report, so the test count it produces exists only in the line of prose that states it.
- [2026-08-09]: Stryker's `cleanTempDir` only runs on a successful exit, and even set to `always` a sandbox can stay locked for a moment on Windows, so removal needs retries.
- [2026-08-09]: A tool's binary name is not its package name: `stryker` comes from `@stryker-mutator/core`, and asking a registry for the binary name fetches something else.
- [2026-08-09]: Repository-local state belongs in the git common directory: git ignores it, worktrees share it, and it dies with the repository instead of outliving it in a hashed cache.
- [2026-08-09]: lefthook and fallow both compose with `extends`, so a tool can own its own config file and the project's own file only gains one line.
- [2026-08-09]: `lefthook-local.yml` is auto-merged without touching the project's config, but it is conventionally gitignored, so it cannot carry anything meant to be shared.
- [2026-08-09]: `react-doctor install --yes` writes five things — skills for every detected agent, a package script, a dev dependency, a git hook that competes with an existing gate, and a CI workflow — and has no flag to ask for the skill alone.
- [2026-08-09]: `claude mcp add <name> <command> [args]` is not interactive, and fallow ships the `fallow-mcp` binary in its own package.
- [2026-08-09]: An ESLint rule runs unmodified as a react-doctor plugin, `messageId` and `meta.messages` included, but `sourceCode.getJSDocComment` is present and throws.
- [2026-08-09]: react-doctor rule severity accepts only `error`, `warn` or `off`, so a rule cannot carry a threshold and `context.options` arrives empty.
- [2026-08-09]: oxlint has neither `no-restricted-syntax` nor `jsdoc/require-jsdoc`, which is why shape and documentation rules cannot be expressed as configuration.
- [2026-08-09]: react-doctor does not run oxlint's core rules from `.oxlintrc.json`; adopting that config means reading severities for its own rules, not executing others.
- [2026-08-09]: fallow rejects an unknown config key by listing every valid one, and `fallow recommend --format json` classifies each decision as detected, defaulted or taste, with the question already shaped for an agent to ask.
- [2026-08-09]: In fallow's boundaries a type-only import still crosses a zone while an external package does not, and `boundary-violation` defaults to `error`.
- [2026-08-09]: A comment above `export function f() {}` belongs to the export statement, not to the function inside it, so asking the inner node finds nothing and every documented export reports as bare.
- [2026-08-09]: ESLint's `estree` types describe JavaScript only — interfaces, type aliases and enums exist solely in the AST the TypeScript parser hands over, so a rule that reads them has to name the shape it reads.
- [2026-08-09]: A rule that relies on config globs for scope reports every file in the project under react-doctor, whose rule configuration is a severity and nothing else.
- [2026-08-09]: npm returns E404 on an unauthenticated publish, so "not found" can mean "I do not know who you are" rather than anything about the package.
- [2026-08-09]: A trusted publisher cannot be configured before the package exists, so the first publish needs a token and only later releases can be OIDC-only.
- [2026-08-09]: npm validates `package.json` against the signed provenance attestation and rejects the publish when `repository.url` does not match the repository the build came from.
- [2026-08-09]: release-please cannot open its pull request until the repository allows Actions to create one, which is a repository setting rather than a workflow permission.
- [2026-08-09]: Explicit per-job `permissions:` override the repository default, so `default_workflow_permissions` can stay `read` even for a workflow that writes.
- [2026-08-09]: A pull request opened with `GITHUB_TOKEN` does not trigger workflow runs, which is why the publish job has to live inside the release workflow.
- [2026-08-09]: `tee /dev/stderr` does not exist in the Git Bash the Windows runner uses, so a check can fail on how it reports rather than on what it found.
- [2026-08-09]: The Windows runner checks out CRLF by default and gofmt counts a CRLF file as unformatted, so the format check failed on every file at once.
- [2026-08-09]: `fallow audit` cannot detect a base branch in a repository with no commits, which is exactly the first commit after adoption.
- [2026-08-09]: `git diff --cached --name-only` quotes any path that is not plain ASCII, so `src/café.ts` arrives as the literal `"src/caf\303\251.ts"` and `filepath.Ext` reads `.ts"` — the gate dropped the file and exited 0 over a change it never read. `-z` is the fix.
- [2026-08-09]: A Wails repository has two roots — git and lefthook at the top, the whole JS project in a subdirectory — and one `Root` field answered both, so a Go module reported as an npm project with no lockfile in sight.
- [2026-08-09]: `package.json` appears at every level of a workspace and a Wails project generates one nobody wrote (`frontend/wailsjs/runtime/`), so the lockfile is the signal for where a package manager installs; workspaces keep exactly one, at the root where `node_modules` lives.
- [2026-08-09]: `git ls-files` already excludes `node_modules` and honours `.gitignore`, so asking git for lockfiles needs no skip list and no depth bound, which a filesystem walk does.
- [2026-08-09]: `git rev-parse --show-toplevel` reports forward slashes on Windows while `os.Getwd` reports backslashes, and on a case-insensitive volume two spellings name one directory — comparing paths as strings after `EvalSymlinks` still gets it wrong, so identity is `os.Stat` plus `os.SameFile`.
- [2026-08-09]: lefthook has a per-command `root:` key, so a gate installed at the repository root running against a subdirectory project is one line of configuration rather than anything dharness has to solve.
- [2026-08-09]: Stryker's initial test run is where it starts its runner processes, so `--dryRunOnly` without `--concurrency` starts one per core — 19 on a 20-thread machine, and the dry run outlasted the mutation it was measuring. A resource budget does not stop applying because a run mutates nothing.
- [2026-08-10]: A dependency install belongs in init's transaction: package-manager compensation removes exactly that run's additions, then byte snapshots restore the manifest and lockfile even after a partial install failure.
- [2026-08-10]: A remote Stryker runner must share Core's transient install and be named with `--appendPlugins`; pnpm hides it from the default glob, while Yarn dlx cannot also supply the project's Vitest safely.
- [2026-08-10]: Yarn dlx isolation does not justify blocking every Yarn project: `node_modules` layouts can use npx's joint package environment, while generated PnP loaders prove dependency resolution is incompatible.
- [2026-08-10]: A zero-mutant ooze run needs a candidate preflight before `ooze.Release`, because its internal `t.Fatal` can make every later diagnostic unreachable.
- [2026-08-10]: `Delegated` moved from a compile-time type assertion on a second interface to a per-project `Step` method, closing both the rollback bug that aborted the whole run on a pre-configured `.fallowrc.json` and the unenforced Figure 1 repository stop in one contract change.
