# sync: ESLint wiring stays pending without an installed ESLint

Objective: apply design principle 22 to `eslintExtendsStep`. Branch `fix/sync-eslint-runner-pending` (from `main` + `docs/readme-sync-replaces-init`). Delivery: one PR, ask-on-risk. Runner: `go test ./...`.

## Specs

- S1. Principle 22, as approved: "Si falta, el paso queda pendiente: lo dice, nombra el comando que la instala y no se da por cumplido. dharness no la instala por su cuenta, porque la versión y el gestor de paquetes son del proyecto. Tampoco detiene la corrida: los demás pasos siguen, como manda el 20."
- S2. With no local ESLint binary, `eslintExtendsStep` delegates (never "Already in place", never Apply) and its reason names the project's own install command for `eslint`. Same shape as `coverageTiersStep` without Vitest: dharness writes nothing for that step until the runner exists. (Assumption: writing nothing, rather than wiring and staying pending, is the consistent reading of "dos criterios para la misma situación".)
- S3. With ESLint installed, behavior is unchanged: splice, byte convergence and the load probe stay as they are.
- S4. The other steps still run (§20): install, owned files, gate wiring and the rest are unaffected.
- S5. Deliverable is output: build the binary outside the repo and run `sync` in a scratch project with and without ESLint; read the output (CLAUDE.md).

## Tasks

| ID | Specs | Route | Status | Evidence |
| --- | --- | --- | --- | --- |
| T1 | S1-S4 | inline | done | RED `TestEslintExtendsStepDelegatesUntilTheProjectInstallsESLint` (ok=false) → GREEN; gofmt/vet/`go test ./...` green; commit `aefeaa3` |
| T2 | S5 | inline | done | scratch binary run, see L4 |

## Log

- L1 (2026-10-16): "En efecto, hágalo como una ODD." — approving: "El cambio va en `eslintExtendsStep`: que no se dé por cumplido si falta el binario de ESLint, y que su descripción nombre el comando de instalación. Así coverage tiers y ESLint siguen el mismo criterio."
- L2: Prior comment in `eslintdiagnostics.go` deliberately kept silence on missing ESLint ("noise on every sync ... for a state dharness deliberately does not change"); principle 22 supersedes that reasoning, so the comment is rewritten with the change.
- L3: Implementation: runner check first in `eslintExtendsStep.Delegated`; 6 tests that exercised the wiring without ESLint now install it via `installESLintForTest`; `TestEslintSatisfiedStaysTrueWithoutESLint` inverted into `TestEslintStepStaysPendingWithoutESLint` (still asserts no probe starts). Seven goldens regenerated (generic pair through a temporary in-package helper, removed; framework via `-update`): each changes only step 7 to delegated and drops the `eslint.config.js` bytes. `openspec/specs/setup/spec.md`: new principle-22 requirement; "independent questions" requirement rewritten.
- L4: Scratch (npm, fresh git repo, outside the worktree, removed after): no ESLint → step 7 delegated, names `npm install --save-dev eslint`, no `eslint.config.js`, 5 applied/5 delegated, exit 0. After `npm i -D eslint`, next sync writes `eslint.config.js`. Differential with ESLint installed: base `main` binary vs this change → stdout identical except npm's own timing line, trees identical (S3).
- L5: Out of scope, reported not fixed: the human renderer (`report.wrap`, `strings.Fields`) folds a delegated reason's indented command block into the paragraph ("…things: npm install --save-dev eslint dharness never installs…"). Pre-existing; the coverage-tiers Vitest reason has the same shape.
