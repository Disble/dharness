package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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

func syncOutput(t *testing.T, prepare func(root string)) string {
	t.Helper()

	root := t.TempDir()
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
	writeFile(t, filepath.Join(root, ".dharness", "fallow.jsonc"), "{}\n")
	writeFile(t, filepath.Join(root, ".dharness", "rules.json"), "{}\n")

	writeFile(t, filepath.Join(root, ".fallowrc.json"), `{"extends":[".dharness/fallow.jsonc"]}`)
	writeFile(t, filepath.Join(root, "lefthook.yml"), "extends:\n  - .dharness/lefthook.yml\n")
	writeFile(t, filepath.Join(root, "doctor.config.json"), `{"plugins":["`+setup.RulesPackage+`"]}`)
	writeFile(t, filepath.Join(root, ".mcp.json"), `{"mcpServers":{"fallow":{"command":"bunx"}}}`)
	writeFile(t, filepath.Join(root, ".git", "hooks", "pre-commit"), "lefthook run pre-commit\n")
	writeFile(t, filepath.Join(root, ".claude", "skills", "react-doctor", "SKILL.md"), "# skill\n")
}

// Every step that has a command shows the command, in the form this project
// uses. A bun project must never be told to run npm.
func TestSyncSpeaksTheProjectsOwnPackageManager(t *testing.T) {
	out := syncOutput(t, func(root string) {
		writeFile(t, filepath.Join(root, "bun.lock"), "")
		writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	})

	for _, want := range []string{"bun add -d", setup.RulesPackage, "@stryker-mutator/vitest-runner"} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not mention %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "npm install") {
		t.Errorf("a bun project was told to run npm:\n%s", out)
	}
}

// A step dharness cannot run says why. "Ask a person" without a reason is a
// shrug, and this one has a measured reason.
func TestSyncSaysWhyTheDelegatedStepIsDelegated(t *testing.T) {
	out := syncOutput(t, func(root string) {
		writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	})

	if !strings.Contains(out, "dharness cannot run this") {
		t.Errorf("the delegated step was listed without a reason:\n%s", out)
	}
	if !strings.Contains(out, "git hook that competes with this gate") {
		t.Errorf("the reason does not name the collision that causes it:\n%s", out)
	}
}

// With nothing outstanding the command answers instead of listing a step that
// adoption can never satisfy.
func TestSyncReachesATerminalAnswer(t *testing.T) {
	out := syncOutput(t, func(root string) { satisfied(t, root) })

	if !strings.Contains(out, "Nothing to do") {
		t.Errorf("a fully configured project got no terminal answer:\n%s", out)
	}
	if strings.Contains(out, "## 1.") {
		t.Errorf("a fully configured project was still given steps:\n%s", out)
	}
}

// sync writes nothing. It is the half of the pair that is safe at any moment,
// and a report that changed the repository would not be.
func TestSyncWritesNothing(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)

	before := tree(t, root)
	previous := workingDirectory
	workingDirectory = func() (string, error) { return root, nil }
	t.Cleanup(func() { workingDirectory = previous })

	if err := RunSync(nil, &bytes.Buffer{}); err != nil {
		t.Fatalf("RunSync() = %v", err)
	}

	if after := tree(t, root); after != before {
		t.Errorf("sync changed the repository:\nbefore %q\nafter  %q", before, after)
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
