# Target report — approved

This is the approved shape for `dharness sync`'s report. It was designed by hand,
reviewed, and approved verbatim. **It is a design reference, not a byte-exact
contract**: wording and spacing may be adjusted where the real data demands it, but
the structure, the information carried, and the ordering are settled.

Downstream phases read this file rather than re-deriving the format. Do not
propose alternative formats.

## Where the conventions come from

Each convention below was taken from a CLI that dharness wraps, after running it
and reading its real output — not from memory.

**fallow** (the richest reference):

- Provenance header — `loaded config: <absolute path>`
- Explicit scope — `Audit scope: 40 changed files vs de0b18e7e792 (merge-base with origin/main)`
- Summary before detail — `■ Metrics: dead code 2 · complexity 4 (warn) · duplication 9`
- Section rules — `── Dead Code ──────`
- Stable addressable IDs per finding — `dup:c064407b`
- **Says what it did not do, and the flag to see it** —
  `note: hid 8 clone groups below minOccurrences=3 (lower --min-occurrences to see them)`,
  `audit gate excluded 15 inherited findings (run with --gate all to enforce)`
- Per-section verdict with its own timing, plus one final verdict
- Doc link per finding class, and the suppression syntax
- `--format json`, and a documented agent-contract JSON under `--walkthrough`
- **The two-audience split**, from fallow's own help: `audit` answers "will CI block
  this?" and gates with exit 1; `review --brief` answers "where do I look?" and
  always exits 0. One analysis, two renderings. `sync`'s report is the second
  audience and has never had that rendering.

**ESLint** (stylish formatter): groups by file with the path as a header; aligned
columns `line:col  severity  message  rule-id`; the rule ID is the machine handle in
a fixed final position; closes with a total and the flag that fixes it.

**react-doctor**: when there is nothing to report it *says so* —
`No staged source files found.` It does not print emptiness. This is the fix for an
absent step being ambiguous between "already done", "not applicable", and
"never reached".

## The report

```
dharness 1.2.0 · sync · D:\dev\disble\autoreas-sp\autoreas-bridge

  js project   frontend/            package manager  bun
  gate host    repository root      test runner      vitest
  presets      none matched         owned files      .dharness/
  read         frontend/.fallowrc.json · frontend/eslint.config.js
               frontend/doctor.config.json · .dharness/

■ 11 steps · 3 applied · 1 left to you · 7 already in place · 1 note   6.1s


── Applied ───────────────────────────────────────────────── 3 ──────

 ✓ 1/11  install what this project is missing                   5.56s
         │ bun add dharness-eslint-plugin
         │ installed dharness-eslint-plugin@0.3.0
         ~ frontend/package.json · frontend/bun.lock

 ✓ 2/11  write the files dharness owns                          0.02s
         + .dharness/eslint.config.js      62 lines, new
         + .dharness/fallow.jsonc          18 lines, new
         ~ .dharness/rules.json            thresholds changed
         = .dharness/lefthook.yml          already correct

 ✓ 6/11  point eslint.config.js at the file dharness owns       0.01s
         ~ frontend/eslint.config.js       2 regions spliced
           dharness:eslint-import   lines 1-3     inserted
           dharness:eslint-layer    lines 16-20   inserted

         legend  + created   ~ modified   = unchanged   │ subprocess


── Left to you ───────────────────────────────────────────── 1 ──────

 ! 4/11  `duplicates` has two owners        id  sync:collision/duplicates

         fallow's `extends` REPLACES a key, it does not merge it. Two
         declarations exist, one runs, nothing errors, and the file
         gives no sign of which.

           dharness  .dharness/fallow.jsonc:8
                     { "minOccurrences": 3, "mode": "semantic",
                       "threshold": 3 }
           project   frontend/.fallowrc.json:12-16
                     { "minOccurrences": 2, "mode": "exact",
                       "threshold": 5 }                  ← this one runs

         Pick one owner:
           a  delete `duplicates` from frontend/.fallowrc.json
           b  move your value into .dharness/fallow.jsonc, delete the copy

         Then re-run `dharness sync` to confirm.
         https://docs.dharness.dev/collisions#extends-replaces


── Already in place ──────────────────────────────────────── 7 ──────

 = 3/11   point .fallowrc.json at the file dharness owns
          extends → .dharness/fallow.jsonc
 = 5/11   point lefthook.yml at the file dharness owns
          extends → .dharness/lefthook.yml
 = 7/11   retire the legacy .eslintrc.json      not present
 = 8/11   give the agent fallow's own tools     .mcp.json declares fallow
 = 9/11   wire the gate into git                .git/hooks/pre-commit
 = 10/11  install react-doctor's agent skill    .claude/skills/ present
 = 11/11  decide this project's architecture    boundaries declared


── Notes ─────────────────────────────────────────────────── 1 ──────

 i residue  frontend/doctor.config.json declares 7 dharness entries this
            version no longer writes. They are inert: the gate runs
            react-doctor with --staged, and plugin rules do not fire
            under that flag.
            dharness will not remove them — it cannot tell its own
            earlier write from a value you set afterwards (§05).
            note: 7 entries hidden (use --show residue to list them)


─────────────────────────────────────────────────────────────────────
✓ 3 applied · 1 left to you · 7 in place · 1 note        6.1s  exit 0

  next  resolve sync:collision/duplicates, then `dharness sync`
  gate  `dharness check` — eslint · fallow dupes · fallow audit ·
        react-doctor, cheapest first
```

## The failure variant

The closing block **explicitly retracts** the tick already printed for step 1. A
step's status line is printed before its outcome is known, which is unavoidable if
progress is to be live; what is avoidable is leaving that line asserting something
false. Today the error message says *"No earlier step is reported as having
succeeded"* while stdout has just reported them.

```
── Applied ───────────────────────────────────────────────── 0 ──────

 ✓ 1/11  install what this project is missing                   5.56s
 ✗ 2/11  write the files dharness owns                          0.01s
         .dharness/fallow.jsonc: permission denied

─────────────────────────────────────────────────────────────────────
✗ rolled back — nothing was applied, including step 1     5.7s  exit 1

  restored  frontend/package.json · frontend/bun.lock
  removed   dharness-eslint-plugin@0.3.0
  left      .dharness/  created, not removed (directories are not
            rolled back — see the writer-undo-completeness backlog entry)

  1 failed · 0 applied · 9 not reached
```

## The machine twin — `dharness sync --format json`

Rendered from the same single analysis as the human view. Never a second
computation.

```json
{
  "version": "1.2.0",
  "root": "D:/dev/disble/autoreas-sp/autoreas-bridge",
  "source": "frontend",
  "summary": { "steps": 11, "applied": 3, "delegated": 1,
               "satisfied": 7, "failed": 0, "ms": 6100 },
  "steps": [
    { "n": 1, "id": "install", "status": "applied", "ms": 5560,
      "wrote": ["frontend/package.json", "frontend/bun.lock"],
      "installed": ["dharness-eslint-plugin@0.3.0"] },
    { "n": 4, "id": "sync:collision/duplicates", "status": "delegated",
      "key": "duplicates",
      "ours":   { "path": ".dharness/fallow.jsonc", "line": 8,
                  "value": { "minOccurrences": 3, "mode": "semantic", "threshold": 3 } },
      "theirs": { "path": "frontend/.fallowrc.json", "line": 12,
                  "value": { "minOccurrences": 2, "mode": "exact", "threshold": 5 } },
      "effective": "theirs",
      "resolutions": ["delete-theirs", "move-into-ours"] },
    { "n": 3, "id": "fallow-extends", "status": "satisfied",
      "evidence": "extends → .dharness/fallow.jsonc" }
  ],
  "notes": [
    { "kind": "residue", "path": "frontend/doctor.config.json",
      "entries": ["dharness-eslint-plugin", "dharness/max-file-lines"],
      "actionable": false,
      "reason": "react-doctor runs with --staged; plugin rules do not fire" }
  ]
}
```

`effective` is the highest-value field in the document. It answers the one question
the current report raises and never resolves: *I know there are two values — which
one runs?* Per the exploration, that answer is available by measurement from
`fallow config --format json`, which resolves `extends` and prints clean JSON on
stdout. It must never be inferred.

## The thirteen defects this replaces

Each was verified against the code before the format was designed.

1. Satisfied steps are invisible — `setup.Pending` filters them and nothing prints
   them. Of 11 steps, a real run showed 3 lines and 1 delegated block; the other 7
   were an ambiguous absence.
2. `Applying:` lists intentions, not outcomes — `applySteps` prints the step ID
   before calling `Apply`. No status glyph, no path.
3. On rollback the output contradicts itself — steps already printed under
   `Applying:` are then declared never to have succeeded.
4. The `Writer` records every path it touches, for rollback, and the report names
   none of them.
5. Sink leak — `installStep.Apply` and `hookInstallStep.Apply` write to `os.Stdout`
   rather than the writer `applySteps` was handed. `Step.Apply` has no output
   channel at all.
6. The delegated block renders every colliding key twice, in near-identical prose,
   from two independent renderers.
7. The delegated heading asserts "two architectures" when the collision was one key
   with two values.
8. The project's value prints truncated to the fragment `"duplicates": {`, and the
   honest fallback only fires when no line matches at all.
9. Paths are unresolvable — `.fallowrc.json` and `doctor.config.json` never say
   which directory they are in.
10. No final tally, no exit-code statement, no next command.
11. No machine-readable output on any command, which contradicts the repository's
    own doctrine that the verdict comes from exit codes and JSON, never prose.
12. Three inconsistent heading levels, with the applied steps the only block having
    no heading at all.
13. The residue section ends on five lines of justification, when the last thing
    read should be the action.
