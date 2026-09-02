package charts

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

const indexBody = `
apiVersion: v1
entries:
  podinfo:
    - version: 6.14.0
    - version: 6.15.1
    - version: 6.16.0-rc.1
    - version: 6.9.0
  other:
    - version: 1.2.3
`

type interruptedReader struct {
	delivered bool
}

func (r *interruptedReader) Read(into []byte) (int, error) {
	if r.delivered {
		return 0, errors.New("repository response interrupted")
	}
	r.delivered = true
	return copy(into, "entries:\n  podinfo:"), nil
}

func cacheFor(t *testing.T, handler http.HandlerFunc) (*Cache, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	address := ts.Listener.Addr().String()
	ts.URL = "http://charts.example.com"
	return cacheThrough(t, ts.Client(), address), ts
}

func tlsCacheFor(t *testing.T, handler http.HandlerFunc) (*Cache, *httptest.Server) {
	t.Helper()
	ts := httptest.NewTLSServer(handler)
	t.Cleanup(ts.Close)
	address := ts.Listener.Addr().String()
	ts.URL = "https://registry.example.com"
	return cacheThrough(t, ts.Client(), address), ts
}

func cacheThrough(t *testing.T, client *http.Client, address string) *Cache {
	t.Helper()
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
	}
	dial := func(ctx context.Context, network, _ string) (net.Conn, error) {
		var dialer net.Dialer
		return dialer.DialContext(ctx, network, address)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return newCache(ctx, client, DefaultTTL, lookup, dial)
}

func indexHandler(hits *int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/index.yaml") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		*hits++
		_, _ = w.Write([]byte(indexBody))
	}
}

func TestResolveIndexPicksTheHighestStableVersion(t *testing.T) {
	hits := 0
	cache, ts := cacheFor(t, indexHandler(&hits))

	found, err := cache.Resolve(context.Background(), Repo{URL: ts.URL}, "podinfo")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if newest(found, "podinfo") != "6.15.1" {
		t.Fatalf("podinfo = %q, want 6.15.1 (prerelease 6.16.0-rc.1 must be skipped)", newest(found, "podinfo"))
	}
	if newest(found, "other") != "1.2.3" {
		t.Fatalf("other = %q, want 1.2.3", newest(found, "other"))
	}
}

func newest(found map[string][]string, chart string) string {
	list := found[chart]
	if len(list) == 0 {
		return ""
	}
	return list[0]
}

func TestResolveIndexTrimsTrailingSlash(t *testing.T) {
	hits := 0
	cache, ts := cacheFor(t, indexHandler(&hits))

	_, err := cache.Resolve(context.Background(), Repo{URL: ts.URL + "/"}, "podinfo")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if hits != 1 {
		t.Fatalf("hits = %d, want 1", hits)
	}
}

func TestWarmPopulatesEveryChartFromOneFetch(t *testing.T) {
	hits := 0
	cache, ts := cacheFor(t, indexHandler(&hits))
	repo := Repo{URL: ts.URL}

	cache.Warm(repo, "podinfo")
	cache.Warm(repo, "other")
	cache.Wait()

	if cache.Latest(repo, "podinfo") != "6.15.1" {
		t.Fatalf("podinfo = %q", cache.Latest(repo, "podinfo"))
	}
	if cache.Latest(repo, "other") != "1.2.3" {
		t.Fatalf("other = %q", cache.Latest(repo, "other"))
	}
	if hits != 1 {
		t.Fatalf("index fetched %d times, want 1 for an http repo", hits)
	}
}

func TestWarmHonoursTheTTL(t *testing.T) {
	hits := 0
	cache, ts := cacheFor(t, indexHandler(&hits))
	repo := Repo{URL: ts.URL}

	cache.Warm(repo, "podinfo")
	cache.Wait()
	cache.Warm(repo, "podinfo")
	cache.Wait()

	if hits != 1 {
		t.Fatalf("hits = %d, want 1 while inside the ttl", hits)
	}

	cache.now = func() time.Time { return time.Now().Add(2 * DefaultTTL) }
	cache.Warm(repo, "podinfo")
	cache.Wait()

	if hits != 2 {
		t.Fatalf("hits = %d, want 2 after the ttl expired", hits)
	}
}

func TestASeededIndexAnswersWithoutTheNetwork(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		return nil, errors.New("the network was used")
	}
	cache := newCache(t.Context(), nil, time.Hour, lookup, nil)
	cache.now = func() time.Time { return now }
	repo := Repo{URL: "https://charts.example.com"}
	body := indexBody + "\n  preview-only:\n    - version: 7.0.0-rc.1\n"

	err := cache.Seed(repo, strings.NewReader(body), now)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	versions, versionsErr := cache.Versions(t.Context(), repo, "podinfo")
	if versionsErr != nil {
		t.Fatalf("versions: %v", versionsErr)
	}
	if strings.Join(versions, ",") != "6.15.1,6.14.0,6.9.0" {
		t.Fatalf("versions = %v, want the cached stable versions", versions)
	}
	hits, searchErr := cache.Search(t.Context(), repo, "pod", 10)
	if searchErr != nil {
		t.Fatalf("search: %v", searchErr)
	}
	if len(hits) != 1 || hits[0].Name != "podinfo" {
		t.Fatalf("hits = %+v, want podinfo from the cached catalog", hits)
	}
	if cache.Latest(repo, "preview-only") != "" {
		t.Fatal("a preview-only chart entered the stable cache")
	}
}

func TestABrokenSeedDoesNotChangeTheCache(t *testing.T) {
	cache := New(t.Context(), nil, time.Hour)
	repo := Repo{URL: "https://charts.example.com"}

	err := cache.Seed(repo, strings.NewReader("entries: [not closed"), time.Now())
	if err == nil {
		t.Fatal("a malformed cached index was accepted")
	}
	if cache.Latest(repo, "podinfo") != "" {
		t.Fatal("a malformed cached index populated the cache")
	}
}

func TestWarmIgnoresEmptyInputs(t *testing.T) {
	hits := 0
	cache, ts := cacheFor(t, indexHandler(&hits))

	cache.Warm(Repo{URL: ""}, "podinfo")
	cache.Warm(Repo{URL: ts.URL}, "")
	cache.Wait()

	if hits != 0 {
		t.Fatalf("hits = %d, want 0", hits)
	}
}

func TestLatestIsEmptyBeforeAnyFetch(t *testing.T) {
	cache, ts := cacheFor(t, indexHandler(new(int)))

	if cache.Latest(Repo{URL: ts.URL}, "podinfo") != "" {
		t.Fatalf("expected an empty version before warming")
	}
}

func TestWarmSurvivesAFailedFetch(t *testing.T) {
	cache, ts := cacheFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	repo := Repo{URL: ts.URL}

	cache.Warm(repo, "podinfo")
	cache.Wait()

	if cache.Latest(repo, "podinfo") != "" {
		t.Fatalf("expected no version after a failed fetch")
	}
}

func TestAFailedWarmDoesNotHideTheNextSuccessfulFetch(t *testing.T) {
	hits := 0
	cache, ts := cacheFor(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(indexBody))
	})
	repo := Repo{URL: ts.URL}
	cache.Warm(repo, "podinfo")
	cache.Wait()

	versions, err := cache.Versions(t.Context(), repo, "podinfo")
	if err != nil {
		t.Fatalf("versions after the repository recovered: %v", err)
	}
	if len(versions) == 0 || versions[0] != "6.15.1" {
		t.Fatalf("versions after recovery = %v", versions)
	}
	if hits != 2 {
		t.Fatalf("repository fetched %d times, want the failed warm and the recovery", hits)
	}
}

func TestResolveIndexRejectsMalformedYAML(t *testing.T) {
	cache, ts := cacheFor(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("entries: [oops"))
	})

	_, err := cache.Resolve(context.Background(), Repo{URL: ts.URL}, "podinfo")

	if err == nil {
		t.Fatalf("expected a parse error")
	}
}

func TestResolveRefusesALocalRepositoryPassedDirectly(t *testing.T) {
	cache := New(t.Context(), http.DefaultClient, DefaultTTL)

	_, err := cache.Resolve(t.Context(), Repo{URL: "http://127.0.0.1/charts"}, "podinfo")

	if err == nil || !strings.Contains(err.Error(), "public address") {
		t.Fatalf("error = %v, want the local repository refused", err)
	}
}

func TestResolveRequiresTheURLAndOCISettingToAgree(t *testing.T) {
	cache := New(t.Context(), http.DefaultClient, DefaultTTL)
	cases := []Repo{
		{URL: "https://charts.example.com", OCI: true},
		{URL: "oci://registry.example.com/team", OCI: false},
	}

	for _, repo := range cases {
		_, err := cache.Resolve(t.Context(), repo, "podinfo")

		if err == nil || !strings.Contains(err.Error(), "does not match its oci setting") {
			t.Fatalf("repo = %+v, error = %v", repo, err)
		}
	}
}

func ociHandler(t *testing.T, tokens *int) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			*tokens++
			_, _ = w.Write([]byte(`{"token":"abc123"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer abc123" {
			w.Header().Set("WWW-Authenticate",
				fmt.Sprintf(`Bearer realm="https://%s/token",service="registry",scope="repository:team/keycloak:pull"`, r.Host))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/v2/team/keycloak/tags/list" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"tags":["0.21.14","0.22.0","latest","sha256-abc.sig","0.23.0-rc1"]}`))
	}
}

func ociRepo(ts *httptest.Server) Repo {
	return Repo{URL: "oci://" + strings.TrimPrefix(ts.URL, "https://") + "/team", OCI: true}
}

func TestResolveOCIFollowsTheBearerChallenge(t *testing.T) {
	tokens := 0
	cache, ts := tlsCacheFor(t, ociHandler(t, &tokens))

	found, err := cache.Resolve(context.Background(), ociRepo(ts), "keycloak")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if newest(found, "keycloak") != "0.22.0" {
		t.Fatalf("keycloak = %q, want 0.22.0", newest(found, "keycloak"))
	}
	if tokens != 1 {
		t.Fatalf("token requests = %d, want 1", tokens)
	}
}

func TestOCICachesPerChart(t *testing.T) {
	tokens := 0
	cache, ts := tlsCacheFor(t, ociHandler(t, &tokens))
	repo := ociRepo(ts)

	cache.Warm(repo, "keycloak")
	cache.Wait()

	if cache.Latest(repo, "keycloak") != "0.22.0" {
		t.Fatalf("keycloak = %q", cache.Latest(repo, "keycloak"))
	}
	if cache.Latest(repo, "other") != "" {
		t.Fatalf("an unrelated chart must not be populated by an oci fetch")
	}
}

func TestResolveOCIRejectsABadURL(t *testing.T) {
	cache, _ := tlsCacheFor(t, ociHandler(t, new(int)))

	_, err := cache.Resolve(context.Background(), Repo{URL: "oci://registry-only", OCI: true}, "keycloak")

	if err == nil {
		t.Fatalf("expected an error for an oci url without a path")
	}
}

func TestResolveOCIGivesUpWhenTheTokenIsRejected(t *testing.T) {
	cache, ts := tlsCacheFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = w.Write([]byte(`{"token":"abc123"}`))
			return
		}
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="https://%s/token"`, r.Host))
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := cache.Resolve(context.Background(), ociRepo(ts), "keycloak")

	if err == nil {
		t.Fatalf("expected the second 401 to surface rather than looping")
	}
}

func TestResolveOCIWithoutARealm(t *testing.T) {
	cache, ts := tlsCacheFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", "Basic")
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := cache.Resolve(context.Background(), ociRepo(ts), "keycloak")

	if err == nil {
		t.Fatalf("expected an error when the challenge has no bearer realm")
	}
}

func TestResolveOCITokenEndpointFailure(t *testing.T) {
	cache, ts := tlsCacheFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="https://%s/token"`, r.Host))
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := cache.Resolve(context.Background(), ociRepo(ts), "keycloak")

	if err == nil {
		t.Fatalf("expected the token failure to surface")
	}
}

func TestResolveOCIReportsATokenConnectionThatBreaks(t *testing.T) {
	cache, ts := tlsCacheFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				return
			}
			_ = conn.Close()
			return
		}
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="https://%s/token"`, r.Host))
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := cache.Resolve(context.Background(), ociRepo(ts), "keycloak")

	if err == nil {
		t.Fatal("a broken token connection was reported as a successful registry read")
	}
}

func TestResolveOCIAcceptsAccessToken(t *testing.T) {
	cache, ts := tlsCacheFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = w.Write([]byte(`{"access_token":"xyz"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer xyz" {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="https://%s/token"`, r.Host))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"tags":["1.0.0"]}`))
	})

	found, err := cache.Resolve(context.Background(), ociRepo(ts), "keycloak")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if newest(found, "keycloak") != "1.0.0" {
		t.Fatalf("keycloak = %q", newest(found, "keycloak"))
	}
}

func TestResolveOCIEmptyTokenResponse(t *testing.T) {
	cache, ts := tlsCacheFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="https://%s/token"`, r.Host))
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := cache.Resolve(context.Background(), ociRepo(ts), "keycloak")

	if err == nil {
		t.Fatalf("expected an error for an empty token response")
	}
}

func TestResolveOCIRejectsMalformedTokenJSON(t *testing.T) {
	cache, ts := tlsCacheFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = w.Write([]byte("{not json"))
			return
		}
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="https://%s/token"`, r.Host))
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := cache.Resolve(context.Background(), ociRepo(ts), "keycloak")

	if err == nil {
		t.Fatalf("expected a token parse error")
	}
}

func TestResolveOCIRejectsMalformedTagJSON(t *testing.T) {
	cache, ts := tlsCacheFor(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{not json"))
	})

	_, err := cache.Resolve(context.Background(), ociRepo(ts), "keycloak")

	if err == nil {
		t.Fatalf("expected a tag parse error")
	}
}

func TestResolveOCIWithNoUsableTags(t *testing.T) {
	cache, ts := tlsCacheFor(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tags":["latest","edge"]}`))
	})

	found, err := cache.Resolve(context.Background(), ociRepo(ts), "keycloak")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("found = %v, want empty", found)
	}
}

func TestResolveSurfacesNonAuthErrors(t *testing.T) {
	cache, ts := tlsCacheFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})

	_, err := cache.Resolve(context.Background(), ociRepo(ts), "keycloak")

	if err == nil {
		t.Fatalf("expected a 502 to surface")
	}
}

func TestResolveRejectsAnUnusableURL(t *testing.T) {
	cache, _ := cacheFor(t, indexHandler(new(int)))

	_, err := cache.Resolve(context.Background(), Repo{URL: "://bad"}, "podinfo")

	if err == nil {
		t.Fatalf("expected a request build error")
	}
}

func internetFor(t *testing.T, handler http.HandlerFunc) *Cache {
	t.Helper()
	ts := httptest.NewTLSServer(handler)
	t.Cleanup(ts.Close)
	return cacheThrough(t, ts.Client(), ts.Listener.Addr().String())
}

func TestCheckRepoURLSortsFetchableFromNot(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want bool
	}{
		{"an https repository", "https://charts.example.com/stable", true},
		{"a plain http repository", "http://charts.example.com/stable", true},
		{"an oci registry", "oci://registry.example.com/team", true},
		{"a public address", "https://93.184.216.34/charts", true},
		{"a file url", "file:///etc/passwd", false},
		{"a gopher url", "gopher://example.com/", false},
		{"no host at all", "https:///charts", false},
		{"cloud metadata", "http://169.254.169.254/latest/meta-data", false},
		{"loopback", "http://127.0.0.1:9090/charts", false},
		{"loopback by name", "http://localhost:9090/charts", false},
		{"a localhost subdomain", "http://spinoza.localhost/charts", false},
		{"localhost with a dns root", "http://localhost./charts", false},
		{"ipv6 loopback", "http://[::1]:9090/charts", false},
		{"an ipv4-mapped loopback", "http://[::ffff:127.0.0.1]/charts", false},
		{"a private range", "http://10.4.0.9/charts", false},
		{"carrier grade nat", "http://100.100.100.200/charts", false},
		{"a unique local address", "http://[fd00::1]/charts", false},
		{"the unspecified address", "http://0.0.0.0/charts", false},
		{"multicast", "http://224.0.0.1/charts", false},
		{"userinfo", "https://operator:secret@charts.example.com/charts", false},
		{"an encoded host", "https://%31%32%37.0.0.1/charts", false},
		{"unparseable", "://bad", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckRepoURL(tc.url)
			if (err == nil) != tc.want {
				t.Fatalf("CheckRepoURL(%q) = %v", tc.url, err)
			}
		})
	}
}

func TestARepositoryConnectionUsesTheAddressThatWasChecked(t *testing.T) {
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
	}
	connected := ""
	dial := func(_ context.Context, _, address string) (net.Conn, error) {
		connected = address
		return nil, errors.New("test dial stopped")
	}

	_, _ = publicDial(lookup, dial)(t.Context(), "tcp", "charts.example.com:443")

	if connected != "93.184.216.34:443" {
		t.Fatalf("dialed %q, want the checked address rather than the hostname", connected)
	}
}

func TestARepositoryConnectionNeedsAHostAndPort(t *testing.T) {
	lookedUp := false
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		lookedUp = true
		return nil, nil
	}
	dial := func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("unexpected dial")
	}

	_, err := publicDial(lookup, dial)(t.Context(), "tcp", "charts.example.com")

	if err == nil || !strings.Contains(err.Error(), "repository address") {
		t.Fatalf("error = %v, want the malformed address named", err)
	}
	if lookedUp {
		t.Fatal("a malformed address reached DNS")
	}
}

func TestATLSRepositoryConnectionNeedsAHostAndPort(t *testing.T) {
	lookedUp := false
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		lookedUp = true
		return nil, nil
	}
	dial := func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("unexpected dial")
	}

	_, err := publicTLSDial(defaultTransport(), lookup, dial)(t.Context(), "tcp", "charts.example.com")

	if err == nil || !strings.Contains(err.Error(), "repository address") {
		t.Fatalf("error = %v, want the malformed TLS address named", err)
	}
	if lookedUp {
		t.Fatal("a malformed TLS address reached DNS")
	}
}

func TestRepositoryDialRefusesLocalhostWithoutDNS(t *testing.T) {
	lookedUp := false
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		lookedUp = true
		return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
	}
	dial := func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("unexpected dial")
	}

	_, err := publicDial(lookup, dial)(t.Context(), "tcp", "localhost:443")

	if err == nil || !strings.Contains(err.Error(), "this machine") {
		t.Fatalf("error = %v, want localhost refused", err)
	}
	if lookedUp {
		t.Fatal("localhost reached DNS")
	}
}

func TestARepositoryDNSFailureIsReported(t *testing.T) {
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		return nil, errors.New("dns unavailable")
	}

	_, err := resolvePublic(t.Context(), lookup, "charts.example.com")

	if err == nil || !strings.Contains(err.Error(), "resolve repository host") {
		t.Fatalf("error = %v, want the DNS operation named", err)
	}
}

func TestARepositoryResolvingToNothingIsRefused(t *testing.T) {
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		return nil, nil
	}

	_, err := resolvePublic(t.Context(), lookup, "charts.example.com")

	if err == nil || !strings.Contains(err.Error(), "resolved to no addresses") {
		t.Fatalf("error = %v, want the empty DNS answer named", err)
	}
}

func TestAMixedPublicAndPrivateDNSAnswerIsRefusedBeforeDialling(t *testing.T) {
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{
			netip.MustParseAddr("93.184.216.34"),
			netip.MustParseAddr("10.0.0.8"),
		}, nil
	}
	dialed := false
	dial := func(context.Context, string, string) (net.Conn, error) {
		dialed = true
		return nil, errors.New("test dial stopped")
	}

	_, err := publicDial(lookup, dial)(t.Context(), "tcp", "charts.example.com:443")

	if err == nil {
		t.Fatal("a mixed public and private dns answer was accepted")
	}
	if dialed {
		t.Fatal("an address was dialed before the complete dns answer was validated")
	}
}

func TestAReboundHostnameIsCheckedAgainForTheNextConnection(t *testing.T) {
	lookups := 0
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		lookups++
		if lookups == 1 {
			return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}
	dials := 0
	dial := func(context.Context, string, string) (net.Conn, error) {
		dials++
		return nil, errors.New("test dial stopped")
	}
	guarded := publicDial(lookup, dial)

	_, _ = guarded(t.Context(), "tcp", "charts.example.com:443")
	_, err := guarded(t.Context(), "tcp", "charts.example.com:443")

	if err == nil || !strings.Contains(err.Error(), "non-public") {
		t.Fatalf("second connection error = %v, want the rebound address refused", err)
	}
	if lookups != 2 {
		t.Fatalf("dns lookups = %d, want one for every connection", lookups)
	}
	if dials != 1 {
		t.Fatalf("socket dials = %d, want only the first public answer dialed", dials)
	}
}

func TestAnIPv4MappedPrivateDNSAnswerIsRefused(t *testing.T) {
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("::ffff:127.0.0.1")}, nil
	}
	dialed := false
	dial := func(context.Context, string, string) (net.Conn, error) {
		dialed = true
		return nil, errors.New("test dial stopped")
	}

	_, err := publicDial(lookup, dial)(t.Context(), "tcp", "charts.example.com:443")

	if err == nil {
		t.Fatal("an ipv4-mapped loopback answer was accepted")
	}
	if dialed {
		t.Fatal("the mapped loopback address was dialed")
	}
}

func TestTheGuardedTransportCannotBypassAddressValidation(t *testing.T) {
	transport := defaultTransport()
	transport.Proxy = http.ProxyFromEnvironment
	bypassedHTTP := false
	transport.DialContext = func(context.Context, string, string) (net.Conn, error) {
		bypassedHTTP = true
		return nil, errors.New("unguarded http dial")
	}
	bypassedTLS := false
	transport.DialTLSContext = func(context.Context, string, string) (net.Conn, error) {
		bypassedTLS = true
		return nil, errors.New("unguarded tls dial")
	}
	client := &http.Client{Transport: transport}
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}
	dial := func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("guarded dial")
	}

	guarded := publicOnly(client, lookup, dial)
	configured, ok := guarded.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T", guarded.Transport)
	}
	if configured.Proxy != nil {
		t.Fatal("the guarded client can bypass validation through an http proxy")
	}
	_, httpErr := configured.DialContext(t.Context(), "tcp", "charts.example.com:80")
	if httpErr == nil || !strings.Contains(httpErr.Error(), "non-public") {
		t.Fatalf("http dial error = %v, want address validation", httpErr)
	}
	_, tlsErr := configured.DialTLSContext(t.Context(), "tcp", "charts.example.com:443")
	if tlsErr == nil || !strings.Contains(tlsErr.Error(), "non-public") {
		t.Fatalf("tls dial error = %v, want address validation", tlsErr)
	}
	if bypassedHTTP || bypassedTLS {
		t.Fatal("the guarded client used one of the caller's dialers")
	}
}

func TestTheGuardedTLSClientRefusesLegacyProtocols(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configured *tls.Config
		want       uint16
	}{
		{name: "no caller configuration", want: tls.VersionTLS12},
		{name: "a legacy minimum", configured: &tls.Config{MinVersion: tls.VersionTLS10}, want: tls.VersionTLS12},
		{name: "a stricter minimum", configured: &tls.Config{MinVersion: tls.VersionTLS13}, want: tls.VersionTLS13},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := &http.Transport{TLSClientConfig: tc.configured}

			got := tlsConfigFor(transport, "charts.example.com")

			if got.MinVersion != tc.want {
				t.Fatalf("minimum TLS version = %x, want %x", got.MinVersion, tc.want)
			}
			if got.ServerName != "charts.example.com" {
				t.Fatalf("server name = %q", got.ServerName)
			}
		})
	}
}

func TestARedirectOntoThisMachineIsRefused(t *testing.T) {
	cache := internetFor(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:9/index.yaml", http.StatusFound)
	})

	_, err := cache.Resolve(context.Background(), Repo{URL: "https://charts.example.com"}, "podinfo")

	if err == nil {
		t.Fatal("a chart repository redirected the fetch onto loopback and it followed")
	}
}

func TestARedirectToAPublicHostIsFollowed(t *testing.T) {
	cache := internetFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.yaml" {
			http.Redirect(w, r, "https://example.com/mirror/index.yaml", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(indexBody))
	})

	found, err := cache.Resolve(context.Background(), Repo{URL: "https://example.com"}, "podinfo")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if newest(found, "podinfo") != "6.15.1" {
		t.Fatalf("podinfo = %q", newest(found, "podinfo"))
	}
}

func TestAnEndlessRedirectChainIsCutOff(t *testing.T) {
	hops := 0
	cache := internetFor(t, func(w http.ResponseWriter, r *http.Request) {
		hops++
		http.Redirect(w, r, "https://example.com/again", http.StatusFound)
	})

	_, err := cache.Resolve(context.Background(), Repo{URL: "https://example.com"}, "podinfo")

	if err == nil {
		t.Fatal("expected the redirect chain to be cut off")
	}
	if hops > maxRedirects+1 {
		t.Fatalf("hops = %d, want the chain stopped at %d", hops, maxRedirects)
	}
}

func TestABearerRealmOnAnotherHostIsRefused(t *testing.T) {
	cache := internetFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="http://169.254.169.254/latest/meta-data",service="registry"`)
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := cache.Resolve(context.Background(), Repo{URL: "oci://registry.example.com/team", OCI: true}, "keycloak")

	if err == nil {
		t.Fatal("the registry pointed the token request at another host and it followed")
	}
}

func TestABearerRealmOnASiblingOfTheRegistryIsAllowed(t *testing.T) {
	cache := internetFor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "auth.example.com" {
			_, _ = w.Write([]byte(`{"token":"abc123"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer abc123" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="https://auth.example.com/token",service="registry"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"tags":["1.0.0"]}`))
	})

	found, err := cache.Resolve(context.Background(), Repo{URL: "oci://registry.example.com/team", OCI: true}, "keycloak")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if newest(found, "keycloak") != "1.0.0" {
		t.Fatalf("keycloak = %q", newest(found, "keycloak"))
	}
}

func TestTheTokenRequestEscapesTheChallengeAndKeepsTheRealmQuery(t *testing.T) {
	asked := ""
	cache := internetFor(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/token") {
			asked = r.URL.RawQuery
			_, _ = w.Write([]byte(`{"token":"abc123"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer abc123" {
			w.Header().Set("WWW-Authenticate",
				`Bearer realm="https://example.com/token?tenant=acme",service="reg istry",scope="repository:team/keycloak:pull"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"tags":["1.0.0"]}`))
	})

	_, err := cache.Resolve(context.Background(), Repo{URL: "oci://example.com/team", OCI: true}, "keycloak")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	query, parseErr := url.ParseQuery(asked)
	if parseErr != nil {
		t.Fatalf("parse %q: %v", asked, parseErr)
	}
	if query.Get("tenant") != "acme" {
		t.Fatalf("query = %q, want the realm's own parameters kept", asked)
	}
	if query.Get("service") != "reg istry" {
		t.Fatalf("service = %q, want it escaped rather than pasted in", query.Get("service"))
	}
	if query.Get("scope") != "repository:team/keycloak:pull" {
		t.Fatalf("scope = %q", query.Get("scope"))
	}
}

func TestTheChartNameCannotReshapeTheRegistryPath(t *testing.T) {
	asked := ""
	cache := internetFor(t, func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"tags":["1.0.0"]}`))
	})

	_, err := cache.Resolve(context.Background(), Repo{URL: "oci://example.com/team", OCI: true}, "../../secrets")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if asked != "/v2/team/..%2F..%2Fsecrets/tags/list" {
		t.Fatalf("path = %q, want the chart name kept inside one segment", asked)
	}
}

func TestNewerComparesSemver(t *testing.T) {
	cases := []struct {
		current string
		latest  string
		want    bool
	}{
		{"6.14.0", "6.15.1", true},
		{"6.15.1", "6.15.1", false},
		{"6.16.0", "6.15.1", false},
		{"v1.21.0", "v1.22.0", true},
		{"1.0.0+86d6320df72b", "1.0.1", true},
		{"", "6.15.1", false},
		{"6.14.0", "", false},
		{"not-semver", "6.15.1", false},
		{"6.14.0", "not-semver", false},
	}
	for _, tc := range cases {
		if got := Newer(tc.current, tc.latest); got != tc.want {
			t.Fatalf("Newer(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.want)
		}
	}
}

func TestParseChallengeIgnoresJunk(t *testing.T) {
	got := parseChallenge(`Bearer realm="https://auth.example/token",service="reg",bogus`)

	if got["realm"] != "https://auth.example/token" {
		t.Fatalf("realm = %q", got["realm"])
	}
	if got["service"] != "reg" {
		t.Fatalf("service = %q", got["service"])
	}
}

func TestSortVersionsAcceptsAnOCIBuildMetadataTag(t *testing.T) {
	got := sortVersions([]string{"1.2.0", "1.3.0_20260101", "1.1.0"})

	want := []string{"1.3.0_20260101", "1.2.0", "1.1.0"}
	if !slices.Equal(got, want) {
		t.Fatalf("sorted = %v, want %v; Helm writes + as _ in an OCI tag, so that one was being dropped", got, want)
	}
}

func TestSortVersionsKeepsTheTagItWasGiven(t *testing.T) {
	got := sortVersions([]string{"2.0.0_abc"})

	if !slices.Equal(got, []string{"2.0.0_abc"}) {
		t.Fatalf("sorted = %v, want the tag as published rather than a rewritten one", got)
	}
}

func TestSortVersionsStillSkipsPrereleases(t *testing.T) {
	got := sortVersions([]string{"1.0.0", "2.0.0-rc.1"})

	if !slices.Equal(got, []string{"1.0.0"}) {
		t.Fatalf("sorted = %v, want a prerelease left out the way Helm leaves it out", got)
	}
}

func TestSortVersionsReportsNothingForAnAllPrereleaseRepo(t *testing.T) {
	got := sortVersions([]string{"2.0.0-rc.1", "2.0.0-rc.2"})

	if len(got) != 0 {
		t.Fatalf("sorted = %v, want none; suggesting an upgrade to a release candidate is worse than saying nothing", got)
	}
}

func TestSortVersionsSkipsATagThatIsNotSemverEvenAfterTranslation(t *testing.T) {
	got := sortVersions([]string{"1.0.0_build_2"})

	if len(got) != 0 {
		t.Fatalf("sorted = %v; underscores are not legal in build metadata, so this is not a version", got)
	}
}

func TestSortVersionsComparesTranslatedTagsByVersionNotText(t *testing.T) {
	got := sortVersions([]string{"1.10.0_20260101", "1.9.0_20260102"})

	want := []string{"1.10.0_20260101", "1.9.0_20260102"}
	if !slices.Equal(got, want) {
		t.Fatalf("sorted = %v, want the higher version first rather than the later build stamp", got)
	}
}

func TestVersionsReturnsTheSortedListAndCaches(t *testing.T) {
	hits := 0
	cache, ts := cacheFor(t, indexHandler(&hits))
	repo := Repo{URL: ts.URL}

	got, err := cache.Versions(context.Background(), repo, "podinfo")
	if err != nil {
		t.Fatalf("versions: %v", err)
	}
	want := []string{"6.15.1", "6.14.0", "6.9.0"}
	if !slices.Equal(got, want) {
		t.Fatalf("versions = %v, want %v", got, want)
	}

	again, err := cache.Versions(context.Background(), repo, "podinfo")
	if err != nil {
		t.Fatalf("versions again: %v", err)
	}
	if !slices.Equal(again, want) {
		t.Fatalf("versions again = %v, want %v", again, want)
	}
	if hits != 1 {
		t.Fatalf("hits = %d, want the second read served from the cache", hits)
	}
}

func TestVersionsRefetchesAfterTheTTL(t *testing.T) {
	hits := 0
	cache, ts := cacheFor(t, indexHandler(&hits))
	repo := Repo{URL: ts.URL}

	_, err := cache.Versions(context.Background(), repo, "podinfo")
	if err != nil {
		t.Fatalf("versions: %v", err)
	}
	cache.now = func() time.Time { return time.Now().Add(2 * DefaultTTL) }
	_, err = cache.Versions(context.Background(), repo, "podinfo")
	if err != nil {
		t.Fatalf("versions after ttl: %v", err)
	}
	if hits != 2 {
		t.Fatalf("hits = %d, want a refetch once the ttl passed", hits)
	}
}

func TestVersionsReportsAFetchFailure(t *testing.T) {
	cache, _ := cacheFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := cache.Versions(context.Background(), Repo{URL: "https://example.com"}, "podinfo")

	if err == nil {
		t.Fatal("expected the failed index fetch to surface")
	}
}

func TestVersionsForAChartTheRepoDoesNotCarry(t *testing.T) {
	hits := 0
	cache, ts := cacheFor(t, indexHandler(&hits))

	got, err := cache.Versions(context.Background(), Repo{URL: ts.URL}, "absent")
	if err != nil {
		t.Fatalf("versions: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("versions = %v, want none for a chart the index does not list", got)
	}
}

func TestLatestReadsTheFirstCachedVersion(t *testing.T) {
	hits := 0
	cache, ts := cacheFor(t, indexHandler(&hits))
	repo := Repo{URL: ts.URL}

	cache.Warm(repo, "podinfo")
	cache.Wait()

	if cache.Latest(repo, "podinfo") != "6.15.1" {
		t.Fatalf("latest = %q, want the newest of the sorted list", cache.Latest(repo, "podinfo"))
	}
	if cache.Latest(repo, "absent") != "" {
		t.Fatalf("latest = %q, want nothing for an unknown chart", cache.Latest(repo, "absent"))
	}
}

func TestCheckFetchable(t *testing.T) {
	cases := []struct {
		raw string
		ok  bool
	}{
		{"https://charts.example.com", true},
		{"http://127.0.0.1:8879", true},
		{"oci://registry.example.com/team", true},
		{"ftp://charts.example.com", false},
		{"https://", false},
		{"://bad", false},
	}
	for _, tc := range cases {
		err := CheckFetchable(tc.raw)
		if tc.ok && err != nil {
			t.Fatalf("CheckFetchable(%q) = %v, want it allowed", tc.raw, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("CheckFetchable(%q) allowed, want it refused", tc.raw)
		}
	}
}

func TestValidVersion(t *testing.T) {
	cases := []struct {
		version string
		ok      bool
	}{
		{"6.15.1", true},
		{"v1.2.3", true},
		{"1.3.0_20260101", true},
		{"not-semver", false},
		{"--repo", false},
		{"", false},
	}
	for _, tc := range cases {
		if ValidVersion(tc.version) != tc.ok {
			t.Fatalf("ValidVersion(%q) = %v, want %v", tc.version, !tc.ok, tc.ok)
		}
	}
}

func TestNewerComparesOCIBuildMetadataTags(t *testing.T) {
	if !Newer("1.9.0_20260101", "1.10.0_20260102") {
		t.Fatal("an outdated OCI release was reported as up to date; maxVersion hands back the raw underscore tag")
	}
}

func TestNewerRefusesAnUpToDateOCITag(t *testing.T) {
	if Newer("1.10.0_20260102", "1.10.0_20260102") {
		t.Fatal("a release on the newest tag was reported as outdated")
	}
}

func TestNewerStillRefusesGarbage(t *testing.T) {
	if Newer("1.0.0_build_2", "2.0.0") {
		t.Fatal("a tag that is not a version was compared anyway")
	}
}

func TestSameRegistryAcceptsWhatBelongsToTheSameSite(t *testing.T) {
	cases := []struct {
		name     string
		realm    string
		registry string
		want     bool
	}{
		{name: "the very same authority", realm: "ghcr.io", registry: "ghcr.io", want: true},
		{name: "the same host on another port", realm: "ghcr.io:443", registry: "ghcr.io", want: true},
		{name: "a sibling under one domain", realm: "auth.ghcr.io", registry: "registry.ghcr.io", want: true},
		{name: "different sites under a two label public suffix", realm: "auth.attacker.co.uk", registry: "registry.example.co.uk", want: false},
		{name: "a different site", realm: "auth.example.com", registry: "ghcr.io", want: false},
		{name: "an ip against a name", realm: "127.0.0.1", registry: "ghcr.io", want: false},
		{name: "the same ip twice", realm: "127.0.0.1:5000", registry: "127.0.0.1", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameRegistry(tc.realm, tc.registry); got != tc.want {
				t.Fatalf("sameRegistry(%q, %q) = %v, want %v", tc.realm, tc.registry, got, tc.want)
			}
		})
	}
}

func TestHostOnlyDropsThePortWhenThereIsOne(t *testing.T) {
	if got := hostOnly("ghcr.io:443"); got != "ghcr.io" {
		t.Fatalf("host = %q, want ghcr.io", got)
	}
	if got := hostOnly("ghcr.io"); got != "ghcr.io" {
		t.Fatalf("host = %q, want the authority unchanged", got)
	}
}

func TestParentDomainUsesTheRegistrableDomain(t *testing.T) {
	if got := parentDomain("auth.docker.example.com"); got != "example.com" {
		t.Fatalf("parent = %q, want example.com", got)
	}
	if got := parentDomain("auth.docker.example.co.uk"); got != "example.co.uk" {
		t.Fatalf("parent = %q, want example.co.uk", got)
	}
	if got := parentDomain("localhost"); got != "localhost" {
		t.Fatalf("parent = %q, want the single label unchanged", got)
	}
}

func TestTokenEndpointRefusesAChallengeItCannotUse(t *testing.T) {
	registry, err := url.Parse("https://ghcr.io")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cases := []struct {
		name   string
		params map[string]string
		want   string
	}{
		{name: "no realm at all", params: map[string]string{}, want: "no bearer realm"},
		{name: "a realm that is not a url", params: map[string]string{"realm": "://"}, want: "bearer realm"},
		{name: "a realm that is not http", params: map[string]string{"realm": "ftp://ghcr.io/token"}, want: "not an http url"},
		{
			name:   "a realm on another site",
			params: map[string]string{"realm": "https://auth.elsewhere.com/token"},
			want:   "does not belong to ghcr.io",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tokenEndpoint(tc.params, registry)

			if err == nil {
				t.Fatal("tokenEndpoint returned nil error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want it to mention %q", err.Error(), tc.want)
			}
		})
	}
}

func TestCheckHostRefusesAnEmptyHost(t *testing.T) {
	err := checkHost("")

	if err == nil {
		t.Fatal("checkHost returned nil error for an empty host")
	}
	if err.Error() != "repository url has no host" {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestSearchListsWhatTheIndexOffers(t *testing.T) {
	cache, ts := cacheFor(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`entries:
  podinfo:
    - version: 6.10.0
      description: a tiny greeter
    - version: 6.9.0
      description: an older greeter
  redis:
    - version: 20.0.1
      description: an in-memory store
`))
	})

	found, err := cache.Search(context.Background(), Repo{URL: ts.URL}, "pod", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(found) != 1 {
		t.Fatalf("found = %+v, want only podinfo", found)
	}
	if found[0].Version != "6.10.0" {
		t.Fatalf("version = %q, want the newest", found[0].Version)
	}
	if found[0].Description != "a tiny greeter" {
		t.Fatalf("description = %q, want the newest version's", found[0].Description)
	}
}

func TestSearchMatchesTheDescriptionToo(t *testing.T) {
	cache, ts := cacheFor(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`entries:
  redis:
    - version: 20.0.1
      description: an in-memory store
`))
	})

	found, err := cache.Search(context.Background(), Repo{URL: ts.URL}, "in-memory", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(found) != 1 || found[0].Name != "redis" {
		t.Fatalf("found = %+v", found)
	}
}

func TestSearchPutsNameMatchesBeforeDescriptionMatches(t *testing.T) {
	cache, ts := cacheFor(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`entries:
  cache-proxy:
    - version: 1.0.0
      description: a proxy
  redis:
    - version: 20.0.1
      description: a cache
`))
	})

	found, err := cache.Search(context.Background(), Repo{URL: ts.URL}, "cache", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(found) != 2 {
		t.Fatalf("found = %+v", found)
	}
	if found[0].Name != "cache-proxy" {
		t.Fatalf("first = %q, want the name match first", found[0].Name)
	}
}

func TestSearchKeepsAtMostTheLimit(t *testing.T) {
	cache, ts := cacheFor(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`entries:
  chart-a:
    - version: 1.0.0
  chart-b:
    - version: 1.0.0
  chart-c:
    - version: 1.0.0
`))
	})

	found, err := cache.Search(context.Background(), Repo{URL: ts.URL}, "chart", 2)
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(found) != 2 {
		t.Fatalf("found = %d, want the limit", len(found))
	}
}

func TestAnIndexLargerThanItsLimitIsNotAcceptedPartially(t *testing.T) {
	body := "entries:\n  web:\n    - version: 1.0.0\n"

	_, err := parseBoundedIndex(strings.NewReader(body+"extra"), len(body))

	if err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("err = %v", err)
	}
}

func TestJSONLargerThanItsLimitIsNotAcceptedPartially(t *testing.T) {
	body := `{"tags":["1.0.0"]}`
	var doc struct {
		Tags []string `json:"tags"`
	}

	err := decodeBoundedJSON(strings.NewReader(body+"extra"), len(body), &doc)

	if err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("err = %v", err)
	}
}

func TestAnIndexReadFailureIsNotAcceptedPartially(t *testing.T) {
	_, err := parseBoundedIndex(&interruptedReader{}, maxBodyBytes)

	if err == nil || !strings.Contains(err.Error(), "repository response interrupted") {
		t.Fatalf("error = %v, want the interrupted response reported", err)
	}
}

func TestAnEmptySearchListsEverything(t *testing.T) {
	cache, ts := cacheFor(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`entries:
  chart-a:
    - version: 1.0.0
  chart-b:
    - version: 1.0.0
`))
	})

	found, err := cache.Search(context.Background(), Repo{URL: ts.URL}, "", 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(found) != 2 {
		t.Fatalf("found = %+v, want both", found)
	}
}

func TestSearchRefusesAnOCIRegistry(t *testing.T) {
	cache := New(context.Background(), http.DefaultClient, DefaultTTL)

	_, err := cache.Search(context.Background(), Repo{URL: "oci://registry.example.com/charts", OCI: true}, "podinfo", 10)

	if err == nil {
		t.Fatal("an oci registry was listed")
	}
	if !strings.Contains(err.Error(), "name the chart instead") {
		t.Fatalf("error = %v", err)
	}
}

func TestSearchReportsAnIndexItCannotRead(t *testing.T) {
	cache, ts := cacheFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := cache.Search(context.Background(), Repo{URL: ts.URL}, "podinfo", 10)

	if err == nil {
		t.Fatal("a missing index reported success")
	}
}

func TestSearchServesTheSecondAskFromTheCache(t *testing.T) {
	asks := 0
	cache, ts := cacheFor(t, func(w http.ResponseWriter, r *http.Request) {
		asks++
		_, _ = w.Write([]byte("entries:\n  podinfo:\n    - version: 6.10.0\n"))
	})

	_, first := cache.Search(context.Background(), Repo{URL: ts.URL}, "podinfo", 10)
	_, second := cache.Search(context.Background(), Repo{URL: ts.URL}, "pod", 10)

	if first != nil || second != nil {
		t.Fatalf("search: %v %v", first, second)
	}
	if asks != 1 {
		t.Fatalf("index fetched %d times, want once", asks)
	}
}

func TestSearchMatchesTheMiddleOfAName(t *testing.T) {
	cache, ts := cacheFor(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`entries:
  podinfo:
    - version: 6.14.1
      description: a tiny greeter
  kube-prometheus-stack:
    - version: 88.3.0
      description: a bundle
`))
	})

	found, err := cache.Search(context.Background(), Repo{URL: ts.URL}, "prometheus", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(found) != 1 || found[0].Name != "kube-prometheus-stack" {
		t.Fatalf("found = %+v, want the chart whose name carries the word", found)
	}
}

func TestAChartWithNoUsableVersionIsLeftOutOfTheCatalogue(t *testing.T) {
	cache, ts := cacheFor(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`entries:
  podinfo:
    - version: 6.14.1
  broken:
    - version: not-a-version
  prereleased:
    - version: 1.0.0-rc1
`))
	})

	found, err := cache.Search(context.Background(), Repo{URL: ts.URL}, "", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(found) != 1 || found[0].Name != "podinfo" {
		t.Fatalf("found = %+v, want only the chart with a version helm could install", found)
	}
}
