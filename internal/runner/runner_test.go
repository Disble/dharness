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

// stdinHelperEnv turns this test binary into a process that copies its own
// stdin to stdout.
//
// The alternative was naming an external program that reads stdin, which makes
// the test assert something about the machine rather than about Command. The
// re-exec trick is os/exec's own, and it proves the wiring against a real
// process: the child inherits the parent's environment, so setting the
// variable in the test is what selects the helper.
const stdinHelperEnv = "DHARNESS_RUNNER_STDIN_HELPER"

func TestMain(m *testing.M) {
	if _, isHelper := os.LookupEnv(stdinHelperEnv); isHelper {
		_, _ = io.Copy(os.Stdout, os.Stdin)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// A Command's Stdin has to reach the process, because fallow audit's scope
// arrives that way and nothing downstream would notice its absence: fed no
// diff at all, audit widens to the whole branch and still exits 0 or 1 like a
// gate that worked.
func TestCommandStdinReachesTheProcess(t *testing.T) {
	t.Setenv(stdinHelperEnv, "1")
	diff := "diff --git a/src/a.ts b/src/a.ts\n@@ -1 +1 @@\n+export const a = 1;\n"

	var out strings.Builder
	err := Run(Command{Name: os.Args[0], Stdin: strings.NewReader(diff)}, &out, io.Discard)

	if err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	if out.String() != diff {
		t.Errorf("the process read %q on stdin, want %q", out.String(), diff)
	}
}

// A Command with no Stdin must not inherit the terminal: a tool that reads
// stdin would block forever inside a git hook, which is where this gate runs.
func TestCommandWithoutStdinReadsEndOfFile(t *testing.T) {
	t.Setenv(stdinHelperEnv, "1")

	var out strings.Builder
	err := Run(Command{Name: os.Args[0]}, &out, io.Discard)

	if err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	if out.String() != "" {
		t.Errorf("the process read %q on stdin, want nothing", out.String())
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
