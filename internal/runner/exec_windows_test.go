//go:build windows

package runner

import (
	"strings"
	"testing"
)

// Every tool dharness wraps arrives as a .cmd shim in node_modules/.bin, and
// CreateProcess cannot execute one. Without this routing the locally installed
// copy never runs and every invocation silently falls back to the remote form.
func TestPlatformizeRoutesShimsThroughCmd(t *testing.T) {
	cases := []struct {
		name     string
		command  string
		wantName string
		wantHead []string
	}{
		{"npm shim", `C:\repo\node_modules\.bin\fallow.cmd`, "cmd", []string{"/c", `C:\repo\node_modules\.bin\fallow.cmd`}},
		{"batch file", `C:\tools\thing.BAT`, "cmd", []string{"/c", `C:\tools\thing.BAT`}},
		{"plain executable", "npx", "npx", nil},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			name, args := platformize(testCase.command, []string{"audit"})

			if name != testCase.wantName {
				t.Errorf("name = %q, want %q", name, testCase.wantName)
			}
			for i, want := range testCase.wantHead {
				if args[i] != want {
					t.Errorf("args[%d] = %q, want %q", i, args[i], want)
				}
			}
			if last := args[len(args)-1]; last != "audit" {
				t.Errorf("tool arguments were dropped, tail = %q", last)
			}
			if testCase.wantHead == nil && strings.Contains(strings.Join(args, " "), "/c") {
				t.Errorf("plain executable was routed through cmd: %v", args)
			}
		})
	}
}
