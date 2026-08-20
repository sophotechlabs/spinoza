package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"

	"github.com/sophotechlabs/spinoza/internal/resources"
	"github.com/sophotechlabs/spinoza/internal/server"
)

type Options struct {
	DebugImage       string
	NodeShell        func() bool
	NodeShellImage   string
	NodeShellNS      string
	KubectlBinary    string
	HelmBinary       string
	PromSpec         string
	Kubeconfig       string
	ClientQPS        float32
	ClientBurst      int
	SyncTimeout      time.Duration
	WarmConcurrency  int
	CountBudget      time.Duration
	CountPerType     time.Duration
	CountConcurrency int
}

type connection struct {
	manager *resources.Manager
	ref     api.ContextRef
	host    string
}

type builder func(ctx context.Context, ref api.ContextRef) (*connection, error)

// reader fetches one object from a context that is not the current one, without
// starting informers or anything else a full connection carries.
type reader func(ctx context.Context, ref api.ContextRef, target api.ObjectRef) (string, error)

// lister does the same for every object of one kind, which is what a drift report
// across two clusters is made of.
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

	mu        sync.Mutex
	manager   *resources.Manager
	cancel    context.CancelFunc
	current   api.ContextRef
	host      string
	startErr  string
	nextSeq   uint64
	installed uint64
}

func newCluster(ctx context.Context, build builder, sources Kubeconfigs, protection Protection) *Cluster {
	cluster := &Cluster{root: ctx, build: build, sources: sources, protection: protection}
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

func (c *Cluster) Manager() server.Backend {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.manager == nil {
		return nil
	}
	return c.manager
}

func (c *Cluster) unreached() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.startErr
}

func (c *Cluster) Current() api.ContextRef {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.current
}

func (c *Cluster) Contexts() api.ContextList {
	return api.ContextList{
		Current:     c.Current(),
		Error:       c.unreached(),
		Kubeconfigs: c.sources.List(),
		Protection:  c.protection.Verdict(c.server()),
	}
}

func (c *Cluster) server() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.host
}

func (c *Cluster) Protect(protected bool) error {
	return c.protection.Set(c.server(), protected)
}

func (c *Cluster) Protected() bool {
	return c.protection.Verdict(c.server()) == api.ProtectionProtected
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

// List reads every object of one kind from another context, for the same reason
// Read exists: the current connection and its caches stay untouched.
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

// Read renders one object from another context as yaml. It opens a client for that
// read alone, so the current connection and its caches are untouched.
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
	ctx, cancel := context.WithCancel(root)
	opened, err := c.build(ctx, ref)
	if err != nil {
		cancel()
		return err
	}

	c.mu.Lock()
	if seq < c.installed {
		c.mu.Unlock()
		cancel()
		return nil
	}
	previous := c.cancel
	c.installed = seq
	c.manager = opened.manager
	c.cancel = cancel
	c.current = opened.ref
	c.host = opened.host
	c.startErr = ""
	c.mu.Unlock()

	if previous != nil {
		previous()
	}
	return nil
}

func (c *Cluster) claim() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextSeq++
	return c.nextSeq
}
