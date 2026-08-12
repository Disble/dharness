package preset

import (
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/project"
)

func TestNextjsDetectsDependency(t *testing.T) {
	root := t.TempDir()
	writeWailsFixtureFile(t, root, "package.json", `{"dependencies":{"next":"^14.0.0"}}`)

	match, matched := nextjs{}.Detect(project.At(root, root))
	if !matched {
		t.Fatal("nextjs{}.Detect() matched == false, want true")
	}
	if !strings.Contains(match.Evidence, "package.json") || !strings.Contains(match.Evidence, "next") {
		t.Errorf("Evidence = %q, want it to name package.json and next", match.Evidence)
	}
}

func TestNextjsDetectsDevDependency(t *testing.T) {
	root := t.TempDir()
	writeWailsFixtureFile(t, root, "package.json", `{"devDependencies":{"next":"^14.0.0"}}`)

	_, matched := nextjs{}.Detect(project.At(root, root))
	if !matched {
		t.Fatal("nextjs{}.Detect() matched == false for next in devDependencies, want true")
	}
}

func TestNextjsNoMatchWithoutDependency(t *testing.T) {
	root := t.TempDir()
	writeWailsFixtureFile(t, root, "package.json", `{"dependencies":{"react":"^19.0.0"}}`)

	match, matched := nextjs{}.Detect(project.At(root, root))
	if matched {
		t.Fatal("nextjs{}.Detect() matched == true without next declared")
	}
	if match.Evidence != "" {
		t.Errorf("Evidence = %q, want empty for a non-match", match.Evidence)
	}
}

// TestNextjsContributesNoIgnorePatterns pins the measured, decisive rule the
// orchestrator recorded: fallow honours gitignore, .next/ is gitignored by
// every Next.js starter, so a pattern here would re-implement what the CLI
// already does.
func TestNextjsContributesNoIgnorePatterns(t *testing.T) {
	root := t.TempDir()
	writeWailsFixtureFile(t, root, "package.json", `{"dependencies":{"next":"^14.0.0"}}`)

	match, _ := nextjs{}.Detect(project.At(root, root))
	for _, fact := range match.Manifest.Facts {
		if fact.Key == "ignorePatterns" {
			t.Fatalf("nextjs contributed ignorePatterns: %v — fallow already honours gitignore, .next/ needs no pattern", fact.Value)
		}
	}
}

// TestNextjsContributesESLintConfigLayer pins the requirement's own
// scenario ("a Next.js project's owned config layers the framework's own
// package first"): eslint-config-next, namespaced under "dharnessNext" so
// it cannot collide with a project's own "import next from ...", verified
// against Next.js's own ESLint documentation rather than invented.
func TestNextjsContributesESLintConfigLayer(t *testing.T) {
	assertLayerContribution(t, nextjs{}.Detect,
		`{"dependencies":{"next":"^14.0.0"}}`, "eslint-config-next", "dharnessNext")
}

// TestNextjsSeedsNameStructureNotZones pins §21's framing: a seed offers
// what the framework documents, evidenced the same way a Fact.Because is,
// and never decides a zone.
func TestNextjsSeedsNameStructureNotZones(t *testing.T) {
	root := t.TempDir()
	writeWailsFixtureFile(t, root, "package.json", `{"dependencies":{"next":"^14.0.0"}}`)

	match, _ := nextjs{}.Detect(project.At(root, root))
	if len(match.Manifest.Seeds) == 0 {
		t.Fatal("Manifest.Seeds is empty, want Next.js's documented structure")
	}
	for _, seed := range match.Manifest.Seeds {
		if seed.Because == "" {
			t.Errorf("seed %q carries no evidence", seed.Text)
		}
	}
	if err := match.Manifest.Validate(); err != nil {
		t.Errorf("nextjs' manifest fails Validate(): %v", err)
	}
}
