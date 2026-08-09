package tool

import (
	"strings"
	"testing"
)

func newReader(s string) *strings.Reader { return strings.NewReader(s) }

// A timeout is a detection and an uncovered mutant is a coverage gap. Counting
// either as a survivor would fail a commit for something the tests did notice,
// or for something they were never given a chance to.
func TestSurvivorsCountsOnlyMutantsTheTestsMissed(t *testing.T) {
	report := `{"files":{"src/b.ts":{"mutants":[
		{"status":"Survived","mutatorName":"BooleanLiteral","location":{"start":{"line":4}}}
	]},"src/a.ts":{"mutants":[
		{"status":"Killed","mutatorName":"EqualityOperator","location":{"start":{"line":1}}},
		{"status":"Timeout","mutatorName":"ArithmeticOperator","location":{"start":{"line":2}}},
		{"status":"NoCoverage","mutatorName":"StringLiteral","location":{"start":{"line":3}}},
		{"status":"Survived","mutatorName":"ConditionalExpression","location":{"start":{"line":9}}}
	]}}}`

	survivors, err := Survivors(newReader(report))
	if err != nil {
		t.Fatalf("Survivors() = %v", err)
	}

	// Sorted, so a run over several files reads the same way twice.
	want := []string{"src/a.ts:9 ConditionalExpression", "src/b.ts:4 BooleanLiteral"}
	if len(survivors) != len(want) {
		t.Fatalf("Survivors() = %v, want %v", survivors, want)
	}
	for i, expected := range want {
		if survivors[i].String() != expected {
			t.Errorf("survivor %d = %q, want %q", i, survivors[i], expected)
		}
	}
}

func TestSurvivorsRejectsAReportItCannotRead(t *testing.T) {
	if _, err := Survivors(newReader("not json")); err == nil {
		t.Fatal("Survivors() = nil error on a malformed report; silence would read as a pass")
	}
}

// The count is read from prose because --dryRunOnly writes no report. Matching
// a bare number would pick up any sentence that happens to contain one.
func TestRelatedTestsReadsTheInitialRunAndNothingElse(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   int
		fails  bool
	}{
		{"real line", "INFO DryRunExecutor Initial test run succeeded. Ran 42 tests in 3 seconds.", 42, false},
		{"one test", "Initial test run succeeded. Ran 1 test in 0 seconds.", 1, false},
		{"another sentence with a number", "Ran 1.13 tests per mutant on average.", 0, true},
		{"nothing ran", "", 0, true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := RelatedTests(testCase.output)
			if testCase.fails {
				if err == nil {
					t.Fatalf("RelatedTests() = %d, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("RelatedTests() = %v", err)
			}
			if got != testCase.want {
				t.Errorf("RelatedTests() = %d, want %d", got, testCase.want)
			}
		})
	}
}
