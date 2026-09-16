---
name: dharness-release
description: "Trigger: dharness release, release-please release, changelog verification, release failure recovery. Guide safe dharness releases."
license: Apache-2.0
metadata:
  author: "Disble"
  version: "1.0"
---

## Activation Contract

Use for a dharness release, release-please PR, changelog check, or release-build recovery.

## Hard Rules

- Treat the referenced workflow, configuration, manifest, changelog, AGENTS.md, and CI as local sources of truth.
- Release-please owns version bumps, `CHANGELOG.md`, tags, GitHub releases, and release notes. Never manually substitute any of them or run `goreleaser release` locally.
- Merge a one-unit PR by squash or a multi-unit PR by rebase; never create a merge commit. After a rebase, repeat validation on the final SHA.
- Report observed remote policy only; do not infer branch protection or permissions.

## Decision Gates

| Condition | Action |
|---|---|
| No release-please PR | Stop; report that no release is pending. |
| `HEAD` differs from `origin/main` | Stop; safely reconcile the local checkout before release work. |
| Remote state is stale or unavailable | Stop; fetch or obtain current remote evidence. |
| Build fails after tag creation | Stop; require explicit human authorization before an Actions rerun. |
| Remote policy evidence is absent | Stop; obtain policy evidence before claiming release permission. |

## Execution Steps

1. Before starting release work from `main`, run `git fetch origin main --tags` and verify `HEAD` equals `origin/main`. If it differs, stop and safely reconcile the checkout; never treat its changelog or manifest as current remote state.
2. Read the local sources of truth and identify the release-please PR, proposed version, and `v` tag.
3. Enable the gate once per clone with `git config core.hooksPath .githooks`; run local checks: `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l .`, `ditto staged --dry --exclude-prefix tools/`, and `bash scripts/verify-gate.sh`. CI additionally runs `go test ./... -race`; do not claim local `-race` coverage.
4. Before merge, verify the generated `CHANGELOG.md` entry exists and matches the release-please PR, version, and tag. Use squash or rebase, then validate the final SHA if rebased.
5. After merge, verify the generated GitHub release, tag, and uploaded assets all align with the released version. Follow the decision gates for any mismatch or failure.

## Output Contract

Return the release PR, version, tag, final SHA, local and CI evidence, changelog comparison, release/tag/assets comparison, and any unresolved remote-policy evidence.

## References

- [release workflow](../../.github/workflows/release-please.yml)
- [CI](../../.github/workflows/ci.yml)
- [release-please config](../../release-please-config.json)
- [release manifest](../../.release-please-manifest.json)
- [GoReleaser config](../../.goreleaser.yaml)
- [repository rules](../../AGENTS.md)
- [generated changelog](../../CHANGELOG.md)
