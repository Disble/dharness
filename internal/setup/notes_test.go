package setup

import (
	"slices"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/report"
)

// TestWithdrawnLayerNoteNamesEveryLayerAndThePlugin is what keeps the
// withdrawal from being the silent downgrade it would otherwise be. A
// project can lose most of a rule set here, and a run that dropped two
// layers and printed a clean `0 failed` would be indistinguishable from one
// that contributed them.
func TestWithdrawnLayerNoteNamesEveryLayerAndThePlugin(t *testing.T) {
	eslintPrintConfigStub(t, "@", "react-doctor:react-doctor@0.7.4")
	root := t.TempDir()
	writeGoldenFixtureFile(t, root, "package.json", `{"dependencies":{"expo":"~51.0.0"}}`)
	writeGoldenFixtureFile(t, root, "package-lock.json", `{"lockfileVersion":3}`)
	writeGoldenFixtureFile(t, root, "eslint.config.js", "import dlinter from \"dlinter-ts-react\";\nexport default [...dlinter];\n")
	writeLocalESLintBinary(t, root)
	p := project.Project{Root: root, Source: root}

	entries, reason := WithdrawnLayerNote(p)

	if len(entries) != 2 {
		t.Fatalf("WithdrawnLayerNote() entries = %v, want both react-doctor layers named", entries)
	}
	for _, want := range []string{"configs.recommended", "configs.react-native"} {
		if !slices.ContainsFunc(entries, func(entry string) bool { return strings.Contains(entry, want) }) {
			t.Errorf("WithdrawnLayerNote() entries = %v, want one naming %s", entries, want)
		}
	}
	if !strings.Contains(reason, `"react-doctor"`) {
		t.Errorf("WithdrawnLayerNote() reason = %q, want it to name the plugin key", reason)
	}
	if !strings.Contains(reason, "--print-config") {
		t.Errorf("WithdrawnLayerNote() reason = %q, want it to name how to compare the two builds", reason)
	}
}

// TestWithdrawnLayerNoteIsSilentWhenNothingWasWithdrawn holds the other
// side: a note that appeared on every ordinary project would be noise, and
// noise is how a real one gets skipped.
func TestWithdrawnLayerNoteIsSilentWhenNothingWasWithdrawn(t *testing.T) {
	eslintPrintConfigStub(t, "@", "react")
	root := t.TempDir()
	writeGoldenFixtureFile(t, root, "package.json", `{"dependencies":{"expo":"~51.0.0"}}`)
	writeGoldenFixtureFile(t, root, "package-lock.json", `{"lockfileVersion":3}`)
	writeGoldenFixtureFile(t, root, "eslint.config.js", "export default [];\n")
	writeLocalESLintBinary(t, root)
	p := project.Project{Root: root, Source: root}

	if entries, reason := WithdrawnLayerNote(p); len(entries) != 0 || reason != "" {
		t.Errorf("WithdrawnLayerNote() = %v, %q, want nothing when nothing was withdrawn", entries, reason)
	}
}

// TestWithdrawnLayerNoteNeedsAJSProject is the guard every note function
// here shares: a repository with no JS project has no ESLint config to
// resolve and nothing to say about one.
func TestWithdrawnLayerNoteNeedsAJSProject(t *testing.T) {
	if entries, reason := WithdrawnLayerNote(project.Project{Root: t.TempDir()}); len(entries) != 0 || reason != "" {
		t.Errorf("WithdrawnLayerNote() = %v, %q, want nothing without a JS project", entries, reason)
	}
}

// TestCollectNotesCarriesTheWithdrawal wires the note into the report the
// user actually reads. A note function nothing calls is a note nobody sees,
// which is the same silence with extra steps.
func TestCollectNotesCarriesTheWithdrawal(t *testing.T) {
	eslintPrintConfigStub(t, "@", "react-doctor:react-doctor@0.7.4")
	root := t.TempDir()
	writeGoldenFixtureFile(t, root, "package.json", `{"dependencies":{"expo":"~51.0.0"}}`)
	writeGoldenFixtureFile(t, root, "package-lock.json", `{"lockfileVersion":3}`)
	writeGoldenFixtureFile(t, root, "eslint.config.js", "import dlinter from \"dlinter-ts-react\";\nexport default [...dlinter];\n")
	writeLocalESLintBinary(t, root)

	notes := collectNotes(project.Project{Root: root, Source: root})
	var withdrawn *report.Note
	for i := range notes {
		if notes[i].Kind == "withdrawn" {
			withdrawn = &notes[i]
		}
	}
	if withdrawn == nil {
		t.Fatal("collectNotes() carries no withdrawn note")
	}
	if !withdrawn.Actionable {
		t.Error("collectNotes() withdrawn note is not actionable; the project has a decision to make")
	}
	if len(withdrawn.Entries) != 2 {
		t.Errorf("collectNotes() withdrawn note entries = %v, want both layers", withdrawn.Entries)
	}
}

// TestCollectNotesIsSilentWithoutAWithdrawal is the guard on the note's own
// trigger. A withdrawn note on every ordinary project is noise, and noise is
// how the one that matters gets skipped.
func TestCollectNotesIsSilentWithoutAWithdrawal(t *testing.T) {
	eslintPrintConfigStub(t, "@", "react")
	root := t.TempDir()
	writeGoldenFixtureFile(t, root, "package.json", `{"dependencies":{"expo":"~51.0.0"}}`)
	writeGoldenFixtureFile(t, root, "package-lock.json", `{"lockfileVersion":3}`)
	writeGoldenFixtureFile(t, root, "eslint.config.js", "export default [];\n")
	writeLocalESLintBinary(t, root)

	for _, note := range collectNotes(project.Project{Root: root, Source: root}) {
		if note.Kind == "withdrawn" {
			t.Errorf("collectNotes() carries a withdrawn note with nothing withdrawn: %+v", note)
		}
	}
}

// TestWithdrawnLayerNoteNamesOnlyWhatThePluginRuleDropped separates the two
// reasons a layer does not get contributed. A layer the project's own config
// already imports is dropped by specifier and is not news — it is the
// project having done the wiring itself. Only a layer withheld because its
// plugin is already registered costs the project rules it might have wanted,
// and only that one belongs in the note.
func TestWithdrawnLayerNoteNamesOnlyWhatThePluginRuleDropped(t *testing.T) {
	eslintPrintConfigStub(t, "@", "react-doctor:react-doctor@0.7.4")
	root := t.TempDir()
	writeGoldenFixtureFile(t, root, "package.json", `{"dependencies":{"expo":"~51.0.0"}}`)
	writeGoldenFixtureFile(t, root, "package-lock.json", `{"lockfileVersion":3}`)
	// The project spreads Expo's own config itself, so that layer is dropped
	// by the specifier rule rather than by the plugin rule.
	writeGoldenFixtureFile(t, root, "eslint.config.js",
		"import expo from \"eslint-config-expo/flat\";\nimport dlinter from \"dlinter-ts-react\";\nexport default [...expo, ...dlinter];\n")
	writeLocalESLintBinary(t, root)

	entries, _ := WithdrawnLayerNote(project.Project{Root: root, Source: root})

	if len(entries) != 2 {
		t.Fatalf("WithdrawnLayerNote() entries = %v, want only the two react-doctor layers", entries)
	}
	for _, entry := range entries {
		if strings.Contains(entry, "eslint-config-expo") {
			t.Errorf("WithdrawnLayerNote() named %q, which the project already imports itself", entry)
		}
	}
}
