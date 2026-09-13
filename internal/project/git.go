package project

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/Disble/dharness/internal/runner"
)

// gitOutput is swappable so the gate can be tested without a repository. git
// is seamed and the filesystem is not: a temp directory is a real tree, but a
// real repository with a real index is process-global setup a test should not
// need.
//
// The seam has one blind spot, and it cost a release. A stub answers for the
// root the test already holds, so it cannot show what git would have answered
// under an inherited environment — and that is where the worktree defect lived,
// in this function, below every stub. See runner.Environ for what it was, and
// internal/project/worktree_test.go, which drives a real repository for exactly
// that reason.
var gitOutput = func(dir string, args ...string) ([]byte, error) {
	command := exec.Command("git", args...)
	command.Dir = dir
	command.Env = runner.Environ()
	return command.Output()
}

// GitOutput runs git in dir through this package's probe and returns its
// standard output.
//
// It is exported so every package that asks git a question shares one process
// boundary. The worktree defect lived in the environment that boundary hands
// the subprocess, and a second copy of it elsewhere is a second place the same
// defect has to be fixed — and a second place it can quietly come back.
func GitOutput(dir string, args ...string) ([]byte, error) {
	return gitOutput(dir, args...)
}

// SetGitOutputForTest replaces the git probe and returns a restore function.
func SetGitOutputForTest(probe func(string, ...string) ([]byte, error)) func() {
	previous := gitOutput
	gitOutput = probe
	return func() { gitOutput = previous }
}

// NotAGitRepositoryError reports that the index could not be read.
//
// The gate scopes itself to what is staged, so without an index it has no
// scope at all. Failing is deliberate: reporting success here would let a
// commit through a gate that never examined it.
type NotAGitRepositoryError struct {
	Dir   string
	Cause error
}

func (e *NotAGitRepositoryError) Unwrap() error { return e.Cause }

func (e *NotAGitRepositoryError) Error() string {
	return fmt.Sprintf(
		"cannot read the staged files in %s: the gate scopes itself to the index, so it needs a git repository; run it from inside one%s",
		e.Dir, whatFailed(e.Cause),
	)
}

// whatFailed renders the underlying failure beneath the message.
//
// The advice above is right for a caller standing outside a repository, and in
// the worktree defect it was printed to one who had been inside a worktree the
// whole time: the directory named was a path discovery had constructed and
// nothing had created. Following the advice taught that reader nothing, and the
// run had to be reproduced by hand to find out why.
//
// The distinction was never dharness's to phrase. A missing directory and a
// directory that is not a repository already fail with different sentences,
// written by git on its standard error or by the operating system when the
// subprocess could not even change directory. Both are in hand here; only
// printing them was missing.
func whatFailed(cause error) string {
	if cause == nil {
		return ""
	}

	said := cause.Error()
	var exit *exec.ExitError
	if errors.As(cause, &exit) {
		if diagnostic := gitDiagnostic(exit.Stderr); diagnostic != "" {
			said = diagnostic
		}
	}
	return "\n\n    " + strings.ReplaceAll(said, "\n", "\n    ")
}

// gitDiagnostic is git's standard error up to the usage block it appends when
// it rejects the command line outright.
//
// All of stderr is not printable. Outside a repository `git diff --cached`
// falls back to --no-index mode, reports `error: unknown option 'cached'` and
// then prints forty lines of usage — which would bury the one line that says
// what happened under a manual page nobody asked for.
//
// Everything before the usage is kept rather than only the first line, because
// git's most consequential message here is multi-line: dubious ownership names
// the repository, both users, and the safe.directory command that fixes it.
func gitDiagnostic(stderr []byte) string {
	var lines []string
	for line := range strings.SplitSeq(string(stderr), "\n") {
		if strings.HasPrefix(line, "usage:") {
			break
		}
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// HasCommits reports whether the repository has any history behind the index.
//
// It exists because a repository with no commits has nothing to compare a
// change against, which is not a broken configuration — it is the first
// commit, and it happens right after adoption.
func HasCommits(dir string) bool {
	_, err := gitOutput(dir, "rev-parse", "--verify", "HEAD")
	return err == nil
}

// StagedSourceFiles lists the staged paths the wrapped tools can analyse.
//
// --diff-filter=ACMR drops deletions: a deleted file has no content to check,
// and passing one to a tool is an error rather than a finding.
//
// -z is not a detail. Without it git quotes any path that is not plain ASCII —
// `src/café.ts` arrives as the literal `"src/caf\303\251.ts"`, quotes included.
// filepath.Ext of that is `.ts"`, which matches no extension, so IsSourceFile
// drops it and the file leaves the gate's scope. When it was the only staged
// file the gate then reported nothing to check and exited 0: a pass over a
// change nothing looked at. -z emits the bytes raw, separated by NUL.
func StagedSourceFiles(dir string) ([]string, error) {
	out, err := gitOutput(dir, "diff", "--cached", "--name-only", "--diff-filter=ACMR", "-z")
	if err != nil {
		return nil, &NotAGitRepositoryError{Dir: dir, Cause: err}
	}

	var files []string
	for _, path := range splitNUL(out) {
		if IsSourceFile(path) {
			files = append(files, path)
		}
	}
	return files, nil
}

// StagedSourceFiles lists the staged paths that belong to this project's JS
// source, which is the only part of the repository the wrapped tools can say
// anything about.
//
// git reports paths relative to the repository, so the scope is applied here
// rather than by running git from the subdirectory: a Wails repository stages
// Go and TypeScript in one commit, and the Go half must not decide whether the
// frontend gate runs.
func (p Project) StagedSourceFiles() ([]string, error) {
	staged, err := StagedSourceFiles(p.Root)
	if err != nil || !p.HasSource() {
		return nil, err
	}

	prefix := p.SourceRel()
	if prefix == "" {
		return staged, nil
	}

	var scoped []string
	for _, file := range staged {
		if strings.HasPrefix(file, prefix+"/") {
			scoped = append(scoped, file)
		}
	}
	return scoped, nil
}

// StagedDiff renders the index as a unified diff, in the paths the tool that
// reads it will see.
//
// It exists because `fallow audit` on its own audits the wrong thing. Measured
// against fallow 3.16.0 on a branch of 24 files with one file staged: bare
// audit reported "Audit scope: 25 changed files vs main (local main)" and
// exited 1 on 24 complexity findings and 8 clone groups, none of them in the
// staged file. fallow's own --help says why — with no base flag the base is
// "the git merge-base against the branch's upstream or the remote default" —
// and --gate new-only does not save it, because "new" is measured against that
// same base, so everything the branch did since main counts as introduced.
// See FallowAudit for the flags this diff feeds.
//
// Four of the five flags are the same determinism the staged Go mutation tool
// already asks for (tools/mutationstaged): core.quotePath=false so a non-ASCII
// path arrives as bytes rather than as an escaped literal, --no-ext-diff and
// --no-renames so a configured external differ or rename detection cannot
// reshape the hunks, and --diff-filter=ACMR so the diff and StagedSourceFiles
// describe the same set rather than two sets that drift.
//
// -U0 is measured rather than assumed: a staged edit that pushed an existing
// function over the complexity threshold was reported identically at -U0 and
// at the default -U3, because fallow anchors a finding to the function holding
// a changed line rather than to the line. -U0 is the narrower of two answers
// that agree.
//
// --relative, and running from Source rather than Root, is the one that is not
// housekeeping. git reports paths from the repository root while fallow reads
// them from the directory it runs in, so in a split layout the diff would
// offer frontend/src/a.ts to a tool that knows the file as src/a.ts. Measured:
// a diff whose paths match nothing is not an error and is not empty — fallow
// ignores the filter and falls back to file-level scope, silently widening
// what the gate looked at.
func (p Project) StagedDiff() ([]byte, error) {
	out, err := gitOutput(p.Source,
		"-c", "core.quotePath=false",
		"diff", "--cached", "--relative", "-U0",
		"--no-ext-diff", "--no-renames", "--diff-filter=ACMR")
	if err != nil {
		return nil, &NotAGitRepositoryError{Dir: p.Source, Cause: err}
	}
	return out, nil
}

// StagedSourceFilesFromSource is StagedSourceFiles with the source prefix
// removed, for a tool that runs in p.Source and takes explicit paths.
// StagedSourceFiles already filtered to that prefix, so this only strips it.
func (p Project) StagedSourceFilesFromSource() ([]string, error) {
	staged, err := p.StagedSourceFiles()
	if err != nil {
		return nil, err
	}

	prefix := p.SourceRel()
	if prefix == "" {
		return staged, nil
	}

	rebased := make([]string, len(staged))
	for i, file := range staged {
		rebased[i] = strings.TrimPrefix(file, prefix+"/")
	}
	return rebased, nil
}

// PublishesBarrels reports whether this project's resolved Source tree
// publishes at least one index.ts/index.tsx barrel, asked of git rather than
// walked — matching Discover's own precedent of asking the tool: the index
// already excludes node_modules and every generated tree for free, and
// running the pathspec in Source scopes the answer to the JS project, so a
// Wails repository's Go half never has to answer for its frontend.
//
// It answers from the index (git ls-files), not the working tree: an
// unstaged barrel is not yet this project's published architecture, so it
// does not count. That is deliberate — see the design's threat matrix.
//
// The leading "*/" in each pathspec is deliberate too: a single index.ts at
// the source root is a package entry point, not evidence of barrel
// publishing, and requires at least one directory component to match.
//
// A method rather than a Project field: the probe costs a subprocess, and
// only DefaultSeverity's folder-ownership default needs the answer —
// Discover runs for every command and never asks about barrels.
func (p Project) PublishesBarrels() bool {
	if !p.InRepository || !p.HasSource() {
		return false
	}
	out, err := gitOutput(p.Source, "ls-files", "-z", "--", "*/index.ts", "*/index.tsx")
	if err != nil {
		return false
	}
	return len(splitNUL(out)) > 0
}

// splitNUL reads git's -z output: NUL-separated paths, in the bytes the
// filesystem holds, with no quoting and no trailing empty record.
func splitNUL(out []byte) []string {
	var paths []string
	for field := range strings.SplitSeq(string(out), "\x00") {
		if field != "" {
			paths = append(paths, field)
		}
	}
	return paths
}

// HooksDir names the directory git actually runs this repository's hooks
// from, as an absolute path.
//
// Asking git is the difference between a check that works and one that is
// always wrong: `.git/hooks` is only the default, and a repository that sets
// core.hooksPath — dharness's own does, and so does every repository that
// keeps its hooks under version control — runs them from somewhere else.
// `rev-parse --git-path hooks` resolves that setting; joining ".git/hooks"
// by hand ignores it and reports a wired gate as missing forever.
func HooksDir(root string) (string, error) {
	out, err := gitOutput(root, "rev-parse", "--path-format=absolute", "--git-path", "hooks")
	if err != nil {
		return "", &NotAGitRepositoryError{Dir: root, Cause: err}
	}
	return strings.TrimSpace(string(out)), nil
}
