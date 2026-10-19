package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/Disble/dharness/internal/project"
)

// This file answers S2's question: does the active hook, through the local
// package scripts it reaches, already run Vitest — and if so, is that run
// the same complete suite the managed coverage gate runs?
//
// The analysis is read-only and bounded. It reads the active pre-commit
// commands (the lefthook pre-commit section with each job's own `root:`, or
// the husky hook lines) and the package scripts those commands reach — never
// an unused script, never the managed coverage job itself, never CI, and
// never a second interpreter. A script chain is followed at most
// maxVitestScriptDepth deep, and a cycle is an answer (unresolved), not a
// loop.
//
// The verdict has three states, and the difference between the second and
// the third is the whole of S3: a recognized complete run is a confirmed
// duplicate; an opaque command is uncertainty, reported as uncertainty rather
// than assumed safe. Scoped runs — focused files, watch mode, a test
// selection, an unrelated suite in another root or under another config —
// are neither: they are left alone and the routine integration proceeds
// around them.
//
// Ownership is per actual invocation, never a substring anywhere in a
// compound: `dharness check && bun run test` analyzes the second command,
// and a managed segment beside a real duplicate does not hide it. Only
// bounded, static splitting (`&&`, `||`, `;`) is used — no shell is ever
// executed to learn what runs.

// vitestGateStatus is what the active hook resolves to.
type vitestGateStatus int

const (
	// vitestGateClear means no detected conflict: nothing reachable runs the
	// complete suite, or nothing is reachable at all.
	vitestGateClear vitestGateStatus = iota
	// vitestGateEquivalent means a reachable command runs the complete Vitest
	// suite the managed gate runs: a confirmed duplicate.
	vitestGateEquivalent
	// vitestGateUnresolved means a reachable command might run Vitest but
	// cannot be resolved to a concrete invocation: unknown, not confirmed.
	vitestGateUnresolved
)

// vitestGateFinding is one analysis answer: the verdict plus the concrete
// evidence a reader can act on. where names the file and section
// ("lefthook.yml pre-commit", ".husky/pre-commit"), command the hook's own
// command, through the package-script chain that reaches the verdict,
// outermost first ("" when the command runs Vitest directly), and reason the
// explanation when the verdict is uncertain.
type vitestGateFinding struct {
	status  vitestGateStatus
	where   string
	command string
	through string
	reason  string
}

// maxVitestScriptDepth bounds how far one hook command is followed through
// package.json scripts. Deeper than this is not a deeper answer, it is a
// chain nobody can read at a glance, so it resolves as unknown.
const maxVitestScriptDepth = 8

// hookCommand is one active pre-commit invocation plus the working directory
// it runs in: the job's own lefthook `root:` value, or "" when the job runs
// where the hook runs. rootUnknown marks a `root:` the analysis cannot
// resolve, which is uncertainty rather than an assumption.
type hookCommand struct {
	run         string
	root        string
	rootUnknown bool
}

// suiteScope carries what an invocation's identity is resolved against: the
// absolute directory the command starts in, the absolute managed JS source
// root, and the absolute native Vitest/Vite config, or "" when the project
// has none to compare against.
type suiteScope struct {
	base   string
	source string
	native string
}

// analyzeVitestGate derives the hook's Vitest verdict, or the clear zero
// value when there is no managed gate to duplicate: without an active
// coverage integration the consumer's own run is their only gate, and there
// is nothing to conflict with.
func analyzeVitestGate(p project.Project) vitestGateFinding {
	if !coverageTiersActivated(p) {
		return vitestGateFinding{}
	}

	var where string
	var commands []hookCommand
	switch hookManager(p) {
	case managerLefthook:
		raw, err := os.ReadFile(filepath.Join(p.Root, lefthookConfig))
		if err != nil {
			return vitestGateFinding{}
		}
		where = lefthookConfig + " pre-commit"
		commands = lefthookPrecommitRuns(raw)
	case managerHusky:
		raw, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(huskyHook)))
		if err != nil {
			return vitestGateFinding{}
		}
		where = huskyHook
		for _, run := range huskyHookCommands(raw) {
			commands = append(commands, hookCommand{run: run})
		}
	default:
		return vitestGateFinding{}
	}

	scripts := packageScripts(p)
	scope := suiteScope{
		source: filepath.Clean(p.Source),
		native: coverageTiersNativeConfig(p),
	}
	var uncertain *vitestGateFinding
	for _, command := range commands {
		if command.rootUnknown {
			return vitestGateFinding{
				status:  vitestGateUnresolved,
				where:   where,
				command: command.run,
				reason:  "its working directory cannot be resolved without executing it",
			}
		}
		scope.base = filepath.Clean(filepath.Join(p.Root, command.root))
		verdict := classifyGateCommand(scripts, command.run, 0, nil, scope)
		switch verdict.status {
		case vitestGateEquivalent:
			return vitestGateFinding{status: verdict.status, where: where, command: command.run, through: verdict.chain}
		case vitestGateUnresolved:
			if uncertain == nil {
				uncertain = &vitestGateFinding{status: verdict.status, where: where, command: command.run, through: verdict.chain, reason: verdict.reason}
			}
		}
	}
	if uncertain != nil {
		return *uncertain
	}
	return vitestGateFinding{}
}

// isManagedGateCommand reports whether one hook segment is dharness's own
// gate rather than the consumer's: the check line, or the coverage
// invocation the managed gate renders. The gate must never conflict with
// itself, or every activated repository would report a duplicate with its
// own wiring.
func isManagedGateCommand(segment string) bool {
	return strings.Contains(segment, gateCommand) ||
		strings.Contains(segment, coverageBridgeName) ||
		strings.Contains(segment, "vitest.coverage")
}

// lefthookPrecommitRuns lists the pre-commit section's `run:` values in file
// order, each with its job's own `root:` value. Only pre-commit answers: a
// pre-push audit is an unrelated check, not an active gate command. The parse
// is line-based because the product is stdlib-only; a value wrapped in
// matching quotes is unwrapped.
//
// A `root:` belongs to the runs of its own job block: any other key line
// ends the block, so a root in one job can never leak into another's runs.
// List-form jobs (`- run:`) read the same way after one dash is stripped.
func lefthookPrecommitRuns(raw []byte) []hookCommand {
	type item struct {
		kind  string // "run", "root", or "boundary"
		value string
	}
	var items []item
	inPrecommit := false
	for _, line := range strings.Split(string(raw), "\n") {
		if line == "" || line[0] == ' ' || line[0] == '\t' {
			if !inPrecommit {
				continue
			}
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- ") {
				trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			}
			switch {
			case strings.HasPrefix(trimmed, "run:"):
				items = append(items, item{kind: "run", value: unquoteGateValue(strings.TrimSpace(strings.TrimPrefix(trimmed, "run:")))})
			case strings.HasPrefix(trimmed, "root:"):
				items = append(items, item{kind: "root", value: unquoteGateValue(strings.TrimSpace(strings.TrimPrefix(trimmed, "root:")))})
			case isKeyLine(trimmed):
				items = append(items, item{kind: "boundary"})
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		name, _, ok := strings.Cut(trimmed, ":")
		inPrecommit = ok && strings.TrimSpace(name) == "pre-commit"
	}

	var commands []hookCommand
	var group []item
	flush := func() {
		root, unknown := "", false
		for _, it := range group {
			if it.kind == "root" {
				if strings.Contains(it.value, "$") || strings.Contains(it.value, "`") {
					unknown = true
				} else {
					root, unknown = it.value, false
				}
			}
		}
		for _, it := range group {
			if it.kind == "run" && it.value != "" {
				commands = append(commands, hookCommand{run: it.value, root: root, rootUnknown: unknown})
			}
		}
		group = nil
	}
	for _, it := range items {
		if it.kind == "boundary" {
			flush()
			continue
		}
		group = append(group, it)
	}
	flush()
	return commands
}

// isKeyLine reports whether a trimmed indented line opens a key — anything
// that is not a run:, root: or list value, and therefore ends the job block
// a root: could belong to.
func isKeyLine(trimmed string) bool {
	if strings.HasPrefix(trimmed, "- ") || trimmed == "-" {
		return false
	}
	name, _, ok := strings.Cut(trimmed, ":")
	if !ok {
		return false
	}
	name = strings.TrimSpace(name)
	return name != "" && name != "run" && name != "root"
}

// huskyHookCommands splits a husky hook into its own command lines: one per
// line, without the comments and the shebang. The hook is the project's own
// shell script, so each line is read literally — no shell is ever executed to
// learn what runs.
//
// A line is handed over whole, not pre-split on the sequencing operators. The
// classifier already tracks a `cd` across the segments that share it, so
// splitting here would discard that context before anyone could use it: `cd
// frontend && bun run test` runs the suite in the JS project, and judged at
// the repository root it looks like another suite entirely.
func huskyHookCommands(raw []byte) []string {
	var commands []string
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		commands = append(commands, trimmed)
	}
	return commands
}

// unquoteGateValue strips one pair of matching single or double quotes, the
// way a YAML plain value wearing them reads. A half-quoted value is left
// alone: repairing it is the merge's refusal, not this parse.
func unquoteGateValue(value string) string {
	if len(value) >= 2 {
		if first, last := value[0], value[len(value)-1]; (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

// packageScripts reads the local scripts one hook command may reach, or nil
// when there are none to read. A malformed manifest reads as no scripts:
// every reference into it then resolves as unknown, which is the honest
// answer about a file that cannot be read.
func packageScripts(p project.Project) map[string]string {
	if !p.HasSource() {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(p.Source, "package.json"))
	if err != nil {
		return nil
	}
	var manifest struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(raw, &manifest) != nil {
		return nil
	}
	return manifest.Scripts
}

// gateVerdict is what one command — or one segment of one — resolves to:
// the status, the script chain that reaches it, and the explanation when the
// answer is uncertain.
type gateVerdict struct {
	status vitestGateStatus
	chain  string
	reason string
}

// classifyGateCommand resolves one hook command to its verdict, following
// package-script indirection. chain carries the script path, outermost first;
// reason explains an uncertain verdict.
//
// A `cd` moves the working directory the remaining segments run in, resolved
// against where the command starts; the suite is the managed one only when
// that directory is the managed source. A command that cannot be placed — a
// dynamic target, or one no `cd` can parse — is unknown rather than assumed
// elsewhere. An unplaced suite is preserved, never analyzed as this gate's.
func classifyGateCommand(scripts map[string]string, command string, depth int, stack []string, scope suiteScope) gateVerdict {
	cur := scope.base
	var rest []string
	for _, segment := range splitGateSegments(command) {
		tokens := strings.Fields(segment)
		if len(tokens) > 0 && tokens[0] == "cd" {
			target, ok := cdTarget(tokens)
			if !ok {
				return gateVerdict{status: vitestGateUnresolved, reason: "its working directory cannot be resolved without executing it"}
			}
			if filepath.IsAbs(target) {
				cur = filepath.Clean(target)
			} else {
				cur = filepath.Clean(filepath.Join(cur, target))
			}
			continue
		}
		rest = append(rest, segment)
	}
	if filepath.Clean(cur) != filepath.Clean(scope.source) {
		// Another root runs another suite: truly unrelated, so it is left
		// alone rather than analyzed as this gate's duplicate.
		return gateVerdict{}
	}
	scope.base = cur

	uncertain := gateVerdict{}
	for _, segment := range rest {
		verdict := classifyGateSegment(scripts, segment, depth, stack, scope)
		if verdict.status == vitestGateEquivalent {
			return verdict
		}
		if verdict.status == vitestGateUnresolved && uncertain.status == vitestGateClear {
			uncertain = verdict
		}
	}
	return uncertain
}

// cdTarget names the directory a cd segment moves to, or false when the
// segment cannot be parsed statically: no target, extra words, or a dynamic
// value the analysis must not guess at.
func cdTarget(tokens []string) (string, bool) {
	if len(tokens) != 2 {
		return "", false
	}
	target := unquoteGateValue(tokens[1])
	if target == "" || target == "-" || strings.Contains(target, "$") || strings.Contains(target, "`") {
		return "", false
	}
	return target, true
}

// splitGateSegments splits one hook command on the sequencing operators a
// hook line uses. `||` joins the split because whatever follows it still
// names a program that would run; each resulting segment is classified on
// its own, so a managed segment beside a real duplicate hides nothing.
func splitGateSegments(command string) []string {
	var segments []string
	for _, and := range strings.Split(command, "&&") {
		for _, or := range strings.Split(and, "||") {
			for _, part := range strings.Split(or, ";") {
				if segment := strings.TrimSpace(part); segment != "" {
					segments = append(segments, segment)
				}
			}
		}
	}
	return segments
}

// scriptManagers are the package managers whose `run` names a local script.
// bunx, npx and dlx are absent on purpose: they execute packages, not the
// project's own scripts.
var scriptManagers = []string{"npm", "pnpm", "yarn", "bun"}

// classifyGateSegment resolves one sequenced segment: script indirection
// first, a direct Vitest invocation second, an opaque wrapper third —
// everything else is an unrelated check.
func classifyGateSegment(scripts map[string]string, segment string, depth int, stack []string, scope suiteScope) gateVerdict {
	trimmed := strings.TrimSpace(segment)
	if trimmed == "" || isManagedGateCommand(trimmed) {
		return gateVerdict{}
	}

	tokens := strings.Fields(trimmed)
	if len(tokens) > 0 && tokens[0] == "exec" {
		return classifyGateSegment(scripts, strings.Join(tokens[1:], " "), depth, stack, scope)
	}

	// A manager token only answers for a manager invocation. When it names no
	// subcommand the segment is classified on its remaining shape instead:
	// `bun scripts/check.mjs` is a launcher running a file, and the branch
	// below is the one that knows it.
	if len(tokens) > 0 && contains(scriptManagers, tokens[0]) {
		if verdict, handled := classifyManagerInvocation(scripts, tokens, depth, stack, scope); handled {
			return verdict
		}
	}

	if containsVitestWord(tokens) {
		return classifyVitestInvocation(tokens, scope)
	}

	if target, ok := opaqueWrapperTarget(tokens); ok {
		return gateVerdict{status: vitestGateUnresolved, reason: "it runs " + target + ", whose runner cannot be determined without executing it"}
	}

	return gateVerdict{}
}

// classifyManagerInvocation resolves a package-manager invocation and reports
// whether the tokens are one at all. An explicit `run <name>` — or the `test`
// alias npm, pnpm and yarn keep — follows the script it names; the manager's
// own `exec`/`x` prefix classifies the command it runs, which is a complete
// run when that command is one (`pnpm exec vitest run` is `vitest run`). A
// form the analysis cannot parse resolves as unknown rather than guessed at.
//
// A manager token with no subcommand is not a manager invocation, and the
// caller keeps classifying it on its remaining shape: `bun scripts/check.mjs`
// executes a file, and `bun audit` is bun's own check, so neither is answered
// here. `bun test` is deliberately not the alias either: it runs bun's own
// test runner, not a script the project wrote.
//
// A token in the script's position that starts with `-` is a flag, not a
// script name, so `npm run -s` is unparsed — never a missing script called
// "-s".
func classifyManagerInvocation(scripts map[string]string, tokens []string, depth int, stack []string, scope suiteScope) (gateVerdict, bool) {
	switch {
	case len(tokens) >= 2 && (tokens[1] == "exec" || tokens[1] == "x"):
		return classifyGateSegment(scripts, strings.Join(tokens[2:], " "), depth, stack, scope), true
	case len(tokens) >= 2 && tokens[1] == "run":
		if len(tokens) != 3 || strings.HasPrefix(tokens[2], "-") {
			return gateVerdict{status: vitestGateUnresolved, reason: "its package-manager invocation cannot be parsed without executing it"}, true
		}
		return followManagerScript(scripts, tokens[2], depth, stack, scope), true
	case len(tokens) == 2 && tokens[1] == "test" && tokens[0] != "bun":
		return followManagerScript(scripts, "test", depth, stack, scope), true
	default:
		return gateVerdict{}, false
	}
}

// followManagerScript resolves the script a manager invocation names, or the
// uncertainty that keeps it unresolved: a chain deeper than the analysis
// follows, a cycle, or a name package.json does not declare.
func followManagerScript(scripts map[string]string, name string, depth int, stack []string, scope suiteScope) gateVerdict {
	if depth >= maxVitestScriptDepth {
		return gateVerdict{status: vitestGateUnresolved, reason: "script " + quoteScript(name) + " chains deeper than the analysis follows"}
	}
	for _, seen := range stack {
		if seen == name {
			return gateVerdict{status: vitestGateUnresolved, reason: "script " + quoteScript(name) + " refers back to itself"}
		}
	}
	body, ok := scripts[name]
	if !ok {
		return gateVerdict{status: vitestGateUnresolved, reason: "it names script " + quoteScript(name) + ", which package.json does not declare"}
	}
	verdict := classifyGateCommand(scripts, body, depth+1, append(stack, name), scope)
	if verdict.chain != "" {
		verdict.chain = name + " → " + verdict.chain
	} else if verdict.status != vitestGateClear {
		verdict.chain = name
	}
	return verdict
}

// containsVitestWord reports whether any token names the Vitest binary: the
// bare name, or a path to it in either separator's spelling.
func containsVitestWord(tokens []string) bool {
	for _, token := range tokens {
		if token == "vitest" || token == "vitest.cmd" ||
			strings.HasSuffix(token, "/vitest") || strings.HasSuffix(token, `\vitest`) {
			return true
		}
	}
	return false
}

// opaqueWrapperTarget names the file a segment executes through a JavaScript
// launcher, or false when the segment launches nothing of the kind. What a
// `node scripts/check.mjs` runs is decided inside that file, which dharness
// never evaluates — so the invocation stays unknown.
func opaqueWrapperTarget(tokens []string) (string, bool) {
	launcher := false
	for _, token := range tokens {
		if !launcher {
			if token == "node" || token == "bun" || token == "tsx" || token == "ts-node" || token == "vite-node" || token == "jiti" {
				launcher = true
			}
			continue
		}
		if strings.HasPrefix(token, "-") {
			continue
		}
		if looksLikeScriptFile(token) {
			return token, true
		}
		return "", false
	}
	return "", false
}

// looksLikeScriptFile reports whether a token names a JavaScript file rather
// than a subcommand: `audit` is bun's own, `scripts/check.mjs` is not.
func looksLikeScriptFile(token string) bool {
	switch {
	case strings.HasSuffix(token, ".mjs"), strings.HasSuffix(token, ".cjs"),
		strings.HasSuffix(token, ".js"), strings.HasSuffix(token, ".ts"),
		strings.HasSuffix(token, ".mts"), strings.HasSuffix(token, ".cts"),
		strings.HasSuffix(token, ".jsx"), strings.HasSuffix(token, ".tsx"):
		return true
	}
	return false
}

// classifyVitestInvocation decides whether a direct Vitest invocation is the
// managed gate's complete run. The second return reports whether the
// invocation was parseable at all: dynamic shell inside the arguments cannot
// be read without executing, so it resolves as unknown rather than as
// equivalent.
//
// Complete is deliberately narrow: `run` (or `vitest --run`) with no file
// filter, no watch mode, no test selection and no alternate identity — the
// same suite the managed job runs. Resource flags (workers, pools, timeouts)
// do not narrow the suite, so they stay complete. A focused file, `--watch`,
// a `--project` or `--changed` selection, or an explicit `--config`/`--dir`
// naming another suite is a scoped run, and a scoped run is left alone,
// never renamed as the gate. A flag the analysis does not support is
// unknown, not proof of safety.
func classifyVitestInvocation(tokens []string, scope suiteScope) gateVerdict {
	start := -1
	for i, token := range tokens {
		if token == "vitest" || token == "vitest.cmd" ||
			strings.HasSuffix(token, "/vitest") || strings.HasSuffix(token, `\vitest`) {
			start = i
			break
		}
	}
	if start == -1 {
		return gateVerdict{}
	}
	args := tokens[start+1:]

	runs := false
	for _, arg := range args {
		if arg == "run" || arg == "--run" || arg == "--run=true" {
			runs = true
			break
		}
	}
	if !runs {
		// Bare `vitest` watches; `vitest watch` says so out loud. Neither is
		// the complete run the gate performs.
		return gateVerdict{}
	}

	skipNext := false
	for i, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if arg == "vitest" || arg == "run" || arg == "--run" || arg == "--run=true" {
			continue
		}
		if strings.Contains(arg, "$") || strings.Contains(arg, "`") {
			return gateVerdict{status: vitestGateUnresolved, reason: "its arguments cannot be parsed without executing it"}
		}
		base, value := splitFlagValue(arg, args, i)
		switch {
		case arg == "--run=false":
			return gateVerdict{}
		case base == "watch" || arg == "--watch" || arg == "-w" || arg == "--watch=true":
			return gateVerdict{}
		case arg == "--watch=false":
			continue
		case base == "-t" || base == "--testNamePattern" || base == "--testPathPattern" ||
			base == "--project" || base == "--changed" || base == "--related":
			// A selection narrows the suite: distinct, left alone.
			return gateVerdict{}
		case base == "--config" || base == "-c" || base == "--dir" || base == "--root":
			if !strings.Contains(arg, "=") {
				skipNext = true
			}
			same, distinct := resolveSuiteIdentity(base, value, scope)
			switch {
			case same:
				continue
			case distinct:
				return gateVerdict{}
			default:
				return gateVerdict{status: vitestGateUnresolved, reason: "its " + base + " names a suite that cannot be identified without executing it"}
			}
		case isFullSuiteFlag(base):
			if !strings.Contains(arg, "=") && isFlagWithValue(base) {
				skipNext = true
			}
			continue
		case strings.HasPrefix(arg, "-"):
			// A flag the analysis does not support is unknown, not proof
			// that the run is scoped.
			return gateVerdict{status: vitestGateUnresolved, reason: "its " + base + " flag is not supported by the analysis"}
		default:
			// A positional argument after the run marker is a file filter:
			// a focused run.
			return gateVerdict{}
		}
	}
	return gateVerdict{status: vitestGateEquivalent}
}

// splitFlagValue separates a flag's own value: `--flag=value` carries it,
// otherwise it is the next token when the flag takes one.
func splitFlagValue(arg string, args []string, i int) (string, string) {
	if eq := strings.Index(arg, "="); eq >= 0 {
		return arg[:eq], arg[eq+1:]
	}
	if isFlagWithValue(arg) && i+1 < len(args) {
		return arg, args[i+1]
	}
	return arg, ""
}

// resolveSuiteIdentity decides whether a `--config`/`--dir`/`--root` value
// names the managed suite: true when it resolves to the reference the gate
// runs (the native config, or the managed source), distinct when it names an
// existing other path, and neither when the identity cannot be established.
// A missing reference or an unreadable value is unknown, never assumed to be
// either side.
func resolveSuiteIdentity(base, value string, scope suiteScope) (same, distinct bool) {
	if value == "" || strings.Contains(value, "$") || strings.Contains(value, "`") {
		return false, false
	}
	var abs, reference string
	if filepath.IsAbs(value) {
		abs = filepath.Clean(value)
	} else {
		abs = filepath.Clean(filepath.Join(scope.base, value))
	}
	switch base {
	case "--config", "-c":
		reference = scope.native
	default:
		reference = scope.source
	}
	if reference == "" {
		return false, false
	}
	if abs == filepath.Clean(reference) {
		return true, false
	}
	if _, err := os.Stat(abs); err == nil {
		return false, true
	}
	return false, false
}

// isFullSuiteFlag reports whether a flag is one the complete run may carry:
// coverage switches and the resource flags around them. Watch, selection and
// identity flags are answered elsewhere; everything else is unknown.
func isFullSuiteFlag(base string) bool {
	switch base {
	case "--run", "--coverage", "--passWithNoTests",
		"--silent", "--hideSkippedTests", "--no-color",
		"--reporter", "--outputFile",
		"--maxWorkers", "--minWorkers", "--pool",
		"--fileParallelism", "--isolate", "--globals",
		"--testTimeout", "--hookTimeout", "--teardownTimeout",
		"--dangerouslyIgnoreUnhandledErrors", "--update":
		return true
	}
	for _, prefix := range []string{"--coverage.", "--coverage=", "--no-"} {
		if strings.HasPrefix(base, prefix) {
			return true
		}
	}
	return false
}

// isFlagWithValue reports whether a flag takes its value as the next token,
// so the parser reads `--config <path>` whole instead of reaching the path
// as a focused file, and skips what belongs to a full-suite flag instead of
// reading its value the same way.
func isFlagWithValue(base string) bool {
	switch base {
	case "--reporter", "--outputFile",
		"--maxWorkers", "--minWorkers", "--pool",
		"--testTimeout", "--hookTimeout", "--teardownTimeout",
		"--config", "-c", "--dir", "--root":
		return true
	}
	return false
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func quoteScript(name string) string {
	return "\"" + name + "\""
}
