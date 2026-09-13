package staged

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The tests in this file drive a real repository. A stub can answer for
// hunks and numstat text, but rename pairing and partial-staging detection
// are git's own behaviour — internal/project/worktree_test.go established the
// same principle for the inherited-GIT_DIR defect, and it applies again here:
// no stub can honestly claim to reproduce what git's rename detector decides.

// ambientGitPinning are the variables that would make a git command in these
// tests operate on a repository other than the fixture it is holding.
var ambientGitPinning = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_INDEX_FILE",
	"GIT_COMMON_DIR",
	"GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_PREFIX",
}

// isolateFromTheAmbientRepository clears the pinning this process inherited
// — this suite can run inside dharness's own `git commit`, which already
// exports GIT_DIR and GIT_INDEX_FILE naming dharness itself — and restores it
// when the test ends.
func isolateFromTheAmbientRepository(t *testing.T) {
	t.Helper()

	for _, name := range ambientGitPinning {
		inherited, ok := os.LookupEnv(name)
		if !ok {
			continue
		}
		t.Setenv(name, inherited)
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
}

// gitRun runs a real git command for test setup and fails the test if git
// does.
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()

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

// newRepo initialises an isolated repository with one committed file, ready
// for a test to stage its own change on top.
func newRepo(t *testing.T) string {
	t.Helper()
	isolateFromTheAmbientRepository(t)

	root := t.TempDir()
	gitRun(t, root, "init", "--quiet", "--initial-branch=main", ".")
	// Pinned in the repository's own config, which outranks the system and
	// global files. A Windows CI runner's system git sets core.autocrlf=true,
	// so checkout-index smudged LF to CRLF exactly as a real checkout would,
	// and every test comparing the snapshot's bytes failed there while passing
	// on a machine whose global config says otherwise. The product was right;
	// the fixture depended on the machine.
	gitRun(t, root, "config", "core.autocrlf", "false")
	return root
}

func writeStagedFixture(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestScopeReadsTheSourceIndexUnderAnInheritedGitDir pins that this package
// asks git through the same process boundary internal/project does, the one
// that scrubs an inherited GIT_DIR.
//
// git exports GIT_DIR into every hook, and an explicit GIT_DIR switches
// discovery off: git takes the directory it runs in as the top of the working
// tree. Run from a frontend/ source directory, --relative then strips nothing,
// and every range comes back named frontend/src/a.ts — a path Stryker, running
// in that same directory, would never find. The GIT_INDEX_FILE snapshot test
// cannot see this, because an inherited environment carries GIT_INDEX_FILE
// through whether or not it was scrubbed.
func TestScopeReadsTheSourceIndexUnderAnInheritedGitDir(t *testing.T) {
	root := newRepo(t)
	writeStagedFixture(t, filepath.Join(root, "frontend", "package.json"), `{"name":"frontend"}`)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "--quiet", "-m", "init")
	writeStagedFixture(t, filepath.Join(root, "frontend", "src", "a.ts"), "export const a = 1;\n")
	gitRun(t, root, "add", "-A")

	t.Setenv("GIT_DIR", filepath.Join(root, ".git"))

	scopes, err := Scope(filepath.Join(root, "frontend"), nil)
	if err != nil {
		t.Fatalf("Scope() = %v", err)
	}
	if len(scopes) != 1 || scopes[0].Argument() != "src/a.ts:1-1" {
		t.Errorf("Scope() = %v, want exactly src/a.ts:1-1, named from the source directory", scopes)
	}
}

// TestScopeGivesNoRangeForAByteIdenticalRename pins the R100 half of the
// measured rename pairing: a pure rename, no content changed, must produce
// no scope at all for the moved file — there is nothing in it a staged
// change actually justifies mutating.
func TestScopeGivesNoRangeForAByteIdenticalRename(t *testing.T) {
	root := newRepo(t)
	writeStagedFixture(t, filepath.Join(root, "src", "old.ts"), "export const a = 1;\nexport const b = 2;\n")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "--quiet", "-m", "init")

	gitRun(t, root, "mv", filepath.ToSlash(filepath.Join("src", "old.ts")), filepath.ToSlash(filepath.Join("src", "new.ts")))
	gitRun(t, root, "add", "-A")

	scopes, err := Scope(root, nil)
	if err != nil {
		t.Fatalf("Scope() = %v", err)
	}
	if len(scopes) != 0 {
		t.Errorf("Scope() = %v, want none: a byte-identical rename adds nothing to mutate", scopes)
	}
}

// TestScopeGivesOnlyTheChangedLinesForARename pins the R80 half: a rename
// that also changes content must scope only the lines that changed, not the
// whole file re-diffed against nothing — which is exactly what a
// destination-only pathspec used to produce (72 of 146 survivors billed to
// byte-identical moves, when measured on a real change).
func TestScopeGivesOnlyTheChangedLinesForARename(t *testing.T) {
	root := newRepo(t)
	original := "export const a = 1;\nexport const b = 2;\nexport const c = 3;\n"
	writeStagedFixture(t, filepath.Join(root, "src", "old.ts"), original)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "--quiet", "-m", "init")

	gitRun(t, root, "mv", filepath.ToSlash(filepath.Join("src", "old.ts")), filepath.ToSlash(filepath.Join("src", "new.ts")))
	changed := "export const a = 1;\nexport const b = 20;\nexport const c = 3;\n"
	writeStagedFixture(t, filepath.Join(root, "src", "new.ts"), changed)
	gitRun(t, root, "add", "-A")

	scopes, err := Scope(root, nil)
	if err != nil {
		t.Fatalf("Scope() = %v", err)
	}
	if len(scopes) != 1 {
		t.Fatalf("Scope() = %v, want exactly one range", scopes)
	}
	if got := scopes[0].Argument(); got != "src/new.ts:2-2" {
		t.Errorf("Scope()[0].Argument() = %q, want %q — only the changed line, on the new path", got, "src/new.ts:2-2")
	}
}

// TestScopeGivesNoRangeForADeclarationFile pins Scope's declaration-file
// exclusion over a real repository: a staged TypeScript
// declaration file must produce no scope at all. It cannot carry runtime
// code, and project.IsSourceFile does not tell it apart from an ordinary
// source file — filepath.Ext("env.d.ts") is ".ts", identical to a real
// source file's own extension.
func TestScopeGivesNoRangeForADeclarationFile(t *testing.T) {
	root := newRepo(t)
	writeStagedFixture(t, filepath.Join(root, "src", "env.d.ts"), "declare const x: string;\n")
	gitRun(t, root, "add", "-A")

	scopes, err := Scope(root, nil)
	if err != nil {
		t.Fatalf("Scope() = %v", err)
	}
	if len(scopes) != 0 {
		t.Errorf("Scope() = %v, want none: a declaration file cannot contain runtime code", scopes)
	}
}

// TestScopeRefusesAPartiallyStagedFile pins the real-repository half of the
// partial-staging refusal: a file with some hunks staged and others left in
// the working tree must be refused, not silently scoped to only what was
// staged.
func TestScopeRefusesAPartiallyStagedFile(t *testing.T) {
	root := newRepo(t)
	writeStagedFixture(t, filepath.Join(root, "src", "a.ts"),
		"export const a = 1;\nexport const b = 2;\nexport const c = 3;\n")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "--quiet", "-m", "init")

	// Stage a change to line 1, then leave a second, unstaged change to line 3
	// sitting on top of it.
	writeStagedFixture(t, filepath.Join(root, "src", "a.ts"),
		"export const a = 10;\nexport const b = 2;\nexport const c = 3;\n")
	gitRun(t, root, "add", "src/a.ts")
	writeStagedFixture(t, filepath.Join(root, "src", "a.ts"),
		"export const a = 10;\nexport const b = 2;\nexport const c = 30;\n")

	_, err := Scope(root, nil)

	var partial *PartiallyStagedError
	if !errors.As(err, &partial) {
		t.Fatalf("Scope() = %v, want PartiallyStagedError", err)
	}
	if partial.Path != "src/a.ts" {
		t.Errorf("PartiallyStagedError.Path = %q, want %q", partial.Path, "src/a.ts")
	}
}
