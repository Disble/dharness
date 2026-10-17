package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Disble/dharness/internal/project"
)

// TestWriteGeneratedJSForAudit writes every JavaScript file dharness generates
// into .dharness/ to the directory named by DHARNESS_GENERATED_JS_DIR, so CI
// can run react-doctor over them the way a consumer's gate does.
//
// Those files are dharness's code, not the project's: the consumer cannot fix
// them, and `sync` rewrites any edit. A finding the gate raises on one of them
// is a dharness bug that surfaces on the consumer's commit, and only on the
// commits that stage them — the adoption commit and every regeneration. The
// verdict belongs to react-doctor's exit code (§11); this test only produces
// the input, which is why it skips when the variable is unset.
//
// The ESLint layer is taken from Apply over the golden projects, so every
// preset combination and both module systems are covered by the same bytes
// sync writes. The coverage-tiers runtime and its bridge are not part of any
// golden project's tree, so they are rendered directly.
func TestWriteGeneratedJSForAudit(t *testing.T) {
	out := os.Getenv("DHARNESS_GENERATED_JS_DIR")
	if out == "" {
		t.Skip("DHARNESS_GENERATED_JS_DIR is unset; CI sets it to audit the generated JavaScript")
	}

	cases := []struct {
		name    string
		project func(t *testing.T) project.Project
	}{
		{"generic-conventional", genericConventionalProject},
		{"wails", wailsProject},
		{"nextjs", nextjsProject},
		{"expo", expoProject},
		{"expo-commonjs", expoCommonJSProject},
		{"wails-nextjs", wailsNextjsProject},
	}
	for _, tc := range cases {
		p := tc.project(t)
		renderGolden(t, p)

		owned := filepath.Join(p.Root, project.Dir)
		entries, err := os.ReadDir(owned)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			switch filepath.Ext(entry.Name()) {
			case ".js", ".mjs", ".cjs":
				data, err := os.ReadFile(filepath.Join(owned, entry.Name()))
				if err != nil {
					t.Fatal(err)
				}
				writeAuditFile(t, filepath.Join(out, tc.name, entry.Name()), data)
			}
		}
	}

	p := genericConventionalProject(t)
	bridge, err := coverageBridgeModule(filepath.Join(p.Root, coverageVitestConfigNames[0]), p)
	if err != nil {
		t.Fatal(err)
	}
	writeAuditFile(t, filepath.Join(out, "coverage", coverageBridgeName), bridge)
	writeAuditFile(t, filepath.Join(out, "coverage", coverageTiersRuntimeName), coverageTiersModule())
}

func writeAuditFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
