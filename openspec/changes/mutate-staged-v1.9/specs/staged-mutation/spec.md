# Staged Mutation Specification

## Purpose

`dharness mutate --staged` evaluates staged runtime ranges that Stryker discovers as eligible, checks aggregate related-test availability before mutation, and reports phase progress and scoped JSON verdicts.

## Requirements

### Requirement: Preserved Staged Behavior

The system MUST preserve index snapshot isolation, MUST read project configuration JSON from the snapshot, MUST classify types-only files before discovery, MUST keep `--exclude-prefix` as a pre-classification filter, MUST run Stryker only for the retained staged scope, and MUST derive the scoped verdict from Stryker JSON.

#### Scenario: Index and working tree disagree

- GIVEN staged source or configuration bytes differ from working-tree bytes
- WHEN `dharness mutate --staged` evaluates membership and the mutation report
- THEN its printed outcomes and exit code SHALL correspond to the index snapshot bytes and snapshot configuration JSON
- AND working-tree-only bytes SHALL NOT change the verdict

#### Scenario: Exclude prefix removes every candidate

- GIVEN every staged candidate matches an `--exclude-prefix` value
- WHEN `dharness mutate --staged` runs
- THEN the command SHALL exit 0
- AND the phase record SHALL show `classify not started` and `discover not started`
- AND no Stryker membership outcome SHALL be printed for an excluded path

#### Scenario: Preserved scoped run completes

- GIVEN discovery retains one or more staged runtime ranges
- WHEN Stryker returns a JSON mutation report
- THEN only mutants within the retained staged ranges SHALL contribute to the printed tally and exit code

### Requirement: Deterministic Mutation Membership

The system MUST treat each staged runtime range as the ranged-discovery membership unit. It MUST retain a range only when Stryker's ranged discovery reports a mutant within that range under the configured effective `mutate` semantics. Path-only discovery MUST run only for a file for which ranged discovery retained no staged runtime range. Local test-file-name heuristics MUST NOT determine membership. The system MUST apply `--exclude-prefix` to the staged candidate scope before types-only classification and both discovery forms, so a prefix-excluded file never participates in discovery or ambiguity reporting.

#### Scenario: Runtime range is accepted

- GIVEN ranged discovery reports one or more mutants within a staged runtime range
- WHEN staged membership is evaluated
- THEN that range SHALL be retained for mutation
- AND the scoped Stryker JSON verdict SHALL include statuses from that retained range

#### Scenario: A file is accepted for only some staged ranges

- GIVEN a file has multiple staged runtime ranges
- AND ranged discovery reports mutants within some but not all of those ranges
- WHEN staged membership is evaluated
- THEN only the represented ranges SHALL be retained in the mutation scope
- AND the unrepresented ranges SHALL NOT contribute mutants or status counts from the Stryker JSON report
- AND path-only discovery SHALL NOT run for that file

#### Scenario: Path-only discovery identifies zero-mutant staged lines

- GIVEN ranged discovery retains no staged runtime range for a file
- AND path-only discovery returns that file
- WHEN membership is evaluated
- THEN output SHALL print `in the set, 0 mutants in the staged lines: <path>`
- AND the file SHALL not enter the mutation run

#### Scenario: Both discovery forms omit every runtime file

- GIVEN ranged discovery retains no staged runtime range
- AND path-only discovery omits every staged runtime file
- WHEN membership is evaluated
- THEN output SHALL print `outside Stryker's mutate set: <path>` for each omitted file
- AND output MUST NOT claim that any file is a configured exclusion or an intrinsically zero-mutant file

#### Scenario: Discovery leaves no retained range

- GIVEN every staged runtime file is reported either as `outside Stryker's mutate set: <path>` or as `in the set, 0 mutants in the staged lines: <path>`
- WHEN discovery completes
- THEN the command SHALL exit 0 without running the related-test command or Stryker mutation

#### Scenario: Discovery response is malformed or fails

- GIVEN an MSP discovery response is malformed, incomplete, or returns an error
- WHEN membership is evaluated
- THEN the command SHALL exit non-zero without mutation
- AND the phase record SHALL print `discover failed: <reason>`

### Requirement: Snapshot-Scoped Configuration

The system MUST obtain discovery and related-test results using the snapshot's configuration and index bytes. It MUST NOT infer membership from working-tree bytes or replace the configured `mutate` semantics.

#### Scenario: Snapshot and working-tree mutate configuration differ

- GIVEN the index snapshot excludes a path that the working-tree configuration includes
- WHEN staged discovery runs
- THEN output SHALL print `outside Stryker's mutate set: <path>` according to the snapshot configuration
- AND the working-tree configuration SHALL NOT cause that path to enter mutation

### Requirement: Related-Test Reachability

For retained files, the system MUST run exactly one runner-specific related-test command and MUST derive its outcome only from that command's exit code and structured JSON. Vitest MUST run `vitest related <files> --run --passWithNoTests --reporter=json --outputFile <json-output> [--config <vitest-config>]` and MUST read the aggregate `numTotalTests` field from that JSON output. When J2 confirms the list-only contract, Jest MUST run `jest --findRelatedTests <files> --listTests --json` and MUST read the resulting JSON array. If J2 rejects that contract, Jest MUST run the pending J2 fallback `jest --json --findRelatedTests <files> --passWithNoTests` and MUST read its aggregate `numTotalTests` field.

A successful aggregate total of zero is a sound universal claim that none of the retained files has a related test, so the system MAY name every retained file as unreachable without claiming per-file evidence. A successful non-zero aggregate MUST NOT be presented as proof that any particular retained file is reachable or unreachable.

#### Scenario: No related tests exist

- GIVEN the runner exits 0
- AND Vitest or the Jest fallback reports `numTotalTests` equal to `0`, or Jest list-only returns the JSON array `[]`
- WHEN retained files are evaluated
- THEN output SHALL print `no test reaches: <files>; the fix is a test that imports them`
- AND the command SHALL exit 1 without invoking Stryker mutation

#### Scenario: At least one related test exists

- GIVEN the runner exits 0
- AND Vitest or the Jest fallback reports `numTotalTests` greater than `0`, or Jest list-only returns a non-empty JSON array
- WHEN retained files are evaluated
- THEN output MUST NOT claim that any individual retained file is reachable or unreachable
- AND Stryker mutation SHALL proceed for the retained ranges

#### Scenario: Related suites fail to load or run

- GIVEN the runner-specific related-test command exits non-zero because a related suite fails to load, fails to run, or is red
- WHEN reachability is evaluated
- THEN output SHALL print `related-test suite load/run failure: <reason>`
- AND the command SHALL exit non-zero without invoking Stryker mutation

#### Scenario: Related-test output is malformed

- GIVEN the runner-specific related-test command exits 0
- AND its output lacks the required `numTotalTests` field or JSON array
- WHEN reachability is evaluated
- THEN output SHALL print `related-test JSON failure: <reason>`
- AND the command SHALL exit non-zero without treating the result as zero tests or invoking Stryker mutation

### Requirement: Honest Empty Outcomes

The system MUST name types-only files before discovery. Omission by both discovery forms MUST remain an honest ambiguous result because the observable discovery data cannot distinguish an intrinsic zero-mutant file from a configured exclusion. Aggregate related-test totals MUST NOT be presented as per-file test proof except for the sound universal zero-total outcome defined above.

#### Scenario: Every candidate is types-only

- GIVEN classification emits no runtime code for every staged candidate
- WHEN staged mutation runs
- THEN output SHALL print `types-only: compiler emitted no runtime code: <path>` for each candidate
- AND the command SHALL exit 0 without invoking discovery, the related-test command, or Stryker mutation
- AND the phase record SHALL show `discover not started`, `related not started`, and `stryker not started`

#### Scenario: Empty discovery cannot identify the cause

- GIVEN ranged and path-only discovery both omit a staged runtime file
- WHEN the empty outcome is printed
- THEN output SHALL use only `outside Stryker's mutate set: <path>` for that outcome
- AND output MUST NOT print a configured-exclusion or intrinsic-zero-mutant explanation

### Requirement: Classifier Disagreement Safety Check

When Stryker mutation proceeds, the final mutation configuration MUST include types-only classifier-dropped files solely to detect classifier disagreement. A dropped file that produces any mutant in the JSON report MUST fail the command; dropped files MUST NOT contribute to the scoped mutation verdict.

#### Scenario: Types-only classification disagrees with Stryker

- GIVEN a classifier-dropped file appears with one or more mutants in the Stryker JSON report
- WHEN the mutation report is evaluated
- THEN the command SHALL exit non-zero
- AND output SHALL name the classifier disagreement and the affected file

#### Scenario: Types-only classification agrees with Stryker

- GIVEN classifier-dropped files have no mutants in the Stryker JSON report
- WHEN the mutation report is evaluated
- THEN those files SHALL NOT contribute status counts or change the scoped exit code

### Requirement: MSP Framing and Lifecycle

The system MUST exchange strict framed JSON-RPC messages with the MSP server and MUST remove the server process tree on success, error, or cancellation. It MUST NOT treat malformed framing, protocol errors, or an early server exit as successful discovery.

#### Scenario: MSP session ends successfully

- GIVEN discovery completes through valid framed JSON-RPC responses
- WHEN the command returns
- THEN no MSP server descendant SHALL remain
- AND the discover phase SHALL be printed as completed with its duration

#### Scenario: MSP session is cancelled

- GIVEN cancellation occurs during an MSP session
- WHEN the command returns
- THEN no MSP server descendant SHALL remain
- AND the command SHALL exit non-zero with `discover failed: <reason>` in the phase record

#### Scenario: MSP request fails before discovery completes

- GIVEN configure or discovery returns a protocol error, contains invalid framing, or the server exits early
- WHEN the command returns
- THEN no MSP server descendant SHALL remain
- AND the command SHALL exit non-zero without invoking mutation
- AND the phase record SHALL print `discover failed: <reason>`

### Requirement: Scoped Mutation Verdict

The system MUST derive the staged mutation verdict only from Stryker JSON statuses inside the retained staged ranges. Any in-scope `Survived` or `NoCoverage` status MUST fail the command. In-scope `Killed`, `Timeout`, and `Ignored` statuses MUST pass. Statuses outside the retained staged ranges and classifier-dropped files without a disagreement MUST NOT contribute to the tally or exit code.

#### Scenario: An in-scope mutant fails the verdict

- GIVEN the Stryker JSON report contains an in-scope mutant with status `Survived` or `NoCoverage`
- WHEN the scoped verdict is evaluated
- THEN the command SHALL exit 1
- AND output SHALL name the failing status and its in-scope location

#### Scenario: All in-scope mutants pass the verdict

- GIVEN every in-scope mutant in the Stryker JSON report has status `Killed`, `Timeout`, or `Ignored`
- WHEN the scoped verdict is evaluated
- THEN the command SHALL exit 0
- AND the printed tally SHALL be derived only from those in-scope JSON statuses

#### Scenario: A failing status is outside the retained scope

- GIVEN the Stryker JSON report contains `Survived` or `NoCoverage` only outside the retained staged ranges
- WHEN the scoped verdict is evaluated
- THEN those statuses SHALL NOT appear in the scoped tally
- AND they SHALL NOT cause a non-zero exit

### Requirement: Phase Output

On every `dharness mutate --staged` run, the system MUST print exactly one phase record to standard output. The record MUST use the labels `snapshot`, `classify`, `discover`, `related`, and `stryker`, exactly once each and in that order. A completed phase MUST render as `<label> <duration>s`; a phase not entered MUST render as `<label> not started`; and a failed phase MUST render as `<label> failed: <reason>`. The record MUST begin with `phases:` and separate phase states with ` · `.

#### Scenario: Every phase completes

- GIVEN snapshot, classification, discovery, related-test evaluation, and Stryker mutation complete
- WHEN the command returns
- THEN standard output SHALL contain `phases: snapshot <duration>s · classify <duration>s · discover <duration>s · related <duration>s · stryker <duration>s`

#### Scenario: Mutation is skipped after discovery

- GIVEN discovery completes with no retained staged runtime range
- WHEN the command returns
- THEN standard output SHALL contain `phases: snapshot <duration>s · classify <duration>s · discover <duration>s · related not started · stryker not started`

#### Scenario: Any phase fails

- GIVEN `snapshot`, `classify`, `discover`, `related`, or `stryker` fails
- WHEN the command returns
- THEN every earlier completed phase SHALL retain its duration in the standard-output phase record
- AND the failing phase SHALL render as `<label> failed: <reason>`
- AND every later phase SHALL render as `<label> not started`

#### Scenario: No staged candidate reaches snapshot

- GIVEN staged scope is empty after `--exclude-prefix` filtering
- WHEN the command returns
- THEN standard output SHALL contain `phases: snapshot not started · classify not started · discover not started · related not started · stryker not started`

### Requirement: Documentation Deliverables

The change MUST update `README.md`, `docs/flujo-implementado.md`, `docs/design-principles.md`, `docs/learning-log.md`, applicable backlog entries, and the artifact mirrors of `docs/flujo-implementado.md` and `docs/design-principles.md`. The documentation MUST describe the five-phase flow, the membership and related-test outcomes, and the preserved scoped verdict. `docs/design-principles.md` MUST amend principle 01's invalid flag-only premise so that Stryker MSP discovery is recognized as Stryker's own effective-membership mechanism, and `docs/learning-log.md` MUST remain append-only.

#### Scenario: Documentation is reviewed against observable behavior

- GIVEN the v1.9 documentation deliverables are present
- WHEN their staged-mutation examples are compared with command output
- THEN they SHALL use the five phase labels `snapshot`, `classify`, `discover`, `related`, and `stryker`
- AND they SHALL include the observable outcomes `outside Stryker's mutate set: <path>`, `in the set, 0 mutants in the staged lines: <path>`, and `no test reaches: <files>; the fix is a test that imports them`
- AND the principle 01 amendment SHALL NOT claim that every Stryker option is a flag

## Planned Boundaries

Planned implementation files are `internal/cli/mutate_staged.go`, `internal/staged/`, `internal/runner/runner.go`, and `internal/tool/tool.go`; their v1.8 baseline paths are attributed to `origin/main`. WU-E documentation scope includes `README`, `docs/flujo-implementado.md`, `docs/design-principles.md`, `docs/learning-log.md`, applicable backlog entries, and the `flujo-implementado.md` and `design-principles.md` artifact mirrors. `docs/mutation-testing.md`, `chat-ytchannel/`, and bridge adoption remain outside scope.
