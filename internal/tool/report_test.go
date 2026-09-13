package tool

import (
	"strconv"
	"strings"
	"testing"
)

func newReader(s string) *strings.Reader { return strings.NewReader(s) }

// A timeout is a detection: the mutant hung the suite, which is exactly what
// mutation testing exists to notice. NoCoverage is the opposite failure — no
// test ever exercised the mutated line at all — and it now counts the same
// as Survived. Measured on dharness 1.7.6: a range with 11 Killed
// plus 8 NoCoverage mutants (an exported function no test called) exited 0
// and printed "Every mutant was caught: these tests notice this code
// breaking." Counting Timeout as a survivor would fail a commit for
// something the tests did notice; leaving NoCoverage out reports a pass for
// code no test ever ran.
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
	want := []string{
		"src/a.ts:3 StringLiteral (no test ran it)",
		"src/a.ts:9 ConditionalExpression",
		"src/b.ts:4 BooleanLiteral",
	}
	if len(survivors) != len(want) {
		t.Fatalf("Survivors() = %v, want %v", survivors, want)
	}
	for i, expected := range want {
		if survivors[i].String() != expected {
			t.Errorf("survivor %d = %q, want %q", i, survivors[i], expected)
		}
	}
}

// TestSurvivorStringMarksNoCoverage pins the reader-facing half: a mutant no
// test ever ran reads differently from one a test ran and missed, so the
// suffix names which failure this is.
func TestSurvivorStringMarksNoCoverage(t *testing.T) {
	s := Survivor{File: "src/a.ts", Line: 3, Description: "StringLiteral", Status: "NoCoverage"}
	if got := s.String(); got != "src/a.ts:3 StringLiteral (no test ran it)" {
		t.Errorf("String() = %q, want the no-test-ran-it suffix", got)
	}
}

// TestIgnoredStringNamesTheReason pins the reader-facing form. Measured:
// `// Stryker disable next-line all: reason` produces
// "status":"Ignored","statusReason":"reason" in the JSON report, and
// Stryker's own clear-text reporter never prints either — an author who
// marked a mutant equivalent otherwise has no way to see dharness agrees.
func TestIgnoredStringNamesTheReason(t *testing.T) {
	i := Ignored{File: "src/a.ts", Line: 5, Description: "ArrayDeclaration", Reason: "equivalent: order does not matter here"}
	want := "src/a.ts:5 ArrayDeclaration — equivalent: order does not matter here"
	if got := i.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// A report is not obligated to carry a reason.
func TestIgnoredStringWithNoReason(t *testing.T) {
	i := Ignored{File: "src/a.ts", Line: 5, Description: "ArrayDeclaration"}
	if got := i.String(); got != "src/a.ts:5 ArrayDeclaration" {
		t.Errorf("String() = %q, want no dash when there is no reason", got)
	}
}

func TestSurvivorsRejectsAReportItCannotRead(t *testing.T) {
	if _, err := Survivors(newReader("not json")); err == nil {
		t.Fatal("Survivors() = nil error on a malformed report; silence would read as a pass")
	}
}

// TestFileMutantCountsCountsRegardlessOfStatus pins the self-check's own
// question, which is not the verdict's: a staged mutate run's classifier
// disagreement check asks whether a file believed to compile to nothing
// carries ANY mutant at all, killed ones included — Survivors would report
// zero for a file whose only mutant was Killed, and that silence is exactly
// what would let a wrong classification through unnoticed.
func TestFileMutantCountsCountsRegardlessOfStatus(t *testing.T) {
	report := `{"files":{
		"src/a.ts":{"mutants":[
			{"status":"Killed","mutatorName":"EqualityOperator","location":{"start":{"line":1}}},
			{"status":"Survived","mutatorName":"BooleanLiteral","location":{"start":{"line":2}}}
		]},
		"src/types.ts":{"mutants":[]}
	}}`

	counts, err := FileMutantCounts(newReader(report))
	if err != nil {
		t.Fatalf("FileMutantCounts() = %v", err)
	}
	if counts["src/a.ts"] != 2 {
		t.Errorf(`counts["src/a.ts"] = %d, want 2`, counts["src/a.ts"])
	}
	if counts["src/types.ts"] != 0 {
		t.Errorf(`counts["src/types.ts"] = %d, want 0`, counts["src/types.ts"])
	}
}

// TestFileMutantCountsIsEmptyForAFileTheReportNeverNames pins the missing
// case directly: a lookup by zero value must not be mistaken for "present
// with none", so a caller doing that lookup gets the same zero either way —
// which is fine for the self-check specifically, since both mean "no
// disagreement here", but must not panic or misbehave on an absent key.
func TestFileMutantCountsIsEmptyForAFileTheReportNeverNames(t *testing.T) {
	counts, err := FileMutantCounts(newReader(`{"files":{}}`))
	if err != nil {
		t.Fatalf("FileMutantCounts() = %v", err)
	}
	if counts["src/never-named.ts"] != 0 {
		t.Errorf("counts for an absent file = %d, want 0", counts["src/never-named.ts"])
	}
}

func TestFileMutantCountsRejectsAReportItCannotRead(t *testing.T) {
	if _, err := FileMutantCounts(newReader("not json")); err == nil {
		t.Fatal("FileMutantCounts() = nil error on a malformed report; silence would read as no disagreement")
	}
}

// TestMutantTallyInScopeCountsEveryStatusOnlyInsideTheRanges pins the numbers
// a staged verdict prints. Each status lands in its own count, both error
// statuses in one, mutants before and after the range and a file outside the
// scope count nowhere, and the total is what the five kinds and the errors add up
// to — so a run that generated nothing reads as zero, not as a pass.
func TestMutantTallyInScopeCountsEveryStatusOnlyInsideTheRanges(t *testing.T) {
	mutant := func(status string, line int) string {
		return `{"status":"` + status + `","mutatorName":"M","location":{"start":{"line":` + strconv.Itoa(line) + `}}}`
	}
	report := `{"files":{
		"src/a.ts":{"mutants":[` + strings.Join([]string{
		mutant("Survived", 1), mutant("Killed", 2), mutant("Killed", 3), mutant("Survived", 3), mutant("NoCoverage", 4),
		mutant("Timeout", 4), mutant("CompileError", 5), mutant("RuntimeError", 5), mutant("Ignored", 5),
		mutant("Killed", 9),
	}, ",") + `]},
		"src/b.ts":{"mutants":[` + mutant("Survived", 2) + `]}
	}}`

	tally, err := MutantTallyInScope(newReader(report), []MutationScope{ParseMutationScope("src/a.ts:2-5")})
	if err != nil {
		t.Fatalf("MutantTallyInScope() = %v", err)
	}

	want := MutantTally{Killed: 2, Survived: 1, NoCoverage: 1, Timeout: 1, Errors: 2, Ignored: 1}
	if tally != want {
		t.Errorf("MutantTallyInScope() = %+v, want %+v", tally, want)
	}
	if tally.Total() != 8 {
		t.Errorf("Total() = %d, want 8", tally.Total())
	}
}

func TestMutantTallyInScopeRejectsAReportItCannotRead(t *testing.T) {
	if _, err := MutantTallyInScope(newReader("not json"), nil); err == nil {
		t.Fatal("MutantTallyInScope() = nil error on a malformed report; zero would read as a run that generated nothing")
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

// TestSurvivorsCarryWhatTheMutantBecame is M2: a mutator name and a line
// number do not say what changed.
//
// From the field report: "MethodExpression at line 59 does not say which
// method, or what it became. Line 59 in that file was a five-line chained
// expression." Working out each mutant meant parsing the JSON by hand — the
// same JSON dharness had just read to reach its verdict.
//
// Measured on a bun/vitest fixture: 4 of 4 survivors carried a non-empty
// `replacement` (`score > 90`, `true`, `n >= 0`). The schema marks it
// optional, so an absent one falls back to the bare description rather than
// printing an arrow pointing at nothing.
func TestSurvivorsCarryWhatTheMutantBecame(t *testing.T) {
	report := `{"files":{"src/a.ts":{"mutants":[
		{"status":"Survived","mutatorName":"EqualityOperator","replacement":"n >= 0","location":{"start":{"line":5}}},
		{"status":"Survived","mutatorName":"MethodExpression","location":{"start":{"line":9}}}
	]}}}`

	survivors, err := Survivors(strings.NewReader(report))
	if err != nil {
		t.Fatalf("Survivors() = %v", err)
	}
	if len(survivors) != 2 {
		t.Fatalf("got %d survivors, want 2", len(survivors))
	}
	if got := survivors[0].String(); got != "src/a.ts:5 EqualityOperator → n >= 0" {
		t.Errorf("Survivor.String() = %q, want it to show what the mutant became", got)
	}
	if got := survivors[1].String(); got != "src/a.ts:9 MethodExpression" {
		t.Errorf("Survivor.String() = %q, want no arrow when the report carries no replacement", got)
	}
}

// TestSurvivorReplacementIsCollapsedToOneLine keeps the survivor list one
// entry per line. A mutated multi-line expression carries its newlines into
// `replacement`, and a list that reflows is a list nobody can scan.
func TestSurvivorReplacementIsCollapsedToOneLine(t *testing.T) {
	report := `{"files":{"src/a.ts":{"mutants":[
		{"status":"Survived","mutatorName":"BlockStatement","replacement":"{\n  return 1;\n}","location":{"start":{"line":3}}}
	]}}}`

	survivors, err := Survivors(strings.NewReader(report))
	if err != nil {
		t.Fatalf("Survivors() = %v", err)
	}
	if got := survivors[0].String(); strings.Contains(got, "\n") {
		t.Errorf("Survivor.String() = %q, want a single line", got)
	}
}
