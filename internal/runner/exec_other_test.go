//go:build !windows

package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// startAtNormalPriority is a no-op here: lowering a niceness back to 0 needs
// privilege, so a test binary started niced stays niced. The Windows twin
// exists because a below-normal class is inherited the same way and can be
// raised freely.
func startAtNormalPriority() {}

// endAsAConsoleInterruptWould ends this process with SIGINT under the default
// disposition, which is how a terminal's Ctrl-C ends a process that does not
// handle it.
func endAsAConsoleInterruptWould() {
	signal.Reset(syscall.SIGINT)
	_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	time.Sleep(time.Minute)
}

// priorityOf names the niceness pid runs at: "normal" for 0, "low" for
// lowNiceness, or the raw value for anything else.
//
// Linux's getpriority system call answers 20 minus the niceness, so that a
// successful result is never negative, and Go's syscall package hands that
// value back unconverted. The BSDs answer the niceness itself.
func priorityOf(t *testing.T, pid int) string {
	t.Helper()
	value, err := syscall.Getpriority(syscall.PRIO_PROCESS, pid)
	if err != nil {
		t.Fatalf("Getpriority(%d): %v", pid, err)
	}
	nice := value
	if runtime.GOOS == "linux" {
		nice = 20 - value
	}
	switch nice {
	case 0:
		return "normal"
	case lowNiceness:
		return "low"
	}
	return fmt.Sprintf("nice %d", nice)
}

// processGone reports whether pid no longer runs. A zombie counts as gone: it
// has ended, and only its parent's reaping is still pending.
func processGone(pid int) bool {
	if stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
		// The state follows the parenthesised command name, which may itself
		// hold spaces or parentheses, so it is found from the last ")".
		rest := string(stat[strings.LastIndexByte(string(stat), ')')+1:])
		return strings.HasPrefix(strings.TrimSpace(rest), "Z")
	}
	return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

// TestContextCancellationEndsTheWholeGroup pins what the process group buys a
// cancelled run. Killing only the leader leaves its children running —
// runner.go's own Context watcher documents a grandchild that outlived it —
// while the group the leader heads holds everything it started.
func TestContextCancellationEndsTheWholeGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tree := startTree(t, Command{Context: ctx, LowerPriorityWhen: func() bool { return false }})

	cancel()
	if err := tree.wait(t, "the Context was cancelled"); err == nil {
		t.Error("Run() = nil, want an error: a killed process is not a clean exit")
	}

	deadline := time.Now().Add(5 * time.Second)
	for !processGone(tree.grandchild) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !processGone(tree.grandchild) {
		t.Errorf("grandchild %d still runs after the run was cancelled", tree.grandchild)
	}
}

// TestAnInterruptReachesTheWholeGroup pins the forwarding a terminal's Ctrl-C
// depends on once the child heads its own group. The terminal signals only
// its foreground group, which is dharness's, and the measured result without
// forwarding was Stryker and both workers left alive, orphaned to init. The
// child must still end the way an interrupt ends it, or Interrupted stops
// telling a Ctrl-C from a failure.
func TestAnInterruptReachesTheWholeGroup(t *testing.T) {
	tree := startTree(t, Command{LowerPriorityWhen: func() bool { return false }})

	// What the terminal does: interrupt the process dharness runs in.
	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	if err := tree.wait(t, "the interrupt"); !Interrupted(err) {
		t.Errorf("Run() = %v, want the child ended by the interrupt", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for !processGone(tree.grandchild) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !processGone(tree.grandchild) {
		t.Errorf("grandchild %d still runs after the interrupt", tree.grandchild)
	}
}
