# Design: mutate-staged-v1.9

Inputs: `proposal.md` (authoritative scope P1-P3 and exclusions),
`specs/staged-mutation/spec.md` (binding 10-requirement / 29-scenario
contract), `probe-evidence.md` (H-MSP0b, J2, and H-TK),
`docs/handoff-mutate-staged-v1.9.md` (§3 current code, §5 planned flow,
§7 verification, §9 measurements), `openspec/config.yaml`, `AGENTS.md`, and
`docs/design-principles.md`.

Read alongside the v1.8.0 implementation in
`internal/cli/mutate_staged.go`, `internal/staged/scope.go`,
`internal/staged/classify.go`, `internal/staged/guard.go`,
`internal/runner/runner.go`, `internal/tool/tool.go`, and
`internal/project/detect.go`.

The three prerequisite probes are closed. Their results are design inputs, not
open alternatives:

- **H-MSP0b PASS:** use the project-local Stryker npm shim for
  `stryker serve stdio` on Windows. Do not add a direct-Node path.
- **J2 PASS:** use Jest's list-only JSON-array form. Do not run the fallback
  test-executing command.
- **H-TK PASS:** on Windows, own the server with a Job Object configured with
  `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`, using suspended start, assignment, and
  resume. Do not add a `taskkill` fallback.

This design changes only P1-P3: effective mutation membership, retained-file
related-test reachability, and phase-cost exposure. MSP `mutationTest`, new
Stryker configuration semantics, bridge adoption, publication, upstream work,
and changes to the scoped status verdict remain outside this change.

## Technical Approach

`dharness mutate --staged` remains an index-snapshot operation. It still obtains
added ranges from Git, refuses partial staging, materialises the index, classifies
types-only TypeScript, runs Stryker only over staged ranges, and derives the
verdict from Stryker JSON. v1.9 inserts two evidence-producing stages before the
existing mutation run and makes the five stage outcomes visible:

1. **snapshot** — materialise the index and re-read JSON configuration from it;
2. **classify** — run the existing one-way types-only classifier;
3. **discover** — ask Stryker MSP which staged ranges produce mutants under the
   snapshot's effective `mutate` semantics;
4. **related** — run exactly one runner-specific aggregate related-test command
   over the retained files;
5. **stryker** — run the existing config-file-scoped mutation and derive the
   existing range-scoped JSON verdict.

The orchestration remains in `internal/cli`. Protocol parsing, membership
classification, and related-result parsing belong in `internal/staged`.
External command construction belongs in `internal/tool`. Process creation and
process-tree ownership belong in `internal/runner`. `internal/project` continues
to detect the selected runner and the snapshot's JSON config; it does not build
commands or interpret MSP.

This division follows `docs/design-principles.md` principle 03: dharness owns
invocation and its transient configuration, while project configuration remains
project-owned. It follows principles 01, 02, and 09 by using Stryker's own MSP
`discover` mechanism instead of implementing Stryker's glob and configuration
semantics. It follows principles 11 and 17 by deriving outcomes from framed JSON,
runner JSON, process exit codes, and the final mutation report—never prose.

## Architecture Decisions

### Decision 1 — one deferred phase record owns all five phase states

**Choice.** Create the phase record at the `runMutateStaged` entry point, before
flag refusals or repository preflight can return. Pass it into `mutateStaged` and
render it exactly once with a defer. The record has a fixed ordered set of labels:
`snapshot`, `classify`, `discover`, `related`, and `stryker`. A phase can move only
from `not started` to running and then to completed or failed.

A completed phase records elapsed monotonic time and renders as
`<label> <duration>s`. A failed phase stores the single-line error reason and
renders as `<label> failed: <reason>` without inventing a duration the
specification does not permit in that state. Labels after a failure remain
`not started`. Carriage returns and newlines in an underlying error are collapsed
to spaces for the one-line record; the returned error remains unchanged.

The recorder uses an injected package-level clock function with a restore closure
for tests. Time is the uncontrollable input, so this follows the repository's
existing seam rule without introducing a filesystem abstraction.

The semantic transitions are:

- no staged candidate after `--exclude-prefix`: all five phases are
  `not started`;
- all candidates classify as types-only: snapshot and classify complete;
  discover, related, and stryker are `not started`;
- discovery retains no range: snapshot, classify, and discover complete;
  related and stryker are `not started`;
- a successful related command reporting zero tests completes the related phase,
  prints/returns `no test reaches: <files>; the fix is a test that imports them`,
  exits 1, and leaves stryker `not started`;
- malformed related JSON or a non-zero runner exit fails the related phase;
- a Stryker run or final report read failure fails the stryker phase;
- the scoped survivor verdict is a completed stryker phase with exit 1, because
  mutation and report interpretation completed successfully and found a blocking
  status.

This preserves the distinction between operational failure and a successfully
computed negative verdict. It also means phase timings expose the cost that P3
measured instead of turning expected gate failures into failed instrumentation.

**Rejected: print timings opportunistically at each return.** That duplicates
ordering and omission logic across every branch and cannot guarantee exactly one
record. A single deferred renderer makes the fixed order structural.

**Rejected: classify a zero-test result as `related failed`.** The command and
JSON both succeeded; the result is a gate verdict, not an inability to obtain
one. Principles 11 and 17 reject conflating a measured negative answer with a
broken measurement.

**Guard boundary (AGENTS P10).** The phase record measures the five named phases.
It does not measure initial project discovery, staged-diff parsing, local-binary
resolution, deferred snapshot/report cleanup, or output rendering. A failed phase
intentionally exposes no duration because the binding output grammar provides no
failed-with-duration form. The line is phase-cost evidence, not total wall-clock
profiling.

### Decision 2 — local staged scope stops guessing which files Stryker mutates

**Choice.** Keep `Scope`'s Git recipes and safety checks unchanged:

- `git diff --cached -M -C -U0 --relative --no-ext-diff
  --diff-filter=ACMR` remains unscoped so Git can pair renames;
- `--numstat -M -C --relative -z` continues to fail closed when added lines have
  no parsed hunk;
- source-extension and declaration-file filters remain local structural filters;
- `--exclude-prefix` remains an explicit caller filter and runs before
  classification or discovery;
- `checkFullyStaged` still runs over the locally admitted candidate files.

Remove only the `.test.` / `.spec.` / `__tests__/` heuristic. Whether a test-like
or configuration-like source path is part of mutation belongs to Stryker's
configured effective `mutate` set, not to dharness. This accepts principles 01,
05, and 09 and removes the hand-maintained policy that caused P1.

A path retained by the local filters is a **candidate**, not yet mutation scope.
Only ranged MSP discovery can promote one of its ranges into the final retained
scope.

**Rejected: parse `mutate` patterns locally.** Reimplementing ordered inclusion,
negation, hidden-file defaults, default patterns, range stripping, and future
Stryker changes would violate principles 01, 02, 05, and 09. It would also make
the configured tool and the gate two authorities over one set.

**Rejected: keep the test-file heuristic as a cheap prefilter.** A project can
explicitly include a test-shaped path. Dropping it before discovery would override
that decision, violating principle 05. `--exclude-prefix` stays because it is an
explicit dharness input rather than an inferred project choice.

**Guard boundary (AGENTS P10).** The staged diff guard proves that every added
line Git reports for a locally admitted candidate has a parsed staged range and
that the candidate is not partially staged. It does not prove that a file belongs
to Stryker's effective set, and it does not inspect non-source, declaration, or
explicitly prefix-excluded files. MSP discovery owns membership after this point.

### Decision 3 — strict MSP is a small sequential client over a managed process

**Choice.** Add a stdlib-only MSP client in `internal/staged`. It performs one
request at a time over `stryker serve stdio`:

1. start the project-local Stryker shim in `snapshotSource`;
2. send `configure`, naming the snapshot's JSON Stryker config when one exists
   and otherwise allowing Stryker's own default configuration resolution in that
   snapshot;
3. send one ranged `discover` request containing every candidate runtime range;
4. if any file retained no range, send one path-only `discover` request containing
   exactly those files;
5. close the managed process tree on success, protocol failure, context
   cancellation, or early server exit.

`serve stdio` receives no extra CLI options. Configuration travels through the
MSP `configure` payload/config file, matching the measured protocol. Discovery
uses snapshot-relative, forward-slash paths and the staged range coordinates
expected by MSP. No generated mutation config is used for discovery, because
that config replaces `mutate`; discovery must observe the project's effective
`mutate` decision before dharness narrows it.

The framing codec is deliberately strict:

- each message starts with exactly one ASCII `Content-Length: <decimal>` header;
- header termination and the empty-line separator are required;
- the body is read to the declared byte count, so split and coalesced reads are
  equivalent;
- stray stdout bytes, missing/duplicate headers, invalid or overflowing lengths,
  truncated bodies, invalid JSON, non-`2.0` JSON-RPC, mismatched response IDs,
  protocol errors, and an early EOF are errors;
- stderr is drained separately and never enters the frame parser;
- an MSP error response is a failure, never an empty discovery result.

Requests use monotonically increasing IDs. The client does not need concurrent
request routing because configure and the two possible discovery calls are
strictly ordered. Cancellation closes the process tree and unblocks both frame
reads and writes.

`internal/tool` owns the `stryker serve stdio` command constructor. It returns a
`runner.Command` with label `stryker`, the already-resolved local binary,
`Dir: snapshotSource`, and low priority. `internal/staged` owns protocol bytes,
not executable syntax. This preserves the repository rule that every product
invocation lives in `internal/tool`.

**Rejected: direct Node on Windows.** H-MSP0b observed eight valid frames, zero
stray bytes, and zero malformed headers through the npm `.cmd` shim. Its kill
condition was not met, so a direct-Node branch would be unsupported duplicate
resolution.

**Rejected: MSP `mutationTest`.** It is outside P1-P3. The existing `stryker run`
and JSON report remain the mutation and verdict path.

**Rejected: infer empty discovery from process success.** An empty result is a
valid but ambiguous data value. Framing, JSON-RPC, and process completion must all
be valid before it can be classified.

**Guard boundary (AGENTS P10).** Strict framing proves only that dharness received
a complete, correlated JSON-RPC response. It does not prove why Stryker omitted a
file, and it does not validate Stryker's mutation semantics independently of
Stryker. The server is the membership authority by design.

### Decision 4 — ranged discovery is the membership unit; path-only discovery only classifies a whole-file fallback

**Choice.** Treat each staged runtime range as an independent membership unit.
For every ranged response, retain a range only when Stryker reports at least one
mutant whose location lies within that requested range. A response for the same
file does not retain all of that file's ranges. Returned file names and mutant
locations are validated against the request; unexpected paths or unusable
locations fail discovery instead of widening scope.

After the ranged response, group by file:

- if at least one range was retained, keep only those represented ranges and do
  not path-discover that file;
- if no range was retained, include that file in the single path-only request;
- if the path-only response reports the file with mutants elsewhere, emit
  `in the set, 0 mutants in the staged lines: <path>` and do not mutate it;
- if both responses omit the file, emit
  `outside Stryker's mutate set: <path>` and do not mutate it.

All printed paths are stable, sorted, snapshot-source-relative slash paths. The
retained `tool.MutationScope` values keep the original index-derived ranges; MSP
is an intersection, never a source of broader ranges.

The both-omission wording is binding but intentionally limited. H-MSP0b proved
that a configured-in file generating zero mutants in the whole file and a file
outside the configured set both produce `{}` in ranged and path-only discovery.
Therefore the owner-approved line
`outside Stryker's mutate set: <path>` is an outcome label for that indistinguish-
able observation. No adjacent explanation, error, documentation, or phase reason
may claim “configured exclusion” or “intrinsically zero-mutant.”

If discovery retains no range, return exit 0 after printing every per-file
outcome. Do not start the related command or Stryker mutation. This follows
principles 12 and 13: the measured cheap stages run first, and an empty
intersection avoids all later cost.

**Rejected: path-only discovery for a partially retained file.** The binding
contract says path-only runs only when ranged discovery retained no range for the
file. Running it for a partially retained file adds cost and cannot change which
staged ranges are represented.

**Rejected: report every ranged omission as “zero mutants.”** Ranged omission
alone cannot distinguish a path outside the set from an in-set path with mutants
outside the staged range. The path-only call exists to separate the one
observable case it can separate.

**Guard boundary (AGENTS P10).** A ranged hit proves that Stryker discovered a
mutant in that staged range under the snapshot configuration. A path-only hit
after ranged omission proves only that the file can produce a mutant somewhere;
it does not prove the staged lines are intrinsically non-mutable. Omission by
both forms proves neither configured exclusion nor intrinsic zero-mutant status.
The fixed “outside” label must not be documented as stronger evidence.

### Decision 5 — classification remains before discovery and remains self-checking when mutation proceeds

**Choice.** Preserve `staged.Classify` before MSP. It keeps non-TypeScript files,
runs the project-local `tsc` from an empty temporary working directory, batches
paths under the existing Windows command-line budget, drops only empty emit or
`export {};`, and emits a notice while dropping nothing whenever classification
is unavailable.

Discovery receives only `classification.Kept`. Types-only drops are named before
any discovery call with the existing fixed line:

`types-only: compiler emitted no runtime code: <path>`

When at least one runtime range reaches mutation, the generated mutation config
contains:

- exactly the retained runtime ranges; plus
- the original staged ranges for classifier-dropped files, solely as the existing
  classifier disagreement check.

Outside-set and “in the set, 0 mutants in the staged lines” files do not enter the
mutation config. Classifier-dropped files never contribute to the scoped tally;
any mutant reported for one fails with the existing classifier disagreement.
The final report verdict continues to use only retained runtime ranges.

Classification must precede discovery because empty emit and MSP both-form
omission cannot be distinguished after the fact. Keeping this order preserves
the existing explicit types-only outcome and principle 16's requirement that a
skip be actionable and visible.

**Rejected: ask MSP first and classify only omitted files.** The omission is the
ambiguous signal measured in H-MSP0b. Using it to select classifier inputs would
make “types-only” depend on a signal that cannot identify that cause.

**Guard boundary (AGENTS P10).** The classifier is a one-way optimization, not a
proof that every kept file has mutants. Its disagreement check runs only when a
mutation run proceeds, as required by the specification. If every candidate is
dropped, the command exits before Stryker and therefore cannot empirically
cross-check that all-dropped result. If classification is unavailable, no file is
dropped and discovery remains authoritative for runtime membership.

### Decision 6 — related reachability is one aggregate runner command, never a per-file claim

**Choice.** After discovery, collect the unique sorted file paths represented by
retained ranges and invoke exactly one command selected by the snapshot's
`StrykerSelection.TestRunner`.

For Vitest, `internal/tool` constructs exactly:

```text
vitest related <files> --run --passWithNoTests --reporter=json --outputFile <json-output> [--config <vitest-config>]
```

The output file lives in a run-owned temporary directory. On exit 0,
`internal/staged` decodes it and requires a non-negative numeric
`numTotalTests`. Missing, wrong-typed, malformed, or unreadable JSON returns
`related-test JSON failure: <reason>`.

For Jest, `internal/tool` constructs exactly the J2-confirmed list-only command:

```text
jest --findRelatedTests <files> --listTests --json
```

On exit 0, `internal/staged` requires stdout to be a JSON array. An empty array is
zero related tests; a non-empty array is a positive aggregate. Stderr is not a
verdict and is not interpreted. The J2 fallback
`jest --json --findRelatedTests ... --passWithNoTests` is not implemented because
J2's kill condition was not met.

For either runner:

- non-zero exit returns
  `related-test suite load/run failure: <reason>` and mutation does not start;
- malformed structured output returns
  `related-test JSON failure: <reason>` and mutation does not start;
- a successful zero aggregate returns exit 1 with
  `no test reaches: <files>; the fix is a test that imports them`, and mutation
  does not start;
- a successful non-zero aggregate permits mutation but prints no per-file
  reachability claim.

The command constructor belongs in `internal/tool`; the result parser and typed
outcome belong in `internal/staged`; the decision to continue belongs in
`internal/cli`. `internal/staged/guard.go`'s repository-wide `vitest list` guard
is retired rather than run in addition. The Vitest related run subsumes its load/
run refusal for the selected related set at substantially lower measured cost.
Jest list-only deliberately executes no tests, as proven by J2's marker control.

This is the specification's “per-file related-test reachability” boundary: every
retained file is supplied to the one query, and a zero aggregate soundly names all
of them. A positive aggregate is not per-file evidence and must not be rendered
as if it were.

**Rejected: one related command per file.** It would provide stronger attribution
at multiplicative startup cost, contradict “exactly one” in the binding contract
and P3's cost purpose.

**Rejected: map a positive aggregate back to every input file.** One test may
reach one retained file while another retained file remains unreachable. That
would be a false universal claim, rejected by principles 09, 11, and 17.

**Guard boundary (AGENTS P10).** Aggregate zero proves no retained file has a
related test under that command and snapshot. Aggregate non-zero proves only that
at least one related test was listed or run for the set; it proves nothing about
any individual file. Jest list-only proves discoverability, not that listed tests
load or pass. Vitest does load/run the related set and therefore also guards that
selected set's execution, but neither runner proves that every mutant receives
coverage; the final Stryker statuses own that verdict.

### Decision 7 — MSP process ownership is explicit and platform-specific

**Choice.** Extend `internal/runner` with a managed long-lived process abstraction
used by MSP. It reuses the existing platformization and environment rules, so npm
shims are invoked exactly as ordinary commands and `GIT_DIR`/`GIT_WORK_TREE` stay
removed while `GIT_INDEX_FILE` stays present. The abstraction exposes stdin,
stdout, stderr draining, wait, and an idempotent close that kills and reaps the
entire owned process tree.

On Windows:

1. create a Job Object and set `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`;
2. start the platformized process with `CREATE_SUSPENDED`;
3. assign the process to the Job Object before user code can run;
4. resume it with `NtResumeProcess`;
5. close the Job handle on success, error, cancellation, or caller death.

The implementation uses Windows stdlib syscall bindings in a build-tagged file.
Failure to create/configure/assign/resume the job fails process start; it never
falls back to an unowned process. H-TK proved that this ordering removes the shim,
`node ... stryker.js serve stdio`, and proxy workers, including when the owner is
force-killed. No `taskkill` command exists in the product path.

On Unix, start the process in a new process group (`Setpgid`) and close it by
sending `SIGKILL` to the negative process-group ID, followed by wait. An already
exited leader does not suppress the group kill. Close combines lifecycle errors
with the primary discovery error so a cleanup failure cannot turn into successful
discovery.

The existing one-shot `runner.Run` contract remains available and unchanged for
ordinary commands. v1.9 does not generalize Job Objects/process groups to every
wrapped invocation; the required new lifecycle belongs to the long-lived MSP
server. Generalizing `stryker run` would be a separate process-runner change
outside P1-P3.

**Rejected: `Process.Kill` plus `WaitDelay`.** The current comment states its
boundary: it kills only the shim and may leave an idle grandchild running. MSP is
a persistent server, so that known gap violates the lifecycle requirement.

**Rejected: `taskkill /T /F`.** H-TK's pre-registered fallback was conditional on
a Job Object survivor. No survivor was observed, including the crash control, so
the fallback is forbidden by the accepted evidence.

**Guard boundary (AGENTS P10).** The Windows Job Object is crash-safe while the
job handle is owned by dharness. The Unix process-group cleanup covers normal
return, errors, and context cancellation required by the specification, but a
non-catchable death of dharness itself can prevent userspace cleanup; v1.9 makes
no cross-platform crash-survival claim. The lifecycle guard also does not change
the existing one-shot `stryker run` descendant boundary.

### Decision 8 — the existing Stryker run and scoped JSON verdict stay intact

**Choice.** After membership and related checks pass, preserve v1.8.0's run-owned
report directory, run-owned Stryker sandbox, generated JSON config beside the
snapshot config, `--inPlace`, no incremental file, and explicit concurrency. The
only scope change is the generated config's `mutate` list described in Decision
5.

`reportStagedVerdict` keeps its current rules:

- classifier-dropped files with any reported mutant fail as disagreement;
- only statuses located in retained staged ranges enter the tally;
- `Survived` and `NoCoverage` fail;
- `Killed`, `Timeout`, and `Ignored` pass;
- statuses outside retained ranges do not affect output or exit code;
- zero-generated, zero-tested, ignored, and survivor messages retain their
  existing honest distinctions.

This follows principles 11 and 17: Stryker JSON, not clear-text prose, decides the
verdict. It also preserves principle 13's scope-limited cost without reintroducing
project-wide status into a staged gate.

**Rejected: use MSP `mutationTest` to avoid the generated config/report.** That is
a recorded next round and explicitly outside the proposal. It would replace the
verdict transport as well as membership and is not required for P1-P3.

**Rejected: count path-only discovery mutants in the final tally.** Path-only is
classification evidence, not staged scope. Only original staged ranges retained
by ranged discovery may contribute.

**Guard boundary (AGENTS P10).** The verdict answers only for mutants Stryker
reports inside retained staged ranges. It is not a project mutation score, does
not assess unstaged or omitted lines, and does not prove that an MSP-omitted file
was configured out.

### Decision 9 — snapshot configuration is the single configuration authority

**Choice.** Keep the early real-tree `StrykerRunner` read only for pre-snapshot
refusals and local-package guidance. After snapshot creation, re-run
`project.At(snapshotRoot, snapshotSource).StrykerRunner()` and use that selection
for all of the following:

- MSP configure and effective membership;
- runner choice for related-test execution;
- Vitest's optional config path;
- generated mutation config inheritance;
- final Stryker invocation plugin/test-runner choices.

The binary remains the project-local installed Stryker resolved before snapshot;
ignored `node_modules` is linked into the snapshot by the existing snapshot
mechanism. Configuration bytes and source bytes come from the index snapshot.
No working-tree config is consulted after the snapshot begins.

`internal/project/detect.go` already carries the required JSON selection fields.
It should change only if implementation reveals a mechanical need to expose an
existing parsed value; it must not learn invocation syntax or execute project
configuration. Executable Stryker configs remain refused exactly as in v1.8.0.

**Guard boundary (AGENTS P10).** Snapshot isolation covers tracked index bytes and
ignored entries linked by the existing snapshot contract. It does not include
working-tree-only tracked edits or non-ignored untracked generated inputs. A
monorepo whose dependencies are hoisted outside Source can still fail loudly at
the related or Stryker phase; this change does not redesign snapshot topology.

## Data Flow

```text
runMutateStaged
  ├─ initialize fixed five-phase record; defer exactly-one render
  ├─ validate flags / discover Project / staged.Scope
  │    ├─ Git -M/-C, -U0 ranges from index
  │    ├─ numstat cross-check
  │    ├─ source + declaration + --exclude-prefix filters
  │    ├─ no local test-name heuristic
  │    └─ partial-staging refusal
  │
  ├─ no candidate ───────────────────────────────→ exit 0; all not started
  ├─ require project-local Stryker
  │
  ├─ snapshot
  │    ├─ checkout index and link ignored entries
  │    └─ re-read StrykerSelection from snapshot JSON
  │
  ├─ classify(candidate files)
  │    ├─ dropped ──→ print types-only outcome
  │    └─ kept runtime candidates
  │         └─ empty ────────────────────────────→ exit 0
  │
  ├─ discover
  │    ├─ tool.StrykerServe → runner managed tree
  │    ├─ MSP configure(snapshot config)
  │    ├─ ranged discover(all candidate ranges)
  │    │    └─ retain only represented original ranges
  │    ├─ path-only discover(files with zero retained ranges)
  │    │    ├─ present → in-set / zero staged mutants outcome
  │    │    └─ omitted → fixed outside-set outcome, cause unknown
  │    └─ close and reap whole MSP tree
  │         └─ no retained range ────────────────→ exit 0
  │
  ├─ related(unique retained files), exactly one command
  │    ├─ vitest → exit code + numTotalTests JSON file
  │    └─ jest   → exit code + list-only JSON array
  │         ├─ malformed/non-zero ───────────────→ phase failure
  │         ├─ aggregate zero ───────────────────→ exit 1; no Stryker
  │         └─ aggregate positive ───────────────→ no per-file claim
  │
  └─ stryker
       ├─ generated config:
       │    retained runtime ranges + classifier-drop self-check ranges
       ├─ existing `stryker run ... --inPlace`
       └─ JSON report
            ├─ classifier disagreement check
            └─ existing retained-range status tally and exit code
```

## Interfaces and Ownership Contracts

The exact Go names may be adjusted to existing package naming during RED/GREEN,
but these ownership boundaries are binding:

```text
internal/tool
  StrykerServe(localBinary, snapshotSource) -> runner.Command
  VitestRelated(localBinary, snapshotSource, files, output, config) -> runner.Command
  JestRelated(localBinary, snapshotSource, files) -> runner.Command

internal/runner
  start a long-lived Command -> managed process
  managed process exposes stdin/stdout and idempotent tree close + wait
  Windows implementation: Job Object, suspended → assign → resume
  Unix implementation: process group, kill group → wait

internal/staged
  MSP codec/client over readers and writers
  Discover(ctx, command, snapshot config, staged scopes) -> membership result
  membership result: retained ranges, in-set-zero paths, both-omitted paths
  Related(ctx, runner selection, retained files) -> aggregate outcome

internal/cli
  fixed phase recorder
  ordering, early exits, user-visible outcome lines
  generated final mutation config and existing scoped report verdict
```

The process seam remains a package-level function with a restore closure for
tests, matching `runner.SetForTest`. Filesystem behavior uses real temporary
directories. Protocol tests use in-memory streams or a controlled helper process;
they do not require a fake filesystem.

## File Changes

| File | Action | Design responsibility |
|---|---|---|
| `internal/cli/mutate_staged.go` | Modify | Five-phase orchestration, early exits, discovery/related integration, retained final scope, exactly-one phase record |
| `internal/cli/*_test.go` | Modify | Binding output, exit, invocation-order, snapshot-isolation, and preserved-verdict scenarios |
| `internal/staged/scope.go` | Modify | Retire only the local test-file heuristic; keep Git/range/prefix/declaration/partial-staging behavior |
| `internal/staged/classify.go` | Preserve or narrowly modify | Existing classifier behavior and disagreement inputs; no new classification semantics |
| `internal/staged/guard.go` | Replace/retire | Remove repository-wide `vitest list` guard in favor of related-test evaluation |
| `internal/staged/msp*.go` | New | Strict Content-Length JSON-RPC codec, sequential client, discovery decoding |
| `internal/staged/membership*.go` | New or combined with MSP | Per-range retention and honest path-only outcomes |
| `internal/staged/related*.go` | New | Vitest/Jest structured-result parsing and aggregate outcome |
| `internal/runner/runner.go` | Modify | Common managed-process entry point and existing platformization reuse |
| `internal/runner/*_windows.go` | Modify/New | Job Object lifecycle, suspended assignment, resume, close/reap |
| `internal/runner/*_unix.go` | Modify/New | Process-group lifecycle and close/reap |
| `internal/tool/tool.go` | Modify | Stryker serve, Vitest related, and Jest related command construction |
| `internal/project/detect.go` | Normally unchanged | Existing snapshot JSON runner/config selection remains authoritative |
| `README.md` | Modify in documentation slice | User-facing staged phases and outcomes |
| `docs/flujo-implementado.md` + artifact mirror | Modify | Five-phase flow and boundaries |
| `docs/design-principles.md` + artifact mirror | Modify | Amend principle 01's invalid flag-only premise; MSP discover is Stryker's own effective-membership mechanism |
| `docs/learning-log.md` | Append only | Dated probe and implementation lessons |
| applicable mutation backlog docs | Modify | Close/update v1.9 entries without expanding product scope |

`docs/mutation-testing.md`, bridge files, `chat-ytchannel/`, and MSP
`mutationTest` work are explicitly unchanged.

## Testing Strategy for Apply

Strict TDD is active for implementation, not for this design-only write. Each
implementation slice follows RED → GREEN → MUTATE → REFACTOR, with focused tests
first and `ditto staged` against the owning package after green.

### Phase behavior

- A fake clock makes all five completed durations deterministic.
- Table tests cover all-not-started, all-types-only, no-retained-range,
  zero-related, each operational phase failure, survivor verdict, and full pass.
- Every row asserts exactly one `phases:` record, the five labels exactly once and
  in order, and no later phase invocation after an early exit.
- A multiline underlying error proves the phase record remains one line while the
  returned error retains its details.

### MSP framing and protocol

- Split every header/body boundary across reads and join multiple frames in one
  read.
- Reject stray bytes, malformed header spelling, duplicate/missing length,
  invalid/overflow length, truncated body, invalid JSON, wrong JSON-RPC version,
  mismatched ID, error response, and early server exit.
- Prove stderr bytes never reach the frame parser.
- Prove cancellation unblocks an incomplete frame and returns only after process
  tree cleanup.
- A controlled real-process test exercises configure + discover when local
  Stryker is available; it may skip only on explicit local-tool absence.

### Membership

- One file with multiple ranges retains only ranges containing returned mutants.
- A partially represented file never enters path-only discovery.
- A file with no retained range and a path-only hit emits the exact in-set-zero
  string.
- Both-form omission emits the exact outside-set string and no configured-
  exclusion or intrinsic-zero-mutant explanation.
- Prefix-excluded paths never reach classification, either discovery call, or an
  outcome line.
- Test-shaped files reach discovery, proving the retired heuristic is gone.
- Existing real-repository tests preserve rename pairing, index ranges, CRLF,
  `GIT_INDEX_FILE`, Source ≠ Root, and partial-staging refusal.

### Related tests

- Vitest command tokens and order exactly match the specification, including the
  optional snapshot config.
- Vitest exit 0 + `numTotalTests: 0`, positive total, missing field, wrong type,
  malformed JSON, and non-zero exit each own a scenario.
- Jest command tokens and order pin `--findRelatedTests <files> --listTests
  --json`; no fallback command exists.
- Jest `[]`, a non-empty array, malformed JSON, non-array JSON, and non-zero exit
  each own a scenario.
- Zero aggregate asserts exit 1, the exact action string, and zero Stryker
  mutation invocations.
- Positive aggregate asserts mutation proceeds and no individual file is called
  reachable or unreachable.

### Lifecycle

- A helper process creates descendants; normal close, protocol error, and context
  cancellation all leave none.
- Windows tests prove the child cannot run before job assignment and that closing
  the job removes descendants. The production path contains no `taskkill`.
- Unix tests prove the whole process group is killed and reaped.
- A lifecycle cleanup failure cannot produce successful discovery.

### Final verdict and observable output

- Existing scoped report tests remain green for all five statuses and
  out-of-range mutants.
- Classifier-dropped files remain excluded from tally and fail only on reported
  disagreement when mutation proceeds.
- Snapshot-versus-working-tree config disagreement is exercised end to end.
- Build the binary outside the repository and run it in a scratch JS project with
  Source ≠ Root. Read every output line for outside-set, in-set-zero, no-related,
  types-only, failed phase, full phase timing, and retained-range survivor cases.
  Never run the binary against this repository.

Tests own behavior scenarios rather than mutants (AGENTS P09). Defensive error,
cancellation, lifecycle, and early-return guards receive focused manual deletion
checks in addition to staged mutation. The full gate and Windows/Linux CI remain
the final platform proof.

## Review Slices and 400-Line Budget

Delivery auto-chaining is pre-authorized. The implementation should preserve one
writer and split before review whenever a slice approaches 400 changed lines,
including tests. The following are review boundaries, not permission to combine
them:

| Slice | Content | Forecast | Budget risk |
|---|---|---:|---|
| A | Phase state model, fake clock, deferred renderer, orchestration tests | 220-320 | Low |
| B1 | Pure Content-Length/JSON-RPC codec and malformed-frame tests | 280-380 | Medium |
| B2 | Managed-process API plus platform tree ownership and lifecycle tests | 340-480 | **High — split Windows and Unix/common work if forecast exceeds 400** |
| C1 | MSP configure/discover client over the codec and controlled-process tests | 280-380 | Medium |
| C2 | Per-range membership classification, scope heuristic retirement, output tests | 300-400 | High edge |
| C3 | CLI discovery wiring, snapshot config authority, final-scope assembly | 300-400 | High edge |
| D | Tool-owned Vitest/Jest commands, parsers, and CLI early exits | 320-400 | High edge |
| E1 | README, flow, design-principle amendment, backlog, learning-log | 250-380 | Medium |
| E2 | Required artifact mirrors and observable binary readback evidence | 200-350 | Medium |

B2 is expected to exceed the budget if both platform implementations and all
lifecycle scenarios grow together; split it before review rather than accepting a
large slice. C2/C3 are already separated so pure membership review is not buried
inside orchestration. E1/E2 keep the required documentation set feasible without
mixing mirror mechanics into behavior prose. Each slice must be green in the
index tree before the next, and any rebase invalidates prior verification.

The likely total is approximately 2,500-3,400 changed lines including tests and
documentation. That requires a chained delivery; it does not authorize scope
outside P1-P3.

## Threat and Boundary Matrix

| Boundary | Design response | What it does not cover |
|---|---|---|
| Index versus working tree | Scope and configuration come from the index snapshot | Non-ignored untracked inputs and working-tree-only edits are intentionally absent |
| Rename/copy parsing | Preserve unscoped `-M -C` diff plus numstat cross-check | It does not determine mutation membership |
| Partial staging | Refuse locally admitted candidate files with unstaged changes | Prefix-excluded, declaration, and non-source files are outside this guard |
| Types-only classification | Drop only exact empty emit; keep all on classifier failure; self-check when mutation runs | All-dropped runs cannot perform the disagreement self-check |
| Effective membership | Stryker MSP ranged discovery under snapshot config | Both-form omission cannot identify exclusion versus whole-file zero mutants |
| MSP transport | Strict framed, correlated JSON-RPC; errors fail closed | Valid protocol does not independently prove Stryker semantics |
| Process tree | Windows Job Object; Unix process group; cleanup on success/error/cancel | Unix cannot guarantee cleanup after uncatchable parent death; ordinary `stryker run` is unchanged |
| Related tests | One structured aggregate command; zero is universal | Positive aggregate is not per-file reachability; Jest list-only does not run suites |
| Final verdict | Existing JSON statuses inside retained ranges only | No project-wide score or statement about omitted/unstaged code |
| Phase cost | Five fixed phase durations/states | Preflight and cleanup are outside the timing record |
| Configuration ownership | Snapshot JSON remains authoritative; generated config is transient | Executable Stryker configs remain unsupported |
| Command ownership | Every Stryker/Vitest/Jest invocation is constructed in `internal/tool` | Protocol parsing remains outside `internal/tool` |

## Migration and Rollback

There is no persisted data migration. The only transient additions are MSP
process state, related-test JSON, the existing run-owned report/sandbox, and the
existing generated staged Stryker config inside the disposable snapshot.

Roll back the chained v1.9 slices in reverse order. Removing related/discovery
wiring restores v1.8.0's `vitest list` guard and locally filtered scope; removing
the managed-process and codec slices then leaves no unused product path. Do not
roll back by weakening the scoped JSON verdict, snapshot isolation, partial-
staging refusal, classifier disagreement check, or Git rename behavior.

## Open Questions

All choices that could change P1-P3 are closed:

- [x] Windows MSP launch path: npm `.cmd` shim; H-MSP0b passed.
- [x] Jest reachability command: list-only JSON array; J2 passed.
- [x] Windows tree lifecycle: Job Object KILL_ON_JOB_CLOSE; H-TK passed.
- [x] Both-form omission: fixed outside-set label, explicitly ambiguous cause.
- [x] Related attribution: one aggregate command; only aggregate zero supports a
      universal statement about every retained file.
- [x] Mutation transport: preserve `stryker run` + JSON report; MSP
      `mutationTest` remains a later change.
- [x] Delivery: chained slices are pre-authorized; no reviewed slice may silently
      exceed the 400-line budget.

Implementation naming within the package boundaries may be settled during TDD,
but no open naming choice may alter an observable string, exit code, phase order,
probe-forced platform path, or the scope exclusions above.
