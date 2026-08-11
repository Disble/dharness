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

## Slice 3 — Contributed-key collision step, generalising `boundariesOwnerStep` — DONE

All of Phase 9, 10, 11 (tasks 9.1–9.8, 10.1–10.10, 11.1–11.3) are complete.
Nothing from Slice 4 (`folder-ownership`/`DefaultSeverity`) or Slice 5 (real
framework presets) was started, per the assigned scope.

### What was built

- `internal/setup/files.go`:
  - `declaredKeys(path string, candidates []string) []string` replaces
    `declaresBoundaries(path string) bool` — same quoted-key textual test,
    generalised to N candidates, returned in candidate order. A file that
    cannot be read declares nothing (nil slice).
  - `fallowConfigCandidates` (`.fallowrc.json`, `.fallowrc.jsonc`) and
    `fallowConfigPath(source string) string` — "which one responds", the
    same pattern `hookManager` already uses, resolving Decision 5's
    `.fallowrc.jsonc` widening. `fallow.toml` is deliberately absent from
    this list (task 9.8).
  - `declaredLine(path, key string) string` — a best-effort, single-line
    textual extraction of what the project itself wrote for a key, used to
    show "both values" in the collision report without a JSON/JSONC parser.
- `internal/setup/steps.go`:
  - `architectureStep.Satisfied` now calls `declaredKeys` with a one-element
    candidate list instead of `declaresBoundaries`.
  - `boundariesOwnerStep` widened from one key (`boundaries`) to
    `{"boundaries"} ∪ preset.Keys(preset.Resolve(p))`:
    - `collidingKeys(source string, matches []preset.Match) []string` — the
      pure intersection rule, split out for the same reason `resolve` is
      split out of `Resolve` (the real registry contributes nothing beyond
      `generic`'s empty manifest until Slice 5, so this is tested against
      stub matches).
    - `boundaryCollision(p project.Project) (colliding []string, matches
      []preset.Match, unreadable bool)` — the per-call guard: short-circuits
      on `!p.HasSource()` (same unsafe-cwd-read guard
      `TestBoundariesOwnerStepIsSatisfiedWithoutASource` already pinned for
      `Satisfied`, now extended to `Describe`/`Delegated` too since they now
      read a file where before they were fully static), and reports
      `unreadable == true` when the project's only fallow config is
      `fallow.toml` (see the Slice 3 judgment call below).
    - `Satisfied`/`Describe`/`Delegated` all route through
      `boundaryCollision`. `Describe`/`Delegated` fall back to the exact
      pre-widening single-key text (`boundariesFallbackDescribe`/
      `boundariesFallbackWhy`) when the collision set is empty — required
      for the golden fixtures' byte-identity (see below), and otherwise
      unreachable through `Pending`/the CLI since neither ever calls
      `Describe`/`Delegated` on a satisfied step.
    - `describeBoundaries`/`delegateBoundaries` — the dynamic, non-empty-
      collision renderer: one line per colliding key naming both dharness's
      value (`ownedValue`) and the project's declared value
      (`declaredValue`).
    - `ownedValue(key string, matches []preset.Match) string` — the
      architecture-block sentence for `"boundaries"`, or the composed
      preset value (via `composeFacts`, JSON-marshalled) for any other key.
    - `declaredValue`/`quotedKeys` — small rendering helpers.
    - `describeUnreadableFallowConfig`/`delegateUnreadableFallowConfig` —
      the `fallow.toml`-only case (judgment call, below).
- `internal/setup/files_test.go` — new file, Phase 9's RED/GREEN suite plus
  two extra tests for `fallowConfigPath`'s "which one responds" order and
  empty case, and two for `declaredLine`'s single-line honest limit.
- `internal/setup/steps_test.go` — new file, Phase 10's RED/GREEN suite plus
  a direct unit test for `quotedKeys` (needed to kill a mutation survivor —
  see below) and a test extending the cwd-safety guard to
  `Describe`/`Delegated`.

### The Slice 3 judgment call — tasks 10.9/10.10 and `doctor.config.ts`

Task 10.9's literal wording names `doctor.config.ts` as the fixture for
`TestCollisionCheckDescribesAndContinuesOnACodeConfig`. This does not match
the actual data flow: the collision check (`boundariesOwnerStep`) only ever
reads `.fallowrc.json`/`.fallowrc.jsonc` (task 9.8, design.md's Data Flow
diagram); `doctor.config.ts` belongs to `doctorConfigStep`, a step this
slice does not touch, and `declaredKeys` never reads it. Separately, task
9.8 states `fallow.toml` "stays excluded... an accepted false negative, not
fixed", while design.md's Decision 5 prose says "Where the config is TOML,
the step describes and continues, following the precedent already set for
`doctor.config.ts`" — two statements in tension for the same file.

Reconciled as follows, rather than guessed silently:

1. `declaredKeys`/`fallowConfigCandidates` stay exactly as task 9.8 states —
   no TOML branch inside `declaredKeys` itself, `fallow.toml` never joins
   the candidate file list. This is the literal, narrow reading of "stays
   excluded... not fixed".
2. `boundariesOwnerStep` separately detects "the project's only fallow
   config is a format I cannot check" via `p.HasFallowConfig()` (an
   existing `internal/project` method, unrelated to `declaredKeys`) and
   reports it — `Satisfied() == false`, `Delegated() == true`, `Describe`
   names the limit — rather than silently returning "no collision", which
   would be an unearned claim rather than the honest "cannot check" the
   spec's requirement text asks for ("MUST describe that its keys could not
   be checked and continue").
3. This is implemented and tested against `fallow.toml`, the one real file
   this reconciliation can ever encounter — not `doctor.config.ts`, which
   the step never reads under any implementation.

This was not raised as a blocking question because both readings of the
task list are internally coherent and testable; the alternative (leaving
10.9/10.10 undone) would under-deliver against spec.md's explicit MUST
requirement for no clear gain, and the reconciliation does not touch
`declaredKeys`'s own behaviour (task 9.8's stated scope) at all. Flagged
here for review rather than silently absorbed.

### RED/GREEN evidence

- Phase 9: `go test ./internal/setup/... -run TestDeclaresBoundariesIsNowDeclaredKeys`
  failed once (a stray prose mention of `declaresBoundaries` inside
  `declaredKeys`'s own doc comment, not a real symbol) before the comment
  was reworded to reference "its single-key predecessor" without naming it;
  green after.
- Phase 10: `TestCollisionNamesEveryContributedKeyTheProjectDeclares` was
  first written with the project's declared value and the preset's
  contributed value both literally `["wailsjs/**"]` — every assertion
  passed, but mutation testing (see below) proved the fixture could not
  distinguish `ownedValue`'s output from `declaredValue`'s, because both
  happened to render the same bytes. Rewritten with three distinct values
  (`dist/**` project-declared, `wailsjs/**` preset-contributed,
  `src/main.ts` an unrelated stand-in key) — this is the RED that actually
  proved the assertions test what they claim to.

### Mutation evidence

First `go run ./tools/mutationstaged` run: **0.91** (40/44 killed, 4
survived) — all four in `ownedValue`, all traceable to the fixture-collision
problem above (the "wailsjs/**" appearing on both the preset and project
side let three `Comparison Invert` mutants and one `Range Break` mutant
survive undetected, since the assertion was satisfied by the wrong source
either way). Fixing the test fixture (see RED/GREEN evidence) reduced this
to one remaining survivor.

Second run: **0.98** (44/45) — one `Range Break` survivor in `quotedKeys`
(the loop's first statement replaced with an unconditional `break`, leaving
every backticked key an empty string; nothing asserted the exact rendered
key list, only that the key name appeared somewhere in the message, which
it still did via the per-key loop in `delegateBoundaries` that does not use
`quotedKeys`). Added `TestQuotedKeysBackticksEachKey`, a direct unit test
against the helper's exact output — matching the codebase's existing
precedent of testing small pure helpers like `dedupe` directly rather than
only through what calls them.

Third run: **1.00** (45/45 killed, 0 survived) — floor (0.80) met with no
remaining survivors to explain away.

### Golden byte-identity (the acceptance test that outranks everything else)

`go test ./internal/setup/... -run TestGenericGoldenIsUnchanged -v` passes
byte-for-byte against the Slice 1 fixtures, unmodified — confirmed
explicitly, not merely inferred from the full suite passing. This is the
reason `Describe`/`Delegated` fall back to the exact pre-widening static
text when the collision set is empty (see "What was built" above): both
golden fixtures (`generic-conventional`, `generic-split`) declare no fallow
config at all, so `boundariesOwnerStep`'s collision set is always empty for
them, and the plan report's `why`/`describe` text for that step has to stay
byte-identical to what the single-key implementation always produced —
even though, in the real `Pending`/CLI flow, that text is never shown for a
satisfied step. No fixture under `testdata/golden/` was regenerated,
hand-edited, or deleted.

### Verification (all clean)

```
go build ./...        # exit 0
go vet ./...           # exit 0
gofmt -l .              # no output
go test ./...           # ok, all 9 packages
bash scripts/verify-gate.sh   # "verify-gate: the hook refused a broken file, as it must."
git add -A && go run ./tools/mutationstaged
  # Total: 45, Killed: 45, Survived: 0, Score: 1.00 (minimum: 0.80) — PASS
```

### `dirIgnore` check

No new committed file was introduced under `.dharness/` — this slice only
changes `internal/setup/files.go`, `internal/setup/steps.go`, and adds two
`_test.go` files, all ordinary Go source outside `.dharness/` entirely. No
`dirIgnore` edit needed.

### Notes for Slice 4 and Slice 5

- Slice 4 (`folder-ownership`/`DefaultSeverity`) is untouched — `steps.go`'s
  `doctorConfigStep.Apply` still calls the one-argument `DefaultSeverity(id
  string)`; that call site is Slice 4's to widen to `DefaultSeverity(p,
  id)`, not this slice's.
- `preset.Keys(preset.Resolve(p))` (Slice 2) is what `collidingKeys` already
  consumes; once Slice 5 registers `wails`/`nextjs`/`expo`, a real matched
  project will exercise `collidingKeys`'s non-empty-candidate path end to
  end for the first time — today only `TestBoundariesAloneStillCollidesUnchanged`-
  style tests (real registry, `boundaries` alone) and the stub-match tests
  in `steps_test.go` (simulated multi-key contributions) exercise it.
- Task 19.4 (`TestIgnorePatternsCollidesInTheMotivatingShape`) is the
  end-to-end proof this slice's collision mechanism was built for; nothing
  about `boundariesOwnerStep`'s shape is expected to change once a real
  preset exists — only `preset.Resolve`'s registry contents will.
- The Slice 3 judgment call above (10.9/10.10, `fallow.toml` vs
  `doctor.config.ts`) should be reviewed at Slice 3's PR review; if the
  reconciliation is rejected, `boundaryCollision`'s `unreadable` branch and
  `describeUnreadableFallowConfig`/`delegateUnreadableFallowConfig` are the
  isolated surface to revert or replace.

## Slice 4 — `folder-ownership` reclassification and `DefaultSeverity` parameterisation — DONE

All of Phase 12, 13, 14, 15 (tasks 12.1–12.8, 13.1–13.7, 14.1, 15.1–15.3) are
complete. Nothing from Slice 5 (real framework presets) was started, per the
assigned scope — no Wails, Expo, or Next.js preset was written.

### What was built

- `internal/project/git.go` — `PublishesBarrels() bool`, a new method beside
  the other git questions. Runs `git ls-files -z -- "*/index.ts"
  "*/index.tsx"` in `p.Source` through the existing `gitOutput` seam, exactly
  as the design specifies. Three early-return guards, in order:
  `!p.InRepository`, `!p.HasSource()`, and a non-nil error from `gitOutput` —
  all answer `false`, matching today's behaviour and never propagating an
  error (the same swallow precedent `Discover` already sets). The doc comment
  states both deliberate consequences directly: the probe reads the **index**
  (`ls-files`), so an unstaged barrel does not count as published; and the
  leading `*/` in each pathspec requires at least one directory component, so
  a root-level `index.ts` (a package entry point) is not barrel evidence.
- `internal/project/git_test.go` — new file. `stubBarrelIndex(t,
  present...)` is a test helper that simulates git's own pathspec matching
  via `path.Match` against the exact patterns the probe passes, rather than
  returning a canned response regardless of the invocation — this is what
  makes `TestPublishesBarrelsRequiresADirectoryComponent` an actual pin on
  the leading `*/`: weakening the pathspec in production code changes what
  the stub matches, not just what a fixed response happens to contain. Six
  tests: barrel present (true), no matches (false), git error (false), the
  two early-return guards (with a panicking stub proving the probe is never
  even called, not merely that the answer is false), the unstaged-barrel
  threat-matrix pin, and the directory-component pin.
- `internal/setup/plugin.go` — `offByDefault` (the package-level map) is
  deleted. `DefaultSeverity(rule string) string` becomes `DefaultSeverity(p
  project.Project, rule string) string`, switching on `folder-ownership` via
  `p.PublishesBarrels()`; every other rule still answers `"error"`,
  unchanged. The comment block above it is rewritten, not dropped, per task
  14.1: the eight-non-actionable-finding measurement stays verbatim, the
  conclusion changes from "therefore off everywhere" to "therefore off where
  the tree has no barrels" (error where it has at least one), and the
  first-write-only limit (`doctorConfigStep.Satisfied` returns true as soon
  as `RulesPackage` is in `plugins`, so this runs once, at first adoption) is
  now stated in the same place a reader would otherwise assume
  re-derivation happens on every `sync`.
- `internal/setup/plugin_test.go` — new file. `TestDefaultSeverityCompilesOnlyWithAProject`
  is the compile-time RED for the signature change — a call with two
  arguments, which failed to build against the one-argument form
  (`go vet ./internal/setup/...` reported "too many arguments in call to
  DefaultSeverity" before the signature changed). `TestFolderOwnershipIsErrorWhereBarrelsExist`
  / `TestFolderOwnershipIsOffWithoutBarrels` pin both directions of decision
  6 end to end, through `DefaultSeverity` rather than `PublishesBarrels`
  directly.
- `internal/setup/steps.go` — the one call site inside
  `doctorConfigStep.Apply`'s `if _, chosen := config.Rules[id]; !chosen`
  branch gains `p`: `DefaultSeverity(p, id)`. Nothing else about that
  branch's shape changed — the `!chosen` guard (§05) is untouched.
- `internal/setup/steps_test.go` — two new tests, in the file the task list
  names, using `stubMatch`/`doctorConfig`/`doctorConfigFile` already present
  there. `TestDefaultSeverityNeverCalledWhenProjectChoseIt` writes a
  `doctor.config.json` that already declares `dharness/folder-ownership`,
  then runs `doctorConfigStep{}.Apply` with a `gitOutput` stub that panics if
  called at all — proving the `!chosen` guard is unchanged, not merely that
  the written value still matches what was chosen.
  `TestAddingBarrelsAfterAdoptionDoesNotFlipSeverity` writes an already-adopted
  `doctor.config.json` (`RulesPackage` already in `plugins`,
  `folder-ownership` already `"off"`), stubs the barrel probe to report
  barrels now exist, and asserts `doctorConfigStep{}.Satisfied` is still
  `true` — pinning that a second `sync` never runs `Apply` again, so the
  first-write-only limit holds structurally rather than by care. Both tests
  passed on first write (no code change was needed to make them green), per
  the task list's own note that these regression guards must already pass.

### RED/GREEN evidence

- Phase 12: `go test ./internal/project/...` failed to build — `p.PublishesBarrels
  undefined (type Project has no field or method PublishesBarrels)` at all
  seven call sites in `git_test.go` — before `PublishesBarrels` existed.
  GREEN after `git.go` gained the method: all six new tests pass, full
  package green.
- Phase 13: `go vet ./internal/setup/...` failed with `too many arguments in
  call to DefaultSeverity — have (project.Project, string) want (string)`
  before the signature changed. GREEN after `plugin.go` and `steps.go`'s
  call site were updated: all three new `plugin_test.go` tests pass, and the
  two new `steps_test.go` tests (13.6/13.7) passed immediately — they pin
  properties the existing `!chosen` guard and `Satisfied` check already had,
  and the task list itself expected this ("write it to prove that, not
  assume it").

### Golden byte-identity (still the acceptance test that outranks everything else)

`go test ./internal/setup/... -run TestGenericGoldenIsUnchanged -v` passes
byte-for-byte against the Slice 1 fixtures, unmodified. This is the reason
`golden_test.go`'s `renderGolden` stubs `gitOutput` to return `nil, nil` for
every call: an empty `ls-files` response means `PublishesBarrels()` reads no
barrels, `DefaultSeverity` answers `"off"` for `folder-ownership`, and both
fixtures' `doctor.config.json` region already recorded `"dharness/folder-ownership":
"off"` — confirmed by grepping both fixtures before making any change. No
fixture under `testdata/golden/` was regenerated, hand-edited, or deleted.

### Mutation evidence

`go run ./tools/mutationstaged` over the staged set (`internal/project/git.go`,
`internal/setup/plugin.go`, `internal/setup/steps.go` — `steps.go`'s one-line
call-site change stages alongside these two, and mutationstaged scopes to the
whole set of staged production files, not this slice's file list alone):
**0.91** (20/22 killed, 2 survived) — floor (0.80) met.

Both survivors are `Comparison Replace` mutants on
`internal/project/git.go`'s **pre-existing, unmodified** guard in
`StagedSourceFiles`: `if err != nil || !p.HasSource() {` → `if false ||
!p.HasSource() {` and `if err != nil || false {`. This line is nowhere near
this slice's change. Confirmed with
`git diff --cached --unified=0 -- internal/project/git.go`: the entire
staged diff for that file is one insertion, `@@ -108,0 +109,29 @@` (the new
`PublishesBarrels` method, inserted after `StagedSourceFiles` ends) — no
other line in the file is touched.

This matches `docs/backlog/mutation-wrapper.md` entry 1
("Line scope loses which file a byte offset came from") exactly: when more
than one file is staged, the wrapper's byte-offset ranges are merged into one
flat list with no file identity, so a range computed from one file's diff can
land on a node in a different file. That entry's own measured symptom — "the
run mutated it anyway... in a file with no changed lines" for a
*different* file, three functions the diff never touched — is the identical
shape reproduced here, one slice later, against a different pair of files.
This is a known, already-recorded defect in the mutation tooling itself, not
a gap in this slice's code, and fixing it is out of this slice's scope (it
belongs to the wrapper's own backlog, not to `framework-presets`). Neither
mutation-coverage target the design names for this slice — the Source-scope
guard (Slice 2's) and the `*/index.ts` prefix (this slice's,
`TestPublishesBarrelsRequiresADirectoryComponent`) — has a survivor.

### Verification (all clean)

```
go build ./...        # exit 0
go vet ./...           # exit 0
gofmt -l .              # no output
go test ./...           # ok, all 9 packages
bash scripts/verify-gate.sh   # "verify-gate: the hook refused a broken file, as it must."
git add -A && go run ./tools/mutationstaged
  # Total: 22, Killed: 20, Survived: 2, Score: 0.91 (minimum: 0.80) — PASS
  # (staged set: internal/project/git.go, internal/setup/plugin.go, internal/setup/steps.go)
```

### `dirIgnore` check

No new committed file was introduced under `.dharness/` — this slice only
touches `internal/project/git.go`, `internal/setup/plugin.go`,
`internal/setup/steps.go`, and adds two `_test.go` files, all ordinary Go
source outside `.dharness/` entirely. No `dirIgnore` edit needed.

### Notes for Slice 5

- `DefaultSeverity(p, rule)`'s new signature and `PublishesBarrels()` are
  both stable surfaces now — Slice 5 does not touch either. No preset
  manifest may name `folder-ownership` or any rule severity (spec's own
  explicit non-requirement, already enforced structurally: no preset's
  `Manifest.Facts` has anywhere to put a rule id, and nothing in
  `internal/preset` imports `internal/setup`).
- The golden fixtures' `doctor.config.json` region continuing to show
  `"dharness/folder-ownership": "off"` is now derived (via the stubbed empty
  `gitOutput`) rather than hard-coded (via the deleted `offByDefault` map) —
  worth knowing if a future golden regeneration ever changes what
  `renderGolden`'s `gitOutput` stub returns, since that stub now answers two
  different questions (`Discover`'s lockfile/toplevel queries *and* the
  barrel probe) with the same blanket `nil, nil`.
- The mutation-wrapper's entry-1 defect (byte-offset scope losing file
  identity) is now reproduced a second time, against a second pair of files,
  independently of Slice 3's own encounters with this tool. Worth surfacing
  to whoever picks up that backlog: it is not a one-off.

## Slice 5 — Wails preset and the `Match.Uncertain` design change — PARTIAL, split by framework

Only Phase 16 (Wails), plus the orchestrator's design-review addition
(`preset.Match.Uncertain` and its surfacing), plus the parts of Phases 18/19
that only needed Wails, are complete. Next.js, Expo, `factory.go`, the
multi-preset composition proof, and the registry-wide contract test are
**not started**. Reported rather than crammed, per the review workload
forecast's own explicit permission to split per framework.

### Why this was split

The staged diff for Wails alone (`internal/cli/sync.go`,
`internal/preset/preset.go`, `internal/preset/wails.go`,
`internal/setup/steps.go`, plus the four test files and the golden fixture)
came to 744 lines total, ~475 of them code/tests/docs excluding the
generated `wails.txt` fixture (269 lines) — already over the 400-line
budget before Next.js or Expo existed. The task list's own Review Workload
Forecast for Slice 5 named exactly this outcome as the likely one ("split
per framework (Wails first, then Expo+Next.js together or separately) if
the real diff exceeds budget") and the delivery strategy is `auto-chain`, so
a further PR in the chain is the expected continuation, not a shortfall.

### What was built

- **The orchestrator's design change, implemented first, per its own
  instruction.** `preset.Match` gained an `Uncertain string` field
  (`internal/preset/preset.go`) — empty when a match read everything it
  needed, or naming what it could not read otherwise. `Detect` stays pure
  (no error return): uncertainty is data on the match, not a failure.
  Pinned by `TestMatchCarriesUncertain` (`internal/preset/preset_test.go`),
  a compile-time-shaped RED before the field existed.
- **Surfacing, following the `UncheckableConfigNote` precedent exactly**
  (`internal/setup/steps.go`): `UncertainPresetNote(p project.Project)
  string` walks `preset.Resolve(p)` and joins every non-empty `Uncertain`,
  naming the preset's ID beside what it could not read. Split into a pure
  `uncertainNotes(matches []preset.Match) string` for the same reason
  `collidingKeys` is split from `boundaryCollision` — no real preset carried
  a non-empty `Uncertain` until wails registered, so the rendering rule is
  tested against stub matches (`TestUncertainNotesNamesTheMatchAndWhatItCouldNotRead`,
  `TestUncertainNotesEmptyWhenNothingIsUncertain`, both in
  `internal/setup/steps_test.go`). Wired into `internal/cli/sync.go`
  alongside `uncheckable`, printed under its own `## Not checked` block —
  the same header `UncheckableConfigNote` already uses, kept as two
  independent blocks rather than merged, since the two blind spots have
  nothing to do with each other and merging them would couple two
  unrelated notes' wording.
- `internal/preset/wails.go` — the `wails` preset (Root scope). `Detect`
  distinguishes "file absent" (`os.Stat` fails → no match, this is not a
  Wails project) from "file present but unreadable" (a read or JSON decode
  failure after the `os.Stat` check succeeds → still matches, contributes
  the documented `frontend`-relative default, and sets `Uncertain` naming
  `wails.json` and what went wrong). The task list's own wording at 16.5
  ("on read/decode failure, fall back... and say so in evidence") reads as
  if both cases were the same branch; they are not, and treating them the
  same would make a genuinely absent `wails.json` match anyway. Read the
  distinction from `TestWailsNoMatchNoEvidence` (no file → no match) versus
  `TestWailsMalformedJSONStillMatchesAndReportsUncertain` (file present,
  garbage inside → matches, `Uncertain` set).
- `wailsIgnorePattern(p, dir)` computes
  `filepath.Rel(source, filepath.Join(p.Root, dir, "wailsjs"))` +
  `"/**"`, `filepath.ToSlash`'d — design decision 9's formula, verified
  against Wails' own source (`v2/pkg/commands/build/base.go`'s
  `frontend` default, `v2/internal/project/project.go`'s `wailsjsdir` JSON
  tag). `source` falls back to `p.Root` when `p.Source == ""` (no JS
  project detected at all) — a case the design's formula doesn't name, so
  reading the closest analogue was a judgment call rather than a documented
  answer: with nothing more specific to be relative to, the repository root
  is what's left.
- `internal/preset/wails_test.go` — Phase 16's full RED/GREEN suite
  (16.1–16.9), plus `TestWailsMalformedJSONStillMatchesAndReportsUncertain`,
  the orchestrator's own explicitly-requested acceptance test for the
  design change (not in the original task list — labelled 16.10 in
  `tasks.md`).
- `internal/preset/preset.go`'s `registry` var now holds `wails{}` ahead of
  `generic{}` (both Root scope; order between two Root-scope presets that
  both match — `generic` always does — doesn't affect correctness, since
  `generic`'s manifest is empty and contributes no key to collide with
  anything). This is a direct edit to the existing `registry` var, **not**
  the `factory.go` Phase 18.2 names — that abstraction is deferred to the
  PR that also adds `nextjs`/`expo`, since a "one switch in a factory" for
  a registry of one new entry would be premature structure for what this
  pass actually needed.
- `internal/setup/steps_test.go` — `TestWailsMatchedOwnedFileIsNoLongerEmpty`
  (19.3) and `TestIgnorePatternsCollidesInTheMotivatingShape` (19.4), both
  through the real registry (`Apply`/`preset.Resolve`, not stub matches) —
  the first time in this change a framework preset is exercised end to end
  rather than through `stubMatch`.
- `internal/setup/golden_test.go` — `wailsProject(t)`, the split-layout
  fixture (`wails.json` at Root, JS project at `Root/frontend`) design
  decision 9 was verified against. `TestFrameworkGoldens`'s case table
  gained one entry, `{"wails", wailsProject}`; the fixture
  `internal/setup/testdata/golden/wails.txt` was captured with
  `-update`, confirmed RED before it existed
  (`open testdata\golden\wails.txt: ... cannot find the file`), confirmed
  GREEN after, and grepped for `Temp`/`AppData`/`Users`/drive letters —
  none found.
- `docs/learning-log.md` — three new dated lines: the `Uncertain` field's
  own rationale, the cross-platform path bug this slice found and fixed
  (below), and the Root == Source vs split-layout derivation distinction
  task 16.4's literal wording elided.

### A real bug the golden fixture caught, not a task list ambiguity

`wails.go`'s first draft built `Evidence`/`Because` from
`filepath.Join(p.Root, "wails.json")` directly — a path with the OS's own
separator. Rendered into text and captured on Windows, the fixture read
`<root>\wails.json`: `golden_test.go`'s own `substitutePaths` replaces both
native and slash spellings of `p.Root`, but the literal `\wails.json` suffix
survives untouched, and the same code run on Linux/macOS would render
`<root>/wails.json` instead — a fixture that cannot pass on both platforms
at once. Fixed by introducing a `display := filepath.ToSlash(path)`
specifically for the rendered text, keeping the native `path` for the
actual `os.Stat`/`os.ReadFile` calls. The fixture was captured only after
this fix; nothing under `testdata/golden/` reflects the pre-fix shape.

### The task 16.4 reconciliation (a judgment call, not silently absorbed)

Task 16.4's literal wording names `"wailsjs/**"` as the expected contributed
value for "a Root == Source fixture" with `wailsjsdir` absent. That is task
16.7's split-layout answer, not this one: design decision 9's own formula —
`filepath.Rel(p.Source, filepath.Join(p.Root, wailsJSDir, "wailsjs"))`,
verified against Wails' own source — computes `"frontend/wailsjs/**"` when
Root and Source are the same directory, because the default `wailsjsdir`
("frontend") still names a subdirectory one level below both. Implemented
and pinned against the formula's actual output, with the discrepancy
recorded here rather than silently matching the task's literal string (which
would have required either weakening the formula or duplicating the
split-layout answer for a fixture that isn't split) — the same posture
Slice 3 took for task 10.9's `doctor.config.ts` tension.

### RED/GREEN evidence

- `preset.Match.Uncertain`: no formal RED run (a struct field addition is a
  compile-time change); `TestMatchCarriesUncertain` was written and passed
  immediately once the field was added, functioning as the compile pin the
  task's RED would have been.
- `internal/preset/wails.go`: `go test ./internal/preset/... -run TestWails`
  failed to build (`undefined: wails`) before `wails.go` existed; all seven
  tests pass after.
- `internal/setup/steps.go`'s `uncertainNotes`: `go vet ./internal/setup/...`
  failed with `undefined: uncertainNotes` before the function existed;
  green after.
- `TestFrameworkGoldens/wails`: failed with
  `open testdata\golden\wails.txt: The system cannot find the file
  specified` before the fixture was captured; passed after `-update`, and
  again on a second unmodified run (confirming the fixture is stable, not
  merely written once).

### Verification (all clean, for the diff actually staged this pass)

```
go build ./...        # exit 0
go vet ./...           # exit 0
gofmt -l .              # no output
go test ./...           # ok, all 9 packages
bash scripts/verify-gate.sh   # "verify-gate: the hook refused a broken file, as it must."
git add -A && go run ./tools/mutationstaged
  # Total: 42, Killed: 42, Survived: 0, Score: 1.00 (minimum: 0.80) — PASS
  # staged set: internal/cli/sync.go, internal/preset/preset.go,
  # internal/preset/wails.go, internal/setup/steps.go
```

No survivors to explain away this round.

### Not done this pass — belongs to the next PR in the chain

- Phase 17 (Next.js, Expo — dependency-only presets over `package.json`).
  Per the orchestrator's evidence-discipline instruction: neither has been
  verified against its own documentation yet, so the next pass either
  verifies what each generates and contributes a real fact, or ships an
  empty manifest with detection only and records that scoping choice
  explicitly — not invented here.
- Phase 18: `factory.go` (the registry currently holds a direct edit to
  `preset.go`'s `registry` var instead), the multi-preset composition test
  (18.1, needs `nextjs`), and the `nextjs`/`wails-nextjs` framework goldens
  (18.3/18.4 — only `wails` was captured).
- Phase 19.1/19.2: the registry-wide `TestEveryFactCarriesEvidence` walk
  needs all four presets to exist; `TestWailsEvidenceValidates` (16.9)
  covers wails alone in the meantime.
- Phase 20: the `entryPoints`-rejection precedent reaffirmation named in
  20.1 wasn't reached — no Next.js/Expo code exists yet where that
  temptation could appear. The region-decision half of 20.1 was already
  satisfied in Slice 2.
- Phase 21.2 (mutation over `nextjs.go`/`expo.go`/`factory.go`) and 21.4
  (the full proposal.md success-criteria sweep) — neither file nor a
  complete change exists yet to check.

---

## Slice 5b — Next.js, Expo, `factory.go`, and the `Manifest.Seeds` scope-closure (this pass, PR6, final PR of the chain)

Branch `feat/preset-nextjs-expo`, stacked on `feat/preset-frameworks` (PR #13)
→ #12 → #11 → #10 → #9. This closes both Phases 17–21 left open by the first
Slice 5 pass, and a scope miss the orchestrator identified: the proposal
wanted presets carrying facts *and* decisions, and the design's `Manifest`
only ever grew a `Facts` field — the decisions half (structural context, not
zones) was lost. This pass adds it.

### The `Manifest.Seeds` dimension (orchestrator-directed, not in design.md)

`internal/preset/preset.go` gained a `Seed{Text, Because}` type and
`Manifest.Seeds []Seed`, validated the same way `Fact` is —
`Manifest.Validate()` now also rejects a `Seed` with an empty `Because`.
`Seed` carries no `Key` field at all, so it structurally cannot collide with
Decision 8's reserved-`"boundaries"`-key guard: a preset cannot accidentally
write a zone through the seed path even if it tried. `preset.Seeds(matches
[]Match) []Seed` enumerates them in the same Root-then-Source, registry
order `Keys` already uses.

Wired into `ArchitecturePrompt` (`internal/setup/prompt.go`) as a fourth
`preset.Resolve` call site (design decision 4 names three: `ownedFilesStep`,
`boundariesOwnerStep`; this is the fourth, documented in the function's own
doc comment as such). Extracted into `renderSeeds(seeds []preset.Seed)
string`, returning `""` for an empty slice — the byte-identity path
`TestGenericGoldenIsUnchanged` depends on, confirmed unchanged (see
Verification below). The rendered framing is explicit per §21: "Confirm or
correct this against the tree. It names structure, not zones — those are
still read from the code and the person who wrote it, never from this
list." — never "these are your zones."

### `internal/preset` package doc — the "no per-framework rules" decision, written down

Per the orchestrator's fourth instruction: the user confirmed the eslint
plugin's rules are framework-agnostic, and no evidence supports a
framework-specific `maxFileLines` or severity. Rather than leave that an
implicit absence, `internal/preset`'s package doc comment (`preset.go`) now
states the reasoning directly: five of six rules are guardrails on generated
code with no framework axis, and `folder-ownership` already left the preset
rung entirely for git-derived detection (`DefaultSeverity`, Slice 4). A
future preset arguing otherwise needs a measured case, the same discipline
`CLAUDE.md` applies everywhere else in this repository.

### Next.js (`internal/preset/nextjs.go`)

Source scope; `Detect` reads `package.json`'s `dependencies`/
`devDependencies` for `next` (shared `declaresDependency` helper with expo).
**Contributes no `ignorePatterns` fact — measured, not left open.** fallow
honours gitignore; `.next/` is gitignored by every Next.js starter, so an
orphan file inside it is already invisible to fallow (an orphan file in a
*tracked* directory is reported as unused, the same file inside a gitignored
one is not — this is the decisive test, recorded in
`docs/learning-log.md`). A pattern here would re-implement what the CLI
already does — `CLAUDE.md`'s first rule, the same lesson this repository
already recorded once for `entryPoints`.

Contributes two `Seed`s instead, verified directly against Next.js's own
`project-structure` documentation page (not carried over from any local
checkout — the instruction was explicit that no local path to an upstream
reference project may appear in any artifact, and none does): the four
documented top-level folders (`app`/`pages` for routing, `public` for static
assets, optional `src`), framed as "routes are a delivery shell around the
domain, not domain modules themselves" — and the unopinionated-elsewhere
quote, verbatim: *"Next.js is unopinionated about how you organize and
colocate your project files,"* plus *"components and lib folders are
generalized placeholders, their naming has no special framework
significance."* Deliberately **not** conditioned on which of `app/`/`pages/`
actually exists in the tree — the seed states what Next.js documents, and
"What to find out" (unchanged, immediately below the seed section) is where
the agent reads the real tree. This keeps `Detect` a single package.json
read with no extra directory probing, and avoids inventing a naming
convention (`components/`, `lib/`) the framework's own docs explicitly
disclaim.

### Expo (`internal/preset/expo.go`)

Source scope; `Detect` reads `package.json` for `expo`, same shape as
Next.js. **Detection only — empty manifest, no facts, no seeds.** Expo's
file-based-routing documentation returned 404 during this pass; shipping an
invented seed would repeat a mistake this repository has already corrected
more than once (recorded in the package doc comment and
`docs/learning-log.md`). No `ignorePatterns` for the same gitignore reason
as Next.js (`.expo/` is gitignored by every Expo starter).

### `factory.go`

`registry` (`[]Preset{wails{}, nextjs{}, expo{}, generic{}}`) moved out of
`preset.go` into its own file — the design's "one switch in a factory" —
worth separating now that it is a four-entry registration point rather than
a two-line list next to `Resolve`/`resolve`/`Keys`/`Seeds`.

### Multi-preset composition and registry-wide validation

`TestWailsRootWithNextjsSourceContributesFromBoth` (`preset_test.go`, task
18.1) is the real scenario through the real registry (not stubs): a `wails.
json`-at-Root + `next`-dependency-at-Source fixture. **Correction against
the task's own prose**: the task's informal "one Root match and one Source
match" is not literal — `generic` always matches too (resolve's own
documented rule: "a matching preset short-circuits nothing"), so the
fixture actually resolves to three matches (`wails`, `generic`, `nextjs`),
with `generic`'s empty manifest contributing nothing observable. The test
asserts on the two real preset IDs being present and `expo` being absent,
not on an exact count — the first version of this test asserted `len(matches)
!= 2` and would have failed for the wrong reason (a correct implementation,
not a bug) had it not been caught before this run.

`TestRealRegistryFactsAndSeedsValidate` (task 19.1's registry-wide form,
named apart from the existing stub-only `TestEveryFactCarriesEvidence` to
avoid a name collision) walks `Resolve` over a fixture matching wails,
nextjs and expo simultaneously and asserts `Manifest.Validate()` on every
match — covering both `Facts` and the new `Seeds` in one pass.

### Framework goldens

`nextjs` and `wails-nextjs` captured via `-update` (task 18.3/18.4). RED
confirmed first: both reported "file not found" before capture.
`nextjs.txt` pins the two seeds rendered under a new "### What the framework
already documents" section in `architectureStep`'s `Describe` output, with
no `ignorePatterns` region (empty facts). `wails-nextjs.txt` pins both
contributions in one repository — wails' `ignorePatterns` fact in the
`fallow.jsonc` region and nextjs' seeds in the architecture prompt. **`wails.
txt` (already committed from the first Slice 5 pass) is confirmed
byte-for-byte unchanged** — `git status` shows no modification — because
wails contributes no seeds and this pass added no new `Fact`; grepped both
new fixtures for leaked absolute paths (`AppData`, drive-letter, `/tmp/`),
clean.

### Two real mutation survivors caught and fixed, not task-list ambiguities

First staged mutation run (`expo.go`, `factory.go`, `nextjs.go`, `preset.go`,
`prompt.go`) scored **0.90 (19/21 killed)**:

1. `Manifest.Validate`'s `Seeds` loop survived a `Range Break` mutant — no
   test constructed a `Manifest` with a `Seed` whose `Because` was empty, so
   the loop being replaced with an immediate `break` (never checking
   anything) went undetected. Fixed: `TestEverySeedCarriesEvidence`
   (`preset_test.go`), the direct counterpart to the existing
   `TestEveryFactCarriesEvidence`.
2. `internal/setup/prompt.go`'s `len(seeds) > 0` survived an `Integer
   Increment` mutant to `> 1` — every real preset that seeds (`nextjs`)
   happens to contribute exactly two seeds, so `>0` and `>1` were
   indistinguishable through it. Fixed by extracting the render branch into
   `renderSeeds(seeds []preset.Seed) string`, tested directly
   (`TestRenderSeedsEmptyForNoSeeds`, `TestRenderSeedsRendersExactlyOneSeed`
   — deliberately constructed with exactly one seed, not two) rather than
   only through whatever a real preset happens to contribute.

Re-run after both fixes: **1.00 (25/25 killed, 0 survived)**.

### Verification (all clean)

```
go build ./...          # exit 0
go vet ./...             # exit 0
gofmt -l .                # no output
go test ./...              # ok, all 9 packages (internal/app, internal/cli,
                              internal/preset, internal/project, internal/runner,
                              internal/setup, internal/testsupport/mutation,
                              internal/tool, tools/mutationstaged)
bash scripts/verify-gate.sh    # "verify-gate: the hook refused a broken file, as it must."
git add -A && go run ./tools/mutationstaged
  # Total: 25, Killed: 25, Survived: 0, Score: 1.00 (minimum: 0.80) — PASS
  # staged set: internal/preset/expo.go, internal/preset/factory.go,
  # internal/preset/nextjs.go, internal/preset/preset.go, internal/setup/prompt.go
```

### `docs/learning-log.md` — three new dated lines this pass

1. The gitignore/`ignorePatterns` measurement for Next.js and Expo.
2. The `Manifest.Seeds` scope-closure decision and why `Seed` cannot collide
   with the reserved-`boundaries` guard.
3. Expo's unverified-documentation state, recorded as a decision.

(Decision 8's region-marker line was already recorded in Slice 2; not
duplicated here.)

### Line count against the 400-line budget

Code changes (excluding golden fixtures and documentation):
`expo.go` 60, `expo_test.go` 56, `factory.go` 12, `nextjs.go` 95,
`nextjs_test.go` 81, `preset.go` +68/-diff, `preset_test.go` +67,
`golden_test.go` +43, `prompt.go` +34, `setup_test.go` +29 — **~545 lines**,
over the nominal 400-line budget the forecast already flagged this slice as
"High risk" for. Not split further: Next.js/Expo and the `Seeds` dimension
are interdependent (Next.js is the only preset that populates `Seeds` at
all), and this is explicitly the final PR of the chain per the task
assignment. Reported here rather than silently absorbed, per this pass's own
instruction to report an overrun rather than cram it.

### Full proposal.md success-criteria sweep (task 21.4)

| Criterion | Status |
|---|---|
| Generic project byte-identical (`Plan()` + tree) | Met — `TestGenericGoldenIsUnchanged`, unmodified fixtures, confirmed unchanged this pass |
| Wails project's `fallow.jsonc` no longer empty | Met — Slice 5 first pass, `TestWailsMatchedOwnedFileIsNoLongerEmpty` |
| Collision names the key, shows both values, run continues | Met — Slice 3 + Slice 5 first pass, `TestIgnorePatternsCollidesInTheMotivatingShape` |
| Resolving a collision makes the step disappear, nothing recorded | Met — Slice 3, `TestCollisionStepDisappearsWhenIntersectionEmpties` |
| Every manifest fact carries evidence | Met — `Manifest.Validate`, `TestRealRegistryFactsAndSeedsValidate` (this pass, extended to Seeds too) |
| `folder-ownership` derived, never overwrites a chosen severity | Met — Slice 4 |
| Wails root + Next.js source contributes from both, resolved by scope | Met — this pass, `TestWailsRootWithNextjsSourceContributesFromBoth` |
| `go test`/`go vet`/`gofmt`/`mutationstaged` clean on every slice | Met — every slice's own verification section, this one included |

All eight criteria met. The change is complete pending the orchestrator's
own review and merge sequencing.

### Not committed

Per instruction: no commit, no PR opened. All work left in the tree on
`feat/preset-nextjs-expo`.
