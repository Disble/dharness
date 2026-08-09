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

// StagedSourceFiles lists the staged paths the wrapped tools can analyse.
//
// --diff-filter=ACMR drops deletions: a deleted file has no content to check,
// and passing one to a tool is an error rather than a finding.
func StagedSourceFiles(dir string) ([]string, error) {
	out, err := gitOutput(dir, "diff", "--cached", "--name-only", "--diff-filter=ACMR")
	if err != nil {
		return nil, &NotAGitRepositoryError{Dir: dir, Cause: err}
	}

	var files []string
	for line := range strings.SplitSeq(string(out), "\n") {
		if path := strings.TrimSpace(line); path != "" && IsSourceFile(path) {
			files = append(files, path)
		}
	}
	return files, nil
}
