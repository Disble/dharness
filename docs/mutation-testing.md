# Staged Go Mutation Testing

`tools/mutationstaged` runs ooze against the exact staged identity of dharness Go
product code. It is repository development tooling. It does not add a fourth CLI
to the dharness product and it is deliberately absent from the commit hook.

## Run It

```sh
go run ./tools/mutationstaged -dry
go run ./tools/mutationstaged
```

The dry run prints selected files, owning test packages, staged byte ranges,
excluded-file count, candidate mutants, and kept/dropped node counters. The real
run uses the same plan and defaults to a minimum score of `0.80`.

Set `DHARNESS_MUTATION_THRESHOLD` to a number from 0 to 1 only when a reviewed
workflow needs a different threshold. There is currently no repository
measurement supporting a different default.

## Contract

| Input or condition | Result |
| --- | --- |
| No staged production Go | Exit 0 before starting ooze. |
| Staged test or tooling file | Excluded from mutation. |
| Unstaged edits over a staged production file | Fail with a partial-staging correction. |
| Derivable staged lines | Convert index content lines to byte ranges and filter all 14 ooze mutators. |
| Underivable staged scope | Fail open to whole-file mutation and print the reason. |
| Derived scope with zero mutation candidates | Fail before `ooze.Release` with a zero-execution diagnosis. |

Production source means `.go` files excluding `_test.go`, `tools/`, and
`internal/testsupport/`. Each mutant runs only the packages owning selected
files through `go test -short -count=1`.

## Index Identity

The wrapper reads paths and diffs from Git's index, converts ranges against
`git show :path`, then materializes the full index with `git checkout-index` in a
temporary sandbox. ooze mutates and tests that sandbox. Worktree-only content is
never part of the verdict.

Paths from Git are NUL-delimited, normalized for Windows, and ooze exclusion
patterns match both slash forms. The sandbox prefix uses the platform separator
required by `git checkout-index`.

ooze's `Virus.Incubate` API does not identify the source file. Ranges from
multiple staged files are therefore unioned for the runtime filter. This can
retain an extra mutant when equal byte offsets occur in different files. The
over-approximation costs execution time and never drops a staged mutant.

## Silent No-Ops

`ooze.Release` can call `t.Fatal` before control returns to the harness. A guard
placed after that call cannot diagnose zero execution. The wrapper therefore
parses staged index content first and asks the same 14 mutators for candidate
infections without applying them. A real run starts only after this reachable
preflight reports at least one candidate.

The opt-in fixture pins a known baseline of one candidate and one killed mutant:

```powershell
$env:DHARNESS_MUTATION_FIXTURE='1'
go test ./tools/mutationstaged -run TestMutationFixtureRunsOozeEndToEnd -count=1 -v
Remove-Item Env:DHARNESS_MUTATION_FIXTURE
```

The fixture creates and stages a separate temporary Git repository. It never
reads or changes the user's index.

## Dependency Boundary

ooze v0.2.0 is the module's sole direct development dependency. Its imports are
confined to `tools/mutationstaged` and `internal/testsupport/mutation`; the
`cmd/dharness` dependency graph remains stdlib-only. `AGENTS.md` records this
tradeoff as an explicit deviation from the former module-wide stdlib-only rule.
