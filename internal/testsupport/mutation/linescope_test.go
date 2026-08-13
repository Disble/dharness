package mutation

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"github.com/Disble/ditto/viruses"
)

type alwaysInfects struct{ calls int }

func (v *alwaysInfects) Incubate(ast.Node, *types.Info) []*viruses.Infection {
	v.calls++
	return []*viruses.Infection{viruses.NewInfection("test", func() {}, func() {})}
}

func TestPositionMinusOneIsByteOffsetWithFreshFileSet(t *testing.T) {
	t.Parallel()

	source := "package p\nvar answer = 42\n"
	tree, err := parser.ParseFile(token.NewFileSet(), "sample.go", source, parser.AllErrors)
	if err != nil {
		t.Fatal(err)
	}
	var found ast.Node
	ast.Inspect(tree, func(node ast.Node) bool {
		if literal, ok := node.(*ast.BasicLit); ok && literal.Value == "42" {
			found = node
		}
		return true
	})
	if found == nil || int(found.Pos())-1 != strings.Index(source, "42") {
		t.Fatalf("position did not map to byte offset: %v", found)
	}
}

func TestParseOffsetRangesAndBoundaries(t *testing.T) {
	t.Parallel()

	ranges, err := ParseOffsetRanges("10-20,40-45")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		offset int
		want   bool
	}{{9, false}, {10, true}, {19, true}, {20, false}, {44, true}, {45, false}} {
		if got := ranges.Contains(test.offset); got != test.want {
			t.Fatalf("Contains(%d) = %t, want %t", test.offset, got, test.want)
		}
	}
}

func TestParseOffsetRangesRejectsMalformedInput(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"10", "10-", "-20", "a-b", "20-10", "10-20,oops"} {
		if _, err := ParseOffsetRanges(input); err == nil {
			t.Fatalf("ParseOffsetRanges(%q) succeeded", input)
		}
	}
}

func TestLineScopedDelegatesOnlyInsideScope(t *testing.T) {
	t.Parallel()

	inner := &alwaysInfects{}
	counter := &ScopeCounter{}
	scoped := NewLineScoped(inner, OffsetRanges{{Start: 100, End: 200}}, counter)
	if got := scoped.Incubate(&ast.Ident{NamePos: 151}, nil); len(got) != 1 {
		t.Fatalf("inside infections = %d", len(got))
	}
	if got := scoped.Incubate(&ast.Ident{NamePos: 251}, nil); len(got) != 0 {
		t.Fatalf("outside infections = %d", len(got))
	}
	if inner.calls != 1 || counter.Kept() != 1 || counter.Dropped() != 1 {
		t.Fatalf("calls/kept/dropped = %d/%d/%d", inner.calls, counter.Kept(), counter.Dropped())
	}
}

func TestLineScopedFailsOpenWithoutRanges(t *testing.T) {
	t.Parallel()

	inner := &alwaysInfects{}
	if got := NewLineScoped(inner, nil, &ScopeCounter{}).Incubate(&ast.Ident{NamePos: 999}, nil); len(got) != 1 {
		t.Fatalf("unscoped infections = %d", len(got))
	}
}

func TestLineScopedHandlesNilAndNoPosition(t *testing.T) {
	t.Parallel()

	inner := &alwaysInfects{}
	scoped := NewLineScoped(inner, OffsetRanges{{Start: 100, End: 200}}, &ScopeCounter{})
	if got := scoped.Incubate(nil, nil); len(got) != 1 {
		t.Fatalf("nil infections = %d", len(got))
	}
	if got := scoped.Incubate(&ast.Ident{NamePos: token.NoPos}, nil); len(got) != 1 {
		t.Fatalf("NoPos infections = %d", len(got))
	}
}

func TestAnalyzeSourceCountsCandidatesBeforeOozeRelease(t *testing.T) {
	t.Parallel()

	source := []byte("package p\n\nfunc add(a, b int) int { return a + b }\n")
	start := strings.Index(string(source), "return")
	stats, err := AnalyzeSource("sample.go", source, OffsetRanges{{Start: start, End: len(source)}})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Candidates == 0 || stats.Kept == 0 || stats.Dropped == 0 {
		t.Fatalf("stats = %+v, want visible candidate/kept/dropped counts", stats)
	}
}

func TestAnalyzeSourceExposesSilentNoOp(t *testing.T) {
	t.Parallel()

	source := []byte("package p\n\nfunc answer() int { return 1 }\n")
	stats, err := AnalyzeSource("sample.go", source, OffsetRanges{{Start: 0, End: len("package p\n")}})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Candidates != 0 || stats.Dropped == 0 {
		t.Fatalf("stats = %+v, want zero candidates and visible drops", stats)
	}
}

func TestDefaultVirusesPinsDittoMutatorSet(t *testing.T) {
	t.Parallel()

	if got := len(DefaultViruses()); got != 14 {
		t.Fatalf("default mutators = %d, want 14; review upstream drift before changing this contract", got)
	}
}
