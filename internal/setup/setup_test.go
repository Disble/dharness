package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/runner"
)

// A run that rolls back left the repository as it found it, so nothing may
// survive: neither a file that was created nor an edit to one that existed.
func TestWriterUndoRestoresEverythingItTouched(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "kept.json")
	if err := os.WriteFile(existing, []byte(`{"mine":true}`), 0o600); err != nil {
		t.Fatal(err)
	}

	w := &Writer{}
	if err := w.Write(existing, []byte(`{"overwritten":true}`)); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(filepath.Join(root, "nested", "new.yml"), []byte("added\n")); err != nil {
		t.Fatal(err)
	}

	if err := w.Undo(); err != nil {
		t.Fatalf("Undo() = %v", err)
	}

	restored, err := os.ReadFile(existing)
	if err != nil || string(restored) != `{"mine":true}` {
		t.Errorf("an existing file was not put back: %q, %v", restored, err)
	}
	if _, err := os.Stat(filepath.Join(root, "nested", "new.yml")); !os.IsNotExist(err) {
		t.Error("a created file survived the undo")
	}
}

// The snapshot is of the original, not of whatever the previous write left, so
// two writes to one file still restore what was there before either.
func TestWriterUndoKeepsTheOriginalAcrossRepeatedWrites(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	w := &Writer{}
	for _, contents := range []string{"first", "second"} {
		if err := w.Write(path, []byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Undo(); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(path)
	if string(raw) != "original" {
		t.Errorf("restored %q, want the contents from before the first write", raw)
	}
}

// A stub is enough here: the contract under test is that Apply consults
// Delegated before touching Apply at all, not any particular step's logic.
type stubDelegatedStep struct {
	applied *bool
}

func (stubDelegatedStep) ID() string                      { return "stub delegated step" }
func (stubDelegatedStep) Describe(project.Project) string { return "" }
func (stubDelegatedStep) Satisfied(project.Project) bool  { return false }

func (stubDelegatedStep) Delegated(project.Project) (string, bool) {
	return "handed to the agent", true
}

func (s stubDelegatedStep) Apply(project.Project, *Writer) error {
	*s.applied = true
	return nil
}

func TestApplySkipsEveryDelegatedStep(t *testing.T) {
	applied := false
	step := stubDelegatedStep{applied: &applied}

	if err := applySteps([]Step{step}, project.Project{}, io.Discard); err != nil {
		t.Fatalf("applySteps() = %v", err)
	}
	if applied {
		t.Error("Apply was called on a step Delegated() reported as ok == true")
	}
}

// agentSkillStep.Delegated always returns ok == true, so applySteps never
// calls its Apply. The error Apply returns is a contract assertion for the
// case that should be unreachable, not a code path any run takes.
func TestAgentSkillApplyIsUnreachable(t *testing.T) {
	if err := (agentSkillStep{}).Apply(project.Project{}, &Writer{}); err == nil {
		t.Error("agentSkillStep.Apply() = nil, want the delegated-and-must-not-be-applied assertion")
	}
}

func TestInstallStepPlansOnlyMissingIntegrationPackages(t *testing.T) {
	p, _, _ := integrationProject(t)
	want := []string{RulesPackage}

	if got := integrationPackages(); !slices.Equal(got, want) {
		t.Fatalf("integrationPackages() = %v, want %v", got, want)
	}
	description := (installStep{}).Describe(p)
	for _, wrapped := range []string{"react-doctor", "fallow", "@stryker-mutator/core", "@stryker-mutator/vitest-runner", "@stryker-mutator/jest-runner"} {
		if strings.Contains(description, wrapped) {
			t.Errorf("install description includes wrapped CLI %q:\n%s", wrapped, description)
		}
	}
	for _, integration := range want {
		if !strings.Contains(description, integration) {
			t.Errorf("install description omits integration %q:\n%s", integration, description)
		}
	}
}

func TestArchitecturePromptPinsFallowToRemoteLatest(t *testing.T) {
	prompt := (architectureStep{}).Describe(project.Project{PackageManager: "pnpm"})
	for _, invocation := range []string{
		"pnpm dlx fallow@latest list --boundaries",
		"pnpm dlx fallow@latest dead-code --boundary-violations",
	} {
		if !strings.Contains(prompt, invocation) {
			t.Errorf("architecture step description omits %q:\n%s", invocation, prompt)
		}
	}
}

// Undeclared boundaries are Intención: an open decision with no options,
// present in the plan until the agent writes the block.
func TestArchitectureStepDisappearsOnceBoundariesAreDeclared(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root}

	writeFallow(t, root, "{\n}\n")
	if (architectureStep{}).Satisfied(p) {
		t.Error("Satisfied() = true, want false: fallow.jsonc declares no boundaries yet")
	}
	if pending := Pending(p); !containsStep(pending, architectureStep{}) {
		t.Error("architectureStep is missing from Pending() while boundaries is undeclared")
	}

	writeFallow(t, root, "{\n  \"boundaries\": []\n}\n")
	if !(architectureStep{}).Satisfied(p) {
		t.Error("Satisfied() = false, want true: fallow.jsonc now declares boundaries")
	}
	if pending := Pending(p); containsStep(pending, architectureStep{}) {
		t.Error("architectureStep is still in Pending() once boundaries is declared")
	}
}

func containsStep(steps []Step, target Step) bool {
	for _, step := range steps {
		if step.ID() == target.ID() {
			return true
		}
	}
	return false
}

func writeFallow(t *testing.T, root, contents string) {
	t.Helper()
	dir := filepath.Join(root, project.Dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ownedFallow), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMCPConfigRunsTheBundledBinaryFromFallowLatest(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root, PackageManager: "pnpm"}
	if err := (mcpStep{}).Apply(p, &Writer{}); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, mcpConfig))
	if err != nil {
		t.Fatal(err)
	}
	var config mcpConfigFile
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	server := config.Servers["fallow"]
	wantArgs := []string{"--package=fallow@latest", "dlx", "fallow-mcp"}
	if server.Command != "pnpm" || !slices.Equal(server.Args, wantArgs) {
		t.Errorf("fallow MCP command = %s %v, want pnpm %v", server.Command, server.Args, wantArgs)
	}
}

func TestApplySuccessDoesNotCompensateIntegrationInstall(t *testing.T) {
	p, _, _ := integrationProject(t)
	var commands []runner.Command
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		commands = append(commands, cmd)
		return nil
	}))

	if err := Apply(p, io.Discard); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	if len(commands) != 1 {
		t.Fatalf("Apply() ran %d package commands, want only install: %+v", len(commands), commands)
	}
	want := []string{"install", "--save-dev", RulesPackage}
	if commands[0].Name != "npm" || !slices.Equal(commands[0].Args, want) {
		t.Errorf("install command = %s %v, want npm %v", commands[0].Name, commands[0].Args, want)
	}
}

func TestApplyCompensatesRulesPackageAndRestoresPackageFiles(t *testing.T) {
	p, packageJSON, lockfile := integrationProject(t)
	if err := os.WriteFile(filepath.Join(p.Root, project.Dir), []byte("blocks the next step"), 0o600); err != nil {
		t.Fatal(err)
	}

	var commands []runner.Command
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		commands = append(commands, cmd)
		switch {
		case slices.Equal(cmd.Args, []string{"install", "--save-dev", RulesPackage}):
			writePackageState(t, p.Source, "changed by install", "changed lock")
			return os.MkdirAll(filepath.Join(p.Source, "node_modules", RulesPackage), 0o755)
		case slices.Equal(cmd.Args, []string{"uninstall", "--save-dev", RulesPackage}):
			writePackageState(t, p.Source, "changed by uninstall", "changed again")
			return os.RemoveAll(filepath.Join(p.Source, "node_modules", RulesPackage))
		default:
			return errors.New("unexpected package command")
		}
	}))

	err := Apply(p, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "write the files dharness owns failed") {
		t.Fatalf("Apply() = %v, want the later step failure", err)
	}
	if len(commands) != 2 {
		t.Fatalf("Apply() ran %d package commands, want install and compensation: %+v", len(commands), commands)
	}
	assertPackageState(t, p.Source, packageJSON, lockfile)
	if _, err := os.Stat(filepath.Join(p.Source, "node_modules", RulesPackage)); !os.IsNotExist(err) {
		t.Errorf("dependency added by this run survived compensation: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p.Source, "node_modules", "pre-existing-integration")); err != nil {
		t.Errorf("pre-existing dependency was removed: %v", err)
	}
}

func TestApplyCompensatesPartialInstallFailure(t *testing.T) {
	p, packageJSON, lockfile := integrationProject(t)
	installErr := errors.New("install failed after changing files")
	var commands []runner.Command
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		commands = append(commands, cmd)
		if cmd.Args[0] == "install" {
			writePackageState(t, p.Source, "partially changed", "partial lock")
			for _, path := range []string{filepath.Join(p.Source, "node_modules", RulesPackage)} {
				if err := os.MkdirAll(path, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			return installErr
		}
		for _, path := range []string{filepath.Join(p.Source, "node_modules", RulesPackage)} {
			if err := os.RemoveAll(path); err != nil {
				t.Fatal(err)
			}
		}
		return nil
	}))

	err := Apply(p, io.Discard)
	if !errors.Is(err, installErr) {
		t.Fatalf("Apply() = %v, want install failure", err)
	}
	if len(commands) != 2 || commands[1].Args[0] != "uninstall" {
		t.Fatalf("partial install was not compensated: %+v", commands)
	}
	assertPackageState(t, p.Source, packageJSON, lockfile)
}

// Writer.Undo restores files it snapshotted, but not directories created by
// os.MkdirAll nor the .gitignore project.Project.EnsureDir writes outside the
// Writer. The rollback report must not claim more than Undo actually did.
func TestApplyRollbackNamesWhatWasUndoneNotEverything(t *testing.T) {
	p, _, _ := integrationProject(t)
	if err := os.WriteFile(filepath.Join(p.Root, project.Dir), []byte("blocks the next step"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		return nil
	}))

	err := Apply(p, io.Discard)
	if err == nil {
		t.Fatal("Apply() = nil, want the write-the-files-dharness-owns failure")
	}
	for _, overclaim := range []string{"everything this run wrote was undone", "the repository was fully restored"} {
		if strings.Contains(err.Error(), overclaim) {
			t.Errorf("Apply() error claims more than Undo covers: %q contains %q", err, overclaim)
		}
	}
	if !strings.Contains(err.Error(), "put back") {
		t.Errorf("Apply() error does not say what was undone: %q", err)
	}
}

func TestApplyReportsTheOriginalAndCompensationFailures(t *testing.T) {
	p, _, _ := integrationProject(t)
	installErr := errors.New("install failed")
	compensationErr := errors.New("uninstall failed")
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		if cmd.Args[0] == "install" {
			return installErr
		}
		return compensationErr
	}))

	err := Apply(p, io.Discard)
	if !errors.Is(err, installErr) || !errors.Is(err, compensationErr) {
		t.Fatalf("Apply() = %v, want both install and compensation failures", err)
	}
}

func integrationProject(t *testing.T) (project.Project, []byte, []byte) {
	t.Helper()
	root := t.TempDir()
	packageJSON := []byte(`{"devDependencies":{"vitest":"^4.0.0"}}`)
	lockfile := []byte(`{"lockfileVersion":3}`)
	if err := os.WriteFile(filepath.Join(root, "package.json"), packageJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package-lock.json"), lockfile, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "node_modules", "pre-existing-integration"), 0o755); err != nil {
		t.Fatal(err)
	}
	return project.Project{Root: root, Source: root, PackageManager: "npm", TestRunner: "vitest"}, packageJSON, lockfile
}

func writePackageState(t *testing.T, root, packageJSON, lockfile string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(packageJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package-lock.json"), []byte(lockfile), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertPackageState(t *testing.T, root string, packageJSON, lockfile []byte) {
	t.Helper()
	gotPackageJSON, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil || !bytes.Equal(gotPackageJSON, packageJSON) {
		t.Errorf("package.json = %q, %v; want exact original %q", gotPackageJSON, err, packageJSON)
	}
	gotLockfile, err := os.ReadFile(filepath.Join(root, "package-lock.json"))
	if err != nil || !bytes.Equal(gotLockfile, lockfile) {
		t.Errorf("package-lock.json = %q, %v; want exact original %q", gotLockfile, err, lockfile)
	}
}

// A project that already wrote its own .fallowrc.json owns that file: adding
// a key to it is a merge, not a write dharness gets to make on its own.
func TestFallowExtendsIsDelegatedWhenTheProjectOwnsTheConfig(t *testing.T) {
	root := t.TempDir()
	original := []byte(`{"custom":true}`)
	if err := os.WriteFile(filepath.Join(root, fallowConfig), original, 0o600); err != nil {
		t.Fatal(err)
	}
	p := project.Project{Root: root, Source: root}

	why, ok := (fallowExtendsStep{}).Delegated(p)
	if !ok {
		t.Fatal("Delegated() ok = false, want true when the project's own config already exists")
	}
	if !strings.Contains(why, fallowConfig) {
		t.Errorf("Delegated() why = %q, want it to name %s", why, fallowConfig)
	}

	if err := applySteps([]Step{fallowExtendsStep{}}, p, io.Discard); err != nil {
		t.Fatalf("applySteps() = %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, fallowConfig))
	if err != nil || !bytes.Equal(raw, original) {
		t.Errorf("the project's own config was touched: %q, %v", raw, err)
	}
}

// Same rule for lefthook.yml, which belongs to the repository rather than the
// JS project.
func TestLefthookExtendsIsDelegatedWhenTheProjectOwnsTheConfig(t *testing.T) {
	root := t.TempDir()
	original := []byte("custom: true\n")
	if err := os.WriteFile(filepath.Join(root, lefthookConfig), original, 0o600); err != nil {
		t.Fatal(err)
	}
	p := project.Project{Root: root, Source: root}

	why, ok := (lefthookExtendsStep{}).Delegated(p)
	if !ok {
		t.Fatal("Delegated() ok = false, want true when the project's own lefthook config already exists")
	}
	if !strings.Contains(why, lefthookConfig) {
		t.Errorf("Delegated() why = %q, want it to name %s", why, lefthookConfig)
	}

	if err := applySteps([]Step{lefthookExtendsStep{}}, p, io.Discard); err != nil {
		t.Fatalf("applySteps() = %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, lefthookConfig))
	if err != nil || !bytes.Equal(raw, original) {
		t.Errorf("the project's own config was touched: %q, %v", raw, err)
	}
}

// With no JS project to adopt, there is no .fallowrc.json to point anywhere,
// so the step is satisfied without asking — the mutation gate for this line
// distinguishes "no source" from "source exists but not wired", which only a
// project with an empty Source exercises.
func TestFallowExtendsSatisfiedWhenTheProjectHasNoSource(t *testing.T) {
	p := project.Project{Root: t.TempDir()}
	if !(fallowExtendsStep{}).Satisfied(p) {
		t.Error("Satisfied() = false, want true when there is no JS project to adopt")
	}
}

// Same rule for lefthook: with no hook manager answering lefthook at all,
// there is nothing to wire, so the step is satisfied without asking.
func TestLefthookExtendsSatisfiedWhenLefthookIsNotTheHookManager(t *testing.T) {
	p := project.Project{Root: t.TempDir()}
	if !(lefthookExtendsStep{}).Satisfied(p) {
		t.Error("Satisfied() = false, want true when lefthook does not answer for this project")
	}
}

// installStep has nothing to install without a JS project: there is no
// package manager to ask.
func TestInstallStepSatisfiedWhenTheProjectHasNoSource(t *testing.T) {
	p := project.Project{Root: t.TempDir()}
	if !(installStep{}).Satisfied(p) {
		t.Error("Satisfied() = false, want true when there is no JS project to install into")
	}
}

// A directory under node_modules is an install artifact, not a declaration.
// It survives a rollback that restored package.json, it is absent under Yarn
// PnP and pnpm's store, and on its own it never meant the package was
// declared. dharness does not read it: the install command is the one that
// decides, so with a JS project present the step always runs.
func TestInstallStepRunsEvenWithThePackageSittingInNodeModules(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "node_modules", RulesPackage), 0o755); err != nil {
		t.Fatal(err)
	}
	p := project.Project{Root: root, Source: root, PackageManager: "bun"}

	if (installStep{}).Satisfied(p) {
		t.Error("Satisfied() = true from a node_modules directory alone; the install command is what decides")
	}
}

// With no project config at all, dharness writes the whole thing itself —
// there is nothing to merge into.
func TestExtendsStepsWriteTheirFileWhenTheProjectHasNone(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root}

	if why, ok := (fallowExtendsStep{}).Delegated(p); ok {
		t.Fatalf("Delegated() = %q, true; want ok=false with no config present", why)
	}
	if why, ok := (lefthookExtendsStep{}).Delegated(p); ok {
		t.Fatalf("Delegated() = %q, true; want ok=false with no config present", why)
	}

	if err := applySteps([]Step{fallowExtendsStep{}, lefthookExtendsStep{}}, p, io.Discard); err != nil {
		t.Fatalf("applySteps() = %v", err)
	}

	fallowRaw, err := os.ReadFile(filepath.Join(root, fallowConfig))
	if err != nil || !strings.Contains(string(fallowRaw), "extends") {
		t.Errorf("%s was not written: %q, %v", fallowConfig, fallowRaw, err)
	}
	lefthookRaw, err := os.ReadFile(filepath.Join(root, lefthookConfig))
	if err != nil || !strings.Contains(string(lefthookRaw), "extends") {
		t.Errorf("%s was not written: %q, %v", lefthookConfig, lefthookRaw, err)
	}
}

// What enables a project is a hook manager that answers, not membership in a
// list: writing a lefthook file into a husky project creates configuration
// nothing reads.
func TestHookManagerAnswersRatherThanBeingAssumed(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  manager
	}{
		{"lefthook by config", []string{"lefthook.yml"}, managerLefthook},
		{"lefthook by dotted config", []string{".lefthook.yaml"}, managerLefthook},
		{"husky by directory", []string{filepath.Join(".husky", "pre-commit")}, managerHusky},
		{"nothing answers", nil, managerNone},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			for _, name := range testCase.files {
				path := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}

			if got := hookManager(project.Project{Root: root}); got != testCase.want {
				t.Errorf("hookManager() = %v, want %v", got, testCase.want)
			}
		})
	}
}

// With no manager answering, choosing one for this project is a decision
// dharness does not get to make, so the step is not satisfied — a delegated
// step blocks nothing else in the plan, so satisfying it artificially is no
// longer needed to keep the run moving.
func TestGateStepIsAnOpenDecisionWhenNoManagerAnswers(t *testing.T) {
	p := project.Project{Root: t.TempDir()}

	if (hookInstallStep{}).Satisfied(p) {
		t.Error("Satisfied() = true, want false: no hook manager answers here")
	}

	why, ok := (hookInstallStep{}).Delegated(p)
	if !ok {
		t.Fatal("Delegated() ok = false, want true: no manager means an open decision")
	}
	if why == "" {
		t.Error("Delegated() why is empty; an open decision handed to the agent needs a reason")
	}
}

// The thresholds live in a file because react-doctor accepts only a severity,
// so a rule cannot carry its own number.
func TestOwnedFilesCarryTheThresholdsTheRulesCannot(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root}

	if err := (ownedFilesStep{}).Apply(p, &Writer{}); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, project.Dir, ownedRules))
	if err != nil {
		t.Fatalf("the thresholds file was not written: %v", err)
	}
	if !strings.Contains(string(raw), `"maxFileLines": 500`) {
		t.Errorf("thresholds do not carry the file ceiling:\n%s", raw)
	}

	// The architecture is deliberately absent: it is decided, not detected.
	architecture, err := os.ReadFile(filepath.Join(root, project.Dir, ownedFallow))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(architecture), "boundaries") {
		t.Errorf("dharness declared an architecture it cannot know:\n%s", architecture)
	}
}

// gateInstalled and huskyWired both answer "will git actually run the gate",
// which is a different question from whether some file mentions it. Each one
// reads a file that may not exist, and a missing file is a "no", never a
// "yes" reached by skipping the read.
func TestHookDetectionReadsTheFileAndSaysNoWhenItIsAbsent(t *testing.T) {
	for _, hook := range []struct {
		name    string
		path    string
		wired   string
		unwired string
		answer  func(project.Project) bool
	}{
		{
			name:    "lefthook writes the git hook",
			path:    filepath.Join(".git", "hooks", "pre-commit"),
			wired:   "#!/bin/sh\nlefthook run pre-commit\n",
			unwired: "#!/bin/sh\necho something else\n",
			answer:  gateInstalled,
		},
		{
			name:    "husky keeps a shell script",
			path:    filepath.FromSlash(huskyHook),
			wired:   gateCommand + "\n",
			unwired: "npm test\n",
			answer:  huskyWired,
		},
	} {
		t.Run(hook.name, func(t *testing.T) {
			root := t.TempDir()
			p := project.Project{Root: root, Source: root}

			if hook.answer(p) {
				t.Error("answered true with no hook file at all")
			}

			full := filepath.Join(root, hook.path)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(hook.unwired), 0o600); err != nil {
				t.Fatal(err)
			}
			if hook.answer(p) {
				t.Error("answered true for a hook that does not invoke the gate")
			}

			if err := os.WriteFile(full, []byte(hook.wired), 0o600); err != nil {
				t.Fatal(err)
			}
			if !hook.answer(p) {
				t.Error("answered false for a hook that does invoke the gate")
			}
		})
	}
}

// husky's hook belongs to the project, so the gate is appended rather than
// written over. The separator is the whole subtlety: a script that already
// ends in a newline must not gain a blank line, and one that does not must
// not have the gate welded onto its last command.
func TestAppendHuskyGateKeepsTheScriptAndSeparatesTheGate(t *testing.T) {
	for _, script := range []struct {
		name     string
		existing string
		want     string
	}{
		{name: "no script yet", existing: "", want: gateCommand + "\n"},
		{name: "ends in a newline", existing: "npm test\n", want: "npm test\n" + gateCommand + "\n"},
		{name: "ends without one", existing: "npm test", want: "npm test\n" + gateCommand + "\n"},
	} {
		t.Run(script.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, filepath.FromSlash(huskyHook))
			if script.existing != "" {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(script.existing), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			if err := appendHuskyGate(project.Project{Root: root}, &Writer{}); err != nil {
				t.Fatalf("appendHuskyGate() = %v", err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != script.want {
				t.Errorf("husky hook = %q, want %q", raw, script.want)
			}
		})
	}
}

// A hook path that cannot be read is not the same as one that is absent, and
// only the second is a "nothing here yet". Reading a directory as a file is
// the portable way to produce the first.
func TestAppendHuskyGateFailsOnAHookItCannotRead(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(huskyHook)), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := appendHuskyGate(project.Project{Root: root}, &Writer{}); err == nil {
		t.Error("appendHuskyGate() = nil for a hook path that is not a readable file")
	}
}

// Five of the six rules are guardrails on generated code and arrive at
// "error". folder-ownership is not one: it requires that a split module
// publish an index.ts, which a project that deliberately has no barrel files
// cannot satisfy. dharness writes it "off" and the architecture prompt says
// how to turn it on.
func TestDoctorConfigLeavesTheArchitecturalRuleOff(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root, PackageManager: "bun"}

	if err := (doctorConfigStep{}).Apply(p, &Writer{}); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	var config doctorConfigFile
	raw, err := os.ReadFile(filepath.Join(root, doctorConfig))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}

	if got := config.Rules[RulesPrefix+"/folder-ownership"]; got != "off" {
		t.Errorf("folder-ownership = %q, want \"off\": it requires an index.ts this project may forbid", got)
	}
	for _, rule := range []string{"max-file-lines", "require-jsdoc", "require-variable-jsdoc", "role-file-shape", "pure-index-barrel"} {
		id := RulesPrefix + "/" + rule
		if got := config.Rules[id]; got != "error" {
			t.Errorf("%s = %q, want \"error\"", id, got)
		}
	}
}

// pure-index-barrel stays at "error" on purpose and is not an exception to
// the rule above: it constrains a barrel that exists rather than requiring
// one, so a project without barrels never sees it fire.
func TestDoctorConfigKeepsASeverityTheProjectAlreadyChose(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root, PackageManager: "bun"}
	writeFile := filepath.Join(root, doctorConfig)
	chosen := `{"rules":{"dharness/folder-ownership":"error","dharness/require-jsdoc":"warn"}}`
	if err := os.WriteFile(writeFile, []byte(chosen), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := (doctorConfigStep{}).Apply(p, &Writer{}); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	var config doctorConfigFile
	raw, _ := os.ReadFile(writeFile)
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	if got := config.Rules[RulesPrefix+"/folder-ownership"]; got != "error" {
		t.Errorf("folder-ownership = %q; a severity the project chose must survive sync", got)
	}
	if got := config.Rules[RulesPrefix+"/require-jsdoc"]; got != "warn" {
		t.Errorf("require-jsdoc = %q; a severity the project chose must survive sync", got)
	}
}

// A rule dharness turns off has to say so where the decision is made, or it
// is a silent default. The architecture prompt names the rule, the file and
// the exact edit.
func TestArchitecturePromptSaysHowToTurnOnTheBarrelRule(t *testing.T) {
	prompt := (architectureStep{}).Describe(project.Project{
		Root: "/repo", Source: "/repo/frontend", PackageManager: "bun",
	})

	for _, expected := range []string{
		"frontend/" + doctorConfig,
		`"dharness/folder-ownership": "error"`,
		"index.ts",
	} {
		if !strings.Contains(prompt, expected) {
			t.Errorf("the architecture prompt omits %q:\n%s", expected, prompt)
		}
	}
}
