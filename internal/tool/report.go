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

// Survivor is one mutant that no test noticed.
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
}

// String is one survivor on one line, which is what makes the list scannable.
//
// The replacement is collapsed rather than wrapped: a mutated multi-line
// expression carries its newlines into the report, and a list that reflows is
// a list nobody can read down. The schema marks the field optional, so an
// absent one prints no arrow rather than an arrow pointing at nothing.
func (s Survivor) String() string {
	if s.Replacement == "" {
		return fmt.Sprintf("%s:%d %s", s.File, s.Line, s.Description)
	}
	return fmt.Sprintf("%s:%d %s → %s", s.File, s.Line, s.Description, collapse(s.Replacement))
}

// collapse folds any run of whitespace into one space and trims the ends.
func collapse(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// mutationReport is the subset of the mutation-testing report schema that
// answers the only question dharness asks of it.
type mutationReport struct {
	Files map[string]struct {
		Mutants []struct {
			Status      string `json:"status"`
			MutatorName string `json:"mutatorName"`
			Replacement string `json:"replacement"`
			Location    struct {
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
func Survivors(r io.Reader) ([]Survivor, error) {
	var report mutationReport
	if err := json.NewDecoder(r).Decode(&report); err != nil {
		return nil, fmt.Errorf("read the mutation report: %w", err)
	}

	var survivors []Survivor
	for path, file := range report.Files {
		for _, mutant := range file.Mutants {
			if mutant.Status != "Survived" {
				continue
			}
			survivors = append(survivors, Survivor{
				File:        path,
				Line:        mutant.Location.Start.Line,
				Description: mutant.MutatorName,
				Replacement: mutant.Replacement,
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
