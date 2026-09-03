package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/auth"
)

func TestRuleValidationFailsPromptlyWhenTheIdentityBudgetIsFull(t *testing.T) {
	srv := New(nil, testAssets(), testToken)
	srv.ruleCompiles = newWorkBudget(2, 1)
	release, claimed := srv.ruleCompiles.claim("alice", 1)
	if !claimed {
		t.Fatal("the setup rule validation was refused")
	}
	defer release()
	req := httptest.NewRequest(http.MethodPost, "/api/checks/rules/faults", strings.NewReader(`[]`))
	req = req.WithContext(auth.WithIdentity(req.Context(), auth.Identity{User: "alice"}))
	recorded := httptest.NewRecorder()
	srv.checkRules(recorded, req)
	if recorded.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", recorded.Code, http.StatusTooManyRequests)
	}
	var failure api.Failure
	bodyOf(t, recorded.Result(), &failure)
	if failure.Message != "rule validation is busy; try again" {
		t.Fatalf("message = %q", failure.Message)
	}
}

func TestRuleValidationReusesItsReleasedBudget(t *testing.T) {
	srv := New(nil, testAssets(), testToken)
	srv.ruleCompiles = newWorkBudget(1, 1)
	release, claimed := srv.ruleCompiles.claim("alice", 1)
	if !claimed {
		t.Fatal("the setup rule validation was refused")
	}
	release()
	req := httptest.NewRequest(http.MethodPost, "/api/checks/rules/faults", strings.NewReader(`[]`))
	req = req.WithContext(auth.WithIdentity(req.Context(), auth.Identity{User: "alice"}))
	recorded := httptest.NewRecorder()
	srv.checkRules(recorded, req)
	if recorded.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorded.Code, http.StatusOK)
	}
}
