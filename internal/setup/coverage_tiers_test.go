package setup

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/report"
	"github.com/Disble/dharness/internal/runner"
	"github.com/Disble/dharness/internal/tiers"
	"github.com/Disble/dharness/internal/tool"
)

// coverageGateFixture is the smallest repository that has opted in: a JS
// project with the project's own Vitest config, the runner installed the way
// the package manager installs it, and the consumer-owned declaration. Those
// three are what the step reads before it writes anything, so a fixture
// missing one of them exercises a different row of the refusal matrix than the
// opted-in path does.
type coverageGateFixture struct {
	root   string
	source string
	config string
}

// coverageGateAdopted builds that fixture in either layout. The adopted
// prefix follows the JS project, which is where a split-layout consumer's
// source actually lives and what the runtime module projects against.
func coverageGateAdopted(t *testing.T, split bool) coverageGateFixture {
	t.Helper()

	root := t.TempDir()
	source := root
	prefix := "src/"
	if split {
		source = filepath.Join(root, "frontend")
		if err := os.MkdirAll(source, 0o755); err != nil {
			t.Fatal(err)
		}
		prefix = "frontend/src/"
	}

	writeGoldenFixtureFile(t, source, "package.json",
		"{\n  \"name\": \"app\",\n  \"packageManager\": \"npm@10.0.0\",\n  \"devDependencies\": {\n    \"vitest\": \"^5.0.0\"\n  }\n}\n")
	writeGoldenFixtureFile(t, source, "package-lock.json", "{\n  \"lockfileVersion\": 3\n}\n")
	writeLocalVitestBinary(t, source)
	writeGoldenFixtureFile(t, source, "vitest.config.ts",
		"import { defineConfig } from \"vitest/config\";\n\nexport default defineConfig({\n  test: {},\n});\n")
	coverageGateFile(t, root, prefix+"checkout/total.ts", "export const total = 1;\n")
	writeGoldenFixtureFile(t, root, tiers.ManifestName,
		"{\n  \"version\": 1,\n  \"enforcedPrefixes\": [\""+prefix+"\"],\n  \"features\": [\n"+
			"    {\"id\": \"checkout\", \"path\": \""+prefix+"checkout/\", \"defaultTier\": \"core\", \"reason\": \"money\"}\n"+
			"  ]\n}\n")

	return coverageGateFixture{root: root, source: source, config: filepath.Join(source, "vitest.config.ts")}
}

// writeLocalVitestBinary creates the file p.LocalBinary(tool.Vitest) answers
// with, in the shape the package manager writes on this platform. Its contents
// never run: this step starts no process at all.
func writeLocalVitestBinary(t *testing.T, source string) {
	t.Helper()
	name := tool.Vitest
	if runtime.GOOS == "windows" {
		name = tool.Vitest + ".cmd"
	}
	writeFixtureBinary(t, source, name)
}

func writeFixtureBinary(t *testing.T, source, name string) {
	t.Helper()
	path := filepath.Join(source, "node_modules", ".bin", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

// stubCoverageProcesses seames runner.Run for a scenario that drives Apply or
// Run: every other step of the plan is still in that plan, and a bare
// t.TempDir() has no npm, lefthook or husky to run.
func stubCoverageProcesses(t *testing.T) *[]runner.Command {
	t.Helper()
	var commands []runner.Command
	t.Cleanup(runner.SetForTest(func(cmd runner.Command, _, _ io.Writer) error {
		commands = append(commands, cmd)
		return nil
	}))
	return &commands
}

// coverageTiersStepResult finds this step's own entry in a run's report, so a
// scenario asserts on the status and reason a reader is shown rather than on a
// boolean only the test can see.
func coverageTiersStepResult(t *testing.T, p project.Project) report.StepResult {
	t.Helper()
	steps, _, err := Run(p)
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}
	for _, step := range steps {
		if step.ID == (coverageTiersStep{}).ID() {
			return step
		}
	}
	t.Fatalf("%s is not in the report at all", (coverageTiersStep{}).ID())
	return report.StepResult{}
}

// coverageGateFile writes a fixture file at a nested path, creating every
// directory it needs: the adopted source lives under a declared prefix, so
// most of this fixture's paths nest.
func coverageGateFile(t *testing.T, dir, name, contents string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFixtureFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

// TestCoverageTiersStepWiresTheAdoptedRunInOneApply is the owning scenario for
// the opt-in path: one sync leaves the managed runtime, the bridge entry, the
// splice in the project's own config and the coverage line in the gate, with
// the project's own bytes preserved.
func TestCoverageTiersStepWiresTheAdoptedRunInOneApply(t *testing.T) {
	for _, tc := range []struct {
		name  string
		split bool
		entry string
	}{
		{name: "repository is its own JS project", entry: ".dharness/vitest.coverage.mjs"},
		{name: "split layout names the owned entry from the JS project", split: true, entry: "../.dharness/vitest.coverage.mjs"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubCoverageProcesses(t)
			fixture := coverageGateAdopted(t, tc.split)
			p := project.At(fixture.root, fixture.source)

			before := readFixtureFile(t, fixture.config)
			if err := Apply(p, io.Discard); err != nil {
				t.Fatalf("Apply() = %v", err)
			}

			runtimePath := filepath.Join(p.Root, project.Dir, coverageTiersRuntimeName)
			if got, want := readFixtureFile(t, runtimePath), string(coverageTiersModule()); got != want {
				t.Error("the managed runtime is not the embedded module byte for byte")
			}

			bridge := readFixtureFile(t, filepath.Join(p.Root, project.Dir, coverageBridgeName))
			if !strings.Contains(bridge, "export default") {
				t.Errorf("the bridge does not export the project's own config:\n%s", bridge)
			}
			for _, marker := range []string{coverageTiersImportBegin, coverageTiersExportBegin, coverageTiersInputBegin} {
				if !strings.Contains(bridge, marker) {
					t.Errorf("the bridge does not refuse an adoption missing %q:\n%s", marker, bridge)
				}
			}
			if !strings.Contains(bridge, filepath.Base(fixture.config)) {
				t.Errorf("the bridge does not reach the owned runtime.\nbridge:\n%s", bridge)
			}

			spliced := readFixtureFile(t, fixture.config)
			if !strings.Contains(spliced, coverageTiersBinding+"(") {
				t.Errorf("the project's config does not call the managed wrapper:\n%s", spliced)
			}
			if !strings.Contains(spliced, `import { defineConfig } from "vitest/config";`) {
				t.Errorf("the splice dropped the project's own bytes:\n%s", spliced)
			}
			candidate, err := spliceCoverageTiersConfig([]byte(before), fixture.config, p)
			if err != nil {
				t.Fatalf("spliceCoverageTiersConfig() = %v", err)
			}
			if spliced != string(candidate) {
				t.Errorf("the config on disk is not the adapter's own candidate:\n%s", spliced)
			}

			// The literal line, not only the renderer's own answer: this is where
			// the split-root projection, the manager's local-only form and
			// lefthook's per-command root key are pinned together.
			gate := readFixtureFile(t, filepath.Join(p.Root, project.Dir, ownedLefthook))
			line := "npx --no-install vitest run --coverage --config " + tc.entry
			if !strings.Contains(gate, "run: "+line) {
				t.Errorf("the gate does not run the coverage line %q:\n%s", line, gate)
			}
			if tc.split && !strings.Contains(gate, "root: frontend/\n") {
				t.Errorf("the gate does not run both commands in the JS project:\n%s", gate)
			}
		})
	}
}

// TestCoverageTiersStepLeavesARepositoryThatNeverDeclaredItAlone is the prior
// behaviour this step must not change: no declaration, no managed file, no
// changed byte in the project's own config, and no process started to find any
// of that out.
func TestCoverageTiersStepLeavesARepositoryThatNeverDeclaredItAlone(t *testing.T) {
	commands := stubCoverageProcesses(t)

	root := t.TempDir()
	writeGoldenFixtureFile(t, root, "package.json", "{\n  \"name\": \"app\",\n  \"devDependencies\": {\n    \"vitest\": \"^5.0.0\"\n  }\n}\n")
	writeGoldenFixtureFile(t, root, "package-lock.json", "{\n  \"lockfileVersion\": 3\n}\n")
	writeLocalVitestBinary(t, root)
	const config = "export default { test: {} };\n"
	writeGoldenFixtureFile(t, root, "vitest.config.js", config)
	p := project.At(root, root)

	if !(coverageTiersStep{}).Satisfied(p) {
		t.Error("Satisfied() = false for a repository that never declared the policy")
	}
	if result := coverageTiersStepResult(t, p); result.Status != report.Satisfied {
		t.Fatalf("the report filed the step as %s, want satisfied", result.Status)
	}

	if err := Apply(p, io.Discard); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	for _, name := range []string{coverageTiersRuntimeName, coverageBridgeName} {
		if _, err := os.Stat(filepath.Join(p.Root, project.Dir, name)); err == nil {
			t.Errorf("a repository that never declared tiers gained %s", name)
		}
	}
	if got := readFixtureFile(t, filepath.Join(p.Root, "vitest.config.js")); got != config {
		t.Errorf("the project's own config changed:\n%s", got)
	}
	gate := readFixtureFile(t, filepath.Join(p.Root, project.Dir, ownedLefthook))
	if gate != gateConfig(p) {
		t.Errorf("the gate is not what the repository state renders:\n%q\nwant\n%q", gate, gateConfig(p))
	}
	if strings.Contains(gate, tool.Vitest) {
		t.Errorf("the gate runs a coverage line for a repository that declared none:\n%s", gate)
	}
	for _, cmd := range *commands {
		if cmd.Label == tool.Vitest {
			t.Errorf("the step started %+v, and it never starts a process at all", cmd)
		}
	}
}

// TestCoverageTiersStepConvergesOnASecondRun pins idempotence the way every
// other step in this package is pinned: the canonical bytes are read back, and
// the next run neither rewrites them nor repeats the splice.
func TestCoverageTiersStepConvergesOnASecondRun(t *testing.T) {
	stubCoverageProcesses(t)
	fixture := coverageGateAdopted(t, true)
	p := project.At(fixture.root, fixture.source)

	if err := Apply(p, io.Discard); err != nil {
		t.Fatalf("first Apply() = %v", err)
	}

	paths := []string{
		fixture.config,
		filepath.Join(p.Root, project.Dir, coverageTiersRuntimeName),
		filepath.Join(p.Root, project.Dir, coverageBridgeName),
		filepath.Join(p.Root, project.Dir, ownedLefthook),
	}
	first := map[string]string{}
	for _, path := range paths {
		first[path] = readFixtureFile(t, path)
	}

	if result := coverageTiersStepResult(t, p); result.Status != report.Satisfied {
		t.Fatalf("the converged step reported %s (wrote %v, error %q), want satisfied", result.Status, result.Wrote, result.Error)
	}
	if err := Apply(p, io.Discard); err != nil {
		t.Fatalf("second Apply() = %v", err)
	}
	for _, path := range paths {
		if got := readFixtureFile(t, path); got != first[path] {
			t.Errorf("the second run changed %s", path)
		}
	}
	if got := strings.Count(readFixtureFile(t, fixture.config), coverageTiersBinding+"("); got != 1 {
		t.Errorf("the wrapper call appears %d times, want 1", got)
	}
}

// TestCoverageTiersStepRepairsEveryByteItOwns is the drift half of the same
// contract: a hand-deleted module, a hand-edited bridge and a gate that lost
// its coverage line are all restored from the repository state, while the
// project's own wiring is left where it is.
func TestCoverageTiersStepRepairsEveryByteItOwns(t *testing.T) {
	stubCoverageProcesses(t)
	fixture := coverageGateAdopted(t, false)
	p := project.At(fixture.root, fixture.source)

	if err := Apply(p, io.Discard); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	spliced := readFixtureFile(t, fixture.config)

	runtimePath := filepath.Join(p.Root, project.Dir, coverageTiersRuntimeName)
	bridgePath := filepath.Join(p.Root, project.Dir, coverageBridgeName)
	gatePath := filepath.Join(p.Root, project.Dir, ownedLefthook)

	if err := os.Remove(runtimePath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bridgePath, []byte("// hand written\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	coverageGateFile(t, p.Root, project.Dir+"/"+ownedLefthook, "pre-commit:\n  commands:\n    dharness:\n      run: dharness check\n")

	if err := Apply(p, io.Discard); err != nil {
		t.Fatalf("repairing Apply() = %v", err)
	}

	if got := readFixtureFile(t, runtimePath); got != string(coverageTiersModule()) {
		t.Error("the deleted managed runtime was not restored byte for byte")
	}
	bridge, err := coverageBridgeModule(fixture.config, p)
	if err != nil {
		t.Fatalf("coverageBridgeModule() = %v", err)
	}
	if got := readFixtureFile(t, bridgePath); got != string(bridge) {
		t.Errorf("the edited bridge was not restored to its canonical bytes:\n%s", got)
	}
	if got := readFixtureFile(t, gatePath); got != gateConfig(p) {
		t.Errorf("the gate was not restored to what the repository state renders:\n%s", got)
	}
	if got := readFixtureFile(t, fixture.config); got != spliced {
		t.Error("the repair rewrote the project's own wired config")
	}

	// The shape the read-back is for: the files this step already wrote are
	// still canonical, and a later one is gone. An interrupted sync has to be
	// repaired by the next run, not stopped at the first file that already
	// matches.
	if err := os.Remove(bridgePath); err != nil {
		t.Fatal(err)
	}
	if err := Apply(p, io.Discard); err != nil {
		t.Fatalf("repairing an interrupted sync: Apply() = %v", err)
	}
	if got := readFixtureFile(t, bridgePath); got != string(bridge) {
		t.Errorf("the interrupted adoption was not repaired:\n%s", got)
	}
}

// TestCoverageTiersStepHandsARealConflictOverWithoutWriting is the refusal
// half: a declaration dharness cannot act on is reported with its reason, and
// nothing of the preparation is written — no module, no bridge, and not one
// byte of the project's own config.
//
// The gate is the other half of it, and it fails closed: an activated
// repository keeps a native coverage command pointing at the entry dharness
// owns, so the run that cannot be prepared fails loudly (the loader refuses a
// config with no managed regions) instead of passing green with coverage
// silently gone.
func TestCoverageTiersStepHandsARealConflictOverWithoutWriting(t *testing.T) {
	cases := []struct {
		name       string
		config     string
		manifest   string
		wantReason string
	}{
		{
			name:       "a config whose binding the managed import would shadow",
			config:     "import { withCoverageTiers } from \"./elsewhere.mjs\";\n\nexport default { test: {} };\n",
			wantReason: coverageTiersBinding,
		},
		{
			name:       "a declaration whose version dharness does not read",
			config:     "export default { test: {} };\n",
			manifest:   "{\n  \"version\": 2,\n  \"enforcedPrefixes\": [\"src/\"],\n  \"features\": [{\"id\": \"a\", \"path\": \"src/\", \"defaultTier\": \"core\", \"reason\": \"r\"}]\n}\n",
			wantReason: "version must be 1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubCoverageProcesses(t)
			fixture := coverageGateAdopted(t, false)
			p := project.At(fixture.root, fixture.source)
			writeGoldenFixtureFile(t, fixture.source, "vitest.config.ts", tc.config)
			if tc.manifest != "" {
				writeGoldenFixtureFile(t, p.Root, tiers.ManifestName, tc.manifest)
			}

			result := coverageTiersStepResult(t, p)
			if result.Status != report.Delegated {
				t.Fatalf("the step reported %s, want delegated", result.Status)
			}
			if !strings.Contains(result.Why, tc.wantReason) {
				t.Errorf("the delegated reason does not name %q:\n%s", tc.wantReason, result.Why)
			}

			if err := Apply(p, io.Discard); err != nil {
				t.Fatalf("Apply() = %v", err)
			}
			for _, name := range []string{coverageTiersRuntimeName, coverageBridgeName} {
				if _, err := os.Stat(filepath.Join(p.Root, project.Dir, name)); err == nil {
					t.Errorf("a refused declaration still produced %s", name)
				}
			}
			if got := readFixtureFile(t, fixture.config); got != tc.config {
				t.Errorf("the refused step rewrote the project's own config:\n%s", got)
			}
			gate := readFixtureFile(t, filepath.Join(p.Root, project.Dir, ownedLefthook))
			if !strings.Contains(gate, "--coverage") {
				t.Errorf("the gate dropped the coverage job for a project that declared it:\n%s", gate)
			}
			if !strings.Contains(gate, coverageBridgeName) {
				t.Errorf("the gate's coverage job does not point at the entry dharness owns:\n%s", gate)
			}
		})
	}
}

// TestCoverageTiersStepReportsAManifestRemovedUnderIt is the other real
// conflict: the manifest is what activates the integration, and `dharness
// policy` refuses to answer while the managed bytes survive without it, so
// this step reports that state instead of quietly writing over it.
//
// The gate keeps its coverage job: the bytes of the integration are still
// there, and a run whose resolver refuses has to fail rather than pass green.
func TestCoverageTiersStepReportsAManifestRemovedUnderIt(t *testing.T) {
	stubCoverageProcesses(t)
	fixture := coverageGateAdopted(t, false)
	p := project.At(fixture.root, fixture.source)

	if err := Apply(p, io.Discard); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	spliced := readFixtureFile(t, fixture.config)
	if err := os.Remove(filepath.Join(p.Root, tiers.ManifestName)); err != nil {
		t.Fatal(err)
	}

	result := coverageTiersStepResult(t, p)
	if result.Status != report.Delegated {
		t.Fatalf("the step reported %s with the manifest gone, want delegated", result.Status)
	}
	for _, named := range []string{tiers.ManifestName, coverageTiersRuntimeName} {
		if !strings.Contains(result.Why, named) {
			t.Errorf("the delegated reason does not name %q:\n%s", named, result.Why)
		}
	}

	if err := Apply(p, io.Discard); err != nil {
		t.Fatalf("second Apply() = %v", err)
	}
	gate := readFixtureFile(t, filepath.Join(p.Root, project.Dir, ownedLefthook))
	if !strings.Contains(gate, "--coverage") || !strings.Contains(gate, coverageBridgeName) {
		t.Errorf("the gate dropped the coverage job while its managed bytes survived:\n%s", gate)
	}
	if got := readFixtureFile(t, fixture.config); got != spliced {
		t.Error("the removed declaration rewrote the project's own wired config")
	}
}

// TestCoverageTiersStepFailsClosedWithoutAProjectRoot is the known boundary:
// an activated repository with no JS project root cannot name the entry the
// coverage job must run, and the gate gets a refusal that fails by name rather
// than an activation that quietly disappears.
func TestCoverageTiersStepFailsClosedWithoutAProjectRoot(t *testing.T) {
	stubCoverageProcesses(t)
	root := t.TempDir()
	writeGoldenFixtureFile(t, root, tiers.ManifestName, "{\n  \"version\": 1,\n  \"enforcedPrefixes\": [\"src/\"],\n  \"features\": [{\"id\": \"a\", \"path\": \"src/\", \"defaultTier\": \"core\", \"reason\": \"r\"}]\n}\n")
	coverageGateFile(t, root, "src/a.ts", "export const a = 1;\n")
	p := project.Project{Root: root}

	if why, delegated := (coverageTiersStep{}).Delegated(p); !delegated || !strings.Contains(why, "no JS project") {
		t.Errorf("Delegated() = %q, want the missing-JS-project reason", why)
	}

	gate := gateConfig(p)
	if !strings.Contains(gate, "coverage") || !strings.Contains(gate, "exit 1") {
		t.Fatalf("the gate has no coverage job at all for an activated repository:\n%s", gate)
	}

	// The refusal is a job, so it is driven: the shell that reads a hook has to
	// end non-zero, or the line would be decoration.
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("sh is not on PATH, so the refusal cannot be driven: %v", err)
	}
	var line string
	for _, candidate := range strings.Split(gate, "\n") {
		trimmed := strings.TrimSpace(candidate)
		if strings.HasPrefix(trimmed, "run: ") && strings.Contains(trimmed, "coverage") {
			line = strings.TrimPrefix(trimmed, "run: ")
		}
	}
	if line == "" {
		t.Fatalf("the gate carries no coverage line to drive:\n%s", gate)
	}
	command := exec.Command(sh, "-c", line)
	if err := command.Run(); err == nil {
		t.Errorf("the refusal line exited 0, so the commit would pass unmeasured:\n%s", line)
	}
}

// TestCoverageTiersStepConvergesTheOwnedGateEvenWhenPreparationRefuses is the
// ordering half of fail-closed: an adopted project whose gate file predates the
// activation — or whose preparation is refused this run — still ends the run
// with the coverage job in the file dharness owns, because the file converges on
// what the repository state renders rather than on whether this step could act.
func TestCoverageTiersStepConvergesTheOwnedGateEvenWhenPreparationRefuses(t *testing.T) {
	stubCoverageProcesses(t)
	fixture := coverageGateAdopted(t, false)
	p := project.At(fixture.root, fixture.source)

	// The bytes a previous version wrote, and a config this run cannot splice.
	coverageGateFile(t, p.Root, project.Dir+"/"+ownedLefthook, "pre-commit:\n  commands:\n    dharness:\n      run: dharness check\n")
	writeGoldenFixtureFile(t, fixture.source, "vitest.config.ts",
		"import { withCoverageTiers } from \"./elsewhere.mjs\";\n\nexport default { test: {} };\n")

	result := coverageTiersStepResult(t, p)
	if result.Status != report.Delegated {
		t.Fatalf("the step reported %s, want delegated for an unsplittable config", result.Status)
	}

	if err := Apply(p, io.Discard); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	gate := readFixtureFile(t, filepath.Join(p.Root, project.Dir, ownedLefthook))
	if gate != gateConfig(p) {
		t.Errorf("the owned gate did not converge on the repository state:\n%s", gate)
	}
	if !strings.Contains(gate, "--coverage") || !strings.Contains(gate, coverageBridgeName) {
		t.Errorf("the converged gate has no coverage job:\n%s", gate)
	}
}

// TestCoverageTiersGateConfigKeepsItsOldBytesWhenNothingIsActivated pins the
// promise the legacy path is judged by: a repository with no declaration and no
// managed footprint renders exactly the bytes the previous version wrote, in
// both layouts.
func TestCoverageTiersGateConfigKeepsItsOldBytesWhenNothingIsActivated(t *testing.T) {
	root := t.TempDir()

	if got, want := gateConfig(project.At(root, root)), "pre-commit:\n  commands:\n    dharness:\n      run: dharness check\n"; got != want {
		t.Errorf("gateConfig() = %q, want the legacy bytes %q", got, want)
	}

	source := filepath.Join(root, "frontend")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, want := gateConfig(project.At(root, source)), "pre-commit:\n  commands:\n    dharness:\n      run: dharness check\n      root: frontend/\n"; got != want {
		t.Errorf("gateConfig() = %q, want the legacy bytes %q", got, want)
	}
}

// TestCoverageTiersGateStaysActivatedByItsOwnFootprint is the activation rule
// itself: once dharness has written any part of the integration, deleting the
// declaration does not silently unwire the gate. The footprint alone — a managed
// module with no manifest and no markers anywhere — is enough.
func TestCoverageTiersGateStaysActivatedByItsOwnFootprint(t *testing.T) {
	stubCoverageProcesses(t)
	fixture := coverageGateAdopted(t, false)
	p := project.At(fixture.root, fixture.source)

	if err := Apply(p, io.Discard); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	if err := os.Remove(filepath.Join(p.Root, tiers.ManifestName)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(p.Root, project.Dir, coverageBridgeName)); err != nil {
		t.Fatal(err)
	}
	// Only the managed module is left: the bridge is gone above, and the
	// project's own config is handed back unwired, so there are no markers
	// anywhere either. One entry in the footprint is the whole state this rule
	// is about, and it is not a file the project reads on its own: recognised
	// as dharness's own is the whole point of the rule.
	writeGoldenFixtureFile(t, fixture.source, "vitest.config.ts",
		"import { defineConfig } from \"vitest/config\";\n\nexport default defineConfig({\n  test: {},\n});\n")
	wantFootprint := filepath.ToSlash(filepath.Join(project.Dir, coverageTiersRuntimeName))
	if got := coverageTiersFootprint(p); len(got) != 1 || got[0] != wantFootprint {
		t.Fatalf("the fixture's footprint is %q, want %q alone", got, wantFootprint)
	}
	line := coverageTiersCommandFor(p, "", tool.HostCoverageShell())
	if !strings.Contains(line, coverageBridgeName) || !strings.Contains(line, "--coverage") {
		t.Errorf("the gate line %q does not point at the managed entry", line)
	}
}

// TestCoverageTiersFootprintNamesEveryByteThisStepOwns is the ownership list
// read back rather than remembered: the two generated modules, the project's
// own config once it carries the managed regions, and — only where husky is the
// manager that answers — the line this step wrote into the hook.
//
// That list is the whole message a reader gets when the declaration is gone and
// the bytes are still there, so it is pinned entry for entry. Each shape asks a
// question the list answers differently: a husky repository whose hook carries
// the line, a lefthook repository still carrying husky's leftover, and a root
// the JS project shares no relative path with, where the config is still named
// by its own file name rather than by an empty string.
func TestCoverageTiersFootprintNamesEveryByteThisStepOwns(t *testing.T) {
	runtime := filepath.ToSlash(filepath.Join(project.Dir, coverageTiersRuntimeName))
	bridge := filepath.ToSlash(filepath.Join(project.Dir, coverageBridgeName))

	cases := []struct {
		name  string
		shape func(t *testing.T, fixture coverageGateFixture) project.Project
		want  []string
	}{
		{
			name: "husky: the marked hook is one of the managed bytes",
			shape: func(t *testing.T, fixture coverageGateFixture) project.Project {
				p := coverageTiersWiredByHand(t, fixture)
				coverageGateFile(t, p.Root, huskyHook, "#!/usr/bin/env sh\n"+gateCommand+" && old "+coverageHuskyMarker+"\n")
				return p
			},
			want: []string{runtime, bridge, "vitest.config.ts", huskyHook},
		},
		{
			name: "lefthook: husky's leftover is not this repository's gate",
			shape: func(t *testing.T, fixture coverageGateFixture) project.Project {
				p := coverageTiersWiredByHand(t, fixture)
				coverageGateFile(t, p.Root, "lefthook.yml", "pre-commit:\n  commands: {}\n")
				coverageGateFile(t, p.Root, huskyHook, "#!/usr/bin/env sh\nold "+coverageHuskyMarker+"\n")
				return p
			},
			want: []string{runtime, bridge, "vitest.config.ts"},
		},
		{
			name: "no relative path between the root and the config",
			shape: func(t *testing.T, fixture coverageGateFixture) project.Project {
				p := coverageTiersWiredByHand(t, fixture)
				// A root that cannot be related to an absolute config path — the
				// same answer filepath.Rel gives a checkout whose JS project sits
				// on another volume. The managed modules are under the real root,
				// so the config is the one entry this shape can carry.
				p.Root = filepath.Join("dharness", "unrelated-root")
				return p
			},
			want: []string{"vitest.config.ts"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := coverageGateAdopted(t, false)
			p := tc.shape(t, fixture)

			if got, want := strings.Join(coverageTiersFootprint(p), "\n"), strings.Join(tc.want, "\n"); got != want {
				t.Errorf("coverageTiersFootprint() =\n%s\nwant\n%s", got, want)
			}
		})
	}
}

// coverageTiersWiredByHand writes the bytes this step owns straight into the
// fixture, so a scenario about reading them back is about the tree the reader
// sees and not about the run that would have written it.
func coverageTiersWiredByHand(t *testing.T, fixture coverageGateFixture) project.Project {
	t.Helper()
	p := project.At(fixture.root, fixture.source)

	coverageGateFile(t, p.Root, filepath.ToSlash(filepath.Join(project.Dir, coverageTiersRuntimeName)), string(coverageTiersModule()))
	bridge, err := coverageBridgeModule(fixture.config, p)
	if err != nil {
		t.Fatalf("coverageBridgeModule() = %v", err)
	}
	coverageGateFile(t, p.Root, filepath.ToSlash(filepath.Join(project.Dir, coverageBridgeName)), string(bridge))
	candidate, err := spliceCoverageTiersConfig([]byte(readFixtureFile(t, fixture.config)), fixture.config, p)
	if err != nil {
		t.Fatalf("spliceCoverageTiersConfig() = %v", err)
	}
	if err := os.WriteFile(fixture.config, candidate, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestCoverageTiersStepFallsBackToTheViteConfigAndNamesTheRunnerItNeeds covers
// the three remaining rows of the wiring matrix: the config Vitest falls back to
// when no vitest.config exists, the refusal that names the install command when
// the runner itself is not there, and the refusal that names the config files to
// add when the project has neither. None of them may end in a written bridge: a
// gate line for a runner that is not installed is a red commit for everybody,
// and a bridge with no config behind it is the silent green this step exists to
// prevent.
func TestCoverageTiersStepFallsBackToTheViteConfigAndNamesTheRunnerItNeeds(t *testing.T) {
	t.Run("vite.config is the fallback vitest reads", func(t *testing.T) {
		stubCoverageProcesses(t)
		fixture := coverageGateAdopted(t, false)
		p := project.At(fixture.root, fixture.source)
		if err := os.Remove(fixture.config); err != nil {
			t.Fatal(err)
		}
		vite := filepath.Join(fixture.source, "vite.config.ts")
		writeGoldenFixtureFile(t, fixture.source, "vite.config.ts", "export default { test: {} };\n")

		if err := Apply(p, io.Discard); err != nil {
			t.Fatalf("Apply() = %v", err)
		}
		if got := readFixtureFile(t, vite); !strings.Contains(got, coverageTiersBinding+"(") {
			t.Errorf("the fallback config was not wired:\n%s", got)
		}
		gate := readFixtureFile(t, filepath.Join(p.Root, project.Dir, ownedLefthook))
		if !strings.Contains(gate, "run: npx --no-install vitest run --coverage --config .dharness/vitest.coverage.mjs") {
			t.Errorf("the gate does not run coverage against the bridge:\n%s", gate)
		}
	})

	t.Run("a missing runner is named, not installed", func(t *testing.T) {
		stubCoverageProcesses(t)
		fixture := coverageGateAdopted(t, false)
		p := project.At(fixture.root, fixture.source)
		if err := os.Remove(filepath.Join(fixture.source, "node_modules", ".bin", tool.Vitest+vitestShimSuffix())); err != nil {
			t.Fatal(err)
		}

		result := coverageTiersStepResult(t, p)
		if result.Status != report.Delegated {
			t.Fatalf("the step reported %s with no local runner, want delegated", result.Status)
		}
		if !strings.Contains(result.Why, "vitest @vitest/coverage-v8") {
			t.Errorf("the delegated reason does not name the install command:\n%s", result.Why)
		}
		for _, name := range []string{coverageTiersRuntimeName, coverageBridgeName} {
			if _, err := os.Stat(filepath.Join(p.Root, project.Dir, name)); err == nil {
				t.Errorf("a project without the runner still gained %s", name)
			}
		}
	})

	t.Run("a project with no config is told which one to add", func(t *testing.T) {
		stubCoverageProcesses(t)
		fixture := coverageGateAdopted(t, false)
		p := project.At(fixture.root, fixture.source)
		if err := os.Remove(fixture.config); err != nil {
			t.Fatal(err)
		}

		result := coverageTiersStepResult(t, p)
		if result.Status != report.Delegated {
			t.Fatalf("the step reported %s with no config to carry the tiers, want delegated", result.Status)
		}
		// The files the reader has to add, by the names Vitest resolves them
		// under: the first Vitest config name, and the Vite config it falls back
		// to.
		if !strings.Contains(result.Why, "Add vitest.config.js (or a vite.config.js Vitest reads)") {
			t.Errorf("the delegated reason does not name the config files to add:\n%s", result.Why)
		}
		for _, name := range []string{coverageTiersRuntimeName, coverageBridgeName} {
			if _, err := os.Stat(filepath.Join(p.Root, project.Dir, name)); err == nil {
				t.Errorf("a project with no config to carry the tiers still gained %s", name)
			}
		}
	})
}

// vitestShimSuffix is the extension the package manager gives the runner's shim
// on this platform, so a scenario can remove exactly the file LocalBinary
// answers with.
func vitestShimSuffix() string {
	if runtime.GOOS == "windows" {
		return ".cmd"
	}
	return ""
}

// TestCoverageTiersBridgeFailsLoadOnAnUnwiredConfig is the native half of the
// bridge's promise, and the reason the gate names a file dharness owns instead
// of the project's config: pointed at a config that never gained the managed
// regions, the run has to fail at load rather than measure nothing and exit 0.
//
// It drives the real Node the project's own runner runs on, in a disposable
// tree, and nothing else: the managed runtime is imported but never asked for
// coverage, so no resolver is spawned and no runner is installed.
func TestCoverageTiersBridgeFailsLoadOnAnUnwiredConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("drives Node in a disposable tree")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skipf("node is not on PATH, so the bridge cannot be loaded: %v", err)
	}

	root := t.TempDir()
	p := project.Project{Root: root, Source: root, PackageManager: "npm"}
	runtimePath := filepath.Join(root, project.Dir, coverageTiersRuntimeName)
	bridgePath := filepath.Join(root, project.Dir, coverageBridgeName)
	coverageGateFile(t, root, project.Dir+"/"+coverageTiersRuntimeName, string(coverageTiersModule()))

	const consumer = "export default { test: {} };\n"
	configPath := filepath.Join(root, "vitest.config.mjs")
	coverageGateFile(t, root, "vitest.config.mjs", consumer)

	bridge, err := coverageBridgeModule(configPath, p)
	if err != nil {
		t.Fatalf("coverageBridgeModule() = %v", err)
	}
	if err := os.WriteFile(bridgePath, bridge, 0o600); err != nil {
		t.Fatal(err)
	}

	// The consumer config here is the one the adapter writes around the
	// project's own value, so the bridge is loaded against a fully wired
	// project first — the state it must accept.
	candidate, err := spliceCoverageTiersConfig([]byte(consumer), configPath, p)
	if err != nil {
		t.Fatalf("spliceCoverageTiersConfig() = %v", err)
	}
	if err := os.WriteFile(configPath, candidate, 0o600); err != nil {
		t.Fatal(err)
	}

	coverageGateFile(t, root, "load.mjs", "const loaded = await import(\"./.dharness/vitest.coverage.mjs\");\nconst config = loaded.default;\nif (typeof config !== \"object\" || config === null || typeof config.test !== \"object\") {\n  throw new Error(\"the bridge did not re-export the project's own config: \" + JSON.stringify(config));\n}\n")

	wired := exec.Command(node, "load.mjs")
	wired.Dir = root
	if output, err := wired.CombinedOutput(); err != nil {
		t.Fatalf("the bridge refused a wired config: %v\n%s", err, output)
	}

	// The same project with the managed regions gone: the load must fail,
	// naming what is missing rather than running an unenforced configuration.
	coverageGateFile(t, root, "vitest.config.mjs", "export default { test: {} };\n")
	unwired := exec.Command(node, "load.mjs")
	unwired.Dir = root
	output, err := unwired.CombinedOutput()
	if err == nil {
		t.Fatalf("the bridge loaded an unwired config:\n%s", output)
	}
	for _, named := range []string{coverageBridgeName, "does not carry the managed regions", "dharness sync"} {
		if !strings.Contains(string(output), named) {
			t.Errorf("the refusal does not name %q:\n%s", named, output)
		}
	}

	// The runtime module is the one file the bridge names, and it is only a
	// file: nothing was spawned to load it.
	if _, err := os.Stat(runtimePath); err != nil {
		t.Fatalf("the fixture lost the managed runtime: %v", err)
	}
}

// TestCoverageTiersStepReadsNothingWithoutARoot pins the row a project with no
// directory reaches: joining a relative path onto an empty root would stat this
// process's own working directory, and a satisfied step that read somebody
// else's tree is worse than one that says nothing.
func TestCoverageTiersStepReadsNothingWithoutARoot(t *testing.T) {
	if !(coverageTiersStep{}).Satisfied(project.Project{}) {
		t.Error("Satisfied() = false for a project with no directory")
	}
	if why, delegated := (coverageTiersStep{}).Delegated(project.Project{}); delegated {
		t.Errorf("Delegated() = %q for a project with no directory", why)
	}
}

// TestCoverageTiersStepSharesTheGeneratedModulesWithGit is the tracked-tree half
// of adoption. The two generated modules are what the gate runs against, and the
// owned directory ignores everything it does not name — so an allow list that
// never gained them leaves both files out of every clone, and a checkout that
// syncs once carries a gate pointing at files that are not there.
//
// The verdict is git's own, asked of a real repository: `git status --porcelain`
// lists an untracked file that is not ignored, and says nothing at all about one
// that is. The second half is the repair a byte-for-byte comparison of the
// modules alone would miss: the generated bytes already match, and the
// allowance is gone.
func TestCoverageTiersStepSharesTheGeneratedModulesWithGit(t *testing.T) {
	if testing.Short() {
		t.Skip("initializes a real repository")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git is not on PATH, so the ignore rules cannot be observed: %v", err)
	}

	stubCoverageProcesses(t)
	fixture := coverageGateAdopted(t, false)
	p := project.At(fixture.root, fixture.source)

	if _, err := project.GitOutput(p.Root, "init", "-q"); err != nil {
		t.Fatalf("git init: %v", err)
	}
	// The ignore file a repository adopted before this change keeps: the
	// template dharness wrote, plus one line the project added for itself.
	if _, err := p.EnsureDir(""); err != nil {
		t.Fatal(err)
	}
	ignorePath := filepath.Join(p.Root, project.Dir, ownedIgnore)
	hand := "# a line this project added\n!its-own-file.json\n"
	if err := os.WriteFile(ignorePath, []byte(readFixtureFile(t, ignorePath)+hand), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Apply(p, io.Discard); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	for _, name := range []string{coverageTiersRuntimeName, coverageBridgeName} {
		rel := filepath.ToSlash(filepath.Join(project.Dir, name))
		status, err := project.GitOutput(p.Root, "status", "--porcelain", "--untracked-files=all", "--", rel)
		if err != nil {
			t.Fatalf("git status -- %s: %v", rel, err)
		}
		if !strings.HasPrefix(string(status), "?? ") {
			t.Errorf("git does not see %s as a file the repository can carry (status %q), so the next checkout never receives the entry the gate runs against", rel, status)
		}
	}

	ignore := readFixtureFile(t, ignorePath)
	for _, kept := range []string{"# dharness — this directory is written by dharness.\n", "*\n", hand} {
		if !strings.Contains(ignore, kept) {
			t.Errorf("the declared allowance did not keep %q:\n%s", kept, ignore)
		}
	}
	for _, added := range []string{"!" + coverageTiersRuntimeName, "!" + coverageBridgeName} {
		if !strings.Contains(ignore, added) {
			t.Errorf("the owned ignore list does not name %q:\n%s", added, ignore)
		}
	}

	// Hand-remove one allowance and leave the modules in place: the bytes this
	// step compares already match, so only a Satisfied that also reads the
	// allow list brings the entry back.
	without := strings.Replace(ignore, "!"+coverageTiersRuntimeName+"\n", "", 1)
	if without == ignore {
		t.Fatalf("the fixture never gained the runtime allowance:\n%s", ignore)
	}
	if err := os.WriteFile(ignorePath, []byte(without), 0o600); err != nil {
		t.Fatal(err)
	}
	if (coverageTiersStep{}).Satisfied(p) {
		t.Error("Satisfied() = true with the generated modules' allowance gone from the owned ignore list")
	}
	// One step, so the change list below is this step's own. The four files it
	// owns are already canonical here, so the only file it may report touching
	// is the ignore list it had to add the allowance back to: the canonical
	// bytes are read back, and a read-back file is never written again.
	results, _, err := run([]Step{coverageTiersStep{}}, p)
	if err != nil {
		t.Fatalf("repairing the allowance failed: %v", err)
	}
	if results[0].Status != report.Applied {
		t.Fatalf("the repair was reported %s (error %q), want applied", results[0].Status, results[0].Error)
	}
	wrote := results[0].Wrote
	wantWrote := filepath.ToSlash(filepath.Join(project.Dir, ownedIgnore))
	if len(wrote) != 1 || filepath.ToSlash(wrote[0].Path) != wantWrote || wrote[0].Kind != report.Modified {
		t.Errorf("the repair reported touching %v, want %q modified alone", wrote, wantWrote)
	}
	if got := readFixtureFile(t, ignorePath); !strings.Contains(got, "!"+coverageTiersRuntimeName) {
		t.Errorf("the repair did not bring the allowance back:\n%s", got)
	}
	if got := readFixtureFile(t, filepath.Join(p.Root, project.Dir, coverageTiersRuntimeName)); got != string(coverageTiersModule()) {
		t.Error("the repair rewrote the module it was already carrying")
	}
}

// TestCoverageTiersStepRollsBackEveryByteThisRunWrote pins the failure path:
// once a later write in the same step fails, the module, the bridge and the
// splice this run already wrote are put back, and the run's error names the
// step.
//
// The plan is the seam run() already documents for exactly this: one step, so
// the rollback under test is this step's own writes and not the eleven others'.
func TestCoverageTiersStepRollsBackEveryByteThisRunWrote(t *testing.T) {
	fixture := coverageGateAdopted(t, false)
	p := project.At(fixture.root, fixture.source)

	before := readFixtureFile(t, fixture.config)
	gatePath := filepath.Join(p.Root, project.Dir, ownedLefthook)
	// A directory where the gate file belongs: the write fails, and it fails
	// after the three files ahead of it in this step's own order.
	if err := os.MkdirAll(gatePath, 0o755); err != nil {
		t.Fatal(err)
	}

	_, _, err := run([]Step{coverageTiersStep{}}, p)
	if err == nil {
		t.Fatal("run() = nil although the step could not write the gate")
	}
	if !strings.Contains(err.Error(), (coverageTiersStep{}).ID()) {
		t.Errorf("the error does not name the step: %v", err)
	}

	for _, name := range []string{coverageTiersRuntimeName, coverageBridgeName} {
		if _, err := os.Stat(filepath.Join(p.Root, project.Dir, name)); err == nil {
			t.Errorf("the rolled-back run left %s behind", name)
		}
	}
	if got := readFixtureFile(t, fixture.config); got != before {
		t.Errorf("the rolled-back run left the project's config spliced:\n%s", got)
	}
}

// TestCoverageTiersStepChainsTheCoverageRunOntoAHuskyGate is the other half of
// the gate wiring: husky keeps a shell script with no `set -e`, so the coverage
// run is chained onto the check line with `&&` — the check first, the verdict
// still the check's — and the project's own lines survive.
func TestCoverageTiersStepChainsTheCoverageRunOntoAHuskyGate(t *testing.T) {
	for _, split := range []bool{false, true} {
		name := "repository is its own JS project"
		if split {
			name = "split layout changes directory in the hook"
		}
		t.Run(name, func(t *testing.T) {
			stubCoverageProcesses(t)
			fixture := coverageGateAdopted(t, split)
			p := project.At(fixture.root, fixture.source)
			if err := os.MkdirAll(filepath.Join(p.Root, ".husky"), 0o755); err != nil {
				t.Fatal(err)
			}
			writeGoldenFixtureFile(t, p.Root, filepath.ToSlash(huskyHook), "#!/usr/bin/env sh\nnpm test\ndharness check\n")

			if err := Apply(p, io.Discard); err != nil {
				t.Fatalf("Apply() = %v", err)
			}

			// husky runs its hook from the repository root and offers no
			// per-command root key, so the split layout is a directory change in
			// the line itself.
			line := "npx --no-install vitest run --coverage --config .dharness/vitest.coverage.mjs"
			if split {
				line = "cd frontend && npx --no-install vitest run --coverage --config ../.dharness/vitest.coverage.mjs"
			}
			chained := gateCommand + " && " + line + " " + coverageHuskyMarker

			hook := readFixtureFile(t, filepath.Join(p.Root, filepath.FromSlash(huskyHook)))
			for _, kept := range []string{"#!/usr/bin/env sh\n", "npm test\n", chained + "\n"} {
				if !strings.Contains(hook, kept) {
					t.Errorf("the hook does not carry %q:\n%s", kept, hook)
				}
			}
			if got := strings.Count(hook, gateCommand); got != 1 {
				t.Errorf("the check line appears %d times, want 1:\n%s", got, hook)
			}
			if got := strings.Count(hook, line); got != 1 {
				t.Errorf("the coverage line appears %d times, want 1:\n%s", got, hook)
			}

			if err := Apply(p, io.Discard); err != nil {
				t.Fatalf("second Apply() = %v", err)
			}
			if got := readFixtureFile(t, filepath.Join(p.Root, filepath.FromSlash(huskyHook))); got != hook {
				t.Errorf("the second run rewrote the converged hook:\n%s", got)
			}
		})
	}
}

// TestHuskyGateConvergesTheCoverageJobWhenPreparationRefuses is the
// first-refusal lifecycle, and husky's half of it: a hook that runs only the
// check, and a declaration this run cannot prepare, still has to end the run
// with the coverage job on the hook.
//
// hookInstallStep owns that hook, and the coverage step only ever rewrites a
// hook it could also prepare. Leaving the line to the coverage step's Apply
// means a refused declaration silently keeps a gate that runs the check alone
// — which exits 0 with nothing staged — so the convergence is pinned here, on
// the step whose postcondition the hook is.
func TestHuskyGateConvergesTheCoverageJobWhenPreparationRefuses(t *testing.T) {
	cases := []struct {
		name    string
		split   bool
		project func(fixture coverageGateFixture) project.Project
		refuse  func(t *testing.T, fixture coverageGateFixture)
		line    string
		refusal bool
	}{
		{
			name: "a config the splice refuses",
			refuse: func(t *testing.T, fixture coverageGateFixture) {
				writeGoldenFixtureFile(t, fixture.source, "vitest.config.ts",
					"import { withCoverageTiers } from \"./elsewhere.mjs\";\n\nexport default { test: {} };\n")
			},
			line: "npx --no-install vitest run --coverage --config .dharness/vitest.coverage.mjs",
		},
		{
			name:  "the runner the declaration needs is not installed",
			split: true,
			refuse: func(t *testing.T, fixture coverageGateFixture) {
				if err := os.Remove(filepath.Join(fixture.source, "node_modules", ".bin", tool.Vitest+vitestShimSuffix())); err != nil {
					t.Fatal(err)
				}
			},
			line: "cd frontend && npx --no-install vitest run --coverage --config ../.dharness/vitest.coverage.mjs",
		},
		{
			name: "no JS project to name the entry from",
			project: func(fixture coverageGateFixture) project.Project {
				return project.Project{Root: fixture.root}
			},
			refuse:  func(t *testing.T, fixture coverageGateFixture) {},
			refusal: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubCoverageProcesses(t)
			fixture := coverageGateAdopted(t, tc.split)
			p := project.At(fixture.root, fixture.source)
			if tc.project != nil {
				p = tc.project(fixture)
			}
			if err := os.MkdirAll(filepath.Join(p.Root, ".husky"), 0o755); err != nil {
				t.Fatal(err)
			}
			writeGoldenFixtureFile(t, p.Root, filepath.ToSlash(huskyHook), "#!/usr/bin/env sh\n"+gateCommand+"\n")
			tc.refuse(t, fixture)

			if err := Apply(p, io.Discard); err != nil {
				t.Fatalf("Apply() = %v", err)
			}

			hook := readFixtureFile(t, filepath.Join(p.Root, filepath.FromSlash(huskyHook)))
			if got := strings.Count(hook, gateCommand); got != 1 {
				t.Errorf("the check line appears %d times, want 1:\n%s", got, hook)
			}
			if tc.refusal {
				if !strings.Contains(hook, "exit 1") {
					t.Errorf("the hook dropped the fail-closed refusal:\n%s", hook)
				}
				return
			}
			chained := gateCommand + " && " + tc.line + " " + coverageHuskyMarker
			for _, kept := range []string{"#!/usr/bin/env sh\n", chained + "\n"} {
				if !strings.Contains(hook, kept) {
					t.Errorf("the hook does not carry %q:\n%s", kept, hook)
				}
			}
		})
	}
}

// TestCoverageTiersStepRepairsAStaleHuskyLine is the husky half of drift: the
// line this step owns is identified by its own marker, so a stale one — a
// project that changed package manager, or a moved JS root — is replaced rather
// than joined by a second one, and the check keeps running exactly once.
func TestCoverageTiersStepRepairsAStaleHuskyLine(t *testing.T) {
	stubCoverageProcesses(t)
	fixture := coverageGateAdopted(t, false)
	p := project.At(fixture.root, fixture.source)
	if err := os.MkdirAll(filepath.Join(p.Root, ".husky"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeGoldenFixtureFile(t, p.Root, filepath.ToSlash(huskyHook),
		"#!/usr/bin/env sh\n"+gateCommand+"\nbunx vitest --coverage --config .dharness/old.mjs "+coverageHuskyMarker+"\n")

	if err := Apply(p, io.Discard); err != nil {
		t.Fatalf("Apply() = %v", err)
	}

	hook := readFixtureFile(t, filepath.Join(p.Root, filepath.FromSlash(huskyHook)))
	if strings.Contains(hook, ".dharness/old.mjs") {
		t.Errorf("the stale line this step owns survived:\n%s", hook)
	}
	if got := strings.Count(hook, gateCommand); got != 1 {
		t.Errorf("the check line appears %d times after the repair, want 1:\n%s", got, hook)
	}
	line := "npx --no-install vitest run --coverage --config .dharness/vitest.coverage.mjs"
	if got := strings.Count(hook, line); got != 1 {
		t.Errorf("the coverage line appears %d times, want 1:\n%s", got, hook)
	}
}

// TestCoverageTiersHuskyHalfTouchesOnlyTheHookThatCarriesTheCheck is the husky
// half of this step's own plan, read where hookInstallStep cannot mask it: one
// step in the plan, so the hook on disk afterwards is this step's work and not
// the other writer's convergence of the same line.
//
// The promise has two directions, and both are the bytes of the hook. The hook
// that already carries a check line gets the coverage run chained onto that line
// — identified by its own marker, so a stale one is replaced in place rather
// than joined — and the line it owns is the first one carrying that marker: a
// second marked line the project keeps is not this step's to drop, and the check
// line it chains to is the first check line, not the last. A hook file the
// repository has not gained yet belongs to hookInstallStep, and this step never
// writes an empty one.
func TestCoverageTiersHuskyHalfTouchesOnlyTheHookThatCarriesTheCheck(t *testing.T) {
	const coverageLine = "npx --no-install vitest run --coverage --config .dharness/vitest.coverage.mjs"
	chained := gateCommand + " && " + coverageLine + " " + coverageHuskyMarker
	const staleOne = "bunx vitest --coverage --config .dharness/old.mjs " + coverageHuskyMarker
	const staleTwo = "bunx vitest --coverage --config .dharness/older.mjs " + coverageHuskyMarker

	cases := []struct {
		name string
		// hook is the hook file's bytes before the run, or "" for a repository
		// that has no hook file at all.
		hook string
		// want is the hook file's bytes after the run, or "" for a file that
		// must still not exist.
		want string
	}{
		{
			name: "the hook that carries the check gets the coverage run",
			hook: "#!/usr/bin/env sh\n" + gateCommand + "\n",
			want: "#!/usr/bin/env sh\n" + chained + "\n",
		},
		{
			name: "a hook with no check line is left alone",
			hook: "#!/usr/bin/env sh\nnpm test\n",
			want: "#!/usr/bin/env sh\nnpm test\n",
		},
		{
			name: "no hook file is not an empty hook file",
			hook: "",
			want: "",
		},
		{
			name: "the first line this step owns is the one it replaces",
			hook: "#!/usr/bin/env sh\n" + gateCommand + "\n" + staleOne + "\n" + staleTwo + "\n",
			want: "#!/usr/bin/env sh\n" + chained + "\n" + staleTwo + "\n",
		},
		{
			name: "the first check line is the one the coverage run is chained to",
			hook: "#!/usr/bin/env sh\n" + gateCommand + "\n" + gateCommand + "\n",
			want: "#!/usr/bin/env sh\n" + chained + "\n" + gateCommand + "\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubCoverageProcesses(t)
			fixture := coverageGateAdopted(t, false)
			p := project.At(fixture.root, fixture.source)
			if err := os.MkdirAll(filepath.Join(p.Root, ".husky"), 0o755); err != nil {
				t.Fatal(err)
			}
			hookPath := filepath.Join(p.Root, filepath.FromSlash(huskyHook))
			if tc.hook != "" {
				if err := os.WriteFile(hookPath, []byte(tc.hook), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			results, _, err := run([]Step{coverageTiersStep{}}, p)
			if err != nil {
				t.Fatalf("run() = %v", err)
			}
			if results[0].Status != report.Applied {
				t.Fatalf("the step was reported %s (error %q), want applied", results[0].Status, results[0].Error)
			}

			if tc.want == "" {
				if _, err := os.Stat(hookPath); err == nil {
					t.Errorf("the step wrote %s although there is no check line to chain; that file belongs to the step that installs the gate", huskyHook)
				}
				return
			}
			if got := readFixtureFile(t, hookPath); got != tc.want {
				t.Errorf("the hook is\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}
