package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"

	"github.com/sophotechlabs/spinoza/internal/clusterid"
	"github.com/sophotechlabs/spinoza/internal/resources"
	"github.com/sophotechlabs/spinoza/internal/safe"
	"github.com/sophotechlabs/spinoza/internal/server"
)

type Options struct {
	DebugImage       string
	NodeShell        func() bool
	Columns          func() map[string][]api.CustomColumn
	NodeShellImage   string
	NodeShellNS      string
	KubectlBinary    string
	HelmBinary       string
	PromSpec         string
	Kubeconfig       string
	Context          string
	ClientQPS        float32
	ClientBurst      int
	SyncTimeout      time.Duration
	WarmConcurrency  int
	CountBudget      time.Duration
	CountPerType     time.Duration
	CountConcurrency int
	OpenTimeout      time.Duration
}

type connection struct {
	manager *resources.Manager
	ref     api.ContextRef
	host    string
	id      string
	cancel  context.CancelFunc
}

type builder func(ctx context.Context, ref api.ContextRef) (*connection, error)

type reader func(ctx context.Context, ref api.ContextRef, target api.ObjectRef) (string, error)

type lister func(ctx context.Context, ref api.ContextRef, target api.ObjectRef) ([]*unstructured.Unstructured, error)

type Protection interface {
	Verdict(server string) string
	Set(server string, protected bool) error
}

type Kubeconfigs interface {
	List() []api.Kubeconfig
	Add(path string) error
	Remove(path string) error
	Resolve(path string) (string, error)
}

type Cluster struct {
	root       context.Context
	build      builder
	read       reader
	list       lister
	sources    Kubeconfigs
	protection Protection

	openWithin time.Duration

	mu        sync.Mutex
	open      map[string]*connection
	active    string
	startErr  string
	nextSeq   uint64
	installed uint64
}

const defaultOpenTimeout = 30 * time.Second

func newCluster(
	ctx context.Context,
	build builder,
	sources Kubeconfigs,
	protection Protection,
	openWithin time.Duration,
) *Cluster {
	if openWithin <= 0 {
		openWithin = defaultOpenTimeout
	}
	cluster := &Cluster{
		root:       ctx,
		build:      build,
		sources:    sources,
		protection: protection,
		openWithin: openWithin,
		open:       map[string]*connection{},
	}
	err := cluster.use(ctx, api.ContextRef{})
	if err == nil {
		return cluster
	}
	slog.Error("starting without a cluster; pick a context from the app", "error", err)
	cluster.mu.Lock()
	cluster.startErr = err.Error()
	cluster.mu.Unlock()
	return cluster
}

func (c *Cluster) Manager(id string) server.Backend {
	c.mu.Lock()
	defer c.mu.Unlock()
	held := c.held(id)
	if held == nil {
		return nil
	}
	return held.manager
}

func (c *Cluster) held(id string) *connection {
	wanted := id
	if wanted == "" {
		wanted = c.active
	}
	return c.open[clusterid.Normalize(wanted)]
}

func (c *Cluster) unreached() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.startErr
}

func (c *Cluster) Current() api.ContextRef {
	c.mu.Lock()
	defer c.mu.Unlock()
	held := c.held("")
	if held == nil {
		return api.ContextRef{}
	}
	return held.ref
}

func (c *Cluster) Contexts() api.ContextList {
	return api.ContextList{
		Current:     c.Current(),
		Error:       c.unreached(),
		Kubeconfigs: c.sources.List(),
		Protection:  c.protection.Verdict(c.ID()),
	}
}

func (c *Cluster) ID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.active
}

func (c *Cluster) Protect(cluster string, protected bool) error {
	return c.protection.Set(cluster, protected)
}

func (c *Cluster) Protected(cluster string) bool {
	return c.protection.Verdict(cluster) == api.ProtectionProtected
}

func (c *Cluster) AddKubeconfig(path string) error {
	return c.sources.Add(path)
}

func (c *Cluster) RemoveKubeconfig(path string) error {
	resolved, err := c.sources.Resolve(path)
	if err != nil {
		return err
	}
	if c.Current().Kubeconfig == resolved && c.readable(resolved) {
		return fmt.Errorf("spinoza is connected through %s; switch to a context from another kubeconfig first", resolved)
	}
	return c.sources.Remove(resolved)
}

func (c *Cluster) readable(path string) bool {
	for _, entry := range c.sources.List() {
		if entry.Path == path {
			return entry.Error == ""
		}
	}
	return false
}

func (c *Cluster) useReader(read reader) {
	c.read = read
}

func (c *Cluster) useLister(list lister) {
	c.list = list
}

func (c *Cluster) List(
	ctx context.Context,
	ref api.ContextRef,
	target api.ObjectRef,
) ([]*unstructured.Unstructured, error) {
	if c.list == nil {
		return nil, fmt.Errorf("%w: listing another context is not wired up", api.ErrInternal)
	}
	return c.list(ctx, ref, target)
}

// Read opens a client for that read alone, leaving current caches untouched.
func (c *Cluster) Read(ctx context.Context, ref api.ContextRef, target api.ObjectRef) (string, error) {
	if c.read == nil {
		return "", fmt.Errorf("%w: reading another context is not wired up", api.ErrInternal)
	}
	return c.read(ctx, ref, target)
}

func (c *Cluster) Use(ref api.ContextRef) error {
	return c.use(c.root, ref)
}

func (c *Cluster) use(root context.Context, ref api.ContextRef) error {
	seq := c.claim()
	opened, err := c.dial(root, ref)
	if err != nil {
		return err
	}

	c.mu.Lock()
	if seq < c.installed {
		c.mu.Unlock()
		opened.cancel()
		return nil
	}
	previous := c.retire(opened.id)
	c.installed = seq
	c.open[opened.id] = opened
	c.active = opened.id
	c.startErr = ""
	c.mu.Unlock()

	for _, gone := range previous {
		gone.cancel()
	}
	return nil
}

func (c *Cluster) retire(keep string) []*connection {
	gone := make([]*connection, 0, len(c.open))
	for id, held := range c.open {
		if id == keep {
			continue
		}
		gone = append(gone, held)
		delete(c.open, id)
	}
	return gone
}

func (c *Cluster) Open(ref api.ContextRef) (string, error) {
	opened, err := c.dial(c.root, ref)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	already, held := c.open[opened.id]
	if held {
		c.active = already.id
		opened.cancel()
		return already.id, nil
	}
	c.open[opened.id] = opened
	c.active = opened.id
	c.startErr = ""
	return opened.id, nil
}

func (c *Cluster) Close(id string) error {
	key := clusterid.Normalize(id)
	c.mu.Lock()
	held := c.open[key]
	if held == nil {
		c.mu.Unlock()
		return fmt.Errorf("%w: no cluster %s is open", api.ErrNotOpen, key)
	}
	delete(c.open, key)
	if c.active == key {
		c.active = c.firstOpen()
	}
	c.mu.Unlock()
	held.cancel()
	return nil
}

func (c *Cluster) firstOpen() string {
	ids := make([]string, 0, len(c.open))
	for id := range c.open {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return ""
	}
	slices.Sort(ids)
	return ids[0]
}

func (c *Cluster) Activate(id string) error {
	key := clusterid.Normalize(id)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.open[key] == nil {
		return fmt.Errorf("%w: no cluster %s is open", api.ErrNotOpen, key)
	}
	c.active = key
	return nil
}

func (c *Cluster) Opened() []api.OpenCluster {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]api.OpenCluster, 0, len(c.open))
	for id, held := range c.open {
		out = append(out, api.OpenCluster{
			ID:         id,
			Context:    held.ref.Name,
			Kubeconfig: held.ref.Kubeconfig,
			Active:     id == c.active,
			Protection: c.protection.Verdict(id),
		})
	}
	slices.SortFunc(out, func(left, right api.OpenCluster) int {
		return strings.Compare(left.ID, right.ID)
	})
	return out
}

// dial builds a connection, bounded, so an apiserver that never answers is an
// error rather than a handler that never returns.
func (c *Cluster) dial(root context.Context, ref api.ContextRef) (*connection, error) {
	ctx, cancel := context.WithCancel(root)
	type built struct {
		conn *connection
		err  error
	}
	done := make(chan built, 1)
	safe.Go("opening "+ref.Name, func() {
		conn, err := c.build(ctx, ref)
		done <- built{conn: conn, err: err}
	})
	timer := time.NewTimer(c.openWithin)
	defer timer.Stop()
	select {
	case answer := <-done:
		if answer.err != nil {
			cancel()
			return nil, answer.err
		}
		answer.conn.id = clusterid.Normalize(answer.conn.host)
		answer.conn.cancel = cancel
		return answer.conn, nil
	case <-timer.C:
		cancel()
		return nil, fmt.Errorf("context %q did not answer within %s", ref.Name, c.openWithin)
	}
}

func (c *Cluster) claim() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextSeq++
	return c.nextSeq
}
