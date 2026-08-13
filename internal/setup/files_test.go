package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/project"
)

// TestExtendsWiredIsFalseWithoutTheFile pins the other half of extendsWired's
// contract: a config that does not exist yet is not wired, distinct from one
// that exists and lacks the reference — both must answer false, but only a
// fixture with no file at all exercises the read-error branch.
func TestExtendsWiredIsFalseWithoutTheFile(t *testing.T) {
	root := t.TempDir()
	if extendsWired(root, "eslint.config.js", ".dharness/eslint.config.js") {
		t.Error("extendsWired() = true for a config that does not exist")
	}
}

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

// TestDeclaredAtReturnsALineNumber pins declaredAt's replacement contract
// for declaredLine (design.md Decision 5): locating a key by a textual scan
// is sound, and stays; showing its value with one was defect 8, and does
// not. Reuses declaredLine's own fixtures — same scan, same lines, now a
// 1-based line number instead of the line's text, and the documented
// sentinel 0 when the key is not declared at all.
func TestDeclaredAtReturnsALineNumber(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, fallowConfig)
	if err := os.WriteFile(path, []byte("{\n  \"ignorePatterns\": [\n    \"wailsjs/**\"\n  ]\n}"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := declaredAt(path, "ignorePatterns"); got != 2 {
		t.Errorf("declaredAt() = %d, want line 2 (1-based)", got)
	}
	if got := declaredAt(path, "notDeclared"); got != 0 {
		t.Errorf("declaredAt() = %d, want the documented sentinel 0 for a key the file never declares", got)
	}
	if got := declaredAt(filepath.Join(root, "missing.json"), "ignorePatterns"); got != 0 {
		t.Errorf("declaredAt() = %d, want the documented sentinel 0 for a file that cannot be read at all", got)
	}
}

// TestExistingAllowListGainsTheMissingEntry pins design decision 2's repair:
// a repository adopted before this change carries the five-entry list, and
// ensureShared must append the sixth rather than leaving it gitignored
// forever.
func TestExistingAllowListGainsTheMissingEntry(t *testing.T) {
	p, path := allowListFixture(t, "*\n!.gitignore\n!lefthook.yml\n!fallow.jsonc\n!rules.json\n!evidence.json\n")

	if err := ensureShared(p, &Writer{}, "eslint.config.js"); err != nil {
		t.Fatalf("ensureShared() = %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// The exact suffix, not just Contains: the existing list already ends in
	// a newline, so the entry must be appended directly after it — not
	// behind a spurious blank line, which is what a mutant that appended
	// unconditionally would produce.
	if !strings.HasSuffix(string(raw), "!evidence.json\n!eslint.config.js\n") {
		t.Errorf("ensureShared() = %q, want the entry appended right after the last existing line, no blank line between", raw)
	}
	if !strings.Contains(string(raw), "!lefthook.yml") {
		t.Errorf("ensureShared() dropped an entry that was already there:\n%s", raw)
	}
}

// TestEnsureSharedCreatesTheIgnoreFileWhenAbsent covers the case
// appendHuskyGate's own "no script yet" test covers for its file: a
// .dharness/.gitignore that does not exist yet at all is not a read error,
// and the entry is written into a fresh file with no leading blank line.
func TestEnsureSharedCreatesTheIgnoreFileWhenAbsent(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root}
	if err := os.MkdirAll(filepath.Join(root, project.Dir), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := ensureShared(p, &Writer{}, "eslint.config.js"); err != nil {
		t.Fatalf("ensureShared() = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, project.Dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "!eslint.config.js\n" {
		t.Errorf("ensureShared() = %q, want exactly the one entry with no leading blank line", raw)
	}
}

// TestExistingAllowListWithoutATrailingNewlineGainsOne covers the branch
// TestExistingAllowListGainsTheMissingEntry never reaches: existing content
// that does not already end in "\n" must gain one before the new entry, or
// the entry welds onto the previous line's last byte.
func TestExistingAllowListWithoutATrailingNewlineGainsOne(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root}
	if err := os.MkdirAll(filepath.Join(root, project.Dir), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, project.Dir, ".gitignore")
	if err := os.WriteFile(path, []byte("*\n!.gitignore"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ensureShared(p, &Writer{}, "eslint.config.js"); err != nil {
		t.Fatalf("ensureShared() = %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "*\n!.gitignore\n!eslint.config.js\n"; string(raw) != want {
		t.Errorf("ensureShared() = %q, want %q", raw, want)
	}
}

// TestAllowListRepairKeepsWhatTheProjectAdded is the no-clobber property
// TestOwnedDirectoryKeepsAnExistingIgnoreFile already pins for EnsureDir,
// applied to ensureShared: appending the sixth entry must not disturb
// anything the project itself added to the file.
func TestAllowListRepairKeepsWhatTheProjectAdded(t *testing.T) {
	p, path := allowListFixture(t, "*\n!.gitignore\n!lefthook.yml\n# added by hand\n!notes.md\n")

	if err := ensureShared(p, &Writer{}, "eslint.config.js"); err != nil {
		t.Fatalf("ensureShared() = %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "!notes.md") || !strings.Contains(string(raw), "# added by hand") {
		t.Errorf("ensureShared() discarded what the project added by hand:\n%s", raw)
	}
}

// TestEnsureSharedIsANoOpWhenTheEntryAlreadyExists triangulates the repair
// path above: an already-current allow list gains no duplicate entry.
func TestEnsureSharedIsANoOpWhenTheEntryAlreadyExists(t *testing.T) {
	p, path := allowListFixture(t, "*\n!.gitignore\n!eslint.config.js\n")

	if err := ensureShared(p, &Writer{}, "eslint.config.js"); err != nil {
		t.Fatalf("ensureShared() = %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(raw), "!eslint.config.js"); got != 1 {
		t.Errorf("ensureShared() left %d occurrences of the entry, want exactly 1", got)
	}
}

// TestEslintConfigDetectionOneFixturePerCase pins the four kinds
// eslintExtendsStep tells apart: flat, TypeScript, legacy-only and absent —
// each detector answers independently, so a project with more than one kind
// present is not this test's concern.
func TestEslintConfigDetectionOneFixturePerCase(t *testing.T) {
	cases := []struct {
		name   string
		file   string
		detect func(string) string
	}{
		{"flat .js", "eslint.config.js", eslintFlatConfig},
		{"flat .mjs", "eslint.config.mjs", eslintFlatConfig},
		{"flat .cjs", "eslint.config.cjs", eslintFlatConfig},
		{"TypeScript .ts", "eslint.config.ts", eslintTypeScriptConfig},
		{"TypeScript .mts", "eslint.config.mts", eslintTypeScriptConfig},
		{"TypeScript .cts", "eslint.config.cts", eslintTypeScriptConfig},
		{"legacy .eslintrc.json", ".eslintrc.json", eslintLegacyConfig},
		{"legacy .eslintrc.js", ".eslintrc.js", eslintLegacyConfig},
		{"legacy .eslintrc.cjs", ".eslintrc.cjs", eslintLegacyConfig},
		{"legacy .eslintrc.yml", ".eslintrc.yml", eslintLegacyConfig},
		{"legacy .eslintrc.yaml", ".eslintrc.yaml", eslintLegacyConfig},
		{"legacy .eslintrc", ".eslintrc", eslintLegacyConfig},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, tc.file)
			if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
				t.Fatal(err)
			}

			if got := tc.detect(root); got != path {
				t.Errorf("detect() = %q, want %q", got, path)
			}
		})
	}
}

// TestEslintConfigDetectionIsEmptyWhenAbsent pins the fourth case none of
// the three fixture-per-case detectors reach: nothing present at all.
func TestEslintConfigDetectionIsEmptyWhenAbsent(t *testing.T) {
	root := t.TempDir()

	for name, detect := range map[string]func(string) string{
		"flat":       eslintFlatConfig,
		"TypeScript": eslintTypeScriptConfig,
		"legacy":     eslintLegacyConfig,
	} {
		t.Run(name, func(t *testing.T) {
			if got := detect(root); got != "" {
				t.Errorf("detect() = %q, want \"\" with nothing present", got)
			}
		})
	}
}

// TestEslintFlatConfigPrefersJSOverMjsAndCjs pins the "which one responds"
// order eslintFlatConfigNames fixes, the same shape fallowConfigCandidates
// already establishes for fallow's own config.
func TestEslintFlatConfigPrefersJSOverMjsAndCjs(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"eslint.config.mjs", "eslint.config.js"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	want := filepath.Join(root, "eslint.config.js")
	if got := eslintFlatConfig(root); got != want {
		t.Errorf("eslintFlatConfig() = %q, want %q", got, want)
	}
}

// TestSpliceEslintConfigRefusesAMixedMarkerState pins the branch
// markerRegion cannot see on its own: one marker pair present and the other
// absent is not "malformed" by markerRegion's own per-pair definition, but
// dharness never writes one pair without the other, so spliceEslintConfig
// refuses with an error rather than guess which path applies. Both
// permutations are asserted — layer present with import absent, and import
// present with layer absent — because each is the mutation guard for one of
// the two switch cases: a case whose guard collapsed to ignore the other
// pair's state would take that case's path for one of these two fixtures
// and return a candidate instead of refusing.
func TestSpliceEslintConfigRefusesAMixedMarkerState(t *testing.T) {
	cases := map[string]string{
		"layer present, import absent": "export default [\n  " + eslintLayerBegin + "\n  ...dharnessLayer({ plugin: dharnessPlugin }),\n  " + eslintLayerEnd + "\n];\n",
		"import present, layer absent": eslintImportBegin + "\nimport dharnessPlugin from \"dharness-eslint-plugin\";\n" + eslintImportEnd + "\n\nexport default [\n  { rules: {} },\n];\n",
	}

	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, eslintConfig)
			if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
				t.Fatal(err)
			}
			p := project.Project{Root: root, Source: root}

			_, err := spliceEslintConfig(p, path, []byte(raw))
			if err == nil {
				t.Fatal("spliceEslintConfig() = nil error, want a refusal for a mixed marker state")
			}
			if !strings.Contains(err.Error(), "disagree") {
				t.Errorf("spliceEslintConfig() error = %q, want it to name the disagreement", err)
			}
		})
	}
}

// TestVerifyEslintCandidateCatchesEachInvariant pins verifyEslintCandidate's
// two checks independently: an ERROR node inside the default export, and a
// marker pair that is not exactly one well-formed region — the two
// invariants the candidate guard asserts (design decision 6), with no
// element-count assertion (design decision 1 cuts it).
func TestVerifyEslintCandidateCatchesEachInvariant(t *testing.T) {
	t.Run("an ERROR node inside the default export fails", func(t *testing.T) {
		candidate := []byte("export default [\n  { a: 1 + },\n];\n")
		if err := verifyEslintCandidate(candidate); err == nil {
			t.Fatal("verifyEslintCandidate() = nil, want an error for an ERROR node")
		}
	})

	t.Run("a duplicated marker pair fails", func(t *testing.T) {
		candidate := []byte("export default [\n  " +
			eslintLayerBegin + "\n  ...a,\n  " + eslintLayerEnd + "\n  " +
			eslintLayerBegin + "\n  ...b,\n  " + eslintLayerEnd + "\n" +
			eslintImportBegin + "\n" + eslintImportEnd + "\n];\n")
		if err := verifyEslintCandidate(candidate); err == nil {
			t.Fatal("verifyEslintCandidate() = nil, want an error for a duplicated marker pair")
		}
	})

	t.Run("a well-formed candidate passes", func(t *testing.T) {
		candidate := []byte(eslintImportBegin + "\n" +
			"import dharnessPlugin from \"dharness-eslint-plugin\";\n" +
			eslintImportEnd + "\n" +
			"\nexport default [\n  " +
			eslintLayerBegin + "\n  ...dharnessLayer({ plugin: dharnessPlugin }),\n  " + eslintLayerEnd + "\n];\n")
		if err := verifyEslintCandidate(candidate); err != nil {
			t.Errorf("verifyEslintCandidate() = %v, want nil for a well-formed candidate", err)
		}
	})
}

// allowListFixture builds an already-adopted .dharness/ whose ignore file
// already has content, and hands back the project and that file's path.
//
// The three repair tests differ by the list they start from and by nothing
// else. Spelling the same six-line setup out per test made the one line that
// varies the hardest to find.
func allowListFixture(t *testing.T, existing string) (project.Project, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, project.Dir), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, project.Dir, ".gitignore")
	if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	return project.Project{Root: root}, path
}
