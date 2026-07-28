package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/exec"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

type fakeShell struct {
	mu      sync.Mutex
	stdin   []byte
	sizes   []exec.Size
	entered chan struct{}
	release chan struct{}
	greet   string
	err     error
}

func newFakeShell() *fakeShell {
	return &fakeShell{
		entered: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
}

func (f *fakeShell) Stream(ctx context.Context, _ exec.Request, opts exec.Options) error {
	select {
	case f.entered <- struct{}{}:
	default:
	}
	if f.greet != "" {
		_, _ = opts.Stdout.Write([]byte(f.greet))
	}

	go f.collect(opts.Stdin)

	for {
		select {
		case size := <-opts.Resize:
			f.mu.Lock()
			f.sizes = append(f.sizes, size)
			f.mu.Unlock()
		case <-f.release:
			return f.err
		case <-ctx.Done():
			return f.err
		}
	}
}

func (f *fakeShell) collect(stdin io.Reader) {
	buffer := make([]byte, 256)
	for {
		read, err := stdin.Read(buffer)
		if read > 0 {
			f.mu.Lock()
			f.stdin = append(f.stdin, buffer[:read]...)
			f.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (f *fakeShell) recorded() ([]byte, []exec.Size) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stdin, append([]exec.Size{}, f.sizes...)
}

type fakeImages struct {
	digest string
	err    error
}

func (f *fakeImages) ImageID(context.Context, exec.Request) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.digest, nil
}

const execQuery = "?namespace=monitoring&pod=loki-0&container=loki"

func execServer(t *testing.T, service *exec.Service) *httptest.Server {
	t.Helper()
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{podGVR: "PodList"},
	)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	mgr := resources.NewManager(ctx, dyn, k8sfake.NewClientset(), nil, nil, service, nil, nil)
	ts := httptest.NewServer(New(mgr, testAssets()).Handler())
	t.Cleanup(ts.Close)
	return ts
}

func dialExec(t *testing.T, ts *httptest.Server, query string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/exec" + query
	conn, _, err := websocket.Dial(context.Background(), url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn
}

func readFrame(t *testing.T, conn *websocket.Conn) (byte, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	kind, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if kind != websocket.MessageBinary {
		t.Fatalf("message type = %v", kind)
	}
	if len(data) == 0 {
		t.Fatal("empty frame")
	}
	return data[0], data[1:]
}

func writeFrame(t *testing.T, conn *websocket.Conn, channel byte, payload []byte) {
	t.Helper()
	frame := append([]byte{channel}, payload...)
	err := conn.Write(context.Background(), websocket.MessageBinary, frame)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestExecStreamsStdoutStdinAndResize(t *testing.T) {
	shell := newFakeShell()
	shell.greet = "/ # "
	service := exec.NewService(shell, &fakeImages{digest: "sha256:shelled"})
	ts := execServer(t, service)

	conn := dialExec(t, ts, execQuery)
	channel, payload := readFrame(t, conn)
	if channel != api.ExecChannelStdout {
		t.Fatalf("channel = %d", channel)
	}
	if string(payload) != "/ # " {
		t.Fatalf("payload = %q", payload)
	}

	<-shell.entered
	writeFrame(t, conn, api.ExecChannelStdin, []byte("uptime\n"))
	writeFrame(t, conn, api.ExecChannelResize, []byte(`{"cols":120,"rows":40}`))

	waitForServer(t, func() bool {
		stdin, sizes := shell.recorded()
		if string(stdin) != "uptime\n" {
			return false
		}
		return len(sizes) == 1
	}, "stdin and resize never reached the shell")

	_, sizes := shell.recorded()
	if sizes[0].Cols != 120 {
		t.Fatalf("cols = %d", sizes[0].Cols)
	}
	if sizes[0].Rows != 40 {
		t.Fatalf("rows = %d", sizes[0].Rows)
	}

	close(shell.release)
	channel, payload = readFrame(t, conn)
	if channel != api.ExecChannelError {
		t.Fatalf("channel = %d", channel)
	}
	if len(payload) != 0 {
		t.Fatalf("clean exit carried %q", payload)
	}
}

func TestExecReportsTheStreamError(t *testing.T) {
	shell := newFakeShell()
	shell.err = errors.New(`exec: "/bin/sh": stat /bin/sh: no such file or directory`)
	service := exec.NewService(shell, &fakeImages{digest: "sha256:distroless"})
	ts := execServer(t, service)

	conn := dialExec(t, ts, execQuery)
	<-shell.entered
	close(shell.release)

	channel, payload := readFrame(t, conn)
	if channel != api.ExecChannelError {
		t.Fatalf("channel = %d", channel)
	}
	if !strings.Contains(string(payload), "no such file or directory") {
		t.Fatalf("payload = %q", payload)
	}
}

func TestExecRefusesAnImageKnownToHaveNoShell(t *testing.T) {
	shell := newFakeShell()
	shell.err = errors.New(`exec: "/bin/sh": stat /bin/sh: no such file or directory`)
	service := exec.NewService(shell, &fakeImages{digest: "sha256:distroless"})
	ts := execServer(t, service)

	first := dialExec(t, ts, execQuery)
	<-shell.entered
	close(shell.release)
	readFrame(t, first)

	second := dialExec(t, ts, execQuery)
	channel, payload := readFrame(t, second)
	if channel != api.ExecChannelError {
		t.Fatalf("channel = %d", channel)
	}
	if !strings.Contains(string(payload), exec.ShellPath) {
		t.Fatalf("payload = %q", payload)
	}
}

func TestExecIgnoresJunkFrames(t *testing.T) {
	shell := newFakeShell()
	shell.greet = "ready"
	service := exec.NewService(shell, &fakeImages{digest: "sha256:shelled"})
	ts := execServer(t, service)

	conn := dialExec(t, ts, execQuery)
	readFrame(t, conn)
	<-shell.entered

	err := conn.Write(context.Background(), websocket.MessageText, []byte("hello"))
	if err != nil {
		t.Fatalf("write text: %v", err)
	}
	err = conn.Write(context.Background(), websocket.MessageBinary, nil)
	if err != nil {
		t.Fatalf("write empty: %v", err)
	}
	writeFrame(t, conn, api.ExecChannelResize, []byte("not json"))
	writeFrame(t, conn, api.ExecChannelStderr, []byte("ignored"))
	writeFrame(t, conn, api.ExecChannelStdin, []byte("ls\n"))

	waitForServer(t, func() bool {
		stdin, sizes := shell.recorded()
		if len(sizes) != 0 {
			return false
		}
		return string(stdin) == "ls\n"
	}, "the junk frames were not ignored")
	close(shell.release)
}

func TestExecRejectsAMissingPod(t *testing.T) {
	ts := execServer(t, exec.NewService(newFakeShell(), &fakeImages{}))
	res, err := http.Get(ts.URL + "/api/exec?namespace=monitoring")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", res.StatusCode)
	}
}

func TestExecSupportReportsTheCachedVerdict(t *testing.T) {
	shell := newFakeShell()
	shell.greet = "/ # "
	service := exec.NewService(shell, &fakeImages{digest: "sha256:shelled"})
	ts := execServer(t, service)

	support := getSupport(t, ts, execQuery)
	if support.Shell != api.ShellUnknown {
		t.Fatalf("shell = %q", support.Shell)
	}

	conn := dialExec(t, ts, execQuery)
	readFrame(t, conn)
	close(shell.release)
	readFrame(t, conn)

	support = getSupport(t, ts, execQuery)
	if support.Shell != api.ShellPresent {
		t.Fatalf("shell = %q", support.Shell)
	}
	if support.Image != "sha256:shelled" {
		t.Fatalf("image = %q", support.Image)
	}
}

func TestExecSupportRejectsAMissingPod(t *testing.T) {
	ts := execServer(t, exec.NewService(newFakeShell(), &fakeImages{}))
	res, err := http.Get(ts.URL + "/api/exec/support?namespace=monitoring")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", res.StatusCode)
	}
}

func TestExecSupportReportsALookupFailure(t *testing.T) {
	service := exec.NewService(newFakeShell(), &fakeImages{err: errors.New("boom")})
	ts := execServer(t, service)

	res, err := http.Get(ts.URL + "/api/exec/support" + execQuery)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", res.StatusCode)
	}
}

func TestExecIsUnavailableWithoutAService(t *testing.T) {
	ts := execServer(t, nil)

	res, err := http.Get(ts.URL + "/api/exec/support" + execQuery)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", res.StatusCode)
	}

	conn := dialExec(t, ts, execQuery)
	channel, payload := readFrame(t, conn)
	if channel != api.ExecChannelError {
		t.Fatalf("channel = %d", channel)
	}
	if !strings.Contains(string(payload), "unavailable") {
		t.Fatalf("payload = %q", payload)
	}
}

func getSupport(t *testing.T, ts *httptest.Server, query string) api.ExecSupport {
	t.Helper()
	res, err := http.Get(ts.URL + "/api/exec/support" + query)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	var support api.ExecSupport
	err = json.NewDecoder(res.Body).Decode(&support)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return support
}

func waitForServer(t *testing.T, cond func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(message)
}

func TestResourcesRefreshRejectsOtherMethods(t *testing.T) {
	ts := execServer(t, nil)
	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/resources", nil)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", res.StatusCode)
	}
}

func TestResourcesRefreshReturnsTheCatalog(t *testing.T) {
	ts := execServer(t, nil)
	res, err := http.Post(ts.URL+"/api/resources", "", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	var catalog api.ResourceCatalog
	if err := json.NewDecoder(res.Body).Decode(&catalog); err != nil {
		t.Fatalf("decode: %v", err)
	}
}
