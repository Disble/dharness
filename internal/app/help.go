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
  init                      Set this project up: install what is missing, write
                            the files dharness owns, wire the gate, and end by
                            handing the architecture analysis to the agent.
  sync                      Report what init would do, without doing any of it.
                            Derived from the repository as it is right now, so
                            re-running it reports drift. Writes nothing.
  check                     Run the commit gate: react-doctor on the staged
                            change, then fallow. Stops at the first failure.
  mutate <path...>          Run mutation testing over the given files, to find
                            out whether their tests would notice the code
                            breaking. Use it once those tests are green.
  version                   Print version

FLAGS
  --dry-run                 mutate only: measure how many tests a scoped run
                            executes, without mutating anything
  --concurrency <n>         mutate only: Stryker workers (default 2)
  --help, -h                Show this message; every command also accepts help

dharness owns invocation only. Each tool keeps its own configuration, written
by its own init.
`, version)
}
