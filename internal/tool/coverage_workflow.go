package tool

import (
	"fmt"
	"runtime"
	"strings"
)

// Vitest is the runner the coverage step of the gate invokes: the project's
// own, never a copy dharness provisions.
const Vitest = "vitest"

// localExecutors are each detected manager's own local-only form.
//
// The other four forms in this package — bunx, pnpm dlx, npx and yarn's npx
// fallback — resolve a package the project may not have installed, which means
// the network. A hook that runs on every commit cannot be the moment that
// happens, so the gate gets the forms that only ever answer from the project's
// own node_modules: `--no-install` for the two that would otherwise fetch,
// `exec` for the two that never do. A binary that is genuinely absent fails,
// and the step that read this line is what says how to install it.
var localExecutors = map[string][]string{
	"bun":  {"bunx", "--no-install"},
	"pnpm": {"pnpm", "exec"},
	"yarn": {"yarn", "exec"},
	"npm":  {"npx", "--no-install"},
}

// CoverageShell names the interpreter a hook line is written for.
//
// It is not the operating system, which is the mistake this parameter exists to
// prevent: a husky hook is a POSIX shell script on Windows too, while a lefthook
// job goes through cmd.exe there. The line is quoted for whoever will read it.
type CoverageShell int

const (
	// ShellCmd is cmd.exe, which lefthook hands a command to on Windows.
	ShellCmd CoverageShell = iota

	// ShellPOSIX is sh: every husky hook, and every lefthook job off Windows.
	ShellPOSIX
)

// HostCoverageShell is the shell a lefthook job is read by on this machine.
// husky never asks: its hook is a POSIX script wherever it runs, which is why
// the caller names ShellPOSIX for it instead of calling this.
func HostCoverageShell() CoverageShell {
	if runtime.GOOS == "windows" {
		return ShellCmd
	}
	return ShellPOSIX
}

// VitestCoverageArgs is the argument list vitest itself receives for a
// complete coverage run. It is a separate function because the list is a fact
// about the runner and about nothing else — the same reason every other
// invocation in this package keeps its flags in one place.
func VitestCoverageArgs(config string) []string {
	return []string{"run", "--coverage", "--config", config}
}

// VitestCoverageCommand renders the gate's coverage line: the project's own
// manager, in its local-only form, running the project's own Vitest with
// coverage against the bridge entry dharness owns.
//
// runDir is the JS project root relative to the directory the hook runs in —
// empty when they are the same directory — because vitest resolves its own
// configuration and its project root against the process's working directory.
// config is the bridge entry relative to that JS project root.
//
// shell is the interpreter that will read the line, and the value itself is
// never rewritten for it: an argument cmd.exe would expand before the program
// starts is refused rather than escaped, the same rule runner's own shim
// command line already records for a .cmd shim. A POSIX shell carries a per
// cent sign as an ordinary character, so only the cmd rendering refuses.
func VitestCoverageCommand(shell CoverageShell, packageManager, runDir, config string) (string, error) {
	words := append([]string{}, localExecutor(packageManager)...)
	words = append(words, Vitest)
	words = append(words, VitestCoverageArgs(config)...)

	if runDir != "" && runDir != "." {
		words = append([]string{"cd", runDir, "&&"}, words...)
	}

	var line strings.Builder
	for i, word := range words {
		if i > 0 {
			line.WriteByte(' ')
		}
		if word == "&&" {
			// Shell syntax, not a value: quoting it would turn the operator
			// into an argument in either interpreter.
			line.WriteString(word)
			continue
		}
		if shell == ShellCmd {
			if bad := undeliverableWord(word); bad != "" {
				return "", fmt.Errorf(
					"coverage: %q holds %s, which cmd.exe rewrites before the runner starts; rename the path",
					word, bad,
				)
			}
			line.WriteString(cmdWord(word))
			continue
		}
		line.WriteString(shWord(word))
	}
	return line.String(), nil
}

// CoverageRefusal renders a hook line that fails by name.
//
// It is what an activated repository gets when its coverage line cannot be
// rendered: the gate keeps a job that refuses the commit with the reason,
// instead of losing the job and passing the commit unmeasured. The failure is
// the point, so the line ends in a non-zero exit in both interpreters, and the
// reason is flattened to one line and stripped of the two characters cmd.exe
// rewrites before the program runs.
func CoverageRefusal(shell CoverageShell, reason string) string {
	message := "coverage tiers: " + coverageRefusalReason(reason)
	if shell == ShellCmd {
		return "echo " + cmdWord(message) + " 1>&2 && exit 1"
	}
	return "echo " + shWord(message) + " 1>&2 && exit 1"
}

// coverageRefusalReason flattens a reason into one line without the two
// characters neither shell can be handed reliably: a quote ends the quoting
// this line depends on, and a per cent sign is expanded by cmd.exe before the
// program runs.
func coverageRefusalReason(reason string) string {
	reason = strings.Join(strings.Fields(reason), " ")
	reason = strings.NewReplacer(`"`, "", "%", "", "`", "").Replace(reason)
	if reason == "" {
		return "the coverage tiers are activated but their native command could not be rendered; run `dharness sync`"
	}
	return reason
}

// localExecutor is the detected manager's local-only form, falling back to
// npm's for a manager this package does not know — normalizeManager's own
// answer, so the printable form and the executed one cannot disagree.
func localExecutor(packageManager string) []string {
	return localExecutors[normalizeManager(packageManager)]
}

// undeliverableWord names the character cmd.exe cannot be handed in a value:
// a per cent sign is expanded before the program runs and survives no escape,
// and a quote ends the quoting this line depends on. Neither is reachable from
// a repository-relative path dharness builds, and the answer is to refuse
// rather than to run the runner against something other than what was asked.
func undeliverableWord(word string) string {
	if strings.Contains(word, "%") {
		return "a per cent sign"
	}
	if strings.Contains(word, `"`) {
		return "a quote"
	}
	return ""
}

// shWord quotes a value for a POSIX shell unless every character in it is one
// the shell reads literally, in which case the quotes would be noise.
//
// The safe set is deliberately narrow and every character outside it is quoted,
// because the cost is asymmetric: a quoted ordinary path is still that path,
// while an unquoted `$( … )`, backtick, pipe, ampersand, semicolon, angle
// bracket, parenthesis, bracket, glob, tilde, hash or exclamation mark is
// something the shell does before the runner starts. Single quotes, because
// they are the one form that is literal all the way through.
func shWord(word string) string {
	if word != "" && !strings.ContainsFunc(word, unsafeWithoutQuotes) {
		return word
	}
	return "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
}

// unsafeWithoutQuotes reports whether r is a character a POSIX shell would
// treat as anything other than itself in an unquoted word.
func unsafeWithoutQuotes(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return false
	}
	switch r {
	case '.', '_', '/', '-', ':', '=', '@', '+', ',', '%', '^':
		return false
	}
	return true
}

// cmdWord quotes a value for cmd.exe, whose metacharacters reach further than
// CreateProcess's rule does — the measured lesson runner's shim command line
// already carries for a .cmd shim. Only the characters this line could hold
// are quoted for; the and-operator is handled by the caller, because quoting
// it would turn a separator into an argument.
func cmdWord(word string) string {
	if !strings.ContainsAny(word, " \t&|<>^()") {
		return word
	}
	return `"` + word + `"`
}
