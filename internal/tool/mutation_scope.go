package tool

import (
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// mutationRange matches the tail Stryker documents for a --mutate argument:
// :startLine[:startColumn]-endLine[:endColumn].
//
// It is anchored at the end so a Windows drive letter cannot be read as the
// start of a range: C:\project\src\a.ts has a colon, but nothing after it looks
// like "digits, dash, digits".
var mutationRange = regexp.MustCompile(`:(\d+)(?::\d+)?-(\d+)(?::\d+)?$`)

// MutationScope is one --mutate argument: a path, and the lines of it that the
// run asked about.
//
// Start and End are zero when the argument named a bare path, which covers the
// whole file. The syntax is Stryker's own, not something dharness invented —
// its incremental documentation runs `--mutate src/app.js:5-7` — so dharness
// parses it only to know what it asked for, and passes it through untouched.
type MutationScope struct {
	Path       string
	Start, End int

	// suffix is the range exactly as it was typed, columns included. Rebuilding
	// it from Start and End would silently widen the run: :1:3-1:5 would go back
	// out as :1-1, and dharness would mutate lines the author excluded.
	suffix string
}

// ParseMutationScope splits a --mutate argument into the path and its lines.
func ParseMutationScope(arg string) MutationScope {
	match := mutationRange.FindStringSubmatch(arg)
	if match == nil {
		return MutationScope{Path: arg}
	}

	// Both groups are \d+, so they parse or the regexp did not match.
	start, _ := strconv.Atoi(match[1])
	end, _ := strconv.Atoi(match[2])
	return MutationScope{
		Path:   strings.TrimSuffix(arg, match[0]),
		Start:  start,
		End:    end,
		suffix: match[0],
	}
}

// WithPath re-roots the scope, keeping the range exactly as it was typed.
// Paths are typed against the working directory and Stryker reads them from
// inside the JS project, so the path moves and the range must not.
func (s MutationScope) WithPath(path string) MutationScope {
	s.Path = path
	return s
}

// Argument rebuilds the token Stryker receives.
func (s MutationScope) Argument() string {
	if s.suffix != "" {
		return s.Path + s.suffix
	}
	if s.Start == 0 && s.End == 0 {
		return s.Path
	}
	return s.Path + ":" + strconv.Itoa(s.Start) + "-" + strconv.Itoa(s.End)
}

// covers reports whether a survivor belongs to what this argument asked about.
//
// Columns are deliberately ignored. They narrow which mutants Stryker creates,
// and a mutant it never created cannot survive; carrying the distinction into
// the verdict would add arithmetic that no measurement asked for.
func (s MutationScope) covers(file string, line int) bool {
	if filepath.ToSlash(s.Path) != filepath.ToSlash(file) {
		return false
	}
	if s.Start == 0 && s.End == 0 {
		return true
	}
	return line >= s.Start && line <= s.End
}

// SurvivorsInScope reads a Stryker report and returns only the survivors the
// run asked about.
//
// The filter exists because the report is cumulative and the verdict must not
// be. --incremental keeps results for files and lines this run never named, so
// judging the whole report answers a question nobody asked: measured on
// 2026-08-13, a run scoped to src/a.ts:5-7 instrumented five mutants and then
// exited 1 on a survivor at line 10, left over from an earlier whole-file run.
//
// An author cannot act on that. The lines they asked about passed, and the
// failure names code their command deliberately excluded.
func SurvivorsInScope(r io.Reader, scopes []MutationScope) ([]Survivor, error) {
	all, err := Survivors(r)
	if err != nil {
		return nil, err
	}

	// No sorting here: Survivors already returns them in file and line order,
	// and dropping entries cannot reorder what is left. Sorting a second time
	// was mutation-tested and every mutant of it survived, which is what an
	// ordering that cannot change anything looks like.
	var scoped []Survivor
	for _, survivor := range all {
		for _, scope := range scopes {
			if scope.covers(survivor.File, survivor.Line) {
				// One match is enough. Overlapping arguments name the same
				// survivor twice and it is still one survivor.
				scoped = append(scoped, survivor)
				break
			}
		}
	}
	return scoped, nil
}
