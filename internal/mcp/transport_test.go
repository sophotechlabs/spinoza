package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type interruptedInput struct {
	delivered bool
}

func (i *interruptedInput) Read(into []byte) (int, error) {
	if i.delivered {
		return 0, errRefused
	}
	i.delivered = true
	return copy(into, `{"jsonrpc":"2.0","id":1,"method":"ping"}`), nil
}

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

func TestACommandLineCallRejectsAnArgumentWithNoEquals(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})
	var out bytes.Buffer

	err := server.Call(t.Context(), &out, "list_namespaces", []string{"namespace"})

	if err == nil || !strings.Contains(err.Error(), "key=value") {
		t.Fatalf("error = %v, want the malformed pair named", err)
	}
	if out.Len() != 0 {
		t.Fatalf("output = %q, want no partial answer", out.String())
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

func TestAWriteFailureWhileFillingTheBufferIsReported(t *testing.T) {
	sink := &writer{out: bufio.NewWriter(brokenWriter{})}

	sink.send(bytes.Repeat([]byte("x"), 8192))

	if !errors.Is(sink.err(), errRefused) {
		t.Fatalf("write error = %v, want the underlying failure", sink.err())
	}
}

func TestAStdioInputFailureIsReturnedAfterItsLastMessage(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})
	var out bytes.Buffer

	err := server.Serve(t.Context(), &interruptedInput{}, &out)

	if !errors.Is(err, errRefused) {
		t.Fatalf("serve error = %v, want the input failure", err)
	}
	if !strings.Contains(out.String(), `"jsonrpc":"2.0"`) {
		t.Fatalf("output = %q, want the complete final request answered", out.String())
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
		"-context", "p-mk1", "-allow-write", "-unsafe-raw-output", "-log-lines", "50",
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
	if !opts.UnsafeRawOutput {
		t.Fatal("unsafe-raw-output was not read")
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
	if opts.UnsafeRawOutput {
		t.Fatal("raw logs and values are on by default")
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

func TestPrometheusIsSkippedWhenTheClusterCannotBeLoaded(t *testing.T) {
	opts := Settings{
		Kubeconfig: filepath.Join(t.TempDir(), "missing"),
		PromSpec:   "monitoring/prometheus:9090",
	}

	if PromFor(api.ContextRef{Name: "test"}, opts) != nil {
		t.Fatal("a Prometheus client was built without a readable cluster")
	}
}

func TestPrometheusUsesTheSelectedCluster(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	body := `apiVersion: v1
kind: Config
current-context: test
clusters:
- name: local
  cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
contexts:
- name: test
  context:
    cluster: local
    namespace: default
    user: test
users:
- name: test
  user:
    token: token
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	opts := Settings{Kubeconfig: path, PromSpec: "monitoring/prometheus:9090"}

	if PromFor(api.ContextRef{Name: "test"}, opts) == nil {
		t.Fatal("a valid cluster and Prometheus target produced no client")
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

func TestReadingAnOversizedMessageRetainsOnlyTheLimit(t *testing.T) {
	oversized := strings.Repeat("x", maxMessage+64*1024)
	reader := bufio.NewReaderSize(strings.NewReader(oversized+"\nnext\n"), 64*1024)

	first, err := readMessage(reader)
	if err != nil {
		t.Fatalf("first message: %v", err)
	}
	if len(first) != maxMessage+2 {
		t.Fatalf("retained = %d bytes, want %d", len(first), maxMessage+2)
	}
	second, secondErr := readMessage(reader)
	if secondErr != nil {
		t.Fatalf("second message: %v", secondErr)
	}
	if string(second) != "next" {
		t.Fatalf("second message = %q, want the request after the oversized line", second)
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
	releaseAll := closeOnce(release)
	defer releaseAll()
	cluster := &blockingCluster{fakeCluster: &fakeCluster{}, release: release}
	server := serverFor(cluster, Options{})
	in := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_namespaces","arguments":{}}}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"ping"}` + "\n",
	)
	out := lockedBuffer{written: make(chan struct{}, 4)}

	done := make(chan error, 1)
	go func() { done <- server.Serve(context.Background(), in, &out) }()

	for !strings.Contains(out.read(), `"id":2`) {
		select {
		case <-out.written:
		case <-time.After(5 * time.Second):
			t.Fatal("the ping never came back while a slow tool was running")
		}
	}
	releaseAll()
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
	entered chan struct{}
}

type panickingCluster struct {
	*fakeCluster
}

func (p *panickingCluster) Namespaces(context.Context) api.Namespaces {
	panic("broken namespace reader")
}

func TestStdioTurnsAHandlerPanicIntoAnInternalError(t *testing.T) {
	server := serverFor(&panickingCluster{fakeCluster: &fakeCluster{}}, Options{})
	var out bytes.Buffer

	err := server.Serve(context.Background(), strings.NewReader(namespaceRequests(1)), &out)
	if err != nil {
		t.Fatalf("serve: %v", err)
	}
	if !strings.Contains(out.String(), `"id":1`) {
		t.Fatalf("reply = %s, want the request id", out.String())
	}
	if !strings.Contains(out.String(), `"code":-32603`) {
		t.Fatalf("reply = %s, want an internal error", out.String())
	}
}

func (b *blockingCluster) Namespaces(context.Context) api.Namespaces {
	if b.entered != nil {
		b.entered <- struct{}{}
	}
	<-b.release
	return api.Namespaces{Names: []string{"prod"}}
}

func namespaceRequests(count int) string {
	var in strings.Builder
	for id := 1; id <= count; id++ {
		_, _ = fmt.Fprintf(
			&in,
			`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"list_namespaces","arguments":{}}}`+"\n",
			id,
		)
	}
	return in.String()
}

func waitForEntries(t *testing.T, entered <-chan struct{}, count int) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for received := range count {
		select {
		case <-entered:
		case <-timer.C:
			t.Fatalf("calls entered = %d, want %d", received, count)
		}
	}
}

func closeOnce(ch chan struct{}) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			close(ch)
		})
	}
}

func TestStdioCapsConcurrentCalls(t *testing.T) {
	release := make(chan struct{})
	releaseAll := closeOnce(release)
	entered := make(chan struct{}, atOnce+1)
	cluster := &blockingCluster{fakeCluster: &fakeCluster{}, release: release, entered: entered}
	server := serverFor(cluster, Options{})
	var out lockedBuffer
	done := make(chan error, 1)
	finished := make(chan struct{})
	t.Cleanup(func() {
		releaseAll()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("serve did not stop during cleanup")
		}
	})

	go func() {
		defer close(finished)
		done <- server.Serve(context.Background(), strings.NewReader(namespaceRequests(atOnce+1)), &out)
	}()
	waitForEntries(t, entered, atOnce)
	select {
	case <-entered:
		t.Fatalf("more than %d calls ran at once", atOnce)
	default:
	}
	releaseAll()

	if err := <-done; err != nil {
		t.Fatalf("serve: %v", err)
	}
	if replies := strings.Count(strings.TrimSpace(out.read()), "\n") + 1; replies != atOnce+1 {
		t.Fatalf("replies = %d, want %d", replies, atOnce+1)
	}
}

func TestCancellationWaitsForRunningCalls(t *testing.T) {
	release := make(chan struct{})
	releaseAll := closeOnce(release)
	entered := make(chan struct{}, atOnce+1)
	cluster := &blockingCluster{fakeCluster: &fakeCluster{}, release: release, entered: entered}
	server := serverFor(cluster, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	finished := make(chan struct{})
	t.Cleanup(func() {
		releaseAll()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("serve did not stop during cleanup")
		}
	})

	go func() {
		defer close(finished)
		done <- server.Serve(ctx, strings.NewReader(namespaceRequests(atOnce+1)), &lockedBuffer{})
	}()
	waitForEntries(t, entered, atOnce)
	cancel()
	select {
	case err := <-done:
		t.Fatalf("serve returned before its workers: %v", err)
	default:
	}
	releaseAll()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("serve error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not finish after its workers")
	}
}

type yieldingWriter struct {
	body bytes.Buffer
}

func (w *yieldingWriter) Write(p []byte) (int, error) {
	middle := len(p) / 2
	first, _ := w.body.Write(p[:middle])
	runtime.Gosched()
	second, _ := w.body.Write(p[middle:])
	return first + second, nil
}

func TestConcurrentRepliesStayIndivisible(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{})
	const calls = 64
	var in strings.Builder
	for id := 1; id <= calls; id++ {
		_, _ = fmt.Fprintf(&in, `{"jsonrpc":"2.0","id":%d,"method":"ping"}`+"\n", id)
	}
	var out yieldingWriter

	if err := server.Serve(context.Background(), strings.NewReader(in.String()), &out); err != nil {
		t.Fatalf("serve: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.body.String()), "\n")
	if len(lines) != calls {
		t.Fatalf("replies = %d, want %d", len(lines), calls)
	}
	ids := map[float64]bool{}
	for _, line := range lines {
		var reply map[string]any
		if err := json.Unmarshal([]byte(line), &reply); err != nil {
			t.Fatalf("interleaved reply %q: %v", line, err)
		}
		id, ok := reply["id"].(float64)
		if !ok {
			t.Fatalf("reply id = %#v, want a number", reply["id"])
		}
		ids[id] = true
	}
	if len(ids) != calls {
		t.Fatalf("distinct reply ids = %d, want %d", len(ids), calls)
	}
	for id := 1; id <= calls; id++ {
		if !ids[float64(id)] {
			t.Errorf("missing reply id %d", id)
		}
	}
}

type firstFailureWriter struct {
	calls int
	first error
	later error
}

func (w *firstFailureWriter) Write([]byte) (int, error) {
	w.calls++
	if w.calls == 1 {
		return 0, w.first
	}
	return 0, w.later
}

func TestConcurrentRepliesReturnTheFirstWriteFailure(t *testing.T) {
	first := errors.New("stdout closed")
	later := errors.New("a later failure")
	out := &firstFailureWriter{first: first, later: later}
	server := serverFor(&fakeCluster{}, Options{})
	var in strings.Builder
	for id := 1; id <= 16; id++ {
		_, _ = fmt.Fprintf(&in, `{"jsonrpc":"2.0","id":%d,"method":"ping"}`+"\n", id)
	}

	err := server.Serve(context.Background(), strings.NewReader(in.String()), out)

	if !errors.Is(err, first) {
		t.Fatalf("serve error = %v, want the first write failure", err)
	}
	if out.calls != 1 {
		t.Fatalf("writer calls = %d, want replies suppressed after the failure", out.calls)
	}
}

type lockedBuffer struct {
	mu      sync.Mutex
	body    bytes.Buffer
	written chan struct{}
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	written, err := l.body.Write(p)
	l.mu.Unlock()
	if l.written != nil {
		select {
		case l.written <- struct{}{}:
		default:
		}
	}
	return written, err
}

func (l *lockedBuffer) read() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.body.String()
}
