package project

import (
	"fmt"
	"os/exec"
	"strings"
)

// gitOutput is swappable so the gate can be tested without a repository. git
// is seamed and the filesystem is not: a temp directory is a real tree, but a
// real repository with a real index is process-global setup a test should not
// need.
var gitOutput = func(dir string, args ...string) ([]byte, error) {
	command := exec.Command("git", args...)
	command.Dir = dir
	return command.Output()
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
		"cannot read the staged files in %s: the gate scopes itself to the index, so it needs a git repository; run it from inside one",
		e.Dir,
	)
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
// only doctorConfigStep's first-write default needs the answer — Discover
// runs for every command and never asks about barrels.
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
