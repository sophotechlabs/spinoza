package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// helpers

func as[T any](t *testing.T, value any) T {
	t.Helper()
	found, ok := value.(T)
	if !ok {
		t.Fatalf("value is %T, want %T", value, found)
	}
	return found
}

func ask(t *testing.T, server *Server, body string) map[string]any {
	t.Helper()
	raw := server.Handle(context.Background(), []byte(body))
	if raw == nil {
		t.Fatalf("no reply to %s", body)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("the reply is not JSON: %v", err)
	}
	return out
}

func errorOf(t *testing.T, reply map[string]any) map[string]any {
	t.Helper()
	found, held := reply["error"].(map[string]any)
	if !held {
		t.Fatalf("the reply carries no error: %v", reply)
	}
	return found
}

func resultOf(t *testing.T, reply map[string]any) map[string]any {
	t.Helper()
	found, held := reply["result"].(map[string]any)
	if !held {
		t.Fatalf("the reply carries no result: %v", reply)
	}
	return found
}

// what the server refuses before it does anything

func TestAMessageThatIsNotJSONIsRefusedAsParseError(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})

	reply := ask(t, server, "{not json")

	if code := errorOf(t, reply)["code"]; code != float64(codeParse) {
		t.Fatalf("code = %v, want %d", code, codeParse)
	}
}

func TestAMessageOfTheWrongProtocolIsRefused(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})

	reply := ask(t, server, `{"jsonrpc":"1.0","id":1,"method":"ping"}`)

	found := errorOf(t, reply)
	if found["code"] != float64(codeInvalidRequest) {
		t.Fatalf("code = %v, want %d", found["code"], codeInvalidRequest)
	}
	if !strings.Contains(as[string](t, found["message"]), jsonRPCVersion) {
		t.Fatalf("message = %q, want it to name the version it wants", found["message"])
	}
}

func TestAMethodNobodyServesIsRefused(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})

	reply := ask(t, server, `{"jsonrpc":"2.0","id":1,"method":"cluster/destroy"}`)

	found := errorOf(t, reply)
	if found["code"] != float64(codeMethodNotFound) {
		t.Fatalf("code = %v, want %d", found["code"], codeMethodNotFound)
	}
	if !strings.Contains(as[string](t, found["message"]), "cluster/destroy") {
		t.Fatalf("message = %q, want it to name the method", found["message"])
	}
}

func TestANotificationIsAnsweredWithSilence(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})

	reply := server.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))

	if reply != nil {
		t.Fatalf("a notification drew a reply: %s", reply)
	}
}

func TestTheIDComesBackExactlyAsItWasSent(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})

	cases := []struct {
		name string
		id   string
	}{
		{name: "a number", id: `7`},
		{name: "a string", id: `"call-7"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := server.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":`+tc.id+`,"method":"ping"}`))
			if !strings.Contains(string(raw), `"id":`+tc.id) {
				t.Fatalf("reply = %s, want it to echo id %s", raw, tc.id)
			}
		})
	}
}

// what the handshake says

func TestInitializeNamesTheProtocolAndWhatItServes(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{Version: "v1.2.3", Context: "p-mk1"})

	result := resultOf(t, ask(t, server, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))

	if result["protocolVersion"] != protocolVersion {
		t.Fatalf("protocolVersion = %v, want %s", result["protocolVersion"], protocolVersion)
	}
	info := as[map[string]any](t, result["serverInfo"])
	if info["name"] != serverName || info["version"] != "v1.2.3" {
		t.Fatalf("serverInfo = %v", info)
	}
	capable := as[map[string]any](t, result["capabilities"])
	if _, held := capable["tools"]; !held {
		t.Fatal("the handshake does not offer tools")
	}
	if _, held := capable["resources"]; !held {
		t.Fatal("the handshake does not offer resources")
	}
	instructions := as[string](t, result["instructions"])
	if !strings.Contains(instructions, "p-mk1") {
		t.Fatalf("instructions = %q, want the context named", instructions)
	}
	if !strings.Contains(instructions, "Secret values are never returned") {
		t.Fatalf("instructions = %q, want the secret rule stated", instructions)
	}
}

func TestTheHandshakeSaysWhenTheClusterIsProtected(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{Protected: true, AllowWrite: true})

	result := resultOf(t, ask(t, server, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))

	if !strings.Contains(as[string](t, result["instructions"]), "protected") {
		t.Fatalf("instructions = %q, want the protection said out loud", result["instructions"])
	}
}

func TestPingAnswersWithNothingToSay(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})

	if len(resultOf(t, ask(t, server, `{"jsonrpc":"2.0","id":1,"method":"ping"}`))) != 0 {
		t.Fatal("ping answered with a body")
	}
}

// what the tool list carries

func TestEveryToolCardCarriesASchemaAndAnnotations(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{AllowWrite: true})

	for _, card := range server.cards() {
		if card.Description == "" {
			t.Fatalf("%s has no description", card.Name)
		}
		if card.InputSchema.Type != "object" {
			t.Fatalf("%s has schema type %q, want object", card.Name, card.InputSchema.Type)
		}
		if card.InputSchema.Properties == nil {
			t.Fatalf("%s has no properties object; a client cannot build a call", card.Name)
		}
		for _, needed := range card.InputSchema.Required {
			if _, described := card.InputSchema.Properties[needed]; !described {
				t.Fatalf("%s requires %q but never describes it", card.Name, needed)
			}
		}
		if card.Annotations.ReadOnlyHint == writeToolNames[card.Name] {
			t.Fatalf("%s is annotated readOnly=%v, which contradicts the write list",
				card.Name, card.Annotations.ReadOnlyHint)
		}
	}
}

func TestTheToolListIsSortedSoClientsSeeAStableOrder(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{AllowWrite: true})

	names := server.Tools()

	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Fatalf("tools are not sorted at %d: %q then %q", i, names[i-1], names[i])
		}
	}
}

func TestCallingATheServerDoesNotServeSaysSo(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})

	reply := ask(t, server, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"delete_everything"}}`)

	if !strings.Contains(as[string](t, errorOf(t, reply)["message"]), "delete_everything") {
		t.Fatalf("message = %v", errorOf(t, reply)["message"])
	}
}

func TestCallParametersThatAreNotAnObjectAreRefused(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})

	reply := ask(t, server, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":"nonsense"}`)

	if errorOf(t, reply)["code"] != float64(codeInvalidParams) {
		t.Fatalf("code = %v, want %d", errorOf(t, reply)["code"], codeInvalidParams)
	}
}

func TestAToolThatFailsAnswersWithAnErrorResultNotAProtocolError(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})

	reply := ask(t, server, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search","arguments":{}}}`)

	result := resultOf(t, reply)
	if failed := as[bool](t, result["isError"]); !failed {
		t.Fatalf("result = %v, want isError so the model can read the reason", result)
	}
	contents := as[[]any](t, result["content"])
	body := as[string](t, as[map[string]any](t, contents[0])["text"])
	if !strings.Contains(body, "query is required") {
		t.Fatalf("text = %q, want it to name the missing argument", body)
	}
}
