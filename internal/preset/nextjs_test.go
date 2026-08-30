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
// package first"): the framework's own recommended flat config first, then
// react-doctor's Next.js preset, each namespaced so neither can collide with
// a project's own import of the same package, verified against Next.js's and
// react-doctor's own documentation rather than invented.
//
// The specifier is the core-web-vitals subpath, which is what Next.js calls
// "Recommended for most projects" — the bare package this preset used to
// contribute is the weaker base config.
func TestNextjsContributesESLintConfigLayer(t *testing.T) {
	assertLayerContribution(t, nextjs{}.Detect,
		`{"dependencies":{"next":"^14.0.0"}}`, []wantLayer{
			{pkg: "eslint-config-next/core-web-vitals", binding: "dharnessNext", spread: true},
			{pkg: "eslint-plugin-react-doctor", binding: "dharnessReactDoctor", accessor: []string{"configs", "recommended"}, registers: "react-doctor"},
			{pkg: "eslint-plugin-react-doctor", binding: "dharnessReactDoctor", accessor: []string{"configs", "next"}, registers: "react-doctor"},
		})
}

// TestNextjsAddsTheTypeScriptConfigOnlyForATypeScriptProject pins the one
// conditional layer in the registry. Next.js documents
// eslint-config-next/typescript as a companion "for TypeScript projects", so
// contributing it to a JavaScript one would add rules the framework does not
// ask for — and a tsconfig.json is the direct signal for that question.
func TestNextjsAddsTheTypeScriptConfigOnlyForATypeScriptProject(t *testing.T) {
	root := t.TempDir()
	writeWailsFixtureFile(t, root, "package.json", `{"dependencies":{"next":"^14.0.0"}}`)

	match, _ := nextjs{}.Detect(project.At(root, root))
	for _, layer := range match.Manifest.Layers {
		if layer.Package == "eslint-config-next/typescript" {
			t.Fatal("nextjs contributed the TypeScript config to a project with no tsconfig.json")
		}
	}

	writeWailsFixtureFile(t, root, "tsconfig.json", `{"compilerOptions":{"strict":true}}`)

	match, _ = nextjs{}.Detect(project.At(root, root))
	var found bool
	for _, layer := range match.Manifest.Layers {
		if layer.Package == "eslint-config-next/typescript" {
			found = true
			if !layer.Spread {
				t.Error("the TypeScript config is an array Next.js's own example spreads; Spread = false")
			}
		}
	}
	if !found {
		t.Errorf("Manifest.Layers = %+v, want the TypeScript config once tsconfig.json exists", match.Manifest.Layers)
	}
	if err := match.Manifest.Validate(); err != nil {
		t.Errorf("manifest fails Validate(): %v", err)
	}
}

// TestNextjsTypeScriptLayerInstallsTheBasePackage pins the split subpaths
// force: three distinct import specifiers, one package name to install.
func TestNextjsTypeScriptLayerInstallsTheBasePackage(t *testing.T) {
	root := t.TempDir()
	writeWailsFixtureFile(t, root, "package.json", `{"dependencies":{"next":"^14.0.0"}}`)
	writeWailsFixtureFile(t, root, "tsconfig.json", `{}`)

	match, _ := nextjs{}.Detect(project.At(root, root))
	for _, layer := range match.Manifest.Layers {
		if strings.HasPrefix(layer.Package, "eslint-config-next") && layer.InstallName() != "eslint-config-next" {
			t.Errorf("Layer{Package: %q}.InstallName() = %q, want %q", layer.Package, layer.InstallName(), "eslint-config-next")
		}
	}
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
