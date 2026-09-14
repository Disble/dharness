package cli

// Slice D (mutate-staged-v1.9): related-test wiring in the staged run.

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/runner"
	"github.com/Disble/dharness/internal/staged"
	"github.com/Disble/dharness/internal/tool"
)

func retainSingle(t *testing.T, path string, start, end int) staged.Membership {
	t.Helper()
	return staged.Membership{Retained: []tool.MutationScope{{Path: path, Start: start, End: end}}}
}

func isNoRelatedTests(err error) bool { return errors.Is(err, ErrNoRelatedTests) }

func testContext(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}

func vitestRelatedFake(t *testing.T, captured *record, tscOutputs map[string]string, payload string, fail error) func(runner.Command, io.Writer, io.Writer) error {
	tscFake := stagedRunner(captured, tscOutputs, "")
	return func(cmd runner.Command, stdout, stderr io.Writer) error {
		if cmd.Label != "vitest" {
			return tscFake(cmd, stdout, stderr)
		}
		if fail != nil {
			return fail
		}
		outPath := flagValueCLI(cmd.Args, "--outputFile")
		if outPath == "" {
			t.Fatalf("vitest related ran without --outputFile: %v", cmd.Args)
		}
		writeFile(t, outPath, payload)
		return captured.run(cmd, stdout, stderr)
	}
}

func TestRelatedZeroAggregateExitsOneWithoutStryker(t *testing.T) {
	root, captured := newStagedRepo(t)
	writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName("vitest")), "")
	writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName("tsc")), "")
	stageFile(t, root, "src/a.ts", "export const a = 1;\n")
	t.Cleanup(runner.SetForTest(vitestRelatedFake(t, captured, map[string]string{"src/a.ts": "console.log(1);"}, `{"numTotalTests":0}`, nil)))
	scriptDiscovery(t, retainSingle(t, "src/a.ts", 1, 1), nil)

	var out strings.Builder
	err := RunMutate([]string{"--staged"}, &out)
	if !isNoRelatedTests(err) {
		t.Fatalf("RunMutate() = %v, want the zero-aggregate refusal", err)
	}
	got := out.String()
	if !strings.Contains(got, "no test reaches: src/a.ts; the fix is a test that imports them") {
		t.Errorf("output = %q, want the zero-aggregate line naming the file and the fix", got)
	}
	if strings.Contains(got, "related failed:") || strings.Contains(got, "related not started") {
		t.Errorf("output = %q, want related completed: zero is a verdict, not instrumentation failure", got)
	}
	if !strings.Contains(got, "stryker not started") {
		t.Errorf("output = %q, want stryker never started", got)
	}
	if commandExists(captured, "stryker") {
		t.Errorf("commands = %+v, want no stryker run after a zero aggregate", captured.commands)
	}
}

func TestRelatedPositiveAggregateProceedsToStryker(t *testing.T) {
	root, captured := newStagedRepo(t)
	writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName("vitest")), "")
	writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName("tsc")), "")
	stageFile(t, root, "src/a.ts", "export const a = 1;\n")
	base := stagedRunner(captured, map[string]string{"src/a.ts": "console.log(1);"}, minimalStagedReport("src/a.ts"))
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, stdout, stderr io.Writer) error {
		if cmd.Label == "vitest" {
			writeFile(t, flagValueCLI(cmd.Args, "--outputFile"), `{"numTotalTests":2}`)
		}
		return base(cmd, stdout, stderr)
	}))
	scriptDiscovery(t, retainSingle(t, "src/a.ts", 1, 1), nil)

	var out strings.Builder
	if err := RunMutate([]string{"--staged"}, &out); err != nil {
		t.Fatalf("RunMutate() = %v, want nil", err)
	}
	if !commandExists(captured, "stryker") {
		t.Errorf("commands = %+v, want the mutation run after a positive aggregate", captured.commands)
	}
	if strings.Contains(out.String(), "related failed:") {
		t.Errorf("output = %q, want related completed", out.String())
	}
}

func TestRelatedMalformedJSONFailsThePhase(t *testing.T) {
	root, captured := newStagedRepo(t)
	writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName("vitest")), "")
	writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName("tsc")), "")
	stageFile(t, root, "src/a.ts", "export const a = 1;\n")
	t.Cleanup(runner.SetForTest(vitestRelatedFake(t, captured, map[string]string{"src/a.ts": "console.log(1);"}, `{"numTotalTests":`, nil)))
	scriptDiscovery(t, retainSingle(t, "src/a.ts", 1, 1), nil)

	var out strings.Builder
	err := RunMutate([]string{"--staged"}, &out)
	if err == nil || !strings.Contains(err.Error(), "related-test JSON failure") {
		t.Fatalf("RunMutate() = %v, want the JSON failure", err)
	}
	if !strings.Contains(out.String(), "related failed: related-test JSON failure") {
		t.Errorf("output = %q, want the failed related phase named", out.String())
	}
	if commandExists(captured, "stryker") {
		t.Errorf("commands = %+v, want no stryker run on unreadable related output", captured.commands)
	}
}

func TestRelatedJestBranchListsWithoutExecuting(t *testing.T) {
	root, captured := newStagedRepo(t)
	p, err := project.Discover(root)
	if err != nil {
		t.Fatalf("Discover() = %v", err)
	}
	selection := project.StrykerSelection{TestRunner: "jest"}
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, stdout, stderr io.Writer) error {
		if cmd.Label != "jest" {
			return captured.run(cmd, stdout, stderr)
		}
		for _, arg := range []string{"--findRelatedTests", "--listTests", "--json"} {
			if !argsContain(cmd.Args, arg) {
				t.Errorf("jest args = %v, want the list-only form", cmd.Args)
			}
		}
		_, _ = io.WriteString(stdout, `["src/a.test.js"]`)
		return captured.run(cmd, stdout, stderr)
	}))

	n, err := runRelatedTests(testContext(t), p, selection, root, []string{"src/a.js"})
	if err != nil || n != 1 {
		t.Errorf("runRelatedTests(jest) = %d, %v; want 1, nil", n, err)
	}
}
