package setup

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCoverageTiersModuleIsTheEmbeddedManagedAsset(t *testing.T) {
	asset, err := os.ReadFile("assets/coverage-tiers/runtime.mjs")
	if err != nil {
		t.Fatal(err)
	}
	copy := coverageTiersModule()
	if len(copy) == 0 || !bytes.Equal(copy, asset) {
		t.Fatal("the managed module must contain the embedded asset's bytes")
	}
	copy[0] ^= 1
	if !bytes.Equal(coverageTiersModule(), asset) {
		t.Fatal("a caller's write changed the embedded module")
	}
}

// The JS fixture owns the public contracts (S1-S5, S9, S11). This host only
// supplies the actual Go policy binary and isolated consumer trees. Its Vitest
// API/provider fixtures do not establish native engine compatibility.
func TestCoverageTiersRuntime(t *testing.T) {
	if testing.Short() {
		t.Skip("the runtime contract test builds the policy binary and runs Node")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required to execute the embedded JS runtime contracts")
	}
	moduleRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	name := "dharness"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	build := exec.Command("go", "build", "-o", filepath.Join(bin, name), "./cmd/dharness")
	build.Dir = moduleRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build the policy binary: %v\n%s", err, output)
	}
	// Fault injection only corrupts the policy wire response; it never classifies.
	// The fixture picks the bytes printed and, for the silent-death case, the
	// exit status, so the module's own reporting of a failed resolver stays
	// under test without a second classification path.
	faultBin := filepath.Join(root, "fault-bin")
	if err := os.Mkdir(faultBin, 0o700); err != nil {
		t.Fatal(err)
	}
	faultSource := filepath.Join(root, "fault-policy.go")
	fault := "package main\n" +
		"import (\"fmt\"; \"os\"; \"strconv\")\n" +
		"func main() {\n" +
		"\tfmt.Print(os.Getenv(\"__DHARNESS_TEST_POLICY_WIRE\"))\n" +
		"\traw := os.Getenv(\"__DHARNESS_TEST_POLICY_EXIT\")\n" +
		"\tif raw == \"\" {\n" +
		"\t\treturn\n" +
		"\t}\n" +
		"\tcode, err := strconv.Atoi(raw)\n" +
		"\tif err != nil {\n" +
		"\t\tfmt.Fprintln(os.Stderr, err)\n" +
		"\t\tos.Exit(2)\n" +
		"\t}\n" +
		"\tos.Exit(code)\n" +
		"}\n"
	if err := os.WriteFile(faultSource, []byte(fault), 0o600); err != nil {
		t.Fatal(err)
	}
	faultBuild := exec.Command("go", "build", "-o", filepath.Join(faultBin, name), faultSource)
	if output, err := faultBuild.CombinedOutput(); err != nil {
		t.Fatalf("build the malformed-response fixture: %v\n%s", err, output)
	}
	module := filepath.Join(root, "coverage-tiers.mjs")
	if err := os.WriteFile(module, coverageTiersModule(), 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(moduleRoot, "internal/setup/testdata/coverage-tiers/runtime.test.mjs")
	command := exec.Command(node, script, root, module, bin, faultBin)
	command.Dir = root
	command.Env = os.Environ() // Includes Stryker's active-mutant selector.
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("public runtime contracts: %v\nstdout:\n%sstderr:\n%s", err, &stdout, &stderr)
	}
	if stdout.String() != "coverage tiers runtime contracts passed\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected runtime output: stdout=%q stderr=%q", &stdout, &stderr)
	}
}
