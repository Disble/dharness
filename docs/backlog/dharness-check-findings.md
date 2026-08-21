# `dharness check` — findings from running the commit gate

Report prepared for the dharness maintainers. Companion to `dharness-sync-findings.md`, which
covers `sync`; everything here concerns `check` and the gate it drives.

## Environment

| | |
|---|---|
| dharness | 1.5.1 |
| project | `create-next-app` output, Next.js 16.3.1, React 19.2.8 — **5 source files** |
| package manager | bun 1.4.0 |
| node | v22.19.0 |
| hook manager | lefthook 2.1.10, `pre-commit` → `dharness check` |
| OS | Windows 11 Pro 26200 |

## Summary

The gate is correct — it caught a real `dharness/require-jsdoc` violation and blocked the
commit that introduced it, which is exactly the job. The problem is what it costs to do that.

**A commit touching one file takes ~23 seconds, and roughly 80% of that is dependency
resolution, not analysis.** The tools do their actual work in about 4.6s combined; the rest
is `bunx` re-resolving `@latest` packages against the registry, three separate times, on
every single commit.

---

## C1 — ~80% of gate runtime is repeated `@latest` dependency resolution

**Severity: high for adoption.** A 23s pre-commit hook on a 5-file project is a hook people
will start bypassing with `--no-verify`.

### Measurements

Steady state, one staged file, warm caches, three consecutive runs:

```
=== run 1 === exit=0  elapsed=23080ms
=== run 2 === exit=0  elapsed=23034ms
=== run 3 === exit=0  elapsed=22594ms
```

Mean ≈ **22.9s**. The first ever run, on a cold cache, took **62.7s** — that one is
forgivable; the 23s steady state is the number that matters.

Against that, the work the tools report doing:

| Stage | Self-reported work |
|---|---|
| eslint | 2.21s (measured in isolation: `./node_modules/.bin/eslint eslint.config.mjs`) |
| react-doctor | `✔ Scanned 1 file in 1.6s` |
| fallow audit — dependencies | `✗ 1 dev dependencies in production (0.19s)` |
| fallow audit — styling | `✓ No issues in 1 changed file (0.62s)` |
| fallow dupes | `✓ No code duplication found (0.02s)` |
| **total actual analysis** | **≈ 4.6s** |

That leaves roughly **18 seconds unaccounted for by any stage's own timing**.

### Where it goes

`Resolving dependencies / Resolved, downloaded and extracted [2]` appears **three times** in
a single `dharness check` run — once before react-doctor, once before fallow audit, once
before fallow dupes:

```
── react-doctor ──
Resolving dependencies
Resolved, downloaded and extracted [2]
Scanning 1 staged files...
...
── fallow audit ──
Resolving dependencies
Resolved, downloaded and extracted [2]
loaded config: .../.fallowrc.json
...
── fallow dupes ──
Resolving dependencies
Resolved, downloaded and extracted [2]
loaded config: .../.fallowrc.json
```

Neither tool is a project dependency, so each invocation goes out to the registry:

```
$ ls node_modules/fallow node_modules/react-doctor
fallow         NOT a dependency — fetched per run
react-doctor   NOT a dependency — fetched per run

$ grep -E "fallow|react-doctor" package.json
    "eslint-plugin-react-doctor": "^0.9.12",
```

Only the ESLint *plugin* is installed. The `fallow` and `react-doctor` CLIs are not, and the
`@latest` specifier — visible in the `.mcp.json` dharness writes, `bunx --package
fallow@latest fallow-mcp` — forces a fresh resolution rather than reusing a pinned local
copy.

### Expected

Any of these would recover most of the 18s:

1. **Resolve once per run.** fallow is invoked twice (audit, then dupes) and pays the
   resolution cost both times. Even without other changes, collapsing that to one resolution
   removes a third of the overhead.
2. **Install the CLIs as devDependencies during `sync`.** `sync` already runs `bun add` for
   three packages in step 1; adding `fallow` and `react-doctor` there means `check` runs
   local binaries with no registry round-trip. This also makes the gate work offline and
   makes CI reproducible.
3. **Drop `@latest`.** A gate that silently upgrades its own analyzers on every commit is
   also a gate whose results are not reproducible between two developers on the same commit.
   Pin, and let `sync` be the thing that moves the pin — which fits the tool's existing model,
   where `sync` is the mutating command and `check` is not.

Point 3 is worth weighing on its own merits even setting performance aside.

---

## C2 — dharness passes fallow two flags that fallow itself reports as conflicting

**Severity: medium.** Printed on every gated commit; the scoping behaviour is ambiguous.

fallow warns about the argument list dharness constructs:

```
── fallow audit ──
fallow: --diff-file precedes --changed-since for line-level filtering; --changed-since
still scopes file discovery. Drop one of them to disable this combination.
```

dharness's own help text states *"dharness owns invocation only. Each wrapped tool keeps its
own configuration, written by that tool's own installer."* Invocation is precisely what this
is: dharness passes both `--diff-file` and `--changed-since`, and fallow explicitly asks for
one of them to be dropped.

Beyond the noise, it leaves the audit's real scope unclear to the user — the message says
one flag wins for line-level filtering while the other still scopes file discovery, so what
was actually audited depends on knowing fallow's precedence rules.

### Expected

Pass one or the other. If both are genuinely needed, the warning is a signal to raise with
fallow rather than to ship past on every commit.

---

## C3 — react-doctor's agent-guidance block prints on every human commit

**Severity: low.** Pure noise, but it dominates the gate's output.

Every `dharness check` run prints 13 lines of instructions addressed to an AI agent, in the
middle of a `git commit`:

```
Agent guidance
  - Treat React Doctor diagnostics as starting hypotheses. Read the relevant code before
    confirming or suppressing each finding.
  - For each group, decide true positive, false positive, or needs-human-review, then assign
    high/medium/low confidence.
  - Do not suppress a finding without evidence from the file in question. ...
  - Understand the root cause before editing. ...
  ... (13 lines)
```

followed by react-doctor's own footer:

```
Score disabled by --no-score. Want something custom to your company? Contact us at
https://react.doctor/enterprise.
  ────────────────────────────────────────────────────────────
  Docs: https://react.doctor/docs
  Learn more about fixing issues, setting up CI/CD, and configuring rules with a config file
  GitHub: https://github.com/millionco/react-doctor
  Report issues and star the repository!
```

On a clean run, this is the overwhelming majority of what the gate prints — the actual
result is the single line `✔ No issues found!`. A developer running `git commit` is not the
audience for either block.

### Expected

dharness already passes `--no-score`, so it is clearly shaping react-doctor's output
already. If react-doctor exposes a quieter mode, use it; if it does not, that is worth
asking them for. Failing both, filtering the guidance and marketing blocks out of the gate's
relayed output would be reasonable — dharness is the thing that decided to run this tool
inside a commit.

---

## C4 — the `.dharness/eslint.config.js` module-type warning fires on every gated commit

**Severity: low.** Cross-reference: this is F5 in `dharness-sync-findings.md`, filed there
against the generated file. Noting it here because the gate is where it actually gets seen:

```
── eslint ──
(node:11128) [MODULE_TYPELESS_PACKAGE_JSON] Warning: Module type of
file:///.../.dharness/eslint.config.js is not specified and it doesn't parse as CommonJS.
Reparsing as ES module because module syntax was detected. This incurs a performance overhead.
```

Note the last sentence: Node states this costs performance, on a path that runs on every
commit. Renaming the generated file to `.dharness/eslint.config.mjs` fixes it.

---

## What worked well

The gate did its job. The first commit of the harness setup was **blocked**, correctly:

```
dharness: eslint exited with code 1
D:\...\eslint.config.mjs
  10:1  error  This variable is declared at the top of the file with nothing saying what it
               is for  dharness/require-variable-jsdoc

eslint failed, so react-doctor and fallow audit and fallow dupes did not run.
There may be more to fix behind it.
```

Two details worth keeping:

- **Fail-fast with an honest statement of what did not run.** "eslint failed, so react-doctor
  and fallow audit and fallow dupes did not run. There may be more to fix behind it" is
  better than most tools manage. It does not pretend the remaining checks passed.
- **The fast path is genuinely fast.** With nothing source-shaped staged, the gate exits in
  0.17s with `no staged source files, nothing to check`. A docs-only commit is not taxed at
  all.

The wrapper's boundary is also well drawn — `dharness wraps the gate, not eslint itself. For
anything beyond pass or fail — why a finding fired, what it means, which rules exist — ask
the tool` is the right thing to tell someone, and it points at the real binary path.

None of C1–C4 are about what the gate checks. They are about the cost of getting to the
answer, and that cost is what decides whether the gate survives contact with a real team.
