// Package project resolves what dharness needs to know about a repository:
// which package manager runs it, which test runner it uses, and where each
// wrapped tool runs.
//
// Detection is deliberately shallow: lockfile names, package.json dependencies,
// and the one Stryker config field needed to provision its remote runner. The
// narrow config boundary keeps the project's explicit testRunner authoritative.
package project

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	YarnPnP        bool

	// InRepository reports whether Root came from git or is merely the
	// directory dharness was run in. Detection records the answer; deciding
	// what a missing repository means belongs to the command that needs one.
	InRepository bool
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

// Describe inspects a directory that is both the repository and the JS project,
// which is what a conventional layout looks like. Discover is the entry point
// that tells the two apart.
func Describe(root string) Project { return At(root, root) }

// At describes a repository whose JS project lives in source. It never fails:
// an undetected field is empty, and callers decide whether that matters.
func At(root, source string) Project {
	packageManager := detectPackageManager(source)
	return Project{
		Root:           root,
		Source:         source,
		PackageManager: packageManager,
		TestRunner:     detectTestRunner(source),
		YarnPnP:        packageManager == "yarn" && detectYarnPnP(source),
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
	if knownPackageManager(strings.ToLower(name)) {
		return strings.ToLower(name)
	}
	return ""
}

func knownPackageManager(name string) bool {
	switch name {
	case "bun", "pnpm", "yarn", "npm":
		return true
	default:
		return false
	}
}

func detectTestRunner(source string) string {
	runners := detectTestRunners(source)
	if len(runners) == 1 {
		return runners[0]
	}
	return ""
}

// declaredPackages reads both dependency lists as one set of names.
//
// Which list a package sits in is not a question dharness has an opinion
// about: it asks whether the project already decided on this package at all.
func declaredPackages(source string) map[string]string {
	if source == "" {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(source, "package.json"))
	if err != nil {
		return nil
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if json.Unmarshal(raw, &pkg) != nil {
		return nil
	}

	all := make(map[string]string, len(pkg.Dependencies)+len(pkg.DevDependencies))
	maps.Copy(all, pkg.Dependencies)
	maps.Copy(all, pkg.DevDependencies)
	return all
}

// Declares reports whether the project's manifest already names a package.
//
// It is the question §05 turns on. A project that declared a package chose its
// version — an exact pin most deliberately of all — and installing that package
// at @latest would rewrite that choice into a caret range. Measured against a
// real repository on 2026-08-13: "9.6.1" came back as "^9.6.1", which in a
// mutation engine can move the verdict over a tree nobody touched.
func (p Project) Declares(name string) bool {
	_, ok := declaredPackages(p.Source)[name]
	return ok
}

func detectTestRunners(source string) []string {
	packages := declaredPackages(source)
	declared := func(name string) bool {
		_, ok := packages[name]
		return ok
	}

	var runners []string
	if declared("vitest") {
		runners = append(runners, "vitest")
	}
	if declared("jest") || declared("jest-expo") {
		runners = append(runners, "jest")
	}
	return runners
}

// LocalBinary returns the installed path of a project helper, when present.
// Wrapped CLI resolution lives in internal/tool and never calls this function.
func (p Project) LocalBinary(binary string) string {
	if p.HasSource() {
		base := filepath.Join(p.Source, "node_modules", ".bin", binary)

		candidates := []string{base}
		if runtime.GOOS == "windows" {
			candidates = []string{base + ".cmd", base + ".exe", base}
		}
		for _, candidate := range candidates {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
		}
	}
	return ""
}

// strykerConfigFiles are the names Stryker itself looks for.
var strykerConfigFiles = []string{
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

func (p Project) strykerConfigFile() string {
	for _, name := range strykerConfigFiles {
		if _, err := os.Stat(filepath.Join(p.Source, name)); err == nil {
			return name
		}
	}
	return ""
}

func detectYarnPnP(source string) bool {
	for _, name := range []string{".pnp.cjs", ".pnp.loader.mjs"} {
		if info, err := os.Stat(filepath.Join(source, name)); err == nil && !info.IsDir() {
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

// PackageStateFiles are the manifest and lockfiles the detected package manager
// may rewrite. Setup snapshots them before an install so rollback is byte exact.
func (p Project) PackageStateFiles() []string {
	manager := p.PackageManager
	if !knownPackageManager(manager) {
		manager = "npm"
	}

	files := []string{filepath.Join(p.Source, "package.json")}
	for _, entry := range lockfiles {
		if entry.manager == manager {
			files = append(files, filepath.Join(p.Source, entry.file))
		}
	}
	return files
}

// StrykerSelection identifies the runner Core must load and whether the
// project's config owns the corresponding testRunner argument.
type StrykerSelection struct {
	TestRunner    string
	Configured    bool
	AppendPlugins []string
}

// StrykerRunnerError reports a runner selection that cannot be translated into
// a safe transient Core-plus-runner environment.
type StrykerRunnerError struct{ message string }

func (e *StrykerRunnerError) Error() string { return e.message }

// StrykerRunner selects the supported runner package that must accompany remote
// Core. JSON configs are read only for testRunner; executable configs are left
// untouched and rejected because evaluating or approximating them would make
// dharness an interpreter for project-owned configuration.
func (p Project) StrykerRunner() (StrykerSelection, error) {
	if config := p.strykerConfigFile(); config != "" {
		if filepath.Ext(config) != ".json" {
			return StrykerSelection{}, &StrykerRunnerError{message: fmt.Sprintf(
				"Stryker cannot safely determine testRunner from %s; use a JSON Stryker config with testRunner set to vitest or jest",
				config,
			)}
		}

		raw, err := os.ReadFile(filepath.Join(p.Source, config))
		if err != nil {
			return StrykerSelection{}, &StrykerRunnerError{message: fmt.Sprintf("Stryker cannot read %s: %v; fix the config and retry", config, err)}
		}
		var configured struct {
			TestRunner    string   `json:"testRunner"`
			AppendPlugins []string `json:"appendPlugins"`
		}
		if err := json.Unmarshal(raw, &configured); err != nil {
			return StrykerSelection{}, &StrykerRunnerError{message: fmt.Sprintf("Stryker cannot read testRunner from %s: %v; fix the JSON config and retry", config, err)}
		}
		if configured.TestRunner == "" {
			return StrykerSelection{}, &StrykerRunnerError{message: fmt.Sprintf("Stryker config %s must set testRunner to vitest or jest so its remote runner can be provisioned", config)}
		}
		if !supportedStrykerRunner(configured.TestRunner) {
			return StrykerSelection{}, &StrykerRunnerError{message: fmt.Sprintf("Stryker config %s selects unsupported testRunner %q; dharness supports vitest or jest", config, configured.TestRunner)}
		}
		return StrykerSelection{TestRunner: configured.TestRunner, Configured: true, AppendPlugins: configured.AppendPlugins}, nil
	}

	runners := detectTestRunners(p.Source)
	if len(runners) == 0 && p.TestRunner != "" {
		runners = []string{p.TestRunner}
	}
	switch len(runners) {
	case 0:
		return StrykerSelection{}, &StrykerRunnerError{message: "Stryker found no supported test runner in package.json; declare vitest or jest, or set testRunner in a JSON Stryker config"}
	case 1:
		if !supportedStrykerRunner(runners[0]) {
			return StrykerSelection{}, &StrykerRunnerError{message: fmt.Sprintf("Stryker cannot provision unsupported test runner %q; use vitest or jest", runners[0])}
		}
		return StrykerSelection{TestRunner: runners[0]}, nil
	default:
		return StrykerSelection{}, &StrykerRunnerError{message: "Stryker found both vitest and jest in package.json; set testRunner to vitest or jest in a JSON Stryker config"}
	}
}

func supportedStrykerRunner(testRunner string) bool {
	return testRunner == "vitest" || testRunner == "jest"
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
