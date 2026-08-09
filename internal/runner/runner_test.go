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
