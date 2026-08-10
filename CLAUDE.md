# CLAUDE.md

Project-specific guidance for Claude Code. `AGENTS.md` is the canonical context —
the engineering doctrine, the enforcement ladder, and this repository's recorded
deviations. Read it first. This file names the two things that go wrong here.

## The principle that is easiest to violate

**If the CLI already does it, dharness does not do it.**

Every single time the temptation appeared, the wrapped tool already solved it,
better and faster. The mutation ratchet turned out to be `--gate new-only`, which
fallow already ships. The configuration check turned out to be `entry_points`,
which fallow already reports. A dead-code percentage heuristic was invented to
detect a misconfigured graph, and the tool had been printing the answer the whole
time.

The rule, in order:

1. Read the tool's own `--help` and its JSON output before designing anything.
2. If it exposes a command for the job, dharness runs that command. It does not
   wrap it, extend it, or explain it.
3. If nothing exposes a command, only then does the work belong here — and if it
   needs judgment rather than execution, it goes to the agent as a prompt.

That boundary is deliberately testable: **does a command exist?** Not
configuration versus code, not preparation versus findings.

## The second one

**The verdict comes from exit codes and JSON. Never from prose, and never from a
model.** A command whose conclusion has to be read out of its output is broken —
that is how Stryker exits 0 while reporting survivors. The agent edits; it never
decides whether something passes.

## Before proposing anything structural

`docs/design-principles.md` holds nineteen numbered principles, each traced to
the decision that produced it. Cite them by number to accept or reject a change.
A principle is discussed by pointing at the case that produced it — if the case
was misread, the principle goes with it.

`docs/learning-log.md` is the append-only why-log. One dated line, newest at the
bottom, never rewritten. Append to it when something non-obvious turns out to be
true; it is a memory aid and never a substitute for a check that enforces it.

## Commands

    git config core.hooksPath .githooks   # once per clone: enables the gate
    go build ./...
    go vet ./...
    go test ./... -race
    gofmt -l .
    bash scripts/verify-gate.sh           # proves the gate still refuses

Stdlib only. There is no dependency and no package manager, and a proposal that
adds one argues against that first.
