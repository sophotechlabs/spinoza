package main

import (
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/mcp"
	"github.com/sophotechlabs/spinoza/internal/version"
)

type clusterFacts interface {
	Current() api.ContextRef
	ID() string
	Protected(cluster string) bool
}

func optionsFor(clusters clusterFacts, opts mcp.Settings, prom mcp.Prometheus) mcp.Options {
	return mcp.Options{
		Version: version.String(),
		Context: clusters.Current().Name,
		Protected: func() bool {
			return clusters.Protected(clusters.ID())
		},
		AllowWrite:      opts.AllowWrite,
		UnsafeRawOutput: opts.UnsafeRawOutput,
		Prometheus:      prom,
		LogLines:        opts.LogLines,
		CallBudget:      opts.CallBudget,
	}
}
