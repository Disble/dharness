package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/runner"
	"github.com/Disble/dharness/internal/setup"
)

// binaryName is how npm writes the shim on this platform: a .cmd wrapper on
// Windows, a plain file everywhere else.
func binaryName(tool string) string {
	if runtime.GOOS == "windows" {
		return tool + ".cmd"
	}
	return tool
}

// gitProject stubs the git probe so Discover treats root as a repository with
// exactly these lockfiles tracked, and writes an (empty, unless already
// prepared) placeholder for each so the on-disk tree matches what git
// reports. Decision 6bis makes InRepository production-relevant: a bare
// t.TempDir() is not a repository, and every sync test now needs this to
// reach the plan at all — the swallow branch it used to pass through on is
// exactly what RunSync stops on now.
func gitProject(t *testing.T, root string, lockfiles ...string) {
	t.Helper()

	for _, lockfile := range lockfiles {
		path := filepath.Join(root, filepath.FromSlash(lockfile))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}

	t.Cleanup(project.SetGitOutputForTest(func(_ string, args ...string) ([]byte, error) {
		switch {
		case len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel":
			return []byte(filepath.ToSlash(root) + "\n"), nil
		case len(args) >= 1 && args[0] == "ls-files":
			return []byte(strings.Join(lockfiles, "\x00")), nil
		}
		return nil, errors.New("unexpected git call")
	}))
}

func syncOutput(t *testing.T, prepare func(root string)) string {
	t.Helper()

	root := t.TempDir()
	gitProject(t, root, "bun.lock")
	if prepare != nil {
		prepare(root)
	}

	previous := workingDirectory
	workingDirectory = func() (string, error) { return root, nil }
	t.Cleanup(func() { workingDirectory = previous })

	var out bytes.Buffer
	if err := RunSync(nil, &out); err != nil {
		t.Fatalf("RunSync() = %v", err)
	}
	return out.String()
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

// satisfied builds a project that meets every step, so a test can assert on
// what disappears rather than only on what appears.
func satisfied(t *testing.T, root string) {
	t.Helper()

	writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	writeFile(t, filepath.Join(root, "bun.lock"), "")
	for _, name := range []string{"react-doctor", "fallow", "stryker", "lefthook"} {
		writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName(name)), "")
	}
	writeFile(t, filepath.Join(root, "node_modules", "@stryker-mutator", "vitest-runner", "package.json"), "{}")
	writeFile(t, filepath.Join(root, "node_modules", setup.RulesPackage, "package.json"), "{}")

	writeFile(t, filepath.Join(root, ".dharness", "lefthook.yml"), "pre-commit:\n")
	writeFile(t, filepath.Join(root, ".dharness", "fallow.jsonc"), "{\n  \"boundaries\": []\n}\n")
	writeFile(t, filepath.Join(root, ".dharness", "rules.json"), "{}\n")

	writeFile(t, filepath.Join(root, ".fallowrc.json"), `{"extends":[".dharness/fallow.jsonc"]}`)
	writeFile(t, filepath.Join(root, "lefthook.yml"), "extends:\n  - .dharness/lefthook.yml\n")
	writeFile(t, filepath.Join(root, "doctor.config.json"), `{"plugins":["`+setup.RulesPackage+`"]}`)
	writeFile(t, filepath.Join(root, ".mcp.json"), `{"mcpServers":{"fallow":{"command":"bunx"}}}`)
	writeFile(t, filepath.Join(root, ".git", "hooks", "pre-commit"), "lefthook run pre-commit\n")
	writeFile(t, filepath.Join(root, ".claude", "skills", "react-doctor", "SKILL.md"), "# skill\n")
}

// stubRunner replaces runner.Run with a recorder so installStep and
// hookInstallStep never shell out during a test — RunSync applies now, and a
// bare t.TempDir() has no bun, lefthook or husky to actually run.
func stubRunner(t *testing.T) *[]runner.Command {
	t.Helper()
	var commands []runner.Command
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		commands = append(commands, cmd)
		return nil
	}))
	return &commands
}

// Applying speaks the project's own package manager, in the form the command
// itself would run — not a rendered description, since a non-delegated step
// is applied silently and only names itself in the report. A bun project must
// never be told to run npm.
func TestSyncSpeaksTheProjectsOwnPackageManager(t *testing.T) {
	commands := stubRunner(t)

	syncOutput(t, func(root string) {
		writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	})

	if len(*commands) == 0 {
		t.Fatal("no package command ran even though the integration package is missing")
	}
	install := (*commands)[0]
	if install.Name != "bun" {
		t.Errorf("install command = %s, want bun: %+v", install.Name, install)
	}
	if !slices.Contains(install.Args, setup.RulesPackage) {
		t.Errorf("install command does not name %s: %+v", setup.RulesPackage, install)
	}
	for _, remote := range []string{"@stryker-mutator/core", "@stryker-mutator/vitest-runner", "@stryker-mutator/jest-runner"} {
		if slices.Contains(install.Args, remote) {
			t.Errorf("sync would install remote Stryker package %q: %+v", remote, install)
		}
	}
}

// A step dharness cannot run says why. "Ask a person" without a reason is a
// shrug, and this one has a measured reason. Extended to cover every kind of
// delegation this change adds: the extends steps and the gate.
func TestSyncSaysWhyTheDelegatedStepIsDelegated(t *testing.T) {
	stubRunner(t)

	out := syncOutput(t, func(root string) {
		writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	})

	if !strings.Contains(out, "dharness cannot run this") {
		t.Errorf("the delegated step was listed without a reason:\n%s", out)
	}
	if !strings.Contains(out, "git hook that competes with this gate") {
		t.Errorf("the reason does not name the collision that causes it (agentSkillStep):\n%s", out)
	}
	if !strings.Contains(out, "nothing answers") {
		t.Errorf("the reason does not name the open hook-manager decision (hookInstallStep):\n%s", out)
	}
	if !strings.Contains(out, "no tool can\nread intent off a tree") {
		t.Errorf("the reason does not name the architecture decision (architectureStep):\n%s", out)
	}
}

// With nothing outstanding the command answers instead of listing a step that
// adoption can never satisfy.
func TestSyncReachesATerminalAnswer(t *testing.T) {
	stubRunner(t)

	out := syncOutput(t, func(root string) { satisfied(t, root) })

	if !strings.Contains(out, "Nothing to do") {
		t.Errorf("a fully configured project got no terminal answer:\n%s", out)
	}
	if strings.Contains(out, "## 1.") {
		t.Errorf("a fully configured project was still given steps:\n%s", out)
	}
	if strings.Contains(out, "decide this project's architecture") {
		t.Errorf("the architecture step printed even though boundaries is declared (§15):\n%s", out)
	}
}

// Both regions of Decision 1's report appear, and in order: what dharness did
// itself, then what it hands to the agent.
func TestSyncAppliesAndDelegatesInOneRun(t *testing.T) {
	commands := stubRunner(t)

	out := syncOutput(t, func(root string) {
		writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	})

	applying := strings.Index(out, "Applying:")
	delegated := strings.Index(out, "## Left to you:")
	if applying == -1 {
		t.Fatalf("the Applying region is missing:\n%s", out)
	}
	if delegated == -1 {
		t.Fatalf("the Left to you region is missing:\n%s", out)
	}
	if applying > delegated {
		t.Errorf("the delegated region printed before the applied region:\n%s", out)
	}
	if len(*commands) == 0 {
		t.Error("nothing was applied even though installStep was pending")
	}
}

// A project that already configured fallow completes sync with no error and
// no rollback — the exact bug this change closes. Only the missing extends
// line is handed to the agent; the project's own file is untouched.
func TestSyncCompletesWhenTheProjectAlreadyConfiguredFallow(t *testing.T) {
	stubRunner(t)

	original := `{"custom":true}`
	var root string
	out := syncOutput(t, func(dir string) {
		root = dir
		writeFile(t, filepath.Join(dir, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
		writeFile(t, filepath.Join(dir, ".fallowrc.json"), original)
	})

	if !strings.Contains(out, "point .fallowrc.json at the file dharness owns") {
		t.Errorf("the fallow extends step is missing from the report:\n%s", out)
	}
	if !strings.Contains(out, ".fallowrc.json already exists") {
		t.Errorf("the delegated reason is missing:\n%s", out)
	}

	raw, err := os.ReadFile(filepath.Join(root, ".fallowrc.json"))
	if err != nil || string(raw) != original {
		t.Errorf("the project's own .fallowrc.json was touched: %q, %v", raw, err)
	}
}

// agentSkillStep.Delegated always answers ok == true; if applySteps ever
// called its Apply anyway, the contract-assertion error inside it would
// surface as a failed sync. It must not.
func TestSyncNeverAppliesADelegatedStep(t *testing.T) {
	commands := stubRunner(t)

	out := syncOutput(t, func(root string) {
		writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	})

	if strings.Contains(out, "is delegated and must not be applied") {
		t.Errorf("a delegated step's Apply error surfaced:\n%s", out)
	}
	for _, cmd := range *commands {
		if strings.Contains(cmd.String(), "lefthook") {
			t.Errorf("hookInstallStep ran a command though no hook manager answers this project: %+v", cmd)
		}
	}
}

// RunInit's untested branch: with no JS project to adopt, sync explains why
// and stops before writing anything, at exit 0 — not the same stop as
// Decision 6bis's missing repository.
func TestSyncStopsBeforeWritingWithoutAJSProject(t *testing.T) {
	root := t.TempDir()
	gitProject(t, root)

	previous := workingDirectory
	workingDirectory = func() (string, error) { return root, nil }
	t.Cleanup(func() { workingDirectory = previous })

	var out bytes.Buffer
	if err := RunSync(nil, &out); err != nil {
		t.Fatalf("RunSync() = %v, want nil for a repository with no JS project", err)
	}
	if !strings.Contains(out.String(), "No JS project found") {
		t.Errorf("output does not carry the no-source message:\n%s", out.String())
	}
}

// Pins Decision 3 through RunSync, not just through setup.Apply directly: the
// merged command must not lose the hedge on the way out.
func TestSyncRollbackNamesWhatItRestoredAndNothingMore(t *testing.T) {
	// Only the install itself fails; its own compensation (removing the
	// packages it never actually added) must still succeed, or the errors.Join
	// branch fires instead of the one this test pins.
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		if slices.Contains(cmd.Args, "add") || slices.Contains(cmd.Args, "install") {
			return errors.New("install failed")
		}
		return nil
	}))

	root := t.TempDir()
	gitProject(t, root, "bun.lock")
	writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)

	previous := workingDirectory
	workingDirectory = func() (string, error) { return root, nil }
	t.Cleanup(func() { workingDirectory = previous })

	err := RunSync(nil, &bytes.Buffer{})
	if err == nil {
		t.Fatal("RunSync() = nil, want the apply failure to surface")
	}
	if !strings.Contains(err.Error(), "was put back as it was found") {
		t.Errorf("rollback error does not carry Decision 3's wording: %v", err)
	}
	if strings.Contains(err.Error(), "everything this run wrote was undone") {
		t.Errorf("rollback error overclaims full restoration ahead of writer-undo-completeness: %v", err)
	}
}

// Outside a repository there is no plan to derive and nothing to write:
// Decision 6bis stops before the header. No gitProject stub is used here —
// the real git binary already fails outside a repository the same way,
// leaving InRepository at its zero value.
func TestSyncStopsOutsideAGitRepository(t *testing.T) {
	root := t.TempDir()
	before := tree(t, root)

	previous := workingDirectory
	workingDirectory = func() (string, error) { return root, nil }
	t.Cleanup(func() { workingDirectory = previous })

	err := RunSync(nil, &bytes.Buffer{})
	if err == nil {
		t.Fatal("RunSync() = nil, want an error outside a git repository")
	}
	if !strings.Contains(err.Error(), root) {
		t.Errorf("error %q does not name the repository", err)
	}
	if after := tree(t, root); after != before {
		t.Errorf("sync changed the repository outside a git repository:\nbefore %q\nafter  %q", before, after)
	}
}

func tree(t *testing.T, root string) string {
	t.Helper()

	var paths []string
	err := filepath.Walk(root, func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(paths, "\n")
}
