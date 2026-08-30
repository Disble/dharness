package app

import (
	"fmt"
	"io"
)

func printHelp(w io.Writer, version string) {
	fmt.Fprintf(w, `dharness — a thin wrapper around react-doctor, fallow and Stryker (%s)

USAGE
  dharness <command> [flags]

COMMANDS
  sync                      Set this project up: install what is missing, write
                            the files dharness owns, wire the gate, and hand the
                            rest to the agent. Derived from the repository as it
                            is right now, so re-running it is safe and reports
                            drift.
  check                     Run the commit gate: ESLint and react-doctor on the
                            staged change, then fallow. Stops at the first
                            failure. ESLint runs first, and only when the
                            project installed it; an ESLint exit 2 is a config
                            that never loaded rather than a lint finding, and
                            the gate says which it got.
  mutate <path...>          Run mutation testing over the given files, to find
                            out whether their tests would notice the code
                            breaking. Use it once those tests are green.
                            A path can name lines instead of the whole file,
                            as src/thing.ts:12-40, and the verdict then covers
                            exactly those lines. Runs the Stryker the project
                            installed, which is the only place it can resolve
                            the project's own TypeScript, and adds it at @latest
                            only when the project declares none. It needs the
                            project to run its tests on vitest or jest; Stryker
                            supports no other runner here.
                            A mutant that cannot be killed because it changes
                            nothing is silenced in the code, with Stryker's own
                            directive and a reason that stays next to it:
                              // Stryker disable next-line all: equivalent, X is
                              // undefined either way
                            Those mutants report as ignored and stop failing the
                            run, which is what keeps the verdict worth reading.
  version                   Print version

FLAGS
  --dry-run                 mutate only: measure how many tests a scoped run
                            executes, without mutating anything
  --concurrency <n>         mutate only: Stryker workers (default 2)
  --upgrade                 mutate only: bring Stryker to @latest, rewriting
                            the version the project declares
  --fresh                   mutate only: measure just the paths you named.
                            Without it, results from earlier runs are reused —
                            they are kept in .git/dharness/, they make the run
                            faster, and they are why the table Stryker prints
                            can cover more files than you asked about.
  --help, -h                Show this message; every command also accepts help

dharness owns invocation only. Each wrapped tool keeps its own configuration,
written by that tool's own installer.
`, version)
}
