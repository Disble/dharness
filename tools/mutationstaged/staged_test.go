package main

import (
	"regexp"
	"slices"
	"testing"
)

func TestSelectMutableGoSourcesKeepsProductCode(t *testing.T) {
	t.Parallel()

	got := selectMutableGoSources([]string{
		"internal/cli/check.go",
		"internal/cli/check_test.go",
		`tools\mutationstaged\main.go`,
		"internal/testsupport/mutation/linescope.go",
		"cmd/dharness/main.go",
		"internal/space /name .go",
		"README.md",
	})
	want := []string{"internal/cli/check.go", "cmd/dharness/main.go", "internal/space /name .go"}
	if !slices.Equal(got, want) {
		t.Fatalf("selected = %v, want %v", got, want)
	}
}

func TestBuildIgnorePatternExcludesEverythingExceptStaged(t *testing.T) {
	t.Parallel()

	pattern := buildIgnorePattern(
		[]string{"internal/a.go", "internal/a_test.go", "tools/helper/main.go"},
		[]string{"internal/a.go"},
	)
	matcher := regexp.MustCompile(pattern)
	if matcher.MatchString("internal/a.go") {
		t.Fatal("staged production file was excluded")
	}
	for _, path := range []string{"internal/a_test.go", `internal\a_test.go`, `tools\helper\main.go`} {
		if !matcher.MatchString(path) {
			t.Fatalf("excluded path %q did not match %q", path, pattern)
		}
	}
}

func TestBuildIgnorePatternReturnsEmptySignal(t *testing.T) {
	t.Parallel()

	if got := buildIgnorePattern([]string{"a.go"}, []string{"a.go"}); got != "" {
		t.Fatalf("pattern = %q, want empty", got)
	}
}

func TestBuildTestCommandUsesOwningPackages(t *testing.T) {
	t.Parallel()

	got := buildTestCommand([]string{"internal/cli/check.go", "internal/cli/flags.go", "internal/project/detect.go"})
	want := "go test -short -count=1 ./internal/cli/ ./internal/project/"
	if got != want {
		t.Fatalf("test command = %q, want %q", got, want)
	}
}
