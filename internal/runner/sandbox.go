package runner

import (
	"errors"
	"io/fs"
	"os"
	"time"
)

// sandboxRemovalAttempts and sandboxRetryDelay bound how long a leftover
// sandbox is waited on before the failure is surfaced.
const (
	sandboxRemovalAttempts = 5
	sandboxRetryDelay      = 200 * time.Millisecond
)

// RemoveSandbox deletes a tool's working directory, tolerating the moment
// Windows keeps holding a handle on it after the process exits.
//
// Asking the tool to clean up after itself is not enough. Stryker's own
// cleanTempDir only runs on a successful exit, and even set to always the
// directory can still be locked for a moment on Windows — an unguarded removal
// then throws EBUSY and fails a commit that had nothing wrong with it.
//
// After the attempts are exhausted the error is returned rather than swallowed:
// a sandbox that will not go away is a real problem, and leaving it silently
// means the next run trips over it.
func RemoveSandbox(path string) error {
	for attempt := range sandboxRemovalAttempts {
		err := os.RemoveAll(path)
		if err == nil || errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if !retryableRemoval(err) {
			return err
		}
		if attempt < sandboxRemovalAttempts-1 {
			time.Sleep(sandboxRetryDelay)
		}
	}
	return os.RemoveAll(path)
}

func retryableRemoval(err error) bool {
	return errors.Is(err, fs.ErrPermission) || isBusy(err)
}
