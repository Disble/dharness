package staged

// Slice D (mutate-staged-v1.9): related-test result parsing. RED first —
// these tests fail until internal/staged/related.go exists.

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestParseVitestRelatedRequiresANonNegativeCount(t *testing.T) {
	if n, err := ParseVitestRelated([]byte(`{"numTotalTests":3,"numTotalTestSuites":1}`)); err != nil || n != 3 {
		t.Errorf("ParseVitestRelated(valid) = %d, %v; want 3, nil", n, err)
	}
	if n, err := ParseVitestRelated([]byte(`{"numTotalTests":0}`)); err != nil || n != 0 {
		t.Errorf("ParseVitestRelated(zero) = %d, %v; want 0, nil", n, err)
	}
	for name, data := range map[string]string{
		"missing field":  `{"numPassedTests":3}`,
		"wrong type":     `{"numTotalTests":"3"}`,
		"negative":       `{"numTotalTests":-1}`,
		"fractional":     `{"numTotalTests":2.5}`,
		"malformed":      `{"numTotalTests":`,
		"empty":          ``,
		"wrong envelope": `[1,2,3]`,
	} {
		if n, err := ParseVitestRelated([]byte(data)); err == nil {
			t.Errorf("%s: ParseVitestRelated() = %d, nil; want an error", name, n)
		}
	}
}

func TestParseJestRelatedCountsTheListedArray(t *testing.T) {
	if n, err := ParseJestRelated([]byte(`["src/a.test.js","src/b.test.js"]`)); err != nil || n != 2 {
		t.Errorf("ParseJestRelated(two) = %d, %v; want 2, nil", n, err)
	}
	if n, err := ParseJestRelated([]byte(`[]`)); err != nil || n != 0 {
		t.Errorf("ParseJestRelated(empty) = %d, %v; want 0, nil", n, err)
	}
	for name, data := range map[string]string{
		"object":    `{"tests":[]}`,
		"string":    `"src/a.test.js"`,
		"number":    `3`,
		"malformed": `["a",`,
		"empty":     ``,
	} {
		if n, err := ParseJestRelated([]byte(data)); err == nil {
			t.Errorf("%s: ParseJestRelated() = %d, nil; want an error", name, n)
		}
	}
}

func TestRelatedOutcomeLinesAreExact(t *testing.T) {
	if got := NoTestReachesLine([]string{"src/a.ts", "src/b.ts"}); got != "no test reaches: src/a.ts, src/b.ts; the fix is a test that imports them" {
		t.Errorf("NoTestReachesLine() = %q", got)
	}
	if err := RelatedSuiteFailure(errors.New("exit 1")); err == nil || err.Error() != "related-test suite load/run failure: exit 1" {
		t.Errorf("RelatedSuiteFailure() = %v", err)
	}
	if err := RelatedJSONFailure("unexpected end"); err == nil || !strings.HasPrefix(err.Error(), "related-test JSON failure: ") {
		t.Errorf("RelatedJSONFailure() = %v", err)
	}
}

// TestParseVitestFailuresNamesEachFailedTest pins what a failing related run
// leaves behind: vitest's stderr is empty, so the JSON report is the only
// place the failed test and its reason exist.
func TestParseVitestFailuresNamesEachFailedTest(t *testing.T) {
	report := `{"numTotalTests":3,"testResults":[` +
		`{"name":"src/a.test.ts","assertionResults":[` +
		`{"fullName":"passes","status":"passed","failureMessages":[]},` +
		`{"fullName":"slow under load","status":"failed","failureMessages":["Error: Test timed out in 50ms.\nIf this is a long-running test, pass a timeout value.","second message"]}]},` +
		`{"name":"src/b.test.ts","assertionResults":[` +
		`{"fullName":"no message","status":"failed","failureMessages":[]},` +
		`{"fullName":"crlf","status":"failed","failureMessages":["Error: boom\r\n    at run (src/b.test.ts:3:9)"]}]}]}`

	got, err := ParseVitestFailures([]byte(report))
	if err != nil {
		t.Fatalf("ParseVitestFailures() error = %v", err)
	}
	want := []FailedTest{
		{Name: "slow under load", Message: "Error: Test timed out in 50ms."},
		{Name: "no message", Message: ""},
		{Name: "crlf", Message: "Error: boom"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("ParseVitestFailures() = %+v, want %+v", got, want)
	}
}

func TestParseVitestFailuresFindsNoneInACleanReport(t *testing.T) {
	for name, data := range map[string]string{
		"all passed":      `{"testResults":[{"assertionResults":[{"fullName":"a","status":"passed"}]}]}`,
		"no test results": `{"numTotalTests":0}`,
	} {
		if got, err := ParseVitestFailures([]byte(data)); err != nil || len(got) != 0 {
			t.Errorf("%s: ParseVitestFailures() = %+v, %v; want none, nil", name, got, err)
		}
	}
}

func TestParseVitestFailuresRejectsAnUnreadableReport(t *testing.T) {
	for name, data := range map[string]string{
		"malformed":      `{"testResults":[`,
		"empty":          ``,
		"wrong envelope": `[1,2,3]`,
	} {
		if got, err := ParseVitestFailures([]byte(data)); err == nil {
			t.Errorf("%s: ParseVitestFailures() = %+v, nil; want an error", name, got)
		}
	}
}

// TestRelatedTestsFailedNamesTheTestsAndKeepsTheExit pins the bounded line:
// a few failed tests by name and first failure line, the rest counted, and
// the exit kept both in the text and through errors.As.
func TestRelatedTestsFailedNamesTheTestsAndKeepsTheExit(t *testing.T) {
	exit := errors.New("vitest exited with code 1")

	one := RelatedTestsFailed([]FailedTest{{Name: "slow under load", Message: "Error: Test timed out in 50ms."}}, exit)
	if got, want := one.Error(), `related tests failed: "slow under load": Error: Test timed out in 50ms. (vitest exited with code 1)`; got != want {
		t.Errorf("RelatedTestsFailed(one) = %q, want %q", got, want)
	}
	if !errors.Is(one, exit) {
		t.Errorf("RelatedTestsFailed() does not wrap the exit error")
	}

	five := RelatedTestsFailed([]FailedTest{{Name: "a", Message: "m1"}, {Name: "b"}, {Name: "c", Message: "m3"}, {Name: "d"}, {Name: "e"}}, exit)
	if got, want := five.Error(), `related tests failed: "a": m1; "b"; "c": m3; and 2 more (vitest exited with code 1)`; got != want {
		t.Errorf("RelatedTestsFailed(five) = %q, want %q", got, want)
	}
}

// TestRelatedTestsFailedCountsOnlyTheTestsItLeavesOut pins the boundary: at
// exactly the bound every test is named and nothing is counted.
func TestRelatedTestsFailedCountsOnlyTheTestsItLeavesOut(t *testing.T) {
	three := RelatedTestsFailed([]FailedTest{{Name: "a"}, {Name: "b"}, {Name: "c"}}, errors.New("vitest exited with code 1"))
	if got, want := three.Error(), `related tests failed: "a"; "b"; "c" (vitest exited with code 1)`; got != want {
		t.Errorf("RelatedTestsFailed(three) = %q, want %q", got, want)
	}
	four := RelatedTestsFailed([]FailedTest{{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}}, errors.New("vitest exited with code 1"))
	if got, want := four.Error(), `related tests failed: "a"; "b"; "c"; and 1 more (vitest exited with code 1)`; got != want {
		t.Errorf("RelatedTestsFailed(four) = %q, want %q", got, want)
	}
}
