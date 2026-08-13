package main

import (
	"fmt"
	"strings"
)

// verifyBaseline runs the test command once, unmutated, against the sandbox
// ooze is about to mutate.
//
// A mutant is scored dead when the test command fails. A suite that already
// fails therefore kills every mutant without any mutation being involved, and
// the run reports a perfect score and exits 0 — measured at 4 killed, 0
// survived, 1.00. That is the failure this repository names in its own first
// rules: a verdict that has to be read out of context rather than off an exit
// code. A red baseline is not a low score, it is no measurement.
//
// It is also the cheapest refusal available. Releasing ooze against a red
// suite wastes every mutant execution in the run; one execution buys the
// certainty that the rest are worth paying for.
func (tool *tool) verifyBaseline(sandbox, testCommand string) error {
	// ooze splits the configured command on single spaces and runs it with the
	// repository root as its working directory (WithTestCommand, options.go).
	// Mirroring both exactly is the whole point: a baseline that runs a
	// different command, or reads different bytes, proves nothing about the
	// run it is clearing.
	parts := strings.Split(testCommand, " ")
	var output strings.Builder
	err := tool.runner.Run(commandSpec{
		Dir: sandbox, Name: parts[0], Args: parts[1:],
		Env:    environmentWithoutGitContext(),
		Stdout: &output, Stderr: &output,
	})
	if err == nil {
		return nil
	}
	return fmt.Errorf(
		"go mutation: %q fails on unmutated code; refusing to score, because a failing suite kills every mutant and reports 1.00: %w\n%s",
		testCommand, err, strings.TrimSpace(output.String()))
}
