//go:build windows

package runner

import (
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// belowNormalPriorityClass is CREATE_NO_WINDOW's quieter neighbour: the process
// still runs whenever the machine is idle, and yields the moment anything else
// wants the CPU.
const belowNormalPriorityClass = 0x00004000

// platformize routes .cmd and .bat shims through cmd.exe.
//
// npm-installed binaries land in node_modules/.bin as .cmd shims on Windows,
// and CreateProcess cannot execute those directly. Every tool dharness wraps
// is installed that way, so without this the local path never works and every
// invocation silently falls back to the remote one.
func platformize(name string, args []string) (string, []string) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".cmd", ".bat":
		return "cmd", append([]string{"/c", name}, args...)
	default:
		return name, args
	}
}

func beforeStart(process *exec.Cmd) {
	if process.SysProcAttr == nil {
		process.SysProcAttr = &syscall.SysProcAttr{}
	}
	process.SysProcAttr.CreationFlags |= belowNormalPriorityClass
}

// afterStart is a no-op here: Windows takes the priority at creation time.
func afterStart(int) {}
