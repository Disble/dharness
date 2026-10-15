package setup

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Disble/dharness/internal/jsconfig"
	"github.com/Disble/dharness/internal/project"
)

// This file is the source adapter between a consumer's own Vitest config and
// the managed runtime `coverageTiersModule` returns (S2, S5, S9). It is a pure
// byte-in/byte-out splice: it reads no file, writes no file, and classifies
// nothing. The tier table, the numbers and the adoption rules live in the Go
// resolver and never here; this adapter's whole job is to make the consumer's
// existing configuration reach `withCoverageTiers` unchanged.
//
// Three marker pairs, because a wrapper call holds consumer bytes between the
// bytes dharness owns:
//
//	import  — the binding import, inserted where AnalyzeExport says a top-level
//	          import belongs.
//	export  — the export statement, rewritten to the wrapping call.
//	input   — the exported value itself, copied verbatim between the markers so
//	          a later run can repair its own references without re-deriving, or
//	          re-emitting, the consumer's expression.
//
// The regions are line-bounded and individually addressable, the same grammar
// owned.go's presetBegin/presetEnd and eslintregion.go's marker pairs already
// use, so a reader who has seen one recognises the others.

// coverageTiersRuntimeName is the file dharness owns inside .dharness/ and the
// only thing this adapter references. The parent step writes it from
// coverageTiersModule().
const coverageTiersRuntimeName = "coverage-tiers.mjs"

const (
	coverageTiersImportBegin = "// dharness:coverage-tiers-import begin — rewritten by `dharness sync`; edits here are lost."
	coverageTiersImportEnd   = "// dharness:coverage-tiers-import end"
	coverageTiersExportBegin = "// dharness:coverage-tiers-export begin — rewritten by `dharness sync`; edits here are lost."
	coverageTiersExportEnd   = "// dharness:coverage-tiers-export end"
	coverageTiersInputBegin  = "// dharness:coverage-tiers-input begin — the exported value below is dharness's input and is copied verbatim; the regions around it are rewritten by `dharness sync`."
	coverageTiersInputEnd    = "// dharness:coverage-tiers-input end"
)

// coverageTiersBinding is the identifier the managed import binds the wrapper
// to, and coverageTiersFileURLName the one it binds Node's URL-to-path
// conversion to. They are named constants because the collision check below
// has to answer about exactly these two.
const (
	coverageTiersBinding     = "withCoverageTiers"
	coverageTiersFileURLName = "fileURLToPath"
)

var (
	// coverageTiersBindingMention and coverageTiersURLLinkMention find the
	// managed binding names in the consumer's own bytes.
	//
	// The check is deliberately conservative: it fires on any whole-word
	// occurrence outside the regions dharness owns, comments and strings
	// included. Distinguishing a mention from a declaration means resolving
	// the consumer's scopes, which this adapter does not do; a false refusal
	// costs one rename and no write, while a false acceptance produces a
	// config with a duplicate binding that fails at load.
	coverageTiersBindingMention = regexp.MustCompile(`\b` + coverageTiersBinding + `\b`)
	coverageTiersURLLinkMention = regexp.MustCompile(`\b` + coverageTiersFileURLName + `\b`)

	// coverageTiersClassValue matches a class declaration at the start of the
	// exported value, including TypeScript's `abstract class`. It is the one
	// shape that cannot be wrapped in place without taking a module-scope
	// binding away, and jsconfig.AnalyzeExport reports the class name nowhere,
	// so the answer is a stated boundary rather than a rewrite.
	coverageTiersClassValue = regexp.MustCompile(`^(?:abstract\s+)?class\b`)
)

// coverageTiersRegion is one marker pair's bounds in the bytes being spliced.
//
// from/to are line-bounded — the begin marker's whole line through the end
// marker's whole line — which is the span a replace rewrites. contentFrom and
// contentTo bound the bytes between the marker lines, which is the span a
// value is read out of and never a span a region rewrite touches.
type coverageTiersRegion struct {
	from        int
	to          int
	contentFrom int
	contentTo   int
	state       markerState
}

// locateCoverageTiersRegion bounds one marker pair the way markerRegion does
// for the ESLint regions, and additionally reports where the pair's content
// begins and ends so the preserved value can be read without re-parsing.
//
// A begin marker without a terminating line ending is malformed: the region's
// content follows the begin marker's line, so a begin at end-of-file is a
// half-written region and nothing is guessed at.
func locateCoverageTiersRegion(raw, begin, end string) coverageTiersRegion {
	beginCount := strings.Count(raw, begin)
	endCount := strings.Count(raw, end)
	if beginCount == 0 && endCount == 0 {
		return coverageTiersRegion{state: markersAbsent}
	}
	if beginCount != 1 || endCount != 1 {
		return coverageTiersRegion{state: markersMalformed}
	}

	beginIdx := strings.Index(raw, begin)
	endIdx := strings.Index(raw, end)
	if endIdx < beginIdx {
		return coverageTiersRegion{state: markersMalformed}
	}

	nl := strings.IndexByte(raw[beginIdx:], '\n')
	if nl == -1 {
		return coverageTiersRegion{state: markersMalformed}
	}
	contentFrom := beginIdx + nl + 1

	contentTo := strings.LastIndex(raw[:endIdx], "\n") + 1
	if contentTo < contentFrom {
		return coverageTiersRegion{state: markersMalformed}
	}

	from := strings.LastIndex(raw[:beginIdx], "\n") + 1
	to := endIdx + len(end)
	if trailing := strings.IndexByte(raw[to:], '\n'); trailing != -1 {
		to += trailing + 1
	} else {
		to = len(raw)
	}
	return coverageTiersRegion{
		from:        from,
		to:          to,
		contentFrom: contentFrom,
		contentTo:   contentTo,
		state:       markersPresent,
	}
}

// coverageTiersReferences is everything a run writes into the consumer's
// config besides the preserved value: the specifier the managed runtime is
// referenced by, and the two roots the runtime resolves the policy from.
//
// Every one of them is derived on the call, never recorded (§07). They are
// relative to the config file rather than to this process, so a checkout that
// moves, or a config that is read from an index snapshot at another path,
// resolves to the same repository.
type coverageTiersReferences struct {
	dialect         jsconfig.Module
	moduleSpecifier string
	repositoryRoot  string
	sourceRoot      string
}

// exportKeyword is how this dialect spells the statement the wrapper replaces.
func (refs coverageTiersReferences) exportKeyword() string {
	if refs.dialect == jsconfig.CommonJS {
		return "module.exports = "
	}
	return "export default "
}

// options is the second argument of the wrapper call: the repository root and
// the JS project root, both as expressions the runtime evaluates at load time.
func (refs coverageTiersReferences) options() string {
	return "{ repositoryRoot: " + refs.repositoryRoot + ", sourceRoot: " + fmt.Sprintf("%q", refs.sourceRoot) + " }"
}

// coverageTiersReferencesFor computes the references for one config file in
// one project, or says why the question has no relative answer.
//
// Both roots are anchored at run time — `new URL(..., import.meta.url)` in ESM,
// `__dirname` in CommonJS — because the runtime resolves `repositoryRoot`
// against its own working directory, and a runner started anywhere but the
// project root would otherwise resolve a relative root to the wrong tree. The
// URL is relative on purpose: storing the machine's absolute path would make a
// moved checkout or an index snapshot point at a directory that is no longer
// there (the same boundary `sync` keeps for every other reference it writes).
func coverageTiersReferencesFor(configPath string, p project.Project, dialect jsconfig.Module) (coverageTiersReferences, error) {
	dir := filepath.Dir(configPath)
	owned := filepath.Join(p.Root, project.Dir, coverageTiersRuntimeName)

	moduleRel, err := filepath.Rel(dir, owned)
	if err != nil {
		return coverageTiersReferences{}, fmt.Errorf(
			"this config and the managed runtime %s share no relative path (%v); dharness writes a relative reference so a moved checkout keeps resolving, and will not store an absolute path from this machine",
			owned, err)
	}

	rootRel, err := filepath.Rel(dir, p.Root)
	if err != nil {
		return coverageTiersReferences{}, fmt.Errorf(
			"this config and the repository root %s share no relative path (%v); dharness writes a relative reference so a moved checkout keeps resolving, and will not store an absolute path from this machine",
			p.Root, err)
	}

	sourceRoot := "."
	if p.HasSource() {
		sourceRel, relErr := filepath.Rel(p.Root, p.Source)
		if relErr != nil {
			return coverageTiersReferences{}, fmt.Errorf(
				"the JS project root %s is not inside the repository root %s (%v), so the runtime could not resolve one from the other",
				p.Source, p.Root, relErr)
		}
		sourceRoot = filepath.ToSlash(sourceRel)
	}

	refs := coverageTiersReferences{
		dialect:         dialect,
		moduleSpecifier: moduleSpecifier(filepath.ToSlash(moduleRel)),
		sourceRoot:      sourceRoot,
	}

	if dialect == jsconfig.CommonJS {
		if rootRel == "." {
			refs.repositoryRoot = "__dirname"
		} else {
			refs.repositoryRoot = fmt.Sprintf("require(%q).resolve(__dirname, %q)", "node:path", filepath.ToSlash(rootRel))
		}
		return refs, nil
	}
	refs.repositoryRoot = fmt.Sprintf("%s(new URL(%q, import.meta.url))", coverageTiersFileURLName, filepath.ToSlash(rootRel))
	return refs, nil
}

// coverageTiersImportRegion renders the marked import region: the binding
// import for the runtime, plus Node's fileURLToPath when the dialect needs it
// to turn the runtime URL into a path the policy resolver accepts.
func coverageTiersImportRegion(refs coverageTiersReferences, indent, eol string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s%s%s", indent, coverageTiersImportBegin, eol)
	if refs.dialect == jsconfig.CommonJS {
		fmt.Fprintf(&b, "%sconst { %s } = require(%q);%s", indent, coverageTiersBinding, refs.moduleSpecifier, eol)
	} else {
		fmt.Fprintf(&b, "%simport { %s } from \"node:url\";%s", indent, coverageTiersFileURLName, eol)
		fmt.Fprintf(&b, "%simport { %s } from %q;%s", indent, coverageTiersBinding, refs.moduleSpecifier, eol)
	}
	fmt.Fprintf(&b, "%s%s%s", indent, coverageTiersImportEnd, eol)
	return b.String()
}

// coverageTiersInputRegion renders the marked input region: the consumer's
// exported value, byte for byte, between two marker lines.
func coverageTiersInputRegion(value, eol string) string {
	return coverageTiersInputBegin + eol + value + eol + coverageTiersInputEnd + eol
}

// coverageTiersWrappedExport renders the export region for a value the
// consumer wrote as an expression: the wrapper call, with the marked value
// inside it.
func coverageTiersWrappedExport(refs coverageTiersReferences, value, indent, eol string) string {
	return fmt.Sprintf("%s%s%s", indent, coverageTiersExportBegin, eol) +
		fmt.Sprintf("%s%s%s(%s", indent, refs.exportKeyword(), coverageTiersBinding, eol) +
		coverageTiersInputRegion(value, eol) +
		fmt.Sprintf("%s, %s);%s", indent, refs.options(), eol) +
		fmt.Sprintf("%s%s%s", indent, coverageTiersExportEnd, eol)
}

// coverageTiersDeclarationExport renders the export region for a value that is
// a named default function declaration.
//
// That shape exists because such a declaration is a hoisted, module-scope
// binding: `export default function config() {}` makes `config` callable
// anywhere in the module, and moving the declaration inside a call would
// silently reduce it to a name visible only within its own expression. The
// declaration stays where it was — between the input markers, ahead of this
// region — and the export references the name it binds.
func coverageTiersDeclarationExport(refs coverageTiersReferences, declaration, indent, eol string) string {
	return fmt.Sprintf("%s%s%s", indent, coverageTiersExportBegin, eol) +
		fmt.Sprintf("%s%s%s(%s, %s);%s", indent, refs.exportKeyword(), coverageTiersBinding, declaration, refs.options(), eol) +
		fmt.Sprintf("%s%s%s", indent, coverageTiersExportEnd, eol)
}

// coverageTiersTypescript reports whether cfg is a config the TypeScript
// grammar has to parse. The caller knows the file name; the package that
// parses does not (§ AnalyzeExport's own reasoning).
func coverageTiersTypescript(configPath string) bool {
	switch strings.ToLower(filepath.Ext(configPath)) {
	case ".ts", ".mts", ".cts", ".tsx":
		return true
	}
	return false
}

// spliceCoverageTiersConfig returns the consumer's Vitest config with the
// managed references in place, or why it cannot be spliced. It never writes,
// and it never returns bytes alongside an error.
//
// Three states, and only three:
//
//   - no region at all — the first sync. jsconfig.AnalyzeExport bounds the
//     export, the value between its offsets is copied verbatim, and both
//     regions are inserted.
//   - every region present — a later sync. The preserved value is read back
//     out of its own markers and both references are rendered again, so a
//     moved config repairs its own paths and a converged one is unchanged
//     byte for byte.
//   - anything in between — a half-written state dharness never produces. It
//     is refused rather than guessed at, the same rule the ESLint marker pairs
//     already follow.
//
// The candidate is re-analyzed before it is returned, so an unparseable result
// never leaves this function (design decision 6's guard, applied to a file
// dharness does not own).
func spliceCoverageTiersConfig(raw []byte, configPath string, p project.Project) ([]byte, error) {
	name := filepath.Base(configPath)
	if p.Root == "" {
		return nil, fmt.Errorf("%s: the project has no repository root, so the managed runtime %s has no directory to be referenced from", name, coverageTiersRuntimeName)
	}

	text := string(raw)
	importRegion := locateCoverageTiersRegion(text, coverageTiersImportBegin, coverageTiersImportEnd)
	exportRegion := locateCoverageTiersRegion(text, coverageTiersExportBegin, coverageTiersExportEnd)
	inputRegion := locateCoverageTiersRegion(text, coverageTiersInputBegin, coverageTiersInputEnd)

	for _, pair := range []struct {
		label  string
		region coverageTiersRegion
	}{
		{"dharness:coverage-tiers-import", importRegion},
		{"dharness:coverage-tiers-export", exportRegion},
		{"dharness:coverage-tiers-input", inputRegion},
	} {
		if pair.region.state == markersMalformed {
			return nil, fmt.Errorf(
				"%s: the %s marker pair is malformed — a begin with no matching end, an end with no matching begin, or more than one of either — and dharness refuses to guess at a half-written region",
				name, pair.label)
		}
	}

	present := 0
	for _, state := range []markerState{importRegion.state, exportRegion.state, inputRegion.state} {
		if state == markersPresent {
			present++
		}
	}
	switch present {
	case 0:
		return spliceCoverageTiersFirstRun(raw, configPath, p)
	case 3:
		return repairCoverageTiersConfig(raw, configPath, p, importRegion, exportRegion, inputRegion)
	default:
		return nil, fmt.Errorf(
			"%s: the dharness:coverage-tiers marker pairs disagree on whether they exist — some are present and others are not, which dharness never writes and will not guess at",
			name)
	}
}

// spliceCoverageTiersFirstRun is the splice that adds the regions, over a
// config that carries none.
func spliceCoverageTiersFirstRun(raw []byte, configPath string, p project.Project) ([]byte, error) {
	name := filepath.Base(configPath)
	typescript := coverageTiersTypescript(configPath)

	span, why, ok := jsconfig.AnalyzeExport(raw, typescript)
	if !ok {
		return nil, fmt.Errorf("%s: %s", name, why)
	}

	value := string(raw[span.ValueStart:span.ValueEnd])
	if coverageTiersClassValue.MatchString(value) {
		return nil, fmt.Errorf(
			"%s: the default export is a class declaration, and wrapping one in a call would turn its module-scope binding into a name visible only inside itself; dharness reports that boundary instead of rewriting it — export the configuration as an object, a defineConfig(...) call, or a function",
			name)
	}
	if clash := coverageTiersBindingClash(raw, span.Dialect, nil); clash != "" {
		return nil, fmt.Errorf(
			"%s: this config already mentions %s outside any region dharness owns, and the managed import would bind that name a second time; rename it, or leave the coverage wiring to a config without the collision",
			name, clash)
	}

	refs, err := coverageTiersReferencesFor(configPath, p, span.Dialect)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}

	// The statement's own line, taken whole when it holds nothing but the
	// statement, so the replacement carries exactly the indentation the
	// original line had — and so a later run, which reads the region's own
	// indent, reproduces these bytes.
	from := span.StatementStart
	indent := ""
	if lineStart := strings.LastIndex(string(raw[:span.StatementStart]), "\n") + 1; strings.TrimSpace(string(raw[lineStart:span.StatementStart])) == "" {
		indent = string(raw[lineStart:span.StatementStart])
		from = lineStart
	}
	eol := regionLineEnding(string(raw), span.StatementStart)

	body := coverageTiersWrappedExport(refs, value, indent, eol)
	if span.DefaultFunctionName != "" {
		body = coverageTiersInputRegion(value, eol) + coverageTiersDeclarationExport(refs, span.DefaultFunctionName, indent, eol)
	}
	// AnalyzeExport anchors ImportAt after the last import's *line*, so a
	// config that writes its export on that same line anchors the insert inside
	// the bytes this replacement takes. That region goes ahead of the statement
	// — and, like every managed region, at the start of a line of its own:
	// the region's own bounds are line-bounded, so a marker sharing a line with
	// the consumer's text could not be told apart from it on the next run.
	importRegion := coverageTiersImportRegion(refs, "", eol)
	importAt := min(span.ImportAt, from)
	if importAt > 0 && raw[importAt-1] != '\n' {
		importRegion = eol + importRegion
	}
	// The same rule at the statement's own start. When the consumer wrote
	// something before the export statement on that line, the line's start
	// belongs to the consumer's bytes and cannot also be the region's first
	// line: leaving the begin marker glued to them would put them inside the
	// region, and the next run, whose bounds are line-bounded, would rewrite
	// them away. The body takes a line ending of its own instead, which moves
	// no byte the consumer wrote. The import region above already supplies
	// that separation when it is spliced at this same offset — the case the
	// one-line config below depends on — so this is the case where it is not.
	if from > importAt && raw[from-1] != '\n' {
		body = eol + body
	}

	candidate := replaceRange(raw, from, span.StatementEnd, body)
	candidate = jsconfig.Splice(candidate, importAt, importRegion)

	if err := verifyCoverageTiersCandidate(candidate, configPath); err != nil {
		return nil, err
	}
	return candidate, nil
}

// repairCoverageTiersConfig rewrites only the regions dharness owns over a
// config that already carries all three: the preserved value is read back out
// of its markers and never re-emitted through a re-parse, and the two
// reference regions are rendered again from this run's config path and
// project.
//
// The value is re-analyzed on its own — "export default <value>" — because
// that is the only way to learn again whether the preserved bytes are a
// declaration whose module-scope binding has to survive. Whether the preserved
// value is an expression or a declaration also fixes where its markers must
// sit relative to the export region, so a hand edit that moved them is a
// stated conflict rather than a silent rewrite.
func repairCoverageTiersConfig(raw []byte, configPath string, p project.Project, importRegion, exportRegion, inputRegion coverageTiersRegion) ([]byte, error) {
	name := filepath.Base(configPath)

	if importRegion.to > exportRegion.from {
		return nil, fmt.Errorf("%s: the dharness:coverage-tiers-import region does not precede the dharness:coverage-tiers-export region, which dharness never writes and will not guess at", name)
	}

	value, err := coverageTiersInputValue(raw, inputRegion)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if coverageTiersClassValue.Match(value) {
		return nil, fmt.Errorf(
			"%s: the value between the dharness:coverage-tiers-input markers is a class declaration, whose module-scope binding a wrapping call cannot preserve; dharness reports that boundary instead of rewriting it",
			name)
	}

	dialect := jsconfig.ModuleOf(raw)
	refs, err := coverageTiersReferencesFor(configPath, p, dialect)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}

	span, why, ok := jsconfig.AnalyzeExport(append([]byte(refs.exportKeyword()), value...), coverageTiersTypescript(configPath))
	if !ok {
		return nil, fmt.Errorf("%s: the value preserved between the dharness:coverage-tiers-input markers no longer parses as an export: %s", name, why)
	}
	declaration := span.DefaultFunctionName

	if declaration == "" {
		if exportRegion.from > inputRegion.from || inputRegion.to > exportRegion.to {
			return nil, fmt.Errorf("%s: the dharness:coverage-tiers-input region is not inside the dharness:coverage-tiers-export region, though the preserved value is an expression the wrapper call has to hold", name)
		}
	} else if inputRegion.to > exportRegion.from {
		return nil, fmt.Errorf("%s: the dharness:coverage-tiers-input region follows the dharness:coverage-tiers-export region, though the preserved value is a declaration the export has to reference", name)
	}

	skip := [][2]int{{importRegion.from, importRegion.to}}
	if declaration == "" {
		// The wrapper's own lines are skipped; the preserved value between
		// the input markers is the consumer's and is scanned.
		skip = append(skip, [2]int{exportRegion.from, inputRegion.from}, [2]int{inputRegion.to, exportRegion.to})
	} else {
		skip = append(skip, [2]int{exportRegion.from, exportRegion.to})
	}
	if clash := coverageTiersBindingClash(raw, dialect, skip); clash != "" {
		return nil, fmt.Errorf("%s: this config mentions %s outside any region dharness owns, and the managed import would bind that name a second time; rename it, or leave the coverage wiring to a config without the collision", name, clash)
	}

	text := string(raw)
	indent := regionIndent(text, exportRegion.from)
	eol := regionLineEnding(text, exportRegion.from)
	body := coverageTiersWrappedExport(refs, string(value), indent, eol)
	if declaration != "" {
		// Only the export region is rewritten here: the declaration is the
		// consumer's own bytes and stays where the first run left it.
		body = coverageTiersDeclarationExport(refs, declaration, indent, eol)
	}
	candidate := replaceRange(raw, exportRegion.from, exportRegion.to, body)
	candidate = replaceRange(candidate, importRegion.from, importRegion.to,
		coverageTiersImportRegion(refs, regionIndent(text, importRegion.from), regionLineEnding(text, importRegion.from)))

	if err := verifyCoverageTiersCandidate(candidate, configPath); err != nil {
		return nil, err
	}
	if bytes.Equal(candidate, raw) {
		return raw, nil
	}
	return candidate, nil
}

// coverageTiersInputValue reads the preserved value back out of its markers.
//
// The value is framed by one line ending: the renderer writes the value on its
// own line after the begin marker and the end marker on the line after it, so
// a region whose content does not end that way was not written by this
// adapter and is refused rather than trimmed.
func coverageTiersInputValue(raw []byte, inputRegion coverageTiersRegion) ([]byte, error) {
	content := raw[inputRegion.contentFrom:inputRegion.contentTo]
	switch {
	case bytes.HasSuffix(content, []byte("\r\n")):
		content = content[:len(content)-2]
	case bytes.HasSuffix(content, []byte("\n")):
		content = content[:len(content)-1]
	default:
		return nil, errors.New("the dharness:coverage-tiers-input region's value is not on its own line, which dharness never writes")
	}
	if len(content) == 0 {
		return nil, errors.New("the dharness:coverage-tiers-input region carries no value")
	}
	return content, nil
}

// coverageTiersBindingClash returns the managed binding name the consumer's
// own bytes mention, or "" when there is none.
//
// skip holds the spans dharness wrote, so the scan reads what the consumer
// wrote and nothing else: the preserved value plus every byte outside the
// managed regions. The first-run caller passes no spans; the repair caller
// checks the located regions' order and containment before constructing these
// non-overlapping spans in source order. This helper does not normalize or
// silently discard ranges: validating consumer regions belongs to that caller.
// fileURLToPath is only dharness's binding in ESM, where the repository root is
// resolved through a URL; in CommonJS it is somebody else's business.
func coverageTiersBindingClash(raw []byte, dialect jsconfig.Module, skip [][2]int) string {
	var scan bytes.Buffer
	at := 0
	for _, span := range skip {
		scan.Write(raw[at:span[0]])
		at = span[1]
	}
	scan.Write(raw[at:])

	if coverageTiersBindingMention.Match(scan.Bytes()) {
		return coverageTiersBinding
	}
	if dialect != jsconfig.CommonJS && coverageTiersURLLinkMention.Match(scan.Bytes()) {
		return coverageTiersFileURLName
	}
	return ""
}

// verifyCoverageTiersCandidate re-parses the generated config before it can
// reach the caller's write path. Marker uniqueness is checked on input; the
// region renderers emit exactly one pair each, so there is no second marker
// grammar to validate the bytes those renderers just wrote.
func verifyCoverageTiersCandidate(candidate []byte, configPath string) error {
	name := filepath.Base(configPath)
	if _, why, ok := jsconfig.AnalyzeExport(candidate, coverageTiersTypescript(configPath)); !ok {
		return fmt.Errorf("%s: the resulting config would not parse: %s", name, why)
	}
	return nil
}
