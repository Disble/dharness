package runner

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// Slice B2 (mutate-staged-v1.9): a managed long-lived process with whole-tree
// ownership.
//
// One-shot Run kills only the process it started: behind an npm shim that is
// cmd.exe, while the node grandchild keeps running. A persistent MSP server
// needs the opposite guarantee — closing the handle must end the leader and
// every descendant — so this abstraction owns the tree from suspended start
// to reaped wait. It reuses the platformization and environment rules one-shot
// Run already follows: shims resolve exactly as ordinary commands, and
// GIT_DIR/GIT_WORK_TREE stay removed while GIT_INDEX_FILE stays present.
//
// Server stderr is drained to the null device in the background. It is never
// offered to the frame parser, and a chatty server can never block filling a
// pipe nobody reads. Standard output is the caller's framed stream; standard
// input is the caller's request stream.
type ManagedProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader

	mu       sync.Mutex
	closed   bool
	closeErr error
	waited   bool
	release  func() error
}

// StartManaged starts cmd as an owned tree and hands back its streams. Any
// ownership step that fails refuses the start: it never falls back to an
// unowned process, because an unowned MSP server outlives the run that
// started it.
func StartManaged(cmd Command) (*ManagedProcess, error) {
	target := platformize(cmd.Name, cmd.Args)
	if target.Err != nil {
		return nil, &StartError{Command: cmd.String(), Cause: target.Err}
	}

	process := exec.Command(target.Name, target.Args...)
	if target.CmdLine != "" {
		applyCmdLine(process, target.CmdLine)
	}
	process.Dir = cmd.Dir
	process.Env = Environ()

	stdin, err := process.StdinPipe()
	if err != nil {
		return nil, &StartError{Command: cmd.String(), Cause: fmt.Errorf("open the managed stdin pipe: %w", err)}
	}
	stdoutPipe, err := process.StdoutPipe()
	if err != nil {
		return nil, &StartError{Command: cmd.String(), Cause: fmt.Errorf("open the managed stdout pipe: %w", err)}
	}
	stderrPipe, err := process.StderrPipe()
	if err != nil {
		return nil, &StartError{Command: cmd.String(), Cause: fmt.Errorf("open the managed stderr pipe: %w", err)}
	}

	// Ownership is armed before the process exists and takes effect the
	// moment it does: nothing it spawns can run outside the owned tree.
	owner, err := ownBeforeStart(process)
	if err != nil {
		return nil, &StartError{Command: cmd.String(), Cause: err}
	}
	if cmd.LowPriority {
		beforeStart(process)
	}
	if err := process.Start(); err != nil {
		_ = owner.release()
		return nil, &StartError{Command: cmd.String(), Cause: err}
	}
	if cmd.LowPriority {
		afterStart(process.Process.Pid)
	}
	if err := owner.afterStart(process); err != nil {
		_ = process.Process.Kill()
		_ = process.Wait()
		_ = owner.release()
		return nil, &StartError{Command: cmd.String(), Cause: err}
	}
	go func() {
		_, _ = io.Copy(io.Discard, stderrPipe)
	}()

	return &ManagedProcess{cmd: process, stdin: stdin, stdout: bufio.NewReader(stdoutPipe), release: owner.release}, nil
}

// exitCodeOf reads the status out of a wait failure.
func exitCodeOf(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

// isDeathReport reports whether err only says the process ran and died —
// already reaped by the wait that produced it, so Close has nothing to add.
func isDeathReport(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}

// Stdin is the request stream.
func (m *ManagedProcess) Stdin() io.Writer { return m.stdin }

// Stdout is the framed response stream, buffered so split and coalesced
// reads separate without the caller re-buffering.
func (m *ManagedProcess) Stdout() *bufio.Reader { return m.stdout }

// Wait blocks for the natural exit. It reports a clean exit as nil; a server
// ended by Close is not waited on here, it is reaped by Close.
func (m *ManagedProcess) Wait() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.waited {
		return &StartError{Command: m.cmd.Path, Cause: errors.New("Wait was already called")}
	}
	m.waited = true
	if err := m.cmd.Wait(); err != nil {
		return &ExitError{Command: m.cmd.Path, Code: exitCodeOf(err), Interrupted: false}
	}
	return nil
}

// Close ends the whole owned tree and reaps the leader. It is idempotent:
// the first call's outcome stands and later calls change nothing. A nil
// return means no owned process remains; only a cleanup that itself failed
// reports an error. A tree that already exited is not a failure — the
// postcondition holds either way.
func (m *ManagedProcess) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return m.closeErr
	}
	m.closed = true
	if err := m.release(); err != nil {
		m.closeErr = err
		return m.closeErr
	}
	_ = m.stdin.Close()
	// A Wait the owner already ran reaped the leader; running it again would
	// only report the double call, never the tree.
	if !m.waited {
		m.waited = true
		if err := m.cmd.Wait(); err != nil {
			if !isDeathReport(err) {
				m.closeErr = &StartError{Command: m.cmd.Path, Cause: fmt.Errorf("reap the managed process: %w", err)}
			}
		}
	}
	return m.closeErr
}
