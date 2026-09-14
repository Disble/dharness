package staged

// Slice B1 (mutate-staged-v1.9): strict Content-Length JSON-RPC codec over
// in-memory streams. RED first — these tests fail until
// internal/staged/msp_codec.go exists.

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestMSPWriteFrameBytesAreExact(t *testing.T) {
	var buf bytes.Buffer
	if err := mspWriteFrame(&buf, []byte(`{}`)); err != nil {
		t.Fatalf("mspWriteFrame() = %v, want nil", err)
	}
	want := "Content-Length: 2\r\n\r\n{}"
	if buf.String() != want {
		t.Errorf("frame = %q, want %q", buf.String(), want)
	}
}

func TestMSPReadFrameAcrossSplitAndCoalescedReads(t *testing.T) {
	first := "Content-Length: 2\r\n\r\n{}"
	second := "Content-Length: 7\r\n\r\n{\"a\":1}"
	stream := first + second

	for _, chunk := range []int{1, 3, len(stream)} {
		r := bufio.NewReader(newChunkReader(stream, chunk))
		for _, want := range []string{`{}`, `{"a":1}`} {
			body, err := mspReadFrame(r)
			if err != nil {
				t.Fatalf("chunk %d: mspReadFrame() = %v, want nil", chunk, err)
			}
			if string(body) != want {
				t.Fatalf("chunk %d: body = %q, want %q", chunk, body, want)
			}
		}
	}
}

func TestMSPReadFrameRejectsMalformedInput(t *testing.T) {
	cases := map[string]string{
		"stray bytes before header":   "xxContent-Length: 2\r\n\r\n{}",
		"empty input":                 "",
		"missing header":              "\r\n\r\n{}",
		"wrong header spelling lower": "content-length: 2\r\n\r\n{}",
		"wrong header spelling space": "Content-Length : 2\r\n\r\n{}",
		"wrong header name":           "Length: 2\r\n\r\n{}",
		"duplicate header":            "Content-Length: 2\r\nContent-Length: 2\r\n\r\n{}",
		"extra header":                "Content-Length: 2\r\nX-Extra: 1\r\n\r\n{}",
		"missing separator":           "Content-Length: 2\r\n{}",
		"empty length":                "Content-Length: \r\n\r\n{}",
		"non-numeric length":          "Content-Length: ab\r\n\r\n{}",
		"negative length":             "Content-Length: -2\r\n\r\n{}",
		"overflowing length":          "Content-Length: 99999999999999999999\r\n\r\n{}",
		"absurd length":               "Content-Length: 1073741824\r\n\r\n{}",
		"truncated body":              "Content-Length: 7\r\n\r\n{\"a\":",
		"body shorter than declared":  "Content-Length: 3\r\n\r\n{}",
	}
	for name, input := range cases {
		r := bufio.NewReader(strings.NewReader(input))
		if body, err := mspReadFrame(r); err == nil {
			t.Errorf("%s: mspReadFrame() = %q, want an error", name, body)
		}
	}
}

func TestMSPRequestResponseRoundTrip(t *testing.T) {
	params := map[string]any{"files": []string{"src/a.ts"}}
	raw, err := mspEncodeRequest(4, "discover", params)
	if err != nil {
		t.Fatalf("mspEncodeRequest() = %v, want nil", err)
	}
	var framed bytes.Buffer
	if err := mspWriteFrame(&framed, raw); err != nil {
		t.Fatalf("mspWriteFrame() = %v, want nil", err)
	}
	body, err := mspReadFrame(bufio.NewReader(&framed))
	if err != nil {
		t.Fatalf("mspReadFrame() = %v, want nil", err)
	}
	result, err := mspDecodeResponse(body, 4)
	if err != nil {
		t.Fatalf("mspDecodeResponse() = %v, want nil", err)
	}
	_ = result
}

func TestMSPDecodeResponseRejectsProtocolViolations(t *testing.T) {
	cases := map[string]struct {
		body   string
		wantID int
	}{
		"wrong jsonrpc version": {`{"jsonrpc":"1.0","id":1,"result":{}}`, 1},
		"missing jsonrpc":       {`{"id":1,"result":{}}`, 1},
		"mismatched id":         {`{"jsonrpc":"2.0","id":2,"result":{}}`, 1},
		"missing id":            {`{"jsonrpc":"2.0","result":{}}`, 1},
		"error response":        {`{"jsonrpc":"2.0","id":1,"error":{"code":-32600,"message":"bad"}}`, 1},
		"invalid json":          {`{"jsonrpc":`, 1},
		"empty body":            {``, 1},
	}
	for name, tc := range cases {
		if _, err := mspDecodeResponse([]byte(tc.body), tc.wantID); err == nil {
			t.Errorf("%s: mspDecodeResponse() = nil, want an error", name)
		}
	}
}

func TestMSPErrorResponseIsAFailureNeverEmptyResult(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","id":7,"error":{"code":-32601,"message":"no such method"}}`)
	_, err := mspDecodeResponse(body, 7)
	if err == nil {
		t.Fatal("mspDecodeResponse() = nil, want the protocol error")
	}
	if !strings.Contains(err.Error(), "no such method") {
		t.Errorf("err = %q, want it to carry the server message", err)
	}
}

func TestMSPParserReadsOnlyItsOwnStream(t *testing.T) {
	frames := "Content-Length: 2\r\n\r\n{}Content-Length: 2\r\n\r\n{}"
	stderr := new(bytes.Buffer)
	r := bufio.NewReader(strings.NewReader(frames))
	for i := 0; i < 2; i++ {
		if _, err := mspReadFrame(r); err != nil {
			t.Fatalf("frame %d: mspReadFrame() = %v, want nil", i, err)
		}
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr buffer holds %d bytes, want the parser to never touch it", stderr.Len())
	}
	if r.Buffered() != 0 {
		t.Errorf("reader holds %d buffered bytes, want exact frame consumption", r.Buffered())
	}
}

// newChunkReader returns a reader that yields at most chunk bytes per Read,
// proving split reads reassemble and coalesced reads separate.
func newChunkReader(s string, chunk int) io.Reader {
	return &chunkReader{data: []byte(s), chunk: chunk}
}

type chunkReader struct {
	data  []byte
	chunk int
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if len(c.data) == 0 {
		return 0, io.EOF
	}
	n := c.chunk
	if n > len(c.data) {
		n = len(c.data)
	}
	if n > len(p) {
		n = len(p)
	}
	copy(p, c.data[:n])
	c.data = c.data[n:]
	return n, nil
}

var _ = errors.New
