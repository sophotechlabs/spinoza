package charts

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	if found["podinfo"] != "6.15.1" {
		t.Fatalf("podinfo = %q, want 6.15.1 (prerelease 6.16.0-rc.1 must be skipped)", found["podinfo"])
	}
	if found["other"] != "1.2.3" {
		t.Fatalf("other = %q, want 1.2.3", found["other"])
	}
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
	if found["keycloak"] != "0.22.0" {
		t.Fatalf("keycloak = %q, want 0.22.0", found["keycloak"])
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
	if found["keycloak"] != "1.0.0" {
		t.Fatalf("keycloak = %q", found["keycloak"])
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
