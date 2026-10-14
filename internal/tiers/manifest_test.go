package tiers_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/tiers"
)

const checkout = `{
  "version": 1,
  "enforcedPrefixes": ["src/features/"],
  "features": [{
    "id": "checkout", "path": "src/features/checkout/", "defaultTier": "core",
    "reason": "Checkout decisions",
    "overrides": [
      {"path": "src/features/checkout/ui/", "tier": "important", "reason": "Presentation"},
      {"path": "src/features/checkout/types.ts", "tier": "infrastructure", "reason": "Types"}
    ]
  }]
}`

func TestManifestAcceptsConsumerFeatureDeclaration(t *testing.T) {
	got, err := tiers.Parse([]byte(checkout))
	want := tiers.Manifest{
		Version: 1, EnforcedPrefixes: []string{"src/features/"},
		Features: []tiers.Feature{{
			ID: "checkout", Path: "src/features/checkout/", DefaultTier: "core", Reason: "Checkout decisions",
			Overrides: []tiers.Rule{
				{Path: "src/features/checkout/ui/", Tier: "important", Reason: "Presentation"},
				{Path: "src/features/checkout/types.ts", Tier: "infrastructure", Reason: "Types"},
			},
		}},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("declaration = %+v, error = %v; want %+v", got, err, want)
	}
	t.Run("membership is not lost to an unrelated later prefix", func(t *testing.T) {
		input := strings.Replace(checkout, `["src/features/"]`, `["src/features/", "legacy/"]`, 1)
		want.EnforcedPrefixes = []string{"src/features/", "legacy/"}
		got, err := tiers.Parse([]byte(input))
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("declaration = %+v, error = %v; want %+v", got, err, want)
		}
	})
	t.Run("exact file adoption and override", func(t *testing.T) {
		input := `{"version":1,"enforcedPrefixes":["src/order.ts"],"features":[{"id":"order","path":"src/order.ts","defaultTier":"core","reason":"Order","overrides":[{"path":"src/order.ts","tier":"important","reason":"Presentation"}]}]}`
		want := tiers.Manifest{Version: 1, EnforcedPrefixes: []string{"src/order.ts"}, Features: []tiers.Feature{{ID: "order", Path: "src/order.ts", DefaultTier: "core", Reason: "Order", Overrides: []tiers.Rule{{Path: "src/order.ts", Tier: "important", Reason: "Presentation"}}}}}
		got, err := tiers.Parse([]byte(input))
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("declaration = %+v, error = %v; want %+v", got, err, want)
		}
	})
	t.Run("features are ordered by identity", func(t *testing.T) {
		input := `{"version":1,"enforcedPrefixes":["src/"],"features":[{"id":"b","path":"src/b/","defaultTier":"core","reason":"B"},{"id":"a","path":"src/a/","defaultTier":"important","reason":"A"}]}`
		want := tiers.Manifest{Version: 1, EnforcedPrefixes: []string{"src/"}, Features: []tiers.Feature{{ID: "a", Path: "src/a/", DefaultTier: "important", Reason: "A"}, {ID: "b", Path: "src/b/", DefaultTier: "core", Reason: "B"}}}
		got, err := tiers.Parse([]byte(input))
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("declaration = %+v, error = %v; want %+v", got, err, want)
		}
	})
}

func TestManifestRejectsInvalidDeclarationsWithoutChangingInput(t *testing.T) {
	cases := []struct{ name, input, want string }{
		{"malformed", `{`, "coverage-tiers.json: unexpected EOF"},
		{"version", strings.Replace(checkout, `"version": 1`, `"version": 2`, 1), "coverage-tiers.json: version must be 1"},
		{"unknown field", strings.Replace(checkout, `"version": 1`, `"version": 1, "typo": true`, 1), `coverage-tiers.json: json: unknown field "typo"`},
		{"unknown tier", strings.Replace(checkout, `"defaultTier": "core"`, `"defaultTier": "other"`, 1), `coverage-tiers.json: unknown tier "other"; use core, important or infrastructure`},
		{"escaping feature", strings.Replace(checkout, `"path": "src/features/checkout/"`, `"path": "../outside/"`, 1), `coverage-tiers.json: path "../outside/" must be canonical and repository-relative`},
		{"escaping override", strings.Replace(checkout, `"path": "src/features/checkout/ui/"`, `"path": "src/other/"`, 1), `coverage-tiers.json: override "src/other/" is outside feature "checkout"`},
		{"empty adoption", `{"version":1,"enforcedPrefixes":[],"features":[]}`, "coverage-tiers.json: declare at least one enforced prefix and feature"},
		{"no prefixes", strings.Replace(checkout, `["src/features/"]`, `[]`, 1), "coverage-tiers.json: declare at least one enforced prefix and feature"},
		{"no features", `{"version":1,"enforcedPrefixes":["src/"],"features":[]}`, "coverage-tiers.json: declare at least one enforced prefix and feature"},
		{"exact prefix does not adopt a sibling", `{"version":1,"enforcedPrefixes":["src/order.ts"],"features":[{"id":"other","path":"src/other.ts","defaultTier":"core","reason":"Other"}]}`, `coverage-tiers.json: feature "other" is outside the enforced prefixes`},
		{"reverse parent overlap", `{"version":1,"enforcedPrefixes":["src/"],"features":[{"id":"a","path":"src/cart/child/","defaultTier":"core","reason":"Child"},{"id":"b","path":"src/cart/","defaultTier":"important","reason":"Parent"}]}`, `coverage-tiers.json: features "a" and "b" overlap`},
		{"missing feature id", strings.Replace(checkout, `"id": "checkout"`, `"id": ""`, 1), "coverage-tiers.json: each feature must declare an id and reason"},
		{"missing feature reason", strings.Replace(checkout, `"reason": "Checkout decisions"`, `"reason": " "`, 1), "coverage-tiers.json: each feature must declare an id and reason"},
		{"missing override reason", strings.Replace(checkout, `"reason": "Presentation"`, `"reason": " "`, 1), `coverage-tiers.json: override "src/features/checkout/ui/" must declare a reason`},
		{"unknown override tier", strings.Replace(checkout, `"tier": "important"`, `"tier": "other"`, 1), `coverage-tiers.json: unknown tier "other"; use core, important or infrastructure`},
		{"feature outside adoption", strings.Replace(checkout, `"enforcedPrefixes": ["src/features/"]`, `"enforcedPrefixes": ["src/other/"]`, 1), `coverage-tiers.json: feature "checkout" is outside the enforced prefixes`},
		{"duplicate feature id", `{"version":1,"enforcedPrefixes":["src/"],"features":[{"id":"a","path":"src/a/","defaultTier":"core","reason":"A"},{"id":"a","path":"src/b/","defaultTier":"core","reason":"B"}]}`, `coverage-tiers.json: duplicate feature id "a"`},
		{"second JSON object", checkout + `{}`, "coverage-tiers.json: declare exactly one JSON object"},
		{"overlap", `{"version":1,"enforcedPrefixes":["src/"],"features":[{"id":"a","path":"src/a/","defaultTier":"core","reason":"A"},{"id":"b","path":"src/a/b/","defaultTier":"important","reason":"B"}]}`, `coverage-tiers.json: features "a" and "b" overlap`},
		{"duplicate override", `{"version":1,"enforcedPrefixes":["src/"],"features":[{"id":"a","path":"src/a/","defaultTier":"core","reason":"A","overrides":[{"path":"src/a/x.ts","tier":"important","reason":"UI"},{"path":"src/a/x.ts","tier":"infrastructure","reason":"Types"}]}]}`, `coverage-tiers.json: feature "a" repeats override "src/a/x.ts"`},
	}

	// Canonical repository-relative adoption is the rule, regardless of which
	// invalid path form or prefix position supplied it.
	for _, path := range []string{"", ".", "..", "../outside/", "/outside/", "C:/outside/", `src\outside/`, "src//outside/"} {
		encoded, err := json.Marshal(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, prefixes := range []string{string(encoded), `"src/features/",` + string(encoded)} {
			cases = append(cases, struct{ name, input, want string }{
				"invalid prefix " + prefixes,
				strings.Replace(checkout, `["src/features/"]`, `[`+prefixes+`]`, 1),
				fmt.Sprintf("coverage-tiers.json: path %q must be canonical and repository-relative", path),
			})
		}
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := []byte(tc.input)
			got, err := tiers.Parse(input)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("declaration = %+v, error = %v, want %s", got, err, tc.want)
			}
			if !reflect.DeepEqual(got, tiers.Manifest{}) {
				t.Fatalf("rejected input returned a usable declaration: %+v", got)
			}
			if string(input) != tc.input {
				t.Fatal("parser changed caller input")
			}
		})
	}
}
