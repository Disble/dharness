package setup

import (
	"testing"

	"github.com/Disble/dharness/internal/project"
)

// TestDefaultSeverityCompilesOnlyWithAProject is the RED for a signature
// change, proven by the build failing against today's one-argument form
// rather than by a runtime assertion.
func TestDefaultSeverityCompilesOnlyWithAProject(t *testing.T) {
	p := project.Project{}
	_ = DefaultSeverity(p, "folder-ownership")
}

// TestFolderOwnershipIsErrorWhereBarrelsExist pins decision 6: barrel
// presence, asked of git through the project, decides the first-write
// default — not a preset, and not the removed offByDefault map.
func TestFolderOwnershipIsErrorWhereBarrelsExist(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(project.SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		return []byte("components/index.ts"), nil
	}))
	p := project.Project{Root: root, Source: root, InRepository: true}

	if got := DefaultSeverity(p, RulesPrefix+"/folder-ownership"); got != "error" {
		t.Errorf("DefaultSeverity() = %q, want \"error\" where barrels exist", got)
	}
}

func TestFolderOwnershipIsOffWithoutBarrels(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(project.SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		return nil, nil
	}))
	p := project.Project{Root: root, Source: root, InRepository: true}

	if got := DefaultSeverity(p, RulesPrefix+"/folder-ownership"); got != "off" {
		t.Errorf("DefaultSeverity() = %q, want \"off\" without barrels", got)
	}
}
