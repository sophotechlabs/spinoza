package cluster

import (
	"context"
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

type builder func(ctx context.Context, name string) (*resources.Manager, string, error)

type lister func() ([]string, string, error)

type Cluster struct {
	root  context.Context
	build builder
	list  lister

	mu        sync.Mutex
	manager   *resources.Manager
	cancel    context.CancelFunc
	current   string
	startErr  string
	nextSeq   uint64
	installed uint64
}

func newCluster(ctx context.Context, build builder, list lister) *Cluster {
	cluster := &Cluster{root: ctx, build: build, list: list}
	err := cluster.use(ctx, "")
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

func (c *Cluster) Current() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.current
}

func (c *Cluster) Contexts() api.ContextList {
	names, current, err := c.list()
	list := api.ContextList{Contexts: names, Current: c.Current()}
	if err != nil {
		list.Error = err.Error()
		return list
	}
	if list.Current == "" {
		list.Current = current
	}
	list.Error = c.unreached()
	return list
}

func (c *Cluster) Use(name string) error {
	return c.use(c.root, name)
}

func (c *Cluster) use(root context.Context, name string) error {
	seq := c.claim()
	ctx, cancel := context.WithCancel(root)
	manager, current, err := c.build(ctx, name)
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
