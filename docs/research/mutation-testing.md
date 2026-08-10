# Mutation Testing (Go)

Gremlins mutation testing for the Go side of the bridge. It answers one
question the coverage number cannot: *if this code were broken, would any test
notice?*

This matters more here than in most repos. `go test -race` cannot run on this
project — the race detector needs a C toolchain on Windows, which the pure-Go
`modernc.org/sqlite` driver exists to avoid. Tests are therefore judged by
whether they actually fail when the code changes, and gremlins automates that
judgement.

## Install

```sh
go install github.com/go-gremlins/gremlins/cmd/gremlins@latest
```

Verified with `gremlins 0.6.0 windows/amd64` against `go1.26.5`.

## Run it from a worktree, not the repo root

**This is not optional.** Gremlins copies the entire module tree into a temp
working directory. In this repo that means copying `frontend/node_modules`
(~130,000 files), which turns a 16-second run into a 12-minute one that never
reports. `exclude-files` does not help: it filters which mutants are selected,
not what gets copied.

`node_modules` is gitignored, so a worktree is a clean checkout without it:

```sh
# One-time setup
git worktree add --detach ../autoreas-bridge-worktrees/gremlins HEAD
cd ../autoreas-bridge-worktrees/gremlins

# main.go embeds frontend/dist, which a fresh worktree does not have
mkdir -p frontend/dist && echo '<!doctype html>' > frontend/dist/index.html

go build ./...   # must succeed before gremlins will work
```

Then run scoped to the package under review:

```sh
gremlins unleash ./internal/observability/eventlog/ --output gremlins-report.json
```

Measured cost on this repo: coverage ~4s, full run on `internal/observability/eventlog`
~1m40s for 115 mutants. Never run it across the whole module.

To test uncommitted work, sync it into the worktree first:

```sh
git -C <repo> diff HEAD > /tmp/wip.patch
git apply /tmp/wip.patch            # from inside the worktree
# plus a manual copy for any untracked new files
```

## Reading the output

| Status | Meaning | Act on it? |
| --- | --- | --- |
| `KILLED` | A test failed when the mutant was applied. | No — this is the goal. |
| `TIMED OUT` | Tests hung. The mutation broke behaviour badly enough to stall a wait loop. | Usually no — it is a detection, though it is excluded from the efficacy score. |
| `LIVED` | The mutant survived every test. **A test is missing or asserts nothing.** | Yes. |
| `NOT COVERED` | No test executes that line at all. | Yes. |
| `NOT VIABLE` | The mutant did not compile. | No. |

`Test efficacy` is `KILLED / (KILLED + LIVED)`. `Mutator coverage` is
`(KILLED + LIVED) / total`.

## Known instability — trust `NOT COVERED`, verify `KILLED`

Results on this repo are **not reproducible run to run**. Three consecutive
runs of the same command on `internal/observability/eventlog` produced:

| Run | Killed | Lived | Not covered | Timed out | Efficacy |
| --- | --- | --- | --- | --- | --- |
| 1 | 17 | 1 | 9 | 88 | 94.44% |
| 2 | 0 | 0 | 9 | 106 | 0.00% |
| 3 | 0 | 0 | 9 | 106 | 0.00% |

Two causes, both upstream and both open:

- [#267 — everything times out when running twice in a row](https://github.com/go-gremlins/gremlins/issues/267).
- [#81 — gremlins is extremely slow on Windows](https://github.com/go-gremlins/gremlins/issues/81), open since 2022.

Gremlins derives its per-mutant timeout from the baseline test duration. This
package's tests run in ~1.2s, so the derived budget is small, while a mutant
that stalls a wait loop costs `async.DefaultTimeout` (2s) per occurrence — and
`TestSinkConcurrentWriteDuringBindDeliversExactlyOnce` repeats 100 times.
Nearly everything therefore lands in `TIMED OUT` rather than `KILLED`.

Practical consequence: **`NOT COVERED` is the trustworthy signal** — it comes
from the coverage profile and was identical across all three runs. Treat
`KILLED`/`LIVED` counts as indicative, and confirm any specific `LIVED` finding
by hand: delete the guard, run the test, check that it fails. That manual check
is the ground truth gremlins is approximating.

Clear the temp directories between runs, since gremlins fails to remove them on
Windows and stale state feeds #267:

```sh
rm -rf "$TEMP"/gremlins-*
```

## No CI gate yet

`threshold.efficacy` and `threshold.mutant-coverage` are both `0` in
`.gremlins.yaml`, i.e. advisory. Given the instability above, a gate would fail
on tool nondeterminism rather than on code quality. Pass
`--threshold-efficacy` explicitly when reviewing a single package, and keep it
out of `lefthook.yml`.

## What the first run found

Real gaps in `internal/observability/eventlog`, all confirmed against
`go test -coverprofile`:

- **`queue.go` is largely untested.** `Err()`, `setErr()`, the `Stop` deadline
  branch, the `TryEnqueue` stopping branch, the `persist` nil-store and error
  paths — none are covered. The package was written as a mirror of
  `internal/observability/requestcapture`, whose `queue_test.go` covers exactly
  these cases; the code was mirrored and the tests were not.
- **`reader_search.go:96-97`** — `MalformedRowsSkipped++` / `WarningCount++`
  never execute. `TestSearchTolerateMalformedRowCountsWarning` documents in its
  own comment that malformed *metadata* does not trigger a skip, so the skip
  path has no test feeding a row that fails `scanEventRow`.

Both predate the event-visibility work and are tracked as follow-ups rather
than folded into an unrelated change.

## Staged-diff guard (ooze)

`go run ./tools/mutationstaged` is the Go counterpart to
`frontend/scripts/dlinter-mutation-staged.mjs`: it mutates only the staged
production Go files and fails below a threshold (default `0.80`, matching
`stryker.dlinter.json`'s `break: 80`).

```sh
go run ./tools/mutationstaged        # gate the staged change
go run ./tools/mutationstaged -dry   # print the computed scope, run nothing
```

It exits 0 immediately when no `.go` files are staged, and refuses a file that
has unstaged changes on top of its staged ones -- the same refusal the frontend
guard makes, for the same reason: mutation would otherwise judge a tree state
that is not what gets committed.

### Why it runs against a sandbox

ooze's `fsrepository.LinkAllToTemporaryRepository` walks the repository root and
calls `os.Symlink` for **every file, with no exclusions**. `IgnoreSourceFiles`
filters which files get *mutated*, never which get *linked*. Against the working
tree that means symlinking `frontend/node_modules` (~130k files) once per
mutant, and a run never finishes.

The guard therefore materialises the index into a temp directory with
`git checkout-index --all --prefix=...` and points ooze there. That is 1,625
files instead of 130,000, and it also gives the guard the right semantics for
free: it judges exactly the staged content rather than the working tree that
merely resembles it. A `frontend/dist/index.html` stub is created because
`main.go` embeds that directory and the built assets are not in the index.

### Windows path separators

ooze derives each source path with `filepath.Rel`, which yields **backslashes**
on Windows. A forward-slash-only ignore pattern matches nothing, every exclusion
silently drops, and the whole repository gets mutated while the run still looks
correctly scoped. `buildIgnorePattern` emits `[/\]` for each separator;
`TestBuildIgnorePatternMatchesWindowsSeparators` pins it.

### Measured cost, and why it is not in pre-commit

One staged file (`internal/observability/eventlog/store.go`, 27 mutants):

| Stage | Result |
| --- | --- |
| Nothing staged | 1.0s, exit 0 |
| One staged file | ~100s -- 27 mutants, 20 killed, 7 survived, score 0.74 |

The pre-commit gate already runs ~90s. Doubling that for a one-file change is
not a trade worth making, and two ooze limitations make it worse than the
frontend guard: ooze mutates **whole files**, not the changed line ranges, and
keeps **no incremental cache**, so nothing is reused between commits. A one-line
edit pays for the whole file, every time.

`ooze.Parallel()` is off by default: it deadlocks here. A 30-minute run ended
with goroutines still blocked in `testing.(*T).Parallel` after 29 minutes. Set
`AUTOREAS_MUTATION_PARALLEL=true` only to re-test whether a newer ooze fixed it.

Run it manually before pushing, or wire it into `pre-push` -- not `pre-commit`.
Note that `store.go` currently scores 0.74, so enabling the gate at `0.80`
blocks that file until its seven surviving mutants are addressed.
