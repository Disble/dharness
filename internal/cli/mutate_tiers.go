package cli

import (
	"fmt"
	"path/filepath"

	"github.com/Disble/dharness/internal/tiers"
	"github.com/Disble/dharness/internal/tool"
)

// filterInfrastructureScopes drops the mutation scopes the consumer declared
// infrastructure, and returns every other scope exactly as it arrived.
//
// It is one helper rather than one per workflow because both workflows ask the
// same question — the ordinary named paths and the staged ranges — and two
// answers to it would drift. The classification itself is not decided here:
// the consumer owns it in coverage-tiers.json, and this only projects each
// scope onto the repository root so the policy can recognise the file (§05).
//
// Each scope is appended untouched, never rebuilt. Its line and column suffix
// is the author's own argument, and reconstructing ":1:3-1:5" from the parsed
// numbers would widen it back to ":1-1" and mutate lines they excluded (§01).
//
// A scope the policy cannot even be asked about — one that cannot be expressed
// relative to the repository root — fails the run instead of falling through.
// Treating an unreadable scope as eligible would mutate code the consumer may
// have excluded and report it as this run's verdict, which is the one direction
// this filter must never take.
//
// No declared policy keeps every scope: a repository that never classified its
// source mutates exactly what it named, as it did before this existed.
func filterInfrastructureScopes(root, source string, scopes []tool.MutationScope) ([]tool.MutationScope, error) {
	resolution, err := tiers.Resolve(root)
	if err != nil {
		return nil, err
	}

	eligible := make([]tool.MutationScope, 0, len(scopes))
	for _, scope := range scopes {
		absolute := filepath.Join(source, filepath.FromSlash(scope.Path))
		rel, err := filepath.Rel(root, absolute)
		if err != nil {
			return nil, fmt.Errorf("express %s relative to the repository root: %w", scope.Path, err)
		}
		// Stryker is given forward slashes and the policy is declared in them,
		// so the comparison happens in that one spelling rather than in
		// whatever this platform's separator is.
		if resolution.AllowsMutation(filepath.ToSlash(rel)) {
			eligible = append(eligible, scope)
		}
	}
	return eligible, nil
}
