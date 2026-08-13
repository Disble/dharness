package cli

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Disble/dharness/internal/project"
)

// Stryker runs in the JS project, so the path a person types from the
// repository root is not the path Stryker reads. Left alone it would receive
// frontend/src/a.ts interpreted from inside frontend/ — nothing, mutated
// without complaint, and a clean report to show for it.
func TestMutatePathsAreReExpressedForTheDirectoryStrykerRunsIn(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "frontend")
	p := project.At(root, source)

	cases := []struct {
		name, from, given, want string
	}{
		{"typed from the repository root", root, "frontend/src/a.ts", "src/a.ts"},
		{"typed from the JS project", source, "src/a.ts", "src/a.ts"},
		{"given absolute", root, filepath.Join(source, "src", "a.ts"), "src/a.ts"},
		// The path moves and the range must not: it addresses lines in a file,
		// not a location on disk.
		{"a line range rides along", root, "frontend/src/a.ts:5-7", "src/a.ts:5-7"},
		{"columns ride along verbatim", root, "frontend/src/a.ts:1:3-1:5", "src/a.ts:1:3-1:5"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			scoped, err := scopePaths(p, testCase.from, []string{testCase.given})
			if err != nil {
				t.Fatalf("scopePaths() = %v", err)
			}
			if len(scoped) != 1 || scoped[0].Argument() != testCase.want {
				t.Errorf("scopePaths() = %v, want [%s]", scoped, testCase.want)
			}
		})
	}
}

// A path Stryker could never reach is refused, because mutating nothing and
// mutating something nothing survived look identical in the report.
func TestMutateRefusesPathsOutsideTheJSProject(t *testing.T) {
	root := t.TempDir()
	p := project.At(root, filepath.Join(root, "frontend"))

	_, err := scopePaths(p, root, []string{"internal/app/app.go"})

	var outside *PathOutsideSourceError
	if !errors.As(err, &outside) {
		t.Fatalf("scopePaths() = %v, want PathOutsideSourceError", err)
	}
	if outside.Path != "internal/app/app.go" {
		t.Errorf("the error does not name the path: %v", outside)
	}
}
