package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/report"
)

// A missing opt-in is a choice for the agent, not proof of enforcement.
// Exercise both readers through sync and leave the consumer's policy absent.
func TestSyncOffersCoverageAdoptionWithoutEnablingIt(t *testing.T) {
	for _, format := range []string{"human", "json"} {
		t.Run(format, func(t *testing.T) {
			commands := stubRunner(t)
			root := t.TempDir()
			gitProject(t, root, "bun.lock")
			satisfied(t, root)
			const config = "export default { test: {} };\n"
			writeFile(t, filepath.Join(root, "vitest.config.js"), config)
			previous := workingDirectory
			workingDirectory = func() (string, error) { return root, nil }
			t.Cleanup(func() { workingDirectory = previous })

			for run := 0; run < 2; run++ {
				var out bytes.Buffer
				if err := RunSync([]string{"--format", format}, &out); err != nil {
					t.Fatalf("RunSync(%s), run %d: %v\n%s", format, run, err, out.String())
				}
				guidance := out.String()
				if format == "json" {
					var result report.Report
					if err := json.Unmarshal(out.Bytes(), &result); err != nil {
						t.Fatal(err)
					}
					found := false
					for _, step := range result.Steps {
						if step.ID == "enforce the coverage tiers in the native test run" {
							found = true
							if step.Status != report.Delegated || len(step.Wrote) != 0 || step.Evidence != "" {
								t.Fatalf("missing policy reported as completed: %+v", step)
							}
							guidance = step.Why
						}
					}
					if !found {
						t.Fatal("coverage adoption missing from JSON report")
					}
				} else {
					start := strings.Index(guidance, "Left to you")
					if start < 0 {
						t.Fatalf("adoption not offered under Left to you:\n%s", guidance)
					}
					guidance = guidance[start:]
					if end := strings.Index(guidance, "Already in place"); end >= 0 {
						guidance = guidance[:end]
					}
				}
				for _, want := range []string{
					"coverage-tiers.json", "optional", "not enforced", "Offer",
					"CORE", "100%", "IMPORTANT", "80%", "INFRASTRUCTURE", "0%",
					"enforcedPrefixes", "defaultTier", "reason", "existing", "not-yet-created",
					"Vitest", "Jest", "bun add -d", "@vitest/coverage-v8",
					"dharness policy --root", "dharness sync", "test:coverage", "CI",
				} {
					if !strings.Contains(normalizeSpace(guidance), want) {
						t.Errorf("agent guidance missing %q:\n%s", want, guidance)
					}
				}
			}
			for _, name := range []string{"coverage-tiers.json", ".dharness/coverage-tiers.mjs", ".dharness/vitest.coverage.mjs", ".agents/skills/coverage-tiers/SKILL.md"} {
				if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); !os.IsNotExist(err) {
					t.Errorf("offering adoption created %s (stat error %v)", name, err)
				}
			}
			if raw, err := os.ReadFile(filepath.Join(root, "vitest.config.js")); err != nil || string(raw) != config {
				t.Errorf("offering adoption changed the native config: %q, %v", raw, err)
			}
			for _, cmd := range *commands {
				if strings.Contains(strings.Join(cmd.Args, " "), "@vitest/coverage") || cmd.Label == "vitest" {
					t.Errorf("offering adoption installed or ran coverage: %+v", cmd)
				}
			}
		})
	}
}
