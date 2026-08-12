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

## Slices 2–4

Not started. Tasks 2.1 onward remain unchecked in `tasks.md`.
