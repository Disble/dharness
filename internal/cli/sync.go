package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/tool"
)

// RunSync prints what this project still needs, derived from what dharness
// detected in it.
//
// It is written for an agent to execute, and it is generated rather than
// stored: a document describing how to set up a wails project would name one
// package manager, one test runner and one set of paths, and would be wrong for
// every project that differs and stale for every version that follows. This
// comes from the same detection the commands themselves use, so it cannot
// disagree with them.
//
// The split is the one rule that decides what appears here: anything with a
// command is printed as that exact command, and only what no CLI exposes — the
// adaptation of fallow's entry points to an unusual layout — is described as
// work to do.
//
// It is named sync rather than init because nothing here is a one-time act. The
// project moves: a runner is swapped, a tool is removed, a hook is rewritten.
// Every step is derived from the current state of the repository, so re-running
// this reports drift for free — and the first run is simply the case where
// everything is missing at once. Nothing is written, so it is always safe.
func RunSync(args []string, stdout io.Writer) error {
	flags := newFlagSet("sync", stdout, "Print what this project still needs, as commands to run. Safe to re-run.")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if helpRequested(args) {
		return nil
	}

	dir, err := workingDirectory()
	if err != nil {
		return err
	}
	p := project.Describe(dir)

	fmt.Fprintf(stdout, "# dharness in %s\n\n", p.Root)
	fmt.Fprintf(stdout, "Package manager: %s. Test runner: %s.\n\n", p.PackageManager, orNotDetected(p.TestRunner))

	steps := 0
	step := func(format string, values ...any) {
		steps++
		fmt.Fprintf(stdout, "## %d. %s\n\n", steps, fmt.Sprintf(format, values...))
	}

	missing := missingTools(p)

	// The runner plugin is a package like any other, so it joins the same
	// install line rather than repeating one the project already needs.
	var runnerErr *project.MissingStrykerRunnerError
	if _, err := p.StrykerRunner(); errors.As(err, &runnerErr) {
		if runnerErr.Plugin == "" {
			step("Declare a test runner")
			fmt.Fprintf(stdout, "%s\n\n", err)
		} else {
			missing = append(missing, runnerErr.Plugin)
		}
	}

	if len(missing) > 0 {
		step("Install what this project is missing")
		fmt.Fprintf(stdout, "Without them every run fetches over the network, inside a gate that runs on\nevery commit.\n\n")
		fmt.Fprintf(stdout, "    %s %s\n\n", p.InstallCommand(), strings.Join(dedupe(missing), " "))
		if runnerErr != nil && runnerErr.Plugin != "" {
			fmt.Fprintf(stdout, "Stryker needs no config file — every option is a flag — but it cannot drive\n%s without that last plugin.\n\n", runnerErr.TestRunner)
		}
	}

	if !p.HookWired() {
		step("Wire the commit gate")
		fmt.Fprintf(stdout, "No hook invokes it yet. lefthook and husky both call the same line, so which\none this project uses never becomes a variable here.\n\n")
		fmt.Fprintf(stdout, "    dharness check\n\n")
	}

	if !p.HasFallowConfig() {
		step("Check whether fallow needs entry points")
		fmt.Fprintf(stdout, "This is the only step with no command behind it, which is why it is described\ninstead of printed. fallow needs no configuration in a conventional layout, and\nneeds it badly in one where the sources do not sit where it assumes — a Go module\nat the root with the frontend nested, for instance.\n\n")
		fmt.Fprintf(stdout, "Run it once and read the dead-code figure:\n\n")
		fmt.Fprintf(stdout, "    %s %s --format json\n\n", p.RemoteExec(), tool.Fallow)
		fmt.Fprintf(stdout, "If it reports an implausible share of the project as unreachable, the graph is\nrooted in the wrong place. Write %s declaring this project's real entry\npoints, then run it again and confirm the figure collapsed.\n\n", fallowConfigName)
	}

	if measured := p.ReadEvidence().ScopedMutation; measured != nil {
		if steps == 0 {
			fmt.Fprintf(stdout, "Nothing to do. Scoped mutation ran %d test(s) for %s when it was measured.\n",
				measured.RelatedTests, measured.MeasuredPath)
			return nil
		}
		return nil
	}

	step("Measure whether scoped mutation testing is viable here")
	fmt.Fprintf(stdout, "Stryker mutates the paths you name, but the test runner decides which tests to\nrun by reading the import graph. Barrel files inflate that graph until every\ntest counts as related, and the scoping disappears.\n\n")
	fmt.Fprintf(stdout, "    dharness mutate --dry-run <a source file>\n\n")
	fmt.Fprintf(stdout, "A handful of tests means scoped mutation works here. The whole suite means it does not.\n")

	return nil
}

const fallowConfigName = ".fallowrc.jsonc"

func dedupe(values []string) []string {
	seen := map[string]bool{}
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			unique = append(unique, value)
		}
	}
	return unique
}

func missingTools(p project.Project) []string {
	var missing []string
	for _, name := range []string{tool.ReactDoctor, tool.Fallow, tool.Stryker} {
		if !p.Resolve(name).Local {
			missing = append(missing, project.Package(name))
		}
	}
	return missing
}
