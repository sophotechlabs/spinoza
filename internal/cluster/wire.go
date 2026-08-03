package cluster

import (
	"context"
	"fmt"
	"log"

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
	return newCluster(ctx, func(buildCtx context.Context, name string) (*resources.Manager, string, error) {
		manager, bundle, err := build(buildCtx, name, options)
		if err != nil {
			return nil, "", err
		}
		return manager, bundle.Context, nil
	}, kube.Contexts)
}

func build(ctx context.Context, name string, options Options) (*resources.Manager, *kube.Bundle, error) {
	bundle, err := kube.LoadContext(name)
	if err != nil {
		return nil, nil, fmt.Errorf("kube: %w", err)
	}
	cats, descs, discErr := discovery.List(bundle.Discovery)
	if discErr != nil {
		log.Printf("discovery (partial): %v", discErr)
	}
	log.Printf("spinoza connected to context %q — %d resource types, %d categories", bundle.Context, len(descs), len(cats))
	schemas := jsonschema.NewClient(bundle.Discovery.OpenAPIV3())
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
	promTarget, targetErr := prom.ParseTarget(options.PromSpec)
	if targetErr != nil {
		return nil, nil, targetErr
	}
	promClient := prom.NewClient(bundle.Clientset, promTarget)
	mgr := resources.NewManager(ctx, bundle.Dynamic, bundle.Clientset, schemas, forwards, shells, debugger, promClient, cats, descs)
	mgr.UseDiscovery(bundle.Discovery, discErr)
	return mgr, bundle, nil
}
