package report

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Disble/dharness/internal/runner"
)

// TestStatusValuesMarshalToSpecWords pins the spec's six-value status
// enumeration (amended from five, design.md Decision 1). A mutant that
// renames a constant's underlying string dies here.
func TestStatusValuesMarshalToSpecWords(t *testing.T) {
	cases := []struct {
		status Status
		want   string
	}{
		{Applied, `"applied"`},
		{Delegated, `"delegated"`},
		{Satisfied, `"satisfied"`},
		{Failed, `"failed"`},
		{NotReached, `"not-reached"`},
		{Retracted, `"retracted"`},
	}

	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			got, err := json.Marshal(tc.status)
			if err != nil {
				t.Fatalf("json.Marshal(%v) = %v", tc.status, err)
			}
			if string(got) != tc.want {
				t.Errorf("json.Marshal(%v) = %s, want %s", tc.status, got, tc.want)
			}
		})
	}
}

// TestAbsentIsNotEmpty pins §09/§17's absent-vs-empty rule (design.md
// Property 3): a measurement that was not made produces a missing key, never
// a null one, and a measurement that was made produces the real key. A
// mutant that drops omitempty or fabricates a default dies here.
func TestAbsentIsNotEmpty(t *testing.T) {
	collision := Collision{
		ID:          "sync:collision/duplicates",
		Key:         "duplicates",
		Ours:        Declared{Path: ".dharness/fallow.jsonc", Line: 8},
		Theirs:      Declared{Path: "frontend/.fallowrc.json", Line: 12},
		Resolutions: []string{"delete-theirs", "move-into-ours"},
	}

	raw, err := json.Marshal(collision)
	if err != nil {
		t.Fatalf("json.Marshal(Collision) = %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(Collision) = %v", err)
	}
	if _, present := decoded["effective"]; present {
		t.Errorf("effective present with Effective == nil: %s", raw)
	}
	var ours map[string]json.RawMessage
	if err := json.Unmarshal(decoded["ours"], &ours); err != nil {
		t.Fatalf("json.Unmarshal(ours) = %v", err)
	}
	if _, present := ours["value"]; present {
		t.Errorf("value present with Declared.Value == nil: %s", decoded["ours"])
	}

	effective := "theirs"
	collision.Effective = &effective
	value := json.RawMessage(`{"minOccurrences":2,"mode":"exact","threshold":5}`)
	collision.Ours.Value = &value

	raw, err = json.Marshal(collision)
	if err != nil {
		t.Fatalf("json.Marshal(Collision) = %v", err)
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(Collision) = %v", err)
	}
	var gotEffective string
	if _, present := decoded["effective"]; !present {
		t.Fatalf("effective absent with Effective set: %s", raw)
	}
	if err := json.Unmarshal(decoded["effective"], &gotEffective); err != nil {
		t.Fatalf("json.Unmarshal(effective) = %v", err)
	}
	if gotEffective != effective {
		t.Errorf("effective = %q, want %q", gotEffective, effective)
	}
	if err := json.Unmarshal(decoded["ours"], &ours); err != nil {
		t.Fatalf("json.Unmarshal(ours) = %v", err)
	}
	if _, present := ours["value"]; !present {
		t.Errorf("value absent with Declared.Value set: %s", decoded["ours"])
	}
}

// TestSummaryCarriesNoOmitempty pins "a measured zero is a measured zero,
// not a missing key" — a zero-valued Summary still decodes with every count
// key present, the same absent-versus-empty distinction as
// TestAbsentIsNotEmpty, in the other direction.
func TestSummaryCarriesNoOmitempty(t *testing.T) {
	raw, err := json.Marshal(Summary{})
	if err != nil {
		t.Fatalf("json.Marshal(Summary{}) = %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(Summary{}) = %v", err)
	}

	for _, key := range []string{"steps", "applied", "delegated", "satisfied", "failed", "retracted", "ms"} {
		if _, present := decoded[key]; !present {
			t.Errorf("zero-valued Summary is missing key %q: %s", key, raw)
		}
	}
}

// TestReportExitIsAPlainAssignedField pins "the verdict is not computed"
// (design.md Property 2): Report.Exit carries exactly the value assigned to
// it, unaffected by Summary.Failed or Summary.Delegated. A mutant
// substituting a failed == 0 check for the assignment disagrees with the
// delegated-work case here and dies.
//
// runner.ExitCode is used rather than app.ExitCode: slice 2 moved
// ExitCode's implementation to internal/runner and left
// internal/app.ExitCode as a one-line forwarder (design.md Decision 1), and
// internal/setup now imports internal/report — so internal/app (which
// imports internal/cli, which imports internal/setup) can no longer be
// imported from this package's own tests without an import cycle. Decision
// 1 states the move changes no caller's behaviour, so this substitution
// changes nothing this test pins.
func TestReportExitIsAPlainAssignedField(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"clean run", nil, 0},
		{"tool exit code propagates", &runner.ExitError{Code: 2}, 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Report{
				Summary: Summary{Failed: 0, Delegated: 1},
				Exit:    runner.ExitCode(tc.err),
			}

			raw, err := json.Marshal(r)
			if err != nil {
				t.Fatalf("json.Marshal(Report) = %v", err)
			}
			var decoded struct {
				Exit int `json:"exit"`
			}
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatalf("json.Unmarshal(Report) = %v", err)
			}
			if decoded.Exit != tc.want {
				t.Errorf("exit = %d, want %d (Summary.Failed == 0, Summary.Delegated == 1 must not change it)",
					decoded.Exit, tc.want)
			}
		})
	}
}

// TestFileChangeKindMarshalsToSpecWords pins step-outcome's three-value Kind
// set, the same way TestStatusValuesMarshalToSpecWords pins Status.
func TestFileChangeKindMarshalsToSpecWords(t *testing.T) {
	cases := []struct {
		kind Kind
		want string
	}{
		{Created, `"created"`},
		{Modified, `"modified"`},
		{Unchanged, `"unchanged"`},
	}

	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			got, err := json.Marshal(tc.kind)
			if err != nil {
				t.Fatalf("json.Marshal(%v) = %v", tc.kind, err)
			}
			if string(got) != tc.want {
				t.Errorf("json.Marshal(%v) = %s, want %s", tc.kind, got, tc.want)
			}
		})
	}
}

// TestRollbackRetractedCarriesNoOmitempty pins Rollback.Retracted's own
// documented rule: an empty list on a rolled-back run is itself a claim
// (defect 3), so it must never vanish from the JSON the way an omitempty
// slice would.
func TestRollbackRetractedCarriesNoOmitempty(t *testing.T) {
	raw, err := json.Marshal(Rollback{})
	if err != nil {
		t.Fatalf("json.Marshal(Rollback{}) = %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(Rollback{}) = %v", err)
	}
	if _, present := decoded["retracted"]; !present {
		t.Errorf("retracted missing from a zero-valued Rollback: %s", raw)
	}
	if !reflect.DeepEqual(decoded["retracted"], json.RawMessage("null")) {
		t.Errorf("retracted = %s, want null for a nil slice", decoded["retracted"])
	}
}
