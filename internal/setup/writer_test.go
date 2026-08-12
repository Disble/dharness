package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Disble/dharness/internal/report"
)

// TestChangedClassifiesCreatedModifiedUnchanged pins the classification
// requirement's three scenarios: a file that did not exist is created, a
// pre-existing file rewritten to different bytes is modified, and a
// pre-existing file rewritten to byte-identical bytes is unchanged — the
// named mutation-coverage row (design.md), since a mutant collapsing
// unchanged into modified would still pass every test that never exercises
// an idempotent rewrite. Path is asserted root-relative and naming its
// directory (defect 9).
func TestChangedClassifiesCreatedModifiedUnchanged(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "sub")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	modifiedPath := filepath.Join(dir, "modified.txt")
	unchangedPath := filepath.Join(dir, "unchanged.txt")
	createdPath := filepath.Join(dir, "created.txt")

	if err := os.WriteFile(modifiedPath, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unchangedPath, []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}

	w := &Writer{}
	from := len(w.touched)
	if err := w.Write(modifiedPath, []byte("after")); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(unchangedPath, []byte("same")); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(createdPath, []byte("new")); err != nil {
		t.Fatal(err)
	}
	to := len(w.touched)

	changes := w.Changed(root, from, to)
	if len(changes) != 3 {
		t.Fatalf("Changed() returned %d entries, want 3: %+v", len(changes), changes)
	}

	want := map[string]report.Kind{
		"sub/modified.txt":  report.Modified,
		"sub/unchanged.txt": report.Unchanged,
		"sub/created.txt":   report.Created,
	}
	seen := map[string]bool{}
	for _, change := range changes {
		seen[change.Path] = true
		kind, ok := want[change.Path]
		if !ok {
			t.Errorf("Changed() reported an unexpected path %q", change.Path)
			continue
		}
		if change.Kind != kind {
			t.Errorf("Changed()[%q].Kind = %v, want %v", change.Path, change.Kind, kind)
		}
		if filepath.ToSlash(filepath.Dir(change.Path)) != "sub" {
			t.Errorf("Changed()[%q] does not name its directory", change.Path)
		}
	}
	for path := range want {
		if !seen[path] {
			t.Errorf("Changed() is missing %q", path)
		}
	}
}

// TestChangedUnreadableExistingFileReportsModifiedNotUnchanged kills the
// mutant that treats a failed read-back as "no change": a file this run
// snapshotted as existing, then removed before Changed reads it back, must
// report modified — claiming unchanged from a read it never completed is
// the fabrication §09 forbids (design.md's "Changed's existed arm").
func TestChangedUnreadableExistingFileReportsModifiedNotUnchanged(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "gone.txt")
	if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}

	w := &Writer{}
	from := len(w.touched)
	if err := w.remember(path); err != nil {
		t.Fatal(err)
	}
	to := len(w.touched)

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	changes := w.Changed(root, from, to)
	if len(changes) != 1 || changes[0].Kind != report.Modified {
		t.Fatalf("Changed() = %+v, want exactly one entry with Kind == modified", changes)
	}
}
