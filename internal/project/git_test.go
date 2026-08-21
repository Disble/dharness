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

// StagedDiff has to ask git from Source with --relative, because the tool that
// reads the diff reads it from there. Measured against fallow 3.16.0: a diff
// whose paths do not match is neither an error nor empty — the filter is
// ignored and the scope silently widens to whole files, so a repository-rooted
// path here would degrade the gate without failing it.
func TestStagedDiffAsksFromSourceInSourceRelativePaths(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "frontend")

	var askedIn string
	var askedFor []string
	t.Cleanup(SetGitOutputForTest(func(dir string, args ...string) ([]byte, error) {
		askedIn, askedFor = dir, args
		return []byte("diff --git a/src/a.ts b/src/a.ts\n"), nil
	}))

	diff, err := Project{Root: root, Source: source, InRepository: true}.StagedDiff()
	if err != nil {
		t.Fatalf("StagedDiff() = %v", err)
	}
	if string(diff) != "diff --git a/src/a.ts b/src/a.ts\n" {
		t.Errorf("StagedDiff() = %q, want git's own bytes unchanged", diff)
	}
	if askedIn != source {
		t.Errorf("asked git in %q, want the source directory %q", askedIn, source)
	}
	for _, want := range []string{"--cached", "--relative", "-U0", "--no-ext-diff", "--no-renames", "--diff-filter=ACMR"} {
		if !slices.Contains(askedFor, want) {
			t.Errorf("StagedDiff() asked git for %v, missing %q", askedFor, want)
		}
	}
	if len(askedFor) < 2 || askedFor[0] != "-c" || askedFor[1] != "core.quotePath=false" {
		t.Errorf("StagedDiff() asked git for %v, want core.quotePath=false ahead of the subcommand", askedFor)
	}
}

// The same reasoning as StagedSourceFiles: a git failure that reads as an
// empty diff would hand fallow a scope admitting nothing, and audit exits 0 on
// that — a green gate over an unexamined change.
func TestStagedDiffPropagatesTheGitFailure(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		return nil, errors.New("git failed")
	}))

	_, err := Project{Root: root, Source: root, InRepository: true}.StagedDiff()
	var notAGitRepo *NotAGitRepositoryError
	if !errors.As(err, &notAGitRepo) {
		t.Fatalf("StagedDiff() = %v, want NotAGitRepositoryError", err)
	}
}

// TestHooksDirAsksGitRatherThanAssumingDotGitHooks pins the one reason this
// function exists: `.git/hooks` is where git looks only when the repository
// has not said otherwise, and a repository that sets core.hooksPath — this
// one does — runs its hooks from somewhere else entirely. A check hardcoded
// to `.git/hooks` there is not merely imprecise; it is always false, which
// makes a wired gate look unwired on every run.
func TestHooksDirAsksGitRatherThanAssumingDotGitHooks(t *testing.T) {
	var asked []string
	t.Cleanup(SetGitOutputForTest(func(_ string, args ...string) ([]byte, error) {
		asked = args
		return []byte("D:/repo/.githooks\n"), nil
	}))

	dir, err := HooksDir("D:/repo")
	if err != nil {
		t.Fatalf("HooksDir() = %v", err)
	}
	if dir != "D:/repo/.githooks" {
		t.Errorf("HooksDir() = %q, want the path git named", dir)
	}
	if !slices.Contains(asked, "--git-path") || !slices.Contains(asked, "hooks") {
		t.Errorf("HooksDir() asked git %q; only --git-path hooks resolves core.hooksPath", asked)
	}
}

// TestHooksDirFailsOutsideARepository keeps the error typed: a caller that
// cannot locate the hooks directory has not found an empty one.
func TestHooksDirFailsOutsideARepository(t *testing.T) {
	t.Cleanup(SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		return nil, errors.New("not a git repository")
	}))

	dir, err := HooksDir(t.TempDir())
	if err == nil {
		t.Fatalf("HooksDir() = %q, nil; want an error outside a repository", dir)
	}
	var notARepo *NotAGitRepositoryError
	if !errors.As(err, &notARepo) {
		t.Errorf("HooksDir() = %v, want a *NotAGitRepositoryError", err)
	}
}
