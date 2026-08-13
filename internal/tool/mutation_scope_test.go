package tool

import (
	"slices"
	"strings"
	"testing"
)

func TestParseMutationScopeSeparatesTheRangeFromThePath(t *testing.T) {
	cases := []struct {
		arg   string
		want  MutationScope
		flat  string
		about string
	}{
		{"src/a.ts", MutationScope{Path: "src/a.ts"}, "src/a.ts", "a bare path covers the whole file"},
		{"src/a.ts:5-7", MutationScope{Path: "src/a.ts", Start: 5, End: 7}, "src/a.ts:5-7", "line range"},
		{"src/index.js:1:3-1:5", MutationScope{Path: "src/index.js", Start: 1, End: 1}, "src/index.js:1:3-1:5", "columns narrow the argument, not the judgement"},
		{`C:\project\src\a.ts`, MutationScope{Path: `C:\project\src\a.ts`}, `C:\project\src\a.ts`, "a Windows drive letter is not a range"},
	}

	for _, testCase := range cases {
		t.Run(testCase.about, func(t *testing.T) {
			got := ParseMutationScope(testCase.arg)

			if got.Path != testCase.want.Path || got.Start != testCase.want.Start || got.End != testCase.want.End {
				t.Errorf("ParseMutationScope(%q) = %+v, want %+v", testCase.arg, got, testCase.want)
			}
			if got.Argument() != testCase.flat {
				t.Errorf("Argument() = %q, want %q", got.Argument(), testCase.flat)
			}
		})
	}
}

// The report Stryker writes is cumulative: --incremental carries results for
// files and lines this run never asked about. Measured on 2026-08-13 against a
// real run — `dharness mutate src/a.ts:5-7` instrumented 5 mutants and then
// failed on a survivor at line 10, from the previous whole-file run.
func TestSurvivorsInScopeIgnoresWhatTheRunDidNotAskFor(t *testing.T) {
	report := `{"files":{
		"src/a.ts":{"mutants":[
			{"status":"Survived","mutatorName":"EqualityOperator","location":{"start":{"line":6}}},
			{"status":"Survived","mutatorName":"EqualityOperator","location":{"start":{"line":10}}},
			{"status":"Killed","mutatorName":"BooleanLiteral","location":{"start":{"line":6}}}
		]},
		"src/b.ts":{"mutants":[
			{"status":"Survived","mutatorName":"ArithmeticOperator","location":{"start":{"line":2}}}
		]}
	}}`

	cases := []struct {
		about  string
		scopes []MutationScope
		want   []string
	}{
		{
			about:  "a line range judges only its own lines",
			scopes: []MutationScope{{Path: "src/a.ts", Start: 5, End: 7}},
			want:   []string{"src/a.ts:6 EqualityOperator"},
		},
		{
			about:  "a bare path judges the whole file and nothing else",
			scopes: []MutationScope{{Path: "src/a.ts"}},
			want:   []string{"src/a.ts:6 EqualityOperator", "src/a.ts:10 EqualityOperator"},
		},
		{
			about:  "several arguments judge the union",
			scopes: []MutationScope{{Path: "src/a.ts", Start: 9, End: 14}, {Path: "src/b.ts"}},
			want:   []string{"src/a.ts:10 EqualityOperator", "src/b.ts:2 ArithmeticOperator"},
		},
		{
			about:  "a scope with no survivors of its own passes",
			scopes: []MutationScope{{Path: "src/a.ts", Start: 1, End: 3}},
			want:   nil,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.about, func(t *testing.T) {
			survivors, err := SurvivorsInScope(strings.NewReader(report), testCase.scopes)
			if err != nil {
				t.Fatalf("SurvivorsInScope() = %v", err)
			}

			got := make([]string, 0, len(survivors))
			for _, survivor := range survivors {
				got = append(got, survivor.String())
			}
			if !slices.Equal(got, testCase.want) {
				t.Errorf("SurvivorsInScope() = %v, want %v", got, testCase.want)
			}
		})
	}
}

// The edges are the whole point of a range, and the interior cases above do not
// reach them: a survivor exactly on the first or last line is in scope, and one
// a single line outside is not.
func TestSurvivorsInScopeIncludesBothEndsAndNothingBeyond(t *testing.T) {
	report := `{"files":{"src/a.ts":{"mutants":[
		{"status":"Survived","mutatorName":"M4","location":{"start":{"line":4}}},
		{"status":"Survived","mutatorName":"M5","location":{"start":{"line":5}}},
		{"status":"Survived","mutatorName":"M7","location":{"start":{"line":7}}},
		{"status":"Survived","mutatorName":"M8","location":{"start":{"line":8}}}
	]}}}`

	survivors, err := SurvivorsInScope(strings.NewReader(report), []MutationScope{{Path: "src/a.ts", Start: 5, End: 7}})
	if err != nil {
		t.Fatalf("SurvivorsInScope() = %v", err)
	}

	want := []string{"src/a.ts:5 M5", "src/a.ts:7 M7"}
	got := make([]string, 0, len(survivors))
	for _, survivor := range survivors {
		got = append(got, survivor.String())
	}
	if !slices.Equal(got, want) {
		t.Errorf("SurvivorsInScope() = %v, want %v — line 4 and line 8 sit one outside each end", got, want)
	}
}

// Two arguments can name the same line, and it is still one survivor. Reporting
// it twice would make the count a function of how the command was typed.
func TestSurvivorsInScopeCountsAnOverlappingSurvivorOnce(t *testing.T) {
	report := `{"files":{"src/a.ts":{"mutants":[
		{"status":"Survived","mutatorName":"EqualityOperator","location":{"start":{"line":6}}}
	]}}}`

	survivors, err := SurvivorsInScope(strings.NewReader(report), []MutationScope{
		{Path: "src/a.ts", Start: 5, End: 7},
		{Path: "src/a.ts", Start: 6, End: 8},
	})
	if err != nil {
		t.Fatalf("SurvivorsInScope() = %v", err)
	}
	if len(survivors) != 1 {
		t.Errorf("SurvivorsInScope() = %v, want one survivor named once", survivors)
	}
}

// A scope built by hand, without a typed suffix, still renders its range.
func TestArgumentRendersARangeItDidNotParse(t *testing.T) {
	if got := (MutationScope{Path: "src/a.ts", Start: 5, End: 7}).Argument(); got != "src/a.ts:5-7" {
		t.Errorf("Argument() = %q, want src/a.ts:5-7", got)
	}
	if got := (MutationScope{Path: "src/a.ts"}).Argument(); got != "src/a.ts" {
		t.Errorf("Argument() = %q, want the bare path", got)
	}
}

// Windows types backslashes; Stryker's report keys are always slash-separated.
func TestSurvivorsInScopeMatchesPathsAcrossSeparators(t *testing.T) {
	report := `{"files":{"src/a.ts":{"mutants":[
		{"status":"Survived","mutatorName":"EqualityOperator","location":{"start":{"line":6}}}
	]}}}`

	survivors, err := SurvivorsInScope(strings.NewReader(report), []MutationScope{{Path: `src\a.ts`}})
	if err != nil {
		t.Fatalf("SurvivorsInScope() = %v", err)
	}
	if len(survivors) != 1 {
		t.Fatalf("SurvivorsInScope() = %v, want the survivor a backslash path still names", survivors)
	}
}
