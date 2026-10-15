package setup

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/jsconfig"
	"github.com/Disble/dharness/internal/project"
)

// coverageTiersSpliceRoot builds a disposable repository whose JS project
// root is either the repository itself or the "frontend" directory inside it
// — the two layouts the reference computation has to answer for — and returns
// the project, the repository root and the JS project directory.
func coverageTiersSpliceRoot(t *testing.T, split bool) (project.Project, string, string) {
	t.Helper()
	root := t.TempDir()
	source := root
	if split {
		source = filepath.Join(root, "frontend")
		if err := os.MkdirAll(source, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return project.Project{Root: root, Source: source}, root, source
}

// coverageTiersInputPayload returns the bytes between the input markers — the
// exported value this adapter promises to copy verbatim — and fails when the
// region is not exactly one well-formed pair.
func coverageTiersInputPayload(t *testing.T, raw []byte) []byte {
	t.Helper()
	region := locateCoverageTiersRegion(string(raw), coverageTiersInputBegin, coverageTiersInputEnd)
	if region.state != markersPresent {
		t.Fatalf("this config does not carry exactly one dharness:coverage-tiers-input region:\n%s", raw)
	}
	return raw[region.contentFrom:region.contentTo]
}

// coverageTiersRegionStates fails the scenario unless candidate carries
// exactly one well-formed region of every marker pair this adapter owns.
func coverageTiersRegionStates(t *testing.T, candidate []byte) {
	t.Helper()
	pairs := []struct {
		label string
		begin string
		end   string
	}{
		{"import", coverageTiersImportBegin, coverageTiersImportEnd},
		{"export", coverageTiersExportBegin, coverageTiersExportEnd},
		{"input", coverageTiersInputBegin, coverageTiersInputEnd},
	}
	for _, pair := range pairs {
		if region := locateCoverageTiersRegion(string(candidate), pair.begin, pair.end); region.state != markersPresent {
			t.Errorf("the resulting config does not carry exactly one well-formed dharness:coverage-tiers-%s region:\n%s", pair.label, candidate)
		}
	}
}

// coverageTiersMarkersStartLines fails the scenario unless every managed
// begin marker starts a line of its own. A marker sharing its line with the
// bytes before it puts those bytes inside the region's line-bounded span, and
// the next run — which rewrites exactly that span — drops them, so this is the
// invariant the region's bounds depend on rather than a formatting preference.
func coverageTiersMarkersStartLines(t *testing.T, candidate []byte) {
	t.Helper()
	for _, marker := range []string{coverageTiersImportBegin, coverageTiersExportBegin, coverageTiersInputBegin} {
		at := bytes.Index(candidate, []byte(marker))
		if at < 0 {
			t.Errorf("the config does not carry the %q marker:\n%s", marker, candidate)
			continue
		}
		if at > 0 && candidate[at-1] != '\n' {
			t.Errorf("the managed marker %q shares its line with the bytes before it, which the next run would rewrite away:\n%s", marker, candidate)
		}
	}
}

// TestSpliceCoverageTiersWrapsEachExportShape is the owning scenario for the
// splice itself: every shape a Vitest config is written in keeps its exported
// value byte for byte inside the wrapper, keeps the project's own bytes
// outside the managed regions, and leaves exactly one well-formed region of
// each marker pair behind. It owns both layouts the reference computation has
// to answer for, and the CommonJS half of the reference rendering, which is
// why its cases carry the layout they are spliced in.
//
// It is also the owning scenario for the property the marker regions exist
// for: a converged config is a fixed point, so no run ever adds a second
// wrapper or a second region pair.
//
// The named-function case is the one where "wrap the value" is not enough: a
// default function declaration binds a module-scope name, so rewriting it as
// a call argument would turn a hoisted declaration into a name visible only
// inside its own expression. The assertions pin that the declaration stays a
// declaration and the exported call references it.
func TestSpliceCoverageTiersWrapsEachExportShape(t *testing.T) {
	const note = "// The project's own note, kept byte for byte.\n"
	const rootImport = `import { fileURLToPath } from "node:url";` + "\n" +
		`import { withCoverageTiers } from "./.dharness/coverage-tiers.mjs";`
	const rootOptions = `{ repositoryRoot: fileURLToPath(new URL(".", import.meta.url)), sourceRoot: "." }`
	const cjsImport = `const { withCoverageTiers } = require("./.dharness/coverage-tiers.mjs");`
	const cjsOptions = `{ repositoryRoot: __dirname, sourceRoot: "." }`

	cases := []struct {
		name        string
		file        string
		typescript  bool
		split       bool
		source      string
		value       string
		declaration string
		keep        []string
		absent      []string
		wantImport  string
		wantOptions string
		// wantExact, when set, is the whole spliced file. The rows that
		// carry it are the ones whose observable outcome is where the
		// managed bytes land relative to the consumer's own bytes on the
		// same line, and there a "contains" check cannot tell the
		// difference between the line ending the insert point needs and
		// an extra one.
		wantExact string
	}{
		{
			name:        "esm object literal",
			file:        "vitest.config.js",
			source:      "export default {\n  test: {},\n};\n",
			value:       "{\n  test: {},\n}",
			wantImport:  rootImport,
			wantOptions: rootOptions,
		},
		{
			name:       "esm defineConfig call after the project's own bytes",
			file:       "vitest.config.mjs",
			source:     note + "import { defineConfig } from \"vitest/config\";\n\nexport default defineConfig({\n  test: {},\n});\n",
			value:      "defineConfig({\n  test: {},\n})",
			keep:       []string{note, `import { defineConfig } from "vitest/config";`},
			wantImport: rootImport, wantOptions: rootOptions,
		},
		{
			name:   "esm identifier keeps the shared initializer untouched",
			file:   "vitest.config.js",
			source: "const config = defineConfig({ test: {} });\n\nexport default config;\n",
			value:  "config",
			// The initializer is deliberately not resolved: the wrapper wraps
			// the identifier the consumer exported, not the declaration it
			// names.
			keep:       []string{"const config = defineConfig({ test: {} });"},
			wantImport: rootImport, wantOptions: rootOptions,
		},
		{
			name:        "esm arrow function stays callable",
			file:        "vitest.config.ts",
			typescript:  true,
			source:      "export default () => ({ test: {} });\n",
			value:       "() => ({ test: {} })",
			wantImport:  rootImport,
			wantOptions: rootOptions,
		},
		{
			name:        "esm async function declaration keeps its module-scope binding",
			file:        "vitest.config.js",
			source:      "export default async function config() {\n  return { test: {} };\n}\n",
			value:       "async function config() {\n  return { test: {} };\n}",
			declaration: "config",
			wantImport:  rootImport,
			wantOptions: rootOptions,
		},
		{
			name:       "typescript satisfies expression",
			file:       "vitest.config.ts",
			typescript: true,
			source:     "import type { UserConfig } from \"vitest/config\";\n\nexport default {\n  test: {},\n} satisfies UserConfig;\n",
			value:      "{\n  test: {},\n} satisfies UserConfig",
			keep:       []string{`import type { UserConfig } from "vitest/config";`},
			wantImport: rootImport, wantOptions: rootOptions,
		},
		{
			name:       "esm import and export on one line",
			file:       "vitest.config.js",
			source:     "import { defineConfig } from \"vitest/config\"; export default defineConfig({ test: {} });\n",
			value:      "defineConfig({ test: {} })",
			keep:       []string{`import { defineConfig } from "vitest/config";`},
			wantImport: rootImport, wantOptions: rootOptions,
		},
		{
			// The last import ends the line before the one the export
			// statement starts, so the insert anchor is a line start and the
			// region needs no line ending of its own: it takes the export
			// statement's place exactly, with the consumer's own import line
			// untouched above it and no blank line inserted between them.
			name:       "esm export statement on the line after the last import",
			file:       "vitest.config.js",
			source:     "import { defineConfig } from \"vitest/config\";\nexport default defineConfig({ test: {} });\n",
			value:      "defineConfig({ test: {} })",
			keep:       []string{`import { defineConfig } from "vitest/config";`},
			wantImport: rootImport, wantOptions: rootOptions,
			wantExact: `import { defineConfig } from "vitest/config";` + "\n" +
				coverageTiersImportBegin + "\n" + rootImport + "\n" + coverageTiersImportEnd + "\n" +
				coverageTiersExportBegin + "\n" +
				"export default " + coverageTiersBinding + "(\n" +
				coverageTiersInputBegin + "\n" + "defineConfig({ test: {} })" + "\n" + coverageTiersInputEnd + "\n" +
				", " + rootOptions + ");\n" +
				coverageTiersExportEnd + "\n" + "\n",
		},
		{
			// The insert anchor is one byte into the file: a single space opens
			// the config and the export statement starts the line after the
			// shared declaration. The byte before the anchor is not a line
			// ending, so the region is rendered with one of its own; without it
			// the begin marker would share the file's first line with that
			// space, and the next run would read the space as the region's own
			// indent and rewrite every line of it.
			name:       "esm config whose first line opens with a space",
			file:       "vitest.config.js",
			source:     " const config = { test: {} };\nexport default config;\n",
			value:      "config",
			keep:       []string{"const config = { test: {} };"},
			wantImport: rootImport, wantOptions: rootOptions,
			wantExact: " \n" +
				coverageTiersImportBegin + "\n" + rootImport + "\n" + coverageTiersImportEnd + "\n" +
				"const config = { test: {} };\n" +
				coverageTiersExportBegin + "\n" +
				"export default " + coverageTiersBinding + "(\n" +
				coverageTiersInputBegin + "\n" + "config" + "\n" + coverageTiersInputEnd + "\n" +
				", " + rootOptions + ");\n" +
				coverageTiersExportEnd + "\n" + "\n",
		},
		{
			// The export statement shares its line with a statement the
			// consumer wrote before it, so the managed block cannot also take
			// the line start: the consumer's own bytes are there. The region is
			// moved onto a line of its own instead, and the consumer's line
			// keeps every byte it had. Leaving the begin marker glued to those
			// bytes would put them inside the region, so the next run — whose
			// bounds are line-bounded — would rewrite them away.
			name:       "esm export statement after another statement on the same line",
			file:       "vitest.config.js",
			source:     " const config = { test: {} }; export default config;\n",
			value:      "config",
			keep:       []string{"const config = { test: {} };"},
			wantImport: rootImport, wantOptions: rootOptions,
			wantExact: " \n" +
				coverageTiersImportBegin + "\n" + rootImport + "\n" + coverageTiersImportEnd + "\n" +
				"const config = { test: {} }; \n" +
				coverageTiersExportBegin + "\n" +
				"export default " + coverageTiersBinding + "(\n" +
				coverageTiersInputBegin + "\n" + "config" + "\n" + coverageTiersInputEnd + "\n" +
				", " + rootOptions + ");\n" +
				coverageTiersExportEnd + "\n" + "\n",
		},
		{
			// The declaration half of the same boundary. Here the body the
			// splice renders is the preserved declaration followed by the export
			// that references its module-scope binding, and the statement still
			// shares its line with the consumer's own: the same rule applies, so
			// the managed bytes take a line ending of their own, the declaration
			// stays a declaration and the consumer's line keeps its bytes.
			name:        "esm declaration export after another statement on the same line",
			file:        "vitest.config.js",
			source:      " const config = { test: {} }; export default async function setup() {\n  return { test: {} };\n}\n",
			value:       "async function setup() {\n  return { test: {} };\n}",
			declaration: "setup",
			keep:        []string{"const config = { test: {} };"},
			wantImport:  rootImport, wantOptions: rootOptions,
			wantExact: " \n" +
				coverageTiersImportBegin + "\n" + rootImport + "\n" + coverageTiersImportEnd + "\n" +
				"const config = { test: {} }; \n" +
				coverageTiersInputBegin + "\n" + "async function setup() {\n  return { test: {} };\n}" + "\n" + coverageTiersInputEnd + "\n" +
				coverageTiersExportBegin + "\n" +
				"export default " + coverageTiersBinding + "(setup, " + rootOptions + ");\n" +
				coverageTiersExportEnd + "\n" + "\n",
		},
		{
			name:        "esm defineConfig call in the split layout",
			file:        "vitest.config.ts",
			typescript:  true,
			split:       true,
			source:      "import { defineConfig } from \"vitest/config\";\n\nexport default defineConfig({ test: {} });\n",
			value:       "defineConfig({ test: {} })",
			keep:        []string{`import { defineConfig } from "vitest/config";`},
			wantImport:  `import { withCoverageTiers } from "../.dharness/coverage-tiers.mjs";`,
			wantOptions: `{ repositoryRoot: fileURLToPath(new URL("..", import.meta.url)), sourceRoot: "frontend" }`,
		},
		{
			name:        "commonjs object literal",
			file:        "vitest.config.cjs",
			source:      "module.exports = {\n  test: {},\n};\n",
			value:       "{\n  test: {},\n}",
			wantImport:  cjsImport,
			wantOptions: cjsOptions,
		},
		{
			name:       "commonjs defineConfig call",
			file:       "vitest.config.js",
			source:     "const { defineConfig } = require(\"vitest/config\");\n\nmodule.exports = defineConfig({ test: {} });\n",
			value:      "defineConfig({ test: {} })",
			keep:       []string{`const { defineConfig } = require("vitest/config");`},
			wantImport: cjsImport, wantOptions: cjsOptions,
		},
		{
			name:   "commonjs named function expression stays an expression",
			file:   "vitest.config.js",
			source: "module.exports = function config() {\n  return { test: {} };\n};\n",
			value:  "function config() {\n  return { test: {} };\n}",
			// No declaration shape: a function expression's own name is
			// visible only inside itself, so wrapping it loses no binding.
			wantImport: cjsImport, wantOptions: cjsOptions,
		},
		{
			// The CommonJS half of the reference rendering: require and
			// __dirname instead of an ESM import, still relative to the config
			// file. The fixture mentions fileURLToPath itself, because in
			// CommonJS that name is not dharness's binding — the repository
			// root is resolved through __dirname — and refusing it would be a
			// refusal nothing justifies.
			name:        "commonjs references anchored to the config in the split layout",
			file:        "vitest.config.cjs",
			split:       true,
			source:      "const { fileURLToPath } = require(\"node:url\");\n\nmodule.exports = { test: { root: fileURLToPath } };\n",
			value:       "{ test: { root: fileURLToPath } }",
			keep:        []string{`const { fileURLToPath } = require("node:url");`},
			absent:      []string{"import { fileURLToPath }"},
			wantImport:  `const { withCoverageTiers } = require("../.dharness/coverage-tiers.mjs");`,
			wantOptions: `{ repositoryRoot: require("node:path").resolve(__dirname, ".."), sourceRoot: "frontend" }`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, root, source := coverageTiersSpliceRoot(t, tc.split)
			path := filepath.Join(source, tc.file)

			candidate, err := spliceCoverageTiersConfig([]byte(tc.source), path, p)
			if err != nil {
				t.Fatalf("spliceCoverageTiersConfig() error = %v", err)
			}

			framed := coverageTiersInputBegin + "\n" + tc.value + "\n" + coverageTiersInputEnd
			if !bytes.Contains(candidate, []byte(framed)) {
				t.Errorf("the exported value is not preserved verbatim between the input markers:\n%s", candidate)
			}
			for _, kept := range tc.keep {
				if !bytes.Contains(candidate, []byte(kept)) {
					t.Errorf("the project's own bytes %q did not survive the splice:\n%s", kept, candidate)
				}
			}
			for _, absent := range tc.absent {
				if bytes.Contains(candidate, []byte(absent)) {
					t.Errorf("the spliced config carries bytes it must not, %q:\n%s", absent, candidate)
				}
			}

			if _, why, ok := jsconfig.AnalyzeExport(candidate, tc.typescript); !ok {
				t.Fatalf("the spliced config no longer exports a bounded value: %s\n%s", why, candidate)
			}
			coverageTiersRegionStates(t, candidate)
			coverageTiersMarkersStartLines(t, candidate)

			for _, want := range []string{tc.wantImport, tc.wantOptions} {
				if !bytes.Contains(candidate, []byte(want)) {
					t.Errorf("the spliced config does not contain %q:\n%s", want, candidate)
				}
			}
			if tc.wantExact != "" && string(candidate) != tc.wantExact {
				t.Errorf("the splice of this config is not the bytes it must be:\n got %q\nwant %q", candidate, tc.wantExact)
			}
			if bytes.Contains(candidate, []byte(root)) {
				t.Errorf("the spliced config stores this machine's absolute root %q:\n%s", root, candidate)
			}

			if tc.declaration != "" {
				if !bytes.Contains(candidate, []byte("async function "+tc.declaration+"(")) {
					t.Errorf("the function declaration is no longer a declaration:\n%s", candidate)
				}
				if !bytes.Contains(candidate, []byte("withCoverageTiers("+tc.declaration+", ")) {
					t.Errorf("the export does not reference the declaration's module-scope binding:\n%s", candidate)
				}
				if bytes.Contains(candidate, []byte("withCoverageTiers(\n")) {
					t.Errorf("the declaration was wrapped in place, which would scope its binding to the call:\n%s", candidate)
				}
			}

			// A converged config is a fixed point: repeated calls are
			// byte-identical and never add a second wrapper.
			current := candidate
			for call := 2; call <= 4; call++ {
				next, err := spliceCoverageTiersConfig(current, path, p)
				if err != nil {
					t.Fatalf("call %d: spliceCoverageTiersConfig() error = %v", call, err)
				}
				if !bytes.Equal(next, current) {
					t.Errorf("call %d changed a converged config:\n--- first ---\n%s\n--- call %d ---\n%s", call, candidate, call, next)
				}
				current = next
			}
			if got := strings.Count(string(current), coverageTiersBinding+"("); got != 1 {
				t.Errorf("the converged config wraps the value %d times, want 1:\n%s", got, current)
			}
			coverageTiersRegionStates(t, current)
			coverageTiersMarkersStartLines(t, current)

			// A hand edit appended to the export region's own last line is
			// inside the region the next run rewrites. The region's bounds are
			// line-bounded, and with the end marker's line carrying no line
			// ending of its own the region reaches the end of the file, so the
			// next run must hand back the converged bytes — not a config that
			// still carries the edit behind a region that stopped short of it.
			trimmed := bytes.TrimRight(current, "\n")
			endMarkerAt := bytes.Index(trimmed, []byte(coverageTiersExportEnd))
			if endMarkerAt < 0 || endMarkerAt+len(coverageTiersExportEnd) != len(trimmed) {
				t.Fatalf("this row's converged config does not end at its export end marker, so the hand edit below would land outside the region and prove nothing:\n%q", current)
			}
			handEdited := append(append([]byte(nil), trimmed...), []byte(" // hand edit")...)
			restored, err := spliceCoverageTiersConfig(handEdited, path, p)
			if err != nil {
				t.Fatalf("splicing a config whose export end marker line carries a hand edit: spliceCoverageTiersConfig() error = %v", err)
			}
			if wantRestored := append(append([]byte(nil), trimmed...), '\n'); !bytes.Equal(restored, wantRestored) {
				t.Errorf("the hand edit on the export region's own last line was not rewritten away:\n got %q\nwant %q", restored, wantRestored)
			}

			// A run rewrites the bytes between its own markers and nothing
			// else: a hand edit outside every region is left exactly as it was
			// found, which is what keeps dharness's ownership in a consumer's
			// file narrow enough to be safe.
			noted := append(append([]byte(nil), current...), []byte("// The project's own trailing note.\n")...)
			respliced, err := spliceCoverageTiersConfig(noted, path, p)
			if err != nil {
				t.Fatalf("splicing a config whose trailing note sits outside every region: spliceCoverageTiersConfig() error = %v", err)
			}
			if string(respliced) != string(noted) {
				t.Errorf("a later run rewrote bytes outside the managed regions:\n got %q\nwant %q", respliced, noted)
			}
		})
	}
}

// TestSpliceCoverageTiersRepairsOnlyItsOwnStaleReferences is the owning
// scenario for the repair path: a config that already carries dharness's
// regions is spliced again from a moved path, so the import specifier and the
// option expression change to match the config's new location, and the
// exported value dharness preserves verbatim does not.
//
// The declaration case is the other half of the same repair: the export region
// is rewritten and the declaration it references stays outside that region and
// byte-identical — the reason the value lives between its own markers rather
// than inside the region being replaced.
//
// The compacted case is where the repair path's containment rule is exercised
// at its boundary: an input region that shares its start, or its end, with the
// export region is still inside it, so the repair runs and restores the
// canonical lines rather than reporting a conflict that is not one.
func TestSpliceCoverageTiersRepairsOnlyItsOwnStaleReferences(t *testing.T) {
	cases := []struct {
		name   string
		raw    string
		want   []string
		absent []string
	}{
		{
			name: "an identifier export whose references moved",
			raw:  "// kept\nimport { defineConfig } from \"vitest/config\";\n\nexport default defineConfig({ test: {} });\n",
			want: []string{
				`"../../.dharness/coverage-tiers.mjs"`,
				`new URL("../..", import.meta.url)`,
				`sourceRoot: "frontend"`,
			},
			absent: []string{`"../.dharness/coverage-tiers.mjs"`},
		},
		{
			name: "a declaration binding whose references moved",
			raw:  "export default async function config() {\n  return { test: {} };\n}\n",
			want: []string{
				`"../../.dharness/coverage-tiers.mjs"`,
				"async function config(",
				"withCoverageTiers(config, ",
			},
			// Wrapping the declaration in place would turn a hoisted
			// declaration into a name visible only inside its own
			// expression, so the repair references the binding instead.
			absent: []string{"withCoverageTiers(\n"},
		},
		{
			// A hand edit that compacts the managed block onto fewer lines:
			// the export begin marker shares a line with the input begin
			// marker, and the input end marker shares a line with the export
			// end marker. A region whose start, or whose end, coincides with
			// the export region's is still contained by it, so the repair runs,
			// reads the value back out of its own markers, and hands back the
			// canonical marker lines.
			name: "a hand-compacted managed block whose regions share a line",
			raw: coverageTiersImportBegin + "\n" +
				`import { fileURLToPath } from "node:url";` + "\n" +
				`import { withCoverageTiers } from "../.dharness/coverage-tiers.mjs";` + "\n" +
				coverageTiersImportEnd + "\n" +
				coverageTiersExportBegin + " " + coverageTiersInputBegin + "\n" +
				"{ test: {} }\n" +
				coverageTiersInputEnd + " " + coverageTiersExportEnd + "\n" +
				"\n",
			want: []string{
				`"../../.dharness/coverage-tiers.mjs"`,
				`new URL("../..", import.meta.url)`,
				`sourceRoot: "frontend"`,
				coverageTiersExportBegin + "\n",
				coverageTiersInputBegin + "\n",
				coverageTiersInputEnd + "\n",
			},
			// The compacted lines are gone: each marker ends its own line
			// again, which is the shape the next run has to be able to bound.
			absent: []string{
				coverageTiersExportBegin + " ",
				coverageTiersInputEnd + " ",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, root, source := coverageTiersSpliceRoot(t, true)
			first := filepath.Join(source, "vitest.config.ts")
			moved := filepath.Join(source, "nested", "vitest.config.ts")

			start, err := spliceCoverageTiersConfig([]byte(tc.raw), first, p)
			if err != nil {
				t.Fatalf("first spliceCoverageTiersConfig() error = %v", err)
			}
			if !bytes.Contains(start, []byte(`"../.dharness/coverage-tiers.mjs"`)) {
				t.Fatalf("the first splice did not reach the owned runtime from the JS project directory:\n%s", start)
			}

			repaired, err := spliceCoverageTiersConfig(start, moved, p)
			if err != nil {
				t.Fatalf("repair spliceCoverageTiersConfig() error = %v", err)
			}

			if before, after := coverageTiersInputPayload(t, start), coverageTiersInputPayload(t, repaired); !bytes.Equal(before, after) {
				t.Errorf("the repair rewrote the value dharness preserves verbatim:\n before %q\n after  %q", before, after)
			}
			for _, want := range tc.want {
				if !bytes.Contains(repaired, []byte(want)) {
					t.Errorf("the repaired config does not contain %q:\n%s", want, repaired)
				}
			}
			for _, absent := range tc.absent {
				if bytes.Contains(repaired, []byte(absent)) {
					t.Errorf("the repaired config still carries %q:\n%s", absent, repaired)
				}
			}
			if bytes.Contains(repaired, []byte(root)) {
				t.Errorf("the repaired config stores this machine's absolute root %q:\n%s", root, repaired)
			}
			coverageTiersRegionStates(t, repaired)
		})
	}
}

// TestSpliceCoverageTiersKeepsTheConsumersLineEndings pins that the managed
// bytes follow the convention the config is already written in, and that the
// value's own line endings are copied rather than normalized.
func TestSpliceCoverageTiersKeepsTheConsumersLineEndings(t *testing.T) {
	p, _, source := coverageTiersSpliceRoot(t, false)
	path := filepath.Join(source, "vitest.config.js")

	candidate, err := spliceCoverageTiersConfig([]byte("export default {\r\n  test: {},\r\n};\r\n"), path, p)
	if err != nil {
		t.Fatalf("spliceCoverageTiersConfig() error = %v", err)
	}
	if !bytes.Contains(candidate, []byte(coverageTiersImportBegin+"\r\n")) {
		t.Errorf("the managed lines did not keep the config's CRLF convention:\n%q", candidate)
	}
	if !bytes.Contains(candidate, []byte("{\r\n  test: {},\r\n}")) {
		t.Errorf("the exported value's own line endings were rewritten:\n%q", candidate)
	}

	again, err := spliceCoverageTiersConfig(candidate, path, p)
	if err != nil {
		t.Fatalf("second spliceCoverageTiersConfig() error = %v", err)
	}
	if !bytes.Equal(again, candidate) {
		t.Errorf("the second call on a CRLF config is not byte-identical:\n%q\n%q", candidate, again)
	}
}

// TestSpliceCoverageTiersRefusesWithoutARewrite pins every refusal: the
// caller gets an error and no candidate bytes, and the input it passed is
// untouched — which is the whole safety property, since this function is the
// only writer of these regions and a refusal must leave the consumer's config
// exactly as it was.
func TestSpliceCoverageTiersRefusesWithoutARewrite(t *testing.T) {
	p, _, source := coverageTiersSpliceRoot(t, false)
	path := filepath.Join(source, "vitest.config.js")

	spliced, err := spliceCoverageTiersConfig([]byte("export default { test: {} };\n"), path, p)
	if err != nil {
		t.Fatalf("fixture spliceCoverageTiersConfig() error = %v", err)
	}
	input := locateCoverageTiersRegion(string(spliced), coverageTiersInputBegin, coverageTiersInputEnd)
	if input.state != markersPresent {
		t.Fatalf("the fixture does not carry an input region:\n%s", spliced)
	}
	disagreeing := string(spliced[:input.from]) + string(spliced[input.to:])

	// Hand edits that leave every marker pair intact but move one region out
	// of the position the first run wrote it in. All three pairs are still
	// present, so these reach the repair path's ordering and containment
	// checks instead of the presence check above.
	importRegion := locateCoverageTiersRegion(string(spliced), coverageTiersImportBegin, coverageTiersImportEnd)
	exportRegion := locateCoverageTiersRegion(string(spliced), coverageTiersExportBegin, coverageTiersExportEnd)
	if importRegion.state != markersPresent || exportRegion.state != markersPresent {
		t.Fatalf("the fixture does not carry the import and export regions:\n%s", spliced)
	}

	// moveRegion returns raw with region removed and re-inserted at at, an
	// offset named in raw's own bytes before the removal.
	moveRegion := func(raw []byte, region coverageTiersRegion, at int) string {
		body := string(raw[region.from:region.to])
		rest := string(raw[:region.from]) + string(raw[region.to:])
		if region.from < at {
			at -= region.to - region.from
		}
		return rest[:at] + body + rest[at:]
	}

	importAfterExport := moveRegion(spliced, importRegion, exportRegion.to)
	inputOutsideExport := moveRegion(spliced, input, exportRegion.to)
	inputBeforeExport := moveRegion(spliced, input, 0)

	declared, err := spliceCoverageTiersConfig([]byte("export default function config() {\n  return { test: {} };\n}\n"), path, p)
	if err != nil {
		t.Fatalf("declaration fixture spliceCoverageTiersConfig() error = %v", err)
	}
	declaredInput := locateCoverageTiersRegion(string(declared), coverageTiersInputBegin, coverageTiersInputEnd)
	declaredExport := locateCoverageTiersRegion(string(declared), coverageTiersExportBegin, coverageTiersExportEnd)
	if declaredInput.state != markersPresent || declaredExport.state != markersPresent {
		t.Fatalf("the declaration fixture does not carry its input and export regions:\n%s", declared)
	}
	declarationAfterExport := moveRegion(declared, declaredInput, declaredExport.to)

	// The refusals these rows pin by identity rather than by a keyword: which
	// marker pair the half-written-region rule names, which of the two
	// disagreeing shapes it reports, and which rule answers for a value the
	// repair path cannot use.
	name := filepath.Base(path)
	malformedImportRefusal := name + ": the dharness:coverage-tiers-import marker pair is malformed — a begin with no matching end, an end with no matching begin, or more than one of either — and dharness refuses to guess at a half-written region"
	regionsDisagreeRefusal := name + ": the dharness:coverage-tiers marker pairs disagree on whether they exist — some are present and others are not, which dharness never writes and will not guess at"
	emptyInputValueRefusal := name + ": the dharness:coverage-tiers-input region carries no value"
	inputOutsideExportRefusal := name + ": the dharness:coverage-tiers-input region is not inside the dharness:coverage-tiers-export region, though the preserved value is an expression the wrapper call has to hold"
	bindingClashRefusal := name + ": this config mentions " + coverageTiersBinding + " outside any region dharness owns, and the managed import would bind that name a second time; rename it, or leave the coverage wiring to a config without the collision"

	cases := []struct {
		name        string
		raw         string
		wantError   string
		wantMessage string
	}{
		{
			name:      "an import marker with no matching end",
			raw:       coverageTiersImportBegin + "\nexport default { test: {} };\n",
			wantError: "malformed",
		},
		{
			name: "a duplicated export marker pair",
			raw: "export default { test: {} };\n" +
				coverageTiersExportBegin + "\n" + coverageTiersExportEnd + "\n" +
				coverageTiersExportBegin + "\n" + coverageTiersExportEnd + "\n",
			wantError: "malformed",
		},
		{
			name:      "marker pairs that disagree about existing",
			raw:       disagreeing,
			wantError: "disagree",
		},
		{
			// The mirror of the row above: a begin marker with no end is
			// malformed, and so is an end marker with no begin. Reading the
			// lone end as "no region at all" would splice a config whose
			// stray end marker survives behind the new region.
			name:        "an end marker with no begin marker",
			raw:         coverageTiersImportEnd + "\nexport default { test: {} };\n",
			wantError:   "malformed",
			wantMessage: malformedImportRefusal,
		},
		{
			// The pair's begin marker exists exactly once and its end marker
			// twice, so one of the two markers is unaccounted for even though
			// each count is non-zero and the pair's first end follows its
			// begin.
			name: "a pair with one begin marker and two end markers",
			raw: coverageTiersImportBegin + "\nexport default { test: {} };\n" +
				coverageTiersImportEnd + "\n" + coverageTiersImportEnd + "\n",
			wantError:   "malformed",
			wantMessage: malformedImportRefusal,
		},
		{
			// A begin marker whose line has no line ending at all: the
			// region's content follows that line, so there is no content to
			// bound and the pair is half-written rather than empty.
			name:        "a begin marker whose line carries no line ending",
			raw:         coverageTiersImportBegin + coverageTiersImportEnd,
			wantError:   "malformed",
			wantMessage: malformedImportRefusal,
		},
		{
			// An import region whose begin marker is immediately followed by
			// its end marker line: nothing between the markers, so the region
			// is well-formed on its own and the file is refused for the
			// disagreement with the two pairs that do not exist.
			name:        "an import region with nothing between its markers",
			raw:         coverageTiersImportBegin + "\n" + coverageTiersImportEnd + "\nexport default { test: {} };\n",
			wantError:   "disagree",
			wantMessage: regionsDisagreeRefusal,
		},
		{
			name:      "an import region moved after the export region",
			raw:       importAfterExport,
			wantError: "does not precede the dharness:coverage-tiers-export region",
		},
		{
			name:      "an expression input region moved outside the export region",
			raw:       inputOutsideExport,
			wantError: "is not inside the dharness:coverage-tiers-export region",
		},
		{
			// The other side of the same containment: the input region moved
			// ahead of the export region instead of past it, so the export
			// region's start is the boundary that has to catch it. A containment
			// test that accepted a shared start would let this through and
			// rewrite a value it never read.
			name:        "an expression input region moved before the export region",
			raw:         inputBeforeExport,
			wantError:   "is not inside the dharness:coverage-tiers-export region",
			wantMessage: inputOutsideExportRefusal,
		},
		{
			name:      "a declaration input region moved after the export region",
			raw:       declarationAfterExport,
			wantError: "follows the dharness:coverage-tiers-export region",
		},
		{
			name:      "a binding the managed import would shadow",
			raw:       "import { withCoverageTiers } from \"./elsewhere.mjs\";\n\nexport default { test: {} };\n",
			wantError: coverageTiersBinding,
		},
		{
			name:      "a URL helper binding the managed import would shadow",
			raw:       "import { fileURLToPath } from \"node:url\";\n\nexport default { test: {} };\n",
			wantError: coverageTiersFileURLName,
		},
		{
			name:      "a class declaration default export",
			raw:       "export default class Config {\n}\n",
			wantError: "class",
		},
		{
			// A hand edit that deleted the value and left its line empty: the
			// region is still a well-formed pair, but the bytes between the
			// markers are not a value, and refusing is the only answer that
			// does not re-emit an export with nothing in it.
			name:        "an input region with an empty line where the value was",
			raw:         strings.Replace(string(spliced), "{ test: {} }\n", "\n", 1),
			wantError:   "carries no value",
			wantMessage: emptyInputValueRefusal,
		},
		{
			// The repair path scans the preserved value for the managed
			// binding, because the value is the consumer's own bytes and the
			// repaired import would bind that name on top of it. A hand edit
			// that writes the binding into the value is that collision, and it
			// is refused rather than silently rewritten around.
			name:        "a preserved value that mentions the managed binding",
			raw:         strings.Replace(string(spliced), "{ test: {} }", "{ test: { root: "+coverageTiersBinding+" } }", 1),
			wantError:   coverageTiersBinding,
			wantMessage: bindingClashRefusal,
		},
		{
			name: "source that does not parse",
			raw:  "export default {\n",
		},
		{
			name:      "no default export at all",
			raw:       "const config = { test: {} };\n",
			wantError: "no default export",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(tc.raw)
			original := append([]byte(nil), raw...)

			candidate, err := spliceCoverageTiersConfig(raw, path, p)
			if err == nil {
				t.Fatal("spliceCoverageTiersConfig() = nil error, want a refusal")
			}
			if len(candidate) != 0 {
				t.Errorf("spliceCoverageTiersConfig() = %q, want no candidate bytes", candidate)
			}
			if !bytes.Equal(raw, original) {
				t.Errorf("the refused call rewrote its input:\n got %q\nwant %q", raw, original)
			}
			if tc.wantError != "" && !strings.Contains(err.Error(), tc.wantError) {
				t.Errorf("spliceCoverageTiersConfig() error = %q, want it to name %q", err, tc.wantError)
			}
			if tc.wantMessage != "" && err.Error() != tc.wantMessage {
				t.Errorf("spliceCoverageTiersConfig() error = %q, want exactly %q", err.Error(), tc.wantMessage)
			}
		})
	}
}
