//go:build !windows

package runner

import (
	"errors"
	"syscall"
)

func isBusy(err error) bool {
	return errors.Is(err, syscall.EBUSY) || errors.Is(err, syscall.ENOTEMPTY)
}
