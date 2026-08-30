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
// counterpart for Expo: eslint-config-expo/flat.js then react-doctor's
// react-native preset, both namespaced, verified against Expo's own "Using
// ESLint" guide and react-doctor's plugin page rather than invented — unlike
// Facts and Seeds, these are checkable observables, so they ship even though
// expo otherwise stays detection-only.
//
// The Expo layer is spread and react-doctor's two are not. Expo's own
// example includes eslint-config-expo/flat unspread, which reads as "one
// config object" and is not one: the subpath exports an array of 13, and
// the example survives only because defineConfig() flattens nested arrays.
// dharness's factory returns a plain array, so an unspread layer reaches
// ESLint as `TypeError: Unexpected array` at startup — not a Go failure,
// which is why the sandbox run is what proves this.
//
// The specifier carries ".js" for the same class of reason: the package
// ships both flat/ and flat.js and declares no exports map, so the bare
// subpath is a directory import that Node ESM refuses outright.
func TestExpoContributesESLintConfigLayer(t *testing.T) {
	assertLayerContribution(t, expo{}.Detect,
		`{"dependencies":{"expo":"~51.0.0"}}`, []wantLayer{
			{pkg: "eslint-config-expo/flat.js", binding: "dharnessExpo", spread: true},
			{pkg: "eslint-plugin-react-doctor", binding: "dharnessReactDoctor", accessor: []string{"configs", "recommended"}},
			{pkg: "eslint-plugin-react-doctor", binding: "dharnessReactDoctor", accessor: []string{"configs", "react-native"}},
		})
}

// TestExpoLayerInstallsTheBasePackage is the subpath split over Expo's own
// specifier: ESLint imports eslint-config-expo/flat.js, npm installs
// eslint-config-expo.
func TestExpoLayerInstallsTheBasePackage(t *testing.T) {
	root := t.TempDir()
	writeWailsFixtureFile(t, root, "package.json", `{"dependencies":{"expo":"~51.0.0"}}`)

	match, _ := expo{}.Detect(project.At(root, root))
	if got := match.Manifest.Layers[0].InstallName(); got != "eslint-config-expo" {
		t.Errorf("InstallName() = %q, want %q", got, "eslint-config-expo")
	}
}

func TestExpoManifestValidates(t *testing.T) {
	root := t.TempDir()
	writeWailsFixtureFile(t, root, "package.json", `{"dependencies":{"expo":"~51.0.0"}}`)

	match, _ := expo{}.Detect(project.At(root, root))
	if err := match.Manifest.Validate(); err != nil {
		t.Errorf("expo's manifest fails Validate(): %v", err)
	}
}
