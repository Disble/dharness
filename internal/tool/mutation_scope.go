package tool

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
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
//
// The path is escaped for Stryker's own glob matching; the :start-end suffix
// never is, because it is not a glob at all — see escapeBrackets.
func (s MutationScope) Argument() string {
	path := escapeBrackets(s.Path)
	if s.suffix != "" {
		return path + s.suffix
	}
	if s.Start == 0 && s.End == 0 {
		return path
	}
	return path + ":" + strconv.Itoa(s.Start) + "-" + strconv.Itoa(s.End)
}

// escapeBrackets escapes glob-significant brackets so Stryker's own glob
// matching treats them literally, following the escaping convention measured
// against minimatch's character-class escaping, which is what Stryker's own
// glob matcher uses: [ becomes [[] and ] becomes []].
//
// Measured rather than assumed: backslash-escaping does not work on Windows,
// where a path such as src/app/[id]/page.ts is exactly what a Next.js dynamic
// route segment produces, and left unescaped Stryker's glob matcher reads the
// brackets as a character class rather than as literal characters — so the
// path never matches its own file. This has to run one character at a time
// rather than as two sequential whole-string replacements: replacing every
// "[" first and then every "]" would also rewrite the "]" the first pass just
// introduced.
func escapeBrackets(path string) string {
	if !strings.ContainsAny(path, "[]") {
		return path
	}
	var escaped strings.Builder
	escaped.Grow(len(path))
	for _, r := range path {
		switch r {
		case '[':
			escaped.WriteString("[[]")
		case ']':
			escaped.WriteString("[]]")
		default:
			escaped.WriteRune(r)
		}
	}
	return escaped.String()
}

// slashed normalises a path for comparison, and deliberately does not use
// filepath.ToSlash.
//
// ToSlash is a no-op wherever the separator is already "/", so on Linux a
// backslash path stays a backslash path and silently matches nothing — which
// here means "no survivors", a false pass, the exact failure this file exists
// to remove. Caught by CI on ubuntu after passing on Windows.
func slashed(path string) string {
	return strings.ReplaceAll(path, `\`, "/")
}

// covers reports whether a survivor belongs to what this argument asked about.
//
// Columns are deliberately ignored. They narrow which mutants Stryker creates,
// and a mutant it never created cannot survive; carrying the distinction into
// the verdict would add arithmetic that no measurement asked for.
func (s MutationScope) covers(file string, line int) bool {
	if slashed(s.Path) != slashed(file) {
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

// IgnoredInScope reads a Stryker report and returns the in-scope mutants a
// `// Stryker disable` directive marked, together with the reason each
// directive gave.
//
// Scoped the same way SurvivorsInScope is, and for the same reason: the
// report is cumulative, and a directive covering code outside this run is
// not this run's business to relay.
func IgnoredInScope(r io.Reader, scopes []MutationScope) ([]Ignored, error) {
	var report mutationReport
	if err := json.NewDecoder(r).Decode(&report); err != nil {
		return nil, fmt.Errorf("read the mutation report: %w", err)
	}

	var ignored []Ignored
	for path, file := range report.Files {
		for _, mutant := range file.Mutants {
			if mutant.Status != "Ignored" {
				continue
			}
			line := mutant.Location.Start.Line
			for _, scope := range scopes {
				if scope.covers(path, line) {
					ignored = append(ignored, Ignored{
						File:        path,
						Line:        line,
						Description: mutant.MutatorName,
						Reason:      mutant.StatusReason,
					})
					break
				}
			}
		}
	}

	// Sorted, so a run over several files reads the same way twice — this one
	// has to sort where SurvivorsInScope does not, because it reads and
	// filters the report itself rather than layering on Survivors' own
	// already-sorted output.
	//
	// Two equivalent mutants live here, each a `<` widened to `<=` inside a
	// branch where the two operands are already known to disagree, which
	// makes the two operators return the same result by construction: the
	// File comparison only runs once `!=` has excluded equality, and the
	// Line comparison would only differ from `<=` for two mutants sharing
	// one exact file and line, whose relative display order is not a
	// distinction this type promises to make.
	sort.Slice(ignored, func(i, j int) bool {
		if ignored[i].File != ignored[j].File {
			return ignored[i].File < ignored[j].File
		}
		return ignored[i].Line < ignored[j].Line
	})
	return ignored, nil
}

// FilesOutsideScope names the files the report carries that this run never
// asked about, in path order.
//
// It exists to make one sentence truthful. Stryker's clear-text table is
// printed from the cumulative report, and no Stryker option scopes it to
// --mutate: `clearTextReporter.skipFull` skips fully covered files, not
// out-of-scope ones. Measured on a bun/vitest fixture, naming one file printed
// a table of three, with `All files ... 4 survived` two lines above `Every
// mutant was caught`. Both were true of different things, which is what made
// it cost an afternoon.
//
// dharness does not rewrite that table — it is the wrapped tool's own output
// (§03). It says what the table is, and only when there is something to say:
// an empty answer here means the report and the run cover the same files, and
// a note explaining a mismatch that did not happen is noise on every run.
//
// The question is per file, not per line. A run scoped to src/a.ts:5-7 named
// that file, so its rows are this run's subject even where a survivor at line
// 10 is not this run's verdict — that narrower question is SurvivorsInScope's.
func FilesOutsideScope(r io.Reader, scopes []MutationScope) ([]string, error) {
	var report mutationReport
	if err := json.NewDecoder(r).Decode(&report); err != nil {
		return nil, fmt.Errorf("read the mutation report: %w", err)
	}

	var outside []string
	for path := range report.Files {
		named := false
		for _, scope := range scopes {
			if slashed(scope.Path) == slashed(path) {
				named = true
				break
			}
		}
		if !named {
			outside = append(outside, path)
		}
	}
	sort.Strings(outside)
	return outside, nil
}
