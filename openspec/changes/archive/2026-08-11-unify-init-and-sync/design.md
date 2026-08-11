# Design: unify-init-and-sync

Inputs: `openspec/changes/unify-init-and-sync/proposal.md` (authoritative on
scope), `exploration.md`, `docs/flujo-implementado.md` (specification of record),
`docs/design-principles.md` §15, §16, §20, §21.

## Technical Approach

One command, three phases, two stopping points (Figure 1 of the specification).
`Delegated(p) (why string, ok bool)` moves onto `Step` and is answered during
Prepare; `Apply` runs only for steps whose `ok == false`. `RunSync` prints the
project header, applies, then renders the delegated steps as prompts. Nothing is
printed for a satisfied step — that is §15, and it is the only rule the report
needs. Ahead of all of it, `RunSync` enforces the stop Figure 1 specifies and
the code has never had (Decision 6bis).

## Architecture Decisions

### Decision 1 — Report shape

**Choice.** Three regions, in this order, each one addressed at both readers
(§16):

```
# dharness in <root>

JS project: web/ — the repository root keeps the hook.
Package manager: bun. Test runner: vitest.

Applying:
  install what this project is missing
  write the files dharness owns

## Left to you: point fallow's config at the file dharness owns

dharness cannot run this: .fallowrc.json already exists and belongs to the
project; adding a key to it is a merge, not a write.

Add this line to .fallowrc.json:

    "extends": ["../.dharness/fallow.jsonc"]

## Left to you: decide this project's architecture

dharness cannot run this: boundaries say what the code is meant to be, and no
tool can read intent off a tree.

<the analysis instructions, the file to write, the command that checks it>
```

- **Header** — what was detected. Unchanged from both commands today.
- **`Applying:`** — present tense, one line per step, emitted by `setup.Apply`
  *before* the step succeeds. It stays present tense deliberately: a past-tense
  receipt printed ahead of the result would be a claim the run has not earned.
- **`## Left to you:` blocks** — one per delegated step, in plan order. Heading
  is the step `ID()`, first paragraph is `Delegated`'s `why`, body is
  `Describe(p)`. No count, no summary line, no "N steps remain".
- **Nothing at all** for a satisfied step, and no trailing pointer to another
  command — there is no other command. The terminal answer
  (`Nothing to do: …` plus the `ScopedMutation` line) is kept from `RunSync`
  and now means the whole run, not the report half.

**Alternatives considered.** A "Satisfied" or "Skipped" section listing what was
already in place — rejected, it is the exact §15 violation this change exists to
remove. A single flat list numbered `## 1.`, `## 2.` as `sync` prints today —
rejected: applied and delegated steps are not the same kind of thing, and one
numbering implies the agent owns both.

**The question the proposal left open, answered: yes — `why` + `Describe(p)`
is a prompt, in this shape.** The specification's own test for a prompt is "todo
prompt nombra su propia comprobación". The block gives the agent a heading it
can act on, a reason that survives without options, an instruction, and the
check. `agentSkillStep` already satisfies this today: `Why()` names the hook
collision, `Describe(p)` gives the command and what to decline. So
`docs/flujo-implementado.md`'s **Conducción** row becomes: *"Es un prompt real:
el reporte pone el motivo y la instrucción bajo un mismo encabezado."* The
condition is that `Describe(p)` be an instruction, not a description — which is
why Decision 4 rewrites `extendsStep`'s.

### Decision 2 — The three prompt kinds carry no type

**Choice.** Confirm the exploration. Fusión / Conducción / Intención stay a
property of *why* delegation happened, expressed in the `why` string. No `Kind`
field, no enum, no interface.

**Rationale.** Given Decision 1, all three render identically: heading, reason,
instruction. The difference between them is entirely the *content* of two
strings — Fusión dictates a literal line, Conducción names a command and what to
refuse, Intención gives instructions and no options. Nothing in the code would
ever switch on the kind. Worse, the moment the field exists someone will branch
the report on it, and one contract becomes three output shapes. The taxonomy is
a tool for reasoning about the design, and it belongs in the document that does
that reasoning. §21 hands the agent an executable instruction, not a
classification of the instruction.

### Decision 3 — Rollback wording

**Choice.** `setup.Apply`'s error becomes:

```go
// The hedge is deliberate: Writer.Undo restores files it snapshotted and does
// not remove directories created by os.MkdirAll, nor the .gitignore written
// outside the Writer by project.Project.EnsureDir. Tighten this sentence to
// "everything this run wrote was undone" in `writer-undo-completeness`.
return fmt.Errorf(
    "%s failed. Every file this run wrote was put back as it was found; directories it created are not removed. No earlier step is reported as having succeeded: %w",
    step.ID(), err)
```

The `errors.Join` branch keeps its current wording — `"%s failed and the
repository could not be restored: %w"` already claims nothing.

**Rationale.** The proposal forbids claiming everything written was undone.
Naming the leak is stronger than staying silent about it and costs one clause,
and the comment carries the expiry so the vagueness is never rediscovered as
unexplained.

### Decision 4 — `extendsStep` splits into two steps

**Choice.** `extendsStep` becomes `fallowExtendsStep` (target: `.fallowrc.json`
in `p.Source`) and `lefthookExtendsStep` (target: `lefthook.yml` in `p.Root`).

**Rationale.** This is the one place the fixed decisions do not compose. A single
step with two targets can have two different recipients — the project owns
`.fallowrc.json` but has no `lefthook.yml` — and `Delegated` returns one answer.
Return `ok == true` and the lefthook file is never written while the report says
nothing about it; return `ok == false` and the fallow handoff never reaches the
agent. The specification says the plan is *cada paso con su destinatario*: a step
with two recipients is not a plan entry. Splitting also makes each prompt carry
the right syntax — JSON for one, YAML for the other.

**Per state:**

| State | `Satisfied` | `Delegated` → `why` |
|---|---|---|
| Config absent | false | `ok=false` — dharness writes the whole file |
| Config present, `extends` already points at the owned file | true | not asked |
| Config present, not wired | false | `ok=true` — *"`.fallowrc.json` already exists and belongs to the project; adding a key to it is a merge, not a write"* |
| No JS project (fallow only) | true | not asked |
| `hookManager(p) != managerLefthook` (lefthook only) | true | not asked |

`wireFallowExtends` / `wireLefthookExtends` lose their `error` return branch
entirely — the case that produced it is now answered before `Apply`.

**Where the text comes from.** `Describe(p)` calls the same
`ownedFrom(p, dir, name)` that `Apply` calls, so the path in the prompt is
computed from `p.Root`/`p.Source` at report time and cannot drift from the path
that would be written. File names come from the existing `fallowConfig`,
`lefthookConfig`, `ownedFallow`, `ownedLefthook` constants. No stored string.

### Decision 5 — `hookInstallStep`

`Satisfied`'s `default:` returns **false**. `Delegated` returns:

| `hookManager(p)` | `ok` | `why` |
|---|---|---|
| `managerLefthook` | false | — |
| `managerHusky` | false | — |
| `managerNone` | true | *"nothing answers: there is no lefthook config, no `.husky/` and no lefthook binary. Choosing a hook manager is a decision this project has not made, and not a default dharness gets to pick."* |

`Describe(p)` gains a third branch for `managerNone`, naming both managers, the
one line the gate needs (`gateCommand`), and the check — the step stops appearing
once a hook invokes it. Today's two-branch `Describe` would print the lefthook
sentence to a project with no lefthook.

**Text source:** the `gateCommand` and `huskyHook` constants, already the single
spelling of both. `hookManager(p)` is re-evaluated every run, so installing
husky later flips the answer without anyone recording it.

### Decision 6bis — The hard stop that does not exist yet

**Verified.** `Discover` swallows the not-a-repository error
(`internal/project/discover.go:61-67`): it returns `At(dir, dir), nil`, so
`Source == Root == dir`, `HasSource()` is true, and `RunSync` walks straight
into `setup.Pending(p)`. Outside a repository, `sync` today reports a plan for
an ordinary directory. Figure 1's left red block is specified, not implemented.

**Choice: the stop lives in `RunSync`; `Discover` records the fact and still
does not raise.** `Project` gains one field:

```go
// InRepository reports whether Root came from git or is merely the directory
// dharness was run in. Detection records the answer; deciding what a missing
// repository means belongs to the command that needs one.
InRepository bool
```

Set true in `Discover`'s three success branches, left at its zero value in the
swallow branch. `RunSync` checks it immediately after `Discover`, before the
header and before anything reads the plan, and returns an error.

**Rationale — why not change `Discover`'s policy.** The policy comment says "the
commands that need one fail on their own." That is *still true for two of the
three*: `check` reaches `StagedSourceFiles`, which returns
`NotAGitRepositoryError` (`internal/project/git.go:69`), and its message — "the
gate scopes itself to the index, so it needs a git repository" — is precisely
right for a gate and precisely wrong for adoption. Raising inside `Discover`
would replace that good, specific error with a generic detection-time one in the
two commands that already handle this correctly, to fix the one that does not.
The policy is sound; `sync` is the command that never held up its end. Recording
a derived fact is detection's job — it is what detection *is*. Discarding it was
the defect.

**Rejected:** re-calling `repositoryRoot` from `RunSync`. It re-runs a
subprocess `Discover` already ran and creates a second source of truth for one
question.

**The error** (plain `fmt.Errorf`, no new type — `app.ExitCode` returns 1 for
any non-`runner` error):

> `%s is not inside a git repository. dharness owns a commit gate, so there is
> nothing for adoption to attach to: no .git/hooks to install the hook into and
> nothing to commit .dharness/ to. Run it from inside a repository.`

**"No JS project" stays a graceful message at exit 0, and is not merged into
this stop.** Two different states with two different remedies: no repository
means dharness can do nothing anywhere; no JS project means the repository is
real and the honest answers are "this is a Go repo, dharness is not for it" or
"install it first, the lockfile is what dharness looks for" — which is what
`noSourceMessage` (`internal/cli/flags.go:100-105`) already says. Making it an
error would fail `dharness sync` in every non-JS repository. It is the same
class of complete answer as `Nothing to do:`.

Figure 1 draws the two as one box because the *outcome* is one — no plan is
possible, nothing is written. The exit codes differ, and that is deliberate:
§17 puts the verdict in the exit code, and "there is nothing here for me" is
not a failure. `docs/flujo-implementado.md` gains a line saying so, since the
figure alone reads as one behaviour.

**Consequence for the proposal:** `docs/flujo-implementado.md` is now ahead of
the code in a **second** place, not only the owned-files-version step. This
change closes that second gap.

### Decision 6 — `architectureStep`

New step, last in `Plan()`, preserving today's output position.
`ID()` = `"decide this project's architecture"`. `Satisfied(p)` = the
`.dharness/fallow.jsonc` file contains the literal substring `boundaries`,
per the `extendsWired` precedent (`files.go:106-109`) — no JSONC parser, the
product is stdlib-only. `Delegated` returns `ok=true` always: Intención, no
detection is possible. `ArchitecturePrompt` loses its `## Left to you:` heading
(the report supplies it) and its opening paragraph moves to `why`; the rest
becomes `Describe(p)`.

## Data Flow

    RunSync ──→ project.Discover ──→ setup.Pending(p)
                                          │
              ┌───────────────────────────┴──────────────────────┐
              │ Prepare: Satisfied(p) then Delegated(p)          │
              │ writes nothing, so it cannot fail                │
              └───────────────────────────┬──────────────────────┘
                          ok == false      │      ok == true
                                ↓          │          ↓
                    setup.Apply(p, stdout) │   "## Left to you:" block
                     │ error → Writer.Undo │     why + Describe(p)
                     └────→ return         └────→ report continues

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/project/detect.go` | Modify | `Project` gains `InRepository bool` (Decision 6bis) |
| `internal/project/discover.go` | Modify | Sets `InRepository` in the three success branches; the swallow branch and its policy comment stay |
| `internal/setup/setup.go` | Modify | `Delegated(p)` joins `Step`; `Delegated` interface and the type assertion in `Apply` are removed; rollback wording (Decision 3) |
| `internal/setup/steps.go` | Modify | `Delegated(p)` on every type; `extendsStep` splits (Decision 4); `hookInstallStep` (Decision 5); `architectureStep` (Decision 6) |
| `internal/setup/files.go` | Modify | `wireFallowExtends` / `wireLefthookExtends` drop the error branch |
| `internal/setup/prompt.go` | Modify | `ArchitecturePrompt` loses its heading and opening paragraph |
| `internal/cli/sync.go` | Modify | Absorbs `RunInit`; renders Decision 1; enforces the repository stop (Decision 6bis) |
| `internal/cli/init.go` | Delete | 68 lines |
| `internal/app/app.go` | Modify | `init` case and `UnknownCommandError` string |
| `internal/app/help.go` | Modify | Two COMMANDS entries collapse into one |
| `internal/cli/sync_test.go` | Modify | See Testing Strategy |
| `internal/setup/setup_test.go` | Modify | Reversal plus new contract tests |
| `internal/project/discover_test.go` | Modify | `InRepository` in both branches |
| `docs/flujo-implementado.md` | Modify | The rows named in the proposal, the Conducción row per Decision 1, and a line separating the two left-hand stop states per Decision 6bis |

## Interfaces / Contracts

```go
type Step interface {
    ID() string
    Describe(p project.Project) string
    Satisfied(p project.Project) bool
    // Delegated reports whether this repository leaves the step to the agent,
    // and why. Pure, like Satisfied: it is answered during Prepare, which
    // writes nothing and therefore cannot fail.
    Delegated(p project.Project) (why string, ok bool)
    // Apply runs only when Delegated returned ok == false.
    Apply(p project.Project, w *Writer) error
}
```

`agentSkillStep.Why()` becomes `Delegated(project.Project) (string, bool)`
returning its existing text and `true`. Its `Apply` keeps returning the
"delegated and must not be applied" error — that error is now a contract
assertion, and one test asserts it is never reached.

## Testing Strategy

**Two enabling findings, both existing seams — this change invents none.**

1. `runner.SetForTest` (used at `internal/setup/setup_test.go:130`) is
   importable from `internal/cli`. Every `internal/cli` test must now install a
   stub, because `RunSync` applies and `installStep` / `hookInstallStep` shell
   out; without it a bare `t.TempDir()` would run `bun add` on CI.
2. `project.SetGitOutputForTest` (`internal/project/git.go:19-24`) is exported,
   and its comment states the repository's own policy: *"git is seamed and the
   filesystem is not."* No `internal/cli` test needs a real `git init`.

**The fixture cost Decision 6bis creates, stated plainly.** Every existing
`internal/cli` sync test runs in a bare `t.TempDir()`, which is not a
repository. They pass today *only because of the swallow* — they exercise the
`At(dir, dir)` path that Decision 6bis makes unreachable in production. So
every sync test needs a shared `gitProject(t, root)` helper that stubs
`gitOutput` to answer `rev-parse --show-toplevel` with `root` and `ls-files`
with the fixture's lockfile paths. ~15 lines once, then one line per test.

This is a fixture change, not a test rewrite, and it is a strict improvement:
the current fixtures verify a code path production will no longer reach.

`internal/cli/sync_test.go`:

| Test | Fate |
|---|---|
| `TestSyncWritesNothing` | **Dies.** Categorically false. |
| `TestSyncSpeaksTheProjectsOwnPackageManager` | **Rewritten** — asserts on the command captured by `runner.SetForTest`, not on report text |
| `TestSyncSaysWhyTheDelegatedStepIsDelegated` | **Kept, extended** to the extends and gate delegations |
| `TestSyncReachesATerminalAnswer` | **Kept, extended** — fixture gains `boundaries`; asserts the architecture prompt is absent (the §15 regression) |

New in `internal/cli/sync_test.go`, one per unverified `RunInit` behaviour or
success criterion:

1. `TestSyncAppliesAndDelegatesInOneRun` — both regions present, in order.
2. `TestSyncCompletesWhenTheProjectAlreadyConfiguredFallow` — non-empty
   `.fallowrc.json`, no error, no rollback, the `extends` line in the output.
3. `TestSyncNeverAppliesADelegatedStep` — `agentSkillStep.Apply`'s error never
   surfaces.
4. `TestSyncStopsBeforeWritingWithoutAJSProject` — `RunInit`'s untested branch;
   asserts `noSourceMessage` and **`err == nil`**, pinning the exit-0 half of
   Decision 6bis.
5. `TestSyncRollbackNamesWhatItRestoredAndNothingMore` — pins Decision 3.
6. `TestSyncStopsOutsideAGitRepository` — no `gitProject` stub, so
   `InRepository` is false; asserts a non-nil error, that its text names the
   repository rather than the index, and that the directory is untouched
   (`tree()` before and after — the one assertion `TestSyncWritesNothing`
   made that is still true, kept here where it now belongs).

`internal/setup/setup_test.go`:

| Test | Fate |
|---|---|
| `TestGateStepIsSatisfiedWhenNoManagerAnswers` | **Reversed** into `TestGateStepIsAnOpenDecisionWhenNoManagerAnswers`; comment replaced with the decision-table and §20 rationale, never silently deleted |
| `TestArchitecturePromptPinsFallowToRemoteLatest` | **Retargeted** at `architectureStep{}.Describe(p)` |

New: `TestApplySkipsEveryDelegatedStep`, `TestFallowExtendsIsDelegatedWhenTheProjectOwnsTheConfig`, `TestArchitectureStepDisappearsOnceBoundariesAreDeclared`.

**Mutation coverage** on the reversed `default` branch: the replacement test must
assert both `!Satisfied(p)` *and* a non-empty `why` with `ok == true`. Asserting
only `!Satisfied` leaves `Delegated`'s `managerNone` branch unkilled.

**Platform:** every path assertion goes through `filepath.Join` / the existing
`binaryName` helper. `ownedFrom` returns slashes by design and the prompts show
slashes on both targets.

## Threat Matrix

| Boundary | Applicability | Design response | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | **N/A** — no file is classified as executable or not; the steps address fixed, named config files |
| Git repository selection | **Applicable** — `Discover` decides `Root` vs `Source`, `ownedFrom` computes the prompt path from it, and Decision 6bis makes "no repository at all" a stop for the first time | Absent repository stops before the header and before any write; split layout computes `../.dharness/…`, conventional computes `.dharness/…`, both from the helper `Apply` uses | `TestSyncStopsOutsideAGitRepository`; split-layout prompt names `../.dharness/fallow.jsonc` |
| Commit state | **N/A** — `sync` never reads the index |
| Push state | **N/A** — no remote interaction |
| PR commands | **N/A** — no VCS automation |
| Subprocess execution (added row) | **Applicable** — delegation now gates whether `installStep` and `hookInstallStep` shell out | A delegated step must never reach `runner.Run` | `TestSyncNeverAppliesADelegatedStep`; the `managerNone` case runs no command |

## Migration / Rollout

No data migration, no persisted state, no on-disk format change. `dharness init`
disappears; `UnknownCommandError` and `help` must name only the four surviving
commands, or a user who types `init` gets a list that still contains it.

Decision 6bis is the one **behaviour change a user can be surprised by**:
`dharness sync` in a non-repository directory stops with exit 1 where it
previously printed a plan. There is no flag and no deprecation window — the old
behaviour was never specified, and it produced a plan for a directory dharness
could not have adopted.

## Line forecast — above the proposal's estimate

| Source | Lines |
|---|---|
| Proposal's estimate | ~370–420 |
| Decision 4 — `extendsStep` splits | +~25 |
| Decision 6bis — field, `Discover` branches, the stop, its test | +~40 |
| `gitProject` fixture helper plus one line per sync test | +~30 |
| `internal/cli` test block realistically ~140–180, not ~60–100 | +~80 |

Realistic total: **~470–545 changed lines**, against a 400-line budget.
Decision 6bis is new behaviour, not a rename, and it costs roughly 70 lines with
its fixtures.

**Recommendation: split at the declared seam, and the slices are no longer
close to even.** Slice 1 — the step contract plus the two extends steps
(~200–230). Slice 2 — the command merge, the architecture step, the
`hookInstallStep` fix, the repository stop, and the documentation (~270–315).
Slice 2 is itself near budget; if it must be cut further, Decision 6bis is the
cleanest third slice, because it touches `internal/project` and nothing else in
this change does.

## Open Questions

- [ ] `doctorConfigStep` and the code-not-data `doctor.config.ts` case: this
      design does not deliver it, so the Fusión row's "Dónde se usa" cell must
      say so at apply time rather than imply Fusión is complete. Verify
      `doctorConfigStep.Satisfied` against a `.ts` config during implementation.
