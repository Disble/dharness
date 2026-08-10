package main

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

var mutationExcludedPrefixes = []string{"tools/", "internal/testsupport/"}

func isMutableGoSource(file string) bool {
	if !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") {
		return false
	}
	for _, prefix := range mutationExcludedPrefixes {
		if strings.HasPrefix(file, prefix) {
			return false
		}
	}
	return true
}

func selectMutableGoSources(files []string) []string {
	selected := []string{}
	for _, file := range files {
		file = strings.ReplaceAll(file, "\\", "/")
		if file != "" && isMutableGoSource(file) {
			selected = append(selected, file)
		}
	}
	return selected
}

func normalizeTrackedPaths(files []string) []string {
	normalized := []string{}
	for _, file := range files {
		file = strings.ReplaceAll(file, "\\", "/")
		if file != "" {
			normalized = append(normalized, file)
		}
	}
	return normalized
}

func buildIgnorePattern(tracked, staged []string) string {
	selected := make(map[string]struct{}, len(staged))
	for _, file := range staged {
		selected[file] = struct{}{}
	}
	excluded := []string{}
	for _, file := range tracked {
		if _, ok := selected[file]; ok {
			continue
		}
		excluded = append(excluded, strings.ReplaceAll(regexp.QuoteMeta(file), "/", `[/\\]`))
	}
	if len(excluded) == 0 {
		return ""
	}
	sort.Strings(excluded)
	return "^(?:" + strings.Join(excluded, "|") + ")$"
}

func packagePatternsFor(staged []string) []string {
	seen := map[string]struct{}{}
	patterns := []string{}
	for _, file := range staged {
		dir := path.Dir(file)
		pattern := "./"
		if dir != "." {
			pattern += dir + "/"
		}
		if _, ok := seen[pattern]; !ok {
			seen[pattern] = struct{}{}
			patterns = append(patterns, pattern)
		}
	}
	sort.Strings(patterns)
	return patterns
}

func buildTestCommand(staged []string) string {
	patterns := packagePatternsFor(staged)
	if len(patterns) == 0 {
		return ""
	}
	return "go test -short -count=1 " + strings.Join(patterns, " ")
}

func describeSelection(staged []string) string {
	return fmt.Sprintf("go mutation: %d staged production file(s), %d owning package(s): %s",
		len(staged), len(packagePatternsFor(staged)), strings.Join(packagePatternsFor(staged), " "))
}
