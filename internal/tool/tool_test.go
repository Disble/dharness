package tool

import (
	"slices"
	"testing"
)

// TestFallowConfigPathAndJSONArgs pins the exact two argument lists
// design.md Decision 5 measured against the reference project: local fallow
// binary ~350 ms, 26 top-level keys, and the `loaded config: <path>`
// preamble on stderr only, so stdout needs no stripping. A mutant altering
// either flag string dies here.
func TestFallowConfigPathAndJSONArgs(t *testing.T) {
	if got := FallowConfigPath(); !slices.Equal(got, []string{"config", "--path"}) {
		t.Errorf("FallowConfigPath() = %v, want [config --path]", got)
	}
	if got := FallowConfigJSON(); !slices.Equal(got, []string{"config", "--format", "json"}) {
		t.Errorf("FallowConfigJSON() = %v, want [config --format json]", got)
	}
}
