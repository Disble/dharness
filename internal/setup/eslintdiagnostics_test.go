package setup

import (
	"errors"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/runner"
	"github.com/Disble/dharness/internal/tool"
)

// wiredEslintConfig is a project flat config carrying both marker pairs the
// post-wiring check is guarded on. Its JavaScript never runs: every test
// here seams runner.Run, and the guard reads the markers textually.
const wiredEslintConfig = "export default [\n  " + eslintImportBegin + "\n" +
	"import dharnessLayer from \"./.dharness/eslint.config.mjs\";\n" + eslintImportEnd + "\n" +
	"  " + eslintLayerBegin + "\n  ...dharnessLayer({ plugin: dharnessPlugin }),\n  " +
	eslintLayerEnd + "\n];\n"

// eslintDiagnosticProject builds the tree every case decides over: one flat
// config, one TSX source whose own text is never read, and — unless
// installESLint is false — the local binary LocalBinary looks for.
func eslintDiagnosticProject(t *testing.T, installESLint bool, config string) project.Project {
	t.Helper()
	root := t.TempDir()
	writeGoldenFixtureFile(t, root, "package.json", `{"name":"x"}`)
	writeGoldenFixtureFile(t, root, "eslint.config.mjs", config)
	writeNestedFixtureFile(t, filepath.Join(root, "src"), "a.tsx", "")
	if installESLint {
		writeLocalESLintBinary(t, root)
	}
	return project.Project{Root: root, Source: root}
}

// eslintDiagnosticsJSON renders a case's --format json answer, with <root>
// standing in for the absolute project path ESLint itself prints. A real
// filePath is absolute, and keeping it that way is what makes the note's
// own source-relative entry a measured conversion rather than a formatting
// coincidence.
func eslintDiagnosticsJSON(root, body string) string {
	return strings.ReplaceAll(body, "<root>", filepath.ToSlash(root))
}

// stubESLintDiagnostics seams runner.Run for one case: the check's own lint
// invocation answers with stdout, stderr and err, while a --print-config
// probe — which is not this check but collectNotes' withdrawal detector —
// answers nothing. Filtering the two apart is the point of
// eslintDiagnosticCommands below: the note must be derived from the native
// lint run and never from the package-level plugin memo, which a renderer
// may have filled from the pre-write config.
func stubESLintDiagnostics(t *testing.T, stdout, stderr string, err error) *[]runner.Command {
	t.Helper()
	var commands []runner.Command
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, out, errOut io.Writer) error {
		commands = append(commands, cmd)
		if slices.Contains(cmd.Args, "--print-config") {
			return nil
		}
		_, _ = io.WriteString(out, stdout)
		_, _ = io.WriteString(errOut, stderr)
		return err
	}))
	return &commands
}

// eslintDiagnosticCommands keeps only the invocations this check itself
// makes. --no-warn-ignored is the marker: no other invocation in a sync
// carries it, and the probes the withdrawal detector runs from collectNotes
// do not.
func eslintDiagnosticCommands(commands []runner.Command) []runner.Command {
	var lint []runner.Command
	for _, cmd := range commands {
		if slices.Contains(cmd.Args, "--no-warn-ignored") {
			lint = append(lint, cmd)
		}
	}
	return lint
}

// TestEslintDiagnosticsReadsESLintEvidence is this check's whole decision
// table. ESLint's `fatal` flag is the direct signal (§09) and the only
// thing read: an ordinary rule finding is the layer doing its job, so
// treating one as a config problem would turn every sync of a project with
// lint debt into a correction nobody asked for. A fatal diagnostic is
// carried as evidence, never as a proved cause, and an invocation whose
// output cannot be read is an unanswered question rather than a pass.
func TestEslintDiagnosticsReadsESLintEvidence(t *testing.T) {
	exitOne := &runner.ExitError{Command: "eslint", Code: 1}

	cases := []struct {
		name       string
		stdout     string
		stderr     string
		runErr     error
		wantKind   string
		wantReason []string
		wantEntry  []string
	}{
		{
			name: "a fatal diagnostic is the corrective note, and the rule finding beside it is not",
			stdout: `[{"filePath":"<root>/src/a.tsx","messages":[` +
				`{"fatal":true,"message":"Parsing error: Unexpected token <","line":1,"column":26},` +
				`{"ruleId":"dharness/require-jsdoc","fatal":false,"message":"missing JSDoc","line":1,"column":1}],` +
				`"errorCount":2}]`,
			runErr:     exitOne,
			wantKind:   "not-parsed",
			wantEntry:  []string{"src/a.tsx:1:26: Parsing error: Unexpected token <"},
			wantReason: []string{"ESLint", "does not prove a parser is missing", "eslint.config.mjs", "dharness sync"},
		},
		{
			name: "a fatal diagnostic after an ordinary finding is still the corrective note",
			stdout: `[{"filePath":"<root>/src/a.tsx","messages":[` +
				`{"ruleId":"dharness/require-jsdoc","fatal":false,"message":"missing JSDoc","line":1,"column":1},` +
				`{"fatal":true,"message":"Parsing error: Unexpected token <","line":2,"column":10}],` +
				`"errorCount":2}]`,
			runErr:     exitOne,
			wantKind:   "not-parsed",
			wantEntry:  []string{"src/a.tsx:2:10: Parsing error: Unexpected token <"},
			wantReason: []string{"ESLint", "eslint.config.mjs", "dharness sync"},
		},
		{
			name: "every fatal diagnostic is carried, each with only the coordinates ESLint gave it",
			// Four shapes in one file, because a location is rendered by two
			// guards and each one has a wrong answer ESLint's own output makes
			// reachable. The zeroes are what ESLint writes when it numbered
			// nothing, and a zero rendered as a coordinate points a reader at a
			// place ESLint never named. The line at 1 and the column at 1 are
			// what say each guard is above zero rather than above one.
			stdout: `[{"filePath":"<root>/src/a.tsx","messages":[` +
				`{"fatal":true,"message":"Parsing error: Unexpected token <","line":0,"column":0},` +
				`{"fatal":true,"message":"Parsing error: Unexpected token (","line":1,"column":0},` +
				`{"fatal":true,"message":"Parsing error: Unexpected token {","line":3,"column":0},` +
				`{"fatal":true,"message":"Parsing error: Unexpected token >","line":3,"column":1}],` +
				`"errorCount":4}]`,
			runErr:   exitOne,
			wantKind: "not-parsed",
			wantEntry: []string{
				"src/a.tsx: Parsing error: Unexpected token <",
				"src/a.tsx:1: Parsing error: Unexpected token (",
				"src/a.tsx:3: Parsing error: Unexpected token {",
				"src/a.tsx:3:1: Parsing error: Unexpected token >",
			},
			wantReason: []string{"ESLint", "eslint.config.mjs"},
		},
		{
			name: "rule findings alone are not a config problem",
			stdout: `[{"filePath":"<root>/src/a.tsx","messages":[` +
				`{"ruleId":"dharness/require-jsdoc","fatal":false,"message":"missing JSDoc","line":3,"column":1}],` +
				`"errorCount":1}]`,
			runErr: exitOne,
		},
		{
			name:   "an ignored file the warning was suppressed for leaves no note",
			stdout: `[]`,
		},
		{
			name:   "a clean file with no messages leaves no note",
			stdout: `[{"filePath":"<root>/src/a.tsx","messages":[]}]`,
		},
		{
			name:       "a configuration error is unanswered, not passed",
			stderr:     "ESLint couldn't find an eslint.config.(js|mjs|cjs) file.",
			runErr:     &runner.ExitError{Command: "eslint", Code: 2},
			wantKind:   "not-checked",
			wantReason: []string{"ESLint", "unanswered", "couldn't find an eslint.config", "dharness sync"},
		},
		{
			name:       "a process that never started is unanswered too",
			runErr:     &runner.StartError{Command: "eslint", Cause: errors.New("executable file not found")},
			wantKind:   "not-checked",
			wantReason: []string{"could not run eslint", "executable file not found"},
		},
		{
			name:       "an empty stdout is not a verdict",
			wantKind:   "not-checked",
			wantReason: []string{"did not carry ESLint's JSON results array"},
		},
		{
			name:       "a JSON object is not the array ESLint's reporter writes",
			stdout:     `{"results":[]}`,
			wantKind:   "not-checked",
			wantReason: []string{"did not carry ESLint's JSON results array"},
		},
		{
			name:       "a literal null is not an empty result set",
			stdout:     `null`,
			wantKind:   "not-checked",
			wantReason: []string{"did not carry ESLint's JSON results array"},
		},
		{
			name:       "an exit code that is neither a lint verdict nor a config error makes stdout untrustworthy",
			stdout:     `[{"filePath":"<root>/src/a.tsx","messages":[]}]`,
			runErr:     &runner.ExitError{Command: "eslint", Code: 3},
			wantKind:   "not-checked",
			wantReason: []string{"exited with code 3"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := eslintDiagnosticProject(t, true, wiredEslintConfig)
			commands := stubESLintDiagnostics(t, eslintDiagnosticsJSON(p.Root, tc.stdout), tc.stderr, tc.runErr)

			notes := eslintDiagnostics(p)

			lint := eslintDiagnosticCommands(*commands)
			if len(lint) != 1 {
				t.Fatalf("eslintDiagnostics ran %d native lint invocation(s), want exactly one: %v", len(lint), *commands)
			}
			wantArgs := tool.ESLintDiagnostics(eslintProbePaths(p))
			if got := lint[0]; got.Name != p.LocalBinary(tool.ESLint) || !slices.Equal(got.Args, wantArgs) {
				t.Errorf("eslintDiagnostics ran %q %v, want %q %v", got.Name, got.Args, p.LocalBinary(tool.ESLint), wantArgs)
			}

			if tc.wantKind == "" {
				if len(notes) != 0 {
					t.Fatalf("eslintDiagnostics() = %+v, want no note", notes)
				}
				return
			}
			if len(notes) != 1 {
				t.Fatalf("eslintDiagnostics() = %+v, want exactly one note", notes)
			}

			note := notes[0]
			if note.Kind != tc.wantKind {
				t.Errorf("note.Kind = %q, want %q", note.Kind, tc.wantKind)
			}
			if !note.Actionable {
				t.Error("note.Actionable = false, want the correction or the rerun to reach the agent")
			}
			if note.Path != "eslint.config.mjs" {
				t.Errorf("note.Path = %q, want the consumer's own config, repository-relative", note.Path)
			}
			wantEntries := tc.wantEntry
			if wantEntries == nil {
				wantEntries = eslintProbePaths(p)
			}
			if !slices.Equal(note.Entries, wantEntries) {
				t.Errorf("note.Entries = %v, want %v", note.Entries, wantEntries)
			}
			for _, want := range tc.wantReason {
				if !strings.Contains(note.Reason, want) {
					t.Errorf("note.Reason does not carry %q:\n%s", want, note.Reason)
				}
			}
		})
	}
}

// TestEslintDiagnosticsIsGuardedOnOwnedWiringAndALocalTool pins the ways
// this check stays silent: nothing to say about a project dharness did not
// wire, nothing to invent when the markers are not both there, and nothing
// to measure — or install — when the project has no local ESLint.
func TestEslintDiagnosticsIsGuardedOnOwnedWiringAndALocalTool(t *testing.T) {
	t.Run("a repository with no source is never asked", func(t *testing.T) {
		commands := stubESLintDiagnostics(t, `[]`, "", nil)

		if notes := eslintDiagnostics(project.Project{}); notes != nil {
			t.Errorf("eslintDiagnostics() = %+v, want no note", notes)
		}
		if len(*commands) != 0 {
			t.Errorf("eslintDiagnostics ran %v, want no process", *commands)
		}
	})

	t.Run("a config without dharness's markers is not dharness's to judge", func(t *testing.T) {
		commands := stubESLintDiagnostics(t, `[]`, "", nil)

		if notes := eslintDiagnostics(eslintDiagnosticProject(t, true, "export default [];\n")); notes != nil {
			t.Errorf("eslintDiagnostics() = %+v, want no note", notes)
		}
		if len(*commands) != 0 {
			t.Errorf("eslintDiagnostics ran %v, want no process", *commands)
		}
	})

	t.Run("one marker pair alone is not the owned wiring", func(t *testing.T) {
		commands := stubESLintDiagnostics(t, `[]`, "", nil)
		half := "export default [\n  " + eslintImportBegin + "\n" + eslintImportEnd + "\n];\n"

		if notes := eslintDiagnostics(eslintDiagnosticProject(t, true, half)); notes != nil {
			t.Errorf("eslintDiagnostics() = %+v, want no note", notes)
		}
		if len(*commands) != 0 {
			t.Errorf("eslintDiagnostics ran %v, want no process", *commands)
		}
	})

	t.Run("a half-written import region beside a whole layer region is still not the owned wiring", func(t *testing.T) {
		// The layer pair below is complete and well formed, so a guard that
		// judged whichever pair it found first, or read one half as both,
		// would accept this config and launch ESLint on it. Nothing was
		// launched, which is the assertion: dharness wrote both regions in
		// the same order every time, so an import region that is reversed or
		// half there is a hand edit, and the layer region beside it does not
		// make the file dharness's own.
		cases := map[string]string{
			"reversed": "export default [\n  " + eslintImportEnd + "\n  " + eslintImportBegin +
				"\n  " + eslintLayerBegin + "\n  ...dharnessLayer({ plugin: dharnessPlugin }),\n  " + eslintLayerEnd + "\n];\n",
			"lone end": "export default [\n  " + eslintImportEnd +
				"\n  " + eslintLayerBegin + "\n  ...dharnessLayer({ plugin: dharnessPlugin }),\n  " + eslintLayerEnd + "\n];\n",
		}
		for name, config := range cases {
			commands := stubESLintDiagnostics(t, `[]`, "", nil)

			if notes := eslintDiagnostics(eslintDiagnosticProject(t, true, config)); notes != nil {
				t.Errorf("%s: eslintDiagnostics() = %+v, want no note", name, notes)
			}
			if len(*commands) != 0 {
				t.Errorf("%s: eslintDiagnostics ran %v, want no process: these markers are not both a region dharness wrote", name, *commands)
			}
		}
	})

	t.Run("a wired layer with no local ESLint is not measured, and nothing is installed", func(t *testing.T) {
		commands := stubESLintDiagnostics(t, `[]`, "", nil)
		p := eslintDiagnosticProject(t, false, wiredEslintConfig)

		if notes := eslintDiagnostics(p); notes != nil {
			t.Errorf("eslintDiagnostics() = %+v, want no note: a missing tool is not a parse verdict", notes)
		}
		if len(*commands) != 0 {
			t.Errorf("eslintDiagnostics ran %v, want no process: a missing tool is not an installation", *commands)
		}
	})
}

// TestRunAsksForTheEslintDiagnosticAfterTheWiringIsWritten pins the "after
// the successful loop, before return" placement. A stub step writes the
// wired config, so a helper asked with collectNotes — before the first byte
// changes — would find no markers and derive nothing.
func TestRunAsksForTheEslintDiagnosticAfterTheWiringIsWritten(t *testing.T) {
	root := t.TempDir()
	writeGoldenFixtureFile(t, root, "package.json", `{"name":"x"}`)
	writeNestedFixtureFile(t, filepath.Join(root, "src"), "a.tsx", "")
	writeLocalESLintBinary(t, root)
	p := project.Project{Root: root, Source: root}
	commands := stubESLintDiagnostics(t, `[{"filePath":"`+filepath.ToSlash(filepath.Join(root, "src", "a.tsx"))+
		`","messages":[{"fatal":true,"message":"Parsing error: Unexpected token <","line":1,"column":26}]}]`,
		"", &runner.ExitError{Command: "eslint", Code: 1})

	wiring := stubApplyStep{id: "wire eslint", fn: func(w *Writer, _ io.Writer) (Facts, error) {
		return Facts{}, w.Write(filepath.Join(root, "eslint.config.mjs"), []byte(wiredEslintConfig))
	}}

	_, notes, err := run([]Step{wiring}, p)
	if err != nil {
		t.Fatalf("run() = %v", err)
	}
	if lint := eslintDiagnosticCommands(*commands); len(lint) != 1 {
		t.Fatalf("run() ran %d native lint invocation(s), want the post-wiring diagnostic exactly once: %v", len(lint), *commands)
	}
	if len(notes) != 1 || notes[0].Kind != "not-parsed" {
		t.Fatalf("notes = %+v, want the post-wiring parse note", notes)
	}
}

// TestRunDerivesNoEslintDiagnosticFromARolledBackRun pins the other half of
// the placement: the failure path returns before the diagnostic is asked,
// so a run that rolled back cannot report on bytes it just undid.
func TestRunDerivesNoEslintDiagnosticFromARolledBackRun(t *testing.T) {
	p := eslintDiagnosticProject(t, true, wiredEslintConfig)
	commands := stubESLintDiagnostics(t, `[]`, "", nil)
	failing := stubApplyStep{id: "broken", fn: func(*Writer, io.Writer) (Facts, error) {
		return Facts{}, errors.New("broken")
	}}

	_, notes, err := run([]Step{failing}, p)
	if err == nil {
		t.Fatal("run() = nil, want the step failure to surface")
	}
	if notes != nil {
		t.Errorf("run() notes = %+v on a rolled-back run, want nil", notes)
	}
	if lint := eslintDiagnosticCommands(*commands); len(lint) != 0 {
		t.Errorf("run() ran %v on a rolled-back run, want no native lint invocation", lint)
	}
}
