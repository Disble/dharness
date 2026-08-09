//go:build windows

package runner

import (
	"errors"
	"syscall"
)

// Windows reports a directory still held by an exiting process as sharing
// violation or "directory not empty", neither of which maps to a portable
// errors.Is target.
const (
	errorSharingViolation = syscall.Errno(32)
	errorLockViolation    = syscall.Errno(33)
	errorDirNotEmpty      = syscall.Errno(145)
)

func isBusy(err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	switch errno {
	case errorSharingViolation, errorLockViolation, errorDirNotEmpty:
		return true
	}
	return false
}
