package staged

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Slice B1 (mutate-staged-v1.9): the strict MSP framing codec.
//
// Stryker's `serve stdio` speaks JSON-RPC 2.0 framed like LSP: exactly one
// ASCII `Content-Length: <decimal>` header, an empty-line separator, then the
// declared body bytes. The codec is deliberately strict: anything that is not
// exactly one well-formed frame is an error, never an empty discovery result.
// Split reads reassemble and coalesced reads separate because headers are
// parsed line by line and the body is read to the declared byte count.
//
// The parser reads only from the reader it is given. Server stderr is drained
// on a separate stream by the process owner and never enters this parser.
//
// maxMSPFrameBytes bounds a single frame so a corrupt or hostile length can
// neither allocate absurdly nor wait forever: anything larger is an overflow
// error before a single body byte is read.
const maxMSPFrameBytes = 64 << 20

// mspWriteFrame writes one framed message: the exact header, the separator,
// then the body bytes unchanged.
func mspWriteFrame(w io.Writer, body []byte) error {
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return fmt.Errorf("write the MSP frame header: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("write the MSP frame body: %w", err)
	}
	return nil
}

// mspReadFrame reads exactly one framed message. The bufio.Reader stays with
// the caller, so bytes after the frame remain available for the next read.
func mspReadFrame(r *bufio.Reader) ([]byte, error) {
	line, err := readMSPLine(r)
	if err != nil {
		return nil, err
	}
	length, err := parseMSPContentLength(line)
	if err != nil {
		return nil, err
	}
	separator, err := readMSPLine(r)
	if err != nil {
		return nil, err
	}
	if separator != "" {
		return nil, fmt.Errorf("MSP frame has a second header line %q where the empty separator belongs: it is not one well-formed frame", separator)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("MSP frame declares %d body bytes but the stream ended first: %w", length, err)
	}
	return body, nil
}

// readMSPLine reads one header line, tolerating a trailing carriage return so
// CRLF and LF framings both parse; the line content itself must be exact.
func readMSPLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("MSP frame ended before its header completed: %w", err)
	}
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), nil
}

// parseMSPContentLength accepts only exactly `Content-Length: <decimal>`:
// exact spelling, digits only, fitting in memory and under the frame bound.
func parseMSPContentLength(line string) (int, error) {
	const prefix = "Content-Length: "
	digits, ok := strings.CutPrefix(line, prefix)
	if !ok || digits == "" {
		return 0, fmt.Errorf("MSP frame starts with %q instead of one `Content-Length: <decimal>` header", line)
	}
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("MSP Content-Length %q is not decimal digits", digits)
		}
	}
	length, err := strconv.Atoi(digits)
	if err != nil || length > maxMSPFrameBytes {
		return 0, fmt.Errorf("MSP Content-Length %q overflows the %d-byte frame bound", digits, maxMSPFrameBytes)
	}
	return length, nil
}

// mspRequest is one JSON-RPC request. IDs increase monotonically per client;
// the codec only correlates them, it never mints them.
type mspRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// mspResponse is the response envelope the codec validates. Result stays raw:
// membership decoding belongs to the discovery client, not the transport.
type mspResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *mspError       `json:"error"`
}

type mspError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *mspError) Error() string {
	return fmt.Sprintf("Stryker MSP error %d: %s", e.Code, e.Message)
}

// mspEncodeRequest renders one request body with the caller's ID.
func mspEncodeRequest(id int, method string, params any) ([]byte, error) {
	raw, err := json.Marshal(mspRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	if err != nil {
		return nil, fmt.Errorf("encode the MSP %s request: %w", method, err)
	}
	return raw, nil
}

// mspDecodeResponse validates one response body against the request ID and
// returns its result. A protocol error response is a failure, never an empty
// result; a valid empty result still decodes, so the caller — not the codec —
// classifies it.
func mspDecodeResponse(body []byte, wantID int) (json.RawMessage, error) {
	var res mspResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("MSP response is not JSON: %w", err)
	}
	if res.JSONRPC != "2.0" {
		return nil, fmt.Errorf("MSP response declares JSON-RPC %q, want \"2.0\"", res.JSONRPC)
	}
	if res.ID == nil || *res.ID != wantID {
		return nil, fmt.Errorf("MSP response answers id %v, want %d: it cannot be correlated", idString(res.ID), wantID)
	}
	if res.Error != nil {
		return nil, res.Error
	}
	return res.Result, nil
}

func idString(id *int) any {
	if id == nil {
		return nil
	}
	return *id
}
