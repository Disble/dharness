package app_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// This process fixture proves native command routing and output, not Stryker's
// mutation engine. Neither tool installations nor network access are needed.
const mutationProcessFixture = `package main
import ("encoding/json"; "fmt"; "os"; "path/filepath")
func main() {
 if len(os.Args) < 2 || os.Args[1] != "run" { return }
 raw, err := os.ReadFile(os.Getenv("DHARNESS_PROBE_REPORT")); if err != nil { panic(err) }
 report := filepath.Join("reports", "mutation", "mutation.json")
 if err := os.MkdirAll(filepath.Dir(report), 0755); err != nil { panic(err) }
 if err := os.WriteFile(report, raw, 0600); err != nil { panic(err) }
 args, _ := json.Marshal(os.Args[1:])
 fmt.Printf("process fixture: %s\n", args)
}
`

func TestMutationTiersBinaryScopeAndRefusal(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs native processes in a disposable consumer")
	}
	scratch := t.TempDir()
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	build := func(output, target string) {
		t.Helper()
		cmd := exec.Command("go", "build", "-o", output, target)
		cmd.Dir = filepath.Join("..", "..")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build outside checkout: %v\n%s", err, out)
		}
	}
	binary := filepath.Join(scratch, "dharness.exe")
	build(binary, "./cmd/dharness")
	fixtureSource := filepath.Join(scratch, "process.go")
	fixtureBinary := filepath.Join(scratch, "process.exe")
	write(fixtureSource, mutationProcessFixture)
	build(fixtureBinary, fixtureSource)
	root := filepath.Join(scratch, "consumer")
	manifest := `{"version":1,"enforcedPrefixes":["src/cart/"],"features":[{"id":"cart","path":"src/cart/","defaultTier":"core","reason":"Decisions","overrides":[{"path":"src/cart/ui.ts","tier":"important","reason":"UI"},{"path":"src/cart/types.ts","tier":"infrastructure","reason":"Types"}]}]}`
	for name, content := range map[string]string{
		"coverage-tiers.json": manifest,
		"package.json":        `{"packageManager":"bun@1.3.0","devDependencies":{"vitest":"4.0.0","@stryker-mutator/core":"9.0.0","@stryker-mutator/vitest-runner":"9.0.0"}}`,
		"bun.lock":            "",
		".gitignore":          "node_modules/\n.dharness/\nreports/\n",
		"src/cart/total.ts":   "export const total = 1;\n",
		"src/cart/ui.ts":      "export const ui = true;\n",
		"src/cart/types.ts":   "export type Total = number;\n",
		"src/legacy.ts":       "export const legacy = 1;\n",
	} {
		write(filepath.Join(root, filepath.FromSlash(name)), content)
	}
	// Local shims exercise Windows .cmd routing too. The fixture manager does
	// nothing; the fixture Stryker emits the scripted native report above.
	binDir := filepath.Join(root, "node_modules", ".bin")
	for _, name := range []string{"bun", "stryker"} {
		path := filepath.Join(binDir, name)
		content := "#!/bin/sh\nexec '" + filepath.ToSlash(fixtureBinary) + "' \"$@\"\n"
		if runtime.GOOS == "windows" {
			path += ".cmd"
			content = "@echo off\r\n\"" + fixtureBinary + "\" %*\r\n"
		}
		write(path, content)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	reportFixture := filepath.Join(scratch, "report.json")
	t.Setenv("DHARNESS_PROBE_REPORT", reportFixture)
	write(reportFixture, `{"files":{"src/cart/total.ts":{"mutants":[{"status":"Killed","location":{"start":{"line":1}}}]},"src/cart/ui.ts":{"mutants":[{"status":"Killed","location":{"start":{"line":1}}}]},"src/cart/types.ts":{"mutants":[{"status":"Survived","location":{"start":{"line":1}}}]},"src/legacy.ts":{"mutants":[{"status":"Killed","location":{"start":{"line":1}}}]}}}`)
	// Clear Git pinning inherited from a pre-commit index gate. All Git work
	// and all binary invocations belong to this disposable consumer only.
	for _, name := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_PREFIX"} {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) {
		t.Helper()
		args = append([]string{"-c", "user.name=dharness test", "-c", "user.email=test@dharness.invalid", "-c", "commit.gpgsign=false"}, args...)
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fixture git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "--quiet")
	git("add", ".")
	git("commit", "--quiet", "-m", "fixture baseline")
	run := func(args ...string) (string, string, int) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = root
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		code := 0
		if err := cmd.Run(); err != nil {
			if exit, ok := err.(*exec.ExitError); ok {
				code = exit.ExitCode()
			} else {
				t.Fatal(err)
			}
		}
		return stdout.String(), stderr.String(), code
	}
	stdout, stderr, code := run("mutate", "--help")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "coverage-tiers.json excludes declared infrastructure; unadopted paths stay eligible") {
		t.Fatalf("native mutation help: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	for _, args := range [][]string{{"mutate", "src/cart/types.ts"}, {"mutate", "--dry-run", "src/cart/types.ts"}} {
		stdout, stderr, code := run(args...)
		if code != 0 || stdout != "no runtime source to mutate after coverage-tier exclusions\n" || stderr != "" {
			t.Fatalf("native no-op: code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
		if _, err := os.Stat(filepath.Join(root, ".dharness")); !os.IsNotExist(err) {
			t.Fatalf("no-op wrote mutation state: %v", err)
		}
	}
	// The staged command must read the committed declaration, even when the
	// working manifest attempts to make the staged infrastructure eligible.
	write(filepath.Join(root, "src", "cart", "types.ts"), "export type Total = string;\n")
	git("add", "src/cart/types.ts")
	workingManifest := strings.Replace(manifest, `"tier":"infrastructure"`, `"tier":"core"`, 1)
	write(filepath.Join(root, "coverage-tiers.json"), workingManifest)
	stdout, stderr, code = run("mutate", "--staged")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "no runtime source to mutate after coverage-tier exclusions\n") || !strings.Contains(stdout, "classify not started") || strings.Contains(stdout, "process fixture:") {
		t.Fatalf("native staged no-op: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if after, err := os.ReadFile(filepath.Join(root, "coverage-tiers.json")); err != nil || string(after) != workingManifest {
		t.Fatalf("staged no-op modified working declaration: %q, %v", after, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".dharness")); !os.IsNotExist(err) {
		t.Fatalf("staged no-op wrote mutation state: %v", err)
	}
	write(filepath.Join(root, "coverage-tiers.json"), manifest)
	stdout, stderr, code = run("mutate", "src/cart/total.ts:1:3-1:5", "src/cart/ui.ts", "src/cart/types.ts", "src/legacy.ts")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"--mutate","src/cart/total.ts:1:3-1:5,src/cart/ui.ts,src/legacy.ts"`) || !strings.Contains(stdout, "Every mutant was caught") {
		t.Fatalf("native mixed scope: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	// Cumulative reports may name excluded files in the existing outside-scope
	// note; that is not scope re-entry and must keep its previous behavior.
	if !strings.Contains(stdout, "1 file(s) this run did not name: src/cart/types.ts") {
		t.Fatalf("existing cumulative-report boundary changed: %s", stdout)
	}
	for _, tc := range []struct {
		status, message string
	}{
		{"Survived", "1 mutant(s) survived: a test would not have noticed this code breaking"},
		{"NoCoverage", "1 mutant(s) never ran under a test: nothing would have noticed this code breaking"},
	} {
		write(reportFixture, `{"files":{"src/cart/total.ts":{"mutants":[{"status":"`+tc.status+`","location":{"start":{"line":1}}}]}}}`)
		stdout, stderr, code = run("mutate", "src/cart/total.ts")
		if code != 1 || stderr != "dharness: "+tc.message+"\n" || strings.Contains(stdout, "Every mutant was caught") {
			t.Fatalf("native eligible %s: code=%d stdout=%q stderr=%q", tc.status, code, stdout, stderr)
		}
	}
	reportPath := filepath.Join(root, "reports", "mutation", "mutation.json")
	before, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(root, "coverage-tiers.json"), `{"version":7}`)
	stdout, stderr, code = run("mutate", "src/cart/total.ts")
	if code != 1 || stdout != "" || stderr != "dharness: coverage-tiers.json: version must be 1\n" {
		t.Fatalf("native policy refusal: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if after, err := os.ReadFile(filepath.Join(root, "coverage-tiers.json")); err != nil || string(after) != `{"version":7}` {
		t.Fatalf("rejection modified manifest: %q, %v", after, err)
	}
	if after, err := os.ReadFile(reportPath); err != nil || !bytes.Equal(after, before) {
		t.Fatalf("rejection modified existing mutation report: %q, %v", after, err)
	}
}
