# Alternativas ligeras a Stryker para TypeScript, React, React Native y Expo

**Revisión técnica y recomendación de workflow — 9 de agosto de 2026**

## Veredicto ejecutivo

No existe hoy un reemplazo de StrykerJS que sea simultáneamente más ligero, maduro, profundo y validado para TypeScript, React, React Native y Expo.

La alternativa directa más prometedora es **Mutineer** (`@mutineerjs/mutineer`), sobre todo para TypeScript/React con Vitest. Implementa selección por archivos cambiados, pruebas importadoras, cobertura por prueba, límite de mutantes y workers persistentes. Sin embargo, sigue en `0.12.6`, tiene una comunidad diminuta, no publica un benchmark independiente y no documenta validación específica con `jest-expo`, React Native o Expo.

Para un workflow real, la mejor decisión no es reemplazar Stryker en el mismo lugar, sino cambiar la arquitectura del gate:

1. **Pre-commit (objetivo: ≤10 s):** lint/format/typecheck incremental + pruebas relacionadas. Sin mutation testing completo.
2. **Pre-push o pull request:** mutation testing limitado al diff, con presupuesto explícito de líneas y mutantes.
3. **Nightly o bajo demanda:** Stryker exhaustivo para conservar profundidad y detectar degradación global.

Si mutation testing debe permanecer obligatoriamente en `pre-commit`, la opción con mejor relación riesgo/señal es **Stryker limitado a los rangos exactos del staged diff**, con `perTest`, `incremental`, `ignoreStatic` y un subconjunto pequeño de mutadores. Para proyectos Vitest puede probarse Mutineer detrás de un rollout experimental, pero no sustituiría aún el gate estable.

## Alcance y método

Se evaluaron motores y capas de workflow capaces de trabajar con JavaScript/TypeScript o de ofrecer una señal equivalente sobre la calidad de las pruebas. Los criterios ponderados fueron:

- rendimiento y capacidad de limitar el trabajo al cambio;
- profundidad y precisión de mutaciones;
- compatibilidad con TS, JSX/TSX, React, Jest y Vitest;
- evidencia para React Native/Expo;
- madurez, mantenimiento y adopción;
- operabilidad en hooks, CI y monorepos.

Se priorizaron documentación oficial, repositorios mantenidos, registros de paquetes y publicaciones revisadas por pares. Los números promocionales se distinguen de las mediciones independientes. La prueba local incluida aquí es ilustrativa, reproducible y deliberadamente pequeña; no pretende generalizar el rendimiento de todos los repositorios.

## Comparación resumida

| Opción | Qué es | TS/TSX | React/Vitest | Jest | RN/Expo | Pre-commit | Estado | Decisión |
|---|---|---:|---:|---:|---:|---:|---|---|
| **Mutineer 0.12.6** | Motor alternativo AST/Babel | Alto | Alto | Declarado como first-class | Sin evidencia específica | **Prometedor con límites** | Pre-1.0; adopción mínima | Piloto para Vite/Vitest |
| **Tautest 1.10.1** | Capa diff/PR sobre Stryker | Alto | Soportado | Beta | No validado específicamente | Mejor para PR que commit | Joven; usa Stryker | Útil en PR, no reemplazo |
| **Stryker 9.6.1 afinado** | Baseline maduro | Alto | Oficial | Oficial | Viable vía Jest/jest-expo, con prueba propia | Solo con rangos staged | Maduro | Recomendación estable |
| **Mewt** | Motor multilenguaje en Rust | TS/TSX | Comando genérico | Comando genérico | No específico | **No** | WIP; sin Windows | No usar en hooks |
| **PMAT** | Toolkit Rust multilenguaje | Parcial/single-file | Jest/Vitest por comando | Sí, por comando | No específico | **No** | Limitaciones reconocidas | Descartar para este caso |
| **Mutode 1.4.2** | Motor JS histórico | JS; parser Babylon antiguo | No moderno | Comando genérico | No | No | Última publicación 2022 | Descartar |
| **LLMorpheus** | Prototipo de investigación | JS/TS | Usa Stryker modificado | Indirecto | No | No | Prototipo, requiere LLM | Investigación, no gate |

## La alternativa directa: Mutineer

Mutineer es la única alternativa encontrada cuyo diseño responde de forma explícita al problema de latencia. Su repositorio declara soporte para `.js`, `.jsx`, `.ts`, `.tsx`, Vitest y Jest. Entre sus mecanismos relevantes están:

- `--changed` y `--changed-with-imports`;
- ejecución de pruebas que importan el archivo mutado;
- `--only-covered-lines` y `--per-test-coverage`;
- `maxMutantsPerFile` como fusible de costo;
- filtrado TypeScript previo de mutantes que no compilan;
- shards y workers persistentes;
- conjunto de mutadores más pequeño que Stryker.

### Fortalezas

- Menor superficie conceptual y catálogo más selectivo.
- Buen ajuste con Vite/Vitest y TypeScript moderno.
- Controles de presupuesto pensados para feedback rápido.
- Licencia MIT y distribución npm normal.

### Riesgos

- Versión `0.12.6` y apenas tres estrellas observadas en GitHub al 9-08-2026.
- No hay benchmark independiente publicado; “fast” es una afirmación del proyecto.
- El catálogo todavía está marcado como WIP y es menos profundo que Stryker.
- Su modo `--changed` trabaja por archivo y ref Git, no por el contenido exacto del índice staged. Un archivo parcialmente staged puede producir trabajo y resultados sobre código que no entrará en el commit.
- No existe receta o fixture público para `jest-expo`, React Native, Expo Router o mocks de módulos nativos.

**Conclusión:** candidato serio para un piloto en React + Vitest, no para reemplazo organizacional inmediato ni para Expo sin una prueba de compatibilidad propia.

## Tautest: alternativa de workflow, no de motor

Tautest toma la idea industrial correcta —mutar solo el código cambiado— y la empaqueta alrededor de Stryker. Genera scope desde `git diff`, limita líneas, produce informes Markdown/JSON/HTML y conserva un archivo incremental. Vitest está soportado y Jest figura como beta; monorepos también están en beta.

Su defecto para esta decisión es estructural: instala y ejecuta Stryker. Reduce el trabajo, pero no elimina el runtime ni la complejidad del motor. Además está orientado a pull requests y su interfaz principal usa una referencia base (`--base origin/main`), no el staged diff exacto de un commit local.

**Conclusión:** recomendable para el gate de PR si se quiere una integración lista para usar; no resuelve por sí solo un pre-commit pesado.

## Herramientas que no superan el filtro

### Mewt

Mewt, de Trail of Bits, soporta JavaScript/TypeScript/JSX/TSX y reanuda campañas mediante SQLite. También omite mutantes menos severos cuando uno más severo de la misma línea sobrevive. No obstante, su propia documentación calcula el costo como suite × archivos × mutantes y recomienda campañas infrecuentes, incluso señala que mutation testing no es adecuado para CI en cada push. Windows no está soportado por sus binarios. Para una desarrolladora en Windows 11 y un hook de commit, queda descartado.

### PMAT

PMAT genera mutantes TypeScript rápidamente, pero su documentación reconoce que cada mutante reinicia el test runner (~1.8 s), no selecciona pruebas, opera secuencialmente y todavía limita el flujo TypeScript a un solo archivo. Su propia tabla estima 50 mutantes en ~90 s. Que el generador AST sea rápido no vuelve rápido al ciclo completo.

### Mutode

Mutode tiene mérito histórico y una publicación en ISSTA 2018, pero usa Babylon 6, exige Node 8+ y su última publicación npm fue en 2022. No ofrece soporte moderno demostrado para TypeScript, TSX, ESM, Vitest, Metro, React Native o Expo. Ser pequeño en disco no compensa el riesgo técnico.

### LLMorpheus

LLMorpheus puede producir mutantes parecidos a bugs reales que los operadores fijos de Stryker no generan. Es investigación valiosa, pero necesita un endpoint LLM y después entrega los mutantes a una versión modificada de Stryker. Sus mantenedores lo describen como prototipo de investigación sin soporte oficial. Aumenta costo y variabilidad; no sirve para hooks deterministas.

## Microbenchmark reproducible

**Entorno:** Node 24.14, Vitest 4.1.10, TypeScript 5.9, un archivo TypeScript de ~20 líneas, cinco pruebas, dos workers, dependencias ya instaladas. Tres ejecuciones calientes por caso; se reporta la mediana.

| Caso | Scope | Mutantes ejecutados | Mediana |
|---|---|---:|---:|
| Mutineer 0.12.6 | archivo completo, solo líneas cubiertas, cobertura por test | 10 (otros 10 descartados por typecheck) | **3.11 s** |
| Stryker 9.6.1 | archivo completo, cobertura `perTest` | 23 | **3.59 s** |
| Stryker 9.6.1 | solo líneas 8–9, `perTest`, `ignoreStatic` | 12 | **3.00 s** |

Mutineer redujo la latencia total frente a Stryker completo, pero también ejecutó menos de la mitad de mutantes y encontró dos supervivientes frente a tres de Stryker. Al limitar Stryker a las líneas modificadas, su mediana fue ligeramente menor que Mutineer. En este tamaño domina el costo fijo de inicialización; más workers incluso empeoraron Stryker (3.85 s con ocho workers en una ejecución observada).

**Lectura correcta:** Mutineer puede ahorrar tiempo por selección y menor catálogo; no hay evidencia de una mejora de orden de magnitud del motor. El gran multiplicador es reducir líneas, mutantes y pruebas, exactamente como concluye la literatura industrial.

## Qué dice la evidencia de escala

La experiencia de Google es la referencia más fuerte disponible. Su sistema no calcula mutation score para todo el repositorio ni muta cada línea. En cambio:

- muta únicamente líneas cambiadas y ya cubiertas;
- genera como máximo un mutante por línea;
- suprime nodos “áridos” o poco accionables;
- limita lo que muestra a siete mutantes por archivo;
- presenta supervivientes concretos durante code review.

En una evaluación con más de 24.000 desarrolladores y 1.000 proyectos, este enfoque produjo órdenes de magnitud menos mutantes y mejoró su accionabilidad. Una revisión sistemática de 175 estudios identifica selective mutation, análisis de flujo, higher-order mutation y técnicas evolutivas entre las familias principales para reducir costo.

La implicación práctica es directa: un hook rápido necesita **presupuesto**, no solo paralelismo.

## Recomendación por stack

### TypeScript + React + Vitest

**Piloto recomendado:** Mutineer con archivos cambiados, cobertura por test, solo líneas cubiertas, dos workers y máximo 8–12 mutantes por archivo. Ejecutarlo en pre-push o PR durante dos semanas y comparar supervivientes con Stryker sobre una muestra.

Para pre-commit, usar `vitest --changed` y reservar mutation testing solo para commits pequeños. Si el cambio supera el presupuesto, marcarlo como “mutation deferred” y exigir el gate en pre-push/PR; no contarlo como aprobado.

### React con Jest

Mantener Stryker o usar Tautest en PR. El runner oficial de Stryker activa `findRelatedTests` por defecto y soporta análisis `perTest`. Mutineer declara Jest como first-class, pero el camino tiene menos evidencia y merece piloto, no reemplazo.

### React Native y Expo

Expo recomienda Jest con `jest-expo`. Ninguna alternativa encontrada documenta una integración validada con Expo. El problema no es parsear TSX, sino conservar correctamente presets, transformaciones Babel/Metro, resolución de extensiones de plataforma y mocks nativos.

**Recomendación:**

- pre-commit: `jest --findRelatedTests` sobre archivos staged + lint/typecheck;
- pre-push/PR: Stryker/Jest limitado al diff o Tautest/Jest beta después de una prueba de compatibilidad;
- nightly: Stryker completo por paquete;
- no ejecutar builds nativos ni E2E dentro de mutation testing del hook.

### Turborepo/monorepo

Ejecutar por paquete afectado y con configuración local. Evitar un Stryker global desde la raíz. Tautest workspace sigue beta; Mutineer recomienda configuraciones por dominio. El gate debe resolver primero los paquetes afectados y después aplicar un presupuesto separado a cada uno.

## Arquitectura propuesta

### Gate 1 — `pre-commit`, duro y rápido

Objetivo: p95 menor a 10 s.

1. `lint-staged` para ESLint/format.
2. TypeScript incremental o por paquete afectado.
3. Vitest `--changed` o Jest `--findRelatedTests` con los archivos staged.
4. Mutation micro-gate opcional solo si hay ≤2 archivos de lógica y ≤20 líneas mutables.
5. Si se excede el presupuesto, registrar “deferred”; el gate de PR sigue siendo obligatorio.

### Gate 2 — `pre-push` o PR, mutation diff

- React/Vitest: Mutineer piloto o Tautest/Stryker.
- React Native/Expo: Stryker/Jest.
- Un mutante por línea como objetivo operativo; límite de 7–12 hallazgos por archivo.
- Fallar por supervivientes nuevos y accionables, no por un porcentaje global aislado.

### Gate 3 — nocturno o manual

- Stryker completo.
- TypeScript checker activo.
- Catálogo total de mutadores.
- Tendencia del score global y revisión de equivalentes.

## Configuración base si Stryker debe quedarse en pre-commit

```js
// stryker.commit.config.mjs
export default {
  testRunner: 'vitest', // 'jest' para Expo/RN
  coverageAnalysis: 'perTest',
  incremental: true,
  incrementalFile: '.cache/stryker/commit.json',
  ignoreStatic: true,
  concurrency: 2,
  reporters: ['clear-text'],
  thresholds: { high: 80, low: 60, break: null },
  // El wrapper debe inyectar --mutate file.ts:start-end desde git diff --cached.
};
```

Para Jest/Expo:

```js
export default {
  testRunner: 'jest',
  jest: {
    projectType: 'custom',
    configFile: 'jest.config.js',
    enableFindRelatedTests: true,
  },
  coverageAnalysis: 'perTest',
  incremental: true,
  ignoreStatic: true,
  concurrency: 2,
};
```

Hay que validar que el entorno personalizado de `jest-expo` siga funcionando, porque el runner de Stryker intercepta el entorno Jest para obtener cobertura por prueba. Si se usan docblocks `@jest-environment` o un runner no estándar, puede ser necesario un mixin o degradar a `coverageAnalysis: 'all'`.

## Riesgo de staged vs working tree

Mutation testers leen normalmente el working tree, mientras un commit puede contener solo parte del archivo. Un wrapper que extraiga rangos desde `git diff --cached` pero pruebe contenido unstaged puede aprobar o fallar por código que no se está cometiendo.

Soluciones aceptables:

- ejecutar dentro del manejo de archivos parcialmente staged de `lint-staged`;
- materializar el índice en un worktree temporal;
- o prohibir el micro-gate de mutación cuando detecte archivos parcialmente staged y diferirlo a pre-push.

Ignorar esta diferencia vuelve el hook rápido, pero no determinista respecto del commit real.

## Decisión final

1. **No reemplazar Stryker por Mutode, PMAT, Mewt o LLMorpheus.** Ninguno mejora el caso de pre-commit sin perder compatibilidad, profundidad o estabilidad.
2. **Hacer un piloto de Mutineer solo en paquetes React/Vitest.** Medir p50/p95, mutantes generados, supervivientes únicos y falsos accionables contra Stryker durante 2 semanas.
3. **Para React Native/Expo, conservar Stryker fuera del pre-commit.** Usar pruebas Jest relacionadas en el commit y mutación diff en PR.
4. **Si la política exige mutation testing en cada commit, implementar Stryker staged-line-scoped.** La evidencia y el microbenchmark indican que el scope aporta más que cambiar de motor.
5. **Mantener Stryker exhaustivo nocturno.** El micro-gate no debe convertirse en sustituto silencioso de la auditoría completa.

## Fuentes principales

- StrykerJS, configuración oficial: https://stryker-mutator.io/docs/stryker-js/configuration/
- StrykerJS, modo incremental: https://stryker-mutator.io/docs/stryker-js/incremental/
- StrykerJS, Jest runner: https://stryker-mutator.io/docs/stryker-js/jest-runner/
- StrykerJS, Vitest runner: https://stryker-mutator.io/docs/stryker-js/vitest-runner/
- Mutineer, repositorio y documentación: https://github.com/mutineerjs/mutineer
- Tautest, repositorio y documentación: https://github.com/canblmz1/tautest
- Mewt, repositorio y mecánica de campañas: https://github.com/trailofbits/mewt
- PMAT, documentación TypeScript mutation testing: https://github.com/paiml/paiml-mcp-agent-toolkit/blob/master/docs/features/TYPESCRIPT-MUTATION-TESTING.md
- Mutode, repositorio y publicación ISSTA: https://github.com/TheSoftwareDesignLab/mutode y https://dl.acm.org/doi/10.1145/3213846.3229504
- LLMorpheus, repositorio: https://github.com/neu-se/llmorpheus
- Expo, guía oficial de unit testing con Jest: https://docs.expo.dev/develop/unit-testing/
- Jest, opciones CLI: https://jestjs.io/docs/cli
- Vitest, opciones CLI: https://vitest.dev/guide/cli
- Petrović et al., *Practical Mutation Testing at Scale*: https://arxiv.org/abs/2102.11378
- Petrović et al., *Does mutation testing improve testing practices?*: https://arxiv.org/abs/2103.07189
- Google Testing Blog, *Mutation Testing*: https://testing.googleblog.com/2021/04/mutation-testing.html
- Pizzoleto et al., revisión sistemática sobre reducción de costo: https://www.albany.edu/faculty/offutt/research/papers/SLR-CostReductionMutation.pdf
- Mirshokraie et al., *Efficient JavaScript Mutation Testing*: https://people.ece.ubc.ca/amesbah/resources/papers/icst13.pdf
- Sánchez et al., *Mutation testing in the wild*: https://link.springer.com/article/10.1007/s10664-022-10177-8

## Limitaciones del reporte

- No se encontró un benchmark independiente que compare Mutineer, Stryker y Tautest sobre el mismo corpus React/React Native.
- La compatibilidad de Mutineer con Expo no está documentada; se clasificó como no validada, no como imposible.
- Estrellas, versiones y actividad son una fotografía del 9 de agosto de 2026.
- Los tiempos del microbenchmark dependen del hardware y del fixture; sirven para identificar el costo fijo y la diferencia de profundidad, no como promesa de rendimiento.
