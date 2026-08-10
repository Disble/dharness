# dharness

A commit gate for TypeScript projects, built out of three tools that already
exist: [react-doctor](https://www.react.doctor), [fallow](https://fallow.tools)
and [StrykerJS](https://stryker-mutator.io).

dharness owns their invocation and nothing else. It does not reimplement what
they do, does not proxy their own commands, and does not decide what a finding
means — the verdict is their exit code, passed through untouched.

## Why it exists

The problem was never sharing configuration. Configuration is small, specific to
one repository, and the tools generate it themselves. What repeats across every
project is **execution**: which tool owns which diagnostic, in what order, scoped
to what, with what ceiling on resources, and what the gate does when one fails.
That is what lives here.

## Install

```
go install github.com/Disble/dharness/cmd/dharness@latest
```

`@latest` resolves to the newest tagged release. Releases are cut by
release-please from the conventional commits on main — it opens a pull request
carrying the version and the `CHANGELOG.md` entry, and merging it tags the
release and builds the binaries. Nothing about a release is decided by hand.

## Use

```
dharness init      # set the project up, end to end
dharness sync      # report what init would do, and change nothing
dharness check     # the commit gate: react-doctor on the staged change, then fallow
dharness mutate <path...>   # find out whether these files' tests would notice the code breaking
```

Stryker's JSON config must select `vitest` or `jest`; dharness preserves that
selection and any `appendPlugins` while adding the remote runner it provisions.
Executable `.js`, `.mjs` and `.cjs` configs stop with a clear error because
determining their runner would require dharness to execute project code. Yarn
projects with `node_modules` use the npx transient route. Yarn Plug'n'Play must
switch to `nodeLinker: node-modules` and run `yarn install` before mutation.

`init` installs what is missing, writes the files dharness owns into
`.dharness/`, points the project's own configuration at them with one `extends`
line, wires the gate into git, and ends by handing the architecture analysis to
the agent — because zones encode intent, and no tool can read intent off a tree.

`sync` is the same plan, reported instead of applied. It derives everything from
the repository as it is right now, so re-running it after months reports drift
rather than a stale record of what once happened.

## What it will not do

- **Wrap a command that already exists.** If a tool exposes it, dharness runs it.
- **Interpret output.** Both readers — a person and the model that ran the commit
  — get the tools' own text, and a failure hands over that tool's own help.
- **Let a model decide.** The verdict comes from exit codes and JSON. The agent
  edits; it never decides whether something passes.
- **Touch configuration it does not own.** dharness writes into `.dharness/` and
  adds one reference line to the project's file.

## Ownership

fallow owns the repository graph: dead code, dependencies, cycles, boundaries.
react-doctor owns React and React Native semantics, and runs with `--no-dead-code`
so the two never answer the same question twice. Stryker answers one question
neither can: whether the tests would notice the code breaking.

## Documentation

- [`docs/design-principles.md`](docs/design-principles.md) — nineteen numbered
  principles, each traced to the decision that produced it.
- [`docs/learning-log.md`](docs/learning-log.md) — what turned out to be true,
  dated, one line each.
- [`docs/mutation-testing.md`](docs/mutation-testing.md) — staged-line Go
  mutation testing for contributors.
- [`AGENTS.md`](AGENTS.md) — the engineering doctrine, for humans and agents.

## Development

```
git config core.hooksPath .githooks   # once per clone: enables the local gate
go test ./...
go run ./tools/mutationstaged -dry   # inspect staged Go mutation scope
go run ./tools/mutationstaged        # execute staged Go mutants
bash scripts/verify-gate.sh           # proves the gate still refuses
```

The shipped dharness binary remains stdlib-only. ooze is a development-only Go
module dependency used by the repository mutation wrapper.
