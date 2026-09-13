// Package staged turns the git index into the mutation scope a staged change
// justifies: the exact line ranges it added, expressed as tool.MutationScope
// values dharness's own --mutate argument already knows how to carry.
package staged

import (
	"strings"

	"github.com/Disble/dharness/internal/project"
)

// gitOutput is swappable so parsing can be tested without a repository, and a
// real repository can still exercise the rename pairing and partial-staging
// checks a stub cannot honestly answer for.
//
// The seam is this package's own; the process behind it is not. By default it
// runs git through internal/project, which owns the one place dharness starts
// git and the environment git inherits there. That environment is where the
// worktree defect lived (see runner.Environ), and a copy of the probe here
// would be a second place to fix it. real_repo_test.go's inherited GIT_DIR test
// fails if this stops going through that boundary.
var gitOutput = project.GitOutput

// SetGitOutputForTest replaces the git probe and returns a restore function.
func SetGitOutputForTest(probe func(string, ...string) ([]byte, error)) func() {
	previous := gitOutput
	gitOutput = probe
	return func() { gitOutput = previous }
}

// splitNUL reads a plain git -z listing (ls-files and the like): NUL-separated
// paths, with no path ever legitimately empty.
//
// numstat's own -z output is not this shape — a rename's empty inline path
// field is a real signal, not noise to drop — and is split separately by
// numstatFields in scope.go, which keeps every field including empty ones.
func splitNUL(out []byte) []string {
	var paths []string
	for _, field := range strings.Split(string(out), "\x00") {
		if field != "" {
			paths = append(paths, field)
		}
	}
	return paths
}
