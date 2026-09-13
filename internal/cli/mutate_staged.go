package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/runner"
	"github.com/Disble/dharness/internal/staged"
	"github.com/Disble/dharness/internal/tool"
)

// ClassifierDisagreementError reports a file the types-only classifier
// believed compiled to nothing, but that Stryker's own report — over the exact bytes
// this run mutated — shows carrying at least one mutant anyway.
//
// The dropped files are included in the very Stryker run that decides this
// (mutateStaged asks Stryker about kept and dropped scopes together)
// precisely so this can be checked empirically rather than trusted: a
// classifier that is wrong about one file and silent about it would mean a
// staged commit skips real runtime code without anyone knowing.
type ClassifierDisagreementError struct{ Files []string }

func (e *ClassifierDisagreementError) Error() string {
	return fmt.Sprintf(
		"the types-only classifier disagreed with Stryker's own report: %s compiled to nothing by tsc, but Stryker instrumented mutant(s) for it anyway",
		strings.Join(e.Files, ", "),
	)
}

// runMutateStaged implements `dharness mutate --staged`: mutate exactly the
// line ranges a staged change added, rather than named paths.
func runMutateStaged(paths []string, dryRun, upgrade bool, concurrency int, excludePrefixes []string, stdout io.Writer) error {
	if len(paths) > 0 {
		return fmt.Errorf("--staged mutates exactly what the staged change justifies and does not accept a path; got %q", paths[0])
	}
	if dryRun {
		return errors.New("--staged does not support --dry-run: it always needs a real mutation report to self-check the classifier against, never just the initial test count")
	}
	if upgrade {
		return errors.New("--staged never installs Stryker, so --upgrade has nothing to do; run mutate without --staged to upgrade")
	}
	if concurrency < 1 {
		return fmt.Errorf("--concurrency needs a positive number, got %d", concurrency)
	}

	// The interrupt is intercepted rather than left to end this process,
	// because an uncaught one ends it before any defer runs and the snapshot
	// stays behind. SIGTERM as well as os.Interrupt: measured on Windows,
	// `taskkill /PID` without /F closes the console the process runs in, and
	// Go delivers that as SIGTERM, not as an interrupt.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return mutateStaged(ctx, concurrency, excludePrefixes, stdout)
}

// mutateStaged is a staged run past its refusals, under a context an
// interrupt cancels.
//
// Nothing here runs beside a child process. Cancellation reaches a running
// child through its Command's Context, which kills it; the step that started
// it returns once it has been waited for; the check after that step stops the
// run; and the single deferred cleanup runs last. The snapshot is never
// removed from under a process still working in it.
//
// Scope's and Snapshot's git probes take no context. Each is one short git
// command bounded by the size of the index, and a console interrupt reaches
// git directly, so the run is checked around them instead of cancelling them.
func mutateStaged(ctx context.Context, concurrency int, excludePrefixes []string, stdout io.Writer) error {
	dir, err := workingDirectory()
	if err != nil {
		return err
	}

	p, err := project.Discover(dir)
	if err != nil {
		return err
	}
	if !p.HasSource() {
		return fmt.Errorf("%s", noSourceMessage(p))
	}

	scopes, err := staged.Scope(p.Source, excludePrefixes)
	if err != nil {
		return err
	}
	if len(scopes) == 0 {
		fmt.Fprintln(stdout, "nothing staged to mutate")
		return nil
	}

	selection, err := p.StrykerRunner()
	if err != nil {
		return err
	}

	binary, err := requireLocalStryker(p, selection)
	if err != nil {
		return err
	}

	if err := interrupted(ctx, nil); err != nil {
		return err
	}
	snapshotRoot, snapshotSource, cleanup, err := staged.Snapshot(p.Root, p.Source)
	if err != nil {
		return err
	}
	defer func() { _ = cleanup() }()
	if err := interrupted(ctx, nil); err != nil {
		return err
	}

	// Stryker runs in the snapshot, so it reads the snapshot's Stryker config
	// — the one being committed — and the working tree's copy may carry edits
	// that were never staged. The selection read above only serves the
	// refusals that come before paying for a snapshot. Everything Stryker, the
	// guard and the verdict are handed from here on comes from the copy
	// Stryker runs against.
	selection, err = project.At(snapshotRoot, snapshotSource).StrykerRunner()
	if err != nil {
		return err
	}
	testRunnerArg := selection.TestRunner
	if selection.Configured {
		testRunnerArg = ""
	}

	files := uniqueScopedFiles(scopes)
	classification, err := staged.Classify(ctx, p.LocalBinary("tsc"), snapshotSource, files)
	if err != nil {
		return err
	}
	if err := interrupted(ctx, nil); err != nil {
		return err
	}
	if classification.Notice != "" {
		fmt.Fprintln(stdout, classification.Notice)
	}
	sort.Strings(classification.Dropped)
	for _, path := range classification.Dropped {
		fmt.Fprintf(stdout, "types-only: compiler emitted no runtime code: %s\n", path)
	}
	if len(classification.Kept) == 0 {
		return nil
	}

	guard := staged.GuardVitestSuite(ctx, p.LocalBinary("vitest"), selection.TestRunner, snapshotSource, selection.VitestConfigFile)
	if err := interrupted(ctx, guard); err != nil {
		return err
	}
	if guard != nil {
		return guard
	}

	// A staged run never trusts resolveReportPath (selection.ReportPath, or
	// the shared default): materialise links every gitignored entry from the
	// real project into the snapshot rather than copying it, so a report path
	// under a gitignored directory such as reports/ is the SAME file on disk
	// inside and outside the snapshot. A concurrent plain `dharness mutate` in
	// that checkout can write there between this run's Stryker finishing and
	// this run reading its own verdict, and a report that does not cover the
	// staged files reads as zero in-scope mutants — a silent pass. A path this
	// run alone owns, named below and never the project's, removes the
	// interleaving rather than narrowing its window.
	reportDir, err := os.MkdirTemp("", "dh-report-")
	if err != nil {
		return fmt.Errorf("create the mutation report directory: %w", err)
	}
	defer func() { _ = runner.RemoveSandbox(reportDir) }()
	reportPath := filepath.Join(reportDir, "mutation.json")
	// reportDir is freshly created above, so this can only ever find nothing
	// to remove; the guard stays because a report path is never trusted
	// without it, the same discipline the ordinary path applies.
	if err := clearReport(reportPath); err != nil {
		return err
	}

	sandbox, err := os.MkdirTemp("", "dh-stryker-tmp-")
	if err != nil {
		return fmt.Errorf("create the mutation sandbox: %w", err)
	}
	defer func() { _ = runner.RemoveSandbox(sandbox) }()

	entries := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		entries = append(entries, scope.Argument())
	}
	// The scope goes in a config file rather than on the command line, which
	// a staged change can outgrow; see tool.StrykerMutateFromConfig.
	configFile, err := writeStagedStrykerConfig(snapshotSource, selection.ConfigFile, entries, reportPath)
	if err != nil {
		return err
	}
	// No incremental file: a staged run judges exactly this commit, never
	// results kept from an earlier one.
	args := tool.StrykerMutateFromConfig(configFile, testRunnerArg, sandbox, concurrency)
	// The snapshot is already a disposable copy; Stryker's own additional
	// sandbox copy buys nothing here and --inPlace skips it, backing the
	// originals up in sandbox instead (Stryker's own documented behaviour).
	args = append(args, "--inPlace")

	mutation := runStryker(ctx, binary, snapshotSource, selection, args, stdout)
	if err := interrupted(ctx, mutation); err != nil {
		return err
	}
	if mutation != nil {
		return mutation
	}

	return reportStagedVerdict(reportPath, scopes, classification.Kept, classification.Dropped, stdout)
}

// ErrInterrupted reports a staged run an interrupt stopped before it reached a
// verdict.
var ErrInterrupted = errors.New("mutate --staged was interrupted before it reached a verdict")

// interrupted reports an interrupt as the reason the run stopped, so a child
// the interrupt ended is never read as one that failed on its own. stepErr is
// the step's own error when it has one to give. The classifier folds tsc's
// failure into a notice instead, so an interrupt that ends tsc before ctx is
// cancelled prints that notice and is caught at the next step.
//
// Both answers are needed because they arrive in either order. An interrupt
// dharness receives first cancels ctx and the runner kills the child. A
// console interrupt reaches the child at the same moment it reaches dharness,
// and the child can be dead of it — runner.Interrupted — while ctx is still
// live.
func interrupted(ctx context.Context, stepErr error) error {
	if ctx.Err() != nil || runner.Interrupted(stepErr) {
		return ErrInterrupted
	}
	return nil
}

// requireLocalStryker resolves the project's own local Stryker without ever
// installing one: --staged is invoked mid-session, potentially unattended
// behind a Ctrl-C-cancellable run, and installing at that moment is exactly
// what internal/cli/check.go already declines to do at gate time, for the
// same reason.
func requireLocalStryker(p project.Project, selection project.StrykerSelection) (string, error) {
	if binary := p.LocalBinary(tool.Stryker); binary != "" {
		return binary, nil
	}
	packages, err := tool.StrykerPackages(p.PackageManager, p.YarnPnP, selection.TestRunner)
	if err != nil {
		return "", err
	}
	return "", &StrykerUnavailableError{Packages: packages, Install: tool.InstallCommand(p.PackageManager)}
}

// clearReport removes a previous report at path, tolerating one that never
// existed — a report left by an earlier run must not be read as this run's
// verdict, and a staged run's own report always lands inside a snapshot
// nobody else writes to, so any other removal failure is unexpected and
// worth naming rather than swallowing.
func clearReport(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clear the previous mutation report: %w", err)
	}
	return nil
}

// uniqueScopedFiles collects the distinct files scopes names, sorted so the
// classifier's own output — and any test asserting on it — reads the same
// way twice regardless of how many ranges a file contributed.
func uniqueScopedFiles(scopes []tool.MutationScope) []string {
	seen := make(map[string]bool, len(scopes))
	var files []string
	for _, scope := range scopes {
		if seen[scope.Path] {
			continue
		}
		seen[scope.Path] = true
		files = append(files, scope.Path)
	}
	sort.Strings(files)
	return files
}

// reportStagedVerdict reads the mutation report a staged run just produced
// and turns it into the exit code Stryker itself does not provide.
//
// It deliberately does not reuse reportSurvivors: that function's cumulative
// note explains files an --incremental report carries from earlier runs,
// which never applies here — a staged run's incrementalFile is always empty,
// and every dropped file legitimately belongs to THIS run rather than to
// history reportSurvivors would otherwise misdescribe as "kept from earlier
// runs".
func reportStagedVerdict(reportPath string, scopes []tool.MutationScope, kept, dropped []string, stdout io.Writer) error {
	file, err := os.Open(reportPath)
	if err != nil {
		return fmt.Errorf("mutation ran but wrote no report at %s, so its verdict cannot be read: %w", reportPath, err)
	}
	defer func() { _ = file.Close() }()

	counts, err := tool.FileMutantCounts(file)
	if err != nil {
		return err
	}
	var disagreements []string
	for _, path := range dropped {
		if counts[path] > 0 {
			disagreements = append(disagreements, path)
		}
	}
	if len(disagreements) > 0 {
		sort.Strings(disagreements)
		return &ClassifierDisagreementError{Files: disagreements}
	}

	keptScopes := scopesForFiles(scopes, kept)

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("re-read the mutation report: %w", err)
	}
	if ignored, err := tool.IgnoredInScope(file, keptScopes); err == nil && len(ignored) > 0 {
		fmt.Fprintf(stdout, "\n%d mutant(s) were skipped by Stryker:\n\n", len(ignored))
		for _, entry := range ignored {
			fmt.Fprintf(stdout, "  %s\n", entry)
		}
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("re-read the mutation report: %w", err)
	}
	tally, err := tool.MutantTallyInScope(file, keptScopes)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "\n%d file(s), %d range(s), %d in-scope mutant(s): %d killed, %d survived, %d no coverage, %d timeout, %d errors, %d ignored\n",
		len(kept), len(keptScopes), tally.Total(),
		tally.Killed, tally.Survived, tally.NoCoverage, tally.Timeout, tally.Errors, tally.Ignored)

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("re-read the mutation report: %w", err)
	}
	survivors, err := tool.SurvivorsInScope(file, keptScopes)
	if err != nil {
		return err
	}
	if tally.Total() == 0 {
		fmt.Fprintln(stdout, "\nno mutants were generated in the staged ranges")
		return nil
	}
	if len(survivors) == 0 {
		if tally.Killed+tally.Timeout == 0 {
			fmt.Fprintln(stdout, "\nno mutant was tested: every in-scope mutant was skipped by Stryker (see the reasons above)")
			return nil
		}
		fmt.Fprintln(stdout, "\nEvery mutant was caught: these tests notice this code breaking.")
		return nil
	}

	fmt.Fprintf(stdout, "\n%s\n\n", survivorsHeading(survivors))
	for _, survivor := range survivors {
		fmt.Fprintf(stdout, "  %s\n", survivor)
		if survivor.Status == "NoCoverage" {
			fmt.Fprintln(stdout, "    No test executes this line at all; the fix is a test that calls it, not a disable directive.")
			continue
		}
		fmt.Fprintf(stdout, "    If equivalent, wrap the statement in `// Stryker disable %s: <reason>` … `// Stryker restore %s`; next-line does not reach call arguments such as dependency arrays.\n",
			survivor.Description, survivor.Description)
	}
	return &SurvivorsError{Survivors: survivors}
}

// stagedStrykerConfig is the config file a staged run hands Stryker in place
// of --mutate. It sits in the snapshot's copy of source, beside the project's
// own config, and is removed with the snapshot.
const stagedStrykerConfig = "dharness-staged.stryker.config.json"

// writeStagedStrykerConfig writes the config a staged run names on Stryker's
// command line, and returns its name relative to source.
//
// Naming a config file replaces the one Stryker would have found, so the
// project's own config — projectConfig, relative to source, or empty when
// there is none — is carried over whole: every key except mutate and
// jsonReporter keeps its value byte for byte, mutate holds entries, and
// jsonReporter is reportPath's own object (see overrideJSONReporterFileName).
// Relative paths inside the carried-over keys still resolve, because Stryker
// resolves them from the directory it runs in and the file sits in that same
// directory.
//
// jsonReporter.fileName is always reportPath — an absolute path this run
// alone owns — never the project's configured value: --staged runs in a
// snapshot that links every gitignored entry from the real project in rather
// than copying it, so a project-configured (or default) report path can be
// the same file on disk as a concurrent plain `dharness mutate` in that same
// checkout, and reading whichever report lands there second is a race, not a
// verdict.
//
// Only JSON reaches here: projectConfig is named by project.StrykerRunner,
// which refuses an executable config rather than evaluate it.
func writeStagedStrykerConfig(source, projectConfig string, entries []string, reportPath string) (string, error) {
	fields := map[string]json.RawMessage{}
	if projectConfig != "" {
		var err error
		if fields, err = strykerConfigFields(filepath.Join(source, projectConfig)); err != nil {
			return "", fmt.Errorf("Stryker config %s cannot carry the staged scope: %w; fix the JSON config and retry", projectConfig, err)
		}
	}
	// Marshalling a []string or a string cannot fail.
	fields["mutate"], _ = json.Marshal(entries)

	reporter, err := overrideJSONReporterFileName(fields["jsonReporter"], reportPath)
	if err != nil {
		return "", fmt.Errorf("Stryker config %s cannot carry the staged scope: %w; fix the JSON config and retry", projectConfig, err)
	}
	fields["jsonReporter"] = reporter

	if err := os.WriteFile(filepath.Join(source, stagedStrykerConfig), assembleJSONObject(fields), 0o600); err != nil {
		return "", fmt.Errorf("write the staged Stryker config: %w", err)
	}
	return stagedStrykerConfig, nil
}

// overrideJSONReporterFileName returns the jsonReporter object a staged run's
// generated config carries: fileName set to reportPath, and every other key
// existing held kept byte for byte — the same discipline
// writeStagedStrykerConfig applies to the rest of the project's config.
// existing is empty when the project never set jsonReporter at all.
func overrideJSONReporterFileName(existing json.RawMessage, reportPath string) (json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &fields); err != nil {
			return nil, fmt.Errorf("jsonReporter must be a JSON object: %w", err)
		}
		// A JSON null unmarshals into a nil map, not an error, and assigning
		// into a nil map panics; treat it the same as jsonReporter never
		// having been set at all.
		if fields == nil {
			fields = map[string]json.RawMessage{}
		}
	}
	// Marshalling a string cannot fail.
	fields["fileName"], _ = json.Marshal(reportPath)
	return assembleJSONObject(fields), nil
}

// assembleJSONObject renders fields as a JSON object with sorted keys, each
// value written as exactly the bytes given.
//
// Assembled by hand rather than by json.Marshal, which would compact and
// HTML-escape every RawMessage it re-serialises — collapsing whitespace and
// turning a "<" byte into the six characters backslash-u-0-0-3-c, measured on
// go1.27.0. Both forms parse back to the identical JSON value: json.Marshal's
// output would not be wrong, only reformatted. Hand assembly buys readable,
// byte-unchanged values, not correctness — nobody should later "simplify"
// this back to json.Marshal believing a bug was fixed.
func assembleJSONObject(fields map[string]json.RawMessage) []byte {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		name, _ := json.Marshal(key)
		buf.Write(name)
		buf.WriteByte(':')
		buf.Write(fields[key])
	}
	buf.WriteByte('}')
	return buf.Bytes()
}

// strykerConfigFields reads a JSON Stryker config as its top-level keys, each
// value kept as the exact bytes the file holds.
func strykerConfigFields(path string) (map[string]json.RawMessage, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	// Unmarshal answers a JSON null by leaving the map nil, not with an error.
	if fields == nil {
		return nil, errors.New("it holds null, not an object")
	}
	return fields, nil
}

// scopesForFiles filters scopes down to only the ranges whose Path is named
// in files — kept, for the ordinary verdict, so a survivor in a dropped
// file (which the self-check above already judges on its own terms) is
// never double-counted as an ordinary failure too.
func scopesForFiles(scopes []tool.MutationScope, files []string) []tool.MutationScope {
	want := make(map[string]bool, len(files))
	for _, f := range files {
		want[f] = true
	}
	var filtered []tool.MutationScope
	for _, scope := range scopes {
		if want[scope.Path] {
			filtered = append(filtered, scope)
		}
	}
	return filtered
}
