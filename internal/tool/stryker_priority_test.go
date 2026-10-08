package tool

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The mutation run is the one that asks for the event-recorder: its
// onDryRunCompleted event is the signal that lowers the run's priority once
// the dry run is over (see StrykerDryRunCompleted).
func TestStrykerMutateRecordsEventsForThePrioritySwitch(t *testing.T) {
	args := StrykerMutate([]string{"src/a.ts"}, "vitest", "", "sandbox", 2)

	index := slices.Index(args, "--reporters")
	if index < 0 || index+1 >= len(args) || args[index+1] != "clear-text,json,event-recorder" {
		t.Errorf("StrykerMutate() = %v, want --reporters clear-text,json,event-recorder: without the recorder the dry run's end is never signalled and the mutants run at normal priority", args)
	}
}

// --dryRunOnly is a dry run from start to end, so it runs at normal priority
// throughout and has no switch to signal; recording events would only write
// files nobody reads.
func TestStrykerDryRunRecordsNoEvents(t *testing.T) {
	args := StrykerDryRun([]string{"src/a.ts"}, "vitest", 2)

	index := slices.Index(args, "--reporters")
	if index < 0 || index+1 >= len(args) || args[index+1] != "clear-text,json" {
		t.Errorf("StrykerDryRun() = %v, want --reporters clear-text,json and no event-recorder", args)
	}
}

func TestStrykerDryRunCompletedFiresOnlyOnTheDryRunEvent(t *testing.T) {
	events := filepath.Join(t.TempDir(), "events")
	completed := StrykerDryRunCompleted(events)

	if completed() {
		t.Error("StrykerDryRunCompleted() = true for a directory that does not exist yet, want false: Stryker has not started recording")
	}

	if err := os.MkdirAll(events, 0o755); err != nil {
		t.Fatal(err)
	}
	if completed() {
		t.Error("StrykerDryRunCompleted() = true for an empty events directory, want false")
	}

	writeEvent(t, events, "00001-onMutantTested.json")
	writeEvent(t, events, "00000-onAllMutantsMatchedWithTests.json")
	if completed() {
		t.Error("StrykerDryRunCompleted() = true with only other events recorded, want false: only onDryRunCompleted ends the dry run")
	}

	writeEvent(t, events, "00000-onDryRunCompleted.json")
	if !completed() {
		t.Error("StrykerDryRunCompleted() = false after 00000-onDryRunCompleted.json was written, want true")
	}
}

// Stryker's own default, relative to the directory it runs in, is where the
// interactive route watches for the event.
func TestStrykerDefaultEventsDirIsStrykersOwnDefault(t *testing.T) {
	dir := filepath.Join("C:", "proj")
	if got, want := StrykerDefaultEventsDir(dir), filepath.Join(dir, "reports", "mutation", "events"); got != want {
		t.Errorf("StrykerDefaultEventsDir() = %q, want %q", got, want)
	}
}

func writeEvent(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
}
