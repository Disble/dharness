package tool

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// TestStrykerPackagesNamesCoreAndTheRunnerAtLatest pins the specs dharness asks
// the manager to install, and it is deliberately manager-independent.
//
// The predecessor of this test asserted a bunx argument list that claimed to
// provision both packages and did not. Measured on 2026-08-13: bun keeps only
// the last --package, so the transient environment's package.json read
// {"dependencies":{"@stryker-mutator/vitest-runner":"^9.6.1"}} — Core arrived
// transitively, at the runner's range rather than at @latest. The old test
// passed the whole time because it compared arguments and never looked at what
// landed in node_modules.
func TestStrykerPackagesNamesCoreAndTheRunnerAtLatest(t *testing.T) {
	want := []string{"@stryker-mutator/core@latest", "@stryker-mutator/vitest-runner@latest"}

	for _, manager := range []string{"npm", "pnpm", "yarn", "bun"} {
		t.Run(manager, func(t *testing.T) {
			packages, err := StrykerPackages(manager, false, "vitest")

			if err != nil {
				t.Fatalf("StrykerPackages() = %v", err)
			}
			if !slices.Equal(packages, want) {
				t.Errorf("StrykerPackages() = %v, want %v", packages, want)
			}
		})
	}
}

func TestStrykerPackagesSelectsTheJestRunner(t *testing.T) {
	packages, err := StrykerPackages("npm", false, "jest")
	if err != nil {
		t.Fatalf("StrykerPackages() = %v", err)
	}

	want := "@stryker-mutator/jest-runner@latest"
	if !slices.Contains(packages, want) {
		t.Errorf("StrykerPackages() omits %s: %v", want, packages)
	}
}

func TestStrykerPackagesBlocksYarnPnPBeforeItCanLoseTheRunner(t *testing.T) {
	_, err := StrykerPackages("yarn", true, "vitest")

	if !errors.Is(err, ErrUnsupportedStrykerExecution) {
		t.Fatalf("StrykerPackages() = %v, want unsupported execution", err)
	}
	for _, evidence := range []string{"Stryker", "Yarn Plug'n'Play", "nodeLinker: node-modules", "yarn install"} {
		if !strings.Contains(err.Error(), evidence) {
			t.Errorf("Yarn block omits %q: %s", evidence, err)
		}
	}
}

func TestStrykerPackagesRejectsSelectionsItCannotInstall(t *testing.T) {
	cases := []struct {
		name, manager, testRunner, want string
	}{
		{"unsupported manager", "other", "vitest", "package manager"},
		{"unsupported runner", "npm", "karma", "test runner"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := StrykerPackages(testCase.manager, false, testCase.testRunner)

			if err == nil || !strings.Contains(err.Error(), "Stryker") || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("StrykerPackages() = %v, want actionable %s error", err, testCase.want)
			}
			if !errors.Is(err, ErrUnsupportedStrykerExecution) {
				t.Errorf("StrykerPackages() error does not identify unsupported execution: %v", err)
			}
		})
	}
}

// TestStrykerLocalRunsTheBinaryTheProjectInstalled pins the invocation shape.
//
// The path is the command name, not an argument to a remote executor: Stryker's
// TSConfigPreprocessor imports typescript from its own location, so the only
// place it resolves the project's compiler is inside the project's node_modules.
func TestStrykerLocalRunsTheBinaryTheProjectInstalled(t *testing.T) {
	binary := `C:\project with spaces\node_modules\.bin\stryker.exe`

	command := StrykerLocal(binary, `C:\project with spaces`, "vitest", nil, "run", "--dryRunOnly")

	wantArgs := []string{"run", "--dryRunOnly", "--appendPlugins", "@stryker-mutator/vitest-runner"}
	if command.Label != Stryker || command.Name != binary || !slices.Equal(command.Args, wantArgs) {
		t.Errorf("StrykerLocal() = %s %v, want %s %v", command.Name, command.Args, binary, wantArgs)
	}
	if command.Dir != `C:\project with spaces` || !command.LowPriority {
		t.Errorf("StrykerLocal() lost execution properties: %+v", command)
	}
}

// With nothing to append, the flag itself is left off. Passing an empty
// --appendPlugins would hand Stryker a value the project never asked for.
func TestStrykerLocalOmitsTheFlagWhenThereAreNoPlugins(t *testing.T) {
	command := StrykerLocal("stryker", t.TempDir(), "karma", nil, "run")

	if slices.Contains(command.Args, "--appendPlugins") {
		t.Errorf("an empty plugin list still produced the flag: %v", command.Args)
	}
	if !slices.Equal(command.Args, []string{"run"}) {
		t.Errorf("StrykerLocal() = %v, want the arguments untouched", command.Args)
	}
}

func TestStrykerLocalPreservesConfiguredPluginsWhenAppendingTheRunner(t *testing.T) {
	command := StrykerLocal("stryker", t.TempDir(), "vitest", []string{"custom-plugin"}, "run")

	want := []string{"--appendPlugins", "custom-plugin,@stryker-mutator/vitest-runner"}
	index := slices.Index(command.Args, want[0])
	if index < 0 || index+1 >= len(command.Args) || !slices.Equal(command.Args[index:index+2], want) {
		t.Errorf("configured appendPlugins lost authority: %v", command.Args)
	}
}
