package setup

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/runner"
)

// updateGoldens regenerates the framework fixtures TestFrameworkGoldens reads.
// It reaches only the framework cases — the generic goldens below have no
// update path at all, per Decision 7 of the framework-presets design: any
// diff in them is a regression by definition, and accepting one is a
// deliberate hand edit, never a re-run.
var updateGoldens = flag.Bool("update", false, "regenerate framework golden fixtures (TestFrameworkGoldens only)")

// TestGenericGoldenIsUnchanged is the frozen baseline for this whole change.
// It compares Plan()'s report and the tree Apply writes against a fixture
// captured before the preset registry existed, by plain byte equality. There
// is no -update path here, on purpose: TestFrameworkGoldens below is the one
// mechanism that regenerates a fixture, and it never reaches these two cases.
func TestGenericGoldenIsUnchanged(t *testing.T) {
	cases := []struct {
		name    string
		project func(t *testing.T) project.Project
	}{
		{"generic-conventional", genericConventionalProject},
		{"generic-split", genericSplitProject},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderGolden(t, tc.project(t))
			want, err := os.ReadFile(filepath.Join("testdata", "golden", tc.name+".txt"))
			if err != nil {
				t.Fatalf("read golden fixture: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s golden differs from the committed fixture — this is a regression, not a case to -update:\n--- want ---\n%s\n--- got ---\n%s",
					tc.name, want, got)
			}
		})
	}
}

// TestFrameworkGoldens is the living half of Decision 7's mechanism: a diff
// here is expected once a preset changes what it contributes, and
// `go test ./internal/setup -run TestFrameworkGoldens -update` regenerates
// the fixture. Slice 5 populates the case table; it stays empty here so the
// two categories are structurally distinct from this commit, not merely
// documented as distinct.
func TestFrameworkGoldens(t *testing.T) {
	cases := []struct {
		name    string
		project func(t *testing.T) project.Project
	}{
		{"wails", wailsProject},
		{"nextjs", nextjsProject},
		{"wails-nextjs", wailsNextjsProject},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderGolden(t, tc.project(t))
			path := filepath.Join("testdata", "golden", tc.name+".txt")

			if *updateGoldens {
				if err := os.WriteFile(path, got, 0o600); err != nil {
					t.Fatalf("write golden fixture: %v", err)
				}
				return
			}

			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden fixture: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s golden differs — re-run with -update if this is the manifest fact the commit says changed:\n--- want ---\n%s\n--- got ---\n%s",
					tc.name, want, got)
			}
		})
	}
}

// TestGenericMechanismHasNoUpdatePath pins the structural separation itself:
// TestGenericGoldenIsUnchanged's own source must never wire the -update flag
// it would need to regenerate its fixtures. A grep over the test source, not
// a runtime assertion — the two mechanisms have to stay distinct in the code
// an author reads, not merely behave differently.
func TestGenericMechanismHasNoUpdatePath(t *testing.T) {
	raw, err := os.ReadFile("golden_test.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)

	start := strings.Index(src, "func TestGenericGoldenIsUnchanged")
	if start == -1 {
		t.Fatal("TestGenericGoldenIsUnchanged not found in golden_test.go")
	}
	end := strings.Index(src, "func TestFrameworkGoldens")
	if end == -1 {
		end = len(src)
	}

	if body := src[start:end]; strings.Contains(body, `flag.Bool("update"`) {
		t.Error("TestGenericGoldenIsUnchanged wires an -update flag; the generic goldens have no update path (Decision 7)")
	}
}

// renderGolden captures Plan()'s report and the tree Apply writes, in the
// two-region format Decision 7 fixes: every step's ID/Satisfied/Delegated's
// ok+why/Describe, then every file under p.Root after Apply, sorted and
// fenced.
//
// runner.Run is stubbed so installStep (and, once wired, hookInstallStep)
// never shells out, and gitOutput is stubbed so Discover and the barrel probe
// answer from the fixture rather than a real repository. p.Root and p.Source
// are substituted as <root>/<source> so the fixture is stable across
// machines, and every line is written with an explicit "\n" — never the
// platform's newline — so it is stable across platforms too.
func renderGolden(t *testing.T, p project.Project) []byte {
	t.Helper()

	t.Cleanup(runner.SetForTest(func(runner.Command, io.Writer, io.Writer) error { return nil }))
	t.Cleanup(project.SetGitOutputForTest(func(string, ...string) ([]byte, error) { return nil, nil }))

	var buf bytes.Buffer
	buf.WriteString("== plan ==\n")
	for i, step := range Plan() {
		satisfied := step.Satisfied(p)
		why, delegated := step.Delegated(p)

		fmt.Fprintf(&buf, "%d  %s\n", i+1, step.ID())
		fmt.Fprintf(&buf, "   satisfied=%t delegated=%t\n", satisfied, delegated)
		buf.WriteString("   why |\n")
		writeIndented(&buf, substitutePaths(why, p))
		buf.WriteString("   describe |\n")
		writeIndented(&buf, substitutePaths(step.Describe(p), p))
	}

	if err := Apply(p, io.Discard); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	buf.WriteString("\n== tree ==\n")
	for _, rel := range writtenTree(t, p.Root) {
		content, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		text := substitutePaths(strings.ReplaceAll(string(content), "\r\n", "\n"), p)

		fmt.Fprintf(&buf, "%s\n---\n", rel)
		buf.WriteString(text)
		if !strings.HasSuffix(text, "\n") {
			buf.WriteString("\n")
		}
		buf.WriteString("---\n")
	}

	return buf.Bytes()
}

// writtenTree lists every file under root, relative and slash-separated, in
// sorted order — deterministic regardless of the directory read order the
// platform happens to give back.
func writtenTree(t *testing.T, root string) []string {
	t.Helper()

	var paths []string
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	sort.Strings(paths)
	return paths
}

// writeIndented writes s five spaces deep, one line at a time, so a
// multi-line Describe or Delegated reason nests visibly under its label.
func writeIndented(buf *bytes.Buffer, s string) {
	for _, line := range strings.Split(s, "\n") {
		buf.WriteString("     " + line + "\n")
	}
}

// substitutePaths replaces p.Source (when it differs from p.Root) and p.Root
// with <source>/<root>, in that order — Source nests inside Root in a split
// layout, so replacing Root first would swallow the more specific match.
// Both the native and slash-separated spellings are replaced, since Describe
// text is built with fmt.Sprintf on raw fields while rendered paths go
// through filepath.ToSlash.
func substitutePaths(s string, p project.Project) string {
	if p.HasSource() && p.Source != p.Root {
		s = strings.ReplaceAll(s, p.Source, "<source>")
		s = strings.ReplaceAll(s, filepath.ToSlash(p.Source), "<source>")
	}
	s = strings.ReplaceAll(s, p.Root, "<root>")
	s = strings.ReplaceAll(s, filepath.ToSlash(p.Root), "<root>")
	return s
}

// genericConventionalProject is Root == Source: a conventional layout, no
// framework signal anywhere. This is the fixture Slice 1's golden pins.
func genericConventionalProject(t *testing.T) project.Project {
	t.Helper()
	root := t.TempDir()
	writeGoldenFixtureFile(t, root, "package.json", "{\n  \"name\": \"conventional\"\n}\n")
	writeGoldenFixtureFile(t, root, "package-lock.json", "{\n  \"lockfileVersion\": 3\n}\n")

	p := project.At(root, root)
	p.InRepository = true
	return p
}

// genericSplitProject is Root != Source, the Wails-shaped layout — a
// repository root with no package.json, and the JS project one level down —
// but with no wails.json anywhere, so it still resolves to generic.
func genericSplitProject(t *testing.T) project.Project {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "frontend")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	writeGoldenFixtureFile(t, source, "package.json", "{\n  \"name\": \"split\"\n}\n")
	writeGoldenFixtureFile(t, source, "package-lock.json", "{\n  \"lockfileVersion\": 3\n}\n")

	p := project.At(root, source)
	p.InRepository = true
	return p
}

// wailsProject is the motivating repository's own layout: a repository root
// carrying wails.json with no wailsjsdir override, and the JS project one
// level down at frontend/ — the split shape design decision 9's
// "wailsjs/**" derivation was verified against. This is the "wails"
// framework golden; regenerated with -update, never hand-edited.
func wailsProject(t *testing.T) project.Project {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "frontend")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	writeGoldenFixtureFile(t, root, "wails.json", "{}\n")
	writeGoldenFixtureFile(t, source, "package.json", "{\n  \"name\": \"wails-frontend\"\n}\n")
	writeGoldenFixtureFile(t, source, "package-lock.json", "{\n  \"lockfileVersion\": 3\n}\n")

	p := project.At(root, source)
	p.InRepository = true
	return p
}

// nextjsProject is a conventional layout (Root == Source) whose package.json
// declares the "next" dependency and whose source tree has an app/
// directory — Next.js's App Router. This is the "nextjs" framework golden.
func nextjsProject(t *testing.T) project.Project {
	t.Helper()
	root := t.TempDir()
	writeGoldenFixtureFile(t, root, "package.json", "{\n  \"name\": \"nextjs-app\",\n  \"dependencies\": {\n    \"next\": \"^14.0.0\"\n  }\n}\n")
	writeGoldenFixtureFile(t, root, "package-lock.json", "{\n  \"lockfileVersion\": 3\n}\n")
	if err := os.MkdirAll(filepath.Join(root, "app"), 0o755); err != nil {
		t.Fatal(err)
	}

	p := project.At(root, root)
	p.InRepository = true
	return p
}

// wailsNextjsProject is the multi-preset composition scenario the design's
// Data Flow section names: a Wails root (wails.json) whose split-layout JS
// project depends on Next.js — one Root match and one Source match
// contributing into the same repository. This is the "wails-nextjs"
// framework golden.
func wailsNextjsProject(t *testing.T) project.Project {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "frontend")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	writeGoldenFixtureFile(t, root, "wails.json", "{}\n")
	writeGoldenFixtureFile(t, source, "package.json", "{\n  \"name\": \"wails-nextjs-frontend\",\n  \"dependencies\": {\n    \"next\": \"^14.0.0\"\n  }\n}\n")
	writeGoldenFixtureFile(t, source, "package-lock.json", "{\n  \"lockfileVersion\": 3\n}\n")
	if err := os.MkdirAll(filepath.Join(source, "app"), 0o755); err != nil {
		t.Fatal(err)
	}

	p := project.At(root, source)
	p.InRepository = true
	return p
}

func writeGoldenFixtureFile(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
