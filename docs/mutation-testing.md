# Staged Go Mutation Testing

Mutation testing for this repository's Go code is `ditto staged`. There is no
wrapper here any more: `tools/mutationstaged` and `internal/testsupport/mutation`
were fourteen files that gave ditto conciousness of the index, and ditto has that
now.

## Run It

```sh
go install github.com/Disble/ditto/cmd/ditto@latest

ditto staged --dry --exclude-prefix tools/
ditto staged --exclude-prefix tools/ --threshold 0.80 \
  --test-command "go test -short -count=1 ./internal/cli/"
```

`--dry` reports the staged files and the byte ranges they justify and runs
nothing. The real run mutates only those ranges.

## What moved, and what did not

| Was | Is |
| --- | --- |
| `go run ./tools/mutationstaged` | `ditto staged` |
| `DHARNESS_MUTATION_THRESHOLD` | `--threshold` |
| `mutationExcludedPrefixes` in Go | `--exclude-prefix`, repeatable |
| the test command derived from the staged files' packages | `--test-command`, given by you |

**That last row is the one to know about.** The wrapper worked out which packages
owned the staged files and ran only those, so a mutant cost one package's tests.
ditto takes one command and does not derive it, so `./...` makes every mutant run
the whole suite. Name the owning package when you know it — the measurement below
was taken that way.

Everything else is the same answer. Held against the wrapper on two staged
changes of this repository, with the same test command, exclusions and threshold:
**4 mutants, 1 killed, 3 survived** on one file and **9, 5, 4** on two, identical
on both paths, with the same survivors by mutator. The second case is the
control — the numbers had to move, and both moved together.
`ditto/docs/experiments/replacing-the-wrapper.md`.

And it says more than the wrapper could: every survivor now carries a
`path:line:col` address, which ditto gained in v0.3.0 and the wrapper never saw
because it was pinned to v0.2.0.

## What it refuses, and why

| Condition | Result |
| --- | --- |
| Nothing staged worth mutating | Exits 0 without starting. |
| A staged file also edited in the worktree | Refuses: the bytes measured must be the bytes scored. |
| A red baseline | Refuses. A failing command is how ditto recognises a killed mutant, so a red suite scores every mutant killed and reports a perfect score for a run that tested nothing. Measured at 431 of 431 in 5.46 seconds before the guard existed. |
| A scope that yields no mutants | The score is -1, which is below any threshold, so the run fails rather than reporting a vacuous pass. |
| A diff that cannot be turned into ranges | Fails open to whole files and says why. |

## Why the index and not the working tree

ditto runs the suite against a checkout of the **index**, not of your working
tree. This is not caution. Measured on a fixture built for it: pointed at the
worktree, with one tracked file left dirty and unstaged, **seven of eight
verdicts moved** — a score of 0.13 against 1.00 for the identical eight mutants
of an identical file.

Refusing a partially staged file does not cover that case, because the file that
moved the verdicts was never staged. The two guards look alike and are not.

## Where it belongs in the loop

The cycle is RED → GREEN → **MUTATE** → REFACTOR. Mutation testing answers
whether a test would notice the code breaking, which only means anything once the
tests are green — so it runs when a unit of work is finished, and it is
deliberately absent from the commit hook.

## Dependency boundary

`go.mod` now requires one module, and it is not this one: ditto is a command you
install, not a dependency this repository compiles. `cmd/dharness` is stdlib-only
again, and the deviation `AGENTS.md` recorded for it is closed.
