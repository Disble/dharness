package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type scriptedRunner struct {
	outputs map[string][]byte
}

func (runner scriptedRunner) Output(_ string, name string, args ...string) ([]byte, error) {
	key := name + " " + strings.Join(args, " ")
	output, ok := runner.outputs[key]
	if !ok {
		return nil, fmt.Errorf("unexpected process: %s", key)
	}
	return output, nil
}

func (scriptedRunner) Run(commandSpec) error { return fmt.Errorf("unexpected process execution") }

func TestRunExitsBeforeOozeWithoutStagedProductionGo(t *testing.T) {
	root := newGitFixture(t)
	writeFixtureFile(t, root, "README.md", "changed\n")
	gitFixture(t, root, "add", "README.md")

	var stdout bytes.Buffer
	tool := newTool(root, &stdout, &bytes.Buffer{})
	if err := tool.run(false); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "no staged production Go files") {
		t.Fatalf("output = %q", stdout.String())
	}
}

func TestRunRejectsPartialStaging(t *testing.T) {
	root := newGitFixture(t)
	writeFixtureFile(t, root, "internal/calc/calc.go", "package calc\nfunc Add(a, b int) int { return a - b }\n")
	gitFixture(t, root, "add", "internal/calc/calc.go")
	writeFixtureFile(t, root, "internal/calc/calc.go", "package calc\nfunc Add(a, b int) int { return a * b }\n")

	err := newTool(root, &bytes.Buffer{}, &bytes.Buffer{}).run(true)
	if err == nil || !strings.Contains(err.Error(), "partial staging is unsupported") {
		t.Fatalf("run error = %v", err)
	}
}

func TestMaterializeIndexUsesStagedIdentity(t *testing.T) {
	root := newGitFixture(t)
	staged := "package calc\nfunc Add(a, b int) int { return a - b }\n"
	writeFixtureFile(t, root, "internal/calc/calc.go", staged)
	gitFixture(t, root, "add", "internal/calc/calc.go")
	writeFixtureFile(t, root, "internal/calc/calc.go", "package calc\nfunc Add(a, b int) int { return a * b }\n")

	sandbox, cleanup, err := materializeIndex(osProcessRunner{}, root)
	if err != nil {
		t.Fatalf("materialize index: %v", err)
	}
	defer cleanup()
	got, err := os.ReadFile(filepath.Join(sandbox, "internal", "calc", "calc.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != staged {
		t.Fatalf("sandbox content = %q, want staged %q", got, staged)
	}
}

func TestDryRunReportsDerivedScopeAndCandidates(t *testing.T) {
	root := newGitFixture(t)
	writeFixtureFile(t, root, "internal/calc/calc.go", "package calc\nfunc Add(a, b int) int { return a - b }\n")
	gitFixture(t, root, "add", "internal/calc/calc.go")

	var stdout bytes.Buffer
	if err := newTool(root, &stdout, &bytes.Buffer{}).run(true); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	for _, want := range []string{"line scope", "candidate mutants", "go test -short -count=1 ./internal/calc/"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("dry output missing %q: %s", want, stdout.String())
		}
	}
}

func TestRunRejectsDerivedScopeWithNoMutationNodesBeforeOoze(t *testing.T) {
	root := newGitFixture(t)
	writeFixtureFile(t, root, "internal/empty/empty.go", "package empty\n")
	gitFixture(t, root, "add", "internal/empty/empty.go")

	err := newTool(root, &bytes.Buffer{}, &bytes.Buffer{}).run(false)
	if err == nil || !strings.Contains(err.Error(), "matched no ooze mutation nodes") {
		t.Fatalf("run error = %v", err)
	}
}

func TestComputeScopeFailsOpenWhenDiffScopeIsUnderivable(t *testing.T) {
	file := "internal/calc/calc.go"
	runner := scriptedRunner{outputs: map[string][]byte{
		"git -c core.quotePath=false diff --cached --no-ext-diff --no-renames -U0 -- " + file: []byte("@@ -1 +x @@\n"),
		"git show :" + file: []byte("package calc\nfunc Add(a, b int) int { return a + b }\n"),
	}}
	tool := &tool{cwd: t.TempDir(), runner: runner, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}

	plan, err := tool.computeScope([]string{file})
	if err != nil {
		t.Fatalf("compute scope: %v", err)
	}
	if plan.derived || plan.encoded != "" || plan.stats.Candidates == 0 {
		t.Fatalf("plan = %+v, want whole-file fallback with candidates", plan)
	}
	if !strings.Contains(plan.reason, "failing open") {
		t.Fatalf("fallback reason = %q", plan.reason)
	}
}

func TestMutationFixtureRunsOozeEndToEnd(t *testing.T) {
	if os.Getenv("DHARNESS_MUTATION_FIXTURE") != "1" {
		t.Skip("set DHARNESS_MUTATION_FIXTURE=1 to run the real ooze fixture")
	}
	root := t.TempDir()
	projectRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"go.mod", "go.sum",
		"internal/testsupport/mutation/doc.go",
		"internal/testsupport/mutation/linescope.go",
		"internal/testsupport/mutation/viruses.go",
		"internal/testsupport/mutation/mutation_test.go",
	} {
		copyFixtureFile(t, projectRoot, root, name)
	}
	writeFixtureFile(t, root, "internal/calc/calc.go", "package calc\n")
	writeFixtureFile(t, root, "internal/calc/calc_test.go", "package calc\n")
	gitFixture(t, root, "init")
	gitFixture(t, root, "config", "user.email", "fixture@example.invalid")
	gitFixture(t, root, "config", "user.name", "Mutation Fixture")
	gitFixture(t, root, "add", ".")
	gitFixture(t, root, "commit", "-m", "fixture baseline")

	writeFixtureFile(t, root, "internal/calc/calc.go", "package calc\n\nfunc Add(a, b int) int { return a + b }\n")
	writeFixtureFile(t, root, "internal/calc/calc_test.go", "package calc\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) { if Add(1, 2) != 3 { t.Fatal(\"bad sum\") } }\n")
	gitFixture(t, root, "add", "internal/calc/calc.go", "internal/calc/calc_test.go")

	var output bytes.Buffer
	if err := newTool(root, &output, &output).run(false); err != nil {
		t.Fatalf("real fixture mutation run: %v\n%s", err, output.String())
	}
	t.Logf("fixture mutation output:\n%s", output.String())
	if !strings.Contains(output.String(), "candidate mutants: 1") || !strings.Contains(output.String(), "Killed") {
		t.Fatalf("runtime output lacks scope or killed-mutant evidence:\n%s", output.String())
	}
}

func newGitFixture(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("uses the real Git executable")
	}
	root := t.TempDir()
	gitFixture(t, root, "init")
	gitFixture(t, root, "config", "user.email", "fixture@example.invalid")
	gitFixture(t, root, "config", "user.name", "Mutation Fixture")
	writeFixtureFile(t, root, "go.mod", "module example.invalid/fixture\n\ngo 1.26\n")
	writeFixtureFile(t, root, "internal/calc/calc.go", "package calc\nfunc Add(a, b int) int { return a + b }\n")
	writeFixtureFile(t, root, "internal/calc/calc_test.go", "package calc\nimport \"testing\"\nfunc TestAdd(t *testing.T) { if Add(1, 2) != 3 { t.Fatal(\"bad sum\") } }\n")
	gitFixture(t, root, "add", ".")
	gitFixture(t, root, "commit", "-m", "fixture")
	return root
}

func gitFixture(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func writeFixtureFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func copyFixtureFile(t *testing.T, sourceRoot, targetRoot, name string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(sourceRoot, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, targetRoot, name, string(content))
}
