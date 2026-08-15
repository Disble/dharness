//go:build windows

package runner

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// belowNormalPriorityClass is CREATE_NO_WINDOW's quieter neighbour: the process
// still runs whenever the machine is idle, and yields the moment anything else
// wants the CPU.
const belowNormalPriorityClass = 0x00004000

// platformize routes .cmd and .bat shims through cmd.exe.
//
// npm-installed binaries land in node_modules/.bin as .cmd shims on Windows,
// and CreateProcess cannot execute those directly. Every tool dharness wraps
// is installed that way, so without this the local path never works and every
// invocation silently falls back to the remote one.
//
// The shim case carries its own command line because os/exec cannot build a
// correct one. See shimCommandLine.
func platformize(name string, args []string) invocation {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".cmd", ".bat":
		if err := undeliverable(name, args); err != nil {
			return invocation{Err: err}
		}
		return invocation{
			Name:    "cmd",
			CmdLine: shimCommandLine(name, args),
		}
	default:
		return invocation{Name: name, Args: args}
	}
}

// undeliverable names an argument that quoting cannot carry through cmd.exe.
//
// Two characters qualify, and both are checked on the shim's own path as well
// as on its arguments, because the whole line is parsed the same way.
//
//	%  is expanded before the shim runs, and survives no escape
//	"  ends the quoting this line depends on
//
// Neither is reachable, so neither is escaped. The alternative to refusing is
// running the tool against something other than what was asked, which is worse
// than not running it: a wrong answer here is spent as a verdict.
func undeliverable(name string, args []string) error {
	for _, value := range append([]string{name}, args...) {
		for _, bad := range []string{"%", `"`} {
			if strings.Contains(value, bad) {
				return fmt.Errorf(
					"%w: %q holds %s, which cmd.exe rewrites on the way to %s; rename the path",
					ErrUndeliverableArgument, value, describe(bad), filepath.Base(name),
				)
			}
		}
	}
	return nil
}

func describe(character string) string {
	if character == "%" {
		return "a per cent sign"
	}
	return "a quote"
}

// shimCommandLine builds the exact line cmd.exe is handed for a shim.
//
// os/exec quotes an argument only when it holds a space, a tab or a quote,
// which is the correct rule for CreateProcess and the wrong one here: a shim
// is a batch file, so cmd.exe parses the line a second time against a much
// larger set of metacharacters. Measured on go1.26.5 through a real npm shim,
// an unquoted argument loses "(" and ")" to the shim's own IF block, has "&"
// read as a command separator — running whatever follows it — and has "^"
// silently deleted. Quoting every token closes all three.
//
// The outer pair of quotes around the whole line is the part that looks
// redundant and is not. With /s, cmd.exe strips the first and last character
// of the command string when both are quotes and runs the remainder verbatim;
// without that wrapper it applies a conditional rule to the inner quotes
// instead and mangles the line. Measured: /c and /s /c behave alike here, and
// /s is chosen because it makes the rule unconditional rather than dependent
// on how many quotes the line happens to contain.
//
// One character stays out of reach: cmd.exe expands %VAR% before the shim
// runs, and neither doubling nor a caret survives the second parse. Measured:
// %PROBEVAR% arrived as its value, %%PROBEVAR%% as the value still wrapped in
// per cent signs, and ^%PROBEVAR^% with the carets intact. Quoting cannot
// reach it, so it is handled by refusing rather than by escaping.
func shimCommandLine(name string, args []string) string {
	var line strings.Builder
	// The first token is argv[0] and is skipped by the program being started,
	// so the flags have to follow the name rather than replace it.
	line.WriteString(`cmd /s /c ""`)
	line.WriteString(name)
	line.WriteString(`"`)
	for _, arg := range args {
		line.WriteString(` "`)
		line.WriteString(arg)
		line.WriteString(`"`)
	}
	line.WriteString(`"`)
	return line.String()
}

// applyCmdLine hands os/exec a command line to use verbatim.
func applyCmdLine(process *exec.Cmd, cmdLine string) {
	if process.SysProcAttr == nil {
		process.SysProcAttr = &syscall.SysProcAttr{}
	}
	process.SysProcAttr.CmdLine = cmdLine
}

func beforeStart(process *exec.Cmd) {
	if process.SysProcAttr == nil {
		process.SysProcAttr = &syscall.SysProcAttr{}
	}
	process.SysProcAttr.CreationFlags |= belowNormalPriorityClass
}

// afterStart is a no-op here: Windows takes the priority at creation time.
func afterStart(int) {}
