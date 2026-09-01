package charts

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"golang.org/x/net/publicsuffix"
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

type Chart struct {
	Name        string
	Description string
	Version     string
}

type key struct {
	repo  Repo
	chart string
}

type listing struct {
	versions map[string][]string
	charts   []Chart
}

type lookupIPs func(context.Context, string, string) ([]netip.Addr, error)

type dialContext func(context.Context, string, string) (net.Conn, error)

type Cache struct {
	ctx    context.Context
	client *http.Client
	ttl    time.Duration
	now    func() time.Time
	wg     sync.WaitGroup

	mu       sync.Mutex
	lists    map[key][]string
	catalog  map[Repo][]Chart
	fetched  map[key]time.Time
	inflight map[key]bool
}

func New(ctx context.Context, client *http.Client, ttl time.Duration) *Cache {
	var dialer net.Dialer
	return newCache(ctx, client, ttl, net.DefaultResolver.LookupNetIP, dialer.DialContext)
}

func newCache(ctx context.Context, client *http.Client, ttl time.Duration, lookup lookupIPs, dial dialContext) *Cache {
	return &Cache{
		ctx:      ctx,
		client:   publicOnly(client, lookup, dial),
		ttl:      ttl,
		now:      time.Now,
		lists:    map[key][]string{},
		catalog:  map[Repo][]Chart{},
		fetched:  map[key]time.Time{},
		inflight: map[key]bool{},
	}
}

func publicOnly(client *http.Client, lookup lookupIPs, dial dialContext) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	guarded := *client
	transport := defaultTransport()
	if configured, ok := client.Transport.(*http.Transport); ok {
		transport = configured.Clone()
	}
	transport.Proxy = nil
	transport.DialContext = publicDial(lookup, dial)
	transport.DialTLSContext = publicTLSDial(transport, lookup, dial)
	guarded.Transport = transport
	guarded.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return errors.New("stopped after 10 redirects")
		}
		return CheckRepoURL(req.URL.String())
	}
	return &guarded
}

func defaultTransport() *http.Transport {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		panic("http default transport is not configurable")
	}
	return transport.Clone()
}

func publicDial(lookup lookupIPs, dial dialContext) dialContext {
	return func(ctx context.Context, network, authority string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(authority)
		if err != nil {
			return nil, fmt.Errorf("repository address %q: %w", authority, err)
		}
		addresses, err := resolvePublic(ctx, lookup, host)
		if err != nil {
			return nil, err
		}
		failures := make([]error, 0, len(addresses))
		for _, address := range addresses {
			endpoint := net.JoinHostPort(address.String(), port)
			conn, dialErr := dial(ctx, network, endpoint)
			if dialErr == nil {
				return conn, nil
			}
			failures = append(failures, dialErr)
		}
		return nil, errors.Join(failures...)
	}
}

func publicTLSDial(transport *http.Transport, lookup lookupIPs, dial dialContext) dialContext {
	guarded := publicDial(lookup, dial)
	return func(ctx context.Context, network, authority string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(authority)
		if err != nil {
			return nil, fmt.Errorf("repository address %q: %w", authority, err)
		}
		plain, err := guarded(ctx, network, authority)
		if err != nil {
			return nil, err
		}
		config := tlsConfigFor(transport, host)
		secured := tls.Client(plain, config)
		if err := secured.HandshakeContext(ctx); err != nil {
			_ = plain.Close()
			return nil, err
		}
		return secured, nil
	}
}

func tlsConfigFor(transport *http.Transport, host string) *tls.Config {
	config := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	if transport.TLSClientConfig == nil {
		return config
	}
	config = transport.TLSClientConfig.Clone()
	if config.ServerName == "" {
		config.ServerName = host
	}
	if config.MinVersion < tls.VersionTLS12 {
		config.MinVersion = tls.VersionTLS12
	}
	return config
}

func resolvePublic(ctx context.Context, lookup lookupIPs, host string) ([]netip.Addr, error) {
	if localName(host) {
		return nil, fmt.Errorf("repository host %q is this machine", host)
	}
	address, literal := literalAddress(host)
	addresses := []netip.Addr{address}
	if !literal {
		var err error
		addresses, err = lookup(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("resolve repository host %q: %w", host, err)
		}
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("repository host %q resolved to no addresses", host)
	}
	for i := range addresses {
		addresses[i] = addresses[i].Unmap()
		if !routableAddress(addresses[i]) {
			return nil, fmt.Errorf("repository host %q resolved to non-public address %s", host, addresses[i])
		}
	}
	return addresses, nil
}

func literalAddress(host string) (netip.Addr, bool) {
	address, err := netip.ParseAddr(host)
	return address, err == nil
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
	if parsed.User != nil {
		return nil, errors.New("repository url must not contain user information")
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
	ip, literal := literalAddress(host)
	if !literal {
		return nil
	}
	if !routableAddress(ip.Unmap()) {
		return fmt.Errorf("repository host %q is not a public address", host)
	}
	return nil
}

func localName(host string) bool {
	lowered := strings.ToLower(strings.TrimSuffix(host, "."))
	if lowered == "localhost" {
		return true
	}
	return strings.HasSuffix(lowered, ".localhost")
}

func routableAddress(ip netip.Addr) bool {
	if !ip.IsValid() || ip.Zone() != "" || !ip.IsGlobalUnicast() {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() {
		return false
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return false
	}
	for _, blocked := range nonPublicNetworks {
		if blocked.Contains(ip) {
			return false
		}
	}
	return true
}

var nonPublicNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:10::/28"),
	netip.MustParsePrefix("2001:20::/28"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
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
	err := c.ensure(ctx, repo, chart)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.lists[key{repo: repo, chart: chart}]), nil
}

func (c *Cache) Search(ctx context.Context, repo Repo, query string, limit int) ([]Chart, error) {
	if repo.OCI {
		return nil, fmt.Errorf("%q is an oci registry, which cannot be listed; name the chart instead", repo.URL)
	}
	err := c.ensure(ctx, repo, "")
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	known := slices.Clone(c.catalog[repo])
	c.mu.Unlock()
	return matching(known, query, limit), nil
}

func matching(known []Chart, query string, limit int) []Chart {
	needle := strings.ToLower(strings.TrimSpace(query))
	hits := []Chart{}
	for _, chart := range known {
		if rank(chart, needle) < 0 {
			continue
		}
		hits = append(hits, chart)
	}
	slices.SortStableFunc(hits, func(a, b Chart) int {
		byRank := rank(a, needle) - rank(b, needle)
		if byRank != 0 {
			return byRank
		}
		return strings.Compare(a.Name, b.Name)
	})
	if limit > 0 && len(hits) > limit {
		return hits[:limit]
	}
	return hits
}

func rank(chart Chart, needle string) int {
	if needle == "" {
		return 1
	}
	name := strings.ToLower(chart.Name)
	if strings.HasPrefix(name, needle) {
		return 0
	}
	if strings.Contains(name, needle) {
		return 1
	}
	if strings.Contains(strings.ToLower(chart.Description), needle) {
		return 2
	}
	return -1
}

func (c *Cache) ensure(ctx context.Context, repo Repo, chart string) error {
	unit := fetchUnit(repo, chart)

	c.mu.Lock()
	last, seen := c.fetched[unit]
	fresh := seen && c.now().Sub(last) < c.ttl
	c.mu.Unlock()
	if fresh {
		return nil
	}

	found, err := c.resolve(ctx, repo, chart)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.fetched[unit] = c.now()
	c.keep(repo, found)
	return nil
}

func (c *Cache) keep(repo Repo, found listing) {
	for name, list := range found.versions {
		c.lists[key{repo: repo, chart: name}] = list
	}
	if found.charts != nil {
		c.catalog[repo] = found.charts
	}
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

	found, err := c.resolve(ctx, repo, chart)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.inflight[unit] = false
	if err != nil {
		return
	}
	c.fetched[unit] = c.now()
	c.keep(repo, found)
}

func (c *Cache) Resolve(ctx context.Context, repo Repo, chart string) (map[string][]string, error) {
	found, err := c.resolve(ctx, repo, chart)
	if err != nil {
		return nil, err
	}
	return found.versions, nil
}

func (c *Cache) Seed(repo Repo, body io.Reader, fetched time.Time) error {
	found, err := parseBoundedIndex(body, maxBodyBytes)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fetched[fetchUnit(repo, "")] = fetched
	c.keep(repo, found)
	return nil
}

func (c *Cache) resolve(ctx context.Context, repo Repo, chart string) (listing, error) {
	parsed, err := fetchableURL(repo.URL)
	if err != nil {
		return listing{}, err
	}
	if err := checkHost(parsed.Hostname()); err != nil {
		return listing{}, err
	}
	if (parsed.Scheme == "oci") != repo.OCI {
		return listing{}, fmt.Errorf("repository url %q does not match its oci setting", repo.URL)
	}
	if repo.OCI {
		return c.resolveOCI(ctx, repo, chart)
	}
	return c.resolveIndex(ctx, repo)
}

func (c *Cache) resolveIndex(ctx context.Context, repo Repo) (listing, error) {
	endpoint := strings.TrimSuffix(repo.URL, "/") + "/" + indexFilename
	body, err := c.get(ctx, endpoint, "")
	if err != nil {
		return listing{}, err
	}
	defer func() { _ = body.Close() }()

	found, decodeErr := parseBoundedIndex(body, maxBodyBytes)
	if decodeErr != nil {
		return listing{}, fmt.Errorf("parse %s: %w", endpoint, decodeErr)
	}
	return found, nil
}

func parseIndex(body io.Reader) (listing, error) {
	var doc struct {
		Entries map[string][]struct {
			Version     string `yaml:"version"`
			Description string `yaml:"description"`
		} `yaml:"entries"`
	}
	decodeErr := yaml.NewDecoder(body).Decode(&doc)
	if decodeErr != nil {
		return listing{}, decodeErr
	}

	out := listing{versions: map[string][]string{}, charts: []Chart{}}
	for name, releases := range doc.Entries {
		described := map[string]string{}
		raw := make([]string, 0, len(releases))
		for _, release := range releases {
			raw = append(raw, release.Version)
			described[release.Version] = release.Description
		}
		sorted := sortVersions(raw)
		if len(sorted) == 0 {
			continue
		}
		out.versions[name] = sorted
		out.charts = append(out.charts, Chart{
			Name:        name,
			Description: described[sorted[0]],
			Version:     sorted[0],
		})
	}
	slices.SortFunc(out.charts, func(a, b Chart) int {
		return strings.Compare(a.Name, b.Name)
	})
	return out, nil
}

func parseBoundedIndex(body io.Reader, limit int) (listing, error) {
	raw, err := readBounded(body, limit)
	if err != nil {
		return listing{}, err
	}
	return parseIndex(strings.NewReader(string(raw)))
}

func readBounded(body io.Reader, limit int) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(body, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > limit {
		return nil, fmt.Errorf("response body is larger than %d bytes", limit)
	}
	return raw, nil
}

func decodeBoundedJSON(body io.Reader, limit int, into any) error {
	raw, err := readBounded(body, limit)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, into)
}

func (c *Cache) resolveOCI(ctx context.Context, repo Repo, chart string) (listing, error) {
	host, path, err := splitOCI(repo.URL)
	if err != nil {
		return listing{}, err
	}
	endpoint := fmt.Sprintf("https://%s/v2/%s/%s/tags/list", host, path, url.PathEscape(chart))

	body, err := c.get(ctx, endpoint, "")
	if err != nil {
		return listing{}, err
	}
	defer func() { _ = body.Close() }()

	var doc struct {
		Tags []string `json:"tags"`
	}
	decodeErr := decodeBoundedJSON(body, maxBodyBytes, &doc)
	if decodeErr != nil {
		return listing{}, fmt.Errorf("parse tags for %s: %w", endpoint, decodeErr)
	}

	sorted := sortVersions(doc.Tags)
	if len(sorted) == 0 {
		return listing{versions: map[string][]string{}}, nil
	}
	return listing{versions: map[string][]string{chart: sorted}}, nil
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
	decodeErr := decodeBoundedJSON(resp.Body, maxBodyBytes, &doc)
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
	parent, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return host
	}
	return parent
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
