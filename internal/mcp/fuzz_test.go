package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
)

func FuzzProtocol(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`),
		[]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`),
		[]byte(`{"jsonrpc":"1.0","id":null,"method":"ping"}`),
		[]byte(`{"jsonrpc":"2.0","id":"x","method":"tools/call","params":null}`),
		[]byte(`not json`),
	} {
		f.Add(seed)
	}

	server := serverFor(&fakeCluster{}, Options{})
	f.Fuzz(func(t *testing.T, raw []byte) {
		reply := server.Handle(context.Background(), raw)
		if reply == nil {
			return
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(reply, &body); err != nil {
			t.Fatalf("reply is not JSON: %v", err)
		}
		var version string
		if err := json.Unmarshal(body["jsonrpc"], &version); err != nil {
			t.Fatalf("reply has no protocol version: %v", err)
		}
		if version != jsonRPCVersion {
			t.Fatalf("protocol version = %q", version)
		}
		_, hasResult := body["result"]
		_, hasError := body["error"]
		if hasResult == hasError {
			t.Fatalf("reply must carry exactly one of result or error: %s", reply)
		}
	})
}

func FuzzStdioFraming(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte("one\nsecond\n"),
		[]byte("one\r\n"),
		[]byte("without newline"),
		bytes.Repeat([]byte("x"), 70*1024),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw []byte) {
		line, err := readMessage(bufio.NewReaderSize(bytes.NewReader(raw), 64*1024))
		at := bytes.IndexByte(raw, '\n')
		if at >= 0 {
			if err != nil {
				t.Fatalf("newline-terminated message failed: %v", err)
			}
		} else if !errors.Is(err, io.EOF) {
			t.Fatalf("unterminated message error = %v, want EOF", err)
		}
		want := raw
		if at >= 0 {
			want = raw[:at+1]
		}
		want = bytes.TrimRight(want, "\r\n")
		if !bytes.Equal(line, want) {
			t.Fatalf("message = %q, want %q", line, want)
		}
	})
}
