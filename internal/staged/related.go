package staged

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Slice D (mutate-staged-v1.9): related-test result parsing and outcome lines.
//
// Exactly one aggregate command runs per mutation gate, and its structured
// result is the whole reachability story: a zero aggregate soundly names
// every retained file unreachable, while a positive one proves only that at
// least one related test exists for the set — never which file it reaches.
// The per-file attribution the gate does not have is stated, not smuggled.

// ParseVitestRelated reads the aggregate count out of a Vitest JSON report.
// It requires a present, integer, non-negative numTotalTests: anything else
// is a JSON failure, never a zero.
func ParseVitestRelated(data []byte) (int, error) {
	var report struct {
		NumTotalTests *int `json:"numTotalTests"`
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	if err := dec.Decode(&report); err != nil {
		return 0, fmt.Errorf("read the Vitest related-test JSON: %w", err)
	}
	if report.NumTotalTests == nil {
		return 0, errors.New("Vitest related-test JSON has no numTotalTests")
	}
	if *report.NumTotalTests < 0 {
		return 0, fmt.Errorf("Vitest related-test JSON counts %d tests", *report.NumTotalTests)
	}
	return *report.NumTotalTests, nil
}

// FailedTest is one failed test a related run reported: its full name and
// the first line of its first failure message, empty when it gave none.
type FailedTest struct {
	Name    string
	Message string
}

// ParseVitestFailures reads the failed tests out of a Vitest JSON report.
// A failing related run writes nothing to stderr, so the report is the only
// place that says which test failed and why. A report it cannot read is an
// error; a readable one that names no failed test returns none, and the
// caller decides what a non-zero exit without a named failure means.
func ParseVitestFailures(data []byte) ([]FailedTest, error) {
	var report struct {
		TestResults []struct {
			AssertionResults []struct {
				FullName        string   `json:"fullName"`
				Status          string   `json:"status"`
				FailureMessages []string `json:"failureMessages"`
			} `json:"assertionResults"`
		} `json:"testResults"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("read the Vitest related-test JSON: %w", err)
	}
	var failed []FailedTest
	for _, file := range report.TestResults {
		for _, assertion := range file.AssertionResults {
			if assertion.Status != "failed" {
				continue
			}
			test := FailedTest{Name: assertion.FullName}
			if len(assertion.FailureMessages) > 0 {
				// The first line carries the reason ("Error: Test timed out
				// in 50ms."); the rest is advice and stack, too long for a
				// one-line phase result.
				test.Message, _, _ = strings.Cut(assertion.FailureMessages[0], "\n")
				test.Message = strings.TrimSpace(test.Message)
			}
			failed = append(failed, test)
		}
	}
	return failed, nil
}

// maxNamedFailures bounds the failed-test line: enough to act on the first
// failures, while a suite that fails wholesale still reads as one line.
const maxNamedFailures = 3

// RelatedTestsFailed reports a related-test run that ran and failed: it names
// each failed test with its first failure line, up to maxNamedFailures, and
// counts the rest. Unlike RelatedSuiteFailure the suite did produce a result,
// so the line says what failed rather than that nothing ran. It wraps the
// exit so an interrupt stays visible through errors.As, and states it so the
// exit code is never lost.
func RelatedTestsFailed(failed []FailedTest, exit error) error {
	shown := failed[:min(len(failed), maxNamedFailures)]
	var named []string
	for _, test := range shown {
		entry := fmt.Sprintf("%q", test.Name)
		if test.Message != "" {
			entry += ": " + test.Message
		}
		named = append(named, entry)
	}
	if more := len(failed) - len(shown); more > 0 {
		named = append(named, fmt.Sprintf("and %d more", more))
	}
	return fmt.Errorf("related tests failed: %s (%w)", strings.Join(named, "; "), exit)
}

// ParseJestRelated counts the list-only JSON array. An empty array is zero
// related tests; anything that is not an array is a JSON failure. Listing
// proves discoverability, not that the tests load or pass.
func ParseJestRelated(data []byte) (int, error) {
	// A null body must not read as zero tests found, and an empty array
	// unmarshals to a nil slice too — so null is rejected up front and
	// length alone decides afterwards.
	if string(bytes.TrimSpace(data)) == "null" {
		return 0, errors.New("Jest related-test JSON is null, not an array")
	}
	var listed []json.RawMessage
	if err := json.Unmarshal(data, &listed); err != nil {
		return 0, fmt.Errorf("read the Jest related-test JSON: %w", err)
	}
	return len(listed), nil
}

// RelatedSuiteFailure reports a related-test command that did not exit clean:
// the suite failed to load or run, so reachability is unknown, not zero. It
// wraps the cause so an interrupt the child died of stays visible through
// errors.As, exactly like every other step error the run checks.
func RelatedSuiteFailure(reason error) error {
	return fmt.Errorf("related-test suite load/run failure: %w", reason)
}

// RelatedJSONFailure reports structured output that cannot be read.
func RelatedJSONFailure(reason string) error {
	return errors.New("related-test JSON failure: " + reason)
}

// NoTestReachesLine is the exact zero-aggregate outcome: it names every
// retained file because a zero aggregate soundly covers all of them, and it
// names the fix because the gate refuses rather than guesses.
func NoTestReachesLine(files []string) string {
	return fmt.Sprintf("no test reaches: %s; the fix is a test that imports them", strings.Join(files, ", "))
}
