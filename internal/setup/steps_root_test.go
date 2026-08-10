package setup

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/project"
)

// lefthook already changes directory per command, so the split layout is one
// key in the file dharness owns rather than anything dharness has to solve.
func TestGateConfigNamesTheJSProjectWhenItIsNotTheRepositoryRoot(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "frontend")

	gate := gateConfig(project.At(root, source))

	if !strings.Contains(gate, "run: dharness check") {
		t.Errorf("the gate stopped invoking the command:\n%s", gate)
	}
	if !strings.Contains(gate, "root: frontend/") {
		t.Errorf("the gate does not tell lefthook where to run:\n%s", gate)
	}
}

// A key that says "run here" in a repository with only one directory is noise
// in a file people read.
func TestGateConfigStaysSilentWhenThereIsOnlyOneRoot(t *testing.T) {
	root := t.TempDir()

	gate := gateConfig(project.At(root, root))

	if strings.Contains(gate, "root:") {
		t.Errorf("the gate names a directory it did not need to:\n%s", gate)
	}
}
