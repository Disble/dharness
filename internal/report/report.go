// Package report holds what one dharness run learned, as a value that is
// built once and rendered twice: a human view on stdout, and the same
// analysis as JSON under --format json. It reads nothing and writes no
// file — every field is filled in by its caller.
package report

import "encoding/json"

// Status is what this run did about one step. It is a string rather than an
// int enum because encoding/json marshals a named int as a number: an int
// Status would need a MarshalJSON method, which is a second representation
// of the same fact and exactly the duplicate-renderer defect this change
// exists to end. A string set marshals to the spec's own words with no
// method at all, and its zero value ("") is not a member — so an unfilled
// status is detectably invalid rather than silently "applied".
type Status string

const (
	Applied    Status = "applied"
	Delegated  Status = "delegated"
	Satisfied  Status = "satisfied"
	Failed     Status = "failed"
	NotReached Status = "not-reached"
	Retracted  Status = "retracted"
)

type Kind string

const (
	Created   Kind = "created"
	Modified  Kind = "modified"
	Unchanged Kind = "unchanged"
)

type Report struct {
	Version  string       `json:"version"`
	Root     string       `json:"root"`
	Source   string       `json:"source,omitempty"`
	Summary  Summary      `json:"summary"`
	Steps    []StepResult `json:"steps"`
	Notes    []Note       `json:"notes,omitempty"`
	Evidence *Evidence    `json:"evidence,omitempty"`
	Rollback *Rollback    `json:"rollback,omitempty"`

	// Exit is runner.ExitCode applied to the error RunSync is about to
	// return. It is assigned, never computed here: a report that decided
	// its own verdict would be the gate reporting its own status instead
	// of the tool's (§11, §17).
	Exit int `json:"exit"`
}

// Summary carries no omitempty. A zero count is a measured zero, and a
// missing key would read as "never counted" — the same absent-versus-empty
// distinction Effective depends on, applied in the other direction.
type Summary struct {
	Steps     int   `json:"steps"`
	Applied   int   `json:"applied"`
	Delegated int   `json:"delegated"`
	Satisfied int   `json:"satisfied"`
	Failed    int   `json:"failed"`
	Retracted int   `json:"retracted"`
	MS        int64 `json:"ms"`
}

type StepResult struct {
	N int `json:"n"`

	// ID is Step.ID() verbatim — the heading, which is what that method
	// has always been ("ID names the step for a report",
	// internal/setup/setup.go:24) and what the approved report's human
	// view prints for every applied and satisfied step. It is a sentence,
	// deliberately, and Decision 6 keeps it one.
	//
	// The addressable handle lives on Collision.ID, which is the only
	// thing in this report anything points at: the closing block's `next`.
	// Giving all eleven steps a slug is a follow-up the JSON twin can take
	// whenever check and mutate need one, and nothing here depends on it.
	ID string `json:"id"`

	Status     Status       `json:"status"`
	MS         int64        `json:"ms"`
	Evidence   string       `json:"evidence,omitempty"`
	Why        string       `json:"why,omitempty"`
	Wrote      []FileChange `json:"wrote,omitempty"`
	Installed  []string     `json:"installed,omitempty"`
	Collisions []Collision  `json:"collisions,omitempty"`
	Error      string       `json:"error,omitempty"`

	// Transcript is the bytes this step's Apply produced. It is rendered
	// under the step in the human view and excluded from JSON: the machine
	// reader gets Installed, a fact, and shipping raw subprocess bytes into
	// a JSON field invites a consumer to parse them, which is the
	// re-parsing §01 and §09 forbid.
	Transcript string `json:"-"`
}

type FileChange struct {
	Path string `json:"path"`
	Kind Kind   `json:"kind"`
}

type Collision struct {
	// ID is the stable, addressable handle the closing block points at,
	// in fallow's own `dup:c064407b` shape. It is derived from Key, so a
	// key can never carry two ids.
	ID          string   `json:"id"`
	Key         string   `json:"key"`
	Ours        Declared `json:"ours"`
	Theirs      Declared `json:"theirs"`
	Effective   *string  `json:"effective,omitempty"`
	Resolutions []string `json:"resolutions"`
}

// Declared is one side of a collision. Value is a pointer to raw JSON so
// that "fallow could not be asked" is an absent key rather than an empty
// object, and so the value round-trips whole instead of being re-encoded
// through a Go type dharness does not own.
type Declared struct {
	Path  string           `json:"path"`
	Line  int              `json:"line,omitempty"`
	Value *json.RawMessage `json:"value,omitempty"`
}

type Note struct {
	Kind       string   `json:"kind"`
	Path       string   `json:"path,omitempty"`
	Entries    []string `json:"entries,omitempty"`
	Actionable bool     `json:"actionable"`
	Reason     string   `json:"reason"`
}

type Evidence struct {
	RelatedTests int    `json:"relatedTests"`
	MeasuredPath string `json:"measuredPath"`
}

// Rollback is set only when a step failed and Writer.Undo ran. Retracted
// carries no omitempty: an empty list on a rolled-back run is a claim, and
// the whole point of this block is that a claim already printed gets
// withdrawn by name (defect 3).
type Rollback struct {
	Retracted []string `json:"retracted"`
	Restored  []string `json:"restored,omitempty"`
	Removed   []string `json:"removed,omitempty"`
	Left      []string `json:"left,omitempty"`
}
