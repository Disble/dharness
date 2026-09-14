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
