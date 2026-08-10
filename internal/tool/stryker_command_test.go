package tool

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestStrykerCommandProvisionsCoreAndRunnerTogether(t *testing.T) {
	cases := []struct {
		manager string
		name    string
		args    []string
	}{
		{"npm", "npx", []string{"--yes", "--package=@stryker-mutator/core@latest", "--package=@stryker-mutator/vitest-runner@latest", "stryker", "run", "--dryRunOnly", "--appendPlugins", "@stryker-mutator/vitest-runner"}},
		{"pnpm", "pnpm", []string{"--package=@stryker-mutator/core@latest", "--package=@stryker-mutator/vitest-runner@latest", "dlx", "stryker", "run", "--dryRunOnly", "--appendPlugins", "@stryker-mutator/vitest-runner"}},
		{"yarn", "npx", []string{"--yes", "--package=@stryker-mutator/core@latest", "--package=@stryker-mutator/vitest-runner@latest", "stryker", "run", "--dryRunOnly", "--appendPlugins", "@stryker-mutator/vitest-runner"}},
		{"bun", "bunx", []string{"--package", "@stryker-mutator/core@latest", "--package", "@stryker-mutator/vitest-runner@latest", "stryker", "run", "--dryRunOnly", "--appendPlugins", "@stryker-mutator/vitest-runner"}},
	}

	for _, testCase := range cases {
		t.Run(testCase.manager, func(t *testing.T) {
			command, err := StrykerCommand(testCase.manager, false, `C:\project with spaces`, "vitest", nil, "run", "--dryRunOnly")

			if err != nil {
				t.Fatalf("StrykerCommand() = %v", err)
			}
			if command.Label != Stryker || command.Name != testCase.name || !slices.Equal(command.Args, testCase.args) {
				t.Errorf("StrykerCommand() = %s %v, want %s %v", command.Name, command.Args, testCase.name, testCase.args)
			}
			if command.Dir != `C:\project with spaces` || !command.LowPriority {
				t.Errorf("StrykerCommand() lost execution properties: %+v", command)
			}
		})
	}
}

func TestStrykerCommandBlocksYarnPnPBeforeItCanLoseTheRunner(t *testing.T) {
	_, err := StrykerCommand("yarn", true, t.TempDir(), "vitest", nil, "run")

	if !errors.Is(err, ErrUnsupportedStrykerExecution) {
		t.Fatalf("StrykerCommand() = %v, want unsupported remote execution", err)
	}
	for _, evidence := range []string{"Stryker", "Yarn Plug'n'Play", "nodeLinker: node-modules", "yarn install"} {
		if !strings.Contains(err.Error(), evidence) {
			t.Errorf("Yarn block omits %q: %s", evidence, err)
		}
	}
}

func TestStrykerCommandSelectsTheJestPlugin(t *testing.T) {
	command, err := StrykerCommand("npm", false, t.TempDir(), "jest", nil, "run")
	if err != nil {
		t.Fatalf("StrykerCommand() = %v", err)
	}

	want := "--package=@stryker-mutator/jest-runner@latest"
	if !slices.Contains(command.Args, want) {
		t.Errorf("Stryker command omits %s: %v", want, command.Args)
	}
}

func TestStrykerCommandPreservesConfiguredPluginsWhenAppendingTheRunner(t *testing.T) {
	command, err := StrykerCommand("npm", false, t.TempDir(), "vitest", []string{"custom-plugin"}, "run")
	if err != nil {
		t.Fatalf("StrykerCommand() = %v", err)
	}

	want := []string{"--appendPlugins", "custom-plugin,@stryker-mutator/vitest-runner"}
	index := slices.Index(command.Args, want[0])
	if index < 0 || index+1 >= len(command.Args) || !slices.Equal(command.Args[index:index+2], want) {
		t.Errorf("configured appendPlugins lost authority: %v", command.Args)
	}
}

func TestStrykerCommandRejectsUnsafeTransientSelections(t *testing.T) {
	cases := []struct {
		name, manager, testRunner, want string
	}{
		{"unsupported manager", "other", "vitest", "package manager"},
		{"unsupported runner", "npm", "karma", "test runner"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := StrykerCommand(testCase.manager, false, t.TempDir(), testCase.testRunner, nil, "run")

			if err == nil || !strings.Contains(err.Error(), "Stryker") || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("StrykerCommand() = %v, want actionable %s error", err, testCase.want)
			}
			if !errors.Is(err, ErrUnsupportedStrykerExecution) {
				t.Errorf("StrykerCommand() error does not identify unsupported remote execution: %v", err)
			}
		})
	}
}
