//go:build !windows

package runner

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

// endAsAConsoleInterruptWould ends this process with SIGINT under the default
// disposition, which is how a terminal's Ctrl-C ends a process that does not
// handle it.
func endAsAConsoleInterruptWould() {
	signal.Reset(syscall.SIGINT)
	_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	time.Sleep(time.Minute)
}
