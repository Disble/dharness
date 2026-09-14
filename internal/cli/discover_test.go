package cli

// Slice C3 (mutate-staged-v1.9): discovery wiring in the staged run. RED
// first — these tests fail until mutate_staged.go calls discovery.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/runner"
	"github.com/Disble/dharness/internal/staged"
	"github.com/Disble/dharness/internal/tool"
)

// TestMain keeps the legacy staged tests on their v1.8 paths: they predate
// discovery, and retaining every kept scope is exactly the scope v1.8 ran.
// New discovery tests override the seam per test with a cleanup restore.
func TestMain(m *testing.M) {
	previous := discoverMembership
	discoverMembership = func(_ context.Context, _ string, _ string, _ string, scopes []tool.MutationScope) (staged.Membership, error) {
		return staged.Membership{Retained: scopes}, nil
	}
	code := m.Run()
	discoverMembership = previous
	os.Exit(code)
}

func scriptDiscovery(t *testing.T, membership staged.Membership, err error) {
	t.Helper()
	previous := discoverMembership
	t.Cleanup(func() { discoverMembership = previous })
	discoverMembership = func(_ context.Context, _ string, _ string, _ string, _ []tool.MutationScope) (staged.Membership, error) {
		return membership, err
	}
}

// stageKeptTS stages a TypeScript file the fake classifier keeps: the tsc
// fake emits non-empty output for it, so it survives as a runtime candidate.
// It registers the only runner fake, recording into captured.
func stageKeptTS(t *testing.T, captured *record, root, path, contents, emit string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName("tsc")), "")
	stageFile(t, root, path, contents)
	t.Cleanup(runner.SetForTest(stagedRunner(captured, map[string]string{path: emit}, "")))
}

// stagedConfigMutateEntries reads the mutate list out of the generated staged
// Stryker config sitting in dir.
func stagedConfigMutateEntries(t *testing.T, dir string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, stagedStrykerConfig))
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	var config struct {
		Mutate []string `json:"mutate"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("parse generated config: %v", err)
	}
	return config.Mutate
}

func TestDiscoverNothingRetainedExitsZeroWithoutStryker(t *testing.T) {
	root, captured := newStagedRepo(t)
	stageKeptTS(t, captured, root, "src/a.ts", "export const a = 1;\n", "console.log(1);")
	scriptDiscovery(t, staged.Membership{BothOmitted: []string{"src/a.ts"}}, nil)

	var out strings.Builder
	if err := RunMutate([]string{"--staged"}, &out); err != nil {
		t.Fatalf("RunMutate() = %v, want nil: no retained range means no mutation", err)
	}
	got := out.String()
	if !strings.Contains(got, "outside Stryker's mutate set: src/a.ts") {
		t.Errorf("output = %q, want the both-omitted outcome", got)
	}
	if !strings.Contains(got, "discover ") || strings.Contains(got, "discover not started") || strings.Contains(got, "discover failed:") {
		t.Errorf("output = %q, want discover completed with a duration", got)
	}
	for _, want := range []string{"related not started", "stryker not started"} {
		if !strings.Contains(got, want) {
			t.Errorf("output = %q, want it to contain %q", got, want)
		}
	}
	if commandExists(captured, "stryker") {
		t.Errorf("commands = %+v, want no stryker run without a retained range", captured.commands)
	}
}

func TestDiscoverRetainedNarrowsTheMutationScope(t *testing.T) {
	root, captured := newStagedRepo(t)
	writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName("tsc")), "")
	stageFile(t, root, "src/a.ts", "export const a = 1;\nexport const b = 2;\nexport const c = 3;\nexport const d = 4;\nexport const e = 5;\n")
	tscFake := stagedRunner(captured, map[string]string{"src/a.ts": "console.log(1);"}, "")
	var mutateEntries []string
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, stdout, stderr io.Writer) error {
		if cmd.Label == "stryker" {
			mutateEntries = stagedConfigMutateEntries(t, cmd.Dir)
			fileName, err := jsonReporterFileNameIn(cmd.Dir)
			if err != nil {
				return err
			}
			if err := writeMutationReport(fileName, minimalStagedReport("src/a.ts")); err != nil {
				return err
			}
		}
		return tscFake(cmd, stdout, stderr)
	}))
	scriptDiscovery(t, staged.Membership{
		Retained: []tool.MutationScope{{Path: "src/a.ts", Start: 1, End: 2}},
	}, nil)

	var out strings.Builder
	if err := RunMutate([]string{"--staged"}, &out); err != nil {
		t.Fatalf("RunMutate() = %v, want nil", err)
	}
	if len(mutateEntries) != 1 || mutateEntries[0] != "src/a.ts:1-2" {
		t.Errorf("mutate entries = %v, want exactly [src/a.ts:1-2]", mutateEntries)
	}
}

func TestDiscoverFailureFailsTheDiscoverPhase(t *testing.T) {
	root, captured := newStagedRepo(t)
	stageKeptTS(t, captured, root, "src/a.ts", "export const a = 1;\n", "console.log(1);")
	scriptDiscovery(t, staged.Membership{}, errors.New("MSP configure: no such config"))

	var out strings.Builder
	err := RunMutate([]string{"--staged"}, &out)
	if err == nil || !strings.Contains(err.Error(), "MSP configure") {
		t.Fatalf("RunMutate() = %v, want the discovery failure", err)
	}
	if !strings.Contains(out.String(), "discover failed: MSP configure: no such config") {
		t.Errorf("output = %q, want the failed discover phase named", out.String())
	}
}

func TestDiscoverMarksStrykerCompleteOnSurvivorVerdict(t *testing.T) {
	root, captured := newStagedRepo(t)
	stageFile(t, root, "src/a.js", "export const a = 1;\n")
	report := `{"schemaVersion":"1.0","files":{"src/a.js":{"language":"javascript","mutants":[` +
		`{"id":"1","mutatorName":"BooleanLiteral","status":"Survived","location":{"start":{"line":1,"column":1},"end":{"line":1,"column":2}}}` +
		`]}}}`
	t.Cleanup(runner.SetForTest(stagedRunner(captured, nil, report)))

	var out strings.Builder
	err := RunMutate([]string{"--staged"}, &out)
	var survivors *SurvivorsError
	if !errors.As(err, &survivors) {
		t.Fatalf("RunMutate() = %v, want SurvivorsError", err)
	}
	got := out.String()
	if strings.Contains(got, "stryker failed:") || strings.Contains(got, "stryker not started") {
		t.Errorf("output = %q, want stryker completed: a computed survivor verdict is not instrumentation failure", got)
	}
}
