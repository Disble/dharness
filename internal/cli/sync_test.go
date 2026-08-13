package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/report"
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

// syncJSON is syncOutput's --format json counterpart: it decodes stdout as
// a report.Report rather than returning the human text, so a test can
// assert on the model's own fields instead of rendered prose.
func syncJSON(t *testing.T, prepare func(root string)) report.Report {
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
	if err := RunSync([]string{"--format", "json"}, &out); err != nil {
		t.Fatalf("RunSync() = %v", err)
	}

	var decoded report.Report
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("json.Unmarshal(--format json output) = %v:\n%s", err, out.String())
	}
	return decoded
}

// syncJSONIn is syncJSON's runSyncIn counterpart: it runs against a root a
// test has already prepared (and possibly already run sync against once),
// rather than building a fresh one.
func syncJSONIn(t *testing.T, root string) report.Report {
	t.Helper()

	previous := workingDirectory
	workingDirectory = func() (string, error) { return root, nil }
	t.Cleanup(func() { workingDirectory = previous })

	var out bytes.Buffer
	if err := RunSync([]string{"--format", "json"}, &out); err != nil {
		t.Fatalf("RunSync() = %v", err)
	}

	var decoded report.Report
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("json.Unmarshal(--format json output) = %v:\n%s", err, out.String())
	}
	return decoded
}

// normalizeSpace collapses every run of whitespace (including the newline
// and hanging indent report.wrap introduces mid-sentence) to one space, so
// a test can assert a multi-word phrase survived rendering without pinning
// exactly where the renderer chose to break the line — that break point is
// shape, and shape is exactly what task 4.12's rewrite is free to change,
// where the words themselves are not.
func normalizeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
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

// A step dharness cannot run says why, in the "Left to you" block. Extended
// to cover every kind of delegation this change adds: the extends steps and
// the gate. Multi-word phrases are matched after normalizeSpace, since
// report.wrap is free to reflow the same words onto a different line —
// task 4.12's own rule: shape may change, the words themselves must not.
func TestSyncSaysWhyTheDelegatedStepIsDelegated(t *testing.T) {
	stubRunner(t)

	out := syncOutput(t, func(root string) {
		writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	})
	flat := normalizeSpace(out)

	if !strings.Contains(out, "Left to you") {
		t.Errorf("the delegated block is missing from the report:\n%s", out)
	}
	if !strings.Contains(flat, "a git hook that competes with this gate") {
		t.Errorf("the reason does not name the collision that causes it (agentSkillStep):\n%s", out)
	}
	if !strings.Contains(flat, "nothing answers") {
		t.Errorf("the reason does not name the open hook-manager decision (hookInstallStep):\n%s", out)
	}
	if !strings.Contains(flat, "no tool can read intent off a tree") {
		t.Errorf("the reason does not name the architecture decision (architectureStep):\n%s", out)
	}
}

// With nothing outstanding the command answers instead of listing a step
// that adoption can never satisfy. Every satisfied step, including
// architectureStep, is now reported in "Already in place" rather than
// silently absent — defect 1's whole point, and the opposite of what
// Pending()-filtered output used to do.
func TestSyncReachesATerminalAnswer(t *testing.T) {
	stubRunner(t)

	out := syncOutput(t, func(root string) { satisfied(t, root) })

	if strings.Contains(out, "Left to you") {
		t.Errorf("a fully configured project still reports delegated work:\n%s", out)
	}
	if !strings.Contains(out, "Already in place") {
		t.Errorf("a fully configured project's satisfied steps are not reported:\n%s", out)
	}
	if !strings.Contains(out, "decide this project's architecture") {
		t.Errorf("architectureStep is silently absent instead of reported satisfied (defect 1):\n%s", out)
	}
}

// Both regions of the report appear, and in order: what dharness did
// itself, then what it hands to the agent.
func TestSyncAppliesAndDelegatesInOneRun(t *testing.T) {
	commands := stubRunner(t)

	out := syncOutput(t, func(root string) {
		writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	})

	applied := strings.Index(out, "Applied")
	delegated := strings.Index(out, "Left to you")
	if applied == -1 {
		t.Fatalf("the Applied block is missing:\n%s", out)
	}
	if delegated == -1 {
		t.Fatalf("the Left to you block is missing:\n%s", out)
	}
	if applied > delegated {
		t.Errorf("the delegated block printed before the applied block:\n%s", out)
	}
	if len(*commands) == 0 {
		t.Error("nothing was applied even though installStep was pending")
	}
}

// TestSyncStdoutUnchangedAfterTheSinkMove now pins the fact slice 2's own
// version of this test protected, carried into the new report shape: a
// step's subprocess output is captured through the sink, not leaked to the
// real process stdout ahead of the report, and reaches the reader framed
// under that step's own heading (its Transcript line) rather than
// somewhere disconnected from it. The stdout-byte-identity slice 2 pinned
// is no longer the invariant — slice 4 is the first slice that changes the
// report's shape on purpose (design.md Decision 9) — but the framing fact
// underneath it still is.
func TestSyncStdoutUnchangedAfterTheSinkMove(t *testing.T) {
	t.Cleanup(runner.SetForTest(func(_ runner.Command, stdout, _ io.Writer) error {
		fmt.Fprint(stdout, "added dharness-eslint-plugin@0.3.0\n")
		return nil
	}))

	out := syncOutput(t, func(root string) {
		writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	})

	heading := strings.Index(out, "install what this project is missing")
	marker := strings.Index(out, "added dharness-eslint-plugin@0.3.0")
	if heading == -1 || marker == -1 {
		t.Fatalf("expected sections missing from output:\n%s", out)
	}
	if !(heading < marker) {
		t.Errorf("subprocess output did not appear framed under its own step's heading:\n%s", out)
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
	if !strings.Contains(normalizeSpace(out), ".fallowrc.json already exists") {
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

// With no JS project to adopt, sync explains why and stops before writing
// anything, at exit 0 — not the same stop as an outside-a-repository run.
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

// The rollback error keeps only the step name and cause (design.md
// Decision 8, change #3: "the report states the narrative, the error
// states the cause, and neither repeats the other"); the report itself
// must not claim more than Writer.Undo actually covers.
func TestSyncRollbackNamesWhatItRestoredAndNothingMore(t *testing.T) {
	// Only the install itself fails; its own compensation (removing the
	// packages it never actually added) must still succeed, or the
	// undo-failed branch fires instead of the one this test pins.
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

	var out bytes.Buffer
	err := RunSync(nil, &out)
	if err == nil {
		t.Fatal("RunSync() = nil, want the apply failure to surface")
	}
	if !strings.Contains(err.Error(), "install what this project is missing") {
		t.Errorf("error does not name the failed step: %v", err)
	}

	rendered := out.String()
	if !strings.Contains(rendered, "Failed") {
		t.Errorf("the report does not render the failed step:\n%s", rendered)
	}
	for _, overclaim := range []string{"everything this run wrote was undone", "the repository was fully restored"} {
		if strings.Contains(rendered, overclaim) {
			t.Errorf("the report overclaims more than Writer.Undo covers: %q found in:\n%s", overclaim, rendered)
		}
	}
}

// Outside a repository there is no plan to derive and nothing to write. No
// gitProject stub is used here — the real git binary already fails outside
// a repository the same way, leaving InRepository at its zero value.
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

// treeSet is tree's set-shaped counterpart, over files only (not
// directories), for tests that diff a tree before and after a run rather
// than comparing it whole.
func treeSet(t *testing.T, root string) map[string]bool {
	t.Helper()

	set := map[string]bool{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			set[path] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return set
}

// A split layout is the case the report has to name: git looks for the hook at
// the repository root, but the tools run where the package manager installed
// them. Asserted through Report.Source rather than a rendered heading — the
// underlying fact (which directory the tools run in) is what matters, and
// the JSON channel names it precisely regardless of how the human view
// chooses to phrase it.
func TestSyncNamesTheJSDirectoryOnlyWhenItIsNotTheRoot(t *testing.T) {
	stubRunner(t)

	root := t.TempDir()
	gitProject(t, root, "frontend/bun.lock")
	writeFile(t, filepath.Join(root, "frontend", "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	split := syncJSONIn(t, root)
	if split.Source != "frontend" {
		t.Errorf("a split layout's report.Source = %q, want %q", split.Source, "frontend")
	}

	flat := syncJSON(t, func(root string) {
		writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	})
	if flat.Source != "" {
		t.Errorf("a conventional repository's report.Source = %q, want empty (root and source are the same directory)", flat.Source)
	}
}

// A check that never ran must not be reported as a check that passed. A
// project whose only fallow config is fallow.toml cannot be read by the
// textual test dharness uses, so sync says so as a note, kind not-checked.
func TestSyncReportsTheConfigItCouldNotCheck(t *testing.T) {
	stubRunner(t)

	out := syncOutput(t, func(root string) {
		writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
		writeFile(t, filepath.Join(root, "fallow.toml"), "ignorePatterns = [\"wailsjs/**\"]\n")
	})

	if !strings.Contains(out, "not-checked") {
		t.Errorf("sync stayed silent about a config it could not read:\n%s", out)
	}
	if !strings.Contains(normalizeSpace(out), "unknown, not clear") {
		t.Errorf("sync did not refuse to claim the check passed:\n%s", out)
	}
}

// And it stays quiet for a config it can read, or the note becomes noise on
// every run of every project.
func TestSyncSaysNothingAboutAConfigItCanCheck(t *testing.T) {
	stubRunner(t)

	out := syncOutput(t, func(root string) {
		writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	})

	if strings.Contains(out, "not-checked") {
		t.Errorf("sync reported a blind spot it does not have:\n%s", out)
	}
}

// The end of the Uncertain path, proven through sync's own output rather than
// through the preset in isolation: a Wails project whose wails.json cannot be
// read is still a Wails project. The preset matches, contributes the
// documented default, and says what it guessed from — a note, kind assumed,
// distinct from not-checked: that kind says a check did not run, this one
// says a check ran on a default because the project's own answer could not
// be read.
func TestSyncSaysWhatAMatchedPresetHadToAssume(t *testing.T) {
	stubRunner(t)

	root := t.TempDir()
	gitProject(t, root, "frontend/bun.lock")
	writeFile(t, filepath.Join(root, "frontend", "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	writeFile(t, filepath.Join(root, "wails.json"), `{"wailsjsdir": `)

	out := runSyncIn(t, root)

	if !strings.Contains(out, "assumed") {
		t.Errorf("sync did not report what the preset had to assume:\n%s", out)
	}
	if !strings.Contains(out, "wails") {
		t.Errorf("the note does not name the preset that assumed:\n%s", out)
	}
	if strings.Contains(out, "not-checked") {
		t.Errorf("an assumption was reported under the blind-spot kind instead of its own:\n%s", out)
	}
}

// And a readable wails.json assumes nothing, or the note becomes noise on
// every Wails project.
func TestSyncAssumesNothingWhenTheFrameworkConfigReads(t *testing.T) {
	stubRunner(t)

	root := t.TempDir()
	gitProject(t, root, "frontend/bun.lock")
	writeFile(t, filepath.Join(root, "frontend", "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	writeFile(t, filepath.Join(root, "wails.json"), `{"wailsjsdir": "./frontend"}`)

	if out := runSyncIn(t, root); strings.Contains(out, "assumed") {
		t.Errorf("sync reported an assumption it did not make:\n%s", out)
	}
}

func runSyncIn(t *testing.T, root string) string {
	t.Helper()

	previous := workingDirectory
	workingDirectory = func() (string, error) { return root, nil }
	t.Cleanup(func() { workingDirectory = previous })

	var out bytes.Buffer
	if err := RunSync(nil, &out); err != nil {
		t.Fatalf("RunSync() = %v", err)
	}
	return out.String()
}

// Spec scenario "residue in an already-adopted repository is reported, never
// removed": a repository adopted before this change still has the six
// dharness/* severities and RulesPackage the retired mechanism wrote into
// doctor.config.json. dharness cannot tell that earlier write apart from a
// value the project set afterwards (§05), so the file is reported, not
// touched — and the report says the residue is inert, since the gate's
// react-doctor invocation runs with --staged, under which plugin rules do
// not fire.
func TestSyncReportsEslintResidueInAnAlreadyAdoptedRepository(t *testing.T) {
	stubRunner(t)

	root := t.TempDir()
	gitProject(t, root, "bun.lock")
	writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	residue := `{"plugins":["` + setup.RulesPackage + `"],"rules":{"dharness/max-file-lines":"error"}}`
	writeFile(t, filepath.Join(root, "doctor.config.json"), residue)

	out := runSyncIn(t, root)

	if !strings.Contains(out, "residue") {
		t.Errorf("sync stayed silent about the residue left in doctor.config.json:\n%s", out)
	}
	if !strings.Contains(out, setup.RulesPackage) {
		t.Errorf("the report does not name what it found in doctor.config.json:\n%s", out)
	}
	if !strings.Contains(out, "--staged") {
		t.Errorf("the report does not say the residue is inert under the gate's --staged invocation:\n%s", out)
	}

	got, err := os.ReadFile(filepath.Join(root, "doctor.config.json"))
	if err != nil {
		t.Fatalf("doctor.config.json disappeared: %v", err)
	}
	if string(got) != residue {
		t.Errorf("sync edited or rewrote residue it must only report:\ngot  %s\nwant %s", got, residue)
	}
}

// And a project with no doctor.config.json at all — adopted after this
// change, or one that never had the old mechanism run — gets no note, or the
// kind becomes noise on every project.
func TestSyncSaysNothingAboutEslintResidueWhenThereIsNone(t *testing.T) {
	stubRunner(t)

	out := syncOutput(t, func(root string) {
		writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	})

	if strings.Contains(out, "residue") {
		t.Errorf("sync reported residue it does not have:\n%s", out)
	}
}

// TestSyncFormatJSONEmitsParseableJSONAndNothingElse pins "--format json
// emits parseable JSON on stdout": stdout decodes cleanly and json.Valid
// covers the entire buffer, so no progress line or banner shares the
// stream with it.
func TestSyncFormatJSONEmitsParseableJSONAndNothingElse(t *testing.T) {
	stubRunner(t)

	root := t.TempDir()
	gitProject(t, root, "bun.lock")
	writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)

	previous := workingDirectory
	workingDirectory = func() (string, error) { return root, nil }
	t.Cleanup(func() { workingDirectory = previous })

	var out bytes.Buffer
	if err := RunSync([]string{"--format", "json"}, &out); err != nil {
		t.Fatalf("RunSync() = %v", err)
	}

	if !json.Valid(out.Bytes()) {
		t.Fatalf("--format json output is not valid JSON, or is not the whole stream:\n%s", out.String())
	}
	var decoded report.Report
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("json.Unmarshal() = %v", err)
	}
}

// TestSyncFormatJSONAndHumanAgreeOnSummaryCounts pins "the JSON summary's
// counts match the human summary's counts": both are rendered from the one
// Report a single setup.Run call built, never recomputed per flag.
func TestSyncFormatJSONAndHumanAgreeOnSummaryCounts(t *testing.T) {
	prepare := func(root string) {
		writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	}

	stubRunner(t)
	human := syncOutput(t, prepare)

	stubRunner(t)
	jsonReport := syncJSON(t, prepare)

	for _, want := range []string{
		fmt.Sprintf("%d applied", jsonReport.Summary.Applied),
		fmt.Sprintf("%d delegated", jsonReport.Summary.Delegated),
		fmt.Sprintf("%d satisfied", jsonReport.Summary.Satisfied),
		fmt.Sprintf("%d failed", jsonReport.Summary.Failed),
	} {
		if !strings.Contains(human, want) {
			t.Errorf("human output missing %q, want it to agree with the JSON summary:\n%s", want, human)
		}
	}
}

// TestSyncExitFieldMatchesRunnerExitCode pins both exit-code scenarios
// together, since design.md's Property 2 exists to keep them untestable
// apart: delegated work remaining still reports exit 0, and a step failure's
// JSON exit equals runner.ExitCode of the exact error RunSync returned —
// never a value computed from summary.failed or summary.delegated
// independently.
func TestSyncExitFieldMatchesRunnerExitCode(t *testing.T) {
	t.Run("delegated work only", func(t *testing.T) {
		stubRunner(t)
		jsonReport := syncJSON(t, func(root string) {
			writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
		})
		if jsonReport.Summary.Delegated == 0 {
			t.Fatal("fixture has no delegated work; fix the test before trusting its assertion")
		}
		if jsonReport.Summary.Failed != 0 {
			t.Fatal("fixture has a failure; fix the test before trusting its assertion")
		}
		if jsonReport.Exit != 0 {
			t.Errorf("Exit = %d, want 0 — delegated work remaining is not a failure", jsonReport.Exit)
		}
	})

	t.Run("step failure", func(t *testing.T) {
		t.Cleanup(runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
			if slices.Contains(cmd.Args, "add") || slices.Contains(cmd.Args, "install") {
				return &runner.ExitError{Code: 7}
			}
			return nil
		}))

		root := t.TempDir()
		gitProject(t, root, "bun.lock")
		writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)

		previous := workingDirectory
		workingDirectory = func() (string, error) { return root, nil }
		t.Cleanup(func() { workingDirectory = previous })

		var out bytes.Buffer
		err := RunSync([]string{"--format", "json"}, &out)
		if err == nil {
			t.Fatal("RunSync() = nil, want the apply failure to surface")
		}

		var decoded report.Report
		if jsonErr := json.Unmarshal(out.Bytes(), &decoded); jsonErr != nil {
			t.Fatalf("json.Unmarshal() = %v:\n%s", jsonErr, out.String())
		}
		if want := runner.ExitCode(err); decoded.Exit != want {
			t.Errorf("Exit = %d, want runner.ExitCode(err) = %d", decoded.Exit, want)
		}
	})
}

// TestClosingBlockNamesTheDelegatedStepAsNext pins "a run with delegated
// work names a next step" through RunSync's own assembly, not merely at
// report.WriteHuman's unit layer.
func TestClosingBlockNamesTheDelegatedStepAsNext(t *testing.T) {
	stubRunner(t)

	out := syncOutput(t, func(root string) {
		writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	})

	ruleAt := strings.LastIndex(out, strings.Repeat("─", 70))
	if ruleAt < 0 {
		t.Fatalf("closing separator not found in output:\n%s", out)
	}
	closing := out[ruleAt:]
	if !strings.Contains(closing, "next") {
		t.Errorf("closing block does not name a next step for a run with delegated work:\n%s", closing)
	}
}

// TestScopedMutationEvidenceSurvivesBothViewsRegardless pins "measured
// evidence keeps its place in the model" — no longer gated on left == 0
// (design.md Decision 8, change #2): a project with every step satisfied
// still carries the measured related-test count and path in both views.
func TestScopedMutationEvidenceSurvivesBothViewsRegardless(t *testing.T) {
	stubRunner(t)

	root := t.TempDir()
	gitProject(t, root, "bun.lock")
	satisfied(t, root)
	if err := (project.Project{Root: root}).RecordScopedMutation("src/thing.ts", 25); err != nil {
		t.Fatal(err)
	}

	human := runSyncIn(t, root)
	if !strings.Contains(human, "25") || !strings.Contains(human, "src/thing.ts") {
		t.Errorf("human output does not carry the measured evidence:\n%s", human)
	}

	jsonReport := syncJSONIn(t, root)
	if jsonReport.Evidence == nil {
		t.Fatal("Evidence is nil in the JSON report")
	}
	if jsonReport.Evidence.RelatedTests != 25 || jsonReport.Evidence.MeasuredPath != "src/thing.ts" {
		t.Errorf("Evidence = %+v, want {RelatedTests:25 MeasuredPath:src/thing.ts}", jsonReport.Evidence)
	}
}

// TestNoReportFileIsPersisted pins "no report is persisted to a file" at
// the integration layer: every file that differs after a --format json run
// is one report.Steps[].Wrote itself names — nothing else appeared,
// including a report file.
func TestNoReportFileIsPersisted(t *testing.T) {
	stubRunner(t)

	root := t.TempDir()
	gitProject(t, root, "bun.lock")
	writeFile(t, filepath.Join(root, "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)

	previous := workingDirectory
	workingDirectory = func() (string, error) { return root, nil }
	t.Cleanup(func() { workingDirectory = previous })

	before := treeSet(t, root)

	var out bytes.Buffer
	if err := RunSync([]string{"--format", "json"}, &out); err != nil {
		t.Fatalf("RunSync() = %v", err)
	}

	var decoded report.Report
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("json.Unmarshal() = %v", err)
	}

	wantChanged := map[string]bool{
		// project.Project.EnsureDir writes .dharness/.gitignore outside the
		// Writer entirely (internal/setup/writer.go's own documented gap),
		// so no step's Wrote names it — a known exception, not a report.
		filepath.Join(root, ".dharness", ".gitignore"): true,
	}
	for _, step := range decoded.Steps {
		for _, change := range step.Wrote {
			wantChanged[filepath.Join(root, filepath.FromSlash(change.Path))] = true
		}
	}
	if len(wantChanged) < 2 {
		t.Fatal("fixture applied nothing; fix the test before trusting its assertion")
	}

	after := treeSet(t, root)
	for path := range after {
		if before[path] {
			continue
		}
		if !wantChanged[path] {
			t.Errorf("file %q appeared that no applied step's Wrote names — a report may have been persisted to disk", path)
		}
	}
}
