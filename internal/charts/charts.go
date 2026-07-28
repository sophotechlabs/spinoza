package charts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"
)

const (
	maxBodyBytes  = 64 << 20
	fetchTimeout  = 30 * time.Second
	DefaultTTL    = 30 * time.Minute
	indexFilename = "index.yaml"
)

type Repo struct {
	URL string
	OCI bool
}

type key struct {
	repo  Repo
	chart string
}

type Cache struct {
	ctx    context.Context
	client *http.Client
	ttl    time.Duration
	now    func() time.Time
	wg     sync.WaitGroup

	mu       sync.Mutex
	versions map[key]string
	fetched  map[key]time.Time
	inflight map[key]bool
}

func New(ctx context.Context, client *http.Client, ttl time.Duration) *Cache {
	return &Cache{
		ctx:      ctx,
		client:   client,
		ttl:      ttl,
		now:      time.Now,
		versions: map[key]string{},
		fetched:  map[key]time.Time{},
		inflight: map[key]bool{},
	}
}

func (c *Cache) Latest(repo Repo, chart string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.versions[key{repo: repo, chart: chart}]
}

func (c *Cache) Warm(repo Repo, chart string) {
	if repo.URL == "" {
		return
	}
	if chart == "" {
		return
	}
	unit := fetchUnit(repo, chart)

	c.mu.Lock()
	if c.inflight[unit] {
		c.mu.Unlock()
		return
	}
	last, seen := c.fetched[unit]
	if seen && c.now().Sub(last) < c.ttl {
		c.mu.Unlock()
		return
	}
	c.inflight[unit] = true
	c.mu.Unlock()

	c.wg.Go(func() {
		c.refresh(unit, repo, chart)
	})
}

func (c *Cache) Wait() {
	c.wg.Wait()
}

func (c *Cache) refresh(unit key, repo Repo, chart string) {
	ctx, cancel := context.WithTimeout(c.ctx, fetchTimeout)
	defer cancel()

	found, err := c.Resolve(ctx, repo, chart)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.inflight[unit] = false
	c.fetched[unit] = c.now()
	if err != nil {
		return
	}
	for name, version := range found {
		c.versions[key{repo: repo, chart: name}] = version
	}
}

func (c *Cache) Resolve(ctx context.Context, repo Repo, chart string) (map[string]string, error) {
	if repo.OCI {
		return c.resolveOCI(ctx, repo, chart)
	}
	return c.resolveIndex(ctx, repo)
}

func (c *Cache) resolveIndex(ctx context.Context, repo Repo) (map[string]string, error) {
	url := strings.TrimSuffix(repo.URL, "/") + "/" + indexFilename
	body, err := c.get(ctx, url, "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()

	var doc struct {
		Entries map[string][]struct {
			Version string `yaml:"version"`
		} `yaml:"entries"`
	}
	decodeErr := yaml.NewDecoder(io.LimitReader(body, maxBodyBytes)).Decode(&doc)
	if decodeErr != nil {
		return nil, fmt.Errorf("parse %s: %w", url, decodeErr)
	}

	out := map[string]string{}
	for name, releases := range doc.Entries {
		raw := make([]string, 0, len(releases))
		for _, release := range releases {
			raw = append(raw, release.Version)
		}
		latest := maxVersion(raw)
		if latest != "" {
			out[name] = latest
		}
	}
	return out, nil
}

func (c *Cache) resolveOCI(ctx context.Context, repo Repo, chart string) (map[string]string, error) {
	host, path, err := splitOCI(repo.URL)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("https://%s/v2/%s/%s/tags/list", host, path, chart)

	body, err := c.get(ctx, url, "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()

	var doc struct {
		Tags []string `json:"tags"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(body, maxBodyBytes)).Decode(&doc)
	if decodeErr != nil {
		return nil, fmt.Errorf("parse tags for %s: %w", url, decodeErr)
	}

	latest := maxVersion(doc.Tags)
	if latest == "" {
		return map[string]string{}, nil
	}
	return map[string]string{chart: latest}, nil
}

func (c *Cache) get(ctx context.Context, url, token string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusOK {
		return resp.Body, nil
	}

	challenge := resp.Header.Get("WWW-Authenticate")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		return nil, fmt.Errorf("%s: status %d", url, resp.StatusCode)
	}
	if token != "" {
		return nil, fmt.Errorf("%s: status %d", url, resp.StatusCode)
	}
	fresh, tokenErr := c.token(ctx, challenge)
	if tokenErr != nil {
		return nil, tokenErr
	}
	return c.get(ctx, url, fresh)
}

func (c *Cache) token(ctx context.Context, challenge string) (string, error) {
	params := parseChallenge(challenge)
	realm := params["realm"]
	if realm == "" {
		return "", fmt.Errorf("no bearer realm in %q", challenge)
	}
	url := realm + "?service=" + params["service"] + "&scope=" + params["scope"]

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request: status %d", resp.StatusCode)
	}

	var doc struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(&doc)
	if decodeErr != nil {
		return "", fmt.Errorf("parse token: %w", decodeErr)
	}
	if doc.Token != "" {
		return doc.Token, nil
	}
	if doc.AccessToken != "" {
		return doc.AccessToken, nil
	}
	return "", errors.New("empty token response")
}

func parseChallenge(header string) map[string]string {
	out := map[string]string{}
	trimmed := strings.TrimSpace(strings.TrimPrefix(header, "Bearer"))
	for part := range strings.SplitSeq(trimmed, ",") {
		pair := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(pair) != 2 {
			continue
		}
		out[pair[0]] = strings.Trim(pair[1], `"`)
	}
	return out
}

func splitOCI(url string) (host, path string, err error) {
	trimmed := strings.TrimPrefix(url, "oci://")
	trimmed = strings.Trim(trimmed, "/")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("cannot split oci url %q", url)
	}
	return parts[0], parts[1], nil
}

func fetchUnit(repo Repo, chart string) key {
	if repo.OCI {
		return key{repo: repo, chart: chart}
	}
	return key{repo: repo}
}

func maxVersion(raw []string) string {
	best := ""
	var bestParsed *semver.Version
	for _, candidate := range raw {
		parsed, err := semver.NewVersion(candidate)
		if err != nil {
			continue
		}
		if parsed.Prerelease() != "" {
			continue
		}
		if bestParsed != nil && !parsed.GreaterThan(bestParsed) {
			continue
		}
		bestParsed = parsed
		best = candidate
	}
	return best
}

func Newer(current, latest string) bool {
	if latest == "" {
		return false
	}
	if current == "" {
		return false
	}
	currentParsed, currentErr := semver.NewVersion(current)
	latestParsed, latestErr := semver.NewVersion(latest)
	if currentErr != nil || latestErr != nil {
		return false
	}
	return latestParsed.GreaterThan(currentParsed)
}
