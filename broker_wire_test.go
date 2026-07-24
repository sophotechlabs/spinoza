package main

import (
	"context"
	"testing"
)

func TestMakeBrokerFakeReturnsStub(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := makeBroker(ctx, true)
	if b == nil {
		t.Fatal("makeBroker(fake=true) = nil, want stub broker")
	}
	rows, _ := b.Snapshot()
	if len(rows) != 5 {
		t.Fatalf("stub snapshot rows = %d, want 5", len(rows))
	}
}
