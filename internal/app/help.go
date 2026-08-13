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
  check                     Run the commit gate: react-doctor on the staged
                            change, then fallow. Stops at the first failure.
  mutate <path...>          Run mutation testing over the given files, to find
                            out whether their tests would notice the code
                            breaking. Use it once those tests are green.
                            A path can name lines instead of the whole file,
                            as src/thing.ts:12-40, and the verdict then covers
                            exactly those lines. Installs Stryker at @latest in
                            the project, which is the only place it can resolve
                            the project's own TypeScript.
  version                   Print version

FLAGS
  --dry-run                 mutate only: measure how many tests a scoped run
                            executes, without mutating anything
  --concurrency <n>         mutate only: Stryker workers (default 2)
  --help, -h                Show this message; every command also accepts help

dharness owns invocation only. Each wrapped tool keeps its own configuration,
written by that tool's own installer.
`, version)
}
