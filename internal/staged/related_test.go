package staged

// Slice D (mutate-staged-v1.9): related-test result parsing. RED first —
// these tests fail until internal/staged/related.go exists.

import (
	"errors"
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
