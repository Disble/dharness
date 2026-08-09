package project

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
		{"none defaults to npm", nil, "npm"},
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

func TestResolvePrefersTheProjectCopyOverTheRemoteForm(t *testing.T) {
	root := t.TempDir()
	name := "fallow"
	if runtime.GOOS == "windows" {
		name += ".cmd"
	}
	binDir := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(binDir, name), "")

	resolved := Describe(root).Resolve("fallow")

	if !resolved.Local {
		t.Fatalf("Resolve() did not find the installed copy, got %+v", resolved)
	}
	if !strings.Contains(resolved.Name, "node_modules") {
		t.Errorf("Resolve() = %q, want a node_modules path", resolved.Name)
	}
}

// yarn v1 has no dlx and telling it to use one produces an unhelpful failure,
// so yarn resolves through npx, which works under both major versions.
func TestResolveFallsBackToTheRemoteFormPerPackageManager(t *testing.T) {
	cases := map[string]string{
		"bun":  "bunx",
		"pnpm": "pnpm",
		"yarn": "npx",
		"npm":  "npx",
	}

	for manager, want := range cases {
		t.Run(manager, func(t *testing.T) {
			resolved := Project{Root: t.TempDir(), PackageManager: manager}.Resolve("fallow")

			if resolved.Local {
				t.Fatal("Resolve() reported a local copy that does not exist")
			}
			if resolved.Name != want {
				t.Errorf("Resolve() = %q, want %q", resolved.Name, want)
			}
		})
	}
}

func TestStagedSourceFilesKeepsOnlyAnalysablePaths(t *testing.T) {
	restore := SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		return []byte("src/a.tsx\nREADME.md\n\nsrc/b.ts\nlogo.svg\n"), nil
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

func write(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

// An unpinned remote invocation is not "whatever is current": npx resolved a
// cached 0.2.1 while @latest resolved 0.9.11, and the gate failed on flags the
// current release documents.
func TestResolvePinsTheRemoteFormToLatest(t *testing.T) {
	for _, manager := range []string{"bun", "pnpm", "yarn", "npm"} {
		t.Run(manager, func(t *testing.T) {
			resolved := Project{Root: t.TempDir(), PackageManager: manager}.Resolve("fallow")

			invocation := strings.Join(append([]string{resolved.Name}, resolved.Args...), " ")
			if !strings.Contains(invocation, "fallow@latest") {
				t.Errorf("remote invocation is unpinned: %q", invocation)
			}
			if manager == "npm" || manager == "yarn" {
				if !strings.Contains(invocation, "--yes") {
					t.Errorf("npx would stop to ask permission inside a hook: %q", invocation)
				}
			}
		})
	}
}
