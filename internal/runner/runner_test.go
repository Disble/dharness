package runner

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestExitErrorReportsTheToolAndItsCode(t *testing.T) {
	err := error(&ExitError{Command: "fallow", Code: 3})

	if !strings.Contains(err.Error(), "fallow") || !strings.Contains(err.Error(), "3") {
		t.Errorf("ExitError message loses the tool or the code: %s", err)
	}
}

// A missing binary and a failing check need different remedies, so they are
// different types rather than one error with a code of zero.
func TestStartErrorUnwrapsToItsCause(t *testing.T) {
	err := error(&StartError{Command: "stryker", Cause: os.ErrNotExist})

	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("StartError does not unwrap to its cause: %v", err)
	}
}

func TestSetForTestRestoresTheRealRunner(t *testing.T) {
	restore := SetForTest(func(Command, io.Writer, io.Writer) error { return nil })
	restore()

	if err := Run(Command{Name: "definitely-not-a-real-binary"}, io.Discard, io.Discard); err == nil {
		t.Error("restore() left the test double installed")
	}
}

// TestExitCodeMapsNilZeroPropagatesToolCode pins ExitCode's move from
// internal/app (design.md Decision 1): a nil error is 0, a wrapped tool's own
// exit code is propagated unchanged, and a code of 0 on a non-nil error still
// falls back to 1 — matching internal/app.ExitCode's exact behaviour before
// the move, since internal/app.ExitCode becomes a one-line forwarder to this.
func TestExitCodeMapsNilZeroPropagatesToolCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil error", nil, 0},
		{"tool exit code propagates", &ExitError{Command: "fallow", Code: 2}, 2},
		{"zero code falls back to one", &ExitError{Command: "fallow", Code: 0}, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExitCode(tc.err); got != tc.want {
				t.Errorf("ExitCode() = %d, want %d", got, tc.want)
			}
		})
	}
}
