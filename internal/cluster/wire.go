package cluster

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sophotechlabs/spinoza/internal/debugcontainer"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/exec"
	"github.com/sophotechlabs/spinoza/internal/jsonschema"
	"github.com/sophotechlabs/spinoza/internal/kube"
	"github.com/sophotechlabs/spinoza/internal/portforward"
	"github.com/sophotechlabs/spinoza/internal/prom"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

func New(ctx context.Context, options Options) (*Cluster, error) {
	promTarget, targetErr := prom.ParseTarget(options.PromSpec)
	if targetErr != nil {
		return nil, targetErr
	}
	return newCluster(ctx, func(buildCtx context.Context, name string) (*resources.Manager, string, error) {
		manager, bundle, err := build(buildCtx, name, options, promTarget)
		if err != nil {
			return nil, "", err
		}
		return manager, bundle.Context, nil
	}, func() ([]string, string, error) {
		return kube.Contexts(options.Kubeconfig)
	}), nil
}

func unreachable(name string, discErr error) error {
	if discErr != nil {
		return fmt.Errorf("context %q lists no resource types: %w", name, discErr)
	}
	return fmt.Errorf("context %q lists no resource types", name)
}

func build(ctx context.Context, name string, options Options, promTarget prom.Target) (*resources.Manager, *kube.Bundle, error) {
	bundle, err := kube.LoadContext(name, kube.Options{
		Kubeconfig: options.Kubeconfig,
		QPS:        options.ClientQPS,
		Burst:      options.ClientBurst,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("kube: %w", err)
	}
	cats, descs, discErr := discovery.List(bundle.Discovery)
	if len(descs) == 0 {
		return nil, nil, unreachable(bundle.Context, discErr)
	}
	if discErr != nil {
		slog.Warn("discovery came back incomplete", "error", discErr)
	}
	slog.Info("connected to a cluster", "context", bundle.Context, "resourceTypes", len(descs), "categories", len(cats))
	schemas := jsonschema.NewClient(bundle.Discovery.OpenAPIV3)
	forwards := portforward.NewRegistry(
		ctx,
		portforward.NewRunner(bundle.Clientset, bundle.Config),
		portforward.NewResolver(bundle.Clientset),
		portforward.NewProber(bundle.Clientset),
	)
	shells := exec.NewService(
		exec.NewStreamer(bundle.Clientset, bundle.Config),
		exec.NewImages(bundle.Clientset),
	)
	debugger := debugcontainer.NewService(
		debugcontainer.NewRunner(options.KubectlBinary),
		bundle.Clientset,
		options.DebugImage,
		bundle.Context,
	)
	promClient := prom.NewClient(bundle.Clientset, promTarget)
	mgr := resources.NewManager(ctx, resources.Deps{
		Limits: resources.Limits{
			SyncTimeout:     options.SyncTimeout,
			WarmConcurrency: options.WarmConcurrency,
			Counts: resources.CountLimits{
				Budget:      options.CountBudget,
				PerType:     options.CountPerType,
				Concurrency: options.CountConcurrency,
			},
		},
		Dynamic:     bundle.Dynamic,
		Clientset:   bundle.Clientset,
		Schemas:     schemas,
		Forwards:    forwards,
		Shells:      shells,
		Debugger:    debugger,
		Prometheus:  promClient,
		Categories:  cats,
		Descriptors: descs,
	})
	mgr.UseDiscovery(bundle.Discovery, discErr)
	return mgr, bundle, nil
}
