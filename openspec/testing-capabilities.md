## Testing Capabilities

**Strict TDD Mode**: enabled
**Detected**: 2026-08-10

### Test Runner

- Command: `go test ./...` (local); `go test ./... -race` (CI)
- Framework: Go standard library `testing` package (no third-party test framework)

### Test Layers

| Layer       | Available | Tool        |
| ----------- | --------- | ----------- |
| Unit        | ✅        | `go test` (stdlib) |
| Integration | ⚠️        | `go test` with seamed process/git/cwd vars (`SetXForTest` pattern) — no separate integration framework |
| E2E         | ❌        | — |

### Coverage

- Available: ✅ (stdlib)
- Command: `go test ./... -cover`
- Threshold: none configured — repo relies on mutation proof (P09/L3) over a coverage percentage

### Quality Tools

| Tool         | Available | Command        |
| ------------ | --------- | -------------- |
| Linter       | ❌        | — (deviates: profile:go — not landed; no golangci-lint) |
| Type checker | n/a       | Go compiler is the type check |
| Formatter    | ✅        | `gofmt -l .` (check) / `gofmt -w .` (fix) |
| Vet          | ✅        | `go vet ./...` (this repo's whole L1) |
| Mutation     | ✅        | `go run ./tools/mutationstaged -dry` then `go run ./tools/mutationstaged` (staged-scope, ooze v0.2.0, dev-only dep) |
| Gate proof   | ✅        | `bash scripts/verify-gate.sh` (L5 — proves the local gate still refuses a broken file) |

### Local gate

`.githooks/pre-commit` — one entrypoint, runs gofmt+vet+test (no `-race` locally, deviates: P06). Enable per clone: `git config core.hooksPath .githooks`.

### CI

`.github/workflows/ci.yml` — matrix `[ubuntu-latest, windows-latest]`, runs gofmt check, `go vet`, `go test ./... -race`, then `bash scripts/verify-gate.sh`.
