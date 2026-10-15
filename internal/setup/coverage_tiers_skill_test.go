package setup

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/report"
	"github.com/Disble/dharness/internal/tiers"
)

// These scenarios own the fourteenth step's contract: one embedded source, two
// managed copies, and a bounded reference inside the instruction files the
// project already keeps — never a file dharness creates, never a byte outside
// the region it owns, and never a partial write while one destination refuses.
//
// The repo's own guidance (docs/coverage-tiers.md) explains the policy; it is
// never a second classification authority. Everything asserted here is about
// what reaches disk and what a run reports, not about the wording of the
// procedure, except where the prompt's honesty requirement is the contract: the
// asset must say it is guidance rather than enforcement.

// coverageTiersSkillFixture is the opted-in repository: a declared manifest
// with the runner and config the coverage step needs. The skill step itself
// reads none of that — it follows the declaration alone — so the fixture is
// shared with the coverage scenarios rather than invented here.
func coverageTiersSkillFixture(t *testing.T) project.Project {
	t.Helper()
	fixture := coverageGateAdopted(t, false)
	return project.At(fixture.root, fixture.source)
}

// coverageTiersSkillPaths are the two copies this step owns, in the order the
// plan derives them.
func coverageTiersSkillPaths(p project.Project) (claude, agents string) {
	return filepath.Join(p.Root, ".claude", "skills", "coverage-tiers", "SKILL.md"),
		filepath.Join(p.Root, ".agents", "skills", "coverage-tiers", "SKILL.md")
}

// runCoverageTiersSkill drives this one step through run(), so every scenario
// reads the status and reason a person is shown rather than a private boolean.
func runCoverageTiersSkill(t *testing.T, p project.Project) report.StepResult {
	t.Helper()
	steps, _, err := run([]Step{coverageTiersSkillStep{}}, p)
	if err != nil {
		t.Fatalf("run() = %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("run() returned %d steps, want 1", len(steps))
	}
	return steps[0]
}

// coverageTiersSkillRegionCount counts one marker in a file, so "exactly one
// managed region" is asserted rather than assumed from a substring search.
func coverageTiersSkillRegionCount(t *testing.T, path, marker string) int {
	t.Helper()
	return strings.Count(readFixtureFile(t, path), marker)
}

// TestCoverageTiersSkillIsTheEmbeddedManagedAsset mirrors the runtime module's
// own contract: the accessor is the embedded source byte for byte, and it hands
// out a copy so a caller that edits what it received cannot corrupt the asset
// for the rest of the process.
func TestCoverageTiersSkillIsTheEmbeddedManagedAsset(t *testing.T) {
	asset, err := os.ReadFile("assets/coverage-tiers/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}

	copy := coverageTiersSkill()
	if len(copy) == 0 || !bytes.Equal(copy, asset) {
		t.Fatal("the guidance accessor must return the embedded asset's bytes")
	}
	copy[0] ^= 1
	if !bytes.Equal(coverageTiersSkill(), asset) {
		t.Fatal("a caller's write changed the embedded guidance")
	}
}

// TestCoverageTiersSkillAssetCarriesTheProcedure pins the parts of the guidance
// a consumer's agent must be able to find without reading dharness's source:
// the tiers, the declaration, the wired gate, and the boundary that the skill
// is not the enforcement.
func TestCoverageTiersSkillAssetCarriesTheProcedure(t *testing.T) {
	body := string(coverageTiersSkill())
	for _, want := range []struct{ needle, why string }{
		{"name: coverage-tiers", "the frontmatter names the skill a client loads"},
		{"This skill is guidance, not the enforcement", "the skill states the boundary the gate owns"},
		{"100%", "the CORE target"},
		{"80%", "the IMPORTANT target"},
		{"infrastructure", "the third tier"},
		{tiers.ManifestName, "the declaration an agent reads before classifying"},
		{".dharness/vitest.coverage.mjs", "the gate entry a complete local run points Vitest at"},
		{"dharness mutate", "how mutation scope follows the classification"},
	} {
		if !strings.Contains(body, want.needle) {
			t.Errorf("the guidance never mentions %q (%s)", want.needle, want.why)
		}
	}
}

// TestCoverageTiersSkillStepIsTheFourteenthPlanStep pins the plan's shape: the
// guidance step is appended immediately after the step whose declaration
// activates it, wherever the plan's other steps sit.
func TestCoverageTiersSkillStepIsTheFourteenthPlanStep(t *testing.T) {
	plan := Plan()
	if len(plan) != 14 {
		t.Fatalf("len(Plan()) = %d, want 14 — the coverage-tiers guidance is the fourteenth step", len(plan))
	}
	if got := plan[12].ID(); got != (coverageTiersStep{}).ID() {
		t.Errorf("Plan()[12] = %q, want the tier enforcement step before the guidance", got)
	}
	if got := plan[13].ID(); got != (coverageTiersSkillStep{}).ID() {
		t.Errorf("Plan()[13] = %q, want %q", got, (coverageTiersSkillStep{}).ID())
	}
	if _, ok := plan[13].(coverageTiersSkillStep); !ok {
		t.Errorf("Plan()[13] is %T, want coverageTiersSkillStep", plan[13])
	}
}

// TestCoverageTiersSkillStepWritesTheCanonicalCopies is the distribution
// scenario: one run writes both clients' copies from the embedded asset, and
// reports exactly those two files.
func TestCoverageTiersSkillStepWritesTheCanonicalCopies(t *testing.T) {
	p := coverageTiersSkillFixture(t)
	claude, agents := coverageTiersSkillPaths(p)

	result := runCoverageTiersSkill(t, p)
	if result.Status != report.Applied {
		t.Fatalf("the step was reported %s (error %q), want applied", result.Status, result.Error)
	}

	for _, path := range []string{claude, agents} {
		if got, want := readFixtureFile(t, path), string(coverageTiersSkill()); got != want {
			t.Errorf("%s is not the embedded source byte for byte", rootRelative(p, path))
		}
	}

	wrote := map[string]bool{}
	for _, change := range result.Wrote {
		wrote[change.Path] = true
		if change.Kind != report.Created {
			t.Errorf("%s was reported %s, want created", change.Path, change.Kind)
		}
	}
	if len(result.Wrote) != 2 || !wrote[filepath.ToSlash(filepath.Join(".claude", "skills", "coverage-tiers", "SKILL.md"))] ||
		!wrote[filepath.ToSlash(filepath.Join(".agents", "skills", "coverage-tiers", "SKILL.md"))] {
		t.Errorf("the run reported touching %v, want exactly the two skill copies", result.Wrote)
	}
}

// TestCoverageTiersSkillStepLeavesAnUndeclaredRepositoryAlone is the boundary
// the whole feature keeps: no declaration means no adoption, so this step
// writes nothing at all — no copy, no directory for one, no instruction file.
func TestCoverageTiersSkillStepLeavesAnUndeclaredRepositoryAlone(t *testing.T) {
	stubCoverageProcesses(t)

	root := t.TempDir()
	writeGoldenFixtureFile(t, root, "package.json", "{\n  \"name\": \"app\"\n}\n")
	writeGoldenFixtureFile(t, root, "package-lock.json", "{\n  \"lockfileVersion\": 3\n}\n")
	p := project.At(root, root)

	if !(coverageTiersSkillStep{}).Satisfied(p) {
		t.Fatal("Satisfied() = false for a repository that never declared the tiers")
	}
	if result := runCoverageTiersSkill(t, p); result.Status != report.Satisfied {
		t.Fatalf("the report filed the step as %s, want satisfied", result.Status)
	} else if result.Evidence != "no "+tiers.ManifestName {
		t.Errorf("Evidence = %q, want the declaration that is absent", result.Evidence)
	}

	if err := Apply(p, io.Discard); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	for _, path := range []string{
		filepath.Join(root, ".claude"),
		filepath.Join(root, ".agents"),
		filepath.Join(root, agentsInstructions),
		filepath.Join(root, claudeInstructions),
	} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("a repository that never declared tiers gained %s", rootRelative(p, path))
		}
	}
}

// TestCoverageTiersSkillStepConvergesOnASecondRun is idempotence: the copies
// and the managed region are read back off disk, so a second run neither
// rewrites them nor adds a duplicate block.
func TestCoverageTiersSkillStepConvergesOnASecondRun(t *testing.T) {
	p := coverageTiersSkillFixture(t)
	claude, agents := coverageTiersSkillPaths(p)
	writeGoldenFixtureFile(t, p.Root, agentsInstructions, "# House rules\n\nBe kind to the reviewer.\n")

	if result := runCoverageTiersSkill(t, p); result.Status != report.Applied {
		t.Fatalf("the first run was reported %s (error %q), want applied", result.Status, result.Error)
	}

	first := map[string]string{}
	for _, path := range []string{claude, agents, filepath.Join(p.Root, agentsInstructions)} {
		first[path] = readFixtureFile(t, path)
	}

	result := runCoverageTiersSkill(t, p)
	if result.Status != report.Satisfied {
		t.Fatalf("the converged step reported %s (wrote %v, error %q), want satisfied", result.Status, result.Wrote, result.Error)
	}
	if result.Evidence != "skill copies and references match" {
		t.Errorf("Evidence = %q, want the fact that the managed copies match", result.Evidence)
	}
	// Nothing was written, so nothing may be reported as touched: on a
	// converged repository the run's own report is where a re-write of
	// identical bytes would first show up.
	if len(result.Wrote) != 0 {
		t.Errorf("the converged run reported touching %v, want nothing", result.Wrote)
	}
	for path, want := range first {
		if got := readFixtureFile(t, path); got != want {
			t.Errorf("the second run changed %s", rootRelative(p, path))
		}
	}
	if got := coverageTiersSkillRegionCount(t, filepath.Join(p.Root, agentsInstructions), coverageTiersGuideBegin); got != 1 {
		t.Errorf("AGENTS.md carries %d managed regions, want 1", got)
	}
}

// TestCoverageTiersSkillStepKeepsEveryByteOutsideTheRegion is the bounded half
// of the reference: the project's own instructions survive verbatim, the
// region is appended once, and an instruction file that does not exist is never
// created.
func TestCoverageTiersSkillStepKeepsEveryByteOutsideTheRegion(t *testing.T) {
	p := coverageTiersSkillFixture(t)
	claude, agents := coverageTiersSkillPaths(p)

	const house = "# House rules\n\nBe kind to the reviewer.\n"
	writeGoldenFixtureFile(t, p.Root, agentsInstructions, house)
	const claudeHouse = "# Claude\n\nUse the project's own commands.\n"
	writeGoldenFixtureFile(t, p.Root, claudeInstructions, claudeHouse)

	if result := runCoverageTiersSkill(t, p); result.Status != report.Applied {
		t.Fatalf("the step was reported %s (error %q), want applied", result.Status, result.Error)
	}

	agentsBody := readFixtureFile(t, filepath.Join(p.Root, agentsInstructions))
	if !strings.HasPrefix(agentsBody, house) {
		t.Errorf("the managed region did not arrive after the project's own bytes:\n%s", agentsBody)
	}
	if got := coverageTiersSkillRegionCount(t, filepath.Join(p.Root, agentsInstructions), coverageTiersGuideBegin); got != 1 {
		t.Errorf("AGENTS.md carries %d managed regions, want 1", got)
	}
	if !strings.Contains(agentsBody, ".agents/skills/coverage-tiers/SKILL.md") {
		t.Errorf("AGENTS.md does not point at the copy its clients load:\n%s", agentsBody)
	}
	// The region is the only thing dharness writes into a project's own file, so
	// it may name the copies this binary ships and nothing else: a path that only
	// exists in dharness's repository would send the reader somewhere they cannot
	// go. This is the assertion that keeps the pointer honest.
	for _, absent := range []string{"docs/coverage-tiers.md"} {
		if strings.Contains(agentsBody, absent) {
			t.Errorf("AGENTS.md points at %q, which this repository does not own:\n%s", absent, agentsBody)
		}
	}

	claudeBody := readFixtureFile(t, filepath.Join(p.Root, claudeInstructions))
	if !strings.HasPrefix(claudeBody, claudeHouse) {
		t.Errorf("the managed region did not arrive after the project's own bytes:\n%s", claudeBody)
	}
	if !strings.Contains(claudeBody, ".claude/skills/coverage-tiers/SKILL.md") {
		t.Errorf("CLAUDE.md does not point at the copy its client loads:\n%s", claudeBody)
	}
	for _, absent := range []string{"docs/coverage-tiers.md"} {
		if strings.Contains(claudeBody, absent) {
			t.Errorf("CLAUDE.md points at %q, which this repository does not own:\n%s", absent, claudeBody)
		}
	}

	// An unrelated project file is untouched, and the two copies exist where
	// the regions point at them.
	if got := readFixtureFile(t, filepath.Join(p.Root, "package.json")); !strings.Contains(got, `"name": "app"`) {
		t.Errorf("an unrelated project file changed:\n%s", got)
	}
	for _, path := range []string{claude, agents} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("the region points at %s, and it was not written: %v", rootRelative(p, path), err)
		}
	}

	// A file whose own bytes do not end with a line ending is the case where
	// the region builder inserts the separator itself: every byte the project
	// wrote must survive whole, the region must arrive on a line of its own,
	// and the result is asserted byte for byte rather than by prefix, because
	// a file that lost its opening bytes would still start with a region that
	// a substring check accepts.
	t.Run("an instruction file with no trailing line ending keeps every byte", func(t *testing.T) {
		p := coverageTiersSkillFixture(t)
		const tail = "# House rules\n\nBe kind to the reviewer."
		writeGoldenFixtureFile(t, p.Root, agentsInstructions, tail)

		if result := runCoverageTiersSkill(t, p); result.Status != report.Applied {
			t.Fatalf("the step was reported %s (error %q), want applied", result.Status, result.Error)
		}

		doc := coverageTiersGuideDocs[0]
		if doc.name != agentsInstructions {
			t.Fatalf("coverageTiersGuideDocs[0] is %q, want %q", doc.name, agentsInstructions)
		}
		got := readFixtureFile(t, filepath.Join(p.Root, agentsInstructions))
		if want := tail + "\n\n" + coverageTiersGuideRegion(doc, "\n"); got != want {
			t.Errorf("AGENTS.md is not its own bytes, one blank line and the region:\ngot  %q\nwant %q", got, want)
		}
	})
}

// TestCoverageTiersSkillStepRepairsDriftItOwns is the repair half: a
// hand-edited copy and a hand-edited region are both restored, and the bytes
// around the region are left alone.
func TestCoverageTiersSkillStepRepairsDriftItOwns(t *testing.T) {
	p := coverageTiersSkillFixture(t)
	claude, agents := coverageTiersSkillPaths(p)
	const house = "# House rules\n\nBe kind to the reviewer.\n"
	writeGoldenFixtureFile(t, p.Root, agentsInstructions, house)

	if result := runCoverageTiersSkill(t, p); result.Status != report.Applied {
		t.Fatalf("the first run was reported %s (error %q), want applied", result.Status, result.Error)
	}

	if err := os.WriteFile(claude, []byte("# hand written\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	agentsPath := filepath.Join(p.Root, agentsInstructions)
	edited := strings.Replace(readFixtureFile(t, agentsPath), "Before changing one, read", "Edited by hand.", 1)
	if edited == readFixtureFile(t, agentsPath) {
		t.Fatalf("the fixture never gained the managed reference:\n%s", edited)
	}
	if err := os.WriteFile(agentsPath, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}

	if result := runCoverageTiersSkill(t, p); result.Status != report.Applied {
		t.Fatalf("the repairing run was reported %s (error %q), want applied", result.Status, result.Error)
	}

	if got := readFixtureFile(t, claude); got != string(coverageTiersSkill()) {
		t.Errorf("the hand-edited copy was not restored to the canonical bytes:\n%s", got)
	}
	repaired := readFixtureFile(t, agentsPath)
	if !strings.HasPrefix(repaired, house) {
		t.Errorf("the repair touched the project's own bytes:\n%s", repaired)
	}
	if strings.Contains(repaired, "Edited by hand.") {
		t.Errorf("the repair left the hand-edited region in place:\n%s", repaired)
	}
	if got := coverageTiersSkillRegionCount(t, agentsPath, coverageTiersGuideBegin); got != 1 {
		t.Errorf("the repaired AGENTS.md carries %d managed regions, want 1", got)
	}
	if got := readFixtureFile(t, agents); got != string(coverageTiersSkill()) {
		t.Errorf("the untouched copy was rewritten:\n%s", got)
	}

	// The report separates the files this run wrote from the files it found
	// already correct. Only the copy is hand-edited again, so the plan's other
	// two files — the second copy and the region — may not appear in the
	// report at all, not even as `unchanged`, which still says this run
	// handled them.
	if err := os.WriteFile(claude, []byte("# hand written again\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	second := runCoverageTiersSkill(t, p)
	if second.Status != report.Applied {
		t.Fatalf("the second repairing run was reported %s (error %q), want applied", second.Status, second.Error)
	}
	if len(second.Wrote) != 1 || second.Wrote[0].Path != rootRelative(p, claude) || second.Wrote[0].Kind != report.Modified {
		t.Errorf("the second repairing run reported touching %v, want exactly %s as modified",
			second.Wrote, rootRelative(p, claude))
	}
}

// TestCoverageTiersSkillStepRefusesAnUnwritableDestination is the refusal half
// of the copy contract: a destination this step cannot replace is reported as
// the agent's to fix, and no other copy is written while it is refused.
func TestCoverageTiersSkillStepRefusesAnUnwritableDestination(t *testing.T) {
	p := coverageTiersSkillFixture(t)
	claude, agents := coverageTiersSkillPaths(p)

	// A directory where the skill file must be: os.WriteFile would replace a
	// link's target or fail halfway, and neither is a state dharness guesses
	// its way out of. The same branch answers for a symlink, which is why the
	// assertion is on the reason rather than on how the file was made.
	if err := os.MkdirAll(claude, 0o755); err != nil {
		t.Fatal(err)
	}

	if (coverageTiersSkillStep{}).Satisfied(p) {
		t.Fatal("Satisfied() = true with a destination this step cannot replace")
	}
	why, delegated := (coverageTiersSkillStep{}).Delegated(p)
	if !delegated {
		t.Fatal("Delegated() = false, want the refusal instead of a write")
	}
	for _, want := range []string{filepath.ToSlash(filepath.Join(".claude", "skills", "coverage-tiers", "SKILL.md")), "not a regular file", "dharness sync"} {
		if !strings.Contains(why, want) {
			t.Errorf("the refusal does not name %q:\n%s", want, why)
		}
	}

	result := runCoverageTiersSkill(t, p)
	if result.Status != report.Delegated || result.Why != why {
		t.Fatalf("the run reported %s (why %q), want the refusal %q", result.Status, result.Why, why)
	}
	if _, err := os.Stat(agents); err == nil {
		t.Error("the step wrote the second copy while the first destination was refused")
	}
	if _, err := os.Stat(filepath.Join(p.Root, agentsInstructions)); err == nil {
		t.Error("the step created an instruction file while a destination was refused")
	}
}

// TestCoverageTiersSkillStepRefusesAPartialOrDuplicatedRegion pins the
// three-way region answer: an absent region is inserted, a present one is
// replaced, and a half-written or duplicated one is refused with the fix
// named rather than repaired by guesswork.
func TestCoverageTiersSkillStepRefusesAPartialOrDuplicatedRegion(t *testing.T) {
	region := strings.Join([]string{
		coverageTiersGuideBegin,
		"Coverage targets: CORE 100%, IMPORTANT 80%, INFRASTRUCTURE 0%; classification lives",
		"in `coverage-tiers.json`, and the wired gate enforces it. Before changing one, read",
		"`.agents/skills/coverage-tiers/SKILL.md` and the guide `docs/coverage-tiers.md`.",
		coverageTiersGuideEnd,
		"",
	}, "\n")

	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "only the begin marker", body: "# House rules\n\n" + coverageTiersGuideBegin + "\n"},
		{name: "two complete regions", body: "# House rules\n\n" + region + region},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := coverageTiersSkillFixture(t)
			claude, agents := coverageTiersSkillPaths(p)
			writeGoldenFixtureFile(t, p.Root, agentsInstructions, tc.body)

			if (coverageTiersSkillStep{}).Satisfied(p) {
				t.Fatal("Satisfied() = true for a region this step cannot bound")
			}
			why, delegated := (coverageTiersSkillStep{}).Delegated(p)
			if !delegated {
				t.Fatal("Delegated() = false, want the refusal instead of a rewrite")
			}
			for _, want := range []string{agentsInstructions, coverageTiersGuideBegin, "Delete"} {
				if !strings.Contains(why, want) {
					t.Errorf("the refusal does not name %q:\n%s", want, why)
				}
			}

			result := runCoverageTiersSkill(t, p)
			if result.Status != report.Delegated {
				t.Fatalf("the run reported %s (error %q), want delegated", result.Status, result.Error)
			}
			if got := readFixtureFile(t, filepath.Join(p.Root, agentsInstructions)); got != tc.body {
				t.Errorf("the refused run rewrote AGENTS.md:\n%s", got)
			}
			for _, path := range []string{claude, agents} {
				if _, err := os.Stat(path); err == nil {
					t.Errorf("the step wrote %s while its reference was refused", rootRelative(p, path))
				}
			}
		})
	}
}

// TestCoverageTiersSkillStepReadsNothingWithoutARoot is the known boundary: a
// project with no directory has no instruction file to read and no copy path to
// build, and joining onto an empty root would stat this process's own working
// directory.
func TestCoverageTiersSkillStepReadsNothingWithoutARoot(t *testing.T) {
	if !(coverageTiersSkillStep{}).Satisfied(project.Project{}) {
		t.Error("Satisfied() = false for a project with no directory")
	}
	if why, delegated := (coverageTiersSkillStep{}).Delegated(project.Project{}); delegated {
		t.Errorf("Delegated() = %q for a project with no directory", why)
	}
	if plan := planCoverageTiersSkill(project.Project{}); plan.conflict != "" || len(plan.files) != 0 {
		t.Errorf("planCoverageTiersSkill() = %+v for a project with no directory, want an empty plan", plan)
	}
}

// TestCoverageTiersSkillStepRefusesAnUnreadableInstructionFile is the same
// refusal one file over: an instruction file the step cannot read is the
// project's to fix, and the copies are not written while it stands.
func TestCoverageTiersSkillStepRefusesAnUnreadableInstructionFile(t *testing.T) {
	p := coverageTiersSkillFixture(t)
	claude, agents := coverageTiersSkillPaths(p)

	// A directory where the instruction file must be: os.ReadFile cannot read
	// it, so the reference cannot be bounded and nothing may be guessed at.
	if err := os.MkdirAll(filepath.Join(p.Root, agentsInstructions), 0o755); err != nil {
		t.Fatal(err)
	}

	if (coverageTiersSkillStep{}).Satisfied(p) {
		t.Fatal("Satisfied() = true with an instruction file this step cannot read")
	}
	why, delegated := (coverageTiersSkillStep{}).Delegated(p)
	if !delegated {
		t.Fatal("Delegated() = false, want the refusal instead of a rewrite")
	}
	for _, want := range []string{agentsInstructions, "could not be read", "dharness sync"} {
		if !strings.Contains(why, want) {
			t.Errorf("the refusal does not name %q:\n%s", want, why)
		}
	}

	if result := runCoverageTiersSkill(t, p); result.Status != report.Delegated {
		t.Fatalf("the run reported %s (error %q), want delegated", result.Status, result.Error)
	}
	for _, path := range []string{claude, agents} {
		if _, err := os.Stat(path); err == nil {
			t.Errorf("the step wrote %s while its reference was refused", rootRelative(p, path))
		}
	}
}

// TestCoverageTiersSkillStepHandlesAnEmptyInstructionFile pins the insertion
// boundary: an empty file gains the region and nothing else, rather than a
// leading blank line nobody wrote.
func TestCoverageTiersSkillStepHandlesAnEmptyInstructionFile(t *testing.T) {
	p := coverageTiersSkillFixture(t)
	writeGoldenFixtureFile(t, p.Root, agentsInstructions, "")

	if result := runCoverageTiersSkill(t, p); result.Status != report.Applied {
		t.Fatalf("the step was reported %s (error %q), want applied", result.Status, result.Error)
	}

	got := readFixtureFile(t, filepath.Join(p.Root, agentsInstructions))
	if !strings.HasPrefix(got, coverageTiersGuideBegin) {
		t.Errorf("the region did not land at the start of the empty file:\n%q", got)
	}
	if strings.HasPrefix(got, "\n") {
		t.Errorf("the empty file gained a blank line before the region:\n%q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("the region did not end the file with a newline:\n%q", got)
	}
	if n := coverageTiersSkillRegionCount(t, filepath.Join(p.Root, agentsInstructions), coverageTiersGuideEnd); n != 1 {
		t.Errorf("the empty file carries %d regions, want 1", n)
	}
}

// TestCoverageTiersSkillStepKeepsTheInstructionFilesLineEnding pins the CRLF
// round trip: the region is rendered with the file's own line ending, so a
// Windows checkout does not gain a mixed-ending block, and the next run still
// reads its own bytes back.
func TestCoverageTiersSkillStepKeepsTheInstructionFilesLineEnding(t *testing.T) {
	p := coverageTiersSkillFixture(t)
	const house = "# House rules\r\n\r\nBe kind to the reviewer.\r\n"
	writeGoldenFixtureFile(t, p.Root, agentsInstructions, house)

	if result := runCoverageTiersSkill(t, p); result.Status != report.Applied {
		t.Fatalf("the step was reported %s (error %q), want applied", result.Status, result.Error)
	}

	got := readFixtureFile(t, filepath.Join(p.Root, agentsInstructions))
	if !strings.HasPrefix(got, house) {
		t.Errorf("the project's own CRLF bytes were not preserved:\n%q", got)
	}
	at := strings.Index(got, coverageTiersGuideBegin)
	if at == -1 {
		t.Fatalf("the CRLF file never gained the managed region:\n%q", got)
	}
	tail := got[at:]
	for i := 0; i < len(tail); i++ {
		if tail[i] == '\n' && (i == 0 || tail[i-1] != '\r') {
			t.Errorf("the appended region carries a bare LF at offset %d:\n%q", i, tail)
			break
		}
	}

	if result := runCoverageTiersSkill(t, p); result.Status != report.Satisfied {
		t.Fatalf("the converged CRLF file reported %s (wrote %v), want satisfied", result.Status, result.Wrote)
	}
}

// TestCoverageTiersSkillStepReportsAManifestRemovedUnderIt is the same
// left-behind rule the coverage step already applies: the managed copies are a
// reason to ask, not a state to silently keep or silently delete.
func TestCoverageTiersSkillStepReportsAManifestRemovedUnderIt(t *testing.T) {
	p := coverageTiersSkillFixture(t)
	claude, agents := coverageTiersSkillPaths(p)
	writeGoldenFixtureFile(t, p.Root, agentsInstructions, "# House rules\n")

	if result := runCoverageTiersSkill(t, p); result.Status != report.Applied {
		t.Fatalf("the first run was reported %s (error %q), want applied", result.Status, result.Error)
	}
	if err := os.Remove(filepath.Join(p.Root, tiers.ManifestName)); err != nil {
		t.Fatal(err)
	}

	why, delegated := (coverageTiersSkillStep{}).Delegated(p)
	if !delegated {
		t.Fatal("Delegated() = false with the managed copies still on disk and no declaration")
	}
	for _, want := range []string{tiers.ManifestName, rootRelative(p, claude), rootRelative(p, agents), agentsInstructions} {
		if !strings.Contains(why, want) {
			t.Errorf("the refusal does not name %q:\n%s", want, why)
		}
	}

	result := runCoverageTiersSkill(t, p)
	if result.Status != report.Delegated {
		t.Fatalf("the run reported %s (error %q), want delegated", result.Status, result.Error)
	}
	if got := readFixtureFile(t, claude); got != string(coverageTiersSkill()) {
		t.Error("the refused run rewrote a copy it had already written")
	}

	// An instruction file the walk cannot read is skipped, and the walk goes
	// on: a leftover in a later file is still the project's to find, and
	// stopping there would report no instruction-file leftover at all while
	// the run passes as if the guidance were gone.
	t.Run("an unreadable earlier instruction file does not hide a later one", func(t *testing.T) {
		p := coverageTiersSkillFixture(t)
		writeGoldenFixtureFile(t, p.Root, claudeInstructions, "# Claude\n")

		if result := runCoverageTiersSkill(t, p); result.Status != report.Applied {
			t.Fatalf("the first run was reported %s (error %q), want applied", result.Status, result.Error)
		}
		if err := os.Remove(filepath.Join(p.Root, tiers.ManifestName)); err != nil {
			t.Fatal(err)
		}
		// A directory is this suite's deterministic cross-platform stand-in
		// for a file that cannot be read, and AGENTS.md is the first file the
		// walk reaches; CLAUDE.md still carries the region this step wrote.
		if err := os.MkdirAll(filepath.Join(p.Root, agentsInstructions), 0o755); err != nil {
			t.Fatal(err)
		}

		why, delegated := (coverageTiersSkillStep{}).Delegated(p)
		if !delegated {
			t.Fatal("Delegated() = false with a managed region still on disk and no declaration")
		}
		for _, want := range []string{tiers.ManifestName, claudeInstructions} {
			if !strings.Contains(why, want) {
				t.Errorf("the refusal does not name %q:\n%s", want, why)
			}
		}
		if strings.Contains(why, agentsInstructions) {
			t.Errorf("the refusal names %s, which this step cannot even read:\n%s", agentsInstructions, why)
		}
	})
}
