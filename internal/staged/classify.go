package staged

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Disble/dharness/internal/runner"
)

// emptyModuleOutput is the exact text tsc emits, under --isolatedModules,
// for a source file whose entire body erases: a bare marker forcing the file
// to be treated as a module, with no statement behind it.
const emptyModuleOutput = "export {};"

// Classification is Classify's own verdict: which scoped files still carry
// runtime code, which compile away to nothing, and — when the classifier
// itself could not be asked, or did not answer — the one sentence saying why
// nothing was dropped.
type Classification struct {
	Kept    []string
	Dropped []string

	// Notice is empty when the classifier ran to a verdict. It is set only
	// when there was no local tsc to ask, tsc exited non-zero, or tsc exited
	// zero but never wrote one candidate's own output — three shapes that
	// all answer no question about which files are types-only, so Kept
	// holds every TypeScript candidate this call was given rather than
	// whichever ones happened to produce an output.
	Notice string
}

// Classify separates staged TypeScript files whose compiled output is
// empty — types-only: a `declare` file, a types-only namespace,
// `export type *`, `declare global` — from files that still carry runtime
// code, using the project's own local tsc.
//
// files are scope-relative, forward-slash paths under snapshotSource — the
// same spelling Scope returns. A non-TypeScript file is passed through
// untouched, never handed to tsc: there is nothing here for it to classify.
// A TypeScript declaration file (.d.ts, .d.mts, .d.cts) is passed through
// the same way. Scope already excludes one; this is the second,
// defensive place that boundary is drawn, because a declaration file can
// hold no runtime code and tsc emits no output for one either — which would
// otherwise read as "the classifier could not run" rather than as the
// ordinary types-only case it actually is.
//
// tscBinary is the project's own local tsc, resolved by the caller — an
// empty string means there is none, and the classifier is unavailable.
func Classify(tscBinary, snapshotSource string, files []string) (Classification, error) {
	var kept, candidates []string
	for _, f := range files {
		if isDeclarationFile(f) || !isTypeScriptFile(f) {
			kept = append(kept, f)
			continue
		}
		candidates = append(candidates, f)
	}
	if len(candidates) == 0 {
		return Classification{Kept: kept}, nil
	}
	if tscBinary == "" {
		return Classification{
			Kept:   append(kept, candidates...),
			Notice: "types-only classifier unavailable: no local tsc binary found",
		}, nil
	}

	outDir, err := os.MkdirTemp("", "dh-tsc-out-")
	if err != nil {
		return Classification{}, fmt.Errorf("create the classifier's own output directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(outDir) }()

	// A fresh, empty cwd outside the project and the snapshot: TS 6.0.2
	// errors with TS5112 (exit 1) when files are named on the command line
	// and a tsconfig.json sits in the cwd or any ancestor directory, and the
	// snapshot carries the project's own tracked tsconfig.json.
	cwd, err := os.MkdirTemp("", "dh-tsc-")
	if err != nil {
		return Classification{}, fmt.Errorf("create the classifier's own working directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(cwd) }()

	flags := []string{
		"--noCheck", "--isolatedModules", "--noResolve", "--skipLibCheck",
		"--removeComments", "--module", "esnext", "--target", "esnext",
		"--jsx", "preserve", "--declaration", "false", "--sourceMap", "false",
		"--rootDir", snapshotSource, "--outDir", outDir,
	}
	paths := make([]string, 0, len(candidates))
	for _, f := range candidates {
		paths = append(paths, filepath.Join(snapshotSource, filepath.FromSlash(f)))
	}

	// One invocation per batch, all writing into the same outDir, and every
	// batch runs before any output is read: a batch that fails leaves the
	// whole change unclassified, never the files it happened to carry.
	for _, batch := range tscBatches(paths) {
		args := append(append([]string{}, flags...), batch...)
		cmd := runner.Command{Label: "tsc", Name: tscBinary, Args: args, Dir: cwd}
		var transcript bytes.Buffer
		if err := runner.Run(cmd, &transcript, &transcript); err != nil {
			code := -1
			var exit *runner.ExitError
			if errors.As(err, &exit) {
				code = exit.Code
			}
			return Classification{
				Kept:   append(kept, candidates...),
				Notice: fmt.Sprintf("types-only classifier unavailable: tsc exited %d", code),
			}, nil
		}
	}

	// Every output is read before any drop decision is made: a candidate
	// tsc never wrote for is a failure of the whole run, not of that one
	// file, so a missing output must not let the files that did get one
	// drop while the one that did not silently stays kept for the wrong
	// reason.
	outputs := make(map[string]string, len(candidates))
	for _, f := range candidates {
		out := filepath.Join(outDir, filepath.FromSlash(compiledOutputName(f)))
		data, err := os.ReadFile(out)
		if err != nil {
			return Classification{
				Kept:   append(kept, candidates...),
				Notice: fmt.Sprintf("types-only classifier unavailable: tsc emitted no output for %s", f),
			}, nil
		}
		outputs[f] = string(data)
	}

	var dropped []string
	for _, f := range candidates {
		trimmed := strings.TrimSpace(outputs[f])
		if trimmed == "" || trimmed == emptyModuleOutput {
			dropped = append(dropped, f)
			continue
		}
		kept = append(kept, f)
	}

	return Classification{Kept: kept, Dropped: dropped}, nil
}

// isTypeScriptFile reports whether path is one of the four extensions tsc
// itself compiles: .ts, .tsx, .mts, .cts. A declaration file also matches
// this — Classify checks isDeclarationFile first rather than relying on this
// function to exclude it.
func isTypeScriptFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".ts") ||
		strings.HasSuffix(lower, ".tsx") ||
		strings.HasSuffix(lower, ".mts") ||
		strings.HasSuffix(lower, ".cts")
}

// compiledOutputName maps a source path to the name tsc writes its compiled
// output under, preserving the file's own directory. Measured against a
// real 518-file frontend with --jsx preserve: 0 missing outputs across every
// shape, .tsx included — preserve leaves JSX syntax in the emitted file, so
// its own output keeps the .jsx extension rather than becoming .js.
func compiledOutputName(path string) string {
	switch {
	case strings.HasSuffix(path, ".tsx"):
		return strings.TrimSuffix(path, ".tsx") + ".jsx"
	case strings.HasSuffix(path, ".mts"):
		return strings.TrimSuffix(path, ".mts") + ".mjs"
	case strings.HasSuffix(path, ".cts"):
		return strings.TrimSuffix(path, ".cts") + ".cjs"
	case strings.HasSuffix(path, ".ts"):
		return strings.TrimSuffix(path, ".ts") + ".js"
	}
	return path
}

// tscPathBudget is how many characters of quoted candidate paths one tsc
// invocation may carry.
//
// tsc is reached through its npm .cmd shim, so cmd.exe re-parses the whole
// command line, and cmd.exe refuses a line longer than 8191 characters.
// Measured on Windows with this package's own invocation, candidates of about
// 240 characters apiece: 20 of them made a 5,862-character line and
// classified; 45 made 12,437 and cmd.exe answered "The command line is too
// long." with exit 1. A 150-file staged change reached tsc as one line of
// about 26,000 characters and the classifier went unavailable for the whole
// commit. The 2,191 characters left over are for the rest of the line — the
// shim's own path, the flags, --rootDir and --outDir — which a deep checkout
// lengthens. The platforms without that cap only pay one more tsc start per
// batch.
const tscPathBudget = 6000

// tscBatches splits absolute candidate paths, in order, into groups whose
// quoted length stays within tscPathBudget. A quoted path costs its length
// plus its two quotes and the space before it.
//
// A path longer than the whole budget still gets a batch, alone: dropping it
// would only move the failure somewhere quieter, and cmd.exe refuses it loudly.
// paths is never empty — Classify returns before asking for batches when there
// is no candidate — so the final batch always holds at least one path.
func tscBatches(paths []string) [][]string {
	var batches [][]string
	var current []string
	size := 0
	for _, path := range paths {
		cost := len(path) + len(` ""`)
		if len(current) > 0 && size+cost > tscPathBudget {
			batches = append(batches, current)
			current, size = nil, 0
		}
		current = append(current, path)
		size += cost
	}
	return append(batches, current)
}
