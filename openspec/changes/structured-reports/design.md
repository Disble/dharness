# Design: structured-reports

Inputs: `openspec/changes/structured-reports/proposal.md` (authoritative on
scope, and its `## Question round — resolved` section, which overrides the
proposal body), `spec.md` (three new capabilities plus two modified ones,
every requirement), `target-report.md` (the approved shape — a design
reference, not a byte contract), `explore.md` (current-state citations),
`docs/design-principles.md`, `CLAUDE.md`'s two rules,
`openspec/changes/archive/2026-08-12-eslint-integration/design.md` (house
style, and the merge-condition pattern this design copies for the one
measurement it could not run).

Read alongside: `internal/cli/check.go:71-135`, which already ships the two
seams this change reuses — a locally-resolved binary through
`p.LocalBinary` + `tool.Installed`, and a section rule drawn with `──`.

**Three findings correct the proposal and the spec.** Each is argued from
code in this repository, and each changes work that was already planned:

1. **Changing `boundariesOwnerStep.ID()` moves golden bytes in all six
   fixtures.** `renderGolden` prints `step.ID()` unconditionally
   (`internal/setup/golden_test.go:145`), and every fixture carries the old
   string at line 26 — verified by search across
   `internal/setup/testdata/golden/`. The proposal's "Zero golden bytes move"
   and the spec's matching non-requirement are **false** once the spec's own
   MUST on `ID()` is honoured. The change is held and the six lines are paid;
   Decision 6 gives the evidence and names the spec amendment.
2. **`internal/cli` cannot call `app.ExitCode`.** `internal/app` imports
   `internal/cli` (`app.go:14`, `app.go:39`), so the dependency the §11/§17
   pin requires is a cycle. `ExitCode` has to move. Decision 1.
3. **The five statuses cannot describe a rolled-back step**, and the approved
   report's own failure tally proves it: `1 failed · 0 applied · 9 not
   reached` accounts for ten of eleven steps. A sixth value is needed and the
   spec sentence already names it. Decision 1, with the amendment stated.

## Technical Approach

`internal/report` holds one value and two renderings. It is a data package
with a human renderer and a JSON encoder, and it knows nothing about
adoption: no `project.Project`, no `setup.Step`, no git. `internal/setup`
imports it to populate results; `internal/cli` imports it to render. Nothing
imports `internal/cli`, so the direction never inverts.

Four properties fall out and are load-bearing everywhere below:

1. **One computation, two renderings.** `setup.Run` returns results and
   notes; `RunSync` assembles a `report.Report` and hands the *same value* to
   one of two writers. Neither writer reads the repository. That is what
   makes defects 6 and 7 unreachable rather than merely fixed.
2. **The verdict is `runner.ExitCode`, and nothing else.** `Report.Exit` is
   assigned from the exact error `RunSync` is about to return. There is no
   field derived from counts (§11, §17, `CLAUDE.md`'s second rule).
3. **Absent is a value.** `effective`, `evidence` and a collision's declared
   value are pointers with `omitempty`, so "not measured" is a missing key
   and never a zero one. A measurement that fails produces absence, never a
   guess (§09).
4. **Nothing is recorded.** The report is built, rendered to stdout, and
   dropped. No file, under any flag (§07, §08).

## Architecture Decisions

### Decision 1 — the model is one package of plain structs, `Status` is a string set, and `ExitCode` moves to `internal/runner`

**Choice.** `internal/report/report.go`:

```go
// Package report holds what one dharness run learned, as a value that is
// built once and rendered twice: a human view on stdout, and the same
// analysis as JSON under --format json. It reads nothing and writes no
// file — every field is filled in by its caller.
package report

// Status is what this run did about one step. It is a string rather than an
// int enum because encoding/json marshals a named int as a number: an int
// Status would need a MarshalJSON method, which is a second representation
// of the same fact and exactly the duplicate-renderer defect this change
// exists to end. A string set marshals to the spec's own words with no
// method at all, and its zero value ("") is not a member — so an unfilled
// status is detectably invalid rather than silently "applied".
type Status string

const (
	Applied    Status = "applied"
	Delegated  Status = "delegated"
	Satisfied  Status = "satisfied"
	Failed     Status = "failed"
	NotReached Status = "not-reached"
	Retracted  Status = "retracted"
)

type Kind string

const (
	Created   Kind = "created"
	Modified  Kind = "modified"
	Unchanged Kind = "unchanged"
)

type Report struct {
	Version  string       `json:"version"`
	Root     string       `json:"root"`
	Source   string       `json:"source,omitempty"`
	Summary  Summary      `json:"summary"`
	Steps    []StepResult `json:"steps"`
	Notes    []Note       `json:"notes,omitempty"`
	Evidence *Evidence    `json:"evidence,omitempty"`
	Rollback *Rollback    `json:"rollback,omitempty"`

	// Exit is runner.ExitCode applied to the error RunSync is about to
	// return. It is assigned, never computed here: a report that decided
	// its own verdict would be the gate reporting its own status instead
	// of the tool's (§11, §17).
	Exit int `json:"exit"`
}

// Summary carries no omitempty. A zero count is a measured zero, and a
// missing key would read as "never counted" — the same absent-versus-empty
// distinction Effective depends on, applied in the other direction.
type Summary struct {
	Steps     int   `json:"steps"`
	Applied   int   `json:"applied"`
	Delegated int   `json:"delegated"`
	Satisfied int   `json:"satisfied"`
	Failed    int   `json:"failed"`
	Retracted int   `json:"retracted"`
	MS        int64 `json:"ms"`
}

type StepResult struct {
	N int `json:"n"`

	// ID is Step.ID() verbatim — the heading, which is what that method
	// has always been ("ID names the step for a report",
	// internal/setup/setup.go:24) and what the approved report's human
	// view prints for every applied and satisfied step. It is a sentence,
	// deliberately, and Decision 6 keeps it one.
	//
	// The addressable handle lives on Collision.ID, which is the only
	// thing in this report anything points at: the closing block's `next`.
	// Giving all eleven steps a slug is a follow-up the JSON twin can take
	// whenever check and mutate need one, and nothing here depends on it.
	ID string `json:"id"`

	Status     Status       `json:"status"`
	MS         int64        `json:"ms"`
	Evidence   string       `json:"evidence,omitempty"`
	Why        string       `json:"why,omitempty"`
	Wrote      []FileChange `json:"wrote,omitempty"`
	Installed  []string     `json:"installed,omitempty"`
	Collisions []Collision  `json:"collisions,omitempty"`
	Error      string       `json:"error,omitempty"`

	// Transcript is the bytes this step's Apply produced. It is rendered
	// under the step in the human view and excluded from JSON: the machine
	// reader gets Installed, a fact, and shipping raw subprocess bytes into
	// a JSON field invites a consumer to parse them, which is the
	// re-parsing §01 and §09 forbid.
	Transcript string `json:"-"`
}

type FileChange struct {
	Path string `json:"path"`
	Kind Kind   `json:"kind"`
}

type Collision struct {
	// ID is the stable, addressable handle the closing block points at,
	// in fallow's own `dup:c064407b` shape. It is derived from Key, so a
	// key can never carry two ids.
	ID          string   `json:"id"`
	Key         string   `json:"key"`
	Ours        Declared `json:"ours"`
	Theirs      Declared `json:"theirs"`
	Effective   *string  `json:"effective,omitempty"`
	Resolutions []string `json:"resolutions"`
}

// Declared is one side of a collision. Value is a pointer to raw JSON so
// that "fallow could not be asked" is an absent key rather than an empty
// object, and so the value round-trips whole instead of being re-encoded
// through a Go type dharness does not own.
type Declared struct {
	Path  string           `json:"path"`
	Line  int              `json:"line,omitempty"`
	Value *json.RawMessage `json:"value,omitempty"`
}

type Note struct {
	Kind       string   `json:"kind"`
	Path       string   `json:"path,omitempty"`
	Entries    []string `json:"entries,omitempty"`
	Actionable bool     `json:"actionable"`
	Reason     string   `json:"reason"`
}

type Evidence struct {
	RelatedTests int    `json:"relatedTests"`
	MeasuredPath string `json:"measuredPath"`
}

// Rollback is set only when a step failed and Writer.Undo ran. Retracted
// carries no omitempty: an empty list on a rolled-back run is a claim, and
// the whole point of this block is that a claim already printed gets
// withdrawn by name (defect 3).
type Rollback struct {
	Retracted []string `json:"retracted"`
	Restored  []string `json:"restored,omitempty"`
	Removed   []string `json:"removed,omitempty"`
	Left      []string `json:"left,omitempty"`
}
```

**`ExitCode` moves to `internal/runner`, and this is not optional.**
`app.ExitCode` (`internal/app/app.go:64`) is the function §11/§17 pin the
JSON status to, and `internal/app` imports `internal/cli` (`app.go:14`,
`:39`), so `internal/cli` importing `internal/app` is an import cycle. The
function's whole body is `errors.As` over `*runner.ExitError` plus two
constants — it depends on `internal/runner` and nothing else, and
`ExitError`'s own comment already states the concern ("the code is carried
so dharness can exit with the same one"). So `runner.ExitCode` is where it
belongs, and `app.ExitCode` becomes a one-line forwarder so no existing
caller or test moves.

**Rejected: `internal/report` computing its own status from `Summary`.** It
compiles, needs no move, and is precisely the heuristic verdict §11 was
written about — Stryker exiting 0 while reporting survivors. A field derived
from `failed == 0` would also disagree with the process the first time a
delegated-only run is asked whether it passed.

**Rejected: an int `Status` with `iota`.** It marshals as a number, so every
JSON consumer would read `3` and need dharness's table to decode it; adding
`MarshalJSON`/`UnmarshalJSON` to fix that creates two encodings of one fact
maintained in two places. The string set has no method to drift.

**Rejected: `wrote: []string`, as `target-report.md`'s JSON sample shows.**
The spec MUSTs a created/modified/unchanged classification for every touched
file, and a flat string array cannot carry it. `target-report.md` states it
is a design reference rather than a byte contract; this is the adjustment
that clause exists for, and it is named here rather than discovered in
review.

**The amendment this design proposes to the spec.** The requirement "every
step in `Plan()` carries exactly one status" enumerates five values, and the
failure-variant requirement asks for a step's status to be marked
"retracted/failed-with-the-run". Those cannot both hold with five values:

- `applied` is forbidden by the failure requirement itself.
- `failed` would make `Summary.Failed` report 2 on a run with one failure, so
  the closing block's "1 failed" becomes false — the overclaim defect 3 is
  about, reintroduced by the fix for it.
- `not-reached` and `satisfied` are simply untrue of a step that ran.

The approved report's own tally is the third witness: `1 failed · 0 applied ·
9 not reached` accounts for ten of eleven steps, and the eleventh is named
only in the retraction sentence. **`retracted` is added as a sixth value,
with `Summary.Retracted` beside it**, and the five-value requirement should
be amended to six. The five-value scenario is unaffected — its GIVEN is a
run with no failure.

### Decision 2 — `Step.Apply` widens to `(p, w, out io.Writer) (Facts, error)`, and the sink does not go on `Writer`

**Choice.**

```go
// Facts are what a step knows and a transcript cannot say. A sink of raw
// bytes cannot answer "what package"; only the step that built the install
// command can, and it already has the list.
type Facts struct {
	// Installed names the package specs this step asked the manager to
	// add — exactly the slice it passed to tool.InstallPackages, never
	// anything read back out of the subprocess's output (§01, §09).
	Installed []string
}

// Apply performs the step, recording what it touched so it can be undone.
// Everything it produces goes to out, never to the process's own stdout, so
// applySteps can frame it under this step. A non-nil error means Facts is
// not read.
Apply(p project.Project, w *Writer, out io.Writer) (Facts, error)
```

`applySteps` gives each step its own `bytes.Buffer` and keeps the result as
`StepResult.Transcript`. The three offending sites become
`runner.Run(cmd, out, out)` — one stream for both, which is
`internal/cli/check.go:124`'s existing shape and preserves the interleaving
a person reads.

**Why widening beats a sink field on `*Writer`, despite the larger diff.**
The exploration framed this as "smallest diff versus honest naming", and
that framing dissolves once the second half of the requirement is included:
`installed: [...]` is a fact no byte stream can produce, so **`Apply`'s
signature has to change regardless**. Once it does, putting the sink on
`Writer` buys nothing and costs `Writer` its single meaning — "file writer
with rollback" — for a field that has no relationship to rollback. The
"smallest diff" argument evaporates with the thing that motivated it.

The ripple is real and it is mechanical: ten concrete steps, one test stub,
and the call sites in `internal/setup`'s tests. Nine of the ten steps change
`return err` to `return Facts{}, err` and nothing else. The
`(value, explanation)` return shape is also this package's own grain —
`Delegated` already returns `(why string, ok bool)` rather than a bare bool.

**Rejected: `Apply(p, w, o *Outcome) error`, one in-out parameter carrying
both directions.** Fewer edits per site, and a mutable output parameter that
is written by the callee and read by the caller is the shape Go's multiple
returns exist to avoid; it also makes "is `Installed` meaningful after an
error" a question the type cannot answer, where `(Facts, error)` answers it
by convention.

**Rejected: reporting the installed version, as `target-report.md`'s
`dharness-eslint-plugin@0.3.0` shows.** dharness installs unpinned
(`RulesPackage` is a bare name, and `preset.Layer.Package` is documented as
unpinned on purpose), so the resolved version is knowable only by parsing
`bun add`'s output (§01, §09) or by reading a lockfile in four formats.
Reading `package.json` after the install returns a caret *range*, not the
installed version — a number that looks measured and is not, which is worse
than the honest form. **`Installed` names what dharness asked for, without a
version**, and the report says so.

### Decision 3 — attribution is a `Writer` method, because `snapshot` is unexported and the post-write bytes are on disk

**Choice.** `applySteps` marks the touched slice around each `Apply`, and
`Writer` classifies:

```go
// Changed classifies the files this run touched between two marks, as
// paths relative to root so the report names a directory rather than a
// bare file name (defect 9). It reads each path back from disk because
// Writer stores pre-write bytes only, never post-write.
//
// A file that did not exist is created without a read: its prior absence
// is recorded fact, and Apply would have returned an error if the write
// had failed. A file that existed and cannot be read back now is reported
// modified — of the three kinds, unchanged is the only one that claims
// nothing happened, and claiming it from a failed read is the fabrication
// §09 forbids.
func (w *Writer) Changed(root string, from, to int) []report.FileChange
```

`touched` only ever grows by append within one run, so `touched[from:to]` is
exactly that step's set. `remember` already dedupes by path
(`writer.go:58-62`), so a file two steps both touch is attributed to the
first — stated here because it is reachable: `ownedFilesStep` writes
`.dharness/.gitignore` through `ensureShared`, and `EnsureDir` writes
outside the `Writer` entirely.

**Rejected: exporting `snapshot` so `internal/report` can classify.** It
would export `path`, `existed`, `data` and `mode` — the rollback machinery's
entire private state — so that a package with no filesystem policy could do
one `os.ReadFile`. `internal/report` reads nothing, and that is the property
that keeps it importable by both other packages.

**Rejected: recording post-write bytes in `remember`.** It doubles the
memory a run holds for a fact needed once, at the end, about files that are
still on disk.

### Decision 4 — the collision is one exported computation, and the second renderer loses its input rather than its call site

**Choice.** One function computes, one function renders, and the report
never reads the prose:

```go
// internal/setup

// Collisions computes each colliding key once: the value both the report's
// collision block and boundariesOwnerStep.Delegated are rendered from.
// Exported as a package function rather than reached through the Step
// interface, which is what UncheckableConfigNote, UncertainPresetNote and
// EslintResidueNote already are at this same call site
// (internal/cli/sync.go:58, :63, :70) — so no requirement about Delegated's
// (why string, ok bool) contract is reopened and nothing type-asserts on a
// step (openspec/specs/setup/spec.md:179).
func Collisions(p project.Project) []report.Collision

// renderCollisions is the only place a Collision becomes prose. Delegated
// calls it; the report does not.
func renderCollisions(cs []report.Collision) string
```

`describeBoundaries` and `delegateBoundaries` keep their empty-set fallback
constants byte-for-byte and lose their independent walks: both non-empty
branches become `renderCollisions(Collisions(p))`.

**The seam that makes a second renderer impossible, rather than absent.**
`StepResult` carries `Why` and `Collisions`, and the human renderer reads
`Why` only when `Collisions` is empty — one branch, in one function, over
one value. A future change cannot render a key twice without first giving
the same `StepResult` both a `Why` and a `Collisions` entry, which is a
visible contradiction in one struct rather than two functions in different
parts of a file that happen to agree today. The spec's own assertion is a
count over the rendered output, and it holds structurally.

**Ordering across slices matters and is stated.** `Collisions` lands in
slice 3 while `RunSync` still prints `why`; the branch collapse and the
`Why`/`Collisions` exclusivity land together in slice 4. Landing them apart
would leave a window where a key renders twice — the defect, shipped by the
fix for it.

**Rejected: widening `Delegated` to return structured data.** It would
change the contract for all eleven steps to serve one, which the modified
`step-delegation` requirement explicitly declines, and it would force a type
assertion or a second interface at the call site.

### Decision 5 — the fallow measurement is two commands in `internal/tool` and one function with exactly two outcomes

**Choice.** The syntax, beside `FallowAudit` and `FallowDupes`:

```go
// FallowConfigPath prints the resolved config file path, and exits 3 when
// the project has no fallow config at all. It is the probe: --format json
// exits 0 even with no config file (it prints defaults), so it can never be
// used to detect absence. Measured against the reference project.
func FallowConfigPath() []string { return []string{"config", "--path"} }

// FallowConfigJSON prints the fully resolved config — `extends` already
// applied — as JSON on stdout. Measured against the reference project: 26
// top-level keys, and the `loaded config: <path>` preamble goes to stderr,
// so stdout needs no stripping. Roughly 350 ms on a local binary.
func FallowConfigJSON() []string { return []string{"config", "--format", "json"} }
```

The measurement, in `internal/setup`, because it feeds `Collisions`:

```go
// resolvedConfig asks fallow which value it actually runs for each key, or
// reports that it could not be asked. Two outcomes and no third: a resolved
// map, or nothing. Nothing is never an error this run reports — a missing
// measurement is not a failed sync (§20).
func resolvedConfig(p project.Project) (map[string]json.RawMessage, bool)
```

The order, and every exit from it:

```
Collisions(p)
  └─ colliding := collidingKeys(...)      len == 0 → return nil, no process at all
       │
       └─ resolvedConfig(p)
            ├─ p.LocalBinary(tool.Fallow) == ""  → nil,false   (no subprocess is built)
            ├─ run FallowConfigPath()
            │     ├─ *runner.ExitError code 3    → nil,false   (JSON call never runs)
            │     └─ any other error             → nil,false
            ├─ run FallowConfigJSON() → &stdout, stderr discarded
            │     └─ any error                   → nil,false
            └─ json.Unmarshal(stdout)
                  └─ any error                   → nil,false
```

Every failure collapses to the same absence, and every absence produces
`Effective == nil`, `Theirs.Value == nil`, and a collision block that says
the value could not be shown. The **probe runs only after a collision is
already proved**, so a project with nothing colliding spawns no process at
all — cheaper than the proposal's "one cheap probe", and free (§13).

`effective` is computed by comparing fallow's resolved value for the key
against the value dharness owns: equal → `"ours"`, different → `"theirs"`.
That is a byte comparison over one measured value and one dharness already
knows, not an inference from `extends` direction. When the resolve is
absent, so is `effective`.

`declaredLine` (`internal/setup/files.go:130-143`) is **kept for what it is
correct at and removed from what it lied about**: it becomes
`declaredAt(path, key) int`, returning the 1-based line number, feeding
`Declared.Line`. Locating a key by a textual scan is sound; *showing a
value* with one was defect 8. `declaredValue` (`steps.go:671-676`) leaves
the collision path entirely, and with it the `"duplicates": {` fragment.

**No remote fallback, and this is closed.** The resolved question round
rejected it, and `internal/tool/tool.go:101-103` records the rule: flags are
forced off "because both reach the network, which a gate that runs on every
commit must not do". `check.go`'s own fallow stage keeps `remoteStage`
untouched.

**No new test seam.** `runner.Run` is already a package variable
(`runner.SetForTest`), and it takes the stdout writer as a parameter, so
capturing fallow's JSON is `runner.Run(cmd, &buf, io.Discard)` with no
change to `internal/runner` at all.

### Decision 6 — `ID()` is a heading, not a handle; its string changes, and six golden lines move with it

**`ID()` can never name the colliding key.** It takes no `project.Project`
(`internal/setup/setup.go:25`), so the key is not reachable from it. The
spec's requirement therefore splits in two, and both halves are needed:

1. **`ID()` stops asserting what it cannot know.** It becomes
   `"resolve the keys this project and dharness both declare"` — true for one
   key or several, and it asserts nothing about architectures.
2. **The report's addressable handle for a collision is
   `sync:collision/<key>`**, carried on `Collision.ID`, which is where the
   target report's `id sync:collision/duplicates` and its closing `next`
   pointer come from.

**And this moves golden bytes.** `renderGolden` prints `step.ID()` for every
step unconditionally (`internal/setup/golden_test.go:145`), and every fixture
in `internal/setup/testdata/golden/` carries
`4  resolve the two architectures this project declares` at **line 26** —
`generic-conventional.txt`, `generic-split.txt`, `nextjs.txt`, `expo.txt`,
`wails.txt`, `wails-nextjs.txt`. **The proposal's "Zero golden bytes move"
and the spec's matching non-requirement are false**, and slice 3 carries a
fixture edit that neither planned for.

The cost is exactly measured and it is small: **six lines, one per fixture.**
The four framework fixtures regenerate with
`go test ./internal/setup -run TestFrameworkGoldens -update`; the two generic
fixtures are hand-edited, as `eslint-integration` Decision 10 already did
four times. `TestGenericMechanismHasNoUpdatePath`
(`internal/setup/golden_test.go:101`) stays green: it greps
`golden_test.go`'s own source for `flag.Bool("update"` inside
`TestGenericGoldenIsUnchanged`'s body, and editing a `.txt` fixture does not
touch it.

**Nothing else in this change moves a golden byte, and the reason is worth
recording.** The fixtures render a *generic* project, where `collidingKeys`
returns empty, so `describeBoundaries` and `delegateBoundaries` reach only
their fallback constants (`steps.go:506-516` says exactly this). The whole
collision restructure — Decision 4 — happens in branches the fixtures never
execute. So the six lines above are the complete golden impact of this
change, and the two fallback constants must stay byte-identical for that to
remain true.

#### Rejected: keep `ID()` as an opaque machine handle and move the heading to `StepResult`

This was the cheapest candidate on the table — zero golden bytes, no slice
change — and the separation it proposes is correct and is already in this
design as `Collision.ID`. What it gets wrong is **which of the two `ID()`
is**, and the code answers that three ways:

1. **The interface says so.** `Step`'s own comment is *"ID names the step
   for a report"* (`internal/setup/setup.go:24`). Every production use
   renders it as prose: `applySteps` prints `"  %s\n"`
   (`setup.go:97`), `RunSync` prints `"## Left to you: %s"`
   (`internal/cli/sync.go:97`), and both rollback errors embed it in a
   sentence (`setup.go:101`, `:111`). Nothing consumes it as a key.
2. **These are not stable identifiers.** `eslintExtendsStep.ID()` is
   `fmt.Sprintf("point %s at the file dharness owns", eslintConfig)`
   (`steps.go:305`) — a sentence composed at call time. A handle that is
   `Sprintf`-ed from a filename is not a handle.
3. **The approved report already separates the two, the other way round.**
   Its JSON twin's ids are `install`, `fallow-extends` and
   `sync:collision/duplicates` — short slugs, none of which any `ID()`
   returns. Its human headings are `install what this project is missing`,
   `install react-doctor's agent skill`, `decide this project's architecture`
   — those *are* the `ID()` strings, verbatim. So `ID()` is the heading
   side of the split the report already makes.

Adopting the inverted mapping costs more than it saves. `StepResult` would
carry both a handle and a heading that are the identical sentence for ten of
eleven steps and diverge for one, so `--format json` would emit sentences
under `id` for ten entries and a slug for the eleventh — a worse machine
contract than either alternative, in the field a machine reader keys on.

**And the sentence would be printed more after this change, not less.**
`boundariesOwnerStep` is *satisfied* in every project with no collision,
which is the common case, and this change's whole purpose is that satisfied
steps stop being an ambiguous absence (defect 1). Today `Pending` filters
them out and the string never reaches stdout. Afterwards the "Already in
place" block renders it on nearly every run:

    = 4/11   resolve the two architectures this project declares

for a project that declares one architecture, or none. That is not an opaque
handle nobody reads as documentation — it is a false sentence in the report's
most-travelled block, shipped to both readers §16 names, in exactly the case
where it is most often wrong. Six fixture lines are the cheaper of the two
prices.

**The spec amendment this decision needs, stated rather than assumed.** The
requirement is currently titled *"the collision step names the actual
colliding key"* and its body puts the MUST on `ID()`. `ID()` cannot see the
project, so as written it is unsatisfiable. It should be **split in two**,
matching what the code can actually do:

- `boundariesOwnerStep.ID()` MUST NOT assert that this project declares two
  architectures, because that is false for one key with two values and false
  again for a project with no collision at all;
- the collision's **rendered entry and its addressable handle** MUST name the
  colliding key(s) actually found.

And the non-requirement *"Zero golden bytes move under this change"* should
be replaced with the measured figure: **six lines, one per fixture, all in
slice 3; four regenerated with `-update`, two hand-edited; no fixture gains
an update path.**

### Decision 7 — alignment is two passes and `%-*s`, the glyph set is one constant block, and the width is fixed at 70

**Alignment.** Each block computes its own column widths from its own rows,
then formats:

```go
// widths returns the column width each row must be padded to, measured in
// runes rather than bytes because `──` and the status glyphs are multi-byte
// and %-*s pads by byte count.
//
// It is a separate pure function so alignment is tested as numbers. A test
// that asserts on rendered spacing pins layout; a test that asserts
// widths([]row{...}) == []int{...} pins the rule, and a mutant that drops
// the max comparison dies in it.
func widths(rows [][]string) []int
```

**Rejected: `text/tabwriter`.** It is stdlib and it does compute widths, and
it aligns everything written to one `Writer` as a single table — which is
wrong here, because the applied block, the file lines under a step, and the
satisfied block are three grids that must not share a width. Driving three
tabwriters and interleaving their flushes with the subprocess gutter is more
machinery than `%-*s` over a tested `widths`, and it hides the computation
the mutation tests need to own.

**Wrapping is fixed at 70 columns**, with a hanging indent taken from the
block's own gutter, through one pure `wrap(text string, width, indent int)
[]string`. **Rejected: querying the terminal width** — it needs a syscall
this stdlib-only product does not carry, and it makes the output depend on
the reader's window, which is untestable and useless in the agent transcript
§16 and §21 say is the second reader.

**The glyph set, and the honest state of the measurement.** The risk row
called this unmeasured. What is measurable from this repository, and now is:
**dharness already ships `──` (U+2500) to stdout on the gate path** —
`internal/cli/check.go:122` prints `"\n── %s ──\n"` on every `dharness
check`, and `internal/cli/check_test.go:429` pins it. The em dash (U+2014)
ships from `sync.go:50`, `mutate.go:234` and `help.go:9`. Neither survives a
`cmd.exe` console at code page 437, because Go writes UTF-8 and the console
decodes per its output code page — so **the legacy-console risk is already
taken by shipped code, and this change does not widen it in kind.**

So the approved set is adopted (`── ■ │ ✓ ✗ ·`), confined to one constant
block in `internal/report`, used from nowhere else:

```go
// The glyph set, in one place, because the fallback is a swap of this
// block and nothing else. ── already ships on the gate path
// (internal/cli/check.go:122) and is pinned by a test, so `sync` and
// `check` read as the same product.
const (
	glyphRule      = "─"
	glyphSummary   = "■"
	glyphGutter    = "│"
	glyphApplied   = "✓"
	glyphFailed    = "✗"
	glyphSeparator = "·"
)
```

`+ created`, `~ modified`, `= unchanged`, `! delegated` and `i note` are
ASCII in the approved report already and stay so.

**The measurement was not run in this phase**, which had no shell. It is a
merge condition on slice 1, in the shape `eslint-integration` used for its
split-layout probe and its gate-placement measurement: `chcp 437` in
`cmd.exe`, then `dharness sync`, recorded as a dated line in
`docs/learning-log.md`. If it mangles, the fallback is `-`, `#`, `|`, `+`,
`x`, `.` — six constants, no call site — **and `check.go:122` is the
pre-existing case that measurement also settles**, which is why the result
is worth having either way.

### Decision 8 — `setup.Run` returns results, `RunSync` assembles and renders, and `setup.Apply` keeps its signature

**Choice.**

```go
// internal/setup

// Run derives the plan, applies what it can, and reports one result per
// Plan() step plus every note read before the first byte changed. Notes are
// read first inside Run rather than by its caller, so the ordering
// internal/cli/sync.go:54-70 explains in comments becomes structural.
func Run(p project.Project) (steps []report.StepResult, notes []report.Note, err error)

// Apply keeps its exact signature. Its only remaining caller is
// renderGolden (internal/setup/golden_test.go:153), which passes
// io.Discard, so leaving it alone keeps the golden test file untouched.
func Apply(p project.Project, out io.Writer) error
```

`RunSync` becomes: discover, `setup.Run`, assemble `report.Report` (version,
root, source, summary, evidence, exit), render, return.

Three behaviours change and each is named rather than absorbed:

1. **Status is decided before a reason is asked.** `Satisfied` first →
   `satisfied` with its `Describe`/detection evidence; only an unsatisfied
   step is asked `Delegated`. That preserves today's semantics exactly and
   keeps the two fallback constants unreachable in the product, which
   Decision 6 depends on.
2. **The scoped-mutation evidence is no longer gated on `left == 0`.**
   `internal/cli/sync.go:129-134` prints it only when nothing is left, so a
   measured number vanishes the moment there is delegated work. The spec
   requires it rendered "when present"; `Report.Evidence` is set whenever
   `p.ReadEvidence().ScopedMutation` is non-nil (§08).
3. **The false sentence is deleted.** `"No earlier step is reported as
   having succeeded"` (`internal/setup/setup.go:110-112`) goes; the returned
   error keeps only the step name and the cause, and the retraction moves to
   `Rollback.Retracted`, which names the steps. The error is still returned
   so `runner.ExitCode` and the process agree — the report states the
   narrative, the error states the cause, and neither repeats the other.

**Every string this report renders is read as a sentence, and that is the
rule.** `Step.ID()` reaches the human view as a heading and the JSON as
`id`; the rollback error embeds it in prose. There is no field in this model
that a reader may treat as an opaque token exempt from being true —
`Collision.ID` is the single exception, and it is derived from `Key` rather
than written by hand, so it cannot say anything `Key` does not. This matters
because the repository's recurring failure is prose asserting what the code
does not do (`docs/learning-log.md`), and the usual escape — "it is only an
identifier" — is not available here: the identifiers are printed.

`--format` accepts `human` (default) and `json`; anything else is an error
naming both, in `newFlagSet`'s existing shape. `report.WriteJSON` is
`json.NewEncoder(w).SetIndent("", "  ").Encode(r)` — nothing but the report
reaches stdout under that flag.

### Decision 9 — four slices, and slice 3 is the one whose scope the proposal understated

| Slice | Content | ~lines | Golden bytes |
|---|---|---|---|
| 1 | `internal/report` entire: types, JSON tags, the human renderer, `widths`, `wrap`, the glyph block, tests | 320–420 | 0 |
| 2 | `Step.Apply` → `(Facts, error)` + `out io.Writer` across 10 steps and the test stub; the three sink sites; `Writer.Changed`; `applySteps` marks; `runner.ExitCode` moved with `app.ExitCode` forwarding | 140–200 | 0 |
| 3 | `tool.FallowConfigPath`/`FallowConfigJSON`; `resolvedConfig`; `setup.Collisions`; `declaredAt`; `boundariesOwnerStep.ID()` **and six fixture lines** | 180–260 | **6 lines, 6 files** |
| 4 | `setup.Run`; `RunSync` rewrite; `--format json`; failure variant; the collision branch collapse; `sync_test.go` rewrite | 280–420 | 0 |

Realistic total **~920–1,300 changed lines**, up from the proposal's
~560–1,050. The increase is slice 1's renderer, under-forecast at 200–350
once alignment, wrapping, five blocks and the failure variant are counted,
plus slice 2 carrying the `ExitCode` move that Decision 1 found.

**Which slice first changes observable output, exactly.** **Slice 4 is the
first slice to change the report's shape.** Slice 3 changes one line of its
*text* — `boundariesOwnerStep.ID()` is printed by
`internal/cli/sync.go:97`'s `"## Left to you: %s"` heading for a project
with a collision — and six lines of golden fixture. Slices 1 and 2 change
nothing a user sees: slice 1 has no consumer, and slice 2 moves subprocess
output off the process stdout and onto a sink `RunSync` immediately writes
back to stdout, so the bytes are the same until slice 4 frames them.

**Budget.** 1+2 = 460–620, 3+4 = 460–680; each pair inside the 800-line
session budget, each slice at or under 420 against the 400-line review
budget. **Decision needed before apply: No. Chained PRs recommended: Yes.
400-line budget risk: High.**

## Data Flow

    project.Discover(dir) ──→ Project{Root, Source, …}
              │
              ↓
    setup.Run(p)
      │
      ├─ notes read first, before any byte changes
      │    UncheckableConfigNote / UncertainPresetNote / EslintResidueNote
      │                          └──→ []report.Note   (residue lists its
      │                                entries in full; no flag is named)
      │
      ├─ for each step in Plan(), in order:
      │    Satisfied(p) ──→ status satisfied + Evidence   (Delegated not asked)
      │    Delegated(p) ──→ status delegated + Why
      │         │
      │         └─ boundariesOwnerStep ──→ setup.Collisions(p)
      │                                        │
      │                                        ├─ collidingKeys == 0 → no process
      │                                        └─ resolvedConfig(p)
      │                                             ├─ LocalBinary(fallow)=="" → absent
      │                                             ├─ config --path  exit 3   → absent
      │                                             ├─ config --format json    → map
      │                                             └─ unparseable             → absent
      │                                        ↓
      │                                   []report.Collision
      │                                     (Effective *string, absent
      │                                      whenever unmeasured — §09/§17)
      │    otherwise ──→ applySteps
      │                    before := len(w.touched)
      │                    facts, err := step.Apply(p, w, &log)
      │                    after  := len(w.touched)
      │                    Wrote  = w.Changed(p.Root, before, after)
      │                    Installed, Transcript, MS
      │                       │
      │                       └─ err != nil → w.Undo()
      │                                        step        → failed
      │                                        earlier     → retracted
      │                                        remaining   → not-reached
      │                                        Rollback{Retracted: […]}
      ↓
    RunSync assembles report.Report
      Version, Root, Source, Summary, Evidence, Notes, Steps, Rollback
      Exit = runner.ExitCode(err)          ← assigned, never computed
      ↓
    --format human ──→ report.WriteHuman(stdout, r)
    --format json  ──→ report.WriteJSON(stdout, r)     one value, two writers

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/report/report.go` | New | `Report`, `Summary`, `StepResult`, `FileChange`, `Collision`, `Declared`, `Note`, `Evidence`, `Rollback`, `Status`, `Kind`; JSON tags |
| `internal/report/human.go` | New | `WriteHuman`, the five blocks, `widths`, `wrap`, the glyph constant block |
| `internal/report/json.go` | New | `WriteJSON` — an indented `json.Encoder` over the same value |
| `internal/report/*_test.go` | New | Fact-and-count assertions, `widths`/`wrap` tables, absent-vs-empty marshalling |
| `internal/runner/runner.go` | Modify | `ExitCode` moves here from `internal/app`, beside `ExitError` (Decision 1) |
| `internal/app/app.go` | Modify | `ExitCode` becomes a one-line forwarder to `runner.ExitCode` |
| `internal/setup/setup.go` | Modify | `Step.Apply` signature; `Facts`; `Run`; `applySteps` marks and per-step buffer; the false rollback sentence deleted |
| `internal/setup/writer.go` | Modify | `Changed(root, from, to) []report.FileChange` |
| `internal/setup/steps.go` | Modify | 10 `Apply` signatures; the three sink sites (`:80`, `:82`, `:863`); `Collisions`; `renderCollisions`; `boundariesOwnerStep.ID()`; `declaredValue` off the collision path |
| `internal/setup/files.go` | Modify | `declaredLine` → `declaredAt` returning a line number |
| `internal/setup/notes.go` | New | The three existing notes reshaped into `[]report.Note` |
| `internal/setup/testdata/golden/*.txt` | **Modify** | **Six files, line 26 only** — four via `-update`, two hand-edited (Decision 6) |
| `internal/tool/tool.go` | Modify | `FallowConfigPath`, `FallowConfigJSON` |
| `internal/cli/sync.go` | Modify | Rewritten to build then render; `--format` |
| `internal/cli/sync_test.go` | Modify | Coordinated rewrite, 16 test functions |
| `internal/cli/check.go` | **Unchanged** | The gate's fallow stage keeps remote resolution |
| `internal/setup/golden_test.go` | **Unchanged** | `Apply`'s signature is preserved for exactly this reason |
| `docs/design-principles.md` | Modify | §03: the second recorded local-resolution exception, "locally or not at all" |
| `docs/learning-log.md` | Modify | The `fallow config` cost and exit contract; the legacy-console glyph result |

## Interfaces / Contracts

```go
// internal/runner
func ExitCode(err error) int

// internal/report
type Status string
type Kind string
type Report struct{ … }
func WriteHuman(w io.Writer, r Report) error
func WriteJSON(w io.Writer, r Report) error

// internal/setup
type Facts struct{ Installed []string }

type Step interface {
	ID() string
	Describe(p project.Project) string
	Satisfied(p project.Project) bool
	Delegated(p project.Project) (why string, ok bool)
	Apply(p project.Project, w *Writer, out io.Writer) (Facts, error)
}

func Run(p project.Project) ([]report.StepResult, []report.Note, error)
func Apply(p project.Project, out io.Writer) error   // unchanged
func Collisions(p project.Project) []report.Collision
func (w *Writer) Changed(root string, from, to int) []report.FileChange

// internal/tool
func FallowConfigPath() []string
func FallowConfigJSON() []string
```

## Testing Strategy

**The trap this repository has already paid for: a suite that pins strings
passes while the behaviour rots.** Every rule below exists to keep a test
owning a fact rather than a layout.

| Layer | What to test | Approach |
|---|---|---|
| Unit — model | Absent is not empty | `json.Marshal` a `Report` with `Effective == nil`; assert `effective` is *not in* the output. Then with `Effective = ptr("theirs")`; assert it is. A mutant that drops `omitempty` or fabricates a default dies here |
| Unit — model | The verdict is not computed | `Report.Exit == runner.ExitCode(err)` for a nil error and for an `*ExitError{Code: 2}`. A mutant substituting `failed == 0` dies on the delegated-work case, where the two disagree |
| Unit — alignment | Column rule, not spacing | Table over `widths([][]string)` returning `[]int`. Never an assertion on a rendered line's spaces |
| Unit — wrapping | The bound, not the break points | `wrap` output: no line exceeds `width`; the joined words equal the input's words. A mutant on `<` vs `<=` dies on the first |
| Unit — renderer | Facts and counts | Every `Plan()` step id appears; the summary's text precedes the first step block; `strings.Count(collisionBlock, "duplicates") == 1`; the residue note lists every entry and the output contains no `--show` |
| Unit — collision | One value, two views | Render both views from **one** `Report`; assert the key set and its order agree. A change that reintroduces a second walk cannot satisfy both from one value |
| Unit — measurement | The probe short-circuits | `runner.SetForTest` recording every `Command`. Exit-3 row asserts **exactly one command was run** — that is what kills a mutant deleting the short-circuit, where asserting only on `effective` would not |
| Unit — measurement | Absence has one shape | Table: no local binary (assert **zero** commands), exit 3, non-zero JSON exit, non-JSON stdout, key missing from the map. Every row: `Effective == nil`, and the run's error is nil |
| Unit — classification | Bytes, not calls | `t.TempDir()` with three real files, including a rewrite to **byte-identical** contents → `unchanged`. Without that row a mutant collapsing `unchanged` into `modified` survives |
| Integration | The sink holds | `applySteps` with a stubbed `runner.Run` that writes to the `out` it is given; assert the process stdout the test controls received none of it |
| Integration | Failure variant | Step 2 fails: assert step 1 is `retracted`, `Rollback.Retracted` names it, the nine later steps are `not-reached`, and `len(Steps) == 11` |
| Integration | No file is written | Snapshot the tree before and after `sync --format json`; assert the only differences are files the applied steps own |
| Golden | Six lines, and no more | `TestGenericGoldenIsUnchanged` and `TestFrameworkGoldens` stay green after slice 3's hand edits; `TestGenericMechanismHasNoUpdatePath` untouched |

**Mutation coverage, per `tools/mutationstaged` (floor 0.80).** Four
branches are easy to leave unkilled and each needs a positive assertion:

- the `--path` exit-3 short-circuit — assert the **command count**, not the
  result, or a mutant that runs both commands still produces an absent
  `effective` through the unmarshal failure and survives;
- `Changed`'s `existed` arm — the byte-identical rewrite row above;
- the `Why`/`Collisions` exclusivity in the human renderer — a `StepResult`
  carrying both must render the collision and not the prose, or the arm
  collapsing to "always print `Why`" survives;
- `widths`'s max comparison — a two-row table whose second row is wider.

## Threat Matrix

| Boundary | Applicability | Design response | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | **N/A** — nothing here classifies a file as executable | | |
| Git repository selection | **N/A** — `RunSync`'s existing `InRepository` guard is unchanged | | |
| Commit state | **N/A** — no file is written by the report (§07/§08) | | |
| Push state | **N/A** — no remote interaction | | |
| PR commands | **N/A** — no VCS automation | | |
| **Subprocess execution** | **Applicable, newly** — `sync` gains up to two processes it never spawned | Local binary only, never the remote executor, so the network is never reached (`internal/tool/tool.go:101-103`'s recorded rule); nothing runs until a collision is proved; every failure degrades to absence and never to a failed `sync` (§20) | The zero-command and exit-3 command-count rows above |
| Untrusted subprocess output | **Applicable** — fallow's stdout is decoded as JSON | `encoding/json` into `map[string]json.RawMessage`; a decode error is one of the absence paths, never a panic and never a partial read. `stderr` is discarded, so the `loaded config:` preamble cannot reach the parser | The non-JSON stdout row |
| Destructive write to a file dharness does not own | **N/A** — this change writes no file it did not already write; `Writer.Changed` only reads | | |
| Command-line length | **N/A** — both fallow commands are three fixed tokens | | |
| Console encoding | **Applicable** — a legacy Windows console decodes UTF-8 per its code page | One glyph constant block; no glyph class is introduced that `internal/cli/check.go:122` does not already ship | Slice 1's `chcp 437` merge condition (Decision 7) |

## Migration / Rollout

No migration. Nothing is persisted, no fixture is regenerated beyond
Decision 6's six lines, and an older binary re-derives the older plan from
the repository (§07). Reverting slice 4 alone restores today's output while
leaving the model in place, because nothing else consumes it; reverting
slice 3 restores the six fixture lines with it.

## Open Questions

- [x] **Do golden bytes move — settled, and they do.** Six fixtures, line 26,
      one line each. Verified against `internal/setup/golden_test.go:145` and
      every file in `internal/setup/testdata/golden/`. The proposal and the
      spec's non-requirement both need correcting. Decision 6.
- [x] **Can `ID()` survive as an opaque handle, keeping the goldens frozen —
      evaluated and rejected, with the evidence in Decision 6.** `Step`'s own
      comment calls it a heading, every production use renders it as prose,
      one `ID()` is a `fmt.Sprintf`, and the approved JSON twin's ids are
      slugs that no `ID()` returns. The deciding fact is exposure:
      `boundariesOwnerStep` is satisfied in every project with no collision,
      and this change starts rendering satisfied steps, so the false sentence
      would be printed on nearly every run instead of never. The separation
      the proposal was reaching for is kept — it is `Collision.ID`.
- [x] **Can `internal/cli` reach `app.ExitCode` — settled, and it cannot.**
      Import cycle; `ExitCode` moves to `internal/runner`. Decision 1.
- [ ] **The legacy-console glyph measurement is not run.** This phase had no
      shell. It is a merge condition on slice 1: `chcp 437` in `cmd.exe`,
      `dharness sync`, recorded as a dated line in `docs/learning-log.md`.
      The fallback is a six-constant swap with no call-site change, and the
      same run settles the pre-existing `check.go:122` case.
- [ ] **The spec's five-status enumeration needs a sixth value**,
      `retracted`, with `Summary.Retracted` beside it. Argued in Decision 1
      from the approved report's own tally, which accounts for ten of eleven
      steps without it. Proposed rather than assumed.
- [ ] **The spec's collision-identity requirement needs splitting, and its
      "zero golden bytes" non-requirement needs replacing with the measured
      figure.** As written the requirement puts a MUST on `ID()` naming the
      colliding key, which `ID()` cannot do — it takes no project. Decision 6
      gives both replacement clauses and the six-line figure. Proposed rather
      than assumed.
- [ ] **`UncheckableConfigNote`'s TOML blind spot** stays, per the resolved
      question round. `fallow config` reads all four formats and very likely
      retires it, but `resolvedConfig` is reached only after `declaredKeys`
      has already found a collision — and `declaredKeys` is the mechanism
      that cannot read TOML. Retiring the note would mean asking fallow
      *before* knowing there is anything to ask about, which is the
      unconditional cost §13 argues against. Recorded so the follow-up
      inherits the reason, not just the deferral.
