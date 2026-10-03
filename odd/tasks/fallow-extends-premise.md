# Correct the false `extends` premise, and stop the collision verdict lying

## Why this unit exists

dharness tells consumers, in the collision report, in the comment it writes into their own projects, and in the preset rationale, that fallow's `extends` **replaces a key rather than merging it**. That claim was recorded on 2026-08-11 as measured against fallow 3.14.0.

A measured diagnosis on 2026-10-02 (recorded in `odd/tasks/fallow-extends-premise.md` of the integration evidence tree, mirrored in memory) disproved it:

- Objects **merge field by field**. A child declaring only `duplicates: {mode: "mild"}` over a parent declaring `{mode: "semantic", minOccurrences: 3, threshold: 3}` resolves to `mode: mild`, **`minOccurrences: 3`**, **`threshold: 3.0`** - the parent's unmentioned fields survive.
- **Arrays and scalars are replaced whole** (`ignorePatterns` measured; `boundaries.zones` measured).
- 3.14.0 and the current release behave **identically**. This is not version drift and there is no version boundary to bisect; the recorded premise was never true of object-valued keys.
- The mixture can be **invalid**: a parent declaring `boundaries.zones` + `boundaries.rules` and a child declaring only `boundaries.zones` makes fallow exit **2** with `boundaries.rules[0].from: references undefined zone`. The real failure mode is a hard error, not silence, and the result can be a combination neither side wrote.

Two consequences the current text gets wrong, in opposite directions: it promises silence where there can be an error, and it says one side wins where a mixture runs.

## Deliverable

### D1. Correct the prose

Replace the replacement-premise with the measured semantics, in these places:

1. `internal/setup/steps.go` - the comment block above `boundariesOwnerStep` (currently 'fallow's `extends` replaces a key rather than merging it. Measured against fallow 3.14.0 ... the parent's value is discarded whole - no error, no warning'). Rewrite to the measured semantics: objects merge field by field, arrays and scalars replace whole, the mixture can enforce a combination neither side wrote, and it can make fallow refuse the config outright. Keep the step's motivation honest and narrow: the step exists because a project can silently drop part of dharness's declaration, and because the file does not say which fields are in force.
2. `internal/setup/steps.go` - the `collisionResolutions` comment ('fallow's `extends` can only ever honour one of them'): 'which one wins' is a question about fields, not about sides.
3. `internal/setup/steps.go` - `boundariesFallbackWhy` (a byte-frozen constant):

```
%s declares its own `boundaries`, and fallow's `extends` merges that key\nfield by field while replacing each array in it whole - so the project's\n`zones` can be in force while dharness's `rules` still are, and neither side\nsays which combination you got.
```

4. `internal/setup/steps.go` - `boundariesFallbackDescribe` (also a byte-frozen constant): keep both resolutions, but stop justifying them with 'only one of them runs':

```
Move the zones and rules from %s into %s, or delete the block dharness\nowns and keep the project's. Either is a valid answer; both at once is not,\nbecause the two blocks merge field by field and a zone the other one removed\ncan still be referenced.
```

5. `internal/setup/steps.go` - `renderCollisions`:

```
%s declares its own value for %s, and fallow's `extends` merges each of\nthose objects field by field - the fields the project declares win, the fields\nit omits silently keep dharness's value, and arrays and scalars are replaced\nwhole. More than one value per key is in force, and the configuration does\nnot say which:\n
```

6. `internal/setup/owned.go` - the comment inside `architectureSkeleton`, which is written into consumer projects and never refreshed in already-adopted repositories (stated limit, not a bug to fix here):

```
Declare `boundaries` here rather than in the project's\nown fallow config: `extends` merges this key field by field and replaces each\n`zones`/`rules` array whole, so a project declaring its own zones silently\ndrops the ones here while these rules stay.
```

7. `internal/preset/generic.go` - the rationale comment for carrying the duplication floor as a manifest fact. The measured form is narrower and stronger as a justification: `extends` merges this object field by field, so a project declaring only `mode` silently keeps dharness's `minOccurrences` and `threshold`. Same correction in the comment above `TestGenericCarriesTheDuplicationCeiling` in `internal/preset/generic_test.go`. Do NOT touch the `Because` text or the fact values.

### D2. Stop `Collisions` lying about what is in force

`internal/setup/steps.go` `Collisions` sets `Effective` to `ours` only when the resolved value byte-equals dharness's whole value, otherwise `theirs`. Under field-by-field merge a partially overridden object equals neither, so the report tells the agent the project's value runs while dharness's fields are demonstrably still in force.

Classify honestly, and only from measured values:

- When `Theirs.Value` was never measured, leave `Effective` nil. Never fabricate.
- Compare dharness's declared value against the resolved value as JSON objects (`objectFields` in `internal/report/human.go` already does this decoding and is the model to follow; it is unexported in that package, so implement the comparison where it belongs rather than duplicating a decoder needlessly). A field of dharness's is **kept** when the resolved object carries the same value for it, and **lost** when it is absent or carries a different value.
- No field lost -> `ours`. Every field lost -> `theirs`. Some kept and some lost -> `mixed`.
- For values that are not JSON objects (arrays, scalars), keep today's rule: equal -> `ours`, otherwise `theirs`. Replacement semantics admit no mixture.
- `mixed` is a third state of the `effective` field. Document it on `report.Collision` in `internal/report/report.go` (the field's doc comment currently implies two sides and a whole value).

### D3. Render the third state distinguishably

`internal/report/human.go` `effectiveMark` marks the side where `Effective == side` with `   <- this one runs`. With `mixed` neither side gets a mark, which today means exactly one thing: 'never measured'. Those must not be conflated. Add a `mixed` rendering: both sides carry `   <- this side's fields are in force`, keeping the existing mark text for the single-winner cases. Keep `narrowToDifferences` as it is; it already narrows honestly, and under merge it is what makes the block readable.

## Behaviour this must not change

- The collision step's existence, its two resolutions, and the advice to declare `boundaries` in the file dharness owns.
- The `duplicates` fact values (`semantic`, `minOccurrences: 3`, `threshold: 3`) and the preset `Because` text.
- The JSON shape apart from the new `effective` value; values stay whole and unencoded.

## Tests and fixtures

- Test-first for D2: a table over the three outcomes (all kept, all lost, mixed) plus the not-measured case (nil), and one case per non-object shape. RED first: the current byte-equality rule must fail the mixed case.
- Update the assertions that carry the old sentences: `internal/setup/steps_test.go` (the `wantWhy` literal), `internal/setup/setup_test.go` (the collision note), and the tests guarding the two byte-frozen fallback constants. Changing those constants is the deliberate act of this unit; the guard must be updated, not deleted.
- `internal/report/human_test.go`: keep the existing `ours`/`theirs` mark tests passing and add the `mixed` case (a mark on both sides) and a guard that `mixed` is distinguishable from never-measured.
- Goldens: five framework goldens regenerate with `go test ./internal/setup -run TestFrameworkGoldens -update`. The two generic goldens are frozen by design (`TestGenericGoldenIsUnchanged` has no `-update` path, and another test proves the source never wires the flag): derive them by hand from the failure's want/got output. Read every golden diff and confirm the only changes are the corrected sentences and the new marks.

## Docs and records

- `docs/learning-log.md`: append exactly one new dated line for the correction. The log is append-only; the 2026-08-11 entry stays as it is, and the new line says what was measured and that the old claim was over-generalised from array replacement. The line must fit on one line.
- `openspec/specs/setup/spec.md`: the 'Known limit, measured after this shipped' blockquote carries the same false claim. Correct it to the measured semantics, keeping its status as a stated limit rather than a guarantee. Check `openspec/changes/framework-presets/` for the same sentences and correct them the same way if they are notes rather than approvals.

## Runtime proof (mandatory, not optional)

This changes text a person and an agent read. A green suite is not evidence: build the binary outside the repository and run it in a throwaway project that has a real collision - `.dharness/fallow.jsonc` declaring `boundaries` with `zones` and `rules`, and a project `.fallowrc.json` declaring only `boundaries.zones` - then show the human and JSON output: the corrected sentence, the `mixed` verdict with its mark, and fallow's own hard error when the merged config dangles a zone reference. Never run the binary against this checkout.

## Limits to state, not fix

- The corrected skeleton comment does not reach repositories that already adopted dharness (`replaceRegion` returns the existing text when the preset region is empty). Say so in the commit body and the pull request; do not attempt a migration here.
- `fallow config` was not available to the 2026-08-11 measurement's version and is available now; the correction is worded to the measured behaviour, not to a version range, because none exists.
## Evidence (implemented and green; runtime proof outstanding)

Writer: `mursivq3-f-v8nw`. Nineteen files changed, nothing staged or committed by the writer.

- RED, observed before the implementation: `TestCollisionsClassifiesEffectiveByFieldsNotWholeValue/a_mix_of_kept_and_lost_fields` fails with `Effective = "theirs", want "mixed"`.
- GREEN: the same table passes with rows for every field kept, a mix, every field lost, never measured, an array replaced by an equal array, an array replaced by a different one, and a scalar against a resolved object.
- `report.Effective` reuses the existing `objectFields` decoder and returns `measured == false` rather than fabricating a verdict when a side was never measured; `effectiveMark` renders `mixed` on both sides and leaves the winner mark for the single-winner cases.
- Goldens: the five framework fixtures regenerated with the sanctioned `-update`; the two frozen generic fixtures hand-derived from the failing want/got output. Every one of the seven shows exactly three hunks (`git diff --unified=0 | grep -c '^@@'`), confined to the two fallback sentences and the skeleton comment written into `.dharness/fallow.jsonc`; the human collision marks are not rendered in the plan fixtures and are covered by `human_test.go`.
- Checks: `gofmt -l .` clean, `go build ./...`, `go vet ./...`, `go test ./... -count=1` (all packages ok), focused `./internal/setup ./internal/report ./internal/preset` ok, and the golden plus byte-identity guards ok.
- Glyphs: the specification text was ASCII-normalised in transit and quotes the existing mark as `<- this one runs`. The repository's own glyphs (`←`, `—`) were used in the replacements, which is what `TestBoundariesFallbackConstantsStayByteIdentical` and the goldens assert.

## Residuals, declared rather than silently expanded

These still carry the old premise and are deliberately outside this unit:

- `openspec/changes/framework-presets/proposal.md` (lines ~36-43 and ~213): an approved historical proposal, left as the record of what was decided then.
- `openspec/changes/archive/2026-08-11-unify-init-and-sync/archive-report.md:125` and `openspec/changes/structured-reports/target-report.md:82`: not this unit's surfaces. The second belongs to another change and should be corrected when that change is next touched.
- `docs/learning-log.md:85` and `:100`: the append-only log keeps its 2026-08-11 entries; the correction is the new dated line, not an edit.

## Runtime proof

Delegated to an independent verifier: build the binary outside the repository, run it in a throwaway project carrying a real `boundaries` collision, and show the corrected sentence, the `mixed` verdict with its mark in both human and JSON output, and fallow's hard error when the merged config dangles a zone reference.
## Corrective pass 1 - the two defects the verifier found

The first independent verification reported BLOCKED: `ditto staged` 0.90 with two survivors, and a measured classification defect. Both are fixed here, test-first, and both are recorded as what they were rather than as equivalent mutants.

### Defect 1 - JSON numbers compared by spelling (blocking, measured with the binary)

`objectFields` keeps each field's raw compacted text, and the classifier compared those strings. `threshold: 3` (dharness) against `threshold: 3.0` (fallow's resolved config) is one value written two ways, so a fully-kept declaration was classified `mixed`. The verifier's decisive control: the project declaring the same value dharness declares produced `{"effective": "mixed"}` and a block showing only `{threshold: 3}` versus `{threshold: 3.0}`, both marked mixed. The required case A had read `mixed` correctly for a different reason, which masked it.

Fix: `sameValue` compares values, not spellings - numbers through `math/big.Rat` so integers past float64's mantissa stay distinct, strings, booleans and null by value, arrays and objects structurally at every depth. It is used for the kept/lost counting, for the non-object path, and for the human narrowing's identical-field test, so the comparison and the rendering cannot drift. Raw spellings are still what the human view prints.

### Defect 2 - two defensive branches with no owning scenario (coverage)

`ditto staged` reported `internal/report/human.go:665:6` (`ours == nil` to `false`) and `:665:21` (`theirs == nil` to `false`) as survivors. They were unreachable from the suite: `Collisions` calls the classifier only after a successful measurement, where dharness's own value is always present, and the never-measured table row skipped the assignment entirely. The gap was closed with behaviour coverage: `TestEffectiveReportsNoVerdictWhenASideWasNeverMeasured` calls the exported function directly with either side and both absent, and the setup table gained a row where fallow answers successfully but the resolved value does not carry the key.

### Finding - the corrected `renderCollisions` wording is not observable

`writeDelegatedBlock` renders `case len(step.Collisions) > 0:` and only falls to `case step.Why != "":` otherwise, so a step carrying collisions never prints its `Why`, and its JSON entry carries `collisions` with no `why` key. Confirmed against the verifier's real output: the collision step shows the structured block and no `Why`, while the two delegated steps without collisions show theirs. The corrected `renderCollisions` sentence therefore reaches no human or JSON output in that state - it is unit-asserted text, and this unit does not restructure the renderer.

Because of that, the explanation was added where the agent actually reads it: the collision block prints `mixedExplanation` - fallow merges an object-valued key field by field, the fields dharness declares that the project did not override are still in force, arrays and scalars are replaced whole - when and only when the verdict is `mixed`. It shares the value lines' indentation and is wrapped by the existing helper.

### Mutant evidence for the pass

Each guard was deleted, the owning test observed failing, and the file restored byte-identically (`cmp`), never `git diff --quiet`, because the file already differed from the index:

| Guard | Owning test with the guard deleted |
|---|---|
| `ours == nil` to `false` | FAIL: `TestEffectiveReportsNoVerdictWhenASideWasNeverMeasured/dharness's side absent` |
| `theirs == nil` to `false` | FAIL: `.../the project's side absent` |
| number comparison made spelling-sensitive | FAIL: `Effective({"threshold":3},{"threshold":3.0}) = "theirs", want "ours"`, plus the exponent-form and nested-object rows |
| `mixedExplanation` gated on the wrong state | FAIL: five assertions, including `TestEffectiveMixedMarksBothSides` |

A first attempt at the number mutant did not compile (it left a variable unused) and was discarded rather than counted: `ditto staged` excludes non-compiling mutants, and a build failure is not a kill.

Staged after this pass: 20 files, 964 insertions, 125 deletions. The seven goldens were not touched by this pass.
## Corrective pass 2 - mutation coverage of the corrective pass

The second independent verification: every runtime case green (mixed with its explanation; the all-kept control reading `ours` with the `3`/`3.0` spelling hidden; the dangling boundary case showing fallow exit 2 with the verdict honestly absent), both previous nil survivors gone - and `ditto staged` still blocked at 0.84 with seven survivors.

### Three were equivalent by construction, and were removed rather than chased

`narrowToDifferences` tested `inOurs && inTheirs && sameValue(ov, tv)` before hiding a key. Both presence conjuncts are redundant: a side that never declared the key carries no value, and `sameValue` never equates an absent side with a present one, so no mutation of either conjunct can change the outcome - the two rules below the test already decide a side's absence. The conjunction was deleted, its reasoning recorded in the code, and the three mutants that lived on it can no longer be generated. This is the same resolution the earlier work unit used for an unreadable tuple slot: a mutation that cannot change behaviour is not a test's job to kill, it is a form to remove.

### Four were real coverage gaps, each now owned by a measured scenario

- **The map-branch length check.** The comparison iterates its *first* argument, so a declaration carrying more keys than the resolved value answers "equal" for every key it holds unless the length check stops it. New row: `{"a":{"b":1,"c":2}}` against `{"a":{"b":1}}` is `theirs`.
- **The array-branch length check**, already owned by the existing `[1,2]` against `[1]` row; verified by hand.
- **The classifier's own bookkeeping.** Every row declared a single field, so `continue` and `break` were indistinguishable. New row: `{"mode":"mild","threshold":3}` against `{"mode":"semantic","threshold":3}` is `mixed` - one kept field and one lost field in one declaration.
- **The explanation's wrapping.** No assertion bounded it. New test: the mixed block's explanation lines stay within the report width and every continuation lines up with the first line's indentation.

### Kills proven by hand, with the byte-copy witness

`git diff --quiet` cannot witness a mutation on a file that already differs from the index, so each mutation was witnessed with `cp` and `cmp`, and the mutated hunk was printed to prove it applied:

| Deleted guard | Owning test, observed failing |
|---|---|
| map length check to `false` | `a nested object dharness declares more keys of`: `Effective({"a":{"b":1,"c":2}}, {"a":{"b":1}}) = "ours", want "theirs"` |
| array length check to `false` | `an array of a different length` |
| `continue` to `break` | `a kept field and a lost field in one declaration`: `= "ours", want "mixed"` |
| wrap width `-` to `+` | two explanation lines reported over the report width |
| wrap indent `0` to `1` | continuation lines indented 16 spaces where the first uses 15 |

### The last survivor was a flaky kill, and the fix was to remove the branch

The third `ditto staged` run came back at 0.98 with one survivor: `human.go:825` Loop Break, the `continue` in the classifier's kept/lost loop - the very mutant a hand-run had already shown dying. Both readings were right, and that is the finding. `break` only fires on a **kept** field, so with one kept and one lost field the mutant changes the verdict only when the kept field happens to be visited first, and Go randomises map order: the hand-run hit that order, the gate run did not. A test owning that outcome would fail half the runs, which is worse than no test, and no expected verdict is mutant-proof, because the mutant's possible results cover `ours`, `mixed` and `theirs`.

The loop became arithmetic: count the kept fields, derive the rest as `len(o) - kept`. There is no `continue` left to mutate, and the subtraction is decided by any row where a field is kept - proven by mutation, run four times and failing all four rather than once.
One process lesson, recorded because it produced a false reading: a first attempt at the `continue` mutant used a `^\t\t\tcontinue$` pattern that matched nothing and reported the mutant as surviving. A mutant reported as surviving has to be shown to have applied before it is believed; the scoped address and the printed hunk settled it, and the mutant did die.

