package setup

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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

// writeLocalFallowBinary creates the file p.LocalBinary(tool.Fallow) looks
// for, in the platform-specific shape npm actually writes: a .cmd shim on
// Windows, a plain file everywhere else — the same fixture shape
// internal/cli's own tests already use for a locally installed tool. Its
// contents never run; runner.Run is always seamed in these tests.
func writeLocalFallowBinary(t *testing.T, source string) {
	t.Helper()
	name := "fallow"
	if runtime.GOOS == "windows" {
		name = "fallow.cmd"
	}
	path := filepath.Join(source, "node_modules", ".bin", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestCollisionNamesEveryContributedKeyTheProjectDeclares pins the widened
// step's whole contract: every colliding key is named, and a key the
// project never declared is never mentioned. It uses a real preset match
// (wails) rather than a stub, because design.md Decision 4 wires this slice
// so describeBoundaries/delegateBoundaries's non-empty branch renders from
// Collisions(p) — which resolves matches through preset.Resolve(p), not
// through whatever a caller happens to pass — so a stub matches slice is no
// longer reachable from that branch at all. entryPointsLikeKey exercises
// the "never mentioned" half without depending on a second real preset.
func TestCollisionNamesEveryContributedKeyTheProjectDeclares(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "wails.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeProjectFallow(t, root, `{"boundaries":{"zones":[]},"ignorePatterns":["dist/**"]}`)
	p := project.Project{Root: root, Source: root}

	colliding, _ := boundaryCollision(p)
	if len(colliding) != 2 {
		t.Fatalf("boundaryCollision() = %v, want exactly 2 colliding keys (boundaries, ignorePatterns)", colliding)
	}

	rendered := renderCollisions(Collisions(p))
	why := delegateBoundaries(p, colliding)
	describe := describeBoundaries(p, colliding)

	if why != rendered {
		t.Errorf("delegateBoundaries() = %q, want renderCollisions(Collisions(p)) = %q (the report's collision rendering and the delegated reason must not drift apart)", why, rendered)
	}
	if describe != rendered {
		t.Errorf("describeBoundaries() = %q, want renderCollisions(Collisions(p)) = %q", describe, rendered)
	}

	for _, key := range colliding {
		if !strings.Contains(rendered, key) {
			t.Errorf("rendered collision prose omits colliding key %q:\n%s", key, rendered)
		}
	}
	if strings.Contains(rendered, "entryPoints") {
		t.Errorf("rendered collision prose mentions entryPoints, which the project never declared:\n%s", rendered)
	}
}

// TestDelegatedCollisionMatchesTheComputedReportValue pins step-delegation's
// added requirement: boundariesOwnerStep's delegation hands back the same
// structured Collision value the report is rendered from, not a
// pre-rendered prose string a second, independent renderer could drift
// away from. Asserted for a single colliding key — the "boundaries" fixture
// every other boundariesOwnerStep test already uses, so this test isolates
// the byte-for-byte equality rather than the multi-key case above.
func TestDelegatedCollisionMatchesTheComputedReportValue(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root}
	writeProjectFallow(t, root, `{"extends":["./.dharness/fallow.jsonc"],"boundaries":{"zones":[]}}`)

	why, ok := (boundariesOwnerStep{}).Delegated(p)
	if !ok {
		t.Fatal("Delegated() ok = false; a real collision must delegate")
	}

	want := renderCollisions(Collisions(p))
	if why != want {
		t.Errorf("Delegated() why = %q, want renderCollisions(Collisions(p)) = %q", why, want)
	}
}

// TestBoundariesOwnerStepIDMakesNoArchitectureClaim pins spec's amended
// requirement (a) by content, not exact wording: ID() takes no
// project.Project, so it cannot know whether this project declares one
// architecture, two, or none — the string it returns must not claim two
// regardless.
func TestBoundariesOwnerStepIDMakesNoArchitectureClaim(t *testing.T) {
	id := (boundariesOwnerStep{}).ID()
	if strings.Contains(id, "two architectures") || strings.Contains(id, "architectures this project declares") {
		t.Errorf("ID() = %q, still asserts an architecture claim it cannot back", id)
	}
}

// TestBoundariesFallbackConstantsStayByteIdentical guards design.md
// Decision 6's "six lines is the complete golden impact" claim: the generic
// golden fixtures render a project where collidingKeys is always empty, so
// describeBoundaries/delegateBoundaries reach only these two fallback
// constants. If either changes by even one byte, the six-line figure is no
// longer true and every golden fixture needs re-measuring, not just line 26.
func TestBoundariesFallbackConstantsStayByteIdentical(t *testing.T) {
	const wantDescribe = "Move the zones and rules from %s into %s, or delete the block dharness\nowns and keep the project's. Either is a valid answer; having both is not,\nbecause only one of them runs and the file gives no sign of which."
	const wantWhy = "%s declares its own `boundaries`, and fallow's `extends` replaces that key\nrather than merging it — the project's block replaces the one dharness owns\nentirely, without an error. Only one architecture is being enforced, and the\nconfiguration does not say which."

	if boundariesFallbackDescribe != wantDescribe {
		t.Errorf("boundariesFallbackDescribe changed:\ngot  %q\nwant %q", boundariesFallbackDescribe, wantDescribe)
	}
	if boundariesFallbackWhy != wantWhy {
		t.Errorf("boundariesFallbackWhy changed:\ngot  %q\nwant %q", boundariesFallbackWhy, wantWhy)
	}
}

// TestResolvedConfigShortCircuitsOnNoLocalBinary pins the measurement's
// first exit: with no local fallow binary, resolvedConfig never builds a
// subprocess at all — the cheapest of the absence rows, and the one that
// costs nothing.
func TestResolvedConfigShortCircuitsOnNoLocalBinary(t *testing.T) {
	var commands []runner.Command
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		commands = append(commands, cmd)
		return nil
	}))

	root := t.TempDir()
	p := project.Project{Root: root, Source: root}

	resolved, ok := resolvedConfig(p)
	if ok {
		t.Error("resolvedConfig() ok = true with no local fallow binary")
	}
	if resolved != nil {
		t.Errorf("resolvedConfig() = %v, want nil", resolved)
	}
	if len(commands) != 0 {
		t.Errorf("resolvedConfig() ran %d commands with no local binary, want 0: %v", len(commands), commands)
	}
}

// TestResolvedConfigShortCircuitsOnExit3 pins the exit-3 short circuit
// itself: the --format json call must never run once --path answers "no
// config at all". Asserted on command count, not merely on the result, so a
// mutant that runs both commands and still lands on absence through the
// unmarshal failure cannot survive (design.md's named mutation risk).
func TestResolvedConfigShortCircuitsOnExit3(t *testing.T) {
	root := t.TempDir()
	writeLocalFallowBinary(t, root)
	p := project.Project{Root: root, Source: root}

	var commands []runner.Command
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		commands = append(commands, cmd)
		return &runner.ExitError{Command: "fallow", Code: 3}
	}))

	resolved, ok := resolvedConfig(p)
	if ok {
		t.Error("resolvedConfig() ok = true after a --path exit 3")
	}
	if resolved != nil {
		t.Errorf("resolvedConfig() = %v, want nil", resolved)
	}
	if len(commands) != 1 {
		t.Fatalf("resolvedConfig() ran %d commands after exit 3, want exactly 1 (--format json must never run): %v", len(commands), commands)
	}
	if !slices.Contains(commands[0].Args, "--path") {
		t.Errorf("the one command run was %v, want the --path probe", commands[0].Args)
	}
}

// TestResolvedConfigAbsenceHasOneShape covers the two remaining failure
// modes at resolvedConfig's own layer, beyond the no-local-binary and
// exit-3 cases pinned above: a non-zero, non-3 --path exit, and --format
// json succeeding but returning stdout that does not parse as JSON. Every
// row: absent, never fabricated.
func TestResolvedConfigAbsenceHasOneShape(t *testing.T) {
	cases := []struct {
		name string
		run  func(cmd runner.Command, stdout io.Writer) error
	}{
		{
			name: "non-zero, non-3 exit from --path",
			run: func(cmd runner.Command, _ io.Writer) error {
				if slices.Contains(cmd.Args, "--path") {
					return &runner.ExitError{Command: "fallow", Code: 2}
				}
				return nil
			},
		},
		{
			name: "non-JSON stdout from --format json",
			run: func(cmd runner.Command, stdout io.Writer) error {
				if slices.Contains(cmd.Args, "--format") {
					_, _ = io.WriteString(stdout, "not json")
				}
				return nil
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeLocalFallowBinary(t, root)
			p := project.Project{Root: root, Source: root}

			t.Cleanup(runner.SetForTest(func(cmd runner.Command, stdout, _ io.Writer) error {
				return tc.run(cmd, stdout)
			}))

			resolved, ok := resolvedConfig(p)
			if ok {
				t.Error("resolvedConfig() ok = true, want false")
			}
			if resolved != nil {
				t.Errorf("resolvedConfig() = %v, want nil", resolved)
			}
		})
	}
}

// TestResolvedConfigSucceedsAfterExit0Probe pins the success path: a config
// present, a clean --path probe, and --format json invoked exactly once,
// its resolved map becoming the return value.
func TestResolvedConfigSucceedsAfterExit0Probe(t *testing.T) {
	root := t.TempDir()
	writeLocalFallowBinary(t, root)
	p := project.Project{Root: root, Source: root}

	var jsonCalls int
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, stdout, _ io.Writer) error {
		if slices.Contains(cmd.Args, "--path") {
			return nil
		}
		jsonCalls++
		_, _ = io.WriteString(stdout, `{"duplicates":{"threshold":3}}`)
		return nil
	}))

	resolved, ok := resolvedConfig(p)
	if !ok {
		t.Fatal("resolvedConfig() ok = false, want true")
	}
	if jsonCalls != 1 {
		t.Errorf("--format json ran %d times, want exactly 1", jsonCalls)
	}
	raw, present := resolved["duplicates"]
	if !present {
		t.Fatal(`resolvedConfig() map has no "duplicates" key`)
	}
	if string(raw) != `{"threshold":3}` {
		t.Errorf(`resolvedConfig()["duplicates"] = %s, want {"threshold":3}`, raw)
	}
}

// TestCollisionsComputesEachKeyOnce pins "a collision is computed once and
// rendered from that one value in both views" at the computation layer: two
// colliding keys through the real preset registry (a wails-matched project
// whose own config also declares boundaries), each producing exactly one
// report.Collision. Ours is always populated from the dharness-owned value;
// Effective and Theirs.Value are populated only when the resolve actually
// succeeds — exercised for both outcomes, since a stub that always resolved
// could not tell the two apart.
func TestCollisionsComputesEachKeyOnce(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "wails.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeProjectFallow(t, root, `{"boundaries":{"zones":[]},"ignorePatterns":["dist/**"]}`)
	p := project.Project{Root: root, Source: root}

	// Absent case: no local fallow binary at all.
	absent := Collisions(p)
	if len(absent) != 2 {
		t.Fatalf("Collisions() returned %d entries, want 2: %+v", len(absent), absent)
	}
	for _, c := range absent {
		if c.ID != "sync:collision/"+c.Key {
			t.Errorf("collision %q: ID = %q, want sync:collision/%s", c.Key, c.ID, c.Key)
		}
		if c.Ours.Value == nil {
			t.Errorf("collision %q: Ours.Value is nil, want the dharness-owned value", c.Key)
		}
		if c.Effective != nil || c.Theirs.Value != nil {
			t.Errorf("collision %q: Effective/Theirs.Value populated with no local fallow binary to resolve them", c.Key)
		}
	}

	// Present case: a stubbed local binary resolves both keys.
	writeLocalFallowBinary(t, root)
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, stdout, _ io.Writer) error {
		if slices.Contains(cmd.Args, "--format") {
			_, _ = io.WriteString(stdout, `{"boundaries":{"zones":[]},"ignorePatterns":["dist/**"]}`)
		}
		return nil
	}))

	resolved := Collisions(p)
	if len(resolved) != 2 {
		t.Fatalf("Collisions() (resolved) returned %d entries, want 2: %+v", len(resolved), resolved)
	}
	for _, c := range resolved {
		if c.Theirs.Value == nil {
			t.Errorf("collision %q: Theirs.Value is nil after a successful resolve", c.Key)
		}
		if c.Effective == nil {
			t.Errorf("collision %q: Effective is nil after a successful resolve", c.Key)
		}
	}
}

// TestCollisionsOursNamesTheOwnedFallowPath pins gap 10 from the team
// lead's measured run: Collision.Ours.Path must name the file dharness owns
// (.dharness/fallow.jsonc), never an empty string — a reader cannot act on
// "dharness: <value>" with nowhere to look.
func TestCollisionsOursNamesTheOwnedFallowPath(t *testing.T) {
	root := t.TempDir()
	writeProjectFallow(t, root, `{"boundaries":{"zones":[]}}`)
	p := project.Project{Root: root, Source: root}

	collisions := Collisions(p)
	if len(collisions) != 1 {
		t.Fatalf("Collisions() = %+v, want exactly 1", collisions)
	}
	want := filepath.ToSlash(filepath.Join(project.Dir, ownedFallow))
	if collisions[0].Ours.Path != want {
		t.Errorf("Ours.Path = %q, want %q", collisions[0].Ours.Path, want)
	}
}

// TestCollisionsOursLineMeasuresTheOwnedFileWhenItExists triangulates the
// case above once the owned file is actually on disk (as it is by the time
// boundariesOwnerStep runs inside setup.Run, since ownedFilesStep applies
// first in Plan() order): the line number must be measured from that real
// file, not left at the zero sentinel declaredAt documents for "not found".
func TestCollisionsOursLineMeasuresTheOwnedFileWhenItExists(t *testing.T) {
	root := t.TempDir()
	writeProjectFallow(t, root, `{"boundaries":{"zones":[]}}`)
	ownedPath := filepath.Join(root, project.Dir, ownedFallow)
	if err := os.MkdirAll(filepath.Dir(ownedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ownedPath, []byte("{\n  \"boundaries\": []\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := project.Project{Root: root, Source: root}

	collisions := Collisions(p)
	if len(collisions) != 1 {
		t.Fatalf("Collisions() = %+v, want exactly 1", collisions)
	}
	if collisions[0].Ours.Line != 2 {
		t.Errorf("Ours.Line = %d, want 2 (the line declaring \"boundaries\" in the owned file)", collisions[0].Ours.Line)
	}
}

// TestCollisionsTheirsPathIsRootRelative pins a defect found while verifying
// the human collision block against a real project: fallowConfigPath(p.Source)
// is an absolute filesystem path, and Collisions used to store it verbatim —
// the collision block ends up unreadable ("project
// C:\Users\...\frontend\.fallowrc.json") instead of the short, root-relative
// form Ours.Path already uses (.dharness/fallow.jsonc) and Report.Source
// itself already uses (p.SourceRel()).
func TestCollisionsTheirsPathIsRootRelative(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "frontend")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	writeProjectFallow(t, source, `{"boundaries":{"zones":[]}}`)
	p := project.Project{Root: root, Source: source}

	collisions := Collisions(p)
	if len(collisions) != 1 {
		t.Fatalf("Collisions() = %+v, want exactly 1", collisions)
	}
	want := "frontend/" + fallowConfig
	if collisions[0].Theirs.Path != want {
		t.Errorf("Theirs.Path = %q, want %q (root-relative, matching Ours.Path's own convention)", collisions[0].Theirs.Path, want)
	}
}

// TestCollisionsAlwaysCarriesTheTwoResolutions pins gap 11: Resolutions must
// never be null — there is exactly one way to resolve any config collision
// on a key dharness also owns (delete the project's own declaration, or move
// it into the file dharness owns), whether or not effective was ever
// measured.
func TestCollisionsAlwaysCarriesTheTwoResolutions(t *testing.T) {
	root := t.TempDir()
	writeProjectFallow(t, root, `{"boundaries":{"zones":[]}}`)
	p := project.Project{Root: root, Source: root}

	collisions := Collisions(p)
	if len(collisions) != 1 {
		t.Fatalf("Collisions() = %+v, want exactly 1", collisions)
	}
	want := []string{"delete-theirs", "move-into-ours"}
	if !slices.Equal(collisions[0].Resolutions, want) {
		t.Errorf("Resolutions = %v, want %v", collisions[0].Resolutions, want)
	}
}

// TestCollisionsSpawnsNoProcessWithNoCollidingKey pins design.md Decision
// 5's "cheaper than the proposal's one cheap probe, and free (§13)": with
// nothing colliding, Collisions must never call resolvedConfig at all, not
// merely resolve it to absence. A local fallow binary is present so this
// isolates the collision-count short circuit from the no-binary short
// circuit TestResolvedConfigShortCircuitsOnNoLocalBinary already covers.
func TestCollisionsSpawnsNoProcessWithNoCollidingKey(t *testing.T) {
	root := t.TempDir()
	writeLocalFallowBinary(t, root)
	writeProjectFallow(t, root, `{"extends":["./.dharness/fallow.jsonc"]}`)
	p := project.Project{Root: root, Source: root}

	var commands []runner.Command
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		commands = append(commands, cmd)
		return nil
	}))

	if collisions := Collisions(p); collisions != nil {
		t.Fatalf("Collisions() = %+v, want nil with nothing colliding", collisions)
	}
	if len(commands) != 0 {
		t.Errorf("Collisions() ran %d commands with no colliding key, want 0: %v", len(commands), commands)
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
// ignorePatterns collides, names the key and the preset's own value, and
// still gets the preset's value written into the file dharness owns. The
// project's own declared value is asserted as the honest interim phrase
// (declaredValueUnknown) rather than the fragment it used to show — see
// TestCollisionNamesEveryContributedKeyTheProjectDeclares's own comment.
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
	if !strings.Contains(why, declaredValueUnknown) {
		t.Errorf("Delegated() why does not carry the honest interim phrase for the project's own value:\n%s", why)
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

	wantDescribe := describeBoundaries(p, nil)
	if got := (boundariesOwnerStep{}).Describe(p); got != wantDescribe {
		t.Errorf("Describe() = %q, want the no-collision fallback %q — an unrelated config was read from the working directory", got, wantDescribe)
	}

	wantWhy := delegateBoundaries(p, nil)
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
			name:       "TypeScript config always delegates",
			write:      writeFixture("eslint.config.ts", "export default [];\n"),
			wantOK:     true,
			wantWhyHas: "TypeScript",
		},
		{
			name:       "TypeScript .mts config always delegates",
			write:      writeFixture("eslint.config.mts", "export default [];\n"),
			wantOK:     true,
			wantWhyHas: "TypeScript",
		},
		{
			name:       "legacy .eslintrc.json-only delegates",
			write:      writeFixture(".eslintrc.json", "{}"),
			wantOK:     true,
			wantWhyHas: "legacy",
		},
		{
			name:   "plain array-literal export is not delegated",
			write:  writeFixture("eslint.config.js", "export default [\n  { rules: {} },\n];\n"),
			wantOK: false,
		},
		{
			name:   "recognised defineConfig(...) is not delegated",
			write:  writeFixture("eslint.config.js", "import { defineConfig } from \"eslint/config\";\nexport default defineConfig([\n  { rules: {} },\n]);\n"),
			wantOK: false,
		},
		{
			name:       "unrecognised call expression delegates",
			write:      writeFixture("eslint.config.js", "export default tseslint.config(\n  { rules: {} },\n);\n"),
			wantOK:     true,
			wantWhyHas: "callee",
		},
		{
			name:       "ERROR node delegates",
			write:      writeFixture("eslint.config.js", "export default [\n  { a: 1 + },\n];\n"),
			wantOK:     true,
			wantWhyHas: "ERROR",
		},
		{
			name:       "malformed dharness:eslint-import marker pair delegates",
			write:      writeFixture("eslint.config.js", eslintImportBegin+"\nimport dharnessPlugin from \"dharness-eslint-plugin\";\nexport default [];\n"),
			wantOK:     true,
			wantWhyHas: "eslint-import",
		},
		{
			name:       "malformed dharness:eslint-layer marker pair delegates",
			write:      writeFixture("eslint.config.js", "export default [\n  "+eslintLayerBegin+"\n  ...dharnessLayer({ plugin: dharnessPlugin }),\n];\n"),
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

// writeFixture builds the one-file setup every refusal-matrix case needs.
//
// The cases differ by a file name and its bytes and by nothing else, so
// spelling the closure out per case repeated its signature once per row and
// buried that difference in boilerplate. Returning the closure keeps the
// table a table.
func writeFixture(name, contents string) func(*testing.T, string) {
	return func(t *testing.T, source string) {
		t.Helper()
		writeStepFixtureFile(t, source, name, contents)
	}
}

// TestEslintExtendsStepApplyWritesAConfigWhenNoneExists pins the spec
// scenario "a project with no ESLint config gets one written": Apply writes
// a complete eslint.config.js importing and spreading the owned factory,
// matching wireFallowExtends's write-if-absent shape.
func TestEslintExtendsStepApplyWritesAConfigWhenNoneExists(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root}

	if _, err := (eslintExtendsStep{}).Apply(p, &Writer{}, io.Discard); err != nil {
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
		"import dharnessLayer from \"./.dharness/eslint.config.js\";",
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

	if _, err := (eslintExtendsStep{}).Apply(p, &Writer{}, io.Discard); err != nil {
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
	if _, err := (eslintExtendsStep{}).Apply(p, &Writer{}, io.Discard); err != nil {
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

	if _, err := (eslintExtendsStep{}).Apply(p, &Writer{}, io.Discard); err != nil {
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
		"import dharnessLayer from \"./.dharness/eslint.config.js\";\n" +
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

	if _, err := (eslintExtendsStep{}).Apply(p, &Writer{}, io.Discard); err != nil {
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

	if _, err := (eslintExtendsStep{}).Apply(p, &Writer{}, io.Discard); err == nil {
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

	if _, err := (eslintExtendsStep{}).Apply(p, &Writer{}, io.Discard); err != nil {
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

	if _, err := (eslintExtendsStep{}).Apply(p, &Writer{}, io.Discard); err != nil {
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

// TestEslintExtendsStepIsNeverSatisfiedAndDelegatedAtOnce pins the invariant
// run() depends on: it asks Satisfied first and never asks Delegated once
// Satisfied answers true, so a step that answers true to both loses its
// explanation entirely — the reason is computed and thrown away, and the step
// lands under "Already in place" with freshly installed dependencies wired to
// nothing.
//
// This inverts what the suite asserted before. The earlier rule — "Delegated
// already explains why nothing will be applied, so Satisfied should not
// double-report the same state as pending" — was written against
// Pending/applySteps, where Satisfied selects and Delegated skips, so both
// answering true was harmless there. run() decides status in the other
// direction, and under it that pair is not a double report: it is a silent
// one.
//
// Each shape is asserted on its own, so no case can be carried by another's
// branch.
func TestEslintExtendsStepIsNeverSatisfiedAndDelegatedAtOnce(t *testing.T) {
	shapes := []struct {
		name string
		file string
		body string
	}{
		{"TypeScript config", "eslint.config.ts", "export default [];\n"},
		{"legacy-only config, no TypeScript config present", ".eslintrc.json", "{}"},
		{"unrecognised call expression", eslintConfig, "export default tseslint.config(\n  { rules: {} },\n);\n"},
		{"malformed marker pair", eslintConfig, eslintImportBegin + "\nexport default [];\n"},
	}

	for _, shape := range shapes {
		t.Run(shape.name, func(t *testing.T) {
			root := t.TempDir()
			writeStepFixtureFile(t, root, shape.file, shape.body)
			p := project.Project{Root: root, Source: root}

			why, delegated := (eslintExtendsStep{}).Delegated(p)
			if !delegated {
				t.Fatal("Delegated() = false; the fixture no longer exercises a delegating shape")
			}
			if why == "" {
				t.Error("Delegated() answered true with no reason to report")
			}
			if (eslintExtendsStep{}).Satisfied(p) {
				t.Error("Satisfied() = true for a shape that delegates: run() reports it as already in place and never prints the reason")
			}
		})
	}
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

	if _, err := (eslintExtendsStep{}).Apply(p, &Writer{}, io.Discard); err != nil {
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
