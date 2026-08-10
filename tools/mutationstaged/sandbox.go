package main

import (
	"fmt"
	"os"
)

func materializeIndex(runner processRunner, root string) (string, func(), error) {
	sandbox, err := os.MkdirTemp("", "dharness-mutation-")
	if err != nil {
		return "", nil, fmt.Errorf("create mutation sandbox: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(sandbox) }
	prefix := sandbox + string(os.PathSeparator)
	if _, err := runner.Output(root, "git", "checkout-index", "--all", "--prefix="+prefix); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("materialize staged index: %w", err)
	}
	return sandbox, cleanup, nil
}
