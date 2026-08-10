// Package project resolves what dharness needs to know about a repository:
// which package manager runs it, which test runner it uses, and where each
// wrapped tool lives.
//
// Nothing here reads a tool's configuration. Detection is deliberately shallow
// — lockfile names and package.json dependencies — because the moment dharness
// starts interpreting the tools' own config it becomes responsible for keeping
// up with their schemas.
package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Disble/dharness/internal/runner"
)

// Project is everything dharness detected about a repository.
//
// Root and Source are two answers a conventional layout collapses into one.
// Root is the repository — git, the hook manager, the directory dharness owns.
// Source is where the package manager installs, which is where the wrapped
// tools have to run. A Wails project keeps Go at the root and the whole
// frontend in a subdirectory, and every question below belongs to exactly one
// of the two. Treating them as one is what made a Go module report as an npm
// project.
//
// Source is empty when the repository holds no JS project at all. That is a
// real state and it is not the same as "npm": it means the tools have nothing
// to run on, and a command that needs them says so instead of guessing.
type Project struct {
	Root           string
	Source         string
	PackageManager string
	TestRunner     string
}

// HasSource reports whether this repository holds a JS project to analyse.
func (p Project) HasSource() bool { return p.Source != "" }

// SourceRel is Source expressed the way a config file at the repository root
// has to name it: relative, forward slashes. It is empty when the two roots are
// one directory, which is the case that needs no such key at all.
//
// The comparison is directory identity rather than string equality, because
// Root arrives from git in the spelling the kernel gave it and Source is built
// by joining onto it. On a case-insensitive volume those two spellings can
// differ while naming one directory.
func (p Project) SourceRel() string {
	if !p.HasSource() || sameDirectory(p.Root, p.Source) {
		return ""
	}
	rel, err := filepath.Rel(p.Root, p.Source)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(rel)
}

var lockfiles = []struct{ file, manager string }{
	{"bun.lockb", "bun"},
	{"bun.lock", "bun"},
	{"pnpm-lock.yaml", "pnpm"},
	{"yarn.lock", "yarn"},
	{"package-lock.json", "npm"},
}

// remoteExec is how each package manager runs a package that is not installed.
//
// yarn v1 has no dlx, and distinguishing it from Berry would mean parsing
// yarn's own version; npx works under both, so yarn resolves there. npx also
// carries --yes so it never stops to ask permission inside a git hook, where
// nobody is watching to answer.
var remoteExec = map[string][]string{
	"bun":  {"bunx"},
	"pnpm": {"pnpm", "dlx"},
	"yarn": {"npx", "--yes"},
	"npm":  {"npx", "--yes"},
}

// packages maps a binary name to the package that provides it. Stryker's
// binary is `stryker`; its package is not, and asking a registry for `stryker`
// fetches something else entirely.
var packages = map[string]string{"stryker": "@stryker-mutator/core"}

// Package returns the npm package that provides a tool's binary.
func Package(tool string) string {
	if name, ok := packages[tool]; ok {
		return name
	}
	return tool
}

// LatestSpec pins the remote form to the published version.
//
// Measured, not assumed: `npx react-doctor` resolved 0.2.1 from a stale cache
// while `npx react-doctor@latest` resolved 0.9.11 — seven minor versions apart,
// silently, with flags rejected as unknown that the current release documents.
// An unpinned remote invocation is not "whatever is current", it is "whatever
// this machine happened to download once".
func LatestSpec(tool string) string { return Package(tool) + "@latest" }

// Describe inspects a directory that is both the repository and the JS project,
// which is what a conventional layout looks like. Discover is the entry point
// that tells the two apart.
func Describe(root string) Project { return At(root, root) }

// At describes a repository whose JS project lives in source. It never fails:
// an undetected field is empty, and callers decide whether that matters.
func At(root, source string) Project {
	return Project{
		Root:           root,
		Source:         source,
		PackageManager: detectPackageManager(source),
		TestRunner:     detectTestRunner(source),
	}
}

// detectPackageManager asks the project before deducing anything.
//
// package.json's `packageManager` field is Corepack's, and it is the project
// stating which manager it uses rather than dharness inferring it from what
// happens to be on disk. A declared answer outranks a deduced one, so it is
// read first; the lockfile is the deduction, and it is a good one.
//
// Nothing found returns nothing. The previous default of "npm" was not a
// detection, it was a guess printed in the same sentence as the real answers,
// and in a repository with no package.json at all it was confidently wrong.
func detectPackageManager(source string) string {
	if source == "" {
		return ""
	}
	if declared := declaredPackageManager(source); declared != "" {
		return declared
	}
	for _, entry := range lockfiles {
		if _, err := os.Stat(filepath.Join(source, entry.file)); err == nil {
			return entry.manager
		}
	}
	return ""
}

// declaredPackageManager reads Corepack's field, which carries a version the
// name has to be split from: `bun@1.2.3`.
func declaredPackageManager(source string) string {
	raw, err := os.ReadFile(filepath.Join(source, "package.json"))
	if err != nil {
		return ""
	}
	var pkg struct {
		PackageManager string `json:"packageManager"`
	}
	if json.Unmarshal(raw, &pkg) != nil {
		return ""
	}

	name, _, _ := strings.Cut(strings.TrimSpace(pkg.PackageManager), "@")
	if _, known := installCommands[strings.ToLower(name)]; known {
		return strings.ToLower(name)
	}
	return ""
}

func detectTestRunner(source string) string {
	if source == "" {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(source, "package.json"))
	if err != nil {
		return ""
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if json.Unmarshal(raw, &pkg) != nil {
		return ""
	}
	declared := func(name string) bool {
		if _, ok := pkg.Dependencies[name]; ok {
			return true
		}
		_, ok := pkg.DevDependencies[name]
		return ok
	}
	switch {
	case declared("vitest"):
		return "vitest"
	case declared("jest"), declared("jest-expo"):
		return "jest"
	}
	return ""
}

// Binary is a resolved way to invoke a wrapped tool.
type Binary struct {
	Tool  string
	Name  string
	Args  []string
	Local bool
}

// Command turns a resolved binary plus tool arguments into one invocation.
func (b Binary) Command(root string, args ...string) runner.Command {
	return runner.Command{
		Label: b.Tool,
		Name:  b.Name,
		Args:  append(append([]string{}, b.Args...), args...),
		Dir:   root,
	}
}

// Resolve prefers the copy installed in the project.
//
// A locally installed tool is faster, works offline, and does not change under
// you mid-task — which matters because the gate runs on every commit and a new
// release can change severities. The remote form is a fallback for a project
// that has not installed the tool, not the intended path.
func (p Project) Resolve(tool string) Binary {
	if p.HasSource() {
		base := filepath.Join(p.Source, "node_modules", ".bin", tool)

		candidates := []string{base}
		if runtime.GOOS == "windows" {
			candidates = []string{base + ".cmd", base + ".exe", base}
		}
		for _, candidate := range candidates {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return Binary{Tool: tool, Name: candidate, Local: true}
			}
		}
	}

	exec, ok := remoteExec[p.PackageManager]
	if !ok {
		exec = remoteExec["npm"]
	}
	return Binary{Tool: tool, Name: exec[0], Args: append(append([]string{}, exec[1:]...), LatestSpec(tool))}
}

// strykerConfigFiles are the names Stryker itself looks for.
var strykerConfigFiles = []string{
	"stryker.config.mjs",
	"stryker.config.cjs",
	"stryker.config.js",
	"stryker.config.json",
	"stryker.conf.mjs",
	"stryker.conf.js",
	"stryker.conf.json",
	".stryker.conf.json",
}

// HasStrykerConfig reports whether the project configured Stryker itself.
//
// It decides one thing: whether dharness supplies --testRunner. Command line
// arguments overrule the config file, so passing it unconditionally would let a
// detection mistake override a correct decision the project already made. When
// the project has said nothing, something has to.
func (p Project) HasStrykerConfig() bool {
	for _, name := range strykerConfigFiles {
		if _, err := os.Stat(filepath.Join(p.Source, name)); err == nil {
			return true
		}
	}
	return false
}

// fallowConfigFiles are the names fallow itself looks for.
var fallowConfigFiles = []string{".fallowrc.json", ".fallowrc.jsonc", "fallow.toml"}

// HasFallowConfig reports whether fallow has been told where this project's
// graph starts.
//
// fallow needs no configuration in a conventional layout. It needs it badly in
// an unconventional one: with the wrong entry points the graph is rooted in the
// wrong place and most of the project reports as unreachable.
func (p Project) HasFallowConfig() bool {
	for _, name := range fallowConfigFiles {
		if _, err := os.Stat(filepath.Join(p.Source, name)); err == nil {
			return true
		}
	}
	return false
}

// hookFiles are where the two hook managers keep their pre-commit definition.
var hookFiles = []string{
	"lefthook.yml",
	"lefthook.yaml",
	".lefthook.yml",
	".lefthook.yaml",
	filepath.Join(".husky", "pre-commit"),
}

// HookWired reports whether some hook already invokes the gate.
//
// It looks for the invocation rather than parsing the file, because dharness
// does not own these files and has no business understanding their structure.
// It appears in exactly one line of them, and whether that line is present is
// the entire question.
//
// This is also what makes re-running safe over time: a hook that was removed,
// renamed or rewritten stops matching, and the step reappears on its own.
func (p Project) HookWired() bool {
	for _, name := range hookFiles {
		contents, err := os.ReadFile(filepath.Join(p.Root, name))
		if err == nil && strings.Contains(string(contents), "dharness check") {
			return true
		}
	}
	return false
}

// InstallCommand is how this project's package manager adds dev dependencies.
func (p Project) InstallCommand() string {
	if command, ok := installCommands[p.PackageManager]; ok {
		return command
	}
	return installCommands["npm"]
}

// RemoteExec is how this project's package manager runs a package it has not
// installed — bunx, pnpm dlx, npx.
func (p Project) RemoteExec() string {
	exec, ok := remoteExec[p.PackageManager]
	if !ok {
		exec = remoteExec["npm"]
	}
	return strings.Join(exec, " ")
}

// runnerPlugins maps a detected test runner to the Stryker plugin that drives it.
var runnerPlugins = map[string]string{
	"vitest": "@stryker-mutator/vitest-runner",
	"jest":   "@stryker-mutator/jest-runner",
}

// installCommands is how each package manager adds development dependencies.
var installCommands = map[string]string{
	"bun":  "bun add -d",
	"pnpm": "pnpm add -D",
	"yarn": "yarn add -D",
	"npm":  "npm install --save-dev",
}

// MissingStrykerRunnerError reports a project that can be mutated in principle
// but has no runner plugin installed.
//
// This is the one job `stryker init` did that flags cannot replace, so the
// error carries the exact command that does it instead.
type MissingStrykerRunnerError struct {
	TestRunner string
	Plugin     string
	Install    string
}

func (e *MissingStrykerRunnerError) Error() string {
	if e.TestRunner == "" {
		return "no test runner found in package.json; Stryker needs vitest or jest to drive the tests"
	}
	return fmt.Sprintf(
		"Stryker cannot drive %s without its runner plugin; install it with: %s @stryker-mutator/core %s",
		e.TestRunner, e.Install, e.Plugin,
	)
}

// StrykerRunner returns the value for --testRunner, or an empty string when the
// project's own configuration already answers it.
//
//nolint:revive // the two return values answer two different questions.
func (p Project) StrykerRunner() (string, error) {
	plugin, known := runnerPlugins[p.TestRunner]
	if !known {
		return "", &MissingStrykerRunnerError{TestRunner: p.TestRunner}
	}

	if _, err := os.Stat(filepath.Join(p.Source, "node_modules", plugin)); err != nil {
		install, ok := installCommands[p.PackageManager]
		if !ok {
			install = installCommands["npm"]
		}
		return "", &MissingStrykerRunnerError{TestRunner: p.TestRunner, Plugin: plugin, Install: install}
	}

	if p.HasStrykerConfig() {
		return "", nil
	}
	return p.TestRunner, nil
}

// IsSourceFile reports whether a repository path is something the wrapped
// tools can analyse.
func IsSourceFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ts", ".tsx", ".js", ".jsx", ".mts", ".cts", ".mjs", ".cjs":
		return true
	}
	return false
}
