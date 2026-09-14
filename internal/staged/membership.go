package staged

import (
	"sort"

	"github.com/Disble/dharness/internal/tool"
)

// Slice C2 (mutate-staged-v1.9): per-range membership classification.
//
// Each staged runtime range is an independent membership unit: a range is
// retained only when Stryker's ranged discovery reports a mutant overlapping
// its staged lines. The retained values keep the original index-derived
// ranges — MSP is an intersection, never a source of broader ranges — sorted
// by path for stable output.
//
// A file with no retained range falls into exactly one honest bucket:
//   - InSetZero: the server proved the file is in the set, either through a
//     path-only hit or through ranged mutants outside the staged lines, but
//     no mutant lies in the staged lines themselves;
//   - BothOmitted: ranged and path-only discovery both omit the file, which
//     cannot distinguish a configured exclusion from whole-file zero mutants,
//     so the fixed label names the observation and no cause.
type Membership struct {
	Retained    []tool.MutationScope
	InSetZero   []string
	BothOmitted []string
}

// OutsideSetLine is the fixed both-omission label: cause-free by contract.
func OutsideSetLine(path string) string {
	return "outside Stryker's mutate set: " + path
}

// InSetZeroLine is the fixed in-set zero-staged-mutants label.
func InSetZeroLine(path string) string {
	return "in the set, 0 mutants in the staged lines: " + path
}

func mutantOverlapsScope(m RawMutant, scope tool.MutationScope) bool {
	return m.StartLine <= scope.End && m.EndLine >= scope.Start
}

// ClassifyMembership groups scopes by file over a raw discovery outcome.
// Outcome paths outside the scoped files are ignored here: the discovery
// client already refuses them at the transport, so by the time they reach
// classification they can only be duplicates of what was asked.
func ClassifyMembership(scopes []tool.MutationScope, outcome DiscoveryOutcome) Membership {
	var membership Membership
	seen := map[string]bool{}
	for _, scope := range scopes {
		if seen[scope.Path] {
			continue
		}
		seen[scope.Path] = true
		retained := false
		for _, s := range scopes {
			if s.Path != scope.Path {
				continue
			}
			for _, m := range outcome.Ranged[scope.Path] {
				if mutantOverlapsScope(m, s) {
					membership.Retained = append(membership.Retained, s)
					retained = true
					break
				}
			}
		}
		if retained {
			continue
		}
		if _, ok := outcome.PathOnly[scope.Path]; ok {
			membership.InSetZero = append(membership.InSetZero, scope.Path)
			continue
		}
		if len(outcome.Ranged[scope.Path]) > 0 {
			// Reported but nothing in the staged lines: the server proved
			// the file is in the set, so no path-only round trip is owed.
			membership.InSetZero = append(membership.InSetZero, scope.Path)
			continue
		}
		membership.BothOmitted = append(membership.BothOmitted, scope.Path)
	}
	sort.Slice(membership.Retained, func(i, j int) bool {
		if membership.Retained[i].Path != membership.Retained[j].Path {
			return membership.Retained[i].Path < membership.Retained[j].Path
		}
		return membership.Retained[i].Start < membership.Retained[j].Start
	})
	sort.Strings(membership.InSetZero)
	sort.Strings(membership.BothOmitted)
	return membership
}
