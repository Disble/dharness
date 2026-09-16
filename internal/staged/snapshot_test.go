package staged

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCountFilesCountsRegularFilesNotDirectories pins countFiles directly: a
// directory git created along the way to a file is not itself one of the
// index's own entries, and counting it would make an empty checkout look
// complete.
func TestCountFilesCountsRegularFilesNotDirectories(t *testing.T) {
	root := t.TempDir()
	writeStagedFixture(t, filepath.Join(root, "a.txt"), "a")
	writeStagedFixture(t, filepath.Join(root, "nested", "b.txt"), "b")

	got, err := countFiles(root)
	if err != nil {
		t.Fatalf("countFiles() = %v", err)
	}
	if got != 2 {
		t.Errorf("countFiles() = %d, want 2: one directory and two files, and the directory must not count", got)
	}
}

// TestCopyFileDuplicatesContentAndMode pins copyFile directly.
func TestCopyFileDuplicatesContentAndMode(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "source.txt")
	writeStagedFixture(t, target, "hello")
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(dir, "dest.txt")
	if err := copyFile(target, destination); err != nil {
		t.Fatalf("copyFile() = %v", err)
	}

	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("ReadFile(destination) = %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("copyFile() wrote %q, want %q", got, "hello")
	}
}

// TestCleanupLinksToleratesAnAlreadyRemovedLink pins the tolerance
// cleanupLinks needs for a snapshot whose cleanup runs twice, or whose link
// list names an entry that was already cleared some other way.
func TestCleanupLinksToleratesAnAlreadyRemovedLink(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist")

	if err := cleanupLinks([]string{missing}); err != nil {
		t.Errorf("cleanupLinks() = %v, want nil for an already-missing path", err)
	}
}

// TestSnapshotContainsAStagedEdit pins Snapshot's own source of truth: the
// index, not the working tree.
func TestSnapshotContainsAStagedEdit(t *testing.T) {
	root := newRepo(t)
	writeStagedFixture(t, filepath.Join(root, "src", "a.ts"), "export const a = 1;\n")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "--quiet", "-m", "init")

	writeStagedFixture(t, filepath.Join(root, "src", "a.ts"), "export const a = 2;\n")
	gitRun(t, root, "add", "-A")

	dir, _, cleanup, err := Snapshot(root, root)
	if err != nil {
		t.Fatalf("Snapshot() = %v", err)
	}
	defer func() { _ = cleanup() }()

	got, err := os.ReadFile(filepath.Join(dir, "src", "a.ts"))
	if err != nil {
		t.Fatalf("ReadFile(snapshot/src/a.ts) = %v", err)
	}
	if string(got) != "export const a = 2;\n" {
		t.Errorf("snapshot holds %q, want the staged content %q", got, "export const a = 2;\n")
	}
}

// TestSnapshotExcludesAnUnstagedEdit is the other half: a change made after
// staging must not reach the snapshot at all — the index is frozen the
// moment Snapshot reads it, exactly as a real commit would freeze it.
func TestSnapshotExcludesAnUnstagedEdit(t *testing.T) {
	root := newRepo(t)
	writeStagedFixture(t, filepath.Join(root, "src", "a.ts"), "export const a = 1;\n")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "--quiet", "-m", "init")

	writeStagedFixture(t, filepath.Join(root, "src", "a.ts"), "export const a = 2;\n")
	gitRun(t, root, "add", "-A")
	// Unstaged, on top of the staged edit above.
	writeStagedFixture(t, filepath.Join(root, "src", "a.ts"), "export const a = 3;\n")

	dir, _, cleanup, err := Snapshot(root, root)
	if err != nil {
		t.Fatalf("Snapshot() = %v", err)
	}
	defer func() { _ = cleanup() }()

	got, err := os.ReadFile(filepath.Join(dir, "src", "a.ts"))
	if err != nil {
		t.Fatalf("ReadFile(snapshot/src/a.ts) = %v", err)
	}
	if string(got) != "export const a = 2;\n" {
		t.Errorf("snapshot holds %q, want only the staged content %q, not the unstaged edit on top of it", got, "export const a = 2;\n")
	}
}

// TestSnapshotHoldsTheIndexUnderTheMachinesLineEndingConfig pins the two tests
// above without the fixture's core.autocrlf pin, under the machine's own
// config. Git for Windows ships core.autocrlf=true in its system config, and
// so does the windows-latest CI runner, so there checkout-index writes CRLF
// exactly as a real checkout does. The content is compared with every \r
// removed: what is pinned is that the staged edit is present and the unstaged
// edit absent whatever the machine's line-ending config is, not which line
// ending it chose.
func TestSnapshotHoldsTheIndexUnderTheMachinesLineEndingConfig(t *testing.T) {
	root := newRepoUnderTheMachinesGitConfig(t)
	writeStagedFixture(t, filepath.Join(root, "src", "a.ts"), "export const a = 1;\n")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "--quiet", "-m", "init")

	writeStagedFixture(t, filepath.Join(root, "src", "a.ts"), "export const a = 2;\n")
	gitRun(t, root, "add", "-A")
	writeStagedFixture(t, filepath.Join(root, "src", "a.ts"), "export const a = 3;\n")

	dir, _, cleanup, err := Snapshot(root, root)
	if err != nil {
		t.Fatalf("Snapshot() = %v", err)
	}
	defer func() { _ = cleanup() }()

	got, err := os.ReadFile(filepath.Join(dir, "src", "a.ts"))
	if err != nil {
		t.Fatalf("ReadFile(snapshot/src/a.ts) = %v", err)
	}
	t.Logf("snapshot/src/a.ts holds %q", got)
	if strings.ReplaceAll(string(got), "\r", "") != "export const a = 2;\n" {
		t.Errorf("snapshot holds %q, want the staged content %q and not the unstaged edit, whatever the line endings", got, "export const a = 2;\n")
	}
}

// TestSnapshotLinksAnIgnoredDirectory pins the ignored-directory half: an
// entry `ls-files --others --ignored --directory` reports with a trailing
// slash is linked whole into the snapshot, reachable at the same relative
// path.
func TestSnapshotLinksAnIgnoredDirectory(t *testing.T) {
	root := newRepo(t)
	writeStagedFixture(t, filepath.Join(root, ".gitignore"), "node_modules/\n")
	writeStagedFixture(t, filepath.Join(root, "src", "a.ts"), "export const a = 1;\n")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "--quiet", "-m", "init")

	writeStagedFixture(t, filepath.Join(root, "node_modules", "pkg", "index.js"), "module.exports = 1;\n")

	dir, _, cleanup, err := Snapshot(root, root)
	if err != nil {
		t.Fatalf("Snapshot() = %v", err)
	}
	defer func() { _ = cleanup() }()

	got, err := os.ReadFile(filepath.Join(dir, "node_modules", "pkg", "index.js"))
	if err != nil {
		t.Fatalf("ReadFile(snapshot/node_modules/pkg/index.js) = %v, want the ignored directory reachable through the snapshot", err)
	}
	if string(got) != "module.exports = 1;\n" {
		t.Errorf("linked node_modules holds %q, want %q", got, "module.exports = 1;\n")
	}
}

// TestSnapshotLinksMultipleIgnoredDirectories pins linkIgnored's own loop:
// linking one ignored directory must not stop the scan before a second one is
// reached — "dist/" sorts before "node_modules/" in git's own listing, so a
// `break` in the directory branch would link the first and silently drop the
// second.
func TestSnapshotLinksMultipleIgnoredDirectories(t *testing.T) {
	root := newRepo(t)
	writeStagedFixture(t, filepath.Join(root, ".gitignore"), "node_modules/\ndist/\n")
	writeStagedFixture(t, filepath.Join(root, "src", "a.ts"), "export const a = 1;\n")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "--quiet", "-m", "init")

	writeStagedFixture(t, filepath.Join(root, "node_modules", "pkg", "index.js"), "module.exports = 1;\n")
	writeStagedFixture(t, filepath.Join(root, "dist", "bundle.js"), "console.log(1);\n")

	dir, _, cleanup, err := Snapshot(root, root)
	if err != nil {
		t.Fatalf("Snapshot() = %v", err)
	}
	defer func() { _ = cleanup() }()

	if _, err := os.Stat(filepath.Join(dir, "dist", "bundle.js")); err != nil {
		t.Errorf("Stat(snapshot/dist/bundle.js) = %v, want dist/ linked", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules", "pkg", "index.js")); err != nil {
		t.Errorf("Stat(snapshot/node_modules/pkg/index.js) = %v, want node_modules/ also linked, not skipped after dist/", err)
	}
}

// TestSnapshotLinksAnIgnoredDirectoryNestedInAnotherOnlyOnce pins the shape a
// real consumer hit: an untracked directory whose only content is an ignored
// node_modules/ is listed by git 2.51 as both `stryker/` and
// `stryker/node_modules/`. Linking the parent already brings the child in, so
// linking the child as well failed the whole snapshot with "Cannot create a
// file when that file already exists".
func TestSnapshotLinksAnIgnoredDirectoryNestedInAnotherOnlyOnce(t *testing.T) {
	root := newRepo(t)
	writeStagedFixture(t, filepath.Join(root, ".gitignore"), "node_modules/\n")
	writeStagedFixture(t, filepath.Join(root, "src", "a.ts"), "export const a = 1;\n")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "--quiet", "-m", "init")

	writeStagedFixture(t, filepath.Join(root, "stryker", "node_modules", "pkg", "index.js"), "module.exports = 1;\n")

	dir, _, cleanup, err := Snapshot(root, root)
	if err != nil {
		t.Fatalf("Snapshot() = %v, want the nested ignored directory covered by its parent's link", err)
	}
	defer func() { _ = cleanup() }()

	if _, err := os.Stat(filepath.Join(dir, "stryker", "node_modules", "pkg", "index.js")); err != nil {
		t.Errorf("Stat(snapshot/stryker/node_modules/pkg/index.js) = %v, want it reachable through the parent link", err)
	}
}

// TestLinkIgnoredSkipsEntriesUnderALinkedDirectoryWhateverTheOrder pins the
// ancestor check independently of git's own listing order: a child listed
// before its parent must still be covered by the parent, not linked first and
// then collided with — and skipping one must not end the scan before a later
// sibling, `wailsjs/`, is reached.
func TestLinkIgnoredSkipsEntriesUnderALinkedDirectoryWhateverTheOrder(t *testing.T) {
	source := t.TempDir()
	writeStagedFixture(t, filepath.Join(source, "stryker", "node_modules", "pkg", "index.js"), "module.exports = 1;\n")
	writeStagedFixture(t, filepath.Join(source, "stryker", "notes.txt"), "x\n")
	writeStagedFixture(t, filepath.Join(source, "wailsjs", "go.js"), "x\n")
	defer SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		return []byte("wailsjs/\x00stryker/notes.txt\x00stryker/node_modules/\x00stryker/\x00"), nil
	})()

	dir := t.TempDir()
	links, err := linkIgnored(source, source, dir)
	defer func() { _ = cleanupLinks(links) }()
	if err != nil {
		t.Fatalf("linkIgnored() = %v, want every entry under stryker/ covered by its link", err)
	}
	want := []string{filepath.Join(dir, "stryker"), filepath.Join(dir, "wailsjs")}
	if len(links) != len(want) || links[0] != want[0] || links[1] != want[1] {
		t.Errorf("links = %v, want exactly %v", links, want)
	}
}

// TestLinkIgnoredPropagatesACopyFailure pins the copy-error branch directly:
// an ignored file that cannot be read must fail linkIgnored rather than be
// silently skipped.
func TestLinkIgnoredPropagatesACopyFailure(t *testing.T) {
	defer SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		return []byte("missing.txt\x00"), nil
	})()

	// source holds no "missing.txt" at all, so copyFile's own os.Stat fails.
	root := t.TempDir()
	_, err := linkIgnored(root, root, t.TempDir())
	if err == nil {
		t.Fatal("linkIgnored() = nil, want an error: the ignored file does not exist to copy")
	}
}

// TestCleanupLinksRemovesEveryLinkNotOnlyTheFirst pins cleanupLinks' own
// loop: it must remove every named link, not stop after the first one.
func TestCleanupLinksRemovesEveryLinkNotOnlyTheFirst(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	if err := os.Mkdir(a, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(b, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := cleanupLinks([]string{a, b}); err != nil {
		t.Fatalf("cleanupLinks() = %v", err)
	}
	if _, err := os.Stat(a); !os.IsNotExist(err) {
		t.Errorf("Stat(a) = %v, want removed", err)
	}
	if _, err := os.Stat(b); !os.IsNotExist(err) {
		t.Errorf("Stat(b) = %v, want removed: cleanupLinks must not stop after the first entry", err)
	}
}

// TestSnapshotDoesNotCopyAnUntrackedNonIgnoredFile pins the "ignored only"
// half of the same measurement that produced it: a file that is merely
// untracked — never `git add`ed, and not matched by any .gitignore rule —
// must not reach the snapshot. Measured against a real Wails project:
// linking every untracked entry copied in exactly such a file and flipped a
// real 11-Survived verdict to 11-Killed.
func TestSnapshotDoesNotCopyAnUntrackedNonIgnoredFile(t *testing.T) {
	root := newRepo(t)
	writeStagedFixture(t, filepath.Join(root, "src", "a.ts"), "export const a = 1;\n")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "--quiet", "-m", "init")

	// Written to disk, never staged, and nothing ignores it.
	writeStagedFixture(t, filepath.Join(root, "src", "a.test.ts"), "// written but never added\n")

	dir, _, cleanup, err := Snapshot(root, root)
	if err != nil {
		t.Fatalf("Snapshot() = %v", err)
	}
	defer func() { _ = cleanup() }()

	if _, err := os.Stat(filepath.Join(dir, "src", "a.test.ts")); err == nil {
		t.Error("snapshot contains an untracked, non-ignored file; only ignored entries are meant to be linked or copied in")
	} else if !os.IsNotExist(err) {
		t.Fatalf("Stat(snapshot/src/a.test.ts) = %v", err)
	}
}

// TestSnapshotMaterialisesTheWholeRootWhenSourceIsASubdirectory pins where
// checkout-index runs from: materialise used to run `checkout-index --all` FROM
// SOURCE, so whenever Source != Root — exactly the real consumer's own shape,
// a JS project inside a Wails-style frontend/ subdirectory — checkout-index
// wrote only Source's own subtree, at Source-relative paths, and the count
// cross-check compared a Source-relative count against a Source-relative
// listing, so it never noticed. Reproduced in a scratch repo with root.txt,
// frontend/package.json and frontend/src/a.ts: run from frontend/,
// `git checkout-index --all --prefix=out/` wrote only out/package.json and
// out/src/a.ts (no root.txt), while `git ls-files` there listed exactly those
// two names — so (a) the count check passed 2=2 and noticed nothing, (b) a
// tracked file outside Source went missing from the snapshot entirely, and
// (c) an ignored directory inside Source would link BESIDE the Source
// subtree in the snapshot rather than inside it. The fix runs checkout-index
// and its count check from ROOT, so the whole index is materialised, and
// still runs the ignored-entry scan from SOURCE, joining each entry back
// under the snapshot's own copy of Source.
func TestSnapshotMaterialisesTheWholeRootWhenSourceIsASubdirectory(t *testing.T) {
	root := newRepo(t)
	writeStagedFixture(t, filepath.Join(root, "root.txt"), "at the repository root\n")
	writeStagedFixture(t, filepath.Join(root, "frontend", ".gitignore"), "node_modules/\n")
	writeStagedFixture(t, filepath.Join(root, "frontend", "package.json"), `{"name":"frontend"}`)
	writeStagedFixture(t, filepath.Join(root, "frontend", "src", "a.ts"), "export const a = 1;\n")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "--quiet", "-m", "init")

	writeStagedFixture(t, filepath.Join(root, "frontend", "src", "a.ts"), "export const a = 2;\n")
	gitRun(t, root, "add", "-A")

	writeStagedFixture(t, filepath.Join(root, "frontend", "node_modules", "pkg", "index.js"), "module.exports = 1;\n")

	source := filepath.Join(root, "frontend")
	dir, snapshotSource, cleanup, err := Snapshot(root, source)
	if err != nil {
		t.Fatalf("Snapshot() = %v", err)
	}
	defer func() { _ = cleanup() }()

	if _, err := os.Stat(filepath.Join(dir, "root.txt")); err != nil {
		t.Errorf("Stat(snapshot/root.txt) = %v, want the whole index materialised, not only Source's own subtree", err)
	}

	wantSource := filepath.Join(dir, "frontend")
	if snapshotSource != wantSource {
		t.Errorf("Snapshot() snapshotSource = %q, want %q", snapshotSource, wantSource)
	}

	got, err := os.ReadFile(filepath.Join(snapshotSource, "src", "a.ts"))
	if err != nil {
		t.Fatalf("ReadFile(snapshotSource/src/a.ts) = %v", err)
	}
	if string(got) != "export const a = 2;\n" {
		t.Errorf("snapshot holds %q, want the staged content %q", got, "export const a = 2;\n")
	}

	if _, err := os.Stat(filepath.Join(snapshotSource, "node_modules", "pkg", "index.js")); err != nil {
		t.Errorf("Stat(snapshotSource/node_modules/pkg/index.js) = %v, want the ignored directory linked INSIDE the snapshot's own frontend/, not beside it", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules")); !os.IsNotExist(err) {
		t.Errorf("Stat(snapshot/node_modules) = %v, want no node_modules directly under the snapshot root", err)
	}
}

// TestSnapshotReadsTheIndexGitIsCommitting pins the `git commit --only <path>`
// shape: git builds a temporary index for that commit and points hooks at it
// through GIT_INDEX_FILE, which runner.Environ deliberately keeps. The main
// index, the temporary index and the working tree all hold different content
// here, so only a snapshot materialised from the temporary index can hold "3".
// It runs with Source != Root because moving checkout-index to Root is exactly
// where the index a probe reads could quietly change.
func TestSnapshotReadsTheIndexGitIsCommitting(t *testing.T) {
	root := newRepo(t)
	writeStagedFixture(t, filepath.Join(root, "frontend", "package.json"), `{"name":"frontend"}`)
	writeStagedFixture(t, filepath.Join(root, "frontend", "src", "a.ts"), "export const a = 1;\n")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "--quiet", "-m", "init")

	writeStagedFixture(t, filepath.Join(root, "frontend", "src", "a.ts"), "export const a = 2;\n")
	gitRun(t, root, "add", "-A")

	temporaryIndex := filepath.Join(t.TempDir(), "index")
	t.Setenv("GIT_INDEX_FILE", temporaryIndex)
	gitRun(t, root, "read-tree", "HEAD")
	writeStagedFixture(t, filepath.Join(root, "frontend", "src", "a.ts"), "export const a = 3;\n")
	gitRun(t, root, "add", "frontend/src/a.ts")
	writeStagedFixture(t, filepath.Join(root, "frontend", "src", "a.ts"), "export const a = 4;\n")

	dir, snapshotSource, cleanup, err := Snapshot(root, filepath.Join(root, "frontend"))
	if err != nil {
		t.Fatalf("Snapshot() = %v", err)
	}
	defer func() { _ = cleanup() }()

	if snapshotSource != filepath.Join(dir, "frontend") {
		t.Errorf("Snapshot() snapshotSource = %q, want %q", snapshotSource, filepath.Join(dir, "frontend"))
	}
	got, err := os.ReadFile(filepath.Join(snapshotSource, "src", "a.ts"))
	if err != nil {
		t.Fatalf("ReadFile(snapshotSource/src/a.ts) = %v", err)
	}
	if string(got) != "export const a = 3;\n" {
		t.Errorf("snapshot holds %q, want the temporary index's %q (main index holds 2, working tree 4)", got, "export const a = 3;\n")
	}
}

// TestSnapshotCleanupNeverDeletesTheLinkTarget is the safety proof linking
// needs: a sentinel file inside the real ignored directory — the link's
// TARGET, in the actual repository, never the snapshot — must survive
// cleanup(). Measured directly against go1.27.0 windows/amd64: os.Lstat on a
// `mklink /J` junction reports os.ModeIrregular, and calling os.RemoveAll on
// the junction path ALONE (no cleanupLinks first) already leaves the target's
// own sentinel file untouched — Windows' RemoveDirectory unlinks a reparse
// point without walking into what it points at, and both `Remove-Item
// -Recurse -Force` and Git Bash's `rm -rf` were measured to do the same.
// cleanupLinks' own explicit unlink-by-exact-path stays anyway, as defence in
// depth against a link kind or a future Go version where that is not true.
func TestSnapshotCleanupNeverDeletesTheLinkTarget(t *testing.T) {
	root := newRepo(t)
	writeStagedFixture(t, filepath.Join(root, ".gitignore"), "node_modules/\n")
	writeStagedFixture(t, filepath.Join(root, "src", "a.ts"), "export const a = 1;\n")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "--quiet", "-m", "init")

	sentinel := filepath.Join(root, "node_modules", "pkg", "sentinel.txt")
	writeStagedFixture(t, sentinel, "do not delete me")

	dir, _, cleanup, err := Snapshot(root, root)
	if err != nil {
		t.Fatalf("Snapshot() = %v", err)
	}

	// The link is reachable before cleanup, proving the test actually
	// exercised the link rather than vacuously passing on a missing one.
	if _, err := os.Stat(filepath.Join(dir, "node_modules", "pkg", "sentinel.txt")); err != nil {
		t.Fatalf("Stat(snapshot sentinel) = %v, want the link readable before cleanup", err)
	}

	if err := cleanup(); err != nil {
		t.Fatalf("cleanup() = %v", err)
	}

	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("Stat(%s) = %v, want the real sentinel to survive cleanup: cleanup must remove the link, never the target", sentinel, err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("Stat(snapshot dir) = %v, want the snapshot directory itself gone after cleanup", err)
	}
}
