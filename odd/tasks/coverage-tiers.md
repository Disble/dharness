# Coverage tiers: progressive adoption and deterministic enforcement
Approved ODD: four sequential review units on `worktree/coverage-politic`; implementation authorized, no publishing.
Specification: read through `## Tasks` before writing; `## Log` records decisions and observed evidence, not additional requirements.

## Specs

### S1 — Preserve the consumer-owned business tiers
> No me gusta. Prefiero la que te pasé. Aquí lo importante es qué defines como Core. Tu has definido tu core para tu caso imaginario, otros lo definiran de otra forma. Pero la tabla es bastante buena por si misma, incluso con tus casos extremos.

Approved visual policy: CORE 100%, IMPORTANT 80%, INFRASTRUCTURE 0%; business classification belongs to the consumer. The original screenshot and accepted example are reproduced in `.pi/artifacts/coverage-tiers-ejemplo-de-una-feature.html`.

### S2 — Native test coverage, with the marked example preserved
> ciertamente, esto debería ser para todos los tests a partir a ahora, no?

The user marked this native Vitest example as **keep**:
```ts
test: {
  coverage: {
    include: ['src/features/**/*.{ts,tsx}'],
    thresholds: {
      'src/features/checkout/calculate-total.ts': {
        statements: 100, branches: 100,
        functions: 100, lines: 100, perFile: true,
      },
      'src/features/checkout/validate-order.ts': {
        statements: 100, branches: 100,
        functions: 100, lines: 100, perFile: true,
      },
      'src/features/checkout/ui/Checkout.tsx': {
        statements: 80, branches: 80,
        functions: 80, lines: 80, perFile: true,
      },
      'src/features/checkout/types.ts': {
        statements: 0, branches: 0,
        functions: 0, lines: 0, perFile: true,
      },
    },
  },
}
```

User-selected execution timing:
> Solo gate y CI completos (recomendado)

Coverage is mandatory in the existing complete native verification workflow/local gate, not in ordinary focused/watch runs. Existing CI may consume that same native configuration; this does not authorize creating or managing CI workflows. The complete adopted unit still uses 100/80/0; focused results do not establish whole-unit coverage.

### S3 — Deterministic per-feature classification
> en efecto, pero aquí lo importante es volverlo deterministica. Muchas veces, cada feature tiene su propio CORE, y de allí se bifurca. Necesito que evaluemos la mejor forma de escalarlo.

> Como hemos dicho muchas veces y es parte de la filosofía de dharness, nosotros s no creemos en que los LLMs vayan a seguir las reglas, creemos en los harness que se aseguran de forma deterministica que las cumplan.

### S4 — Progressive adoption includes old code within adopted units
> 2. espera, estoy cambiando de opinion, no retroactivo en el sentido de no obligar a clasificar de una sola todo el codigo anterior, pero si retroactivo al nivel de que codigo nuevo adoptado que incluye codigo viejo ahora debe arreglarse también, igual no puedes ser tan preciso en los tests y mutation tests.

### S5 — One resolver behind existing workflows, not a new runner wrapper
> 3. Ok, pero trata de integrarlo en los workflows actuales, ese comando puede funcionar desde atras.

Scope clarification accepted by reference in L45: integrate native coverage with existing project configuration and local workflows only. No CI workflow generation, provider selection, installation service, or fourth wrapped CLI.

### S6 — Consumer-owned classification risk; equivalence machinery excluded
> 5. eso es un riesgo aceptado, cada proyecto debe gestionar ese riesgo.

> yes, de momento esta fuera de scope. Ok, con eso creo que terminamos de reredefinir el artifact. Estamos listos para planificar el o los ODDs?

The second quote answers the explicit question whether the new equivalence/evidence system is outside the first delivery. It does not relax the existing mutation verdict.

### S7 — Token-efficient agent guidance
> esa es una opción o la otra es que crees directamente la skill, sería un skill para claude y la otra skill para agents y eso lo enlaces simplemente al sistema prompt, es por un tema de optimización de tokens

> Adicional, también los agentes deben conocer cómo está funcionando todo esto. Así que creo que actualizar el agent/claude.md debería ser también un punto clave.

### S8 — Self-contained skill distribution
> Ok, pero no vamos a meter nada al repositorio de Bercel porque nosotros no. No, no, eso es mucho trabajo. Esto es trabajo independiente, entonces mírale cómo le hacemos para hacerlo desde acá

> me gusta, que quede detallado en aratifact

This approved the preceding proposal: one source embedded with Go's stdlib `go:embed`, managed local copies for agents/Claude, and minimal managed instruction blocks. Artifact sections 6–11 retain that proposal and its examples.

### S9 — Vitest first; Jest in backlog
> ok, primero vitest, jest queda para el backlog.

### S10 — Execution authorization
> formalizar el plan ODD y ejecutar las cuatro unidades

## Tasks

- [x] T1 — Manifest and single native resolver; S1,S3,S4,S5,S6,S10; route: inline sequential, public JSON/CLI checks; status: done; commit: parser c2ac282; resolver b86b56f; facade cddacb7.
- [x] T2 — Tier-aware mutation scope in existing workflows, preserving current verdict and unadopted behavior; S1,S3,S4,S5,S6,S10; route: parent-owned native predicate, disjoint workflow writers, sequential proof and independent admission; status: done; commit: plain/shared54f17db; native regression379369d; staged7867858.
- [ ] T3 — Native Vitest coverage integration and actual gate wiring; S1,S2,S3,S4,S5,S9,S10; route: scope audit/correction, native config/export and existing-workflow integration, independent verification; status: in_progress; commit: pending.
- [ ] T4 — Embedded skill and bounded AGENTS/CLAUDE integration; S3,S7,S8,S9,S10; route: inline sequential, independent verification for installer risk; status: pending; commit: pending.

## Log

### L1 — Original request
> C:\Users\User\AppData\Local\Temp\pi-clipboard-fd251c75-3dac-4e71-9fb5-a0cfdab57b89.png, quiero implementar esta nueva politica, qué propones para desplegarno en nuestro dharness a los diferentes proyectos que lo consumen?

### L2 — Approved artifact and subsequent corrections
The accepted explanatory artifact is `.pi/artifacts/coverage-tiers-ejemplo-de-una-feature.html`, sections 12–13. S1–S10 preserve the relevant exact user statements above. The user's latest authorization is S10 and latest runner decision is S9. Earlier suggestions to exempt old source by commit age, create a new coverage runner wrapper, or add equivalence evidence machinery are superseded, not requirements.

### L3 — Exploration and baseline
- Initial branch: `worktree/coverage-politic`; initial HEAD: `2745361`; initial tracked/untracked status: clean (artifact directory is ignored).
- Baseline observed GREEN, background job `job-1`, exit 0: `go test ./... && go vet ./... && go build ./... && formatting=$(gofmt -l .) && printf '%s' "$formatting" && test -z "$formatting"`.
- Existing seams: `internal/app.RunArgs`, `internal/cli.RunMutate`, index snapshot/staged scope, Stryker MSP membership, native setup `Writer` and rollback. `internal/jsconfig` is ESLint-specific; do not assume it parses Vitest configuration semantics.
- No new wrapped product CLI or dependency; source execution and all tool arguments remain at their existing boundaries. Build and run observable-output probes in isolated fixtures, never run the dharness binary against this repository.
- Native review switch is declared off; functional checks and risk-based independent verification remain required. Do not change the switch.

### L4 — Derived implementation boundaries
These are implementation choices within the accepted artifact, not additional user requirements:
- Root consumer `coverage-tiers.json`: versioned features, adopted/enforced path prefixes, default tier and exact/prefix overrides; strict validation and deterministic precedence. Outside declared adoption retain current behavior; never silently exempt old files within an adopted unit.
- One native resolver emits canonical machine-readable results. The integration command is a behind-workflow facade, not another user step or runner wrapper.
- Test public seams: resolver/CLI output, existing mutation invocation/report, native Vitest behavior and `sync`-produced files/observable report. Fixtures use real temporary trees, including preservation checks for rejected inputs.
- Complete adopted-unit coverage is distinct from staged mutation maintenance. Do not claim a staged run proves the entire feature.
- T3 supports Vitest only. Jest coverage support is backlog; existing Jest workflows must not regress.
- T4 uses one canonical embedded source, copies for selected clients, and replaces only managed document regions. Existing custom content and conflicting destinations must not be silently overwritten.
- Each task carries tests and docs and closes with work-unit commit evidence after checks. Split review units rather than compressing source/tests to meet the review budget. No push, PR, merge or release authorized.

### L5 — Progress
Plan formalized before any product write. T1 is in progress. Observed RED for missing resolver API, then GREEN for complete checkout adoption and unadopted legacy. Added invalid-declaration and non-business-source scenarios: observed RED, then GREEN (`go test ./internal/tiers -count=1`, exit 0). `mutation-tdd` skill is unavailable at the configured authorized path; repository mutation protocol remains required. Native facade, active-manifest guard, manual/systematic mutation, functional readback, full checks and task commits are pending. T2–T4 have not started.

### L6 — T1 evidence and execution support decision
- Planning/backlog commit: `582c1232c14e006369a85ba268b9fcf4ffb023c5`; its hook correctly reported no staged Go files.
- Facade RED: `policy` was an unknown command; then app/tiers GREEN after dispatch and read-only JSON implementation. Active integration missing-manifest guard separately observed RED then GREEN.
- `go test ./internal/tiers ./internal/app -count=1` completed with exit 0 (job-2). The native test built the real binary outside this repository into its own `t.TempDir`, ran it only in a disposable consumer, and verified exact JSON, empty success stderr, native error stderr/exit 1, and unchanged rejected input.
- Manual mutation: removed `m.Version != 1`; source diff and mutation marker proved the change; owning `/version` scenario failed (exit 1). Guard restored, owning scenario passed (exit 0). Systematic staged mutation remains pending.
- Original independent verifier `mv755puw-1-ls1k` performed static review and established no severe candidate-caused defect, but executed none of the six requested commands because its tool surface lacks native background execution. No passing independent verdict is claimed.
- User selected **Ejecutar mediante el host** (`host_execution_support`): host will run those six original commands once through its native background tool, capture separate streams/exit codes under `.pi/artifacts/coverage-tiers-t1-verification/`, then return evidence to the same verifier. This is supported execution for the existing verification, not an omitted check or unsupported shell wrapper.
- Resumed repository state confirmed: branch `worktree/coverage-politic`, HEAD `582c123`, expected T1 draft changes only. Product commits, systematic mutation and T2–T4 remain pending.

### L7 — Independent T1 verification admitted
Host job-4 executed the six original commands once, exit 0 in 31 seconds, with separate streams/statuses in the authorized in-project evidence folder. Existing verifier continuation `mv7cbhew-2-lla5` read the whole updated task document and all 24 evidence files, repeated no commands, and returned PASS for the requested T1 verification scope: S1,S3,S4,S5(T1),S6,S10; S9 respected. All tests passed; stderr empty; vet/build/gofmt/diff-check streams empty. Native binary execution confirmed in non-short app suites. No severe candidate-caused defect established. This does not close T1: staged mutation and product commits remain pending. Parser-only staged mutation preflight started as job-5.

### L8 — Systematic mutation failure; no completed T1 claim
- Preflight job-5 passed (exit 0), selecting only `internal/tiers/manifest.go`; 264 staged review lines including its tests/docs.
- Systematic command `ditto staged --exclude-prefix tools/` (job-6) failed, exit 1 after 14m30s. Baseline 20.326s; 53 generated, 3 noncompiling/excluded, 50 valid, 34 killed, 16 survived; score 0.68 versus unchanged minimum 1.00.
- Surviving guard families: canonical root/absolute/Windows paths, separately empty prefixes/features, exact-file membership, multiple-prefix accumulation, reverse-parent overlap and prefix-validation execution. Sort comparator equality also needs semantic triage; no equivalent waiver is asserted.
- Manual version guard was killed and restored, but never replaces systematic proof. Previous independent functional/static PASS does not close this missing proof.
- T1 is blocked for the user's decision about correcting the missing proof. No product commit; staged parser/test/docs and other T1 drafts preserved. T2–T4 not implemented. No threshold weakened or tests skipped.
- T3 read-only scout assumptions corrected: existing `grammars.TypescriptLanguage()` was observed via `go doc` (no new dependency), and runtime policy resolution is not a wrapper that launches Vitest. No T3 code written.

### L9 — Proof-correction authorization
> sí, señor

User authorizes the recommended correction of the failed parser proof without lowering the minimum or adding exceptions. Reconciled HEAD `582c123`, existing staged parser/test/docs and other T1 drafts unchanged. T1 reopened to in_progress; T2–T4 remain pending. Strengthen the owning declaration/path/adoption scenarios; simplify genuinely redundant implementation predicates rather than writing mutant-specific artificial cases. New candidate checks pending.

### L10 — Owning scenarios strengthened and refactor checked
Acceptance/rejection scenarios now exercise exact-file scopes, multiple prefixes, identity ordering, separately empty adoption arrays, canonical invalid root/Windows/absolute/backslash/noncanonical paths at first and later prefix positions, and reverse-parent overlap. Focused scenarios GREEN. Manual prefix-validation removal (break before validation) was proved in source diff and failed the owning invalid-prefix scenarios (exit 1); restored. Removed the redundant empty-path term already rejected by path.Clean; stdlib slices.SortFunc/strings.Compare replaces the custom comparator while preserving tested ordering. `go test ./internal/tiers -count=1` GREEN after refactor; staged parser/test/docs 314 review lines. No exclusions, waivers, threshold changes or new dependencies. Corrected full-candidate checks and systematic mutation pending.

### L11 — Corrected candidate full checks
Host job-7 executed the six standard checks on the corrected candidate, exit 0 in 20 seconds. Raw streams/statuses are under `.pi/artifacts/coverage-tiers-t1-corrected-verification/`, slots 01–06 (targeted app/tiers tests, full suite, vet, build, gofmt list, diff check). Each command exit 0; formatting list empty. New systematic staged mutation started as job-8; still pending. Independent corrected-candidate verdict and product commits remain pending.

### L12 — Corrected candidate independent verification
Verifier continuation `mv7dt3er-4-fuse` read the whole document through L11, corrected parser/tests and 24 host raw evidence files; repeated no commands. PASS for corrected T1 scope (S1,S3,S4,S5(T1),S6,S10); all six checks exit 0, stderr/formatting empty, native binary fixture executed. Removed empty predicate confirmed redundant, stdlib comparator preserves identity ordering and duplicate rejection; no weakening or severe regression established. Job-8 mutation remains pending and is not passing evidence. No product commit or T1 completion claimed.

### L13 — Corrected parser systematic proof passed
Job-8 `ditto staged --exclude-prefix tools/` completed exit 0 in 14m42s: 48 generated, 3 noncompiling/excluded, 45 valid, 45 killed, 0 survived; score 1.00 at unchanged minimum 1.00. Manual prefix-validation guard was killed/restored; corrected functional/structural checks and independent verdict PASS. Parser slice is eligible for its gated work-unit commit. Whole T1 still needs resolver/facade slices and their systematic proof/commits; T2–T4 remain pending. No exemptions, waivers or weakened targets introduced.

### L14 — Proven parser work-unit commit
Commit `c2ac282` — `feat(policy): validate consumer-owned coverage tier declarations` — contains parser, owning tests and documentation (314 review lines). Job-9 exit 0 in 21 seconds; pre-commit explicitly passed gofmt/vet/tests against the staged tree. Parser proof correction is complete: 45/45 valid mutants killed, no survivors, independent corrected-candidate verification PASS. Whole T1 remains open for resolver/facade systematic proof and their commits; T2–T4 remain pending. No push, PR or publication.

### L15 — Resume remaining T1 slices
> continua

Reconciled physical HEAD c2ac282 and expected resolver/facade drafts; parser proof/commit retained. Continue T1 before T2–T4. `docs/mutation-testing.md` explicitly allows selecting the owning package with `--test-command`; resolver mutations will run the complete non-short `internal/tiers` suite at unchanged minimum 1.00 rather than the unrelated whole repository per mutant. Full suite and native consumer probes remain separate mandatory checks; no scenario or threshold is omitted.

### L16 — Resolver proof is incomplete
- Existing resolver whole-package test `go test ./internal/tiers -count=1` and dry-run passed; staged slice `tiers.go`/`tiers_test.go` is 240 review lines. Product files unchanged during this resume; facade remains a draft.
- Job-10 `ditto staged --exclude-prefix tools/ --test-command 'go test -count=1 ./internal/tiers'` failed exit 1 after 35s. Unmutated baseline 1.192s; 32 valid, 26 killed, 6 survived; score 0.81 at unchanged minimum 1.00. No automatic rerun or passing resolver claim.
- Raw survivor evidence: `tiers.go:89` removing .spec exclusion; `:102` continue-to-break when searching features; `:104` initial longest 0-to-minus1/plus1; `:106` greater-to-greater-or-equal override length; `:120` less-to-less-or-equal sorting. Two missing observable scenario families (spec tests and later-feature membership); four boundary/comparator variants need semantic simplification/triage, never a new waiver or suppression.
- Proposed correction: strengthen existing non-business-source/adoption scenarios, then simplify redundant ranking/ordering implementation without changing exact/specific/default precedence. No threshold change, exclusion or equivalence machinery. New candidate needs its own checks if correction authorized.
- T1 blocked for user's proof-correction decision; parser c2ac282 remains proven. Resolver commit, facade completion, fresh whole-candidate checks and independent admission pending. Manual resolver guard proof not performed; no claim it was. T2–T4 remain pending. No push/PR/publication.

### L17 — In-scope proof corrections authorized
> autorizo, y de una vez las que sigan

User authorizes correcting resolver proof and later in-scope proof gaps, without another per-survivor permission prompt. T1 reopened; no change to S1–S10, minimum 1.00, exclusions, waived verdicts, dependency boundary or publication authority. Product/scope decisions and destructive/publishing actions still require human approval. Strengthen owning scenarios and simplify genuinely redundant predicates; any unavailable/exhausted proof remains reportable, never a passing claim.

### L18 — Corrected resolver scenarios and ranking
Strengthened existing adoption scenario with a later feature and overlapping adopted prefixes (unique output), and existing exclusion scenario with .spec, managed/git folders and non-source text. GREEN. Temporarily removed .spec exclusion and changed feature-search continue to break; source diff/markers present, both owning scenarios RED exit 1; restored. Reverse stdlib path order of validated overrides replaces artificial longest baseline; first match implements exact/most-specific precedence. Stdlib file ordering replaces bool comparator. Full tiers package GREEN after refactor, diff-check PASS; corrected resolver/test slice 257 review lines staged. New systematic mutation job-11 pending; whole-candidate/native checks, independent corrected verdict and resolver commit pending.

### L19 — Corrected resolver systematic proof passed
Job-11 completed exit 0 in 30 seconds: 26 valid, 26 killed, zero survivors, score 1.00 at unchanged minimum 1.00. Prior job-10 remains failed historical evidence, not erased. Manual spec-exclusion/later-feature mutations were killed and restored (L18). Corrected whole-candidate/native six-check execution started as job-12 with independent raw streams under `.pi/artifacts/coverage-tiers-t1-resolver-verification/`. Independent admission and resolver commit remain pending; whole T1 still open.

### L20 — Resolver slice verified and committed
Job-12 all six checks exit 0 (12s), raw streams under the L19 folder. Verifier `mv7g8s95-5-nnva` read whole doc and all 24 host files, duplicated no commands; S1,S3,S4,S5(slice),S6,S10 PASS; ancestor-chain reasoning for reverse lexical specificity confirmed, no severe bug established. Commit `b86b56f` — `feat(policy): resolve complete adopted source classifications` — job-13 exit 0 in 22s, 257 review lines; index gate explicitly passed gofmt/vet/tests. Facade remains open; public global-help requirement observed RED, integration-only helper text added. New facade checks/mutation and its commit pending; T2–T4 remain pending.

### L21 — Facade owning scenarios and public help
Strengthened existing root/help scenario with positional rejection and flag parse errors; added public output-failure scenario preserving consumer tree. Global help scenario RED (missing helper description), then integration-only help text added and focused public scenarios GREEN. Native binary scenario extended to execute global help and inspect its actual stdout/stderr/exit in disposable consumer. Manually removed required-root guard and swallowed JSON write error; visible source diff/markers, both owning scenarios RED exit 1; guards restored. Restored full app suite/native probe and stage dry-run started as job-14; facade systematic proof, full checks/independent admission and commit remain pending. No T1 completion claim.

### L22 — Facade native/green and systematic proof
Job-14 exit 0 in 5s: complete app tests including non-short actual binary JSON/error/help in disposable consumer PASS, diff-check and staged dry-run PASS, facade slice 219 review lines. Job-15 owning complete app suite mutation exit 0 in 50s: 10 valid/10 killed/0 survived, score 1.00 at unchanged minimum 1.00 (app dispatch 2, CLI facade 8; help has no generated mutants, native output assertion supplies its proof). Full six checks, independent admission and facade commit remain pending. T1 is not done. Read-only T2 routing map started; no T2 implementation or T3/T4 work.

### L23 — Facade full checks and independent verdict
Job-16 six checks exit 0 in 12s; raw streams in `.pi/artifacts/coverage-tiers-t1-facade-verification/`. Verifier `mv7gm6y4-7-axez` read whole doc, five facade files, existing entry tests and all24hostfiles; duplicated no commands. S1,S3,S4,S5(T1),S6,S10 PASS; no severe candidate bug; nativeconsumer JSON/help/error/preservation and existingcommand behavior checked. Systematic10/10 and restoredmanualroot/outputkills recorded, not re-executed by reviewer. Gated facade commit is last T1 blocker. T2 read-only map identifies snapshot-root resolution after Snapshot (before technical classification/MSP), not working tree; filters must preserve unadopted paths, unlike scout shorthand requested-intersect-adopted. Technical/MSP owners remain unchanged; no final selfcheck may reintroduce excluded infrastructure. All-infrastructure scope is a no-op with no runner (consistent with excluded scope); do not invoke Stryker just to produce zero mutants.

### L24 — T1 complete; T2 implementation starts
Facade commit `cddacb7` — `feat(policy): expose read-only native coverage integration helper` — job-17 exit 0 in 21s, index gate explicitly passed gofmt/vet/tests;219reviewlines. T1 complete only, with parser c2ac282 and resolver b86b56f. Per-slice systematic45/45,26/26,10/10killed (minimum1.00 unchanged), independent judgments PASS, manual guards restored, actual nativeconsumer output checked. T2 starts with one parent-owned shared tier filter behind existing workflows, then staged/non-staged integration as two independent substantial disjoint writers; commits/proofs remain sequential. T3/T4 pending; no publication.

### L25 — Shared native mutation eligibility
Added public `Resolution.AllowsMutation(path)`: delegates to native Tier, only declared infrastructure false; CORE/IMPORTANT/unadopted and inactive policy stay eligible. Public resolved-policy scenario RED (method missing), then GREEN. Guard inverted manually with source diff/marker, owning scenario RED exit1; restored. Whole owning tier suite/dry/systematic execution job-18 pending; no integration/commit claim. Two substantial workflow integration units will use disjoint writers: non-staged `mutate.go`/tests plus shared CLI path-projection helper `filterInfrastructureScopes(root,source,scopes)`, staged `mutate_staged.go`/tests consuming that helper after index Snapshot. Helper returns original ranges narrowed only via native AllowsMutation; root-relative canonical projection; no new JSclassifier or scorethreshold. Writers never stage/commit or edit task/memory projection; parent admits proof/commits sequentially.


### L26 — T2 writers admitted for proof, not completion
Native eligibility job-18 passed: one valid mutant killed, no survivors at minimum 1.00. Plain/shared worker mv7h5upl-8-14lh and staged worker mv7h5uvp-9-7702 returned implementations and public RED/GREEN reports; neither closes T2. Parent read the helper and scenarios, confirmed original range preservation, legacy eligibility, snapshot policy resolution before classification, and final self-check scopes derived from filtered scopes. Native ASSESS attempted once: unassessable because untracked paths were not declared; independent high-risk verification remains required, no retry or RDD switch change. Current index holds the plain/shared/native slice (363 review lines); staged integration/tests remain outside that slice.

### L27 — Resume and visible manual proof
> Continua

Parent bypassed the helper's tier eligibility guard with a visible source diff against its staged baseline. Job-19 owning mixed-scope scenario failed exit 1: infrastructure reached --mutate; original column ranges, important and legacy paths remained observable. Guard restored exactly; job-20 runs complete cli/tiers suites then systematic staged mutation at unchanged minimum 1.00. No passing systematic claim until its exit is observed. Native mixed-consumer probe, full checks, independent admission and gated commits remain pending; T3/T4 not implemented.


### L28 — Plain/shared systematic proof and native probe
Job-20 complete cli/tiers suites and systematic index run passed exit 0 in 3m7s: 10 valid/10 killed/0 survived, minimum 1.00 unchanged (plain flow 4, helper 5, native predicate 1). Job-21 native probe failed an overly broad test assertion: the emitted --mutate correctly excluded infrastructure, but the preserved cumulative report note named it outside the requested scope. No product defect established. Corrected owning probe asserts exact forwarded scopes separately from that note and rejected-input preservation; job-22 PASS exit 0 in 5s. It builds/runs the actual binary outside this checkout in a disposable consumer, with native process fixtures for manager/Stryker, not the real mutation engine. Public mutation help observed RED (missing tier boundary), then text and docs updated; final candidate mutation job-23 and six raw host checks job-24 pending. Native probe regression will travel in its own bounded verification commit; plain/shared behavior slice keeps its owning tests. Staged behavior/proof remains a separate later slice; its current 407 review lines are reported, not compressed to fit 400.


### L29 — Plain candidate full checks passed
Host job-24 completed exit 0 in 1m8s. All six original checks passed with separate raw commands/stdout/stderr/exitcodes in `.pi/artifacts/coverage-tiers-t2-plain-verification/` (01–06): complete cli/tiers/app suites, full suite, vet, build, empty gofmt list, diff check. The non-short app suite includes the actual mixed-consumer native binary probe. Independent admission starts only now; final mutation job-23 still pending, so no plain commit or T2 completion yet. New help was tested through the public seam and is covered by these GREEN suites.


### L30 — Final plain mutation passed
Job-23 final candidate (including public help regression and workflow docs) passed exit 0 in 2m48s: 10 valid/10 killed/0 survived, minimum 1.00 unchanged. No new generated mutants from the help string; its public owning RED/GREEN and actual binary help checks provide behavioral proof. Six full checks job-24 PASS. Bounded independent plain/shared functional admission mv7jbmp4-a-bgbi is running; its prompt was issued before job-23 completion, so parent admits this final mutation separately. No commit yet, staged workflow proof and T2 completion still pending.


### L31 — Evidence boundary correction and staged manual check
Correction to L30: native policy/global help checks belong to T1; the new mutation tier-help text has public `RunMutate(--help)` GREEN evidence, not yet native `mutate --help` evidence. Independent plain verifier read all24 host files and found no severe bounded candidate defect; final result pending. Parent saved its discovery/session handoff because no memory tool was exposed in the child. Staged no-op guard bypass was visible in source diff/marker; job-25 owning all-infrastructure scenario RED exit1 (no-op message missing and classification phase entered), then guard restored. Restored staged owning scenarios job-26 pending. Neither observation is staged systematic proof or a T2 completion claim. T3 scout returned a read-only map; assumptions about wrapping resolved initializers, requiring a TS product exception and leaving coverage wiring manual remain rejected: raw export wrapping, existing TS grammar and actual existing gate/CI integration remain the approved boundaries, not new product decisions.


### L32 — Proven plain work-unit commit; native regression guard killed
Independent verifier mv7jbmp4-a-bgbi final bounded functional/static PASS for S1,S3,S4,S5,S6,S10, all24 job24 raw files read, no duplicate commands or severe candidate defect. Job-27 committed `54f17dbad732e09edb6b2210359e68a1c80447d6` — `feat(mutate): exclude declared infrastructure from named mutation scopes` — exit0 in42s; index gate explicitly passed gofmt/vet/tests. Plain unit is388 review lines (387 insertions,1 deletion); staged paragraph excluded from its docs until that behavior's own commit. T2 remains open.
Native regression now also checks actual `mutate --help` and separate eligible Survived/NoCoverage exit1 stderr; job-28 GREEN exit0. Parent temporarily bypassed tier eligibility with visible source diff; owning actual-binary scenario job-29 RED exit1 (native all-infra request invoked Stryker, wrote state and returned survivor refusal instead of no-op). Guard restored exactly. Job-30 restored native GREEN plus gated regression/why-log commit pending. No upstream real-engine claim; this unit adds tests only and uses the already proven product10/10, no test-file mutation scope asserted. Staged job-26 owning scenarios GREEN exit0 in6s; systematic proof and independent admission remain pending.


### L33 — Native regression committed; staged final candidate checks started
Job-30 restored native scenario GREEN and index gate PASS, exit0 in47s. Commit `379369da6d34dcba73ec6161f6db6c7d04989353` — `test(mutate): verify native tier scope and refusal boundaries` — 171 review lines (170 native test, one append-only why-log line). New native mutation help evidence now exists, correcting the L31 pending boundary; eligible Survived and NoCoverage each produce exact exit1 stderr without a success verdict. Temporary helper mutation restored byte-identical to54f17db; no product alteration. Staged source/tests and its workflow-doc paragraph now staged as one self-contained candidate (over400 reported, not code-golfed). Systematic job-31 runs the complete non-short cli suite at unchanged minimum1.00. Host six checks job-32 pending with separate raw streams in `.pi/artifacts/coverage-tiers-t2-staged-verification/`. Independent staged admission and its gate/commit still pending; T2 open, T3/T4 not implemented.


### L34 — Staged full checks passed
Host job-32 passed all six checks exit0 in47s, raw24files in the L33 folder: full non-short cli/tiers/app suites, full repository suite, vet, build, empty formatting list and diff check. Final staged candidate is409 insertions (391 owning scenario/fixture lines,16 integration lines,2 workflow-doc lines): 9-line overage is explicit, not reduced by compressing tests. Independent staged admission begins now; systematic job-31 still pending. Native mixed/no-op/refusal process regression is committed379369d for the named-path flow; staged owning scenarios drive RunMutate over real git index/snapshot fixtures with runner process seams. Keep these evidence boundaries distinct; no real upstream Stryker engine claim. No staged commit/T2 completion until final proof and admission are observed.


### L35 — Independent staged delivery blocker and one correction batch
Verifier mv7jzxye-c-pgzl returned S1,S3,S4,S5,S6,S10 PASS within static/public RunMutate + real-index/process-seam scope; all24 raw job32files read, no duplicated commands or severe candidate defect. One delivery-evidence blocker remains: actual-binary staged no-op/index-vs-working policy was not executed by the named-path regression. Parent applies the L17-authorized correction to the existing native fixture: baseline commit in disposable consumer, stage only infrastructure, leave an unstaged manifest lifting the exclusion, run actual binary --staged, assert native no-op/classifier not started/no fixture process/no mutation state and working-manifest preservation. No product logic change, real engine claim or new requirement. Host job-33 targeted native correction with separate raw streams/status pending in `.pi/artifacts/coverage-tiers-t2-staged-native-correction/`; then one bounded recheck of this blocker, staged gate/commit and T2 closure. T3 API research confirms native onCoverage report-file-presence hook is available, but its actual throw/failure behavior still requires native evidence before implementation; no T3/T4 writes.


### L36 — Corrected staged native output and guard proof
Job-33 actual-binary native correction passed exit0 in6s; raw4files in L35 folder. The consumer has a committed infrastructure policy, only its infrastructure source staged, and an unstaged working manifest changing that path to core. Actual --staged still prints the no-op, keeps classify/MSP/related/Stryker unstarted, writes no mutation state and preserves the working manifest. Native scope/refusal/help checks still pass in the same scenario. Parent bypassed the staged no-op guard with visible diff/marker; job-34 owning native scenario REDexit1 (native code0 but missing no-op/classify not started), then restored the guard byte-identical to the already mutation-proven index. No permanent product-code correction. Native regression changes staged alongside staged behavior. Fresh corrected final-candidate six checks job-35 pending in `.pi/artifacts/coverage-tiers-t2-staged-corrected-verification/`; same verifier then performs one recheck limited to L35 blocker. No staged commit or T2 completion claim yet.


### L37 — Corrected staged candidate full checks passed
Job-35 corrected final candidate six host checks passed exit0 in34s; raw24files in L36 folder, non-short app/native probe included, formatting list empty. Final staged work unit is428 insertions across four files (409 original plus19 native regression additions); 28-line overage reported explicitly, not compressed. Production `mutate_staged.go` restored byte-identical to job31 mutation-proven index; no new product mutant scope, earlier4/4 applies to unchanged production code, and added native regression strengthens it. Same verifier receives one bounded recheck for the sole L35 output-evidence blocker, no fresh sweep or duplicate commands. Staged gate/commit and T2 closure still pending.


### L38 — Resume corrected staged closure
> Continua

Reconciled current HEAD379369d and exactly the four staged corrected-candidate files (428insertions); only task-log drift outside index, source guards restored and diffcheck clean. Corrected full job35 alreadyPASS (not pending). Same verifier continuation mv85jwhj-1-jgbm runs one limited recheck of L35's native staged output/index-policy gap using4job33rawfiles and24correctedjob35files, no new sweep/reruns. Staged gate/commit and T2 closure are next if that recheck passes; T3/T4 remain unimplemented. No publication.


### L39 — Limited staged recheck passed; gated commit started
Continuation mv85jwhj-1-jgbm PASS for the sole L35 native staged/index-policy output gap: whole task throughL37, corrected fixture, all4job33files and24job35files read, no reruns/writes or fresh sweep; no previous blocker remains. OriginalS1,S3,S4,S5,S6,S10 admission remains; unchanged-production4/4 and manualnativeguardRED/restoration parent-admitted. New host job-1 (log under gentle-jobs-pTvRaH, not historical job-1 under4RUhVa) starts the428-line staged work-unit commit with index gate. Commit identity/exit not observed yet; T2 remainsin_progress until gate/commit completes. T3/T4 untouched.


### L40 — T2 completed after final index gate
New hostjob1pTvRaH completed exit0 in35s, index gate explicitly passed gofmt/vet/tests. Commit `7867858a2478dc5bc6111dd4c4fc8c9f4619061a` — `feat(mutate): apply index coverage tiers to staged mutation scopes` — fourfiles428insertions, reported28-line overage. Together with plain/shared54f17db and native regression379369d closes T2 only:10/10 and4/4 validmutants killed atunchanged minimum1.00; manualguards killed/restored; actualbinary named/staged/help/no-op/refusal/preservation checks; fullsixchecksPASS; independent bounded admissions includingone corrected-gap recheckPASS. Prior failures retained as evidence, no skipped/waived passing claim, no realupstreamengineclaim. RDDoff, no push/PR/release.
T3 starts exploration/decision, no source writes yet. Named product uncertainty: whether native coverage also runs automatically on focused/watch Vitest invocations, or only on complete gate/CI runs while local focused tests remain coverage-free. Both retain100/80/0 and complete adopted-unit enforcement; clarify execution timing before altering existing test workflows. T4 pending.


### L41 — T3 awaits one product timing decision
Before T3 source writes, parent asks whether coverage is mandatory only in full native gate/CI runs (recommended, focused/watch tests stay fast) or also on every focused/watch Vitest invocation. This does not change100/80/0, classification authority, complete adoption or mandatory actual gate/CI wiring. T3 blocked awaiting that human decision; T4pending. T2done and committed, no running jobs/subagents remain.


### L42 — T3 execution timing selected
> Solo gate y CI completos (recomendado)

User selected full gate/CI only. S2 adds this exact choice; existing quotes/100/80/0/complete-unit adoption unchanged. T3 unblocked, focused/watch runs remain coverage-free unless explicitly requested; actual complete native gate/CI invocation remains required. T4pending; no T3source writes yet.


### L43 — Disjoint T3 foundations launched; CI target needs selection
Writer trigger: two independent substantial foundations, disjoint allowed surfaces, launched together in background. Parser worker mv86ix7f-2-mzm3 owns only internal/jsconfig/vitest_export.go/_test.go and jsconfig.go; raw export spans, TS grammar and binding preservation. Native data adapter worker mv86ix9t-3-susi owns only internal/setup/coverage_tiers_runtime.go/_test.go and assets/coverage-tiers/runtime.mjs; one Go resolver, native metrics/report-file presence, no coverage launcher/classifier/dependency/download or forced focused/watch coverage. Parent owns transactional consumer splicing, gate/CI wiring, full/native proof and commits. No T3 completion claim.
Spot-check confirms setup currently owns lefthook/husky wiring but no consumer CI workflow generator (`internal/setup/setup.go:Plan`, `steps.go:gateConfig`, `agentSkillStep.Delegated`); consumer CI platform was not selected by S1-S10/artifact. Before parent writes CI integration, ask one target-scope choice for the first delivery. Independent foundations do not depend on that choice and may finish; do not assume universal GitHub/GitLab coverage or silently modify unknown workflows.


### L44 — User questions CI scope; T3 paused
> Espera un momento, en qué momento hablamos de CIs?

The CI reference originated in the parent's proposal/artifact (section13/unit3: connection with gate and CI) and derived T3 wording, not an explicit user request to create/manage CI workflows. Selecting coverage timing in L42 did not authorize generating new pipelines or choosing a CI provider. The parent's platform-selection question was an unjustified scope expansion; no CI workflow file has been written. Pause T3 source work pending clarification, preserve existing foundations without admission or commits. Both workers are complete: parser mv86ix7f-2-mzm3 and runtime adapter mv86ix9t-3-susi returned bounded RED/GREEN reports; native Vitest/gate integration, parent proof, mutation and independent admission remain pending. Post-writer ASSESS called once for this new T3 candidate: unassessable due untracked files, RDDoff, treat high requiring later independent verification; no retry or authority change. T1/T2done, T4pending.
Worker disclosure requiring separate incident follow-up before resuming: parser worker reported an accidental empty `/tmp/probe2_test.go` outside allowed surfaces and one unauthorized sibling-package build. Parent has neither inspected nor removed that reported file and has not authorized extra outside-root work.


### L45 — User authorizes scope audit, correction and resumption
> Siguiendo este objetivo y alcance, revisa que no nos hayamos salidod el alcance, corrige si lo hicimos en alguna parte, y luego continua con lo planificado

User resumes only the original four-unit coverage policy scope as restated: 100/80/0 ordinary coverage, consumer-owned deterministic classification, progressive complete-unit adoption, native Vitest first/Jest coverage backlog, existing workflows, embedded skill/client instructions. No CI pipeline creation/management or provider selection. S2/S5 clarify the parent's earlier erroneous CI-generation inference while retaining exact quotes and history. T1/T2 stay closed unless the audit produces an actual in-scope defect; do not invalidate their proof by assumption. T3 in_progress for audit/correction then existing-workflow integration and actual native proof. T4 pending until T3 closes.
Separate tooling incident disposition: parser worker disclosed accidental zero-byte `/tmp/probe2_test.go` creation and extra sibling build during AST probing, outside allowed surfaces/commands. Diagnosis is based on that recorded worker report, not a new outside read: a shell exploration write bypassed the declared edit surface. No product behavior or CI workflow was changed by that reported file. Preserve this evidence; do not inspect/delete an external target without fresh explicit per-target authorization. Future handoffs prohibit non-fixture outside writes and sibling checks. This external cleanup is not bundled into product implementation or claimed complete.
Audit compares original specs against committed T1/T2, unadmitted T3 foundations and current docs/artifact. One bounded read-only map, parent spot-check, then correct confirmed deviations; no speculative expansion or repeated verification ceremony. Large T3 foundation workload (2074 newlines) will be admitted in bounded review units, never compressed by deleting tests/comments.


### L46 — Scope audit admitted narrowly; native prerequisite unavailable
Read-only scope map mv8durxk-4-zy4g found no product-scope excess in T1/T2 or unadmitted T3 foundations: Go owns classifications/numbers, native adapter projects data and checks report presence without calculating metrics or matching globs; no new CLI wrapper/dependency/waiver/suppression/CIgenerator. This is scope mapping only, not functional admission. Parent spot-check caught an inaccurate audit citation: docs/coverage-tiers.md:44-58 did not yet contain the claimed explicit CI boundary. Corrected that prose and the artifact's existing-flow diagram/unit3 to rule out new CI workflow management; preserved the marked Vitest example. Corrected stale artifact claims that policy facade/task document/tests did not exist. Historical log preserved.
Real runner availability check once: `bunx --no-install vitest --version` exit0 reports vitest/5.0.3 win32-x64 node-v24.21.0. Official Context7 docs consulted for native Reporter/configureVitest APIs and Bun offline semantics; research is not runtime proof. Newhostjob2pTvRaH prepared only a disposable consumer `/tmp/tmp.KseyIwqdur` with test-only vitest5.0.3 and @vitest/coverage-v8 5.0.3, then `bun install --offline` failed exit1: `no cached manifest for "@vitest/coverage-v8"`; resolved/downloaded/extracted0. No repository dependency change, runtime download or retry. Provider prerequisite missing: actual Vitest coverage/gate proof unavailable until human provides an existing environment or explicitly authorizes isolated fixture dependency provisioning. T3/T4 remain incomplete; no further product integration is written before that decision. Finish only the passive scope-correction work unit, then block T3 on this external proof prerequisite.
