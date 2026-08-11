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

// The pattern is one property with three inputs, so it is one table.
//
// Design decision 9's formula is
// filepath.Rel(p.Source, filepath.Join(p.Root, wailsJSDir, "wailsjs")), and
// each row is a layout it has to hold for. Three separate functions repeated
// the same arrange-detect-assert block and hid that they exercise a single
// derivation; the duplication was real rather than cosmetic, and collapsing
// it is what makes the shared property visible.
//
// The first row is task 16.4, whose own literal example names "wailsjs/**".
// That is the third row's split-layout answer, not this one: with Root and
// Source the same directory and the default wailsjsdir of "frontend", the
// bindings sit one level below both. The formula is authoritative — verified
// against Wails' own project.go and base.go — so the row pins what it
// derives, and the discrepancy is recorded here rather than absorbed, the
// same posture Slice 3 took with task 10.9.
func TestWailsPatternFollowsTheVerifiedFormula(t *testing.T) {
	for _, layout := range []struct {
		name string
		// sourceRel is empty when the JS project sits at the repository root.
		sourceRel  string
		wailsJSON  string
		want       []string
		because    []string
		notBecause []string
	}{
		{
			name:      "no wailsjsdir, source at the root",
			wailsJSON: "{}\n",
			want:      []string{"frontend/wailsjs/**"},
			because:   []string{"wailsjsdir", "frontend/"},
		},
		{
			name:       "wailsjsdir moved the way Wails' own SvelteKit guide instructs",
			wailsJSON:  `{"wailsjsdir": "./frontend/src/lib"}`,
			want:       []string{"frontend/src/lib/wailsjs/**"},
			because:    []string{"wailsjsdir", "frontend/src/lib"},
			notBecause: []string{"default"},
		},
		{
			name:      "split layout — the motivating repository's own shape",
			sourceRel: "frontend",
			wailsJSON: "{}\n",
			want:      []string{"wailsjs/**"},
		},
	} {
		t.Run(layout.name, func(t *testing.T) {
			root := t.TempDir()
			source := root
			if layout.sourceRel != "" {
				source = filepath.Join(root, layout.sourceRel)
				writeWailsFixtureFile(t, source, "package.json", "{\n  \"name\": \"wails-frontend\"\n}\n")
			}
			writeWailsFixtureFile(t, root, "wails.json", layout.wailsJSON)

			match, matched := wails{}.Detect(project.At(root, source))
			if !matched {
				t.Fatal("wails{}.Detect() matched == false, want true")
			}

			fact := onlyFact(t, match)
			if !equalStringSlices(fact.Value.([]string), layout.want) {
				t.Errorf("ignorePatterns = %v, want %v", fact.Value, layout.want)
			}
			for _, expected := range layout.because {
				if !strings.Contains(fact.Because, expected) {
					t.Errorf("Because = %q, want it to name %q", fact.Because, expected)
				}
			}
			for _, unwanted := range layout.notBecause {
				if strings.Contains(fact.Because, unwanted) {
					t.Errorf("Because = %q, want the configured value rather than %q", fact.Because, unwanted)
				}
			}
		})
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
