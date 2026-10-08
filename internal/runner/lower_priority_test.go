package runner

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// treeHelperEnv turns this test binary into the shape of a Stryker run: a
// process that starts a long-lived child of its own — the grandchild of
// whoever ran it — reports both pids, and stays alive until told to leave.
// The value names the directory the two talk through: the helper writes
// "<pid> <grandchild pid>" to pids and exits 0 once release exists.
//
// The grandchild is started before anything can lower the tree, because the
// whole question is whether a switch made later reaches a process that
// already exists. It gets no standard output: a grandchild holding the
// output pipe would keep Run's Wait reading after the helper itself left.
const treeHelperEnv = "DHARNESS_RUNNER_TREE_HELPER"

func startTreeThenWaitForRelease(dir string) {
	grandchild := exec.Command(os.Args[0])
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, treeHelperEnv+"=") {
			grandchild.Env = append(grandchild.Env, entry)
		}
	}
	grandchild.Env = append(grandchild.Env, sleepHelperEnv+"=1")
	if err := grandchild.Start(); err != nil {
		os.Exit(3)
	}
	// Written whole and then renamed, so the test never reads half a line.
	pids := filepath.Join(dir, "pids")
	body := fmt.Sprintf("%d %d", os.Getpid(), grandchild.Process.Pid)
	if err := os.WriteFile(pids+".tmp", []byte(body), 0o600); err != nil {
		os.Exit(4)
	}
	if err := os.Rename(pids+".tmp", pids); err != nil {
		os.Exit(5)
	}
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(dir, "release")); err == nil {
			os.Exit(0)
		}
		time.Sleep(20 * time.Millisecond)
	}
	os.Exit(6)
}

// runningTree is a Run of the tree helper still in flight.
type runningTree struct {
	child, grandchild int
	dir               string

	// finished closes once Run has returned, with its result in err; closed
	// rather than sent on, so the test and its cleanup can both wait.
	finished <-chan struct{}
	err      *error
}

// startTree runs cmd as the tree helper and returns once both pids are known.
// Cleanup kills the grandchild and lets the helper leave, so nothing this
// suite starts outlives it.
func startTree(t *testing.T, cmd Command) runningTree {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(treeHelperEnv, dir)
	cmd.Name = os.Args[0]

	finished := make(chan struct{})
	var result error
	go func() {
		defer close(finished)
		result = Run(cmd, io.Discard, io.Discard)
	}()

	pidFile := filepath.Join(dir, "pids")
	// Registered before anything can fail, so a helper that was slow to
	// report still leaves, and the grandchild it may have started with it.
	t.Cleanup(func() {
		_ = os.WriteFile(filepath.Join(dir, "release"), nil, 0o600)
		select {
		case <-finished:
		case <-time.After(10 * time.Second):
		}
		var child, grandchild int
		if raw, err := os.ReadFile(pidFile); err == nil {
			if _, err := fmt.Sscan(string(raw), &child, &grandchild); err == nil {
				if process, err := os.FindProcess(grandchild); err == nil {
					_ = process.Kill()
				}
			}
		}
	})
	waitForFile(t, pidFile)
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	tree := runningTree{dir: dir, finished: finished, err: &result}
	if _, err := fmt.Sscan(string(raw), &tree.child, &tree.grandchild); err != nil {
		t.Fatalf("pids file %q: %v", raw, err)
	}
	return tree
}

// finish lets the helper exit on its own and returns what Run returned.
func (tree runningTree) finish(t *testing.T) error {
	t.Helper()
	if err := os.WriteFile(filepath.Join(tree.dir, "release"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return tree.wait(t, "the helper was released")
}

// wait returns what Run returned, failing if it has not within ten seconds of
// what was supposed to end it.
func (tree runningTree) wait(t *testing.T, after string) error {
	t.Helper()
	select {
	case <-tree.finished:
		return *tree.err
	case <-time.After(10 * time.Second):
		t.Fatalf("Run() had not returned 10s after %s", after)
		return nil
	}
}

// waitForPriority polls until pid runs at want, failing after five seconds:
// well past the 250ms poll, far short of anything a person would wait for.
func waitForPriority(t *testing.T, pid int, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	got := priorityOf(t, pid)
	for got != want && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		got = priorityOf(t, pid)
	}
	if got != want {
		t.Fatalf("pid %d runs at %s priority, want %s", pid, got, want)
	}
}

// waitForCalls polls until calls reaches at least n, failing after five
// seconds.
func waitForCalls(t *testing.T, calls *atomic.Int32, n int32) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for calls.Load() < n && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := calls.Load(); got < n {
		t.Fatalf("condition consulted %d times in 5s, want at least %d", got, n)
	}
}

// TestLowerPriorityWhenReachesAGrandchildStartedBeforeTheSwitch pins the one
// thing the two-phase Stryker run depends on. Stryker's workers already exist
// when the dry run completes, so a switch that only reached the leader, or
// only processes started after it, would leave the mutant phase — the part
// that saturates the machine — at normal priority.
func TestLowerPriorityWhenReachesAGrandchildStartedBeforeTheSwitch(t *testing.T) {
	var fire atomic.Bool
	var calls atomic.Int32
	tree := startTree(t, Command{LowerPriorityWhen: func() bool {
		calls.Add(1)
		return fire.Load()
	}})

	// Normal first: the dry run is the phase this exists to protect.
	waitForPriority(t, tree.child, "normal")
	waitForPriority(t, tree.grandchild, "normal")

	fire.Store(true)
	waitForPriority(t, tree.child, "low")
	waitForPriority(t, tree.grandchild, "low")

	// Once: a condition that keeps reporting true is not consulted again.
	fired := calls.Load()
	time.Sleep(3 * lowerPriorityPollInterval)
	if got := calls.Load(); got != fired {
		t.Errorf("condition consulted %d times after it fired, want 0", got-fired)
	}

	// Killed rather than released: a below-normal helper on a loaded machine
	// can wait seconds for the CPU it needs to notice the release file, which
	// is the very effect this field exists for.
	if process, err := os.FindProcess(tree.child); err == nil {
		_ = process.Kill()
	}
	_ = tree.wait(t, "the child was killed")
}

// TestLowerPriorityWhenThatNeverFiresLeavesNormalPriority pins the
// degradation: a dry run that fails or a Stryker that exits early never
// produces the signal, and that run simply ends at normal priority.
func TestLowerPriorityWhenThatNeverFiresLeavesNormalPriority(t *testing.T) {
	var calls atomic.Int32
	tree := startTree(t, Command{LowerPriorityWhen: func() bool {
		calls.Add(1)
		return false
	}})

	waitForCalls(t, &calls, 2)
	if got := priorityOf(t, tree.child); got != "normal" {
		t.Errorf("child runs at %s priority, want normal", got)
	}
	if got := priorityOf(t, tree.grandchild); got != "normal" {
		t.Errorf("grandchild runs at %s priority, want normal", got)
	}
	if err := tree.finish(t); err != nil {
		t.Errorf("Run() = %v, want nil", err)
	}
}

// TestBothPrioritiesAreRefusedBeforeAnythingStarts pins what a Command
// asking for both gets: a StartError naming the conflict, and no process. The
// two ask for opposite starting priorities, so honoring either would hide a
// caller's mistake.
func TestBothPrioritiesAreRefusedBeforeAnythingStarts(t *testing.T) {
	t.Setenv(argvHelperEnv, "1")
	var calls atomic.Int32
	var out strings.Builder

	err := Run(Command{Name: os.Args[0], Args: []string{"ran"}, LowPriority: true, LowerPriorityWhen: func() bool {
		calls.Add(1)
		return true
	}}, &out, &out)

	var start *StartError
	if !errors.As(err, &start) || !errors.Is(err, ErrConflictingPriority) {
		t.Fatalf("Run() = %v, want a StartError wrapping ErrConflictingPriority", err)
	}
	if out.Len() != 0 {
		t.Errorf("the process ran and printed %q, want nothing started", out.String())
	}
	if got := calls.Load(); got != 0 {
		t.Errorf("condition consulted %d times, want 0", got)
	}
}
