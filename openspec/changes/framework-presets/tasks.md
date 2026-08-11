# Tasks: framework-presets

Chain: PR1 = Slice 1, PR2 = Slice 2, PR3 = Slice 3, PR4 = Slice 4, PR5 = Slice 5,
`stacked-to-main`. Order is load-bearing per the design's Line Forecast: nothing
after Slice 1 can be proven safe without its golden pin, Slice 3 must land before
Slice 5 writes any real preset key, and Slice 4 is independently mergeable
against `internal/project`/`plugin.go` only — it may move ahead of Slice 3 if
Slice 3 overruns its budget, but not ahead of Slice 1 or 2.

Every phase below is RED before GREEN, per Strict TDD Mode: each RED task names
the test and states why it fails on today's code; the following GREEN task is
the minimum change that turns it green. Mutation and full-suite verification is
the last phase of every slice, and a green `go test ./...` is a precondition for
reading `mutationstaged`'s score, not a consequence of it
(`docs/backlog/mutation-wrapper.md` entry 5) — never read a mutation score
against a red suite.

Any new file written under `.dharness/` in a fixture or golden that simulates a
*committed* dharness-owned tree must stay consistent with `dirIgnore`
(`internal/project/evidence.go:22-31`); this change touches no new committed
file (`fallow.jsonc` already carries the allow-listed name), so no `dirIgnore`
edit is expected — confirmed while reading the file for this task list, and
re-check this note at Slice 2 review in case an implementer requires the
region marker to force a different name.

---

## Slice 1 — Golden pin over `Plan()` and step output

### Review Workload Forecast — Slice 1

| Field | Value |
|---|---|
| Estimated changed lines | ~150–250 |
| 400-line budget risk | Low |
| Chained PRs recommended | Yes (this is PR1 of 5) |
| Chain strategy | stacked-to-main |
| Delivery strategy | auto-chain |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: Low

**Work unit**: PR1. Test: `go test ./internal/setup/...`. Runtime:
`go run ./tools/mutationstaged` over the new renderer only (no production
behaviour changes yet). Rollback: revert the PR; no production code path
changes, so reverting removes only test infrastructure.

#### Phase 1: the golden renderer (spec: `framework-presets` req "`generic` reproduces today's behaviour exactly")

- [ ] 1.1 RED `internal/setup/golden_test.go`: `TestGenericGoldenIsUnchanged` —
      renders `Plan()` (ID/`Satisfied`/`Delegated`'s `ok`+`why`/`Describe`
      verbatim) and the written tree (sorted paths, `filepath.ToSlash`, LF,
      `<root>`/`<source>` substituted) for a `generic-conventional` fixture
      (Root == Source, no framework) against `testdata/golden/generic-
      conventional.txt`; fails because neither the renderer nor the fixture
      file exists yet
- [ ] 1.2 GREEN `internal/setup/golden_test.go`: implement `renderGolden(t,
      p)` — stub `runner.Run` (no install shells out) and `gitOutput`
      (`Discover` and the not-yet-written barrel probe answer from the
      fixture); write LF explicitly, never the platform's
- [ ] 1.3 GREEN: capture `testdata/golden/generic-conventional.txt` by running
      the renderer against **today's code**, before the registry exists —
      this is the behaviour the spec requires the fixture to predate, not a
      hand-authored expectation
- [ ] 1.4 RED `internal/setup/golden_test.go`: `TestGenericGoldenIsUnchanged`
      (second case) for `generic-split` — Root != Source, the Wails-shaped
      split layout with no `wails.json` present; fails, no fixture yet
- [ ] 1.5 GREEN: capture `testdata/golden/generic-split.txt` the same way

#### Phase 2: the two-test mechanism that tells a legitimate change from a regression

- [ ] 2.1 RED `internal/setup/golden_test.go`: assert
      `TestGenericGoldenIsUnchanged` takes no `-update` flag path — a
      constructed test-of-the-test that greps the test source for the
      absence of `flag.Bool("update"` wired to the generic cases; fails
      until 2.2 separates the two mechanisms
- [ ] 2.2 GREEN: split the generic comparison (`TestGenericGoldenIsUnchanged`
      — plain `bytes.Equal`, no update path, ever) from the framework
      mechanism reserved for Slice 5 (`TestFrameworkGoldens`, gated behind
      `go test ./internal/setup -run TestFrameworkGoldens -update`) so the
      two categories in Decision 7's table are structurally distinct from
      the first commit, not merely documented as distinct

#### Phase 3: Slice 1 verification

- [ ] 3.1 `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`
      clean
- [ ] 3.2 `go run ./tools/mutationstaged` over `internal/setup/golden_test.go`
      — dry then real; the renderer is test infrastructure, so a low score
      here is expected and reported, not chased (no production branches to
      kill yet)
- [ ] 3.3 `bash scripts/verify-gate.sh`

---

## Slice 2 — Registry, `Manifest`, `Detect`, `generic`, and the delimited-region machinery

### Review Workload Forecast — Slice 2

| Field | Value |
|---|---|
| Estimated changed lines | ~350–450 |
| 400-line budget risk | High — first candidate to split (region machinery, then registry) if the real diff exceeds budget |
| Chained PRs recommended | Yes (this is PR2 of 5) |
| Chain strategy | stacked-to-main |
| Delivery strategy | auto-chain |

Decision needed before apply: **Yes — the `dharness:presets` marker spelling**
(open question below) must be settled at this slice's review, not deferred;
every repository that runs this binary from here on ships the region with
whatever spelling is chosen, and changing it later orphans the region in every
already-adopted repository.
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High

**Work unit**: PR2, based on PR1's branch, targets main once PR1 merges. Test:
`go test ./internal/preset/... ./internal/setup/...`. Runtime:
`go run ./tools/mutationstaged` over `internal/preset/`, `internal/setup/
owned.go`, `internal/setup/steps.go`'s `ownedFilesStep`. Rollback: revert the
merge commit — `.dharness/fallow.jsonc` returns to the always-empty skeleton,
no migration, nothing persisted to undo.

**Decision point, not a detail — resolve before merging this slice:** the
literal marker text `// dharness:presets begin` / `// dharness:presets end`.
Record the chosen spelling in this slice's PR description and in
`docs/learning-log.md` (Phase 6 below), because Decision 8 in the design states
this cannot be changed cheaply once shipped.

#### Phase 4: `internal/preset` package skeleton (spec: `framework-presets` req "one package per framework")

- [ ] 4.1 RED `internal/preset/preset_test.go`: `TestSchemaConstant` — asserts
      `preset.Schema == "dharness.preset/v1"`; fails, package does not exist
- [ ] 4.2 GREEN `internal/preset/preset.go`: `Scope`, `Schema`, `Preset`,
      `Match`, `Fact`, `Manifest` types per the design's Interfaces/Contracts
      section — no behaviour yet, compiles
- [ ] 4.3 RED `internal/preset/preset_test.go`:
      `TestEveryFactCarriesEvidence` — walks a manifest with one fact whose
      `Because` is empty and asserts `Manifest.Validate()` returns a non-nil
      error; fails, `Validate` does not exist
- [ ] 4.4 GREEN `internal/preset/preset.go`: `Manifest.Validate() error` —
      non-empty `Because`, `json.Marshal`-able `Value`, `Schema ==
      dharness.preset/v1`, and the key `boundaries` rejected outright
- [ ] 4.5 RED `internal/preset/preset_test.go`:
      `TestNoPresetContributesBoundaries` — a manifest with a fact keyed
      `boundaries` fails `Validate`; fails until 4.4's guard exists (may
      already pass if 4.4 lands first — write it anyway as its own named
      pin per Decision 8's two guards, both independently testable)

#### Phase 5: `generic` and the registry's composition rule (spec: `framework-presets` reqs "registry selects through one switch", "`generic` reproduces today's behaviour", "precedence is fixed and total", "multi-preset composition by scope")

- [ ] 5.1 RED `internal/preset/generic_test.go`: `TestGenericAlwaysMatches`
      — `generic{}.Detect(p)` returns `matched == true`, evidence "no
      framework signal matched", empty `Manifest`; fails, `generic` does not
      exist
- [ ] 5.2 GREEN `internal/preset/generic.go`: `generic` type, Root scope,
      always matches, empty manifest
- [ ] 5.3 RED `internal/preset/preset_test.go`: `TestResolveAlwaysReturnsAtLeastGeneric`
      — `resolve(p, []Preset{generic{}})` never returns nil/empty; fails,
      `resolve`/`Resolve` do not exist
- [ ] 5.4 GREEN `internal/preset/preset.go`: `Resolve(p)`/`resolve(p,
      presets)` — Root scope before Source scope, registry order within a
      scope; `resolve` split out exactly so composition is testable against
      stubs
- [ ] 5.5 RED `internal/preset/preset_test.go`:
      `TestSourceScopePresetsAreSkippedWithoutASource` — a stub Source-scope
      preset that always matches is absent from `resolve`'s output when
      `!p.HasSource()`; fails, no guard exists yet
- [ ] 5.6 GREEN `internal/preset/preset.go`: the one `!p.HasSource()` guard
      in `resolve`, per Decision 3 ("one guard in `resolve`, not four copies
      inside four `Detect` methods")
- [ ] 5.7 RED `internal/preset/preset_test.go`: `TestListKeysUnionAcrossScopes`
      — two stub presets (one Root, one Source) each contributing a
      different element to the same list key; composed result is the union
      in resolve order, duplicates removed, each element's evidence
      preserved; fails, composition function does not exist
- [ ] 5.8 GREEN `internal/preset/preset.go` (or `internal/setup/owned.go` —
      whichever package Decision 8's `presetRegion` belongs to per the
      design's File Changes table): list-key union using the existing
      `dedupe` helper
- [ ] 5.9 RED `internal/preset/preset_test.go`: `TestNoScalarKeyIsContributedTwice`
      — two stub presets contributing the same scalar key; Root scope's
      value wins, both evidence strings render as a comment beside it; fails
      until the scalar branch exists
- [ ] 5.10 GREEN: scalar-collision resolution per Decision 3's table (Root
      wins, losing value + both evidences commented)
- [ ] 5.11 GREEN `internal/preset/preset.go`: `Keys(matches []Match)
      []string` — enumerates contributed key names across matches, the
      mechanism proposal decision 4 and the collision step (Slice 3) depend
      on

#### Phase 6: the delimited region — `internal/setup/owned.go` (spec: `owned-config-contribution` req "`ownedFilesStep.Apply` writes the union... instead of an empty object"; design Decision 8)

- [ ] 6.1 RED `internal/setup/owned_test.go`: `TestRegionIsAbsentWhenNothingIsContributed`
      — `presetRegion(matches)` for `generic`'s empty manifest returns the
      empty string; fails, `presetRegion` does not exist
- [ ] 6.2 GREEN `internal/setup/owned.go`: `presetRegion(matches
      []preset.Match) string` — renders the union'd facts as JSONC comments +
      keys, or `""` when there is nothing to render
- [ ] 6.3 RED `internal/setup/owned_test.go`:
      `TestApplyPreservesBoundariesOutsideTheRegion` — a fixture
      `.dharness/fallow.jsonc` with a hand-written `boundaries` block below
      the (absent) region markers; `ownedFilesStep.Apply` with a non-empty
      region rewrites only the bytes between the two markers and leaves
      `boundaries` byte-identical; fails, `Apply` still writes the file
      wholesale
- [ ] 6.4 GREEN `internal/setup/owned.go`: `replaceRegion(existing, region
      string) string` — marker literals fixed by this slice's decision
      point above; markers present → replace between them; markers absent
      and `region != ""` → insert immediately after the first `{`; `region
      == ""` → no region written at all (this is the byte-identity path
      `generic` depends on)
- [ ] 6.5 RED `internal/setup/owned_test.go`:
      `TestRegionIsReinsertedWhenTheMarkersAreRemoved` — a fixture whose
      `fallow.jsonc` has neither marker but a non-empty region to write;
      `replaceRegion` inserts it immediately after the first `{`; fails
      until 6.4's insert branch is proven against a real JSONC shape (not
      just an empty file) — this is the placement test the design calls out
      as argued, not measured; pin it against a fixture shaped like the
      motivating repository's own `.fallowrc.json` (opens with a `{` then a
      comment line)
- [ ] 6.6 GREEN: fix any placement edge case 6.5 surfaces (e.g. a `{`
      immediately followed by a comment rather than a newline)
- [ ] 6.7 RED `internal/setup/steps_test.go` (or `owned_test.go`):
      `TestOwnedFilesSatisfiedComparesRegionBytesOnly` — `Satisfied` is
      `true` when the region's bytes equal what current matches render,
      `false` when they differ, regardless of unrelated bytes elsewhere in
      the file; fails, `Satisfied` still only checks file existence
- [ ] 6.8 GREEN `internal/setup/steps.go`: `ownedFilesStep.Satisfied` gains
      the region-byte comparison (existence check stays for the other two
      owned files, which are still rewritten wholesale)
- [ ] 6.9 GREEN `internal/setup/steps.go`: `ownedFilesStep.Apply` calls
      `preset.Resolve(p)` and writes through `replaceRegion`/`presetRegion`
      instead of the fixed empty-object literal
- [ ] 6.10 RED `internal/setup/steps_test.go`: retarget
      `TestGenericGoldenIsUnchanged` (Slice 1) to run against **today's
      registry** (registry + `generic` now exist) — this is the scenario
      "`generic` matches the golden byte-for-byte"; must still pass
      unchanged against the Slice 1 fixtures, or a regression exists
- [ ] 6.11 GREEN: fix whatever 6.10 reveals (expected: nothing, if 6.4's
      empty-region path is correct — this task exists to prove that rather
      than assume it)
- [ ] 6.12 GREEN `internal/setup/architecture_test.go` (or existing test
      file): a test asserting `architectureStep.Satisfied` stays `false` on
      a Wails-shaped fixture whose agent `boundaries` block has not been
      written — Decision 8's second guard, independent of the first

#### Phase 7: documentation

- [ ] 7.1 `docs/learning-log.md`: one dated line recording the JSONC/
      `json.Unmarshal` measurement (`.fallowrc.json` fails to parse as
      strict JSON on the motivating repository) and the chosen `dharness:
      presets` marker spelling from this slice's decision point

#### Phase 8: Slice 2 verification

- [ ] 8.1 `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`
      clean
- [ ] 8.2 `go run ./tools/mutationstaged` over `internal/preset/`,
      `internal/setup/owned.go`, `internal/setup/steps.go` — dry then real,
      floor 0.80; a green `go test ./...` from 8.1 is the precondition for
      trusting this score
- [ ] 8.3 `bash scripts/verify-gate.sh`

---

## Slice 3 — Contributed-key collision step, generalising `boundariesOwnerStep`

### Review Workload Forecast — Slice 3

| Field | Value |
|---|---|
| Estimated changed lines | ~200–300 |
| 400-line budget risk | Medium |
| Chained PRs recommended | Yes (this is PR3 of 5) |
| Chain strategy | stacked-to-main |
| Delivery strategy | auto-chain |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: Medium

**Work unit**: PR3, based on PR2's branch, targets main once PR2 merges. Test:
`go test ./internal/setup/...`. Runtime: `go run ./tools/mutationstaged` over
`internal/setup/files.go`, `internal/setup/steps.go`'s widened step. Rollback:
revert the merge commit; `boundariesOwnerStep` returns to its single-key form,
no persisted state to undo (§07).

**Must land before Slice 5 writes any real preset key** — per the design and
the orchestrator's framing, writing a preset's contributed keys into the owned
file without this collision report is exactly the silent no-op this whole
change exists to prevent.

#### Phase 9: `declaredKeys` generalises `declaresBoundaries` (spec: `owned-config-contribution` req "`declaresBoundaries` generalises into `declaredKeys`")

- [ ] 9.1 RED `internal/setup/files_test.go`: `TestDeclaredKeysFindsAQuotedKey`
      — `.fallowrc.json` containing `"ignorePatterns": [...]`; `declaredKeys(path,
      ["ignorePatterns", "boundaries"])` returns `["ignorePatterns"]`; fails,
      `declaredKeys` does not exist
- [ ] 9.2 GREEN `internal/setup/files.go`: `declaredKeys(path string,
      candidates []string) []string` — same body shape as `declaresBoundaries`
      (read once, `strings.Contains(raw, `"`+key+`"`)` per candidate),
      results in candidate order; a file that cannot be read declares
      nothing
- [ ] 9.3 RED `internal/setup/files_test.go`:
      `TestDeclaredKeysIgnoresACommentedKey` — a fixture whose file contains
      the bare-word comment sentence (matching `declaresBoundaries`'s
      existing precedent) and no quoted `"boundaries"` anywhere;
      `declaredKeys(path, ["boundaries"])` returns an empty slice; fails
      until the quoted-key test is proven against this exact comment shape,
      not just asserted by reading the design
- [ ] 9.4 GREEN: confirm 9.2 already satisfies 9.3 (expected — same
      mechanism as `declaresBoundaries`); if not, fix the quoting test
- [ ] 9.5 RED `internal/setup/files_test.go`:
      `TestDeclaresBoundariesIsNowDeclaredKeys` — `declaresBoundaries(path)`
      is retired in favour of `declaredKeys(path, []string{"boundaries"})`
      at every call site (`architectureStep.Satisfied`,
      `boundariesOwnerStep.Satisfied` before Phase 10 widens it); a grep-
      style test over the package asserts `declaresBoundaries` no longer
      exists as a symbol; fails until 9.6 removes it
- [ ] 9.6 GREEN `internal/setup/files.go`, `steps.go`: delete
      `declaresBoundaries`; both call sites route through `declaredKeys`
- [ ] 9.7 RED `internal/setup/files_test.go`:
      `TestDeclaredKeysReadsTheJSONCSpelling` — a fixture at
      `.fallowrc.jsonc` (not `.json`) declaring a candidate key is found;
      fails, only `.fallowrc.json` is checked today — **this is Decision
      5's stated widening beyond the proposal's letter; keep it visible
      here rather than folded silently into 9.2's diff**
- [ ] 9.8 GREEN: the collision check (Phase 10) and `architectureStep`'s
      config-path resolution both consider `.fallowrc.json` **and**
      `.fallowrc.jsonc`, matching fallow's own `fallowConfigFiles` list;
      `fallow.toml` stays excluded (TOML keys are bare — recorded as an
      accepted false negative, not fixed)

#### Phase 10: the widened `boundariesOwnerStep` (spec: `owned-config-contribution` reqs "widens from one key to the set of keys", "writes the contributed key anyway", "intersection emptying makes the step disappear", "a project config that is code cannot be checked")

- [ ] 10.1 RED `internal/setup/steps_test.go`:
      `TestCollisionNamesEveryContributedKeyTheProjectDeclares` — a stub
      project matched by presets contributing `{"ignorePatterns",
      "entryPoints"}` (stand-in keys for this test only — no real preset
      contributes `entryPoints`, see the rejected-key note below) whose own
      `.fallowrc.json` declares `ignorePatterns`; `Delegated` names
      `ignorePatterns` with both the preset value and the project's
      declared value, and does not mention `entryPoints`; fails, the step
      still only checks `boundaries`
- [ ] 10.2 GREEN `internal/setup/steps.go`: `boundariesOwnerStep.Satisfied`
      becomes unsatisfied exactly when `declaredKeys(path, {"boundaries"} ∪
      preset.Keys(matches))` is non-empty; `Describe`/`Delegated` iterate
      the intersection, naming every colliding key and both values
- [ ] 10.3 RED `internal/setup/steps_test.go`:
      `TestBoundariesAloneStillCollidesUnchanged` — a project declaring only
      `"boundaries"`, no preset matched beyond `generic`; step behaves
      exactly as `boundariesOwnerStep` does today; regression guard, must
      already pass after 10.2 — write it to prove that, not assume it
- [ ] 10.4 RED `internal/setup/steps_test.go`:
      `TestNoCollisionLeavesTheStepSatisfied` — no declared key intersects
      the contributed set; `Satisfied() == true`, step absent from `Pending`
      (§15); fails if 10.2's intersection logic inverts the check
- [ ] 10.5 RED `internal/setup/owned_test.go`:
      `TestOwnedFileWritesContributedKeyRegardlessOfCollision` — a project
      whose own config already declares a key a matched preset contributes;
      `ownedFilesStep.Apply` still writes that key into the owned file's
      region — the write is unconditional on the collision step's outcome;
      fails if `Apply` were ever made to skip on collision (guards against
      a plausible wrong implementation, not a symptom of current code)
- [ ] 10.6 GREEN: confirm `ownedFilesStep.Apply` (Slice 2) already never
      reads the collision step's result — no code change expected; if the
      test fails, remove whatever coupling was introduced
- [ ] 10.7 RED `internal/setup/steps_test.go`:
      `TestCollisionStepDisappearsWhenIntersectionEmpties` — the collision
      case from 10.1, then the project's key deleted; a second `Pending(p)`
      call omits the step, with no file read besides the project's own
      config (§07) — fails only if a cache is accidentally introduced;
      write it to pin the "nothing recorded" property structurally
- [ ] 10.8 RED `internal/setup/steps_test.go`:
      `TestCollisionStepReappearsWhenTheKeyIsReDeclared` — inverse of 10.7;
      re-adding the key brings the step back, unsatisfied, as if never
      resolved
- [ ] 10.9 RED `internal/setup/steps_test.go`:
      `TestCollisionCheckDescribesAndContinuesOnACodeConfig` — a project
      whose only doctor-adjacent config is `doctor.config.ts` (matching
      `legacyLintConfigStep`'s existing precedent, cited by name); the
      collision check does not attempt to parse it, reports it could not
      check, and `Pending`/`Apply` continue past it without stopping; fails
      until the step's `Satisfied`/`Describe` handle the code-config case
      explicitly rather than reading `declaredKeys` against a `.ts` file
      (which already answers "declares nothing" safely, but the *reporting*
      of "could not check" is new behaviour, not merely a safe default)
- [ ] 10.10 GREEN: the `Describe`/`Delegated` branch for a code-config
      project, following `legacyLintConfigStep`'s wording precedent

#### Phase 11: Slice 3 verification

- [ ] 11.1 `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`
      clean
- [ ] 11.2 `go run ./tools/mutationstaged` over `internal/setup/files.go`,
      `internal/setup/steps.go` — dry then real, floor 0.80
- [ ] 11.3 `bash scripts/verify-gate.sh`

---

## Slice 4 — `folder-ownership` reclassification and `DefaultSeverity` parameterisation

### Review Workload Forecast — Slice 4

| Field | Value |
|---|---|
| Estimated changed lines | ~150–200 |
| 400-line budget risk | Low |
| Chained PRs recommended | Yes (this is PR4 of 5) |
| Chain strategy | stacked-to-main |
| Delivery strategy | auto-chain |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: Low

**Work unit**: PR4. Independently mergeable — touches only
`internal/project/git.go` and `internal/setup/plugin.go`
(+ `steps.go`'s one call site), and nothing else in this change does. Test:
`go test ./internal/project/... ./internal/setup/...`. Runtime:
`go run ./tools/mutationstaged` over `internal/project/git.go`,
`internal/setup/plugin.go`. Rollback: revert the merge commit;
`offByDefault` returns, `DefaultSeverity` returns to its one-argument form.

**Threat-matrix row this slice opens, recorded rather than discovered later
as a bug**: `sync` reads the git **index** for the barrel probe
(`git ls-files`), for the first time in this codebase's history of asking git
questions. An unstaged barrel file on disk does not count as published. That
is deliberate — task 12.7 pins it so a future reader does not "fix" it.

#### Phase 12: `PublishesBarrels` (spec: `rule-severity-derivation` req "derived from barrel presence in the tree, asked of git")

- [ ] 12.1 RED `internal/project/git_test.go`:
      `TestPublishesBarrelsTrueWhenIndexHasABarrel` — `SetGitOutputForTest`
      stubs `git ls-files -z -- "*/index.ts" "*/index.tsx"` to return one
      path; `p.PublishesBarrels() == true`; fails, method does not exist
- [ ] 12.2 GREEN `internal/project/git.go`: `PublishesBarrels() bool` — the
      exact invocation from the design, run through the existing
      `gitOutput` seam, scoped to `p.Source`
- [ ] 12.3 RED `internal/project/git_test.go`:
      `TestPublishesBarrelsFalseWithNoMatches` — empty `ls-files` output →
      `false`
- [ ] 12.4 RED `internal/project/git_test.go`:
      `TestBarrelProbeAnswersOffWhenGitFails` — `gitOutput` stub returns a
      non-nil error → `false`, no propagated error (matches `Discover`'s
      swallow precedent)
- [ ] 12.5 RED `internal/project/git_test.go`:
      `TestPublishesBarrelsFalseOutsideARepositoryOrWithoutSource` —
      `!p.InRepository` and `!p.HasSource()` both answer `false` without
      calling `gitOutput` at all (mutation-coverage target: assert the probe
      is *not invoked*, not merely that the answer is `false` — a stub that
      panics on call proves this)
- [ ] 12.6 GREEN: the three early-return guards in `PublishesBarrels`
- [ ] 12.7 RED `internal/project/git_test.go`:
      `TestUnstagedBarrelDoesNotCount` — `ls-files` stub returns nothing
      while a fixture-only note records that `index.ts` exists on disk (the
      test does not need a real file — it documents the stubbed-index case
      as the whole of the behaviour); asserts `false` — pins the new
      threat-matrix row explicitly so it is never "fixed" as a bug later
- [ ] 12.8 RED `internal/project/git_test.go`:
      `TestPublishesBarrelsRequiresADirectoryComponent` — `ls-files` stub
      returns a root-level `index.ts` (no `*/` prefix match) and nothing
      else; asserts `false` — the mutation-coverage target the design names
      explicitly for the `*/` prefix (drop the prefix and this test must
      fail)

#### Phase 13: `DefaultSeverity(p, rule)` (spec: `rule-severity-derivation` reqs "`offByDefault` removed", "derived from barrel presence", "first-write default only")

- [ ] 13.1 RED `internal/setup/plugin_test.go`:
      `TestDefaultSeverityCompilesOnlyWithAProject` — a compile-time-shaped
      test asserting the new two-argument signature exists (calling
      `DefaultSeverity(p, "folder-ownership")`); fails to compile against
      today's one-argument function — this is the RED for a signature
      change, proven by the build failing, not by a runtime assertion
- [ ] 13.2 GREEN `internal/setup/plugin.go`: delete the package-level
      `offByDefault` map; `DefaultSeverity(p project.Project, rule string)
      string` switches on `folder-ownership` via `p.PublishesBarrels()`;
      every other rule still answers `"error"`
- [ ] 13.3 GREEN `internal/setup/steps.go`: the `DefaultSeverity` call site
      inside `doctorConfigStep.Apply` gains `p`
- [ ] 13.4 RED `internal/setup/plugin_test.go`:
      `TestFolderOwnershipIsErrorWhereBarrelsExist` — a project fixture
      whose `PublishesBarrels() == true` (via `SetGitOutputForTest`);
      `DefaultSeverity(p, "dharness/folder-ownership") == "error"`
- [ ] 13.5 RED `internal/setup/plugin_test.go`:
      `TestFolderOwnershipIsOffWithoutBarrels` — inverse; `"off"`
- [ ] 13.6 RED `internal/setup/steps_test.go`:
      `TestDefaultSeverityNeverCalledWhenProjectChoseIt` — a project whose
      `doctor.config.json` already declares a severity for
      `dharness/folder-ownership`; `doctorConfigStep.Apply` never calls
      `DefaultSeverity` for that id — assert via a `gitOutput` stub that
      panics if the barrel probe runs (proves the `!chosen` guard from
      §05 is unchanged, not merely that the written value matches)
- [ ] 13.7 RED `internal/setup/steps_test.go`:
      `TestAddingBarrelsAfterAdoptionDoesNotFlipSeverity` — a project
      already satisfying `doctorConfigStep.Satisfied` (package already in
      `plugins`) that later gains barrels; a second `sync` leaves
      `folder-ownership` at its original value because `Apply` never runs
      again — pins the stated first-write-only limit as a property, not a
      gap to be "discovered" later

#### Phase 14: documentation

- [ ] 14.1 `internal/setup/plugin.go`: rewrite the comment block above
      `DefaultSeverity` (formerly above `offByDefault`, `plugin.go:74-88`)
      — keep the eight-non-actionable-finding measurement, change the
      conclusion from "therefore off everywhere" to "therefore off where
      the tree has no barrels", and state the first-write-only limit in the
      same place a reader would otherwise assume re-derivation happens

#### Phase 15: Slice 4 verification

- [ ] 15.1 `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`
      clean
- [ ] 15.2 `go run ./tools/mutationstaged` over `internal/project/git.go`,
      `internal/setup/plugin.go` — dry then real, floor 0.80; both
      mutation-coverage targets named in the design (Source-scope guard is
      Slice 2's, `*/index.ts` prefix is this slice's) must show no survivor
      in this slice's files
- [ ] 15.3 `bash scripts/verify-gate.sh`

---

## Slice 5 — Wails, Expo, Next.js manifests and scope composition

### Review Workload Forecast — Slice 5

| Field | Value |
|---|---|
| Estimated changed lines | ~300–450 |
| 400-line budget risk | High — split per framework (Wails first, then Expo+Next.js together or separately) if the real diff exceeds budget |
| Chained PRs recommended | Yes (this is PR5 of 5, or PR5–PR7 if split per framework) |
| Chain strategy | stacked-to-main |
| Delivery strategy | auto-chain |

Decision needed before apply: No — but report back if a per-framework split is
needed; do not silently absorb an overrun.
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High

**Work unit**: PR5 (or PR5–PR7 if split), based on PR4's branch (or PR3's, if
PR4 was reordered ahead per its independence note), targets main once its base
merges. Test: `go test ./internal/preset/... ./internal/setup/...`. Runtime:
`go run ./tools/mutationstaged` over `internal/preset/wails.go`,
`nextjs.go`, `expo.go`. Rollback: revert the merge commit(s) in reverse order;
`.dharness/fallow.jsonc` loses the corresponding region content on the next
`sync`, nothing persisted to undo.

**Closed, not open — do not task `entryPoints`.** The orchestrator measured
`fallow dead-code -f json` reporting `entry_points` with 178 of 188 discovered
by fallow's own framework plugins (123 shipped by fallow itself). `CLAUDE.md`
line 13 records this exact lesson. No preset manifest in this slice contributes
an `entry` or `entryPoints` key. If a task below is found carrying one, delete
it before implementing — do not implement it "for completeness."

#### Phase 16: Wails (spec: `framework-presets` reqs "Wails detection evidence names the file", "the Wails ignore-pattern fact names the key and its fallback"; design Decision 9)

- [ ] 16.1 RED `internal/preset/wails_test.go`:
      `TestWailsDetectsWailsJSON` — a fixture with `wails.json` at Root;
      `wails{}.Detect(p)` returns `matched == true`, evidence naming
      `wails.json`; fails, `wails.go` does not exist
- [ ] 16.2 GREEN `internal/preset/wails.go`: `wails` type, Root scope,
      `Detect` checks `p.Root/wails.json` existence
- [ ] 16.3 RED `internal/preset/wails_test.go`: `TestWailsNoMatchNoEvidence`
      — no `wails.json`; `matched == false`, registry excludes Wails
- [ ] 16.4 RED `internal/preset/wails_test.go`:
      `TestWailsFallsBackToDocumentedDefaultWhenKeyAbsent` — `wails.json`
      present but declares no `wailsjsdir`; the contributed
      `ignorePatterns` fact is `wailsjs/**` (Root == Source fixture), and
      `Because` names both the absent key and the `frontend/` default
- [ ] 16.5 GREEN `internal/preset/wails.go`: read `wails.json` with
      `encoding/json` (plain JSON, not JSONC — Wails' own tooling writes
      it); on read/decode failure, fall back silently to the documented
      default and say so in evidence — a malformed file is the project's
      problem, never a failed `sync`
- [ ] 16.6 RED `internal/preset/wails_test.go`:
      `TestWailsReadsWailsJSDirWhenPresent` — `wails.json` declares
      `"wailsjsdir": "./frontend/src/lib"`; contributed pattern reflects it,
      `Because` names the key and its value, not the fallback
- [ ] 16.7 RED `internal/preset/wails_test.go`:
      `TestWailsPatternIsRelativeToSourceInASplitLayout` — Root != Source
      (Wails-shaped split, matching Slice 1's `generic-split` fixture
      shape), `wailsjsdir` absent; contributed pattern computes to
      `wailsjs/**` via `filepath.Rel(p.Source, filepath.Join(p.Root,
      wailsJSDir, "wailsjs"))`, not `frontend/wailsjs/**` — this is the
      motivating repository's own written-by-hand pattern, and the check
      that the derivation is right
- [ ] 16.8 GREEN `internal/preset/wails.go`: the `filepath.Rel` computation,
      slash-separated via `filepath.ToSlash`
- [ ] 16.9 RED `internal/preset/wails_test.go`: `TestWailsEvidenceValidates`
      — the Wails manifest passes `Manifest.Validate()` (schema, non-empty
      `Because`, no `boundaries` key, encodable `Value`) — the registry-wide
      `TestEveryFactCarriesEvidence` from Slice 2 must also cover this
      preset once registered (Phase 19)

#### Phase 17: Next.js and Expo — dependency-only presets (spec: `framework-presets` req "registry selects through one switch")

- [ ] 17.1 RED `internal/preset/nextjs_test.go`: `TestNextjsDetectsDependency`
      — a `package.json` fixture at Source declaring `next` in
      `dependencies`; `nextjs{}.Detect(p)` returns `matched == true`,
      evidence naming the `package.json` dependency; fails, `nextjs.go`
      does not exist
- [ ] 17.2 GREEN `internal/preset/nextjs.go`: `nextjs` type, Source scope,
      `Detect` checks `dependencies`/`devDependencies` for `next`; empty
      manifest unless slice review decides otherwise (design fixes only
      Wails' shape; Next.js/Expo are populated here — confirm against
      `exploration.md`/`design.md` whether either contributes a fact beyond
      matching. If neither design document specifies a Next.js/Expo
      manifest fact, ship both with an empty manifest and evidence-only
      `Detect`, and record that as this slice's own scoping decision in the
      PR description — do not invent a fact the design never asked for)
- [ ] 17.3 RED `internal/preset/nextjs_test.go`: `TestNextjsNoMatchWithoutDependency`
- [ ] 17.4 RED `internal/preset/expo_test.go`: `TestExpoDetectsDependency` —
      same shape for `expo` in `dependencies`
- [ ] 17.5 GREEN `internal/preset/expo.go`: `expo` type, Source scope,
      mirrors `nextjs.go`
- [ ] 17.6 RED `internal/preset/expo_test.go`: `TestExpoNoMatchWithoutDependency`

#### Phase 18: multi-preset composition, the real scenario (spec: `framework-presets` req "a Wails root with a Next.js source contributes from both presets")

- [ ] 18.1 RED `internal/preset/preset_test.go`:
      `TestWailsRootWithNextjsSourceContributesFromBoth` — a fixture with
      `wails.json` at Root and `next` declared at Source; `Resolve(p)`
      returns both matches, and the union'd manifest handed to
      `ownedFilesStep` carries both presets' keys, neither reported absent;
      fails until both preset packages are registered in the factory
- [ ] 18.2 GREEN `internal/preset/factory.go` (or wherever the design's "one
      switch in a factory" lives): register `wails`, `nextjs`, `expo`,
      `generic` in the real registry
- [ ] 18.3 RED `internal/setup/golden_test.go`: `TestFrameworkGoldens` cases
      `wails`, `nextjs`, `wails-nextjs` (per Decision 7's fixture list) —
      fail, fixtures do not exist yet
- [ ] 18.4 GREEN: capture the three framework goldens via
      `go test ./internal/setup -run TestFrameworkGoldens -update`; the
      commit message/PR description states which manifest fact each golden
      pins, per Decision 7's living-fixture rule

#### Phase 19: registry-wide contract tests over the real four presets

- [ ] 19.1 RED `internal/preset/preset_test.go`:
      `TestEveryFactCarriesEvidence` (registry-wide form, extending Slice
      2's stub-only version) — walks the real registry (`wails`, `nextjs`,
      `expo`, `generic`) against representative fixtures and asserts
      `Manifest.Validate()` on each match's manifest
- [ ] 19.2 GREEN: fix whatever 19.1 finds (expected: nothing, if Phases
      16–17 built evidence correctly)
- [ ] 19.3 RED `internal/setup/steps_test.go`:
      `TestWailsMatchedOwnedFileIsNoLongerEmpty` — the success criterion
      stated directly: a Wails-matched project's `.dharness/fallow.jsonc`
      after `Apply` contains the ignore pattern
- [ ] 19.4 RED `internal/setup/steps_test.go`:
      `TestIgnorePatternsCollidesInTheMotivatingShape` — a Wails-matched
      project whose own `.fallowrc.json` already declares `ignorePatterns`;
      the widened `boundariesOwnerStep` (Slice 3) is unsatisfied, names
      `ignorePatterns` with both values, and `Apply` still writes the
      preset's value into the owned file — the end-to-end proof of the
      proposal's "applied to the motivating repository" claim

#### Phase 20: documentation

- [ ] 20.1 `docs/learning-log.md`: one dated line for the region decision
      (Decision 8) if not already recorded in Slice 2, and one for the
      `entryPoints`-rejection precedent being reaffirmed here rather than
      re-litigated

#### Phase 21: Slice 5 verification

- [ ] 21.1 `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`
      clean
- [ ] 21.2 `go run ./tools/mutationstaged` over `internal/preset/wails.go`,
      `nextjs.go`, `expo.go`, `factory.go` — dry then real, floor 0.80
- [ ] 21.3 `bash scripts/verify-gate.sh`
- [ ] 21.4 Full success-criteria sweep against `proposal.md`'s checklist —
      every box traceable to a task above; report any box this task list
      does not cover rather than silently marking it done

---

## Traceability summary

| Spec requirement (capability) | Slice(s) |
|---|---|
| One package per framework, registry via one switch | 2 (skeleton, `generic`), 5 (real three) |
| `Detect` reports evidence, never a bare boolean | 2 (contract), 5 (real evidence) |
| Every manifest fact carries evidence naming an observable | 2 (`Validate`), 5 (real facts) |
| Preset fact is a default, never an assertion (§07) | 2 (inert-region property), 5 (Wails path-absent case) |
| Precedence fixed and total (project > detection > preset > global) | 2 (composition rule), 4 (detection rung for `folder-ownership`), 5 (project-declaration scenario) |
| `generic` reproduces today's behaviour exactly | 1 (golden pin), 2 (wiring), 6.10–6.11 (regression proof) |
| Multi-preset composition by scope | 2 (union/scalar rules, stubs), 5 (real Wails+Next.js scenario) |
| Multi-framework monorepos stay deferred | inherited unchanged — no task; `AmbiguousSourceError` untouched by this change |
| `ownedFilesStep.Apply` writes contributed keys | 2 (region machinery), 5 (real keys) |
| `declaresBoundaries` → `declaredKeys` | 3 |
| `boundariesOwnerStep` widens to N keys | 3 |
| dharness writes the contributed key anyway (no suppression) | 3 (guard test), 5 (real scenario) |
| Collision step disappears/reappears with nothing recorded | 3 |
| A code config describes and continues | 3 |
| `offByDefault` removed; `DefaultSeverity(p, rule)` | 4 |
| `folder-ownership` derived from barrel presence via git | 4 |
| Derived default is first-write only | 4 |

## Open decision points this task list surfaces rather than resolves silently

1. **The `dharness:presets` marker spelling** — fixed at Slice 2's review
   (Phase 6 introduction). Cannot change cheaply afterward.
2. **Marker re-insertion placement** — pinned by task 6.5 against a JSONC
   shape matching the wild, not just an empty file.
3. **The git-index-only barrel probe** — pinned by task 12.7 as deliberate,
   new threat-matrix row, not a bug to "fix" later.
4. **`.fallowrc.jsonc` widening (Decision 5)** — surfaced explicitly by task
   9.7–9.8 rather than folded silently into the `declaredKeys` diff.
5. **Next.js/Expo manifest content** — task 17.2 flags that the design fixes
   only Wails' shape; if neither document specifies a Next.js/Expo fact
   beyond matching, Slice 5 ships both with an empty manifest and records
   that scoping choice in its own PR rather than inventing a fact.
