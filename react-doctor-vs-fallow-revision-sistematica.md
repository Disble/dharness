# React Doctor vs Fallow para React y React Native

**Revisión sistemática de capacidades, cobertura y adopción**  
**Fecha de corte:** 9 de agosto de 2026  
**Versiones verificadas:** `react-doctor@0.9.11` y `fallow@3.14.0`

> **Conclusión ejecutiva:** React Doctor es la mejor herramienta para analizar la semántica de React y React Native: estado, efectos, hooks, accesibilidad, rendimiento de componentes y patrones nativos. Fallow es superior para analizar el repositorio como sistema: código muerto, dependencias, ciclos, duplicación, complejidad, límites arquitectónicos y auditabilidad. Para un proyecto importante, la mejor solución no es escoger una sola: conviene usar React Doctor para la capa React/RN y Fallow como dueño del grafo del repositorio.

## 1. Pregunta de investigación

¿Cuál de las dos herramientas ofrece la mejor cobertura y utilidad práctica en cada una de sus funcionalidades principales para proyectos React y React Native?

La comparación evalúa capacidades documentadas y observables. No intenta afirmar cuál posee mejor precisión, *recall* o velocidad absoluta, porque no existe un estudio independiente y reproducible que mida ambas bajo un mismo protocolo.

## 2. Metodología

### 2.1 Fuentes aceptadas

Se utilizaron exclusivamente:

- Repositorios oficiales y su código fuente.
- Documentación oficial de las herramientas.
- Paquetes publicados en npm y su contenido distribuido.
- Licencias, changelogs y archivos de configuración oficiales.
- Resultados de ejecuciones controladas con versiones fijadas.
- Publicaciones de investigadores independientes únicamente si su identidad, trayectoria, metodología, reproducibilidad y conflictos de interés podían verificarse.

Se excluyeron videos, blogs, noticias, comparativas comerciales sin metodología y contenido de terceros no verificable.

### 2.2 Evaluación de investigadores independientes

Se buscaron estudios que compararan directamente React Doctor y Fallow. Para considerarlos evidencia se exigió:

1. Identidad y credenciales técnicas verificables.
2. Experiencia pertinente en análisis estático, ingeniería de software o ecosistema JavaScript/TypeScript.
3. Protocolo reproducible y versiones de las herramientas declaradas.
4. Conjunto de proyectos o defectos publicado.
5. Métricas definidas y conflictos de interés declarados.

**Resultado:** no se encontró una comparación independiente directa que cumpliera todos los criterios. Por tanto, no se incorporaron opiniones de terceros ni se atribuyeron diferencias estadísticas de precisión o rendimiento.

### 2.3 Pruebas controladas

Se construyeron dos *fixtures* mínimos:

- **React web:** estado derivado en un efecto, problemas de accesibilidad, claves de lista, archivos y exports sin uso, ciclos, duplicación y complejidad.
- **React Native/Expo:** uso de `TouchableOpacity`, texto fuera de `Text`, renderizado de listas en `ScrollView`, arrays de estilos triviales, claves ausentes y un archivo sin uso.

Comandos equivalentes:

```bash
npx react-doctor@0.9.11 . --json --no-score --no-telemetry --no-supply-chain
npx fallow@3.14.0 --format json --quiet
```

Los *fixtures* comprueban la superficie funcional, pero no constituyen un *benchmark* ni permiten calcular precisión o *recall*.

## 3. Resultado general

| Objetivo principal | Herramienta recomendada |
|---|---|
| Calidad semántica de componentes React | **React Doctor** |
| React Native y Expo | **React Doctor** |
| Accesibilidad JSX | **React Doctor** |
| Rendimiento y antipatrones de componentes | **React Doctor** |
| Código muerto y dependencias | **Fallow** |
| Duplicación y complejidad | **Fallow** |
| Arquitectura y límites entre módulos | **Fallow** |
| Auditoría, privacidad y licencia | **Fallow** |
| CI y revisión de pull requests | **Empate**, con fortalezas diferentes |

## 4. Comparación por funcionalidad

| Funcionalidad | React Doctor | Fallow | Ganador | Confianza |
|---|---|---|---|---|
| Estado, efectos y hooks | Reglas semánticas específicas sobre efectos, estado derivado, hooks y patrones React. | Incluye contexto React en algunas métricas, pero no una suite semántica equivalente. | **React Doctor** | Alta |
| React Native y Expo | Reglas específicas para componentes nativos, texto, listas, estilos y patrones RN. La referencia revisada contenía 33 reglas activas de React Native. | Plugins para reconocer entradas y condiciones de resolución de React Native/Expo, con poca semántica de UI nativa. | **React Doctor** | Alta |
| Accesibilidad | Suite extensa para JSX; la referencia revisada contenía 93 reglas activas de accesibilidad. | No documenta una suite comparable de accesibilidad. | **React Doctor** | Alta |
| Rendimiento de componentes | Diagnósticos sobre renderizados, listas, memoización y antipatrones de componentes. | Métricas estructurales como fan-in, hooks y profundidad JSX. | **React Doctor** | Alta |
| Seguridad de aplicación y *supply chain* | Reglas de aplicación y puntuación de dependencias mediante Socket.dev, activa por defecto. | Emite candidatos de seguridad priorizados por alcanzabilidad que requieren verificación humana. | **React Doctor** | Media |
| Código muerto y dependencias | Integra Deslop y consume cuatro familias principales de hallazgos de grafo: archivos, exports, dependencias sin uso y ciclos. | Analiza archivos, exports, tipos, enums, miembros de clase, dependencias, imports sin resolver, ciclos y reexports. | **Fallow** | Alta |
| Duplicación | No es una capacidad principal documentada. | Detecta clones, porcentaje de líneas duplicadas, vistas previas y umbrales configurables. | **Fallow** | Alta |
| Complejidad y *hotspots* | Se concentra en diagnósticos React, no en una puntuación global del repositorio. | Publica métricas de complejidad ciclomática/cognitiva, acoplamiento, inestabilidad y una fórmula de salud. | **Fallow** | Alta |
| Límites de arquitectura | No presenta un motor principal de zonas o capas permitidas. | Permite definir *boundaries*, capas, imports prohibidos y ciclos. | **Fallow** | Alta |
| Consistencia del *design system* | Detecta varios antipatrones de UI, pero la gobernanza de tokens no es su eje principal. | Incluye detección de deriva del sistema de diseño y patrones de componentes. | **Fallow** | Media |
| CI y revisión de PR | GitHub Action oficial, resumen persistente, comentarios en líneas cambiadas y soporte para otros CI mediante CLI. | GitHub/GitLab, SARIF, Code Climate, Markdown, anotaciones y auditoría por archivos modificados. | **Empate** | Alta |
| Auditabilidad, privacidad y licencia | Telemetría y puntuación compartida activadas por defecto; licencia MIT modificada con restricciones para IA y productos derivados. | Telemetría desactivada por defecto, fórmula de salud publicada y licencia MIT estándar. | **Fallow** | Alta |

## 5. Análisis detallado

### 5.1 React Doctor: fortalezas

React Doctor funciona como un especialista del framework. Sus reglas observan patrones cuyo significado depende de React o React Native, por ejemplo:

- Estado derivado almacenado mediante efectos.
- Uso incorrecto o innecesario de hooks.
- Problemas de accesibilidad en JSX.
- Claves de listas y patrones de renderizado.
- Antipatrones de rendimiento de componentes.
- Uso de componentes y estilos de React Native.

Esta cercanía con la semántica del framework vuelve sus resultados más accionables para una persona que está editando un componente.

También ofrece una experiencia de CI especialmente guiada: acción oficial para GitHub, resumen persistente de la revisión y comentarios sobre líneas modificadas. La CLI puede integrarse en GitLab, CircleCI, Jenkins, Buildkite u otros proveedores.

### 5.2 React Doctor: cautelas

- La comprobación de la cadena de suministro utiliza Socket.dev y está habilitada por defecto.
- La telemetría está activa salvo que se use `--no-telemetry`.
- El sistema de puntuación y compartición puede deshabilitarse con `--no-score`.
- Su licencia es una MIT modificada: restringe usos relacionados con entrenamiento/evaluación de IA y ciertos productos o servicios cuyo valor derive sustancialmente de la herramienta. Debe revisarse antes de una adopción comercial.
- Su código muerto no posee la misma amplitud que Fallow; además, duplicar ambos analizadores genera ruido y posibles discrepancias.

### 5.3 Fallow: fortalezas

Fallow funciona como un analizador de inteligencia del repositorio. Construye un grafo del proyecto y combina varias familias de señales:

- Archivos, exports, tipos y dependencias sin uso.
- Imports sin resolver y dependencias no declaradas.
- Ciclos y ciclos de reexportación.
- Duplicación de código.
- Complejidad y puntos críticos.
- Límites entre capas o zonas arquitectónicas.
- Deriva del sistema de diseño.
- Auditoría de cambios para CI.

Su puntuación de salud es auditable porque publica la fórmula y explica sus fundamentos. La documentación relaciona las métricas con trabajos clásicos como McCabe, Henry/Kafura, Chidamber y Kemerer, además de referencias de NIST y SIG/TÜViT. Esto no demuestra por sí solo que la puntuación prediga defectos, pero permite comprender y discutir exactamente qué penaliza.

Fallow ofrece más formatos de salida —JSON, SARIF, Code Climate, Markdown y anotaciones para plataformas— y una licencia MIT estándar. Su telemetría es opt-in y reconoce `DO_NOT_TRACK`.

### 5.4 Fallow: cautelas

- El análisis predeterminado es sintáctico; no ejecuta el compilador de TypeScript. El modo `--type-aware` es opcional.
- La configuración dinámica de frameworks puede producir falsos positivos si no se declaran correctamente las entradas.
- La documentación reconoce límites en CSS, inyección de dependencias, imports dinámicos y tipos consumidos externamente.
- Los resultados de seguridad son candidatos para revisión, no vulnerabilidades confirmadas.
- Los plugins de React Native y Expo mejoran el grafo, pero no sustituyen las reglas semánticas de React Doctor.

## 6. Resultados de los fixtures

### 6.1 React web

- **React Doctor:** 13 diagnósticos —1 error y 12 advertencias— que cubrieron defectos de componentes React, accesibilidad y parte del grafo.
- **Fallow:** 9 hallazgos de código muerto, un grupo de clones y una señal de complejidad.

La lectura correcta no es “13 contra 9”. Cada herramienta encontró clases distintas de problemas: React Doctor explicó errores dentro de componentes; Fallow describió la estructura y mantenimiento del proyecto.

### 6.2 React Native/Expo

- **React Doctor:** 8 diagnósticos, incluidos dos errores de texto nativo, recomendaciones sobre `Pressable`, estilos, listas, claves y un archivo sin uso.
- **Fallow:** un archivo sin uso y métricas estructurales.

Este fixture confirma la separación principal: React Doctor comprende patrones de UI nativa; Fallow comprende principalmente la alcanzabilidad y estructura del repositorio.

## 7. Recomendación de configuración combinada

La configuración más coherente es:

1. **React Doctor como analizador semántico de React/RN.** Mantener activas las reglas de efectos, hooks, accesibilidad, rendimiento, seguridad y React Native.
2. **Fallow como fuente única del grafo.** Asignarle código muerto, dependencias, imports sin resolver, ciclos, duplicación, complejidad y límites arquitectónicos.
3. **Desactivar el bloque de código muerto de React Doctor** cuando Fallow ya lo ejecuta en el mismo repositorio.
4. **Separar los gates de CI:** bloquear por diagnósticos nuevos y de alta confianza; reportar métricas de salud como tendencia hasta calibrar umbrales.
5. **Fijar versiones en CI** y revisar changelogs antes de actualizar reglas o severidades.
6. **Decidir explícitamente la privacidad:** usar `--no-telemetry`, `--no-score` y, si corresponde, `--no-supply-chain` en React Doctor; conservar la telemetría desactivada en Fallow.

Ejemplo conceptual:

```bash
# Semántica React / React Native
npx react-doctor@0.9.11 . --no-telemetry --no-score

# Grafo, dead code, arquitectura, duplicación y salud
npx fallow@3.14.0 --format sarif
```

La exclusión exacta de categorías debe expresarse en los archivos de configuración del proyecto y comprobarse después de cada actualización.

## 8. Decisión si solo puede instalarse una

- **Aplicación React Native/Expo:** React Doctor.
- **Aplicación React con equipo centrado en calidad de componentes:** React Doctor.
- **Monorepo TypeScript grande o con deuda estructural:** Fallow.
- **Repositorio con problemas de código muerto, ciclos o límites entre paquetes:** Fallow.
- **Proyecto comercial sensible a licencia, telemetría o auditabilidad:** Fallow, después de validar que su menor cobertura semántica React sea aceptable.

## 9. Limitaciones de esta revisión

- Las versiones y reglas pueden cambiar después de la fecha de corte.
- Los números de reglas describen cobertura declarada, no calidad estadística.
- Los fixtures fueron deliberadamente pequeños y no representan repositorios reales completos.
- No se midieron falsos positivos, falsos negativos ni tiempos bajo hardware controlado.
- No se encontró un estudio independiente directo con credenciales y metodología suficientes.
- Algunas capacidades opcionales dependen de servicios externos, configuración avanzada o planes comerciales.

## 10. Fuentes primarias

### React Doctor

1. [Repositorio oficial y README](https://github.com/millionco/react-doctor)
2. [Referencia oficial de reglas](https://www.react.doctor/docs/rules)
3. [Configuración](https://www.react.doctor/docs/configuration/config-files)
4. [GitHub Actions](https://www.react.doctor/docs/ci-and-prs/github-actions-setup)
5. [Otros proveedores de CI](https://www.react.doctor/docs/ci-and-prs/other-ci-providers)
6. [Changelog](https://www.react.doctor/docs/community/changelog)
7. [Uso de datos y privacidad](https://www.react.doctor/docs/legal/data-use)
8. [Licencia del repositorio](https://github.com/millionco/react-doctor/blob/main/LICENSE)
9. [Paquete oficial en npm](https://www.npmjs.com/package/react-doctor)

### Fallow

1. [Repositorio oficial y README](https://github.com/fallow-rs/fallow)
2. [Releases oficiales](https://github.com/fallow-rs/fallow/releases)
3. [Esquema de configuración](https://github.com/fallow-rs/fallow/blob/main/schema.json)
4. [Documentación incluida en el repositorio](https://github.com/fallow-rs/fallow/tree/main/docs)
5. [Licencia MIT](https://github.com/fallow-rs/fallow/blob/main/LICENSE)
6. [Paquete oficial en npm](https://www.npmjs.com/package/fallow)

---

**Reporte interactivo:** [React Doctor vs Fallow — presentación de resultados](https://react-doctor-vs-fallow.disble.chatgpt.site)
