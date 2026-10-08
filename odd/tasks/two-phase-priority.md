# Two-phase Stryker priority

## Objective
Stryker's dry run runs at normal priority, and the mutant phase drops to below normal. The dry run should stop failing on load-induced timeouts, while the mutant phase keeps protecting the machine (§14).

## Problem and rationale
Design and measurements: `docs/research/prioridad-en-dos-fases.md`.
- Today the whole Stryker process tree starts below normal priority (`runner.go:44-52`, `tool.StrykerLocal`).
- The dry run is a single test runner using about one core. At low priority on a loaded machine it loses to everything else, and vitest's 5s per-test timeout fails it. autoreas-bridge measured 2.8s jsdom tests reaching 5.5–6.3s. Locally, low priority stalled a test for ~299s with 12 background threads (2 of 2 runs).
- Rejected alternatives, all measured or triaged:
  - Normal priority for the whole run breaks §14.
  - A CPU hard cap does not protect the machine: its p95 equals normal priority.
  - Retries, worker caps and an "inconclusive" state were rejected.
- Verified pieces:
  - Stryker 10's `event-recorder` reporter writes `*-onDryRunCompleted.json` before the first mutant.
  - Windows: a job's `JOB_OBJECT_LIMIT_PRIORITY_CLASS` reaches processes that already exist.
  - Linux: `setpriority(PRIO_PGRP)` reaches existing descendants.
  - Linux: `Setpgid` orphans the tree on Ctrl-C unless SIGINT/SIGTERM are forwarded to `-pgid`. With forwarding, measured 0 survivors.

## Scope
- TP-01, runner capability. A `runner.Command` can start at normal priority and, once a caller-supplied condition reports true, lower itself and everything it started, exactly once.
  - Windows: `Run` puts the process in a job object (reusing the suspended-start machinery in `managed_windows.go`) and sets the job's priority class limit.
  - Unix:
    - `Setpgid`.
    - SIGINT/SIGTERM forwarded to `-pgid` while the command runs.
    - Context cancellation kills `-pgid`.
    - The switch is `setpriority(PRIO_PGRP)`.
  - The condition is polled about every 250ms while the process runs.
- TP-02, Stryker wiring:
  - `--reporters clear-text,json,event-recorder` on mutation runs.
  - The staged config sets `eventReporter.baseDir` to a run-owned directory.
  - The interactive route clears `<stryker cwd>/reports/mutation/events` before launch and watches it.
  - The condition is "a `*-onDryRunCompleted.json` exists".
  - `mutate --dry-run` runs at normal priority throughout.
  - `stryker serve` (discovery) is unchanged.
- TP-03: build the binary and run it on a scratch fixture under background CPU load. Observe the dry run pass and Stryker's processes move from Normal to BelowNormal.
- TP-04, docs:
  - Close the research doc.
  - Amend §14 (`.md` plus artifact).
  - Update `flujo-implementado.md` where it describes priority (`.md` plus artifact).
  - Add a learning-log line.

## Constraints and non-goals
- Worktree `D:/dev/disble/dharness-worktrees/two-phase-priority`, branch `feat/two-phase-priority`, base `origin/main` at `1155db8`.
- Verdict stays exit code plus JSON. The signal only moves priority and never decides pass or fail.
- If the signal never arrives (dry run fails, Stryker exits early, the project overrides `eventReporter.baseDir`), the run ends at normal priority. That is a documented degradation, not an error.
- No kill-on-job-close for `Run`. Today a dharness that dies leaves Stryker running, and changing that is out of scope.
- Stdlib only, no new dependencies. Never run dharness against this checkout. Build to the scratchpad.
- Push, PR and merge are the user's decisions.

## Tasks
- [x] TP-01: runner capability, test-first with platform tests. A grandchild started before the switch ends up lowered. A condition that never fires leaves priority normal. Context cancel reaches the grandchild on Unix. Route: delegated writer (runner.go, exec_windows.go, exec_other.go, managed_* reuse, tests: 2+ non-trivial files).
- [x] TP-02: Stryker wiring plus updated pinned tests (`check_test.go`, `stryker_command_test.go`, `command_test.go`). Route: delegated writer.
- [x] TP-03: e2e on the built binary in the scratchpad fixture. Route: inline (bounded runs).
- [x] TP-04: docs plus artifacts. Route: inline or delegated depending on size.

## Acceptance and checks
- `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l .` are clean, on Windows locally and on the Ubuntu CI leg.
- `ditto staged --dry --exclude-prefix tools/` recorded per commit.
- TP-03 evidence:
  - The dry run passes under load where today it would fail.
  - The process priority is observed as Normal before the signal and BelowNormal after it.

## Delivery
- Strategy `ask-on-risk`. Chain strategy `stacked-to-main` (chosen by the user 2026-10-07).
- Forecast: about 750 authored lines.
- PR 1 holds TP-01 plus the research doc: a runner capability with no caller, so no behavior change.
- PR 2 holds TP-02 to TP-04, against main after PR 1 merges.

## Progress
- Created 2026-10-07. RDD is off (clone-local), so there is no native review.
- 2026-10-07, TP-01 done. Route: delegated writer (7 runner files, 2+ non-trivial).
  - Field `runner.Command.LowerPriorityWhen func() bool`, polled every 250ms (`lowerPriorityPollInterval`) from a goroutine beside the Context watcher, fires once, stops when the process exits. Only `Run` honors it.
  - Decision: setting it together with `LowPriority` is refused with `ErrConflictingPriority` (a `StartError`) before anything starts. "LowPriority wins" was tried first and dropped: its test had to run a below-normal child, which starved under the full suite's load (pids never appeared in 10s), the very effect this feature addresses.
  - Windows: suspended start, unlimited job (`createJob`, factored out of `createKillOnCloseJob`), `JOB_OBJECT_LIMIT_PRIORITY_CLASS` + `BELOW_NORMAL` on the signal, handle closed after Wait, no kill-on-close, Context kill still leader-only.
  - Unix: `Setpgid`, SIGINT/SIGTERM caught from before Start and forwarded to `-pgid`, Context kills `-pgid` with SIGKILL, `setpriority(PRIO_PGRP)` on the signal.
  - RED: compile failure (`unknown field LowerPriorityWhen`); then with the field and no behavior, Windows: grandchild-lowered test "runs at normal priority, want low", never-fires test "condition consulted 0 times in 5s"; Linux (WSL): the same two plus "grandchild still runs after the run was cancelled". Forwarding test with the forward removed: "Run() had not returned 10s after the interrupt". Refusal test with the guard removed: "Run() = <nil>, want a StartError".
  - GREEN: Windows `go test -count=1 ./...` ok (3 consecutive runs); Linux runner tests via WSL (cross-compiled `runner.test -test.v`) all PASS, 2 runs.
  - Checks: `go build ./...` ok; `go vet ./...` ok; `GOOS=linux go vet ./...` ok; `gofmt -l .` empty; `ditto staged --dry --exclude-prefix tools/` listed 4 staged files (exec_other.go, exec_windows.go, managed_windows.go, runner.go) with their mutation ranges, dry run only.
  - Next: TP-02.
- 2026-10-07, TP-02 done. Route: delegated writer (tool.go, mutate.go, mutate_staged.go plus tests: 2+ non-trivial files).
  - Priority is chosen per run: `tool.StrykerLocal` sets none, and `cli.runStryker` takes `lowerPriorityWhen func() bool` and sets it on the command. Mutation runs (interactive and `--staged`) pass `tool.StrykerDryRunCompleted(eventsDir)`; `mutate --dry-run` passes nil (normal throughout); `StrykerServe` keeps `LowPriority: true`.
  - `tool.StrykerDryRunCompleted` reads the directory (no glob, so `[id]` paths cannot change matching) and fires on a `*-onDryRunCompleted.json` entry; a missing dir is false. `tool.StrykerDefaultEventsDir(dir)` = `<dir>/reports/mutation/events`.
  - `StrykerMutate` (and `StrykerMutateFromConfig`) pass `--reporters clear-text,json,event-recorder`; `StrykerDryRun` keeps `clear-text,json`.
  - Interactive: `os.RemoveAll(<p.Source>/reports/mutation/events)` before launch, so a stale event cannot lower the dry run before Stryker's own cleanup. A project config setting `eventReporter.baseDir` leaves the run at normal priority (documented degradation, config not read).
  - `--staged`: events go to `<dh-report-*>/events`, beside the run-owned report and removed with it; the generated config sets `eventReporter.baseDir` to it, keeping other `eventReporter` keys byte for byte (`overrideJSONReporterFileName` generalised to `overrideObjectField`).
  - RED: tool, compile failure, then with stubs `StrykerMutate` lacked event-recorder, predicate stayed false after `00000-onDryRunCompleted.json`, default dir empty. cli, with signatures only: interactive "mutation has no LowerPriorityWhen", staged "generated eventReporter.baseDir = empty, config-level "baseDir = project/events", plus the two pinned priority tests. Dry-run test proven by reverting `StrykerLocal` to `LowPriority: true`: "dry run = LowPriority true ... want neither".
  - GREEN: new tests pass; pinned tests rewritten to the new contract (`check_test.go` runStryker pass-through and mutation switch, `stryker_command_test.go` no priority in StrykerLocal, `command_test.go` serve LowPriority and no switch, `tool_test.go` reporters, staged config key counts +eventReporter).
  - Checks: `go build ./...` ok; `go vet ./...` ok; `GOOS=linux go vet ./...` ok; `gofmt -l .` empty; `go test -count=1 ./...` all ok; `ditto staged --dry --exclude-prefix tools/` listed 3 staged files (internal/cli/mutate.go, internal/cli/mutate_staged.go, internal/tool/tool.go) with their mutation ranges, dry run only.
  - Unverified until TP-03: that Stryker accepts an absolute `eventReporter.baseDir` and writes there.
  - Next: TP-03.
- TP-03 done inline. A binary built from `a27fc50` ran in `scratchpad/m1` (Stryker 10.0.0, vitest 5). A ~1.3s test appends its own `os.getPriority()` when it finishes.
  - No load: the same vitest pid finished at priority 0 in the dry run and 10 in the mutant phase. Exit 0, 2 killed.
  - 12 background CPU threads for 150s, two runs per binary:
    - v1.10.1 stalled ~150s at runner startup, before the dry run began. The dry run then ran at priority 10.
    - This branch began the dry run within ~5s and it ran at priority 0 (1139 and 1150 ms). The mutant phase then stalled ~150s at priority 10.
    - Exit 0 in all four runs.
  - The false dry-run timeout did not reproduce. At v1.10.1 the stall came before any test timer started. What the run proves is the priority each phase gets.
  - New finding, out of scope and pre-existing (v1.10.1 shows it too): under that stall, mutants that are killed without load came out `timeout` (2 of 2 in one run). Stryker counts that as detected, so a real survivor can be masked. Recorded in the research doc (risk 7) and the learning log. Not fixed here; reported to the user.
- TP-04 done inline:
  - Research doc: status, a Result section, and risk 7 corrected.
  - §14 amended. The §05 amendment from main's working tree was brought in as its own commit.
  - `flujo-implementado.md` figure 7 and its note updated.
  - Learning-log line added.
  - Artifacts republished: principles v9 (§01/§03 September amendments, §05, §14) and flujo-implementado v21 (only this change; the rest of that artifact predates the September `.md` edits and is still behind).
