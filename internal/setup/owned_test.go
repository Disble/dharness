package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/preset"
	"github.com/Disble/dharness/internal/project"
)

func TestRegionIsAbsentWhenNothingIsContributed(t *testing.T) {
	matches := []preset.Match{{ID: "generic", Scope: preset.Root, Manifest: preset.Manifest{Schema: preset.Schema}}}

	if got := presetRegion(matches); got != "" {
		t.Errorf("presetRegion() = %q, want \"\" for an empty manifest", got)
	}
}

func TestApplyPreservesBoundariesOutsideTheRegion(t *testing.T) {
	// No preset with a non-empty manifest exists until slice 5, so the
	// mechanism is proven directly against replaceRegion — exactly the unit
	// the design's Testing Strategy calls for ("region and replace are
	// directly testable" without depending on the real four presets).
	existing := architectureSkeleton + "\n  \"boundaries\": {\n    \"zones\": []\n  }\n"
	region := "  " + presetBegin + "\n  \"ignorePatterns\": [\"wailsjs/**\"],\n  " + presetEnd + "\n"

	got := replaceRegion(existing, region)

	if !strings.Contains(got, `"boundaries": {`) {
		t.Errorf("replaceRegion() dropped the boundaries block:\n%s", got)
	}
	if !strings.Contains(got, `"ignorePatterns": ["wailsjs/**"]`) {
		t.Errorf("replaceRegion() did not write the region:\n%s", got)
	}
}

func TestRegionIsReinsertedWhenTheMarkersAreRemoved(t *testing.T) {
	// Shaped like the motivating repository's own owned file: opens with a
	// "{" then a comment line, no markers anywhere.
	existing := "{\n  // dharness writes this file; the architecture below is decided by analysis,\n  // not by detection.\n}\n"
	region := "  " + presetBegin + "\n  \"ignorePatterns\": [\"wailsjs/**\"],\n  " + presetEnd + "\n"

	got := replaceRegion(existing, region)

	if !strings.HasPrefix(got, "{\n"+region) {
		t.Errorf("replaceRegion() did not insert the region immediately after the first {:\n%s", got)
	}
	if !strings.Contains(got, "// dharness writes this file") {
		t.Errorf("replaceRegion() dropped the file's own content:\n%s", got)
	}
}

func TestRegionReplacesOnlyBetweenTheMarkersOnASecondRun(t *testing.T) {
	first := replaceRegion(architectureSkeleton, "  "+presetBegin+"\n  \"ignorePatterns\": [\"wailsjs/**\"],\n  "+presetEnd+"\n")
	second := replaceRegion(first, "  "+presetBegin+"\n  \"ignorePatterns\": [\"wailsjs/**\", \".next/**\"],\n  "+presetEnd+"\n")

	if strings.Contains(second, `"wailsjs/**"]`) {
		t.Errorf("replaceRegion() left the old region content behind:\n%s", second)
	}
	if !strings.Contains(second, `".next/**"`) {
		t.Errorf("replaceRegion() did not write the new region:\n%s", second)
	}
	if !strings.Contains(second, "// dharness writes this file") {
		t.Errorf("replaceRegion() disturbed content outside the region:\n%s", second)
	}
}

func TestListKeysUnionAcrossScopes(t *testing.T) {
	rootMatch := preset.Match{ID: "root-preset", Scope: preset.Root, Manifest: preset.Manifest{
		Schema: preset.Schema,
		Facts:  []preset.Fact{{Key: "ignorePatterns", Value: []string{"wailsjs/**"}, Because: "root.json declares it"}},
	}}
	sourceMatch := preset.Match{ID: "source-preset", Scope: preset.Source, Manifest: preset.Manifest{
		Schema: preset.Schema,
		Facts:  []preset.Fact{{Key: "ignorePatterns", Value: []string{".next/**"}, Because: "package.json dependency next"}},
	}}

	region := presetRegion([]preset.Match{rootMatch, sourceMatch})

	if !strings.Contains(region, `"wailsjs/**"`) || !strings.Contains(region, `".next/**"`) {
		t.Errorf("presetRegion() did not union both contributed elements:\n%s", region)
	}
	if !strings.Contains(region, "root-preset:") || !strings.Contains(region, "source-preset:") {
		t.Errorf("presetRegion() did not preserve each contributor's evidence:\n%s", region)
	}
}

func TestNoScalarKeyIsContributedTwice(t *testing.T) {
	rootMatch := preset.Match{ID: "root-preset", Scope: preset.Root, Manifest: preset.Manifest{
		Schema: preset.Schema,
		Facts:  []preset.Fact{{Key: "maxDepth", Value: 3, Because: "root.json declares it"}},
	}}
	sourceMatch := preset.Match{ID: "source-preset", Scope: preset.Source, Manifest: preset.Manifest{
		Schema: preset.Schema,
		Facts:  []preset.Fact{{Key: "maxDepth", Value: 5, Because: "package.json declares it"}},
	}}

	region := presetRegion([]preset.Match{rootMatch, sourceMatch})

	if !strings.Contains(region, `"maxDepth": 3`) {
		t.Errorf("presetRegion() did not let Root scope win the scalar collision:\n%s", region)
	}
	if strings.Contains(region, `"maxDepth": 5`) {
		t.Errorf("presetRegion() wrote the losing scalar as a live value, not a comment:\n%s", region)
	}
	if !strings.Contains(region, "root-preset:") || !strings.Contains(region, "source-preset:") {
		t.Errorf("presetRegion() did not name both contributors:\n%s", region)
	}
}

// TestArchitectureStepStaysUnsatisfiedWithoutTheAgentBlock pins design
// decision 8's second guard: the region and architectureStep's own check are
// independent. A Wails-shaped fixture (Root != Source, matching generic
// here since no framework preset exists until slice 5) with a fully-applied
// owned file still has no `boundaries` declared, so the step it gates stays
// unsatisfied regardless of what the region machinery wrote.
func TestArchitectureStepStaysUnsatisfiedWithoutTheAgentBlock(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "frontend")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	p := project.At(root, source)
	p.InRepository = true

	w := &Writer{}
	if err := (ownedFilesStep{}).Apply(p, w); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	if (architectureStep{}).Satisfied(p) {
		t.Error("architectureStep.Satisfied() = true after ownedFilesStep.Apply alone, want false — the agent has not written boundaries yet")
	}
}

// TestRegionBytesExtractsExactlyWhatWasWritten pins regionBounds's byte
// arithmetic: the extracted region must be the marker lines and everything
// between them, no more and no fewer bytes, on both sides.
func TestRegionBytesExtractsExactlyWhatWasWritten(t *testing.T) {
	region := "  " + presetBegin + "\n  \"ignorePatterns\": [\"wailsjs/**\"],\n  " + presetEnd + "\n"
	raw := "{\n  // before\n" + region + "  // after\n}\n"

	if got := regionBytes(raw); got != region {
		t.Errorf("regionBytes() = %q, want %q", got, region)
	}
}

// TestRegionBytesReachesEndOfStringWithNoTrailingNewline pins the branch
// regionBounds takes when the end marker's own line is the last thing in the
// file — there is no further "\n" to bound the region against, so it must
// run to len(raw) rather than stop short.
func TestRegionBytesReachesEndOfStringWithNoTrailingNewline(t *testing.T) {
	region := "  " + presetBegin + "\n  \"k\": 1,\n  " + presetEnd
	raw := "{\n" + region

	if got := regionBytes(raw); got != region {
		t.Errorf("regionBytes() = %q, want %q", got, region)
	}
}

// TestRegionBytesFindsMarkersAtTheVeryStartOfTheString pins beginIdx == 0 as
// a legitimate found position, not the sentinel for "absent".
func TestRegionBytesFindsMarkersAtTheVeryStartOfTheString(t *testing.T) {
	raw := presetBegin + "\n  \"k\": 1,\n  " + presetEnd + "\n"

	got := regionBytes(raw)
	if got == "" || !strings.Contains(got, presetBegin) {
		t.Errorf("regionBytes() = %q, want the region extracted even though it begins at byte 0", got)
	}
}

// TestRegionBytesEmptyWithoutBothMarkers covers the guard's three failure
// shapes — only begin present, only end present, and the markers in the
// wrong order — none of which is a valid region.
func TestRegionBytesEmptyWithoutBothMarkers(t *testing.T) {
	cases := map[string]string{
		"neither marker":   "{\n  // nothing here\n}\n",
		"begin only":       "{\n  " + presetBegin + "\n}\n",
		"end only":         "{\n  " + presetEnd + "\n}\n",
		"end before begin": "{\n  " + presetEnd + "\n  " + presetBegin + "\n}\n",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if got := regionBytes(raw); got != "" {
				t.Errorf("regionBytes(%q) = %q, want \"\"", raw, got)
			}
		})
	}
}

// TestReplaceRegionLeavesAFileWithNoBraceUnchanged pins brace == -1 as the
// guard against inserting into a file that never had one — the case Apply
// can never actually produce, but the function must not corrupt input it
// does not recognise rather than silently splicing at index 0.
func TestReplaceRegionLeavesAFileWithNoBraceUnchanged(t *testing.T) {
	existing := "not json at all\n"
	got := replaceRegion(existing, "  "+presetBegin+"\n  \"k\": 1,\n  "+presetEnd+"\n")
	if got != existing {
		t.Errorf("replaceRegion() = %q, want %q unchanged", got, existing)
	}
}

// TestPresetRegionRendersASingleContributorFact pins that composition works
// with exactly one contributor for a key, not only the two-contributor union
// and collision cases above — real slice-2 traffic (generic alone) never
// reaches this, but slice 5's first single-preset match will.
func TestPresetRegionRendersASingleContributorFact(t *testing.T) {
	m := preset.Match{ID: "solo-preset", Scope: preset.Root, Manifest: preset.Manifest{
		Schema: preset.Schema,
		Facts:  []preset.Fact{{Key: "ignorePatterns", Value: []string{"wailsjs/**"}, Because: "wails.json"}},
	}}

	region := presetRegion([]preset.Match{m})
	if !strings.Contains(region, `"ignorePatterns": ["wailsjs/**"]`) {
		t.Errorf("presetRegion() = %q, want the single contributor's value rendered", region)
	}
}

// TestPresetRegionProcessesEveryKeyNotJustTheFirst pins that composing one
// key's facts does not stop composition from reaching the next key.
func TestPresetRegionProcessesEveryKeyNotJustTheFirst(t *testing.T) {
	m := preset.Match{ID: "solo-preset", Scope: preset.Root, Manifest: preset.Manifest{
		Schema: preset.Schema,
		Facts: []preset.Fact{
			{Key: "ignorePatterns", Value: []string{"wailsjs/**"}, Because: "wails.json"},
			{Key: "ignoreDependencies", Value: []string{"react"}, Because: "package.json"},
		},
	}}

	region := presetRegion([]preset.Match{m})
	if !strings.Contains(region, `"ignorePatterns"`) || !strings.Contains(region, `"ignoreDependencies"`) {
		t.Errorf("presetRegion() did not render every contributed key:\n%s", region)
	}
}

// TestScalarCollisionCommentsTheWinnerExactlyOnce guards the boundary
// between the winner's own comment line and the loop that comments every
// losing contributor: the winner must not also appear in that loop.
func TestScalarCollisionCommentsTheWinnerExactlyOnce(t *testing.T) {
	rootMatch := preset.Match{ID: "root-preset", Scope: preset.Root, Manifest: preset.Manifest{
		Schema: preset.Schema,
		Facts:  []preset.Fact{{Key: "maxDepth", Value: 3, Because: "root.json declares it"}},
	}}
	sourceMatch := preset.Match{ID: "source-preset", Scope: preset.Source, Manifest: preset.Manifest{
		Schema: preset.Schema,
		Facts:  []preset.Fact{{Key: "maxDepth", Value: 5, Because: "package.json declares it"}},
	}}

	region := presetRegion([]preset.Match{rootMatch, sourceMatch})

	if got := strings.Count(region, "root-preset:"); got != 1 {
		t.Errorf("presetRegion() names the winner %d times, want exactly 1:\n%s", got, region)
	}
}

func TestOwnedFilesSatisfiedRequiresEveryOwnedFileToExist(t *testing.T) {
	root := t.TempDir()
	p := project.At(root, root)
	p.InRepository = true

	if err := os.MkdirAll(filepath.Join(root, project.Dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, project.Dir, ownedLefthook), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// fallow.jsonc matches the current region byte-for-byte (both empty, no
	// preset contributes anything to a generic project), so only the loop
	// over ownedLefthook/ownedRules can still fail this — ownedRules is
	// deliberately absent, and the loop must not skip checking it just
	// because ownedLefthook, checked first, already exists.
	if err := os.WriteFile(filepath.Join(root, project.Dir, ownedFallow), []byte(architectureSkeleton), 0o600); err != nil {
		t.Fatal(err)
	}

	if (ownedFilesStep{}).Satisfied(p) {
		t.Error("Satisfied() = true with rules.json missing, even though lefthook.yml and fallow.jsonc's region both already match")
	}
}

func TestOwnedFilesSatisfiedComparesRegionBytesOnly(t *testing.T) {
	root := t.TempDir()
	p := project.At(root, root)
	p.InRepository = true

	// Not satisfied before Apply has run at all.
	if (ownedFilesStep{}).Satisfied(p) {
		t.Fatal("Satisfied() = true before the owned files exist")
	}

	w := &Writer{}
	if err := (ownedFilesStep{}).Apply(p, w); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	// generic contributes nothing, so a freshly-applied project satisfies the
	// step regardless of what its region byte-comparison would say about a
	// framework-matched one — this pins the existence-plus-region contract
	// end to end, not just presetRegion's own composition rule.
	if !(ownedFilesStep{}).Satisfied(p) {
		t.Fatal("Satisfied() = false immediately after Apply()")
	}
}

// TestApplyWritesTheOwnedEslintConfigAndDeclaresItShared is task 2a.7's
// observable: a fresh .dharness/ after Apply holds eslint.config.js and an
// allow list naming it, so the file is never gitignored by default.
func TestApplyWritesTheOwnedEslintConfigAndDeclaresItShared(t *testing.T) {
	root := t.TempDir()
	p := project.At(root, root)
	p.InRepository = true

	w := &Writer{}
	if err := (ownedFilesStep{}).Apply(p, w); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	eslintPath := filepath.Join(root, project.Dir, ownedEslint)
	raw, err := os.ReadFile(eslintPath)
	if err != nil {
		t.Fatalf("Apply() did not write %s: %v", ownedEslint, err)
	}
	if want := ownedEslintConfig(p, preset.Layers(preset.Resolve(p))); string(raw) != want {
		t.Errorf("Apply() wrote %q, want %q", raw, want)
	}

	ignore, err := os.ReadFile(filepath.Join(root, project.Dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ignore), "!eslint.config.js") {
		t.Errorf("Apply() left the allow list without eslint.config.js:\n%s", ignore)
	}
}

// TestOwnedFilesSatisfiedRequiresTheEslintAllowListEntry pins task 2a.6's
// repair clause: hand-removing the allow-list entry for eslint.config.js
// must bring the step back, derived rather than recorded (§15) — the file
// itself is left alone; only the declaration that it is shared is missing.
func TestOwnedFilesSatisfiedRequiresTheEslintAllowListEntry(t *testing.T) {
	root := t.TempDir()
	p := project.At(root, root)
	p.InRepository = true

	if err := (ownedFilesStep{}).Apply(p, &Writer{}); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if !(ownedFilesStep{}).Satisfied(p) {
		t.Fatal("Satisfied() = false immediately after Apply()")
	}

	ignorePath := filepath.Join(root, project.Dir, ".gitignore")
	raw, err := os.ReadFile(ignorePath)
	if err != nil {
		t.Fatal(err)
	}
	repaired := strings.ReplaceAll(string(raw), "!eslint.config.js\n", "")
	if err := os.WriteFile(ignorePath, []byte(repaired), 0o600); err != nil {
		t.Fatal(err)
	}

	if (ownedFilesStep{}).Satisfied(p) {
		t.Error("Satisfied() = true after the allow-list entry was removed by hand, want false")
	}
}

// TestOwnedFilesSatisfiedComparesTheEslintConfigBytes triangulates the
// clause above with a different cause: the allow list stays intact but the
// rendered config itself changed underneath it (a preset started
// contributing, or the barrel default flipped) — the first-write-only limit
// design decision 8 retires, proven directly against Satisfied rather than
// by re-running Apply and hoping it converges.
func TestOwnedFilesSatisfiedComparesTheEslintConfigBytes(t *testing.T) {
	root := t.TempDir()
	p := project.At(root, root)
	p.InRepository = true

	if err := (ownedFilesStep{}).Apply(p, &Writer{}); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	eslintPath := filepath.Join(root, project.Dir, ownedEslint)
	if err := os.WriteFile(eslintPath, []byte("stale bytes from an earlier run\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if (ownedFilesStep{}).Satisfied(p) {
		t.Error("Satisfied() = true with a stale eslint.config.js, want false")
	}
}

// TestOwnedFilesSatisfiedFalseWhenTheEslintConfigFileIsMissing distinguishes
// "the file is stale" (TestOwnedFilesSatisfiedComparesTheEslintConfigBytes)
// from "the file was never written at all" — the fallow.jsonc region already
// matches (generic contributes nothing), so only the eslint.config.js read
// failure can make Satisfied answer false here.
func TestOwnedFilesSatisfiedFalseWhenTheEslintConfigFileIsMissing(t *testing.T) {
	root := t.TempDir()
	p := project.At(root, root)
	p.InRepository = true

	if err := (ownedFilesStep{}).Apply(p, &Writer{}); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if err := os.Remove(filepath.Join(root, project.Dir, ownedEslint)); err != nil {
		t.Fatal(err)
	}

	if (ownedFilesStep{}).Satisfied(p) {
		t.Error("Satisfied() = true with eslint.config.js missing entirely, want false")
	}
}

// TestOwnedFilesSatisfiedFalseWhenTheGitignoreFileIsMissing is the same
// distinction for the allow-list check: a .gitignore that does not exist at
// all is not the same reachable state as one that is merely missing the
// entry, but Satisfied must answer false for both.
func TestOwnedFilesSatisfiedFalseWhenTheGitignoreFileIsMissing(t *testing.T) {
	root := t.TempDir()
	p := project.At(root, root)
	p.InRepository = true

	if err := (ownedFilesStep{}).Apply(p, &Writer{}); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if err := os.Remove(filepath.Join(root, project.Dir, ".gitignore")); err != nil {
		t.Fatal(err)
	}

	if (ownedFilesStep{}).Satisfied(p) {
		t.Error("Satisfied() = true with .gitignore missing entirely, want false")
	}
}

// The unit tests above prove replaceRegion. This proves the wiring, which is a
// different question: Apply must read the file that is already there rather
// than rewriting the skeleton over it. If it did the latter, every test above
// would still pass and the agent's boundaries block would still die on the
// second run — the exact failure Decision 8 exists to prevent.
//
// Repeated three times because once proves nothing about idempotence: the file
// is written, read back, and written again, and the bytes must not drift.
func TestApplyReadsTheExistingFileRatherThanRewritingTheSkeleton(t *testing.T) {
	root := t.TempDir()
	p := project.Project{Root: root, Source: root, PackageManager: "bun"}
	if _, err := p.EnsureDir(""); err != nil {
		t.Fatal(err)
	}

	authored := "{\n  // written by the agent\n  \"boundaries\": {\n    \"zones\": [{\"name\": \"app\"}]\n  }\n}\n"
	path := filepath.Join(root, project.Dir, ownedFallow)
	if err := os.WriteFile(path, []byte(authored), 0o600); err != nil {
		t.Fatal(err)
	}

	var after [3]string
	for run := range after {
		if err := (ownedFilesStep{}).Apply(p, &Writer{}); err != nil {
			t.Fatalf("Apply() run %d = %v", run+1, err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		after[run] = string(raw)
	}

	// Two properties, and only the first one changed when generic started
	// contributing a cross-cutting fact. The agent's block must survive every
	// run — that is Decision 8's whole reason for existing — and the bytes
	// must not drift after the first write, or every sync produces a diff.
	if !strings.Contains(after[2], `"boundaries"`) || !strings.Contains(after[2], "written by the agent") {
		t.Errorf("the agent's block did not survive three Apply runs:\n%s", after[2])
	}
	if after[1] != after[0] || after[2] != after[0] {
		t.Errorf("the file drifted across runs:\nrun 1 %q\nrun 2 %q\nrun 3 %q", after[0], after[1], after[2])
	}
	if !strings.HasSuffix(after[2], "}\n") {
		t.Errorf("the file no longer closes cleanly:\n%s", after[2])
	}
}
