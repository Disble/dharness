package app

import (
	"bytes"
	"errors"
	"runtime/debug"
	"strings"
	"testing"

	"dharness/internal/runner"
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
