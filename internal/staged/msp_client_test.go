package staged

// Slice C1 (mutate-staged-v1.9): the sequential MSP configure/discover client.
// RED first — these tests fail until internal/staged/msp_client.go exists.
//
// The fake server speaks framed MSP over net.Pipe: scripted responses per
// method, so the tests prove request order, conditional path-only discovery,
// and failure handling without a real Stryker.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeMSP struct {
	t       *testing.T
	methods []string
	// respond returns the result payload per method, or an error to send a
	// protocol error response instead. pathOnlySeen records path-only files.
	respond      func(method string, params json.RawMessage) (any, error)
	pathOnlySeen []string
	mu           sync.Mutex
}

func (f *fakeMSP) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	r := bufio.NewReader(conn)
	for {
		body, err := mspReadFrame(r)
		if err != nil {
			return
		}
		var req struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      int             `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			return
		}
		f.mu.Lock()
		f.methods = append(f.methods, req.Method)
		f.mu.Unlock()
		payload, respondErr := f.respond(req.Method, req.Params)
		var resp []byte
		if respondErr != nil {
			resp, _ = json.Marshal(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"error": map[string]any{"code": -32600, "message": respondErr.Error()},
			})
		} else {
			resp, _ = json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": payload})
		}
		if err := mspWriteFrame(conn, resp); err != nil {
			return
		}
	}
}

func runFakeMSP(t *testing.T, respond func(method string, params json.RawMessage) (any, error)) (*fakeMSP, net.Conn, net.Conn) {
	t.Helper()
	fake := &fakeMSP{t: t, respond: respond}
	client, server := net.Pipe()
	go fake.serve(server)
	t.Cleanup(func() { _ = client.Close() })
	return fake, client, server
}

func filesResult(files map[string][]map[string]any) map[string]any {
	out := map[string]any{}
	for path, mutants := range files {
		out[path] = map[string]any{"mutants": mutants}
	}
	return map[string]any{"files": out}
}

func mutantAt(id string, startLine, startCol, endLine, endCol int) map[string]any {
	return map[string]any{
		"id": id,
		"location": map[string]any{
			"start": map[string]any{"line": startLine, "column": startCol},
			"end":   map[string]any{"line": endLine, "column": endCol},
		},
	}
}

func TestMSPDiscoverRunsConfigureThenRangedThenPathOnly(t *testing.T) {
	var mu sync.Mutex
	var configureParams []json.RawMessage
	var rangedFiles, pathOnlyFiles []map[string]any
	fake, client, _ := runFakeMSP(t, func(method string, params json.RawMessage) (any, error) {
		mu.Lock()
		defer mu.Unlock()
		switch method {
		case "configure":
			configureParams = append(configureParams, params)
			return map[string]any{}, nil
		case "discover":
			var p struct {
				Files []map[string]any `json:"files"`
			}
			_ = json.Unmarshal(params, &p)
			if len(pathOnlyFiles) == 0 && len(rangedFiles) == 0 {
				rangedFiles = p.Files
				return filesResult(map[string][]map[string]any{
					"src/a.ts": {mutantAt("0", 2, 3, 2, 9)},
				}), nil
			}
			pathOnlyFiles = p.Files
			return filesResult(map[string][]map[string]any{}), nil
		}
		return nil, errors.New("unexpected method")
	})

	closed := false
	outcome, err := discoverOver(context.Background(), client, bufio.NewReader(client),
		"stryker.config.json",
		[]CandidateRange{{Path: "src/a.ts", StartLine: 1, EndLine: 5}, {Path: "src/b.ts", StartLine: 1, EndLine: 3}},
		func() error { closed = true; return nil })
	if err != nil {
		t.Fatalf("discoverOver() = %v, want nil", err)
	}
	if !closed {
		t.Error("tree was not closed after successful discovery")
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.methods) != 3 || fake.methods[0] != "configure" || fake.methods[1] != "discover" || fake.methods[2] != "discover" {
		t.Errorf("methods = %v, want [configure discover discover]", fake.methods)
	}
	if len(configureParams) != 1 || !strings.Contains(string(configureParams[0]), "stryker.config.json") {
		t.Errorf("configure params = %v, want the snapshot config named", configureParams)
	}
	if len(rangedFiles) != 2 {
		t.Errorf("ranged discover covered %d files, want both candidates", len(rangedFiles))
	}
	if len(pathOnlyFiles) != 1 {
		t.Fatalf("path-only discover covered %d files, want only the uncovered one", len(pathOnlyFiles))
	}
	if pathOnlyFiles[0]["path"] != "src/b.ts" {
		t.Errorf("path-only file = %v, want src/b.ts", pathOnlyFiles[0]["path"])
	}
	if _, ok := pathOnlyFiles[0]["range"]; ok {
		t.Errorf("path-only file carries a range: %v, want path only", pathOnlyFiles[0])
	}
	if len(outcome.Ranged["src/a.ts"]) != 1 {
		t.Errorf("ranged mutants for src/a.ts = %v, want the reported mutant", outcome.Ranged)
	} else if outcome.Ranged["src/a.ts"][0].ID != "0" {
		t.Errorf("ranged mutant id = %q, want \"0\": Stryker reports string ids per the mutation-testing-report-schema", outcome.Ranged["src/a.ts"][0].ID)
	}
}

func TestMSPDiscoverSkipsPathOnlyWhenRangedCoversAll(t *testing.T) {
	fake, client, _ := runFakeMSP(t, func(method string, params json.RawMessage) (any, error) {
		if method == "configure" {
			return map[string]any{}, nil
		}
		return filesResult(map[string][]map[string]any{
			"src/a.ts": {mutantAt("0", 1, 1, 1, 5)},
		}), nil
	})
	outcome, err := discoverOver(context.Background(), client, bufio.NewReader(client), "",
		[]CandidateRange{{Path: "src/a.ts", StartLine: 1, EndLine: 5}},
		func() error { return nil })
	if err != nil {
		t.Fatalf("discoverOver() = %v, want nil", err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.methods) != 2 {
		t.Errorf("methods = %v, want exactly [configure discover]", fake.methods)
	}
	if outcome.PathOnlyRequested != nil {
		t.Errorf("path-only requested = %v, want none", outcome.PathOnlyRequested)
	}
}

func TestMSPDiscoverSendsEmptyConfigureWithoutSnapshotConfig(t *testing.T) {
	var mu sync.Mutex
	var got []json.RawMessage
	_, client, _ := runFakeMSP(t, func(method string, params json.RawMessage) (any, error) {
		mu.Lock()
		defer mu.Unlock()
		if method == "configure" {
			got = append(got, params)
			return map[string]any{}, nil
		}
		return filesResult(nil), nil
	})
	_, err := discoverOver(context.Background(), client, bufio.NewReader(client), "",
		[]CandidateRange{{Path: "src/a.ts", StartLine: 1, EndLine: 2}},
		func() error { return nil })
	if err != nil {
		t.Fatalf("discoverOver() = %v, want nil", err)
	}
	if len(got) != 1 || strings.Contains(string(got[0]), "configFilePath") {
		t.Errorf("configure params = %v, want empty params for default resolution", got)
	}
}

func TestMSPDiscoverFailsClosedAndClosesTree(t *testing.T) {
	t.Run("configure error", func(t *testing.T) {
		_, client, _ := runFakeMSP(t, func(method string, params json.RawMessage) (any, error) {
			return nil, errors.New("no such config")
		})
		closed := false
		_, err := discoverOver(context.Background(), client, bufio.NewReader(client), "gone.json",
			[]CandidateRange{{Path: "src/a.ts", StartLine: 1, EndLine: 2}},
			func() error { closed = true; return nil })
		if err == nil || !strings.Contains(err.Error(), "configure") {
			t.Errorf("err = %v, want the configure failure", err)
		}
		if !closed {
			t.Error("tree was not closed after the failure")
		}
	})

	t.Run("unexpected path", func(t *testing.T) {
		_, client, _ := runFakeMSP(t, func(method string, params json.RawMessage) (any, error) {
			if method == "configure" {
				return map[string]any{}, nil
			}
			return filesResult(map[string][]map[string]any{
				"src/elsewhere.ts": {mutantAt("2", 1, 1, 1, 2)},
			}), nil
		})
		_, err := discoverOver(context.Background(), client, bufio.NewReader(client), "",
			[]CandidateRange{{Path: "src/a.ts", StartLine: 1, EndLine: 2}},
			func() error { return nil })
		if err == nil || !strings.Contains(err.Error(), "src/elsewhere.ts") {
			t.Errorf("err = %v, want it to name the unexpected path", err)
		}
	})

	t.Run("cleanup failure cannot become success", func(t *testing.T) {
		_, client, _ := runFakeMSP(t, func(method string, params json.RawMessage) (any, error) {
			return nil, errors.New("boom")
		})
		_, err := discoverOver(context.Background(), client, bufio.NewReader(client), "",
			[]CandidateRange{{Path: "src/a.ts", StartLine: 1, EndLine: 2}},
			func() error { return errors.New("tree would not die") })
		if err == nil || !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "tree would not die") {
			t.Errorf("err = %v, want both the primary and the cleanup failure", err)
		}
	})
}

func TestMSPDiscoverCancellationClosesTreeAndUnblocks(t *testing.T) {
	_, client, _ := runFakeMSP(t, func(method string, params json.RawMessage) (any, error) {
		if method == "configure" {
			return map[string]any{}, nil
		}
		select {} // never answer discovery: the cancel must unblock the read
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(500*time.Millisecond, cancel)
	closed := make(chan struct{}, 1)
	go func() {
		// The production Discover wires the managed Close here; the test
		// closes the stream, which unblocks the pending frame read.
		<-ctx.Done()
		_ = client.Close()
		closed <- struct{}{}
	}()
	_, err := discoverOver(ctx, client, bufio.NewReader(client), "",
		[]CandidateRange{{Path: "src/a.ts", StartLine: 1, EndLine: 2}},
		func() error { return nil })
	if err == nil {
		t.Fatal("discoverOver() = nil, want the cancelled read to fail")
	}
	select {
	case <-closed:
	default:
		t.Error("cancel did not run the tree-close hook")
	}
}
