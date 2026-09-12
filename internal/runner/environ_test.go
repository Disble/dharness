package runner

import (
	"io"
	"os"
	"slices"
	"strings"
	"testing"
)

// TestEnvironDropsTheInheritedWorkingTree pins which variables survive into a
// child process.
//
// GIT_DIR and GIT_WORK_TREE name a working tree, and when git runs a hook they
// name the one the parent process was operating on. Everything dharness
// launches is meant to read the directory it was pointed at, so inherited
// pinning is never what it means. The pair goes together: GIT_WORK_TREE left
// behind without GIT_DIR would override the discovered tree just as wrongly.
func TestEnvironDropsTheInheritedWorkingTree(t *testing.T) {
	environ := []string{
		"PATH=/usr/bin",
		"GIT_DIR=D:/main/.git/worktrees/wt",
		"GIT_WORK_TREE=D:/main",
		"GIT_INDEX_FILE=D:/main/.git/worktrees/wt/next-index.lock",
		"GIT_AUTHOR_NAME=someone",
	}

	got := withoutWorkingTreePinning(environ)

	for _, dropped := range []string{"GIT_DIR", "GIT_WORK_TREE"} {
		if slices.ContainsFunc(got, func(entry string) bool {
			return strings.HasPrefix(entry, dropped+"=")
		}) {
			t.Errorf("Environ() kept %s; it pins git to another working tree", dropped)
		}
	}
	for _, kept := range []string{
		"PATH=/usr/bin",
		"GIT_INDEX_FILE=D:/main/.git/worktrees/wt/next-index.lock",
		"GIT_AUTHOR_NAME=someone",
	} {
		if !slices.Contains(got, kept) {
			t.Errorf("Environ() dropped %q, which does not pin a working tree", kept)
		}
	}
}

// TestEnvironMatchesVariableNamesTheWayWindowsDoes keeps the filter on the name
// rather than on its spelling. Environment names are case-insensitive on
// Windows — the platform the worktree failure was reported from — so git reads
// git_dir and GIT_DIR as one variable, and so must this.
func TestEnvironMatchesVariableNamesTheWayWindowsDoes(t *testing.T) {
	got := withoutWorkingTreePinning([]string{"git_dir=D:/main/.git", "Git_Work_Tree=D:/main"})
	if len(got) != 0 {
		t.Errorf("Environ() = %q, want both dropped regardless of case", got)
	}
}

// TestEnvironKeepsAnEntryWithoutASeparator proves the filter reads a name
// before deciding. A malformed entry carries no name to match, and dropping it
// would edit an environment this only narrows.
func TestEnvironKeepsAnEntryWithoutASeparator(t *testing.T) {
	if got := withoutWorkingTreePinning([]string{"GIT_DIR"}); !slices.Equal(got, []string{"GIT_DIR"}) {
		t.Errorf("Environ() = %q, want the entry left alone", got)
	}
}

// TestTheProcessDoesNotInheritTheWorkingTreePinning is the end the filter
// exists for.
//
// react-doctor, fallow and Stryker each read the repository themselves, and
// react-doctor was measured refusing under an inherited GIT_DIR — "configuration
// differs between the index and worktree" when nothing differed. Asserting on
// the filter alone would have passed while execute handed the child os.Environ()
// anyway, which is the defect: the environment was never set on the process at
// all.
func TestTheProcessDoesNotInheritTheWorkingTreePinning(t *testing.T) {
	t.Setenv(environHelperEnv, "1")
	t.Setenv("GIT_DIR", "D:/main/.git/worktrees/wt")
	t.Setenv("GIT_WORK_TREE", "D:/main")
	t.Setenv("GIT_INDEX_FILE", "D:/main/.git/worktrees/wt/next-index.lock")

	var out strings.Builder
	if err := Run(Command{Name: os.Args[0]}, &out, io.Discard); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}

	reported := strings.Fields(out.String())
	for _, dropped := range []string{"GIT_DIR", "GIT_WORK_TREE"} {
		if slices.Contains(reported, dropped) {
			t.Errorf("the process inherited %s; it names a working tree dharness was not asked about", dropped)
		}
	}
	if !slices.Contains(reported, "GIT_INDEX_FILE") {
		t.Error("the process did not inherit GIT_INDEX_FILE; it names the index the commit is being built from")
	}
}
