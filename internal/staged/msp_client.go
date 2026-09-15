package staged

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/Disble/dharness/internal/runner"
)

// Slice C1 (mutate-staged-v1.9): the sequential MSP configure/discover client.
//
// One request runs at a time over `stryker serve stdio`: configure naming the
// snapshot's JSON config (or empty params for Stryker's own default
// resolution), one ranged discovery over every candidate range, and — only
// for files the ranged response leaves uncovered — one path-only discovery.
// No generated mutation config is used here: discovery must observe the
// project's effective `mutate` decision before dharness narrows it, and
// `serve stdio` receives no CLI options at all.
//
// The client owns no process: it runs over streams, and the tree-close hook
// (the managed Close in production) runs on success, protocol failure, early
// server exit, and cancellation. A cleanup failure joins the primary error so
// it can never turn into successful discovery. Cancellation unblocks through
// the hook — closing the tree breaks the pending frame read and write —
// never through a timeout: discovery over a large project legitimately takes
// a while.
//
// Column coordinates follow the measured probe shape: whole-line ranges from
// column 1 to column 10000. A mutant outside that window on a boundary line
// is not requested; see the boundary matrix.

// CandidateRange is one staged runtime range to discover, snapshot-relative
// with forward slashes.
type CandidateRange struct {
	Path               string
	StartLine, EndLine int
}

// RawMutant is one discovered mutant location, exactly as Stryker reports it.
// Classification against the requested ranges belongs to membership, not to
// this transport.
type RawMutant struct {
	ID                  string
	StartLine, StartCol int
	EndLine, EndCol     int
}

// DiscoveryOutcome carries both raw responses. Ranged maps every response
// file to its mutants; PathOnlyRequested names the files the path-only call
// covered, and PathOnly maps those with a hit. An empty Files map is a valid
// ambiguous observation, never a transport error.
type DiscoveryOutcome struct {
	Ranged            map[string][]RawMutant
	PathOnlyRequested []string
	PathOnly          map[string][]RawMutant
}

type mspPosition struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

type mspRange struct {
	Start mspPosition `json:"start"`
	End   mspPosition `json:"end"`
}

type mspDiscoverFile struct {
	Path  string    `json:"path"`
	Range *mspRange `json:"range,omitempty"`
}

type mspRawMutant struct {
	ID       string `json:"id"`
	Location *struct {
		Start mspPosition `json:"start"`
		End   mspPosition `json:"end"`
	} `json:"location"`
}

type mspDiscoverResult struct {
	Files map[string]struct {
		Mutants []mspRawMutant `json:"mutants"`
	} `json:"files"`
}

// Discover runs the full sequence against a real server: it starts the serve
// command as an owned tree, runs the stream sequence over it, and closes the
// tree on every exit. Tests script discoverOver directly over pipes instead.
func Discover(ctx context.Context, serve runner.Command, configFile string, candidates []CandidateRange) (DiscoveryOutcome, error) {
	mp, err := runner.StartManaged(serve)
	if err != nil {
		return DiscoveryOutcome{Ranged: map[string][]RawMutant{}, PathOnly: map[string][]RawMutant{}}, err
	}
	return discoverOver(ctx, mp.Stdin(), mp.Stdout(), configFile, candidates, mp.Close)
}

// discoverOver runs the sequence over streams. closeTree ends the owned tree;
// it must be idempotent because cancellation and return can both reach it.
func discoverOver(ctx context.Context, stdin io.Writer, stdout *bufio.Reader, configFile string, candidates []CandidateRange, closeTree func() error) (outcome DiscoveryOutcome, retErr error) {
	stop := context.AfterFunc(ctx, func() { _ = closeTree() })
	defer stop()
	defer func() {
		if cerr := closeTree(); cerr != nil {
			if retErr != nil {
				retErr = errors.Join(retErr, cerr)
			} else {
				retErr = cerr
			}
		}
	}()

	outcome.Ranged = map[string][]RawMutant{}
	outcome.PathOnly = map[string][]RawMutant{}

	nextID := 0
	request := func(method string, params any) (json.RawMessage, error) {
		nextID++
		raw, err := mspEncodeRequest(nextID, method, params)
		if err != nil {
			return nil, err
		}
		if err := mspWriteFrame(stdin, raw); err != nil {
			return nil, fmt.Errorf("MSP %s: %w", method, err)
		}
		body, err := mspReadFrame(stdout)
		if err != nil {
			return nil, fmt.Errorf("MSP %s: %w", method, err)
		}
		result, err := mspDecodeResponse(body, nextID)
		if err != nil {
			return nil, fmt.Errorf("MSP %s: %w", method, err)
		}
		return result, nil
	}

	var configureParams any = map[string]any{}
	if configFile != "" {
		configureParams = map[string]any{"configFilePath": configFile}
	}
	if _, err := request("configure", configureParams); err != nil {
		return outcome, fmt.Errorf("MSP configure: %w", err)
	}

	rangedFiles := make([]mspDiscoverFile, 0, len(candidates))
	for _, c := range candidates {
		path := filepath.ToSlash(c.Path)
		rangedFiles = append(rangedFiles, mspDiscoverFile{
			Path: path,
			Range: &mspRange{
				Start: mspPosition{Line: c.StartLine, Column: 1},
				End:   mspPosition{Line: c.EndLine, Column: 10000},
			},
		})
	}
	rangedResult, err := request("discover", map[string]any{"files": rangedFiles})
	if err != nil {
		return outcome, fmt.Errorf("MSP ranged discover: %w", err)
	}
	known := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		known[filepath.ToSlash(c.Path)] = true
	}
	outcome.Ranged, err = decodeDiscoverResult(rangedResult, known)
	if err != nil {
		return outcome, fmt.Errorf("MSP ranged discover: %w", err)
	}

	var uncovered []string
	for _, c := range candidates {
		path := filepath.ToSlash(c.Path)
		if len(outcome.Ranged[path]) == 0 {
			uncovered = append(uncovered, path)
		}
	}
	if len(uncovered) == 0 {
		return outcome, nil
	}
	if err := ctx.Err(); err != nil {
		return outcome, fmt.Errorf("MSP path-only discover skipped, context ended: %w", err)
	}
	pathOnlyFiles := make([]mspDiscoverFile, 0, len(uncovered))
	for _, path := range uncovered {
		pathOnlyFiles = append(pathOnlyFiles, mspDiscoverFile{Path: path})
	}
	outcome.PathOnlyRequested = uncovered
	pathOnlyResult, err := request("discover", map[string]any{"files": pathOnlyFiles})
	if err != nil {
		return outcome, fmt.Errorf("MSP path-only discover: %w", err)
	}
	allowed := make(map[string]bool, len(uncovered))
	for _, path := range uncovered {
		allowed[path] = true
	}
	outcome.PathOnly, err = decodeDiscoverResult(pathOnlyResult, allowed)
	if err != nil {
		return outcome, fmt.Errorf("MSP path-only discover: %w", err)
	}
	return outcome, nil
}

// decodeDiscoverResult keeps only response files in the allowed set. An
// unexpected path fails discovery instead of widening scope, and a mutant
// without a usable location fails it instead of entering classification half
// described. An absent or empty files map decodes to nothing: omission is
// data for membership, not a transport error.
func decodeDiscoverResult(result json.RawMessage, allowed map[string]bool) (map[string][]RawMutant, error) {
	var decoded mspDiscoverResult
	if len(result) > 0 {
		if err := json.Unmarshal(result, &decoded); err != nil {
			return nil, fmt.Errorf("discovery result is not the files shape: %w", err)
		}
	}
	kept := make(map[string][]RawMutant, len(decoded.Files))
	for path, file := range decoded.Files {
		if !allowed[path] {
			return nil, fmt.Errorf("discovery reported unexpected path %q", path)
		}
		for _, m := range file.Mutants {
			if m.Location == nil {
				return nil, fmt.Errorf("discovery reported mutant %q in %q without a usable location", m.ID, path)
			}
			kept[path] = append(kept[path], RawMutant{
				ID:        m.ID,
				StartLine: m.Location.Start.Line, StartCol: m.Location.Start.Column,
				EndLine: m.Location.End.Line, EndCol: m.Location.End.Column,
			})
		}
	}
	return kept, nil
}
