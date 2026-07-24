package main

import (
	"context"
	"log"

	"github.com/sophotechlabs/spinoza/internal/broker"
	"github.com/sophotechlabs/spinoza/internal/kube"
)

func makeBroker(ctx context.Context, fake bool) broker.Broker {
	if fake {
		return broker.NewStub(ctx)
	}
	cs, contextName, namespace, err := kube.Load()
	if err != nil {
		log.Fatalf("kube: %v", err)
	}
	b, err := broker.NewInformer(ctx, cs)
	if err != nil {
		log.Fatalf("informer: %v", err)
	}
	log.Printf("spinoza connected to context %q (namespace %q)", contextName, namespace)
	return b
}
