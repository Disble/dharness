package app

import (
	"bytes"
	"errors"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/cli"
	"github.com/Disble/dharness/internal/runner"
)

func TestRunArgsWithoutArgumentsPrintsHelp(t *testing.T) {
	var out bytes.Buffer

	if err := RunArgs(nil, &out); err != nil {
		t.Fatalf("RunArgs() = %v, want nil", err)
	}
	if !strings.Contains(out.String(), "USAGE") {
		t.Errorf("help not printed, got:\n%s", out.String())
	}
}

func TestRunArgsUnknownCommandNamesTheRealOnes(t *testing.T) {
	var out bytes.Buffer

	err := RunArgs([]string{"chekc"}, &out)

	var unknown *UnknownCommandError
	if !errors.As(err, &unknown) {
		t.Fatalf("RunArgs() = %v, want UnknownCommandError", err)
	}
	for _, command := range []string{"sync", "check", "mutate"} {
		if !strings.Contains(err.Error(), command) {
			t.Errorf("error does not mention %q: %s", command, err)
		}
	}
	// init merged into sync (Decision 1): a user typing the old command must
	// not be pointed back at it.
	if strings.Contains(err.Error(), "init") {
		t.Errorf("error still names the removed init command: %s", err)
	}
}

// dharness init used to dispatch into a RunInit-shaped function; it is now an
// unknown command like any other typo.
func TestRunArgsInitIsUnknown(t *testing.T) {
	var out bytes.Buffer

	err := RunArgs([]string{"init"}, &out)

	var unknown *UnknownCommandError
	if !errors.As(err, &unknown) {
		t.Fatalf("RunArgs() = %v, want UnknownCommandError for the removed init command", err)
	}
}

func TestRunArgsVersionPrintsResolvedVersion(t *testing.T) {
	previous := Version
	Version = "9.9.9"
	t.Cleanup(func() { Version = previous })

	var out bytes.Buffer
	if err := RunArgs([]string{"version"}, &out); err != nil {
		t.Fatalf("RunArgs() = %v, want nil", err)
	}
	if got := strings.TrimSpace(out.String()); got != "dharness 9.9.9" {
		t.Errorf("version output = %q", got)
	}
}

// TestRunArgsPropagatesVersionToCLI pins the other half of gap 7/12:
// internal/cli cannot import internal/app (app already imports cli), so the
// only way RunSync's report can ever carry a real version is if RunArgs
// forwards it through cli.Version before dispatch — for every command, not
// only "sync", since the forwarding has to happen before RunArgs knows
// which command was asked for.
func TestRunArgsPropagatesVersionToCLI(t *testing.T) {
	previousApp, previousCLI := Version, cli.Version
	Version = "9.9.9"
	t.Cleanup(func() { Version = previousApp; cli.Version = previousCLI })

	if err := RunArgs([]string{"help"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("RunArgs() = %v", err)
	}
	if cli.Version != "9.9.9" {
		t.Errorf("cli.Version = %q, want %q", cli.Version, "9.9.9")
	}
}

// A wrapped tool's exit code has to survive the trip to the process status,
// otherwise a failing check can still produce a successful commit.
func TestExitCodePropagatesTheToolStatus(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"no error", nil, 0},
		{"tool failed", &runner.ExitError{Command: "fallow", Code: 3}, 3},
		{"tool never started", &runner.StartError{Command: "fallow"}, 1},
		{"plain error", errors.New("broken"), 1},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := ExitCode(testCase.err); got != testCase.want {
				t.Errorf("ExitCode() = %d, want %d", got, testCase.want)
			}
		})
	}
}

func TestResolveVersionPrefersLdflagsThenBuildInfo(t *testing.T) {
	cases := []struct {
		name    string
		ldflags string
		info    *debug.BuildInfo
		ok      bool
		want    string
	}{
		{"ldflags wins", "1.2.3", nil, false, "1.2.3"},
		{"build info tag", "dev", &debug.BuildInfo{Main: debug.Module{Version: "v4.5.6"}}, true, "4.5.6"},
		{"untagged build", "dev", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, true, "dev"},
		{"no build info", "dev", nil, false, "dev"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			previous := buildInfoReader
			buildInfoReader = func() (*debug.BuildInfo, bool) { return testCase.info, testCase.ok }
			t.Cleanup(func() { buildInfoReader = previous })

			if got := ResolveVersion(testCase.ldflags); got != testCase.want {
				t.Errorf("ResolveVersion(%q) = %q, want %q", testCase.ldflags, got, testCase.want)
			}
		})
	}
}

// TestRunArgsPointsTheSetupVerbsAtSync is F7: `init` is what a person reaches
// for on a project that has never seen the tool, and dharness merged that
// verb into sync (Decision 1). The help text says so one line down a list of
// four commands; the error a mistyped command produces is the one moment
// they are certainly reading.
//
// The list of real commands stays — this adds a sentence, it does not replace
// the answer to "what does exist".
func TestRunArgsPointsTheSetupVerbsAtSync(t *testing.T) {
	for _, command := range []string{"init", "setup", "bootstrap", "install"} {
		t.Run(command, func(t *testing.T) {
			var out bytes.Buffer

			err := RunArgs([]string{command}, &out)

			var unknown *UnknownCommandError
			if !errors.As(err, &unknown) {
				t.Fatalf("RunArgs() = %v, want UnknownCommandError", err)
			}
			if !strings.Contains(err.Error(), "dharness sync") {
				t.Errorf("error does not point at sync: %s", err)
			}
			for _, real := range []string{"sync", "check", "mutate"} {
				if !strings.Contains(err.Error(), real) {
					t.Errorf("error stopped naming %q: %s", real, err)
				}
			}
		})
	}
}

// TestRunArgsSuggestsNothingForAnUnrelatedTypo keeps the suggestion from
// becoming noise every unknown command carries: a typo of an existing
// command gets the list and nothing else.
func TestRunArgsSuggestsNothingForAnUnrelatedTypo(t *testing.T) {
	var out bytes.Buffer

	err := RunArgs([]string{"chekc"}, &out)

	if strings.Contains(err.Error(), "dharness sync") {
		t.Errorf("an unrelated typo was given the setup suggestion: %s", err)
	}
}
