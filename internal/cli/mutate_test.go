package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Disble/dharness/internal/tiers"
	"github.com/Disble/dharness/internal/tool"
)

// declareTiers writes the consumer-owned coverage-tier manifest that decides
// which of a repository's sources count as runtime code worth mutating.
//
// It is a real file in a real temporary tree rather than a fake, because
// Resolve reads the tree: a manifest whose adopted prefixes do not exist is an
// error, not a classification. §05 — the classification is the consumer's
// decision, so the test supplies it the way a consumer would.
func declareTiers(t *testing.T, root, manifest string) {
	t.Helper()
	writeFile(t, filepath.Join(root, tiers.ManifestName), manifest)
}

// checkoutManifest is the fixture policy the mutation filter is measured
// against: a core money path, an important UI directory, a runtime-free types
// file declared infrastructure, and one unadopted path outside every enforced
// prefix. The three tiers and the unadopted case all appear at once so one
// scenario exercises the whole decision instead of four near-copies.
const checkoutManifest = `{
  "version": 1,
  "enforcedPrefixes": ["src/features/checkout/"],
  "features": [
    {
      "id": "checkout",
      "path": "src/features/checkout/",
      "defaultTier": "core",
      "reason": "money path",
      "overrides": [
        {"path": "src/features/checkout/ui/", "tier": "important", "reason": "rendering, not arithmetic"},
        {"path": "src/features/checkout/types.ts", "tier": "infrastructure", "reason": "no runtime code to mutate"}
      ]
    }
  ]
}`

// adoptedCheckoutTree materialises the sources the manifest declares, because
// Resolve derives the adopted universe from the tree and refuses a declaration
// whose sources are missing or unclassified.
func adoptedCheckoutTree(t *testing.T, root string) {
	t.Helper()
	for _, path := range []string{
		"src/features/checkout/calculate-total.ts",
		"src/features/checkout/types.ts",
		"src/features/checkout/ui/Checkout.tsx",
	} {
		writeFile(t, filepath.Join(root, filepath.FromSlash(path)), "")
	}
}

// TestMutateSkipsDeclaredInfrastructureAndForwardsTheRestVerbatim is the
// filter's owning scenario: the mixed case, which is the only one where the
// decision has to be per scope rather than per run.
//
// The adopted core path keeps its range — columns included, because rebuilding
// `:1:3-1:5` from the parsed numbers would widen it back to `:1-1` (§01) — the
// important path stays, the unadopted path stays, and only the file the
// consumer declared infrastructure disappears from `--mutate`.
func TestMutateSkipsDeclaredInfrastructureAndForwardsTheRestVerbatim(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)
	declareTiers(t, root, checkoutManifest)
	adoptedCheckoutTree(t, root)
	stubWritesReportOnRun(captured, root, `{"files":{
		"src/features/checkout/calculate-total.ts":{"mutants":[
			{"status":"Killed","mutatorName":"ArithmeticOperator","location":{"start":{"line":1}}}]},
		"src/features/checkout/ui/Checkout.tsx":{"mutants":[
			{"status":"Killed","mutatorName":"BooleanLiteral","location":{"start":{"line":2}}}]},
		"src/legacy/old.ts":{"mutants":[
			{"status":"Killed","mutatorName":"EqualityOperator","location":{"start":{"line":3}}}]}}}`)

	var out bytes.Buffer
	err := RunMutate([]string{
		"src/features/checkout/calculate-total.ts:1:3-1:5",
		"src/features/checkout/ui/Checkout.tsx",
		"src/features/checkout/types.ts",
		"src/legacy/old.ts",
	}, &out)
	if err != nil {
		t.Fatalf("RunMutate() = %v", err)
	}

	args := strykerArgs(t, captured)
	want := "--mutate src/features/checkout/calculate-total.ts:1:3-1:5,src/features/checkout/ui/Checkout.tsx,src/legacy/old.ts"
	if !strings.Contains(args, want) {
		t.Errorf("stryker was invoked without %q:\n%s", want, args)
	}
	if strings.Contains(args, "types.ts") {
		t.Errorf("a file declared infrastructure still reached --mutate:\n%s", args)
	}
	if got := out.String(); !strings.Contains(got, "Every mutant was caught") {
		t.Errorf("the eligible paths did not keep their verdict:\n%s", got)
	}
}

// TestMutateRunsNothingWhenEveryNamedPathIsInfrastructure pins the all-filtered
// case as a computed no-op rather than a failure or an invocation.
//
// §13: the cheapest way to run something is not to run it. An empty intersection
// after the consumer's own exclusions means no install, no sandbox, no cache
// path and no Stryker process — the run says why in one line and exits 0. A
// dry run is the same answer: there is nothing whose test count could be
// measured.
func TestMutateRunsNothingWhenEveryNamedPathIsInfrastructure(t *testing.T) {
	for _, testCase := range []struct {
		name string
		args []string
	}{
		{name: "plain run", args: []string{"src/features/checkout/types.ts"}},
		{name: "dry run", args: []string{"--dry-run", "src/features/checkout/types.ts"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			captured, root := stub(t, "")
			mutable(t, root)
			declareTiers(t, root, checkoutManifest)
			adoptedCheckoutTree(t, root)

			var out bytes.Buffer
			if err := RunMutate(testCase.args, &out); err != nil {
				t.Fatalf("RunMutate(%v) = %v, want nil: an excluded scope is a computed no-op, not a failure", testCase.args, err)
			}

			if len(captured.commands) != 0 {
				t.Errorf("invoked %+v, want no command at all: nothing was left to mutate", captured.commands)
			}
			if got := out.String(); !strings.Contains(got, "no runtime source to mutate after coverage-tier exclusions") {
				t.Errorf("the run did not say why nothing happened:\n%s", got)
			}
			for _, path := range []string{
				filepath.Join(root, ".dharness"),
				filepath.Join(root, "reports", "mutation", "mutation.json"),
			} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("%s exists, so the no-op still paid for install, sandbox or cache IO", path)
				}
			}
		})
	}
}

// TestMutateFailsClosedOnAMalformedPolicy covers the other half of "no policy
// is not the same as no answer": a declaration that cannot be parsed must
// refuse the run, never fall through to mutating everything.
//
// Fail-closed is the only defensible direction here. Treating an unreadable
// policy as "no exclusions" would mutate infrastructure the consumer excluded
// and report it as this run's verdict; refusing costs one message naming the
// file to fix.
func TestMutateFailsClosedOnAMalformedPolicy(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)
	declareTiers(t, root, `{"version":1,"enforcedPrefixes":["src/"],
		"features":[{"id":"app","path":"src/","defaultTier":"critical","reason":"unknown tier"}]}`)
	writeFile(t, filepath.Join(root, "src", "a.ts"), "")

	err := RunMutate([]string{"src/a.ts"}, io.Discard)

	if err == nil {
		t.Fatal("RunMutate() = nil, want the declaration error: an unreadable policy must not mutate unclassified code")
	}
	if !strings.Contains(err.Error(), tiers.ManifestName) {
		t.Errorf("error does not name the declaration to fix: %v", err)
	}
	if len(captured.commands) != 0 {
		t.Errorf("invoked %+v, want no command after a refused policy", captured.commands)
	}
}

// TestMutateStillFailsOnEligibleSurvivorsAndNoCoverageMutants guards the
// verdict the filter must not touch. A core path is eligible, so both ways a
// mutant goes uncaught — a test that ran the line and missed it, and a line no
// test ever ran — keep failing the run and keep being reported apart.
func TestMutateStillFailsOnEligibleSurvivorsAndNoCoverageMutants(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)
	declareTiers(t, root, checkoutManifest)
	adoptedCheckoutTree(t, root)
	stubWritesReportOnRun(captured, root, `{"files":{"src/features/checkout/calculate-total.ts":{"mutants":[
		{"status":"Survived","mutatorName":"EqualityOperator","location":{"start":{"line":4}}},
		{"status":"NoCoverage","mutatorName":"StringLiteral","location":{"start":{"line":9}}}]}}}`)

	var out bytes.Buffer
	err := RunMutate([]string{"src/features/checkout/calculate-total.ts"}, &out)

	var survivors *SurvivorsError
	if !errors.As(err, &survivors) {
		t.Fatalf("RunMutate() = %v, want SurvivorsError: an eligible path keeps its verdict", err)
	}
	if len(survivors.Survivors) != 2 {
		t.Fatalf("reported %d uncaught mutants, want the survived and the never-covered one: %+v", len(survivors.Survivors), survivors.Survivors)
	}
	got := err.Error()
	if !strings.Contains(got, "1 survived") || !strings.Contains(got, "1 never ran under a test") {
		t.Errorf("verdict no longer separates the two ways a mutant goes uncaught: %s", got)
	}
}

// TestFilterInfrastructureScopesRefusesAScopeItCannotProject checks the one
// branch the public seam cannot reach: scopePaths has already proved every
// named path sits inside the JS project, so the command in front of it never
// hands this helper a root and a source that cannot be related.
//
// The branch still decides a real outcome rather than decorating the code —
// refuse the run, never fall through to mutating a path the policy was never
// asked about — so it is exercised directly, where the public scheduler cannot
// go, instead of being left to survive unproven.
func TestFilterInfrastructureScopesRefusesAScopeItCannotProject(t *testing.T) {
	absolute := t.TempDir()

	_, err := filterInfrastructureScopes("relative-root", absolute, []tool.MutationScope{tool.ParseMutationScope("src/a.ts:1:3-1:5")})

	if err == nil {
		t.Fatal("filterInfrastructureScopes() = nil, want a refusal: a scope the policy cannot be asked about must not be mutated by default")
	}
	if !strings.Contains(err.Error(), "src/a.ts") {
		t.Errorf("refusal does not name the scope it could not project: %v", err)
	}
}

// TestMutateKeepsEveryPathWhenThereIsNoPolicy is the unadopted case: a
// repository that never declared a classification mutates exactly what it
// named, ranges included, as it did before this filter existed.
func TestMutateKeepsEveryPathWhenThereIsNoPolicy(t *testing.T) {
	captured, root := stub(t, "")
	mutable(t, root)
	stubWritesReportOnRun(captured, root, `{"files":{
		"src/a.ts":{"mutants":[{"status":"Killed","mutatorName":"BooleanLiteral","location":{"start":{"line":1}}}]},
		"src/b.js":{"mutants":[{"status":"Killed","mutatorName":"StringLiteral","location":{"start":{"line":6}}}]}}}`)

	if err := RunMutate([]string{"src/a.ts", "src/b.js:5-7"}, io.Discard); err != nil {
		t.Fatalf("RunMutate() = %v", err)
	}

	args := strykerArgs(t, captured)
	want := "--mutate src/a.ts,src/b.js:5-7"
	if !strings.Contains(args, want) {
		t.Errorf("a repository with no declaration lost paths it used to mutate: got %s, want %q", args, want)
	}
}
