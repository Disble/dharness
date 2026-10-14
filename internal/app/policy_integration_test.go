package app_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPolicyBinaryReportsTiersAndRejectsInvalidInput(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the real binary in a disposable project")
	}
	scratch := t.TempDir()
	binary := filepath.Join(scratch, "dharness.exe")
	build := exec.Command("go", "build", "-o", binary, "./cmd/dharness")
	build.Dir = filepath.Join("..", "..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build outside repository: %v\n%s", err, output)
	}
	root := filepath.Join(scratch, "consumer")
	if err := os.MkdirAll(filepath.Join(root, "src", "ui"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"version":1,"enforcedPrefixes":["src/"],"features":[{"id":"cart","path":"src/","defaultTier":"core","reason":"Decisions","overrides":[{"path":"src/ui/","tier":"important","reason":"UI"},{"path":"src/types.ts","tier":"infrastructure","reason":"Types"}]}]}`
	for name, content := range map[string]string{
		"coverage-tiers.json": manifest,
		"src/total.ts":        "export const total = 1;\n",
		"src/types.ts":        "export type Amount = number;\n",
		"src/ui/Cart.tsx":     "export const Cart = () => null;\n",
	} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) (string, string, int) {
		if len(args) == 0 {
			args = []string{"policy", "--root", root}
		}
		cmd := exec.Command(binary, args...)
		cmd.Dir = root
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		code := 0
		if err != nil {
			if exit, ok := err.(*exec.ExitError); ok {
				code = exit.ExitCode()
			} else {
				t.Fatalf("start policy process: %v", err)
			}
		}
		return stdout.String(), stderr.String(), code
	}
	stdout, stderr, code := run()
	want := "{\"version\":1,\"active\":true,\"files\":[{\"path\":\"src/total.ts\",\"tier\":\"core\",\"coverage\":100},{\"path\":\"src/types.ts\",\"tier\":\"infrastructure\",\"coverage\":0},{\"path\":\"src/ui/Cart.tsx\",\"tier\":\"important\",\"coverage\":80}]}\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Fatalf("native output: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	stdout, stderr, code = run("--help")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "policy --root") || !strings.Contains(stdout, "not a test runner") {
		t.Fatalf("native help: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	manifestPath := filepath.Join(root, "coverage-tiers.json")
	if err := os.WriteFile(manifestPath, []byte(`{"version":7}`), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code = run()
	if code != 1 || stdout != "" || stderr != "dharness: coverage-tiers.json: version must be 1\n" {
		t.Fatalf("native rejection: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	after, err := os.ReadFile(manifestPath)
	if err != nil || string(after) != `{"version":7}` {
		t.Fatalf("native rejection modified consumer input: %q, %v", after, err)
	}
}
