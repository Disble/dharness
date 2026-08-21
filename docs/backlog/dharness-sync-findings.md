# `dharness sync` — findings from a first-run bootstrap

Report prepared for the dharness maintainers.

## Environment

| | |
|---|---|
| dharness | 1.5.1 |
| project | fresh `create-next-app` output, Next.js 16.3.1, React 19.2.8 |
| package manager | bun 1.4.0 (text `bun.lock`) |
| node | v22.19.0 |
| git | 2.51.0.windows.1 |
| OS | Windows 11 Pro 26200 |
| detected by sync | `js project` · `bun` · `test runner: none detected` · `presets: nextjs` |
| hook manager | lefthook 2.1.10, installed during the session |

Scenario: run `dharness sync` on a repository that has never seen dharness, then follow its `next` prompts until it stops asking for anything.

## Summary

`sync` finished `exit 0` with `0 failed` three times in a row while two of the things it reported as done were not done. Both failures are silent, and both look like success in the summary line — which is the part a user reads.

The pattern behind the two high-severity findings is the same: **`sync` reports the outcome of the subprocess it invoked, not the state of the thing it was asked to produce.** A step whose job is "the gate runs on commit" is marked applied because `lefthook install` exited 0; a step whose job is "the installed lint plugins are in effect" is filed under *Already in place* because the rewriter declined to act.

---

## F1 — `sync` reports the commit gate as wired when no `pre-commit` hook exists

**Severity: high.** The gate silently never runs.

Step 9 `wire the gate into git` is marked applied and prints the subprocess line `sync hooks: ✔️`. No `pre-commit` hook is installed, so `dharness check` never executes.

### What happens

On the first run, with no hook manager present, step 9 correctly delegates:

```
 ! 9/11   wire the gate into git
   nothing answers: there is no lefthook config, no .husky/ and no
      lefthook binary. Choosing a hook manager is a decision this
      project has not made, and not a default dharness gets to pick.
```

After installing lefthook and re-running:

```
 ✓ 9/11 wire the gate into git                0.18s
         │ sync hooks: ✔️
```

But:

```
$ ls .git/hooks | grep -v sample
prepare-commit-msg

$ ls .git/hooks/pre-commit
ls: cannot access '.git/hooks/pre-commit': No such file or directory
```

### Root cause

Step 9 shells out to `lefthook install`. On a repository with no lefthook config, `lefthook install` does **not** fail — it scaffolds an all-commented example config, installs only its default `prepare-commit-msg` hook, and reports success. Isolated repro, no dharness involved:

```
$ git init -q .
$ lefthook install
Config not found, creating...
Added config: .../lefthook.yml
sync hooks: ✔️

$ ls .git/hooks | grep -v sample
prepare-commit-msg
```

`sync hooks: ✔️` is lefthook's own output. dharness relays it as proof the gate is armed. It is not: it is proof that lefthook wrote a placeholder.

### Expected

Step 9's postcondition is "a commit hook carries `dharness check`". Verify that, not the exit code:

- assert `.git/hooks/pre-commit` exists, **and**
- assert the resolved config actually carries the job — e.g. `lefthook dump` contains the `pre-commit` command dharness owns.

If the postcondition does not hold, the step is `failed` or `delegated`, not `applied`.

### Fix applied here

Root `lefthook.yml` replaced with `extends: [.dharness/lefthook.yml]`, then `lefthook install` re-run. Verified:

```
$ lefthook dump
extends:
  - .dharness/lefthook.yml
pre-commit:
  commands:
    dharness:
      run: dharness check

$ ls .git/hooks | grep -v sample
pre-commit
prepare-commit-msg
```

---

## F2 — an unwired ESLint layer is reported under *Already in place*

**Severity: high.** Two installed plugins are completely inert and nothing says so.

Step 1 installs `dharness-eslint-plugin@0.3.0` and `eslint-plugin-react-doctor@0.9.12`. Step 2 writes `.dharness/eslint.config.js`. Step 6 is then supposed to point the project's ESLint config at that file. It does not, and files the non-result in the satisfied bucket:

```
── Already in place (6) ──
 = 6/11   point eslint.config.js at the file dharness owns
         config shape not recognised
 = 7/11   fix the lint config react-doctor silently drops
         not present
```

Nothing in `eslint.config.mjs` imports `.dharness/eslint.config.js`, so every rule in the layer — `dharness/max-file-lines`, `dharness/require-jsdoc`, `dharness/role-file-shape`, the whole of `react-doctor` — is dead. The run summary reads `8 satisfied · 0 failed · exit 0`, and the closing `next` line points at an unrelated step.

Declining to rewrite an unrecognised config is the right call; guessing at someone's config shape is worse. Reporting the decline as *Already in place* is not.

### The unrecognised shape

Current `create-next-app` output for Next 16.3.1:

```js
import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  globalIgnores([".next/**", "out/**", "build/**", "next-env.d.ts"]),
]);

export default eslintConfig;
```

The array is built by `defineConfig(...)` from `eslint/config` and bound to a `const` before export, rather than being exported as a bare array literal. Given the `nextjs` preset is detected and this is what the framework's own scaffolder emits today, it is worth teaching the rewriter this one shape.

### Expected

Two options, either is an improvement:

1. Recognise `defineConfig([...])` and the `const` indirection.
2. Keep declining, but move the step to **Left to you** with the exact snippet to paste. dharness already carries the template — the strings `// dharness:eslint-import begin`, `import dharnessLayer from %q;` and `...dharnessLayer({ %s }),` are in the binary. Emitting them for a human to paste costs nothing and closes the gap.

A step that leaves freshly installed dependencies with no effect should never land in the same bucket as `= 8/11 give the agent fallow's own tools / .mcp.json declares fallow`.

### Fix applied here

```js
// dharness:eslint-import begin — rewritten by `dharness sync`; edits here are lost.
import dharnessPlugin from "dharness-eslint-plugin";
import dharnessNext from "eslint-config-next/core-web-vitals";
import dharnessNextTypeScript from "eslint-config-next/typescript";
import dharnessReactDoctor from "eslint-plugin-react-doctor";
import dharnessLayer from "./.dharness/eslint.config.js";
// dharness:eslint-import end

const eslintConfig = defineConfig([
  ...dharnessLayer({
    plugin: dharnessPlugin,
    dharnessNext,
    dharnessNextTypeScript,
    dharnessReactDoctor,
  }),
  globalIgnores([".next/**", ".dharness/**", "out/**", "build/**", "next-env.d.ts"]),
]);
```

Note the original `...nextVitals` / `...nextTs` spreads have to be dropped, because `dharnessLayer()` already spreads `dharnessNext` and `dharnessNextTypeScript` internally. A user wiring this by hand from the factory signature alone will very plausibly keep both and end up with the Next configs applied twice. Worth a line in whatever guidance the delegated step emits.

After wiring, the layer is live and reports 8 errors / 2 warnings on the untouched starter.

---

## F3 — step 9 creates the file that then permanently blocks step 5

**Severity: medium.** Ordering defect; leaves the run in a state no further `sync` can exit.

Run 1, before any hook manager exists:

```
 = 5/11   point lefthook.yml at the file dharness owns
         no lefthook config
```

Run 2, after `bun add -d lefthook`, step 9 executes `lefthook install`, which scaffolds `lefthook.yml` (see F1). In the same run, step 5 now reports:

```
 ! 5/11   point lefthook.yml at the file dharness owns
   lefthook.yml already exists and belongs to the project; adding a
      key to it is a merge, not a write.
```

The file dharness refuses to touch as *belonging to the project* was created seconds earlier by dharness's own step 9, and contains no project content at all — it is 100% commented-out lefthook boilerplate (`# EXAMPLE USAGE: ...`, a commented `pre-push`, a commented `pre-commit`). There is nothing to merge and nothing to lose.

Left alone, this never resolves: `sync` reports step 5 as delegated forever, on a file it authored itself.

### Expected

Either:

- run step 5 before step 9, so the config is pointed at `.dharness/lefthook.yml` before `lefthook install` gets the chance to scaffold; or
- recognise lefthook's pristine scaffold (no uncommented keys) and treat it as writable rather than as a project decision.

---

## F4 — `.dharness/eslint.config.js` violates dharness's own rule, and the user cannot fix it

**Severity: medium.**

Once the layer from F2 is wired, the first error ESLint reports is in dharness's own file:

```
.dharness\eslint.config.js
  5:16  error  This function is declared at the top of the file with nothing saying what it is for  dharness/require-jsdoc
```

Line 5 is the `export default function dharnessLayer(...)` that dharness writes. The file header states dharness owns and rewrites it, so any JSDoc a user adds is lost on the next `sync`. The rule is unsatisfiable by the person it fires at.

### Expected

Either add `.dharness/**` to the ignores inside the layer `dharnessLayer()` returns, or emit the JSDoc comment on the generated factory. The first is better — it also covers whatever else lands in `.dharness/` later.

Worked around here by adding `.dharness/**` to the project's `globalIgnores`, which is a patch the user has to know to write.

---

## F5 — `.dharness/eslint.config.js` triggers a Node module-type warning on every ESLint run

**Severity: low.** Cosmetic, but it fires on every lint and every gated commit.

The generated file uses ESM syntax with a `.js` extension, in a project whose `package.json` has no `"type": "module"` — which is the norm for Next.js apps:

```
(node:17824) [MODULE_TYPELESS_PACKAGE_JSON] Warning: Module type of
file:///.../.dharness/eslint.config.js is not specified and it doesn't parse as CommonJS.
Reparsing as ES module because module syntax was detected. This incurs a performance overhead.
To eliminate this warning, add "type": "module" to .../package.json.
```

The suggested remedy is wrong for this project — adding `"type": "module"` to a Next.js `package.json` to satisfy a tool-owned file is a far larger change than the warning warrants.

### Expected

Name the generated file `.dharness/eslint.config.mjs`. The extension is unambiguous, needs no `package.json` change, and the project config imports it by path either way.

---

## F6 — step 1 reports `+ bun.lockb` on every run, for a file that is never created

**Severity: low.** Noise, and it keeps a no-op step from ever reporting as satisfied.

Every `dharness sync` run, including runs where nothing changes:

```
 ✓ 1/11 install what this project is missing  0.04s
         │ bun add v1.4.0 (34cbb9a40)
         │ installed dharness-eslint-plugin@0.3.0
         │ ...
         = package.json
         + bun.lockb
         = bun.lock
```

`package.json` and `bun.lock` are correctly reported unchanged. `bun.lockb` is reported as created, and does not exist:

```
$ ls -la bun.lock*
-rw-r--r-- 1 User 197609 124683 Aug 20 21:48 bun.lock
```

bun 1.4 uses the text `bun.lock`; the binary `bun.lockb` is legacy. Because of this phantom entry, step 1 is marked *applied* on every run instead of *satisfied*, so a genuinely idempotent `sync` never reports as fully settled.

### Expected

Stat the file before listing it as created, and drop `bun.lockb` from the watched set when the project uses a text lockfile.

---

## F7 — `dharness init` is the natural first command and there is no hint

**Severity: nit.**

`init` is what a user reaches for on a project that has never seen the tool. The help text does describe `sync` as *"Set this project up"*, but that is one line down a list of four commands, and an unknown-command error prints usage without pointing anywhere in particular.

An `init` alias for `sync`, or an unknown-command message that names `sync` when the input is `init` / `setup` / `bootstrap`, would cost very little.

---

## Additional note — bun blocks lefthook's postinstall

Not a dharness defect, but it sits directly in the path of the step dharness delegates, and dharness already knows the package manager is bun.

`bun add -d lefthook` installs the package, but bun blocks lifecycle scripts by default, so lefthook's `postinstall.js` — which downloads the platform binary — never runs:

```
$ bun add -d lefthook
installed lefthook@2.1.10 with binaries:
 - lefthook
Blocked 1 postinstall. Run `bun pm untrusted` for details.
```

The binary is unusable until `bun pm trust lefthook`. When step 9 delegates hook-manager setup on a bun project, mentioning this would save the next person the detour.

---

## Final state of the run

```
■ 11 steps · 1 applied · 2 delegated · 8 satisfied · 0 failed
```

Remaining delegated steps, both reasonable and clearly explained:

- `10/11 install react-doctor's agent skill` — declined because its only non-interactive form installs five things including a competing git hook.
- `11/11 decide this project's architecture` — declined because intent cannot be read off a tree.

Step 6 still reports `config shape not recognised` even though the layer is now wired by hand, which is consistent with F2: the check looks for its own marker rewrite, not for whether the layer is in effect.

## What worked well

Worth saying, since the above is all defects: the delegation model is the best part of this tool. Steps 9, 10 and 11 each refuse to act and explain precisely *why* the decision is not dharness's to make. That is unusual and correct.

The two high-severity findings are both cases where a step failed to hold itself to that same standard — it claimed a result instead of either verifying it or handing it back.
