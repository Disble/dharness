package tool

import (
	"slices"
	"strings"
	"testing"
)

func TestRemoteLatestOwnsEveryWrappedExecutorShape(t *testing.T) {
	managers := []struct {
		name string
		exec string
		args []string
	}{
		{"bun", "bunx", nil},
		{"pnpm", "pnpm", []string{"dlx"}},
		{"yarn", "npx", []string{"--yes"}},
		{"npm", "npx", []string{"--yes"}},
	}
	wrapped := []struct {
		binary, spec string
	}{
		{ReactDoctor, "react-doctor@latest"},
		{Fallow, "fallow@latest"},
		{Stryker, "@stryker-mutator/core@latest"},
	}

	for _, manager := range managers {
		for _, wrappedTool := range wrapped {
			t.Run(manager.name+"/"+wrappedTool.binary, func(t *testing.T) {
				command := RemoteLatest(manager.name, wrappedTool.binary, `C:\project with spaces`, "arg")
				wantArgs := append(append([]string{}, manager.args...), wrappedTool.spec, "arg")

				if command.Label != wrappedTool.binary || command.Name != manager.exec || command.Dir != `C:\project with spaces` || !slices.Equal(command.Args, wantArgs) {
					t.Errorf("RemoteLatest() = %+v, want %s %v", command, manager.exec, wantArgs)
				}
			})
		}
	}
}

func TestRemotePackageBinaryOwnsAlternateBinarySyntax(t *testing.T) {
	cases := []struct {
		manager string
		name    string
		args    []string
	}{
		{"bun", "bunx", []string{"--package", "fallow@latest", "fallow-mcp"}},
		{"pnpm", "pnpm", []string{"--package=fallow@latest", "dlx", "fallow-mcp"}},
		{"yarn", "npx", []string{"--yes", "--package=fallow@latest", "fallow-mcp"}},
		{"npm", "npx", []string{"--yes", "--package=fallow@latest", "fallow-mcp"}},
	}

	for _, testCase := range cases {
		t.Run(testCase.manager, func(t *testing.T) {
			command := RemotePackageBinary(testCase.manager, "fallow@latest", "fallow-mcp", t.TempDir())

			if command.Name != testCase.name || !slices.Equal(command.Args, testCase.args) {
				t.Errorf("RemotePackageBinary() = %s %v, want %s %v", command.Name, command.Args, testCase.name, testCase.args)
			}
		})
	}
}

func TestDependencyCommandsOwnPackageManagerSyntax(t *testing.T) {
	cases := []struct {
		manager     string
		installName string
		installArgs []string
		removeName  string
		removeArgs  []string
	}{
		{"bun", "bun", []string{"add", "-d", "integration"}, "bun", []string{"remove", "integration"}},
		{"pnpm", "pnpm", []string{"add", "-D", "integration"}, "pnpm", []string{"remove", "integration"}},
		{"yarn", "yarn", []string{"add", "-D", "integration"}, "yarn", []string{"remove", "integration"}},
		{"npm", "npm", []string{"install", "--save-dev", "integration"}, "npm", []string{"uninstall", "--save-dev", "integration"}},
	}

	for _, testCase := range cases {
		t.Run(testCase.manager, func(t *testing.T) {
			dir := t.TempDir()
			install := InstallPackages(testCase.manager, dir, []string{"integration"})
			remove := RemovePackages(testCase.manager, dir, []string{"integration"})

			if install.Name != testCase.installName || install.Dir != dir || !slices.Equal(install.Args, testCase.installArgs) {
				t.Errorf("InstallPackages() = %+v, want %s %v", install, testCase.installName, testCase.installArgs)
			}
			if remove.Name != testCase.removeName || remove.Dir != dir || !slices.Equal(remove.Args, testCase.removeArgs) {
				t.Errorf("RemovePackages() = %+v, want %s %v", remove, testCase.removeName, testCase.removeArgs)
			}
		})
	}
}

func TestInstalledBuildsTheLocalHelperCommand(t *testing.T) {
	command := Installed("lefthook", `C:\project\node_modules\.bin\lefthook.cmd`, `C:\project`, "install")

	if command.Label != "lefthook" || command.Name != `C:\project\node_modules\.bin\lefthook.cmd` || command.Dir != `C:\project` || !slices.Equal(command.Args, []string{"install"}) {
		t.Errorf("Installed() = %+v", command)
	}
}

func TestPackageNameStripsTheTagFromScopedNames(t *testing.T) {
	cases := map[string]string{
		"@stryker-mutator/core@latest": "@stryker-mutator/core",
		"fallow@latest":                "fallow",
		"@stryker-mutator/core":        "@stryker-mutator/core",
		"react-doctor":                 "react-doctor",
		// A one-character name puts the tag's @ at index 1, which is the
		// boundary the guard against a leading scope @ has to clear.
		"a@latest": "a",
	}

	for spec, want := range cases {
		if got := PackageName(spec); got != want {
			t.Errorf("PackageName(%q) = %q, want %q", spec, got, want)
		}
	}
}

// Restoring is `install`, never `add`. `add` resolves a version and writes it
// back even with no tag, which turns an exact pin into a caret range.
func TestRestoreDeclaredInstallsTheManifestWithoutNamingAVersion(t *testing.T) {
	cases := map[string][]string{
		"npm":  {"npm", "install"},
		"pnpm": {"pnpm", "install"},
		"yarn": {"yarn", "install"},
		"bun":  {"bun", "install"},
	}

	for manager, want := range cases {
		t.Run(manager, func(t *testing.T) {
			command := RestoreDeclared(manager, `C:\project`)

			if command.Name != want[0] || !slices.Equal(command.Args, want[1:]) {
				t.Errorf("RestoreDeclared() = %s %v, want %s %v", command.Name, command.Args, want[0], want[1:])
			}
			for _, arg := range command.Args {
				if strings.Contains(arg, "@") {
					t.Errorf("restore named a version: %v", command.Args)
				}
			}
		})
	}
}
