package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveSandboxDeletesTheWholeTree(t *testing.T) {
	root := t.TempDir()
	sandbox := filepath.Join(root, "stryker-tmp", "sandbox-abc", "src")
	if err := os.MkdirAll(sandbox, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sandbox, "a.ts"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(root, "stryker-tmp")
	if err := RemoveSandbox(target); err != nil {
		t.Fatalf("RemoveSandbox() = %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("the sandbox survived removal")
	}
}

// Cleaning runs before a mutation as well as after, so an absent directory is
// the normal case and must not be reported as a failure.
func TestRemoveSandboxAcceptsAnAbsentDirectory(t *testing.T) {
	if err := RemoveSandbox(filepath.Join(t.TempDir(), "never-existed")); err != nil {
		t.Errorf("RemoveSandbox() = %v on a path that was never created", err)
	}
}
