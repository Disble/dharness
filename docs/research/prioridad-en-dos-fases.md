# Prioridad de Stryker en dos fases

**Propuesta de diseño — 7 de octubre de 2026. Sin implementar.**

## Veredicto

Stryker arranca a prioridad normal y baja a `BELOW_NORMAL` en cuanto escribe
`onDryRunCompleted.json`. El dry run corre en igualdad con la máquina y los
mutantes siguen cediendo, como hoy. Las dos piezas existen y están verificadas
por separado; la combinación dentro de dharness no.

## El problema

La prioridad baja (`runner.go:44-52`) protege la máquina durante la fase de
mutantes, que satura la CPU con `concurrency` workers. Pero se aplica al proceso
entero, y el dry run es otra cosa: un solo test runner
(`3-dry-run-executor.js:44`, `schedule(of(0), …)`) con vitest en `maxWorkers: 1`
(`vitest-test-runner.js`). Ocupa más o menos un núcleo. A prioridad baja no
compite con el usuario: pierde contra todo lo demás, y vitest corta cada test a
los 5 s de reloj.

autoreas-bridge lo observó el 7 de octubre: tests jsdom de 2.8 s llegaron a
5.5–6.3 s a prioridad baja, y `There were failed tests in the initial test run`
bloqueó el commit. A prioridad normal, el mismo conjunto pasa 388/388.

Pedirle al proyecto que suba su `testTimeout` no es una salida: es adaptar su
suite a una decisión nuestra. Quitar la prioridad baja tampoco: §14 salió de que
el pre-commit congelaba el equipo.

## Lo que se midió

i7-12700K (20 hilos lógicos, núcleos híbridos), Windows 11. Las herramientas del
experimento vivieron en el scratchpad de la sesión y no están en el repo.

**Un techo de CPU no reemplaza la prioridad baja (descartado).** Un Job object
con `JOBOBJECT_CPU_RATE_CONTROL` en modo hard cap. Una sonda mide lo que tarda
una tarea fija mientras 19 hilos saturan la CPU:

| Modo | p50 | p95 |
|---|---|---|
| Ociosa | 38 ms | 42 ms |
| Prioridad normal | 186 ms | 431 ms |
| **Prioridad baja** | **37 ms** | **39 ms** |
| Techo 75 % | 43 ms | 427 ms |
| Techo 50 % | 38 ms | 325 ms |

Windows aplica el techo por ventanas de tiempo. Entre una y otra, el proceso
satura a prioridad normal, así que los picos son los mismos que sin techo. La
prioridad baja deja la máquina igual que ociosa: la decisión original era
correcta para esta fase.

**La prioridad baja puede congelar el dry run, no solo hacerlo más lento.** Un
worker de vitest con cuatro tests de 1.7–2.7 s:
- Con 12 hilos de carga de fondo, un test quedó parado **~299 s**, toda la vida
  del generador de carga, en 2 de 2 corridas, aunque había núcleos libres.
- Con 20 hilos de carga, la prioridad baja fue rápida (2.8 s), también 2 de 2.

Es reproducible pero no monótono, y no está explicado. Puede ser el planificador
híbrido o el aparcado de núcleos; no está probado.

**Las dos piezas del diseño, verificadas:**

1. **La señal.** El reporter `event-recorder` viene con Stryker 10.0.0. Escribe
   `00000-onDryRunCompleted.json`, luego `onMutationTestingPlanReady` y luego los
   `onMutantTested`. En una corrida real, el archivo del dry run apareció ~200 ms
   antes del primer mutante.
   - Se activa solo por la línea de comandos: `--reporters clear-text,json,event-recorder`.
   - El directorio por defecto es `reports/mutation/events`, relativo al cwd de Stryker.
   - `--eventReporter.baseDir` **no** se acepta como flag (`unknown option`); solo
     se puede fijar en el archivo de configuración.
2. **El cambio en vuelo.**
   - **Windows:** `SetInformationJobObject` con `JOB_OBJECT_LIMIT_PRIORITY_CLASS`
     sobre un Job ya en marcha también alcanza a los procesos existentes: un
     nieto creado antes del cambio pasó de `Normal` a `BelowNormal`, y la sonda
     bajó de p95 460 ms a 55 ms.
   - **Linux (WSL2, kernel 6.6):** `setpriority(PRIO_PGRP, …)` alcanza a hijos y
     nietos que ya existen (`ni` de 0 a 10 en todo el grupo).
   - **macOS:** sin verificar.

## El diseño

**Señal (§09: la señal directa existe).** dharness agrega `event-recorder` a la
lista `--reporters` que ya pasa en todas las rutas (`tool.go:352-355`) y espera
`*-onDryRunCompleted.json`. No se lee prosa: es un archivo que Stryker escribe
por sí mismo.

**Dónde mira:**
- **`--staged`:** el config generado (`writeStagedStrykerConfig`,
  `mutate_staged.go:535-556`) fija `eventReporter.baseDir` en un directorio
  temporal de la corrida, igual que hoy con `jsonReporter.fileName`.
- **Ruta interactiva:** no hay config generado, así que queda el directorio por
  defecto, `reports/mutation/events`, junto al `mutation.json` que Stryker ya
  escribe ahí. dharness lo borra antes de lanzar: Stryker también lo limpia,
  pero un archivo viejo leído antes de esa limpieza dispararía el cambio antes de
  tiempo.

**Runner.** `runner.Command` gana un campo, el directorio de eventos. Mientras
el proceso corre, una goroutine, hermana del watcher de `Context`
(`runner.go:185-207`), revisa el directorio cada ~250 ms. Al encontrar el
archivo baja la prioridad del árbol una sola vez y termina.

| | Hoy | Propuesto |
|---|---|---|
| Windows | `BELOW_NORMAL` como flag de creación; `runner.Run` no tiene Job | `Run` crea el Job (la maquinaria de `managed_windows.go`) y aplica `JOB_OBJECT_LIMIT_PRIORITY_CLASS` al ver la señal |
| Unix | `setpriority(PRIO_PROCESS, pid)` justo después de arrancar | `Setpgid` al arrancar, SIGINT/SIGTERM reenviados a `-pgid` mientras corre, y `setpriority(PRIO_PGRP, pgid)` al ver la señal |

**Por ruta:**
- `mutate --dry-run` (`--dryRunOnly`) corre entero a prioridad normal: es todo
  dry run.
- `mutate <ruta…>` y `mutate --staged` usan las dos fases.
- `stryker serve` (discovery) queda como está.

**Si la señal nunca llega** (el dry run falla, o Stryker sale antes), el proceso
termina a prioridad normal y no hay nada que bajar. El veredicto sigue saliendo
del exit code y del JSON, nunca de la señal.

## Prueba de sobre-ingeniería

Agrega un campo a `runner.Command` y una goroutine, y no borra nada. Se justifica
solo porque las alternativas sin estado nuevo están medidas y descartadas:
- Prioridad normal entera rompe §14.
- Prioridad baja entera congela el dry run.
- El techo de CPU no protege la máquina.
- Reintentar, limitar workers o un estado "inconcluso" se descartaron en el
  triage del 7 de octubre.

## Riesgos y abiertos

1. **`Setpgid` cambia Ctrl-C en Unix: medido, y se resuelve reenviando.** Un
   arnés imita a dharness → Stryker → dos workers (WSL2, procesos Go, que mueren
   con SIGINT como node). Ctrl-C se simula como lo hace la terminal: SIGINT al
   grupo del trabajo en primer plano.

   | Modo | Después de Ctrl-C |
   |---|---|
   | Hoy (mismo grupo) | 0 vivos |
   | `Setpgid` sin más | **3 vivos**: Stryker y sus workers, huérfanos (padre pasa a init) |
   | `Setpgid` + reenviar SIGINT/SIGTERM a `-pgid` | 0 vivos; el hijo sale por `signal: interrupt` |
   | Lo mismo con `PRIO_PGRP` aplicado antes | 0 vivos; los workers ya existentes tenían `ni` 10 |

   Consecuencias para el diseño:
   - El reenvío va en el runner, no en la CLI: `mutate <ruta…>` no instala
     `NotifyContext` (solo `--staged` lo hace, `mutate_staged.go:73`), y sin
     reenvío dejaría Stryker corriendo tras un Ctrl-C.
   - Como el hijo sigue muriendo por SIGINT, `endedByConsoleInterrupt`
     (`exec_other.go`) mantiene su lectura.
   - El grupo también mejora la cancelación por `Context`: `kill(-pgid)` alcanza a
     los nietos, que hoy sobreviven (`runner.go:191-195` lo documenta).
   - Lo que no cubre: un SIGKILL a dharness no se reenvía. Hoy tampoco llega a los
     hijos, así que no es una regresión.
   - En Windows no hay riesgo equivalente: el evento de consola llega a todos los
     procesos de la consola, con Job o sin él. La carrera entre `Start` y la
     asignación al Job ya está resuelta en `managed_windows.go:17-19`
     (`CREATE_SUSPENDED`), y `Run` puede reusarla.
2. **macOS sin verificar.** `PRIO_PGRP` es POSIX y debería comportarse igual.
3. **§05 en tensión, antes de este cambio.** El principio dice no pisar los
   `--reporters` del proyecto, y `tool.go:352-355` ya los pisa en todas las rutas.
   Agregar `event-recorder` profundiza esa contradicción: hay que resolverla
   aparte, no esconderla aquí.
4. **Tamaño de `onDryRunCompleted.json`.** Lleva los resultados de todos los
   tests; en suites grandes pueden ser varios MB. No se midió.
5. **Tests que fijan la prioridad baja y cambian de forma:**
   `check_test.go:357-359` y `:1239`, `stryker_command_test.go:98`.
6. **Antigüedad de `event-recorder`.** Verificado en Stryker 10.0.0. dharness
   pide `@latest`, así que alcanza, pero conviene un test que lo detecte si
   Stryker lo retira.
7. **El congelamiento de ~299 s sigue sin explicar.** El diseño lo evita en el
   dry run, pero la fase de mutantes sigue a prioridad baja. Allí un timeout es
   una detección para Stryker, no un fallo, así que el costo sería tiempo, no un
   veredicto falso.

## Siguiente paso

Implementar detrás de un test de extremo a extremo con el binario: un proyecto
scratch, carga de fondo y un test de ~3 s, observando que el dry run pase y que
la máquina responda durante los mutantes. Es la misma sonda de este documento.
El riesgo 1 quedó medido (arriba): el reenvío de señales es parte del cambio.
