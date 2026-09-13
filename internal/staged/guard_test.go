package staged

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/runner"
)

// fakeVitestNeverCalled fails the test the instant runner.Run is invoked, for
// asserting the guard never even asks vitest anything.
func fakeVitestNeverCalled(t *testing.T) func() {
	t.Helper()
	return runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		t.Fatalf("vitest invoked, want no invocation at all: %v", cmd.Args)
		return nil
	})
}

// TestGuardVitestSuiteRefusesWhenListFails pins the measured basis directly:
// Stryker's own vitest-runner dry run silently drops a test file that fails
// to load rather than reporting it — measured on a real project, 177 of 267
// tests dropped and the mutation score read 0.00 with no error at all —
// while `vitest list` fails loudly on the same load error. A non-zero exit
// from `list` must refuse before Stryker ever runs.
func TestGuardVitestSuiteRefusesWhenListFails(t *testing.T) {
	defer runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		return &runner.ExitError{Command: cmd.String(), Code: 1}
	})()

	err := GuardVitestSuite("vitest", "vitest", t.TempDir(), "")

	var suite *VitestSuiteError
	if !errors.As(err, &suite) {
		t.Fatalf("GuardVitestSuite() = %v, want VitestSuiteError", err)
	}
	if !strings.Contains(err.Error(), "exited with code 1") {
		t.Errorf("GuardVitestSuite() = %q, want it to name the exit code", err.Error())
	}
}

// TestGuardVitestSuitePassesWhenListSucceeds is the other side: a suite that
// loads cleanly must not be refused.
func TestGuardVitestSuitePassesWhenListSucceeds(t *testing.T) {
	defer runner.SetForTest(func(_ runner.Command, _, _ io.Writer) error {
		return nil
	})()

	if err := GuardVitestSuite("vitest", "vitest", t.TempDir(), ""); err != nil {
		t.Errorf("GuardVitestSuite() = %v, want nil", err)
	}
}

// TestGuardVitestSuiteNeverRunsForJest pins the runner boundary: jest's own
// runner already fails loudly on a file that cannot load, so there is
// nothing here for jest to guard against, and `vitest list` must never even
// be invoked for a jest project.
func TestGuardVitestSuiteNeverRunsForJest(t *testing.T) {
	defer fakeVitestNeverCalled(t)()

	if err := GuardVitestSuite("vitest", "jest", t.TempDir(), ""); err != nil {
		t.Errorf("GuardVitestSuite() = %v, want nil: jest is not guarded", err)
	}
}

// TestGuardVitestSuitePassesTheConfiguredConfigFile pins the config the
// guard asks about: a project that set vitest.configFile in its JSON Stryker config must
// have that same file handed to `vitest list --config <file>`, or list could
// resolve a different vitest config than the one Stryker itself will load.
func TestGuardVitestSuitePassesTheConfiguredConfigFile(t *testing.T) {
	var captured []string
	defer runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		captured = append([]string{}, cmd.Args...)
		return nil
	})()

	if err := GuardVitestSuite("vitest", "vitest", t.TempDir(), "vitest.unit.config.ts"); err != nil {
		t.Fatalf("GuardVitestSuite() = %v", err)
	}
	want := []string{"list", "--config", "vitest.unit.config.ts"}
	if len(captured) != len(want) {
		t.Fatalf("captured args = %v, want %v", captured, want)
	}
	for i := range want {
		if captured[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, captured[i], want[i])
		}
	}
}

// TestGuardVitestSuiteOmitsConfigFlagWhenNotSet pins the other side of the
// same boundary: an unconfigured project must not receive a --config flag
// at all — not even an empty one, which vitest would try to load as a path.
func TestGuardVitestSuiteOmitsConfigFlagWhenNotSet(t *testing.T) {
	var captured []string
	defer runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		captured = append([]string{}, cmd.Args...)
		return nil
	})()

	if err := GuardVitestSuite("vitest", "vitest", t.TempDir(), ""); err != nil {
		t.Fatalf("GuardVitestSuite() = %v", err)
	}
	if len(captured) != 1 || captured[0] != "list" {
		t.Errorf("captured args = %v, want [list]", captured)
	}
}

// TestGuardVitestSuiteRunsFromTheSnapshotSource pins where `list` runs: the
// snapshot's own copy of source, the same tree Stryker itself is about to
// run in, not the real project.
func TestGuardVitestSuiteRunsFromTheSnapshotSource(t *testing.T) {
	snapshotSource := t.TempDir()
	var dir string
	defer runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		dir = cmd.Dir
		return nil
	})()

	if err := GuardVitestSuite("vitest", "vitest", snapshotSource, ""); err != nil {
		t.Fatalf("GuardVitestSuite() = %v", err)
	}
	if dir != snapshotSource {
		t.Errorf("cmd.Dir = %q, want %q", dir, snapshotSource)
	}
}
