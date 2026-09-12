package runner

import (
	"os"
	"slices"
	"strings"
)

// workingTreePinning names the two variables that tell git which working tree
// to operate on, overriding the one it would have discovered.
var workingTreePinning = []string{"GIT_DIR", "GIT_WORK_TREE"}

// Environ is the environment every process dharness launches runs in: this
// one's, minus the working-tree pinning it inherited.
//
// git exports GIT_DIR into every hook it runs, and in a linked worktree that
// path lives under the main checkout rather than under the worktree. An
// explicit GIT_DIR switches discovery off: git stops walking up from the
// directory it was handed and takes the working tree to be the current
// directory instead.
//
// Measured on 2026-09-12 against a worktree whose hook manager runs the gate
// from frontend/, both halves of the same inheritance:
//
//   - dharness asked `rev-parse --show-toplevel` and was told frontend/, then
//     joined the configured source prefix onto it — <worktree>/frontend/frontend,
//     a directory that does not exist. The gate died before reading a file.
//   - react-doctor --staged, handed the same GIT_DIR, refused with
//     "configuration differs between the index and worktree:
//     frontend/package.json, package.json". Nothing differed. Run in the same
//     worktree against the same index with GIT_DIR cleared, it scanned.
//
// So this belongs to launching a process, not to asking git a question. Every
// tool dharness wraps reads the repository, and every one of them is meant to
// read the directory it was pointed at. Scrubbing at the one place that starts
// a child process covers the tools that exist and the ones added later.
//
// GIT_INDEX_FILE is deliberately kept. It names the index the commit is being
// built from, and `git commit --only <paths>` points it at a temporary one — a
// gate that dropped it would scope itself to everything staged rather than to
// the commit under way.
func Environ() []string {
	return withoutWorkingTreePinning(os.Environ())
}

func withoutWorkingTreePinning(environ []string) []string {
	kept := make([]string, 0, len(environ))
	for _, entry := range environ {
		if !pinsWorkingTree(entry) {
			kept = append(kept, entry)
		}
	}
	return kept
}

// pinsWorkingTree reports whether an environment entry is one of the pinning
// variables. It compares the name without regard to case, because Windows —
// the platform the worktree failure was reported from — reads git_dir and
// GIT_DIR as one variable, and so does git.
func pinsWorkingTree(entry string) bool {
	name, _, ok := strings.Cut(entry, "=")
	if !ok {
		return false
	}
	return slices.ContainsFunc(workingTreePinning, func(pinning string) bool {
		return strings.EqualFold(name, pinning)
	})
}
