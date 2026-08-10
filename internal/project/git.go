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
