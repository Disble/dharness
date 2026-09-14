# Probe evidence: H-MSP0b, J2, H-TK

Recorded 2026-09-14 during the `mutate-staged-v1.9` planning phase, in scratch
fixtures outside this repository. These three measurements were the blocking
prerequisites named by `docs/handoff-mutate-staged-v1.9.md` §6 and by the
proposal's risk table. Each entry states the pre-registered prediction, the
pre-registered kill condition, the exact protocol, the observed result, and the
decision the result forces.

Host: Windows 11 (10.0.26200), Git Bash, Node 22.19.0, npm 10.9.3, Go 1.27.0.
Fixtures: `D:\dev\disble\dharness-probes\{h-msp0b,j2-jest,h-tk}`.
Raw logs and fixture sources: `D:\dev\disble\dharness-handoff-v1.9\probes-v1.9\`.

---

## H-MSP0b — an npm `.cmd` Stryker shim keeps MSP stdout frame-clean

**Prediction.** An npm `.cmd` shim keeps stdout frame-clean: only
`Content-Length`-framed JSON-RPC, logs on stderr, no CRLF corruption.

**Kill condition.** Any stray stdout byte or broken frame → spawn
`node node_modules/@stryker-mutator/core/bin/stryker.js` directly for MSP.

**Fixture.** Stryker 9.6.1, `@stryker-mutator/vitest-runner` 9.6.1, vitest
4.1.11, TypeScript 6.0.2, `mutate: ["src/logic.ts", "src/noop.ts"]`.
Installed with `--legacy-peer-deps` (see *Environment notes*).

**Protocol.** A Go client (`frame_probe.go`) parses raw stdout bytes strictly:
every byte before a `Content-Length:` header is stray, and every header must be
exactly `Content-Length: <n>`. Two spawn shapes were measured, because the
fallback decision is about how dharness spawns the server:

- A — `exec.Command("<fixture>/node_modules/.bin/stryker.cmd", "serve", "stdio")`,
  the shape `project.LocalBinary` + `tool` produce on Windows.
- B — `cmd.exe /d /s /c "<that .cmd> serve stdio"`, the handoff's pre-registered shape.

Both then ran `configure` (options travel in the configured JSON; `serve stdio`
accepts no extra CLI arguments) followed by six `discover` calls.

**Observed (both shapes identical).**

    A .cmd direct      frames=8  stray=0  malformed=0  -> PASS (frame-clean)
    B cmd.exe wrapper  frames=8  stray=0  malformed=0  -> PASS (frame-clean)
    first stderr line: "Stryker server started on stdio channel"
    stderr bytes=980, stdout carried only the 8 framed responses

**Verdict.** Prediction holds. **Kill condition not met: no fallback.** The
`.cmd` shim is usable for MSP, so dharness does not need a direct-Node
invocation path for the mutation server on Windows.

**Two facts discovered while framing, both load-bearing for the design.**

1. `discover` omits a file entirely from `result.files` rather than reporting it
   with zero mutants. The three distinguishable observables are: a file with
   mutants in the requested ranges, a file present in the set with no mutant in
   those ranges, and a file absent from `result.files` altogether.
2. **Both-form omission is genuinely ambiguous, and it is now measured rather
   than asserted.** With `mutate` naming both files, `stryker run --dryRunOnly`
   reported `Found 2 of 11 file(s) to be mutated` and
   `Instrumented 2 source file(s) with 14 mutant(s)` — so `src/noop.ts` is inside
   the configured set. Yet ranged *and* path-only discovery both returned
   `{} (no file reported)` for it, byte-identical to `src/logic.test.ts`, which is
   outside the configured set. A file excluded by configuration and a file in the
   set that generates no mutant anywhere are therefore the same observation.
   (`dryrun-confirm.log` carries the independent confirmation.)

   Consequence: an outcome line that claims a configured exclusion for the
   both-omission case would overclaim. The specification keeps the
   owner-approved acceptance string `outside Stryker's mutate set: <path>` while
   forbidding output that explains the omission as a configured exclusion or as
   an intrinsically zero-mutant file. That pairing is intentional, and this
   measurement is why the second half exists.

---

## J2 — `jest --findRelatedTests --listTests --json`

**Prediction.** `jest --findRelatedTests <file> --listTests --json` prints a JSON
array of test paths (`[]` for a file no test imports), exits 0, and executes
nothing.

**Kill condition.** It runs tests or emits non-JSON → use the
`jest --json --findRelatedTests … --passWithNoTests` run and read `numTotalTests`.

**Fixture.** jest 30.3.0 with `@stryker-mutator/core` 9.6.1 and
`@stryker-mutator/jest-runner` 9.6.1, CommonJS. `src/a.js` (runtime code imported
by `src/a.test.js`) and `src/no-related.js` (runtime code no test imports).

**Instrument with teeth.** `src/a.test.js` writes `executed.marker` when it runs,
so "nothing executed" is measured rather than assumed. The kill-condition
fallback deliberately runs the same suite so the marker fires, proving the
instrument can detect execution.

**Observed** (`j2-run.log`).

| command | exit | stdout | executed.marker |
|---|---|---|---|
| `--findRelatedTests src/a.js --listTests --json` | 0 | `["D:\\…\\src\\a.test.js"]` | absent |
| `--findRelatedTests src/no-related.js --listTests --json` | 0 | `[]` | absent |
| `--findRelatedTests src/a.js src/no-related.js --listTests --json` | 0 | `["D:\\…\\src\\a.test.js"]` | absent |
| fallback `--json --findRelatedTests src/no-related.js --passWithNoTests` | 0 | `numTotalTests: 0` | absent |
| fallback `--json --findRelatedTests src/a.js --passWithNoTests` | 0 | `numTotalTests: 1` | **PRESENT** |

The last row is the control: the marker appears exactly when the suite runs, so
the four preceding absences are evidence and not silence.

**Verdict.** Prediction holds. **Kill condition not met: the list-only form is
confirmed**, so the specification's Jest branch is
`jest --findRelatedTests <files> --listTests --json` reading the JSON array. The
`numTotalTests` fallback is not needed. The list-only form writes the array to
stdout with **zero bytes on stderr**, so stderr must not be treated as failure.

---

## H-TK — a Job Object with `KILL_ON_JOB_CLOSE` removes the whole tree

**Prediction.** A Job Object (created `KILL_ON_JOB_CLOSE`, process started
`CREATE_SUSPENDED`, assigned, then resumed) kills the shim plus
`node …/stryker.js serve stdio` plus any `child-process-proxy-worker` within 1 s
of the job closing, including when the probe itself is force-killed.

**Kill condition.** Any survivor → `taskkill /T /F /PID` on normal exit, and the
crash gap has to be documented.

**Protocol.** `jobprobe.go` creates the job, starts the child suspended, assigns
it, resumes it via `NtResumeProcess`, and closes the job handle — no `taskkill`
is ever issued. Survivors are listed with
`Get-CimInstance Win32_Process -Filter "Name='node.exe'"` filtered on the command
line. A `HTK_NOJOB=1` control runs the same scenarios without a job; it must
leave survivors, or the probe cannot separate the two situations.

**Observed.**

| scenario | result |
|---|---|
| sanity, minimal child, job | child observed alive, then gone 871 ms after job close |
| normal, real shim + `configure` + `discover`, job | shim signalled **5 ms** after job close; whole tree zero at the first CIM observation |
| normal, same, **no job** (control) | `stryker.js=1 serve-stdio=1` persisted for the full 30 s of polling |
| `mutationTest`, job | peak `child-process-proxy-worker` = 3 while mutating; whole tree zero after job close |
| crash, probe force-killed, job | `taskkill /F` on the probe → `stryker.js=0 serve-stdio=0 proxy-worker=0` |
| crash, probe force-killed, **no job** (control) | `stryker.js=1 serve-stdio=1` survived the probe's death |

**Verdict.** Prediction holds in every case. **Kill condition not met: no
`taskkill /T /F` fallback and no documented crash gap.** Because
`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` makes the kernel deliver the kill when the
last handle closes, the tree dies on normal exit, on cancellation, and when the
process is killed without running any cleanup at all.

The two control rows are what make this a measurement: the same probe, on the
same fixture, keeps the tree alive indefinitely when no job holds it. The
`suspended → assign → resume` ordering is preserved deliberately, so nothing the
child spawns can escape before the job exists.

---

## Environment notes (not product findings)

- `npm install` of the vitest fixture failed with
  `Cannot read properties of null (reading 'edgesOut')` from npm 10.9.3's
  arborist inside `#loadPeerSet` on the `vitest` node. `--legacy-peer-deps`
  installed the same pinned versions in 6 s. Recorded so a later session does not
  read it as a Stryker or dharness problem.
- **A survivor check that runs through a shell matches its own command line.**
  The first version of the H-TK probe used
  `Get-CimInstance Win32_Process | Where-Object { $_.CommandLine -like '*stryker.js*' }`
  and reported exactly one survivor of every kind while nothing was running: the
  PowerShell (and bash) process running the query carries the wildcard in its own
  command line. Filtering on an exact image name first is loading-bearing, not
  tidiness. Without it the job-object result read as a failure that was not real.
- **`$!` in Git Bash is not the Windows PID.** The first crash-path script killed
  `$!` and `taskkill` answered `The process … not found` while the probe ran on,
  so no job was ever closed and the run reported survivors that were only the
  still-live probe's children. The PID must be read from Windows.
- Go's `exec.Command` runs a `.cmd` through `cmd.exe` without an explicit
  wrapper, which is why shape A and shape B produce identical framing.

## Learning-log lines owed at WU-E

`docs/learning-log.md` is append-only, tracked, and written in Spanish (the
convention of its recent entries), and this checkout is behind `origin/main` with
foreign uncommitted work, so these lines were **not** appended here: appending
them would strand them in a stale checkout. They belong in the v1.9 worktree's
WU-E documentation slice. Candidates, one dated line each, not yet dated:

1. `discover` de Stryker MSP **omite el archivo** de `result.files` en vez de
   reportarlo con cero mutantes, así que "fuera del `mutate` configurado" y "en el
   set pero sin ningún mutante en el archivo entero" son **la misma observación**
   — medido: con dos archivos en `mutate`, `stryker run --dryRunOnly` dice
   `Found 2 of 11 file(s) to be mutated` / `Instrumented 2 source file(s) with 14
   mutant(s)`, y sin embargo `src/noop.ts` vuelve `{}` tanto en la consulta por
   rangos como en la consulta por ruta, byte a byte igual que `src/logic.test.ts`,
   que sí está fuera del set. La línea de salida no puede afirmar cuál de las dos
   causas fue; una que diga "excluido por configuración" está adivinando.
2. Un shim `.cmd` de npm mantiene el stdout de MSP limpio a nivel de frames: 8
   frames, cero bytes sueltos, cero headers malformados, logs en stderr, idéntico
   arrancando el `.cmd` directo (la forma que produce `project.LocalBinary`) o a
   través de `cmd.exe /d /s /c`. **No hace falta** un camino de invocación directa
   a Node para el servidor de mutación en Windows.
3. `jest --findRelatedTests <archivo> --listTests --json` contesta `[]` con exit 0
   y **no ejecuta nada**, así que el guard de tests relacionados no necesita correr
   Jest: la lista vacía es el veredicto. Probado con un marcador que el suite
   escribe al ejecutarse: ausente en las cuatro corridas list-only, presente en el
   control con `--passWithNoTests`, que es lo que hace que las ausencias sean
   evidencia y no silencio.
4. Un **Job Object** con `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` (arranque
   `CREATE_SUSPENDED`, asignar, resumir con `NtResumeProcess`) borra el árbol de
   `stryker serve stdio` — shim, server y los `child-process-proxy-worker` — **5 ms**
   después de cerrar el handle, y también cuando al proceso padre lo matan con
   `taskkill /F` sin correr nada de limpieza. Sin el job el mismo árbol sobrevive a
   su padre indefinidamente: es lo que hace que la medición tenga dientes.
5. Un chequeo de supervivientes que corre su propio filtro a través de un shell
   **se encuentra a sí mismo**: `Get-CimInstance Win32_Process | Where-Object {
   $_.CommandLine -like '*stryker.js*' }` reportó un superviviente de cada tipo con
   nada corriendo, porque el proceso de PowerShell (y el de bash) lleva el wildcard
   en su propia línea de comandos. Filtrar primero por nombre de imagen no es
   prolijidad, es lo que separa un falso positivo de un resultado.
6. En Git Bash, `$!` **no** es el PID de Windows. Un `taskkill /F /PID $!` responde
   `The process … not found` mientras el proceso sigue vivo, así que el job nunca se
   cerró y la corrida informó supervivientes que en realidad eran hijos de un padre
   todavía vivo. El PID hay que leerlo desde Windows.
