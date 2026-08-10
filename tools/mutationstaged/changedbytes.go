package main

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var hunkHeaderPattern = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

type lineRange struct{ first, last int }
type offsetRange struct{ start, end int }

func parseChangedLineRanges(diff string) (map[string][]lineRange, error) {
	return parseChangedLineRangesForFile(diff, "")
}

func parseChangedLineRangesForFile(diff, knownFile string) (map[string][]lineRange, error) {
	changed := map[string][]lineRange{}
	currentFile := knownFile
	for line := range strings.SplitSeq(diff, "\n") {
		if header, ok := strings.CutPrefix(line, "+++ "); ok && knownFile == "" {
			currentFile = strings.TrimPrefix(strings.TrimSuffix(header, "\r"), "b/")
			continue
		}
		matches := hunkHeaderPattern.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		if currentFile == "" || currentFile == "/dev/null" {
			return nil, fmt.Errorf("hunk header %q appeared before a destination file", line)
		}
		start, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil, fmt.Errorf("parse hunk start in %q: %w", line, err)
		}
		count := 1
		if matches[2] != "" {
			count, err = strconv.Atoi(matches[2])
			if err != nil {
				return nil, fmt.Errorf("parse hunk count in %q: %w", line, err)
			}
		}
		if count == 0 {
			changed[currentFile] = append(changed[currentFile], lineRange{first: max(start, 1), last: start + 1})
		} else {
			changed[currentFile] = append(changed[currentFile], lineRange{first: start, last: start + count - 1})
		}
	}
	return changed, nil
}

func lineRangesToOffsets(content []byte, ranges []lineRange) []offsetRange {
	starts := []int{0}
	for index, value := range content {
		if value == '\n' && index+1 < len(content) {
			starts = append(starts, index+1)
		}
	}
	offsets := make([]offsetRange, 0, len(ranges))
	for _, span := range ranges {
		if span.first > len(starts) {
			continue
		}
		start := starts[max(span.first, 1)-1]
		end := len(content)
		if span.last < len(starts) {
			end = starts[span.last]
		}
		if end > start {
			offsets = append(offsets, offsetRange{start: start, end: end})
		}
	}
	return offsets
}

func mergeOffsetRanges(ranges []offsetRange) []offsetRange {
	if len(ranges) == 0 {
		return nil
	}
	sorted := append([]offsetRange(nil), ranges...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].start == sorted[j].start {
			return sorted[i].end < sorted[j].end
		}
		return sorted[i].start < sorted[j].start
	})
	merged := []offsetRange{sorted[0]}
	for _, next := range sorted[1:] {
		last := &merged[len(merged)-1]
		if next.start <= last.end {
			last.end = max(last.end, next.end)
			continue
		}
		merged = append(merged, next)
	}
	return merged
}

func encodeOffsetRanges(ranges []offsetRange) string {
	var encoded bytes.Buffer
	for index, span := range ranges {
		if index > 0 {
			encoded.WriteByte(',')
		}
		fmt.Fprintf(&encoded, "%d-%d", span.start, span.end)
	}
	return encoded.String()
}
