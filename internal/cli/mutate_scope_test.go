package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

// TestMutateAcceptsAPathReachedThroughAnotherSpellingOfTheSameDirectory pins
// the Windows defect where one directory has two names: the working directory
// arrives spelled the way it was entered, and project discovery asks git, which
// answers with the reparse point resolved. filepath.Rel compares the two
// spellings textually, so every path under the pair read as "outside this
// project's JS source" and a genuinely mutatable file was refused.
//
// filepath.EvalSymlinks is not the answer here: measured on Windows 11 with Go
// 1.27, EvalSymlinks(link) returns the junction spelling unchanged. Filesystem
// identity is: os.SameFile reports the junction and the directory it points at
// as the same directory, and reports a child reached through the junction as
// the same file as the child reached through the directory itself.
func TestMutateAcceptsAPathReachedThroughAnotherSpellingOfTheSameDirectory(t *testing.T) {
	real := t.TempDir()
	consumer := filepath.Join(real, "consumer")
	for _, name := range []string{"src/nested/a.ts", "src/nested/b.ts"} {
		writeFile(t, filepath.Join(consumer, filepath.FromSlash(name)), "export const x = 1;\n")
	}
	// A real file outside the JS project, so the refusal below is decided by
	// containment and not by a path that fails to stat.
	writeFile(t, filepath.Join(real, "elsewhere", "b.ts"), "export const y = 1;\n")

	alternate := secondSpellingOf(t, real)

	// Both directions are the same physical tree. Which side carries the
	// junction spelling is an accident of how the command was started, and the
	// containment decision must not depend on it.
	cases := []struct {
		name, source, from, given string
	}{
		{
			"the working directory arrives through the junction and git reports the target",
			consumer, filepath.Join(alternate, "consumer"), "src/nested/a.ts",
		},
		{
			"git reports the junction and the working directory arrives at the target",
			filepath.Join(alternate, "consumer"), consumer, "src/nested/b.ts",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			p := project.At(real, testCase.source)
			scoped, err := scopePaths(p, testCase.from, []string{testCase.given})
			if err != nil {
				t.Fatalf("scopePaths(%q) refused a path inside the source: %v", testCase.given, err)
			}
			if len(scoped) != 1 || scoped[0].Argument() != testCase.given {
				t.Errorf("scopePaths() = %v, want [%s]", scoped, testCase.given)
			}
		})
	}

	// The added tolerance is for spelling, not for location: a path that only
	// exists outside the JS project stays refused, in the same words.
	t.Run("a path that is genuinely outside is still refused", func(t *testing.T) {
		p := project.At(real, consumer)
		given := filepath.Join("..", "elsewhere", "b.ts")

		_, err := scopePaths(p, filepath.Join(alternate, "consumer"), []string{given})

		var outside *PathOutsideSourceError
		if !errors.As(err, &outside) {
			t.Fatalf("scopePaths(%q) = %v, want PathOutsideSourceError", given, err)
		}
		want := fmt.Sprintf("%s is outside this project's JS source (%s), and Stryker only mutates what is inside it", given, consumer)
		if err.Error() != want {
			t.Errorf("PathOutsideSourceError = %q, want %q", err.Error(), want)
		}
	})
}

// secondSpellingOf returns a second path naming dir, created rather than found.
//
// Windows uses a directory junction because creating a symlink needs a
// privilege a plain checkout does not have, and a junction is the case the
// field report came from. When the platform cannot create either, the caller is
// skipped with the reason rather than left passing without covering the case.
func secondSpellingOf(t *testing.T, dir string) string {
	t.Helper()
	link := filepath.Join(t.TempDir(), "alternate")
	if runtime.GOOS == "windows" {
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, dir).CombinedOutput(); err != nil {
			t.Skipf("cannot create a directory junction without it: %v: %s", err, out)
		}
		return link
	}
	if err := os.Symlink(dir, link); err != nil {
		t.Fatalf("symlink %s -> %s: %v", link, dir, err)
	}
	return link
}

// TestMutateRefusesPathsWithCharactersStrykerWouldMisread pins the path
// refusal. Escaping a comma inside one --mutate argument would be
// indistinguishable from the comma that separates paths, and *, ?, {, } and a
// leading ! are minimatch metacharacters Stryker's own glob matching already
// treats specially — escaping any of them is unmeasured here, so dharness
// refuses rather than guesses.
func TestMutateRefusesPathsWithCharactersStrykerWouldMisread(t *testing.T) {
	root := t.TempDir()
	p := project.At(root, root)

	for _, given := range []string{"src/a,b.ts", "src/*.ts", "src/a?.ts", "src/{a,b}.ts", "!src/a.ts"} {
		t.Run(given, func(t *testing.T) {
			_, err := scopePaths(p, root, []string{given})

			var invalid *InvalidMutatePathError
			if !errors.As(err, &invalid) {
				t.Fatalf("scopePaths(%q) = %v, want InvalidMutatePathError", given, err)
			}
			if invalid.Path != given {
				t.Errorf("InvalidMutatePathError.Path = %q, want %q", invalid.Path, given)
			}
		})
	}
}

// TestMutateRefusesPathsThatOnlyBecomeInvalidAfterReRooting pins where the
// check runs: validateMutatePath used to run on the path exactly as typed,
// before filepath.Rel re-rooted it against the JS project. "./!x.ts" and
// "src/../!x.ts" do not start with "!" and carry none of the other refused
// characters, so the old ordering let them through — and Join/Rel then
// cleaned away the leading "./" or "src/../", leaving a re-rooted argument
// "!x.ts" that Stryker's glob matcher reads as a negation nobody typed.
func TestMutateRefusesPathsThatOnlyBecomeInvalidAfterReRooting(t *testing.T) {
	root := t.TempDir()
	p := project.At(root, root)

	for _, given := range []string{"./!x.ts", "src/../!x.ts"} {
		t.Run(given, func(t *testing.T) {
			_, err := scopePaths(p, root, []string{given})

			var invalid *InvalidMutatePathError
			if !errors.As(err, &invalid) {
				t.Fatalf("scopePaths(%q) = %v, want InvalidMutatePathError", given, err)
			}
		})
	}
}

// TestShortestPathNamesTheStateSomebodyCanFind pins the reader-facing half of
// the cumulative note. The absolute form of a state path under the git common
// directory runs past eighty characters before it reaches the part that
// matters, and the whole point of printing it is that somebody can find the
// file — the field report found it with `find . -iname '*stryker*'` after the
// tool never mentioned it.
func TestShortestPathNamesTheStateSomebodyCanFind(t *testing.T) {
	dir := t.TempDir()

	inside := shortestPath(dir, filepath.Join(dir, ".git", "dharness", "stryker-incremental.json"))
	if inside != ".git/dharness/stryker-incremental.json" {
		t.Errorf("shortestPath() = %q, want the path relative to the project, slash-separated", inside)
	}

	outside := shortestPath(dir, filepath.Join(t.TempDir(), "elsewhere.json"))
	if !strings.HasSuffix(outside, "elsewhere.json") || strings.HasPrefix(outside, "..") {
		t.Errorf("shortestPath() = %q, want the absolute path for a target outside the project", outside)
	}
}
