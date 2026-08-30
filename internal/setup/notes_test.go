package setup

import (
	"os"
	"path/filepath"
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
	// No node_modules in this fixture, so dharness's own build is unknown
	// and the note must not print half a comparison.
	if strings.Contains(reason, "dharness installs") {
		t.Errorf("WithdrawnLayerNote() reason = %q, want no version gap when one side is unknown", reason)
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

// writeInstalledPackage puts a package in node_modules the way a package
// manager would, so installedBuild has a version to read.
func writeInstalledPackage(t *testing.T, source, name, version string) {
	t.Helper()
	dir := filepath.Join(source, "node_modules", filepath.FromSlash(name))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeGoldenFixtureFile(t, dir, "package.json", `{"name":"`+name+`","version":"`+version+`"}`)
}

// TestWithdrawnLayerNoteLeadsWithTheVersionGap is the fact a reader acts on,
// and the reason it leads rather than follows.
//
// Both numbers are in hand when the note is written: the resident build came
// out of the same `plugins` entry the withdrawal was decided from, and the
// other is the package dharness installs. An earlier version described the
// gap in the abstract while holding both, which asks the reader to suspect a
// gap and then go measure it.
func TestWithdrawnLayerNoteLeadsWithTheVersionGap(t *testing.T) {
	eslintPrintConfigStub(t, "@", "react-doctor:react-doctor@0.7.4")
	root := t.TempDir()
	writeGoldenFixtureFile(t, root, "package.json", `{"dependencies":{"expo":"~51.0.0"}}`)
	writeGoldenFixtureFile(t, root, "package-lock.json", `{"lockfileVersion":3}`)
	writeGoldenFixtureFile(t, root, "eslint.config.js", dlinterConfigSource)
	writeLocalESLintBinary(t, root)
	writeInstalledPackage(t, root, "eslint-plugin-react-doctor", "0.9.12")

	_, reason := WithdrawnLayerNote(project.Project{Root: root, Source: root})

	for _, want := range []string{"0.7.4", "0.9.12", "not enforced"} {
		if !strings.Contains(reason, want) {
			t.Errorf("WithdrawnLayerNote() reason = %q, want it to state %s", reason, want)
		}
	}
	if !strings.HasPrefix(reason, "the copy of ") {
		t.Errorf("WithdrawnLayerNote() reason = %q, want the version gap first: it is the sentence worth seeing", reason)
	}
}

// TestWithdrawnLayerNoteSkipsAMatchingBuild keeps the gap sentence honest.
// Two registrations of the same build lose no rules at all, and announcing a
// gap that is not there is how a reader learns to stop reading the note.
func TestWithdrawnLayerNoteSkipsAMatchingBuild(t *testing.T) {
	eslintPrintConfigStub(t, "@", "react-doctor:react-doctor@0.9.12")
	root := t.TempDir()
	writeGoldenFixtureFile(t, root, "package.json", `{"dependencies":{"expo":"~51.0.0"}}`)
	writeGoldenFixtureFile(t, root, "package-lock.json", `{"lockfileVersion":3}`)
	writeGoldenFixtureFile(t, root, "eslint.config.js", dlinterConfigSource)
	writeLocalESLintBinary(t, root)
	writeInstalledPackage(t, root, "eslint-plugin-react-doctor", "0.9.12")

	entries, reason := WithdrawnLayerNote(project.Project{Root: root, Source: root})

	if len(entries) == 0 {
		t.Fatal("WithdrawnLayerNote() withdrew nothing; the fixture registers the plugin")
	}
	if strings.Contains(reason, "not enforced") {
		t.Errorf("WithdrawnLayerNote() reason = %q, want no version gap when the builds match", reason)
	}
}

// dlinterConfigSource is a project config that reaches react-doctor through
// another package, which is the shape every withdrawal test needs.
const dlinterConfigSource = `import dlinter from "dlinter-ts-react";
export default [...dlinter];
`

// TestVersionGapSkipsAnEntryWithNoBuild covers the row where ESLint names a
// plugin without a version. Printing "0.7.4 against " with a blank on the
// other side would be worse than the hedge this replaced, so an entry
// carrying no build is not a gap.
func TestVersionGapSkipsAnEntryWithNoBuild(t *testing.T) {
	root := t.TempDir()
	writeInstalledPackage(t, root, "eslint-plugin-react-doctor", "0.9.12")
	p := project.Project{Root: root, Source: root}

	got := versionGap(p,
		map[string]bool{"react-doctor": true},
		map[string]string{"react-doctor": "eslint-plugin-react-doctor"},
		map[string]string{"react-doctor": "react-doctor"}) // no @version

	if got != "" {
		t.Errorf("versionGap() = %q, want nothing when ESLint reported no build", got)
	}
}

// TestVersionGapConsidersEveryPlugin keeps the loop a filter rather than a
// search. One plugin matching its resident build must not stop the walk
// before a second one that does not — a note that names one gap and hides
// another is the silence this whole mechanism exists to end.
func TestVersionGapConsidersEveryPlugin(t *testing.T) {
	root := t.TempDir()
	writeInstalledPackage(t, root, "eslint-plugin-aaa", "1.0.0")
	writeInstalledPackage(t, root, "eslint-plugin-zzz", "2.0.0")
	p := project.Project{Root: root, Source: root}

	got := versionGap(p,
		map[string]bool{"aaa": true, "zzz": true},
		map[string]string{"aaa": "eslint-plugin-aaa", "zzz": "eslint-plugin-zzz"},
		// "aaa" matches what is installed; "zzz" does not.
		map[string]string{"aaa": "aaa@1.0.0", "zzz": "zzz@1.5.0"})

	if !strings.Contains(got, "1.5.0") || !strings.Contains(got, "2.0.0") {
		t.Errorf("versionGap() = %q, want the second plugin's gap named", got)
	}
	if strings.Contains(got, "1.0.0") {
		t.Errorf("versionGap() = %q, want no gap for the plugin whose build matches", got)
	}
}
