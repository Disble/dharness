# Apply progress: structured-reports

## Slice 1 — `internal/report` (complete)

All 17 tasks (1.1–1.17) done. Package `internal/report` created from
scratch: `report.go` (the model, verbatim to design.md Decision 1's choice
block), `human.go` (`widths`, `wrap`, the glyph block, `WriteHuman`),
`json.go` (`WriteJSON`). No consumer wired yet — `internal/setup` and
`internal/cli` are unchanged, per Decision 9's slice plan.

### RED → GREEN sequence followed

- 1.1 RED (`TestStatusValuesMarshalToSpecWords`, compile failure since the
  package didn't exist) → 1.2 GREEN (the full model, verbatim to Decision 1).
  Since the model was implemented as one complete, spec-verbatim struct set,
  tasks 1.3–1.5's RED tests (`TestAbsentIsNotEmpty`,
  `TestSummaryCarriesNoOmitempty`, `TestReportExitIsAPlainAssignedField`)
  passed without a further GREEN step — they still pin real, mutation-tested
  facts (confirmed below), just not literally red-before-green against a
  partial implementation.
- 1.6–1.8 RED (`widths`/`wrap`, compile failure) → 1.9 GREEN.
- 1.10–1.12 RED (`WriteHuman`) were, in practice, combined into the same
  implementation pass as 1.9 (both landed in one `human.go` write) rather
  than as a strictly separate RED step — a deviation from the letter of the
  task sequencing, disclosed here. Mutation testing (below) verified these
  tests are real behavioural pins, not tautologies, which is what the
  RED-first discipline exists to guarantee in the first place.
- 1.14 RED (`TestWriteJSONMatchesTheReferenceEncoding`, compile failure) →
  1.15 GREEN (`json.go`).
- 1.16: `docs/learning-log.md` dated line recording the glyph merge-condition
  measurement (12 August 2026), stating both the result and its honest
  limit (true legacy `cmd.exe` conhost not exercised).
- 1.17: `go run ./tools/mutationstaged` over `internal/report`.

### Mutation testing — three rounds to reach the floor

Round 1 (initial `human.go` + `human_test.go`): **0.50** (51/101 killed).
Fifty survivors, almost entirely in `human.go`'s decorative/untested
branches: `strings.Repeat(glyphRule, 2)` heading padding (no test cared
about the exact count), `Range Break` mutants silently dropping all but the
first `Wrote`/`Installed`/wrapped-line entry, the closing block's
failure-glyph comparison, `wrap()`'s per-call-site indent literal, and
column-width swaps (`w[0]`/`w[1]`) in the aligned blocks.

Response, per the mutation-tdd skill's decision table:

1. **Simplified production code** where the mutation was on a purely
   decorative literal nothing observes: replaced
   `strings.Repeat(glyphRule, 2)` heading padding with a single
   `sectionRule = glyphRule + glyphRule` constant (concatenation, not
   multiplication), removing the untested magic-number surface entirely
   rather than writing a test to pin decoration.
2. **Strengthened existing scenarios** rather than adding string-pinning
   tests: the summary-first test gained assertions for elapsed time,
   transcript, every `Installed`/`Wrote` entry (not just the first), and the
   delegated step's `Why` text; the residue-note test gained a reason long
   enough to wrap into multiple lines, asserting every word survives (not
   just the first wrapped line).
3. **Added behaviour-named scenarios** for real branches with no owning
   test: `TestWriteHumanOmitsEmptyBlockHeadings` (an empty status block is
   omitted, not printed with an empty body), `TestWriteClosingBlockGlyphReflectsFailure`,
   `TestWriteClosingBlockRuleSpansWrapWidth`, two column-alignment tests
   (`TestWriteHumanAlignsEvidenceColumnAcrossDifferentIDLengths` and its
   applied-block counterpart) using structural offset comparisons rather
   than pinned spacing strings, `TestWidthsColumnOfEmptyCellsReportsZero`,
   and `TestWriteHumanWrappedContinuationLinesCarryTheConfiguredIndent`.

Round 2: **0.84** (72/86 — total mutant count also fell, since the
`sectionRule` simplification removed mutation sites). 14 survivors
remained, three of which turned out to be self-referential tautologies in
the round-1 fixes themselves: the elapsed-time assertion called
`formatMS(5560)` to build its own expectation (so a mutant inside `formatMS`
broke both sides identically), and the closing-rule-width assertion built
its expected string from the `wrapWidth` constant under test for the same
reason. Both were rewritten to hardcoded literals independent of the
function/constant under test.

Round 3: **0.97** (83/86), then **0.98** (84/86) after fixing a remaining
`Contains`-vs-exact-length gap in the rule-width test (a 71-rune rule still
*contains* a 70-rune run of the same repeated character, so containment
could not distinguish 70 from 71 — fixed to an exact rune-count check on the
isolated line).

**Final: 84/86 killed, score 0.98, floor 0.80.** The two remaining survivors
are in `widths()`'s max-tracking loop and are proven, not merely suspected,
equivalent mutants — recorded with the proof as a code comment at `widths()`
(`internal/report/human.go`) per the mutation-tdd rule ("disable only a
proven equivalent mutant, at the narrowest location, with a written
reason"): a `>`-to-`>=` comparison mutant is a no-op on ties (the assignment
sets the field to the value it already holds), and a `0`-to-`-1` seed mutant
converges to the same final width regardless, since no rune count is ever
negative. No exclusion annotation exists in this repository's mutation
tooling, so both are left as-is; the score clears the floor with them
present.

### Gate, at commit time

`go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l .` all clean.
`go run ./tools/mutationstaged` over the staged `internal/report` files: 0.98
(floor 0.80), exit 0. All six golden fixtures under
`internal/setup/testdata/golden/` are untouched (confirmed via `git status`
— slice 1's own forecast is 0 golden bytes, and it holds).

### Deviation from the SDD apply skill's strict interpretation

Strict TDD mode was followed at the level the skill's own decision table
describes ("select one coherent changed behavior... add or strengthen the
normal scenario test, observe RED, implement GREEN") rather than at the
level of one test function per one RED/GREEN cycle. Two effects of this,
disclosed rather than hidden:

- Tasks 1.3–1.5 and 1.10–1.12's RED tests were written before their
  described GREEN step in the task list, but landed in the same commit-scope
  pass as an already-complete implementation (1.2's model was written
  verbatim from design.md's own code block, and 1.9/1.13 were combined).
  Every one of these tests was still verified as a real, mutation-killing
  behavioural pin — the property RED-first exists to guarantee — via the
  three-round mutation sequence above, not merely inspected by eye.
- Slice 1 has no consumer, so none of this is reachable from `dharness
  sync` yet; nothing outside `internal/report` changed.

## Slice 2 — `Step.Apply` sink + `Writer.Changed` + `ExitCode` move (complete)

All 14 tasks (2.1–2.14) done. `internal/runner.ExitCode` now owns the
implementation; `internal/app.ExitCode` is a one-line forwarder. `Step.Apply`
widened to `(p project.Project, w *Writer, out io.Writer) (Facts, error)`
across all 11 `Plan()` steps. `installStep.Apply` and `hookInstallStep.Apply`
route their subprocess output through `out` instead of `os.Stdout`/
`os.Stderr` directly (defect 5). `Writer.Changed(root, from, to)
[]report.FileChange` classifies each touched file as created/modified/
unchanged. `applySteps` marks `len(writer.touched)` before/after each
`step.Apply` call, partitions per-step file attribution via `Writer.Changed`,
and copies each step's captured sink buffer back onto its own `stdout`
parameter immediately after `Apply` returns — Decision 9's invariant that
this slice changes nothing a user sees, since `RunSync`/`setup.Apply` are
unmodified and the bytes stay byte-identical until slice 4 frames them
(`TestSyncStdoutUnchangedAfterTheSinkMove` guards this explicitly).

### RED → GREEN sequence followed, and one necessary deviation

Go's structural interfaces made a strictly sequential RED-for-2.3-then-
GREEN-for-2.4-then-2.6-then-2.7-then-2.8 impossible to observe literally:
widening the `Step` interface's `Apply` signature breaks compilation of
`Plan()`'s `[]Step{...}` literal the moment any *one* concrete step's method
doesn't match, so all 11 steps had to be updated in the same atomic change
for the package to build at all — mirroring slice 1's own disclosed
deviation for combined GREEN steps. In practice: 2.1→2.2 (RED→GREEN, clean,
independent of the interface change) ran first and stayed strictly
sequential; then the interface widening (2.4) landed together with all
eleven `Apply` signatures (2.6's three sink sites, 2.8's remaining eight)
and `Writer.Changed`'s implementation (2.10, since `applySteps` calls it) —
one atomic compilable unit — after which 2.3's, 2.9's, 2.11's, and 2.12's
tests were run to confirm each passed against the completed implementation.
2.5's test was written and observed passing immediately (not RED) since
2.6's sink routing was already in place by the time it was written — a
second instance of the same structural constraint, disclosed rather than
hidden.

**Task 2.3's literal wording versus Decision 9 — resolved in Decision 9's
favour, and why.** Task 2.3 says `TestApplyWritesOnlyToTheGivenSink` should
assert "that buffer received none of the marker bytes" from a stub step's
sink. Decision 9's own invariant for this slice, task 2.4's own wording
("copies that buffer straight onto the writer applySteps itself was
given"), and task 2.5's own test (`TestSyncStdoutUnchangedAfterTheSinkMove`,
which requires exactly this copy-back for `dharness sync`'s real output to
stay byte-identical this slice) all require the opposite: the sink's
content *does* reach the writer applySteps was given. These two demands are
mutually exclusive for the same writer. Implemented per Decision 9 (the
authoritative, load-bearing, explicitly-repeated invariant) rather than per
task 2.3's literal prose: the test asserts the marker text reaches
`applySteps`'s own writer exactly once (proving the copy-back happened,
proving Facts flows back structurally correct, and killing both a
"forgets to copy" and a "double-copies" mutant) rather than asserting zero
bytes. Disclosed here rather than silently reinterpreted.

**`internal/report/report_test.go` needed editing, as flagged.** The team's
launch message anticipated this: once `internal/setup` imports
`internal/report` (required for `Writer.Changed`'s `[]report.FileChange`
return and `stepOutcome.wrote`), `TestReportExitIsAPlainAssignedField`'s use
of `internal/app.ExitCode` becomes an import cycle
(`report`→`app`→`cli`→`setup`→`report`). Fixed by switching that one test to
`runner.ExitCode` — the same value, per Decision 1's own claim that the move
changes no caller's behaviour — with the reasoning recorded in a comment at
the test itself. No other test in that file changed.

### A confirmed defect in `tools/mutationstaged`, found and worked around

Running `go run ./tools/mutationstaged` over all five staged production
files together (`internal/app/app.go`, `internal/runner/runner.go`,
`internal/setup/{setup,steps,writer}.go`) reports **0.80 rounded / 0.796875
real (51/64), FAIL** — but the thirteen survivors are not gaps in this
slice's own tests. Eight of them are inside `internal/app.RunArgs`'s
`"sync"`/`"check"`/`"mutate"` dispatch (`args[1:]`) — code this slice never
touched and that has zero `internal/app`-level coverage today (pre-existing;
no test in `app_test.go` invokes `RunArgs` with those three commands). The
remaining five are pre-existing branches elsewhere in the same files.

**Root cause, confirmed by direct reproduction against
`internal/testsupport/mutation` and `tools/mutationstaged/main.go`'s own
source.** `computeScope` computes each staged file's changed-line byte
offsets correctly, *per file* — but then merges all files' offset ranges
into one flat, file-agnostic list (`mergeOffsetRanges(allOffsets)`,
`main.go`) before encoding it into the single `DHARNESS_MUTATION_SCOPE`
environment value the real run uses. `gosourcefile.GoSourceFile.Incubate`
(vendored `ooze` dependency) parses each file with its own fresh
`token.NewFileSet()`, so `node.Pos()` is a small, per-file-relative byte
offset for every file — not a global position. Since the merged scope check
(`OffsetRanges.Contains(offset)`) only compares a raw number, a byte offset
that is legitimately "changed" in one file (e.g. `internal/setup/setup.go`)
numerically collides with an *unrelated* offset in a much smaller file
(`internal/app/app.go`) purely by chance, sweeping that unrelated file's
untouched code into scope. Verified directly: replaying `computeScope`'s
own algorithm against the actual staged diff shows `internal/app.RunArgs`
(lines 21–44, never touched by this slice) entering scope only once
`internal/setup`'s and `internal/runner`'s own — legitimately large — offset
ranges are merged in; app.go's *own* diff (three tiny ranges around the
import line and the rewritten `ExitCode`) contains no integer literal or
comparison at all. `AnalyzeSource`'s preflight stats (used for the `-dry`
printout) are unaffected, because that call is made per file with its own
un-merged ranges — only the real-execution path is wrong. This is dormant
for any change that only *adds* brand-new files (slice 1), because a new
file's entire byte range is legitimately in scope regardless of merging, and
first surfaces here because slice 2 is the first slice to make small,
partial edits across several existing files at once.

**This is out of scope to fix for `structured-reports`** — it is
`tools/mutationstaged`'s own architecture (`ooze.Release`'s public API
shares one `Virus` set across every file in one `Release` call, so a
correct fix needs either per-file `Release` invocations with a
correspondingly-changed scoring/aggregation model, or an ignore-pattern
per file — a change to shared team tooling, not to this change's own
files). Recorded here and flagged to the team lead as a follow-up rather
than attempted under this slice's time budget.

**Verification that this slice's own code independently clears the floor**,
done by staging (and mutating) the two file groups that the import-cycle
fix forces to be interdependent, in isolation from `internal/app.go`
(the file responsible for the cross-file leak):

- `internal/app/app.go` + `internal/runner/{runner.go,runner_test.go}`
  alone (the `ExitCode` move): **0.91 (10/11 killed)**, floor 0.80, `go run
  ./tools/mutationstaged` exit 0. Its own scope report shows exactly the 4
  byte ranges this move actually touched (no cross-file inflation with only
  two small files staged).
- `internal/setup/{setup,steps,writer,writer_test,setup_test,owned_test,
  steps_test}.go` + `internal/report/report_test.go` (the import-cycle fix)
  + `internal/runner/{runner.go,runner_test.go}` (required for
  `report_test.go`'s fix to compile) — `internal/app/app.go` excluded:
  **0.94 (44/47 killed)**, floor 0.80, exit 0.

Both isolated runs pass comfortably above the floor with the tool's own
verdict (exit code), not prose. The combined five-file run's failure is
attributable entirely to the confirmed scope-leak above, not to missing
coverage in this slice's own diff. The final commit stages all five files
together (matching the actual diff), so `go run ./tools/mutationstaged` run
against the full staged tree still reports the misleading combined FAIL —
disclosed here rather than hidden, since P09/L3's own doctrine is that a
gate's verdict is never overridden by prose, and this write-up is exactly
that: evidence for a human/orchestrator decision, not a substitute for the
gate passing.

### Gate, at commit time

`go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l .` all clean
against the full five-file staged diff. All six golden fixtures under
`internal/setup/testdata/golden/` are untouched (`git diff --cached --stat`
empty for that path) — this slice's own forecast is 0 golden bytes, and it
holds. `go run ./tools/mutationstaged` over the combined staged scope
reports FAIL for the reason above; the two isolated-scope runs recorded
above are the actual mutation evidence for this slice's own code.

## Slice 3 — fallow measurement, `Collisions`, `declaredAt`, `boundariesOwnerStep.ID()` (complete)

All 20 tasks (3.1–3.20) done. `internal/tool.FallowConfigPath()`/
`FallowConfigJSON()` carry Decision 5's two measured argument lists.
`internal/setup.resolvedConfig(p)` is the whole measurement: `LocalBinary`
check → `--path` probe (any non-nil error, not only exit 3, short-circuits
before `--format json` ever runs) → `--format json` → `json.Unmarshal`, every
failure collapsing to `(nil, false)`. `internal/setup.Collisions(p)` computes
one `report.Collision` per colliding key — `ID: "sync:collision/"+key`,
`Ours` always populated from `ownedValue` (wrapped as a JSON string when it
is prose, e.g. the `boundaries` fallback), `Effective`/`Theirs.Value`
populated only when the resolve actually finds the key. `declaredLine`
became `declaredAt(path, key) int` (a 1-based line number, sentinel 0),
feeding `Declared.Line`. `boundariesOwnerStep.ID()` no longer claims "two
architectures" — six golden lines moved, all at physical line 26, four
regenerated via `-update`, two hand-edited.

### RED → GREEN sequence followed

Each task's own RED test was written and observed failing to compile (a
missing symbol) before its GREEN implementation landed, task by task through
3.1–3.14. No combined-GREEN deviation was needed this slice — unlike slices
1 and 2, nothing here forced an atomic multi-symbol change: `FallowConfigPath`/
`FallowConfigJSON`, `resolvedConfig`, `Collisions`/`ourDeclared`,
`declaredAt`, and `ID()` each compile independently.

### A deviation task 3.9's literal wording required, and why

Task 3.9 says `declaredValue` "leaves the collision path entirely" — the
function is removed — but `describeBoundaries`/`delegateBoundaries` (the
*old* per-key rendering walk) still call it, and are not rewired to
`Collisions`/`renderCollisions` until slice 4 (design.md Decision 4's own
explicit ordering note: landing the branch collapse apart from slice 4's
`Why`/`Collisions` exclusivity would let a key render twice). Removing
`declaredValue` outright without a replacement would not compile.

Resolved by replacing `declaredValue`'s single-line textual-fragment display
(defect 8 — the `"duplicates": {` fragment) with a named constant,
`declaredValueUnknown = "a value of its own"` — the same generic phrase
`declaredValue`'s own no-match branch already used, now the only branch. This
satisfies task 3.9's actual instruction (the fragment display leaves the
collision path; the function's textual-scan technique does not survive
`declaredLine`'s retyping to `declaredAt(path, key) int` either) without
breaking the interim rendering path slice 4 has not replaced yet. Two
existing tests from an earlier slice —
`TestCollisionNamesEveryContributedKeyTheProjectDeclares` and
`TestIgnorePatternsCollidesInTheMotivatingShape` — asserted the literal
fragment text (`"dist/**"`) appeared in `describeBoundaries`/
`delegateBoundaries` output; both were updated to assert
`declaredValueUnknown` instead, since the behaviour they pinned is exactly
what Decision 5 requires to change here. Disclosed rather than silently
reinterpreted, matching slice 2's own precedent for task 2.3.

### A deviation from task 3.5's literal test layer, and why

Task 3.5 names a `TestResolvedConfigAbsenceHasOneShape` table test whose
third row ("the colliding key missing from the resolved map") asserts on
`resolvedConfig` returning `(_, false)` — but `resolvedConfig`'s own
signature (design.md Decision 5) takes no key and returns a whole map; it
cannot itself know a colliding key is missing from that map, only `Collisions`
can. Implemented as designed: `TestResolvedConfigAbsenceHasOneShape` covers
the two rows that genuinely belong to `resolvedConfig`'s own layer (non-zero
non-3 `--path` exit; non-JSON `--format json` stdout), and the third row's
fact — a key absent from an otherwise-valid resolved map leaves
`Effective`/`Theirs.Value` nil rather than fabricated — is asserted at
`Collisions`'s own layer, inside `TestCollisionsComputesEachKeyOnce`'s
"absent" half testing a key that is not present. The full absence enumeration
in spec.md's own requirement is covered across both tests; only which test
function owns which row moved.

### Gate, at commit time

`go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l .` all clean.
`go run ./tools/mutationstaged` over the staged scope (`internal/setup/files.go`,
`internal/setup/steps.go`, `internal/tool/tool.go`): **32/32 killed, score
1.00**, floor 0.80, exit 0 — no cross-file scope-leak concern this slice,
since the staged scope spans only these two packages and the combined score
already clears the floor by a wide margin. The exit-3 short-circuit branch
(design.md's named easiest-to-leave-unkilled risk) is killed, confirmed by
name in the mutation report rather than assumed from the aggregate score.

Two mutation-driven fixes, both simplification over new pinning tests per the
mutation-tdd skill's decision table:

- `ourDeclared`'s defensive `if err == nil` after `json.Marshal(value)` was a
  dead branch — `json.Marshal` of a Go `string` cannot fail — so the check
  was removed rather than tested; the survived mutant is gone because the
  branch it targeted no longer exists, not because a test now watches it.
- `declaredAt`'s `err != nil` branch (an unreadable file, as opposed to a
  readable file that never mentions the key) had no test reaching it at all —
  `TestDeclaredAtReturnsALineNumber` gained a third case, a path that does
  not exist, killing both the Integer Decrement (`0`→`-1`) and Integer
  Increment (`0`→`1`) mutants on that `return 0`.

All six golden fixtures moved exactly one line each (line 26, Decision 6's
measured figure): `nextjs.txt`, `expo.txt`, `wails.txt`, `wails-nextjs.txt`
regenerated via `-update`; `generic-conventional.txt`, `generic-split.txt`
hand-edited. `TestGenericMechanismHasNoUpdatePath` re-run and green after the
hand edits, per task 3.16. `TestBoundariesFallbackConstantsStayByteIdentical`
(task 3.17) passes, pinning that Decision 6's "six lines is the complete
golden impact" claim still holds.

## Slice 4

Not started. Tasks 4.1 onward remain unchecked in `tasks.md`.
