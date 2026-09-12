package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The tests in this file drive a real repository, which every other test in
// this package deliberately avoids — the git probe is seamed precisely so a
// temp directory is enough.
//
// That seam is also why this defect shipped. It replaces gitOutput, and the
// defect lived inside gitOutput: in the environment the subprocess inherits.
// Every stub in the suite answers --show-toplevel with the root the test
// already has in mind, so the one question that mattered — what would git
// have said here? — was never asked. No stub can reproduce an inherited
// GIT_DIR. Only git can.

// ambientGitPinning are the variables that make a git command operate on a
// repository other than the one the test is holding.
var ambientGitPinning = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_INDEX_FILE",
	"GIT_COMMON_DIR",
	"GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_PREFIX",
}

// isolateFromTheAmbientRepository clears the pinning this process inherited,
// restoring it when the test ends.
//
// This suite runs inside this repository's own pre-commit gate, which means it
// runs inside a `git commit` — so GIT_DIR and GIT_INDEX_FILE already name
// dharness itself before the first line of a test. Under that environment
// building the fixture fails outright (`git worktree add` could not create
// <fixture>/.git/index.lock) and a test that read an index would have read the
// wrong one. The gate reported all three, which is the same lesson twice: the
// inherited environment is invisible until something real runs under it.
//
// The only pinning these tests are about is the pinning they set themselves,
// afterwards.
func isolateFromTheAmbientRepository(t *testing.T) {
	t.Helper()

	for _, name := range ambientGitPinning {
		inherited, ok := os.LookupEnv(name)
		if !ok {
			continue
		}
		// Setenv first: it registers the restore that Unsetenv would not.
		t.Setenv(name, inherited)
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
}

// gitRun runs a real git command for test setup and fails the test if git
// does. Setup happens before the test sets GIT_DIR, so these calls discover
// the repository the same way a developer's shell does.
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()

	// Identity and signing are supplied per command rather than written into
	// config: the fixture must not depend on the developer's global git
	// settings, and a globally enabled commit.gpgsign would otherwise make
	// these commits fail on the machine of whoever runs the suite.
	full := append([]string{
		"-c", "user.name=dharness test",
		"-c", "user.email=test@dharness.invalid",
		"-c", "commit.gpgsign=false",
	}, args...)

	command := exec.Command("git", full...)
	command.Dir = dir
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// repositoryWithWorktree builds the layout the gate failed in: a main
// checkout whose JS project is a subdirectory, and a linked worktree of it.
// It returns both roots and the worktree's git directory, which is the path
// git exports as GIT_DIR when it runs that worktree's hooks.
func repositoryWithWorktree(t *testing.T) (main, worktree, gitDir string) {
	t.Helper()
	isolateFromTheAmbientRepository(t)

	base := t.TempDir()
	main = filepath.Join(base, "main")
	if err := os.MkdirAll(filepath.Join(main, "frontend"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(main, "frontend", "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	write(t, filepath.Join(main, "frontend", "package-lock.json"), "{}")

	gitRun(t, main, "init", "--quiet", "--initial-branch=main", ".")
	gitRun(t, main, "add", "-A")
	gitRun(t, main, "commit", "--quiet", "-m", "init")

	worktree = filepath.Join(base, "wt")
	gitRun(t, main, "worktree", "add", "--quiet", worktree, "-b", "wt")

	return main, worktree, filepath.Join(main, ".git", "worktrees", "wt")
}

// TestDiscoverIgnoresAnInheritedGitDirInAWorktree reproduces the 1.7.5 gate
// failure, reported on 2026-09-12 against a worktree of autoreas-bridge.
//
// An explicit GIT_DIR turns discovery off: git stops walking up from the
// directory it was given and takes the working tree to be the current
// directory instead. `rev-parse --show-toplevel` then answers with the
// directory it ran in — here the source subdirectory, because lefthook's
// `root: frontend/` puts the command there — while `ls-files` keeps reporting
// index paths from the real worktree root. Joining one onto the other named
// <worktree>/frontend/frontend, a directory that does not exist, and the gate
// died before reading a single staged file.
//
// The four rows that isolated it: the gate passed from the worktree root, it
// passed from the source directory, it passed under `lefthook run pre-commit`
// — and it failed only under a real `git commit`, the one invocation that
// exports GIT_DIR.
func TestDiscoverIgnoresAnInheritedGitDirInAWorktree(t *testing.T) {
	main, worktree, gitDir := repositoryWithWorktree(t)
	t.Setenv("GIT_DIR", gitDir)

	p, err := Discover(filepath.Join(worktree, "frontend"))
	if err != nil {
		t.Fatalf("Discover() = %v", err)
	}

	if !sameDirectory(p.Root, worktree) {
		t.Errorf("Root = %q, want the worktree root %q", p.Root, worktree)
	}
	if want := filepath.Join(worktree, "frontend"); !sameDirectory(p.Source, want) {
		t.Errorf("Source = %q, want %q", p.Source, want)
	}
	// The main checkout is where GIT_DIR points. Nothing dharness resolves
	// belongs to it.
	if contains(main, p.Root) {
		t.Errorf("Root = %q, inside the main checkout %q rather than the worktree", p.Root, main)
	}
}

// TestTheGateReadsTheIndexUnderAnInheritedGitDir carries the reproduction
// through to the two answers the gate acts on. Discovery being right is not
// the deliverable; the staged file list and the staged diff are.
//
// StagedDiff is the call that actually died in the field. It runs git from
// Source, and Source was the doubled path, so the subprocess could not even
// change directory. StagedSourceFiles runs from Root and survived the bug by
// arithmetic — one wrong level in the root cancelled against one wrong level
// in the prefix it strips — which is worth a test of its own: the two halves
// have to be right, not merely consistent with each other.
func TestTheGateReadsTheIndexUnderAnInheritedGitDir(t *testing.T) {
	_, worktree, gitDir := repositoryWithWorktree(t)
	write(t, filepath.Join(worktree, "frontend", "changed.ts"), "export const a = 1\n")
	gitRun(t, worktree, "add", "frontend/changed.ts")

	t.Setenv("GIT_DIR", gitDir)

	p, err := Discover(filepath.Join(worktree, "frontend"))
	if err != nil {
		t.Fatalf("Discover() = %v", err)
	}

	staged, err := p.StagedSourceFiles()
	if err != nil {
		t.Fatalf("StagedSourceFiles() = %v", err)
	}
	if want := []string{"frontend/changed.ts"}; !slices.Equal(staged, want) {
		t.Errorf("StagedSourceFiles() = %q, want %q", staged, want)
	}

	diff, err := p.StagedDiff()
	if err != nil {
		t.Fatalf("StagedDiff() = %v", err)
	}
	// --relative, and running from Source, is what fallow reads the paths as.
	if !strings.Contains(string(diff), "+++ b/changed.ts") {
		t.Errorf("StagedDiff() = %q, want the file named relative to the source directory", diff)
	}
}

// TestStagedSourceFilesHonoursGitIndexFile pins the variable that is kept
// rather than dropped.
//
// `git commit --only <paths>` builds a temporary index and points
// GIT_INDEX_FILE at it — measured on 2026-09-12, at
// .git/worktrees/wt/next-index-25888.lock. That temporary index is the commit
// under way; the repository's own index still holds everything else the
// developer staged earlier. A gate that scrubbed GIT_INDEX_FILE along with
// GIT_DIR would read the wrong one and report on files this commit does not
// contain.
func TestStagedSourceFilesHonoursGitIndexFile(t *testing.T) {
	_, worktree, gitDir := repositoryWithWorktree(t)
	write(t, filepath.Join(worktree, "frontend", "staged-earlier.ts"), "export const a = 1\n")
	write(t, filepath.Join(worktree, "frontend", "committing-now.ts"), "export const b = 2\n")
	gitRun(t, worktree, "add", "frontend/staged-earlier.ts")

	partial := filepath.Join(t.TempDir(), "next-index.lock")
	t.Setenv("GIT_DIR", gitDir)
	t.Setenv("GIT_INDEX_FILE", partial)
	gitRun(t, worktree, "read-tree", "HEAD")
	gitRun(t, worktree, "add", "frontend/committing-now.ts")

	p, err := Discover(filepath.Join(worktree, "frontend"))
	if err != nil {
		t.Fatalf("Discover() = %v", err)
	}
	staged, err := p.StagedSourceFiles()
	if err != nil {
		t.Fatalf("StagedSourceFiles() = %v", err)
	}
	if want := []string{"frontend/committing-now.ts"}; !slices.Equal(staged, want) {
		t.Errorf("StagedSourceFiles() = %q, want %q — the index named by GIT_INDEX_FILE", staged, want)
	}
}

// TestNotAGitRepositoryErrorNamesWhatFailed is the second half of the 1.7.5
// report: the message sent the diagnosis the wrong way.
//
// "run it from inside one" is advice for a caller standing outside a
// repository, and it was printed to one who had been inside a worktree the
// whole time — the directory it named was a path discovery had constructed and
// nothing had created. git and the operating system had already written the
// sentence that distinguishes those two failures. Only printing it was
// missing, and without it the run had to be reproduced by hand.
func TestNotAGitRepositoryErrorNamesWhatFailed(t *testing.T) {
	// A temp directory is outside every repository, unless an inherited
	// GIT_DIR says otherwise — and under the gate one does.
	isolateFromTheAmbientRepository(t)

	t.Run("not a repository", func(t *testing.T) {
		_, err := HooksDir(t.TempDir())
		if err == nil {
			t.Fatal("HooksDir() = nil, want an error outside a repository")
		}
		if !strings.Contains(err.Error(), "not a git repository") {
			t.Errorf("HooksDir() = %v, want git's own words about the repository", err)
		}
	})

	t.Run("directory that does not exist", func(t *testing.T) {
		// The shape the gate died in: a constructed source path, one level
		// deeper than anything on disk.
		missing := filepath.Join(t.TempDir(), "frontend", "frontend")

		_, err := StagedSourceFiles(missing)
		if err == nil {
			t.Fatal("StagedSourceFiles() = nil, want an error for a directory that does not exist")
		}
		if !strings.Contains(err.Error(), missing) {
			t.Errorf("StagedSourceFiles() = %v, want the path it tried", err)
		}
		// git never ran, so the sentence comes from the operating system
		// refusing to change directory. It is the one that names the real
		// problem, and advice about standing inside a repository is not.
		if !strings.Contains(strings.ToLower(err.Error()), "cannot find the path") &&
			!strings.Contains(strings.ToLower(err.Error()), "no such file or directory") {
			t.Errorf("StagedSourceFiles() = %v, want the message to say the directory is missing", err)
		}
	})

	// Outside a repository `git diff --cached` falls back to --no-index mode,
	// rejects the flag and prints its whole usage. Reporting the rejection is
	// the point; reporting forty lines of manual page underneath it buries the
	// rejection and makes a gate failure unreadable in a hook's output.
	t.Run("without the usage git appends", func(t *testing.T) {
		_, err := StagedSourceFiles(t.TempDir())
		if err == nil {
			t.Fatal("StagedSourceFiles() = nil, want an error outside a repository")
		}
		if !strings.Contains(err.Error(), "unknown option") {
			t.Errorf("StagedSourceFiles() = %v, want git's rejection of the flag", err)
		}
		if strings.Contains(err.Error(), "usage:") {
			t.Errorf("StagedSourceFiles() = %v, want the usage block left out", err)
		}
	})
}
