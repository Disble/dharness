# `dharness mutate` — findings from a first real mutation-testing session

Report prepared for the dharness maintainers. Third in a set: `dharness-sync-findings.md`
covers `sync`, `dharness-check-findings.md` covers the commit gate, and everything here
concerns `mutate`.

## Environment

| | |
|---|---|
| dharness | 1.5.1 |
| project | Next.js 16.3.1, React 19.2.8, TypeScript 5, ~30 source files |
| package manager | bun 1.4.0 |
| test runner | vitest 4.1.11 |
| OS | Windows 11 Pro 26200 |

## Summary

`mutate` did its job and did it well: it found four genuine holes in a test suite that was
already green, including one that had let a real bug through. That part is not the story.

The story is that **the printed table silently mixes the current run with cached results from
previous runs, and the state that causes it lives inside `.git/`.** I read the same score
twice in a row, concluded my new tests had done nothing, and went looking for a bug in my
own test code. There was no bug. I was reading a stale row.

---

## M1 — `mutate <one-file>` prints a table of files you did not ask about

**Severity: high.** It is the difference between "my tests did nothing" and "my tests
worked", and the output gives you no way to tell which you are looking at.

### Reproduction

Two runs, each naming exactly one file, from a clean `reports/`:

```
$ rm -rf reports
$ dharness mutate application/exam-session/seed.ts

All files             |  99.03 |   99.03 |      205 |  0 |  2 | 0 | 0 |
 exam-session         |  99.01 |   99.01 |      100 |  0 |  1 | 0 | 0 |
  seed.ts             |  94.74 |   94.74 |       18 |  0 |  1 | 0 | 0 |
  session-progress.ts | 100.00 |  100.00 |       59 |  0 |  0 | 0 | 0 |
  session-view.ts     | 100.00 |  100.00 |       23 |  0 |  0 | 0 | 0 |
 marking              |  99.06 |   99.06 |      105 |  0 |  1 | 0 | 0 |
  mark-exam.ts        |  99.06 |   99.06 |      105 |  0 |  1 | 0 | 0 |
```

One file was named. Seven rows came back, and 205 mutants are reported for a file that has
19. The three extra files were mutated in earlier, separate invocations.

### Why `rm -rf reports` does not help

This was the part that cost the most time. The obvious reset — deleting the report directory
the tool points you at — changes nothing, because the state is not there:

```
$ find . -iname '*stryker*' -not -path './node_modules/*'
./.git/dharness/stryker-incremental.json      (146 KB, four files tracked)
```

Deleting **that** file gives the expected output:

```
$ rm -f .git/dharness/stryker-incremental.json && rm -rf reports
$ dharness mutate application/exam-session/seed.ts

All files |  94.74 |   94.74 |       18 |  0 |  1 | 0 | 0 |
  application/exam-session/seed.ts:12 ConditionalExpression
```

One file named, one file reported, 19 mutants. Correct.

### Expected

Incremental state is a good default for a tool that costs real time to run — the objection
is not to the cache, it is to reading it back as if it were this run's result. Any of these
would fix it:

1. **Scope the table to the paths the invocation named.** Everything else is cached context;
   printing it as a result is the whole bug.
2. **Mark cached rows.** If the wider table is deliberate, a `(cached)` column would make
   the stale rows readable instead of misleading.
3. **Say where the state lives, and offer a flag to drop it.** `.git/dharness/` is a
   defensible location — out of the working tree, out of the diff — but nothing in the
   output, the `--help` text or the report path hints at it, and it is the last place
   somebody looks. A `--no-incremental` or `--reset` flag would remove the guesswork
   entirely.

The `--help` text currently documents `-concurrency`, `-dry-run` and `-upgrade`. None of
them turn the cache off.

### Aggravating detail

The final verdict line is computed from the current run, while the table is computed from
the cache. So a run can print

```
  seed.ts | 94.74 | ... | 1 survived
  ...
Every mutant was caught: these tests notice this code breaking.
```

— a table showing survivors directly above a line saying there were none. Both are true of
different things, which is exactly what makes it hard to notice.

---

## M2 — the survivor list is line numbers only, with no way to see the mutation

**Severity: medium.** It turns a fast loop into a slow one.

The output ends with:

```
15 mutant(s) survived — a test would not have noticed:

  application/marking/mark-exam.ts:59 MethodExpression
  application/marking/mark-exam.ts:61 Regex
  application/marking/mark-exam.ts:63 Regex
```

`MethodExpression` at line 59 does not say which method, or what it became. Line 59 in that
file was a five-line chained expression, so "a method call somewhere in here was removed" is
not enough to act on. Working out what each mutant actually was meant parsing the JSON by
hand:

```python
d = json.load(open('reports/mutation/mutation.json'))
for path, f in d['files'].items():
    for m in f['mutants']:
        if m['status'] == 'Survived':
            print(m['location']['start']['line'], m['mutatorName'], '->', m['replacement'])
```

which gives what the terminal could have:

```
59  MethodExpression      -> text.toLowerCase().replace(...).trim().replace(...)   [final .trim() dropped]
84  ConditionalExpression -> false
88  ConditionalExpression -> true
142 MethodExpression      -> [...missed]                                            [.sort() dropped]
```

That one extra column — the replacement — is the difference between a survivor list you can
act on and one you have to go decode.

### Expected

Print `replacement` beside each survivor, at least behind a `--verbose`. The data is already
in the JSON the run just wrote.

---

## M3 — no signal for equivalent mutants

**Severity: low, but it shapes behaviour.** Chasing 100% is the wrong instinct and the tool
currently encourages it.

Two of the survivors here cannot be killed, because they do not change behaviour:

```js
// mark-exam.ts:89
return answer.kind === "choice" && answer.key === item.answerKey;
//     ^^^^^^^^^^^^^^^^^^^^^^^^ mutated to `true`
```

An answer of any other kind has no `key`, so `undefined === "B"` is false either way. The
mutant is equivalent. Same story in `seed.ts:12`, where short-circuiting past an `undefined`
check leaves the regex to reject `"undefined"` a moment later.

The tool reports these identically to real gaps, and its exit code fails on them. On a
codebase with a few dozen such mutants that is a standing red build nobody can clear, and
the usual response is to stop running the tool.

### Expected

No request for automatic equivalence detection — that is undecidable in general. But a
documented suppression (`// dharness-ignore-next-line mutant <reason>` in the spirit of the
`fallow-ignore-*` markers already in the toolchain) would let a team record "this one is
equivalent, here is why" and keep the build meaningful. Right now the only options are an
unclearable failure or not running `mutate` in CI.

---

## What worked well

The core of the tool is genuinely good and found things nothing else did.

**It caught a real bug that a green suite had missed.** A normalisation function stripped
trailing punctuation before trimming whitespace, so `" Keen. "` kept its full stop and a
correct answer was marked wrong. The unit tests passed; the mutant did not survive.

**It found four test-quality holes worth fixing**, each of which turned into a real test:

- A word-limit guard that was never exercised, because every over-long answer in the tests
  was also absent from the accepted list — so the guard could be deleted with no failure.
- A `.sort()` whose test data was already in sorted order.
- Fixtures using `startedAt: 0`, which makes `now - startedAt` and `now + startedAt`
  identical. A whole class of arithmetic mutants was unkillable purely because of a zero in
  a fixture. That is a lesson that generalises well beyond this project.
- Two answer shapes (`written` and `text` with content) that only ever appeared in tests
  asserting the negative case.

Scores after acting on all of it: `session-progress.ts` and `session-view.ts` at 100%,
`mark-exam.ts` at 99.06%, `seed.ts` at 94.74% — the two remainders being the equivalent
mutants above.

**The scoped invocation is the right shape.** `dharness mutate <path...>` with line ranges
(`src/thing.ts:12-40`) is exactly how mutation testing becomes usable day to day, and
`-dry-run` reporting how many tests a scoped run would execute is a thoughtful touch.

**The failure message is honest.** "N mutant(s) survived: a test would not have noticed this
code breaking" says the useful thing, in the terms that matter, without jargon.

---

## One note on the runner requirement

Not a defect, but worth documenting somewhere findable. The first invocation failed with:

```
dharness: Stryker found no supported test runner in package.json;
declare vitest or jest, or set testRunner in a JSON Stryker config
```

The project was using `bun test`, whose API is close enough to vitest that migrating nine
test files meant changing one import line in each. The message is clear about what to do,
but a line in the `mutate --help` text saying the command needs vitest or jest would save
people discovering it at the moment they wanted to use the tool.

None of M1–M3 are about what `mutate` measures. The measuring is the part that works. They
are about reading the answer, and M1 in particular can send you debugging tests that were
never broken.
