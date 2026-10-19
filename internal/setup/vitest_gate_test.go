package setup

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/project"
)

// writeVitestGateProject builds a repository whose coverage tiers are active
// (so the managed gate exists and a duplicate means something) with the given
// root files. The manifest's content is never read by the analysis — only its
// presence activates the gate — so the fixture stays a stat, not a policy.
func writeVitestGateProject(t *testing.T, files map[string]string) project.Project {
	t.Helper()
	root := t.TempDir()
	for name, contents := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := files["coverage-tiers.json"]; !ok {
		if err := os.WriteFile(filepath.Join(root, "coverage-tiers.json"), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return project.Project{Root: root, Source: root, PackageManager: "npm"}
}

// vitestGateHookFixture is the shape these scenarios share: one hook and a
// manifest declaring no scripts, so the analysis has nothing else to read.
func vitestGateHookFixture(t *testing.T, hook string) project.Project {
	t.Helper()
	return writeVitestGateProject(t, map[string]string{
		lefthookConfig: hook,
		"package.json": "{\"name\":\"app\"}\n",
	})
}

// TestLefthookExtendsAutoIntegratesAnUnrelatedHook is the S2 plain case: a
// hook carrying only unrelated checks is not a decision, so sync adds the
// safe additive reference instead of delegating merely because the file
// exists. The consumer's bytes survive verbatim below the reference, and a
// second run finds nothing left to do.
func TestLefthookExtendsAutoIntegratesAnUnrelatedHook(t *testing.T) {
	original := "pre-commit:\n  commands:\n    lint:\n      run: yarn eslint {staged_files}\n\npre-push:\n  commands:\n    audit:\n      run: bun audit\n"
	p := vitestGateHookFixture(t, original)

	if why, delegated := (lefthookExtendsStep{}).Delegated(p); delegated {
		t.Fatalf("Delegated() = %q, true for a hook with no Vitest in it", why)
	}

	raw := applyLefthookExtendsStep(t, p)
	if !strings.Contains(raw, "extends:") || !strings.Contains(raw, ownedFrom(p, p.Root, ownedLefthook)) {
		t.Errorf("Apply() did not point the config at the file dharness owns:\n%s", raw)
	}
	if !strings.Contains(raw, original) {
		t.Errorf("Apply() did not preserve the consumer's hook byte for byte:\n%s", raw)
	}

	if !(lefthookExtendsStep{}).Satisfied(p) {
		t.Error("Satisfied() = false right after Apply wrote the reference")
	}
	before := raw
	if _, err := (lefthookExtendsStep{}).Apply(p, &Writer{}, io.Discard); err != nil {
		t.Fatalf("second Apply() = %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(p.Root, lefthookConfig))
	if string(after) != before {
		t.Errorf("repeat sync rewrote the converged hook:\n%s", after)
	}
}

// TestLefthookExtendsRoutesFullVitestDuplicatesToTheAgent is the S3/S4
// recognized case: a reachable complete Vitest run executes the same suite
// the managed coverage job runs, so sync must not add a second gate. The
// reason names the concrete file and command, states the single complete
// Vitest testing gate including coverage tiers, and leaves the resolution —
// one gate, the managed one — to the consuming agent. Nothing is deleted or
// rewritten.
func TestLefthookExtendsRoutesFullVitestDuplicatesToTheAgent(t *testing.T) {
	cases := []struct {
		name    string
		hook    string
		scripts string
		command string
		through string
	}{
		{
			name:    "direct full run",
			hook:    "pre-commit:\n  commands:\n    test:\n      run: npx --no-install vitest run\n",
			command: "npx --no-install vitest run",
		},
		{
			name:    "second coverage invocation duplicates the same suite",
			hook:    "pre-commit:\n  commands:\n    test:\n      run: bunx --no-install vitest run --coverage\n",
			command: "bunx --no-install vitest run --coverage",
		},
		{
			name:    "reached through a local package script",
			hook:    "pre-commit:\n  commands:\n    test:\n      run: npm run test\n",
			scripts: "{\"test\": \"vitest run\"}",
			command: "npm run test",
			through: "test",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{lefthookConfig: tc.hook}
			if tc.scripts != "" {
				files["package.json"] = "{\"name\":\"app\",\"scripts\":" + tc.scripts + "}\n"
			} else {
				files["package.json"] = "{\"name\":\"app\"}\n"
			}
			before := tc.hook
			p := writeVitestGateProject(t, files)

			if (lefthookExtendsStep{}).Satisfied(p) {
				t.Fatal("Satisfied() = true while the managed reference is missing")
			}
			why, delegated := (lefthookExtendsStep{}).Delegated(p)
			if !delegated {
				t.Fatal("Delegated() = false for a reachable complete Vitest run")
			}
			got := analyzeVitestGate(p)
			if got.status != vitestGateEquivalent {
				t.Fatalf("analyzeVitestGate() status = %s, want a confirmed duplicate", gateStatusName(got.status))
			}
			if got.through != tc.through {
				t.Errorf("analyzeVitestGate() through = %q, want %q", got.through, tc.through)
			}
			for _, want := range []string{
				lefthookConfig, tc.command, "one", "Vitest", "coverage",
				// The duplicate is named as one, and the managed gate is named
				// by the entry the agent must keep.
				"a complete Vitest suite",
				"remove the duplicate full-suite invocation",
				filepath.ToSlash(filepath.Join(project.Dir, coverageBridgeName)),
			} {
				if !strings.Contains(why, want) {
					t.Errorf("reconciliation does not name %q:\n%s", want, why)
				}
			}
			if strings.Contains(why, "narrow it so the analysis") {
				t.Errorf("a confirmed duplicate is not handed the uncertainty advice:\n%s", why)
			}
			if tc.through != "" {
				if !strings.Contains(why, "via package script "+tc.through) {
					t.Errorf("reconciliation does not name the package-script chain %q:\n%s", tc.through, why)
				}
				if !strings.Contains(why, "the package script it reaches") {
					t.Errorf("reconciliation does not point the removal at the script it reaches:\n%s", why)
				}
			} else if strings.Contains(why, "via package script") || strings.Contains(why, "the package script it reaches") {
				t.Errorf("a direct invocation is reported as reaching a package script:\n%s", why)
			}

			if _, err := (lefthookExtendsStep{}).Apply(p, &Writer{}, io.Discard); err == nil {
				t.Error("Apply() = nil for a duplicate the agent has not resolved")
			}
			raw, _ := os.ReadFile(filepath.Join(p.Root, lefthookConfig))
			if string(raw) != before {
				t.Errorf("the refused run touched the consumer's hook:\n%s", raw)
			}
		})
	}
}

// TestLefthookExtendsRoutesOpaqueInvocationsToTheAgent is the S3 unknown
// case: a command that might run Vitest but cannot be resolved to a concrete
// invocation takes the same reconciliation route as a confirmed duplicate,
// while the diagnostics say the command is uncertain rather than claiming it
// is Vitest.
func TestLefthookExtendsRoutesOpaqueInvocationsToTheAgent(t *testing.T) {
	cases := []struct {
		name    string
		run     string
		scripts string
		command string
		reason  string
	}{
		{
			name:    "node wrapper script hides the runner",
			scripts: "{\"test\": \"node scripts/run-check.mjs\"}",
			command: "npm run test",
			reason:  "scripts/run-check.mjs",
		},
		{
			name:    "a launcher flag comes before the file",
			run:     "node --experimental-strip-types scripts/check.ts",
			command: "node --experimental-strip-types scripts/check.ts",
			reason:  "scripts/check.ts",
		},
		{name: "a tsx wrapper", run: "tsx scripts/check.ts", command: "tsx scripts/check.ts", reason: "scripts/check.ts"},
		{name: "a ts-node wrapper", run: "ts-node scripts/check.ts", command: "ts-node scripts/check.ts", reason: "scripts/check.ts"},
		{name: "a vite-node wrapper", run: "vite-node scripts/check.ts", command: "vite-node scripts/check.ts", reason: "scripts/check.ts"},
		{name: "a jiti wrapper", run: "jiti scripts/check.ts", command: "jiti scripts/check.ts", reason: "scripts/check.ts"},
		{name: "a launcher after a non-launcher token", run: "npx bun scripts/check.ts", command: "npx bun scripts/check.ts", reason: "scripts/check.ts"},
		{
			name:    "named script does not exist",
			scripts: "{\"lint\": \"eslint .\"}",
			command: "npm run test",
			reason:  "which package.json does not declare",
		},
		{
			name:    "script cycle never resolves",
			scripts: "{\"test\": \"npm run check\", \"check\": \"npm run test\"}",
			command: "npm run test",
			reason:  "refers back to itself",
		},
		{
			name:    "script chain deeper than the analysis follows",
			scripts: "{\"s1\": \"npm run s2\", \"s2\": \"npm run s3\", \"s3\": \"npm run s4\", \"s4\": \"npm run s5\", \"s5\": \"npm run s6\", \"s6\": \"npm run s7\", \"s7\": \"npm run s8\", \"s8\": \"npm run s9\", \"s9\": \"vitest run\"}",
			command: "npm run s1",
			reason:  "chains deeper than the analysis follows",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			run := tc.run
			if run == "" {
				run = tc.command
			}
			hook := "pre-commit:\n  commands:\n    test:\n      run: " + run + "\n"
			manifest := "{\"name\":\"app\"}\n"
			if tc.scripts != "" {
				manifest = "{\"name\":\"app\",\"scripts\":" + tc.scripts + "}\n"
			}
			p := writeVitestGateProject(t, map[string]string{lefthookConfig: hook, "package.json": manifest})

			if got := analyzeVitestGate(p); got.status != vitestGateUnresolved {
				t.Fatalf("analyzeVitestGate() status = %s, want uncertainty", gateStatusName(got.status))
			}
			why, delegated := (lefthookExtendsStep{}).Delegated(p)
			if !delegated {
				t.Fatal("Delegated() = false for an invocation that cannot be resolved")
			}
			for _, want := range []string{lefthookConfig, tc.command, "coverage", "uncertainty", tc.reason} {
				if !strings.Contains(why, want) {
					t.Errorf("reconciliation does not name %q:\n%s", want, why)
				}
			}
			for _, unwanted := range []string{"a complete Vitest suite", "remove the duplicate full-suite invocation"} {
				if strings.Contains(why, unwanted) {
					t.Errorf("uncertain command reported as %q:\n%s", unwanted, why)
				}
			}
			if !strings.Contains(why, "narrow it so the analysis") {
				t.Errorf("uncertain command does not carry the reconciliation advice:\n%s", why)
			}

			if _, err := (lefthookExtendsStep{}).Apply(p, &Writer{}, io.Discard); err == nil {
				t.Error("Apply() = nil for an invocation the agent has not clarified")
			}
			raw, _ := os.ReadFile(filepath.Join(p.Root, lefthookConfig))
			if string(raw) != hook {
				t.Errorf("the refused run touched the consumer's hook:\n%s", raw)
			}
		})
	}
}

// TestLefthookExtendsLeavesScopedRunsAlone pins what the analysis must not
// call equivalent: a focused file filter, a watch invocation, an alternate
// config and a suite in another root are not the managed gate's complete
// run, so they stay and the reference is still added around them.
func TestLefthookExtendsLeavesScopedRunsAlone(t *testing.T) {
	runs := []string{
		"npx --no-install vitest run src/login.test.ts",
		"npx --no-install vitest --watch",
		"npx --no-install vitest run --changed",
	}

	for _, run := range runs {
		t.Run(run, func(t *testing.T) {
			hook := "pre-commit:\n  commands:\n    test:\n      run: " + run + "\n"
			p := writeVitestGateProject(t, map[string]string{
				lefthookConfig: hook,
				"package.json": "{\"name\":\"app\"}\n",
			})

			if why, delegated := (lefthookExtendsStep{}).Delegated(p); delegated {
				t.Fatalf("Delegated() = %q, true for a scoped run that is not the managed gate", why)
			}
			if _, err := (lefthookExtendsStep{}).Apply(p, &Writer{}, io.Discard); err != nil {
				t.Fatalf("Apply() = %v", err)
			}
			raw, _ := os.ReadFile(filepath.Join(p.Root, lefthookConfig))
			if !strings.Contains(string(raw), run) {
				t.Errorf("Apply() dropped the consumer's scoped run:\n%s", raw)
			}
			if !(lefthookExtendsStep{}).Satisfied(p) {
				t.Error("Satisfied() = false right after Apply wrote the reference")
			}
		})
	}
}

// TestLefthookExtendsIgnoresWhatItDoesNotRead proves the two exclusions: a
// package script no hook reaches is not a conflict, and neither is the
// managed coverage invocation itself — otherwise every activated repository
// would report a duplicate with its own gate.
func TestLefthookExtendsIgnoresWhatItDoesNotRead(t *testing.T) {
	t.Run("unused script", func(t *testing.T) {
		p := writeVitestGateProject(t, map[string]string{
			lefthookConfig: "pre-commit:\n  commands:\n    lint:\n      run: npx eslint .\n",
			"package.json": "{\"name\":\"app\",\"scripts\":{\"test:coverage\":\"vitest run --coverage\"}}\n",
		})
		if why, delegated := (lefthookExtendsStep{}).Delegated(p); delegated {
			t.Fatalf("Delegated() = %q, true for a script no hook reaches", why)
		}
	})

	t.Run("managed coverage invocation", func(t *testing.T) {
		line := "bunx --no-install vitest run --coverage --config .dharness/vitest.coverage.mjs"
		p := writeVitestGateProject(t, map[string]string{
			lefthookConfig: "pre-commit:\n  commands:\n    coverage:\n      run: " + line + "\n",
			"package.json": "{\"name\":\"app\"}\n",
		})
		if why, delegated := (lefthookExtendsStep{}).Delegated(p); delegated {
			t.Fatalf("Delegated() = %q, true for the managed invocation itself", why)
		}
	})
}

// TestLefthookExtendsRefusesUnsupportedShapesWithoutRewriting is the damage
// guard: a config whose structure cannot take an additive reference — here an
// existing top-level extends the merge would duplicate — is precise
// reconciliation, never a rewritten file.
func TestLefthookExtendsRefusesUnsupportedShapesWithoutRewriting(t *testing.T) {
	// A top-level `extends` is the one key the additive prepend would
	// duplicate. A comment or a blank line before it is not a reason to miss
	// it: both are skipped, and the key is still the document's own.
	cases := []struct {
		name   string
		config string
	}{
		{"first key", "extends:\n  - ./shared.yml\n\npre-commit:\n  commands:\n    lint:\n      run: npx eslint .\n"},
		{"after a comment", "# hooks\n\nextends:\n  - ./shared.yml\n\npre-commit:\n  commands:\n    lint:\n      run: npx eslint .\n"},
		{"after a blank line", "\nextends:\n  - ./shared.yml\n\npre-commit:\n  commands:\n    lint:\n      run: npx eslint .\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := writeVitestGateProject(t, map[string]string{
				lefthookConfig: tc.config,
				"package.json": "{\"name\":\"app\"}\n",
			})

			why, delegated := (lefthookExtendsStep{}).Delegated(p)
			if !delegated {
				t.Fatal("Delegated() = false for a shape the additive merge cannot take")
			}
			for _, want := range []string{lefthookConfig, "extends"} {
				if !strings.Contains(why, want) {
					t.Errorf("reconciliation does not name %q:\n%s", want, why)
				}
			}

			if _, err := (lefthookExtendsStep{}).Apply(p, &Writer{}, io.Discard); err == nil {
				t.Error("Apply() = nil for a shape it cannot merge into")
			}
			raw, _ := os.ReadFile(filepath.Join(p.Root, lefthookConfig))
			if string(raw) != tc.config {
				t.Errorf("the refused run rewrote the consumer's config:\n%s", raw)
			}
		})
	}
}

// TestHuskyHookWithAFullVitestRunAsksTheAgentToReconcile covers the husky
// half: the hook is the project's own script, so a reachable complete run
// there is reconciled by the agent rather than chained over. A hook with no
// conflict still gains the gate line, which is the prior behavior this
// change keeps.
func TestHuskyHookWithAFullVitestRunAsksTheAgentToReconcile(t *testing.T) {
	t.Run("duplicate delegates", func(t *testing.T) {
		hook := "#!/usr/bin/env sh\nnpx --no-install vitest run\n"
		p := writeVitestGateProject(t, map[string]string{
			".husky/pre-commit": hook,
			"package.json":      "{\"name\":\"app\"}\n",
		})

		why, delegated := (hookInstallStep{}).Delegated(p)
		if !delegated {
			t.Fatal("Delegated() = false for a husky hook running the full suite")
		}
		for _, want := range []string{".husky/pre-commit", "vitest run", "one", "Vitest", "coverage"} {
			if !strings.Contains(why, want) {
				t.Errorf("reconciliation does not name %q:\n%s", want, why)
			}
		}
	})

	t.Run("no conflict still wires the gate", func(t *testing.T) {
		p := writeVitestGateProject(t, map[string]string{
			".husky/pre-commit": "#!/usr/bin/env sh\nnpm run lint\n",
			"package.json":      "{\"name\":\"app\",\"scripts\":{\"lint\":\"eslint .\"}}\n",
		})

		if why, delegated := (hookInstallStep{}).Delegated(p); delegated {
			t.Fatalf("Delegated() = %q, true for a hook with no Vitest in it", why)
		}
		if _, err := (hookInstallStep{}).Apply(p, &Writer{}, io.Discard); err != nil {
			t.Fatalf("Apply() = %v", err)
		}
		raw, _ := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(huskyHook)))
		if !strings.Contains(string(raw), gateCommand) {
			t.Errorf("Apply() did not wire the gate:\n%s", raw)
		}
		if !strings.Contains(string(raw), "npm run lint") {
			t.Errorf("Apply() dropped the consumer's own check:\n%s", raw)
		}
	})

	t.Run("a commented-out run is not the gate", func(t *testing.T) {
		hook := "#!/usr/bin/env sh\n# npx --no-install vitest run\nnpm run lint\n"
		p := writeVitestGateProject(t, map[string]string{
			".husky/pre-commit": hook,
			"package.json":      "{\"name\":\"app\",\"scripts\":{\"lint\":\"eslint .\"}}\n",
		})

		if got := analyzeVitestGate(p); got.status != vitestGateClear {
			t.Fatalf("analyzeVitestGate() status = %s, want clear: a commented-out invocation runs nothing", gateStatusName(got.status))
		}
		if why, delegated := (hookInstallStep{}).Delegated(p); delegated {
			t.Fatalf("Delegated() = %q, true for a comment that runs nothing", why)
		}
	})
}

// TestLefthookWiredDuplicateDelegatesInsteadOfAborting is the V1 lifecycle:
// a config already carrying the managed reference plus a reachable complete
// duplicate is a delegation that lets unrelated setup continue, never a
// failed Apply that rolls the run back. The same holds for a wired config
// carrying an opaque wrapper.
func TestLefthookWiredDuplicateDelegatesInsteadOfAborting(t *testing.T) {
	wired := "extends:\n  - .dharness/lefthook.yml\n"
	cases := []struct {
		name      string
		hook      string
		scripts   string
		uncertain bool
	}{
		{
			name: "wired duplicate",
			hook: wired + "\npre-commit:\n  commands:\n    test:\n      run: npx --no-install vitest run\n",
		},
		{
			name:      "wired opaque wrapper",
			hook:      wired + "\npre-commit:\n  commands:\n    test:\n      run: npm run check\n",
			scripts:   "{\"check\": \"node scripts/ci.mjs\"}",
			uncertain: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{lefthookConfig: tc.hook}
			if tc.scripts != "" {
				files["package.json"] = "{\"name\":\"app\",\"scripts\":" + tc.scripts + "}\n"
			} else {
				files["package.json"] = "{\"name\":\"app\"}\n"
			}
			p := writeVitestGateProject(t, files)

			if (lefthookExtendsStep{}).Satisfied(p) {
				t.Fatal("Satisfied() = true while a second complete run duplicates the managed gate")
			}
			why, delegated := (lefthookExtendsStep{}).Delegated(p)
			if !delegated {
				t.Fatal("Delegated() = false for a wired config carrying a duplicate")
			}
			for _, want := range []string{lefthookConfig, "coverage"} {
				if !strings.Contains(why, want) {
					t.Errorf("reconciliation does not name %q:\n%s", want, why)
				}
			}
			if tc.uncertain && !strings.Contains(why, "uncertain") {
				t.Errorf("uncertain command not reported as uncertainty:\n%s", why)
			}

			// Delegation lets the rest of the plan continue: no error, no
			// rollback, and the consumer's bytes are exactly as found.
			if _, err := applySteps([]Step{(lefthookExtendsStep{}), (hookInstallStep{})}, p, io.Discard); err != nil {
				t.Fatalf("applySteps() = %v, want nil: delegation continues the run instead of aborting it", err)
			}
			raw, _ := os.ReadFile(filepath.Join(p.Root, lefthookConfig))
			if string(raw) != tc.hook {
				t.Errorf("the delegated run changed the consumer's config:\n%s", raw)
			}
		})
	}
}

// TestHuskyDuplicateSurvivesRepeatSync is the V2 lifecycle: a husky hook
// carrying the gate line plus an appended complete run is not satisfied, on
// the fresh sync or the repeat, until the agent leaves one gate. Removing the
// duplicate converges the step.
func TestHuskyDuplicateSurvivesRepeatSync(t *testing.T) {
	// The V2 shape: the hook already carries the full managed gate line and
	// an appended complete run beside it.
	root := t.TempDir()
	writeVitestGateFile(t, root, "coverage-tiers.json", "{}\n")
	writeVitestGateFile(t, root, "package.json", "{\"name\":\"app\"}\n")
	p := project.Project{Root: root, Source: root, PackageManager: "npm"}
	hook := "#!/usr/bin/env sh\n" + huskyGateLine(p) + "\nnpx --no-install vitest run\n"
	writeVitestGateFile(t, root, filepath.ToSlash(huskyHook), hook)

	for run := 0; run < 2; run++ {
		if (hookInstallStep{}).Satisfied(p) {
			t.Fatalf("run %d: Satisfied() = true while the hook runs a second complete suite", run)
		}
		why, delegated := (hookInstallStep{}).Delegated(p)
		if !delegated {
			t.Fatalf("run %d: Delegated() = false for a hook running a second complete suite", run)
		}
		for _, want := range []string{".husky/pre-commit", "vitest run", "coverage"} {
			if !strings.Contains(why, want) {
				t.Errorf("run %d: reconciliation does not name %q:\n%s", run, want, why)
			}
		}
	}

	resolved := "#!/usr/bin/env sh\n" + huskyGateLine(p) + "\n"
	if err := os.WriteFile(filepath.Join(p.Root, filepath.FromSlash(huskyHook)), []byte(resolved), 0o600); err != nil {
		t.Fatal(err)
	}
	if !(hookInstallStep{}).Satisfied(p) {
		t.Error("Satisfied() = false after the agent left the gate alone")
	}
	if why, delegated := (hookInstallStep{}).Delegated(p); delegated {
		t.Errorf("Delegated() = %q, true after the duplicate was removed", why)
	}
}

// TestCompoundCommandsAnalyzeRemainingInvocations is the V3 ownership rule:
// the verdict belongs to each actual invocation, not to a substring anywhere
// in the compound. A managed segment beside a real duplicate still delegates.
func TestCompoundCommandsAnalyzeRemainingInvocations(t *testing.T) {
	cases := []struct {
		name   string
		hook   string
		where  string
		script string
	}{
		{
			name:   "check chained with a duplicate",
			hook:   "pre-commit:\n  commands:\n    test:\n      run: dharness check && bun run test\n",
			where:  lefthookConfig,
			script: "{\"test\":\"vitest run\"}",
		},
		{
			name:   "managed coverage chained with a duplicate",
			hook:   "pre-commit:\n  commands:\n    test:\n      run: bunx --no-install vitest run --coverage --config .dharness/vitest.coverage.mjs && bun run test\n",
			where:  lefthookConfig,
			script: "{\"test\":\"vitest run\"}",
		},
		{
			name:   "husky check chained with a duplicate",
			hook:   "#!/usr/bin/env sh\ndharness check && bun run test\n",
			where:  ".husky/pre-commit",
			script: "{\"test\":\"vitest run\"}",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := writeVitestGateProject(t, map[string]string{
				tc.where:       tc.hook,
				"package.json": "{\"name\":\"app\",\"scripts\":" + tc.script + "}\n",
			})

			var why string
			var delegated bool
			if tc.where != lefthookConfig {
				why, delegated = (hookInstallStep{}).Delegated(p)
			} else {
				why, delegated = (lefthookExtendsStep{}).Delegated(p)
			}
			if !delegated {
				t.Fatal("Delegated() = false while a remaining invocation duplicates the managed gate")
			}
			for _, want := range []string{"bun run test", "coverage"} {
				if !strings.Contains(why, want) {
					t.Errorf("reconciliation does not name %q:\n%s", want, why)
				}
			}
		})
	}
}

// TestRootAndConfigIdentity is the V4 rule: obvious same paths resolve as
// the managed suite, obvious different roots stay distinct and integrate
// around, and an identity that cannot be established is reconciliation.
func TestRootAndConfigIdentity(t *testing.T) {
	t.Run("cd into the managed source is the same suite", func(t *testing.T) {
		root := t.TempDir()
		writeVitestGateFile(t, root, "coverage-tiers.json", "{}\n")
		writeVitestGateFile(t, root, "frontend/package.json", "{\"name\":\"app\",\"scripts\":{\"test\":\"vitest run\"}}\n")
		writeVitestGateFile(t, root, lefthookConfig, "pre-commit:\n  commands:\n    test:\n      run: cd frontend && bun run test\n")
		p := project.Project{Root: root, Source: filepath.Join(root, "frontend"), PackageManager: "bun"}

		why, delegated := (lefthookExtendsStep{}).Delegated(p)
		if !delegated {
			t.Fatal("Delegated() = false for a duplicate running in the managed source")
		}
		if !strings.Contains(why, "cd frontend") {
			t.Errorf("reconciliation does not name the concrete command:\n%s", why)
		}
	})

	t.Run("root naming the managed source is the same suite", func(t *testing.T) {
		root := t.TempDir()
		writeVitestGateFile(t, root, "coverage-tiers.json", "{}\n")
		writeVitestGateFile(t, root, "frontend/package.json", "{\"name\":\"app\"}\n")
		// The job runs in the managed JS source because its own `root:` says
		// so — the same suite, even though the hook itself is at the
		// repository root.
		hook := "pre-commit:\n  commands:\n    test:\n      root: frontend/\n      run: npx --no-install vitest run\n"
		writeVitestGateFile(t, root, lefthookConfig, hook)
		p := project.Project{Root: root, Source: filepath.Join(root, "frontend"), PackageManager: "npm"}

		why, delegated := (lefthookExtendsStep{}).Delegated(p)
		if !delegated {
			t.Fatal("Delegated() = false for a duplicate whose own root is the managed source")
		}
		for _, want := range []string{lefthookConfig, "vitest run", "coverage"} {
			if !strings.Contains(why, want) {
				t.Errorf("reconciliation does not name %q:\n%s", want, why)
			}
		}
	})

	t.Run("another root is a distinct suite", func(t *testing.T) {
		root := t.TempDir()
		writeVitestGateFile(t, root, "coverage-tiers.json", "{}\n")
		writeVitestGateFile(t, root, "frontend/package.json", "{\"name\":\"app\"}\n")
		hook := "pre-commit:\n  commands:\n    test:\n      run: npx --no-install vitest run\n      root: backend/\n"
		writeVitestGateFile(t, root, lefthookConfig, hook)
		p := project.Project{Root: root, Source: filepath.Join(root, "frontend"), PackageManager: "npm"}

		if why, delegated := (lefthookExtendsStep{}).Delegated(p); delegated {
			t.Fatalf("Delegated() = %q, true for a suite in another root", why)
		}
		if _, err := (lefthookExtendsStep{}).Apply(p, &Writer{}, io.Discard); err != nil {
			t.Fatalf("Apply() = %v", err)
		}
		raw, _ := os.ReadFile(filepath.Join(root, lefthookConfig))
		if !strings.Contains(string(raw), "root: backend/") {
			t.Errorf("Apply() dropped the consumer's root:\n%s", raw)
		}
	})

	t.Run("explicit native config is the same suite", func(t *testing.T) {
		hook := "pre-commit:\n  commands:\n    test:\n      run: npx --no-install vitest run --config ./vitest.config.js\n"
		p := writeVitestGateProject(t, map[string]string{
			lefthookConfig:     hook,
			"package.json":     "{\"name\":\"app\"}\n",
			"vitest.config.js": "export default {};\n",
		})

		if _, delegated := (lefthookExtendsStep{}).Delegated(p); !delegated {
			t.Fatal("Delegated() = false for the native config named explicitly")
		}
	})

	t.Run("another existing config is a distinct suite", func(t *testing.T) {
		hook := "pre-commit:\n  commands:\n    lint:\n      run: npx --no-install vitest run --config vitest.e2e.js\n"
		p := writeVitestGateProject(t, map[string]string{
			lefthookConfig:     hook,
			"package.json":     "{\"name\":\"app\"}\n",
			"vitest.config.js": "export default {};\n",
			"vitest.e2e.js":    "export default {};\n",
		})

		if why, delegated := (lefthookExtendsStep{}).Delegated(p); delegated {
			t.Fatalf("Delegated() = %q, true for a distinct config", why)
		}
		if _, err := (lefthookExtendsStep{}).Apply(p, &Writer{}, io.Discard); err != nil {
			t.Fatalf("Apply() = %v", err)
		}
	})

	t.Run("missing config cannot be identified", func(t *testing.T) {
		hook := "pre-commit:\n  commands:\n    test:\n      run: npx --no-install vitest run --config ./missing.mjs\n"
		p := writeVitestGateProject(t, map[string]string{
			lefthookConfig:     hook,
			"package.json":     "{\"name\":\"app\"}\n",
			"vitest.config.js": "export default {};\n",
		})

		why, delegated := (lefthookExtendsStep{}).Delegated(p)
		if !delegated {
			t.Fatal("Delegated() = false for an identity that cannot be established")
		}
		if !strings.Contains(why, "uncertain") {
			t.Errorf("unidentified config not reported as uncertainty:\n%s", why)
		}
	})
}

// TestCompleteRunFormsAndUnknownFlags is the V5 rule: `vitest --run` and
// resource flags are the complete run, while a flag the analysis does not
// support stays uncertain rather than silently clearing.
func TestCompleteRunFormsAndUnknownFlags(t *testing.T) {
	t.Run("vitest --run is the complete run", func(t *testing.T) {
		p := writeVitestGateProject(t, map[string]string{
			lefthookConfig: "pre-commit:\n  commands:\n    test:\n      run: npx --no-install vitest --run\n",
			"package.json": "{\"name\":\"app\"}\n",
		})
		if _, delegated := (lefthookExtendsStep{}).Delegated(p); !delegated {
			t.Fatal("Delegated() = false for vitest --run")
		}
	})

	t.Run("resource flags do not narrow the suite", func(t *testing.T) {
		p := writeVitestGateProject(t, map[string]string{
			lefthookConfig: "pre-commit:\n  commands:\n    test:\n      run: npx --no-install vitest run --maxWorkers=2\n",
			"package.json": "{\"name\":\"app\"}\n",
		})
		if _, delegated := (lefthookExtendsStep{}).Delegated(p); !delegated {
			t.Fatal("Delegated() = false for a complete run with resource flags")
		}
	})

	t.Run("unsupported flags stay uncertain", func(t *testing.T) {
		hook := "pre-commit:\n  commands:\n    test:\n      run: npx --no-install vitest run --frobnicate\n"
		p := writeVitestGateProject(t, map[string]string{
			lefthookConfig: hook,
			"package.json": "{\"name\":\"app\"}\n",
		})
		why, delegated := (lefthookExtendsStep{}).Delegated(p)
		if !delegated {
			t.Fatal("Delegated() = false for a flag the analysis does not support")
		}
		if !strings.Contains(why, "uncertain") {
			t.Errorf("unsupported flag not reported as uncertainty:\n%s", why)
		}
		if _, err := (lefthookExtendsStep{}).Apply(p, &Writer{}, io.Discard); err == nil {
			t.Error("Apply() = nil for an uncertain invocation")
		}
		raw, _ := os.ReadFile(filepath.Join(p.Root, lefthookConfig))
		if string(raw) != hook {
			t.Errorf("the refused run touched the consumer's hook:\n%s", raw)
		}
	})
}

// TestUnsupportedDocumentShapesRefuseWithoutRewriting is the V6 guard: a
// document the additive merge cannot take — an explicit marker, a flow
// mapping — is precise reconciliation with its own reason, and the bytes
// stay exactly as found.
func TestUnsupportedDocumentShapesRefuseWithoutRewriting(t *testing.T) {
	cases := []struct {
		name   string
		config string
		reason string
	}{
		// Fields are positional: name, config, reason. One line per shape keeps
		// the shapes readable without repeating a field label under every
		// entry, which the duplication gate counts as repeated code.
		{"explicit document marker", "---\npre-commit:\n  commands:\n    lint:\n      run: npx eslint .\n", "document"},
		{"flow mapping", "{pre-commit: {commands: {lint: {run: \"npx eslint .\"}}}}\n", "mapping"},
		{"flow sequence", "[pre-commit]\n", "flow or sequence"},
		// The comment is skipped, so the flow sequence is still the document's
		// first content line and is refused as that shape — not as the generic
		// "not a block mapping key" the comment would become if it were read as
		// one.
		{"flow sequence after a comment", "# hooks\n[pre-commit]\n", "flow or sequence"},
		{"sequence document", "- run: npx eslint .\n", "flow or sequence"},
		{"bare sequence item", "-\n", "flow or sequence"},
		{"document marker after a comment", "# hooks\n---\npre-commit:\n  commands:\n    lint:\n      run: npx eslint .\n", "document"},
		{"document marker after a blank line", "\n---\npre-commit:\n  commands:\n    lint:\n      run: npx eslint .\n", "document"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := writeVitestGateProject(t, map[string]string{
				lefthookConfig: tc.config,
				"package.json": "{\"name\":\"app\"}\n",
			})

			why, delegated := (lefthookExtendsStep{}).Delegated(p)
			if !delegated {
				t.Fatal("Delegated() = false for a shape the additive merge cannot take")
			}
			for _, want := range []string{lefthookConfig, tc.reason} {
				if !strings.Contains(strings.ToLower(why), want) {
					t.Errorf("reconciliation does not name %q:\n%s", want, why)
				}
			}
			if strings.Contains(why, "already carries an `extends`") {
				t.Errorf("reason blames an extends key the file does not have:\n%s", why)
			}

			if _, err := (lefthookExtendsStep{}).Apply(p, &Writer{}, io.Discard); err == nil {
				t.Error("Apply() = nil for a shape it cannot merge into")
			}
			raw, _ := os.ReadFile(filepath.Join(p.Root, lefthookConfig))
			if string(raw) != tc.config {
				t.Errorf("the refused run rewrote the consumer's config:\n%s", raw)
			}
		})
	}
}

// TestLefthookExtendsReadsPrecommitJobsAndRoots pins what the active hook's
// commands are: only the pre-commit section answers, each job keeps its own
// `root:`, a root the analysis cannot resolve is uncertainty rather than an
// assumption, and the section survives the blank lines, the comments and the
// line endings a consumer actually writes.
func TestLefthookExtendsReadsPrecommitJobsAndRoots(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		hook    string
		scripts string
		want    vitestGateStatus
		command string
		through string
		reason  string
	}{
		{
			name:    "list form job",
			hook:    "pre-commit:\n  commands:\n    - run: npm run test\n",
			scripts: "{\"test\":\"vitest run\"}",
			want:    vitestGateEquivalent,
			command: "npm run test",
			through: "test",
		},
		{
			name:    "a dynamic root is unresolved",
			hook:    "pre-commit:\n  commands:\n    test:\n      root: $PKG_DIR\n      run: npx --no-install vitest run\n",
			want:    vitestGateUnresolved,
			command: "npx --no-install vitest run",
			reason:  "working directory cannot be resolved",
		},
		{
			// One clause per case: this root is command-substituted with no
			// variable in it, so only the backtick check can make it unknown.
			name:    "a command-substituted root is unresolved",
			hook:    "pre-commit:\n  commands:\n    test:\n      root: `pwd`\n      run: npx --no-install vitest run\n",
			want:    vitestGateUnresolved,
			command: "npx --no-install vitest run",
			reason:  "working directory cannot be resolved",
		},
		{
			name:    "a blank line between jobs keeps the section open",
			hook:    "pre-commit:\r\n  commands:\r\n    lint:\r\n      run: npx --no-install eslint .\r\n\r\n    test:\r\n      run: npx --no-install vitest run\r\n",
			want:    vitestGateEquivalent,
			command: "npx --no-install vitest run",
		},
		{
			name:    "a top-level comment between jobs keeps the section open",
			hook:    "pre-commit:\n  commands:\n    lint:\n      run: npx --no-install eslint .\n# the suite runs on every commit\n    test:\n      run: npx --no-install vitest run\n",
			want:    vitestGateEquivalent,
			command: "npx --no-install vitest run",
		},
		{
			name:    "a root belongs to its own job",
			hook:    "pre-commit:\n  commands:\n    lint:\n      root: frontend/\n    test:\n      run: npx --no-install vitest run\n",
			want:    vitestGateEquivalent,
			command: "npx --no-install vitest run",
		},
		{
			// A key spelled with a space before the colon is the same key, so
			// it neither ends the job block nor opens a new one: the root it
			// follows is still the root its runs run in.
			name: "a key spelled with a space keeps the job's root",
			hook: "pre-commit:\n  commands:\n    test:\n      root: frontend/\n      run : npx --no-install eslint .\n      run: npx --no-install vitest run\n",
			want: vitestGateClear,
		},
		{
			name: "a root spelled with a space is not a root",
			hook: "pre-commit:\n  commands:\n    test:\n      root: frontend/\n      root : ignore/\n      run: npx --no-install vitest run\n",
			want: vitestGateClear,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manifest := "{\"name\":\"app\"}\n"
			if tc.scripts != "" {
				manifest = "{\"name\":\"app\",\"scripts\":" + tc.scripts + "}\n"
			}
			got := gateFindingFor(t, tc.source, map[string]string{lefthookConfig: tc.hook, "package.json": manifest})
			if got.status != tc.want {
				t.Fatalf("analyzeVitestGate() status = %s, want %s (command %q, reason %q)",
					gateStatusName(got.status), gateStatusName(tc.want), got.command, got.reason)
			}
			if tc.command != "" && got.command != tc.command {
				t.Errorf("analyzeVitestGate() command = %q, want %q", got.command, tc.command)
			}
			if tc.through != "" && got.through != tc.through {
				t.Errorf("analyzeVitestGate() through = %q, want %q", got.through, tc.through)
			}
			if tc.reason != "" && !strings.Contains(got.reason, tc.reason) {
				t.Errorf("analyzeVitestGate() reason = %q, want it to mention %q", got.reason, tc.reason)
			}
		})
	}
}

// TestLefthookExtendsQuotesPlainValues pins the one transformation a YAML
// plain value wearing quotes needs: one matching pair is stripped, and a
// half-quoted or short value is left exactly as it was written.
func TestLefthookExtendsQuotesPlainValues(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"double quoted", `"npm run test"`, "npm run test"},
		{"single quoted", `'npm run test'`, "npm run test"},
		{"opening double quote only", `"npm run test`, `"npm run test`},
		{"closing double quote only", `npm run test"`, `npm run test"`},
		{"opening single quote only", `'npm run test`, `'npm run test`},
		{"closing single quote only", `npm run test'`, `npm run test'`},
		{"empty quoted value", `""`, ""},
		{"one quote", `"`, `"`},
		{"two unquoted characters", "ab", "ab"},
		{"one unquoted character", "a", "a"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := unquoteGateValue(tc.value); got != tc.want {
				t.Errorf("unquoteGateValue(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

// TestLefthookExtendsManagerFormMatrix pins the script forms a hook actually
// reaches: the two-token `test` alias, an explicit `run`, the chain a reader
// could not follow by eye, and the cycle and depth that stay unresolved. A
// form that cannot be parsed is uncertainty, never a guess in either
// direction.
func TestLefthookExtendsManagerFormMatrix(t *testing.T) {
	deep := `{"s1":"npm run s2","s2":"npm run s3","s3":"npm run s4","s4":"npm run s5","s5":"npm run s6","s6":"npm run s7","s7":"npm run s8","s8":"vitest run"}`
	cases := []struct {
		name    string
		run     string
		scripts string
		want    vitestGateStatus
		through string
		reason  string
	}{
		{name: "the test alias", run: "npm test", scripts: `{"test":"vitest run"}`, want: vitestGateEquivalent, through: "test"},
		{name: "yarn keeps the alias", run: "yarn test", scripts: `{"test":"vitest run"}`, want: vitestGateEquivalent, through: "test"},
		{name: "an exec prefix is read through", run: "exec npm run test", scripts: `{"test":"vitest run"}`, want: vitestGateEquivalent, through: "test"},
		{name: "bun test is bun's own runner", run: "bun test", scripts: `{"test":"vitest run"}`, want: vitestGateClear},
		{name: "the alias is exactly two tokens", run: "npm test src/login.test.ts", scripts: `{"test":"vitest run"}`, want: vitestGateClear},
		{name: "a declared chain is named whole", run: "npm run test", scripts: `{"test":"npm run verify","verify":"npm run check","check":"vitest run"}`, want: vitestGateEquivalent, through: "test → verify → check"},
		{name: "a chain the analysis still follows", run: "npm run s1", scripts: deep, want: vitestGateEquivalent, through: "s1 → s2 → s3 → s4 → s5 → s6 → s7 → s8"},
		{name: "a cycle past the first link", run: "npm run a", scripts: `{"a":"npm run b","b":"npm run c","c":"npm run b"}`, want: vitestGateUnresolved, reason: "refers back to itself"},
		{name: "a bare manager runs nothing", run: "npm", scripts: `{"test":"vitest run"}`, want: vitestGateClear},
		{name: "a bare run cannot be parsed", run: "npm run", scripts: `{"test":"vitest run"}`, want: vitestGateUnresolved, reason: "cannot be parsed"},
		{name: "extra arguments cannot be parsed", run: "npm run test --silent", scripts: `{"test":"vitest run"}`, want: vitestGateUnresolved, reason: "cannot be parsed"},
		{name: "a flag is not a script name", run: "npm run --silent", scripts: `{"test":"vitest run"}`, want: vitestGateUnresolved, reason: "cannot be parsed"},
		{name: "an exec prefix is read through", run: "pnpm exec vitest run", scripts: `{"test":"vitest run"}`, want: vitestGateEquivalent},
		{name: "an exec prefix classifies the script it runs", run: "pnpm exec npm run test", scripts: `{"test":"vitest run"}`, want: vitestGateEquivalent, through: "test"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := gateFindingFor(t, "", hookFixture(tc.run, tc.scripts))
			if got.status != tc.want {
				t.Fatalf("analyzeVitestGate(%q) status = %s, want %s (reason %q)", tc.run, gateStatusName(got.status), gateStatusName(tc.want), got.reason)
			}
			if tc.through != "" && got.through != tc.through {
				t.Errorf("analyzeVitestGate(%q) through = %q, want %q", tc.run, got.through, tc.through)
			}
			if tc.reason != "" && !strings.Contains(got.reason, tc.reason) {
				t.Errorf("analyzeVitestGate(%q) reason = %q, want it to mention %q", tc.run, got.reason, tc.reason)
			}
		})
	}
}

// TestCompleteRunFormsFlagMatrix widens V5: a complete run keeps its suite
// through the resource and coverage flags, a scoping form is left alone, and
// a flag or an argument the analysis cannot read is uncertain rather than
// silently clear.
func TestCompleteRunFormsFlagMatrix(t *testing.T) {
	cases := []struct {
		name string
		run  string
		want vitestGateStatus
	}{
		{"a bare run", "npx --no-install vitest run", vitestGateEquivalent},
		{"run as a flag", "npx --no-install vitest --run", vitestGateEquivalent},
		{"run with a value", "npx --no-install vitest --run=true", vitestGateEquivalent},
		{"a local binary by path", "node_modules/.bin/vitest run", vitestGateEquivalent},
		{"a windows binary by path", ".\\node_modules\\.bin\\vitest run", vitestGateEquivalent},
		{"a windows shim", "vitest.cmd run", vitestGateEquivalent},
		{"a flag before the binary", "npx --yes vitest run", vitestGateEquivalent},
		{"watch by name", "npx --no-install vitest run --watch", vitestGateClear},
		{"watch by short flag", "npx --no-install vitest run -w", vitestGateClear},
		{"watch with a value", "npx --no-install vitest run --watch=true", vitestGateClear},
		{"watch disabled is still a run", "npx --no-install vitest run --watch=false", vitestGateEquivalent},
		{"run mode disabled is a watcher", "npx --no-install vitest --run=false", vitestGateClear},
		{"watch disabled hides nothing", "npx --no-install vitest run --watch=false --frobnicate", vitestGateUnresolved},
		{"name selection", "npx --no-install vitest run -t login", vitestGateClear},
		{"name pattern selection", "npx --no-install vitest run --testNamePattern login", vitestGateClear},
		{"path pattern selection", "npx --no-install vitest run --testPathPattern login", vitestGateClear},
		{"project selection", "npx --no-install vitest run --project web", vitestGateClear},
		{"changed selection", "npx --no-install vitest run --changed", vitestGateClear},
		{"related selection", "npx --no-install vitest run --related", vitestGateClear},
		{name: "a repeated binary token", run: "npx --no-install vitest run vitest", want: vitestGateEquivalent},
		{"coverage is a complete run", "npx --no-install vitest run --coverage", vitestGateEquivalent},
		{"no-color is a complete run", "npx --no-install vitest run --no-color", vitestGateEquivalent},
		{"a reporter value is not a file", "npx --no-install vitest run --reporter dot src/login.test.ts", vitestGateClear},
		{"an inline resource value does not hide a file", "npx --no-install vitest run --maxWorkers=2 src/login.test.ts", vitestGateClear},
		{"a negated flag is not narrowing", "npx --no-install vitest run --no-file-parallelism", vitestGateEquivalent},
		{"an unknown flag beside coverage", "npx --no-install vitest run --coverage --frobnicate", vitestGateUnresolved},
		{"dynamic arguments", "npx --no-install vitest run $EXTRA", vitestGateUnresolved},
		{"a command substitution", "npx --no-install vitest run `ls tests`", vitestGateUnresolved},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := gateFindingFor(t, "", hookFixture(tc.run, ""))
			if got.status != tc.want {
				t.Fatalf("analyzeVitestGate(%q) status = %s, want %s (reason %q)", tc.run, gateStatusName(got.status), gateStatusName(tc.want), got.reason)
			}
		})
	}
}

// TestRootIdentityAndWorkingDirectory pins the identity half of the root of
// execution filter: a directory the analysis cannot place is uncertain, the
// managed suite named explicitly is the same suite, and an existing other
// root or config is a distinct one, left alone.
func TestRootIdentityAndWorkingDirectory(t *testing.T) {
	config := map[string]string{"vitest.config.js": "export default {};\n"}
	cases := []struct {
		name   string
		source string
		run    string
		files  map[string]string
		want   vitestGateStatus
		reason string
	}{
		{name: "cd with no target", run: "cd && bun run test", want: vitestGateUnresolved, reason: "working directory cannot be resolved"},
		{name: "cd with an empty target", run: `cd "" && bun run test`, want: vitestGateUnresolved, reason: "working directory cannot be resolved"},
		{name: "cd with extra words", run: "cd frontend extra && bun run test", want: vitestGateUnresolved, reason: "working directory cannot be resolved"},
		{name: "cd to a dynamic target", run: "cd $DIR && bun run test", want: vitestGateUnresolved, reason: "working directory cannot be resolved"},
		{name: "cd to the previous directory", run: "cd - && bun run test", want: vitestGateUnresolved, reason: "working directory cannot be resolved"},
		{name: "cd to a command substitution", run: "cd `pwd` && bun run test", want: vitestGateUnresolved, reason: "working directory cannot be resolved"},
		{name: "cd into the source keeps the suite", run: "cd . && bun run test", want: vitestGateEquivalent},
		{name: "the native config named by value", run: "npx --no-install vitest run --config=./vitest.config.js src/login.test.ts", files: config, want: vitestGateClear},
		{name: "another config by value", run: "npx --no-install vitest run --config=vitest.e2e.js", files: map[string]string{"vitest.config.js": "export default {};\n", "vitest.e2e.js": "export default {};\n"}, want: vitestGateClear},
		{name: "a config with no value", run: "npx --no-install vitest run --config", files: config, want: vitestGateUnresolved, reason: "cannot be identified"},
		{name: "the native config by short flag", source: "frontend", run: "cd frontend && npx --no-install vitest run -c vitest.config.js", files: map[string]string{"frontend/vitest.config.js": "export default {};\n"}, want: vitestGateEquivalent},
		{name: "the source by dir", source: "frontend", run: "cd frontend && npx --no-install vitest run --dir .", want: vitestGateEquivalent},
		{name: "the source by root", source: "frontend", run: "cd frontend && npx --no-install vitest run --root .", want: vitestGateEquivalent},
		{name: "another project by dir", source: "frontend", run: "cd frontend && npx --no-install vitest run --dir=../backend", files: map[string]string{"backend/package.json": "{\"name\":\"api\"}\n"}, want: vitestGateClear},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manifest := "{\"name\":\"app\",\"scripts\":{\"test\":\"vitest run\"}}\n"
			files := map[string]string{"package.json": manifest, lefthookConfig: "pre-commit:\n  commands:\n    test:\n      run: " + tc.run + "\n"}
			if tc.source != "" {
				delete(files, "package.json")
				files[tc.source+"/package.json"] = manifest
			}
			for name, contents := range tc.files {
				files[name] = contents
			}
			got := gateFindingFor(t, tc.source, files)
			if got.status != tc.want {
				t.Fatalf("analyzeVitestGate(%q) status = %s, want %s (reason %q)", tc.run, gateStatusName(got.status), gateStatusName(tc.want), got.reason)
			}
			if tc.reason != "" && !strings.Contains(got.reason, tc.reason) {
				t.Errorf("analyzeVitestGate(%q) reason = %q, want it to mention %q", tc.run, got.reason, tc.reason)
			}
		})
	}
}

// TestLefthookExtendsIntegratesAroundANestedExtendsKey keeps the other half
// of the top-level rule: only the document's own `extends` blocks the
// additive prepend. A key of that name nested under another key is not it, so
// the reference is added and every byte the project wrote survives.
func TestLefthookExtendsIntegratesAroundANestedExtendsKey(t *testing.T) {
	original := "pre-commit:\n extends: ./shared-job.yml\n commands:\n  lint:\n   run: npx --no-install eslint .\n"
	p := vitestGateHookFixture(t, original)

	if why, delegated := (lefthookExtendsStep{}).Delegated(p); delegated {
		t.Fatalf("Delegated() = %q, true for a nested `extends` key that is not the document's own", why)
	}
	raw := applyLefthookExtendsStep(t, p)
	if !strings.Contains(raw, ownedFrom(p, p.Root, ownedLefthook)) || !strings.Contains(raw, original) {
		t.Errorf("the additive merge lost the project's own config:\n%s", raw)
	}
	if !(lefthookExtendsStep{}).Satisfied(p) {
		t.Error("Satisfied() = false right after Apply wrote the reference")
	}
}

// TestCompoundCommandsKeepTheFirstUncertainInvocation is the compound half
// of the uncertainty rule: a segment that runs nothing does not clear an
// invocation the analysis cannot resolve, and the first one the agent must
// look at is the one that is reported.
func TestCompoundCommandsKeepTheFirstUncertainInvocation(t *testing.T) {
	cases := []struct {
		name    string
		run     string
		scripts string
		through string
	}{
		{
			name:    "an unrelated check does not clear an unresolved wrapper",
			run:     "npx eslint . && npm run check",
			scripts: "{\"check\":\"node scripts/ci.mjs\"}",
			through: "check",
		},
		{
			name:    "the first unresolved invocation is the one reported",
			run:     "npm run first && npm run second",
			scripts: "{\"first\":\"node scripts/one.mjs\",\"second\":\"node scripts/two.mjs\"}",
			through: "first",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := analyzeVitestGate(writeVitestGateProject(t, hookFixture(tc.run, tc.scripts)))
			if got.status != vitestGateUnresolved {
				t.Fatalf("analyzeVitestGate() status = %s, want uncertainty (reason %q)", gateStatusName(got.status), got.reason)
			}
			if got.through != tc.through {
				t.Errorf("analyzeVitestGate() through = %q, want %q", got.through, tc.through)
			}
		})
	}
}

// TestLefthookLauncherScriptIsUncertainRatherThanIntegrated is the first
// verifier blocker, at the public boundary a consumer reads: `bun
// scripts/check.mjs` executes a file whose contents dharness never reads,
// exactly like `node scripts/check.mjs`, so the answer is uncertainty handed
// to the agent — never the unrelated-command default the script-manager
// branch used to answer with for the `bun` spelling.
func TestLefthookLauncherScriptIsUncertainRatherThanIntegrated(t *testing.T) {
	cases := []struct {
		name   string
		run    string
		target string
	}{
		{name: "bun runs a file", run: "bun scripts/check.mjs", target: "scripts/check.mjs"},
		{name: "node runs a file", run: "node scripts/check.mjs", target: "scripts/check.mjs"},
		{name: "bun runs a file after a flag", run: "bun --bun scripts/check.mjs", target: "scripts/check.mjs"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := writeVitestGateProject(t, map[string]string{
				lefthookConfig: "pre-commit:\n  commands:\n    check:\n      run: " + tc.run + "\n",
				"package.json": "{\"name\":\"app\"}\n",
			})

			got := analyzeVitestGate(p)
			if got.status != vitestGateUnresolved {
				t.Fatalf("analyzeVitestGate(%q) status = %s, want uncertainty: a launcher runs a file this analysis never reads",
					tc.run, gateStatusName(got.status))
			}
			if !strings.Contains(got.reason, tc.target) {
				t.Errorf("analyzeVitestGate(%q) reason = %q, want it to name %q", tc.run, got.reason, tc.target)
			}

			why, delegated := (lefthookExtendsStep{}).Delegated(p)
			if !delegated {
				t.Fatalf("Delegated() = false for %q, which cannot be resolved to a concrete invocation", tc.run)
			}
			for _, want := range []string{tc.run, "uncertain"} {
				if !strings.Contains(why, want) {
					t.Errorf("reconciliation does not name %q:\n%s", want, why)
				}
			}

			if _, err := (lefthookExtendsStep{}).Apply(p, &Writer{}, io.Discard); err == nil {
				t.Error("Apply() = nil for an invocation the analysis cannot resolve")
			}
			raw, _ := os.ReadFile(filepath.Join(p.Root, lefthookConfig))
			if !strings.Contains(string(raw), tc.run) {
				t.Errorf("the refused run rewrote the consumer's hook:\n%s", raw)
			}
		})
	}
}

// TestLefthookManagerExecPrefixRunsTheCommandItNames is the second verifier
// blocker: `pnpm exec vitest run` runs Vitest through the manager's own `exec`
// (or `x`) prefix, so it is a complete run of the same suite and takes the
// reconciliation route exactly like the direct form does. The consumer's
// config survives the delegated run byte for byte.
func TestLefthookManagerExecPrefixRunsTheCommandItNames(t *testing.T) {
	cases := []struct {
		name string
		run  string
	}{
		{name: "pnpm exec", run: "pnpm exec vitest run"},
		{name: "yarn exec", run: "yarn exec vitest run"},
		{name: "npm exec", run: "npm exec vitest run"},
		{name: "bun x", run: "bun x vitest run"},
		{name: "pnpm x", run: "pnpm x vitest run"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hook := "pre-commit:\n  commands:\n    test:\n      run: " + tc.run + "\n"
			p := writeVitestGateProject(t, map[string]string{
				lefthookConfig: hook,
				"package.json": "{\"name\":\"app\"}\n",
			})

			got := analyzeVitestGate(p)
			if got.status != vitestGateEquivalent {
				t.Fatalf("analyzeVitestGate(%q) status = %s, want a confirmed duplicate",
					tc.run, gateStatusName(got.status))
			}

			why, delegated := (lefthookExtendsStep{}).Delegated(p)
			if !delegated {
				t.Fatalf("Delegated() = false for %q, a complete run of the managed suite", tc.run)
			}
			for _, want := range []string{tc.run, coverageBridgeName, "one complete"} {
				if !strings.Contains(why, want) {
					t.Errorf("reconciliation does not name %q:\n%s", want, why)
				}
			}

			if _, err := applySteps([]Step{(lefthookExtendsStep{}), (hookInstallStep{})}, p, io.Discard); err != nil {
				t.Fatalf("applySteps() = %v, want nil: the delegated run continues instead of aborting", err)
			}
			raw, _ := os.ReadFile(filepath.Join(p.Root, lefthookConfig))
			if string(raw) != hook {
				t.Errorf("the delegated run changed the consumer's config:\n%s", raw)
			}
		})
	}
}

// TestHuskyHookLineKeepsItsOwnWorkingDirectory is the third verifier blocker:
// a husky hook line is handed to the classifier whole, so the `cd` it opens
// still applies to the command it chains. `cd frontend && bun run test` runs
// the managed suite from the managed source and delegates, exactly as the
// identical lefthook job already does, instead of being split before analysis
// and judged at the repository root.
func TestHuskyHookLineKeepsItsOwnWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	writeVitestGateFile(t, root, "coverage-tiers.json", "{}\n")
	writeVitestGateFile(t, root, "frontend/package.json", "{\"name\":\"app\",\"scripts\":{\"test\":\"vitest run\"}}\n")
	hook := "#!/usr/bin/env sh\ncd frontend && bun run test\n"
	writeVitestGateFile(t, root, filepath.ToSlash(huskyHook), hook)
	p := project.Project{Root: root, Source: filepath.Join(root, "frontend"), PackageManager: "npm"}

	got := analyzeVitestGate(p)
	if got.status != vitestGateEquivalent {
		t.Fatalf("analyzeVitestGate() status = %s, want a confirmed duplicate: the cd names the managed source (reason %q)",
			gateStatusName(got.status), got.reason)
	}

	if (hookInstallStep{}).Satisfied(p) {
		t.Error("Satisfied() = true while the hook runs a second complete suite")
	}
	why, delegated := (hookInstallStep{}).Delegated(p)
	if !delegated {
		t.Fatal("Delegated() = false for a husky hook running the full suite from the managed source")
	}
	for _, want := range []string{".husky/pre-commit", "cd frontend && bun run test", "coverage"} {
		if !strings.Contains(why, want) {
			t.Errorf("reconciliation does not name %q:\n%s", want, why)
		}
	}

	if _, err := applySteps([]Step{(hookInstallStep{})}, p, io.Discard); err != nil {
		t.Fatalf("applySteps() = %v, want nil: the delegated run continues instead of aborting", err)
	}
	raw, _ := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(huskyHook)))
	if string(raw) != hook {
		t.Errorf("the delegated run changed the consumer's hook:\n%s", raw)
	}
}

// gateFindingFor builds one activated repository and reports what sync's
// analysis answers. js is the JS project root relative to the repository
// root, or "" when the two are the same directory.
func gateFindingFor(t *testing.T, js string, files map[string]string) vitestGateFinding {
	t.Helper()
	root := t.TempDir()
	for name, contents := range files {
		writeVitestGateFile(t, root, name, contents)
	}
	if _, ok := files["coverage-tiers.json"]; !ok {
		writeVitestGateFile(t, root, "coverage-tiers.json", "{}\n")
	}
	source := root
	if js != "" {
		source = filepath.Join(root, filepath.FromSlash(js))
	}
	return analyzeVitestGate(project.Project{Root: root, Source: source, PackageManager: "npm"})
}

// hookFixture is the smallest lefthook fixture that runs one command, with
// the given package scripts.
func hookFixture(run, scripts string) map[string]string {
	manifest := "{\"name\":\"app\"}\n"
	if scripts != "" {
		manifest = "{\"name\":\"app\",\"scripts\":" + scripts + "}\n"
	}
	return map[string]string{
		lefthookConfig: "pre-commit:\n  commands:\n    test:\n      run: " + run + "\n",
		"package.json": manifest,
	}
}

// gateStatusName names a verdict the way the reconciliation reads it, so a
// failure says which answer was expected.
func gateStatusName(status vitestGateStatus) string {
	switch status {
	case vitestGateEquivalent:
		return "a confirmed duplicate"
	case vitestGateUnresolved:
		return "uncertainty"
	default:
		return "clear"
	}
}

// writeVitestGateFile writes one fixture file below an explicitly built root,
// for layouts writeVitestGateProject's conventional default cannot express.
func writeVitestGateFile(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
