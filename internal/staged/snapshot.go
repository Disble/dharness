package staged

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Snapshot materialises the staged index into a fresh directory, with every
// ignored entry linked or copied in alongside it, and returns that directory,
// the snapshot's own copy of source, and the cleanup that undoes both.
//
// The index alone is not enough to run anything real over: a scoped mutation
// run needs node_modules/ to resolve imports, and Stryker's own sandbox needs
// whatever a build already produced. Those are exactly what git ignores, so
// they never appear in checkout-index's own output — this reads for them a
// second time, deliberately, from the untracked-and-ignored side of the
// index instead.
//
// root and source are the same directory in a conventional layout, and
// different ones whenever the JS project lives in a subdirectory — the real
// consumer's own shape, a Wails-style frontend/. checkout-index has to run
// from root: run from source instead, it only ever writes source's own
// subtree at source-relative paths, so a tracked file outside source goes
// missing and the file count silently matches anyway (both sides are
// source-relative). snapshotSource is dir joined with root's own relative
// path to source, which is where a caller runs Stryker, tsc or vitest.
func Snapshot(root, source string) (dir, snapshotSource string, cleanup func() error, err error) {
	// The prefix stays short on purpose: a 118-character prefix here measured
	// "Filename too long" once every staged path was joined onto it, on a
	// machine whose base temp directory was already deep.
	dir, err = os.MkdirTemp("", "dh-")
	if err != nil {
		return "", "", nil, fmt.Errorf("create the snapshot directory: %w", err)
	}

	rel, err := filepath.Rel(root, source)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", "", nil, fmt.Errorf("relate %s to %s: %w", source, root, err)
	}
	snapshotSource = filepath.Join(dir, rel)

	links, err := materialise(root, source, dir)
	if err != nil {
		_ = cleanupLinks(links)
		_ = os.RemoveAll(dir)
		return "", "", nil, err
	}

	return dir, snapshotSource, func() error {
		// Every link is removed by its own exact path before anything
		// recurses into the tree that holds it. Measured directly against
		// go1.27.0 windows/amd64: os.RemoveAll called on a `mklink /J`
		// junction alone already stops at the reparse point rather than
		// recursing into its target — see snapshot_test.go's own sentinel
		// proof — so this unlink-first order is defence in depth against a
		// different link kind or a future Go version, not the mechanism the
		// safety depends on today.
		if err := cleanupLinks(links); err != nil {
			return err
		}
		return os.RemoveAll(dir)
	}, nil
}

// materialise checks the whole index out into dir from root and links every
// ignored entry from source in alongside it, returning the paths of the
// links it created — and only those, so cleanup never has to guess which
// entries under dir are links rather than checked-out files.
func materialise(root, source, dir string) ([]string, error) {
	prefix := dir + string(filepath.Separator)
	if _, err := gitOutput(root, "-c", "core.longpaths=true", "checkout-index", "--all", "--prefix="+prefix); err != nil {
		return nil, fmt.Errorf("check the index out into %s: %w", dir, err)
	}

	tracked, err := gitOutput(root, "ls-files", "-z")
	if err != nil {
		return nil, fmt.Errorf("count the index: %w", err)
	}
	want := len(splitNUL(tracked))

	got, err := countFiles(dir)
	if err != nil {
		return nil, fmt.Errorf("count the checked-out snapshot: %w", err)
	}
	if got != want {
		return nil, fmt.Errorf("checkout-index wrote %d file(s) into %s, the index holds %d", got, dir, want)
	}

	return linkIgnored(root, source, dir)
}

// countFiles counts the regular files under root, the same unit `git
// ls-files` counts in — a directory git created along the way is not itself
// one of the index's own entries.
func countFiles(root string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			count++
		}
		return nil
	})
	return count, err
}

// linkIgnored reads the untracked-and-ignored side of the index, from
// source, and brings each entry into the snapshot's own copy of source under
// dir: a directory is linked whole (node_modules/, dist/, wailsjs/ and the
// like), a file is copied.
//
// Ignored only, deliberately, never every untracked entry. Measured: linking
// every untracked entry in a real Wails project copied in a test file that
// had been written but never `git add`ed, and its presence flipped a real
// 11-Survived verdict to 11-Killed — a false green from a file the staged
// change never touched at all. `--ignored --exclude-standard` gave exactly
// the three directories that project's own .gitignore named.
//
// root and dir are only used to place source's own copy correctly when
// source is a subdirectory of root: an ignored entry belongs at
// <dir>/<root-to-source>/<entry>, not at <dir>/<entry>, or it would land
// beside the snapshot's copy of source rather than inside it.
func linkIgnored(root, source, dir string) ([]string, error) {
	out, err := gitOutput(source, "ls-files", "--others", "--ignored", "--exclude-standard", "--directory", "-z")
	if err != nil {
		return nil, fmt.Errorf("list ignored entries: %w", err)
	}

	sourceRel, err := filepath.Rel(root, source)
	if err != nil {
		return nil, fmt.Errorf("relate %s to %s: %w", source, root, err)
	}
	snapshotSource := filepath.Join(dir, sourceRel)

	// git 2.51 lists an untracked directory holding only ignored content
	// alongside the ignored directory nested in it — `stryker/` and
	// `stryker/node_modules/` both. Linking the parent already brings every
	// descendant in, and linking one again fails on the path the first link
	// created. Sorted, an ancestor always precedes its descendants, so one
	// pass over the linked prefixes skips them whatever order git used.
	entries := splitNUL(out)
	sort.Strings(entries)

	var links []string
	var linked []string
	for _, entry := range entries {
		if coveredBy(linked, entry) {
			continue
		}
		isDir := strings.HasSuffix(entry, "/")
		relative := filepath.FromSlash(strings.TrimSuffix(entry, "/"))
		target := filepath.Join(source, relative)
		destination := filepath.Join(snapshotSource, relative)

		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return links, fmt.Errorf("prepare %s: %w", destination, err)
		}

		if isDir {
			if err := link(target, destination); err != nil {
				return links, fmt.Errorf("link %s: %w", entry, err)
			}
			links = append(links, destination)
			linked = append(linked, entry)
			continue
		}

		if err := copyFile(target, destination); err != nil {
			return links, fmt.Errorf("copy %s: %w", entry, err)
		}
	}
	return links, nil
}

// coveredBy reports whether entry sits under one of the linked directories,
// each of which carries git's own trailing slash, so `stryker/` never covers
// a sibling named `stryker-old/`.
func coveredBy(linked []string, entry string) bool {
	for _, directory := range linked {
		if strings.HasPrefix(entry, directory) {
			return true
		}
	}
	return false
}

// copyFile duplicates target's bytes and mode at destination.
//
// A copy rather than a link for an ignored file: a single ignored file
// outside any ignored directory is rare enough that dharness has not needed
// to link it, and copying keeps the cleanup story simple — a copy is always
// safe for os.RemoveAll to recurse into, and only directories carry the
// junction/symlink risk cleanupLinks exists for.
func copyFile(target, destination string) error {
	info, err := os.Stat(target)
	if err != nil {
		return err
	}

	in, err := os.Open(target)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, in)
	return err
}

// cleanupLinks removes every link by its own exact path, tolerating one
// already gone. Called before the recursive removal that follows it, never
// after: see Snapshot's own cleanup closure for why the order is load-bearing
// rather than cosmetic.
func cleanupLinks(links []string) error {
	for _, path := range links {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove link %s: %w", path, err)
		}
	}
	return nil
}
