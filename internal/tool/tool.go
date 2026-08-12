// Package tool holds the invocation of every CLI dharness wraps.
//
// These argument lists are the product. dharness owns no configuration file
// and translates no format: what it contributes over calling the tools by hand
// is exactly the flags below, in the order below. Keeping them in one file
// means a flag is changed in one place and reviewed as a whole.
package tool

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Disble/dharness/internal/runner"
)

// Binary names exposed by the wrapped packages.
const (
	ReactDoctor = "react-doctor"
	Fallow      = "fallow"
	Stryker     = "stryker"
	ESLint      = "eslint"
)

const strykerCoreLatest = "@stryker-mutator/core@latest"

var strykerRunnerPackages = map[string]string{
	"vitest": "@stryker-mutator/vitest-runner",
	"jest":   "@stryker-mutator/jest-runner",
}

// ErrUnsupportedStrykerExecution identifies a package manager or runner that
// cannot form the required transient Core-plus-runner environment.
var ErrUnsupportedStrykerExecution = errors.New("unsupported Stryker remote execution")

// LatestSpec names the current release of the package behind a wrapped binary.
// The explicit tag matters: npx once resolved react-doctor 0.2.1 from cache
// while react-doctor@latest resolved 0.9.11.
func LatestSpec(binary string) string {
	if binary == Stryker {
		return strykerCoreLatest
	}
	return binary + "@latest"
}

// StrykerCommand provisions Core and its selected official runner in one
// transient package environment. Every argument is a separate token so paths
// and package specs remain safe through the Windows cmd.exe shim.
func StrykerCommand(packageManager string, yarnPnP bool, dir, testRunner string, configuredAppendPlugins []string, args ...string) (runner.Command, error) {
	runnerPackage, ok := strykerRunnerPackages[testRunner]
	if !ok {
		return runner.Command{}, fmt.Errorf("%w: Stryker has no supported remote package for test runner %q; use vitest or jest", ErrUnsupportedStrykerExecution, testRunner)
	}

	runnerLatest := runnerPackage + "@latest"
	if packageManager == "yarn" && yarnPnP {
		return runner.Command{}, fmt.Errorf("%w: Stryker cannot run in a Yarn Plug'n'Play project because the remote runner cannot resolve the project's test dependencies; configure nodeLinker: node-modules, run yarn install, then retry", ErrUnsupportedStrykerExecution)
	}
	if !knownManager(packageManager) {
		return runner.Command{}, fmt.Errorf("%w: Stryker has no transient multi-package route for package manager %q; use npm, pnpm, yarn, or bun", ErrUnsupportedStrykerExecution, packageManager)
	}
	name, remoteArgs := transientPackages(packageManager, []string{strykerCoreLatest, runnerLatest}, Stryker)

	appendPlugins := append([]string{}, configuredAppendPlugins...)
	if !contains(appendPlugins, runnerPackage) {
		appendPlugins = append(appendPlugins, runnerPackage)
	}
	commandArgs := append(remoteArgs, args...)
	commandArgs = append(commandArgs, "--appendPlugins", strings.Join(appendPlugins, ","))

	return runner.Command{
		Label:       Stryker,
		Name:        name,
		Args:        commandArgs,
		Dir:         dir,
		LowPriority: true,
	}, nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// ReactDoctorStaged scopes react-doctor to the index.
//
// Verified against react-doctor 0.9.11 by running it, not by reading about it.
// Its own --help examples advertise a --blocking flag that the published
// binary rejects outright; the exit code already fails on errors by default,
// which is what that flag was for.
//
// --no-dead-code is the ownership split made real: fallow owns the repository
// graph — dead code, dependencies, cycles — and running both analysers over it
// buys duplicated work and two sets of findings that can disagree.
//
// --no-score also covers telemetry, which is documented as its alias. It and
// --no-supply-chain are both on by default and both reach the network, which a
// gate that runs on every commit must not do.
func ReactDoctorStaged() []string {
	return []string{
		"--staged",
		"--no-dead-code",
		"--no-score",
		"--no-supply-chain",
		"-y",
	}
}

// FallowAudit reports on changed files and returns a pass/warn/fail verdict.
//
// Verified against fallow 3.x by running it. Two behaviours matter and only one
// of them needed a flag:
//
// audit already exits 1 on a fail verdict, and already attributes findings so
// that only the ones this changeset introduced affect it — its --gate default
// is new-only. The ratchet is theirs, not something dharness has to build.
//
// The base is left to audit as well, after trying to improve on it and being
// wrong. Passing --changed-since HEAD looked safer for a repository with no
// remote, but --changed-since resolves a commit range, and a staged change is
// not a commit: in a fresh repository it failed outright on HEAD...HEAD.
//
// Measured instead: in a local repository with no remote at all, bare audit
// resolved "1 changed file vs master (local master)", saw the staged file, and
// exited 1 on the finding. fallow's own installed pre-commit hook runs audit
// with no base flag for the same reason.
func FallowAudit() []string {
	return []string{"audit"}
}

// FallowDupes enforces the duplication ceiling, which audit does not.
//
// Measured against fallow 3.14.0: a repository at 80% duplication with
// `duplicates.threshold` set to 3 passes `audit` with exit 0 and fails
// `dupes` with exit 1. audit's verdict is scoped to what the changeset
// introduces — its `--gate` default is new-only — and a percentage over the
// whole repository is a different question, which `dupes` is the command
// that asks.
//
// No `--threshold` flag. The number lives in the config dharness owns, where
// the project can read it, argue with it and override it; a flag here would
// be a ceiling nobody could find, and command-line arguments overrule the
// config file, so it would also silently win over a project that had
// disagreed on purpose.
//
// This is a wall rather than a ratchet, and deliberately so after being
// weighed: a repository that inherits duplication above the ceiling is
// blocked until it either refactors or raises the number in its own config.
// `--changed-since` would scope it, but it resolves a commit range and a
// staged change is not a commit — the same reason FallowAudit passes no base.
func FallowDupes() []string {
	return []string{"dupes"}
}

// runner returns the --testRunner argument, or nothing when the project's own
// configuration already answers it.
//
// Command-line arguments overrule the config file, so this stays empty when the
// project's own config selected its runner.
func testRunnerArgs(testRunner string) []string {
	if testRunner == "" {
		return nil
	}
	return []string{"--testRunner", testRunner}
}

// StrykerMutate mutates exactly the given paths.
//
// --mutate accepts `file:startLine-endLine`, and a range cannot be combined
// with a glob in the same entry, so the list is always fully computed rather
// than expressed as a pattern.
//
// --incremental reuses the previous report and --force reruns the current
// scope anyway. The pair is deliberate: without --force a scoped rerun would
// reuse stale results, and without --incremental the accumulated report for
// everything outside the scope would be lost.
//
// --concurrency is passed explicitly because the default is derived from the
// core count. It stays deliberately small rather than sized from whatever the
// machine has free at this instant: the process already runs at reduced
// priority, which yields continuously instead of going stale a minute into a
// long run, and a scoped mutation over named paths does not need more.
// The verdict is not a flag. Stryker exposes no --break and no dotted
// equivalent, so the exit code has to come from reading the report it writes;
// see Survivors.
func StrykerMutate(paths []string, testRunner, incrementalFile, sandbox string, concurrency int) []string {
	args := []string{"run"}
	args = append(args, mutate(paths)...)
	args = append(args, testRunnerArgs(testRunner)...)
	args = append(args,
		"--incremental",
		"--incrementalFile", incrementalFile,
		"--force",
		"--concurrency", strconv.Itoa(concurrency),
		// The sandbox goes where dharness can find it again and clean it.
		// cleanTempDir only runs on a successful exit by default, so a failed
		// run leaves a copy of the project inside the repository.
		"--tempDirName", sandbox,
		"--cleanTempDir", "always",
		// clear-text so a person can read it, json so dharness can reach a
		// verdict. progress paints a live bar into what is usually a pipe, and
		// html writes a file report nobody in this flow opens.
		"--reporters", "clear-text,json",
	)
	return args
}

// StrykerDryRun runs the initial test run without mutating anything.
//
// The number of tests it executes is the measurement that decides whether
// scoped review is viable in a repository: the runner derives which tests to
// run from the import graph, and barrel files inflate that graph until every
// test counts as related.
//
// It carries the same --concurrency as the mutation run, and leaving it off was
// a hole rather than a decision. The initial test run is where Stryker starts
// its runner processes, so the phase the budget was written for is the one that
// went unbudgeted: measured on a 20-thread machine, a dry run announced
// "Creating 19 test runner process(es)" and took longer than the mutation it
// was measuring. The budget is a resource budget, and it does not stop applying
// because this run mutates nothing.
func StrykerDryRun(paths []string, testRunner string, concurrency int) []string {
	args := []string{"run"}
	args = append(args, mutate(paths)...)
	args = append(args, testRunnerArgs(testRunner)...)
	return append(args,
		"--dryRunOnly",
		"--concurrency", strconv.Itoa(concurrency),
		"--reporters", "clear-text,json",
	)
}

// ESLintStaged lints exactly the staged files, by explicit path.
//
// No --cache. It writes .eslintcache into the project's tree, which §03
// would then have to account for as project-owned or dharness-owned, and it
// is stale-prone across branches — a cache keyed on a file's content and
// the config's mtime reports a clean file after a branch switch that
// changed neither. The staged file list is the larger win and it is already
// available. Caching is a measured optimisation, deferred, and this comment
// is here so it is not re-added as an obvious improvement.
//
// The paths arrive relative to p.Source, not to the repository: the command
// runs where the package manager installed ESLint, and git reports paths
// from the repository root — internal/project's
// StagedSourceFilesFromSource does the stripping, so this function stays a
// pure pass-through rather than duplicating that path math.
func ESLintStaged(files []string) []string {
	return files
}

func mutate(paths []string) []string {
	args := make([]string, 0, len(paths)*2)
	for _, path := range paths {
		args = append(args, "--mutate", path)
	}
	return args
}
