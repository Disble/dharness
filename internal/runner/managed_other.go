//go:build !windows

package runner

import (
	"os/exec"
	"syscall"
)

// Slice B2.1 (mutate-staged-v1.9): Unix tree ownership through a process
// group.
//
// The leader starts in a new process group and Close signals the negative
// group ID, so children the leader spawned die with it. An already exited
// leader does not suppress the group kill: ESRCH only means the postcondition
// — no owned process remains — already holds.
type treeOwner struct {
	pgid int
}

func ownBeforeStart(cmd *exec.Cmd) (*treeOwner, error) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	return &treeOwner{}, nil
}

func (o *treeOwner) afterStart(cmd *exec.Cmd) error {
	o.pgid = cmd.Process.Pid
	return nil
}

func (o *treeOwner) release() error {
	if o.pgid == 0 {
		return nil
	}
	if err := syscall.Kill(-o.pgid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		return err
	}
	return nil
}
