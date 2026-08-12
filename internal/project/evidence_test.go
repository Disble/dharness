package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The record is evidence, not progress: it holds what the measurement cost and
// what produced it, so a reader can tell whether it still describes the code.
func TestEvidenceSurvivesAWriteAndRead(t *testing.T) {
	p := Project{Root: t.TempDir()}

	if p.ReadEvidence().ScopedMutation != nil {
		t.Fatal("a fresh repository reported a measurement it never took")
	}

	if err := p.RecordScopedMutation("src/thing.ts", 25); err != nil {
		t.Fatalf("RecordScopedMutation() = %v", err)
	}

	measured := p.ReadEvidence().ScopedMutation
	if measured == nil {
		t.Fatal("the measurement did not survive the write")
	}
	if measured.RelatedTests != 25 || measured.MeasuredPath != "src/thing.ts" {
		t.Errorf("read back %+v", measured)
	}
	if measured.MeasuredAt.IsZero() {
		t.Error("the measurement carries no date, so nobody can tell how stale it is")
	}
}

// An unreadable record means the question is unanswered, which is the normal
// state of a new repository — not an error that should stop a command.
func TestUnreadableEvidenceReadsAsUnanswered(t *testing.T) {
	p := Project{Root: t.TempDir()}
	if err := p.RecordScopedMutation("src/a.ts", 1); err != nil {
		t.Fatal(err)
	}
	write(t, p.evidencePath(), "{ not json")

	if p.ReadEvidence().ScopedMutation != nil {
		t.Error("a corrupt record was read as a real measurement")
	}
}

// The directory declares what of its own contents is machine-local, so the
// project's root .gitignore never has to know that dharness exists.
func TestOwnedDirectoryIgnoresItsOwnTransientFiles(t *testing.T) {
	p := Project{Root: t.TempDir()}

	path, err := p.EnsureDir("stryker-tmp")
	if err != nil {
		t.Fatalf("EnsureDir() = %v", err)
	}
	if filepath.Dir(path) != filepath.Join(p.Root, Dir) {
		t.Errorf("EnsureDir() = %q, want a path inside %s", path, Dir)
	}

	raw, err := os.ReadFile(filepath.Join(p.Root, Dir, ".gitignore"))
	if err != nil {
		t.Fatalf("the directory was created without its ignore rules: %v", err)
	}
	ignore := string(raw)

	// An allow list: anything new is ignored unless it was declared shared.
	if !strings.Contains(ignore, "\n*\n") {
		t.Errorf("the ignore file does not ignore by default:\n%s", ignore)
	}
	for _, shared := range []string{"!.gitignore", "!lefthook.yml", "!fallow.jsonc", "!rules.json", "!evidence.json", "!eslint.config.js"} {
		if !strings.Contains(ignore, shared) {
			t.Errorf("the ignore file would hide %s, which describes the repository", shared)
		}
	}
	if strings.Contains(ignore, "!stryker-incremental.json") {
		t.Error("a machine-local file was declared shared")
	}
}

// TestOwnedEslintConfigIsDeclaredShared pins design decision 2: the owned
// ESLint config must be named in the directory's allow list from the moment
// the directory is created, or it stays gitignored forever in every
// already-adopted repository — EnsureDir writes the ignore file only when
// it is absent.
func TestOwnedEslintConfigIsDeclaredShared(t *testing.T) {
	p := Project{Root: t.TempDir()}
	if _, err := p.EnsureDir(""); err != nil {
		t.Fatalf("EnsureDir() = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(p.Root, Dir, ".gitignore"))
	if err != nil {
		t.Fatalf("the directory was created without its ignore rules: %v", err)
	}
	if !strings.Contains(string(raw), "!eslint.config.js") {
		t.Errorf("a fresh .dharness/ does not declare eslint.config.js shared:\n%s", raw)
	}
}

// Writing the ignore rules must not clobber a file the user edited.
func TestOwnedDirectoryKeepsAnExistingIgnoreFile(t *testing.T) {
	p := Project{Root: t.TempDir()}
	if _, err := p.EnsureDir("first"); err != nil {
		t.Fatal(err)
	}

	ignore := filepath.Join(p.Root, Dir, ".gitignore")
	write(t, ignore, "# edited by hand\n*\n!.gitignore\n")

	if _, err := p.EnsureDir("second"); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(ignore)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "edited by hand") {
		t.Error("EnsureDir overwrote an ignore file that was already there")
	}
}

// Repository-local state belongs in the git common directory: git already
// ignores it, worktrees share it, and it dies with the repository instead of
// leaking into a cache keyed by a path that may no longer exist.
func TestStatePathLandsInsideTheGitCommonDirectory(t *testing.T) {
	t.Cleanup(SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		return []byte(filepath.Join(t.TempDir(), ".git") + "\n"), nil
	}))

	path, err := Project{Root: t.TempDir()}.StatePath("stryker-incremental.json")
	if err != nil {
		t.Fatalf("StatePath() = %v", err)
	}
	if filepath.Base(filepath.Dir(path)) != "dharness" {
		t.Errorf("StatePath() = %q, want it under a dharness directory", path)
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Errorf("StatePath() did not create its directory: %v", err)
	}
}

// A corrupt cache makes the tool fail in a way that reads like a problem with
// the code under test, so it is discarded rather than handed over.
func TestDiscardIfUnreadableRemovesOnlyWhatCannotBeRead(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.json")
	bad := filepath.Join(dir, "bad.json")
	write(t, good, `{"ok":true}`)
	write(t, bad, `{ truncated`)

	readable := func(raw []byte) bool { return json.Valid(raw) }
	DiscardIfUnreadable(good, readable)
	DiscardIfUnreadable(bad, readable)
	DiscardIfUnreadable(filepath.Join(dir, "absent.json"), readable)

	if _, err := os.Stat(good); err != nil {
		t.Error("a readable cache was discarded")
	}
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Error("an unreadable cache was kept")
	}
}
