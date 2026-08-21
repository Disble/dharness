package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/preset"
	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/report"
	"github.com/Disble/dharness/internal/runner"
)

// A run that rolls back left the repository as it found it, so nothing may
// survive: neither a file that was created nor an edit to one that existed.
func TestWriterUndoRestoresEverythingItTouched(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "kept.json")
	if err := os.WriteFile(existing, []byte(`{"mine":true}`), 0o600); err != nil {
		t.Fatal(err)
	}

	w := &Writer{}
	if err := w.Write(existing, []byte(`{"overwritten":true}`)); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(filepath.Join(root, "nested", "new.yml"), []byte("added\n")); err != nil {
		t.Fatal(err)
	}

	if err := w.Undo(); err != nil {
		t.Fatalf("Undo() = %v", err)
	}

	restored, err := os.ReadFile(existing)
	if err != nil || string(restored) != `{"mine":true}` {
		t.Errorf("an existing file was not put back: %q, %v", restored, err)
	}
	if _, err := os.Stat(filepath.Join(root, "nested", "new.yml")); !os.IsNotExist(err) {
		t.Error("a created file survived the undo")
	}
}

// The snapshot is of the original, not of whatever the previous write left, so
// two writes to one file still restore what was there before either.
func TestWriterUndoKeepsTheOriginalAcrossRepeatedWrites(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	w := &Writer{}
	for _, contents := range []string{"first", "second"} {
		if err := w.Write(path, []byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Undo(); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(path)
	if string(raw) != "original" {
		t.Errorf("restored %q, want the contents from before the first write", raw)
	}
}

// A stub is enough here: the contract under test is that Apply consults
// Delegated before touching Apply at all, not any particular step's logic.
type stubDelegatedStep struct {
	applied *bool
}

func (stubDelegatedStep) ID() string                      { return "stub delegated step" }
func (stubDelegatedStep) Describe(project.Project) string { return "" }
func (stubDelegatedStep) Satisfied(project.Project) bool  { return false }

func (stubDelegatedStep) Delegated(project.Project) (string, bool) {
	return "handed to the agent", true
}

func (s stubDelegatedStep) Apply(project.Project, *Writer, io.Writer) (Facts, error) {
	*s.applied = true
	return Facts{}, nil
}

func TestApplySkipsEveryDelegatedStep(t *testing.T) {
	applied := false
	step := stubDelegatedStep{applied: &applied}

	if _, err := applySteps([]Step{step}, project.Project{}, io.Discard); err != nil {
		t.Fatalf("applySteps() = %v", err)
	}
	if applied {
		t.Error("Apply was called on a step Delegated() reported as ok == true")
	}
}

// agentSkillStep.Delegated always returns ok == true, so applySteps never
// calls its Apply. The error Apply returns is a contract assertion for the
// case that should be unreachable, not a code path any run takes.
func TestAgentSkillApplyIsUnreachable(t *testing.T) {
	if _, err := (agentSkillStep{}).Apply(project.Project{}, &Writer{}, io.Discard); err == nil {
		t.Error("agentSkillStep.Apply() = nil, want the delegated-and-must-not-be-applied assertion")
	}
}

// stubSinkStep is a minimal Step whose Apply writes a marker to the out
// parameter it is handed and returns structured Facts, standing in for a
// real step (installStep, hookInstallStep) without depending on either.
type stubSinkStep struct{}

func (stubSinkStep) ID() string                               { return "stub sink step" }
func (stubSinkStep) Describe(project.Project) string          { return "" }
func (stubSinkStep) Satisfied(project.Project) bool           { return false }
func (stubSinkStep) Delegated(project.Project) (string, bool) { return "", false }

func (stubSinkStep) Apply(_ project.Project, _ *Writer, out io.Writer) (Facts, error) {
	fmt.Fprint(out, "marker-bytes-from-the-stub-step")
	return Facts{Installed: []string{"pkg"}}, nil
}

// stubWriteStep writes one file per path it is given, letting a test build
// a step whose attributed files are known in advance.
type stubWriteStep struct {
	id    string
	paths []string
}

func (s stubWriteStep) ID() string                             { return s.id }
func (stubWriteStep) Describe(project.Project) string          { return "" }
func (stubWriteStep) Satisfied(project.Project) bool           { return false }
func (stubWriteStep) Delegated(project.Project) (string, bool) { return "", false }

func (s stubWriteStep) Apply(_ project.Project, w *Writer, _ io.Writer) (Facts, error) {
	for _, path := range s.paths {
		if err := w.Write(path, []byte("content")); err != nil {
			return Facts{}, err
		}
	}
	return Facts{}, nil
}

// TestPerStepFileAttributionIsPartitioned pins the file-attribution
// requirement's own scenario: two steps that both write files are
// attributed independently, with no overlap between them, even though both
// share the one Writer applySteps threads through the whole run.
func TestPerStepFileAttributionIsPartitioned(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a.txt")
	b1 := filepath.Join(root, "b1.txt")
	b2 := filepath.Join(root, "b2.txt")

	steps := []Step{
		stubWriteStep{id: "step A", paths: []string{a}},
		stubWriteStep{id: "step B", paths: []string{b1, b2}},
	}

	outcomes, err := applySteps(steps, project.Project{Root: root}, io.Discard)
	if err != nil {
		t.Fatalf("applySteps() = %v", err)
	}
	if len(outcomes) != 2 {
		t.Fatalf("applySteps() returned %d outcomes, want 2", len(outcomes))
	}

	wantPaths := func(changes []report.FileChange) []string {
		paths := make([]string, len(changes))
		for i, change := range changes {
			paths[i] = change.Path
		}
		return paths
	}

	if got := wantPaths(outcomes[0].wrote); !slices.Equal(got, []string{"a.txt"}) {
		t.Errorf("step A's attributed files = %v, want [a.txt]", got)
	}
	if got := wantPaths(outcomes[1].wrote); !slices.Equal(got, []string{"b1.txt", "b2.txt"}) {
		t.Errorf("step B's attributed files = %v, want [b1.txt b2.txt]", got)
	}
}

// TestApplyWritesOnlyToTheGivenSink pins the sink requirement (spec.md
// step-outcome, first two requirements) at the applySteps layer: a step's
// Apply writes only through the out parameter it is handed — a per-step
// buffer applySteps controls — never through some channel of its own
// choosing, and its structured Facts return value survives the trip back to
// the caller, a fact no byte stream could carry.
//
// The marker text is expected to reach the writer applySteps itself was
// given: Decision 9's own invariant for this slice ("nothing a user sees
// changes") and TestSyncStdoutUnchangedAfterTheSinkMove both require
// applySteps to copy each step's captured sink back onto its own stdout, so
// dharness sync's real output stays byte-identical until slice 4 rewrites
// RunSync to render a report instead of streaming live. What the sink
// prevents is a step writing to some channel applySteps never sees at all
// (os.Stdout/os.Stderr directly — defect 5), not the framed copy back.
func TestApplyWritesOnlyToTheGivenSink(t *testing.T) {
	var stdout bytes.Buffer
	outcomes, err := applySteps([]Step{stubSinkStep{}}, project.Project{}, &stdout)
	if err != nil {
		t.Fatalf("applySteps() = %v", err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("applySteps() returned %d outcomes, want 1", len(outcomes))
	}
	if !slices.Equal(outcomes[0].facts.Installed, []string{"pkg"}) {
		t.Errorf("outcome Facts.Installed = %v, want [pkg]", outcomes[0].facts.Installed)
	}
	if strings.Count(stdout.String(), "marker-bytes-from-the-stub-step") != 1 {
		t.Errorf("the step's sink content did not reach the writer applySteps was given, exactly once: %q", stdout.String())
	}
}

// TestRunReturnsAStepResultForEveryPlanStep pins "every step in Plan()
// carries exactly one status, and none is an ambiguous absence": Run(p),
// over the real registry, returns one entry per Plan() step and every
// entry's status is one of the six defined values.
func TestRunReturnsAStepResultForEveryPlanStep(t *testing.T) {
	p, _, _ := integrationProject(t)
	t.Cleanup(runner.SetForTest(func(runner.Command, io.Writer, io.Writer) error { return nil }))

	steps, _, err := Run(p)
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if len(steps) != len(Plan()) {
		t.Fatalf("Run() returned %d steps, want %d (len(Plan()))", len(steps), len(Plan()))
	}

	valid := map[report.Status]bool{
		report.Applied: true, report.Delegated: true, report.Satisfied: true,
		report.Failed: true, report.NotReached: true, report.Retracted: true,
	}
	for _, step := range steps {
		if !valid[step.Status] {
			t.Errorf("step %q has status %q, want one of the six defined values, none empty or unrecognised", step.ID, step.Status)
		}
	}
}

// TestRunNumbersStepsFromOneInPlanOrder is a mutation guard for run's own
// `n := i + 1`: N must be the step's 1-based position in Plan() order, not
// merely non-zero — the human view's "n/total" numbering (gap 6) reads
// directly from it, so an off-by-one here would misnumber every step.
func TestRunNumbersStepsFromOneInPlanOrder(t *testing.T) {
	steps, _, err := run([]Step{stubSatisfiedStep{}, stubSatisfiedStep{}, stubSatisfiedStep{}}, project.Project{})
	if err != nil {
		t.Fatalf("run() = %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("run() returned %d steps, want 3", len(steps))
	}
	for i, step := range steps {
		if want := i + 1; step.N != want {
			t.Errorf("steps[%d].N = %d, want %d", i, step.N, want)
		}
	}
}

// TestRunReadsNotesBeforeAnyByteChanges pins design.md Decision 8: notes are
// read first inside Run, before the loop touches anything. A stub step
// whose Apply writes .fallowrc.json would, if UncheckableConfigNote were
// evaluated after it ran instead of before, make the blind spot look
// resolved by the very run that could not see past it.
func TestRunReadsNotesBeforeAnyByteChanges(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "fallow.toml"), []byte("ignorePatterns = [\"wailsjs/**\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := project.Project{Root: root, Source: root}

	step := stubWriteStep{id: "stub", paths: []string{filepath.Join(root, fallowConfig)}}

	_, notes, err := run([]Step{step}, p)
	if err != nil {
		t.Fatalf("run() = %v", err)
	}

	found := false
	for _, note := range notes {
		if note.Kind == "not-checked" {
			found = true
		}
	}
	if !found {
		t.Errorf("notes = %+v, want the not-checked note — it must be read before the stub step's Apply wrote .fallowrc.json, not after", notes)
	}
}

// stubSatisfiedStep is a minimal Step whose Satisfied always answers true,
// recording whether Delegated was ever asked of it.
type stubSatisfiedStep struct {
	delegatedCalled *bool
}

func (stubSatisfiedStep) ID() string                      { return "stub satisfied step" }
func (stubSatisfiedStep) Describe(project.Project) string { return "already done" }
func (stubSatisfiedStep) Satisfied(project.Project) bool  { return true }

func (s stubSatisfiedStep) Delegated(project.Project) (string, bool) {
	*s.delegatedCalled = true
	return "should never be asked", true
}

func (stubSatisfiedStep) Apply(project.Project, *Writer, io.Writer) (Facts, error) {
	return Facts{}, nil
}

// TestRunOrdersSatisfiedBeforeDelegated pins design.md Decision 8, change
// #1: Satisfied is asked first, and Delegated is never asked of a step that
// already answers true.
func TestRunOrdersSatisfiedBeforeDelegated(t *testing.T) {
	called := false
	step := stubSatisfiedStep{delegatedCalled: &called}

	steps, _, err := run([]Step{step}, project.Project{})
	if err != nil {
		t.Fatalf("run() = %v", err)
	}
	if called {
		t.Error("Delegated() was called on a step Satisfied() already reported true")
	}
	if len(steps) != 1 || steps[0].Status != report.Satisfied {
		t.Errorf("run() = %+v, want exactly one satisfied step", steps)
	}
	if steps[0].Evidence != "already done" {
		t.Errorf("Evidence = %q, want the stub's single-line Describe() text unchanged", steps[0].Evidence)
	}
}

// TestFirstLineTakesOnlyTheFirstLine pins the boundary firstLine exists
// for: a single-line input passes through unchanged, and a multi-line one
// is cut at its first newline, dropping everything after it.
func TestFirstLineTakesOnlyTheFirstLine(t *testing.T) {
	if got := firstLine("one line only"); got != "one line only" {
		t.Errorf("firstLine(%q) = %q, want the input unchanged", "one line only", got)
	}
	if got := firstLine("first\nsecond\nthird"); got != "first" {
		t.Errorf("firstLine(%q) = %q, want %q", "first\nsecond\nthird", got, "first")
	}
}

// TestSatisfiedStepCarriesEvidenceNotBareStatus pins the same requirement's
// second scenario: a satisfied step's entry names the fact that satisfied
// it, not merely the status word.
func TestSatisfiedStepCarriesEvidenceNotBareStatus(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root}
	if err := wireFallowExtends(p, &Writer{}); err != nil {
		t.Fatal(err)
	}
	if !(fallowExtendsStep{}).Satisfied(p) {
		t.Fatal("fixture is not satisfied; fix the test before trusting its assertion")
	}

	steps, _, err := run([]Step{fallowExtendsStep{}}, p)
	if err != nil {
		t.Fatalf("run() = %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("run() returned %d steps, want 1", len(steps))
	}
	if steps[0].Status != report.Satisfied {
		t.Fatalf("status = %q, want satisfied", steps[0].Status)
	}
	if steps[0].Evidence == "" {
		t.Error("Evidence is empty for a satisfied step, want a status word plus a supporting fact")
	}
	// Gap 4 from the team lead's measured run: a satisfied step's evidence
	// must be the detection fact that satisfied it, not Describe's first
	// line — which for most steps is fix instructions for the unsatisfied
	// case, actively wrong read as evidence nothing needs doing. spec.md's
	// own scenario names this exact step and this exact fact
	// ("`.fallowrc.json` already contains `extends →
	// .dharness/fallow.jsonc`").
	want := "extends → " + ownedFrom(p, p.Source, ownedFallow)
	if steps[0].Evidence != want {
		t.Errorf("Evidence = %q, want the step's own detection fact %q", steps[0].Evidence, want)
	}
}

// TestSatisfiedStepsCarryTheirOwnDetectionFactNotDescribesFixInstructions
// extends the guard above (gap 4) to every other step whose satisfied
// evidence used to reuse Describe's first line — instructions for the
// unsatisfied case, not a fact about why this run found nothing to do. The
// team lead's measured example was legacyLintConfigStep, observed showing
// "Make .eslintrc.json parse, or delete it..." as if that were evidence
// nothing needed fixing.
func TestSatisfiedStepsCarryTheirOwnDetectionFactNotDescribesFixInstructions(t *testing.T) {
	t.Run("lefthookExtendsStep, wired", func(t *testing.T) {
		root := t.TempDir()
		p := project.Project{Root: root, Source: root}
		writeStepFixtureFile(t, root, "lefthook.yml", "extends:\n  - .dharness/lefthook.yml\n")
		if !(lefthookExtendsStep{}).Satisfied(p) {
			t.Fatal("fixture is not satisfied; fix the test before trusting its assertion")
		}
		steps, _, err := run([]Step{lefthookExtendsStep{}}, p)
		if err != nil || len(steps) != 1 {
			t.Fatalf("run() = %+v, %v", steps, err)
		}
		if want := "extends → " + ownedFrom(p, p.Root, ownedLefthook); steps[0].Evidence != want {
			t.Errorf("Evidence = %q, want %q", steps[0].Evidence, want)
		}
	})

	t.Run("lefthookExtendsStep, no lefthook manager", func(t *testing.T) {
		root := t.TempDir()
		p := project.Project{Root: root, Source: root}
		if !(lefthookExtendsStep{}).Satisfied(p) {
			t.Fatal("fixture is not satisfied; fix the test before trusting its assertion")
		}
		steps, _, err := run([]Step{lefthookExtendsStep{}}, p)
		if err != nil || len(steps) != 1 {
			t.Fatalf("run() = %+v, %v", steps, err)
		}
		if steps[0].Evidence == "" {
			t.Error("Evidence is empty")
		}
	})

	t.Run("legacyLintConfigStep, file not present", func(t *testing.T) {
		root := t.TempDir()
		p := project.Project{Root: root, Source: root}
		if !(legacyLintConfigStep{}).Satisfied(p) {
			t.Fatal("fixture is not satisfied; fix the test before trusting its assertion")
		}
		steps, _, err := run([]Step{legacyLintConfigStep{}}, p)
		if err != nil || len(steps) != 1 {
			t.Fatalf("run() = %+v, %v", steps, err)
		}
		if steps[0].Evidence != "not present" {
			t.Errorf("Evidence = %q, want %q", steps[0].Evidence, "not present")
		}
	})

	t.Run("legacyLintConfigStep, file present and parses", func(t *testing.T) {
		root := t.TempDir()
		p := project.Project{Root: root, Source: root}
		writeStepFixtureFile(t, root, legacyLintConfig, "{}")
		if !(legacyLintConfigStep{}).Satisfied(p) {
			t.Fatal("fixture is not satisfied; fix the test before trusting its assertion")
		}
		steps, _, err := run([]Step{legacyLintConfigStep{}}, p)
		if err != nil || len(steps) != 1 {
			t.Fatalf("run() = %+v, %v", steps, err)
		}
		if steps[0].Evidence != "parses" {
			t.Errorf("Evidence = %q, want %q", steps[0].Evidence, "parses")
		}
		if strings.Contains(steps[0].Evidence, "Make") {
			t.Errorf("Evidence = %q still reuses Describe's fix-instruction text", steps[0].Evidence)
		}
	})

	t.Run("mcpStep", func(t *testing.T) {
		root := t.TempDir()
		p := project.Project{Root: root, Source: root}
		writeStepFixtureFile(t, root, mcpConfig, `{"mcpServers":{"fallow":{"command":"bunx"}}}`)
		if !(mcpStep{}).Satisfied(p) {
			t.Fatal("fixture is not satisfied; fix the test before trusting its assertion")
		}
		steps, _, err := run([]Step{mcpStep{}}, p)
		if err != nil || len(steps) != 1 {
			t.Fatalf("run() = %+v, %v", steps, err)
		}
		if want := mcpConfig + " declares fallow"; steps[0].Evidence != want {
			t.Errorf("Evidence = %q, want %q", steps[0].Evidence, want)
		}
	})

	t.Run("hookInstallStep, lefthook", func(t *testing.T) {
		root := t.TempDir()
		p := project.Project{Root: root, Source: root}
		if err := os.MkdirAll(filepath.Join(root, ".git", "hooks"), 0o755); err != nil {
			t.Fatal(err)
		}
		writeStepFixtureFile(t, root, "lefthook.yml", "pre-commit:\n")
		writeStepFixtureFile(t, filepath.Join(root, ".git", "hooks"), "pre-commit", "lefthook run pre-commit\n")
		if !(hookInstallStep{}).Satisfied(p) {
			t.Fatal("fixture is not satisfied; fix the test before trusting its assertion")
		}
		steps, _, err := run([]Step{hookInstallStep{}}, p)
		if err != nil || len(steps) != 1 {
			t.Fatalf("run() = %+v, %v", steps, err)
		}
		if want := filepath.ToSlash(filepath.Join(".git", "hooks", "pre-commit")); steps[0].Evidence != want {
			t.Errorf("Evidence = %q, want %q", steps[0].Evidence, want)
		}
	})

	t.Run("agentSkillStep", func(t *testing.T) {
		root := t.TempDir()
		p := project.Project{Root: root, Source: root}
		skillDir := filepath.Join(root, ".claude", "skills", "react-doctor")
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeStepFixtureFile(t, skillDir, "SKILL.md", "# skill\n")
		if !(agentSkillStep{}).Satisfied(p) {
			t.Fatal("fixture is not satisfied; fix the test before trusting its assertion")
		}
		steps, _, err := run([]Step{agentSkillStep{}}, p)
		if err != nil || len(steps) != 1 {
			t.Fatalf("run() = %+v, %v", steps, err)
		}
		// The exact candidate is asserted, not merely the word "present":
		// a mutant inverting os.Stat's error check would skip the real
		// (existing) candidate and fall through to a later, non-existent
		// one — which also renders "<path> present", the wrong path.
		want := filepath.ToSlash(filepath.Join(".claude", "skills", "react-doctor")) + " present"
		if steps[0].Evidence != want {
			t.Errorf("Evidence = %q, want %q", steps[0].Evidence, want)
		}
	})

	t.Run("eslintExtendsStep, spliced regions already match", func(t *testing.T) {
		root := t.TempDir()
		p := project.Project{Root: root, Source: root}
		if _, err := (eslintExtendsStep{}).Apply(p, &Writer{}, io.Discard); err != nil {
			t.Fatalf("Apply() = %v", err)
		}
		if !(eslintExtendsStep{}).Satisfied(p) {
			t.Fatal("fixture is not satisfied; fix the test before trusting its assertion")
		}
		steps, _, err := run([]Step{eslintExtendsStep{}}, p)
		if err != nil || len(steps) != 1 {
			t.Fatalf("run() = %+v, %v", steps, err)
		}
		if strings.Contains(steps[0].Evidence, "ESLint's flat config has no") {
			t.Errorf("Evidence = %q still reuses Describe's truncated fix-instruction text (found live during verification, mirroring the team lead's legacyLintConfigStep example)", steps[0].Evidence)
		}
		if steps[0].Evidence == "" {
			t.Error("Evidence is empty")
		}
	})

	t.Run("architectureStep", func(t *testing.T) {
		root := t.TempDir()
		p := project.Project{Root: root, Source: root}
		if err := os.MkdirAll(filepath.Join(root, project.Dir), 0o755); err != nil {
			t.Fatal(err)
		}
		writeStepFixtureFile(t, filepath.Join(root, project.Dir), ownedFallow, `{"boundaries":[]}`)
		if !(architectureStep{}).Satisfied(p) {
			t.Fatal("fixture is not satisfied; fix the test before trusting its assertion")
		}
		steps, _, err := run([]Step{architectureStep{}}, p)
		if err != nil || len(steps) != 1 {
			t.Fatalf("run() = %+v, %v", steps, err)
		}
		if steps[0].Evidence != "boundaries declared" {
			t.Errorf("Evidence = %q, want %q", steps[0].Evidence, "boundaries declared")
		}
	})
}

// TestDelegatingEslintShapesReportTheirReasonRatherThanSatisfaction is the
// other half of the same rule, asserted through run() rather than against
// Satisfied directly: the three shapes this suite used to file as satisfied
// — a TypeScript config, a legacy-only project, a config that cannot be
// read — now reach report.Delegated and carry the sentence Delegated wrote.
//
// The measured failure this replaces: a fresh create-next-app whose
// eslint.config.mjs jsconfig does not recognise reported `= 6/11 point
// eslint.config.js at the file dharness owns / config shape not recognised`
// under "Already in place (6)", with the plugins step 1 had just installed
// wired to nothing and the run summary reading `8 satisfied · 0 failed`.
//
// The unreadable-config case builds a *directory* named eslint.config.js:
// os.Stat succeeds on it so eslintFlatConfig finds it, and os.ReadFile then
// fails, which is the portable way to force that branch without relying on
// filesystem permissions.
func TestDelegatingEslintShapesReportTheirReasonRatherThanSatisfaction(t *testing.T) {
	cases := []struct {
		name    string
		fixture func(t *testing.T, root string)
		want    string
	}{
		{
			name: "TypeScript config",
			fixture: func(t *testing.T, root string) {
				writeStepFixtureFile(t, root, "eslint.config.ts", "export default [];\n")
			},
			want: "TypeScript",
		},
		{
			name: "legacy-only config",
			fixture: func(t *testing.T, root string) {
				writeStepFixtureFile(t, root, ".eslintrc.json", "{}")
			},
			want: "legacy",
		},
		{
			name: "config could not be read",
			fixture: func(t *testing.T, root string) {
				if err := os.MkdirAll(filepath.Join(root, eslintConfig), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			want: "could not be read",
		},
		{
			name: "shape jsconfig does not recognise",
			fixture: func(t *testing.T, root string) {
				writeStepFixtureFile(t, root, eslintConfig, "export default tseslint.config({});\n")
			},
			want: "not a plain imported identifier",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			p := project.Project{Root: root, Source: root}
			tc.fixture(t, root)

			steps, _, err := run([]Step{eslintExtendsStep{}}, p)
			if err != nil || len(steps) != 1 {
				t.Fatalf("run() = %+v, %v", steps, err)
			}
			if steps[0].Status != report.Delegated {
				t.Fatalf("Status = %q, want %q: an unwired ESLint layer must not be filed as already in place", steps[0].Status, report.Delegated)
			}
			if !strings.Contains(steps[0].Why, tc.want) {
				t.Errorf("Why = %q, want it to contain %q", steps[0].Why, tc.want)
			}
		})
	}
}

// TestSatisfiedBoundariesStepNeverReusesTheFallbackFixInstructions pins
// satisfiedEvidence's own guard: boundariesOwnerStep.Describe's no-collision
// branch is boundariesFallbackDescribe, a fixed instruction for the
// unsatisfied case ("move the zones and rules from... or delete the
// block..."), which must never surface as evidence that the step is
// already satisfied — that would tell a reader to do something they do not
// need to do.
func TestSatisfiedBoundariesStepNeverReusesTheFallbackFixInstructions(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root}
	writeProjectFallow(t, root, `{"extends":["./.dharness/fallow.jsonc"]}`)

	if !(boundariesOwnerStep{}).Satisfied(p) {
		t.Fatal("fixture is not satisfied; fix the test before trusting its assertion")
	}

	steps, _, err := run([]Step{boundariesOwnerStep{}}, p)
	if err != nil {
		t.Fatalf("run() = %v", err)
	}
	if len(steps) != 1 || steps[0].Status != report.Satisfied {
		t.Fatalf("run() = %+v, want exactly one satisfied step", steps)
	}
	if strings.Contains(steps[0].Evidence, "Move the zones and rules") {
		t.Errorf("Evidence = %q, reuses the unsatisfied-case fix instructions as if they were a fact", steps[0].Evidence)
	}
	if steps[0].Evidence == "" {
		t.Error("Evidence is empty for a satisfied step")
	}
}

// stubApplyStep is a minimal Step that is never satisfied and never
// delegated, calling an injected fn so a test can control success or
// failure at a chosen plan index.
type stubApplyStep struct {
	id string
	fn func(*Writer, io.Writer) (Facts, error)
}

func (s stubApplyStep) ID() string                             { return s.id }
func (stubApplyStep) Describe(project.Project) string          { return "" }
func (stubApplyStep) Satisfied(project.Project) bool           { return false }
func (stubApplyStep) Delegated(project.Project) (string, bool) { return "", false }

func (s stubApplyStep) Apply(_ project.Project, w *Writer, out io.Writer) (Facts, error) {
	return s.fn(w, out)
}

// TestFailureRetractsEarlierStepsAndMarksRemainingNotReached pins the
// failure-variant requirement's both scenarios together: step 2 fails in an
// 11-step stub plan, so step 1 (already applied and reported) is retracted
// rather than left standing as applied, and the nine steps after step 2
// that this run never attempted are each not-reached — none of them simply
// absent, and the report's step count still equals the plan's length.
func TestFailureRetractsEarlierStepsAndMarksRemainingNotReached(t *testing.T) {
	ok := func(*Writer, io.Writer) (Facts, error) { return Facts{}, nil }

	plan := make([]Step, 11)
	plan[0] = stubApplyStep{id: "step 1", fn: ok}
	plan[1] = stubApplyStep{id: "step 2", fn: func(*Writer, io.Writer) (Facts, error) {
		return Facts{}, errors.New("step 2 broke")
	}}
	for i := 2; i < 11; i++ {
		plan[i] = stubApplyStep{id: fmt.Sprintf("step %d", i+1), fn: ok}
	}

	steps, notes, err := run(plan, project.Project{})
	if err == nil {
		t.Fatal("run() = nil, want the step 2 failure to surface")
	}
	if notes != nil {
		t.Errorf("run() notes = %+v on failure, want nil — the report's narrative is the caller's job, not a second copy here", notes)
	}
	if len(steps) != 11 {
		t.Fatalf("run() returned %d steps, want 11 — the plan's own length, not merely as far as this run reached", len(steps))
	}

	if steps[0].Status != report.Retracted {
		t.Errorf("step 1 status = %q, want retracted — it is not left standing as applied once step 2 fails", steps[0].Status)
	}
	if steps[1].Status != report.Failed {
		t.Errorf("step 2 status = %q, want failed", steps[1].Status)
	}
	for i := 2; i < 11; i++ {
		if steps[i].Status != report.NotReached {
			t.Errorf("step %d status = %q, want not-reached — never attempted, never silently absent", i+1, steps[i].Status)
		}
	}

	var retracted []string
	for _, s := range steps {
		if s.Status == report.Retracted {
			retracted = append(retracted, s.ID)
		}
	}
	rollback := report.Rollback{Retracted: retracted}
	if !slices.Equal(rollback.Retracted, []string{"step 1"}) {
		t.Errorf("the retracted step names = %v, want [step 1] — what Rollback.Retracted must name", rollback.Retracted)
	}
}

func TestInstallStepPlansOnlyMissingIntegrationPackages(t *testing.T) {
	p, _, _ := integrationProject(t)
	want := []string{RulesPackage}

	if got := integrationPackages(p); !slices.Equal(got, want) {
		t.Fatalf("integrationPackages() = %v, want %v", got, want)
	}
	description := (installStep{}).Describe(p)
	for _, wrapped := range []string{"react-doctor", "fallow", "@stryker-mutator/core", "@stryker-mutator/vitest-runner", "@stryker-mutator/jest-runner"} {
		if strings.Contains(description, wrapped) {
			t.Errorf("install description includes wrapped CLI %q:\n%s", wrapped, description)
		}
	}
	for _, integration := range want {
		if !strings.Contains(description, integration) {
			t.Errorf("install description omits integration %q:\n%s", integration, description)
		}
	}
}

// TestRenderSeedsEmptyForNoSeeds pins the byte-identity path: a project no
// preset seeds gets nothing added to ArchitecturePrompt.
func TestRenderSeedsEmptyForNoSeeds(t *testing.T) {
	if got := renderSeeds(nil); got != "" {
		t.Errorf("renderSeeds(nil) = %q, want empty", got)
	}
}

// TestRenderSeedsRendersExactlyOneSeed is deliberately exercised with one
// seed, not two — every real preset today (nextjs) happens to contribute
// two, which would leave the `len(seeds) > 0` guard indistinguishable from
// `> 1` if this test only ever went through a real preset.
func TestRenderSeedsRendersExactlyOneSeed(t *testing.T) {
	got := renderSeeds([]preset.Seed{{Text: "a structural fact", Because: "a documented observable"}})
	if !strings.Contains(got, "a structural fact") || !strings.Contains(got, "a documented observable") {
		t.Errorf("renderSeeds() = %q, want the seed's text and evidence", got)
	}
}

// TestRenderSeedsFramesConfirmNotDecide pins §21's wording: a seed is
// offered as something to confirm against the tree, never as a zone.
func TestRenderSeedsFramesConfirmNotDecide(t *testing.T) {
	got := renderSeeds([]preset.Seed{{Text: "a structural fact", Because: "a documented observable"}})
	if !strings.Contains(got, "Confirm or correct this against the tree") {
		t.Errorf("renderSeeds() = %q, want the confirm-not-decide framing", got)
	}
}

func TestArchitecturePromptPinsFallowToRemoteLatest(t *testing.T) {
	prompt := (architectureStep{}).Describe(project.Project{PackageManager: "pnpm"})
	for _, invocation := range []string{
		"pnpm dlx fallow@latest list --boundaries",
		"pnpm dlx fallow@latest dead-code --boundary-violations",
	} {
		if !strings.Contains(prompt, invocation) {
			t.Errorf("architecture step description omits %q:\n%s", invocation, prompt)
		}
	}
}

// Undeclared boundaries are Intención: an open decision with no options,
// present in the plan until the agent writes the block.
func TestArchitectureStepDisappearsOnceBoundariesAreDeclared(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root}

	writeFallow(t, root, "{\n}\n")
	if (architectureStep{}).Satisfied(p) {
		t.Error("Satisfied() = true, want false: fallow.jsonc declares no boundaries yet")
	}
	if pending := Pending(p); !containsStep(pending, architectureStep{}) {
		t.Error("architectureStep is missing from Pending() while boundaries is undeclared")
	}

	writeFallow(t, root, "{\n  \"boundaries\": []\n}\n")
	if !(architectureStep{}).Satisfied(p) {
		t.Error("Satisfied() = false, want true: fallow.jsonc now declares boundaries")
	}
	if pending := Pending(p); containsStep(pending, architectureStep{}) {
		t.Error("architectureStep is still in Pending() once boundaries is declared")
	}
}

func containsStep(steps []Step, target Step) bool {
	for _, step := range steps {
		if step.ID() == target.ID() {
			return true
		}
	}
	return false
}

func writeFallow(t *testing.T, root, contents string) {
	t.Helper()
	dir := filepath.Join(root, project.Dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ownedFallow), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMCPConfigRunsTheBundledBinaryFromFallowLatest(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root, PackageManager: "pnpm"}
	if _, err := (mcpStep{}).Apply(p, &Writer{}, io.Discard); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, mcpConfig))
	if err != nil {
		t.Fatal(err)
	}
	var config mcpConfigFile
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	server := config.Servers["fallow"]
	wantArgs := []string{"--package=fallow@latest", "dlx", "fallow-mcp"}
	if server.Command != "pnpm" || !slices.Equal(server.Args, wantArgs) {
		t.Errorf("fallow MCP command = %s %v, want pnpm %v", server.Command, server.Args, wantArgs)
	}
}

func TestApplySuccessDoesNotCompensateIntegrationInstall(t *testing.T) {
	p, _, _ := integrationProject(t)
	var commands []runner.Command
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		commands = append(commands, cmd)
		return nil
	}))

	if err := Apply(p, io.Discard); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	if len(commands) != 1 {
		t.Fatalf("Apply() ran %d package commands, want only install: %+v", len(commands), commands)
	}
	want := []string{"install", "--save-dev", RulesPackage}
	if commands[0].Name != "npm" || !slices.Equal(commands[0].Args, want) {
		t.Errorf("install command = %s %v, want npm %v", commands[0].Name, commands[0].Args, want)
	}
}

// TestInstallStepAppliesReportsTheResolvedVersion pins gap 3 from the team
// lead's measured run: Facts.Installed must carry the version the package
// manager actually resolved into package.json, not the bare package name —
// "installed dharness-eslint-plugin" says nothing about what actually
// landed. Measured, not fabricated: read back from package.json after the
// (stubbed) install writes it, the same file installStep's own snapshot
// already watches for rollback.
func TestInstallStepAppliesReportsTheResolvedVersion(t *testing.T) {
	p, _, _ := integrationProject(t)

	t.Cleanup(runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		if !slices.Contains(cmd.Args, RulesPackage) {
			return nil
		}
		var pkg map[string]any
		raw, err := os.ReadFile(filepath.Join(p.Source, "package.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &pkg); err != nil {
			t.Fatal(err)
		}
		devDeps, _ := pkg["devDependencies"].(map[string]any)
		if devDeps == nil {
			devDeps = map[string]any{}
		}
		devDeps[RulesPackage] = "^0.3.0"
		pkg["devDependencies"] = devDeps
		encoded, err := json.Marshal(pkg)
		if err != nil {
			t.Fatal(err)
		}
		return os.WriteFile(filepath.Join(p.Source, "package.json"), encoded, 0o600)
	}))

	facts, err := (installStep{}).Apply(p, &Writer{}, io.Discard)
	if err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	want := RulesPackage + "@0.3.0"
	if !slices.Contains(facts.Installed, want) {
		t.Errorf("Installed = %v, want %q with its caret-free resolved version", facts.Installed, want)
	}
}

// TestInstallStepAppliesFallsBackToTheBareNameWhenUnmeasured pins the other
// half: when package.json cannot be read back (or never gained the entry —
// a stubbed test double that never wrote it, matching an install that ran
// against a package.json this step cannot re-read for any reason), Facts
// still names the package by its bare name — never a fabricated version.
func TestInstallStepAppliesFallsBackToTheBareNameWhenUnmeasured(t *testing.T) {
	p, _, _ := integrationProject(t)
	t.Cleanup(runner.SetForTest(func(runner.Command, io.Writer, io.Writer) error { return nil }))

	facts, err := (installStep{}).Apply(p, &Writer{}, io.Discard)
	if err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if !slices.Contains(facts.Installed, RulesPackage) {
		t.Errorf("Installed = %v, want the bare package name %q with no version measured", facts.Installed, RulesPackage)
	}
	for _, name := range facts.Installed {
		if strings.Contains(name, "@") {
			t.Errorf("Installed = %v carries a fabricated version for %q", facts.Installed, name)
		}
	}
}

// TestInstalledWithVersionsTreatsALiteralEmptyVersionAsUnmeasured is a
// mutation guard for the `!ok || version == ""` check: package.json
// declaring a package against a literal empty-string version (syntactically
// possible, if nonsensical) must fall back to the bare name — not render
// "name@" with nothing after the @, which `!ok || false` would produce
// since the package IS present (ok == true).
func TestInstalledWithVersionsTreatsALiteralEmptyVersionAsUnmeasured(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"devDependencies":{"`+RulesPackage+`":""}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	p := project.Project{Root: root, Source: root}

	got := installedWithVersions(p, []string{RulesPackage})
	if !slices.Equal(got, []string{RulesPackage}) {
		t.Errorf("installedWithVersions() = %v, want %v (empty version treated as unmeasured)", got, []string{RulesPackage})
	}
}

// TestInstalledWithVersionsContinuesPastAnUnresolvedPackage is a mutation
// guard for the loop's `continue` (not `break`): when an earlier package in
// the list has no matching package.json entry, a later package in the same
// list must still get its own resolved version — `break` would abandon the
// rest of the slice, leaving every later package unresolved too.
func TestInstalledWithVersionsContinuesPastAnUnresolvedPackage(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"devDependencies":{"second-package":"1.2.3"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	p := project.Project{Root: root, Source: root}

	got := installedWithVersions(p, []string{"first-package-not-in-package-json", "second-package"})
	want := []string{"first-package-not-in-package-json", "second-package@1.2.3"}
	if !slices.Equal(got, want) {
		t.Errorf("installedWithVersions() = %v, want %v", got, want)
	}
}

func TestApplyCompensatesRulesPackageAndRestoresPackageFiles(t *testing.T) {
	p, packageJSON, lockfile := integrationProject(t)
	if err := os.WriteFile(filepath.Join(p.Root, project.Dir), []byte("blocks the next step"), 0o600); err != nil {
		t.Fatal(err)
	}

	var commands []runner.Command
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		commands = append(commands, cmd)
		switch {
		case slices.Equal(cmd.Args, []string{"install", "--save-dev", RulesPackage}):
			writePackageState(t, p.Source, "changed by install", "changed lock")
			return os.MkdirAll(filepath.Join(p.Source, "node_modules", RulesPackage), 0o755)
		case slices.Equal(cmd.Args, []string{"uninstall", "--save-dev", RulesPackage}):
			writePackageState(t, p.Source, "changed by uninstall", "changed again")
			return os.RemoveAll(filepath.Join(p.Source, "node_modules", RulesPackage))
		default:
			return errors.New("unexpected package command")
		}
	}))

	err := Apply(p, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "write the files dharness owns failed") {
		t.Fatalf("Apply() = %v, want the later step failure", err)
	}
	if len(commands) != 2 {
		t.Fatalf("Apply() ran %d package commands, want install and compensation: %+v", len(commands), commands)
	}
	assertPackageState(t, p.Source, packageJSON, lockfile)
	if _, err := os.Stat(filepath.Join(p.Source, "node_modules", RulesPackage)); !os.IsNotExist(err) {
		t.Errorf("dependency added by this run survived compensation: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p.Source, "node_modules", "pre-existing-integration")); err != nil {
		t.Errorf("pre-existing dependency was removed: %v", err)
	}
}

func TestApplyCompensatesPartialInstallFailure(t *testing.T) {
	p, packageJSON, lockfile := integrationProject(t)
	installErr := errors.New("install failed after changing files")
	var commands []runner.Command
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		commands = append(commands, cmd)
		if cmd.Args[0] == "install" {
			writePackageState(t, p.Source, "partially changed", "partial lock")
			for _, path := range []string{filepath.Join(p.Source, "node_modules", RulesPackage)} {
				if err := os.MkdirAll(path, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			return installErr
		}
		for _, path := range []string{filepath.Join(p.Source, "node_modules", RulesPackage)} {
			if err := os.RemoveAll(path); err != nil {
				t.Fatal(err)
			}
		}
		return nil
	}))

	err := Apply(p, io.Discard)
	if !errors.Is(err, installErr) {
		t.Fatalf("Apply() = %v, want install failure", err)
	}
	if len(commands) != 2 || commands[1].Args[0] != "uninstall" {
		t.Fatalf("partial install was not compensated: %+v", commands)
	}
	assertPackageState(t, p.Source, packageJSON, lockfile)
}

// TestInstallIncludesPresetContributedPackages pins integrationPackages(p)
// becoming preset-aware (design decision 7): a matched preset's Layer
// package joins the fixed RulesPackage set, for both presets that
// contribute one today.
func TestInstallIncludesPresetContributedPackages(t *testing.T) {
	cases := []struct {
		name    string
		project func(t *testing.T) (project.Project, []byte, []byte)
		want    string
	}{
		{"nextjs", nextjsIntegrationProject, "eslint-config-next"},
		{"expo", expoIntegrationProject, "eslint-config-expo"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, _, _ := tc.project(t)
			got := integrationPackages(p)
			if !slices.Contains(got, RulesPackage) {
				t.Errorf("integrationPackages() = %v, want %q present", got, RulesPackage)
			}
			if !slices.Contains(got, tc.want) {
				t.Errorf("integrationPackages() = %v, want %q contributed by the matched preset", got, tc.want)
			}
		})
	}
}

// TestFailedInstallRollsBackOnlyWhatThisRunAdded is spec.md's own scenario
// title: a Next.js-matched project whose installStep.Apply fails partway
// through must roll back exactly the packages this run's own
// integrationPackages(p) added — RulesPackage and the preset-contributed
// eslint-config-next together — through the existing snapshot-and-compensate
// mechanism, with no new rollback path for preset-contributed packages
// (design decision 7).
func TestFailedInstallRollsBackOnlyWhatThisRunAdded(t *testing.T) {
	p, packageJSON, lockfile := nextjsIntegrationProject(t)
	installErr := errors.New("install failed after changing files")
	var commands []runner.Command
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		commands = append(commands, cmd)
		if cmd.Args[0] == "install" {
			writePackageState(t, p.Source, "partially changed", "partial lock")
			return installErr
		}
		return nil
	}))

	err := Apply(p, io.Discard)
	if !errors.Is(err, installErr) {
		t.Fatalf("Apply() = %v, want install failure", err)
	}
	if len(commands) != 2 || commands[1].Args[0] != "uninstall" {
		t.Fatalf("partial install was not compensated: %+v", commands)
	}
	for _, cmd := range commands {
		for _, pkg := range []string{RulesPackage, "eslint-config-next"} {
			if !slices.Contains(cmd.Args, pkg) {
				t.Errorf("command %v did not carry package %q", cmd.Args, pkg)
			}
		}
	}
	assertPackageState(t, p.Source, packageJSON, lockfile)
}

// Writer.Undo restores files it snapshotted, but not directories created by
// os.MkdirAll nor the .gitignore project.Project.EnsureDir writes outside the
// Writer. The rollback report must not claim more than Undo actually did.
func TestApplyRollbackNamesWhatWasUndoneNotEverything(t *testing.T) {
	p, _, _ := integrationProject(t)
	if err := os.WriteFile(filepath.Join(p.Root, project.Dir), []byte("blocks the next step"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		return nil
	}))

	err := Apply(p, io.Discard)
	if err == nil {
		t.Fatal("Apply() = nil, want the write-the-files-dharness-owns failure")
	}
	for _, overclaim := range []string{"everything this run wrote was undone", "the repository was fully restored"} {
		if strings.Contains(err.Error(), overclaim) {
			t.Errorf("Apply() error claims more than Undo covers: %q contains %q", err, overclaim)
		}
	}
	if !strings.Contains(err.Error(), "put back") {
		t.Errorf("Apply() error does not say what was undone: %q", err)
	}
}

func TestApplyReportsTheOriginalAndCompensationFailures(t *testing.T) {
	p, _, _ := integrationProject(t)
	installErr := errors.New("install failed")
	compensationErr := errors.New("uninstall failed")
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		if cmd.Args[0] == "install" {
			return installErr
		}
		return compensationErr
	}))

	err := Apply(p, io.Discard)
	if !errors.Is(err, installErr) || !errors.Is(err, compensationErr) {
		t.Fatalf("Apply() = %v, want both install and compensation failures", err)
	}
}

func integrationProject(t *testing.T) (project.Project, []byte, []byte) {
	t.Helper()
	root := t.TempDir()
	packageJSON := []byte(`{"devDependencies":{"vitest":"^4.0.0"}}`)
	lockfile := []byte(`{"lockfileVersion":3}`)
	if err := os.WriteFile(filepath.Join(root, "package.json"), packageJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package-lock.json"), lockfile, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "node_modules", "pre-existing-integration"), 0o755); err != nil {
		t.Fatal(err)
	}
	return project.Project{Root: root, Source: root, PackageManager: "npm", TestRunner: "vitest"}, packageJSON, lockfile
}

// nextjsIntegrationProject is integrationProject's counterpart for a
// project the nextjs preset matches: package.json declares "next", so
// integrationPackages(p) includes eslint-config-next alongside RulesPackage
// (design decision 7).
func nextjsIntegrationProject(t *testing.T) (project.Project, []byte, []byte) {
	t.Helper()
	root := t.TempDir()
	packageJSON := []byte(`{"dependencies":{"next":"^14.0.0"},"devDependencies":{"vitest":"^4.0.0"}}`)
	lockfile := []byte(`{"lockfileVersion":3}`)
	if err := os.WriteFile(filepath.Join(root, "package.json"), packageJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package-lock.json"), lockfile, 0o600); err != nil {
		t.Fatal(err)
	}
	return project.Project{Root: root, Source: root, PackageManager: "npm", TestRunner: "vitest"}, packageJSON, lockfile
}

// expoIntegrationProject is nextjsIntegrationProject's counterpart for the
// expo preset.
func expoIntegrationProject(t *testing.T) (project.Project, []byte, []byte) {
	t.Helper()
	root := t.TempDir()
	packageJSON := []byte(`{"dependencies":{"expo":"~51.0.0"},"devDependencies":{"vitest":"^4.0.0"}}`)
	lockfile := []byte(`{"lockfileVersion":3}`)
	if err := os.WriteFile(filepath.Join(root, "package.json"), packageJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package-lock.json"), lockfile, 0o600); err != nil {
		t.Fatal(err)
	}
	return project.Project{Root: root, Source: root, PackageManager: "npm", TestRunner: "vitest"}, packageJSON, lockfile
}

func writePackageState(t *testing.T, root, packageJSON, lockfile string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(packageJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package-lock.json"), []byte(lockfile), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertPackageState(t *testing.T, root string, packageJSON, lockfile []byte) {
	t.Helper()
	gotPackageJSON, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil || !bytes.Equal(gotPackageJSON, packageJSON) {
		t.Errorf("package.json = %q, %v; want exact original %q", gotPackageJSON, err, packageJSON)
	}
	gotLockfile, err := os.ReadFile(filepath.Join(root, "package-lock.json"))
	if err != nil || !bytes.Equal(gotLockfile, lockfile) {
		t.Errorf("package-lock.json = %q, %v; want exact original %q", gotLockfile, err, lockfile)
	}
}

// A project that already wrote its own .fallowrc.json owns that file: adding
// a key to it is a merge, not a write dharness gets to make on its own.
func TestFallowExtendsIsDelegatedWhenTheProjectOwnsTheConfig(t *testing.T) {
	root := t.TempDir()
	original := []byte(`{"custom":true}`)
	if err := os.WriteFile(filepath.Join(root, fallowConfig), original, 0o600); err != nil {
		t.Fatal(err)
	}
	p := project.Project{Root: root, Source: root}

	why, ok := (fallowExtendsStep{}).Delegated(p)
	if !ok {
		t.Fatal("Delegated() ok = false, want true when the project's own config already exists")
	}
	if !strings.Contains(why, fallowConfig) {
		t.Errorf("Delegated() why = %q, want it to name %s", why, fallowConfig)
	}

	if _, err := applySteps([]Step{fallowExtendsStep{}}, p, io.Discard); err != nil {
		t.Fatalf("applySteps() = %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, fallowConfig))
	if err != nil || !bytes.Equal(raw, original) {
		t.Errorf("the project's own config was touched: %q, %v", raw, err)
	}
}

// Same rule for lefthook.yml, which belongs to the repository rather than the
// JS project.
func TestLefthookExtendsIsDelegatedWhenTheProjectOwnsTheConfig(t *testing.T) {
	root := t.TempDir()
	original := []byte("custom: true\n")
	if err := os.WriteFile(filepath.Join(root, lefthookConfig), original, 0o600); err != nil {
		t.Fatal(err)
	}
	p := project.Project{Root: root, Source: root}

	why, ok := (lefthookExtendsStep{}).Delegated(p)
	if !ok {
		t.Fatal("Delegated() ok = false, want true when the project's own lefthook config already exists")
	}
	if !strings.Contains(why, lefthookConfig) {
		t.Errorf("Delegated() why = %q, want it to name %s", why, lefthookConfig)
	}

	if _, err := applySteps([]Step{lefthookExtendsStep{}}, p, io.Discard); err != nil {
		t.Fatalf("applySteps() = %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, lefthookConfig))
	if err != nil || !bytes.Equal(raw, original) {
		t.Errorf("the project's own config was touched: %q, %v", raw, err)
	}
}

// With no JS project to adopt, there is no .fallowrc.json to point anywhere,
// so the step is satisfied without asking — the mutation gate for this line
// distinguishes "no source" from "source exists but not wired", which only a
// project with an empty Source exercises.
func TestFallowExtendsSatisfiedWhenTheProjectHasNoSource(t *testing.T) {
	p := project.Project{Root: t.TempDir()}
	if !(fallowExtendsStep{}).Satisfied(p) {
		t.Error("Satisfied() = false, want true when there is no JS project to adopt")
	}
}

// Same rule for lefthook: with no hook manager answering lefthook at all,
// there is nothing to wire, so the step is satisfied without asking.
func TestLefthookExtendsSatisfiedWhenLefthookIsNotTheHookManager(t *testing.T) {
	p := project.Project{Root: t.TempDir()}
	if !(lefthookExtendsStep{}).Satisfied(p) {
		t.Error("Satisfied() = false, want true when lefthook does not answer for this project")
	}
}

// installStep has nothing to install without a JS project: there is no
// package manager to ask.
func TestInstallStepSatisfiedWhenTheProjectHasNoSource(t *testing.T) {
	p := project.Project{Root: t.TempDir()}
	if !(installStep{}).Satisfied(p) {
		t.Error("Satisfied() = false, want true when there is no JS project to install into")
	}
}

// A directory under node_modules is an install artifact, not a declaration.
// It survives a rollback that restored package.json, it is absent under Yarn
// PnP and pnpm's store, and on its own it never meant the package was
// declared. dharness does not read it: the install command is the one that
// decides, so with a JS project present the step always runs.
func TestInstallStepRunsEvenWithThePackageSittingInNodeModules(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "node_modules", RulesPackage), 0o755); err != nil {
		t.Fatal(err)
	}
	p := project.Project{Root: root, Source: root, PackageManager: "bun"}

	if (installStep{}).Satisfied(p) {
		t.Error("Satisfied() = true from a node_modules directory alone; the install command is what decides")
	}
}

// With no project config at all, dharness writes the whole thing itself —
// there is nothing to merge into.
func TestExtendsStepsWriteTheirFileWhenTheProjectHasNone(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root}

	if why, ok := (fallowExtendsStep{}).Delegated(p); ok {
		t.Fatalf("Delegated() = %q, true; want ok=false with no config present", why)
	}
	if why, ok := (lefthookExtendsStep{}).Delegated(p); ok {
		t.Fatalf("Delegated() = %q, true; want ok=false with no config present", why)
	}

	if _, err := applySteps([]Step{fallowExtendsStep{}, lefthookExtendsStep{}}, p, io.Discard); err != nil {
		t.Fatalf("applySteps() = %v", err)
	}

	fallowRaw, err := os.ReadFile(filepath.Join(root, fallowConfig))
	if err != nil || !strings.Contains(string(fallowRaw), "extends") {
		t.Errorf("%s was not written: %q, %v", fallowConfig, fallowRaw, err)
	}
	lefthookRaw, err := os.ReadFile(filepath.Join(root, lefthookConfig))
	if err != nil || !strings.Contains(string(lefthookRaw), "extends") {
		t.Errorf("%s was not written: %q, %v", lefthookConfig, lefthookRaw, err)
	}
}

// What enables a project is a hook manager that answers, not membership in a
// list: writing a lefthook file into a husky project creates configuration
// nothing reads.
func TestHookManagerAnswersRatherThanBeingAssumed(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  manager
	}{
		{"lefthook by config", []string{"lefthook.yml"}, managerLefthook},
		{"lefthook by dotted config", []string{".lefthook.yaml"}, managerLefthook},
		{"husky by directory", []string{filepath.Join(".husky", "pre-commit")}, managerHusky},
		{"nothing answers", nil, managerNone},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			for _, name := range testCase.files {
				path := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}

			if got := hookManager(project.Project{Root: root}); got != testCase.want {
				t.Errorf("hookManager() = %v, want %v", got, testCase.want)
			}
		})
	}
}

// With no manager answering, choosing one for this project is a decision
// dharness does not get to make, so the step is not satisfied — a delegated
// step blocks nothing else in the plan, so satisfying it artificially is no
// longer needed to keep the run moving.
func TestGateStepIsAnOpenDecisionWhenNoManagerAnswers(t *testing.T) {
	p := project.Project{Root: t.TempDir()}

	if (hookInstallStep{}).Satisfied(p) {
		t.Error("Satisfied() = true, want false: no hook manager answers here")
	}

	why, ok := (hookInstallStep{}).Delegated(p)
	if !ok {
		t.Fatal("Delegated() ok = false, want true: no manager means an open decision")
	}
	if why == "" {
		t.Error("Delegated() why is empty; an open decision handed to the agent needs a reason")
	}
}

// The thresholds live in a file because react-doctor accepts only a severity,
// so a rule cannot carry its own number.
func TestOwnedFilesCarryTheThresholdsTheRulesCannot(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root}

	if _, err := (ownedFilesStep{}).Apply(p, &Writer{}, io.Discard); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, project.Dir, ownedRules))
	if err != nil {
		t.Fatalf("the thresholds file was not written: %v", err)
	}
	if !strings.Contains(string(raw), `"maxFileLines": 500`) {
		t.Errorf("thresholds do not carry the file ceiling:\n%s", raw)
	}

	// The architecture is deliberately absent: it is decided, not detected.
	architecture, err := os.ReadFile(filepath.Join(root, project.Dir, ownedFallow))
	if err != nil {
		t.Fatal(err)
	}
	// The quoted key, not the bare word: the file's comment says where to
	// declare boundaries, and saying where is not declaring one. Same
	// discriminator declaredKeys uses.
	if strings.Contains(string(architecture), `"boundaries"`) {
		t.Errorf("dharness declared an architecture it cannot know:\n%s", architecture)
	}
}

// gateInstalled and huskyWired both answer "will git actually run the gate",
// which is a different question from whether some file mentions it. Each one
// reads a file that may not exist, and a missing file is a "no", never a
// "yes" reached by skipping the read.
func TestHookDetectionReadsTheFileAndSaysNoWhenItIsAbsent(t *testing.T) {
	for _, hook := range []struct {
		name    string
		path    string
		wired   string
		unwired string
		answer  func(project.Project) bool
	}{
		{
			name:    "lefthook writes the git hook",
			path:    filepath.Join(".git", "hooks", "pre-commit"),
			wired:   "#!/bin/sh\nlefthook run pre-commit\n",
			unwired: "#!/bin/sh\necho something else\n",
			answer:  gateInstalled,
		},
		{
			name:    "husky keeps a shell script",
			path:    filepath.FromSlash(huskyHook),
			wired:   gateCommand + "\n",
			unwired: "npm test\n",
			answer:  huskyWired,
		},
	} {
		t.Run(hook.name, func(t *testing.T) {
			root := t.TempDir()
			p := project.Project{Root: root, Source: root}

			if hook.answer(p) {
				t.Error("answered true with no hook file at all")
			}

			full := filepath.Join(root, hook.path)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(hook.unwired), 0o600); err != nil {
				t.Fatal(err)
			}
			if hook.answer(p) {
				t.Error("answered true for a hook that does not invoke the gate")
			}

			if err := os.WriteFile(full, []byte(hook.wired), 0o600); err != nil {
				t.Fatal(err)
			}
			if !hook.answer(p) {
				t.Error("answered false for a hook that does invoke the gate")
			}
		})
	}
}

// husky's hook belongs to the project, so the gate is appended rather than
// written over. The separator is the whole subtlety: a script that already
// ends in a newline must not gain a blank line, and one that does not must
// not have the gate welded onto its last command.
func TestAppendHuskyGateKeepsTheScriptAndSeparatesTheGate(t *testing.T) {
	for _, script := range []struct {
		name     string
		existing string
		want     string
	}{
		{name: "no script yet", existing: "", want: gateCommand + "\n"},
		{name: "ends in a newline", existing: "npm test\n", want: "npm test\n" + gateCommand + "\n"},
		{name: "ends without one", existing: "npm test", want: "npm test\n" + gateCommand + "\n"},
	} {
		t.Run(script.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, filepath.FromSlash(huskyHook))
			if script.existing != "" {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(script.existing), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			if err := appendHuskyGate(project.Project{Root: root}, &Writer{}); err != nil {
				t.Fatalf("appendHuskyGate() = %v", err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != script.want {
				t.Errorf("husky hook = %q, want %q", raw, script.want)
			}
		})
	}
}

// A hook path that cannot be read is not the same as one that is absent, and
// only the second is a "nothing here yet". Reading a directory as a file is
// the portable way to produce the first.
func TestAppendHuskyGateFailsOnAHookItCannotRead(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(huskyHook)), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := appendHuskyGate(project.Project{Root: root}, &Writer{}); err == nil {
		t.Error("appendHuskyGate() = nil for a hook path that is not a readable file")
	}
}

// A rule dharness turns off has to say so where the decision is made, or it
// is a silent default. The architecture prompt names the rule, the file and
// the exact edit.
func TestArchitecturePromptSaysHowToTurnOnTheBarrelRule(t *testing.T) {
	prompt := (architectureStep{}).Describe(project.Project{
		Root: "/repo", Source: "/repo/frontend", PackageManager: "bun",
	})

	for _, expected := range []string{
		"frontend/" + eslintConfig,
		`"dharness/folder-ownership": "error",`,
		"index.ts",
	} {
		if !strings.Contains(prompt, expected) {
			t.Errorf("the architecture prompt omits %q:\n%s", expected, prompt)
		}
	}
}

// fallow's `extends` replaces a key rather than merging it, so a `boundaries`
// block in the project's own config discards the one dharness owns — with no
// error and no warning. The wiring still looks correct, which is what makes
// it worth a step of its own.
func TestProjectBoundariesAreReportedBecauseTheyReplaceTheOwnedOnes(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root}

	// Nothing declared: the owned file is the only architecture.
	writeProjectFallow(t, root, `{"extends":["./.dharness/fallow.jsonc"]}`)
	if !(boundariesOwnerStep{}).Satisfied(p) {
		t.Error("Satisfied() = false with no boundaries in the project's own config")
	}

	// The word in a comment is not a declaration. This project's real config
	// carries exactly this sentence, and a substring check would fire on it.
	writeProjectFallow(t, root,
		"{\n  // Architecture boundaries live in the file dharness owns.\n  \"extends\": [\"./.dharness/fallow.jsonc\"]\n}")
	if !(boundariesOwnerStep{}).Satisfied(p) {
		t.Error("Satisfied() = false for the word 'boundaries' inside a comment")
	}

	// A real declaration: fallow keeps this one and drops the owned block.
	writeProjectFallow(t, root,
		`{"extends":["./.dharness/fallow.jsonc"],"boundaries":{"zones":[]}}`)
	if (boundariesOwnerStep{}).Satisfied(p) {
		t.Error("Satisfied() = true while the project declares boundaries of its own")
	}

	why, ok := (boundariesOwnerStep{}).Delegated(p)
	if !ok {
		t.Fatal("Delegated() = false; dharness cannot merge two architectures")
	}
	if !strings.Contains(why, "replaces") {
		t.Errorf("the reason does not say the owned block is replaced:\n%s", why)
	}
}

// Without a JS project there is no config to conflict with — and asking is
// not merely pointless, it is unsafe. Project.Source is empty, so the joined
// path is `.fallowrc.json` relative to the working directory: dropping the
// HasSource guard makes dharness read whatever config happens to sit where
// the process was started. The chdir is what makes that observable.
func TestBoundariesOwnerStepIsSatisfiedWithoutASource(t *testing.T) {
	elsewhere := t.TempDir()
	if err := os.WriteFile(filepath.Join(elsewhere, fallowConfig), []byte(`{"boundaries":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(elsewhere)

	if !(boundariesOwnerStep{}).Satisfied(project.Project{Root: t.TempDir()}) {
		t.Error("Satisfied() = false with no JS project: an unrelated config was read from the working directory")
	}
}

// The same quoting rule guards the owned file: an agent that writes a comment
// about boundaries before writing the block has not declared one yet.
func TestArchitectureStepIgnoresBoundariesInAComment(t *testing.T) {
	root := t.TempDir()
	writeFallow(t, root, "{\n  // boundaries go here once the analysis is done\n}\n")
	if (architectureStep{}).Satisfied(project.Project{Root: root}) {
		t.Error("Satisfied() = true for the word 'boundaries' inside a comment")
	}
}

func writeProjectFallow(t *testing.T, source, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(source, fallowConfig), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

// react-doctor adopts .eslintrc.json and runs the rules it declares. Measured
// against react-doctor 0.5.7: a valid one reports `eslint/no-console` and
// exits 1; the same file with a trailing comma drops those findings and exits
// 0, and says nothing about the file even under --verbose.
//
// That is dharness's gate quietly losing rules it was enforcing, which is why
// the step exists. Only this one format is adopted — .eslintrc.cjs, .yml,
// package.json#eslintConfig and flat config were all measured and ignored —
// so the check is one file, not a family.
func TestBrokenLegacyESLintConfigIsReportedBecauseReactDoctorAdoptsIt(t *testing.T) {
	newProject := func(t *testing.T) (project.Project, string) {
		t.Helper()
		root := t.TempDir()
		return project.Project{Root: root, Source: root}, root
	}

	t.Run("no legacy config", func(t *testing.T) {
		p, _ := newProject(t)
		if !(legacyLintConfigStep{}).Satisfied(p) {
			t.Error("Satisfied() = false with no .eslintrc.json to adopt")
		}
	})

	t.Run("valid legacy config", func(t *testing.T) {
		p, root := newProject(t)
		writeProjectSource(t, root, legacyLintConfig, `{"rules":{"no-console":"error"}}`)
		if !(legacyLintConfigStep{}).Satisfied(p) {
			t.Error("Satisfied() = false for a config react-doctor can read")
		}
	})

	t.Run("broken legacy config", func(t *testing.T) {
		p, root := newProject(t)
		writeProjectSource(t, root, legacyLintConfig, `{"rules":{"no-console":"error",}`)

		if (legacyLintConfigStep{}).Satisfied(p) {
			t.Error("Satisfied() = true for a config react-doctor silently drops")
		}
		why, ok := (legacyLintConfigStep{}).Delegated(p)
		if !ok {
			t.Fatal("Delegated() = false; the project's own lint config is not dharness's to rewrite")
		}
		if !strings.Contains(why, "exits 0") {
			t.Errorf("the reason does not name the silence that makes this worth reporting:\n%s", why)
		}
	})

	// Flat config is the ESLint 9 default and react-doctor ignores it, so a
	// project on flat config has nothing to report however broken it is.
	t.Run("flat config is not adopted", func(t *testing.T) {
		p, root := newProject(t)
		writeProjectSource(t, root, "eslint.config.js", "export default [ this is not javascript")
		if !(legacyLintConfigStep{}).Satisfied(p) {
			t.Error("Satisfied() = false for flat config, which react-doctor never reads")
		}
	})

	t.Run("no JS project", func(t *testing.T) {
		if !(legacyLintConfigStep{}).Satisfied(project.Project{Root: t.TempDir()}) {
			t.Error("Satisfied() = false with no JS project")
		}
	})
}

func writeProjectSource(t *testing.T, root, rel, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
