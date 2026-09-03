package auth

import (
	"errors"
	"testing"
	"time"
)

func TestConsumedFlowRegistryIsBoundedSingleUseAndExpiring(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	flows := newConsumedFlows(2, func() time.Time { return now })
	expires := now.Add(time.Minute)
	if err := flows.consume("one", expires); err != nil {
		t.Fatalf("consume first flow: %v", err)
	}
	if err := flows.consume("one", expires); !errors.Is(err, errFlowReplay) {
		t.Fatalf("replay error = %v", err)
	}
	if err := flows.consume("two", expires); err != nil {
		t.Fatalf("consume second flow: %v", err)
	}
	if err := flows.consume("three", expires); !errors.Is(err, errFlowRegistryFull) {
		t.Fatalf("capacity error = %v", err)
	}
	now = expires
	if err := flows.consume("three", now.Add(time.Minute)); err != nil {
		t.Fatalf("consume after expiry: %v", err)
	}
}

func TestTokenExchangeBudgetBoundsGlobalAndSourceConcurrency(t *testing.T) {
	budget := newExchangeBudget(2, 1)
	releaseA, claimed := budget.claim("a")
	if !claimed {
		t.Fatal("source a was refused below the limits")
	}
	if _, secondClaimed := budget.claim("a"); secondClaimed {
		t.Fatal("source a exceeded its limit")
	}
	releaseB, claimed := budget.claim("b")
	if !claimed {
		t.Fatal("source b was refused below the global limit")
	}
	if _, thirdClaimed := budget.claim("c"); thirdClaimed {
		t.Fatal("the global exchange limit was exceeded")
	}
	releaseA()
	releaseB()
	releaseC, claimed := budget.claim("c")
	if !claimed {
		t.Fatal("released exchange capacity was not reusable")
	}
	releaseC()
}
