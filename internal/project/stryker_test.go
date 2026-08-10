package project

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func testRunnerProject(t *testing.T, packageJSON string) string {
	t.Helper()

	root := t.TempDir()
	write(t, filepath.Join(root, "package.json"), packageJSON)
	return root
}

func TestStrykerRunnerInfersSupportedProjectRunnerWithoutALocalPlugin(t *testing.T) {
	cases := []struct {
		name        string
		packageJSON string
		want        string
	}{
		{"vitest", `{"devDependencies":{"vitest":"^4.0.0"}}`, "vitest"},
		{"jest", `{"devDependencies":{"jest":"^30.0.0"}}`, "jest"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			selection, err := Describe(testRunnerProject(t, testCase.packageJSON)).StrykerRunner()

			if err != nil {
				t.Fatalf("StrykerRunner() = %v", err)
			}
			if selection.TestRunner != testCase.want || selection.Configured {
				t.Errorf("StrykerRunner() = %+v, want inferred %s", selection, testCase.want)
			}
		})
	}
}

// Command-line arguments overrule the config file. The config's runner still
// determines which remote plugin accompanies Core, while staying off the CLI.
func TestStrykerRunnerTreatsJSONConfigAsAuthoritative(t *testing.T) {
	root := testRunnerProject(t, `{"devDependencies":{"vitest":"^4.0.0"}}`)
	write(t, filepath.Join(root, "stryker.config.json"), `{"testRunner":"jest","appendPlugins":["custom-plugin"]}`)

	selection, err := Describe(root).StrykerRunner()

	if err != nil {
		t.Fatalf("StrykerRunner() = %v", err)
	}
	if selection.TestRunner != "jest" || !selection.Configured {
		t.Errorf("StrykerRunner() = %+v, want configured jest", selection)
	}
	if len(selection.AppendPlugins) != 1 || selection.AppendPlugins[0] != "custom-plugin" {
		t.Errorf("StrykerRunner() discarded configured appendPlugins: %+v", selection)
	}
}

func TestStrykerRunnerRecognizesEveryDefaultConfigFile(t *testing.T) {
	configFiles := []string{
		"stryker.conf.json",
		"stryker.conf.js",
		"stryker.conf.mjs",
		"stryker.conf.cjs",
		"stryker.config.json",
		"stryker.config.js",
		"stryker.config.mjs",
		"stryker.config.cjs",
		".stryker.conf.json",
		".stryker.conf.js",
		".stryker.conf.mjs",
		".stryker.conf.cjs",
		".stryker.config.json",
		".stryker.config.js",
		".stryker.config.mjs",
		".stryker.config.cjs",
	}

	for _, configFile := range configFiles {
		t.Run(configFile, func(t *testing.T) {
			root := testRunnerProject(t, `{"devDependencies":{"vitest":"^4.0.0"}}`)
			if filepath.Ext(configFile) == ".json" {
				write(t, filepath.Join(root, configFile), `{"testRunner":"jest","appendPlugins":["custom-plugin"]}`)

				selection, err := Describe(root).StrykerRunner()
				if err != nil {
					t.Fatalf("StrykerRunner() = %v", err)
				}
				if selection.TestRunner != "jest" || !selection.Configured || len(selection.AppendPlugins) != 1 || selection.AppendPlugins[0] != "custom-plugin" {
					t.Errorf("StrykerRunner() inferred through authoritative %s: %+v", configFile, selection)
				}
				return
			}

			write(t, filepath.Join(root, configFile), `export default { testRunner: "jest" }`)
			selection, err := Describe(root).StrykerRunner()
			var selectionErr *StrykerRunnerError
			if !errors.As(err, &selectionErr) {
				t.Fatalf("StrykerRunner() = %+v, %v; want executable config block", selection, err)
			}
			if !strings.Contains(err.Error(), configFile) || !strings.Contains(err.Error(), "JSON") {
				t.Errorf("executable config error is not actionable: %s", err)
			}
		})
	}
}

func TestStrykerRunnerRejectsConfigItCannotProvisionSafely(t *testing.T) {
	cases := []struct {
		name, config, contents, want string
	}{
		{"executable config", "stryker.config.mjs", `export default { testRunner: "vitest" }`, "JSON"},
		{"missing selection", ".stryker.conf.json", `{}`, "testRunner"},
		{"unsupported selection", "stryker.conf.json", `{"testRunner":"karma"}`, "karma"},
		{"invalid JSON", "stryker.config.json", `{`, "read"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := testRunnerProject(t, `{"devDependencies":{"vitest":"^4.0.0"}}`)
			write(t, filepath.Join(root, testCase.config), testCase.contents)

			_, err := Describe(root).StrykerRunner()

			var selectionErr *StrykerRunnerError
			if !errors.As(err, &selectionErr) {
				t.Fatalf("StrykerRunner() = %v, want StrykerRunnerError", err)
			}
			if !strings.Contains(err.Error(), "Stryker") || !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("error is not actionable for %s: %s", testCase.config, err)
			}
		})
	}
}

func TestStrykerRunnerRejectsMissingAndAmbiguousProjectRunners(t *testing.T) {
	cases := []struct {
		name, packageJSON, want string
	}{
		{"missing", `{}`, "no supported test runner"},
		{"ambiguous", `{"devDependencies":{"vitest":"^4.0.0","jest":"^30.0.0"}}`, "both vitest and jest"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := Describe(testRunnerProject(t, testCase.packageJSON)).StrykerRunner()

			var selectionErr *StrykerRunnerError
			if !errors.As(err, &selectionErr) {
				t.Fatalf("StrykerRunner() = %v, want StrykerRunnerError", err)
			}
			if !strings.Contains(err.Error(), testCase.want) || !strings.Contains(err.Error(), "vitest or jest") {
				t.Errorf("error does not explain the supported correction: %s", err)
			}
		})
	}
}
