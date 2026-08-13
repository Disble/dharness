package report

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestWriteJSONMatchesTheReferenceEncoding pins "--format json emits valid
// JSON... no non-JSON text precedes or follows it": WriteJSON's output is
// byte-equal to the reference encoding, and json.Valid covers the whole
// buffer.
func TestWriteJSONMatchesTheReferenceEncoding(t *testing.T) {
	r := Report{
		Version: "1.2.0",
		Root:    "D:/dev/disble/autoreas-sp/autoreas-bridge",
		Source:  "frontend",
		Summary: Summary{Steps: 11, Applied: 3, Delegated: 1, Satisfied: 7, MS: 6100},
		Steps: []StepResult{
			{N: 1, ID: "install", Status: Applied, MS: 5560,
				Wrote:     []FileChange{{Path: "frontend/package.json", Kind: Modified}},
				Installed: []string{"dharness-eslint-plugin@0.3.0"}},
		},
		Exit: 0,
	}

	var got bytes.Buffer
	if err := WriteJSON(&got, r); err != nil {
		t.Fatalf("WriteJSON() = %v", err)
	}

	var want bytes.Buffer
	enc := json.NewEncoder(&want)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		t.Fatalf("reference json.Encoder.Encode() = %v", err)
	}

	if !bytes.Equal(got.Bytes(), want.Bytes()) {
		t.Errorf("WriteJSON() = %s, want %s", got.String(), want.String())
	}
	if !json.Valid(got.Bytes()) {
		t.Errorf("WriteJSON() output is not valid JSON: %s", got.String())
	}
}
