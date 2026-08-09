package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/project"
)

// A run that rolls back left the repository as it found it, so nothing may
// survive: neither a file that was created nor an edit to one that existed.
func TestWriterUndoRestoresEverythingItTouched(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "kept.json")
	if err := os.WriteFile(existing, []byte(`{"mine":true}`), 0o600); err != nil {
		t.Fatal(err)
	}

	w := &Writer{}
	if err := w.Write(existing, []byte(`{"overwritten":true}`)); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(filepath.Join(root, "nested", "new.yml"), []byte("added\n")); err != nil {
		t.Fatal(err)
	}

	if err := w.Undo(); err != nil {
		t.Fatalf("Undo() = %v", err)
	}

	restored, err := os.ReadFile(existing)
	if err != nil || string(restored) != `{"mine":true}` {
		t.Errorf("an existing file was not put back: %q, %v", restored, err)
	}
	if _, err := os.Stat(filepath.Join(root, "nested", "new.yml")); !os.IsNotExist(err) {
		t.Error("a created file survived the undo")
	}
}

// The snapshot is of the original, not of whatever the previous write left, so
// two writes to one file still restore what was there before either.
func TestWriterUndoKeepsTheOriginalAcrossRepeatedWrites(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	w := &Writer{}
	for _, contents := range []string{"first", "second"} {
		if err := w.Write(path, []byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Undo(); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(path)
	if string(raw) != "original" {
		t.Errorf("restored %q, want the contents from before the first write", raw)
	}
}

// What enables a project is a hook manager that answers, not membership in a
// list: writing a lefthook file into a husky project creates configuration
// nothing reads.
func TestHookManagerAnswersRatherThanBeingAssumed(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  manager
	}{
		{"lefthook by config", []string{"lefthook.yml"}, managerLefthook},
		{"lefthook by dotted config", []string{".lefthook.yaml"}, managerLefthook},
		{"husky by directory", []string{filepath.Join(".husky", "pre-commit")}, managerHusky},
		{"nothing answers", nil, managerNone},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			for _, name := range testCase.files {
				path := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}

			if got := hookManager(project.Project{Root: root}); got != testCase.want {
				t.Errorf("hookManager() = %v, want %v", got, testCase.want)
			}
		})
	}
}

// With no manager answering there is nothing to satisfy: choosing one for a
// project is a decision, and it must not block everything else in the plan.
func TestGateStepIsSatisfiedWhenNoManagerAnswers(t *testing.T) {
	if !(hookInstallStep{}).Satisfied(project.Project{Root: t.TempDir()}) {
		t.Error("the plan stalled on a project that has no hook manager at all")
	}
}

// The thresholds live in a file because react-doctor accepts only a severity,
// so a rule cannot carry its own number.
func TestOwnedFilesCarryTheThresholdsTheRulesCannot(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root}

	if err := (ownedFilesStep{}).Apply(p, &Writer{}); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, project.Dir, ownedRules))
	if err != nil {
		t.Fatalf("the thresholds file was not written: %v", err)
	}
	if !strings.Contains(string(raw), `"maxFileLines": 500`) {
		t.Errorf("thresholds do not carry the file ceiling:\n%s", raw)
	}

	// The architecture is deliberately absent: it is decided, not detected.
	architecture, err := os.ReadFile(filepath.Join(root, project.Dir, ownedFallow))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(architecture), "boundaries") {
		t.Errorf("dharness declared an architecture it cannot know:\n%s", architecture)
	}
}
