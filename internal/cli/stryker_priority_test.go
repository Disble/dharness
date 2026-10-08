package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/runner"
)

// A mutation run starts at normal priority and is lowered once Stryker
// records the end of its dry run, in Stryker's default events directory under
// the directory it runs in. The dry run is one test runner on about one core,
// and at low priority on a loaded machine vitest's 5s per-test timeout failed
// it: docs/research/prioridad-en-dos-fases.md.
//
// A stale onDryRunCompleted event from an earlier run sits there before this
// one starts. dharness removes it, because the switch is polled from launch
// and Stryker only clears the folder once its event-recorder starts: left in
// place, the stale file would lower the dry run this feature protects.
func TestMutateLowersPriorityOnlyAfterItsOwnDryRun(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)
	events := filepath.Join(root, "reports", "mutation", "events")
	writeFile(t, filepath.Join(events, "00000-onDryRunCompleted.json"), "{}")

	var probed, atLaunch, afterEvent bool
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, stdout, stderr io.Writer) error {
		if cmd.Label == "stryker" && cmd.LowerPriorityWhen != nil {
			probed = true
			atLaunch = cmd.LowerPriorityWhen()
			writeFile(t, filepath.Join(events, "00000-onDryRunCompleted.json"), "{}")
			afterEvent = cmd.LowerPriorityWhen()
		}
		return captured.run(cmd, stdout, stderr)
	}))

	// The stub writes no mutation report, so the run ends in an error after
	// Stryker was invoked; the command is what this test is about.
	_ = RunMutate([]string{"src/a.ts"}, io.Discard)

	stryker := commandFor(t, captured, "stryker")
	if stryker.LowPriority {
		t.Error("mutation started at low priority, want normal until the dry run completes: a low-priority dry run fails on a loaded machine")
	}
	if !probed {
		t.Fatal("mutation has no LowerPriorityWhen, want the switch that lowers the mutant phase: it is the one phase that can freeze the machine")
	}
	if atLaunch {
		t.Error("LowerPriorityWhen() = true at launch, want false: a stale onDryRunCompleted event from an earlier run was left in place")
	}
	if !afterEvent {
		t.Errorf("LowerPriorityWhen() = false after Stryker recorded onDryRunCompleted in %s, want true", events)
	}
}

// --dry-run is a dry run from start to end: the phase low priority breaks, and
// no mutant phase follows it to lower.
func TestMutateDryRunRunsAtNormalPriorityThroughout(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)
	captured.emit = "16:52:00 INFO DryRunExecutor Initial test run succeeded. Ran 3 tests in 0 seconds.\n"

	if err := RunMutate([]string{"src/a.ts", "--dry-run"}, io.Discard); err != nil {
		t.Fatalf("RunMutate() = %v", err)
	}

	stryker := commandFor(t, captured, "stryker")
	if stryker.LowPriority || stryker.LowerPriorityWhen != nil {
		t.Errorf("dry run = LowPriority %t, LowerPriorityWhen set %t, want neither: the whole run is the phase low priority breaks", stryker.LowPriority, stryker.LowerPriorityWhen != nil)
	}
	if strings.Contains(strings.Join(stryker.Args, " "), "event-recorder") {
		t.Errorf("dry run args = %v, want no event-recorder: there is no switch to signal", stryker.Args)
	}
}

// --staged moves the events into a directory the run owns, beside its own
// report, through the config it already generates (the base dir has no
// command-line flag), and watches exactly that directory.
func TestMutateStagedWatchesItsOwnEventsDirectory(t *testing.T) {
	root, captured := newStagedRepo(t)
	stageFile(t, root, "src/a.js", "export const a = 1;\n")
	run := stagedRunner(captured, nil, minimalStagedReport("src/a.js"))

	var baseDir string
	var probed, before, after bool
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, stdout, stderr io.Writer) error {
		if cmd.Label == "stryker" && argsContain(cmd.Args, stagedStrykerConfig) {
			raw, err := os.ReadFile(filepath.Join(cmd.Dir, stagedStrykerConfig))
			if err != nil {
				return err
			}
			var config struct {
				EventReporter struct {
					BaseDir string `json:"baseDir"`
				} `json:"eventReporter"`
			}
			if err := json.Unmarshal(raw, &config); err != nil {
				return err
			}
			baseDir = config.EventReporter.BaseDir
			if cmd.LowerPriorityWhen != nil && baseDir != "" {
				probed = true
				before = cmd.LowerPriorityWhen()
				writeFile(t, filepath.Join(baseDir, "00000-onDryRunCompleted.json"), "{}")
				after = cmd.LowerPriorityWhen()
			}
		}
		return run(cmd, stdout, stderr)
	}))

	if err := RunMutate([]string{"--staged"}, io.Discard); err != nil {
		t.Fatalf("RunMutate() = %v, want nil", err)
	}

	stryker := commandFor(t, captured, "stryker")
	if stryker.LowPriority {
		t.Error("staged mutation started at low priority, want normal until the dry run completes")
	}
	if !filepath.IsAbs(baseDir) || !strings.HasPrefix(filepath.Base(filepath.Dir(baseDir)), "dh-report-") {
		t.Fatalf("generated eventReporter.baseDir = %q, want an absolute directory inside the run's own dh-report- directory", baseDir)
	}
	if !probed {
		t.Fatal("staged mutation has no LowerPriorityWhen, want the switch that lowers the mutant phase")
	}
	if before || !after {
		t.Errorf("LowerPriorityWhen() = %t before and %t after onDryRunCompleted was recorded in %s, want false then true: the switch must watch the directory the config names", before, after, baseDir)
	}
	if _, err := os.Stat(baseDir); !os.IsNotExist(err) {
		t.Errorf("events directory %s survived the run (stat err %v), want it removed with the run's report directory", baseDir, err)
	}
}

// The project's own eventReporter keys travel into the generated config byte
// for byte; only baseDir is the run's, the same discipline jsonReporter gets.
func TestMutateStagedOverridesEventReporterBaseDirButKeepsOtherKeys(t *testing.T) {
	source := t.TempDir()
	writeFile(t, filepath.Join(source, "stryker.config.json"),
		`{"testRunner":"vitest","eventReporter":{"baseDir":"project/events","otherOption":true}}`)
	reportDir := filepath.Join(t.TempDir(), "dh-report-x")
	eventsDir := filepath.Join(reportDir, "events")

	configFile, err := writeStagedStrykerConfig(source, "stryker.config.json", []string{"src/a.js:1-1"}, filepath.Join(reportDir, "mutation.json"), eventsDir)
	if err != nil {
		t.Fatalf("writeStagedStrykerConfig() = %v, want nil", err)
	}

	raw, err := os.ReadFile(filepath.Join(source, configFile))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		EventReporter struct {
			BaseDir     string `json:"baseDir"`
			OtherOption bool   `json:"otherOption"`
		} `json:"eventReporter"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("generated config %s is not a JSON object: %v", raw, err)
	}
	if config.EventReporter.BaseDir != eventsDir {
		t.Errorf("generated eventReporter.baseDir = %q, want %q: the run's own events directory, overriding the project's", config.EventReporter.BaseDir, eventsDir)
	}
	if !config.EventReporter.OtherOption {
		t.Error("generated eventReporter.otherOption = false, want true: preserved from the project's config")
	}
}
