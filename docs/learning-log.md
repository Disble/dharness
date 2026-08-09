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
