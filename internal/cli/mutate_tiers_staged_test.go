package cli

// Tier-aware staged mutation scope: `mutate --staged` mutates the staged
// ranges the consumer's own coverage-tier declaration leaves eligible, and
// nothing else. These tests drive the public RunMutate seam over the real
// repository fixtures mutate_staged_test.go already builds, because the
// manifest is resolved from the snapshot — a real git index — and no stub can
// honestly answer for that.
//
// The behaviour owned here is narrow on purpose: filtering may only ever
// remove declared INFRASTRUCTURE from the staged scope. Unadopted legacy
// source, CORE and IMPORTANT source, the technical classifier's own drops,
// the MSP membership decision and the existing verdicts all keep the exact
// behavior they had.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/runner"
)

// coverageTierManifest is the consumer declaration these tests run against:
// src/features/checkout/ adopted as CORE, with its own type-declaration file
// at the tier the caller names — the consumer's decision, never dharness's.
func coverageTierManifest(typesTier string) string {
	return fmt.Sprintf(`{"version":1,"enforcedPrefixes":["src/features/checkout/"],"features":[{"id":"checkout","path":"src/features/checkout/","defaultTier":"core","reason":"Checkout decisions","overrides":[{"path":"src/features/checkout/types.ts","tier":"%s","reason":"Type declarations only"}]}]}`, typesTier)
}

// commitCoverageTierManifest writes the manifestation of the consumer's
// classification and commits it, so the index the snapshot materialises
// carries this declaration rather than whatever the working tree holds.
func commitCoverageTierManifest(t *testing.T, root, manifest string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "coverage-tiers.json"), manifest)
	gitRunStaged(t, root, "add", "coverage-tiers.json")
	gitRunStaged(t, root, "commit", "--quiet", "-m", "coverage tiers")
}

// stagedReportWithMutants is a Stryker JSON report naming each entry's file
// with no mutants at all; a caller that needs a mutant builds its own string.
func stagedReportWithEmptyMutants(paths ...string) string {
	entries := make([]string, 0, len(paths))
	for _, path := range paths {
		entries = append(entries, fmt.Sprintf(`%q:{"language":"typescript","mutants":[]}`, path))
	}
	return `{"schemaVersion":"1.0","files":{` + strings.Join(entries, ",") + `}}`
}

// mutateEntriesIn reads the mutate list out of a generated staged Stryker
// config captured while the run was executing.
func mutateEntriesIn(t *testing.T, generated string) []string {
	t.Helper()
	var config struct {
		Mutate []string `json:"mutate"`
	}
	if err := json.Unmarshal([]byte(generated), &config); err != nil {
		t.Fatalf("generated config %q is not JSON: %v", generated, err)
	}
	return config.Mutate
}

// TestMutateStagedTierExcludesInfrastructureFromTheScopeAndTheSelfcheck pins
// the whole filter in one run: a file the consumer declared INFRASTRUCTURE
// never reaches the classifier, never reaches Stryker's mutate list, and —
// the part that is easy to get wrong — never re-enters the report self-check
// as a dropped file either. The stubbed report deliberately carries a
// surviving mutant for the excluded file: if the excluded path had been kept
// as a classifier drop, that mutant would raise a classifier disagreement
// over a file the consumer exempted on purpose.
func TestMutateStagedTierExcludesInfrastructureFromTheScopeAndTheSelfcheck(t *testing.T) {
	root, captured := newStagedRepo(t)
	commitCoverageTierManifest(t, root, coverageTierManifest("infrastructure"))
	writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName("tsc")), "")
	stageFile(t, root, "src/features/checkout/total.ts", "export const total = 1;\n")
	stageFile(t, root, "src/features/checkout/types.ts", "export interface Total { value: number }\n")

	report := `{"schemaVersion":"1.0","files":{` +
		`"src/features/checkout/total.ts":{"language":"typescript","mutants":[` +
		`{"id":"1","mutatorName":"BooleanLiteral","status":"Killed","location":{"start":{"line":1,"column":1},"end":{"line":1,"column":2}}}]},` +
		`"src/features/checkout/types.ts":{"language":"typescript","mutants":[` +
		`{"id":"2","mutatorName":"StringLiteral","status":"Survived","location":{"start":{"line":1,"column":1},"end":{"line":1,"column":2}}}]}` +
		`}}`
	var generated string
	var tscArgs []string
	base := generatedConfigAtRun(captured, map[string]string{
		"src/features/checkout/total.ts": "exports.total = 1;",
		"src/features/checkout/types.ts": "",
	}, report, &generated)
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, stdout, stderr io.Writer) error {
		if cmd.Label == "tsc" {
			tscArgs = cmd.Args
		}
		return base(cmd, stdout, stderr)
	}))

	var out strings.Builder
	if err := RunMutate([]string{"--staged"}, &out); err != nil {
		t.Fatalf("RunMutate() = %v, want nil: the excluded infrastructure must not fail the run", err)
	}

	if got := mutateEntriesIn(t, generated); !slices.Equal(got, []string{"src/features/checkout/total.ts:1-1"}) {
		t.Errorf("mutate entries = %v, want exactly the eligible file", got)
	}
	if arg := findArgWithSuffix(tscArgs, filepath.FromSlash("src/features/checkout/types.ts")); arg != "" {
		t.Errorf("tsc received %s, want the excluded file kept out of the classifier", arg)
	}
	if strings.Contains(out.String(), "types-only") {
		t.Errorf("output = %q, want no classifier drop for a file that was never classified", out.String())
	}
	if !strings.Contains(out.String(), "1 file(s), 1 range(s), 1 in-scope mutant(s): 1 killed, 0 survived, 0 no coverage") {
		t.Errorf("output = %q, want the tally over the eligible scope only", out.String())
	}
	if strings.Contains(out.String(), "types.ts") {
		t.Errorf("output = %q, want the excluded path nowhere in the stream, including the self-check's dropped list", out.String())
	}
}

// TestMutateStagedTierReadsTheManifestItRunsAgainst pins where the
// declaration comes from. The snapshot is what is being committed, so its
// coverage-tiers.json decides the scope — an unstaged edit to the working
// tree's copy moves nothing, exactly like the Stryker config beside it. Both
// directions are exercised, so a run that read either copy would fail one of
// the two cases whatever the fixture order.
func TestMutateStagedTierReadsTheManifestItRunsAgainst(t *testing.T) {
	cases := []struct {
		name      string
		committed string
		working   string
		want      []string
	}{
		{
			name:      "the committed exclusion survives an unstaged attempt to lift it",
			committed: coverageTierManifest("infrastructure"),
			working:   coverageTierManifest("core"),
			want:      []string{"src/features/checkout/total.ts:1-1"},
		},
		{
			name:      "the committed adoption survives an unstaged attempt to drop it",
			committed: coverageTierManifest("core"),
			working:   coverageTierManifest("infrastructure"),
			want: []string{
				"src/features/checkout/total.ts:1-1",
				"src/features/checkout/types.ts:1-1",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, captured := newStagedRepo(t)
			commitCoverageTierManifest(t, root, tc.committed)
			// Never staged: the working tree's opinion, and only that.
			writeFile(t, filepath.Join(root, "coverage-tiers.json"), tc.working)
			writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName("tsc")), "")
			stageFile(t, root, "src/features/checkout/total.ts", "export const total = 1;\n")
			stageFile(t, root, "src/features/checkout/types.ts", "export interface Total { value: number }\n")

			var generated string
			t.Cleanup(runner.SetForTest(generatedConfigAtRun(captured, map[string]string{
				"src/features/checkout/total.ts": "exports.total = 1;",
				"src/features/checkout/types.ts": "exports.types = 1;",
			}, stagedReportWithEmptyMutants("src/features/checkout/total.ts", "src/features/checkout/types.ts"), &generated)))

			var out strings.Builder
			if err := RunMutate([]string{"--staged"}, &out); err != nil {
				t.Fatalf("RunMutate() = %v, want nil", err)
			}
			if got := mutateEntriesIn(t, generated); !slices.Equal(got, tc.want) {
				t.Errorf("mutate entries = %v, want %v: the committed declaration decides", got, tc.want)
			}
		})
	}
}

// newSplitStagedRepo builds the same real, isolated repository
// newStagedRepo does, except that the JS project lives in frontend/ — the
// layout where the repository root and the JS source are different
// directories, which is exactly what the scope projection has to get right.
func newSplitStagedRepo(t *testing.T) (root string) {
	t.Helper()
	isolateFromAmbientGitForMutateStaged(t)

	root = t.TempDir()
	gitRunStaged(t, root, "init", "--quiet", "--initial-branch=main", ".")
	writeFile(t, filepath.Join(root, "frontend", "package.json"), `{"devDependencies":{"vitest":"^4.0.0"}}`)
	writeFile(t, filepath.Join(root, "frontend", "package-lock.json"), `{"lockfileVersion":3}`)
	gitRunStaged(t, root, "add", "-A")
	gitRunStaged(t, root, "commit", "--quiet", "-m", "init")

	binDir := filepath.Join(root, "frontend", "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(binDir, binaryName("stryker")), "")

	previous := workingDirectory
	workingDirectory = func() (string, error) { return root, nil }
	t.Cleanup(func() { workingDirectory = previous })

	return root
}

// TestMutateStagedTierKeepsUnadoptedLegacySourceInScope pins the boundary of
// adoption: source outside every enforced prefix has no declaration at all,
// and no declaration means the legacy behavior, never an exemption. The
// staged change here touches one adopted file and one untouched-by-policy
// legacy file, and both stay in scope.
func TestMutateStagedTierKeepsUnadoptedLegacySourceInScope(t *testing.T) {
	root, captured := newStagedRepo(t)
	commitCoverageTierManifest(t, root, coverageTierManifest("infrastructure"))
	writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName("tsc")), "")
	stageFile(t, root, "src/features/checkout/total.ts", "export const total = 1;\n")
	stageFile(t, root, "src/legacy/old.ts", "export const legacy = 1;\n")

	var generated string
	t.Cleanup(runner.SetForTest(generatedConfigAtRun(captured, map[string]string{
		"src/features/checkout/total.ts": "exports.total = 1;",
		"src/legacy/old.ts":              "exports.legacy = 1;",
	}, stagedReportWithEmptyMutants("src/features/checkout/total.ts", "src/legacy/old.ts"), &generated)))

	if err := RunMutate([]string{"--staged"}, io.Discard); err != nil {
		t.Fatalf("RunMutate() = %v, want nil", err)
	}

	got := mutateEntriesIn(t, generated)
	slices.Sort(got)
	want := []string{"src/features/checkout/total.ts:1-1", "src/legacy/old.ts:1-1"}
	if !slices.Equal(got, want) {
		t.Errorf("mutate entries = %v, want %v: unadopted legacy source keeps its scope", got, want)
	}
}

// TestMutateStagedTierAllExcludedStartsNoTool pins the early exit: when every
// staged range belongs to declared infrastructure there is no runtime source
// left to mutate, so dharness says exactly that and starts no classifier, no
// MSP discovery, no related-test command and no Stryker — the cheapest run is
// the one that never happens (design-principles.md §13).
func TestMutateStagedTierAllExcludedStartsNoTool(t *testing.T) {
	root, captured := newStagedRepo(t)
	commitCoverageTierManifest(t, root, `{"version":1,"enforcedPrefixes":["src/vendor/"],"features":[{"id":"vendor","path":"src/vendor/","defaultTier":"infrastructure","reason":"Generated vendor bundle"}]}`)
	stageFile(t, root, "src/vendor/generated.ts", "export const generated = 1;\n")
	t.Cleanup(runner.SetForTest(captured.run))

	var out strings.Builder
	if err := RunMutate([]string{"--staged"}, &out); err != nil {
		t.Fatalf("RunMutate() = %v, want nil: nothing eligible to mutate is a no-op, not a failure", err)
	}
	if !strings.Contains(out.String(), "no runtime source to mutate after coverage-tier exclusions") {
		t.Errorf("output = %q, want the coverage-tier no-op named", out.String())
	}
	if len(captured.commands) != 0 {
		t.Errorf("commands = %+v, want none after every staged range was excluded", captured.commands)
	}
}

// TestMutateStagedTierKeepsAdoptedVerdicts pins what filtering must not touch:
// an eligible adopted file keeps its own verdict. A Survived mutant still
// fails the run and still carries its disable hint, and a NoCoverage mutant
// still carries its call-a-test note — the eligibility filter narrows who is
// asked, never what the answer means.
func TestMutateStagedTierKeepsAdoptedVerdicts(t *testing.T) {
	root, captured := newStagedRepo(t)
	commitCoverageTierManifest(t, root, coverageTierManifest("infrastructure"))
	stageFile(t, root, "src/features/checkout/total.ts", "export const a = 1;\nexport const b = 2;\n")
	report := `{"schemaVersion":"1.0","files":{"src/features/checkout/total.ts":{"language":"typescript","mutants":[` +
		`{"id":"1","mutatorName":"StringLiteral","status":"NoCoverage","location":{"start":{"line":1,"column":1},"end":{"line":1,"column":2}}},` +
		`{"id":"2","mutatorName":"BooleanLiteral","status":"Survived","location":{"start":{"line":2,"column":1},"end":{"line":2,"column":2}}}` +
		`]}}}`
	t.Cleanup(runner.SetForTest(stagedRunner(captured, nil, report)))

	var out strings.Builder
	err := RunMutate([]string{"--staged"}, &out)

	var survivors *SurvivorsError
	if !errors.As(err, &survivors) {
		t.Fatalf("RunMutate() = %v, want SurvivorsError: an eligible adopted file keeps its verdict", err)
	}
	if !strings.Contains(out.String(), "No test executes this line") {
		t.Errorf("output = %q, want the NoCoverage note preserved", out.String())
	}
	if !strings.Contains(out.String(), "Stryker disable BooleanLiteral") {
		t.Errorf("output = %q, want the survivor's disable hint preserved", out.String())
	}
}

// TestMutateStagedTierKeepsTheExistingJestRelatedFlow pins the other flow the
// same code path serves: with an active declaration, a project whose Stryker
// config selects Jest still takes the Jest related-test branch — list-only,
// no Vitest command anywhere — and still reaches Stryker over the eligible
// scope. S9 leaves Jest coverage support to the backlog; its existing
// mutation workflow is untouched.
func TestMutateStagedTierKeepsTheExistingJestRelatedFlow(t *testing.T) {
	root, captured := newStagedRepo(t)
	commitCoverageTierManifest(t, root, coverageTierManifest("infrastructure"))
	writeFile(t, filepath.Join(root, "stryker.config.json"), `{"testRunner":"jest"}`)
	gitRunStaged(t, root, "add", "stryker.config.json")
	gitRunStaged(t, root, "commit", "--quiet", "-m", "stryker config")
	writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName("tsc")), "")
	writeFile(t, filepath.Join(root, "node_modules", ".bin", binaryName("jest")), "")
	stageFile(t, root, "src/features/checkout/total.ts", "export const total = 1;\n")

	var generated string
	base := generatedConfigAtRun(captured, map[string]string{"src/features/checkout/total.ts": "exports.total = 1;"}, minimalStagedReport("src/features/checkout/total.ts"), &generated)
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, stdout, stderr io.Writer) error {
		if cmd.Label == "jest" {
			for _, arg := range []string{"--findRelatedTests", "--listTests", "--json"} {
				if !argsContain(cmd.Args, arg) {
					t.Errorf("jest args = %v, want the list-only form", cmd.Args)
				}
			}
			_, _ = io.WriteString(stdout, `["src/features/checkout/total.test.ts"]`)
		}
		return base(cmd, stdout, stderr)
	}))

	var out strings.Builder
	if err := RunMutate([]string{"--staged"}, &out); err != nil {
		t.Fatalf("RunMutate() = %v, want nil", err)
	}
	if !commandExists(captured, "jest") {
		t.Errorf("commands = %+v, want the Jest related command", captured.commands)
	}
	if commandExists(captured, "vitest") {
		t.Errorf("commands = %+v, want no Vitest command for a Jest project", captured.commands)
	}
	if got := mutateEntriesIn(t, generated); !slices.Equal(got, []string{"src/features/checkout/total.ts:1-1"}) {
		t.Errorf("mutate entries = %v, want the eligible scope", got)
	}
}

// TestMutateStagedTierRefusesAManifestItCannotResolve pins fail-closed: a
// declaration dharness cannot resolve refuses the run before any tool starts,
// rather than falling back to mutating everything. Silently ignoring a broken
// manifest is the one outcome that would make the consumer's classification
// meaningless without saying so.
func TestMutateStagedTierRefusesAManifestItCannotResolve(t *testing.T) {
	root, captured := newStagedRepo(t)
	writeFile(t, filepath.Join(root, "coverage-tiers.json"), "{")
	gitRunStaged(t, root, "add", "coverage-tiers.json")
	gitRunStaged(t, root, "commit", "--quiet", "-m", "broken coverage tiers")
	stageFile(t, root, "src/features/checkout/total.ts", "export const total = 1;\n")
	t.Cleanup(runner.SetForTest(captured.run))

	err := RunMutate([]string{"--staged"}, io.Discard)

	if err == nil || !strings.Contains(err.Error(), "coverage-tiers.json") {
		t.Fatalf("RunMutate() = %v, want an error naming the declaration", err)
	}
	if len(captured.commands) != 0 {
		t.Errorf("commands = %+v, want none: an unresolvable manifest refuses before anything runs", captured.commands)
	}
}

// TestMutateStagedTierResolvesTheManifestFromTheRepositoryRoot pins the
// geometry of the projection the root-as-source fixture cannot see. In a
// split layout the declaration sits at the repository root while the staged
// ranges are relative to the JS source, so the policy has to be asked about
// frontend/src/... and not about src/.... A run that asked the wrong question
// finds no manifest where it looks, resolves an inactive policy and mutates
// the exempted file anyway — the one direction this filter must never take.
func TestMutateStagedTierResolvesTheManifestFromTheRepositoryRoot(t *testing.T) {
	root := newSplitStagedRepo(t)
	writeFile(t, filepath.Join(root, "coverage-tiers.json"), `{"version":1,"enforcedPrefixes":["frontend/src/features/checkout/"],"features":[{"id":"checkout","path":"frontend/src/features/checkout/","defaultTier":"core","reason":"Checkout decisions","overrides":[{"path":"frontend/src/features/checkout/types.ts","tier":"infrastructure","reason":"Type declarations only"}]}]}`)
	gitRunStaged(t, root, "add", "coverage-tiers.json")
	gitRunStaged(t, root, "commit", "--quiet", "-m", "coverage tiers")
	writeFile(t, filepath.Join(root, "frontend", "node_modules", ".bin", binaryName("tsc")), "")
	stageFile(t, root, "frontend/src/features/checkout/total.ts", "export const total = 1;\n")
	stageFile(t, root, "frontend/src/features/checkout/types.ts", "export interface Total { value: number }\n")

	captured := &record{fail: map[string]error{}}
	var generated string
	t.Cleanup(runner.SetForTest(generatedConfigAtRun(captured, map[string]string{
		"src/features/checkout/total.ts": "exports.total = 1;",
		"src/features/checkout/types.ts": "",
	}, stagedReportWithEmptyMutants("src/features/checkout/total.ts"), &generated)))

	if err := RunMutate([]string{"--staged"}, io.Discard); err != nil {
		t.Fatalf("RunMutate() = %v, want nil", err)
	}

	if got := mutateEntriesIn(t, generated); !slices.Equal(got, []string{"src/features/checkout/total.ts:1-1"}) {
		t.Errorf("mutate entries = %v, want only the eligible file: the declaration lives at the repository root, the ranges under frontend/", got)
	}
}
