package cluster

import (
	"context"
	"sync"

	"github.com/sophotechlabs/spinoza/internal/api"

	"github.com/sophotechlabs/spinoza/internal/resources"
)

type Options struct {
	DebugImage    string
	KubectlBinary string
	PromSpec      string
}

type builder func(ctx context.Context, name string) (*resources.Manager, string, error)

type lister func() ([]string, string, error)

type Cluster struct {
	root  context.Context
	build builder
	list  lister

	mu      sync.Mutex
	manager *resources.Manager
	cancel  context.CancelFunc
	current string
}

func newCluster(ctx context.Context, build builder, list lister) (*Cluster, error) {
	cluster := &Cluster{root: ctx, build: build, list: list}
	err := cluster.use(ctx, "")
	if err != nil {
		return nil, err
	}
	return cluster, nil
}

func (c *Cluster) Manager() *resources.Manager {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.manager
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
	return list
}

func (c *Cluster) Use(name string) error {
	return c.use(c.root, name)
}

func (c *Cluster) use(root context.Context, name string) error {
	ctx, cancel := context.WithCancel(root)
	manager, current, err := c.build(ctx, name)
	if err != nil {
		cancel()
		return err
	}

	c.mu.Lock()
	previous := c.cancel
	c.manager = manager
	c.cancel = cancel
	c.current = current
	c.mu.Unlock()

	if previous != nil {
		previous()
	}
	return nil
}
