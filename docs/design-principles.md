# Principios de diseño de dharness

> Espejo en markdown del artifact publicado. Si los dos discrepan, gana este
> archivo, porque es el que viaja con el código.
>
> Complemento: [`learning-log.md`](./learning-log.md).

Ninguno de estos principios se escribió de antemano. Cada uno viene de una
discusión concreta, y varios de haber estado equivocado primero.

## Qué cuenta como principio

Un principio responde **qué debe ser dharness**. Restringe alcance, propiedad o
comportamiento, y se cita para aceptar o rechazar algo que todavía no existe.
La prueba es directa: *si no sirve para rechazar una propuesta futura, no es un
principio*.

Lo que descubrimos sobre las herramientas, la plataforma o nuestros propios
modos de fallar no es un principio por más caro que haya salido. Eso responde
**qué resultó ser cierto**, va fechado al learning log, y su valor es no volver
a pagarlo. Tres entradas de la primera versión de este documento se fueron por
ahí: medir antes de afirmar, el pineo de versiones remotas y dónde poner los
puntos de sustitución para pruebas. Los tres son buenos consejos y ninguno
rechaza una propuesta.

La distinción importa porque los dos documentos envejecen distinto. Un
aprendizaje queda fijo con su fecha y nunca se reescribe. Un principio se
discute señalando el caso que lo produjo, y si el caso resulta mal leído, el
principio se cae con él.

---

## Alcance — qué es dharness y qué no

Casi todo el diseño se decidió sacando cosas. Estos cuatro son los que más veces
evitaron construir de más.

### 01. Si la CLI ya lo hace, dharness no lo hace

No se envuelve una funcionalidad que ya existe, no se reimplementa y no se le da
soporte. Cada vez que apareció la tentación, la herramienta ya lo resolvía mejor
y más rápido.

*Salió de:* el ratchet de mutación resultó ser el `--gate new-only` que fallow ya
trae; la verificación de configuración resultó ser el campo `entry_points` que
fallow ya reporta; y Stryker no necesita archivo de configuración porque toda
opción suya es un flag.

### 02. La frontera de la delegación es «¿existe un comando?»

No es configuración contra código, ni preparación contra hallazgos. Todo lo que
tiene un comando lo ejecuta dharness. Solo va a una skill aquello para lo que
ninguna CLI expone un comando. Es un criterio que se puede aplicar paso por paso
sin discutirlo.

*Salió de:* que si esas CLIs ofrecieran un comando para adaptar su propia
configuración a un layout, lo correcto sería ejecutarlo y no complicarse. Con ese
criterio, la superficie de la skill se redujo a una sola cosa.

### 03. dharness posee la invocación, y solo la configuración que escribe él

Alcance, orden, techo de recursos y código de salida. De la configuración
ajena no toca nada: escribe la suya en `.dharness/` y le agrega al archivo del
proyecto una línea que apunta ahí.

*Salió de:* tres revisiones girando alrededor de cómo compartir configuraciones
entre repositorios. Puesto a resolverlo en concreto, el problema se disolvió: la
configuración por proyecto es chica, es específica de ese repositorio, y las
herramientas ya la generan. Lo caro y repetido era la ejecución.

*Enmendado el 9 de agosto de 2026.* La primera versión decía que dharness no
escribe configuración en absoluto, y dejó de ser cierta dos veces. La primera
cuando lefthook y fallow resultaron componer con `extends`: escribir un archivo
propio y referenciarlo es menos invasivo que fusionar el ajeno, no más. La
segunda cuando decidimos publicar un paquete de reglas propio, porque los
umbrales que necesita no caben en la severidad que react-doctor acepta y tienen
que vivir en un archivo. La frontera real no es escribir o no escribir: es de
quién es el archivo.

*Enmendado el 10 de agosto de 2026.* Poseer la invocación también fija su
resolución: las tres CLIs salen siempre del ejecutor remoto del gestor detectado
como `react-doctor@latest`, `fallow@latest` y
`@stryker-mutator/core@latest`. Sus copias en `node_modules/.bin` quedan fuera
de la resolución y un fallo remoto se propaga sin fallback. Core y el runner de
Stryker se provisionan juntos con `@latest` en un único entorno transitorio, y
`--appendPlugins` hace explícita la carga sin pisar los plugins configurados por
el proyecto. Toda la sintaxis de esas invocaciones vive en `internal/tool`; la
detección entrega metadatos y nunca arma comandos. En Yarn con `node_modules`, la
ruta transitoria conjunta es npx; la presencia de los loaders PnP la rechaza
antes de ejecutar Stryker porque el runner remoto no puede resolver las
dependencias de prueba del proyecto. Los dieciséis nombres de configuración por
defecto se reconocen: JSON conserva `testRunner` y `appendPlugins`, mientras
`.js`, `.mjs` y `.cjs` fallan con una corrección explícita porque conocer su
selección exigiría ejecutar código ajeno. `init` instala solo el plugin de reglas
que falte; esa instalación pertenece a la misma transacción que los archivos y,
ante un fallo, desinstala exactamente lo agregado por esa ejecución y restaura
byte por byte el manifiesto y el lockfile.

*Enmendado el 12 de agosto de 2026.* ESLint es la excepción registrada a lo
anterior: la fase del gate se resuelve con `p.LocalBinary("eslint")`, el
binario que el propio proyecto instaló, nunca con el ejecutor remoto — porque
el flat config del proyecto importa sus propios plugins y configuraciones de
framework, algo que un entorno transitorio no puede resolver. La medición
contra las otras tres fases (react-doctor, fallow audit, fallow dupes) sobre
la misma lista explícita de archivos en stage, tres corridas cada una, la
ubicó además como la más barata de las cuatro — no la más cara, como se había
asumido de forma provisional — así que corre primero en el orden del gate, no
al final; react-doctor y fallow conservan el orden relativo que ya tenían,
que esta medición no revisó (`docs/learning-log.md`, 12 de agosto de 2026).

### 04. Un comando que no se puede nombrar está haciendo dos cosas

El nombre no es presentación, es diagnóstico. Cuando ninguno encaja, casi siempre
es porque la responsabilidad no es una sola.

*Salió de:* `doctor` no tenía nombre posible porque imprimía diagnóstico *y*
medía el costo de mutación. Se partió en dos y desapareció: el diagnóstico ya lo
daba `sync`, la medición pasó a `mutate --dry-run`.

---

## Propiedad — de quién es cada decisión

### 05. Lo que el proyecto decidió no se pisa

Si un repositorio configuró una herramienta, eligió a propósito. dharness pasa
solo lo que le pertenece y se aparta del resto, aunque técnicamente pueda ganar.

*Salió de:* que los argumentos de línea de comandos le ganan al archivo de
configuración. Pasar `--testRunner` siempre dejaría que un error de detección
pisara una decisión deliberada. La misma regla gobierna después `--break` y
`--reporters`.

### 06. La política de stack viaja; la topología del repositorio se queda

Qué herramienta es dueña de qué diagnóstico, qué preset de arquitectura se usa:
eso es idéntico en todos los proyectos y pertenece al harness. Los entry points y
los globs de zonas describen *ese* repositorio y no se pueden compartir con
ningún otro.

*Salió de:* buscar un preset por combinación de stack y encontrar que explotaba.
La frontera correcta no era por proyecto sino por naturaleza del dato, y se
sostuvo cuando llegaron los boundaries de fallow.

---

## Verdad — cómo se sabe algo

Este grupo es el que más veces me dejó mal parado. Los cuatro salieron de afirmar
cosas que no eran.

### 07. Derivar, no rastrear

El estado se lee del repositorio, no de un archivo que dice qué se hizo. Un
archivo de progreso puede mentir: si algo se marca como hecho y el archivo nunca
se escribe, el sistema cree algo falso y deja de preguntar. La derivación no
puede equivocarse así, y la resumibilidad y la detección de deriva salen gratis.

*Salió de:* pedir resumibilidad para el proceso de adopción. De cinco cosas del
proceso, tres sobreviven en disco y no hacía falta recordarlas.

### 08. Persistir evidencia, nunca progreso

Lo único que merece un archivo es lo que cuesta obtener y no se puede volver a
derivar: un número medido, y contra qué se midió. No «paso 4 completado».

*Salió de:* las otras dos de esas cinco cosas: la medición del costo de mutación
y si la configuración quedó *correcta*, no si existe. Y de que sin eso `sync` no
tiene condición de salida, así que nunca puede decir que no queda nada por hacer.

### 09. No inventar una señal proxy si existe la directa

Antes de construir un heurístico, revisar qué reporta la herramienta. Suele
reportar exactamente lo que uno estaba por aproximar.

*Salió de:* haber inventado un umbral de porcentaje de código muerto para
detectar entry points mal configurados. fallow imprime `1 entry point detected` y
expone `entry_points` en su JSON. El heurístico sobraba entero.

### 10. Un mecanismo sin momento de ejecutarse no es un diseño

Antes de proponer que algo pase periódicamente, hay que poder nombrar qué lo
dispara. Si no existe ese momento, la propuesta es una ilusión con forma de plan.

*Salió de:* proponer resolver versiones «una vez por día, fuera del commit».
dharness solo corre cuando lo dispara un hook: fuera del commit no es un momento
que exista, y crear una tarea programada era inventar infraestructura que nadie
pidió.

---

## Puertas — cómo se comporta un gate

### 11. El código de salida es la respuesta

Se propaga el de la herramienta sin tocarlo, y un comando cuya conclusión hay que
leer de su prosa está roto. Una puerta que informa su propio estado en lugar del
de la herramienta convierte un check rojo en un commit verde en cuanto los dos
discrepan.

*Salió de:* que el default de Stryker es `break: null`, documentado como *«never
let your build fail»*: reportaba supervivientes y salía 0. Un agente lo corría,
veía éxito y seguía de largo.

### 12. Orden por costo ascendente

Lo barato primero, y la primera falla corta el resto. No es preferencia de
estilo: en una puerta que corre en cada commit, ese corte es la mayor parte del
ahorro.

*Salió de:* que react-doctor se acota al cambio con `--staged` mientras fallow
construye el grafo del repositorio igual, así que tiene un piso más alto.

### 13. La salida temprana vale más que cualquier optimización

La forma más barata de correr algo es no correrlo. Si no hay fuentes en el
índice, no se levanta ningún proceso; si la intersección de alcance queda vacía,
no se arranca el motor de mutación.

*Salió de:* medir que en cambios pequeños el costo de Stryker es casi todo
arranque: acotar de 23 mutantes a 12 ahorró un 16 %. Cuando el costo es fijo, lo
único que lo evita es no pagarlo.

### 14. Presupuesto de recursos, no de tiempo

La puerta no puede dejar la máquina inusable mientras corre. Ejecución en serie y
techo de concurrencia explícito por proceso, porque ninguna herramienta sabe de
la existencia de las otras y todas calculan su paralelismo sobre los núcleos
disponibles.

*Salió de:* que el pre-commit llegaba a congelar el equipo. Y de un
microbenchmark contraintuitivo: ocho workers dejaron a Stryker más lento que el
default. El techo no cuesta velocidad.

### 15. Nada es inmutable, así que todo paso debe poder reaparecer

Un proyecto cambia de gestor, pierde una herramienta, reescribe un hook. No hay
comando de instalación y otro de mantenimiento: hay uno solo, que se vuelve a
correr y dice qué dejó de ser cierto.

*Salió de:* notar que el paso de cablear la puerta se imprimía siempre,
contradiciendo la regla de que los pasos satisfechos desaparecen. Al arreglarlo
apareció gratis la detección de deriva.

---

## Lectores — para quién es la salida

### 16. La salida tiene dos lectores: una persona y el modelo que corrió el commit

Pasa en crudo, sin reinterpretar, pero con lo mínimo para que se pueda leer: qué
herramienta produjo qué, qué no llegó a ejecutarse, y a dónde ir para la pregunta
siguiente. Un fallo entrega la ayuda de la propia CLI, en la forma que ese
proyecto usa, y se detiene ahí.

*Salió de:* que servir de middleware y pasarle la salida de las herramientas al
modelo que ejecutó el pre-commit es invaluable. Y de encontrar que el fallo
nombraba una ruta absoluta en lugar de la herramienta.

### 17. La IA nunca es el gate

El veredicto sale de códigos de salida y de JSON, siempre. La IA edita; no decide
si algo pasa. El no determinismo vive en el arreglo, jamás en el fallo.

*Salió de:* la objeción de que confiar en que el modelo de turno siga
instrucciones no convence. Con esta separación deja de hacer falta que convenza.

### 18. Nunca devolver el trabajo al usuario

Un paso que consiste en que una persona lea una salida y traslade datos a mano no
es automatización: es el mismo trabajo con una capa de ceremonia encima. Y un
proceso con un humano intercalado no se reanuda solo. Lo único que sí es una
decisión humana es un conflicto real, donde ninguna máquina puede saber cuál
gana.

*Salió de:* una propuesta donde alguien revisaba la salida de un `init` y
trasladaba la topología a un manifiesto. Si hay que hacerlo a mano, es más rápido
hacer todo a mano.

*Enmendado el 10 de agosto de 2026.* «El usuario» de este principio es el lector
del reporte, nunca un actor del flujo: dharness no le dirige pasos a nadie que no
pueda ver. Un conflicto real se le entrega al agente, que decide si preguntarlo.
Ver 21.

---

## Método — cómo se resuelve un desacuerdo

### 19. Cuando la propuesta contradice al mecanismo, se mata la propuesta

Dos veces apareció una contradicción interna, y las dos veces la salida fue
eliminar, no arbitrar ni buscar un punto medio. Una contradicción es
información: dice que la pieza no debía existir.

*Salió de:* `verify --frozen`, que exigía ser autor único de archivos que a la
vez se delegaban a las herramientas. Y del manifiesto, que para funcionar
obligaba a dharness a generar la configuración de fallow, en contra de todo lo
demás.

---

## Adopción — qué pasa cuando un paso no se puede ejecutar

### 20. Un bloqueo es solo lo irrecuperable

Un paso que dharness no puede ejecutar baja un escalón y la corrida sigue.
Detenerse se justifica en dos casos y en ninguno más: cuando no hay plan posible,
y cuando ya se escribieron bytes que hay que devolver. Todo lo demás termina en
el plan. Abortar tampoco conserva nada, porque el estado se vuelve a derivar del
repositorio en cada corrida (07, 15): un paso pendiente reaparece solo.

*Salió de:* un `init` que abortó y desinstaló todo porque el proyecto ya tenía su
propio `.fallowrc.json`. Ese archivo no le hace falta al gate, y el que dharness
escribe para referenciarlo nace vacío hasta que el agente declare la
arquitectura, así que la corrida entera se deshizo para proteger una referencia a
un archivo sin contenido. Peor: el bloqueo disparaba justo cuando el repositorio
estaba mejor configurado.

### 21. El cliente es el agente, y la escalera termina ahí

dharness le habla al agente que lo corre, no a la persona. El agente programa;
dharness le entrega instrucciones ejecutables y se detiene. Si un paso necesita
algo que solo una persona sabe, es el agente quien decide preguntarlo, y esa
conversación no pertenece a este flujo. Un paso delegado entrega su prompt al
agente, nunca una instrucción dirigida a alguien que dharness no puede ver.

*Salió de:* haber dibujado un escalón para el usuario en el flujo de `init`, y
descubrir que no existe. Agregarle una clave a un JSON ajeno es leerlo y decidir,
que es exactamente el trabajo del agente. La rama de más tapaba esa
simplificación, y venía de leer el 18 como si nombrara un actor del flujo.

---

## Lo que tienen en común

Casi todos dicen que no hagamos algo. Ninguno se dedujo de una teoría: cada uno
apareció cuando una decisión concreta salió mal, o cuando alguien preguntó de
dónde salía algo que se había dado por bueno.

Y ninguno se queda por respeto. Cualquiera de estos veintiuno se discute
señalando el caso que lo produjo; si el caso está mal leído, el principio se va
con él, como ya se fueron los tres que resultaron ser aprendizajes.
