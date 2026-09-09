package mcp

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestStdioDoesNotReplyWhenANotificationHandlerPanics(t *testing.T) {
	var out bytes.Buffer
	server := serverFor(&panickingCluster{fakeCluster: &fakeCluster{}}, Options{})
	request := `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"list_namespaces","arguments":{}}}` + "\n"

	err := server.Serve(context.Background(), strings.NewReader(request), &out)
	if err != nil {
		t.Fatalf("serve: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("notification panic produced a reply: %s", out.String())
	}
}
