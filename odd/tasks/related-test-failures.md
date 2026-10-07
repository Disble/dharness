# Related-test failures

## Objective
Stop the `mutate --staged` related-test step from producing load-induced false failures, and make a real failure name the tests that failed.

## Problem and rationale
Reported by the autoreas-bridge session on 2026-10-07 (Windows 11):

- **False failures.** `VitestRelated` and `JestRelated` (`internal/tool/tool.go`) run with `LowPriority: true`. That is `BELOW_NORMAL_PRIORITY_CLASS` on Windows. The only stated rationale (`internal/runner/runner.go:44-52`) covers mutation testing, which saturates the machine. Commit `df70c43` added low priority to the related step with no reason given, and no test pins it there. The peer observed jsdom integration tests exceeding vitest's 5s default at low priority under ordinary load: 6280ms and 5582ms, four failures in a row. At normal priority the same set passes 388/388, and the same tests pass at normal priority in the project's own test job.
- **Opaque message.** On a non-zero vitest exit, `runRelatedTests` returns before reading the `--outputFile` JSON, and the deferred sandbox removal deletes it. Reproduced on a fresh binary: vitest's stderr is empty, and the failing test name plus its timeout message exist only in that JSON (`assertionResults[].status == "failed"`, `fullName`, `failureMessages`). The backlog M1 suggestion to attach the stderr tail would therefore print nothing.

## Scope
- RT-01: run the related step at normal priority.
- RT-02: on a non-zero vitest exit with a readable JSON report, name the failed tests and the first line of each failure message. Without a JSON report, keep reporting that the suite produced no result.
- RT-03: record the finding in `docs/learning-log.md`.

## Constraints and non-goals
- Worktree `D:/dev/disble/dharness-worktrees/related-test-failures`, branch `fix/related-test-failures`, base `main` at `2745361`.
- Stryker's own priority (`StrykerServe`, `Stryker`) is a documented decision and stays unchanged. Revisiting it is an open question for the user.
- No retries, no worker caps, no "inconclusive" state, no fallback path. The verdict stays exit-code and JSON driven (CLAUDE.md).
- The Jest path lists tests without executing them and is untouched, apart from priority consistency.
- No new dependencies. Never run dharness against this checkout. Build to the scratchpad and run in a scratch project.
- Push, PR, merge and release are the user's decisions.

## Tasks
- [x] RT-01: normal priority for the related step, plus a test pinning it. Route: delegated writer, shared with RT-02 (2+ non-trivial files across tool/staged/cli).
- [x] RT-02: name the failed tests from the JSON on a non-zero exit. Test-first: parser unit test, plus CLI tests for exit≠0 with and without JSON. Route: delegated writer.
- [ ] RT-03: learning-log line. Route: inline (one mechanical append).
- [ ] RT-04: build the binary and run it on the scratch fixture (`scratchpad/m1`, with a test that fails on timeout). Read the actual message. Route: inline.

## Acceptance and checks
- `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l .` are clean.
- `ditto staged --dry --exclude-prefix tools/` is recorded per commit.
- The scratch binary names `slow under load` and `Test timed out` in the related failure line, and exits 1.
- A missing or unreadable JSON report still produces the no-result failure, never a zero.

## Progress
- Created 2026-10-07.
- RT-01 done (delegated writer). RED observed: `TestRelatedCommandsRunAtNormalPriority` failed on both constructors with `LowPriority = true`; GREEN after removing the flag. Checks: `go build ./...` ok, `go vet ./...` ok, `go test ./...` all ok, `gofmt -l .` empty. `ditto staged --dry --exclude-prefix tools/` scoped `internal/tool/tool.go` only; a scoped real run (`--test-command "go test -count=1 -json ./internal/tool/"`) produced 0 mutants: removing a boolean field leaves no mutable site. Commit `749a733`.
- RT-02 done (delegated writer). RED observed: parser tests failed to build (undefined `ParseVitestFailures`, `FailedTest`, `RelatedTestsFailed`); `TestMutateStagedNamesTheFailedRelatedTests` failed with the old `related-test suite load/run failure: vitest exited with code 1`. The no-failed-test and unreadable-report CLI cases passed before and after (characterization of the kept path). New JSON-path message: `related tests failed: "slow under load": Error: Test timed out in 50ms. (vitest exited with code 1)`; at most 3 tests named, then `and N more`; the ExitError stays wrapped. Checks: `go build ./...` ok, `go vet ./...` ok, `go test -count=1 ./...` all ok, `gofmt -l .` empty, `ditto staged --dry --exclude-prefix tools/` scoped `internal/cli/mutate_staged.go` and `internal/staged/related.go`. Scoped mutation (`--test-command "go test -count=1 -json ./internal/staged/ ./internal/cli/"`): 27 mutants, 24 killed, 3 survived; the boundary survivor (`more > 1`) was then killed by a four-failure case (`./internal/staged/` run: 20/20 killed on related.go). Remaining 2 survivors are equivalent: `return 0` mutated to `-1`/`1` beside a non-nil error the caller never reads the count of.

## Delivery
Strategy `ask-on-risk`. Forecast is about 150 authored lines, under the 400-line slice budget, so a single PR.
