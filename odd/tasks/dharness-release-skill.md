# dharness release skill

## Objective
Create a project-local Pi skill that guides safe, observable dharness releases and explicitly verifies the release-please-generated changelog.

## Problem and why
The repository has release automation but no repository-local skill. The available global skill targets another package and does not cover dharness's release-please workflow or changelog verification.

## Scope
- Add `skills/dharness-release/SKILL.md` as a tracked repository skill.
- Register the skill in `AGENTS.md`.
- Validate frontmatter, local references, the intended release checklist, and its tracked status.

## Constraints
- Release-please owns versioning, tags, GitHub release notes, and `CHANGELOG.md`.
- Never introduce manual changelog, tag, release, or local GoReleaser steps.
- Keep the skill concise and use only local repository references.
- Preserve unrelated working-tree changes.
- ODD TDD mode: not applicable; this is an instruction artifact with structural validation.

## Tasks
- [x] R1 Map the release workflow, configuration, CI gates, and failure boundaries.
  - Evidence: `.github/workflows/release-please.yml`, `release-please-config.json`, `.release-please-manifest.json`, `.goreleaser.yaml`, `AGENTS.md`, `CHANGELOG.md`, and `README.md` inspected; a read-only explorer independently mapped them.
- [x] R2 Create and register the repository-local release skill.
  - Evidence: `.pi/skills/dharness-release/SKILL.md` created and `AGENTS.md` registered it. The skill makes release-please authoritative and requires pre-merge and post-merge changelog checks.
- [x] R3 Validate the skill and report known remote-policy limits.
  - Evidence: the writer and independent verifier confirmed valid frontmatter, required section order, seven resolving tracked references, workflow alignment, and a clean whitespace diff. `gentle_review assess` returned `unassessable` because its native command emitted no output, so an independent read-only verifier was run and found no defects. Remote branch protection and live GitHub release state remain intentionally unverified.
- [x] R4 Require an up-to-date local checkout before release work.
  - Evidence: the skill now requires `git fetch origin main --tags`, a `HEAD`/`origin/main` equality check, and a safe-reconciliation stop gate. An independent verifier confirmed frontmatter, references, and whitespace checks remain valid.
- [x] R5 Move the skill out of ignored session state.
  - Evidence: `skills/dharness-release/SKILL.md` now contains the validated skill with rebased local references; `AGENTS.md` points to it; the ignored `.pi/` copy was removed without changing `.gitignore`.
- [ ] R6 Commit and push the isolated skill work unit.
  - Acceptance: stage only the tracked skill, its AGENTS.md registration, and the ODD task record; run the repository gate; create one conventional commit and push `main` without creating a release.

## Progress
- Current task: R6.
- Next step: stage the isolated work unit, run the repository gate, then commit and push it without starting a release.
