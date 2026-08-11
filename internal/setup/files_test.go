package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeclaredKeysFindsAQuotedKey(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, fallowConfig)
	if err := os.WriteFile(path, []byte(`{"ignorePatterns": ["wailsjs/**"]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	got := declaredKeys(path, []string{"ignorePatterns", "boundaries"})
	if len(got) != 1 || got[0] != "ignorePatterns" {
		t.Errorf("declaredKeys() = %v, want exactly [ignorePatterns] — boundaries is absent from the file", got)
	}
}

// TestDeclaredKeysIgnoresACommentedKey pins the same honest limit
// declaresBoundaries already documented, against the exact comment shape the
// motivating repository's own config carries: a sentence that mentions the
// bare word, no quoted key anywhere.
func TestDeclaredKeysIgnoresACommentedKey(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, fallowConfig)
	contents := "{\n  // Architecture boundaries live in the file dharness owns.\n  \"extends\": [\"./.dharness/fallow.jsonc\"]\n}"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	got := declaredKeys(path, []string{"boundaries"})
	if len(got) != 0 {
		t.Errorf("declaredKeys() = %v, want none — the bare word inside a comment is not a declaration", got)
	}
}

// TestDeclaresBoundariesIsNowDeclaredKeys is a grep-style test, not a
// runtime assertion: declaresBoundaries must no longer exist as a symbol
// anywhere in this package, so declaredKeys is the one mechanism behind
// every quoted-key check rather than a second, divergent test living
// alongside it.
func TestDeclaresBoundariesIsNowDeclaredKeys(t *testing.T) {
	for _, name := range []string{"files.go", "steps.go"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "declaresBoundaries") {
			t.Errorf("%s still references declaresBoundaries; declaredKeys should have replaced it everywhere", name)
		}
	}
}

// TestDeclaredKeysReadsTheJSONCSpelling pins design decision 5's stated
// widening: fallowConfigPath must also find .fallowrc.jsonc, not only
// .fallowrc.json, matching fallow's own fallowConfigFiles list.
func TestDeclaredKeysReadsTheJSONCSpelling(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".fallowrc.jsonc")
	if err := os.WriteFile(path, []byte("{\n  // comment\n  \"ignorePatterns\": []\n}"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := fallowConfigPath(root)
	if got != path {
		t.Fatalf("fallowConfigPath() = %q, want %q", got, path)
	}
	if found := declaredKeys(got, []string{"ignorePatterns"}); len(found) != 1 {
		t.Errorf("declaredKeys() = %v, want [ignorePatterns] read from the .jsonc spelling", found)
	}
}

// TestFallowConfigPathPrefersJSONOverJSONC pins the "which one responds"
// order fallowConfigCandidates fixes, mirroring hookManager's own precedent
// for the same kind of question.
func TestFallowConfigPathPrefersJSONOverJSONC(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, fallowConfig), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".fallowrc.jsonc"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got, want := fallowConfigPath(root), filepath.Join(root, fallowConfig); got != want {
		t.Errorf("fallowConfigPath() = %q, want %q", got, want)
	}
}

// TestFallowConfigPathIsEmptyWithoutEitherSpelling matches declaredKeys's own
// "a file that cannot be read declares nothing": fallowConfigPath must not
// invent a path that does not exist.
func TestFallowConfigPathIsEmptyWithoutEitherSpelling(t *testing.T) {
	root := t.TempDir()
	if got := fallowConfigPath(root); got != "" {
		t.Errorf("fallowConfigPath() = %q, want \"\" with neither file present", got)
	}
}

// TestDeclaredLineShowsTheOpeningLineOnly pins declaredLine's stated honest
// limit: it is a grep for the declaring line, not a value parser.
func TestDeclaredLineShowsTheOpeningLineOnly(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, fallowConfig)
	if err := os.WriteFile(path, []byte("{\n  \"ignorePatterns\": [\n    \"wailsjs/**\"\n  ]\n}"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := declaredLine(path, "ignorePatterns")
	if got != `"ignorePatterns": [` {
		t.Errorf("declaredLine() = %q, want the opening line trimmed", got)
	}
}

// TestDeclaredLineIsEmptyWhenTheKeyIsAbsent covers declaredLine's other
// branch: no line contains the quoted key at all.
func TestDeclaredLineIsEmptyWhenTheKeyIsAbsent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, fallowConfig)
	if err := os.WriteFile(path, []byte(`{"extends": ["./.dharness/fallow.jsonc"]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := declaredLine(path, "ignorePatterns"); got != "" {
		t.Errorf("declaredLine() = %q, want \"\" for a key the file never declares", got)
	}
}
