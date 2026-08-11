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
