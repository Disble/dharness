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

func TestInstallStepPlansOnlyMissingIntegrationPackages(t *testing.T) {
	p, _, _ := integrationProject(t)
	want := []string{RulesPackage}

	if got := missing(p); !slices.Equal(got, want) {
		t.Fatalf("missing() = %v, want %v", got, want)
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
	prompt := ArchitecturePrompt(project.Project{PackageManager: "pnpm"})
	for _, invocation := range []string{
		"pnpm dlx fallow@latest list --boundaries",
		"pnpm dlx fallow@latest dead-code --boundary-violations",
	} {
		if !strings.Contains(prompt, invocation) {
			t.Errorf("architecture prompt omits %q:\n%s", invocation, prompt)
		}
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

// With no manager answering there is nothing to satisfy: choosing one for a
// project is a decision, and it must not block everything else in the plan.
func TestGateStepIsSatisfiedWhenNoManagerAnswers(t *testing.T) {
	if !(hookInstallStep{}).Satisfied(project.Project{Root: t.TempDir()}) {
		t.Error("the plan stalled on a project that has no hook manager at all")
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
