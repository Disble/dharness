package app_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/app"
)

func TestPolicyFacadeIsReadOnlyAndPreservesUnadoptedProjects(t *testing.T) {
	root := t.TempDir()
	var output bytes.Buffer
	err := app.RunArgs([]string{"policy", "--root", root}, &output)
	if err != nil || output.String() != "{\"version\":1,\"active\":false,\"files\":[]}\n" {
		t.Fatalf("output = %q, error = %v", output.String(), err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("policy facade wrote into the project: %v, %v", entries, err)
	}
}

type rejectedPolicyOutput struct {
	err error
}

func (w rejectedPolicyOutput) Write([]byte) (int, error) { return 0, w.err }

func TestPolicyFacadeReportsJSONWriteFailureWithoutChangingConsumer(t *testing.T) {
	root := t.TempDir()
	failure := errors.New("closed output")
	err := app.RunArgs([]string{"policy", "--root", root}, rejectedPolicyOutput{err: failure})
	if !errors.Is(err, failure) || err.Error() != "policy: write resolved JSON: closed output" || app.ExitCode(err) != 1 {
		t.Fatalf("write error = %v, exit = %d", err, app.ExitCode(err))
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("output failure changed consumer: %v, %v", entries, readErr)
	}
}

func TestPolicyFacadeRejectsInvalidManifestWithoutPartialJSON(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "coverage-tiers.json")
	if err := os.WriteFile(path, []byte(`{"version":7}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err := app.RunArgs([]string{"policy", "--root", root}, &output)
	if err == nil || err.Error() != "coverage-tiers.json: version must be 1" || output.Len() != 0 || app.ExitCode(err) == 0 {
		t.Fatalf("output = %q, error = %v, exit = %d", output.String(), err, app.ExitCode(err))
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != `{"version":7}` {
		t.Fatalf("rejection changed manifest: %q, %v", after, err)
	}
}

func TestPolicyFacadeDescribesItsIntegrationOnlyRootRequirement(t *testing.T) {
	var output bytes.Buffer
	err := app.RunArgs([]string{"policy"}, &output)
	if err == nil || err.Error() != "policy: --root is required by the coverage integration" || output.Len() != 0 {
		t.Fatalf("missing-root output = %q, error = %v", output.String(), err)
	}
	root := t.TempDir()
	output.Reset()
	err = app.RunArgs([]string{"policy", "--root", root, "extra"}, &output)
	if err == nil || err.Error() != "policy: unexpected positional arguments; use --root <repository-root>" || output.Len() != 0 || app.ExitCode(err) != 1 {
		t.Fatalf("positional output = %q, error = %v, exit = %d", output.String(), err, app.ExitCode(err))
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("rejected positional input changed consumer: %v, %v", entries, readErr)
	}
	output.Reset()
	err = app.RunArgs([]string{"policy", "--unsupported"}, &output)
	if err == nil || err.Error() != "flag provided but not defined: -unsupported" || app.ExitCode(err) != 1 {
		t.Fatalf("unknown flag output = %q, error = %v", output.String(), err)
	}
	output.Reset()
	if err := app.RunArgs([]string{"policy", "--help"}, &output); err != nil || !strings.Contains(output.String(), "Read-only coverage-tier resolution") || !strings.Contains(output.String(), "--root") {
		t.Fatalf("help output = %q, error = %v", output.String(), err)
	}
	output.Reset()
	if err := app.RunArgs(nil, &output); err != nil || !strings.Contains(output.String(), "INTEGRATION HELPERS") || !strings.Contains(output.String(), "policy --root") || !strings.Contains(output.String(), "not a test runner") {
		t.Fatalf("global help output = %q, error = %v", output.String(), err)
	}
}
