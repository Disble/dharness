package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// EvidenceDir is the one directory dharness owns inside a repository.
const EvidenceDir = ".dharness"

// evidenceFile holds facts that cost something to obtain and cannot be derived
// from the tree.
const evidenceFile = "evidence.json"

// Evidence is what dharness knows about this repository that it could not work
// out by looking at it.
//
// It is evidence, not progress. Nothing here records "step 4 done": a step is
// always re-derived from the repository, because a progress file can claim
// something was done after someone undid it. What cannot be re-derived is a
// measurement — the number of tests a scoped mutation run executes costs a full
// initial test run to learn, and it is a property of this repository's import
// graph rather than of the machine that measured it.
//
// It lives in the repository and is meant to be committed, for the same reason:
// the fact belongs to the repository.
type Evidence struct {
	Schema string `json:"schema"`

	// ScopedMutation records the measurement and what produced it, so a reader
	// can tell whether it still describes the code in front of them.
	ScopedMutation *ScopedMutation `json:"scopedMutation,omitempty"`
}

// ScopedMutation is the answer to "what would mutating one file cost here?".
type ScopedMutation struct {
	MeasuredPath string    `json:"measuredPath"`
	RelatedTests int       `json:"relatedTests"`
	MeasuredAt   time.Time `json:"measuredAt"`
}

const evidenceSchema = "dharness.evidence/v1"

func (p Project) evidencePath() string {
	return filepath.Join(p.Root, EvidenceDir, evidenceFile)
}

// ReadEvidence returns what was recorded, or a zero value when nothing was.
// A missing or unreadable record is not an error: it means the question has
// not been answered yet, which is the normal state of a new repository.
func (p Project) ReadEvidence() Evidence {
	raw, err := os.ReadFile(p.evidencePath())
	if err != nil {
		return Evidence{Schema: evidenceSchema}
	}
	var evidence Evidence
	if json.Unmarshal(raw, &evidence) != nil {
		return Evidence{Schema: evidenceSchema}
	}
	evidence.Schema = evidenceSchema
	return evidence
}

// RecordScopedMutation persists a measurement.
func (p Project) RecordScopedMutation(path string, relatedTests int) error {
	evidence := p.ReadEvidence()
	evidence.ScopedMutation = &ScopedMutation{
		MeasuredPath: path,
		RelatedTests: relatedTests,
		MeasuredAt:   time.Now().UTC().Truncate(time.Second),
	}

	raw, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the evidence: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(p.Root, EvidenceDir), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", EvidenceDir, err)
	}
	return os.WriteFile(p.evidencePath(), append(raw, '\n'), 0o600)
}
