package tool

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The manager is the project's own, and every form here resolves what that
// project installed: no `bunx <package>`, no `pnpm dlx`, no bare `npx`. A gate
// that runs on every commit must not be the moment an install reaches the
// network, and none of these four does.
func TestVitestCoverageCommandSpeaksTheProjectsOwnManagerWithoutTheNetwork(t *testing.T) {
	cases := []struct {
		name    string
		manager string
		want    string
	}{
		{name: "bun", manager: "bun", want: "bunx --no-install"},
		{name: "pnpm", manager: "pnpm", want: "pnpm exec"},
		{name: "yarn", manager: "yarn", want: "yarn exec"},
		{name: "npm", manager: "npm", want: "npx --no-install"},
		{name: "no manager detected", manager: "", want: "npx --no-install"},
		{name: "an unknown manager", manager: "cargo", want: "npx --no-install"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := VitestCoverageCommand(ShellPOSIX, tc.manager, "", ".dharness/vitest.coverage.mjs")
			if err != nil {
				t.Fatalf("VitestCoverageCommand() = %v", err)
			}
			want := tc.want + " vitest run --coverage --config .dharness/vitest.coverage.mjs"
			if got != want {
				t.Errorf("VitestCoverageCommand() = %q, want %q", got, want)
			}
			for _, downloading := range []string{"pnpm dlx", " dlx ", "npx vitest", "bunx vitest"} {
				if strings.Contains(got, downloading) {
					t.Errorf("the command reaches the network through %q: %q", downloading, got)
				}
			}
		})
	}
}

// Vitest resolves its own configuration against the process's working
// directory, so a split layout has to change into the JS project first — and
// the entry dharness owns is named from there, not from the hook's own
// directory.
func TestVitestCoverageCommandChangesDirectoryOnlyWhenTheRootsDiffer(t *testing.T) {
	root, err := VitestCoverageCommand(ShellPOSIX, "npm", "", ".dharness/vitest.coverage.mjs")
	if err != nil {
		t.Fatalf("VitestCoverageCommand() = %v", err)
	}
	if strings.Contains(root, "cd ") {
		t.Errorf("a repository that is its own JS project changed directory: %q", root)
	}

	split, err := VitestCoverageCommand(ShellPOSIX, "npm", "frontend", "../.dharness/vitest.coverage.mjs")
	if err != nil {
		t.Fatalf("VitestCoverageCommand() = %v", err)
	}
	want := "cd frontend && npx --no-install vitest run --coverage --config ../.dharness/vitest.coverage.mjs"
	if split != want {
		t.Errorf("VitestCoverageCommand() = %q, want %q", split, want)
	}

	same, err := VitestCoverageCommand(ShellPOSIX, "npm", ".", ".dharness/vitest.coverage.mjs")
	if err != nil {
		t.Fatalf("VitestCoverageCommand() = %v", err)
	}
	if same != root {
		t.Errorf("a run directory of %q rendered %q, want the root form %q", ".", same, root)
	}
}

// The shell is chosen by the interpreter that will read the line, not by the
// machine it is written on: a husky hook is a POSIX shell script on every
// platform, so its line is spelled for sh even on Windows, while a lefthook job
// goes through cmd.exe there.
func TestVitestCoverageCommandQuotesForTheInterpreterThatReadsIt(t *testing.T) {
	posix, err := VitestCoverageCommand(ShellPOSIX, "npm", "my app", "../.dharness/vitest.coverage.mjs")
	if err != nil {
		t.Fatalf("VitestCoverageCommand() = %v", err)
	}
	wantPosix := "cd 'my app' && npx --no-install vitest run --coverage --config ../.dharness/vitest.coverage.mjs"
	if posix != wantPosix {
		t.Errorf("VitestCoverageCommand(ShellPOSIX) = %q, want %q", posix, wantPosix)
	}

	// Backslashes are not metacharacters to cmd.exe, so a Windows path is
	// handed over as it is written; only the value with a space gains quotes.
	cmd, err := VitestCoverageCommand(ShellCmd, "npm", "my app", `..\.dharness\vitest.coverage.mjs`)
	if err != nil {
		t.Fatalf("VitestCoverageCommand() = %v", err)
	}
	wantCmd := `cd "my app" && npx --no-install vitest run --coverage --config ..\.dharness\vitest.coverage.mjs`
	if cmd != wantCmd {
		t.Errorf("VitestCoverageCommand(ShellCmd) = %q, want %q", cmd, wantCmd)
	}
	if strings.Count(cmd, `"`) != 2 {
		t.Errorf("the and-operator was quoted along with the value: %q", cmd)
	}

	spaced, err := VitestCoverageCommand(ShellCmd, "npm", "my app", "my entry.mjs")
	if err != nil {
		t.Fatalf("VitestCoverageCommand() = %v", err)
	}
	if !strings.Contains(spaced, `"my entry.mjs"`) {
		t.Errorf("a cmd value with a space was not quoted: %q", spaced)
	}

	// cmd.exe expands %VAR% before the program runs and no quoting survives
	// that, so the line is refused rather than delivered to something other
	// than what was asked (§03, the same rule runner's shim command line
	// already records for a .cmd shim). A POSIX shell reads a per cent sign as
	// an ordinary character, so the same value is not refused there.
	if _, err := VitestCoverageCommand(ShellCmd, "npm", "100%app", "x.mjs"); err == nil {
		t.Error("VitestCoverageCommand(ShellCmd) = nil for a value cmd.exe would rewrite")
	}
	if _, err := VitestCoverageCommand(ShellPOSIX, "npm", "100%app", "x.mjs"); err != nil {
		t.Errorf("VitestCoverageCommand(ShellPOSIX) = %v for a POSIX value with a per cent sign", err)
	}

	// The machine answers exactly one question, and only for a lefthook job:
	// which interpreter will read that job's line. A husky hook never asks —
	// its line is spelled for sh everywhere — so an answer from this function
	// is a lefthook job, and on this host it is the interpreter the job is
	// actually read by. The consequence is the one an activated repository
	// gets, so the answer is checked by what that shell does with a value only
	// cmd.exe rewrites: refused there, carried here.
	hostShell := HostCoverageShell()
	_, hostErr := VitestCoverageCommand(hostShell, "npm", "100%app", "x.mjs")
	if runtime.GOOS == "windows" {
		if hostShell != ShellCmd {
			t.Errorf("HostCoverageShell() = %v on Windows, want ShellCmd: a lefthook job is read by cmd.exe there", hostShell)
		}
		if hostErr == nil {
			t.Error("the host's own line carried a value cmd.exe rewrites before the runner starts")
		}
	} else {
		if hostShell != ShellPOSIX {
			t.Errorf("HostCoverageShell() = %v off Windows, want ShellPOSIX: a lefthook job is read by sh there", hostShell)
		}
		if hostErr != nil {
			t.Errorf("the host's own line refused a POSIX value: %v", hostErr)
		}
	}
}

// TestVitestCoverageCommandQuotesEveryShellMetacharacter is the owning scenario
// for the POSIX rendering, driven through the interpreter that will actually
// read the line: the run directory and the entry name hold every character a
// shell would otherwise act on, and the line is executed with the runner
// deliberately unreachable. The same decision has a second half — the
// characters sh reads literally, and the empty value — and both halves are the
// line dharness writes into a consumer's own hook.
//
// An unquoted `$( … )`, backtick, pipe, ampersand or semicolon is not cosmetic:
// sh would run it before the runner is ever looked for, and a gate that starts
// something other than what it names is the failure §03 refuses. The command
// not being found is the expected end of this run; what the scenario refuses is
// a file one of those names would have created.
func TestVitestCoverageCommandQuotesEveryShellMetacharacter(t *testing.T) {
	// Every character the shell reads literally stays as it was written, at
	// both ends of the three ranges this package recognises. These are the
	// characters a consumer's own project root is spelled with, and the line
	// is what that consumer's hook is written with, so a boundary that drifted
	// would rewrite the line of every repository whose root carries the
	// character.
	bare := "apps/Zebra9A0z"
	wantBare := "cd " + bare + " && npx --no-install vitest run --coverage --config ../.dharness/vitest.coverage.mjs"
	bareLine, err := VitestCoverageCommand(ShellPOSIX, "npm", bare, "../.dharness/vitest.coverage.mjs")
	if err != nil {
		t.Fatalf("VitestCoverageCommand() = %v", err)
	}
	if bareLine != wantBare {
		t.Errorf("VitestCoverageCommand() = %q, want %q", bareLine, wantBare)
	}

	// A value with no characters at all is still one word: the shell is handed
	// an argument rather than a hole where one was, because dropping the empty
	// value moves every argument after it and the runner then reads a switch
	// list other than the one the caller named.
	emptyLine, err := VitestCoverageCommand(ShellPOSIX, "npm", "", "")
	if err != nil {
		t.Fatalf("VitestCoverageCommand() = %v", err)
	}
	if wantEmpty := "npx --no-install vitest run --coverage --config ''"; emptyLine != wantEmpty {
		t.Errorf("VitestCoverageCommand() = %q, want %q", emptyLine, wantEmpty)
	}

	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("sh is not on PATH, so the rendered line cannot be driven: %v", err)
	}

	root := t.TempDir()
	// The run directory is one a shell would act on and one both platforms can
	// name; the entry never reaches the filesystem, so it carries every
	// remaining metacharacter.
	hostile := "my $(touch pwned) `touch pwned2` ; & dir"
	entry := "dir with $(touch pwned3) ; & | < > * ? ' \" `x`/vitest.coverage.mjs"
	if err := os.MkdirAll(filepath.Join(root, hostile), 0o755); err != nil {
		t.Fatal(err)
	}

	line, err := VitestCoverageCommand(ShellPOSIX, "npm", hostile, entry)
	if err != nil {
		t.Fatalf("VitestCoverageCommand() = %v", err)
	}
	for _, quoted := range []string{shWord(hostile), shWord(entry)} {
		if !strings.Contains(line, quoted) {
			t.Errorf("the rendered line does not carry %s:\n%s", quoted, line)
		}
	}

	empty := filepath.Join(root, "empty-bin")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(strings.ToUpper(kv), "PATH=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "PATH="+empty)

	command := exec.Command(sh, "-c", line)
	command.Dir = root
	command.Env = env
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("the line ran although the runner is unreachable:\n%s", output)
	}
	if !strings.Contains(strings.ToLower(string(output)), "not found") {
		t.Errorf("the shell did not get as far as looking for the runner: %v\n%s", err, output)
	}
	for _, created := range []string{"pwned", "pwned2", "pwned3"} {
		if _, err := os.Stat(filepath.Join(root, created)); err == nil {
			t.Errorf("the name ran %q instead of being passed as one argument:\n%s", created, line)
		}
	}

	// The pipe is the one metacharacter every range in the safe set accepts,
	// so it is where an over-wide range shows and nowhere else: this value is
	// letters and a pipe, the shape that stays a single argument only while
	// the ranges are the ones this package measured. The shell names every
	// command it is asked to look up, so a line that lost the quoting names
	// the probe — the runner would be two commands around an operator instead
	// of one argument.
	const probe = "npx|dharness-pipe-probe"
	probeLine, err := VitestCoverageCommand(ShellPOSIX, "npm", "", probe)
	if err != nil {
		t.Fatalf("VitestCoverageCommand() = %v", err)
	}
	if !strings.Contains(probeLine, "'"+probe+"'") {
		t.Errorf("a value holding a pipe was not quoted as one argument: %q", probeLine)
	}
	probeRun := exec.Command(sh, "-c", probeLine)
	probeRun.Dir = root
	probeRun.Env = env
	probeOutput, probeErr := probeRun.CombinedOutput()
	if probeErr == nil {
		t.Fatalf("the line ran although the runner is unreachable:\n%s", probeOutput)
	}
	if strings.Contains(string(probeOutput), "dharness-pipe-probe") {
		t.Errorf("the pipe reached the shell as an operator and it looked up the probe:\n%s\n%s", probeLine, probeOutput)
	}
}

// A refusal is a job, not an omission: when an activated repository's coverage
// line cannot be rendered, the gate gets a line that fails by name, so the
// commit is refused with the reason instead of passing without coverage.
func TestCoverageRefusalFailsByNameInBothShells(t *testing.T) {
	const reason = "coverage-tiers.json is declared but this project has no JS project root to name the managed entry from; run dharness sync from the project"

	posix := CoverageRefusal(ShellPOSIX, reason)
	if !strings.Contains(posix, "exit 1") {
		t.Errorf("CoverageRefusal(ShellPOSIX) does not fail: %q", posix)
	}
	if !strings.Contains(posix, "coverage-tiers.json is declared") {
		t.Errorf("CoverageRefusal(ShellPOSIX) does not name the reason: %q", posix)
	}

	cmd := CoverageRefusal(ShellCmd, reason)
	if !strings.Contains(cmd, "exit 1") {
		t.Errorf("CoverageRefusal(ShellCmd) does not fail: %q", cmd)
	}
	if !strings.Contains(cmd, "coverage-tiers.json is declared") {
		t.Errorf("CoverageRefusal(ShellCmd) does not name the reason: %q", cmd)
	}

	// cmd.exe expands a per cent sign and a backtick-quoted command is a POSIX
	// construct the message must not carry, so both leave the message rather
	// than reaching a shell half-escaped. The rendered line's own quotes are
	// the quoting it needs, so the assertion is on the message it carries.
	const hostile = `100% of "this" is not deliverable`
	message := coverageRefusalReason(hostile)
	if strings.ContainsAny(message, "%\"`") {
		t.Errorf("the refusal message kept a character a shell would rewrite: %q", message)
	}
	if !strings.Contains(message, "100 of this is not deliverable") {
		t.Errorf("the refusal message was not flattened: %q", message)
	}
	if !strings.Contains(CoverageRefusal(ShellCmd, hostile), message) {
		t.Errorf("CoverageRefusal(ShellCmd) did not carry the flattened reason: %q", CoverageRefusal(ShellCmd, hostile))
	}

	// Which rendering the named shell gets is the decision this scenario exists
	// to refuse, and the shell's own quoting is what makes it visible: a POSIX
	// hook line is written for sh, so the reason is single-quoted as one
	// literal word, while the cmd.exe line carries the double-quoted form.
	const plain = "coverage-tiers.json is declared but there is no JS project root"
	wantPosixRefusal := "echo 'coverage tiers: " + plain + "' 1>&2 && exit 1"
	if posixRefusal := CoverageRefusal(ShellPOSIX, plain); posixRefusal != wantPosixRefusal {
		t.Errorf("CoverageRefusal(ShellPOSIX) = %q, want %q", posixRefusal, wantPosixRefusal)
	}
	wantCmdRefusal := `echo "coverage tiers: ` + plain + `" 1>&2 && exit 1`
	if cmdRefusal := CoverageRefusal(ShellCmd, plain); cmdRefusal != wantCmdRefusal {
		t.Errorf("CoverageRefusal(ShellCmd) = %q, want %q", cmdRefusal, wantCmdRefusal)
	}

	// The consequence, run through the interpreter the line names: the reason
	// reaches the reader as it was written, including the characters the
	// flattening keeps because cmd.exe never rewrites them. Handed cmd.exe's
	// double quotes on a POSIX hook instead, `$( … )` stays live, and the line
	// whose whole job is to refuse the commit writes a file and drops that
	// text from the reason it prints.
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("sh is not on PATH, so the refusal line cannot be driven: %v", err)
	}
	root := t.TempDir()
	refusal := CoverageRefusal(ShellPOSIX, "the shipped root $(>pwned) is not deliverable")
	refusalRun := exec.Command(sh, "-c", refusal)
	refusalRun.Dir = root
	refusalOutput, refusalErr := refusalRun.CombinedOutput()
	if refusalErr == nil {
		t.Errorf("the refusal line did not fail the commit:\n%s", refusalOutput)
	}
	if !strings.Contains(string(refusalOutput), "$(>pwned)") {
		t.Errorf("the refusal did not deliver its reason as written:\n%s\n%s", refusal, refusalOutput)
	}
	if _, err := os.Stat(filepath.Join(root, "pwned")); err == nil {
		t.Errorf("the refusal ran part of its own reason instead of printing it:\n%s", refusal)
	}
}

func TestVitestCoverageArgsAreTheRunnersOwnCoverageSwitch(t *testing.T) {
	want := []string{"run", "--coverage", "--config", "entry.mjs"}
	got := VitestCoverageArgs("entry.mjs")
	if len(got) != len(want) {
		t.Fatalf("VitestCoverageArgs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("VitestCoverageArgs() = %v, want %v", got, want)
		}
	}
}
