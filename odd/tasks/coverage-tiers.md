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

### S3 — Deterministic per-feature classification
> en efecto, pero aquí lo importante es volverlo deterministica. Muchas veces, cada feature tiene su propio CORE, y de allí se bifurca. Necesito que evaluemos la mejor forma de escalarlo.

> Como hemos dicho muchas veces y es parte de la filosofía de dharness, nosotros s no creemos en que los LLMs vayan a seguir las reglas, creemos en los harness que se aseguran de forma deterministica que las cumplan.

### S4 — Progressive adoption includes old code within adopted units
> 2. espera, estoy cambiando de opinion, no retroactivo en el sentido de no obligar a clasificar de una sola todo el codigo anterior, pero si retroactivo al nivel de que codigo nuevo adoptado que incluye codigo viejo ahora debe arreglarse también, igual no puedes ser tan preciso en los tests y mutation tests.

### S5 — One resolver behind existing workflows, not a new runner wrapper
> 3. Ok, pero trata de integrarlo en los workflows actuales, ese comando puede funcionar desde atras.

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

- [ ] T1 — Manifest and single native resolver; S1,S3,S4,S5,S6,S10; route: inline sequential, public JSON/CLI checks; status: in_progress; commit: pending.
- [ ] T2 — Tier-aware mutation scope in existing workflows, preserving current verdict and unadopted behavior; S1,S3,S4,S5,S6,S10; route: inline sequential, isolated invocation checks; status: pending; commit: pending.
- [ ] T3 — Native Vitest coverage integration and actual gate wiring; S1,S2,S3,S4,S5,S9,S10; route: inline sequential, independent verification for setup/invocation risk; status: pending; commit: pending.
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
