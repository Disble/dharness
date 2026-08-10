package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Disble/dharness/internal/tool"
)

// BenchmarkDescribeAndBuildRemoteInvocations measures everything check does before it spawns
// anything: detect the package manager, read package.json, and build all three
// remote invocations. The question it answers is whether caching that result
// would save anything worth a stale-cache failure mode.
func BenchmarkDescribeAndBuildRemoteInvocations(b *testing.B) {
	root := b.TempDir()
	if err := os.WriteFile(filepath.Join(root, "bun.lock"), nil, 0o600); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"devDependencies":{"vitest":"^4.0.0","react":"^19.0.0","typescript":"^5.9.0"}}`), 0o600); err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		p := Describe(root)
		tool.RemoteLatest(p.PackageManager, tool.ReactDoctor, p.Source)
		tool.RemoteLatest(p.PackageManager, tool.Fallow, p.Source)
		_, _ = tool.StrykerCommand(p.PackageManager, p.YarnPnP, p.Source, p.TestRunner, nil, "run")
	}
}
