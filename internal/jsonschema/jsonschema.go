package jsonschema

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"k8s.io/client-go/openapi"
)

const (
	openapiRefPrefix = "#/components/schemas/"
	bundleRefPrefix  = "#/definitions/"
	//nolint:revive // draft-07 identifies itself over http; validators compare the string verbatim
	draft = "http://json-schema.org/draft-07/schema#"
)

type GVK struct {
	Group   string
	Version string
	Kind    string
}

func (g GVK) String() string {
	if g.Group == "" {
		return g.Version + "/" + g.Kind
	}
	return g.Group + "/" + g.Version + "/" + g.Kind
}

type Source func() openapi.Client

const fetchTimeout = 30 * time.Second

var ErrNoSchema = errors.New("this kind has no schema")

type fetch struct {
	done       chan struct{}
	schemas    map[string]map[string]any
	err        error
	generation uint64
}

type Client struct {
	source     Source
	timeout    time.Duration
	mu         sync.Mutex
	docs       map[string]map[string]map[string]any
	cache      map[GVK]json.RawMessage
	pending    map[string]*fetch
	generation uint64
}

func NewClient(source Source) *Client {
	return &Client{
		source:  source,
		timeout: fetchTimeout,
		docs:    map[string]map[string]map[string]any{},
		cache:   map[GVK]json.RawMessage{},
		pending: map[string]*fetch{},
	}
}

func (c *Client) Refresh() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	c.docs = map[string]map[string]map[string]any{}
	c.cache = map[GVK]json.RawMessage{}
	c.pending = map[string]*fetch{}
}

func (c *Client) cached(gvk GVK) (json.RawMessage, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	raw, ok := c.cache[gvk]
	return raw, ok
}

func (c *Client) keep(gvk GVK, raw json.RawMessage, generation uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if generation != c.generation {
		return
	}
	c.cache[gvk] = raw
}

func (c *Client) version() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.generation
}

func (c *Client) For(ctx context.Context, gvk GVK) (json.RawMessage, error) {
	generation := c.version()
	hit, ok := c.cached(gvk)
	if ok {
		return hit, nil
	}

	bounded, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	schemas, err := c.schemas(bounded, pathFor(gvk))
	if err != nil {
		return nil, err
	}
	root, found := rootName(schemas, gvk)
	if !found {
		return nil, fmt.Errorf("%w: %s", ErrNoSchema, gvk)
	}
	raw, marshalErr := json.Marshal(bundle(schemas, root))
	if marshalErr != nil {
		return nil, fmt.Errorf("marshal schema: %w", marshalErr)
	}
	c.keep(gvk, raw, generation)
	return raw, nil
}

func (c *Client) schemas(ctx context.Context, path string) (map[string]map[string]any, error) {
	leader, waiting := c.claim(path)
	if waiting != nil {
		return await(ctx, waiting)
	}
	schemas, err := c.load(path)
	c.settle(path, leader, schemas, err)
	return schemas, err
}

func (c *Client) claim(path string) (*fetch, *fetch) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cached, ok := c.docs[path]
	if ok {
		return nil, &fetch{done: closedChan(), schemas: cached}
	}
	running, busy := c.pending[path]
	if busy {
		return nil, running
	}
	leader := &fetch{done: make(chan struct{}), generation: c.generation}
	c.pending[path] = leader
	return leader, nil
}

func closedChan() chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}

func (c *Client) settle(path string, leader *fetch, schemas map[string]map[string]any, err error) {
	c.mu.Lock()
	if c.pending[path] == leader {
		delete(c.pending, path)
		if err == nil && leader.generation == c.generation {
			c.docs[path] = schemas
		}
	}
	c.mu.Unlock()

	leader.schemas = schemas
	leader.err = err
	close(leader.done)
}

func await(ctx context.Context, running *fetch) (map[string]map[string]any, error) {
	select {
	case <-running.done:
		return running.schemas, running.err
	case <-ctx.Done():
		return nil, fmt.Errorf("openapi fetch: %w", ctx.Err())
	}
}

func (c *Client) load(path string) (map[string]map[string]any, error) {
	paths, err := c.source().Paths()
	if err != nil {
		return nil, fmt.Errorf("openapi paths: %w", err)
	}
	gv, ok := paths[path]
	if !ok {
		return nil, fmt.Errorf("%w: no openapi document for %s", ErrNoSchema, path)
	}
	raw, schemaErr := gv.Schema("application/json")
	if schemaErr != nil {
		return nil, fmt.Errorf("openapi schema for %s: %w", path, schemaErr)
	}

	var doc struct {
		Components struct {
			Schemas map[string]map[string]any `json:"schemas"`
		} `json:"components"`
	}
	unmarshalErr := json.Unmarshal(raw, &doc)
	if unmarshalErr != nil {
		return nil, fmt.Errorf("parse openapi document for %s: %w", path, unmarshalErr)
	}

	return doc.Components.Schemas, nil
}

func pathFor(gvk GVK) string {
	if gvk.Group == "" {
		return "api/" + gvk.Version
	}
	return "apis/" + gvk.Group + "/" + gvk.Version
}

func rootName(schemas map[string]map[string]any, gvk GVK) (string, bool) {
	match := ""
	for name, schema := range schemas {
		if !declares(schema, gvk) {
			continue
		}
		if strings.HasSuffix(name, "."+gvk.Kind) {
			return name, true
		}
		if match == "" {
			match = name
		}
	}
	return match, match != ""
}

func declares(schema map[string]any, gvk GVK) bool {
	entries, ok := schema["x-kubernetes-group-version-kind"].([]any)
	if !ok {
		return false
	}
	for _, entry := range entries {
		if matches(entry, gvk) {
			return true
		}
	}
	return false
}

func matches(entry any, gvk GVK) bool {
	mapped, ok := entry.(map[string]any)
	if !ok {
		return false
	}
	group, ok := field(mapped, "group")
	if !ok {
		return false
	}
	if group != gvk.Group {
		return false
	}
	version, ok := field(mapped, "version")
	if !ok {
		return false
	}
	if version != gvk.Version {
		return false
	}
	kind, ok := field(mapped, "kind")
	if !ok {
		return false
	}
	return kind == gvk.Kind
}

func field(m map[string]any, key string) (string, bool) {
	v, ok := m[key].(string)
	return v, ok
}

func bundle(schemas map[string]map[string]any, root string) map[string]any {
	definitions := map[string]any{}
	pending := []string{root}
	for len(pending) > 0 {
		name := pending[0]
		pending = pending[1:]
		_, done := definitions[name]
		if done {
			continue
		}
		schema, ok := schemas[name]
		if !ok {
			continue
		}
		refs := []string{}
		definitions[name] = rewrite(schema, &refs)
		pending = append(pending, refs...)
	}
	return map[string]any{
		"$schema":     draft,
		"$ref":        bundleRefPrefix + root,
		"definitions": definitions,
	}
}

func rewrite(node any, refs *[]string) any {
	switch value := node.(type) {
	case map[string]any:
		return rewriteMap(value, refs)
	case []any:
		return rewriteSlice(value, refs)
	default:
		return node
	}
}

func rewriteMap(node map[string]any, refs *[]string) any {
	out := make(map[string]any, len(node))
	for key, value := range node {
		name, isRef := refTarget(key, value)
		if isRef {
			*refs = append(*refs, name)
			out[key] = bundleRefPrefix + name
			continue
		}
		out[key] = rewrite(value, refs)
	}
	return out
}

func rewriteSlice(node []any, refs *[]string) any {
	out := make([]any, 0, len(node))
	for _, item := range node {
		out = append(out, rewrite(item, refs))
	}
	return out
}

func refTarget(key string, value any) (string, bool) {
	if key != "$ref" {
		return "", false
	}
	target, ok := value.(string)
	if !ok {
		return "", false
	}
	if !strings.HasPrefix(target, openapiRefPrefix) {
		return "", false
	}
	return strings.TrimPrefix(target, openapiRefPrefix), true
}
