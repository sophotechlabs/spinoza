//go:build clustermode

package clustermode

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	base        = "https://spinoza.localtest.me:8443"
	socketBase  = "wss://spinoza.localtest.me:8443"
	realm       = "https://keycloak.localtest.me:8443/realms/spinoza"
	innerRealm  = "http://keycloak.keycloak.svc.cluster.local:8080/realms/spinoza"
	shimRealm   = "http://kcshim.keycloak.svc.cluster.local/realms/spinoza"
	release     = "spinoza"
	pathSession = "/api/auth/me"
	namespace   = "spinoza"
	chart       = "../../deploy/helm/spinoza"
	deployWait  = "8m"
	rolloutWait = 5 * time.Minute
)

func context1(t *testing.T) string {
	t.Helper()
	held := os.Getenv("SPINOZA_CM_CONTEXT")
	if held == "" {
		t.Skip("SPINOZA_CM_CONTEXT is not set; run just test-cluster-mode")
	}
	return held
}

func run(t *testing.T, name string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

func kubectl(t *testing.T, args ...string) string {
	t.Helper()
	return run(t, "kubectl", append([]string{"--context", context1(t)}, args...)...)
}

func maybeKubectl(t *testing.T, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "kubectl", append([]string{"--context", context1(t)}, args...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func baseValues() map[string]string {
	return map[string]string{
		"image.repository":                       "spinoza",
		"image.tag":                              "cluster-mode",
		"image.pullPolicy":                       "Never",
		"publicURL":                              base,
		"ingress.enabled":                        "true",
		"ingress.className":                      "nginx",
		"ingress.hosts[0].host":                  "spinoza.localtest.me",
		"ingress.hosts[0].paths[0].path":         "/",
		"ingress.tls[0].secretName":              "localtest-tls",
		"ingress.tls[0].hosts[0]":                "spinoza.localtest.me",
		"logLevel":                               "debug",
		"impersonate":                            "true",
		"rbac.read":                              "everything",
		"rbac.impersonation.unsafeAllowAnyUser":  "true",
		"rbac.impersonation.unsafeAllowAnyGroup": "true",
	}
}

func oidcValues() map[string]string {
	values := baseValues()
	values["auth.mode"] = "oidc"
	values["auth.sessionSecret"] = "a-cluster-mode-session-secret-for-tests"
	values["auth.oidc.issuerURL"] = realm
	values["auth.oidc.internalIssuerURL"] = innerRealm
	values["auth.oidc.unsafeAllowHTTP"] = "true"
	values["auth.oidc.clientID"] = "spinoza"
	values["auth.oidc.clientSecret"] = "spinoza-client-secret"
	values["auth.oidc.backchannelLogout"] = "true"
	values["auth.adminGroups[0]"] = "platform-admins"
	values["auth.editorGroups[0]"] = "platform"
	return values
}

func deploy(t *testing.T, values map[string]string) {
	t.Helper()
	args := []string{
		"--kube-context", context1(t),
		"upgrade", "--install", release, chart,
		"--namespace", namespace,
		"--wait", "--timeout", deployWait,
	}
	for key, value := range values {
		args = append(args, "--set", key+"="+value)
	}
	run(t, "helm", args...)
	kubectl(t, "-n", namespace, "rollout", "status", "deployment/"+release, "--timeout="+deployWait)
	waitEndpoints(t)
	waitReachable(t)
	waitServing(t, values["auth.mode"])
}

// waitEndpoints holds until the ingress has somewhere to send a request. The
// chart replaces the pod rather than adding one, so between the two there is a
// window where nginx answers 503 on its own.
func waitEndpoints(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(rolloutWait)
	for time.Now().Before(deadline) {
		out, err := maybeKubectl(t, "-n", namespace, "get", "endpoints", release,
			"-o", "jsonpath={.subsets[0].addresses[0].ip}")
		if err == nil && strings.TrimSpace(out) != "" {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatal("the service never got an endpoint")
}

// waitServing holds until the ingress is handing requests to the pod this deploy
// asked for, which /healthz cannot tell you: the one before it answers too.
func waitServing(t *testing.T, mode string) {
	t.Helper()
	if mode == "" {
		mode = "none"
	}
	deadline := time.Now().Add(rolloutWait)
	seen := ""
	steady := 0
	for time.Now().Before(deadline) {
		resp := get(t, anonymous(t), pathSession)
		var out api.Session
		decodeErr := json.NewDecoder(resp.Body).Decode(&out)
		_ = resp.Body.Close()
		if decodeErr == nil && out.Mode == mode {
			steady++
			if steady == 3 {
				return
			}
		} else {
			steady = 0
			if decodeErr == nil {
				seen = out.Mode
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("the address still answers as %q, want %q", seen, mode)
}

func waitReachable(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(rolloutWait)
	for time.Now().Before(deadline) {
		resp, err := anonymous(t).Get(base + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatal("spinoza never answered on its ingress")
}

func roots(t *testing.T) *x509.CertPool {
	t.Helper()
	path := os.Getenv("SPINOZA_CM_CA")
	if path == "" {
		t.Skip("SPINOZA_CM_CA is not set; run just test-cluster-mode")
	}
	pem, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the test certificate: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		t.Fatalf("%s holds no certificate", path)
	}
	return pool
}

func transport(t *testing.T) *http.Transport {
	t.Helper()
	return &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots(t), MinVersion: tls.VersionTLS12}}
}

func anonymous(t *testing.T) *http.Client {
	t.Helper()
	return &http.Client{Transport: transport(t), Timeout: 30 * time.Second}
}

func browser(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return &http.Client{Transport: transport(t), Jar: jar, Timeout: 60 * time.Second}
}

var formAction = regexp.MustCompile(`action="([^"]+)"`)

func signIn(t *testing.T, user string) *http.Client {
	t.Helper()
	held := browser(t)
	page := body(t, get(t, held, "/auth/login?next=%2F"))
	found := formAction.FindStringSubmatch(page)
	if found == nil {
		t.Fatalf("no login form came back for %s: %s", user, truncate(page))
	}
	action := strings.ReplaceAll(found[1], "&amp;", "&")
	form := url.Values{"username": {user}, "password": {user}, "credentialId": {""}}
	resp, err := held.PostForm(action, form)
	if err != nil {
		t.Fatalf("signing %s in: %v", user, err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	if !signedIn(t, held) {
		t.Fatalf("%s did not end up signed in", user)
	}
	return held
}

func signedIn(t *testing.T, held *http.Client) bool {
	t.Helper()
	return whoami(t, held).Authenticated
}

func whoami(t *testing.T, held *http.Client) api.Session {
	t.Helper()
	resp := get(t, held, pathSession)
	defer func() { _ = resp.Body.Close() }()
	var out api.Session
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("reading the session: %v", err)
	}
	return out
}

func request(t *testing.T, held *http.Client, method, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, base+path, http.NoBody)
	if err != nil {
		t.Fatalf("building %s %s: %v", method, path, err)
	}
	req.Header.Set("Origin", base)
	resp, doErr := held.Do(req)
	if doErr != nil {
		t.Fatalf("%s %s: %v", method, path, doErr)
	}
	return resp
}

func get(t *testing.T, held *http.Client, path string) *http.Response {
	t.Helper()
	return request(t, held, http.MethodGet, path)
}

func post(t *testing.T, held *http.Client, path string) (int, string) {
	t.Helper()
	resp := request(t, held, http.MethodPost, path)
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, body(t, resp)
}

func read(t *testing.T, held *http.Client, path string) (int, string) {
	t.Helper()
	resp := get(t, held, path)
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, body(t, resp)
}

func body(t *testing.T, resp *http.Response) string {
	t.Helper()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the body: %v", err)
	}
	return string(raw)
}

func truncate(text string) string {
	if len(text) <= 400 {
		return text
	}
	return text[:400] + "..."
}

func podIn(t *testing.T, ns string) string {
	t.Helper()
	return strings.TrimSpace(kubectl(t, "-n", ns, "get", "pod", "-o", "jsonpath={.items[0].metadata.name}"))
}

func aNode(t *testing.T) string {
	t.Helper()
	return strings.TrimSpace(kubectl(t, "get", "node", "-o", "jsonpath={.items[0].metadata.name}"))
}

func shell(t *testing.T, held *http.Client, path string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	conn, resp, err := websocket.Dial(ctx, socketBase+path, &websocket.DialOptions{
		HTTPClient: held,
		HTTPHeader: http.Header{"Origin": []string{base}},
	})
	if err != nil {
		status := 0
		reason := err.Error()
		if resp != nil {
			status = resp.StatusCode
			defer func() { _ = resp.Body.Close() }()
			if responseBody := body(t, resp); responseBody != "" {
				reason = messageOf(t, responseBody)
			}
		}
		return fmt.Sprintf("refused before the shell opened (%d): %s", status, reason)
	}
	defer func() { _ = conn.CloseNow() }()
	for {
		kind, data, readErr := conn.Read(ctx)
		if readErr != nil {
			return "the socket ended: " + readErr.Error()
		}
		if kind != websocket.MessageBinary || len(data) == 0 {
			continue
		}
		payload := strings.TrimSpace(string(data[1:]))
		if payload == "" {
			continue
		}
		switch data[0] {
		case api.ExecChannelError:
			return payload
		case api.ExecChannelStdout, api.ExecChannelStderr:
			return "OPENED " + payload
		}
	}
}

type feedReply struct {
	Type    string    `json:"type"`
	SubID   string    `json:"subId"`
	Rows    []api.Row `json:"rows"`
	Total   int       `json:"total"`
	Message string    `json:"message"`
}

func subscribe(t *testing.T, held *http.Client, msg api.ClientMsg) feedReply {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, socketBase+"/ws", &websocket.DialOptions{
		HTTPClient: held,
		HTTPHeader: http.Header{"Origin": []string{base}},
	})
	if err != nil {
		t.Fatalf("opening the feed: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()
	if msg.Type == "" {
		msg.Type = "subscribe"
	}
	msg.SubID = "probe"
	if writeErr := wsjson.Write(ctx, conn, msg); writeErr != nil {
		t.Fatalf("subscribing: %v", writeErr)
	}
	for {
		var reply feedReply
		if readErr := wsjson.Read(ctx, conn, &reply); readErr != nil {
			t.Fatalf("reading the feed: %v", readErr)
		}
		if reply.SubID != "probe" {
			continue
		}
		return reply
	}
}

func namespacesIn(rows []api.Row) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, row := range rows {
		if seen[row.Namespace] {
			continue
		}
		seen[row.Namespace] = true
		out = append(out, row.Namespace)
	}
	slices.Sort(out)
	return out
}

func installProbe(t *testing.T, ns string) {
	t.Helper()
	run(t, "helm", "--kube-context", context1(t), "upgrade", "--install", "probe", "chart",
		"--namespace", ns, "--wait", "--timeout", "2m")
}

func removeProbe(t *testing.T, ns string) {
	t.Helper()
	_, _ = maybeHelm(t, "--kube-context", context1(t), "uninstall", "probe", "--namespace", ns)
}

func maybeHelm(t *testing.T, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "helm", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func direct(t *testing.T) *http.Client {
	t.Helper()
	return &http.Client{
		Transport: transport(t),
		Timeout:   30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func loginRedirect(t *testing.T) string {
	t.Helper()
	resp := get(t, direct(t), "/auth/login?next=%2F")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want %d with the provider's address", resp.StatusCode, http.StatusFound)
	}
	return resp.Header.Get("Location")
}

func proxiedRequest(t *testing.T, method, path, user, groups string, follow bool) *http.Response {
	t.Helper()
	held := anonymous(t)
	if !follow {
		held = direct(t)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, base+path, http.NoBody)
	if err != nil {
		t.Fatalf("building %s %s: %v", method, path, err)
	}
	req.Header.Set("Origin", base)
	req.Header.Set("X-Forwarded-User", user)
	req.Header.Set("X-Forwarded-Groups", groups)
	req.Header.Set("X-Spinoza-Proxy-Secret", "a-cluster-mode-proxy-secret-that-is-long-enough")
	resp, doErr := held.Do(req)
	if doErr != nil {
		t.Fatalf("%s %s: %v", method, path, doErr)
	}
	return resp
}

func asProxied(t *testing.T, user, groups string) api.Session {
	t.Helper()
	resp := proxiedRequest(t, http.MethodGet, "/api/auth/me", user, groups, true)
	defer func() { _ = resp.Body.Close() }()
	var out api.Session
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("reading the session: %v", err)
	}
	return out
}

func proxiedPost(t *testing.T, user, groups, path string) (int, string) {
	t.Helper()
	resp := proxiedRequest(t, http.MethodPost, path, user, groups, true)
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, body(t, resp)
}

func messageOf(t *testing.T, raw string) string {
	t.Helper()
	var out struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return raw
	}
	if out.Message == "" {
		return raw
	}
	return out.Message
}

func sessionCookieFrom(t *testing.T, user string) *http.Cookie {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	seen := []*http.Cookie{}
	held := &http.Client{
		Transport: transport(t),
		Jar:       jar,
		Timeout:   60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.Response != nil {
				seen = append(seen, req.Response.Cookies()...)
			}
			if len(via) >= 10 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
	page := body(t, get(t, held, "/auth/login?next=%2F"))
	found := formAction.FindStringSubmatch(page)
	if found == nil {
		t.Fatalf("no login form came back: %s", truncate(page))
	}
	resp, postErr := held.PostForm(strings.ReplaceAll(found[1], "&amp;", "&"),
		url.Values{"username": {user}, "password": {user}, "credentialId": {""}})
	if postErr != nil {
		t.Fatalf("signing %s in: %v", user, postErr)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	seen = append(seen, resp.Cookies()...)
	for _, cookie := range seen {
		if cookie.Name == "spinoza_session" && cookie.Value != "" {
			return cookie
		}
	}
	t.Fatalf("no session cookie was set for %s", user)
	return nil
}

func subscribeUntilReady(t *testing.T, held *http.Client, msg api.ClientMsg) feedReply {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	var last feedReply
	for time.Now().Before(deadline) {
		last = subscribe(t, held, msg)
		if last.Type == "snapshot" && len(last.Rows) > 0 {
			return last
		}
		time.Sleep(2 * time.Second)
	}
	return last
}
