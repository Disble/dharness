package jsconfig

import (
	"strings"
	"testing"
)

// sliceAt returns src[start:end] and fails the owning scenario when the
// offsets are not a slice of src. The offsets are the whole contract, so an
// inverted or out-of-range pair has to fail loudly instead of panicking.
func sliceAt(t *testing.T, src string, start, end int) string {
	t.Helper()
	if start < 0 || end < start || end > len(src) {
		t.Fatalf("span [%d,%d) is not a slice of a %d-byte source", start, end, len(src))
	}
	return src[start:end]
}

// indexOf returns where want appears in src, failing when the fixture does
// not contain it — expected offsets are derived from the fixture text, not
// from the implementation's parsing of it.
func indexOf(t *testing.T, src, want string) int {
	t.Helper()
	at := strings.Index(src, want)
	if at < 0 {
		t.Fatalf("fixture does not contain %q", want)
	}
	return at
}

// TestAnalyzeExportReportsRawValueAndStatementBounds pins the two boundary
// pairs the caller splices with: the exported value exactly as the consumer
// wrote it, and the whole statement that carries it. Both dialects and every
// shape a Vitest config is written in are covered — object literal,
// defineConfig call, arrow function, function, TypeScript satisfies
// expression, and CommonJS module.exports.
func TestAnalyzeExportReportsRawValueAndStatementBounds(t *testing.T) {
	cases := []struct {
		name          string
		src           string
		typescript    bool
		wantValue     string
		wantStatement string
		wantDialect   Module
	}{
		{
			name:          "esm object literal",
			src:           "export default {\n  test: {},\n};\n",
			wantValue:     "{\n  test: {},\n}",
			wantStatement: "export default {\n  test: {},\n};",
			wantDialect:   ESM,
		},
		{
			name:          "esm defineConfig call",
			src:           "import { defineConfig } from \"vitest/config\";\nexport default defineConfig({\n  test: {},\n});\n",
			wantValue:     "defineConfig({\n  test: {},\n})",
			wantStatement: "export default defineConfig({\n  test: {},\n});",
			wantDialect:   ESM,
		},
		{
			name:          "esm arrow function",
			src:           "export default () => ({ test: {} });\n",
			wantValue:     "() => ({ test: {} })",
			wantStatement: "export default () => ({ test: {} });",
			wantDialect:   ESM,
		},
		{
			name:          "esm anonymous function expression",
			src:           "export default function () {\n  return { test: {} };\n}\n",
			wantValue:     "function () {\n  return { test: {} };\n}",
			wantStatement: "export default function () {\n  return { test: {} };\n}",
			wantDialect:   ESM,
		},
		{
			name:          "esm async function declaration",
			src:           "export default async function config() {\n  return { test: {} };\n}\n",
			wantValue:     "async function config() {\n  return { test: {} };\n}",
			wantStatement: "export default async function config() {\n  return { test: {} };\n}",
			wantDialect:   ESM,
		},
		{
			name:          "esm class declaration stays inside the value",
			src:           "export default class Config {\n}\n",
			wantValue:     "class Config {\n}",
			wantStatement: "export default class Config {\n}",
			wantDialect:   ESM,
		},
		{
			name:          "typescript satisfies expression",
			src:           "export default {\n  test: {},\n} satisfies UserConfig;\n",
			typescript:    true,
			wantValue:     "{\n  test: {},\n} satisfies UserConfig",
			wantStatement: "export default {\n  test: {},\n} satisfies UserConfig;",
			wantDialect:   ESM,
		},
		{
			name:          "esm default export after a named export",
			src:           "export const helper = 1;\nexport default {\n  test: {},\n};\n",
			wantValue:     "{\n  test: {},\n}",
			wantStatement: "export default {\n  test: {},\n};",
			wantDialect:   ESM,
		},
		{
			name:          "commonjs object literal",
			src:           "module.exports = {\n  test: {},\n};\n",
			wantValue:     "{\n  test: {},\n}",
			wantStatement: "module.exports = {\n  test: {},\n};",
			wantDialect:   CommonJS,
		},
		{
			name:          "commonjs defineConfig call",
			src:           "const { defineConfig } = require(\"vitest/config\");\nmodule.exports = defineConfig({ test: {} });\n",
			wantValue:     "defineConfig({ test: {} })",
			wantStatement: "module.exports = defineConfig({ test: {} });",
			wantDialect:   CommonJS,
		},
		{
			name:          "commonjs named function expression",
			src:           "module.exports = function config() {\n  return { test: {} };\n};\n",
			wantValue:     "function config() {\n  return { test: {} };\n}",
			wantStatement: "module.exports = function config() {\n  return { test: {} };\n};",
			wantDialect:   CommonJS,
		},
		{
			name:          "commonjs export after an unrelated statement",
			src:           "prepare();\nmodule.exports = { test: {} };\n",
			wantValue:     "{ test: {} }",
			wantStatement: "module.exports = { test: {} };",
			wantDialect:   CommonJS,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			span, why, ok := AnalyzeExport([]byte(tc.src), tc.typescript)
			if !ok {
				t.Fatalf("AnalyzeExport() ok = false, why = %q, want ok = true", why)
			}
			if why != "" {
				t.Errorf("AnalyzeExport() why = %q, want empty on ok = true", why)
			}
			if got := sliceAt(t, tc.src, span.ValueStart, span.ValueEnd); got != tc.wantValue {
				t.Errorf("value span = %q, want %q", got, tc.wantValue)
			}
			if got := sliceAt(t, tc.src, span.StatementStart, span.StatementEnd); got != tc.wantStatement {
				t.Errorf("statement span = %q, want %q", got, tc.wantStatement)
			}
			if span.Dialect != tc.wantDialect {
				t.Errorf("Dialect = %v, want %v", span.Dialect, tc.wantDialect)
			}
		})
	}
}

// TestAnalyzeExportReportsTheImportInsertionBoundary pins ImportAt: the
// line after the last existing import or require, the line after a
// directive prologue when the file only opens with one, and the start of the
// file when neither exists. It also pins what does not move the boundary — a
// call that merely looks like a require, a statement that arrives later than
// the last require, and a string statement that is not a leading prologue.
func TestAnalyzeExportReportsTheImportInsertionBoundary(t *testing.T) {
	cases := []struct {
		name       string
		src        string
		typescript bool
		after      string
	}{
		{
			name:  "no imports: the file start is the boundary",
			src:   "export default {\n  test: {},\n};\n",
			after: "",
		},
		{
			name:  "after the last esm import line",
			src:   "import { defineConfig } from \"vitest/config\";\nexport default defineConfig({\n  test: {},\n});\n",
			after: "export default",
		},
		{
			name:  "after the last require line",
			src:   "const { defineConfig } = require(\"vitest/config\");\nmodule.exports = defineConfig({ test: {} });\n",
			after: "module.exports",
		},
		{
			name:  "after a directive prologue and a require",
			src:   "\"use strict\";\nconst { defineConfig } = require(\"vitest/config\");\nmodule.exports = defineConfig({ test: {} });\n",
			after: "module.exports",
		},
		{
			name:  "after a directive prologue with no imports",
			src:   "\"use strict\";\nexport default {\n  test: {},\n};\n",
			after: "export default",
		},
		{
			name:  "after a later require line",
			src:   "const first = require(\"./first.js\");\nconst { defineConfig } = require(\"vitest/config\");\nmodule.exports = defineConfig({ test: {} });\n",
			after: "module.exports",
		},
		{
			name:  "after a var require line",
			src:   "var { defineConfig } = require(\"vitest/config\");\nmodule.exports = defineConfig({ test: {} });\n",
			after: "module.exports",
		},
		{
			name:  "after a require declared beside a declarator without an initializer",
			src:   "let pending, cfg = require(\"vitest/config\");\nmodule.exports = cfg;\n",
			after: "module.exports",
		},
		{
			name:  "a later statement that only looks like a require does not move the boundary",
			src:   "const { defineConfig } = require(\"vitest/config\");\nconst pkg = JSON.parse(readFileSync(\"package.json\"));\nmodule.exports = defineConfig({ test: {} });\n",
			after: "const pkg = JSON.parse",
		},
		{
			name:  "a require behind a member expression is not an import binding",
			src:   "const cfg = pkg.require(\"vitest/config\");\nmodule.exports = { test: {} };\n",
			after: "",
		},
		{
			name:  "a require call with no argument is not an import binding",
			src:   "const cfg = require();\nmodule.exports = { test: {} };\n",
			after: "",
		},
		{
			name:  "a require call with several arguments is not an import binding",
			src:   "const cfg = require(\"vitest/config\", \"extra\");\nmodule.exports = { test: {} };\n",
			after: "",
		},
		{
			name:  "a require call with a computed specifier is not an import binding",
			src:   "const cfg = require(specifier);\nmodule.exports = { test: {} };\n",
			after: "",
		},
		{
			name:  "a directive prologue ends at the first statement that is not one",
			src:   "\"use strict\";\nconst helper = 1;\nexport default {\n  test: {},\n};\n",
			after: "const helper",
		},
		{
			name:  "a string statement after the export is not a leading prologue",
			src:   "export default {\n  test: {},\n};\n\"use strict\";\n",
			after: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := 0
			if tc.after != "" {
				want = indexOf(t, tc.src, tc.after)
			}
			span, why, ok := AnalyzeExport([]byte(tc.src), tc.typescript)
			if !ok {
				t.Fatalf("AnalyzeExport() ok = false, why = %q, want ok = true", why)
			}
			if span.ImportAt != want {
				t.Errorf("ImportAt = %d, want %d", span.ImportAt, want)
			}
		})
	}
}

// TestAnalyzeExportPreservesNamedDefaultFunctionBinding pins
// DefaultFunctionName to the one case that needs it: a named default
// function declaration binds a module-local name that a caller rewriting the
// statement has to keep. Anonymous functions, function expressions — whose
// name is visible only inside themselves — and class declarations carry no
// such name through this field.
func TestAnalyzeExportPreservesNamedDefaultFunctionBinding(t *testing.T) {
	cases := []struct {
		name       string
		src        string
		typescript bool
		wantName   string
	}{
		{
			name:     "named function declaration",
			src:      "export default function config() {\n  return { test: {} };\n}\n",
			wantName: "config",
		},
		{
			name:     "async named function declaration",
			src:      "export default async function config() {\n  return { test: {} };\n}\n",
			wantName: "config",
		},
		{
			name:     "named generator declaration",
			src:      "export default function* config() {\n  yield { test: {} };\n}\n",
			wantName: "config",
		},
		{
			name:     "anonymous function expression has no local binding",
			src:      "export default function () {\n  return { test: {} };\n}\n",
			wantName: "",
		},
		{
			name:     "commonjs function expression name is not module-scope",
			src:      "module.exports = function config() {\n  return { test: {} };\n};\n",
			wantName: "",
		},
		{
			name:     "class declaration keeps its name inside the value span",
			src:      "export default class Config {\n}\n",
			wantName: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			span, why, ok := AnalyzeExport([]byte(tc.src), tc.typescript)
			if !ok {
				t.Fatalf("AnalyzeExport() ok = false, why = %q, want ok = true", why)
			}
			if span.DefaultFunctionName != tc.wantName {
				t.Errorf("DefaultFunctionName = %q, want %q", span.DefaultFunctionName, tc.wantName)
			}
		})
	}
}

// TestAnalyzeExportKeepsIdentifierExportRaw pins the boundary that must not
// be repurposed: when the export names a binding, the span is the
// identifier's own bytes, never the initializer that binding was declared
// with. The caller resolves that hop, because it is the caller that decides
// what its own declaration has to keep.
func TestAnalyzeExportKeepsIdentifierExportRaw(t *testing.T) {
	src := "const config = { test: {} };\nexport default config;\n"

	span, why, ok := AnalyzeExport([]byte(src), false)
	if !ok {
		t.Fatalf("AnalyzeExport() ok = false, why = %q, want ok = true", why)
	}
	if got := sliceAt(t, src, span.ValueStart, span.ValueEnd); got != "config" {
		t.Errorf("value span = %q, want %q — the raw identifier, not its initializer", got, "config")
	}

	initializer := indexOf(t, src, "{ test: {} }")
	if span.ValueStart <= initializer && initializer < span.ValueEnd {
		t.Errorf("value span [%d,%d) covers the initializer at %d, want the identifier only", span.ValueStart, span.ValueEnd, initializer)
	}

	if span.Dialect != ESM {
		t.Errorf("Dialect = %v, want ESM", span.Dialect)
	}
	if got := sliceAt(t, src, span.StatementStart, span.StatementEnd); got != "export default config;" {
		t.Errorf("statement span = %q, want %q", got, "export default config;")
	}
}

// TestAnalyzeExportLeavesESLintResolvedInitializerAlone pins that the new
// API is additive. The ESLint path still resolves a const-bound default
// export down to its initializer and anchors inside the array, exactly as it
// did before; the export-span API answers the same file with the raw
// identifier, because a Vitest splice is the caller's decision and not this
// package's.
func TestAnalyzeExportLeavesESLintResolvedInitializerAlone(t *testing.T) {
	src := "import { defineConfig } from \"eslint/config\";\n" +
		"const eslintConfig = defineConfig([\n" +
		"  { rules: {} },\n" +
		"]);\n" +
		"export default eslintConfig;\n"

	anchor, why, ok := Analyze([]byte(src))
	if !ok {
		t.Fatalf("Analyze() ok = false, why = %q, want ok = true", why)
	}
	if want := indexOf(t, src, "  { rules: {} },"); anchor.LayerAt != want {
		t.Errorf("Analyze() LayerAt = %d, want %d — the layer still lands on the array's first element", anchor.LayerAt, want)
	}
	if want := indexOf(t, src, "const eslintConfig"); anchor.ImportAt != want {
		t.Errorf("Analyze() ImportAt = %d, want %d", anchor.ImportAt, want)
	}

	span, why, ok := AnalyzeExport([]byte(src), false)
	if !ok {
		t.Fatalf("AnalyzeExport() ok = false, why = %q, want ok = true", why)
	}
	if got := sliceAt(t, src, span.ValueStart, span.ValueEnd); got != "eslintConfig" {
		t.Errorf("AnalyzeExport() value span = %q, want %q", got, "eslintConfig")
	}
	if span.DefaultFunctionName != "" {
		t.Errorf("DefaultFunctionName = %q, want empty", span.DefaultFunctionName)
	}
}

// TestAnalyzeExportRefusesWithoutAnUnambiguousBoundary walks every shape
// that must refuse, and pins that the three refusal families — no export,
// more than one export, unparseable source — each say something different.
// A refusal always returns the zero span: a caller that splices with half an
// answer is the failure this contract exists to prevent.
func TestAnalyzeExportRefusesWithoutAnUnambiguousBoundary(t *testing.T) {
	families := []string{"no export", "more than one export", "unparseable source"}

	cases := []struct {
		name       string
		src        string
		typescript bool
		cell       string
	}{
		{name: "empty file", src: "", cell: "no export"},
		{name: "named export only", src: "export const config = { test: {} };\n", cell: "no export"},
		{name: "named function export only", src: "export function config() { return { test: {} }; }\n", cell: "no export"},
		{name: "re-exported default from another module", src: "export { default } from \"./vitest.shared.js\";\n", cell: "no export"},
		{name: "default alias export statement", src: "const a = { test: {} };\nexport { a as default };\n", cell: "no export"},
		{name: "namespace re-export as default", src: "export * as default from \"./vitest.shared.js\";\n", cell: "no export"},
		{name: "exports.default is not module.exports", src: "exports.default = { test: {} };\n", cell: "no export"},
		{name: "bare exports assignment is not module.exports", src: "exports = { test: {} };\n", cell: "no export"},
		{name: "property assignment on module.exports is not the module export", src: "module.exports.default = { test: {} };\n", cell: "no export"},
		{name: "augmented assignment to module.exports is not an export", src: "module.exports += { test: {} };\n", cell: "no export"},
		{name: "duplicate default exports", src: "export default { a: 1 };\nexport default { b: 2 };\n", cell: "more than one export"},
		{name: "default export and module.exports", src: "module.exports = { a: 1 };\nexport default { b: 2 };\n", cell: "more than one export"},
		{name: "error inside the exported value", src: "export default { a: 1 + };\n", cell: "unparseable source"},
		{name: "error elsewhere in the file", src: "const broken = { a: };\nexport default { test: {} };\n", cell: "unparseable source"},
		{name: "typescript syntax read as javascript", src: "export default { test: {} } satisfies UserConfig;\n", cell: "unparseable source"},
	}

	reasons := map[string]string{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			span, why, ok := AnalyzeExport([]byte(tc.src), tc.typescript)
			if ok {
				t.Fatalf("AnalyzeExport() ok = true, want false for %q", tc.src)
			}
			if why == "" {
				t.Fatal("AnalyzeExport() why is empty on ok = false")
			}
			if span != (ExportSpan{}) {
				t.Errorf("AnalyzeExport() returned %+v with ok = false, want the zero span", span)
			}
			if prior, seen := reasons[tc.cell]; seen && prior != why {
				t.Errorf("cell %q reported %q here and %q earlier, want one reason per family", tc.cell, why, prior)
			}
			reasons[tc.cell] = why
		})
	}

	for _, family := range families {
		if reasons[family] == "" {
			t.Errorf("refusal family %q was never exercised", family)
		}
	}

	seen := map[string]string{}
	for cell, why := range reasons {
		if prior, dup := seen[why]; dup {
			t.Errorf("families %q and %q share the identical reason %q, want distinguishable reasons", prior, cell, why)
		}
		seen[why] = cell
	}
}
