package preset

import (
	"testing"

	"github.com/Disble/dharness/internal/project"
)

func TestExpoDetectsDependency(t *testing.T) {
	root := t.TempDir()
	writeWailsFixtureFile(t, root, "package.json", `{"dependencies":{"expo":"~51.0.0"}}`)

	match, matched := expo{}.Detect(project.At(root, root))
	if !matched {
		t.Fatal("expo{}.Detect() matched == false, want true")
	}
	if len(match.Manifest.Facts) != 0 {
		t.Errorf("Manifest.Facts = %v, want empty — expo ships detection-only", match.Manifest.Facts)
	}
	if len(match.Manifest.Seeds) != 0 {
		t.Errorf("Manifest.Seeds = %v, want empty — expo's structure is unverified against its own docs", match.Manifest.Seeds)
	}
}

func TestExpoDetectsDevDependency(t *testing.T) {
	root := t.TempDir()
	writeWailsFixtureFile(t, root, "package.json", `{"devDependencies":{"expo":"~51.0.0"}}`)

	_, matched := expo{}.Detect(project.At(root, root))
	if !matched {
		t.Fatal("expo{}.Detect() matched == false for expo in devDependencies, want true")
	}
}

func TestExpoNoMatchWithoutDependency(t *testing.T) {
	root := t.TempDir()
	writeWailsFixtureFile(t, root, "package.json", `{"dependencies":{"react-native":"^0.74.0"}}`)

	match, matched := expo{}.Detect(project.At(root, root))
	if matched {
		t.Fatal("expo{}.Detect() matched == true without expo declared")
	}
	if match.Evidence != "" {
		t.Errorf("Evidence = %q, want empty for a non-match", match.Evidence)
	}
}

// TestExpoContributesESLintConfigLayer is TestNextjsContributesESLintConfigLayer's
// counterpart for Expo: eslint-config-expo, namespaced under "dharnessExpo",
// verified against Expo's own "Using ESLint" guide rather than invented —
// unlike Facts and Seeds, this is a checkable observable this change could
// verify, so it ships even though expo otherwise stays detection-only.
func TestExpoContributesESLintConfigLayer(t *testing.T) {
	assertLayerContribution(t, expo{}.Detect,
		`{"dependencies":{"expo":"~51.0.0"}}`, "eslint-config-expo", "dharnessExpo")
}

func TestExpoManifestValidates(t *testing.T) {
	root := t.TempDir()
	writeWailsFixtureFile(t, root, "package.json", `{"dependencies":{"expo":"~51.0.0"}}`)

	match, _ := expo{}.Detect(project.At(root, root))
	if err := match.Manifest.Validate(); err != nil {
		t.Errorf("expo's manifest fails Validate(): %v", err)
	}
}
