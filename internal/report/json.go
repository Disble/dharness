package report

import (
	"encoding/json"
	"io"
)

// WriteJSON renders r as the machine twin of WriteHuman: the same computed
// value, indented, with nothing else on the stream. It is decoded from the
// same Report the human rendering consumes — never a second computation.
func WriteJSON(w io.Writer, r Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
