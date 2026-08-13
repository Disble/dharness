# Backlog: the staged mutation wrapper

Observations about `tools/mutationstaged` and `internal/testsupport/mutation`
that are worth acting on but were not worth derailing the change that found
them. Each entry records what was measured, not what was guessed. An entry
whose evidence turns out to be a misreading goes away with the misreading.

Nothing here is a promise. This is a list of things somebody noticed.

The wrapper exists to make mutation testing **cheap enough to run**, and it
buys that by mutating only what is staged. Cost is therefore the axis these
entries are read on: a defect that inflates the mutant count attacks the only
reason the tool exists. Correctness of attribution matters too, but it is the
second question, not the first.

**Validated 2026-08-13.** Entries 1 through 5 and 7 were turned into
falsifiable predictions and measured against a controlled fixture rather than
against a real change. The fixture is three files of **exactly 409 bytes each,
byte-identical in layout** (`internal/exp/{alpha,beta,gamma}.go` on the
throwaway branch `exp/mutation-hypotheses`), so function *k* of one file
occupies the same byte range as function *k* of every other. That makes the
prediction arithmetic instead of impressionistic, and it deliberately builds
the **worst case** for cross-file collision. Three entries did not survive
contact with measurement and have been corrected below.

---

## 1. The flat scope makes the mutant count quadratic in staged files

**Confirmed 2026-08-13 with a control.** Previously filed as "line scope loses
which file a byte offset came from", first noticed 2026-08-10.

`computeScope` (`tools/mutationstaged/main.go:127`) computes per-file line
ranges correctly and passes them per file to `mutation.AnalyzeSource`, so the
**forecast** is right. The defect is one line later. `allOffsets` accumulates
`offsetRange` values carrying `start` and `end` and **no file identity**, and
`plan.encoded` is a single flat `mergeOffsetRanges(allOffsets)` over the lot
(`main.go:158`). `TestStagedMutation` then builds one `ScopeAll` from those
ranges and hands the same virus instances to every file ooze mutates.

That the collision is unavoidable, rather than merely possible, is settled by
ooze itself: `GoSourceFile.Incubate` calls `token.NewFileSet()` **fresh for
every file** (`internal/gosourcefile/gosourcefile.go:26`). Every file's byte
offsets therefore restart at zero, so a merged flat range list necessarily
addresses every file at once. This is what entry 4 was worried about, resolved
in the direction that proves this entry.

The measured cost, one changed line per file, each at a distinct position:

| Staged files | `-dry` forecast | Mutants actually run | Ratio |
|---|---|---|---|
| 1 | 4 | 4 | 1× |
| 2 | 8 | 16 | 2× |
| 3 | 12 | **36** | 3× |

The intended work is `4N`. The work performed is `4N²`. Each of the N files is
measured against all N ranges, so **total mutant executions grow quadratically
in the number of staged files**, and every mutant is one full run of the test
command. The three-file run took 27s of wall clock for 36 mutants where 12 were
called for.

The single-file control is what makes this a finding rather than a coincidence:
with one staged file the forecast and the run agree exactly (4 and 4). Nothing
about the wrapper is broken until a second file joins.

The `4N²` figure is the ceiling, realised here because the fixture is
byte-identical by construction. Real files have different layouts and realise a
fraction of it — the slice-2 measurement below is one such fraction — but the
growth is in the wrong direction either way.

**Earlier occurrences, now explained by the above.** A pure deletion in
`internal/setup/files.go` (2026-08-10) produced eight survivors in
`gateInstalled`, `huskyWired` and `appendHuskyGate`. An insertion into
`internal/project/git.go` (2026-08-11) produced two survivors twenty lines
above the insertion, with five other files staged. Slice 2 of
`structured-reports` (2026-08-12) staged five files and reported
**0.796875 (51/64)**, failing the 0.80 floor; isolating the file groups along
the dependency boundary gave **0.91 (10/11)** and **0.94 (44/47)** for the same
code and the same tests, with only the grouping changed.

Two consequences, in the order they cost something:

- **The run is slower than the tool promises.** That is the whole product.
- **The score is measured over the wrong node set**, so it reads low, and an
  author trusting it writes tests for code their change never touched.

A fix needs the ranges keyed by file and the file identity carried through to
the node comparison. The virus signature (`Incubate(node ast.Node)`) carries no
file identity and ooze does not expose its `FileSet` to viruses, so this
probably means one scoped pass per staged file rather than one merged pass.
Until then, a multi-file edit should be read per file group, and a combined
score below the floor is not evidence on its own.

## 2. The forecast is right and the run overcharges

**Corrected 2026-08-13. The original entry blamed the wrong side.**

It read: `-dry` reported 6 candidates where the real run produced 32, and
concluded that "a forecast off by more than five times" fails at the one job it
has. The measurement says the opposite. `AnalyzeSource` is called per file with
that file's own ranges, so `-dry` reports the number of mutants the staged
change actually justifies. It is the execution that inflates, by exactly the
factor in entry 1.

Measured: 3 staged files, `-dry` says 12, the run performs 36.

This matters more under a cost objective than under a correctness one. `-dry`
exists so the price of a run can be known before it is paid. The price it
quotes is honest; the run then charges N times it. For a tool whose reason to
exist is fitting inside a gate, a forecast the execution does not honour is the
central defect, not a reporting nuisance.

Nothing needs fixing in `-dry`. Fixing entry 1 fixes this entry.

## 3. A pure deletion is harmless alone and expensive in company

**Corrected 2026-08-13. The original premise was wrong; the risk is real.**

The original entry stated that a pure deletion "contributes zero byte ranges to
the scope". It does not. The `count == 0` branch in
`changedbytes.go:48` turns a zero-length new side into
`lineRange{first: max(start, 1), last: start + 1}` — a synthetic two-line
window around the deletion point. That branch has been present since the
wrapper's first commit, so the premise was already false when it was written.

Measured, deleting five lines from one staged file (`@@ -8,5 +7,0 @@`):

    line scope       : 1 staged byte range(s)
    candidate mutants: 0 (kept nodes 28, dropped nodes 714)
    go mutation: staged line scope matched no ooze mutation nodes;
                 refusing a zero-execution run

Alone, a pure deletion costs nothing: the window lands on a closing brace and a
blank line, no mutator fires, and the wrapper refuses before ooze starts. That
refusal is correct and should stay.

In company it is pure waste. The same deletion staged alongside one ordinary
one-line edit in a second file: `-dry` forecast 4, run performed **8**, and the
deletion's own file received four of them — in `AaSix`, a function no diff
touched — while contributing zero candidates to the forecast. A file with
nothing to measure paid for four test-command runs.

So the entry stands, for the reason in entry 1 rather than the reason
originally given. Dropping a zero-candidate file from the mutable set would
remove the cost and cannot lose information, because the file has none to give.

## 4. Resolved: `token.Pos` minus one is a byte offset

**Closed 2026-08-13.** The assumption holds, and checking it produced the proof
for entry 1.

`TestPositionMinusOneIsByteOffsetWithFreshFileSet` names the condition in its
title: the identity holds when the `token.FileSet` contains one file, so its
base is 1. ooze satisfies it — `GoSourceFile.Incubate` constructs
`token.NewFileSet()` per file before parsing
(`internal/gosourcefile/gosourcefile.go:26`).

The worry was that a shared fileset would shift every position by that file's
base. The reality is the opposite and worse: because every file starts at zero,
offsets from different files overlap exactly. The guard this entry asked for —
assert the position belongs to the file whose ranges are held — is still worth
having, but as the fix for entry 1, not as a defence against this.

## 5. A red suite reports a perfect score, and bills for it

**Confirmed 2026-08-13, exactly as originally described.** First measured
2026-08-11 during the boundaries-ownership change.

With one test failing on unmutated code and an ordinary one-line staged change:

    baseline suite on unmutated code : FAIL
    Killed: 4   Survived: 0   Score: 1.00 (minimum: 0.80)
    wrapper exit code: 0

The gate's own verdict says the change is perfectly covered. The arithmetic is
doing what it was told: a mutant dies when the test command fails, and a suite
that already fails kills every mutant without any mutation being involved.

Two notes from reproducing it. The failing test must be **independent of the
mutated expression** — a first attempt whose red test read the very code being
mutated scored 0.75, because one mutant happened to turn that test green. And
the same run against a green suite scored 0.83 with one survivor: a real gap,
in a real guard, which the perfect score had hidden.

Under a cost objective this is the cheapest fix on the list and the largest
saving. A red baseline wastes **one hundred percent** of the run: every mutant
execution is paid for and returns no information. One unmutated run of the test
command — a single execution, the cheapest check available — prevents all of
them.

This is also the failure the repository names in its own first rules: a verdict
that has to be read out of context rather than off an exit code. A gate that
reports 1.00 when its inputs are broken is worse than one that reports nothing,
because it answers the question it was asked.

What would close it: run the test command once, unmutated, before releasing
ooze, and refuse to score at all if it does not pass. A baseline that fails is
not a low score — it is no measurement.

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

## 7. Nothing enforces the mutation floor automatically

`.githooks/pre-commit` runs gofmt, vet and `go test ./...`;
`.github/workflows/ci.yml` runs gofmt, vet, `go test ./... -race` and
`scripts/verify-gate.sh`. Neither invokes `go run ./tools/mutationstaged`. The
floor is real but it is enforced by an author choosing to run it, which is why
the failing combined score in entry 1 did not block a commit.

**Retracted 2026-08-13: hunk proximity does not sweep in neighbours.** This
entry previously claimed that a function the change never touched is mutated
when it sits close enough to a hunk, on the evidence of a survivor in
`lefthookExtendsStep.Satisfied`. The claim is false. `computeScope` reads the
diff with `-U0`, so the scope is the changed lines exactly, and the single-file
control measures one changed line producing exactly one mutation site and four
mutants. The only proximity path that exists is the synthetic two-line window a
pure deletion creates, which is entry 3. That survivor was a cross-file
collision — entry 1 — in a run with several files staged.

The coverage gap that survivor exposed was real and separate from the
attribution question: `lefthookExtendsStep.Satisfied` is

    hookManager(p) != managerLefthook || extendsWired(root, lefthookConfig, target)

and no test reaches the case where lefthook *is* the hook manager and the
config is already wired, so replacing the right operand with `true` survives.

## 8. The wrapper's own fixtures commit to the real repository

**Measured 2026-08-13**, by causing it.

`newGitFixture` in `tools/mutationstaged/flow_test.go` builds a throwaway repo
in a temporary directory and runs `git` against it. Those `git` invocations
inherit the ambient environment, and git exports different variables to hooks
depending on the shape of the checkout:

    plain repository  →  GIT_DIR=[]                       GIT_INDEX_FILE=.git/index
    linked worktree   →  GIT_DIR=<repo>/.git/worktrees/X  GIT_INDEX_FILE=<...>/index

In a plain clone `GIT_DIR` is not exported at all and `GIT_INDEX_FILE` is
relative, so a subprocess whose working directory is the fixture resolves to
the fixture and everything behaves. In a **linked worktree** both are absolute,
every `git` subprocess on the machine retargets the real repository, and the
fixture's `git commit -m fixture` commits against it — replacing the tree with
the fixture's contents and leaving a commit named `fixture` on the branch.

Observed on both the worktree branch and, through an inherited absolute
`GIT_DIR`, on `feat/structured-reports`. Recovery was `git reset --mixed` back
to the real HEAD plus rebuilding the index; no file content was lost, because
`git commit` does not touch the working tree.

Two things this costs. Running `go test ./...` from `.githooks/pre-commit`
inside a linked worktree is unsafe today. And the failure is silent in the
worst way: the test **passes**, because the misdirected commit succeeds.

The fix is small: clear `GIT_DIR`, `GIT_INDEX_FILE` and `GIT_WORK_TREE` in the
fixture's git invocations, so a fixture can only ever address its own
repository. A test that can reach outside its fixture is not isolated,
whatever it asserts.
