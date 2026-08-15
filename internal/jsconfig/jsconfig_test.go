package jsconfig

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// TestDefineConfigResolvesByImportSpecifier pins Decision 5: recognition of
// defineConfig(...) is by import specifier, never by identifier name. The
// documented shape splices; a project-local function of the same name,
// imported from anywhere else, delegates.
func TestDefineConfigResolvesByImportSpecifier(t *testing.T) {
	t.Run("imported from eslint/config splices", func(t *testing.T) {
		src := []byte("import { defineConfig } from \"eslint/config\";\n" +
			"export default defineConfig([\n  { rules: {} },\n]);\n")

		anchor, why, ok := Analyze(src)
		if !ok {
			t.Fatalf("Analyze() ok = false, why = %q, want ok = true", why)
		}
		if why != "" {
			t.Errorf("Analyze() why = %q, want empty on ok = true", why)
		}
		if anchor.LayerAt <= 0 {
			t.Errorf("Analyze() LayerAt = %d, want a positive offset inside the array", anchor.LayerAt)
		}
	})

	t.Run("project-local defineConfig of the same name delegates", func(t *testing.T) {
		src := []byte("import { defineConfig } from \"./local-helpers.js\";\n" +
			"export default defineConfig([\n  { rules: {} },\n]);\n")

		_, why, ok := Analyze(src)
		if ok {
			t.Fatal("Analyze() ok = true, want false — the identifier name matches but the specifier does not")
		}
		if why == "" {
			t.Error("Analyze() why is empty on ok = false")
		}
	})
}

// TestAnalyzeRefusesEveryRefusalMatrixCell walks every shape that must
// delegate rather than splice. ".ts" is not jsconfig's case — the caller
// decides on extension before ever calling Analyze — so it is not a row
// here. Every case refuses; the four representative cells below must also
// refuse for four distinguishable reasons.
func TestAnalyzeRefusesEveryRefusalMatrixCell(t *testing.T) {
	cases := map[string]string{
		"member expression callee":               "export default tseslint.config(\n  { rules: {} },\n);\n",
		"unimported identifier callee":           "export default withNuxt([\n  { rules: {} },\n]);\n",
		"error isolated inside an array element": "export default [\n  { a: 1 + },\n  { rules: {} },\n];\n",
		"object literal export":                  "export default { rules: {} };\n",
		"identifier reference export":            "const config = [];\nexport default config;\n",
		"no export default statement":            "export const config = [];\n",
	}

	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			_, why, ok := Analyze([]byte(src))
			if ok {
				t.Fatalf("Analyze(%q) ok = true, want false", src)
			}
			if why == "" {
				t.Fatal("Analyze() why is empty on ok = false")
			}
		})
	}

	// The four refusal-matrix cells the spec names — unrecognised call,
	// ERROR node, non-array export, no default export — must each report a
	// distinguishable why, even though more than one fixture above can
	// share a cell.
	cellFixtures := map[string]string{
		"unrecognised call": cases["member expression callee"],
		"ERROR node":        cases["error isolated inside an array element"],
		"non-array export":  cases["object literal export"],
		"no default export": cases["no export default statement"],
	}
	seen := map[string]string{}
	for cell, src := range cellFixtures {
		_, why, _ := Analyze([]byte(src))
		if prior, dup := seen[why]; dup {
			t.Errorf("cell %q and cell %q share the identical why %q — distinct refusal-matrix cells must be distinguishable", prior, cell, why)
		}
		seen[why] = cell
	}
}

// TestDefineConfigWithAnErrorNodeArgumentRefuses pins the three-condition
// requirement: the callee and its import both resolve correctly, but a
// syntax error inside the array argument still refuses. All three
// conditions must hold, not two of three.
func TestDefineConfigWithAnErrorNodeArgumentRefuses(t *testing.T) {
	src := []byte("import { defineConfig } from \"eslint/config\";\n" +
		"export default defineConfig([{ a: 1 + }, { rules: {} }]);\n")

	_, why, ok := Analyze(src)
	if ok {
		t.Fatal("Analyze() ok = true, want false — the argument's parse contains an ERROR node")
	}
	if why == "" {
		t.Error("Analyze() why is empty on ok = false")
	}
}

// TestLayerLandsAtTheFirstElement pins Decision 5's position rule: the
// layer always lands at index 0, unconditionally — including when the
// array's first element is itself a framework spread. There is no
// recognition step that treats a framework spread differently from any
// other element.
func TestLayerLandsAtTheFirstElement(t *testing.T) {
	cases := map[string]struct {
		src        string
		afterMatch string // the anchor's line, which LayerAt must precede
	}{
		"plain object first": {
			src:        "export default [\n  { rules: {} },\n];\n",
			afterMatch: "{ rules: {} },",
		},
		"framework spread first": {
			src:        "export default [\n  ...nextConfig,\n  { rules: {} },\n];\n",
			afterMatch: "...nextConfig,",
		},
		"project's own element first": {
			src:        "export default [\n  { rules: { semi: \"error\" } },\n  { rules: {} },\n];\n",
			afterMatch: "{ rules: { semi: \"error\" } },",
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			src := []byte(c.src)
			anchor, why, ok := Analyze(src)
			if !ok {
				t.Fatalf("Analyze() ok = false, why = %q", why)
			}
			rest := string(src[anchor.LayerAt:])
			if !strings.HasPrefix(strings.TrimLeft(rest, " \t"), c.afterMatch) {
				t.Errorf("LayerAt does not precede the array's first element:\nrest = %q\nwant prefix (after trimming indent) %q", rest, c.afterMatch)
			}
		})
	}

	t.Run("empty array", func(t *testing.T) {
		src := []byte("export default [];\n")
		anchor, why, ok := Analyze(src)
		if !ok {
			t.Fatalf("Analyze() ok = false, why = %q", why)
		}
		if got := src[anchor.LayerAt-1]; got != '[' {
			t.Errorf("LayerAt = %d, want the byte just after '[' (got byte %q before it)", anchor.LayerAt, got)
		}
	})
}

// TestCommentRunPrecedingTheAnchorIsNotOrphaned pins the comment-run scan:
// a comment immediately above the anchor element, with no blank line
// between them, moves with it. A comment separated by a blank line is a
// section header and stays above dharness's inserted region.
func TestCommentRunPrecedingTheAnchorIsNotOrphaned(t *testing.T) {
	t.Run("comment directly above the element is not orphaned", func(t *testing.T) {
		src := []byte("export default [\n  // keep this rule\n  { rules: {} },\n];\n")
		anchor, why, ok := Analyze(src)
		if !ok {
			t.Fatalf("Analyze() ok = false, why = %q", why)
		}
		rest := string(src[anchor.LayerAt:])
		if !strings.HasPrefix(strings.TrimLeft(rest, " \t"), "// keep this rule") {
			t.Errorf("LayerAt landed after the comment, not before it:\nrest = %q", rest)
		}
	})

	t.Run("comment separated by a blank line is a section header and stays above", func(t *testing.T) {
		src := []byte("export default [\n  // section header\n\n  { rules: {} },\n];\n")
		anchor, why, ok := Analyze(src)
		if !ok {
			t.Fatalf("Analyze() ok = false, why = %q", why)
		}
		rest := string(src[anchor.LayerAt:])
		if strings.Contains(rest, "section header") {
			t.Errorf("LayerAt swallowed the section-header comment across a blank line:\nrest = %q", rest)
		}
		if !strings.HasPrefix(strings.TrimLeft(rest, " \t"), "{ rules: {} },") {
			t.Errorf("LayerAt does not land at the element after the blank line:\nrest = %q", rest)
		}
	})
}

// TestSpliceInsertsAndChangesNothingElse is the byte-identity assertion a
// golden .expected.js file alone cannot make: the output IS the input with
// one contiguous run inserted, not a re-render that happens to look the
// same.
func TestSpliceInsertsAndChangesNothingElse(t *testing.T) {
	src := []byte("export default [\n  { rules: {} },\n];\n")
	region := "  dharnessLayer,\n"
	at := 18

	got := Splice(src, at, region)

	if len(got) != len(src)+len(region) {
		t.Fatalf("len(Splice()) = %d, want %d", len(got), len(src)+len(region))
	}
	if !bytes.Equal(got[:at], src[:at]) {
		t.Errorf("bytes before at changed:\ngot  %q\nwant %q", got[:at], src[:at])
	}
	if !bytes.Equal(got[at:at+len(region)], []byte(region)) {
		t.Errorf("inserted region = %q, want %q", got[at:at+len(region)], region)
	}
	if !bytes.Equal(got[at+len(region):], src[at:]) {
		t.Errorf("bytes after at changed:\ngot  %q\nwant %q", got[at+len(region):], src[at:])
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture %q: %v", name, err)
	}
	return raw
}

// TestCRLFIsMatchedNotNormalised builds the CRLF variant from the committed
// LF-only fixture at test time (Decision 11) — a committed CRLF fixture
// would be normalised to LF by .gitattributes on checkout, testing nothing.
func TestCRLFIsMatchedNotNormalised(t *testing.T) {
	lf := readFixture(t, "eslint.config.js")
	crlf := bytes.ReplaceAll(lf, []byte("\n"), []byte("\r\n"))

	anchor, why, ok := Analyze(crlf)
	if !ok {
		t.Fatalf("Analyze(crlf) ok = false, why = %q", why)
	}
	if anchor.LineEnding != "\r\n" {
		t.Errorf("LineEnding = %q, want \"\\r\\n\" for a CRLF file", anchor.LineEnding)
	}

	region := "  dharnessLayer,\r\n"
	got := Splice(crlf, anchor.LayerAt, region)

	wantCRLF := bytes.Count(crlf, []byte("\r\n")) + strings.Count(region, "\r\n")
	if gotCRLF := bytes.Count(got, []byte("\r\n")); gotCRLF != wantCRLF {
		t.Errorf("CRLF count after Splice() = %d, want %d — a line ending was normalised", gotCRLF, wantCRLF)
	}
	if !bytes.Equal(got[:anchor.LayerAt], crlf[:anchor.LayerAt]) {
		t.Error("Splice() changed bytes before the anchor on a CRLF file")
	}
}

// TestBOMSurvives asserts the byte-order mark at the start of the file
// survives both Analyze and Splice untouched.
func TestBOMSurvives(t *testing.T) {
	lf := readFixture(t, "eslint.config.js")
	bom := append([]byte{0xEF, 0xBB, 0xBF}, lf...)

	anchor, why, ok := Analyze(bom)
	if !ok {
		t.Fatalf("Analyze(bom) ok = false, why = %q", why)
	}

	got := Splice(bom, anchor.LayerAt, "  dharnessLayer,\n")
	if !bytes.HasPrefix(got, []byte{0xEF, 0xBB, 0xBF}) {
		t.Error("Splice() output lost the BOM")
	}
}

// TestImportAtPrecedesLayerAtSoTheLaterOffsetSplicesFirst is the mutation
// guard for Decision 6's splice order: ImportAt < LayerAt always, so a
// caller must splice the layer region first — the later offset — or the
// import insertion shifts every byte after it and the layer region lands in
// the wrong place. This constructs both orders and proves they diverge, so
// a mutant that breaks the ordering (or the offsets themselves) is killed
// rather than merely covered.
func TestImportAtPrecedesLayerAtSoTheLaterOffsetSplicesFirst(t *testing.T) {
	src := readFixture(t, "eslint.config.js")

	anchor, why, ok := Analyze(src)
	if !ok {
		t.Fatalf("Analyze() ok = false, why = %q", why)
	}
	if anchor.ImportAt >= anchor.LayerAt {
		t.Fatalf("ImportAt (%d) must be less than LayerAt (%d)", anchor.ImportAt, anchor.LayerAt)
	}

	importRegion := "import dharnessPlugin from \"dharness-eslint-plugin\";\n"
	layerRegion := "  dharnessLayer,\n"

	correct := Splice(Splice(src, anchor.LayerAt, layerRegion), anchor.ImportAt, importRegion)
	wrong := Splice(Splice(src, anchor.ImportAt, importRegion), anchor.LayerAt, layerRegion)

	if bytes.Equal(correct, wrong) {
		t.Fatal("both splice orders produced identical bytes — this fixture does not exercise ordering")
	}
	if !strings.Contains(string(correct), layerRegion+"  ...next,") {
		t.Errorf("correct order (later offset first) did not place the layer region immediately before the anchor element:\n%s", correct)
	}
	if strings.Contains(string(wrong), layerRegion+"  ...next,") {
		t.Error("wrong order (earlier offset first) still landed correctly — this fixture does not distinguish the two orders")
	}
}

// TestSpliceHandlesARegionLongerThanTheSource pins that Splice's capacity
// hint to make() never constrains correctness: growing into a source
// shorter than the inserted region (the empty-file, first-ever-splice case)
// must not panic and must still produce the exact byte identity.
func TestSpliceHandlesARegionLongerThanTheSource(t *testing.T) {
	src := []byte("")
	region := "a region longer than the empty source it is spliced into"

	got := Splice(src, 0, region)

	if string(got) != region {
		t.Errorf("Splice(\"\", 0, region) = %q, want %q", got, region)
	}
}

// TestStartOfNextLineFindsTheByteAfterTheNextNewline pins startOfNextLine's
// exact arithmetic directly — the return value must be pos + i + 1, not an
// approximation that happens to satisfy a looser check.
func TestStartOfNextLineFindsTheByteAfterTheNextNewline(t *testing.T) {
	cases := []struct {
		name string
		src  string
		pos  int
		want int
	}{
		{"newline immediately follows pos", "ab\ncd", 2, 3},
		{"newline several bytes after pos", "abcdef\nghi", 2, 7},
		{"no further newline returns len(src)", "abcdef", 3, 6},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := startOfNextLine([]byte(c.src), c.pos); got != c.want {
				t.Errorf("startOfNextLine(%q, %d) = %d, want %d", c.src, c.pos, got, c.want)
			}
		})
	}
}

// TestIndentOfLineCopiesLeadingWhitespaceVerbatim pins indentOfLine's loop
// boundary and character class directly, including the case where the
// whitespace run reaches end-of-source with no trailing non-whitespace
// byte — the boundary a relaxed "<=" comparison reads out of bounds.
func TestIndentOfLineCopiesLeadingWhitespaceVerbatim(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"no leading whitespace", "foo", ""},
		{"spaces", "   foo", "   "},
		{"tabs", "\t\tfoo", "\t\t"},
		{"mixed spaces and tabs", " \tfoo", " \t"},
		{"whitespace runs to the end of source", "   ", "   "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := indentOfLine([]byte(c.src), 0); got != c.want {
				t.Errorf("indentOfLine(%q, 0) = %q, want %q", c.src, got, c.want)
			}
		})
	}
}

// TestLineEndingAtReadsTheAnchorLinesOwnEnding pins lineEndingAt's exact
// boundary on nl, including nl == 0 — a newline as the very first byte of
// the file — where a relaxed ">=" reads src[-1] out of bounds instead of
// skipping the CRLF check the way ">" does.
func TestLineEndingAtReadsTheAnchorLinesOwnEnding(t *testing.T) {
	cases := []struct {
		name string
		src  string
		from int
		want string
	}{
		{"LF, preceding byte is not \\r", "abc\ndef", 0, "\n"},
		{"CRLF at nl == 1", "\r\nabc", 0, "\r\n"},
		{"newline is the file's very first byte (nl == 0)", "\nabc", 0, "\n"},
		{"no further newline defaults to LF", "abc", 0, "\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := lineEndingAt([]byte(c.src), c.from); got != c.want {
				t.Errorf("lineEndingAt(%q, %d) = %q, want %q", c.src, c.from, got, c.want)
			}
		})
	}
}

// TestImportAtIsTheStartOfTheLineAfterTheLastImport pins the exact ImportAt
// value on a file with exactly one import — a loop that starts at index 1
// instead of 0, or never runs at all, would fall through to the no-import
// branch and land at byte 0 instead.
func TestImportAtIsTheStartOfTheLineAfterTheLastImport(t *testing.T) {
	src := []byte("import next from \"eslint-config-next\";\nexport default [\n  ...next,\n];\n")
	want := "import next from \"eslint-config-next\";\n"

	anchor, why, ok := Analyze(src)
	if !ok {
		t.Fatalf("Analyze() ok = false, why = %q", why)
	}
	if got := string(src[:anchor.ImportAt]); got != want {
		t.Errorf("ImportAt = %d, bytes before it = %q, want %q", anchor.ImportAt, got, want)
	}
}

// TestImportAtSkipsTheDirectivePrologueWhenThereIsNoImport pins the exact
// ImportAt value when the file has no import statement at all but opens
// with a "use strict"-style prologue: ImportAt must land after the
// prologue, not at byte 0.
func TestImportAtSkipsTheDirectivePrologueWhenThereIsNoImport(t *testing.T) {
	src := []byte("\"use strict\";\nexport default [];\n")
	want := "\"use strict\";\n"

	anchor, why, ok := Analyze(src)
	if !ok {
		t.Fatalf("Analyze() ok = false, why = %q", why)
	}
	if got := string(src[:anchor.ImportAt]); got != want {
		t.Errorf("ImportAt = %d, bytes before it = %q, want %q", anchor.ImportAt, got, want)
	}
}

// TestImportAtStopsAtTheFirstNonPrologueStatement pins that the
// prologue-skip loop stops at the first statement that is not a directive,
// rather than continuing past it to a later string-only expression
// statement that only happens to have the same shape.
func TestImportAtStopsAtTheFirstNonPrologueStatement(t *testing.T) {
	src := []byte("\"use strict\";\nconst x = 1;\n\"gotcha\";\nexport default [];\n")
	want := "\"use strict\";\n"

	anchor, why, ok := Analyze(src)
	if !ok {
		t.Fatalf("Analyze() ok = false, why = %q", why)
	}
	if got := string(src[:anchor.ImportAt]); got != want {
		t.Errorf("ImportAt = %d, bytes before it = %q, want %q (must stop at the const declaration, not swallow past it to \"gotcha\")", anchor.ImportAt, got, want)
	}
}

// TestImportTableScansPastNonImportAndSideEffectStatements pins that the
// import scan does not abort early: a leading non-import statement and a
// side-effect-only import (which carries no clause) must not stop the scan
// before it reaches the real "eslint/config" import that follows.
func TestImportTableScansPastNonImportAndSideEffectStatements(t *testing.T) {
	src := []byte("\"use strict\";\n" +
		"import \"polyfill\";\n" +
		"import { defineConfig } from \"eslint/config\";\n" +
		"export default defineConfig([{ rules: {} }]);\n")

	_, why, ok := Analyze(src)
	if !ok {
		t.Fatalf("Analyze() ok = false, why = %q — the scan must reach the eslint/config import past the leading prologue and side-effect import", why)
	}
}

// TestStringValueReadsTheFragmentOrEmptyWhenThereIsNone pins stringValue's
// exact loop boundary directly: an empty string literal gives the grammar
// zero named children — no string_fragment at all — so the loop must stop
// at NamedChildCount() rather than read one past it.
func TestStringValueReadsTheFragmentOrEmptyWhenThereIsNone(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"non-empty specifier", "import x from \"eslint/config\";\n", "eslint/config"},
		{"empty specifier has no string_fragment child", "import x from \"\";\n", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			parser := gotreesitter.NewParser(jsLanguage)
			tree, err := parser.Parse([]byte(c.src))
			if err != nil {
				t.Fatalf("Parse() = %v", err)
			}
			stmt := tree.RootNode().NamedChild(0)
			source := stmt.ChildByFieldName("source", jsLanguage)

			if got := stringValue(source, []byte(c.src)); got != c.want {
				t.Errorf("stringValue() = %q, want %q", got, c.want)
			}
		})
	}
}

// TestAnalyzeRecognisesExpoGeneratedConfig is the shape `npx expo lint`
// writes, verbatim from Expo's own guide: CommonJS, defineConfig required
// rather than imported, module.exports rather than export default. Before
// CommonJS support this returned "no default export found" and dharness
// delegated — so an Expo project, one of the two frameworks it has a preset
// for, never got its layer wired.
func TestAnalyzeRecognisesExpoGeneratedConfig(t *testing.T) {
	src := []byte(`const { defineConfig } = require('eslint/config');
const expoConfig = require('eslint-config-expo/flat');

module.exports = defineConfig([
  expoConfig,
  {
    ignores: ['dist/*'],
  },
]);
`)

	anchor, why, ok := Analyze(src)
	if !ok {
		t.Fatalf("Analyze() refused Expo's own generated config: %s", why)
	}
	if got := string(src[anchor.LayerAt : anchor.LayerAt+len(anchor.Indent)+len("expoConfig")]); got != anchor.Indent+"expoConfig" {
		t.Errorf("LayerAt does not anchor on the array's first element, got %q", got)
	}
	if ModuleOf(src) != CommonJS {
		t.Error("ModuleOf() = ESM for a require/module.exports config")
	}
}

// TestAnalyzeRecognisesAPlainCommonJSArray triangulates the case above
// without defineConfig: module.exports assigned an array literal directly.
func TestAnalyzeRecognisesAPlainCommonJSArray(t *testing.T) {
	src := []byte("const expoConfig = require('eslint-config-expo/flat');\n\nmodule.exports = [\n  expoConfig,\n];\n")

	if _, why, ok := Analyze(src); !ok {
		t.Fatalf("Analyze() refused a plain CommonJS array export: %s", why)
	}
	if ModuleOf(src) != CommonJS {
		t.Error("ModuleOf() = ESM for a module.exports array")
	}
}

// TestImportAtFollowsTheLastRequire pins where a spliced declaration lands
// in a CommonJS config. Anchoring at the top of the file instead would put
// the new require above the existing ones and, in a file that opens with a
// directive prologue, above "use strict" as well.
func TestImportAtFollowsTheLastRequire(t *testing.T) {
	src := []byte("'use strict';\nconst a = require('one');\nconst b = require('two');\n\nmodule.exports = [a, b];\n")

	anchor, why, ok := Analyze(src)
	if !ok {
		t.Fatalf("Analyze() = %s", why)
	}
	if got := string(src[:anchor.ImportAt]); got != "'use strict';\nconst a = require('one');\nconst b = require('two');\n" {
		t.Errorf("ImportAt does not follow the last require, everything before it is %q", got)
	}
}

// TestExportsAloneIsNotModuleExports pins the boundary moduleExportsValue
// draws. `exports = [...]` does not replace the module's exports in Node, so
// ESLint never reads it; recognising it would splice into a value that has
// no effect and report success.
func TestExportsAloneIsNotModuleExports(t *testing.T) {
	src := []byte("exports = [\n  {},\n];\n")

	if _, _, ok := Analyze(src); ok {
		t.Error("Analyze() accepted a bare `exports =` assignment, which Node ignores")
	}
}

// TestAMemberExpressionEndingInExportsIsNotModuleExports triangulates the
// same guard from the other side: a project's own `config.exports = [...]`
// is not the module's export.
func TestAMemberExpressionEndingInExportsIsNotModuleExports(t *testing.T) {
	src := []byte("const config = {};\nconfig.exports = [\n  {},\n];\n")

	if _, _, ok := Analyze(src); ok {
		t.Error("Analyze() accepted config.exports as the module's export")
	}
}

// TestDefineConfigIsRecognisedThroughAnAliasedRequire pins that recognition
// is by module specifier and not by spelling, in the CommonJS dialect too:
// a destructuring rename still resolves to "eslint/config".
func TestDefineConfigIsRecognisedThroughAnAliasedRequire(t *testing.T) {
	src := []byte("const { defineConfig: dc } = require('eslint/config');\n\nmodule.exports = dc([\n  {},\n]);\n")

	if _, why, ok := Analyze(src); !ok {
		t.Fatalf("Analyze() refused an aliased defineConfig require: %s", why)
	}
}

// TestARequireFromElsewhereIsNotDefineConfig is the refusal that keeps the
// rule "recognition is by specifier": a locally-defined helper of the same
// name is not eslint/config's defineConfig.
func TestARequireFromElsewhereIsNotDefineConfig(t *testing.T) {
	src := []byte("const { defineConfig } = require('./my-helpers');\n\nmodule.exports = defineConfig([\n  {},\n]);\n")

	if _, _, ok := Analyze(src); ok {
		t.Error("Analyze() accepted a defineConfig imported from somewhere other than eslint/config")
	}
}

// TestModuleOfDefaultsToESM pins the answer for a file this package does not
// recognise at all — the dialect dharness writes when it creates the config
// itself, so the created file and the owned file it loads agree.
func TestModuleOfDefaultsToESM(t *testing.T) {
	if ModuleOf([]byte("")) != ESM {
		t.Error("ModuleOf() of an empty file is not ESM")
	}
}

// TestModuleOfReadsTheExportNotTheImport pins which signal decides when a
// file carries both. A config that requires something but says `export
// default` is ESM: the export form is what Node's loader has already
// settled by the time either statement runs.
func TestModuleOfReadsTheExportNotTheImport(t *testing.T) {
	src := []byte("const a = require('one');\n\nexport default [a];\n")
	if ModuleOf(src) != ESM {
		t.Error("ModuleOf() let a require outvote an ESM export")
	}
}
