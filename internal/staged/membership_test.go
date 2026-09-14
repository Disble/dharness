package staged

// Slice C2 (mutate-staged-v1.9): per-range membership classification over raw
// discovery. RED first — these tests fail until
// internal/staged/membership.go exists.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/tool"
)

func scopesForMembership(path string, ranges ...[2]int) []tool.MutationScope {
	var out []tool.MutationScope
	for _, r := range ranges {
		out = append(out, tool.MutationScope{Path: path, Start: r[0], End: r[1]})
	}
	return out
}

func rawMutant(id, startLine, endLine int) RawMutant {
	return RawMutant{ID: id, StartLine: startLine, StartCol: 1, EndLine: endLine, EndCol: 5}
}

func TestMembershipRetainsOnlyRepresentedRanges(t *testing.T) {
	scopes := append(
		scopesForMembership("src/a.ts", [2]int{1, 5}, [2]int{10, 15}),
		scopesForMembership("src/b.ts", [2]int{1, 3})...,
	)
	outcome := DiscoveryOutcome{
		Ranged: map[string][]RawMutant{
			"src/a.ts": {rawMutant(0, 2, 2)},
			"src/b.ts": {rawMutant(1, 40, 41)},
		},
		PathOnly: map[string][]RawMutant{},
	}
	got := ClassifyMembership(scopes, outcome)
	wantRetained := scopesForMembership("src/a.ts", [2]int{1, 5})
	if !reflect.DeepEqual(got.Retained, wantRetained) {
		t.Errorf("Retained = %+v, want %+v", got.Retained, wantRetained)
	}
	if len(got.BothOmitted) != 0 {
		t.Errorf("BothOmitted = %v, want none: src/b.ts was reported", got.BothOmitted)
	}
	if len(got.InSetZero) != 1 || got.InSetZero[0] != "src/b.ts" {
		t.Errorf("InSetZero = %v, want [src/b.ts]: ranged proved the set", got.InSetZero)
	}
}

func TestMembershipPathOnlyHitIsInSetZero(t *testing.T) {
	scopes := scopesForMembership("src/c.ts", [2]int{1, 4})
	outcome := DiscoveryOutcome{
		Ranged:   map[string][]RawMutant{},
		PathOnly: map[string][]RawMutant{"src/c.ts": {rawMutant(0, 30, 30)}},
	}
	got := ClassifyMembership(scopes, outcome)
	if len(got.Retained) != 0 {
		t.Errorf("Retained = %+v, want none", got.Retained)
	}
	if len(got.InSetZero) != 1 || got.InSetZero[0] != "src/c.ts" {
		t.Errorf("InSetZero = %v, want [src/c.ts]", got.InSetZero)
	}
	if got := InSetZeroLine("src/c.ts"); got != "in the set, 0 mutants in the staged lines: src/c.ts" {
		t.Errorf("InSetZeroLine = %q", got)
	}
}

func TestMembershipDoubleOmissionIsBothOmitted(t *testing.T) {
	scopes := scopesForMembership("src/d.ts", [2]int{1, 4})
	outcome := DiscoveryOutcome{Ranged: map[string][]RawMutant{}, PathOnly: map[string][]RawMutant{}}
	got := ClassifyMembership(scopes, outcome)
	if len(got.Retained) != 0 || len(got.InSetZero) != 0 {
		t.Errorf("got = %+v, want only BothOmitted", got)
	}
	if len(got.BothOmitted) != 1 || got.BothOmitted[0] != "src/d.ts" {
		t.Errorf("BothOmitted = %v, want [src/d.ts]", got.BothOmitted)
	}
	line := OutsideSetLine("src/d.ts")
	if line != "outside Stryker's mutate set: src/d.ts" {
		t.Errorf("OutsideSetLine = %q", line)
	}
	for _, cause := range []string{"exclu", "configur", "zero-mutant", "zero mutant"} {
		if strings.Contains(line, cause) {
			t.Errorf("OutsideSetLine = %q, names a cause %q; the label must stay cause-free", line, cause)
		}
	}
}

func TestMembershipOutputsAreSortedAndStable(t *testing.T) {
	scopes := append(
		scopesForMembership("src/z.ts", [2]int{1, 2}),
		scopesForMembership("src/a.ts", [2]int{1, 2}, [2]int{5, 6})...,
	)
	outcome := DiscoveryOutcome{
		Ranged: map[string][]RawMutant{
			"src/a.ts": {rawMutant(0, 5, 5)},
		},
		PathOnly: map[string][]RawMutant{},
	}
	got := ClassifyMembership(scopes, outcome)
	if len(got.Retained) != 1 || got.Retained[0].Path != "src/a.ts" || got.Retained[0].Start != 5 {
		t.Errorf("Retained = %+v, want only src/a.ts:5-6", got.Retained)
	}
	if len(got.BothOmitted) != 1 || got.BothOmitted[0] != "src/z.ts" {
		t.Errorf("BothOmitted = %v, want [src/z.ts]", got.BothOmitted)
	}
}
