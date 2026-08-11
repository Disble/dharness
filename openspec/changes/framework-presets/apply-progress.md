# Apply progress: framework-presets

## Slice 1 — Golden pin over `Plan()` and step output — DONE

All of Phase 1, 2, and 3 (tasks 1.1–1.5, 2.1–2.2, 3.1–3.3) are complete.
Nothing from Slice 2 onward was started, per the assigned scope.

### What was built

- `internal/setup/golden_test.go` — new file:
  - `renderGolden(t, p) []byte`: renders Plan()'s report (per-step ID,
    `Satisfied`, `Delegated`'s ok+why, `Describe`, all verbatim) followed by
    the tree `Apply` writes (every file under `p.Root`, sorted,
    `filepath.ToSlash`, byte-fenced with `---`). Stubs `runner.Run` (no
    install shells out) and `project.SetGitOutputForTest` (so `Discover`/the
    future barrel probe answer from the fixture, not a real repository).
    Writes every line with an explicit `"\n"` so the fixture is
    platform-stable, and substitutes `p.Source`/`p.Root` (both native and
    slash-separated spellings, source before root since it nests inside root
    in a split layout) with `<source>`/`<root>`.
  - `TestGenericGoldenIsUnchanged` — table over `generic-conventional`
    (Root == Source) and `generic-split` (Root != Source, Wails-shaped, no
    `wails.json`). Plain `bytes.Equal` against the committed fixture. No
    `-update` path exists for this test, on purpose (Decision 7).
  - `TestFrameworkGoldens` — the living mechanism Slice 5 populates. Empty
    case table today; gated behind
    `go test ./internal/setup -run TestFrameworkGoldens -update` for whoever
    adds cases later. Exists now so the two categories are structurally
    distinct from this commit, not merely documented as distinct (task 2.2).
  - `TestGenericMechanismHasNoUpdatePath` — greps this file's own source
    between `TestGenericGoldenIsUnchanged` and `TestFrameworkGoldens` for
    `flag.Bool("update"` and fails if present, pinning that the generic
    mechanism never gains an update path.
- `internal/setup/testdata/golden/generic-conventional.txt` and
  `generic-split.txt` — captured by running `renderGolden` against today's
  code (no preset registry exists yet) via a temporary capture test that was
  written, run once, verified for leaked absolute paths, and deleted. Not
  hand-authored.

### RED/GREEN evidence

- RED: `go test ./internal/setup/... -run TestGenericGoldenIsUnchanged` failed
  with `open testdata\golden\generic-conventional.txt: The system cannot find
  the path specified` (and the same for `generic-split.txt`) before the
  fixtures existed.
- GREEN: same command passes after the fixtures were captured and the capture
  harness removed.

### Verification (all clean)

```
go build ./...        # exit 0
go vet ./...           # exit 0
gofmt -l .              # no output
go test ./...           # ok, all packages
bash scripts/verify-gate.sh   # "verify-gate: the hook refused a broken file, as it must."
git add -A && go run ./tools/mutationstaged
  # "go mutation: no staged production Go files; ooze was not started."
  # Expected per task 3.2: golden_test.go is test infrastructure, not
  # production code, so mutationstaged correctly finds nothing to mutate.
```

### `dirIgnore` check

`internal/setup/testdata/golden/*.txt` lives outside `.dharness/`, so
`internal/project/evidence.go`'s `dirIgnore` allow list does not apply — no
edit needed, matching the task list's own note.

### Notes for Slice 2

- Task 6.10 (Slice 2) retargets `TestGenericGoldenIsUnchanged` to run against
  the real registry once it exists — must still pass unchanged against these
  same two fixtures, or a regression exists.
- The golden renderer (`renderGolden`, `writtenTree`, `writeIndented`,
  `substitutePaths`, the two fixture builders) is reusable as-is for
  `TestFrameworkGoldens`'s Slice 5 cases; no changes anticipated there beyond
  populating the case table and adding `wails.json`/`package.json`
  dependency fixtures.

## Slice 2 — Registry, `Manifest`, `Detect`, `generic`, and the delimited-region machinery — DONE

All of Phase 4, 5, 6, 7, 8 (tasks 4.1–4.5, 5.1–5.11, 6.1–6.12, 7.1, 8.1–8.3)
are complete. Nothing from Slice 3 (collision step), Slice 4
(`folder-ownership`/`DefaultSeverity`), or Slice 5 (real framework presets)
was started, per the assigned scope.

### What was built

- `internal/preset/preset.go` — new package. `Schema` (`"dharness.preset/v1"`),
  `Scope` (`Root`/`Source`), `Preset` interface (`ID`, `Scope`, `Detect`),
  `Match`, `Fact`, `Manifest`. `Manifest.Validate() error` rejects an empty
  `Because`, an unencodable `Value` (via `json.Marshal`), a wrong `Schema`,
  and the reserved key `"boundaries"` — called only by tests, never by a run
  (design decision 2, §20). `Resolve(p)`/`resolve(p, presets)` — Root scope
  before Source, registry order within a scope, one `!p.HasSource()` guard
  for Source-scope presets (decision 3/4). `Keys(matches)` enumerates
  contributed key names in first-seen order. `registry` is a package var
  holding just `generic{}` this slice — Slice 5's factory widens it.
- `internal/preset/generic.go` — the `generic` preset: Root scope, always
  matches, evidence `"no framework signal matched"`, empty manifest. This is
  what makes the golden pin's byte-identity claim provable (decision 9).
- `internal/setup/owned.go` — new file, Decision 8's region machinery:
  - `presetBegin`/`presetEnd` — the marker literals, spelled
    `// dharness:presets begin — rewritten by \`dharness sync\`; edits here
    are lost.` / `// dharness:presets end`. The wording states its own
    machine-written nature and consequence directly in the file (see
    "Marker spelling" below).
  - `architectureSkeleton` — the constant string `ownedFilesStep.Apply` used
    to write wholesale before this slice; now doubles as the base a region
    is inserted into on a project's first `sync`.
  - `presetRegion(matches []preset.Match) string` — renders the composed
    facts as JSONC, or `""` when nothing is contributed.
  - `composeFacts` / `composedFact` — Decision 3's per-key rule: a list
    unions across every contributor in resolve order via the existing
    `dedupe`; a scalar is won by the first contributor (Root, since matches
    arrives Root-before-Source), losing contributors rendered as comments.
    **Judgment call**: this composition function lives in
    `internal/setup/owned.go`, not `internal/preset` — the design's File
    Changes table assigns `presetRegion` to `internal/setup`, and its Data
    Flow diagram routes union/scalar-collision logic through
    `setup.presetRegion` rather than through `preset.Resolve`. Tasks
    5.7–5.10 named `internal/preset/preset_test.go` as one option but
    explicitly hedged with "or internal/setup/owned.go — whichever package
    Decision 8's presetRegion belongs to"; a test calling an unexported
    `internal/setup` function from `internal/preset` would need an import
    cycle, so the tests for this logic live in
    `internal/setup/owned_test.go` instead.
  - `replaceRegion(existing, region string) string` — `region == ""` returns
    `existing` byte-for-byte unchanged (checked first, ahead of marker
    detection — this is the path `generic` depends on and it takes priority
    regardless of what markers are already present); markers present →
    replace only the bytes between them; markers absent → insert
    immediately after the first `{`.
  - `regionBounds`/`regionBytes` — locate and extract the currently-written
    region's byte range, shared by `replaceRegion` (splice out the old
    region) and `ownedFilesStep.Satisfied` (byte-compare against what the
    current matches would render).
- `internal/setup/steps.go` — `ownedFilesStep.Satisfied` now checks
  `ownedLefthook`/`ownedRules` for existence (unchanged) and compares
  `fallow.jsonc`'s region bytes against `presetRegion(preset.Resolve(p))`
  (new). `ownedFilesStep.Apply` reads the existing `fallow.jsonc` if present
  (falling back to `architectureSkeleton`) and writes
  `replaceRegion(base, presetRegion(preset.Resolve(p)))` instead of the
  fixed literal — the `boundaries` block the agent writes now survives a
  second `sync`.
- `internal/preset/preset_test.go`, `generic_test.go`,
  `internal/setup/owned_test.go` — RED/GREEN test suites for all of the
  above, plus a battery of mutation-killing tests added after the first
  `mutationstaged` run (see Mutation section below): exact byte-boundary
  pins for `regionBounds` (markers at byte 0, no trailing newline, absent
  markers in three shapes), a no-brace fallback test, single- and
  multi-contributor `composeFacts` tests, a comment-count-exactness test for
  the scalar-collision winner, two `resolve`-loop-continuation tests
  (`stubPreset`), an unencodable-`Value` `Validate` test, and an
  existence-precedence test for `ownedFilesStep.Satisfied`.
- `docs/learning-log.md` — two new dated lines: the JSONC/`json.Unmarshal`
  measurement, and the marker spelling decision with its rationale.

### The marker spelling (Decision point, resolved)

`// dharness:presets begin — rewritten by \`dharness sync\`; edits here are
lost.` / `// dharness:presets end`, exactly as design.md proposed. Chosen
as-is rather than shortened, specifically because the design's own
self-explanatory requirement (a human opening the file by hand should be
able to tell the region is machine-written and that edits are lost without
reading external docs) is satisfied by stating both facts — "rewritten by
`dharness sync`" and "edits here are lost" — directly in the begin marker's
own line, rather than only in a separate comment above it. This ships from
this slice onward and is recorded in `docs/learning-log.md` per the task
list's instruction, since changing it later orphans the region in every
repository that ran an older binary.

### Marker re-insertion placement (Decision point, resolved)

Pinned by `TestRegionIsReinsertedWhenTheMarkersAreRemoved` against a fixture
shaped like the wild — opens with `{` then a comment line, no markers
present — not just an empty file. `replaceRegion` inserts immediately after
the first `{`, which is the byte dharness itself always writes first. No
edge case beyond the base "insert after first `{`, then a newline" rule
surfaced; task 6.6 found nothing to fix.

### RED/GREEN evidence

- Phase 4/5 (`internal/preset`): confirmed RED by writing `preset_test.go`
  and `generic_test.go` against an empty `internal/preset/` directory —
  `go test ./internal/preset/...` failed with `no non-test Go files` before
  `preset.go`/`generic.go` existed. GREEN after both files were added:
  `go test ./internal/preset/...` → `ok`.
- Phase 6 (`internal/setup/owned.go`): confirmed RED with
  `go vet ./internal/setup/...` failing `undefined: presetRegion` after
  `owned_test.go` was written but before `owned.go` existed. GREEN after
  `owned.go` was added and `steps.go` was wired to it.
- Mutation-killing tests (added after the first `mutationstaged` run scored
  0.69): each new test was written to reproduce a specific surviving diff
  (see below), confirmed to fail against the pre-existing code shape the
  survivor exploited via manual reasoning about each mutant's semantics,
  then confirmed green once present. The one exception —
  `TestOwnedFilesSatisfiedRequiresEveryOwnedFileToExist` — did NOT kill its
  target `Range Break` mutant on the first attempt (the fixture accidentally
  left `fallow.jsonc` absent too, so the missing-region path returned
  `false` regardless of the mutation); it was corrected to also write a
  matching `fallow.jsonc` so only the `ownedRules`-existence check could
  still fail, and the second `mutationstaged` run confirmed the kill.

### Mutation evidence

First `go run ./tools/mutationstaged` run (test-only, before the
mutation-killing battery): **0.69** (65/94 killed, 29 survived) — below the
0.80 floor. All 29 survivors were `internal/preset/preset.go` and
`internal/setup/owned.go`/`steps.go` diffs, none in a red suite (`go test
./...` was green throughout, satisfying the "green suite is a precondition
for reading the score" rule).

18 killing tests were added (listed under "What was built" above). Second
run: **0.88** (83/94). One target (`Range Break` in
`ownedFilesStep.Satisfied`'s existence loop) was not yet killed — the
fixture bug described above. Fixed, third run: **0.89** (84/94 killed, 10
survived) — floor met.

The 10 remaining survivors are all in `regionBounds`'s guard clause
(`if beginIdx == -1 || endIdx == -1 || endIdx < beginIdx`) and are
equivalent mutants, not gaps:

1. **`endIdx == -1` is a redundant disjunct.** Given `beginIdx >= 0` (the
   only case this clause could matter in), `endIdx == -1` implies
   `endIdx < beginIdx` is already true, so the third clause always catches
   it — three mutants exercise this dead code (`endIdx == -2`,
   `endIdx == -0`, and `endIdx == -1` replaced with `false`), and none is
   observable.
2. **`endIdx <= beginIdx` is unreachable as a distinct case from
   `endIdx < beginIdx`.** `presetBegin` and `presetEnd` are two different
   string literals, neither a substring of the other at any shared
   position, so `beginIdx == endIdx` can never happen — one mutant.
3. **Four "return a different dead value" mutants** (`return -1, 0, false`,
   `return 0, -1, false`, `return 1, 0, false`, `return 0, 1, false`) change
   `regionBounds`'s `begin`/`end` on the `ok == false` return path. Every
   caller (`replaceRegion`, `regionBytes`) branches on `ok` before ever
   reading `begin`/`end`, so the values are unobservable by construction.

None of these were chased further, per the mutation floor's own rule: a
survivor gets killed with a behavioural test or an explanation of why it is
equivalent, and the floor (0.89 ≥ 0.80) does not move.

### Verification (all clean)

```
go build ./...        # exit 0
go vet ./...           # exit 0
gofmt -l .              # no output
go test ./...           # ok, all 9 packages
bash scripts/verify-gate.sh   # "verify-gate: the hook refused a broken file, as it must."
git add -A && go run ./tools/mutationstaged
  # Total: 94, Killed: 84, Survived: 10, Score: 0.89 (minimum: 0.80) — PASS
```

### `dirIgnore` check

No new committed file was introduced under `.dharness/` — the region lives
inside the already-allow-listed `fallow.jsonc`. `internal/preset/` and
`internal/setup/owned.go` are ordinary Go source outside `.dharness/`
entirely. No `dirIgnore` edit needed, confirmed per the task list's own
re-check note.

### Notes for Slice 3

- The collision step (`declaredKeys`, the widened `boundariesOwnerStep`)
  will need `preset.Keys(preset.Resolve(p))` — already available from this
  slice, untouched.
- `composeFacts`/`presetRegion` are ready to render real facts once Slice 5
  registers `wails`/`nextjs`/`expo`; nothing about the rendering shape is
  expected to change, only the registry's contents (`registry` var in
  `internal/preset/preset.go`, to be replaced by Slice 5's `factory.go`).
- `TestApplyPreservesBoundariesOutsideTheRegion` and
  `TestPresetRegionRendersASingleContributorFact` currently exercise
  `replaceRegion`/`presetRegion` directly with hand-built `preset.Match`
  values rather than through a real preset, since none with a non-empty
  manifest exists yet — Slice 5 should add an end-to-end equivalent once
  `wails` exists, per its own task 19.3.
