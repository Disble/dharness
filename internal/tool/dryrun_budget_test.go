package tool

import (
	"slices"
	"testing"
)

// The initial test run is where Stryker starts its runner processes, so a dry
// run without the budget is the phase the budget was written for running
// unbudgeted. Measured on a 20-thread machine: "Creating 19 test runner
// process(es)", and the dry run outlasted the mutation it was measuring.
func TestDryRunCarriesTheSameResourceBudgetAsMutation(t *testing.T) {
	args := StrykerDryRun([]string{"src/a.ts"}, "vitest", 2)

	index := slices.Index(args, "--concurrency")
	if index < 0 {
		t.Fatalf("the dry run is unbudgeted, so it starts one runner per core: %v", args)
	}
	if index+1 >= len(args) || args[index+1] != "2" {
		t.Errorf("--concurrency did not carry the given budget: %v", args)
	}
	if !slices.Contains(args, "--dryRunOnly") {
		t.Errorf("the dry run stopped being a dry run: %v", args)
	}
}
