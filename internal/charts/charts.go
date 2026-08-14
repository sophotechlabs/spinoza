package charts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
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
	maxRedirects  = 10
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
	lists    map[key][]string
	fetched  map[key]time.Time
	inflight map[key]bool
}

func New(ctx context.Context, client *http.Client, ttl time.Duration) *Cache {
	return &Cache{
		ctx:      ctx,
		client:   publicOnly(client),
		ttl:      ttl,
		now:      time.Now,
		lists:    map[key][]string{},
		fetched:  map[key]time.Time{},
		inflight: map[key]bool{},
	}
}

func publicOnly(client *http.Client) *http.Client {
	guarded := *client
	guarded.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return errors.New("stopped after 10 redirects")
		}
		return CheckRepoURL(req.URL.String())
	}
	return &guarded
}

func CheckRepoURL(raw string) error {
	parsed, err := fetchableURL(raw)
	if err != nil {
		return err
	}
	return checkHost(parsed.Hostname())
}

func CheckFetchable(raw string) error {
	_, err := fetchableURL(raw)
	return err
}

func fetchableURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("repository url %q: %w", raw, err)
	}
	if !fetchableScheme(parsed.Scheme) {
		return nil, fmt.Errorf("repository url %q: spinoza fetches http, https and oci only", raw)
	}
	if parsed.Host == "" {
		return nil, errors.New("repository url has no host")
	}
	return parsed, nil
}

func ValidVersion(version string) bool {
	_, err := semver.NewVersion(ociTagToSemver(version))
	return err == nil
}

func fetchableScheme(scheme string) bool {
	switch scheme {
	case "http", "https", "oci":
		return true
	default:
		return false
	}
}

func checkHost(host string) error {
	if host == "" {
		return errors.New("repository url has no host")
	}
	if localName(host) {
		return fmt.Errorf("repository host %q is this machine", host)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil
	}
	if !routableIP(ip) {
		return fmt.Errorf("repository host %q is not a public address", host)
	}
	return nil
}

func localName(host string) bool {
	lowered := strings.ToLower(host)
	if lowered == "localhost" {
		return true
	}
	return strings.HasSuffix(lowered, ".localhost")
}

func routableIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() {
		return false
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	return !ip.IsMulticast()
}

func (c *Cache) Latest(repo Repo, chart string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	list := c.lists[key{repo: repo, chart: chart}]
	if len(list) == 0 {
		return ""
	}
	return list[0]
}

func (c *Cache) Versions(ctx context.Context, repo Repo, chart string) ([]string, error) {
	unit := fetchUnit(repo, chart)
	entry := key{repo: repo, chart: chart}

	c.mu.Lock()
	last, seen := c.fetched[unit]
	if seen && c.now().Sub(last) < c.ttl {
		cached := slices.Clone(c.lists[entry])
		c.mu.Unlock()
		return cached, nil
	}
	c.mu.Unlock()

	found, err := c.Resolve(ctx, repo, chart)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.fetched[unit] = c.now()
	for name, list := range found {
		c.lists[key{repo: repo, chart: name}] = list
	}
	return slices.Clone(c.lists[entry]), nil
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
	for name, list := range found {
		c.lists[key{repo: repo, chart: name}] = list
	}
}

func (c *Cache) Resolve(ctx context.Context, repo Repo, chart string) (map[string][]string, error) {
	if repo.OCI {
		return c.resolveOCI(ctx, repo, chart)
	}
	return c.resolveIndex(ctx, repo)
}

func (c *Cache) resolveIndex(ctx context.Context, repo Repo) (map[string][]string, error) {
	endpoint := strings.TrimSuffix(repo.URL, "/") + "/" + indexFilename
	body, err := c.get(ctx, endpoint, "")
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
		return nil, fmt.Errorf("parse %s: %w", endpoint, decodeErr)
	}

	out := map[string][]string{}
	for name, releases := range doc.Entries {
		raw := make([]string, 0, len(releases))
		for _, release := range releases {
			raw = append(raw, release.Version)
		}
		sorted := sortVersions(raw)
		if len(sorted) > 0 {
			out[name] = sorted
		}
	}
	return out, nil
}

func (c *Cache) resolveOCI(ctx context.Context, repo Repo, chart string) (map[string][]string, error) {
	host, path, err := splitOCI(repo.URL)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("https://%s/v2/%s/%s/tags/list", host, path, url.PathEscape(chart))

	body, err := c.get(ctx, endpoint, "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()

	var doc struct {
		Tags []string `json:"tags"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(body, maxBodyBytes)).Decode(&doc)
	if decodeErr != nil {
		return nil, fmt.Errorf("parse tags for %s: %w", endpoint, decodeErr)
	}

	sorted := sortVersions(doc.Tags)
	if len(sorted) == 0 {
		return map[string][]string{}, nil
	}
	return map[string][]string{chart: sorted}, nil
}

func (c *Cache) get(ctx context.Context, endpoint, token string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
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
		return nil, fmt.Errorf("%s: status %d", endpoint, resp.StatusCode)
	}
	if token != "" {
		return nil, fmt.Errorf("%s: status %d", endpoint, resp.StatusCode)
	}
	fresh, tokenErr := c.token(ctx, challenge, req.URL)
	if tokenErr != nil {
		return nil, tokenErr
	}
	return c.get(ctx, endpoint, fresh)
}

func (c *Cache) token(ctx context.Context, challenge string, registry *url.URL) (string, error) {
	params := parseChallenge(challenge)
	endpoint, err := tokenEndpoint(params, registry)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
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

func tokenEndpoint(params map[string]string, registry *url.URL) (string, error) {
	realm := params["realm"]
	if realm == "" {
		return "", fmt.Errorf("no bearer realm in the challenge from %s", registry.Host)
	}
	parsed, err := url.Parse(realm)
	if err != nil {
		return "", fmt.Errorf("bearer realm %q: %w", realm, err)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", fmt.Errorf("bearer realm %q is not an http url", realm)
	}
	if !sameRegistry(parsed.Host, registry.Host) {
		return "", fmt.Errorf("bearer realm %q does not belong to %s", realm, registry.Host)
	}
	query := parsed.Query()
	query.Set("service", params["service"])
	query.Set("scope", params["scope"])
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func sameRegistry(realm, registry string) bool {
	if strings.EqualFold(realm, registry) {
		return true
	}
	realmHost := hostOnly(realm)
	registryHost := hostOnly(registry)
	if strings.EqualFold(realmHost, registryHost) {
		return true
	}
	if net.ParseIP(realmHost) != nil || net.ParseIP(registryHost) != nil {
		return false
	}
	return strings.EqualFold(parentDomain(realmHost), parentDomain(registryHost))
}

func hostOnly(authority string) string {
	host, _, err := net.SplitHostPort(authority)
	if err != nil {
		return authority
	}
	return host
}

func parentDomain(host string) string {
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return host
	}
	return strings.Join(labels[len(labels)-2:], ".")
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

func splitOCI(raw string) (host, path string, err error) {
	trimmed := strings.TrimPrefix(raw, "oci://")
	trimmed = strings.Trim(trimmed, "/")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("cannot split oci url %q", raw)
	}
	return parts[0], parts[1], nil
}

func fetchUnit(repo Repo, chart string) key {
	if repo.OCI {
		return key{repo: repo, chart: chart}
	}
	return key{repo: repo}
}

type sortable struct {
	original string
	parsed   *semver.Version
}

func sortVersions(raw []string) []string {
	entries := make([]sortable, 0, len(raw))
	for _, candidate := range raw {
		parsed, err := semver.NewVersion(ociTagToSemver(candidate))
		if err != nil {
			continue
		}
		if parsed.Prerelease() != "" {
			continue
		}
		entries = append(entries, sortable{original: candidate, parsed: parsed})
	}
	slices.SortStableFunc(entries, func(a, b sortable) int {
		return b.parsed.Compare(a.parsed)
	})
	out := make([]string, 0, len(entries))
	for _, item := range entries {
		out = append(out, item.original)
	}
	return out
}

func ociTagToSemver(tag string) string {
	return strings.Replace(tag, "_", "+", 1)
}

func Newer(current, latest string) bool {
	if latest == "" {
		return false
	}
	if current == "" {
		return false
	}
	currentParsed, currentErr := semver.NewVersion(ociTagToSemver(current))
	latestParsed, latestErr := semver.NewVersion(ociTagToSemver(latest))
	if currentErr != nil || latestErr != nil {
		return false
	}
	return latestParsed.GreaterThan(currentParsed)
}
