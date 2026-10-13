package setup

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Disble/dharness/internal/project"
)

// TestDefaultSeverityCompilesOnlyWithAProject is the RED for a signature
// change, proven by the build failing against today's one-argument form
// rather than by a runtime assertion.
func TestDefaultSeverityCompilesOnlyWithAProject(t *testing.T) {
	p := project.Project{}
	_ = DefaultSeverity(p, "folder-ownership")
}

// TestFolderOwnershipIsErrorWhereBarrelsExist pins decision 6: barrel
// presence, asked of git through the project, decides the first-write
// default — not a preset, and not the removed offByDefault map.
func TestFolderOwnershipIsErrorWhereBarrelsExist(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(project.SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		return []byte("components/index.ts"), nil
	}))
	p := project.Project{Root: root, Source: root, InRepository: true}

	if got := DefaultSeverity(p, RulesPrefix+"/folder-ownership"); got != "error" {
		t.Errorf("DefaultSeverity() = %q, want \"error\" where barrels exist", got)
	}
}

func TestFolderOwnershipIsOffWithoutBarrels(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(project.SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		return nil, nil
	}))
	p := project.Project{Root: root, Source: root, InRepository: true}

	if got := DefaultSeverity(p, RulesPrefix+"/folder-ownership"); got != "off" {
		t.Errorf("DefaultSeverity() = %q, want \"off\" without barrels", got)
	}
}

// TestDefaultThresholdsCountCodeOnlyByDefault pins the counting policy every
// project starts with: a file ceiling that counts code, not the comments and
// blank lines around it. Both flags are strict booleans, so an explicit false
// is what a project receives and what it can leave in place.
func TestDefaultThresholdsCountCodeOnlyByDefault(t *testing.T) {
	defaults := DefaultThresholds()

	if defaults.CountComments || defaults.CountBlankLines {
		t.Errorf("DefaultThresholds() counts comments=%t blanks=%t, want both false",
			defaults.CountComments, defaults.CountBlankLines)
	}
	if defaults.MaxFileLines != 500 {
		t.Errorf("DefaultThresholds().MaxFileLines = %g, want 500", defaults.MaxFileLines)
	}
	if defaults.Schema != "dharness.rules/v1" {
		t.Errorf("DefaultThresholds().Schema = %q, want dharness.rules/v1", defaults.Schema)
	}
}

// TestRenderThresholdsMatchesTheStableDocumentShape pins the exact bytes
// dharness writes and compares against — an independent literal, not a
// recomputation of the renderer — so a change to key order, indentation or
// the trailing newline fails here rather than as an unexplained golden diff.
func TestRenderThresholdsMatchesTheStableDocumentShape(t *testing.T) {
	want := "{\n" +
		"  \"schema\": \"dharness.rules/v1\",\n" +
		"  \"maxFileLines\": 500,\n" +
		"  \"roleSuffixes\": [\n" +
		"    \".types.ts\",\n" +
		"    \".constants.ts\",\n" +
		"    \".helpers.ts\",\n" +
		"    \".schema.ts\"\n" +
		"  ],\n" +
		"  \"countComments\": false,\n" +
		"  \"countBlankLines\": false\n" +
		"}\n"

	if got := string(renderThresholds(DefaultThresholds(), nil)); got != want {
		t.Errorf("renderThresholds() = %q, want %q", got, want)
	}
}

// defaultRoleSuffixes is the role list both this table and DefaultThresholds
// answer with, named once so a case that does not change it stays readable.
var defaultRoleSuffixes = []string{".types.ts", ".constants.ts", ".helpers.ts", ".schema.ts"}

// TestNormalizeThresholdsValidatesEachFieldOnItsOwn is the contract with the
// plugin's own src/thresholds.ts: one bad value falls back to its default and
// costs the project nothing else it decided, and a missing flag on a file
// written before the flags existed is filled in rather than treated as a
// decision.
func TestNormalizeThresholdsValidatesEachFieldOnItsOwn(t *testing.T) {
	cases := []struct {
		name         string
		raw          string
		wantMax      float64
		wantRoles    []string
		wantComments bool
		wantBlanks   bool
	}{
		{
			name:      "a file written before the counting flags existed keeps its numbers",
			raw:       `{"schema":"dharness.rules/v1","maxFileLines":300,"roleSuffixes":[".ts"]}`,
			wantMax:   300,
			wantRoles: []string{".ts"},
		},
		{
			name:         "both flags true are kept",
			raw:          `{"maxFileLines":300,"roleSuffixes":[".ts"],"countComments":true,"countBlankLines":true}`,
			wantMax:      300,
			wantRoles:    []string{".ts"},
			wantComments: true,
			wantBlanks:   true,
		},
		{
			name:      "an explicit false is a decision too",
			raw:       `{"countComments":false,"countBlankLines":false}`,
			wantMax:   500,
			wantRoles: defaultRoleSuffixes,
		},
		{
			name:         "one bad flag costs only that flag",
			raw:          `{"maxFileLines":-1,"roleSuffixes":[".ts"],"countComments":true}`,
			wantMax:      500,
			wantRoles:    []string{".ts"},
			wantComments: true,
		},
		{
			name:      "a fractional ceiling is a typo",
			raw:       `{"maxFileLines":12.5,"roleSuffixes":[".ts"]}`,
			wantMax:   500,
			wantRoles: []string{".ts"},
		},
		{
			name:      "the smallest valid ceiling is a decision",
			raw:       `{"maxFileLines":1,"roleSuffixes":[".ts"]}`,
			wantMax:   1,
			wantRoles: []string{".ts"},
		},
		{
			name:      "a zero ceiling is a typo, not a decision",
			raw:       `{"maxFileLines":0}`,
			wantMax:   500,
			wantRoles: defaultRoleSuffixes,
		},
		{
			name:      "a null ceiling is not a decision",
			raw:       `{"maxFileLines":null}`,
			wantMax:   500,
			wantRoles: defaultRoleSuffixes,
		},
		{
			name:      "a non-numeric ceiling is not a decision",
			raw:       `{"maxFileLines":"500"}`,
			wantMax:   500,
			wantRoles: defaultRoleSuffixes,
		},
		{
			name:      "a ceiling that overflows to Infinity is not a decision",
			raw:       `{"maxFileLines":1e309}`,
			wantMax:   500,
			wantRoles: defaultRoleSuffixes,
		},
		{
			name:      "a ceiling above any 64-bit integer is a decision",
			raw:       `{"maxFileLines":1e19,"roleSuffixes":[".ts"]}`,
			wantMax:   1e19,
			wantRoles: []string{".ts"},
		},
		{
			name:      "a ceiling past the float64 integer precision matches the plugin's own reading",
			raw:       `{"maxFileLines":9007199254740993,"roleSuffixes":[".ts"]}`,
			wantMax:   9007199254740992,
			wantRoles: []string{".ts"},
		},
		{
			name:      "the largest finite float64 ceiling is a decision",
			raw:       `{"maxFileLines":1e308,"roleSuffixes":[".ts"]}`,
			wantMax:   1e308,
			wantRoles: []string{".ts"},
		},
		{
			name:       "one non-string suffix falls back alone",
			raw:        `{"maxFileLines":300,"roleSuffixes":[".ts",1],"countBlankLines":true}`,
			wantMax:    300,
			wantRoles:  defaultRoleSuffixes,
			wantBlanks: true,
		},
		{
			name:      "an empty suffix list is a decision",
			raw:       `{"roleSuffixes":[]}`,
			wantMax:   500,
			wantRoles: []string{},
		},
		{
			name:      "a non-boolean flag is not a decision",
			raw:       `{"countComments":1,"countBlankLines":"true"}`,
			wantMax:   500,
			wantRoles: defaultRoleSuffixes,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			document, repair := normalizeThresholds([]byte(tc.raw))
			if repair != "" {
				t.Fatalf("normalizeThresholds() repair = %q, want none", repair)
			}

			got := decodedThresholds(t, document)
			if got.MaxFileLines != tc.wantMax {
				t.Errorf("maxFileLines = %g, want %g", got.MaxFileLines, tc.wantMax)
			}
			if !slices.Equal(got.RoleSuffixes, tc.wantRoles) {
				t.Errorf("roleSuffixes = %v, want %v", got.RoleSuffixes, tc.wantRoles)
			}
			if got.CountComments != tc.wantComments {
				t.Errorf("countComments = %t, want %t", got.CountComments, tc.wantComments)
			}
			if got.CountBlankLines != tc.wantBlanks {
				t.Errorf("countBlankLines = %t, want %t", got.CountBlankLines, tc.wantBlanks)
			}
		})
	}
}

// TestNormalizeThresholdsRefusesWhatIsNotAnObject pins the preserve-and-
// delegate boundary: a document that is not a JSON object has no field to
// fix and no way to tell which parts were meant, so it is returned with a
// reason instead of a normalized document.
func TestNormalizeThresholdsRefusesWhatIsNotAnObject(t *testing.T) {
	for _, raw := range []string{"", "{", "null", "[]", "5", `"text"`, "true"} {
		t.Run(raw, func(t *testing.T) {
			document, repair := normalizeThresholds([]byte(raw))
			if repair == "" {
				t.Errorf("normalizeThresholds(%q) repair = \"\", want a reason", raw)
			}
			if document != nil {
				t.Errorf("normalizeThresholds(%q) document = %q, want nil", raw, document)
			}
		})
	}
}

// TestReadThresholdsFileGivesAMissingFileTheDefaults pins the adoption case:
// a project that never decided gets the defaults, not an unsatisfiable state.
func TestReadThresholdsFileGivesAMissingFileTheDefaults(t *testing.T) {
	file := readThresholdsFile(filepath.Join(t.TempDir(), ownedRules))

	if file.repair != "" {
		t.Errorf("repair = %q, want none for a missing file", file.repair)
	}
	if file.raw != nil {
		t.Errorf("raw = %q, want nil for a missing file", file.raw)
	}
	if !bytes.Equal(file.document, renderThresholds(DefaultThresholds(), nil)) {
		t.Errorf("document = %q, want the defaults", file.document)
	}
}

// decodedThresholds reads a normalized document back into the values the
// plugin's own reader would answer with.
func decodedThresholds(t *testing.T, document []byte) Thresholds {
	t.Helper()

	var decoded Thresholds
	if err := json.Unmarshal(document, &decoded); err != nil {
		t.Fatalf("the normalized document is not readable as JSON: %v\n%s", err, document)
	}
	return decoded
}
