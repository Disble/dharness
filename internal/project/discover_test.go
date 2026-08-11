package project

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repo builds a tree and the git probe that answers for it: the toplevel, and
// the lockfiles git tracks, which is what discovery reads.
func repo(t *testing.T, lockfiles ...string) string {
	t.Helper()

	root := t.TempDir()
	for _, lockfile := range lockfiles {
		path := filepath.Join(root, filepath.FromSlash(lockfile))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		write(t, path, "")
	}

	t.Cleanup(SetGitOutputForTest(func(_ string, args ...string) ([]byte, error) {
		switch {
		case len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel":
			// git reports the toplevel with forward slashes, including on
			// Windows, which is the spelling discovery has to survive.
			return []byte(filepath.ToSlash(root) + "\n"), nil
		case len(args) >= 1 && args[0] == "ls-files":
			return []byte(strings.Join(lockfiles, "\x00")), nil
		}
		return nil, errors.New("unexpected git call")
	}))
	return root
}

// The layout that started this: Go at the repository root, the whole frontend
// in a subdirectory, one lockfile. Detection has to read the subdirectory and
// the hook has to stay at the root.
func TestDiscoverSeparatesTheRepositoryFromTheJSProject(t *testing.T) {
	root := repo(t, "frontend/bun.lock")
	write(t, filepath.Join(root, "frontend", "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)

	p, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() = %v", err)
	}

	if !sameDirectory(p.Root, root) {
		t.Errorf("Root = %q, want %q", p.Root, root)
	}
	if want := filepath.Join(root, "frontend"); !sameDirectory(p.Source, want) {
		t.Errorf("Source = %q, want %q", p.Source, want)
	}
	if p.SourceRel() != "frontend" {
		t.Errorf("SourceRel() = %q, want %q", p.SourceRel(), "frontend")
	}
	if p.PackageManager != "bun" {
		t.Errorf("PackageManager = %q, want bun", p.PackageManager)
	}
	if p.TestRunner != "vitest" {
		t.Errorf("TestRunner = %q, want vitest", p.TestRunner)
	}
}

// InRepository is what Decision 6bis's stop reads. It must be true on both
// success branches — one lockfile, and none — and false only on the swallow
// branch, which is what a directory that is not a repository at all reaches.
func TestDiscoverRecordsInRepositoryOnEverySuccessBranch(t *testing.T) {
	t.Run("one JS project", func(t *testing.T) {
		root := repo(t, "pnpm-lock.yaml")
		p, err := Discover(root)
		if err != nil {
			t.Fatalf("Discover() = %v", err)
		}
		if !p.InRepository {
			t.Error("InRepository = false, want true for a repository with a JS project")
		}
	})

	t.Run("no JS project", func(t *testing.T) {
		root := repo(t)
		p, err := Discover(root)
		if err != nil {
			t.Fatalf("Discover() = %v", err)
		}
		if !p.InRepository {
			t.Error("InRepository = false, want true for a repository with no JS project")
		}
	})
}

// Outside a repository at all, Discover still swallows the git error (its own
// commands raise it), but it must not claim InRepository — that is the fact
// Decision 6bis's stop in RunSync depends on.
func TestDiscoverLeavesInRepositoryFalseOutsideAGitRepository(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		return nil, errors.New("not a git repository")
	}))

	p, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() = %v", err)
	}
	if p.InRepository {
		t.Error("InRepository = true, want false outside a git repository")
	}
}

// A conventional repository is the same repository with the two roots on top of
// each other, and it must not grow a `root:` key that points at itself.
func TestDiscoverLeavesOneRootAloneWhenThereIsOnlyOne(t *testing.T) {
	root := repo(t, "pnpm-lock.yaml")

	p, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() = %v", err)
	}
	if !sameDirectory(p.Root, p.Source) {
		t.Errorf("Root %q and Source %q are one directory here", p.Root, p.Source)
	}
	if p.SourceRel() != "" {
		t.Errorf("SourceRel() = %q, want empty for a conventional layout", p.SourceRel())
	}
}

// No lockfile anywhere is a real answer, and it is not npm. This is the Go
// repository that reported as an npm project and was offered npm install.
func TestDiscoverReportsNoJSProjectRatherThanGuessing(t *testing.T) {
	root := repo(t)

	p, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() = %v", err)
	}
	if p.HasSource() {
		t.Errorf("Source = %q, want none", p.Source)
	}
	if p.PackageManager != "" {
		t.Errorf("PackageManager = %q, want none", p.PackageManager)
	}
	if p.LocalBinary("lefthook") != "" {
		t.Error("Resolve() found a local binary in a repository with no JS project")
	}
}

// Two independent JS projects is the one question no derivation answers, so it
// is asked rather than guessed — and the error names both candidates.
func TestDiscoverRefusesToPickBetweenTwoJSProjects(t *testing.T) {
	root := repo(t, "web/bun.lock", "mobile/bun.lock")

	_, err := Discover(filepath.Dir(root))
	var ambiguous *AmbiguousSourceError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("Discover() = %v, want AmbiguousSourceError", err)
	}
	if len(ambiguous.Candidates) != 2 {
		t.Fatalf("named %d candidates, want 2: %v", len(ambiguous.Candidates), ambiguous.Candidates)
	}
	for _, want := range []string{"web", "mobile"} {
		if !strings.Contains(ambiguous.Error(), want) {
			t.Errorf("the error does not name %q:\n%s", want, ambiguous.Error())
		}
	}
}

// Standing inside one of them is an answer, so it settles the question without
// asking it. Running the gate from a package of a monorepo is the ordinary case.
func TestDiscoverTakesTheDirectoryYouAreStandingInAsTheAnswer(t *testing.T) {
	root := repo(t, "web/bun.lock", "mobile/bun.lock")

	p, err := Discover(filepath.Join(root, "web"))
	if err != nil {
		t.Fatalf("Discover() = %v", err)
	}
	if p.SourceRel() != "web" {
		t.Errorf("SourceRel() = %q, want web", p.SourceRel())
	}
}

// Two lockfiles in one directory is a migration, not two projects.
func TestDiscoverCountsOneDirectoryOnceHoweverManyLockfilesItHolds(t *testing.T) {
	root := repo(t, "frontend/package-lock.json", "frontend/bun.lock")

	p, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() = %v", err)
	}
	if p.SourceRel() != "frontend" {
		t.Errorf("SourceRel() = %q, want frontend", p.SourceRel())
	}
	// Detection still prefers the newer manager within that one directory.
	if p.PackageManager != "bun" {
		t.Errorf("PackageManager = %q, want bun", p.PackageManager)
	}
}

// The gate only asks about the half of the repository its tools can read. A
// Wails commit carries Go and TypeScript together, and the Go half must not
// decide whether the frontend gate runs.
func TestStagedSourceFilesScopeThemselvesToTheJSProject(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "frontend")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		return []byte("frontend/src/a.ts\x00tools/gen.ts\x00main.go\x00"), nil
	}))

	files, err := At(root, source).StagedSourceFiles()
	if err != nil {
		t.Fatalf("StagedSourceFiles() = %v", err)
	}
	if len(files) != 1 || files[0] != "frontend/src/a.ts" {
		t.Errorf("StagedSourceFiles() = %v, want only the frontend path", files)
	}
}
