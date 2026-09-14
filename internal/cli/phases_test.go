package cli

// Slice A (mutate-staged-v1.9): the fixed five-phase record. RED first —
// these tests fail until internal/cli/phases.go exists.

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Disble/dharness/internal/runner"
)

func TestPhaseRecordRendersAllNotStarted(t *testing.T) {
	rec := newPhaseRecord()
	want := "phases: snapshot not started · classify not started · discover not started · related not started · stryker not started"
	if got := rec.render(); got != want {
		t.Errorf("render() = %q, want %q", got, want)
	}
}

func TestPhaseRecordRendersDeterministicDurations(t *testing.T) {
	now := time.Now()
	advance := func(d time.Duration) { now = now.Add(d) }
	t.Cleanup(SetPhaseNowForTest(func() time.Time { return now }))

	rec := newPhaseRecord()
	rec.start(phaseSnapshot)
	advance(250 * time.Millisecond)
	rec.complete(phaseSnapshot)
	rec.start(phaseClassify)
	advance(1500 * time.Millisecond)
	rec.complete(phaseClassify)

	want := "phases: snapshot 0.25s · classify 1.5s · discover not started · related not started · stryker not started"
	if got := rec.render(); got != want {
		t.Errorf("render() = %q, want %q", got, want)
	}
}

func TestPhaseRecordFailureFreezesLaterPhases(t *testing.T) {
	now := time.Now()
	t.Cleanup(SetPhaseNowForTest(func() time.Time { return now }))

	rec := newPhaseRecord()
	rec.start(phaseSnapshot)
	now = now.Add(100 * time.Millisecond)
	rec.complete(phaseSnapshot)
	rec.start(phaseClassify)
	rec.fail(phaseClassify, errors.New("tsc exited 1"))

	want := "phases: snapshot 0.1s · classify failed: tsc exited 1 · discover not started · related not started · stryker not started"
	if got := rec.render(); got != want {
		t.Errorf("render() = %q, want %q", got, want)
	}
}

func TestPhaseRecordCollapsesMultilineReasonToOneLine(t *testing.T) {
	rec := newPhaseRecord()
	rec.start(phaseSnapshot)
	rec.fail(phaseSnapshot, errors.New("git failed\nline two\r\nline three"))

	got := rec.render()
	if strings.Contains(got, "\n") {
		t.Errorf("render() = %q, want a single line", got)
	}
	if !strings.Contains(got, "snapshot failed: git failed line two line three") {
		t.Errorf("render() = %q, want the reason flattened with spaces", got)
	}
}

func TestPhaseRecordRendersExactlyOnce(t *testing.T) {
	rec := newPhaseRecord()
	var out strings.Builder
	rec.renderOnce(&out)
	rec.start(phaseSnapshot)
	rec.renderOnce(&out)

	lines := strings.Count(out.String(), "phases:")
	if lines != 1 {
		t.Errorf("renderOnce printed %d records, want exactly 1: %q", lines, out.String())
	}
}

func TestPhaseRecordFirstTransitionWins(t *testing.T) {
	rec := newPhaseRecord()
	rec.start(phaseSnapshot)
	rec.complete(phaseSnapshot)
	rec.fail(phaseSnapshot, errors.New("late failure"))
	rec.start(phaseSnapshot)

	got := rec.render()
	if !strings.HasPrefix(got, "phases: snapshot ") || strings.Contains(got, "snapshot not started") || strings.Contains(got, "failed") {
		t.Errorf("render() = %q, want the first completion to stand", got)
	}
}

func TestRunMutateStagedRefusalPrintsAllNotStartedRecordOnce(t *testing.T) {
	var out strings.Builder
	if err := RunMutate([]string{"--staged", "src/a.js"}, &out); err == nil {
		t.Fatal("RunMutate() = nil, want the positional-path refusal")
	}
	want := "phases: snapshot not started · classify not started · discover not started · related not started · stryker not started"
	if !strings.Contains(out.String(), want) {
		t.Errorf("output = %q, want it to contain %q", out.String(), want)
	}
	if n := strings.Count(out.String(), "phases:"); n != 1 {
		t.Errorf("output prints %d phase records, want exactly 1: %q", n, out.String())
	}
}

func TestMutateStagedNothingStagedPrintsAllNotStartedRecord(t *testing.T) {
	_, captured := newStagedRepo(t)
	t.Cleanup(runner.SetForTest(captured.run))

	var out strings.Builder
	if err := RunMutate([]string{"--staged"}, &out); err != nil {
		t.Fatalf("RunMutate() = %v, want nil", err)
	}
	want := "phases: snapshot not started · classify not started · discover not started · related not started · stryker not started"
	if !strings.Contains(out.String(), want) {
		t.Errorf("output = %q, want it to contain %q", out.String(), want)
	}
}

func TestMutateStagedTypesOnlyCompletesSnapshotAndClassify(t *testing.T) {
	root, captured := newStagedRepo(t)
	writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName("tsc")), "")
	stageFile(t, root, "src/types.ts", "export type A = string;\n")
	t.Cleanup(runner.SetForTest(stagedRunner(captured, map[string]string{"src/types.ts": ""}, "")))

	var out strings.Builder
	if err := RunMutate([]string{"--staged"}, &out); err != nil {
		t.Fatalf("RunMutate() = %v, want nil", err)
	}
	got := out.String()
	if n := strings.Count(got, "phases:"); n != 1 {
		t.Fatalf("output prints %d phase records, want exactly 1: %q", n, got)
	}
	for _, label := range []string{"snapshot ", "classify "} {
		if !strings.Contains(got, "phases: "+label) && !strings.Contains(got, "· "+label) {
			t.Errorf("output = %q, want %s completed with a duration", got, label)
		}
	}
	for _, want := range []string{"discover not started", "related not started", "stryker not started"} {
		if !strings.Contains(got, want) {
			t.Errorf("output = %q, want it to contain %q", got, want)
		}
	}
	assertPhaseLabelOrder(t, got)
}

func assertPhaseLabelOrder(t *testing.T, out string) {
	t.Helper()
	labels := []string{"snapshot", "classify", "discover", "related", "stryker"}
	last := -1
	for _, label := range labels {
		idx := strings.Index(out, "phases: "+label)
		if idx < 0 {
			idx = strings.Index(out, "· "+label)
		}
		if idx < 0 {
			t.Fatalf("output = %q, want label %q once", out, label)
		}
		if idx <= last {
			t.Fatalf("output = %q, want labels in fixed order", out)
		}
		last = idx
	}
}
