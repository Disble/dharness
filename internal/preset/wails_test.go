package preset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/project"
)

func writeWailsFixtureFile(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestWailsDetectsWailsJSON(t *testing.T) {
	root := t.TempDir()
	writeWailsFixtureFile(t, root, "wails.json", "{}\n")

	match, matched := wails{}.Detect(project.At(root, root))
	if !matched {
		t.Fatal("wails{}.Detect() matched == false, want true")
	}
	if !strings.Contains(match.Evidence, "wails.json") {
		t.Errorf("Evidence = %q, want it to name wails.json", match.Evidence)
	}
}

func TestWailsNoMatchNoEvidence(t *testing.T) {
	root := t.TempDir()

	match, matched := wails{}.Detect(project.At(root, root))
	if matched {
		t.Fatal("wails{}.Detect() matched == true with no wails.json present")
	}
	if match.Evidence != "" {
		t.Errorf("Evidence = %q, want empty for a non-match", match.Evidence)
	}
}

// TestWailsFallsBackToDocumentedDefaultWhenKeyAbsent is task 16.4's case.
//
// The task's own literal example names "wailsjs/**" as the expected value
// for a Root == Source fixture, but that is task 16.7's split-layout answer,
// not this one: design decision 9's formula
// (filepath.Rel(p.Source, filepath.Join(p.Root, wailsJSDir, "wailsjs"))) is
// the authoritative source, verified against Wails' own project.go/base.go,
// and it computes "frontend/wailsjs/**" here — Root and Source are the same
// directory, and the default wailsjsdir is "frontend", so the bindings sit
// one level below both. Reconciled the same way Slice 3 reconciled task
// 10.9 against declaredKeys' actual data flow: recorded here rather than
// silently absorbed, and pinned against the derivation the design verified,
// not the task list's copy of task 16.7's answer.
func TestWailsFallsBackToDocumentedDefaultWhenKeyAbsent(t *testing.T) {
	root := t.TempDir()
	writeWailsFixtureFile(t, root, "wails.json", "{}\n")

	match, matched := wails{}.Detect(project.At(root, root))
	if !matched {
		t.Fatal("wails{}.Detect() matched == false, want true")
	}

	fact := onlyFact(t, match)
	want := []string{"frontend/wailsjs/**"}
	if !equalStringSlices(fact.Value.([]string), want) {
		t.Errorf("ignorePatterns = %v, want %v", fact.Value, want)
	}
	if !strings.Contains(fact.Because, "wailsjsdir") || !strings.Contains(fact.Because, "frontend/") {
		t.Errorf("Because = %q, want it to name the absent key and the frontend/ default", fact.Because)
	}
}

// TestWailsReadsWailsJSDirWhenPresent is task 16.6, using the exact override
// Wails' own SvelteKit guide instructs: "./frontend/src/lib".
func TestWailsReadsWailsJSDirWhenPresent(t *testing.T) {
	root := t.TempDir()
	writeWailsFixtureFile(t, root, "wails.json", `{"wailsjsdir": "./frontend/src/lib"}`)

	match, matched := wails{}.Detect(project.At(root, root))
	if !matched {
		t.Fatal("wails{}.Detect() matched == false, want true")
	}

	fact := onlyFact(t, match)
	want := []string{"frontend/src/lib/wailsjs/**"}
	if !equalStringSlices(fact.Value.([]string), want) {
		t.Errorf("ignorePatterns = %v, want %v", fact.Value, want)
	}
	if !strings.Contains(fact.Because, "wailsjsdir") || !strings.Contains(fact.Because, "frontend/src/lib") {
		t.Errorf("Because = %q, want it to name the key and its value, not the fallback", fact.Because)
	}
	if strings.Contains(fact.Because, "default") {
		t.Errorf("Because = %q, want the configured value, not the fallback text", fact.Because)
	}
}

// TestWailsPatternIsRelativeToSourceInASplitLayout is task 16.7 — the
// motivating repository's own layout and the value design decision 9 was
// verified against.
func TestWailsPatternIsRelativeToSourceInASplitLayout(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "frontend")
	writeWailsFixtureFile(t, root, "wails.json", "{}\n")
	writeWailsFixtureFile(t, source, "package.json", "{\n  \"name\": \"wails-frontend\"\n}\n")

	match, matched := wails{}.Detect(project.At(root, source))
	if !matched {
		t.Fatal("wails{}.Detect() matched == false, want true")
	}

	fact := onlyFact(t, match)
	want := []string{"wailsjs/**"}
	if !equalStringSlices(fact.Value.([]string), want) {
		t.Errorf("ignorePatterns = %v, want %v — not frontend/wailsjs/**", fact.Value, want)
	}
}

func TestWailsEvidenceValidates(t *testing.T) {
	root := t.TempDir()
	writeWailsFixtureFile(t, root, "wails.json", `{"wailsjsdir": "./frontend/src/lib"}`)

	match, _ := wails{}.Detect(project.At(root, root))
	if err := match.Manifest.Validate(); err != nil {
		t.Errorf("wails' manifest fails Validate(): %v", err)
	}
}

// TestWailsMalformedJSONStillMatchesAndReportsUncertain is the orchestrator's
// own acceptance test for this slice's design change: a wails.json this
// preset cannot parse must not read as "not a Wails project". The project
// still matches, still contributes the documented default, and says what it
// could not read.
func TestWailsMalformedJSONStillMatchesAndReportsUncertain(t *testing.T) {
	root := t.TempDir()
	writeWailsFixtureFile(t, root, "wails.json", "{not valid json")

	match, matched := wails{}.Detect(project.At(root, root))
	if !matched {
		t.Fatal("wails{}.Detect() matched == false for a malformed wails.json, want true — this IS a Wails project")
	}
	if match.Uncertain == "" {
		t.Fatal("Uncertain == \"\", want it to name what could not be read")
	}
	if !strings.Contains(match.Uncertain, "wails.json") {
		t.Errorf("Uncertain = %q, want it to name wails.json", match.Uncertain)
	}

	fact := onlyFact(t, match)
	want := []string{"frontend/wailsjs/**"}
	if !equalStringSlices(fact.Value.([]string), want) {
		t.Errorf("ignorePatterns = %v, want the documented default %v even though the file did not parse", fact.Value, want)
	}
	if err := match.Manifest.Validate(); err != nil {
		t.Errorf("a match with Uncertain set still fails Validate(): %v — Uncertain is not part of the manifest contract", err)
	}
}

func onlyFact(t *testing.T, match Match) Fact {
	t.Helper()
	if len(match.Manifest.Facts) != 1 {
		t.Fatalf("Manifest.Facts = %v, want exactly one fact", match.Manifest.Facts)
	}
	return match.Manifest.Facts[0]
}

func equalStringSlices(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// Rendered evidence must name the file, never the path to it.
//
// Everything a preset renders is written into .dharness/fallow.jsonc, which
// is committed. An absolute path there leaks the layout of one machine's disk
// into a shared file, makes the file differ on every developer's checkout,
// and — because ownedFilesStep.Satisfied compares the region's bytes — leaves
// the step permanently unsatisfied for everyone except whoever ran sync last,
// rewriting it on every run forever.
//
// The golden fixtures cannot catch this: they substitute the root path with
// <root> precisely so a fixture captured on one machine passes on another, so
// the leak is invisible to them by construction. Hence a direct test.
func TestRenderedEvidenceNamesNoAbsolutePath(t *testing.T) {
	root := t.TempDir()
	writeWailsFixtureFile(t, root, "wails.json", `{"wailsjsdir": "./frontend/src/lib"}`)
	p := project.Project{Root: root, Source: filepath.Join(root, "frontend")}

	match, ok := wails{}.Detect(p)
	if !ok {
		t.Fatal("Detect() = false for a repository with wails.json")
	}

	rendered := []string{match.Evidence, match.Uncertain}
	for _, fact := range match.Manifest.Facts {
		rendered = append(rendered, fact.Because)
	}
	for _, seed := range match.Manifest.Seeds {
		rendered = append(rendered, seed.Text, seed.Because)
	}

	for _, text := range rendered {
		if text == "" {
			continue
		}
		if strings.Contains(text, filepath.ToSlash(root)) || strings.Contains(text, root) {
			t.Errorf("rendered evidence carries the absolute path to the repository:\n%s", text)
		}
		if !strings.Contains(text, wailsJSONFile) && strings.Contains(text, "wails") {
			continue
		}
	}

	if !strings.Contains(match.Evidence, wailsJSONFile) {
		t.Errorf("Evidence does not name the file it read: %q", match.Evidence)
	}
}
