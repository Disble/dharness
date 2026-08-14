package cli

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/runner"
	"github.com/Disble/dharness/internal/tool"
)

// RunCheck is the commit gate.
//
// ESLint runs first: measured cheapest of the four stages against the same
// explicit staged file list on a reference project — 1008 ms median against
// react-doctor's 2959 ms, fallow audit's 2102 ms and fallow dupes' 1398 ms
// (three runs each, docs/learning-log.md, 12 August 2026). Local resolution
// is most of why: it skips the remote executor's package-manager round trip
// that react-doctor and fallow both pay on every run.
//
// react-doctor runs next because --staged scopes it to the change, so its cost
// tracks the diff rather than the repository. fallow runs last because audit
// limits what it reports to changed files but still builds the repository graph,
// which gives it a higher floor — the measurement above found fallow cheaper
// than react-doctor on this reference project too, but that question was
// settled before this slice and stays open rather than being resolved here.
// Cheapest first is not a style preference: a failure in an earlier stage
// skips every stage behind it, and that is where most of the saving in a
// gate that runs on every commit comes from.
//
// One dependency this gate cannot enforce: react-doctor adopts the project's
// ESLint or oxlint JSON configuration when one exists, so a broken lint config
// is read as a broken config in silence. Lint is not a dharness step, so
// ordering it before this one is the hook's responsibility.
func RunCheck(args []string, stdout io.Writer) error {
	flags := newFlagSet("check", stdout, "Run the commit gate: ESLint, react-doctor, then fallow.")
	if err := parseFlags(flags, args); err != nil {
		return err
	}
	if helpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected check argument %q", flags.Arg(0))
	}

	dir, err := workingDirectory()
	if err != nil {
		return err
	}

	p, err := project.Discover(dir)
	if err != nil {
		return err
	}
	if !p.HasSource() {
		fmt.Fprintln(stdout, noSourceMessage(p))
		return nil
	}

	staged, err := p.StagedSourceFiles()
	if err != nil {
		return err
	}
	if len(staged) == 0 {
		fmt.Fprintln(stdout, "no staged source files, nothing to check")
		return nil
	}

	var stages []stage
	var notices []string

	// dharness does not install ESLint at gate time; that is eslintExtendsStep's
	// question at sync time, and the two are independent by construction — one
	// asks about wiring, this one asks about running what is already wired.
	if eslintPath := p.LocalBinary(tool.ESLint); eslintPath != "" {
		files, err := p.StagedSourceFilesFromSource()
		if err != nil {
			return err
		}
		stages = append(stages, localStage(p, tool.ESLint, eslintPath, tool.ESLintStaged(files)...))
	} else {
		notices = append(notices, fmt.Sprintf(
			"\n%s did not run: no local install was found under node_modules/.bin.\nInstall it as a project dependency to add it to the gate.\n",
			tool.ESLint))
	}

	stages = append(stages, remoteStage(p, tool.ReactDoctor, tool.ReactDoctorStaged()...))

	// fallow compares the change against a base, and a repository with no
	// commits has none. Its own error names the fix, but there is nothing to
	// fix: this is the first commit, which is exactly when adoption ends. The
	// cheapest way to run something is not to run it when it cannot answer.
	if project.HasCommits(p.Root) {
		diff, err := p.StagedDiff()
		if err != nil {
			return err
		}
		// An empty diff is refused rather than passed on. fallow reads it as a
		// scope that admits nothing and exits 0, so handing one over would buy
		// a green verdict over an unexamined change — the failure this whole
		// gate exists to prevent. The staged list above already returned early
		// when nothing was staged, which makes this unreachable; it is here
		// because the consequence of reaching it is silence, and silence is
		// what nobody notices.
		if len(diff) == 0 {
			return fmt.Errorf(
				"%s cannot run: %d staged file(s) produced an empty diff, and an empty scope would pass without auditing anything",
				tool.Fallow, len(staged))
		}

		// audit first, dupes second. audit is scoped to the staged change — the
		// base and the diff both come from here, see tool.FallowAudit — so it
		// answers the cheaper question and, failing, spares the second graph
		// build entirely. dupes measures the whole repository against the
		// ceiling dharness writes — a ceiling audit does not enforce, which is
		// the only reason this is a separate invocation rather than a flag.
		audit := remoteStage(p, tool.Fallow, tool.FallowAudit()...)
		audit.command.Stdin = bytes.NewReader(diff)
		stages = append(stages, audit,
			remoteStage(p, tool.Fallow, tool.FallowDupes()...))
	} else {
		notices = append(notices, fmt.Sprintf(
			"\n%s did not run: this repository has no commits yet, so there is\nno base to compare against. It runs from the next commit on.\n",
			tool.Fallow))
	}

	// Ranging over a nil slice is already a no-op, so nothing here needs to
	// ask whether there is anything to print before deferring the loop.
	defer func() {
		for _, notice := range notices {
			fmt.Fprint(stdout, notice)
		}
	}()

	for index, stage := range stages {
		// Two tools writing into one stream with nothing between them leaves
		// whoever reads the gate — a person, or the model that ran it — to work
		// out where one report ends and the next begins.
		fmt.Fprintf(stdout, "\n── %s ──\n", stage.tool)

		if err := runner.Run(stage.command, stdout, stdout); err != nil {
			if skipped := stages[index+1:]; len(skipped) > 0 {
				fmt.Fprintf(stdout, "\n%s failed, so %s did not run. There may be more to fix behind it.\n",
					stage.tool, names(skipped))
			}
			fmt.Fprint(stdout, pointer(stage.help))
			return err
		}
	}

	return nil
}

// stage is one wrapped tool, the command that runs it, and the command that
// points at its help. It carries commands rather than arguments because the
// stages no longer resolve the same way: react-doctor and fallow run through
// the remote executor, ESLint runs the project's own installed binary (§03's
// recorded exception — a flat config imports the project's own plugins and
// framework configs, which a transient environment cannot resolve).
type stage struct {
	tool    string
	command runner.Command
	help    runner.Command
}

// remoteStage builds a stage that resolves through the detected package
// manager's remote executor — react-doctor's and fallow's existing
// resolution, unchanged by stage now carrying a command instead of args.
func remoteStage(p project.Project, name string, args ...string) stage {
	return stage{
		tool:    name,
		command: tool.RemoteLatest(p.PackageManager, name, p.Source, args...),
		help:    tool.RemoteLatest(p.PackageManager, name, p.Source, "--help"),
	}
}

// localStage builds a stage that resolves through the project's own
// installed binary at path, never the remote executor — the recorded
// exception this file's own comment names.
func localStage(p project.Project, name, path string, args ...string) stage {
	return stage{
		tool:    name,
		command: tool.Installed(name, path, p.Source, args...),
		help:    tool.Installed(name, path, p.Source, "--help"),
	}
}

func names(stages []stage) string {
	list := make([]string, 0, len(stages))
	for _, stage := range stages {
		list = append(list, stage.tool)
	}
	return strings.Join(list, " and ")
}
