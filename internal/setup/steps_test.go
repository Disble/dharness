package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/preset"
	"github.com/Disble/dharness/internal/project"
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

// TestDefaultSeverityNeverCalledWhenProjectChoseIt proves the `!chosen`
// guard (§05) is unchanged by the barrel probe: a project whose
// doctor.config.json already declares a severity for folder-ownership must
// never trigger the probe at all — the stub panics if it does, which is a
// stronger pin than checking the written value merely still matches.
func TestDefaultSeverityNeverCalledWhenProjectChoseIt(t *testing.T) {
	root := t.TempDir()
	chosen := `{"rules":{"dharness/folder-ownership":"warn"}}`
	if err := os.WriteFile(filepath.Join(root, doctorConfig), []byte(chosen), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(project.SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		panic("DefaultSeverity must not probe for barrels when the project already chose a severity")
	}))

	p := project.Project{Root: root, Source: root, InRepository: true}
	if err := (doctorConfigStep{}).Apply(p, &Writer{}); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(root, doctorConfig))
	if err != nil {
		t.Fatal(err)
	}
	var config doctorConfigFile
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	if got := config.Rules[RulesPrefix+"/folder-ownership"]; got != "warn" {
		t.Errorf("folder-ownership = %q, want the project's own choice \"warn\" preserved", got)
	}
}

// TestAddingBarrelsAfterAdoptionDoesNotFlipSeverity pins the first-write-only
// limit as a property, not a gap: doctorConfigStep.Satisfied is already true
// once RulesPackage is in `plugins`, so a second sync never runs Apply
// again — folder-ownership stays at its original value even after the
// project later grows barrels. The gitOutput stub answers "barrels exist"
// and is never asked, because Satisfied never reads it.
func TestAddingBarrelsAfterAdoptionDoesNotFlipSeverity(t *testing.T) {
	root := t.TempDir()
	adopted := `{"plugins":["dharness-eslint-plugin"],"rules":{"dharness/folder-ownership":"off"}}`
	if err := os.WriteFile(filepath.Join(root, doctorConfig), []byte(adopted), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(project.SetGitOutputForTest(func(string, ...string) ([]byte, error) {
		return []byte("components/index.ts"), nil // barrels now exist
	}))

	p := project.Project{Root: root, Source: root, InRepository: true}
	if !(doctorConfigStep{}).Satisfied(p) {
		t.Fatal("Satisfied() = false; the package is already in plugins, so a second sync must not run Apply again")
	}
}
