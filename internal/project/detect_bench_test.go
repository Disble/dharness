package project

import (
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkDescribeAndResolve measures everything check does before it spawns
// anything: detect the package manager, read package.json, and resolve all
// three tools. The question it answers is whether caching that result would
// save anything worth a stale-cache failure mode.
func BenchmarkDescribeAndResolve(b *testing.B) {
	root := b.TempDir()
	if err := os.WriteFile(filepath.Join(root, "bun.lock"), nil, 0o600); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"devDependencies":{"vitest":"^4.0.0","react":"^19.0.0","typescript":"^5.9.0"}}`), 0o600); err != nil {
		b.Fatal(err)
	}
	binDir := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		b.Fatal(err)
	}
	for _, name := range []string{"react-doctor", "fallow", "stryker"} {
		if err := os.WriteFile(filepath.Join(binDir, name), nil, 0o600); err != nil {
			b.Fatal(err)
		}
	}

	for b.Loop() {
		p := Describe(root)
		p.Resolve("react-doctor")
		p.Resolve("fallow")
		p.Resolve("stryker")
	}
}
