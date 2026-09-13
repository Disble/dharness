package staged

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Disble/dharness/internal/project"
	"github.com/Disble/dharness/internal/tool"
)

// hunkNewRange matches a unified diff hunk header's new-side range:
// @@ -oldStart[,oldCount] +newStart[,newCount] @@. Only the new side is read —
// a scoped mutation run only ever asks about lines the change added.
var hunkNewRange = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

// diffNewPath matches the "+++ b/<path>" line every diff section carries,
// naming the file a hunk that follows belongs to.
//
// It is always the post-rename path: with -M/-C and no pathspec, git already
// re-pairs a renamed file against its own prior content, so the hunks that
// follow here are only the lines that actually changed, not the whole file
// re-diffed against nothing.
var diffNewPath = regexp.MustCompile(`^\+\+\+ b/(.*)$`)

// UnparsedAddedLinesError reports a file numstat says gained lines, for which
// the hunk parser found no range at all.
//
// This is dharness's own parsing failing to explain what git already reported,
// and it fails closed rather than silently scoping nothing: a scope with no
// ranges over a file that changed is indistinguishable from a scope that
// correctly found the file untouched.
type UnparsedAddedLinesError struct{ Path string }

func (e *UnparsedAddedLinesError) Error() string {
	return fmt.Sprintf(
		"%s gained lines according to git's own numstat, but the staged diff produced no range for it; refusing rather than scoping nothing over a file that changed",
		e.Path,
	)
}

// Scope reads the staged index from source and returns one tool.MutationScope
// per range it added — several ranges in one file become several scopes.
//
// No pathspec is ever passed to the diff that produces these ranges.
// Measured: a destination-only pathspec defeated git's own rename pairing and
// billed 72 of 146 survivors to byte-identical moves, because limiting the
// diff to only the new path stops git from finding the old content to pair it
// against. The full, unscoped diff is read first, and the paths dharness does
// not care about are filtered out afterward.
func Scope(source string, excludePrefixes []string) ([]tool.MutationScope, error) {
	diff, err := gitOutput(source,
		"-c", "core.quotePath=false",
		"diff", "--cached", "-M", "-C", "-U0", "--relative",
		"--no-ext-diff", "--diff-filter=ACMR")
	if err != nil {
		return nil, fmt.Errorf("read the staged diff: %w", err)
	}

	ranges, err := parseHunkRanges(diff)
	if err != nil {
		return nil, err
	}

	numstat, err := gitOutput(source, "diff", "--cached", "--numstat", "-M", "-C", "--relative", "-z")
	if err != nil {
		return nil, fmt.Errorf("read the staged numstat: %w", err)
	}
	added, err := parseNumstatAdded(numstat)
	if err != nil {
		return nil, err
	}

	// Fail closed before filtering: whether a file ends up in dharness's own
	// scope is a policy question (source file, not a test, not excluded), but
	// whether the parser accounted for every added line is not — a file
	// numstat says gained lines and the hunk parser never saw is a parsing
	// gap, not a file nobody asked about.
	for path, addedCount := range added {
		if addedCount > 0 && len(ranges[path]) == 0 {
			return nil, &UnparsedAddedLinesError{Path: path}
		}
	}

	// Sorted rather than ranged over directly: map iteration order is random,
	// and a --mutate argument built from it would reorder from one run to the
	// next over the exact same staged change.
	paths := make([]string, 0, len(ranges))
	for path := range ranges {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var scopes []tool.MutationScope
	var scopedFiles []string
	for _, path := range paths {
		if !included(path, excludePrefixes) {
			continue
		}
		scopedFiles = append(scopedFiles, path)
		for _, r := range ranges[path] {
			scopes = append(scopes, tool.ParseMutationScope(fmt.Sprintf("%s:%d-%d", path, r.start, r.end)))
		}
	}

	if err := checkFullyStaged(source, scopedFiles); err != nil {
		return nil, err
	}

	return scopes, nil
}

// lineRange is one hunk's new-side span, inclusive on both ends.
type lineRange struct{ start, end int }

// parseHunkRanges reads a -U0 unified diff and returns every range it added,
// keyed by the post-rename path each hunk's own "+++ b/" line names.
//
// A hunk whose new-side count is explicitly 0 is a pure deletion at that
// point in the file — nothing was added there — and contributes no range.
func parseHunkRanges(diff []byte) (map[string][]lineRange, error) {
	ranges := make(map[string][]lineRange)
	var currentPath string

	scanner := bufio.NewScanner(bytes.NewReader(diff))
	for scanner.Scan() {
		line := scanner.Text()

		if match := diffNewPath.FindStringSubmatch(line); match != nil {
			currentPath = match[1]
			continue
		}

		match := hunkNewRange.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		if currentPath == "" {
			return nil, fmt.Errorf("read a hunk before any file header in the staged diff: %q", line)
		}

		start, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, fmt.Errorf("read the hunk start in %q: %w", line, err)
		}
		count := 1
		if match[2] != "" {
			count, err = strconv.Atoi(match[2])
			if err != nil {
				return nil, fmt.Errorf("read the hunk count in %q: %w", line, err)
			}
		}
		if count == 0 {
			continue
		}
		ranges[currentPath] = append(ranges[currentPath], lineRange{start: start, end: start + count - 1})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read the staged diff: %w", err)
	}
	return ranges, nil
}

// numstatEntry matches one numstat record's leading counts: <added> TAB
// <deleted> TAB, followed by either the path (a plain entry) or nothing (a
// rename, whose paths follow as their own NUL-terminated fields).
var numstatEntry = regexp.MustCompile(`^(-|\d+)\t(-|\d+)\t(.*)$`)

// parseNumstatAdded reads `--numstat -z` output into added-line counts keyed
// by the final path — the same path a rename's own hunks are keyed by in
// parseHunkRanges, so the two can be compared directly.
//
// A binary file reports "-" for both counts and is skipped: it never
// produces a hunk either, so there is nothing to cross-check it against.
func parseNumstatAdded(numstat []byte) (map[string]int, error) {
	fields := numstatFields(numstat)
	added := make(map[string]int)

	for i := 0; i < len(fields); i++ {
		match := numstatEntry.FindStringSubmatch(fields[i])
		if match == nil {
			return nil, fmt.Errorf("read a numstat record: %q", fields[i])
		}

		path := match[3]
		if path == "" {
			// A rename: the destination path is the next field over, after
			// the old path this run never needs.
			if i+2 >= len(fields) {
				return nil, fmt.Errorf("read a rename's paths from numstat: %q", fields[i])
			}
			path = fields[i+2]
			i += 2
		}

		if match[1] == "-" {
			continue
		}
		count, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, fmt.Errorf("read the added count in %q: %w", fields[i], err)
		}
		added[path] = count
	}
	return added, nil
}

// numstatFields splits `--numstat -z` output into its raw NUL-delimited
// fields, keeping every empty field except the one trailing terminator.
//
// This cannot reuse splitNUL: numstat's own empty field is the signal that
// the next two fields are a rename's old and new paths rather than the
// record's path arriving inline, and splitNUL exists precisely to discard
// exactly that kind of empty field from an ordinary path listing.
func numstatFields(out []byte) []string {
	trimmed := bytes.TrimSuffix(out, []byte{0})
	if len(trimmed) == 0 {
		return nil
	}
	return strings.Split(string(trimmed), "\x00")
}

// included reports whether a staged path belongs in dharness's own mutation
// scope: a JS/TS source file, not a test, and not under a caller-excluded
// prefix.
func included(path string, excludePrefixes []string) bool {
	if !project.IsSourceFile(path) {
		return false
	}
	if isDeclarationFile(path) {
		return false
	}
	if isTestFile(path) {
		return false
	}
	for _, prefix := range excludePrefixes {
		if strings.HasPrefix(path, prefix) {
			return false
		}
	}
	return true
}

// isDeclarationFile reports whether path is a TypeScript declaration file:
// .d.ts, .d.mts or .d.cts.
//
// project.IsSourceFile does not tell one apart from an ordinary source
// file — filepath.Ext only ever returns the final extension, so
// "env.d.ts".Ext() is ".ts", the same extension a real source file carries.
// A declaration file can hold no runtime code, and leaving it in scope would
// cost more than a wasted mutant: the types-only classifier disengages for the
// whole commit the moment tsc emits no output at all for a scoped file, which
// is exactly what it does for a .d.ts, so a .d.ts-only commit would silently
// stop excluding types-only files it should have excluded on its own terms.
func isDeclarationFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".d.ts") ||
		strings.HasSuffix(lower, ".d.mts") ||
		strings.HasSuffix(lower, ".d.cts")
}

// isTestFile reports whether path is a test file by dharness's own
// convention, matched against the full repository-relative path: git always
// reports it slash-separated, on every platform.
func isTestFile(path string) bool {
	return strings.Contains(path, ".test.") ||
		strings.Contains(path, ".spec.") ||
		strings.Contains(path, "__tests__/")
}
