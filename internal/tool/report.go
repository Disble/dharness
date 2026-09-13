package tool

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// MutationReportPath is where Stryker's json reporter writes, relative to the
// project root.
//
// It is not configurable from the command line: --jsonReporter.fileName does
// not exist, and neither does any dotted form of it. The report lands in the
// repository whether dharness wants it there or not, which is worth knowing
// when deciding what to ignore in git.
var MutationReportPath = filepath.Join("reports", "mutation", "mutation.json")

// Survivor is one mutant that no test noticed — either by surviving one that
// ran, or by never being run at all.
type Survivor struct {
	File        string
	Line        int
	Description string

	// Replacement is what the mutant put in the code's place, as the report
	// records it, or "" when the report carries none.
	//
	// A mutator name and a line number do not say what changed: the field
	// report's example was `MethodExpression` on a line holding a five-line
	// chained expression, which narrows it to "a method call somewhere in
	// here was removed". Working the rest out meant parsing by hand the same
	// JSON dharness had just read to reach its verdict.
	Replacement string

	// Status is the report's own mutant status: "Survived" or "NoCoverage".
	// It is what String uses to tell the two apart, since they fail for
	// different reasons — a test ran and missed it, or no test ran at all.
	Status string
}

// String is one survivor on one line, which is what makes the list scannable.
//
// The replacement is collapsed rather than wrapped: a mutated multi-line
// expression carries its newlines into the report, and a list that reflows is
// a list nobody can read down. The schema marks the field optional, so an
// absent one prints no arrow rather than an arrow pointing at nothing.
//
// NoCoverage carries a trailing note because it is a different failure from
// Survived: nothing ever exercised the mutated line, so there is no test run
// to say "missed it" about.
func (s Survivor) String() string {
	suffix := ""
	if s.Status == "NoCoverage" {
		suffix = " (no test ran it)"
	}
	if s.Replacement == "" {
		return fmt.Sprintf("%s:%d %s%s", s.File, s.Line, s.Description, suffix)
	}
	return fmt.Sprintf("%s:%d %s → %s%s", s.File, s.Line, s.Description, collapse(s.Replacement), suffix)
}

// collapse folds any run of whitespace into one space and trims the ends.
func collapse(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// Ignored is one mutant Stryker skipped because a `// Stryker disable`
// directive marked it, carrying the reason the directive gave.
//
// Measured: `// Stryker disable next-line all: reason` produces
// "status":"Ignored","statusReason":"reason" in the JSON report, and
// Stryker's own clear-text reporter never prints either one — an author who
// marked a mutant equivalent has no way to see that dharness agrees, or what
// reason is on record, without opening the JSON report by hand.
type Ignored struct {
	File        string
	Line        int
	Description string
	Reason      string
}

// String reads the same way a Survivor does, with an em dash marking the
// reason rather than an arrow: nothing here "became" anything, the mutant
// was never run at all.
func (i Ignored) String() string {
	if i.Reason == "" {
		return fmt.Sprintf("%s:%d %s", i.File, i.Line, i.Description)
	}
	return fmt.Sprintf("%s:%d %s — %s", i.File, i.Line, i.Description, i.Reason)
}

// mutationReport is the subset of the mutation-testing report schema that
// answers the only question dharness asks of it.
type mutationReport struct {
	Files map[string]struct {
		Mutants []struct {
			Status       string `json:"status"`
			MutatorName  string `json:"mutatorName"`
			Replacement  string `json:"replacement"`
			StatusReason string `json:"statusReason"`
			Location     struct {
				Start struct {
					Line int `json:"line"`
				} `json:"start"`
			} `json:"location"`
		} `json:"mutants"`
	} `json:"files"`
}

// Survivors reads a Stryker report and returns the mutants that lived.
//
// dharness has to read this because Stryker cannot be told to fail: the
// threshold that produces a non-zero exit lives only in `thresholds.break`
// inside a config file. There is no --break, and no --thresholds.break either
// — both were tried and rejected as unknown options. Left alone, Stryker
// prints surviving mutants and exits 0.
//
// Timeouts are not survivors. A mutant that hangs the suite was detected by
// it, which is the question being asked; Stryker's own score treats it the
// same way.
//
// NoCoverage mutants are survivors, and this is the one deviation from
// Stryker's own score. Measured on dharness 1.7.6: a range with 11
// Killed and 8 NoCoverage mutants — an exported function no test called —
// exited 0 and printed "Every mutant was caught: these tests notice this
// code breaking", over code no test ever ran at all. Also measured:
// the same untested function reports Survived when no other in-scope mutant
// is hit and NoCoverage when one is, so the two statuses describe the same
// underlying failure and have to share one verdict — treating them
// differently would make the exit code depend on which other lines happened
// to be in scope alongside the untested one.
func Survivors(r io.Reader) ([]Survivor, error) {
	var report mutationReport
	if err := json.NewDecoder(r).Decode(&report); err != nil {
		return nil, fmt.Errorf("read the mutation report: %w", err)
	}

	var survivors []Survivor
	for path, file := range report.Files {
		for _, mutant := range file.Mutants {
			if mutant.Status != "Survived" && mutant.Status != "NoCoverage" {
				continue
			}
			survivors = append(survivors, Survivor{
				File:        path,
				Line:        mutant.Location.Start.Line,
				Description: mutant.MutatorName,
				Replacement: mutant.Replacement,
				Status:      mutant.Status,
			})
		}
	}

	sort.Slice(survivors, func(i, j int) bool {
		if survivors[i].File != survivors[j].File {
			return survivors[i].File < survivors[j].File
		}
		return survivors[i].Line < survivors[j].Line
	})
	return survivors, nil
}

// FileMutantCounts reports how many mutants Stryker's report instrumented
// for each file it names, regardless of status.
//
// This is a different question from Survivors: a staged mutation run checks
// its types-only classifier against Stryker by asking whether a file the
// classifier believed compiled to nothing carries ANY mutant at all, killed
// ones included. Survivors would report zero mutants for a file whose only
// mutant was Killed, which is exactly the silence that would let a wrong
// classification through unnoticed.
func FileMutantCounts(r io.Reader) (map[string]int, error) {
	var report mutationReport
	if err := json.NewDecoder(r).Decode(&report); err != nil {
		return nil, fmt.Errorf("read the mutation report: %w", err)
	}

	counts := make(map[string]int, len(report.Files))
	for path, file := range report.Files {
		counts[path] = len(file.Mutants)
	}
	return counts, nil
}

// initialTestRun matches the one line where a dry run says what it cost.
//
// The count is read from prose because there is nowhere else to read it:
// --dryRunOnly completes without writing the json report, verified by running
// it. Anchoring on "Initial test run" rather than on the bare number keeps this
// from matching some other sentence that happens to contain a count.
var initialTestRun = regexp.MustCompile(`Initial test run succeeded\. Ran (\d+) tests?`)

// RelatedTests reports how many tests a dry run executed.
//
// This is the measurement that decides whether scoped mutation is viable in a
// repository. Stryker does not derive the set from the paths: the test runner
// derives it from the import graph, and barrel files inflate that graph until
// every test counts as related.
func RelatedTests(output string) (int, error) {
	match := initialTestRun.FindStringSubmatch(output)
	if match == nil {
		return 0, fmt.Errorf("the run did not report an initial test run, so nothing was measured")
	}
	count, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, fmt.Errorf("read the test count %q: %w", match[1], err)
	}
	return count, nil
}

// MutantTally counts the mutants a run generated inside its scope, by the
// status Stryker gave each. CompileError and RuntimeError share Errors, the
// column Stryker's own clear-text table gives them.
type MutantTally struct {
	Killed, Survived, NoCoverage, Timeout, Errors, Ignored int
}

// Total is every mutant the tally counted.
func (t MutantTally) Total() int {
	return t.Killed + t.Survived + t.NoCoverage + t.Timeout + t.Errors + t.Ignored
}

// MutantTallyInScope reads a Stryker report and counts the mutants inside
// scopes, status by status.
//
// It exists because a verdict computed from survivors alone cannot tell a
// clean run from an empty one. No survivor is what a range with eleven killed
// mutants reports, and also what a range Stryker generated nothing in
// reports — a staged enum, a side-effect import, a --mutate that matched no
// line — and both printed "Every mutant was caught". Counting what was
// generated is what separates them.
func MutantTallyInScope(r io.Reader, scopes []MutationScope) (MutantTally, error) {
	var report mutationReport
	if err := json.NewDecoder(r).Decode(&report); err != nil {
		return MutantTally{}, fmt.Errorf("read the mutation report: %w", err)
	}

	var tally MutantTally
	for path, file := range report.Files {
		for _, mutant := range file.Mutants {
			if !anyCovers(scopes, path, mutant.Location.Start.Line) {
				continue
			}
			switch mutant.Status {
			case "Killed":
				tally.Killed++
			case "Survived":
				tally.Survived++
			case "NoCoverage":
				tally.NoCoverage++
			case "Timeout":
				tally.Timeout++
			case "CompileError", "RuntimeError":
				tally.Errors++
			case "Ignored":
				tally.Ignored++
			}
		}
	}
	return tally, nil
}

// anyCovers reports whether one of scopes asked about this line of path.
func anyCovers(scopes []MutationScope, path string, line int) bool {
	for _, scope := range scopes {
		if scope.covers(path, line) {
			return true
		}
	}
	return false
}
