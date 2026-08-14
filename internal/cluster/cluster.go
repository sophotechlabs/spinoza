package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"

	"github.com/sophotechlabs/spinoza/internal/resources"
	"github.com/sophotechlabs/spinoza/internal/server"
)

type Options struct {
	DebugImage       string
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

type builder func(ctx context.Context, ref api.ContextRef) (*resources.Manager, api.ContextRef, error)

type Kubeconfigs interface {
	List() []api.Kubeconfig
	Add(path string) error
	Remove(path string) error
	Resolve(path string) (string, error)
}

type Cluster struct {
	root    context.Context
	build   builder
	sources Kubeconfigs

	mu        sync.Mutex
	manager   *resources.Manager
	cancel    context.CancelFunc
	current   api.ContextRef
	startErr  string
	nextSeq   uint64
	installed uint64
}

func newCluster(ctx context.Context, build builder, sources Kubeconfigs) *Cluster {
	cluster := &Cluster{root: ctx, build: build, sources: sources}
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
	}
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

func (c *Cluster) Use(ref api.ContextRef) error {
	return c.use(c.root, ref)
}

func (c *Cluster) use(root context.Context, ref api.ContextRef) error {
	seq := c.claim()
	ctx, cancel := context.WithCancel(root)
	manager, current, err := c.build(ctx, ref)
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
	c.manager = manager
	c.cancel = cancel
	c.current = current
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
