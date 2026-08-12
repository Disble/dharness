# Exploration: eslint-integration

> Reconstructed by the orchestrator from the exploring agent's Engram record
> (`sdd/eslint-integration/explore`, #8037). The agent believed it had written
> this file and had no write access, so its full body did not survive; what
> follows is its recorded summary, kept in its own words where they were
> recorded, plus the orchestrator's note where the reconstruction is partial.
> The **Correction to the brief** section is the agent's finding, not the
> orchestrator's, and it overturns how this change was framed.

## Why the change exists

`dharness check` invokes react-doctor with `--staged`, and **plugin rules
declared in `doctor.config.json` do not run under that flag**. Measured
against react-doctor 0.5.7 with the plugin installed and resolvable and
`maxFileLines: 1` — a limit no file can satisfy:

| invocation | plugin rules | exit |
|---|---|---|
| the same flags without `--staged` | `max-file-lines` and `require-jsdoc` fire | **1** |
| the same flags with `--staged` | "No issues found" | **0** |

react-doctor's own native rules do run under `--staged`, so what the flag
drops is the plugins. dharness therefore installs a plugin, writes six rule
severities, and gates on none of them.

**The mechanism is an open question.** Two runs emitted
`config.plugins entry "dharness-eslint-plugin" could not be resolved from
C:\…\react-doctor-staged-XXXXXX` alongside a temp-directory path under
`--verbose`, which suggests `--staged` materialises files where
`node_modules` is unreachable. That did not reproduce on later runs. The
outcome is established; the cause is not, and no artifact downstream may
assert it.

## Correction to the brief — the departure is not the one that was named

The brief framed this change as breaking §03, "the project's own files only
ever gain one line pointing at the files dharness owns". **That framing is
imprecise.** §03 was already amended twice (9 and 10 August 2026) to cover
exactly the shape proposed here: dharness writes its own file, and the
project's file gains one reference to it. `fallowExtendsStep` and
`lefthookExtendsStep` are that pattern.

The real departure is narrower and was not investigated in the brief:

**Those two steps only ever write a project file when it does not exist
yet.** The instant the target file is already there, `Delegated(p)` returns
true — the merge becomes the agent's job and dharness never touches it.

Virtually every real ESLint 9 project already has an `eslint.config.js`. So
applying the existing precedent literally means **dharness would delegate
nearly every time**, and almost never write the spread reference itself.

The working demonstration in the brief — importing `./.dharness/eslint.config.js`
and spreading it into an existing project config — assumes dharness **splices
into a file the project wrote**, and that has no precedent in this codebase.
The closest analogue, the delimited-region technique in `owned.go`
(`presetBegin`/`presetEnd`), is today reserved for a file dharness itself
created and owns.

## Affected areas

- `internal/cli/check.go` — the gate's stage list and its ordering rationale.
- `internal/tool/tool.go`, `command.go` — `ReactDoctorStaged`, `RemoteLatest`,
  per-manager remote execution. An ESLint stage fits this shape with no new
  abstraction.
- `internal/setup/steps.go` — `fallowExtendsStep`, `lefthookExtendsStep`, and
  `legacyLintConfigStep`, which is direct precedent for detecting a lint config
  react-doctor silently drops and delegating the fix; relevant to `.eslintrc.*`.
- `internal/setup/files.go` — `extendsWired`, `wireFallowExtends`,
  `wireLefthookExtends`, `ownedFrom`: the exact write-if-absent,
  delegate-if-present pattern.
- `internal/setup/owned.go` — the delimited-region technique, reserved today
  for dharness-owned files.
- `internal/setup/plugin.go` — `Rules`, `RulesPrefix`, `RulesPackage`,
  `DefaultSeverity`. The six severities already live in `doctor.config.json`
  and would gain a second home, which is the two-homes-for-one-severity drift
  this repository keeps finding.
- `internal/preset/` — `Manifest` carries only `Facts` (fallow keys) and
  `Seeds` (prompt text). No preset installs a package or contributes framework
  config today; `eslint-config-next` / `eslint-config-expo` as an installable
  dependency plus a config layer is **a contribution shape the `Preset`
  abstraction does not have**.
- `docs/design-principles.md` §03 — already amended twice; does not need a
  third reversal, because the brief's framing was inaccurate.
- `internal/setup/golden_test.go` — the frozen generic golden. Any change to
  what a project with no framework and no ESLint config receives must leave
  those fixtures untouched.

## Approaches

1. **Write-if-absent, delegate-if-present** — reuse the exact extends-wiring
   precedent. Safe, consistent with everything already shipped, and delegates
   in nearly every real project.
2. **Direct textual splice into an existing config** — a new mechanism, no
   precedent, and the blast radius is the project's whole lint and often its
   pre-commit.
3. **Hybrid, with an idempotency marker** — splice, but delimited so a re-run
   replaces only dharness's block.

**Recommended: approach 1**, as the safe default. Approaches 2 and 3 need
explicit sign-off, because they invent a mechanism for the highest-blast-radius
file in the project.

## The parser, settled by the orchestrator after this exploration

Open question 1 — how to edit `eslint.config.js` at all — is answered, and it
settles the approach with it: **dharness splices into the project's config**,
so approach 2/3 rather than approach 1.

Every candidate was checked for ESM support, source positions, importability
and whether it is still maintained:

| Library | ESM | Positions | Importable | Alive |
|---|---|---|---|---|
| `tdewolff/parse/v2/js` | yes | **no** | yes | yes |
| `dop251/goja` | **no** — fails on `export` | yes | yes | yes |
| `robertkrimen/otto` | **no** — ES5 | yes | yes | yes |
| `evanw/esbuild` | yes | yes | **no** — parser under `internal/` | yes |
| `microsoft/typescript-go` | yes | yes | **no** — `internal/`, no tags | yes |
| `kdy1/go-typescript-eslint` | yes | yes | yes | **no** — 4 days of commits, then 10 months silent |
| `malivvan/tree-sitter` | yes | yes | yes | **no** — created and abandoned the same day |

The first search looked in the wrong family: esbuild and typescript-go are a
bundler and a compiler, not parser libraries. The right family is tree-sitter,
whose whole purpose is parsing source for editors — which is why every node
carries a byte range and why a partial tree is returned instead of an error.

**Chosen: `github.com/odvcencio/gotreesitter`** — a pure-Go tree-sitter
runtime, 552 stars, updated the day this was written. Verified by building a
probe against the reference repository's own `eslint.config.js`:

- **Compiles with `CGO_ENABLED=0`**, so `.goreleaser.yml` is untouched and the
  six platform binaries still cross-compile from one runner. The official
  `tree-sitter/go-tree-sitter` bindings need cgo and were rejected for this.
- **Byte ranges on every node** (`StartByte`/`EndByte`), so the splice is
  surgical and the rest of the file survives byte-for-byte.
- **Grammars are embedded** — `grammars.JavascriptLanguage()` needed no
  separate fetch.
- **Comments are array elements in their own right.** The probe reported the
  reference file's `export default` array as objects *and* comments with
  their own ranges, which means an insert can respect the comments the project
  wrote rather than landing on top of them.

Costs, accepted:

- It is **the product's first dependency**. dharness has been stdlib-only,
  with ooze confined to development tooling.
- It is a **single-author reimplementation** of a complex runtime. Active and
  popular, but not the official binding. If it stops, the fallback is cgo with
  the official bindings, which costs the single-binary cross-compile.

## Open questions

1. ~~How does dharness edit `eslint.config.js` without a JS parser?~~
   **Settled above.** What remains of it: what the splice does with
   `eslint.config.ts`, and with a config whose default export is a function
   call rather than an array literal.
2. What happens when there is no `eslint.config.js` at all — write one, or
   delegate?
3. Legacy `.eslintrc.*` cannot spread anything. Detect and delegate, or ignore?
4. Do the six rules stay in `doctor.config.json` as well?
5. Where does the ESLint stage sit in the gate, and what does it cost?
6. How does a preset contribute a package to install, given `Fact` is a fallow
   config key today?
7. What makes a malformed edit survivable, and what proves the edit is
   idempotent?
