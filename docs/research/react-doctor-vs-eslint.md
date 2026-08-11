# ¿React Doctor puede reemplazar completamente a ESLint?

**Fecha de revisión:** 11 de agosto de 2026  
**Ámbito:** React, TypeScript, Next.js, React Native, Expo y monorepos.

## Respuesta ejecutiva

**No en un proyecto profesional de TypeScript, Next.js, React Native o Expo.** React Doctor puede reemplazar una parte considerable de la configuración de ESLint dedicada exclusivamente a detectar malos patrones de React, pero no sustituye de manera completa las demás responsabilidades de ESLint ni el análisis de tipos de TypeScript.

La arquitectura más razonable es:

1. **ESLint como infraestructura general y punto de integración con el editor**, con una configuración pequeña.
2. **`typescript-eslint` para reglas que necesitan información real de tipos.**
3. **La configuración oficial de Next.js o Expo** para el entorno y las particularidades del framework.
4. **React Doctor como especialista en React**, integrado mediante su plugin de ESLint para retroalimentación inmediata y mediante su CLI para auditorías de proyecto, código muerto, dependencias y seguridad.

Por tanto, React Doctor permite **simplificar ESLint**, no eliminarlo sin perder cobertura.

## Por qué 825 reglas no equivalen a un reemplazo completo

La referencia actual de React Doctor registra **825 reglas activas y 6 entradas de compatibilidad retiradas**. Sin embargo, “activa” significa que la regla pertenece al catálogo vigente, no que necesariamente se ejecute en cualquier proyecto. Algunas reglas:

- solo se habilitan cuando se detecta un framework o una dependencia determinada;
- pertenecen a presets distintos, como Next.js, React Native, TanStack o Preact;
- están deshabilitadas por defecto, especialmente varias reglas etiquetadas como diseño;
- pueden cambiar de severidad según la configuración del proyecto;
- requieren evidencia adicional antes de considerar el diagnóstico un defecto confirmado.

La propia documentación recomienda ejecutar `react-doctor rules list` para conocer la severidad **efectiva** en el proyecto. Por ello, la cifra total del catálogo no debe compararse directamente con el número de reglas que ESLint ejecuta realmente en una aplicación concreta.

Fuentes: [React Doctor — referencia de reglas](https://www.react.doctor/docs/rules) y [React Doctor — archivos de configuración](https://www.react.doctor/docs/configuration/config-files).

## Las responsabilidades de ambas herramientas son diferentes

React Doctor se presenta oficialmente como un analizador determinista especializado en React. Revisa estado y efectos, rendimiento, arquitectura, seguridad y accesibilidad. Su CLI también incorpora análisis de código muerto y comprobaciones de dependencias. Significativamente, su propia documentación afirma que **complementa el lint existente**, no que lo reemplace.

ESLint, por el contrario, es una infraestructura configurable para analizar JavaScript y otros lenguajes mediante parsers y plugins. Permite definir:

- qué archivos se procesan;
- qué parser y lenguaje corresponde a cada archivo;
- qué variables globales existen en cada entorno;
- qué plugins y configuraciones compartidas se cargan;
- qué reglas se aplican a cada grupo de archivos;
- excepciones y reglas internas del proyecto;
- severidades capaces de bloquear commits o CI.

Esta diferencia es importante: React Doctor ofrece una política curada de salud para aplicaciones React; ESLint ofrece el mecanismo general con el que un equipo construye y ejecuta su política de análisis estático.

Fuentes: [React Doctor — qué es y cómo encaja con el lint](https://www.react.doctor/docs), [ESLint — archivos de configuración](https://eslint.org/docs/latest/use/configure/configuration-files) y [ESLint — configuración de reglas](https://eslint.org/docs/latest/use/configure/rules).

## Comparación de cobertura

| Necesidad | React Doctor | ESLint y su ecosistema | Decisión recomendada |
|---|---|---|---|
| Hooks, efectos, estado y pureza de componentes | Cobertura amplia y especializada | Cubierto parcialmente por `eslint-plugin-react-hooks` y otros plugins | Priorizar React Doctor; verificar duplicados |
| Patrones de rendimiento y arquitectura React | Una de sus fortalezas principales | Cobertura fragmentada entre plugins | React Doctor |
| React Compiler | Incluye reglas relacionadas y depende del plugin oficial de hooks | El plugin oficial expone directamente diagnósticos del compilador | Cualquiera de los dos, pero no reportar la misma regla dos veces |
| Next.js, React Native y Expo | Presets y reglas específicas | Configuraciones oficiales controlan parser, runtime, globals y reglas del framework | Mantener ambos con responsabilidades separadas |
| JavaScript general | Tiene reglas generales seleccionadas | ESLint incluye reglas base y un ecosistema mucho más amplio | ESLint |
| TypeScript basado únicamente en sintaxis | Cobertura parcial | `typescript-eslint` ofrece cobertura extensa | `typescript-eslint` |
| TypeScript con información de tipos | No sustituye los presets type-checked de `typescript-eslint` | Reglas capaces de consultar el programa TypeScript completo | `typescript-eslint` |
| Reglas propias del equipo y límites arquitectónicos | Admite plugins personalizados con forma compatible con oxlint | Ecosistema maduro de plugins, overrides y reglas locales | ESLint, salvo casos muy concretos |
| Jest, Vitest y Testing Library | No es su foco principal | Plugins especializados | ESLint |
| Código muerto | Incluido en la CLI | Requiere herramientas o plugins adicionales | React Doctor CLI |
| Dependencias y cadena de suministro | Incluido en la CLI | No es una responsabilidad central de ESLint | React Doctor CLI |
| Diagnóstico mientras se escribe | Disponible mediante plugin de ESLint/oxlint o integración compatible | Integración madura con editores | Ejecutar React Doctor dentro de ESLint |
| Formato | No reemplaza un formateador | ESLint tampoco debería confundirse con un formateador | Prettier o herramienta equivalente |
| Comprobación completa de tipos | No | ESLint tampoco; corresponde a TypeScript | `tsc --noEmit` |

## La principal brecha: análisis con información de tipos

Las reglas tradicionales trabajan principalmente sobre la sintaxis y el árbol AST del archivo. Algunas clases de errores solo pueden detectarse de forma fiable cuando la herramienta conoce los tipos inferidos y las relaciones entre archivos.

`typescript-eslint` documenta presets como `recommendedTypeChecked` y el uso de `parserOptions.projectService: true`. Estas reglas hacen que TypeScript analice el proyecto completo y permiten detectar problemas que un análisis puramente sintáctico no puede resolver con la misma precisión. Ejemplos habituales incluyen:

- promesas ignoradas o utilizadas en posiciones incorrectas;
- operaciones inseguras originadas por valores `any`;
- condiciones con valores cuyo tipo no es realmente booleano;
- comparaciones o llamadas incompatibles según los tipos inferidos;
- APIs utilizadas con contratos de tipos peligrosos.

React Doctor tiene reglas relacionadas con promesas, coerciones y TypeScript, pero eso no demuestra equivalencia con el conjunto type-aware de `typescript-eslint`. Para afirmar que ESLint ya no es necesario, React Doctor tendría que sustituir tanto la integración con el programa TypeScript como las reglas configuradas sobre esa información; su documentación actual no realiza esa promesa.

Fuente: [typescript-eslint — linting con información de tipos](https://typescript-eslint.io/getting-started/typed-linting/).

## Expo demuestra por qué todavía se necesita una configuración de entorno

Desde Expo SDK 53, Expo genera una configuración Flat Config en `eslint.config.js` que extiende `eslint-config-expo`. No se limita a agregar reglas estilísticas: también configura diferencias entre los entornos donde se ejecutan los archivos.

Por ejemplo, `metro.config.js`, `babel.config.js`, archivos de configuración y código de la aplicación no comparten exactamente los mismos globals ni el mismo runtime. Parte del código se ejecuta en Node.js, mientras que la aplicación puede ejecutarse en Hermes, navegador o Node. `eslint-config-expo` ayuda a modelar esas diferencias.

React Doctor conoce Expo y React Native, pero sus presets no deben interpretarse automáticamente como sustitutos de toda la configuración de entorno de Expo.

Fuente: [Expo — uso y configuración de ESLint](https://docs.expo.dev/guides/using-eslint/).

## Una limitación especialmente importante con Flat Config

React Doctor puede adoptar una configuración ESLint u oxlint existente, pero la adopción automática está limitada a configuraciones en JSON. Su changelog aclara que no puede evaluar configuraciones JavaScript o TypeScript mediante el mecanismo de `extends` de oxlint; por ello se omiten formatos como:

- `eslint.config.js`;
- `.eslintrc.js` y `.eslintrc.cjs`;
- `oxlint.config.ts`.

Esto importa porque tanto Expo moderno como buena parte del ecosistema actual utilizan Flat Config en archivos JavaScript. En esos proyectos no debe asumirse que ejecutar únicamente la CLI de React Doctor también ejecutará la política definida en `eslint.config.js`.

La solución oficial es la inversa: instalar `eslint-plugin-react-doctor` e incorporar sus presets dentro de ESLint. React Doctor proporciona presets `recommended`, `next`, `react-native`, `tanstack-start`, `tanstack-query`, `preact` y `all`.

Fuentes: [React Doctor — plugins de ESLint y oxlint](https://www.react.doctor/docs/configuration/eslint-and-oxlint-plugins), [React Doctor — configuración existente](https://www.react.doctor/docs/configuration/config-files#existing-lint-config) y [React Doctor — changelog, adopción y límites de configuraciones JSON](https://www.react.doctor/docs/community/changelog).

## Lo que sí puede eliminarse de ESLint

Después de incorporar el preset adecuado de React Doctor, es razonable revisar y posiblemente retirar configuraciones React redundantes. Esto puede incluir reglas duplicadas provenientes de:

- `eslint-plugin-react`;
- `eslint-plugin-react-hooks`;
- `eslint-plugin-jsx-a11y`;
- plugins de rendimiento React;
- reglas locales creadas para detectar patrones que React Doctor ya cubre.

No conviene eliminarlas únicamente porque exista una regla con nombre parecido. Antes hay que comparar:

1. el patrón exacto que detecta cada regla;
2. sus excepciones y opciones;
3. su severidad;
4. si funciona en React Native además de web;
5. si usa información de tipos;
6. si una de las reglas ofrece autofix;
7. si el preset de React Doctor realmente la habilita para ese proyecto.

React mantiene además su plugin oficial `eslint-plugin-react-hooks`, que cubre `rules-of-hooks`, `exhaustive-deps` y diagnósticos del React Compiler. React Doctor lo incluye como dependencia para que estas reglas funcionen, pero un proyecto que también active el preset oficial directamente debe comprobar que no esté publicando dos diagnósticos equivalentes.

Fuentes: [React — eslint-plugin-react-hooks](https://react.dev/reference/eslint-plugin-react-hooks), [React — exhaustive-deps](https://react.dev/reference/eslint-plugin-react-hooks/lints/exhaustive-deps) y [React Doctor — referencia de reglas](https://www.react.doctor/docs/rules).

## Configuración recomendada para Next.js, Expo y Turborepo

### Capa rápida: editor y pre-commit

Mantener ESLint con:

- configuración oficial de Next.js o Expo;
- `typescript-eslint`;
- preset de React Doctor apropiado;
- reglas de imports, tests y arquitectura que el proyecto realmente necesite;
- caché y ejecución limitada a archivos modificados.

ESLint documenta que `--cache` conserva información sobre los archivos procesados para trabajar posteriormente solo con los que han cambiado, lo que puede mejorar considerablemente el tiempo de ejecución.

```json
{
  "scripts": {
    "lint": "eslint . --cache --cache-location .cache/eslint",
    "typecheck": "tsc --noEmit"
  }
}
```

Fuente: [ESLint CLI — caché](https://eslint.org/docs/latest/use/command-line-interface#--cache).

### Capa profunda: pre-push o CI

Ejecutar React Doctor mediante su CLI para conservar las comprobaciones que no están disponibles en el plugin convencional:

- análisis de código muerto;
- dependencias y cadena de suministro;
- secretos, configuración y artefactos a nivel de proyecto;
- puntuación y reporte por proyecto;
- comparación contra la rama base.

```json
{
  "scripts": {
    "doctor": "react-doctor --scope changed --blocking error",
    "doctor:full": "react-doctor --scope full --blocking error"
  }
}
```

El modo `--staged` también existe para pre-commit, pero React Doctor omite allí el análisis de código muerto y la comprobación de cadena de suministro. Es adecuado si su tiempo real en el repositorio es aceptable; no sustituye la auditoría completa de CI.

Fuentes: [React Doctor — referencia de CLI](https://www.react.doctor/docs/reference/cli-reference) y [React Doctor — configuración de código muerto y dependencias](https://www.react.doctor/docs/configuration/config-files).

## Cuándo React Doctor sí podría utilizarse sin ESLint

La decisión puede ser aceptable en un proyecto pequeño que cumpla casi todas estas condiciones:

- JavaScript o TypeScript muy sencillo;
- código exclusivamente React;
- sin reglas type-aware;
- sin tests que requieran plugins especializados;
- sin convenciones de imports o arquitectura;
- sin necesidad de configuración avanzada por entorno;
- sin dependencia de la integración ESLint del editor;
- tolerancia a que la política de lint quede determinada principalmente por React Doctor.

Incluso en ese escenario seguirían siendo independientes el formateador y la comprobación de tipos.

## Decisión final

Para React + TypeScript + Next.js/Expo/React Native, la recomendación es:

> **No eliminar ESLint. Reducirlo a una infraestructura mínima y dejar que React Doctor se encargue de la capa especializada en React.**

La cantidad de reglas de React Doctor es impresionante —sí, bastante—, pero el criterio correcto no es cuántas reglas existen sino qué información usan y qué responsabilidades cubren. React Doctor supera a una configuración ESLint convencional en varias clases de diagnóstico React y de proyecto; ESLint continúa siendo superior como plataforma de integración, configuración por entorno, análisis TypeScript extensible y aplicación de convenciones propias.

La combinación óptima no es “React Doctor o ESLint”, sino:

```text
Editor y pre-commit
└── ESLint con caché
    ├── configuración oficial de Next.js o Expo
    ├── typescript-eslint
    ├── eslint-plugin-react-doctor
    └── reglas propias, imports y tests

Pre-push o CI
├── tsc --noEmit
├── ESLint completo
└── React Doctor CLI
    ├── reglas React
    ├── código muerto
    ├── seguridad de proyecto
    └── dependencias
```

## Fuentes principales

Todas las fuentes siguientes son documentación oficial o el changelog oficial del proyecto correspondiente:

1. [React Doctor — documentación general](https://www.react.doctor/docs)
2. [React Doctor — referencia de 825 reglas](https://www.react.doctor/docs/rules)
3. [React Doctor — configuración](https://www.react.doctor/docs/configuration/config-files)
4. [React Doctor — plugins ESLint y oxlint](https://www.react.doctor/docs/configuration/eslint-and-oxlint-plugins)
5. [React Doctor — referencia de CLI](https://www.react.doctor/docs/reference/cli-reference)
6. [React Doctor — changelog](https://www.react.doctor/docs/community/changelog)
7. [ESLint — archivos de configuración](https://eslint.org/docs/latest/use/configure/configuration-files)
8. [ESLint — reglas y severidades](https://eslint.org/docs/latest/use/configure/rules)
9. [ESLint — plugins](https://eslint.org/docs/latest/use/configure/plugins)
10. [ESLint — CLI y caché](https://eslint.org/docs/latest/use/command-line-interface#--cache)
11. [typescript-eslint — introducción](https://typescript-eslint.io/)
12. [typescript-eslint — linting con información de tipos](https://typescript-eslint.io/getting-started/typed-linting/)
13. [React — eslint-plugin-react-hooks](https://react.dev/reference/eslint-plugin-react-hooks)
14. [React — regla exhaustive-deps](https://react.dev/reference/eslint-plugin-react-hooks/lints/exhaustive-deps)
15. [Expo — uso de ESLint y Prettier](https://docs.expo.dev/guides/using-eslint/)

