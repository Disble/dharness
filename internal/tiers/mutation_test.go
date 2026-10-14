package tiers_test

import (
	"testing"

	"github.com/Disble/dharness/internal/tiers"
)

func TestResolvedPolicyAllowsMutationExceptDeclaredInfrastructure(t *testing.T) {
	root := t.TempDir()
	put(t, root, "coverage-tiers.json", checkout)
	for _, path := range []string{"src/features/checkout/total.ts", "src/features/checkout/ui/Checkout.tsx", "src/features/checkout/types.ts", "src/legacy/old.ts"} {
		put(t, root, path, "export const value = 1;\n")
	}
	policy, err := tiers.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{
		"src/features/checkout/total.ts":        true,
		"src/features/checkout/ui/Checkout.tsx": true,
		"src/features/checkout/types.ts":        false,
		"src/legacy/old.ts":                     true,
	} {
		if got := policy.AllowsMutation(path); got != want {
			t.Fatalf("mutation eligibility for %s = %v, want %v", path, got, want)
		}
	}
	inactive, err := tiers.Resolve(t.TempDir())
	if err != nil || !inactive.AllowsMutation("src/old.ts") {
		t.Fatalf("inactive policy changed legacy mutation: %+v, %v", inactive, err)
	}
}
