package main

import (
	"context"
	"log"

	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/exec"
	"github.com/sophotechlabs/spinoza/internal/jsonschema"
	"github.com/sophotechlabs/spinoza/internal/kube"
	"github.com/sophotechlabs/spinoza/internal/portforward"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

func makeManager(ctx context.Context) *resources.Manager {
	bundle, err := kube.Load()
	if err != nil {
		log.Fatalf("kube: %v", err)
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
	return resources.NewManager(ctx, bundle.Dynamic, bundle.Clientset, schemas, forwards, shells, cats, descs)
}
