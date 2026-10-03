package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/report"
	"github.com/Disble/dharness/internal/runner"
	"github.com/Disble/dharness/internal/tool"
)

// eslintDiagnostics returns the notes a successful sync owes its reader
// about the ESLint layer it just wired: whether the project's own ESLint can
// actually read the project's sources through that layer.
//
// It is asked after the step loop rather than with collectNotes, because it
// is a question about bytes this run wrote and its answer does not exist
// until they are on disk. A failed run returns before ever reaching it, so a
// rolled-back repository derives no note from a config that was undone.
//
// This is a different question from the one eslintExtendsStep.Verify asks,
// and the two must not be collapsed. Verify runs `eslint --print-config`,
// which proves the config array resolves and parses nothing at all: a `.tsx`
// file no configured parser can read resolves cleanly, which is the silence
// this whole change exists to close. Verify's failure is broken wiring, so
// it rolls the write back (§20, "bytes already written that have to be
// returned"). A source ESLint cannot parse is not broken wiring — the layer
// is exactly where it belongs — so it is a recoverable note and never a
// rollback. Lint debt is a project linting successfully.
//
// It asks once, over eslintProbePaths' own bounded set, and reads only
// ESLint's machine JSON (§09: the direct signal, never a heuristic over
// prose). No local ESLint is no measurement at all, and this stays silent
// about it: a missing tool is not a broken config (§20), dharness never
// installs one at sync time, and the ESLint absence rule is already
// recorded twice for the same reason — eslintConfigLoads answers "loads"
// and resolvedConfig answers "absent" rather than inventing a verdict. A
// note here would also be noise on every sync of every project that has not
// installed ESLint yet, for a state dharness deliberately does not change.
func eslintDiagnostics(p project.Project) []report.Note {
	configPath, wired := eslintWiredConfig(p)
	if !wired {
		return nil
	}
	binary := p.LocalBinary(tool.ESLint)
	if binary == "" {
		return nil
	}

	display := rootRelative(p, configPath)
	files := eslintProbePaths(p)

	var stdout, stderr bytes.Buffer
	probe := tool.Installed(tool.ESLint, binary, p.Source, tool.ESLintDiagnostics(files)...)
	runErr := runner.Run(probe, &stdout, &stderr)

	results, parseErr := parseESLintResults(stdout.Bytes())
	if parseErr != nil || !lintVerdict(runErr) {
		return []report.Note{eslintNotCheckedNote(display, files, eslintMeasurementDetail(runErr, stderr.String(), parseErr))}
	}

	entries := eslintFatalEntries(p, results)
	if len(entries) == 0 {
		return nil
	}
	return []report.Note{eslintParseNote(display, entries)}
}

// eslintWiredConfig names the project's flat config when this run's owned
// markers are both in it, or wired == false when the question does not
// arise.
//
// The guard is what keeps this check from judging a config dharness does not
// own. A TypeScript config, a legacy-only project and a config shape
// jsconfig refuses are all delegated by eslintExtendsStep, so dharness's
// markers are absent and nothing here speaks — reporting ESLint's verdict
// about somebody else's configuration was never the ask (§03, §05).
//
// Both pairs are required, not either: dharness never writes one region
// without the other, so a config carrying half of one can only be a hand
// edit, and treating it as wired would be reading a state dharness cannot
// have produced.
func eslintWiredConfig(p project.Project) (path string, wired bool) {
	if !p.HasSource() {
		return "", false
	}

	path = eslintFlatConfig(p.Source)
	if path == "" {
		return "", false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return path, false
	}

	text := string(raw)
	for _, pair := range []eslintMarkerPair{
		{begin: eslintImportBegin, end: eslintImportEnd},
		{begin: eslintLayerBegin, end: eslintLayerEnd},
	} {
		if !hasMarkerPair(text, pair) {
			return path, false
		}
	}
	return path, true
}

// eslintMarkerPair names one region dharness writes into a project's own
// flat config by both of its halves rather than by position.
//
// markerRegion reads a pair as "the region between this begin and that
// end", and a pair is the only thing that can say so. A positional pair
// cannot: `[][2]string` accepts a third slot nothing reads, and `pair[0]`
// beside `pair[1]` leaves a call site free to repeat or transpose the two
// halves — including one half standing in for both — while still compiling
// and still reading like a marker check.
//
// Both pairs are required, never either (see eslintWiredConfig): each half
// of each pair is named here so the guard cannot be satisfied by the wrong
// marker without the call site saying so out loud.
type eslintMarkerPair struct {
	begin string
	end   string
}

// hasMarkerPair reports whether raw carries exactly one well-formed region
// for pair — markerRegion's markersPresent as the bool this guard asks for.
func hasMarkerPair(raw string, pair eslintMarkerPair) bool {
	_, _, state := markerRegion(raw, pair.begin, pair.end)
	return state == markersPresent
}

// rootRelative names a file the way a report names every other place:
// relative to the repository root, so a split layout produces a path a
// reader can copy rather than one only this machine can resolve.
func rootRelative(p project.Project, path string) string {
	if rel, err := filepath.Rel(p.Root, path); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

// eslintDiagnosticResult is the part of ESLint's JSON reporter this check
// reads: which file a diagnostic came from, and the messages themselves.
// Everything else the reporter writes is a fact about a report dharness does
// not own (§01).
type eslintDiagnosticResult struct {
	FilePath string             `json:"filePath"`
	Messages []eslintDiagnostic `json:"messages"`
}

// eslintDiagnostic carries the three fields a note needs: whether ESLint
// called this message fatal, where it points, and ESLint's own words.
//
// `fatal` is the whole classification. It marks a message ESLint could not
// produce from a parsed file — a parsing error, or an integration failure
// such as a configured parser that refuses the source — and an ordinary rule
// finding never carries it. It does not say what the cause is; nothing in
// the message is read to guess (§17: the verdict is the flag, and the agent
// edits).
type eslintDiagnostic struct {
	Fatal   bool   `json:"fatal"`
	Message string `json:"message"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
}

// parseESLintResults reads the JSON array ESLint's own json reporter writes,
// and refuses anything that is not one. --format json writes an array of
// per-file results, so blank output, an object or a literal null is not a
// measurement dharness may read a verdict out of — reporting one as clean
// would be the exact silence this change closes.
func parseESLintResults(raw []byte) ([]eslintDiagnosticResult, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, errors.New("stdout did not carry ESLint's JSON results array")
	}
	var results []eslintDiagnosticResult
	if err := json.Unmarshal(trimmed, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// lintVerdict reports whether the invocation ended the way a lint result can
// end. ESLint 9 and 10 use 0 for no findings, 1 for findings — a fatal parse
// diagnostic among them — and 2 for a configuration or internal failure.
// Anything else, a process that never started included, leaves stdout
// untrustworthy rather than authoritative: §11 makes the exit code the
// tool's own statement about what it did, and a code dharness cannot place
// is not a verdict to read files out of.
func lintVerdict(err error) bool {
	if err == nil {
		return true
	}
	var exit *runner.ExitError
	return errors.As(err, &exit) && exit.Code == 1
}

// eslintFatalEntries names every source ESLint reported a fatal diagnostic
// about, with its own location and words, in ESLint's own order.
//
// Rule findings are left out on purpose. They are what the layer exists to
// produce, they are not a config problem, and a sync that reported them
// would be a whole-tree lint gate nobody asked for — the gate checks the
// staged change, and this check is about whether the wiring reaches the
// sources at all.
func eslintFatalEntries(p project.Project, results []eslintDiagnosticResult) []string {
	var entries []string
	for _, result := range results {
		for _, message := range result.Messages {
			if !message.Fatal {
				continue
			}
			entries = append(entries, eslintFatalEntry(p, result.FilePath, message))
		}
	}
	return entries
}

// eslintFatalEntry renders one fatal diagnostic as the source it points at,
// its location when ESLint gave one, and ESLint's message verbatim. The
// message is never rewritten: the fix is almost never in dharness, and
// ESLint names parser and syntax problems better than this code could.
func eslintFatalEntry(p project.Project, file string, message eslintDiagnostic) string {
	location := ""
	if message.Line > 0 {
		location = fmt.Sprintf(":%d", message.Line)
		if message.Column > 0 {
			location += fmt.Sprintf(":%d", message.Column)
		}
	}
	return fmt.Sprintf("%s%s: %s", rootRelative(p, file), location, message.Message)
}

// eslintParseNote is the corrective handoff for a wired layer whose own
// ESLint cannot read some of the project's sources.
//
// It leads with what was measured and is explicit about what was not: a
// fatal diagnostic is evidence, and it does not prove a parser is missing.
// The three plausible causes are named without choosing between them —
// a matcher that does not reach the file, a parser that rejects the syntax,
// and a source that is genuinely invalid are all answered by different
// edits, and only the agent can look. The instructions stay inside what the
// agent may change: the consumer's parser and `files` matchers, and the
// source when it is wrong. dharness's marked regions, the project's global
// ignores and its framework configuration are named as things to keep, and
// silencing the symptom with rule disables or added `ignores` is named as
// the wrong fix, because it turns a visible gap into an invisible one. §18:
// nothing here asks a person to move data by hand.
func eslintParseNote(configPath string, entries []string) report.Note {
	return report.Note{
		Kind:       "not-parsed",
		Path:       configPath,
		Entries:    entries,
		Actionable: true,
		Reason: fmt.Sprintf(
			"ESLint itself ran with the configuration wired into %s and reported a fatal "+
				"diagnostic for the sources above, so the rules that configuration declares never "+
				"ran on them. That evidence does not prove a parser is missing: the file may fall "+
				"outside every `files` matcher the project declares, the parser configured for it "+
				"may not accept this syntax, or the source itself may be invalid. Review the "+
				"parser and the `files` matchers in %s for these extensions, and fix the source "+
				"when the diagnostic says the syntax is invalid. Keep dharness's marked regions, "+
				"the project's global ignores and its framework configuration as they are, and do "+
				"not silence this with `eslint-disable` comments or by adding these paths to "+
				"`ignores` — that hides the gap instead of closing it. Nothing was rolled back: "+
				"the wiring stays, and the next `dharness sync` asks again, so this note "+
				"disappears by itself once ESLint parses these sources.",
			configPath, configPath),
	}
}

// eslintNotCheckedNote is the handoff for a wired layer this run could not
// measure: ESLint failed to execute, or its output was not the array this
// check reads.
//
// It is not a verdict about the sources, and it says so — an unanswered
// question reported as a pass is the failure mode both this note and the
// exit-code rule behind it exist to prevent. The tool's own words are
// carried verbatim, with the runner's error when there is one, and the
// rerun it asks for is one the agent can perform (§16, §21).
func eslintNotCheckedNote(configPath string, files []string, detail string) report.Note {
	return report.Note{
		Kind:       "not-checked",
		Path:       configPath,
		Entries:    files,
		Actionable: true,
		Reason: fmt.Sprintf(
			"the ESLint layer is wired into %s, but this run could not read a verdict out of the "+
				"project's own ESLint, so whether the sources above parse is unanswered rather than "+
				"confirmed. %s The wiring is untouched: this is a skipped measurement, not a "+
				"rollback. Rerun the project's own ESLint over those files with "+
				"`--no-warn-ignored --format json` to see it directly; `dharness sync` asks again "+
				"on the next run.",
			configPath, detail),
	}
}

// eslintMeasurementDetail states what the tool said, in the tool's own
// words, and never what they mean.
//
// Every branch here is reachable from one of the two failures the caller
// names: an invocation that could not be run or did not end in a lint
// verdict has a run error, and output that is not the results array has a
// parse error. The note's own reason supplies the surrounding prose, so this
// returns fragments rather than a sentence (§16).
func eslintMeasurementDetail(runErr error, stderr string, parseErr error) string {
	var parts []string
	if trimmed := strings.TrimSpace(stderr); trimmed != "" {
		parts = append(parts, "ESLint's own output was: "+trimmed)
	}
	if runErr != nil {
		parts = append(parts, "the invocation ended with: "+runErr.Error()+".")
	}
	if parseErr != nil {
		parts = append(parts, "and "+parseErr.Error()+".")
	}
	return strings.Join(parts, " ")
}
