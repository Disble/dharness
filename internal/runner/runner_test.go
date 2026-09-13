package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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

// argvHelperEnv turns this test binary into a process that reports the
// arguments it was handed, one per line.
//
// It exists because the only question worth asking about a shim is what the
// wrapped program finally receives. Asserting on the command dharness builds
// would pass while cmd.exe rewrote it in transit, which is the exact defect
// these tests were added for.
const argvHelperEnv = "DHARNESS_RUNNER_ARGV_HELPER"

// environHelperEnv turns this test binary into a process that reports the
// names of the git variables it inherited, one per line.
//
// Names only, values never: the question is which variables reached the child,
// and a test that compared paths would start asserting about the machine the
// suite runs on.
const environHelperEnv = "DHARNESS_RUNNER_ENVIRON_HELPER"

// sleepHelperEnv turns this test binary into a process that sleeps far
// longer than any test's own patience, so a test can prove a Context
// cancellation actually kills it rather than waiting the sleep out.
const sleepHelperEnv = "DHARNESS_RUNNER_SLEEP_HELPER"

func TestMain(m *testing.M) {
	if _, isHelper := os.LookupEnv(stdinHelperEnv); isHelper {
		_, _ = io.Copy(os.Stdout, os.Stdin)
		os.Exit(0)
	}
	if _, isHelper := os.LookupEnv(argvHelperEnv); isHelper {
		for _, arg := range os.Args[1:] {
			fmt.Printf("<%s>\n", arg)
		}
		os.Exit(0)
	}
	if _, isHelper := os.LookupEnv(environHelperEnv); isHelper {
		for _, entry := range os.Environ() {
			if name, _, ok := strings.Cut(entry, "="); ok && strings.HasPrefix(name, "GIT_") {
				fmt.Println(name)
			}
		}
		os.Exit(0)
	}
	if pidFile, isHelper := os.LookupEnv(grandchildHelperEnv); isHelper {
		startGrandchildThenSleep(pidFile)
	}
	if how, isHelper := os.LookupEnv(endingHelperEnv); isHelper {
		if how == "interrupted" {
			endAsAConsoleInterruptWould()
		}
		os.Exit(3)
	}
	if _, isHelper := os.LookupEnv(sleepHelperEnv); isHelper {
		time.Sleep(2 * time.Minute)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// endingHelperEnv turns this test binary into a process that ends the way its
// value says: "interrupted" as a console interrupt ends a process that leaves
// it to the default handler, anything else as an ordinary failure.
const endingHelperEnv = "DHARNESS_RUNNER_ENDING_HELPER"

// TestExitErrorTellsAConsoleInterruptFromAFailure pins what a staged run needs
// to report an interrupt as an interrupt. A console interrupt reaches every
// process attached to the console at once, so the child can die of it before
// dharness's own handler has cancelled anything. Measured through a real
// `taskkill /PID` mid-run: Stryker's shim came back with 3221225786 and the run
// reported "stryker exited with code 3221225786" beside a pointer to Stryker's
// help. How the process ended is the answer that does not race.
func TestExitErrorTellsAConsoleInterruptFromAFailure(t *testing.T) {
	for how, want := range map[string]bool{"interrupted": true, "failed": false} {
		t.Run(how, func(t *testing.T) {
			t.Setenv(endingHelperEnv, how)

			err := Run(Command{Name: os.Args[0]}, io.Discard, io.Discard)

			var exit *ExitError
			if !errors.As(err, &exit) {
				t.Fatalf("Run() = %v, want an ExitError", err)
			}
			if Interrupted(err) != want {
				t.Errorf("Interrupted(%v) = %v, want %v", err, Interrupted(err), want)
			}
		})
	}
	if Interrupted(errors.New("not a process at all")) {
		t.Error("Interrupted() = true for an error no process returned")
	}
}

// grandchildHelperEnv turns this test binary into the shape of an npm .cmd
// shim: a process that starts the real program as its own child, hands it the
// standard output it was given, and waits. The value names the file the
// grandchild's pid is written to.
const grandchildHelperEnv = "DHARNESS_RUNNER_GRANDCHILD_HELPER"

func startGrandchildThenSleep(pidFile string) {
	grandchild := exec.Command(os.Args[0])
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, grandchildHelperEnv+"=") {
			grandchild.Env = append(grandchild.Env, entry)
		}
	}
	grandchild.Env = append(grandchild.Env, sleepHelperEnv+"=1")
	grandchild.Stdout = os.Stdout
	if err := grandchild.Start(); err != nil {
		os.Exit(3)
	}
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(grandchild.Process.Pid)), 0o600); err != nil {
		os.Exit(4)
	}
	time.Sleep(2 * time.Minute)
	os.Exit(0)
}

// TestCommandContextCancellationIsNotPinnedByAGrandchildHoldingTheOutput pins
// what killing a shim does not reach.
//
// Kill ends the process runner started, which for every locally installed
// tool on Windows is cmd.exe running a .cmd shim, never the node process the
// shim started. That grandchild still holds the output pipe, and Wait reads
// the pipe to its end. Measured through a real npm-style shim running a node
// script that sleeps 30 seconds: cancelled after 1 second, Run returned after
// 30.10. The staged path hands tsc and `vitest list` a buffer — a pipe — so
// an interrupt during a 70-second `vitest list` kept dharness waiting out the
// whole suite before its cleanup could run.
func TestCommandContextCancellationIsNotPinnedByAGrandchildHoldingTheOutput(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
	t.Setenv(grandchildHelperEnv, pidFile)
	t.Cleanup(func() { killRecordedProcess(pidFile) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	var output strings.Builder
	go func() {
		done <- Run(Command{Name: os.Args[0], Context: ctx}, &output, io.Discard)
	}()
	waitForFile(t, pidFile)
	cancel()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run() had not returned 10s after cancellation: the grandchild still holding the output pipe pinned Wait")
	}
}

// waitForFile blocks until path exists, failing the test after ten seconds.
func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s never appeared: the helper did not start its grandchild", path)
}

// killRecordedProcess ends the process whose pid is in pidFile, if any, so a
// grandchild this suite started never outlives it.
func killRecordedProcess(pidFile string) {
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(string(raw))
	if err != nil {
		return
	}
	if process, err := os.FindProcess(pid); err == nil {
		_ = process.Kill()
	}
}

// TestCommandContextCancellationKillsTheProcess pins a long mutation run's
// own Ctrl-C story directly: a Command whose Context is cancelled while the
// process is still running must have that process killed, so Run returns
// instead of blocking on a child that outlives the run that started it —
// and so a staged mutation run's own snapshot cleanup can proceed rather
// than racing a still-running Stryker still reading from it.
func TestCommandContextCancellationKillsTheProcess(t *testing.T) {
	t.Setenv(sleepHelperEnv, "1")
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- Run(Command{Name: os.Args[0], Context: ctx}, io.Discard, io.Discard)
	}()

	// Give the process a moment to actually start before cancelling, so this
	// exercises "kill a running process" rather than "never start one".
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Error("Run() = nil, want an error: a killed process is not a clean exit")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run() did not return after Context cancellation: the process was not killed")
	}
}

// TestCommandWithAnUncancelledContextStillCompletes pins the no-op half: a
// Context that is never cancelled must not change ordinary completion at
// all, and the watcher goroutine it starts must not leak past Run returning.
func TestCommandWithAnUncancelledContextStillCompletes(t *testing.T) {
	t.Setenv(stdinHelperEnv, "1")

	var out strings.Builder
	err := Run(Command{Name: os.Args[0], Context: context.Background(), Stdin: strings.NewReader("hi")}, &out, io.Discard)

	if err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	if out.String() != "hi" {
		t.Errorf("the process read %q on stdin, want %q", out.String(), "hi")
	}
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
