# Tasks: structured-reports

Inputs: `spec.md` (three new capabilities — `sync-report`, `step-outcome`,
`config-collision` — plus two modified — `project-sync`, `step-delegation` —
amended: six-value `Status` with `retracted`; the collision-identity
requirement split into `ID()` making no architecture claim and the rendered
entry/handle naming the actual keys; "Zero golden bytes move" replaced by the
measured six-line figure), `design.md` (nine decisions, Decision 9's
authoritative slice plan and line forecast), `target-report.md` (the approved
shape, its failure variant, its JSON twin), `proposal.md` (scope, the
resolved question round). TDD: each task's named test is the RED step,
written before its implementation and made GREEN; `go run
./tools/mutationstaged` (MUTATE, floor 0.80) closes every slice per P09; a
REFACTOR pass runs the four repo-wide checks before each slice is considered
done.

**Word-budget note.** This artifact exceeds the sdd-tasks skill's word
guideline. The brief mandates a named RED test and the fact it pins for every
task, the six explicit golden-fixture lines, the `ExitCode` move, and the
`sync_test.go` shape-vs-behaviour rule stated as its own task — a change this
detailed cannot carry that content and stay short. Completeness is kept;
brevity is sacrificed deliberately.

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | ~920–1,300 (design.md Decision 9's realistic total) |
| 400-line budget risk | High — slice 1 (320–420) and slice 4 (280–420) can each individually exceed the 400-line review budget |
| Chained PRs recommended | Yes |
| Suggested split | PR1(slice 1) → PR2(slice 2) → PR3(slice 3) → PR4(slice 4) |
| Delivery strategy | auto-chain |
| Chain strategy | stacked-to-main |

```text
Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High
```

### Suggested Work Units

| Unit | Goal | PR | Focused test | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 1 | `internal/report`: types, JSON tags, human renderer, `widths`/`wrap`, glyph block | PR1 | `go test ./internal/report/...` | pure value construction, no `project`/`setup` dependency | Revert PR1; no consumer yet (Decision 9) |
| 2 | `Step.Apply` → `(Facts, error)` + `out io.Writer` across ten steps; `Writer.Changed`; `ExitCode` moved to `internal/runner` | PR2 | `go test ./internal/setup/... ./internal/runner/... ./internal/app/...` | stubbed `runner.Run` capturing sink writes | Revert PR2; independent of PR3/PR4 |
| 3 | `tool.FallowConfigPath`/`FallowConfigJSON`; `resolvedConfig`; `setup.Collisions`; `declaredAt`; `boundariesOwnerStep.ID()`; six golden lines | PR3 | `go test ./internal/setup/... ./internal/tool/...` | `runner.SetForTest` recording every `Command` | Revert PR3; six-line golden diff reviewable alone |
| 4 | `setup.Run`; `RunSync` rewrite; `--format json`; failure variant; collision branch collapse; `sync_test.go` rewrite | PR4 | `go test ./internal/cli/...` | full `RunSync` integration against stubbed `runner.Run` and a constructed `project.Project` | Revert PR4 alone restores today's output (design.md, Migration/Rollout) |

---

## Slice 1 — `internal/report` (~320–420 lines, golden bytes: 0)

Capability: `sync-report` (the model, `Summary`, the human renderer, the
summary-before-detail requirement), `step-outcome` (`FileChange`/`Kind`
types), `config-collision` (`Collision`/`Declared` types). Design Decisions
1, 3 (type only), 4 (type only), 7 (alignment, wrapping, glyphs). Why here:
pure addition with no consumer — reviewable against the approved report
alone (design.md Decision 9).

**MERGE CONDITION — MEASURED AND CLEARED, 12 August 2026.** Decision 7's
legacy-console glyph risk is a merge condition on this slice. The
orchestrator ran it: a Go binary printing `── ✓ ✗ ■ │ ·` on Windows 11
(build 10.0.26200) rendered every glyph correctly at the console's default
code page 437, and again after forcing `[Console]::OutputEncoding` to 437
and to 850 from PowerShell, in a no-profile PowerShell session, and in Git
Bash. The glyph set is cleared. **Honest limit**: true legacy `cmd.exe`
conhost could not be exercised — `cmd` would not launch non-interactively in
this environment — so this measures Windows 11's modern console host at code
page 437, not a genuine legacy conhost. The ASCII fallback (`- # | + x .`,
six constants, no call-site change) stays a recorded contingency in
design.md; it is not implemented here. Recorded in `docs/learning-log.md`
(task 1.16).

- [x] **1.1** RED: `internal/report/report_test.go` —
      `TestStatusValuesMarshalToSpecWords`: table over all six `Status`
      constants (`Applied`, `Delegated`, `Satisfied`, `Failed`,
      `NotReached`, `Retracted`); `json.Marshal` each and assert the exact
      words `"applied"`, `"delegated"`, `"satisfied"`, `"failed"`,
      `"not-reached"`, `"retracted"`. Pins spec's six-value status
      enumeration (amended from five). The package does not exist yet, so
      this fails to compile.
- [x] **1.2** GREEN: `internal/report/report.go` — package doc; `Status`
      (six constants) and `Kind` (three constants) string sets; `Report`,
      `Summary`, `StepResult`, `FileChange`, `Collision`, `Declared`,
      `Note`, `Evidence`, `Rollback` structs with JSON tags, verbatim to
      design.md Decision 1's choice block (`Declared.Value` is
      `*json.RawMessage`). Obs: 1.1 passes.
- [x] **1.3** RED: `TestAbsentIsNotEmpty` — marshal a `StepResult`/`Collision`
      with `Effective == nil` and `Declared.Value == nil`; decode into
      `map[string]json.RawMessage` and assert `"effective"`/`"value"` keys
      are absent, not present with `null`. Then set `Effective =
      ptr("theirs")` and assert the key is present with that value. Pins
      §09/§17's absent-vs-empty rule (design.md Property 3) — a mutant that
      drops `omitempty` or fabricates a default dies here.
- [x] **1.4** RED: `TestSummaryCarriesNoOmitempty` — a zero-valued `Summary`
      decodes into `map[string]json.RawMessage` with every key present
      (`steps`/`applied`/`delegated`/`satisfied`/`failed`/`retracted`/`ms`)
      even at zero. Pins "a measured zero is a measured zero, not a missing
      key" — the same distinction as 1.3, in the other direction.
- [x] **1.5** RED: `TestReportExitIsAPlainAssignedField` — construct two
      `Report` values whose `Summary.Failed == 0` and `Summary.Delegated ==
      1` (the disagreeing case Decision 1's rejected-alternative names),
      one with `Exit: runner.ExitCode(nil)` and one with `Exit:
      runner.ExitCode(&runner.ExitError{Code: 2})`; assert the encoded
      `"exit"` equals exactly the value assigned, unaffected by
      `Summary.Failed`. Pins "the verdict is not computed" (design.md
      Property 2) — a mutant substituting `failed == 0` disagrees with this
      case and dies.
- [x] **1.6** RED: `TestWidthsIsTheMaxPerColumnMeasuredInRunes` —
      `widths([][]string)` table including a column holding the
      multi-byte glyph `"──"` alongside single-byte cells of the same
      visual width; assert the returned width is a rune count. Never
      asserts on a rendered line's spacing — pins design.md's "column
      rule, not spacing" testing-strategy row.
- [x] **1.7** Mutation guard: `TestWidthsMaxComparisonSurvivesASecondWiderRow`
      — a two-row table whose second row is wider in every column; assert
      `widths` reports the second row's measurement. Kills the mutant that
      drops or inverts the max comparison — design.md's Mutation coverage
      table, 4th named branch. Obs: killed mutant in `go run
      ./tools/mutationstaged`, not merely covered.
- [x] **1.8** RED: `TestWrapNeverExceedsWidthAndPreservesWords` — `wrap(text,
      width, indent) []string` table over several word counts and widths,
      including a word whose length equals `width` exactly; assert (a) no
      returned line's rune length exceeds `width`, (b) the words rejoined
      from the output equal the input's words, in order. A mutant on `<`
      vs `<=` dies on the boundary case. Pins "the bound, not the break
      points."
- [x] **1.9** GREEN: `internal/report/human.go` — `widths`, `wrap`, the
      six-constant glyph block (`glyphRule ─`, `glyphSummary ■`,
      `glyphGutter │`, `glyphApplied ✓`, `glyphFailed ✗`, `glyphSeparator
      ·`), used from nowhere else, per Decision 7. Obs: 1.6–1.8 pass.
- [x] **1.10** RED: `TestWriteHumanRendersEverySummaryFirst` — construct a
      `Report` with several `StepResult` entries; call `WriteHuman`; assert
      every `StepResult.ID` string appears in the output, and the summary
      line's marker text appears at a byte offset before the first
      per-step detail block's marker. Pins "the summary is rendered before
      per-step detail" (both scenarios).
- [x] **1.11** RED: `TestWriteHumanCollisionKeyRendersExactlyOnce` — a
      `Report` whose one `StepResult` carries `Collisions:
      []report.Collision{{Key: "duplicates"}}`; assert
      `strings.Count(output, "duplicates") == 1`. Pins `config-collision`'s
      "one colliding key renders exactly once in the human view" scenario
      at the renderer layer.
- [x] **1.12** RED: `TestWriteHumanResidueNoteListsEveryEntryAndNoFlagRef` —
      a `Report` with a `Note{Kind: "residue", Entries: [...]}}`; assert
      every entry string appears in the output and the output contains no
      substring `"--show"`. Pins "notes list their entries in full and
      never reference an unimplemented flag."
- [x] **1.13** GREEN: `internal/report/human.go` — `WriteHuman(w io.Writer,
      r Report) error`: the five blocks (summary, applied, left-to-you,
      already-in-place, notes) plus the closing tally block, wired to
      `widths`/`wrap`/the glyph block. Obs: 1.10–1.12 pass; `go vet ./...`
      clean.
- [x] **1.14** RED: `TestWriteJSONMatchesTheReferenceEncoding` — call
      `WriteJSON(&buf, r)`; assert `buf.Bytes()` is byte-equal to
      `json.NewEncoder(&ref).SetIndent("", "  ").Encode(r)`'s output for the
      same value, and `json.Valid(buf.Bytes())` covers the whole buffer
      (no leading/trailing non-JSON bytes). Pins "`--format json` emits
      valid JSON... no non-JSON text precedes or follows it."
- [x] **1.15** GREEN: `internal/report/json.go` — `WriteJSON(w io.Writer, r
      Report) error` = `json.NewEncoder(w).SetIndent("", "  ").Encode(r)`.
      Obs: 1.14 passes.
- [x] **1.16** Documentation: `docs/learning-log.md` — one dated line (12
      August 2026) recording this slice's merge-condition measurement: the
      glyph set rendered correctly at code page 437 by default, and again
      forced to 437 and 850, in PowerShell (with and without a profile) and
      Git Bash, on Windows 11 build 10.0.26200; and its stated limit — true
      legacy `cmd.exe` conhost was not exercised because `cmd` would not
      launch non-interactively in this environment. Obs: the line states
      the limit alongside the result, not the result alone.
- [x] **1.17** MUTATE + REFACTOR: `go run ./tools/mutationstaged` over
      `internal/report` (floor 0.80, verdict from its exit code, never
      prose); `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l
      .` all clean.

---

## Slice 2 — `Step.Apply` sink + `Writer.Changed` + `ExitCode` move (~140–200 lines, golden bytes: 0)

Capability: `step-outcome` (sink requirement, structured-outcome
requirement, file-attribution requirement, classification requirement).
Design Decision 2 (`Facts`, `Apply` signature), Decision 3 (`Writer.Changed`),
Decision 1 (`ExitCode` move — the import-cycle finding: `internal/app`
imports `internal/cli`, so `internal/cli` cannot reach `app.ExitCode`). Why
here: mechanical across ten sites; lands before anything reads it (design.md
Decision 9). Decision 9's own invariant for this slice: **nothing a user
sees changes** — subprocess output moves off the process's real stdout and
onto a sink, but `RunSync` (unmodified until slice 4) still writes that
sink's bytes straight back to its own stdout, so the bytes stay identical
until slice 4 frames them.

- [x] **2.1** RED: `internal/runner/runner_test.go` —
      `TestExitCodeMapsNilZeroPropagatesToolCode`: `ExitCode(nil) == 0`;
      `ExitCode(&ExitError{Code: 2}) == 2`; `ExitCode(&ExitError{Code: 0})
      == 1` (falls back, matching today's `app.ExitCode`). `internal/runner`
      has no `ExitCode` yet — this fails to compile.
- [x] **2.2** GREEN: `internal/runner/runner.go` — move `ExitCode(err
      error) int` here verbatim from `internal/app/app.go:64`, beside
      `ExitError`. `internal/app/app.go` — `ExitCode` becomes a one-line
      forwarder: `func ExitCode(err error) int { return
      runner.ExitCode(err) }`. Obs: 2.1 passes; `internal/app`'s existing
      `ExitCode` test, unmoved, still passes against the forwarder — no
      caller moves (Decision 1).
- [x] **2.3** RED: `internal/setup/setup_test.go` —
      `TestApplyWritesOnlyToTheGivenSink`: a stub `Step` whose `Apply(p, w,
      out)` writes a marker string to `out` and returns `Facts{Installed:
      []string{"pkg"}}`, `nil`; drive it through `applySteps` with a
      separate buffer standing in for the real process stdout; assert that
      buffer received none of the marker bytes, and the returned outcome
      carries `Installed == []string{"pkg"}`. Names the fact: the sink
      separates subprocess bytes from the process's own stream, and
      `Facts` carries structured data a byte stream cannot (defect 5;
      spec `step-outcome`, first two requirements).
- [x] **2.4** GREEN: `internal/setup/setup.go` — widen `Step.Apply` to
      `(p project.Project, w *Writer, out io.Writer) (Facts, error)`; add
      `type Facts struct { Installed []string }`; `applySteps` gives each
      step a `bytes.Buffer` as `out`, then copies that buffer straight
      onto the writer `applySteps` itself was given, immediately after
      `Apply` returns — preserving today's interleaving and byte content
      per this slice's invariant. Obs: 2.3 passes.
- [x] **2.5** RED: `internal/cli/sync_test.go` —
      `TestSyncStdoutUnchangedAfterTheSinkMove`: run `RunSync` (unmodified
      this slice) with a stubbed `runner.Run` producing known,
      recognisable subprocess output for `installStep`; assert that output
      still appears inline, in the same position it does today. The
      explicit guard for Decision 9's "slice 2 changes nothing a user
      sees."
- [x] **2.6** GREEN: `internal/setup/steps.go` — the three sink sites
      (`installStep.Apply:80`, its rollback compensation at `:82`,
      `hookInstallStep.Apply:863`) stop writing `os.Stdout`/`os.Stderr`
      directly and route through the given `out`:
      `runner.Run(cmd, out, out)`, matching `check.go:124`'s one-stream
      shape. `installStep.Apply` additionally returns
      `Facts{Installed: integrationPackages(p)}` on success. Obs: 2.3 and
      2.5 both pass; `rg "os.Stdout|os.Stderr" internal/setup/steps.go`
      returns nothing inside an `Apply` body.
- [x] **2.7** RED: table-driven, over the ten concrete `Step` `Apply`
      implementations design.md Decision 2 counts (nine of ten change
      only their return statement; the remaining one, `installStep`,
      is 2.6's): each recompiles against the widened signature and every
      existing error path returns `Facts{}, err` with no other behaviour
      change. Obs: each step's existing `*_test.go` recompiles and passes
      unmodified in assertions — signature-only change.
- [x] **2.8** GREEN: `internal/setup/steps.go` — update the remaining nine
      `Apply` signatures (`ownedFilesStep`, `fallowExtendsStep`,
      `boundariesOwnerStep`, `lefthookExtendsStep`, `eslintExtendsStep`,
      `legacyLintConfigStep`, `mcpStep`, `hookInstallStep`,
      `agentSkillStep`, `architectureStep` — the "Apply is unreachable"
      contract-assertion bodies for `boundariesOwnerStep` and
      `legacyLintConfigStep` return `Facts{}, err` too). Obs: 2.7 passes;
      `go build ./...` succeeds.
- [x] **2.9** RED: `internal/setup/writer_test.go` —
      `TestChangedClassifiesCreatedModifiedUnchanged`: `t.TempDir()` with
      three real files — one written where none existed (`created`), one
      pre-existing rewritten to different bytes (`modified`), one
      pre-existing rewritten to byte-identical bytes (`unchanged` —
      design.md's named mutation-coverage row); call `w.Changed(root,
      from, to)` around the writes; assert each `FileChange.Kind` matches
      and `FileChange.Path` is root-relative and names its directory
      (defect 9). Pins the classification requirement's three scenarios
      and the file-attribution requirement's directory scenario.
- [x] **2.10** GREEN: `internal/setup/writer.go` — `func (w *Writer)
      Changed(root string, from, to int) []report.FileChange`: for
      `touched[from:to]`, `!existed` → `created` with no read; `existed` →
      read back from disk and compare to the stored pre-write bytes —
      equal → `unchanged`, different → `modified`, and a failed read-back
      → `modified` (Decision 3: claiming `unchanged` from a failed read is
      the fabrication §09 forbids). Paths are expressed relative to
      `root`. Obs: 2.9 passes.
- [x] **2.11** Mutation guard:
      `TestChangedUnreadableExistingFileReportsModifiedNotUnchanged` — an
      existing file snapshotted, then removed before `Changed` reads it
      back; assert `Kind == modified`, never `unchanged`. Kills the mutant
      that treats a failed read as "no change" — design.md's Mutation
      coverage table, "`Changed`'s `existed` arm." Obs: killed mutant in
      `go run ./tools/mutationstaged`, not merely covered.
- [x] **2.12** RED: `internal/setup/setup_test.go` —
      `TestPerStepFileAttributionIsPartitioned`: two stub steps in one
      `applySteps` run — step A writes one file, step B (running later)
      writes two different files; assert step A's attributed files are
      exactly its one and step B's are exactly its two, with no overlap.
      Pins "two steps that both write files are attributed
      independently."
- [x] **2.13** GREEN: `internal/setup/setup.go` — `applySteps` records
      `before := len(writer.touched)` / `after := len(writer.touched)`
      around each `step.Apply` call and computes that step's touched-file
      set as `writer.Changed(p.Root, before, after)`, held on a per-step
      result value for slice 4 to consume. Obs: 2.12 passes.
- [x] **2.14** MUTATE + REFACTOR: `go run ./tools/mutationstaged` over
      `internal/setup`, `internal/runner`, `internal/app` (floor 0.80);
      `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l .`
      clean; confirm all six golden fixtures remain byte-identical (this
      slice's forecast is 0 golden bytes — design.md Decision 9's table).

---

## Slice 3 — fallow measurement, `Collisions`, `declaredAt`, `boundariesOwnerStep.ID()` (~180–260 lines, golden bytes: **6 lines, 6 files**)

Capability: `config-collision` (the measurement requirements: exit-3
short-circuit, local-binary-only with no remote fallback, `effective`
absent-never-fabricated, whole-value-or-not-at-all), and requirement (a) of
the amended collision-identity split (`ID()` asserts nothing about
architectures). Design Decision 5 (the measurement), Decision 6 (`ID()` and
the six golden lines). `Collisions`'s computation lands here per Decision 4's
explicit ordering note; its *rendering* through `renderCollisions` and the
branch collapse land in slice 4, so a key still renders through the old path
until then. Why here: the only slice that adds a subprocess and the only one
touching §03 — the whole subject of its own review (design.md Decision 9).

- [x] **3.1** RED: `internal/tool/tool_test.go` —
      `TestFallowConfigPathAndJSONArgs`: `FallowConfigPath() ==
      []string{"config", "--path"}`; `FallowConfigJSON() ==
      []string{"config", "--format", "json"}`. Pins the exact two
      argument lists Decision 5 measured (local ~350 ms, 26 top-level
      keys, `loaded config:` preamble on stderr only) — a mutant altering
      either flag string dies here.
- [x] **3.2** GREEN: `internal/tool/tool.go` — add `FallowConfigPath()
      []string` and `FallowConfigJSON() []string` beside `FallowAudit`/
      `FallowDupes`, each carrying Decision 5's measured-cost doc comment.
      Obs: 3.1 passes.
- [x] **3.3** RED: `internal/setup/steps_test.go` —
      `TestResolvedConfigShortCircuitsOnNoLocalBinary`: `runner.SetForTest`
      recording every `Command`; a `project.Project` whose
      `LocalBinary(tool.Fallow)` returns `""`; call `resolvedConfig(p)`;
      assert **zero** commands were recorded and the function returns
      `(nil, false)`. Pins the measurement layer's "probe short-circuits"
      row and "a project with no local fallow binary never reaches the
      network."
- [x] **3.4** RED: `TestResolvedConfigShortCircuitsOnExit3` —
      `runner.SetForTest` returns `*runner.ExitError{Code: 3}` for the
      `--path` command; assert exactly **one** command was recorded and
      `--format json` never ran; `resolvedConfig` returns `(nil, false)`.
      Pins "the exit-3 probe short-circuits the whole measurement, never
      just the resolve call" directly — design.md's named mutation risk: a
      mutant that runs both commands still produces an absent `effective`
      through the unmarshal failure, so the assertion must be on command
      count, not on `effective` alone.
- [x] **3.5** RED: table-driven `TestResolvedConfigAbsenceHasOneShape` —
      rows: non-zero non-3 exit from `--path`; non-JSON stdout from
      `--format json`; the colliding key missing from the resolved map.
      Each row asserts `resolvedConfig` returns `(_, false)`. Pins
      "`effective` is absent, never fabricated, whenever it cannot be
      measured"'s enumerated failure list.
- [x] **3.6** RED: `TestResolvedConfigSucceedsAfterExit0Probe` —
      `--path` exits 0; `--format json` returns a small valid JSON map;
      assert `--format json` is invoked exactly once and its map becomes
      the returned value. Pins "a project with a config present probes
      successfully and resolves."
- [x] **3.7** GREEN: `internal/setup/steps.go` — `func resolvedConfig(p
      project.Project) (map[string]json.RawMessage, bool)`, exactly
      Decision 5's order: `LocalBinary` check → `--path` probe (exit 3 or
      any other error → absent, no second command) → `--format json` (any
      error → absent) → `json.Unmarshal` (any error → absent); stderr
      discarded via `io.Discard`. Obs: 3.3–3.6 pass.
- [x] **3.8** RED: `internal/setup/files_test.go` —
      `TestDeclaredAtReturnsALineNumber`: reusing `declaredLine`'s
      fixtures, assert `declaredAt(path, key)` returns the correct 1-based
      line number when the key is present and a documented sentinel (0)
      when absent. Pins Decision 5: "locating a key by a textual scan is
      sound" — kept for what it is correct at.
- [x] **3.9** GREEN: `internal/setup/files.go` — `declaredLine(path, key)
      string` → `declaredAt(path, key) int` (same scan, returns the
      1-based line number instead of the line's text); remove
      `declaredValue`'s line-fragment display path per Decision 5
      ("`declaredValue` leaves the collision path entirely, and with it
      the `"duplicates": {` fragment"). Obs: 3.8 passes.
- [x] **3.10** RED: `internal/setup/steps_test.go` —
      `TestCollisionsComputesEachKeyOnce`: a fixture with two colliding
      keys (extending the existing `collidingKeys` fixtures); call
      `setup.Collisions(p)` with a stubbed `resolvedConfig` path exercised
      both absent and present; assert the returned `[]report.Collision`
      has exactly two entries, each `Collision.ID ==
      "sync:collision/"+Key`, `Ours` from the dharness-owned value, and
      `Effective`/`Theirs.Value` populated only when the resolve
      succeeded. Pins "a collision is computed once and rendered from
      that one value in both views" at the computation layer.
- [x] **3.11** GREEN: `internal/setup/steps.go` — `func Collisions(p
      project.Project) []report.Collision`: `colliding :=
      collidingKeys(...)`; `len(colliding) == 0` → `return nil` (no
      process at all); else call `resolvedConfig(p)` once and build one
      `report.Collision` per key. Obs: 3.10 passes.
- [x] **3.12** RED: `TestCollisionsSpawnsNoProcessWithNoCollidingKey` —
      `runner.SetForTest` recording commands; a project with zero
      colliding keys; call `Collisions(p)`; assert zero commands recorded.
      Pins Decision 5's "cheaper than the proposal's one cheap probe, and
      free (§13)" as its own dedicated assertion, distinct from 3.3's
      no-local-binary case.
- [x] **3.13** RED: `internal/setup/steps_test.go` —
      `TestBoundariesOwnerStepIDMakesNoArchitectureClaim`:
      `boundariesOwnerStep{}.ID()` does not contain `"two architectures"`
      or `"architectures this project declares"`. Fails against today's
      string; pins spec's amended requirement (a) by content, not exact
      wording.
- [x] **3.14** GREEN: `internal/setup/steps.go:467-469` — change `ID()` to
      `"resolve the keys this project and dharness both declare"`
      (Decision 6's replacement — true for one key or several, asserts
      nothing about architectures). Obs: 3.13 passes.
- [x] **3.15** Golden: regenerate the four framework fixtures via `go test
      ./internal/setup -run TestFrameworkGoldens -update`
      (`nextjs.txt`, `expo.txt`, `wails.txt`, `wails-nextjs.txt`); hand-edit
      the two generic fixtures (`generic-conventional.txt`,
      `generic-split.txt`) at line 26 only, matching the new `ID()`
      string — never via `-update`. Obs: `go test ./internal/setup/...`
      green; diff review confirms line 26 is the only line touched in all
      six files (six lines, six files — Decision 6's measured figure).
- [x] **3.16** RED (regression guard): re-run
      `TestGenericMechanismHasNoUpdatePath` explicitly after 3.15's hand
      edits, rather than assuming it — this is what proves editing a
      `.txt` fixture never touches `golden_test.go`'s own source. Obs: `go
      test ./internal/setup/... -run TestGenericMechanismHasNoUpdatePath`
      green; the test's own source is untouched by this slice.
- [x] **3.17** RED (regression guard):
      `TestBoundariesFallbackConstantsStayByteIdentical` —
      `boundariesFallbackDescribe` and `boundariesFallbackWhy`
      (`internal/setup/steps.go:506-516`) compared against a literal
      string captured in the test; fails if either constant's text
      changes by even one byte. Decision 6's "six lines is the complete
      golden impact" claim rests on these two constants staying
      byte-identical, since the generic fixtures reach only their
      fallback branches.
- [x] **3.18** GREEN: `docs/design-principles.md` — §03: record the
      second local-resolution exception ("`sync` resolves fallow locally
      **or not at all**"), beside the existing 12 August 2026 ESLint
      exception, citing Decision 5's measurement (local ~347/358/349 ms;
      no remote fallback on the `sync` path — the resolved question
      round's rejection of it, and `internal/tool/tool.go:101-103`'s
      network rule). Obs: the exception is named where the general rule
      is recorded, matching `eslint-integration`'s own precedent.
- [x] **3.19** GREEN: `docs/learning-log.md` — one dated line (12 August
      2026) for the `fallow config` cost and exit contract: local binary
      ~347/358/349 ms measured; `fallow config --path` exits 3 on a
      zero-config project; `fallow config --format json` never exits
      non-zero on a zero-config project (prints defaults), so it can
      never be used to detect absence. Obs: append-only, newest at the
      bottom, matching the file's existing convention.
- [x] **3.20** MUTATE + REFACTOR: `go run ./tools/mutationstaged` over
      `internal/setup`, `internal/tool` (floor 0.80) — verify the exit-3
      short-circuit branch (3.4) is killed, not merely covered, since
      design.md names it as the easiest of the four to leave unkilled;
      `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l .`
      clean.

---

## Slice 4 — `setup.Run`, `RunSync` rewrite, `--format json`, failure variant (~280–420 lines, golden bytes: 0)

Capability: `sync-report` (every remaining requirement), `step-delegation`'s
added requirement (`boundariesOwnerStep` hands back the same structured
`Collision`), `project-sync`'s added requirement (explicit retraction),
`config-collision` requirement (b)'s full rendering (the entry and handle
name the actual keys). Design Decision 8 (`setup.Run`/`RunSync` split),
Decision 4 (the branch collapse and `Why`/`Collisions` exclusivity — must
land together, per Decision 4's own ordering note, or a key renders twice in
the window between them). Why here: the only slice that changes the report's
shape; its test rewrite is inseparable from it (design.md Decision 9).

**MERGE CONDITION — run the binary and look at it.** Added after the fact,
because its absence cost this slice a full pass. `go test` was green and the
mutation score was 0.98–1.00 while the rendered report was missing eleven
things the approved report specifies, including the entire body of the
collision block — the richest block in the report and the reason this change
exists. Not one of this file's tasks asked anyone to look at the output.

A suite proves the suite agrees with itself. For a change whose deliverable
*is* observable output, that is not evidence. This slice is not done until
task 4.22 passes, and no future slice that changes observable output ships
without an equivalent task.

- [x] **4.22** ACCEPTANCE (blocking): build and run the real binary, then
      compare its output against `target-report.md` line by line.

      ```
      go build -o <scratch>/dharness.exe ./cmd/dharness
      ```

      Run `sync` and `sync --format json` in a throwaway git repo holding
      `frontend/package.json` and a `frontend/.fallowrc.json` that declares
      both `boundaries` and `duplicates`, so the collision path is actually
      exercised. Check, at minimum:

      - the header block, the `■` summary line, and the closing tally
      - per-step numbering (`4/11`)
      - each collision rendering its key, both values with their paths, the
        measured `effective` **or its stated absence**, and the lettered
        resolutions
      - subprocess output framed on **every** line, not just the first
      - `installed <package>@<version>`, with the version
      - a satisfied step carrying the evidence that satisfied it, wrapped
        rather than truncated mid-clause
      - the legend, and no glyph used for two meanings
      - `--format json` parsing, and agreeing with the human view because
        both came from one analysis

      Record what did not match. A mismatch is a defect in this slice, not a
      note for later.

      **Result, 13 August 2026.** Ran against a throwaway repository with
      `frontend/package.json` and `frontend/.fallowrc.json` declaring both
      `boundaries` and `duplicates`. All eleven items closed: the collision
      block now renders both sides' value, path and line, the measured
      `effective` marker (or its honest absence), and the lettered
      resolutions; subprocess output is framed line by line; `installed`
      carries `name@version`, measured from `package.json` post-install
      (disclosed deviation from design.md Decision 1's "no version",
      documented at `installedWithVersions`, `internal/setup/steps.go`);
      satisfied-step evidence is the real detection fact, not `Describe`'s
      fix instructions, wrapped within the fixed width; the "Already in
      place" glyph no longer collides with the subprocess gutter; every
      per-step row carries `n/total`; a header block and a legend line
      exist; the closing block's `next` pointer uses `Collision.ID` when the
      delegated step carries one. Two further defects were found live and
      fixed in the same pass, not on the original eleven-item list:
      `Collision.Theirs.Path` was an absolute filesystem path
      (`setup.Collisions`, `internal/setup/steps.go`) instead of
      root-relative like `Ours.Path`; `eslintExtendsStep`'s satisfied
      evidence still reused `Describe`'s truncated fix-instruction text for
      its own "spliced regions already match" case, the same defect shape
      the legacyLintConfigStep example named. The header block is a
      deliberately simplified single-column layout, not target-report.md's
      two-column grid, and omits the "read" line (no tracked concept of
      "files read this run" exists in the model); both disclosed in
      `writeHeaderBlock`'s own doc comment.

- [x] **4.1** RED: `internal/setup/setup_test.go` —
      `TestRunReturnsAStepResultForEveryPlanStep`: an 11-step (or stubbed)
      plan mixing satisfied/delegated/applied steps; call `setup.Run(p)`;
      assert `len(steps) == len(Plan())` and every `Status` is one of the
      six defined values, none empty or unrecognised. Pins "every step in
      `Plan()` carries exactly one status."
- [x] **4.2** RED: `TestRunReadsNotesBeforeAnyByteChanges` — a stub step
      whose `Satisfied`/`Delegated` would observe a side effect if called
      after a write; assert the three notes
      (`UncheckableConfigNote`/`UncertainPresetNote`/`EslintResidueNote`)
      are read before the first `Apply` runs. Pins Decision 8: "notes are
      read first inside `Run`... becomes structural."
- [x] **4.3** RED: `TestRunOrdersSatisfiedBeforeDelegated` — a stub step
      whose `Satisfied` returns `true`; assert `Delegated` is never
      called for it. Preserves today's semantics and keeps
      `boundariesOwnerStep`'s fallback constants unreachable in the
      product, which 3.17 depends on remaining true (Decision 8, change
      #1).
- [x] **4.4** RED: `TestSatisfiedStepCarriesEvidenceNotBareStatus` — a
      step already satisfied (e.g. `fallowExtendsStep`); assert its
      `StepResult.Evidence` is non-empty and names the detection fact
      (e.g. `"extends → .dharness/fallow.jsonc"`), not merely
      `Status == "satisfied"` with empty `Evidence`. Pins the spec
      scenario of the same name.
- [x] **4.5** GREEN: `internal/setup/setup.go` — `func Run(p
      project.Project) (steps []report.StepResult, notes []report.Note,
      err error)`; `internal/setup/notes.go` (new) reshapes the three
      existing notes into `[]report.Note`; for each `Plan()` step,
      `Satisfied` first, else `Delegated`, else the applying path
      building `Wrote`/`Installed`/`Transcript`/`MS` from slice 2's
      `Facts` + `Writer.Changed` wiring. `Apply(p, out) error` keeps its
      exact signature for `renderGolden`'s sole remaining call. Obs:
      4.1–4.4 pass.
- [x] **4.6** RED: `internal/setup/setup_test.go` —
      `TestFailureRetractsEarlierStepsAndMarksRemainingNotReached`: step 2
      fails in an 11-step stub plan; assert step 1's `Status ==
      Retracted` (never `Applied`), `Rollback.Retracted` names it, the
      nine steps after step 2 are each `NotReached`, and `len(steps) ==
      11`. Pins the failure-variant requirement's both scenarios.
- [x] **4.7** GREEN: `internal/setup/setup.go` — on `Apply` error: run
      `Writer.Undo()`; mark the failed step `Failed`, every
      earlier-applied step `Retracted`, every remaining step
      `NotReached`; build `Rollback{Retracted: [...]}` naming them;
      delete the false "No earlier step is reported as having succeeded"
      sentence (`setup.go:110-112`) — the returned error keeps only the
      step name and cause. Obs: 4.6 passes.
- [x] **4.8** RED: `internal/setup/steps_test.go` —
      `TestDelegatedCollisionMatchesTheComputedReportValue` — for a
      project with one colliding key, assert
      `boundariesOwnerStep.Delegated(p)`'s `why` string equals
      `renderCollisions(Collisions(p))` byte for byte. Pins
      `step-delegation`'s added requirement: "the report's collision
      rendering and the delegated reason cannot drift apart."
- [x] **4.9** GREEN: `internal/setup/steps.go` — `func renderCollisions(cs
      []report.Collision) string`; `describeBoundaries` and
      `delegateBoundaries`'s non-empty branches become
      `renderCollisions(Collisions(p))`, keeping the empty-set fallback
      constants byte-for-byte (Decision 6, guarded by 3.17). Obs: 4.8
      passes; 3.17's byte-identity test still passes unmodified.
- [x] **4.10** RED: `internal/report/human_test.go` —
      `TestWhyAndCollisionsAreMutuallyExclusiveInRendering` — a
      `StepResult` carrying both a non-empty `Why` and a non-empty
      `Collisions` slice (a constructed adversarial value); assert the
      output contains the collision block and does not contain the `Why`
      string. Pins Decision 4's "seam that makes a second renderer
      impossible, rather than absent."
- [x] **4.11** Mutation guard: 4.10's test, verified as a killed mutant in
      `go run ./tools/mutationstaged` — design.md names this branch as
      easy to leave unkilled (Mutation coverage table, 3rd item).

- [x] **4.11a** RED: `internal/report/human_test.go` —
      `TestFailureVariantRendersEveryNonTerminalStatus`: a `Report` whose
      steps carry `Failed`, `NotReached` and `Retracted`; assert each is
      rendered with its own line, that the retracted step is named in the
      closing block as included in the rollback, and that the tally sums
      to the plan's length. Pins the six-value status enumeration
      (`spec.md`, amended) and `project-sync`'s added retraction
      requirement.
- [x] **4.11b** GREEN: `internal/report/human.go` — render the
      `Failed`/`NotReached`/`Retracted` per-step lines and the failure
      variant's closing block.

      **Added by the orchestrator after slice 1, from its apply report.**
      Slice 1 implemented `WriteHuman`'s five blocks and closing tally for
      `Applied`/`Delegated`/`Satisfied`/`Notes` only, exactly as its own
      task list specified — the failure variant was never assigned to a
      slice. Without this task slice 4 would have to make an undocumented
      `human.go` edit, or ship a report that cannot render the one case
      `project-sync`'s retraction requirement exists for. Counted inside
      slice 4's existing 280–420 forecast; the rendering is three status
      arms and one closing block over a model that already carries every
      field (`report.Rollback`, `Summary.Retracted`).
- [x] **4.11c** Mutation guard: 4.11a's test over `internal/report`,
      verified killed. Status-arm branches are the same shape as the
      decorative-literal survivors slice 1 had to chase down; prefer
      simplifying the arm to pinning its text.
- [x] **4.12** RED + rewrite rule: `internal/cli/sync_test.go` — coordinated
      rewrite of all 16 existing test functions
      (`TestSyncSpeaksTheProjectsOwnPackageManager`,
      `TestSyncSaysWhyTheDelegatedStepIsDelegated`,
      `TestSyncReachesATerminalAnswer`,
      `TestSyncAppliesAndDelegatesInOneRun`,
      `TestSyncCompletesWhenTheProjectAlreadyConfiguredFallow`,
      `TestSyncNeverAppliesADelegatedStep`,
      `TestSyncStopsBeforeWritingWithoutAJSProject`,
      `TestSyncRollbackNamesWhatItRestoredAndNothingMore`,
      `TestSyncStopsOutsideAGitRepository`,
      `TestSyncNamesTheJSDirectoryOnlyWhenItIsNotTheRoot`,
      `TestSyncReportsTheConfigItCouldNotCheck`,
      `TestSyncSaysNothingAboutAConfigItCanCheck`,
      `TestSyncSaysWhatAMatchedPresetHadToAssume`,
      `TestSyncAssumesNothingWhenTheFrameworkConfigReads`,
      `TestSyncReportsEslintResidueInAnAlreadyAdoptedRepository`,
      `TestSyncSaysNothingAboutEslintResidueWhenThereIsNone`). Eight lines
      across these tests assert on six literal headings (`"Applying:"`,
      `"## Left to you"`, `"## Not checked"`, `"## Assumed"`,
      `"## Residue"`, `"Nothing to do"`), all of which change shape.
      **Binding rule**: the rewrite MAY change assertions on the report's
      shape (headings, block structure) and MUST NOT relax assertions on
      what `sync` actually does — which files it writes, which step it
      applies, what it names as delegated, what it retracts, which
      residue entries it lists. Obs: every non-heading assertion in each
      of the 16 tests is at least as strict after the rewrite as before it.
- [x] **4.13** GREEN: `internal/cli/sync.go` — rewrite `RunSync`: discover,
      `setup.Run(p)`, assemble `report.Report{Version, Root, Source,
      Summary, Evidence, Notes, Steps, Rollback, Exit:
      runner.ExitCode(err)}`, dispatch to `report.WriteHuman` or
      `report.WriteJSON` per a new `--format` flag (default `human`; any
      other value errors naming both). Obs: 4.12's rewritten suite
      passes; `go build ./...`.
- [x] **4.14** RED: `TestSyncFormatJSONEmitsParseableJSONAndNothingElse` —
      run `RunSync(["--format", "json"], &buf)` against a constructed
      project; assert `json.Unmarshal` succeeds and `json.Valid` covers
      the entire buffer. Pins "`--format json` emits parseable JSON on
      stdout."
- [x] **4.15** RED: `TestSyncFormatJSONAndHumanAgreeOnSummaryCounts` — run
      `RunSync` twice against identical constructed project state, once
      default and once `--format json`; assert
      `summary.steps`/`applied`/`delegated`/`satisfied`/`failed` decoded
      from JSON equal the counts asserted from the human output. Pins
      "the JSON summary's counts match the human summary's counts."
- [x] **4.16** RED: `TestSyncExitFieldMatchesRunnerExitCode` — one run with
      a step failure and one clean run, including a delegated-work-only
      case (`Summary.Delegated > 0`, `Summary.Failed == 0`); assert the
      JSON's `exit` field equals `runner.ExitCode` of the exact error
      `RunSync` returns in each case, and that no rule keyed on
      `summary.delegated > 0` overrides it. Pins both exit-code scenarios
      together, since design.md's Property 2 exists to keep them
      untestable apart.
- [x] **4.17** RED: `TestClosingBlockNamesTheDelegatedStepAsNext` — a run
      leaving one step delegated; assert the closing block names that
      step's identifier as next, distinct from the earlier "Left to you"
      detail. Pins "a run with delegated work names a next step."
- [x] **4.18** RED: `TestScopedMutationEvidenceSurvivesBothViewsRegardless`
      — a constructed `project.Project` whose `ReadEvidence().ScopedMutation`
      is non-nil and every step satisfied (so `left != 0` is not the
      case that used to gate it); assert the measured related-test count
      and path appear in both the human output and the JSON's evidence
      object. Pins "measured evidence keeps its place in the model,"
      no longer gated on `left == 0` (Decision 8, change #2).
- [x] **4.19** RED: `TestNoReportFileIsPersisted` — snapshot the file tree
      before and after a `--format json` run against a real
      `t.TempDir()` project fixture; assert the only files differing are
      ones the applied steps themselves own. Pins "no report is
      persisted to a file" at the integration layer.
- [x] **4.20** GREEN: `internal/cli/sync.go` / `internal/cli/flags.go` —
      wire the `--format` flag; wire `report.Evidence` from
      `p.ReadEvidence().ScopedMutation` whenever non-nil; wire the closing
      block's `next` pointer from the first delegated `StepResult` found.
      Obs: 4.14–4.19 pass.
- [x] **4.21** REFACTOR + MUTATE: `go run ./tools/mutationstaged` over
      `internal/cli`, `internal/setup`, `internal/report` (floor 0.80);
      `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l .`
      clean; confirm all six golden fixtures remain byte-identical to
      slice 3's state — this slice changes observable prose, not
      `Plan()`/`Step.ID()`, so goldens do not move a second time.

---

## Before every commit

`go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l .`, `go run
./tools/mutationstaged` (floor 0.80).
