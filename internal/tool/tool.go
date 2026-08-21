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

// StrykerPackages names what the project must have installed to mutate, each
// spec pinned to @latest.
//
// dharness installs these instead of provisioning a transient environment, and
// the reason is measured rather than reasoned. Stryker's TSConfigPreprocessor
// imports typescript dynamically from its own location, so a copy of Core
// unpacked into a bunx or npx temporary directory cannot resolve the compiler
// the project installed: every project holding a tsconfig.json died with
// ERR_MODULE_NOT_FOUND before its first mutant. The project's own node_modules
// is the one place Core and the compiler resolve each other.
//
// The transient route also did not do what it claimed. Measured on 2026-08-13,
// bun keeps only the last --package of an invocation and drops the rest, so the
// environment built for "Core and its runner together" held the runner alone
// and reached Core transitively, at the runner's pinned range instead of the
// requested @latest.
//
// @latest on every run is that route's one real benefit, kept: the manager
// re-resolves the tag, so a project sitting on 8.2.6 is lifted to 9.6.1 by the
// install itself and dharness never compares a version. Measured — that exact
// upgrade took 5.7s, and a run with nothing to do costs 436ms and leaves
// package.json byte-identical.
func StrykerPackages(packageManager string, yarnPnP bool, testRunner string) ([]string, error) {
	runnerPackage, ok := strykerRunnerPackages[testRunner]
	if !ok {
		return nil, fmt.Errorf("%w: Stryker has no supported package for test runner %q; use vitest or jest", ErrUnsupportedStrykerExecution, testRunner)
	}
	if packageManager == "yarn" && yarnPnP {
		return nil, fmt.Errorf("%w: Stryker cannot run in a Yarn Plug'n'Play project because it exposes no node_modules for Stryker to resolve the project's compiler through; configure nodeLinker: node-modules, run yarn install, then retry", ErrUnsupportedStrykerExecution)
	}
	if !knownManager(packageManager) {
		return nil, fmt.Errorf("%w: dharness cannot install Stryker with package manager %q; use npm, pnpm, yarn, or bun", ErrUnsupportedStrykerExecution, packageManager)
	}
	return []string{strykerCoreLatest, runnerPackage + "@latest"}, nil
}

// StrykerLocal invokes the copy of Stryker the project has installed. The path
// is the command name, never an argument to a remote executor.
func StrykerLocal(binaryPath, dir, testRunner string, configuredAppendPlugins []string, args ...string) runner.Command {
	appendPlugins := append([]string{}, configuredAppendPlugins...)
	if runnerPackage, ok := strykerRunnerPackages[testRunner]; ok && !contains(appendPlugins, runnerPackage) {
		appendPlugins = append(appendPlugins, runnerPackage)
	}

	commandArgs := append([]string{}, args...)
	if len(appendPlugins) > 0 {
		commandArgs = append(commandArgs, "--appendPlugins", strings.Join(appendPlugins, ","))
	}

	return runner.Command{
		Label:       Stryker,
		Name:        binaryPath,
		Args:        commandArgs,
		Dir:         dir,
		LowPriority: true,
	}
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
//
// There is no flag for the "Agent guidance" block — thirteen lines of
// instructions addressed to a model — or for the marketing footer that
// follows it, which together are most of what a clean run prints. Measured
// against 0.9.12's own --help: the only switch that removes them is --json,
// documented as "suppresses other output", and taking it would make dharness
// the renderer of react-doctor's findings rather than the thing that runs it.
// That is the boundary §03 draws and the same one FallowAudit's notice is
// left alone for. Filtering the block out of the relayed stream is the same
// trade wearing a different hat. So it stays, and the ask belongs upstream.
func ReactDoctorStaged() []string {
	return []string{
		"--staged",
		"--no-dead-code",
		"--no-score",
		"--no-supply-chain",
		"-y",
	}
}

// FallowAudit reports on the staged change and returns a pass/warn/fail
// verdict.
//
// audit already exits 1 on a fail verdict and already ratchets — its --gate
// default is new-only, so inherited findings are reported without affecting
// the verdict. That part was right and is still not dharness's to build.
//
// What was wrong was leaving the base to audit. The claim this comment used to
// carry — that bare audit is scoped to the changeset — held only in the
// repository it was measured in, which had one changed file, so the branch's
// diff and the staged change were the same set. fallow's --help states the
// real rule: with no base flag the base is "the git merge-base against the
// branch's upstream or the remote default". Measured against fallow 3.16.0 on
// a branch of 24 files with one clean file staged, bare audit exited 1 on 24
// complexity findings and 8 clone groups that the staged change did not touch.
// new-only does not help, because "new" is measured against that same base.
//
// Two flags, and each covers a hole the other leaves:
//
// --changed-since HEAD moves the base to the last commit, which is what the
// ratchet needed all along. Measured: a harmless edit inside a file already
// carrying debt now passes, because against HEAD that debt is correctly
// inherited rather than introduced. The recorded objection to this flag — that
// it "resolves a commit range that cannot see the index" — is false as
// measured on 3.16.0, which reported "Audit scope: 1 changed file vs HEAD" for
// a change that existed only in the index. What is true is that it needs a
// commit to point at, and RunCheck already guards that with HasCommits.
//
// --diff-stdin takes the staged diff and is not a refinement. On its own,
// --changed-since HEAD reported a finding in a file that was never staged —
// the working tree is not the index, and dharness's gate is the index (see
// NotAGitRepositoryError). The diff filter is what holds that line.
//
// The order matters in one direction only: --changed-since HEAD is the safety
// net. Measured, a diff whose paths match nothing is ignored rather than
// rejected, and scope falls back to the file level — which is still the staged
// file set, not the branch. The reverse is the trap: an *empty* diff filters
// everything and audit exits 0 over a genuinely bad staged change, which is
// why RunCheck must never reach this stage with nothing staged.
//
// fallow notices the pair and says so on every run:
//
//	fallow: --diff-file precedes --changed-since for line-level filtering;
//	--changed-since still scopes file discovery. Drop one of them to disable
//	this combination.
//
// That notice is left where it is — dharness reports what a tool says rather
// than editing it — but its closing advice is the one thing not to follow.
// Both flags were measured, and dropping either reopens a hole the other was
// covering: without --changed-since the base returns to the merge-base, and
// without the diff the scope reaches the working tree.
func FallowAudit() []string {
	return []string{"audit", "--changed-since", "HEAD", "--diff-stdin"}
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

// FallowConfigPath prints the resolved config file path, and exits 3 when
// the project has no fallow config at all. It is the probe: --format json
// exits 0 even with no config file (it prints defaults), so it can never be
// used to detect absence. Measured against the reference project: local
// binary roughly 350 ms.
func FallowConfigPath() []string {
	return []string{"config", "--path"}
}

// FallowConfigJSON prints the fully resolved config — `extends` already
// applied — as JSON on stdout. Measured against the reference project: 26
// top-level keys, and the `loaded config: <path>` preamble goes to stderr,
// so stdout needs no stripping. Roughly 350 ms on a local binary.
func FallowConfigJSON() []string {
	return []string{"config", "--format", "json"}
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
//
// --no-warn-ignored answers a warning dharness itself causes. Naming files
// explicitly makes ESLint report each one its config ignores — "File ignored
// because of a matching ignore pattern" — and the layer dharness writes
// ignores the .dharness/ directory, so committing a change there prints the
// warning about a file list dharness built rather than about anything the
// user did. It is ESLint's own flag for exactly this, in its own --help
// ("Suppress warnings when the file list includes ignored files"), and it
// suppresses nothing actionable: the file list is not the user's to change.
func ESLintStaged(files []string) []string {
	return append([]string{"--no-warn-ignored"}, files...)
}

func mutate(paths []string) []string {
	args := make([]string, 0, len(paths)*2)
	for _, path := range paths {
		args = append(args, "--mutate", path)
	}
	return args
}
