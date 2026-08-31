package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestStdioAnswersOneMessagePerLine(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})
	in := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"ping"}` + "\n",
	)
	var out bytes.Buffer

	if err := server.Serve(context.Background(), in, &out); err != nil {
		t.Fatalf("serve: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("wrote %d lines, want one per request:\n%s", len(lines), out.String())
	}
	for index, line := range lines {
		var reply map[string]any
		if err := json.Unmarshal([]byte(line), &reply); err != nil {
			t.Fatalf("line %d is not JSON: %v", index, err)
		}
	}
}

func TestStdioSkipsBlankLinesAndSaysNothingToNotifications(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})
	in := strings.NewReader("\n" + `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n\n")
	var out bytes.Buffer

	if err := server.Serve(context.Background(), in, &out); err != nil {
		t.Fatalf("serve: %v", err)
	}

	if out.Len() != 0 {
		t.Fatalf("wrote %q, want nothing", out.String())
	}
}

func TestStdioStopsWhenTheContextIsDone(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer

	err := server.Serve(ctx, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n"), &out)

	if err == nil {
		t.Fatal("serve carried on after the context was canceled")
	}
}

func TestTheToolListingMarksWhatWrites(t *testing.T) {
	server := serverFor(writableCluster(), Options{AllowWrite: true})
	var out bytes.Buffer

	if err := server.List(&out); err != nil {
		t.Fatalf("list: %v", err)
	}

	body := out.String()
	if !strings.Contains(body, "get_dashboard          read") {
		t.Fatalf("listing does not mark a read tool:\n%s", body)
	}
	if !strings.Contains(body, "manage_node            write") {
		t.Fatalf("listing does not mark a write tool:\n%s", body)
	}
}

func TestCallingOneToolFromTheCommandLine(t *testing.T) {
	cluster := &fakeCluster{spaces: api.Namespaces{Names: []string{"prod"}}}
	server := serverFor(cluster, Options{})
	var out bytes.Buffer

	if err := server.Call(context.Background(), &out, "list_namespaces", nil); err != nil {
		t.Fatalf("call: %v", err)
	}

	if !strings.Contains(out.String(), `"name": "prod"`) {
		t.Fatalf("output = %s", out.String())
	}
}

func TestCallingWithArgumentsFromTheCommandLine(t *testing.T) {
	cluster := &fakeCluster{catalog: catalogOf(descriptor("", "v1", "pods", "Pod"))}
	server := serverFor(cluster, Options{})
	var out bytes.Buffer

	err := server.Call(context.Background(), &out, "list_resources", []string{"resource=pods", "namespace=prod"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if cluster.lastKind.Resource != "pods" {
		t.Fatalf("resource = %q", cluster.lastKind.Resource)
	}
}

func TestCommandLineArgumentsCarryTheirType(t *testing.T) {
	cases := []struct {
		name  string
		pair  string
		key   string
		value any
	}{
		{name: "a word", pair: "resource=pods", key: "resource", value: "pods"},
		{name: "a number", pair: "limit=5", key: "limit", value: float64(5)},
		{name: "true", pair: "events=true", key: "events", value: true},
		{name: "false", pair: "events=false", key: "events", value: false},
		{name: "a value with an equals in it", pair: "query=up==1", key: "query", value: "up==1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args, err := parsePairs([]string{tc.pair})
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if args[tc.key] != tc.value {
				t.Fatalf("%s = %#v, want %#v", tc.key, args[tc.key], tc.value)
			}
		})
	}
}

func TestAnArgumentWithNoEqualsIsRefused(t *testing.T) {
	if _, err := parsePairs([]string{"resource"}); err == nil {
		t.Fatal("a bare word was accepted as an argument")
	}
}

func TestCallingSomethingTheServerDoesNotServe(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})
	var out bytes.Buffer

	err := server.Call(context.Background(), &out, "manage_node", nil)

	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("error = %v, want the read-only reason", err)
	}
}

func TestCallingAToolThatFailsReportsIt(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})
	var out bytes.Buffer

	if err := server.Call(context.Background(), &out, "search", nil); err == nil {
		t.Fatal("a failing tool reported success")
	}
	if _, err := parsePairs([]string{"a=b"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
}

func TestEveryResourceIsListedWithAURIAndAType(t *testing.T) {
	for _, card := range resourceCards() {
		if !strings.HasPrefix(card.URI, "cluster://") {
			t.Fatalf("%s does not look like a cluster resource", card.URI)
		}
		if card.MimeType != jsonMime {
			t.Fatalf("%s is %q, want %s", card.URI, card.MimeType, jsonMime)
		}
		if card.Name == "" || card.Description == "" {
			t.Fatalf("%s is unlabelled", card.URI)
		}
	}
}

func TestReadingEachResource(t *testing.T) {
	cluster := &fakeCluster{
		overview: api.ClusterOverview{Version: "v1.30.0"},
		graph:    api.Graph{Nodes: []api.GraphNode{{ID: "a", Name: "web"}}},
		events:   []api.Event{{Reason: "Pulled", Message: "ok"}},
	}
	server := serverFor(cluster, Options{})

	for _, card := range resourceCards() {
		t.Run(card.URI, func(t *testing.T) {
			body, err := server.resourceBody(context.Background(), card.URI)
			if err != nil {
				t.Fatalf("read %s: %v", card.URI, err)
			}
			if body == nil {
				t.Fatalf("%s came back empty", card.URI)
			}
		})
	}
}

func TestReadingAResourceNobodyServes(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})

	_, err := server.resourceBody(context.Background(), "cluster://secrets")

	if err == nil || !strings.Contains(err.Error(), "cluster://secrets") {
		t.Fatalf("error = %v, want the uri named", err)
	}
}

func TestReadingAResourceOverTheProtocol(t *testing.T) {
	server := serverFor(&fakeCluster{overview: api.ClusterOverview{Version: "v1.30.0"}}, Options{})

	reply := ask(t, server, `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"cluster://health"}}`)

	contents := as[[]any](t, resultOf(t, reply)["contents"])
	first := as[map[string]any](t, contents[0])
	if first["mimeType"] != jsonMime {
		t.Fatalf("mimeType = %v", first["mimeType"])
	}
	if !strings.Contains(as[string](t, first["text"]), "v1.30.0") {
		t.Fatalf("text = %v", first["text"])
	}
}

func TestReadParametersThatAreNotAnObjectAreRefused(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})

	reply := ask(t, server, `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":[]}`)

	if errorOf(t, reply)["code"] != float64(codeInvalidParams) {
		t.Fatalf("code = %v", errorOf(t, reply)["code"])
	}
}

func TestAskingForAResourceThatIsNotThereOverTheProtocol(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})

	reply := ask(t, server, `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"cluster://nope"}}`)

	if errorOf(t, reply)["code"] != float64(codeInvalidParams) {
		t.Fatalf("code = %v", errorOf(t, reply)["code"])
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errRefused }

func TestAWriteFailureIsReportedRatherThanSwallowed(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})

	if err := server.List(brokenWriter{}); !errors.Is(err, errRefused) {
		t.Fatalf("list error = %v", err)
	}
	if err := server.Call(context.Background(), brokenWriter{}, "list_namespaces", nil); !errors.Is(err, errRefused) {
		t.Fatalf("call error = %v", err)
	}
	err := server.Serve(context.Background(), strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n"), brokenWriter{})
	if !errors.Is(err, errRefused) {
		t.Fatalf("serve error = %v", err)
	}
}

func TestListingToolsOverTheProtocol(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})

	result := resultOf(t, ask(t, server, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))

	tools := as[[]any](t, result["tools"])
	if len(tools) == 0 {
		t.Fatal("tools/list came back empty")
	}
	first := as[map[string]any](t, tools[0])
	if first["name"] == nil || first["inputSchema"] == nil {
		t.Fatalf("card = %v", first)
	}
}

func TestListingResourcesOverTheProtocol(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})

	result := resultOf(t, ask(t, server, `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`))

	if len(as[[]any](t, result["resources"])) != len(resourceCards()) {
		t.Fatalf("resources = %v", result["resources"])
	}
}

func TestASuccessfulCallComesBackAsTextContent(t *testing.T) {
	cluster := &fakeCluster{spaces: api.Namespaces{Names: []string{"prod"}}}
	server := serverFor(cluster, Options{})

	result := resultOf(t, ask(t, server,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_namespaces","arguments":{}}}`))

	if result["isError"] != nil {
		t.Fatalf("result = %v, want no error flag", result)
	}
	contents := as[[]any](t, result["content"])
	body := as[map[string]any](t, contents[0])
	if body["type"] != "text" {
		t.Fatalf("content type = %v", body["type"])
	}
	if !strings.Contains(as[string](t, body["text"]), "prod") {
		t.Fatalf("text = %v", body["text"])
	}
}

func TestParsingTheCommandLine(t *testing.T) {
	var out bytes.Buffer

	opts, err := Parse([]string{
		"-context", "p-mk1", "-allow-write", "-log-lines", "50",
		"call", "get_dashboard",
	}, &out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.Context != "p-mk1" {
		t.Fatalf("context = %q", opts.Context)
	}
	if !opts.AllowWrite {
		t.Fatal("allow-write was not read")
	}
	if opts.LogLines != 50 {
		t.Fatalf("logLines = %d", opts.LogLines)
	}
	if len(opts.Args) != 2 || opts.Args[0] != "call" {
		t.Fatalf("args = %v", opts.Args)
	}
}

func TestTheDefaultsWhenNothingIsAsked(t *testing.T) {
	var out bytes.Buffer

	opts, err := Parse(nil, &out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.LogLines != defaultLogLines {
		t.Fatalf("logLines = %d, want %d", opts.LogLines, defaultLogLines)
	}
	if opts.AllowWrite {
		t.Fatal("writes are on by default; they must be asked for")
	}
	if opts.SyncWait == 0 {
		t.Fatal("no sync timeout was set, so a cold cache would wait forever")
	}
	if len(opts.Args) != 0 {
		t.Fatalf("args = %v, want none", opts.Args)
	}
}

func TestAskingForHelpIsNotAFailure(t *testing.T) {
	var out bytes.Buffer

	_, err := Parse([]string{"-h"}, &out)

	if !errors.Is(err, ErrHelp) {
		t.Fatalf("error = %v, want ErrHelp", err)
	}
	if !strings.Contains(out.String(), "spinoza-mcp") {
		t.Fatalf("usage = %q, want it to name the command", out.String())
	}
}

func TestAFlagNobodyKnowsIsRefused(t *testing.T) {
	var out bytes.Buffer

	if _, err := Parse([]string{"-nonsense"}, &out); err == nil || errors.Is(err, ErrHelp) {
		t.Fatalf("error = %v, want a real failure", err)
	}
}

func TestDispatchPicksTheModeFromTheArguments(t *testing.T) {
	cluster := &fakeCluster{spaces: api.Namespaces{Names: []string{"prod"}}}
	server := serverFor(cluster, Options{})

	cases := []struct {
		name  string
		args  []string
		in    string
		wants string
		fails string
	}{
		{name: "no arguments serves the protocol", args: nil, in: `{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n", wants: `"id":1`},
		{name: "tools lists them", args: []string{"tools"}, wants: "list_namespaces"},
		{name: "call runs one", args: []string{"call", "list_namespaces"}, wants: "prod"},
		{name: "call with no name", args: []string{"call"}, fails: "call needs a tool name"},
		{name: "a command nobody knows", args: []string{"drain"}, fails: `unknown command "drain"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := server.Dispatch(context.Background(), Settings{Args: tc.args}, strings.NewReader(tc.in), &out)
			if tc.fails != "" {
				if err == nil || !strings.Contains(err.Error(), tc.fails) {
					t.Fatalf("error = %v, want %q", err, tc.fails)
				}
				return
			}
			if err != nil {
				t.Fatalf("dispatch: %v", err)
			}
			if !strings.Contains(out.String(), tc.wants) {
				t.Fatalf("output = %q, want it to contain %q", out.String(), tc.wants)
			}
		})
	}
}

func TestPrometheusIsSkippedWhenTheSpecIsNonsense(t *testing.T) {
	if PromFor(api.ContextRef{Name: "nowhere"}, Settings{PromSpec: "not a target"}) != nil {
		t.Fatal("a Prometheus client was built from a spec that does not parse")
	}
}

func TestAMessageTooLargeIsAnsweredRatherThanEndingTheSession(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})
	huge := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search","arguments":{"query":"` +
		strings.Repeat("x", maxMessage) + `"}}}`
	in := strings.NewReader(huge + "\n" + `{"jsonrpc":"2.0","id":2,"method":"ping"}` + "\n")
	var out bytes.Buffer

	if err := server.Serve(context.Background(), in, &out); err != nil {
		t.Fatalf("an oversized message ended the session: %v", err)
	}

	body := out.String()
	if !strings.Contains(body, "larger than this server accepts") {
		t.Fatalf("the oversized message drew no answer:\n%s", body)
	}
	if !strings.Contains(body, `"id":2`) {
		t.Fatalf("the message after the oversized one was never served:\n%s", body)
	}
}

func TestALineLongerThanTheBufferIsStillOneMessage(t *testing.T) {
	padding := strings.Repeat("y", 200*1024)
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping","params":{"pad":"` + padding + `"}}` + "\n")
	server := serverFor(&fakeCluster{}, Options{})
	var out bytes.Buffer

	if err := server.Serve(context.Background(), in, &out); err != nil {
		t.Fatalf("serve: %v", err)
	}

	if !strings.Contains(out.String(), `"id":1`) {
		t.Fatalf("a message larger than the read buffer was not reassembled:\n%s", out.String())
	}
	if strings.Count(strings.TrimSpace(out.String()), "\n") != 0 {
		t.Fatalf("one message drew more than one reply:\n%s", out.String())
	}
}

func TestALineWithNoNewlineAtTheEndIsStillServed(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})
	var out bytes.Buffer

	if err := server.Serve(context.Background(), strings.NewReader(`{"jsonrpc":"2.0","id":9,"method":"ping"}`), &out); err != nil {
		t.Fatalf("serve: %v", err)
	}

	if !strings.Contains(out.String(), `"id":9`) {
		t.Fatalf("the last message was dropped for want of a newline:\n%s", out.String())
	}
}

func TestASlowToolDoesNotHoldUpTheOnesBehindIt(t *testing.T) {
	release := make(chan struct{})
	cluster := &blockingCluster{fakeCluster: &fakeCluster{}, release: release}
	server := serverFor(cluster, Options{})
	in := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_namespaces","arguments":{}}}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"ping"}` + "\n",
	)
	var out lockedBuffer

	done := make(chan error, 1)
	go func() { done <- server.Serve(context.Background(), in, &out) }()

	deadline := time.After(5 * time.Second)
	for !strings.Contains(out.read(), `"id":2`) {
		select {
		case <-deadline:
			t.Fatal("the ping never came back while a slow tool was running")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("serve: %v", err)
	}
	if !strings.Contains(out.read(), `"id":1`) {
		t.Fatalf("the slow tool never answered:\n%s", out.read())
	}
}

type blockingCluster struct {
	*fakeCluster

	release chan struct{}
}

func (b *blockingCluster) Namespaces(context.Context) api.Namespaces {
	<-b.release
	return api.Namespaces{Names: []string{"prod"}}
}

type lockedBuffer struct {
	mu   sync.Mutex
	body bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.body.Write(p)
}

func (l *lockedBuffer) read() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.body.String()
}
