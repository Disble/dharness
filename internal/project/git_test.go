package project

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// stubBarrelIndex simulates git's own pathspec matching for the exact
// patterns PublishesBarrels asks for ("*/index.ts", "*/index.tsx"), rather
// than returning a canned response regardless of what was asked. That is
// what makes TestPublishesBarrelsRequiresADirectoryComponent an actual pin
// on the leading "*/": weakening the pathspec in production code changes
// what this stub matches, not just what a fixed response happens to hold.
func stubBarrelIndex(t *testing.T, present ...string) {
	t.Helper()
	t.Cleanup(SetGitOutputForTest(func(_ string, args ...string) ([]byte, error) {
		var matches []string
		for _, candidate := range present {
			for _, spec := range args {
				if ok, _ := path.Match(spec, candidate); ok {
					matches = append(matches, candidate)
					break
				}
			}
		}
		return []byte(strings.Join(matches, "\x00")), nil
	}))
}

func TestPublishesBarrelsTrueWhenIndexHasABarrel(t *testing.T) {
	root := t.TempDir()
	stubBarrelIndex(t, "components/index.ts")

	p := Project{Root: root, Source: root, InRepository: true}
	if !p.PublishesBarrels() {
		t.Error("PublishesBarrels() = false, want true for an index.ts one directory deep")
	}
}

func TestPublishesBarrelsFalseWithNoMatches(t *testing.T) {
	root := t.TempDir()
	stubBarrelIndex(t)

	p := Project{Root: root, Source: root, InRepository: true}
	if p.PublishesBarrels() {
		t.Error("PublishesBarrels() = true, want false with no barrels in the index")
	}
}

func TestBarrelProbeAnswersOffWhenGitFails(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		return nil, errors.New("git failed")
	}))

	p := Project{Root: root, Source: root, InRepository: true}
	if p.PublishesBarrels() {
		t.Error("PublishesBarrels() = true, want false when git cannot answer")
	}
}

// TestPublishesBarrelsFalseOutsideARepositoryOrWithoutSource pins the two
// early-return guards, and proves they short-circuit before the probe runs
// at all: the stub panics if PublishesBarrels ever calls it in either case,
// rather than merely asserting the answer happens to be false.
func TestPublishesBarrelsFalseOutsideARepositoryOrWithoutSource(t *testing.T) {
	panicProbe := func(string, ...string) ([]byte, error) {
		panic("PublishesBarrels must not ask git when it cannot answer without a repository and a source")
	}

	t.Run("not in a repository", func(t *testing.T) {
		t.Cleanup(SetGitOutputForTest(panicProbe))
		p := Project{Root: t.TempDir(), Source: t.TempDir()}
		if p.PublishesBarrels() {
			t.Error("PublishesBarrels() = true outside a repository")
		}
	})

	t.Run("no source", func(t *testing.T) {
		t.Cleanup(SetGitOutputForTest(panicProbe))
		p := Project{Root: t.TempDir(), InRepository: true}
		if p.PublishesBarrels() {
			t.Error("PublishesBarrels() = true without a source")
		}
	})
}

// TestUnstagedBarrelDoesNotCount pins a deliberate threat-matrix row: the
// probe reads the git index (ls-files), not the working tree, so a barrel
// that exists on disk but was never staged does not count as published.
// This is not a bug to "fix" later — an unstaged file is not yet this
// project's published architecture. The index answering nothing is the
// whole of the scenario; no file needs to exist on disk to prove it.
func TestUnstagedBarrelDoesNotCount(t *testing.T) {
	root := t.TempDir()
	stubBarrelIndex(t)

	p := Project{Root: root, Source: root, InRepository: true}
	if p.PublishesBarrels() {
		t.Error("PublishesBarrels() = true for an unstaged barrel; only the index publishes it")
	}
}

// TestPublishesBarrelsRequiresADirectoryComponent pins the leading "*/" in
// the pathspec: a root-level index.ts is a package entry point, not
// evidence of barrel publishing. Dropping the "*/" would let this stub's
// path.Match succeed on a bare "index.ts" too, flipping this test to true.
func TestPublishesBarrelsRequiresADirectoryComponent(t *testing.T) {
	root := t.TempDir()
	stubBarrelIndex(t, "index.ts")

	p := Project{Root: root, Source: root, InRepository: true}
	if p.PublishesBarrels() {
		t.Error("PublishesBarrels() = true for a root-level index.ts, want false")
	}
}

// The gate hands ESLint explicit paths and runs the command with
// Dir: p.Source, while StagedSourceFiles reports paths relative to Root — a
// Wails-shaped split layout would hand ESLint "frontend/src/a.ts" while it
// runs inside "frontend/", which resolves nothing. This pins the strip.
func TestEslintStagePathsAreRelativeToSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "frontend")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		return []byte("frontend/src/a.ts\x00frontend/src/b.tsx\x00main.go\x00"), nil
	}))

	files, err := At(root, source).StagedSourceFilesFromSource()
	if err != nil {
		t.Fatalf("StagedSourceFilesFromSource() = %v", err)
	}
	want := []string{"src/a.ts", "src/b.tsx"}
	if !slices.Equal(files, want) {
		t.Errorf("StagedSourceFilesFromSource() = %v, want %v", files, want)
	}
}

// A conventional single-directory project has no prefix to strip: Root and
// Source coincide, so StagedSourceFiles' own paths are already what ESLint
// needs, and the stripping must not corrupt them by touching a "" prefix.
func TestEslintStagePathsUnchangedInAConventionalLayout(t *testing.T) {
	root := t.TempDir()

	t.Cleanup(SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		return []byte("src/a.ts\x00"), nil
	}))

	files, err := Describe(root).StagedSourceFilesFromSource()
	if err != nil {
		t.Fatalf("StagedSourceFilesFromSource() = %v", err)
	}
	if len(files) != 1 || files[0] != "src/a.ts" {
		t.Errorf("StagedSourceFilesFromSource() = %v, want [src/a.ts]", files)
	}
}

// StagedSourceFilesFromSource must propagate a git failure rather than
// swallowing it, since a silent empty list would look identical to "nothing
// staged" and skip the ESLint stage without saying why.
func TestEslintStagePathsPropagatesTheGitFailure(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		return nil, errors.New("git failed")
	}))

	_, err := Describe(root).StagedSourceFilesFromSource()
	var notAGitRepo *NotAGitRepositoryError
	if !errors.As(err, &notAGitRepo) {
		t.Fatalf("StagedSourceFilesFromSource() = %v, want NotAGitRepositoryError", err)
	}
}
