package project

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestDetectPackageManagerPrefersBunOverOlderLockfiles(t *testing.T) {
	cases := []struct {
		name      string
		lockfiles []string
		want      string
	}{
		{"bun", []string{"bun.lock"}, "bun"},
		{"pnpm", []string{"pnpm-lock.yaml"}, "pnpm"},
		{"yarn", []string{"yarn.lock"}, "yarn"},
		{"npm", []string{"package-lock.json"}, "npm"},
		// Nothing found is not npm. A repository with no lockfile at all is a
		// repository dharness has not identified, and saying "npm" there is a
		// guess printed in the same sentence as the facts.
		{"none detects nothing", nil, ""},
		// A repository that migrated package managers can carry both files;
		// the newer one is the one actually in use.
		{"bun alongside npm", []string{"package-lock.json", "bun.lock"}, "bun"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			for _, lockfile := range testCase.lockfiles {
				write(t, filepath.Join(root, lockfile), "")
			}

			if got := Describe(root).PackageManager; got != testCase.want {
				t.Errorf("PackageManager = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestDetectTestRunnerReadsDeclaredDependencies(t *testing.T) {
	cases := []struct {
		name        string
		packageJSON string
		want        string
	}{
		{"vitest in devDependencies", `{"devDependencies":{"vitest":"^4.0.0"}}`, "vitest"},
		{"jest in dependencies", `{"dependencies":{"jest":"^29.0.0"}}`, "jest"},
		{"expo preset counts as jest", `{"devDependencies":{"jest-expo":"^54.0.0"}}`, "jest"},
		{"nothing declared", `{}`, ""},
		{"unreadable package.json", `{ not json`, ""},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			write(t, filepath.Join(root, "package.json"), testCase.packageJSON)

			if got := Describe(root).TestRunner; got != testCase.want {
				t.Errorf("TestRunner = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestDetectYarnPlugAndPlayFromGeneratedLoaders(t *testing.T) {
	for _, evidence := range []string{".pnp.cjs", ".pnp.loader.mjs"} {
		t.Run(evidence, func(t *testing.T) {
			root := t.TempDir()
			write(t, filepath.Join(root, "package.json"), `{"packageManager":"yarn@4.9.1","devDependencies":{"vitest":"^4.0.0"}}`)
			write(t, filepath.Join(root, evidence), "")

			if p := Describe(root); !p.YarnPnP {
				t.Errorf("Project = %+v, want confirmed Yarn PnP", p)
			}
		})
	}
}

func TestYarnNodeModulesLayoutDoesNotReportPlugAndPlay(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "package.json"), `{"packageManager":"yarn@4.9.1","devDependencies":{"vitest":"^4.0.0"}}`)
	if err := os.MkdirAll(filepath.Join(root, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}

	if p := Describe(root); p.YarnPnP {
		t.Errorf("Project = %+v, want node_modules layout", p)
	}
}

func TestLocalBinaryFindsTheProjectHelper(t *testing.T) {
	root := t.TempDir()
	name := "lefthook"
	if runtime.GOOS == "windows" {
		name += ".cmd"
	}
	binDir := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(binDir, name), "")

	resolved := Describe(root).LocalBinary("lefthook")

	if resolved == "" {
		t.Fatal("LocalBinary() did not find the installed copy")
	}
	if !strings.Contains(resolved, "node_modules") {
		t.Errorf("LocalBinary() = %q, want a node_modules path", resolved)
	}
}

func TestStagedSourceFilesKeepsOnlyAnalysablePaths(t *testing.T) {
	restore := SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		return []byte("src/a.tsx\x00README.md\x00\x00src/b.ts\x00logo.svg\x00"), nil
	})
	t.Cleanup(restore)

	files, err := StagedSourceFiles(t.TempDir())
	if err != nil {
		t.Fatalf("StagedSourceFiles() = %v", err)
	}

	want := []string{"src/a.tsx", "src/b.ts"}
	if len(files) != len(want) {
		t.Fatalf("StagedSourceFiles() = %v, want %v", files, want)
	}
	for i := range want {
		if files[i] != want[i] {
			t.Errorf("file %d = %q, want %q", i, files[i], want[i])
		}
	}
}

// Reporting success without an index would let a commit through a gate that
// never examined it, so the failure has to surface.
func TestStagedSourceFilesFailsWhenThereIsNoIndex(t *testing.T) {
	restore := SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		return nil, os.ErrNotExist
	})
	t.Cleanup(restore)

	_, err := StagedSourceFiles("/nowhere")

	var notARepo *NotAGitRepositoryError
	if !errors.As(err, &notARepo) {
		t.Fatalf("StagedSourceFiles() = %v, want NotAGitRepositoryError", err)
	}
	if !strings.Contains(err.Error(), "git repository") {
		t.Errorf("error does not say what is missing: %s", err)
	}
}

// Measured, not assumed: with core.quotePath at its default, `git diff
// --cached --name-only` reports src/café.ts as the literal "src/caf\303\251.ts",
// quotes included. filepath.Ext of that is `.ts"`, so IsSourceFile dropped it —
// and when it was the only staged file the gate reported nothing to check and
// exited 0. -z is what stops the gate passing over a change it never read.
func TestStagedSourceFilesKeepsPathsGitWouldHaveQuoted(t *testing.T) {
	var asked []string
	t.Cleanup(SetGitOutputForTest(func(_ string, args ...string) ([]byte, error) {
		asked = args
		return []byte("src/café.ts\x00"), nil
	}))

	files, err := StagedSourceFiles(t.TempDir())
	if err != nil {
		t.Fatalf("StagedSourceFiles() = %v", err)
	}
	if len(files) != 1 || files[0] != "src/café.ts" {
		t.Errorf("StagedSourceFiles() = %v, want the accented path intact", files)
	}

	if !slices.Contains(asked, "-z") {
		t.Errorf("git was asked without -z, so it will quote non-ASCII paths: %v", asked)
	}
}

// Corepack's field is the project stating which manager it uses. A declared
// answer outranks one deduced from whatever happens to be on disk.
func TestDetectPackageManagerPrefersTheDeclaredFieldOverTheLockfile(t *testing.T) {
	cases := []struct {
		name, declared, want string
	}{
		{"pinned version", `"bun@1.2.3"`, "bun"},
		{"bare name", `"pnpm"`, "pnpm"},
		{"unknown manager falls back to the lockfile", `"corepack-only@1"`, "npm"},
		{"absent falls back to the lockfile", `""`, "npm"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			write(t, filepath.Join(root, "package-lock.json"), "")
			write(t, filepath.Join(root, "package.json"), `{"packageManager":`+testCase.declared+`}`)

			if got := Describe(root).PackageManager; got != testCase.want {
				t.Errorf("PackageManager = %q, want %q", got, testCase.want)
			}
		})
	}
}

func write(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Which list a package sits in is not a question dharness has an opinion
// about: it asks whether the project already decided on this package at all.
func TestDeclaresFindsBothDependencyLists(t *testing.T) {
	root := t.TempDir()
	manifest := `{"dependencies":{"react":"19.0.0"},"devDependencies":{"@stryker-mutator/core":"9.6.1"}}`
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	p := Describe(root)

	for _, name := range []string{"react", "@stryker-mutator/core"} {
		if !p.Declares(name) {
			t.Errorf("Declares(%q) = false, want true", name)
		}
	}
	if p.Declares("vitest") {
		t.Error("Declares() found a package the manifest never names")
	}
	if (Project{}).Declares("react") {
		t.Error("a project with no source declared something")
	}
}

// TestSourceExtensionsAreTheSpelledOutRecognisedSet pins the list the ESLint
// layer renders its matchers from. It is spelled out here rather than compared
// against IsSourceFile, which now reads the same list: a test that asks the
// implementation to confirm itself would pass through a typo that dropped an
// extension.
//
// The entries are a dot and lower-case letters, which is what
// setup.sourceMatcher's per-letter character class assumes — anything else
// would render a glob matching something other than that extension.
func TestSourceExtensionsAreTheSpelledOutRecognisedSet(t *testing.T) {
	want := []string{".ts", ".tsx", ".js", ".jsx", ".mts", ".cts", ".mjs", ".cjs"}

	got := SourceExtensions()
	if !slices.Equal(got, want) {
		t.Fatalf("SourceExtensions() = %v, want %v", got, want)
	}

	spelling := regexp.MustCompile(`^\.[a-z]+$`)
	for _, extension := range got {
		if !spelling.MatchString(extension) {
			t.Errorf("SourceExtensions() = %v, want every entry to be a dot and lower-case letters", got)
		}
	}
}

// TestIsSourceFileFoldsTheExtensionCase holds the recognition semantics to
// what the ESLint matchers have to reproduce: the extension is compared
// lower-cased, so src/App.TSX is a source file, and it is compared as the
// extension alone, so .vue, .json and an unrelated suffix are not.
func TestIsSourceFileFoldsTheExtensionCase(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"src/app.ts", true},
		{"src/App.TS", true},
		{"src/panel.TsX", true},
		{"src/index.mjs", true},
		{"src/legacy.CJS", true},
		{"src/types.d.ts", true},
		{"src/app.vue", false},
		{"src/data.json", false},
		{"src/app", false},
		{"src/app.tsx.bak", false},
	}

	for _, tc := range cases {
		if got := IsSourceFile(tc.path); got != tc.want {
			t.Errorf("IsSourceFile(%q) = %t, want %t", tc.path, got, tc.want)
		}
	}
}
