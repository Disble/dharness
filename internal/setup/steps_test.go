package setup

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/preset"
	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/runner"
)

// stubMatch builds a preset.Match with one fact, for tests that need to
// simulate a matched preset without depending on the real registry — which,
// until slice 5 registers a framework preset, contributes nothing beyond
// generic's empty manifest.
func stubMatch(id string, scope preset.Scope, key string, value any, because string) preset.Match {
	return preset.Match{ID: id, Scope: scope, Manifest: preset.Manifest{
		Schema: preset.Schema,
		Facts:  []preset.Fact{{Key: key, Value: value, Because: because}},
	}}
}

// stepIDPresent reports whether steps contains one whose ID matches id —
// used to assert a step's presence or absence in Pending() without depending
// on Plan()'s exact index.
func stepIDPresent(steps []Step, id string) bool {
	for _, step := range steps {
		if step.ID() == id {
			return true
		}
	}
	return false
}

// TestCollisionNamesEveryContributedKeyTheProjectDeclares pins the widened
// step's whole contract: every colliding key is named, with both the
// preset's contributed value and the project's own declared value, and a
// key the project never declared is never mentioned. entryPoints is a
// stand-in key for this test only — no real preset contributes it (see
// CLAUDE.md's own recorded lesson about that key).
func TestCollisionNamesEveryContributedKeyTheProjectDeclares(t *testing.T) {
	root := t.TempDir()
	// The project's own value (dist/**) is deliberately different from the
	// preset's (wailsjs/**) and from the unrelated stand-in preset's
	// (src/main.ts), so an assertion that finds one cannot be satisfied by
	// mistaking it for another — the fixture two identical values would
	// produce cannot tell collidingKeys/ownedValue/declaredValue apart.
	writeProjectFallow(t, root, `{"ignorePatterns": ["dist/**"]}`)

	matches := []preset.Match{
		stubMatch("wails", preset.Root, "ignorePatterns", []string{"wailsjs/**"}, "wails.json"),
		stubMatch("stub", preset.Root, "entryPoints", []string{"src/main.ts"}, "test stand-in"),
	}

	colliding := collidingKeys(root, matches)
	if len(colliding) != 1 || colliding[0] != "ignorePatterns" {
		t.Fatalf("collidingKeys() = %v, want exactly [ignorePatterns]", colliding)
	}

	p := project.Project{Root: root, Source: root}

	why := delegateBoundaries(p, colliding, matches)
	if !strings.Contains(why, "ignorePatterns") {
		t.Errorf("Delegated() why does not name ignorePatterns:\n%s", why)
	}
	if !strings.Contains(why, `["wailsjs/**"]`) {
		t.Errorf("Delegated() why does not show the preset's contributed value:\n%s", why)
	}
	if !strings.Contains(why, `"dist/**"`) {
		t.Errorf("Delegated() why does not show the project's own declared value:\n%s", why)
	}
	if strings.Contains(why, "entryPoints") || strings.Contains(why, "src/main.ts") {
		t.Errorf("Delegated() why mentions entryPoints, which the project never declared:\n%s", why)
	}

	describe := describeBoundaries(p, colliding, matches)
	if !strings.Contains(describe, `["wailsjs/**"]`) {
		t.Errorf("Describe() does not show the preset's contributed value:\n%s", describe)
	}
	if !strings.Contains(describe, `"ignorePatterns": ["dist/**"]`) {
		t.Errorf("Describe() does not show the project's own declared value:\n%s", describe)
	}
	if strings.Contains(describe, "entryPoints") || strings.Contains(describe, "src/main.ts") {
		t.Errorf("Describe() mentions entryPoints, which the project never declared:\n%s", describe)
	}
}

func TestQuotedKeysBackticksEachKey(t *testing.T) {
	got := quotedKeys([]string{"boundaries", "ignorePatterns"})
	want := "`boundaries`, `ignorePatterns`"
	if got != want {
		t.Errorf("quotedKeys() = %q, want %q", got, want)
	}
}

// TestBoundariesAloneStillCollidesUnchanged is the regression guard: with no
// preset matched beyond generic (the real registry, contributing nothing),
// the widened step must behave exactly as boundariesOwnerStep did before
// this slice for the one key it has always checked.
func TestBoundariesAloneStillCollidesUnchanged(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root}
	writeProjectFallow(t, root, `{"extends":["./.dharness/fallow.jsonc"],"boundaries":{"zones":[]}}`)

	if (boundariesOwnerStep{}).Satisfied(p) {
		t.Error("Satisfied() = true while the project declares boundaries of its own")
	}

	why, ok := (boundariesOwnerStep{}).Delegated(p)
	if !ok {
		t.Fatal("Delegated() = false; dharness cannot merge two values for one key")
	}
	if !strings.Contains(why, "replaces") {
		t.Errorf("the reason does not say the project's value replaces dharness's:\n%s", why)
	}
	if !strings.Contains(why, "boundaries") {
		t.Errorf("the reason does not name the colliding key:\n%s", why)
	}

	describe := (boundariesOwnerStep{}).Describe(p)
	if !strings.Contains(describe, "the architecture block the agent writes") {
		t.Errorf("Describe() does not name dharness's own side of the boundaries collision:\n%s", describe)
	}
}

// TestNoCollisionLeavesTheStepSatisfied pins the intersection-empty branch:
// Satisfied is true and the step is absent from Pending.
func TestNoCollisionLeavesTheStepSatisfied(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root}
	writeProjectFallow(t, root, `{"extends":["./.dharness/fallow.jsonc"]}`)

	if !(boundariesOwnerStep{}).Satisfied(p) {
		t.Error("Satisfied() = false with nothing declared that a matched preset also contributes")
	}
	if stepIDPresent(Pending(p), (boundariesOwnerStep{}).ID()) {
		t.Error("boundariesOwnerStep is present in Pending() despite being satisfied")
	}
}

// TestOwnedFileWritesContributedKeyRegardlessOfCollision pins that the
// region write is unconditional on the collision step's outcome. No real
// preset with a non-empty manifest exists until slice 5, so — matching the
// judgment call slice 2 already made for presetRegion/replaceRegion — this
// is proven directly against presetRegion with a stub match, alongside
// collidingKeys computed from the exact same fixture, rather than through a
// real preset end to end.
func TestOwnedFileWritesContributedKeyRegardlessOfCollision(t *testing.T) {
	root := t.TempDir()
	writeProjectFallow(t, root, `{"ignorePatterns": ["src/**"]}`)

	matches := []preset.Match{stubMatch("stub", preset.Root, "ignorePatterns", []string{"wailsjs/**"}, "wails.json")}

	if colliding := collidingKeys(root, matches); len(colliding) != 1 || colliding[0] != "ignorePatterns" {
		t.Fatalf("collidingKeys() = %v, want the fixture to actually collide on ignorePatterns", colliding)
	}

	region := presetRegion(matches)
	if !strings.Contains(region, `"ignorePatterns": ["wailsjs/**"]`) {
		t.Errorf("presetRegion() withheld the contributed key because it collides with the project's own:\n%s", region)
	}
}

// TestWailsMatchedOwnedFileIsNoLongerEmpty is the success criterion stated
// directly (task 19.3): a Wails-matched project's .dharness/fallow.jsonc
// carries the ignore pattern after Apply(), through the real registry —
// wails{} — for the first time in this change, not a stub.
func TestWailsMatchedOwnedFileIsNoLongerEmpty(t *testing.T) {
	t.Cleanup(runner.SetForTest(func(runner.Command, io.Writer, io.Writer) error { return nil }))
	t.Cleanup(project.SetGitOutputForTest(func(string, ...string) ([]byte, error) { return nil, nil }))

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "wails.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{\n  \"name\": \"wails-app\"\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	p := project.At(root, root)
	p.InRepository = true

	if err := Apply(p, io.Discard); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, project.Dir, ownedFallow))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"ignorePatterns": ["frontend/wailsjs/**"]`) {
		t.Errorf("%s does not carry the Wails ignore pattern after Apply():\n%s", ownedFallow, raw)
	}
}

// TestIgnorePatternsCollidesInTheMotivatingShape is task 19.4, the end-to-end
// proof of the proposal's "applied to the motivating repository" claim: a
// Wails-matched project whose own .fallowrc.json already declares
// ignorePatterns collides, names both values, and still gets the preset's
// value written into the file dharness owns.
func TestIgnorePatternsCollidesInTheMotivatingShape(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "wails.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeProjectFallow(t, root, `{"ignorePatterns": ["dist/**"]}`)

	p := project.Project{Root: root, Source: root}

	if (boundariesOwnerStep{}).Satisfied(p) {
		t.Fatal("Satisfied() = true while the project's own ignorePatterns collides with wails'")
	}

	why, ok := (boundariesOwnerStep{}).Delegated(p)
	if !ok {
		t.Fatal("Delegated() = false; dharness cannot merge two values for one key")
	}
	if !strings.Contains(why, "ignorePatterns") {
		t.Errorf("Delegated() why does not name ignorePatterns:\n%s", why)
	}
	if !strings.Contains(why, `["frontend/wailsjs/**"]`) {
		t.Errorf("Delegated() why does not show wails' contributed value:\n%s", why)
	}
	if !strings.Contains(why, `"dist/**"`) {
		t.Errorf("Delegated() why does not show the project's own declared value:\n%s", why)
	}

	region := presetRegion(preset.Resolve(p))
	if !strings.Contains(region, `"ignorePatterns": ["frontend/wailsjs/**"]`) {
		t.Errorf("presetRegion() withheld wails' contribution because the project's own config collides with it:\n%s", region)
	}
}

// TestCollisionStepDisappearsWhenIntersectionEmpties pins §07/§15: resolving
// a collision by removing the project's own key makes the step disappear on
// the next Pending() call, with nothing recorded to remember it existed.
func TestCollisionStepDisappearsWhenIntersectionEmpties(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root}
	writeProjectFallow(t, root, `{"boundaries":{"zones":[]}}`)

	if !stepIDPresent(Pending(p), (boundariesOwnerStep{}).ID()) {
		t.Fatal("boundariesOwnerStep absent from Pending() while the project still declares boundaries")
	}

	writeProjectFallow(t, root, `{}`)

	if stepIDPresent(Pending(p), (boundariesOwnerStep{}).ID()) {
		t.Error("boundariesOwnerStep is still present in Pending() after the project's own key was removed")
	}
}

// TestCollisionStepReappearsWhenTheKeyIsReDeclared is TestCollisionStepDisappearsWhenIntersectionEmpties's
// inverse: Satisfied is re-derived every run, never cached.
func TestCollisionStepReappearsWhenTheKeyIsReDeclared(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root}
	writeProjectFallow(t, root, `{}`)

	if stepIDPresent(Pending(p), (boundariesOwnerStep{}).ID()) {
		t.Fatal("boundariesOwnerStep present in Pending() before the project ever declared boundaries")
	}

	writeProjectFallow(t, root, `{"boundaries":{"zones":[]}}`)

	if !stepIDPresent(Pending(p), (boundariesOwnerStep{}).ID()) {
		t.Error("boundariesOwnerStep did not reappear after the key was re-declared")
	}
}

// TestBoundariesOwnerDescribeAndDelegatedDoNotReadCwdWithoutASource extends
// TestBoundariesOwnerStepIsSatisfiedWithoutASource's guard to Describe and
// Delegated: both now read a file, where before this slice they were fully
// static, so both need the same !HasSource() guard Satisfied has always had.
func TestBoundariesOwnerDescribeAndDelegatedDoNotReadCwdWithoutASource(t *testing.T) {
	elsewhere := t.TempDir()
	if err := os.WriteFile(filepath.Join(elsewhere, fallowConfig), []byte(`{"boundaries":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(elsewhere)

	p := project.Project{Root: t.TempDir()}

	wantDescribe := describeBoundaries(p, nil, nil)
	if got := (boundariesOwnerStep{}).Describe(p); got != wantDescribe {
		t.Errorf("Describe() = %q, want the no-collision fallback %q — an unrelated config was read from the working directory", got, wantDescribe)
	}

	wantWhy := delegateBoundaries(p, nil, nil)
	if got, _ := (boundariesOwnerStep{}).Delegated(p); got != wantWhy {
		t.Errorf("Delegated() = %q, want the no-collision fallback %q — an unrelated config was read from the working directory", got, wantWhy)
	}
}

// A config dharness cannot read textually is a blind spot, and a blind spot
// is not a step.
//
// fallow.toml's keys are bare, so the quoted-key test cannot answer for it.
// Reporting that as an unsatisfied step would put an entry in the plan that
// the project can never clear: the agent can read the file by hand, but there
// is no state it can reach that makes dharness stop asking. §15 says a step
// disappears once satisfied, and a step with no completion state is not
// pending work — it is a note, and it belongs in the report beside the plan
// rather than inside it.
//
// The alternative was worse in the other direction: answering "nothing
// collides" for a file dharness never read is the silent no-op this whole
// change exists to end. So the check stays honest and moves out of the plan.
// uncertainNotes is tested against stub matches for the same reason
// collidingKeys is: the real registry carried nothing with a non-empty
// Uncertain until wails registered.
func TestUncertainNotesNamesTheMatchAndWhatItCouldNotRead(t *testing.T) {
	matches := []preset.Match{{ID: "wails", Uncertain: "wails.json exists but does not parse"}}
	got := uncertainNotes(matches)
	if !strings.Contains(got, "wails") || !strings.Contains(got, "does not parse") {
		t.Errorf("uncertainNotes() = %q, want it to name the match and what it could not read", got)
	}
}

func TestUncertainNotesEmptyWhenNothingIsUncertain(t *testing.T) {
	matches := []preset.Match{{ID: "generic"}}
	if got := uncertainNotes(matches); got != "" {
		t.Errorf("uncertainNotes() = %q, want empty — no match left anything unread", got)
	}
}

func TestAnUncheckableConfigIsANoteRatherThanAnUnclearableStep(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root}
	if err := os.WriteFile(filepath.Join(root, "fallow.toml"), []byte("ignorePatterns = [\"wailsjs/**\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if !(boundariesOwnerStep{}).Satisfied(p) {
		t.Error("Satisfied() = false for a config dharness cannot check: the plan gains an entry nothing can clear")
	}
	if stepIDPresent(Pending(p), (boundariesOwnerStep{}).ID()) {
		t.Error("boundariesOwnerStep is in Pending() for a blind spot the project cannot resolve")
	}

	note := UncheckableConfigNote(p)
	if note == "" {
		t.Fatal("UncheckableConfigNote() = \"\"; the blind spot must still be reported somewhere")
	}
	for _, expected := range []string{"fallow.toml", "unknown"} {
		if !strings.Contains(strings.ToLower(note), expected) {
			t.Errorf("the note omits %q — it must name the file and refuse to claim the check passed:\n%s", expected, note)
		}
	}
}

// The note is silent where there is nothing to say, or it becomes noise on
// every run of every project.
func TestUncheckableConfigNoteIsSilentWhenTheConfigCanBeRead(t *testing.T) {
	for _, layout := range []struct {
		name  string
		write func(t *testing.T, root string)
	}{
		{name: "no fallow config at all", write: func(*testing.T, string) {}},
		{name: "a config declaredKeys can read", write: func(t *testing.T, root string) {
			if err := os.WriteFile(filepath.Join(root, fallowConfig), []byte(`{"extends":["./.dharness/fallow.jsonc"]}`), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(layout.name, func(t *testing.T) {
			root := t.TempDir()
			layout.write(t, root)
			if note := UncheckableConfigNote(project.Project{Root: root, Source: root}); note != "" {
				t.Errorf("UncheckableConfigNote() = %q, want \"\"", note)
			}
		})
	}

	// With no JS project, Source is empty and every path built from it would
	// resolve against the working directory instead of nowhere. The chdir is
	// what makes dropping that guard observable: without it, dharness reports
	// on whatever config happens to sit where the process was started.
	elsewhere := t.TempDir()
	if err := os.WriteFile(filepath.Join(elsewhere, "fallow.toml"), []byte("ignorePatterns = []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(elsewhere)

	if note := UncheckableConfigNote(project.Project{Root: t.TempDir()}); note != "" {
		t.Errorf("UncheckableConfigNote() = %q with no JS project: an unrelated config was read from the working directory", note)
	}
}

// TestEslintExtendsStepDelegatedRefusalMatrix walks every eslint-config-splice
// refusal-matrix cell in spec.md: ok == true only for a TypeScript config, a
// legacy-only project, an unreadable file, an unrecognised call, an ERROR
// node, or a malformed marker pair. No config at all, or a config
// jsconfig.Analyze recognises with well-formed markers, must not delegate —
// the write-if-absent and (later) splice/replace paths both start from
// ok == false.
func TestEslintExtendsStepDelegatedRefusalMatrix(t *testing.T) {
	cases := []struct {
		name       string
		write      func(t *testing.T, source string)
		wantOK     bool
		wantWhyHas string
	}{
		{
			name:       "no config at all",
			write:      func(*testing.T, string) {},
			wantOK:     false,
			wantWhyHas: "",
		},
		{
			name: "TypeScript config always delegates",
			write: func(t *testing.T, source string) {
				writeStepFixtureFile(t, source, "eslint.config.ts", "export default [];\n")
			},
			wantOK:     true,
			wantWhyHas: "TypeScript",
		},
		{
			name: "TypeScript .mts config always delegates",
			write: func(t *testing.T, source string) {
				writeStepFixtureFile(t, source, "eslint.config.mts", "export default [];\n")
			},
			wantOK:     true,
			wantWhyHas: "TypeScript",
		},
		{
			name: "legacy .eslintrc.json-only delegates",
			write: func(t *testing.T, source string) {
				writeStepFixtureFile(t, source, ".eslintrc.json", "{}")
			},
			wantOK:     true,
			wantWhyHas: "legacy",
		},
		{
			name: "plain array-literal export is not delegated",
			write: func(t *testing.T, source string) {
				writeStepFixtureFile(t, source, "eslint.config.js", "export default [\n  { rules: {} },\n];\n")
			},
			wantOK: false,
		},
		{
			name: "recognised defineConfig(...) is not delegated",
			write: func(t *testing.T, source string) {
				writeStepFixtureFile(t, source, "eslint.config.js",
					"import { defineConfig } from \"eslint/config\";\nexport default defineConfig([\n  { rules: {} },\n]);\n")
			},
			wantOK: false,
		},
		{
			name: "unrecognised call expression delegates",
			write: func(t *testing.T, source string) {
				writeStepFixtureFile(t, source, "eslint.config.js", "export default tseslint.config(\n  { rules: {} },\n);\n")
			},
			wantOK:     true,
			wantWhyHas: "callee",
		},
		{
			name: "ERROR node delegates",
			write: func(t *testing.T, source string) {
				writeStepFixtureFile(t, source, "eslint.config.js", "export default [\n  { a: 1 + },\n];\n")
			},
			wantOK:     true,
			wantWhyHas: "ERROR",
		},
		{
			name: "malformed dharness:eslint-import marker pair delegates",
			write: func(t *testing.T, source string) {
				writeStepFixtureFile(t, source, "eslint.config.js",
					eslintImportBegin+"\nimport dharnessPlugin from \"dharness-eslint-plugin\";\nexport default [];\n")
			},
			wantOK:     true,
			wantWhyHas: "eslint-import",
		},
		{
			name: "malformed dharness:eslint-layer marker pair delegates",
			write: func(t *testing.T, source string) {
				writeStepFixtureFile(t, source, "eslint.config.js",
					"export default [\n  "+eslintLayerBegin+"\n  ...dharnessLayer({ plugin: dharnessPlugin }),\n];\n")
			},
			wantOK:     true,
			wantWhyHas: "eslint-layer",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tc.write(t, root)
			p := project.Project{Root: root, Source: root}

			why, ok := (eslintExtendsStep{}).Delegated(p)
			if ok != tc.wantOK {
				t.Fatalf("Delegated() ok = %t, why = %q, want ok = %t", ok, why, tc.wantOK)
			}
			if tc.wantOK && why == "" {
				t.Error("Delegated() why is empty on ok = true")
			}
			if tc.wantWhyHas != "" && !strings.Contains(why, tc.wantWhyHas) {
				t.Errorf("Delegated() why = %q, want it to mention %q", why, tc.wantWhyHas)
			}
			if !tc.wantOK && why != "" {
				t.Errorf("Delegated() why = %q, want empty on ok = false", why)
			}
		})
	}
}

// TestEslintExtendsStepDelegatedWithoutASource pins the same guard every
// other extends step already has: no JS project means nothing to inspect.
func TestEslintExtendsStepDelegatedWithoutASource(t *testing.T) {
	why, ok := (eslintExtendsStep{}).Delegated(project.Project{Root: t.TempDir()})
	if ok {
		t.Errorf("Delegated() ok = true with no source, why = %q", why)
	}
}

func writeStepFixtureFile(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestEslintExtendsStepApplyWritesAConfigWhenNoneExists pins the spec
// scenario "a project with no ESLint config gets one written": Apply writes
// a complete eslint.config.js importing and spreading the owned factory,
// matching wireFallowExtends's write-if-absent shape.
func TestEslintExtendsStepApplyWritesAConfigWhenNoneExists(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root}

	if err := (eslintExtendsStep{}).Apply(p, &Writer{}); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, eslintConfig))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)

	for _, want := range []string{
		eslintImportBegin,
		"import dharnessPlugin from \"dharness-eslint-plugin\";",
		"import dharnessLayer from \".dharness/eslint.config.js\";",
		eslintImportEnd,
		"export default [",
		eslintLayerBegin,
		"...dharnessLayer({ plugin: dharnessPlugin }),",
		eslintLayerEnd,
		"];",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Apply() wrote a file missing %q:\n%s", want, got)
		}
	}
}

// TestEslintExtendsStepApplyResolvesFromASplitLayout triangulates the case
// above against the split-layout target ownedFrom already gives every other
// project file.
func TestEslintExtendsStepApplyResolvesFromASplitLayout(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "frontend")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	p := project.Project{Root: root, Source: source}

	if err := (eslintExtendsStep{}).Apply(p, &Writer{}); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(source, eslintConfig))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "import dharnessLayer from \"../.dharness/eslint.config.js\";") {
		t.Errorf("Apply() did not resolve the owned factory from the split layout:\n%s", raw)
	}
}

// TestEslintExtendsStepApplyResultIsAlreadySatisfied triangulates
// write-if-absent against Satisfied: once Apply has run, a fresh Satisfied
// call over the same project must already report true — the same
// idempotency shape every other extends step gives.
func TestEslintExtendsStepApplyResultIsAlreadySatisfied(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root}

	if (eslintExtendsStep{}).Satisfied(p) {
		t.Fatal("Satisfied() = true before Apply ever ran")
	}
	if err := (eslintExtendsStep{}).Apply(p, &Writer{}); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if !(eslintExtendsStep{}).Satisfied(p) {
		t.Error("Satisfied() = false right after Apply wrote the file")
	}
}

// TestEslintExtendsStepApplyInsertsBothRegionsIntoAnExistingArrayLiteral
// pins slice 3b's insert path, exercised through the step rather than
// jsconfig directly: the layer region lands before the project's own
// element, the import region lands after the project's own import, and
// nothing the project wrote is lost — the later offset splices first so it
// does not shift the earlier one (design decision 6).
func TestEslintExtendsStepApplyInsertsBothRegionsIntoAnExistingArrayLiteral(t *testing.T) {
	root := t.TempDir()
	original := "import next from \"eslint-config-next\";\n\nexport default [\n  ...next,\n];\n"
	writeStepFixtureFile(t, root, eslintConfig, original)
	p := project.Project{Root: root, Source: root}

	if err := (eslintExtendsStep{}).Apply(p, &Writer{}); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, eslintConfig))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)

	if !strings.Contains(got, "import next from \"eslint-config-next\";") {
		t.Errorf("Apply() lost the project's own import:\n%s", got)
	}
	if !strings.Contains(got, "  ...next,\n") {
		t.Errorf("Apply() lost the project's own array element:\n%s", got)
	}
	if strings.Count(got, eslintImportBegin) != 1 || strings.Count(got, eslintImportEnd) != 1 {
		t.Errorf("Apply() did not insert exactly one dharness:eslint-import region:\n%s", got)
	}
	if strings.Count(got, eslintLayerBegin) != 1 || strings.Count(got, eslintLayerEnd) != 1 {
		t.Errorf("Apply() did not insert exactly one dharness:eslint-layer region:\n%s", got)
	}

	layerIdx, nextIdx := strings.Index(got, eslintLayerBegin), strings.Index(got, "...next,")
	if layerIdx == -1 || nextIdx == -1 || layerIdx > nextIdx {
		t.Errorf("Apply() did not insert dharness's layer before the project's own element:\n%s", got)
	}

	importIdx, projectImportIdx := strings.Index(got, eslintImportBegin), strings.Index(got, "import next from")
	if importIdx == -1 || projectImportIdx == -1 || importIdx < projectImportIdx {
		t.Errorf("Apply() did not insert the import region after the project's existing import:\n%s", got)
	}
}

// TestPresentMarkersWithStaleBytesAreReplacedNotDuplicated pins slice 3b's
// replace path (design decision 6): markers already present with bytes that
// differ from what this run renders converge by rewriting the region in
// place, not by inserting a second one — an implementation that could only
// insert would leave two regions of one kind, which the candidate guard
// must reject.
func TestPresentMarkersWithStaleBytesAreReplacedNotDuplicated(t *testing.T) {
	root := t.TempDir()
	stale := eslintImportBegin + "\n" +
		"import dharnessPlugin from \"dharness-eslint-plugin\";\n" +
		"import dharnessLayer from \".dharness/eslint.config.js\";\n" +
		eslintImportEnd + "\n" +
		"\n" +
		"export default [\n" +
		"  " + eslintLayerBegin + "\n" +
		"  ...dharnessLayer({ plugin: dharnessPlugin, stale: true }),\n" +
		"  " + eslintLayerEnd + "\n" +
		"  { rules: { semi: \"error\" } },\n" +
		"];\n"
	writeStepFixtureFile(t, root, eslintConfig, stale)
	p := project.Project{Root: root, Source: root}

	if err := (eslintExtendsStep{}).Apply(p, &Writer{}); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, eslintConfig))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)

	if strings.Count(got, eslintLayerBegin) != 1 || strings.Count(got, eslintLayerEnd) != 1 {
		t.Fatalf("Apply() left more than one dharness:eslint-layer region — a second insertion, not a replacement:\n%s", got)
	}
	if strings.Count(got, eslintImportBegin) != 1 || strings.Count(got, eslintImportEnd) != 1 {
		t.Fatalf("Apply() left more than one dharness:eslint-import region — a second insertion, not a replacement:\n%s", got)
	}
	if strings.Contains(got, "stale: true") {
		t.Errorf("Apply() kept the stale bytes instead of replacing the region:\n%s", got)
	}
	if !strings.Contains(got, "...dharnessLayer({ plugin: dharnessPlugin }),") {
		t.Errorf("Apply() did not render the current layer call:\n%s", got)
	}
	if !strings.Contains(got, "{ rules: { semi: \"error\" } },") {
		t.Errorf("Apply() did not preserve the project's own array element outside the marked region:\n%s", got)
	}
}

// TestSpliceGuardRollsBackAnUnparseableResult pins design decision 6's
// candidate guard: a splice that, applied, would leave an ERROR node inside
// the default export must never reach disk. The adversarial region is
// injected through spliceEslintRegion — every real caller only ever
// composes well-formed regions, so there is no way to reach this candidate
// through the public rendering functions alone.
func TestSpliceGuardRollsBackAnUnparseableResult(t *testing.T) {
	root := t.TempDir()
	original := "export default [\n  { rules: {} },\n];\n"
	path := filepath.Join(root, eslintConfig)
	writeStepFixtureFile(t, root, eslintConfig, original)
	p := project.Project{Root: root, Source: root}

	restore := SetEslintSpliceForTest(func(src []byte, at int, region string) []byte {
		if strings.Contains(region, "eslint-layer") {
			region = "  " + eslintLayerBegin + "\n  ...dharnessLayer({ a: 1 + }),\n  " + eslintLayerEnd + "\n"
		}
		out := make([]byte, 0, len(src)+len(region))
		out = append(out, src[:at]...)
		out = append(out, region...)
		out = append(out, src[at:]...)
		return out
	})
	defer restore()

	if err := (eslintExtendsStep{}).Apply(p, &Writer{}); err == nil {
		t.Fatal("Apply() = nil, want an error from the candidate guard")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != original {
		t.Errorf("Apply() modified the file despite the guard failing:\ngot  %q\nwant %q", raw, original)
	}
}

// TestSecondSyncWritesNothing pins slice 3b's idempotency requirement:
// Satisfied(p) is already true right after the first Apply, and even a
// direct second Apply call renders byte-identical output — no drift on
// repeat.
func TestSecondSyncWritesNothing(t *testing.T) {
	root := t.TempDir()
	original := "import next from \"eslint-config-next\";\n\nexport default [\n  ...next,\n];\n"
	writeStepFixtureFile(t, root, eslintConfig, original)
	p := project.Project{Root: root, Source: root}

	if err := (eslintExtendsStep{}).Apply(p, &Writer{}); err != nil {
		t.Fatalf("first Apply() = %v", err)
	}
	if !(eslintExtendsStep{}).Satisfied(p) {
		t.Fatal("Satisfied() = false right after the first Apply")
	}

	path := filepath.Join(root, eslintConfig)
	afterFirst, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := (eslintExtendsStep{}).Apply(p, &Writer{}); err != nil {
		t.Fatalf("second Apply() = %v", err)
	}
	afterSecond, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterSecond) != string(afterFirst) {
		t.Errorf("second Apply() changed the file:\nfirst  %q\nsecond %q", afterFirst, afterSecond)
	}
}

// TestEslintExtendsStepSatisfiedIsFalseWhenMarkedRegionsAreStale pins that
// Satisfied is a byte comparison (design decision 6), not merely "do the
// marker regions exist": a splice-eligible config whose regions are present
// but stale must report unsatisfied, or applySteps never converges it.
func TestEslintExtendsStepSatisfiedIsFalseWhenMarkedRegionsAreStale(t *testing.T) {
	root := t.TempDir()
	stale := "export default [\n  " + eslintLayerBegin + "\n  ...dharnessLayer({ stale: true }),\n  " + eslintLayerEnd + "\n];\n"
	writeStepFixtureFile(t, root, eslintConfig, stale)
	p := project.Project{Root: root, Source: root}

	if (eslintExtendsStep{}).Satisfied(p) {
		t.Error("Satisfied() = true for a config whose marker pairs are mixed/stale")
	}
}

// TestEslintExtendsStepSatisfiedIsTrueForShapesThatAlwaysDelegate
// triangulates the case above: a TypeScript config, a legacy-only project
// and an unrecognised call expression all always delegate, so Satisfied
// must still answer true for them even though no marker region was ever
// written — Delegated already explains why nothing will be applied, and
// Satisfied should not double-report the same state as pending. The
// legacy-only case is asserted on its own, with no TypeScript config
// present, so it exercises the "||" right-hand side independently of the
// left.
func TestEslintExtendsStepSatisfiedIsTrueForShapesThatAlwaysDelegate(t *testing.T) {
	t.Run("TypeScript config", func(t *testing.T) {
		root := t.TempDir()
		writeStepFixtureFile(t, root, "eslint.config.ts", "export default [];\n")
		p := project.Project{Root: root, Source: root}

		if !(eslintExtendsStep{}).Satisfied(p) {
			t.Error("Satisfied() = false for a TypeScript config, which always delegates")
		}
	})

	t.Run("legacy-only config, no TypeScript config present", func(t *testing.T) {
		root := t.TempDir()
		writeStepFixtureFile(t, root, ".eslintrc.json", "{}")
		p := project.Project{Root: root, Source: root}

		if !(eslintExtendsStep{}).Satisfied(p) {
			t.Error("Satisfied() = false for a legacy-only config, which always delegates")
		}
	})

	t.Run("unrecognised call expression", func(t *testing.T) {
		root := t.TempDir()
		writeStepFixtureFile(t, root, eslintConfig, "export default tseslint.config(\n  { rules: {} },\n);\n")
		p := project.Project{Root: root, Source: root}

		if !(eslintExtendsStep{}).Satisfied(p) {
			t.Error("Satisfied() = false for an unrecognised call expression, which always delegates")
		}
	})
}

// TestEslintExtendsStepApplyPreservesCRLFAndBOMThroughTheSplice replays
// 1.9's constructed CRLF/BOM bytes (design decision 11) through the real
// step: every pre-existing line keeps its CRLF, the newly inserted regions
// adopt CRLF too, and the leading BOM survives.
func TestEslintExtendsStepApplyPreservesCRLFAndBOMThroughTheSplice(t *testing.T) {
	lf := "import next from \"eslint-config-next\";\n\nexport default [\n  ...next,\n];\n"
	crlf := strings.ReplaceAll(lf, "\n", "\r\n")
	bom := "\xEF\xBB\xBF" + crlf

	root := t.TempDir()
	writeStepFixtureFile(t, root, eslintConfig, bom)
	p := project.Project{Root: root, Source: root}

	if err := (eslintExtendsStep{}).Apply(p, &Writer{}); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, eslintConfig))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)

	if !strings.HasPrefix(got, "\xEF\xBB\xBF") {
		t.Error("Apply() lost the BOM")
	}
	for i := 0; i < len(got); i++ {
		if got[i] == '\n' && (i == 0 || got[i-1] != '\r') {
			t.Fatalf("Apply() wrote a lone LF at byte %d — a line ending was normalised:\n%q", i, got)
		}
	}
	if !strings.Contains(got, eslintLayerBegin+"\r\n") {
		t.Errorf("Apply() did not adopt CRLF for the inserted layer region:\n%q", got)
	}
}
