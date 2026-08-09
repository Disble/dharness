package setup

import (
	"encoding/json"
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

// ---------------------------------------------------------------- install

type installStep struct{}

func (installStep) ID() string { return "install what this project is missing" }

func (installStep) Satisfied(p project.Project) bool { return len(missing(p)) == 0 }

func (s installStep) Describe(p project.Project) string {
	return fmt.Sprintf("Without them every run fetches over the network, inside a gate that runs on\nevery commit.\n\n    %s %s",
		p.InstallCommand(), strings.Join(missing(p), " "))
}

func (installStep) Apply(p project.Project, _ *Writer) error {
	packages := missing(p)
	if len(packages) == 0 {
		return nil
	}

	command := strings.Fields(p.InstallCommand())
	return runner.Run(runner.Command{
		Label: command[0],
		Name:  command[0],
		Args:  append(command[1:], packages...),
		Dir:   p.Root,
	}, os.Stdout, os.Stderr)
}

// missing lists the packages this project needs and does not have.
func missing(p project.Project) []string {
	var packages []string
	for _, name := range []string{tool.ReactDoctor, tool.Fallow, tool.Stryker} {
		if !p.Resolve(name).Local {
			packages = append(packages, project.Package(name))
		}
	}

	var runnerErr *project.MissingStrykerRunnerError
	if _, err := p.StrykerRunner(); asMissingRunner(err, &runnerErr) && runnerErr.Plugin != "" {
		packages = append(packages, runnerErr.Plugin)
	}
	if !installed(p, RulesPackage) {
		packages = append(packages, RulesPackage)
	}
	return dedupe(packages)
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

func (ownedFilesStep) Apply(p project.Project, w *Writer) error {
	// EnsureDir also writes the ignore rules, so a transient file appearing
	// later is never the first thing to create them.
	if _, err := p.EnsureDir(""); err != nil {
		return err
	}

	gate := fmt.Sprintf("pre-commit:\n  commands:\n    dharness:\n      run: %s\n", gateCommand)
	if err := w.Write(filepath.Join(p.Root, project.Dir, ownedLefthook), []byte(gate)); err != nil {
		return err
	}

	// The boundaries block is deliberately absent: zones encode intent, and
	// no detection can read intent off a tree. The model fills this in.
	architecture := "{\n  // dharness writes this file; the architecture below is decided by analysis,\n  // not by detection. See `dharness init`.\n}\n"
	if err := w.Write(filepath.Join(p.Root, project.Dir, ownedFallow), []byte(architecture)); err != nil {
		return err
	}

	return w.WriteJSON(filepath.Join(p.Root, project.Dir, ownedRules), DefaultThresholds())
}

// ------------------------------------------------------------- extends

type extendsStep struct{}

func (extendsStep) ID() string { return "point the project's config at the files dharness owns" }

func (extendsStep) Satisfied(p project.Project) bool {
	return extendsWired(p, fallowConfig, filepath.ToSlash(filepath.Join(project.Dir, ownedFallow))) &&
		(hookManager(p) != managerLefthook ||
			extendsWired(p, lefthookConfig, filepath.ToSlash(filepath.Join(project.Dir, ownedLefthook))))
}

func (extendsStep) Describe(project.Project) string {
	return "Both tools compose with `extends`, so the gate and the architecture arrive by\nreference. Nothing of the project's own configuration is rewritten."
}

func (extendsStep) Apply(p project.Project, w *Writer) error {
	if err := wireFallowExtends(p, w); err != nil {
		return err
	}
	if hookManager(p) == managerLefthook {
		return wireLefthookExtends(p, w)
	}
	return nil
}

// -------------------------------------------------------- doctor config

type doctorConfigStep struct{}

func (doctorConfigStep) ID() string { return "declare the rules react-doctor should run" }

func (doctorConfigStep) Satisfied(p project.Project) bool {
	raw, err := os.ReadFile(filepath.Join(p.Root, doctorConfig))
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

func (doctorConfigStep) Apply(p project.Project, w *Writer) error {
	path := filepath.Join(p.Root, doctorConfig)

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
			config.Rules[id] = "error"
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

	// The binary ships with fallow, so it is reached the way the project
	// reaches everything else it installed rather than from a global PATH.
	command := strings.Fields(p.RemoteExec())
	config.Servers["fallow"] = mcpServer{
		Command: command[0],
		Args:    append(command[1:], "fallow-mcp"),
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
		// Nothing answers, so there is nothing to satisfy or to do. Choosing a
		// hook manager for a project is not a default dharness gets to pick.
		return true
	}
}

func (hookInstallStep) Describe(p project.Project) string {
	if hookManager(p) == managerHusky {
		return fmt.Sprintf("husky keeps a shell script, so the gate is one appended line in %s.", huskyHook)
	}
	return "lefthook writes the git hook itself; without running it the configuration\nexists and nothing calls it."
}

func (hookInstallStep) Apply(p project.Project, w *Writer) error {
	if hookManager(p) == managerHusky {
		return appendHuskyGate(p, w)
	}

	binary := p.Resolve("lefthook")
	return runner.Run(binary.Command(p.Root, "install"), os.Stdout, os.Stderr)
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
	return fmt.Sprintf("    %s %s install\n\nChoose the skill and decline the rest.", p.RemoteExec(), project.LatestSpec(tool.ReactDoctor))
}

func (agentSkillStep) Why() string {
	return "its only non-interactive form installs five things: skills for every agent it\ndetects, a package script, a git hook that competes with this gate, and a CI\nworkflow. There is no flag to ask for the skill alone."
}

func (agentSkillStep) Apply(project.Project, *Writer) error {
	return fmt.Errorf("%s is delegated and must not be applied", agentSkillStep{}.ID())
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
