//go:build !windows

package runner

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// lowNiceness is high enough to yield to anything interactive and low enough
// that the run still finishes.
const lowNiceness = 10

// platformize is a no-op everywhere except Windows, where npm's .cmd shims
// cannot be executed directly.
func platformize(name string, args []string) invocation {
	return invocation{Name: name, Args: args}
}

// applyCmdLine is unreachable here: platformize never sets a command line off
// Windows, because no other platform re-parses one.
func applyCmdLine(*exec.Cmd, string) {}

// beforeStart is a no-op here: POSIX renices a process that already exists.
func beforeStart(*exec.Cmd) {}

// afterStart lowers the process priority. A failure is deliberately ignored:
// the run is still correct at normal priority, and refusing to mutate because
// the machine would not renice would be a worse trade than a busy laptop.
func afterStart(pid int) {
	_ = syscall.Setpriority(syscall.PRIO_PROCESS, pid, lowNiceness)
}

// lowerableTree is a process started as the leader of its own process group,
// so that one call can lower everything it ever started:
// setpriority(PRIO_PGRP) reaches children and grandchildren that already
// exist. Measured on WSL2 (kernel 6.6): niceness went from 0 to 10 across the
// whole group.
//
// The group changes who a terminal's Ctrl-C reaches, so the tree forwards it.
// The terminal signals its foreground process group, which is dharness's and
// no longer the child's. Measured through a harness shaped like dharness,
// Stryker and two workers: with Setpgid alone, Ctrl-C left Stryker and both
// workers alive, orphaned to init; with SIGINT and SIGTERM forwarded to the
// group, none survived, and the child still ended by `signal: interrupt`, so
// endedByConsoleInterrupt keeps its meaning.
type lowerableTree struct {
	pgid    int
	signals chan os.Signal
	quit    chan struct{}
	done    chan struct{}
}

// holdTree asks for the new group and starts catching the interrupts before
// the process exists: one caught in between waits in the channel and is
// forwarded once the group does.
func holdTree(process *exec.Cmd) (*lowerableTree, error) {
	if process.SysProcAttr == nil {
		process.SysProcAttr = &syscall.SysProcAttr{}
	}
	process.SysProcAttr.Setpgid = true
	t := &lowerableTree{signals: make(chan os.Signal, 1), quit: make(chan struct{}), done: make(chan struct{})}
	signal.Notify(t.signals, syscall.SIGINT, syscall.SIGTERM)
	return t, nil
}

// started records the group and forwards every caught interrupt to it until
// release.
func (t *lowerableTree) started(process *exec.Cmd) error {
	t.pgid = process.Process.Pid
	go func() {
		defer close(t.done)
		for {
			select {
			case caught := <-t.signals:
				if sig, ok := caught.(syscall.Signal); ok {
					_ = syscall.Kill(-t.pgid, sig)
				}
			case <-t.quit:
				return
			}
		}
	}()
	return nil
}

// lower renices the whole group. A failure is ignored for the reason
// afterStart's is.
func (t *lowerableTree) lower() {
	_ = syscall.Setpriority(syscall.PRIO_PGRP, t.pgid, lowNiceness)
}

// kill ends the whole group, not just the leader, so a cancelled run leaves
// no grandchild behind.
func (t *lowerableTree) kill() error {
	return syscall.Kill(-t.pgid, syscall.SIGKILL)
}

// release stops catching interrupts, which fall back to whatever handled
// them before.
func (t *lowerableTree) release() {
	signal.Stop(t.signals)
	if t.pgid != 0 {
		close(t.quit)
		<-t.done
	}
}

// endedByConsoleInterrupt reports whether the process died of SIGINT or
// SIGTERM under their default disposition — the two signals dharness itself
// treats as an interrupt, and the ones a terminal's Ctrl-C or a signal to the
// whole process group delivers to it and its children at once.
func endedByConsoleInterrupt(state *os.ProcessState) bool {
	status, ok := state.Sys().(syscall.WaitStatus)
	return ok && status.Signaled() && (status.Signal() == syscall.SIGINT || status.Signal() == syscall.SIGTERM)
}
