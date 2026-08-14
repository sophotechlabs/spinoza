package charts

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
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

func cacheFor(t *testing.T, handler http.HandlerFunc) (*Cache, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return New(ctx, ts.Client(), DefaultTTL), ts
}

func tlsCacheFor(t *testing.T, handler http.HandlerFunc) (*Cache, *httptest.Server) {
	t.Helper()
	ts := httptest.NewTLSServer(handler)
	t.Cleanup(ts.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return New(ctx, ts.Client(), DefaultTTL), ts
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

func TestResolveIndexRejectsMalformedYAML(t *testing.T) {
	cache, ts := cacheFor(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("entries: [oops"))
	})

	_, err := cache.Resolve(context.Background(), Repo{URL: ts.URL}, "podinfo")

	if err == nil {
		t.Fatalf("expected a parse error")
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
	client := ts.Client()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want one that can be redialled", client.Transport)
	}
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		var dialer net.Dialer
		return dialer.DialContext(ctx, network, ts.Listener.Addr().String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return New(ctx, client, DefaultTTL)
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
		{"ipv6 loopback", "http://[::1]:9090/charts", false},
		{"an ipv4-mapped loopback", "http://[::ffff:127.0.0.1]/charts", false},
		{"a private range", "http://10.4.0.9/charts", false},
		{"a unique local address", "http://[fd00::1]/charts", false},
		{"the unspecified address", "http://0.0.0.0/charts", false},
		{"multicast", "http://224.0.0.1/charts", false},
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
