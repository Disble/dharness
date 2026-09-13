package staged

import (
	"bytes"
	"context"
	"fmt"

	"github.com/Disble/dharness/internal/runner"
)

// VitestSuiteError reports a vitest suite that fails to even load under
// `vitest list` — the same load failure Stryker's own vitest-runner dry run
// silently drops the file for, rather than reporting it.
type VitestSuiteError struct{ Cause error }

func (e *VitestSuiteError) Error() string {
	return fmt.Sprintf(
		"a test file failed to load, so Stryker would silently drop it from the run rather than fail on it: %v",
		e.Cause,
	)
}

func (e *VitestSuiteError) Unwrap() error { return e.Cause }

// GuardVitestSuite refuses a staged run whose vitest suite cannot even load.
//
// Measured against a real project: Stryker's own vitest-runner dry run
// silently drops a test file that fails to load rather than reporting the
// failure — 177 of 267 tests dropped, and the mutation score read 0.00 with
// no error at all. `vitest list` exits non-zero on the same load error, so
// this asks vitest's own CLI first and refuses before Stryker ever runs,
// rather than let Stryker's own dry run silently narrow what it tests.
//
// It only ever runs for vitest — jest's own runner already fails loudly on a
// file that cannot load, so there is nothing here for jest to guard against,
// and `list` is never invoked for one.
//
// vitestConfigFile is the project's own vitest.configFile from its JSON
// Stryker config, or "" when it never set one; when present it is passed to
// `list` the same way, so this asks about the same config Stryker's own
// vitest-runner will load rather than whatever vitest would resolve on its
// own. snapshotSource is where `list` runs from — the snapshot's own copy of
// source, the same tree Stryker itself is about to run in.
//
// ctx reaches `vitest list`, which runs for about 70 seconds on a real
// project, so cancelling it kills the listing rather than waiting it out. A
// killed listing reads here as a suite that failed to load; telling an
// interrupt apart is the caller's, which holds ctx.
func GuardVitestSuite(ctx context.Context, vitestBinary, testRunner, snapshotSource, vitestConfigFile string) error {
	if testRunner != "vitest" {
		return nil
	}

	args := []string{"list"}
	if vitestConfigFile != "" {
		args = append(args, "--config", vitestConfigFile)
	}

	cmd := runner.Command{Label: "vitest", Name: vitestBinary, Args: args, Dir: snapshotSource, Context: ctx}
	var transcript bytes.Buffer
	if err := runner.Run(cmd, &transcript, &transcript); err != nil {
		return &VitestSuiteError{Cause: err}
	}
	return nil
}
