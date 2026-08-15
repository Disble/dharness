//go:build windows

package runner

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cmdShimTemplate is the shape cmd-shim writes into node_modules/.bin, copied
// from a yarn 4.9.1 install of eslint 9 and reduced to the part that matters.
//
// Which shim a project has decides whether it meets this bug at all, so the
// fixture cannot be invented. Measured, both shapes are in the wild:
//
//	cmd-shim   yarn and older npm     %* expands inside an IF/ELSE block
//	npm 10+    modern npm             %* is on the last line, outside it
//
// Only the first breaks, because an unquoted ")" in an argument closes the
// block early. A fixture built on the npm 10 shape passes while a yarn user's
// gate dies, which is exactly how far this defect travelled before anyone ran
// the binary against a project that had one.
const cmdShimTemplate = `@IF EXIST "%%~dp0\nothere.exe" (
  ECHO unreachable
) ELSE (
  "%s" %%*
)
`

// writeCmdShim installs a real shim that re-execs this test binary as the
// wrapped tool, and returns its path.
func writeCmdShim(t *testing.T) string {
	t.Helper()

	shim := filepath.Join(t.TempDir(), "tool.cmd")
	body := fmt.Sprintf(cmdShimTemplate, os.Args[0])
	// CRLF, because that is what a batch file on this platform really has.
	if err := os.WriteFile(shim, []byte(strings.ReplaceAll(body, "\n", "\r\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return shim
}

// A route group — (landing) in the Next.js App Router — is an ordinary path
// that dharness has to be able to hand a linter.
//
// Measured before the fix against a yarn 4.9.1 install of eslint 9: the
// argument reached the shim unquoted, cmd.exe read the ")" as the end of the
// shim's own IF block, and the run died with "/page.tsx was unexpected at this
// time" and exit 255 — reported as eslint's verdict, though eslint never ran.
func TestRunDeliversARouteGroupPathIntact(t *testing.T) {
	t.Setenv(argvHelperEnv, "1")
	shim := writeCmdShim(t)
	path := `src/app/[locale]/(landing)/page.tsx`

	var out strings.Builder
	err := Run(Command{Name: shim, Args: []string{path}}, &out, &out)

	if err != nil {
		t.Fatalf("Run() = %v, want nil; the tool printed: %s", err, out.String())
	}
	if got, want := strings.TrimSpace(out.String()), "<"+path+">"; got != want {
		t.Errorf("the tool received %s, want %s", got, want)
	}
}

// cmd.exe reparses the line the shim was started with, so a path is not safe
// merely because CreateProcess accepted it. Each character below is legal in a
// Windows path and in a git tree, and each failed differently before the fix —
// measured through a real cmd-shim on go1.26.5:
//
//	&   split the line and ran the tail as a command
//	^   was deleted, handing the tool a path that does not exist
//	( ) closed the shim's own IF block
//
// The ampersand is the one that matters most: a file named a&whoami&b.tsx in a
// cloned repository turned a lint run into command execution.
func TestRunDeliversMetacharacterPathsIntact(t *testing.T) {
	paths := []string{
		`src/app/(landing)/page.tsx`,
		`src/a&b/page.tsx`,
		`src/a^b/page.tsx`,
		`src/my app/page.tsx`,
		`src/a!b/page.tsx`,
		`src/a,b/page.tsx`,
		`src/a;b/page.tsx`,
		`src/a'b/page.tsx`,
		`src/a$b/page.tsx`,
		`src/a#b/page.tsx`,
		`src/a=b/page.tsx`,
		`src/plain/page.tsx`,
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			t.Setenv(argvHelperEnv, "1")
			shim := writeCmdShim(t)

			var out strings.Builder
			err := Run(Command{Name: shim, Args: []string{path}}, &out, &out)

			if err != nil {
				t.Fatalf("Run() = %v, want nil; the tool printed: %s", err, out.String())
			}
			if got, want := strings.TrimSpace(out.String()), "<"+path+">"; got != want {
				t.Errorf("the tool received %s, want %s", got, want)
			}
		})
	}
}

// One character cannot be delivered at all. cmd.exe expands %VAR% before the
// shim runs, and the second parse defeats every escape — measured: %PROBEVAR%
// arrived as its value, %%PROBEVAR%% as the value still wrapped in per cent
// signs, ^%PROBEVAR^% with the carets intact.
//
// So the choice is to refuse or to lie. Quietly handing over an altered path
// makes the linter report on a file that does not exist, or on a different
// one, and the gate would call either result a verdict. Refusing is the only
// answer that keeps the exit code honest.
func TestRunRefusesAnArgumentItCannotDeliver(t *testing.T) {
	t.Setenv(argvHelperEnv, "1")
	shim := writeCmdShim(t)
	path := `src/a%PATH%b/page.tsx`

	var out strings.Builder
	err := Run(Command{Name: shim, Args: []string{path}}, &out, &out)

	if !errors.Is(err, ErrUndeliverableArgument) {
		t.Fatalf("Run() = %v, want it to refuse the path", err)
	}
	if out.String() != "" {
		t.Errorf("the tool ran anyway and received: %s", out.String())
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("the refusal does not name the path at fault: %v", err)
	}
}

// The shim's own path travels the same line as its arguments, so a project
// installed under a directory holding a per cent sign is undeliverable for the
// same reason — and would otherwise fail as "the tool is missing", sending
// whoever reads it to reinstall a dependency that is already there.
func TestRunRefusesAShimItCannotName(t *testing.T) {
	shim := filepath.Join(t.TempDir(), "to%ol.cmd")
	if err := os.WriteFile(shim, []byte("@ECHO off\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Run(Command{Name: shim, Args: []string{"audit"}}, io.Discard, io.Discard)

	if !errors.Is(err, ErrUndeliverableArgument) {
		t.Fatalf("Run() = %v, want it to refuse the shim path", err)
	}
}

// A per cent sign is only undeliverable through a shim. Refusing it for an
// ordinary executable would ground the gate over a path CreateProcess hands
// over untouched.
func TestRunAcceptsAPerCentSignForAPlainExecutable(t *testing.T) {
	t.Setenv(argvHelperEnv, "1")
	path := `src/a%PATH%b/page.tsx`

	var out strings.Builder
	err := Run(Command{Name: os.Args[0], Args: []string{path}}, &out, &out)

	if err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	if got, want := strings.TrimSpace(out.String()), "<"+path+">"; got != want {
		t.Errorf("the tool received %s, want %s", got, want)
	}
}

// The staged set arrives as one list, so the separators between arguments have
// to survive as well as the arguments themselves. A line that delivers one
// path correctly can still fuse or split a list of them.
func TestRunDeliversAWholeArgumentListIntact(t *testing.T) {
	t.Setenv(argvHelperEnv, "1")
	shim := writeCmdShim(t)
	paths := []string{
		`src/app/(landing)/page.tsx`,
		`src/app/(marketing)/layout.tsx`,
		`src/my app/x.tsx`,
		`src/a&b/y.tsx`,
	}

	var out strings.Builder
	err := Run(Command{Name: shim, Args: paths}, &out, &out)

	if err != nil {
		t.Fatalf("Run() = %v, want nil; the tool printed: %s", err, out.String())
	}
	want := "<" + strings.Join(paths, ">\n<") + ">"
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("the tool received:\n%s\nwant:\n%s", got, want)
	}
}

// Every tool dharness wraps arrives as a .cmd shim in node_modules/.bin, and
// CreateProcess cannot execute one. Without this routing the locally installed
// copy never runs and every invocation silently falls back to the remote form.
func TestPlatformizeRoutesShimsThroughCmd(t *testing.T) {
	cases := []struct {
		name     string
		command  string
		wantName string
		wantShim bool
	}{
		{"npm shim", `C:\repo\node_modules\.bin\fallow.cmd`, "cmd", true},
		{"batch file", `C:\tools\thing.BAT`, "cmd", true},
		{"plain executable", "npx", "npx", false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			target := platformize(testCase.command, []string{"audit"})

			if target.Name != testCase.wantName {
				t.Errorf("Name = %q, want %q", target.Name, testCase.wantName)
			}
			if !testCase.wantShim {
				if target.CmdLine != "" {
					t.Errorf("plain executable was routed through cmd: %q", target.CmdLine)
				}
				if strings.Join(target.Args, " ") != "audit" {
					t.Errorf("Args = %v, want the tool arguments unchanged", target.Args)
				}
				return
			}
			// A shim carries its whole invocation in CmdLine, so Args must stay
			// empty: a list that is built and never run is a lie to whoever
			// reads it next.
			if len(target.Args) != 0 {
				t.Errorf("Args = %v, want none — CmdLine is what runs", target.Args)
			}
			if !strings.Contains(target.CmdLine, testCase.command) {
				t.Errorf("CmdLine = %q, want it to name the shim", target.CmdLine)
			}
			if !strings.Contains(target.CmdLine, "/c") {
				t.Errorf("CmdLine = %q, want it routed through cmd", target.CmdLine)
			}
			if !strings.HasSuffix(target.CmdLine, `"audit""`) {
				t.Errorf("CmdLine = %q, want the tool arguments quoted at the tail", target.CmdLine)
			}
		})
	}
}
