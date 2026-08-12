# Backlog: the staged mutation wrapper

Observations about `tools/mutationstaged` and `internal/testsupport/mutation`
that are worth acting on but were not worth derailing the change that found
them. Each entry records what was measured, not what was guessed. An entry
whose evidence turns out to be a misreading goes away with the misreading.

Nothing here is a promise. This is a list of things somebody noticed.

---

## 1. Line scope loses which file a byte offset came from

**Measured on 2026-08-10**, during the change that made `installStep` stop
reading `node_modules`.

`internal/setup/files.go` was staged with a **pure deletion**: the `installed`
helper was removed and nothing was added. Its staged diff is one hunk,
`@@ -66,8 +65,0 @@`, with no new-side lines at all. It therefore contributes
zero byte ranges to the scope.

The run mutated it anyway. Eight of the nine surviving mutants landed in
`gateInstalled`, `huskyWired` and `appendHuskyGate` — three functions the diff
never touched, in a file with no changed lines.

The likely mechanism, stated as a hypothesis rather than a finding:
`computeScope` merges the offsets of every staged file into one flat list
(`allOffsets = append(allOffsets, offsets...)`, then `encodeOffsetRanges` over
the merge), and `lineScoped.Incubate` tests `int(node.Pos()) - 1` against that
flat list with no file identity anywhere in the comparison. Ranges derived from
one file are then matched against node positions in another.

Why it matters: on any change touching more than one file, the mutation score
is measured over a set of nodes that is neither "the changed lines" nor "the
whole file". It is whatever the arithmetic happens to overlap. A score computed
that way can pass or fail for reasons unrelated to the change.

What would settle it: keep the ranges keyed by file and give the virus the file
identity of the node it is inspecting, or confirm that ooze parses each file
with a fresh `token.FileSet` and that the collision has some other cause.

**Second occurrence, 2026-08-11, with sharper evidence.** A change that
inserted a new method into `internal/project/git.go` — a pure insertion, the
staged diff being one hunk at `@@ -108,0 +109,29 @@` — produced two survivors
at line 91 of that same file, inside `StagedSourceFiles`, a function twenty
lines *above* the insertion and untouched by the change. Five other files were
staged in the same run.

So this is not a one-off, and it has a cost beyond noise: a score reported for
a slice is not a measure of that slice's coverage, and an author reading it is
pushed toward writing tests for code their change never touched. The first
occurrence could be read as a pure-deletion edge case; this one cannot.

## 2. The dry-run forecast and the real run disagree

Same run, same staged set:

    go run ./tools/mutationstaged -dry
      candidate mutants: 6 (kept nodes 1582, dropped nodes 40894)

    go run ./tools/mutationstaged
      Total: 32

`-dry` exists so the cost of a run can be known before paying it. A forecast
off by more than five times does not do that. Either `AnalyzeSource` counts
something narrower than what ooze goes on to mutate, or the two are scoped
differently — which would make this the same defect as entry 1 seen from the
other side.

## 3. A pure-deletion file has no defensible place in the mutable set

Independent of how 1 and 2 resolve: a file whose staged diff only removes lines
has no changed line to mutate. Today it is still handed to ooze as a mutable
file, which is how three untested husky helpers came to decide whether an
unrelated change could be committed.

Dropping such a file from the mutable set is one option. Failing open to whole
files is another, and is what `wholeFileScope` already does elsewhere. What is
not defensible is the current state, where the answer depends on arithmetic
nobody chose.

The coverage gap those survivors exposed was real and has since been closed, so
this entry is about the selection rule, not about those three functions.

## 4. `token.Pos` equals a byte offset only under an assumption nothing enforces

`TestPositionMinusOneIsByteOffsetWithFreshFileSet` names the assumption in its
own title: the identity holds when the `token.FileSet` contains one file, so
its base is 1. Nothing checks that ooze parses that way. If it ever reuses a
fileset across files, every position shifts by that file's base and the scope
silently addresses the wrong bytes — silently, because a wrong offset still
produces a valid mutant somewhere.

A cheap guard: have the virus assert that the position it is about to compare
belongs to the file whose ranges it holds, and fail loudly rather than drop or
keep the wrong node.

## 5. A red suite reports a perfect score

**Measured on 2026-08-11**, during the boundaries-ownership change.

The wrapper was run with one test already failing on unmutated code. It
reported **6 mutants, 6 killed, score 1.00** and exited 0 — the gate's own
verdict said the change was perfectly covered.

The arithmetic is doing exactly what it was told: a mutant "dies" when the
test command fails, and a suite that already fails kills every mutant without
any mutation being involved. The score is not wrong so much as meaningless,
and nothing in the output distinguishes the two.

The same run repeated against a green suite scored 0.83 with one survivor —
a real gap, in a real guard, which the perfect score had hidden.

This is the failure the repository names in its own first rules: a verdict
that has to be read out of context rather than off an exit code. A gate that
reports 1.00 when its inputs are broken is worse than one that reports
nothing, because it answers the question it was asked.

What would close it: run the test command once, unmutated, before releasing
ooze, and refuse to score at all if it does not pass. A baseline that fails
is not a low score — it is no measurement.

## 6. A `!p.HasSource()` guard is where the working directory leaks in

Not a wrapper defect — a pattern the wrapper has now caught twice, worth
naming so the next author writes the test without being told.

`project.Project.Source` is empty when a repository holds no JS project. Every
path built from it — `filepath.Join(p.Source, name)` — then resolves against
the **process working directory** rather than against nowhere. A guard clause
that returns early on `!p.HasSource()` is therefore load-bearing, and its
mutant survives any test whose temporary directory happens not to contain the
file being looked for.

Both survivors found this way were killed the same way: `t.Chdir` into a
temporary directory that *does* contain the file, so dropping the guard makes
dharness read an unrelated repository's config and the assertion fails.

The general shape: **a guard against an empty path needs a test that puts a
matching file where the unguarded code would look.** An empty temp directory
proves nothing, because the unguarded code finds nothing there either.

---

## 7. Hunk proximity sweeps untouched functions into scope

**Measured on 2026-08-12**, during the ESLint slice that deleted
`doctorConfigStep`'s neighbours in `internal/setup/steps.go`.

Staged scope is computed from the diff's line ranges, so a function the change
never touched is mutated when it sits close enough to a hunk. The run reported
a survivor in `lefthookExtendsStep.Satisfied` (`steps.go:243-246`), which that
diff did not modify.

The survivor is real, not an artefact. `Satisfied` is

    hookManager(p) != managerLefthook || extendsWired(root, lefthookConfig, target)

and the suite pins only the left side:
`TestLefthookExtendsSatisfiedWhenLefthookIsNotTheHookManager` covers the
short-circuit, `TestExtendsWiredIsFalseWithoutTheFile` covers `extendsWired`
alone. **No test reaches the case where lefthook *is* the hook manager and the
config is already wired**, so replacing the right operand with `true` survives.

Two separate things worth doing, and they are not the same thing:

1. The gap itself — one test with lefthook as the manager and a
   `lefthook.yml` that already extends the owned file.
2. The scoping behaviour. A survivor in code the diff never touched is
   indistinguishable, in the report, from one the change introduced. The
   author has to open each survivor to find out which it is, and the honest
   answer differs: your own new code gets fixed, pre-existing code gets
   recorded rather than silently absorbed into an unrelated review.

Entry 1 is about scope losing *which file* an offset came from. This is the
neighbouring problem: scope keeping code that no longer belongs to the change.

**Third occurrence, 2026-08-12, and the hypothesis is now confirmed.** Slice 2
of `structured-reports` staged five files together — the first change in this
repository to partially edit several existing files at once, rather than adding
new ones. The combined run reported **0.796875 (51/64)** and failed the 0.80
floor. Isolating the file groups along the dependency boundary gave **0.91
(10/11)** for the `ExitCode`-move pair and **0.94 (44/47)** for the setup
cluster. Same code, same tests; only the grouping changed.

The swept-in survivors were in `internal/app`'s `RunArgs` dispatch over
`"sync"`/`"check"`/`"mutate"` — untouched by the slice, and the smallest staged
file in the run. A byte offset legitimately changed in the much larger
`internal/setup/setup.go` collides numerically with an unrelated offset inside
`app.go`, which is exactly the mechanism entry 1 proposed as a hypothesis in
August.

What was still open in the hypothesis is now closed by reading the code rather
than inferring from symptoms. `computeScope` (`tools/mutationstaged/main.go:127`)
already computes per-file ranges correctly and already passes them per file to
`mutation.AnalyzeSource`, so the **statistics** are right. The defect is one
line later: `allOffsets` accumulates `offsetRange` values that carry `start` and
`end` and **no file identity at all**, and `plan.encoded` is one flat
`mergeOffsetRanges(allOffsets)` over the lot. The encoded scope handed to ooze
therefore cannot distinguish which file a range came from, while ooze parses
each file with its own `token.FileSet` and so produces offsets that are only
meaningful per file.

Why it went unnoticed for two days of work on this change: slice 1 added only
brand-new files, where every offset is legitimately in scope whichever file it
is attributed to. The defect needs a multi-file *partial* edit to show itself.

Two things this occurrence also settled, both worth stating plainly:

- **Nothing enforces the mutation floor automatically.** `.githooks/pre-commit`
  runs gofmt, vet and tests; `.github/workflows/ci.yml` runs gofmt, vet,
  `go test ./... -race` and `scripts/verify-gate.sh`. Neither invokes
  `go run ./tools/mutationstaged`. The floor is real but it is enforced by an
  author choosing to run it, which is why a failing combined score did not
  block the commit.
- **The failure mode is the expensive direction.** A collision inflates the
  denominator with untested code from an unrelated file, so the score reads
  *low*. An author trusting it writes tests for code their change never
  touched — the same cost entry 1 named, now measured at 0.797 against a real
  0.91/0.94.

A fix needs the ranges keyed by file and the file identity carried through to
the node comparison. Until then, a multi-file partial edit should be read per
file group, and a combined score below the floor is not evidence on its own.
