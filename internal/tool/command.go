package tool

import (
	"strings"

	"github.com/Disble/dharness/internal/runner"
)

var remoteExecutors = map[string][]string{
	"bun":  {"bunx"},
	"pnpm": {"pnpm", "dlx"},
	"yarn": {"npx", "--yes"},
	"npm":  {"npx", "--yes"},
}

var installExecutors = map[string][]string{
	"bun":  {"bun", "add", "-d"},
	"pnpm": {"pnpm", "add", "-D"},
	"yarn": {"yarn", "add", "-D"},
	"npm":  {"npm", "install", "--save-dev"},
}

var removeExecutors = map[string][]string{
	"bun":  {"bun", "remove"},
	"pnpm": {"pnpm", "remove"},
	"yarn": {"yarn", "remove"},
	"npm":  {"npm", "uninstall", "--save-dev"},
}

// RemoteLatest builds a wrapped invocation through the detected manager.
func RemoteLatest(packageManager, binary, dir string, args ...string) runner.Command {
	exec := remoteExecutor(packageManager)
	commandArgs := append(append([]string{}, exec[1:]...), LatestSpec(binary))
	return command(binary, exec[0], dir, append(commandArgs, args...)...)
}

// RemotePackageBinary names the package when it exposes a different binary.
func RemotePackageBinary(packageManager, packageSpec, binary, dir string, args ...string) runner.Command {
	name, commandArgs := transientPackages(packageManager, []string{packageSpec}, binary)
	return command(binary, name, dir, append(commandArgs, args...)...)
}

// RemoteExec is the printable prefix for a direct remote invocation.
func RemoteExec(packageManager string) string {
	return strings.Join(remoteExecutor(packageManager), " ")
}

// InstallCommand is the printable command that adds development dependencies.
func InstallCommand(packageManager string) string {
	return strings.Join(packageExecutor(installExecutors, packageManager), " ")
}

// InstallPackages builds the integration dependency install command.
func InstallPackages(packageManager, dir string, packages []string) runner.Command {
	return dependencyCommand(installExecutors, packageManager, dir, packages)
}

// RemovePackages compensates InstallPackages for the same exact package set.
func RemovePackages(packageManager, dir string, packages []string) runner.Command {
	return dependencyCommand(removeExecutors, packageManager, dir, packages)
}

// Installed builds an invocation for a helper path detected by project metadata.
func Installed(binary, path, dir string, args ...string) runner.Command {
	return command(binary, path, dir, args...)
}

func transientPackages(packageManager string, packageSpecs []string, binary string) (string, []string) {
	exec := remoteExecutor(packageManager)
	switch normalizeManager(packageManager) {
	case "bun":
		args := make([]string, 0, len(packageSpecs)*2+1)
		for _, spec := range packageSpecs {
			args = append(args, "--package", spec)
		}
		return exec[0], append(args, binary)
	case "pnpm":
		args := make([]string, 0, len(packageSpecs)+2)
		for _, spec := range packageSpecs {
			args = append(args, "--package="+spec)
		}
		return exec[0], append(args, exec[1], binary)
	default:
		args := append([]string{}, exec[1:]...)
		for _, spec := range packageSpecs {
			args = append(args, "--package="+spec)
		}
		return exec[0], append(args, binary)
	}
}

func dependencyCommand(executors map[string][]string, packageManager, dir string, packages []string) runner.Command {
	exec := packageExecutor(executors, packageManager)
	return command(exec[0], exec[0], dir, append(append([]string{}, exec[1:]...), packages...)...)
}

func remoteExecutor(packageManager string) []string {
	return remoteExecutors[normalizeManager(packageManager)]
}

func packageExecutor(executors map[string][]string, packageManager string) []string {
	return executors[normalizeManager(packageManager)]
}

func normalizeManager(packageManager string) string {
	if knownManager(packageManager) {
		return packageManager
	}
	return "npm"
}

func knownManager(packageManager string) bool {
	_, ok := remoteExecutors[packageManager]
	return ok
}

func command(label, name, dir string, args ...string) runner.Command {
	return runner.Command{Label: label, Name: name, Args: args, Dir: dir}
}
