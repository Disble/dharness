package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// binaryName is how npm writes the shim on this platform: a .cmd wrapper on
// Windows, a plain file everywhere else.
func binaryName(tool string) string {
	if runtime.GOOS == "windows" {
		return tool + ".cmd"
	}
	return tool
}

func syncOutput(t *testing.T, prepare func(root string)) string {
	t.Helper()

	root := t.TempDir()
	if prepare != nil {
		prepare(root)
	}

	previous := workingDirectory
	workingDirectory = func() (string, error) { return root, nil }
	t.Cleanup(func() { workingDirectory = previous })

	var out bytes.Buffer
	if err := RunSync(nil, &out); err != nil {
		t.Fatalf("RunSync() = %v", err)
	}
	return out.String()
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The whole point of generating this instead of storing it: a bun project must
// never be told to run npm, and a jest project must never be pointed at the
// vitest plugin.
func TestSyncSpeaksTheProjectsOwnPackageManagerAndRunner(t *testing.T) {
	cases := []struct {
		name        string
		lockfile    string
		packageJSON string
		want        []string
		reject      []string
	}{
		{
			name:        "bun and vitest",
			lockfile:    "bun.lock",
			packageJSON: `{"devDependencies":{"vitest":"^4.0.0"}}`,
			want:        []string{"bun add -d", "@stryker-mutator/vitest-runner", "bunx"},
			reject:      []string{"npm install", "jest-runner"},
		},
		{
			name:        "pnpm and jest",
			lockfile:    "pnpm-lock.yaml",
			packageJSON: `{"devDependencies":{"jest":"^29.0.0"}}`,
			want:        []string{"pnpm add -D", "@stryker-mutator/jest-runner", "pnpm dlx"},
			reject:      []string{"bun add", "vitest-runner"},
		},
		{
			name:        "yarn falls back to npx for remote execution",
			lockfile:    "yarn.lock",
			packageJSON: `{"devDependencies":{"vitest":"^4.0.0"}}`,
			want:        []string{"yarn add -D", "npx"},
			reject:      []string{"yarn dlx"},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			out := syncOutput(t, func(root string) {
				writeFile(t, filepath.Join(root, testCase.lockfile), "")
				writeFile(t, filepath.Join(root, "package.json"), testCase.packageJSON)
			})

			for _, want := range testCase.want {
				if !strings.Contains(out, want) {
					t.Errorf("output does not mention %q:\n%s", want, out)
				}
			}
			for _, reject := range testCase.reject {
				if strings.Contains(out, reject) {
					t.Errorf("output mentions %q, which belongs to another project:\n%s", reject, out)
				}
			}
		})
	}
}

// A step with nothing left to do is noise. Once the tools are installed the
// install step has to disappear, not turn into "already done".
func TestSyncDropsStepsTheProjectAlreadySatisfied(t *testing.T) {
	out := syncOutput(t, func(root string) {
		writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
		for _, name := range []string{"react-doctor", "fallow", "stryker"} {
			writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName(name)), "")
		}
		writeFile(t, filepath.Join(root, "node_modules", "@stryker-mutator", "vitest-runner", "package.json"), "{}")
		writeFile(t, filepath.Join(root, ".fallowrc.jsonc"), "{}")
		writeFile(t, filepath.Join(root, "lefthook.yml"), "pre-commit:\n  commands:\n    gate:\n      run: dharness check\n")
	})

	for _, gone := range []string{"Install the tools", "runner plugin", "entry points", "Wire the commit gate"} {
		if strings.Contains(out, gone) {
			t.Errorf("output still asks for %q on a configured project:\n%s", gone, out)
		}
	}
	// Setup is the whole subject here: with nothing left, the command says so
	// instead of listing a step that adoption can never satisfy.
	if !strings.Contains(out, "Nothing to do") {
		t.Errorf("a fully configured project got no terminal answer:\n%s", out)
	}
}

// fallow's entry points are the one thing no CLI can do for you, so it is the
// one step described as work rather than printed as a command.
func TestSyncAsksAboutFallowEntryPointsOnlyWithoutConfig(t *testing.T) {
	without := syncOutput(t, func(root string) {
		writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	})
	if !strings.Contains(without, ".fallowrc.jsonc") {
		t.Errorf("output does not name the file to write:\n%s", without)
	}

	with := syncOutput(t, func(root string) {
		writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
		writeFile(t, filepath.Join(root, "fallow.toml"), "")
	})
	if strings.Contains(with, "entry points") {
		t.Errorf("output asks about entry points that are already declared:\n%s", with)
	}
}

// Stryker's binary and its package have different names; installing `stryker`
// installs nothing useful.
func TestSyncInstallsTheStrykerPackageNotTheBinaryName(t *testing.T) {
	out := syncOutput(t, func(root string) {
		writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	})

	if !strings.Contains(out, "@stryker-mutator/core") {
		t.Errorf("output does not install the Stryker package:\n%s", out)
	}
}

// Nothing here is a one-time act. A hook that is present makes the step vanish;
// a hook that was rewritten, renamed or deleted stops matching and the step
// comes back on its own. That is the whole drift story, and it needs no file to
// remember anything.
func TestSyncTracksTheHookBothWays(t *testing.T) {
	cases := []struct {
		name     string
		file     string
		contents string
		wired    bool
	}{
		{"no hook at all", "", "", false},
		{"lefthook invokes the gate", "lefthook.yml", "pre-commit:\n  commands:\n    gate:\n      run: dharness check\n", true},
		{"husky invokes the gate", filepath.Join(".husky", "pre-commit"), "#!/bin/sh\ndharness check\n", true},
		{"hook exists but calls something else", "lefthook.yml", "pre-commit:\n  commands:\n    lint:\n      run: eslint .\n", false},
		{"hook was rewritten to an old command", "lefthook.yml", "pre-commit:\n  commands:\n    gate:\n      run: dharness run pre-commit\n", false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			out := syncOutput(t, func(root string) {
				writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
				if testCase.file != "" {
					writeFile(t, filepath.Join(root, testCase.file), testCase.contents)
				}
			})

			asks := strings.Contains(out, "Wire the commit gate")
			if testCase.wired && asks {
				t.Errorf("output still asks to wire a gate that is already wired:\n%s", out)
			}
			if !testCase.wired && !asks {
				t.Errorf("output does not ask to wire the gate:\n%s", out)
			}
		})
	}
}
