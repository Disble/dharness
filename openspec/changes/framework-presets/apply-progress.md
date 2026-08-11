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
