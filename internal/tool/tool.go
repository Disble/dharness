// Package tool holds the invocation of every CLI dharness wraps.
//
// These argument lists are the product. dharness owns no configuration file
// and translates no format: what it contributes over calling the tools by hand
// is exactly the flags below, in the order below. Keeping them in one file
// means a flag is changed in one place and reviewed as a whole.
package tool

import "strconv"

// Names as they appear in node_modules/.bin.
const (
	ReactDoctor = "react-doctor"
	Fallow      = "fallow"
	Stryker     = "stryker"
)

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

// AnySurvivorFails is the threshold below which Stryker exits non-zero.
//
// 100 means any surviving mutant fails the command. That is severe for a whole
// repository and exactly right for this one: mutate always runs over paths you
// named, whose tests you just wrote, and the question being asked is whether
// those tests would notice the code breaking. A survivor is a no.
const AnySurvivorFails = 100

// verdict returns the arguments that make the exit code mean something.
//
// Stryker's own default is `break: null`, documented as "never let your build
// fail": it reports surviving mutants and exits 0. A command whose failure has
// to be read out of its prose is not a command an agent — or a tired human —
// can act on, so dharness sets the threshold and the exit code becomes the
// answer.
//
// The reporters are trimmed for the same reason. The default set includes
// progress, which paints a live bar into a stream that is usually a pipe, and
// html, which writes a file report nobody in this flow opens.
func verdict(breakAt int) []string {
	return []string{
		"--break", strconv.Itoa(breakAt),
		"--reporters", "clear-text",
	}
}

// runner returns the --testRunner argument, or nothing when the project's own
// configuration already answers it.
//
// Stryker needs no config file at all — every option is available on the
// command line, and command line arguments overrule the file when both exist.
// That is why dharness never runs `stryker init`: the only thing init does that
// flags cannot is install the runner plugin, which is a package manager job.
func runner(testRunner string) []string {
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
// core count, and Stryker is the only tool here that can saturate the machine.
// A breakAt of zero leaves the threshold and the reporters to the project's own
// Stryker configuration, on the same rule as testRunner: what the project
// decided deliberately is not something a default should overrule.
func StrykerMutate(paths []string, testRunner string, concurrency, breakAt int) []string {
	args := []string{"run"}
	args = append(args, mutate(paths)...)
	args = append(args, runner(testRunner)...)
	args = append(args,
		"--incremental",
		"--force",
		"--concurrency", strconv.Itoa(concurrency),
	)
	if breakAt > 0 {
		args = append(args, verdict(breakAt)...)
	}
	return args
}

// StrykerDryRun runs the initial test run without mutating anything.
//
// The number of tests it executes is the measurement that decides whether
// scoped review is viable in a repository: the runner derives which tests to
// run from the import graph, and barrel files inflate that graph until every
// test counts as related.
func StrykerDryRun(paths []string, testRunner string) []string {
	args := []string{"run"}
	args = append(args, mutate(paths)...)
	args = append(args, runner(testRunner)...)
	return append(args, "--dryRunOnly")
}

func mutate(paths []string) []string {
	args := make([]string, 0, len(paths)*2)
	for _, path := range paths {
		args = append(args, "--mutate", path)
	}
	return args
}
