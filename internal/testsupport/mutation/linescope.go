package mutation

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/gtramontina/ooze/viruses"
)

// OffsetRange is a half-open byte range in one source file.
type OffsetRange struct {
	Start int
	End   int
}

// OffsetRanges is sorted by end offset for binary-search lookup.
type OffsetRanges []OffsetRange

func (ranges OffsetRanges) Contains(offset int) bool {
	index := sort.Search(len(ranges), func(i int) bool { return ranges[i].End > offset })
	return index < len(ranges) && offset >= ranges[index].Start
}

// ParseOffsetRanges decodes start-end pairs. Empty input means whole-file scope.
func ParseOffsetRanges(encoded string) (OffsetRanges, error) {
	if strings.TrimSpace(encoded) == "" {
		return nil, nil
	}

	parts := strings.Split(encoded, ",")
	ranges := make(OffsetRanges, 0, len(parts))
	for _, part := range parts {
		start, end, found := strings.Cut(strings.TrimSpace(part), "-")
		if !found {
			return nil, fmt.Errorf("offset range %q is not in start-end form", part)
		}
		startOffset, startErr := strconv.Atoi(start)
		endOffset, endErr := strconv.Atoi(end)
		if startErr != nil || endErr != nil {
			return nil, fmt.Errorf("offset range %q must contain numeric offsets", part)
		}
		if startOffset < 0 || endOffset <= startOffset {
			return nil, fmt.Errorf("offset range %q must have 0 <= start < end", part)
		}
		ranges = append(ranges, OffsetRange{Start: startOffset, End: endOffset})
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].End < ranges[j].End })
	return ranges, nil
}

// ScopeCounter makes line-filter decisions visible to the preflight and harness.
type ScopeCounter struct {
	kept    atomic.Int64
	dropped atomic.Int64
}

func (counter *ScopeCounter) Kept() int    { return int(counter.kept.Load()) }
func (counter *ScopeCounter) Dropped() int { return int(counter.dropped.Load()) }

type lineScoped struct {
	inner   viruses.Virus
	ranges  OffsetRanges
	counter *ScopeCounter
}

// NewLineScoped keeps nodes in the staged ranges. Missing ranges fail open.
func NewLineScoped(inner viruses.Virus, ranges OffsetRanges, counter *ScopeCounter) viruses.Virus {
	if counter == nil {
		counter = &ScopeCounter{}
	}
	return &lineScoped{inner: inner, ranges: ranges, counter: counter}
}

func (scoped *lineScoped) Incubate(node ast.Node) []*viruses.Infection {
	if len(scoped.ranges) == 0 {
		return scoped.inner.Incubate(node)
	}
	// ast.Inspect sends nil after each subtree. NoPos also means unknown scope.
	// Passing both through preserves ooze behavior and prevents a silent drop.
	if node == nil || node.Pos() == token.NoPos {
		return scoped.inner.Incubate(node)
	}
	if !scoped.ranges.Contains(int(node.Pos()) - 1) {
		scoped.counter.dropped.Add(1)
		return nil
	}
	scoped.counter.kept.Add(1)
	return scoped.inner.Incubate(node)
}

// ScopeAll applies one range set to every ooze mutator.
func ScopeAll(all []viruses.Virus, ranges OffsetRanges, counter *ScopeCounter) []viruses.Virus {
	scoped := make([]viruses.Virus, 0, len(all))
	for _, virus := range all {
		scoped = append(scoped, NewLineScoped(virus, ranges, counter))
	}
	return scoped
}

// ScopeStats proves the filter inspected nodes and found executable infections.
type ScopeStats struct {
	Candidates int
	Kept       int
	Dropped    int
}

// AnalyzeSource runs ooze's mutator selection without applying infections. This
// reachable preflight happens before ooze.Release, whose internal t.Fatal can
// otherwise hide a zero-execution run behind the generic score failure.
func AnalyzeSource(name string, content []byte, ranges OffsetRanges) (ScopeStats, error) {
	tree, err := parser.ParseFile(token.NewFileSet(), name, content, parser.ParseComments|parser.AllErrors)
	if err != nil {
		return ScopeStats{}, fmt.Errorf("parse %s for mutation preflight: %w", name, err)
	}
	counter := &ScopeCounter{}
	all := ScopeAll(DefaultViruses(), ranges, counter)
	candidates := 0
	ast.Inspect(tree, func(node ast.Node) bool {
		for _, virus := range all {
			candidates += len(virus.Incubate(node))
		}
		return true
	})
	return ScopeStats{Candidates: candidates, Kept: counter.Kept(), Dropped: counter.Dropped()}, nil
}
