package setup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/runner"
	"github.com/Disble/dharness/internal/tool"
)

// gateCommand is the single line every hook manager ends up invoking, which is
// what keeps lefthook and husky from becoming a variable anywhere else.
const gateCommand = "dharness check"

// gateConfig is the lefthook configuration dharness owns.
//
// The `root` key is lefthook's own, and it is the whole reason dharness does
// not have to solve this itself: the hook is installed at the repository root
// because that is where git looks for it, but in a split layout the tools have
// to run where the package manager installed them. lefthook already changes
// directory per command, so dharness names the directory and stops there.
//
// It is emitted only when the two roots really differ. A key that says `root:
// ./` in a conventional repository is noise in a file people read.
func gateConfig(p project.Project) string {
	gate := fmt.Sprintf("pre-commit:\n  commands:\n    dharness:\n      run: %s\n", gateCommand)
	if rel := p.SourceRel(); rel != "" {
		gate += fmt.Sprintf("      root: %s/\n", rel)
	}
	return gate
}

// ---------------------------------------------------------------- install

type installStep struct{}

func (installStep) ID() string { return "install what this project is missing" }

// Satisfied answers the only question dharness can answer on its own: is there
// a JS project here at all. Whether the package is already there is the
// package manager's question, and every way of asking it from outside is
// wrong. A directory under node_modules is an install artifact — it survives a
// rollback that restored package.json, and Yarn PnP and pnpm's store often
// leave none at all. A name in package.json is a declaration nobody has
// necessarily acted on. Reading a lockfile means parsing four formats to
// re-derive an answer the install command returns for free.
//
// So dharness does not ask. It runs the install, which is idempotent in all
// four managers, and the exit code is the verdict.
func (installStep) Satisfied(p project.Project) bool {
	return !p.HasSource()
}

func (s installStep) Describe(p project.Project) string {
	return fmt.Sprintf("This package provides dharness's project lint rules.\n\n    %s %s",
		tool.InstallCommand(p.PackageManager), strings.Join(integrationPackages(), " "))
}

// Delegated is always false: there is no repository state that hands
// installing a package to the agent instead of dharness.
func (installStep) Delegated(project.Project) (string, bool) { return "", false }

func (installStep) Apply(p project.Project, w *Writer) error {
	packages := integrationPackages()

	for _, path := range p.PackageStateFiles() {
		if err := w.remember(path); err != nil {
			return fmt.Errorf("snapshot package state before install: %w", err)
		}
	}

	installErr := runner.Run(tool.InstallPackages(p.PackageManager, p.Source, packages), os.Stdout, os.Stderr)
	w.compensate(func() error {
		if err := runner.Run(tool.RemovePackages(p.PackageManager, p.Source, packages), os.Stdout, os.Stderr); err != nil {
			return fmt.Errorf("remove integration packages added by this run: %w", err)
		}
		return nil
	})
	return installErr
}

// integrationPackages lists the packages dharness adds to a project, as
// opposed to the CLIs it invokes without installing.
func integrationPackages() []string {
	return dedupe([]string{RulesPackage})
}

// ---------------------------------------------------------- owned files

type ownedFilesStep struct{}

func (ownedFilesStep) ID() string { return "write the files dharness owns" }

func (ownedFilesStep) Satisfied(p project.Project) bool {
	for _, name := range []string{ownedLefthook, ownedFallow, ownedRules} {
		if _, err := os.Stat(filepath.Join(p.Root, project.Dir, name)); err != nil {
			return false
		}
	}
	return true
}

func (ownedFilesStep) Describe(project.Project) string {
	return fmt.Sprintf("The gate, the architecture and the rule thresholds live in %s/, which is\ncommitted. The project's own files only ever gain one line pointing at them.",
		project.Dir)
}

// Delegated is always false: the files dharness owns are always dharness's to
// write.
func (ownedFilesStep) Delegated(project.Project) (string, bool) { return "", false }

func (ownedFilesStep) Apply(p project.Project, w *Writer) error {
	// EnsureDir also writes the ignore rules, so a transient file appearing
	// later is never the first thing to create them.
	if _, err := p.EnsureDir(""); err != nil {
		return err
	}

	if err := w.Write(filepath.Join(p.Root, project.Dir, ownedLefthook), []byte(gateConfig(p))); err != nil {
		return err
	}

	// The boundaries block is deliberately absent: zones encode intent, and
	// no detection can read intent off a tree. The model fills this in.
	// The boundaries block goes here and nowhere else: fallow's `extends`
	// replaces the key rather than merging it, so the same block in the
	// project's own config would silently discard this one.
	architecture := "{\n  // dharness writes this file; the architecture below is decided by analysis,\n  // not by detection. Declare `boundaries` here rather than in the project's\n  // own fallow config: `extends` replaces this key, it does not merge it.\n  //\n  // See `dharness sync`.\n}\n"
	if err := w.Write(filepath.Join(p.Root, project.Dir, ownedFallow), []byte(architecture)); err != nil {
		return err
	}

	return w.WriteJSON(filepath.Join(p.Root, project.Dir, ownedRules), DefaultThresholds())
}

// ------------------------------------------------------------- extends
//
// fallowExtendsStep and lefthookExtendsStep are split rather than one step
// with two targets, because the two targets can have two different
// recipients: the project owns .fallowrc.json but may have no lefthook.yml
// at all, and a single Delegated answer cannot speak for both files at once.

type fallowExtendsStep struct{}

func (fallowExtendsStep) ID() string {
	return fmt.Sprintf("point %s at the file dharness owns", fallowConfig)
}

func (fallowExtendsStep) Satisfied(p project.Project) bool {
	return !p.HasSource() || extendsWired(p.Source, fallowConfig, ownedFrom(p, p.Source, ownedFallow))
}

func (fallowExtendsStep) Describe(p project.Project) string {
	target := ownedFrom(p, p.Source, ownedFallow)
	return fmt.Sprintf(
		"fallow composes with `extends`, so the architecture arrives by reference.\nAdd this line to %s:\n\n    \"extends\": [%q]",
		fallowConfig, target)
}

// Delegated returns ok == true only when the project's own config already
// exists: adding a key to it is then a merge, not a write dharness gets to
// make on its own. With no config present, dharness writes the whole file.
func (fallowExtendsStep) Delegated(p project.Project) (string, bool) {
	if !p.HasSource() {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(p.Source, fallowConfig)); errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	return fmt.Sprintf(
		"%s already exists and belongs to the project; adding a key to it is a merge,\nnot a write.",
		fallowConfig), true
}

func (fallowExtendsStep) Apply(p project.Project, w *Writer) error {
	return wireFallowExtends(p, w)
}

type lefthookExtendsStep struct{}

func (lefthookExtendsStep) ID() string {
	return fmt.Sprintf("point %s at the file dharness owns", lefthookConfig)
}

func (lefthookExtendsStep) Satisfied(p project.Project) bool {
	return hookManager(p) != managerLefthook ||
		extendsWired(p.Root, lefthookConfig, ownedFrom(p, p.Root, ownedLefthook))
}

func (lefthookExtendsStep) Describe(p project.Project) string {
	target := ownedFrom(p, p.Root, ownedLefthook)
	return fmt.Sprintf(
		"lefthook composes with `extends`, so the gate arrives by reference.\nAdd this line to %s:\n\n    extends:\n      - %s",
		lefthookConfig, target)
}

// Delegated follows the same rule as fallowExtendsStep, for the project's own
// lefthook.yml instead of .fallowrc.json.
func (lefthookExtendsStep) Delegated(p project.Project) (string, bool) {
	if hookManager(p) != managerLefthook {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(p.Root, lefthookConfig)); errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	return fmt.Sprintf(
		"%s already exists and belongs to the project; adding a key to it is a merge,\nnot a write.",
		lefthookConfig), true
}

func (lefthookExtendsStep) Apply(p project.Project, w *Writer) error {
	return wireLefthookExtends(p, w)
}

// --------------------------------------------------- boundaries owner
//
// fallow's `extends` replaces a key rather than merging it. Measured against
// fallow 3.14.0: a parent declaring `boundaries` is honoured until the child
// declares its own, and from then on the parent's block is discarded whole —
// no error, no warning, and the `extends` line still reads as correct.
//
// That makes it the one way dharness's architecture can stop being enforced
// while everything else still looks wired, which is why it gets a step rather
// than a line in another step's Describe.

type boundariesOwnerStep struct{}

func (boundariesOwnerStep) ID() string {
	return "resolve the two architectures this project declares"
}

func (boundariesOwnerStep) Satisfied(p project.Project) bool {
	return !p.HasSource() || !declaresBoundaries(filepath.Join(p.Source, fallowConfig))
}

func (boundariesOwnerStep) Describe(p project.Project) string {
	return fmt.Sprintf(
		"Move the zones and rules from %s into %s, or delete the block dharness\nowns and keep the project's. Either is a valid answer; having both is not,\nbecause only one of them runs and the file gives no sign of which.",
		fallowConfig, filepath.ToSlash(filepath.Join(project.Dir, ownedFallow)))
}

// Delegated always returns ok == true where the step is unsatisfied: two
// architectures cannot be merged by a rule. Which zones survive is a decision
// about intent, and dharness does not hold it.
func (boundariesOwnerStep) Delegated(project.Project) (string, bool) {
	return fmt.Sprintf(
		"%s declares its own `boundaries`, and fallow's `extends` replaces that key\nrather than merging it — the project's block replaces the one dharness owns\nentirely, without an error. Only one architecture is being enforced, and the\nconfiguration does not say which.",
		fallowConfig), true
}

// Apply is unreachable: Delegated always answers ok == true, so applySteps
// never calls it. Kept as a contract assertion, matching architectureStep.
func (boundariesOwnerStep) Apply(project.Project, *Writer) error {
	return fmt.Errorf("%s is delegated and must not be applied", boundariesOwnerStep{}.ID())
}

// -------------------------------------------------------- doctor config

type doctorConfigStep struct{}

func (doctorConfigStep) ID() string { return "declare the rules react-doctor should run" }

func (doctorConfigStep) Satisfied(p project.Project) bool {
	if !p.HasSource() {
		return true
	}
	raw, err := os.ReadFile(filepath.Join(p.Source, doctorConfig))
	if err != nil {
		return false
	}
	var config doctorConfigFile
	if json.Unmarshal(raw, &config) != nil {
		return false
	}
	for _, plugin := range config.Plugins {
		if plugin == RulesPackage {
			return true
		}
	}
	return false
}

func (doctorConfigStep) Describe(project.Project) string {
	return fmt.Sprintf("react-doctor does not compose, so this is a merge rather than a reference: the\npackage joins `plugins` and its %d rules join `rules`.", len(Rules))
}

// Delegated is always false: the merge doctorConfigStep performs has no case
// where dharness cannot perform it itself.
func (doctorConfigStep) Delegated(project.Project) (string, bool) { return "", false }

func (doctorConfigStep) Apply(p project.Project, w *Writer) error {
	path := filepath.Join(p.Source, doctorConfig)

	config := doctorConfigFile{Rules: map[string]string{}}
	if raw, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(raw, &config); err != nil {
			return fmt.Errorf("%s is not readable as JSON, so it cannot be merged: %w", doctorConfig, err)
		}
		if config.Rules == nil {
			config.Rules = map[string]string{}
		}
	}

	config.Plugins = dedupe(append(config.Plugins, RulesPackage))
	for _, id := range RuleIDs() {
		if _, chosen := config.Rules[id]; !chosen {
			config.Rules[id] = DefaultSeverity(id)
		}
	}
	return w.WriteJSON(path, config)
}

type doctorConfigFile struct {
	Plugins []string          `json:"plugins,omitempty"`
	Rules   map[string]string `json:"rules,omitempty"`
}

// ----------------------------------------------------------------- mcp

type mcpStep struct{}

func (mcpStep) ID() string { return "give the agent fallow's own tools" }

func (mcpStep) Satisfied(p project.Project) bool {
	raw, err := os.ReadFile(filepath.Join(p.Root, mcpConfig))
	if err != nil {
		return false
	}
	var config mcpConfigFile
	if json.Unmarshal(raw, &config) != nil {
		return false
	}
	_, registered := config.Servers["fallow"]
	return registered
}

func (mcpStep) Describe(project.Project) string {
	return "fallow ships an MCP server with the analysis an agent would otherwise ask a\nwrapper to invent: boundaries, traces, impact, health."
}

// Delegated is always false: the MCP entry dharness writes has no case where
// it belongs to the project instead.
func (mcpStep) Delegated(project.Project) (string, bool) { return "", false }

func (mcpStep) Apply(p project.Project, w *Writer) error {
	path := filepath.Join(p.Root, mcpConfig)

	config := mcpConfigFile{Servers: map[string]mcpServer{}}
	if raw, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(raw, &config); err != nil {
			return fmt.Errorf("%s is not readable as JSON, so it cannot be merged: %w", mcpConfig, err)
		}
		if config.Servers == nil {
			config.Servers = map[string]mcpServer{}
		}
	}

	// The binary ships with fallow, so the remote executor names that package
	// explicitly rather than asking the registry for a package named fallow-mcp.
	binary := tool.RemotePackageBinary(p.PackageManager, tool.LatestSpec(tool.Fallow), "fallow-mcp", p.Source)
	config.Servers["fallow"] = mcpServer{
		Command: binary.Name,
		Args:    binary.Args,
	}
	return w.WriteJSON(path, config)
}

type mcpConfigFile struct {
	Servers map[string]mcpServer `json:"mcpServers"`
}

type mcpServer struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

// --------------------------------------------------------- hook install

type hookInstallStep struct{}

func (hookInstallStep) ID() string { return "wire the gate into git" }

func (hookInstallStep) Satisfied(p project.Project) bool {
	switch hookManager(p) {
	case managerLefthook:
		return gateInstalled(p)
	case managerHusky:
		return huskyWired(p)
	default:
		// Nothing answers, so there is nothing dharness can satisfy on its
		// own. This is an open decision, not a default dharness gets to
		// pick — see Delegated.
		return false
	}
}

func (hookInstallStep) Describe(p project.Project) string {
	switch hookManager(p) {
	case managerHusky:
		return fmt.Sprintf("husky keeps a shell script, so the gate is one appended line in %s.", huskyHook)
	case managerLefthook:
		return "lefthook writes the git hook itself; without running it the configuration\nexists and nothing calls it."
	default:
		return fmt.Sprintf(
			"Choose a hook manager and wire it to run the gate:\n\n    %s\n\nlefthook writes its own git hook once configured and installed; husky keeps\na shell script that needs one appended line. Either way, the step is\nsatisfied once %q runs on commit.",
			gateCommand, gateCommand)
	}
}

// Delegated per manager: lefthook and husky both answer, so dharness installs
// the gate itself. No manager answering is Decision 5's open decision — it is
// not dharness's default to pick.
func (hookInstallStep) Delegated(p project.Project) (string, bool) {
	switch hookManager(p) {
	case managerLefthook, managerHusky:
		return "", false
	default:
		return "nothing answers: there is no lefthook config, no .husky/ and no lefthook\nbinary. Choosing a hook manager is a decision this project has not made, and\nnot a default dharness gets to pick.", true
	}
}

func (hookInstallStep) Apply(p project.Project, w *Writer) error {
	if hookManager(p) == managerHusky {
		return appendHuskyGate(p, w)
	}

	path := p.LocalBinary("lefthook")
	command := tool.RemoteLatest(p.PackageManager, "lefthook", p.Root, "install")
	if path != "" {
		command = tool.Installed("lefthook", path, p.Root, "install")
	}
	return runner.Run(command, os.Stdout, os.Stderr)
}

// --------------------------------------------------------- agent skill

type agentSkillStep struct{}

func (agentSkillStep) ID() string { return "install react-doctor's agent skill" }

func (agentSkillStep) Satisfied(p project.Project) bool {
	// The installer writes into whichever agents it detects, so the honest
	// check is whether any of the places it would write already carries it.
	for _, candidate := range skillLocations {
		if _, err := os.Stat(filepath.Join(p.Root, candidate)); err == nil {
			return true
		}
	}
	return false
}

func (agentSkillStep) Describe(p project.Project) string {
	return fmt.Sprintf("    %s %s install\n\nChoose the skill and decline the rest.", tool.RemoteExec(p.PackageManager), tool.LatestSpec(tool.ReactDoctor))
}

// Delegated always returns ok == true: no non-interactive install exists that
// installs only the skill, so this step is always the agent's to run.
func (agentSkillStep) Delegated(project.Project) (string, bool) {
	return "its only non-interactive form installs five things: skills for every agent it\ndetects, a package script, a git hook that competes with this gate, and a CI\nworkflow. There is no flag to ask for the skill alone.", true
}

// Apply is unreachable: Delegated always answers ok == true, so applySteps
// never calls it. Kept as a contract assertion — see TestAgentSkillApplyIsUnreachable.
func (agentSkillStep) Apply(project.Project, *Writer) error {
	return fmt.Errorf("%s is delegated and must not be applied", agentSkillStep{}.ID())
}

// -------------------------------------------------------- architecture

type architectureStep struct{}

func (architectureStep) ID() string { return "decide this project's architecture" }

// Satisfied follows the extendsWired precedent: a substring check on the
// literal text, not a JSONC parse. The product is stdlib-only, and "does the
// file declare a boundaries block" does not need a parser to answer.
func (architectureStep) Satisfied(p project.Project) bool {
	return declaresBoundaries(filepath.Join(p.Root, project.Dir, ownedFallow))
}

func (architectureStep) Describe(p project.Project) string {
	return ArchitecturePrompt(p)
}

// Delegated always returns ok == true: this is Intención, and no detection
// tells dharness what a project's architecture is meant to be.
func (architectureStep) Delegated(project.Project) (string, bool) {
	return "architecture boundaries say what the code is meant to be, and no tool can\nread intent off a tree. Do the analysis and write the result — nothing else\nneeds to change.", true
}

// Apply is unreachable: Delegated always answers ok == true, so applySteps
// never calls it. Kept as a contract assertion, matching agentSkillStep.
func (architectureStep) Apply(project.Project, *Writer) error {
	return fmt.Errorf("%s is delegated and must not be applied", architectureStep{}.ID())
}

func dedupe(values []string) []string {
	seen := map[string]bool{}
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			unique = append(unique, value)
		}
	}
	return unique
}
