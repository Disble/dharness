package tiers_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Disble/dharness/internal/tiers"
)

func put(t *testing.T, root, path, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveAdoptedFeatureIncludesOldSourcesWithoutAdoptingLegacy(t *testing.T) {
	root := t.TempDir()
	var declaration tiers.Manifest
	if err := json.Unmarshal([]byte(checkout), &declaration); err != nil {
		t.Fatal(err)
	}
	declaration.EnforcedPrefixes = append(declaration.EnforcedPrefixes, "src/features/checkout/")
	declaration.Features = append(declaration.Features, tiers.Feature{ID: "receipt", Path: "src/features/receipt/", DefaultTier: "important", Reason: "Receipts"})
	raw, err := json.Marshal(declaration)
	if err != nil {
		t.Fatal(err)
	}
	put(t, root, "coverage-tiers.json", string(raw))
	for _, path := range []string{
		"src/features/checkout/calculate-total.ts",
		"src/features/checkout/validate-order.ts",
		"src/features/checkout/types.ts",
		"src/features/checkout/ui/Checkout.tsx",
		"src/legacy/old.ts",
		"src/features/receipt/archive.ts",
	} {
		put(t, root, path, "export const value = 1;\n")
	}
	before, err := os.ReadFile(filepath.Join(root, "coverage-tiers.json"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := tiers.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"version":1,"active":true,"files":[{"path":"src/features/checkout/calculate-total.ts","tier":"core","coverage":100},{"path":"src/features/checkout/types.ts","tier":"infrastructure","coverage":0},{"path":"src/features/checkout/ui/Checkout.tsx","tier":"important","coverage":80},{"path":"src/features/checkout/validate-order.ts","tier":"core","coverage":100},{"path":"src/features/receipt/archive.ts","tier":"important","coverage":80}]}`
	if string(encoded) != want {
		t.Fatalf("resolution = %s, want %s", encoded, want)
	}
	if tier, adopted := got.Tier("src/legacy/old.ts"); adopted || tier != "" {
		t.Fatalf("legacy classification = %q, %v; legacy was not adopted", tier, adopted)
	}
	after, err := os.ReadFile(filepath.Join(root, "coverage-tiers.json"))
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("read-only resolution changed the manifest: %v", err)
	}
}

func TestResolveRejectsUnclassifiedSourceInsideAdoptedScope(t *testing.T) {
	root := t.TempDir()
	put(t, root, "coverage-tiers.json", checkout)
	put(t, root, "src/features/search/query.ts", "export const query = 1;\n")
	got, err := tiers.Resolve(root)
	want := "coverage-tiers.json: source src/features/search/query.ts has no declared feature; declare it or narrow the adopted prefixes"
	if err == nil || err.Error() != want || got.Active || len(got.Files) != 0 {
		t.Fatalf("resolution = %+v, error = %v, want %s", got, err, want)
	}
}

func TestResolveUsesSpecificOverridesIndependentOfDeclarationOrder(t *testing.T) {
	parent := `{"path":"src/ui/","tier":"important","reason":"UI"}`
	child := `{"path":"src/ui/forms/","tier":"core","reason":"Validation"}`
	exact := `{"path":"src/ui/forms/types.ts","tier":"infrastructure","reason":"Types"}`
	for _, overrides := range []string{parent + "," + child + "," + exact, exact + "," + child + "," + parent} {
		root := t.TempDir()
		put(t, root, "coverage-tiers.json", `{"version":1,"enforcedPrefixes":["src/"],"features":[{"id":"cart","path":"src/","defaultTier":"core","reason":"Decisions","overrides":[`+overrides+`]}]}`)
		want := map[string]string{"src/total.ts": "core", "src/ui/Cart.tsx": "important", "src/ui/forms/validate.ts": "core", "src/ui/forms/types.ts": "infrastructure"}
		for path := range want {
			put(t, root, path, "export const value = 1;\n")
		}
		got, err := tiers.Resolve(root)
		if err != nil || len(got.Files) != 4 {
			t.Fatalf("specific override resolution = %+v, %v", got, err)
		}
		for path, expected := range want {
			if tier, adopted := got.Tier(path); !adopted || tier != expected {
				t.Fatalf("%s classified as %q, %v; want %s", path, tier, adopted, expected)
			}
		}
	}
}

func TestResolveRequiresManifestWhenManagedCoverageIntegrationExists(t *testing.T) {
	root := t.TempDir()
	put(t, root, ".dharness/coverage-tiers.mjs", "// managed coverage integration\n")
	got, err := tiers.Resolve(root)
	want := "coverage-tiers.json: required by the managed coverage integration; restore the manifest"
	if err == nil || err.Error() != want || got.Active || len(got.Files) != 0 {
		t.Fatalf("missing active policy = %+v, %v; want %s", got, err, want)
	}
	if _, err := os.Stat(filepath.Join(root, "coverage-tiers.json")); !os.IsNotExist(err) {
		t.Fatalf("resolver recreated missing consumer manifest: %v", err)
	}
}

func TestResolveDoesNotClassifyTestsOrDependencies(t *testing.T) {
	root := t.TempDir()
	put(t, root, "coverage-tiers.json", checkout)
	for _, path := range []string{"src/features/checkout/ui/Checkout.test.tsx", "src/features/checkout/ui/Checkout.spec.tsx", "src/features/checkout/__tests__/order.ts", "src/features/checkout/node_modules/pkg/index.ts", "src/features/checkout/.git/ignored.ts", "src/features/checkout/.dharness/ignored.ts", "src/features/checkout/README.md"} {
		put(t, root, path, "export const value = 1;\n")
	}
	got, err := tiers.Resolve(root)
	if err != nil || !got.Active || len(got.Files) != 0 {
		t.Fatalf("non-business sources entered resolution: %+v, %v", got, err)
	}
}
