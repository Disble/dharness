//go:build !windows

package runner

import (
	"os/exec"
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
