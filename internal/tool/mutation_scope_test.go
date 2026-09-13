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

// TestArgumentEscapesGlobSignificantBracketsInThePathOnly pins the escaping.
// --mutate resolves through Stryker's own glob matching, which reads an
// unescaped [ or ] as a character class — so a Next.js dynamic route segment
// such as src/app/[id]/page.ts never matches its own file. Escaping follows
// the convention Stryker documents for its own glob syntax, [ -> [[] and
// ] -> []], and must never reach the :start-end suffix, which is not a glob.
// Measured: backslash-escaping the brackets does not work on Windows, while
// this form matches src/app/[[]id[]]/page.ts to exactly the literal file.
func TestArgumentEscapesGlobSignificantBracketsInThePathOnly(t *testing.T) {
	got := ParseMutationScope("src/app/[id]/page.ts:3-5").Argument()
	want := "src/app/[[]id[]]/page.ts:3-5"
	if got != want {
		t.Errorf("Argument() = %q, want %q", got, want)
	}
}

// A bare path with brackets and no range escapes the same way.
func TestArgumentEscapesBracketsWithNoRangeSuffix(t *testing.T) {
	got := (MutationScope{Path: "src/app/[id]/page.ts"}).Argument()
	want := "src/app/[[]id[]]/page.ts"
	if got != want {
		t.Errorf("Argument() = %q, want %q", got, want)
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

// TestFilesOutsideScopeNamesWhatTheRunDidNotAsk answers the question the
// clear-text table raises and cannot answer for itself: how much of what was
// just printed belongs to this run.
//
// Line ranges do not narrow it. A run scoped to src/a.ts:5-7 named that file,
// so its rows are this run's subject even where a survivor at line 10 is not
// this run's verdict; the question here is which *files* the reader did not
// ask about.
func TestFilesOutsideScopeNamesWhatTheRunDidNotAsk(t *testing.T) {
	report := `{"files":{
		"src/a.ts":{"mutants":[{"status":"Survived","mutatorName":"X","location":{"start":{"line":10}}}]},
		"src/b.ts":{"mutants":[{"status":"Killed","mutatorName":"X","location":{"start":{"line":1}}}]},
		"src/c.ts":{"mutants":[{"status":"Survived","mutatorName":"X","location":{"start":{"line":2}}}]}}}`

	got, err := FilesOutsideScope(strings.NewReader(report), []MutationScope{ParseMutationScope("src/a.ts:5-7")})
	if err != nil {
		t.Fatalf("FilesOutsideScope() = %v", err)
	}
	if !slices.Equal(got, []string{"src/b.ts", "src/c.ts"}) {
		t.Errorf("FilesOutsideScope() = %v, want the two files the run never named, in order", got)
	}
}

// TestIgnoredInScopeReadsTheReasonForAnInScopeDirective pins the reason.
// Measured: `// Stryker disable next-line all: reason` does not reach a
// dependency-array argument at all (the mutant still Survived), while the range
// form `// Stryker disable ArrayDeclaration: reason` … `// Stryker restore
// ArrayDeclaration` gives Ignored plus statusReason — so the reader
// needs the reason a report actually carries, not an assumption that
// next-line always worked.
//
// src/a.ts's own Ignored mutant sits at a higher line (20) than src/b.ts's
// (3), so file order and line order disagree — proving file is the primary
// sort key rather than a coincidence of the data. The Survived mutant sits
// before the Ignored one in src/a.ts's own list, proving a non-Ignored
// mutant does not stop the scan before it reaches an Ignored one later in
// the same file.
func TestIgnoredInScopeReadsTheReasonForAnInScopeDirective(t *testing.T) {
	report := `{"files":{
		"src/b.ts":{"mutants":[
			{"status":"Ignored","mutatorName":"BooleanLiteral","statusReason":"b reason","location":{"start":{"line":3}}}
		]},
		"src/a.ts":{"mutants":[
			{"status":"Survived","mutatorName":"EqualityOperator","location":{"start":{"line":7}}},
			{"status":"Ignored","mutatorName":"ArrayDeclaration","statusReason":"equivalent: order does not matter here","location":{"start":{"line":20}}}
		]}
	}}`

	ignored, err := IgnoredInScope(strings.NewReader(report), []MutationScope{{Path: "src/a.ts"}, {Path: "src/b.ts"}})
	if err != nil {
		t.Fatalf("IgnoredInScope() = %v", err)
	}

	want := []string{
		"src/a.ts:20 ArrayDeclaration — equivalent: order does not matter here",
		"src/b.ts:3 BooleanLiteral — b reason",
	}
	got := make([]string, 0, len(ignored))
	for _, entry := range ignored {
		got = append(got, entry.String())
	}
	if !slices.Equal(got, want) {
		t.Errorf("IgnoredInScope() = %v, want %v", got, want)
	}
}

// TestIgnoredInScopeSortsByLineWithinAFileAndCountsOverlapOnce covers what
// the file-level test above cannot reach: two Ignored mutants in the SAME
// file, which is the only way to exercise the line-based tiebreaker, and an
// Ignored mutant matched by two overlapping scope arguments, which must
// still be named once — the same discipline SurvivorsInScope already holds
// for an overlapping survivor.
func TestIgnoredInScopeSortsByLineWithinAFileAndCountsOverlapOnce(t *testing.T) {
	report := `{"files":{"src/a.ts":{"mutants":[
		{"status":"Ignored","mutatorName":"BooleanLiteral","statusReason":"later","location":{"start":{"line":9}}},
		{"status":"Ignored","mutatorName":"ArrayDeclaration","statusReason":"earlier","location":{"start":{"line":6}}}
	]}}}`

	// Line 6 sits in both scopes and must still be named once; line 9 sits
	// in only the second.
	ignored, err := IgnoredInScope(strings.NewReader(report), []MutationScope{
		{Path: "src/a.ts", Start: 5, End: 7},
		{Path: "src/a.ts", Start: 6, End: 10},
	})
	if err != nil {
		t.Fatalf("IgnoredInScope() = %v", err)
	}

	want := []string{
		"src/a.ts:6 ArrayDeclaration — earlier",
		"src/a.ts:9 BooleanLiteral — later",
	}
	got := make([]string, 0, len(ignored))
	for _, entry := range ignored {
		got = append(got, entry.String())
	}
	if !slices.Equal(got, want) {
		t.Errorf("IgnoredInScope() = %v, want %v", got, want)
	}
}

// A report with nothing Ignored in scope answers with nothing to print.
func TestIgnoredInScopeIsEmptyWithNoDirectives(t *testing.T) {
	report := `{"files":{"src/a.ts":{"mutants":[
		{"status":"Survived","mutatorName":"EqualityOperator","location":{"start":{"line":7}}}
	]}}}`

	ignored, err := IgnoredInScope(strings.NewReader(report), []MutationScope{{Path: "src/a.ts"}})
	if err != nil {
		t.Fatalf("IgnoredInScope() = %v", err)
	}
	if len(ignored) != 0 {
		t.Errorf("IgnoredInScope() = %v, want none", ignored)
	}
}

// TestFilesOutsideScopeIsEmptyWhenTheReportMatchesTheRun keeps the note it
// feeds from printing on a run that has nothing to explain.
func TestFilesOutsideScopeIsEmptyWhenTheReportMatchesTheRun(t *testing.T) {
	report := `{"files":{"src/a.ts":{"mutants":[{"status":"Killed","mutatorName":"X","location":{"start":{"line":1}}}]}}}`

	got, err := FilesOutsideScope(strings.NewReader(report), []MutationScope{ParseMutationScope("src/a.ts")})
	if err != nil {
		t.Fatalf("FilesOutsideScope() = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("FilesOutsideScope() = %v, want none", got)
	}
}
