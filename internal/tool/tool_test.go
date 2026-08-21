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

// TestFallowAuditPinsTheBaseAndTheScope pins the two flags that make audit
// answer about the staged change rather than about the branch, and the
// measurement behind each — see FallowAudit for the full record.
//
// Measured against fallow 3.16.0 on a branch of 24 changed files with one
// clean file staged: bare `audit` reported "Audit scope: 25 changed files vs
// main (local main)" and exited 1 on 24 complexity findings and 8 clone
// groups, none of them in the staged file. Adding both flags took the same
// repository to exit 0, while a deliberately bad staged file still exited 1
// with exactly one finding and one clone group.
//
// A mutant dropping either flag dies here, and they are separate assertions
// because they fail differently: without --changed-since the base returns to
// the merge-base and the branch's inherited debt blocks the commit; without
// --diff-stdin the scope reaches files that were never staged.
func TestFallowAuditPinsTheBaseAndTheScope(t *testing.T) {
	got := FallowAudit()

	if len(got) == 0 || got[0] != "audit" {
		t.Fatalf("FallowAudit() = %v, want the audit subcommand first", got)
	}
	base := slices.Index(got, "--changed-since")
	if base < 0 || base+1 >= len(got) || got[base+1] != "HEAD" {
		t.Errorf("FallowAudit() = %v, want --changed-since HEAD; without it the base is the merge-base and the whole branch is audited", got)
	}
	if !slices.Contains(got, "--diff-stdin") {
		t.Errorf("FallowAudit() = %v, want --diff-stdin; without it the scope reaches files that were never staged", got)
	}
	// No --gate. new-only is already the default, and restating a tool's own
	// default is the noise every adopting repository then has to maintain.
	for _, arg := range got {
		if arg == "--gate" {
			t.Errorf("FallowAudit() carries --gate: new-only is already fallow's default, so this restates it: %v", got)
		}
	}
}

// dupes stays a whole-repository question. It exposes the same --changed-since
// and --diff-stdin flags audit does, and it deliberately takes neither: the
// duplication ceiling is a wall rather than a ratchet, which is the only
// reason it is a separate invocation instead of a flag on audit.
func TestFallowDupesStaysWholeRepository(t *testing.T) {
	got := FallowDupes()

	if !slices.Equal(got, []string{"dupes"}) {
		t.Errorf("FallowDupes() = %v, want [dupes]", got)
	}
}

// TestESLintStagedSuppressesTheIgnoredFileWarning covers a warning dharness
// causes and only dharness can answer for.
//
// The gate names every staged file explicitly, and ESLint warns when a file
// it was handed is one its config ignores: "File ignored because of a
// matching ignore pattern." Since the layer dharness writes ignores the
// .dharness/ directory — so its own generated config stops failing its own
// require-jsdoc rule — committing a change to that directory prints the
// warning on the gate, about a list dharness built rather than about
// anything the user did.
//
// --no-warn-ignored is ESLint's own answer, in its own --help: "Suppress
// warnings when the file list includes ignored files". It suppresses nothing
// actionable, because the file list is not the user's to change.
func TestESLintStagedSuppressesTheIgnoredFileWarning(t *testing.T) {
	args := ESLintStaged([]string{"src/a.ts"})

	if !contains(args, "--no-warn-ignored") {
		t.Errorf("ESLintStaged() = %v, want --no-warn-ignored", args)
	}
	if !contains(args, "src/a.ts") {
		t.Errorf("ESLintStaged() = %v, want it to still name the staged file", args)
	}
}
