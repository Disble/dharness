package project

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func vitestProject(t *testing.T, withPlugin bool) string {
	t.Helper()

	root := t.TempDir()
	write(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	if withPlugin {
		if err := os.MkdirAll(filepath.Join(root, "node_modules", "@stryker-mutator", "vitest-runner"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// Stryker needs no config file: every option is available on the command line.
// A project that never configured it still has to be told which runner to use,
// and that comes from what package.json declares.
func TestStrykerRunnerSuppliesTheRunnerWhenTheProjectHasNoConfig(t *testing.T) {
	runner, err := Describe(vitestProject(t, true)).StrykerRunner()

	if err != nil {
		t.Fatalf("StrykerRunner() = %v", err)
	}
	if runner != "vitest" {
		t.Errorf("StrykerRunner() = %q, want vitest", runner)
	}
}

// Command line arguments overrule the config file, so passing --testRunner over
// a project that already configured Stryker would let a detection mistake
// override a decision the project made deliberately.
func TestStrykerRunnerStaysOutOfTheWayWhenTheProjectConfiguredStryker(t *testing.T) {
	for _, name := range []string{"stryker.config.mjs", "stryker.config.json", ".stryker.conf.json"} {
		t.Run(name, func(t *testing.T) {
			root := vitestProject(t, true)
			write(t, filepath.Join(root, name), "{}")

			runner, err := Describe(root).StrykerRunner()

			if err != nil {
				t.Fatalf("StrykerRunner() = %v", err)
			}
			if runner != "" {
				t.Errorf("StrykerRunner() = %q, want it to defer to the config file", runner)
			}
		})
	}
}

// Installing the runner plugin is the one thing `stryker init` did that flags
// cannot replace, so the error has to carry the command that does it.
func TestStrykerRunnerNamesTheInstallCommandPerPackageManager(t *testing.T) {
	cases := map[string]string{
		"bun":  "bun add -d",
		"pnpm": "pnpm add -D",
		"yarn": "yarn add -D",
		"npm":  "npm install --save-dev",
	}

	for manager, want := range cases {
		t.Run(manager, func(t *testing.T) {
			p := Describe(vitestProject(t, false))
			p.PackageManager = manager

			_, err := p.StrykerRunner()

			var missing *MissingStrykerRunnerError
			if !errors.As(err, &missing) {
				t.Fatalf("StrykerRunner() = %v, want MissingStrykerRunnerError", err)
			}
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error does not carry the install command %q: %s", want, err)
			}
			if !strings.Contains(err.Error(), "@stryker-mutator/vitest-runner") {
				t.Errorf("error does not name the plugin: %s", err)
			}
		})
	}
}

func TestStrykerRunnerRefusesAProjectWithNoTestRunner(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "package.json"), `{}`)

	_, err := Describe(root).StrykerRunner()

	var missing *MissingStrykerRunnerError
	if !errors.As(err, &missing) {
		t.Fatalf("StrykerRunner() = %v, want MissingStrykerRunnerError", err)
	}
	if !strings.Contains(err.Error(), "vitest") || !strings.Contains(err.Error(), "jest") {
		t.Errorf("error does not say what would work: %s", err)
	}
}
