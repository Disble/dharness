package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// StatePath returns a writable path for state that belongs to this repository
// but must never be committed.
//
// It resolves the git common directory, which is the right home for three
// reasons that no other location has all of: git already ignores everything
// inside it, worktrees of the same repository share it, and deleting the
// repository deletes the state with it. A directory keyed by a hash of the
// project path — the previous approach here — leaks state forever the first
// time a repository is moved or removed.
func (p Project) StatePath(name string) (string, error) {
	out, err := gitOutput(p.Root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", &NotAGitRepositoryError{Dir: p.Root, Cause: err}
	}

	dir := filepath.Join(strings.TrimSpace(string(out)), "dharness")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	return filepath.Join(dir, name), nil
}

// DiscardIfUnreadable removes a cache file that cannot be parsed.
//
// A corrupt incremental report is worse than a missing one: Stryker fails on it
// rather than falling back to a full run, and the failure looks like a problem
// with the code under test.
func DiscardIfUnreadable(path string, readable func([]byte) bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if !readable(raw) {
		_ = os.Remove(path)
	}
}
