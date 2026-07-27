package main

import (
	"context"
	"log"

	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/kube"
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
	return resources.NewManager(ctx, bundle.Dynamic, bundle.Clientset, cats, descs)
}
