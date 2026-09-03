package auth

import (
	"fmt"
	"testing"
	"time"
)

func TestLogoutVerificationBudgetEnforcesConcurrencyAndReleases(t *testing.T) {
	budget := newLogoutVerificationBudget(time.Now)
	budget.globalConcurrent = 2
	budget.sourceConcurrent = 1
	releaseAlice, ok := budget.claim("alice")
	if !ok {
		t.Fatal("alice was refused")
	}
	if _, claimedAgain := budget.claim("alice"); claimedAgain {
		t.Fatal("alice exceeded the source concurrency")
	}
	releaseBob, ok := budget.claim("bob")
	if !ok {
		t.Fatal("bob was refused")
	}
	if _, carolClaimed := budget.claim("carol"); carolClaimed {
		t.Fatal("global verification concurrency was exceeded")
	}
	releaseAlice()
	releaseAlice()
	if release, ok := budget.claim("alice"); !ok {
		t.Fatal("released source capacity was not reusable")
	} else {
		release()
	}
	releaseBob()
}

func TestLogoutVerificationBudgetEnforcesRateBoundaries(t *testing.T) {
	now := time.Date(2026, 9, 3, 18, 0, 0, 0, time.UTC)
	budget := newLogoutVerificationBudget(func() time.Time { return now })
	budget.globalRate = 3
	budget.sourceRate = 2

	for range 2 {
		release, ok := budget.claim("alice")
		if !ok {
			t.Fatal("alice was refused below the source rate")
		}
		release()
	}
	if _, ok := budget.claim("alice"); ok {
		t.Fatal("alice exceeded the source rate")
	}
	release, ok := budget.claim("bob")
	if !ok {
		t.Fatal("bob was refused at the global rate boundary")
	}
	release()
	if _, ok := budget.claim("carol"); ok {
		t.Fatal("global verification rate was exceeded")
	}
	now = now.Add(logoutRateWindow)
	if release, ok := budget.claim("alice"); !ok {
		t.Fatal("rate capacity did not expire at the window boundary")
	} else {
		release()
	}
}

func TestLogoutVerificationCooldownAndSourceCacheAreBounded(t *testing.T) {
	now := time.Date(2026, 9, 3, 18, 0, 0, 0, time.UTC)
	budget := newLogoutVerificationBudget(func() time.Time { return now })
	budget.sourceCapacity = 2
	release, ok := budget.claim("first")
	if !ok {
		t.Fatal("first source was refused")
	}
	release()
	now = now.Add(time.Second)
	for _, source := range []string{"second", "third"} {
		release, ok = budget.claim(source)
		if !ok {
			t.Fatalf("%s source was refused", source)
		}
		release()
		now = now.Add(time.Second)
	}
	if len(budget.sources) != 2 {
		t.Fatalf("source cache size = %d, want 2", len(budget.sources))
	}
	if _, ok := budget.sources["first"]; ok {
		t.Fatal("the oldest source survived capacity eviction")
	}
	budget.failed()
	if _, ok := budget.claim("next"); ok {
		t.Fatal("verification was admitted inside the failure cooldown")
	}
	now = now.Add(logoutFailureCooldown)
	if release, ok := budget.claim(fmt.Sprintf("next-%d", now.Unix())); !ok {
		t.Fatal("verification was refused at the cooldown boundary")
	} else {
		release()
	}
}
