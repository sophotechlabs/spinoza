package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/actions"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/checks"
)

func TestInvalidUnmuteRequestsLeaveTheStoredMuteAlone(t *testing.T) {
	for name, body := range map[string]any{
		"not an object": "not an object",
		"no check":      api.Mute{Namespace: "prod"},
	} {
		t.Run(name, func(t *testing.T) {
			ts, srv := dashboardPair(t, newPodObject("prod", "web-0"))
			kept := api.Mute{Check: "requests-missing", Namespace: "prod"}
			if resp := send(t, http.MethodPost, ts.URL+"/api/checks/mutes", kept); resp.StatusCode != http.StatusOK {
				t.Fatalf("preload status = %d, want 200", resp.StatusCode)
			}

			resp := send(t, http.MethodDelete, ts.URL+"/api/checks/mutes", body)

			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", resp.StatusCode)
			}
			held := checks.ParseMutes(srv.stored().All()[checks.MutesKey], mk2)
			if len(held) != 1 || held[0].Check != kept.Check || held[0].Namespace != kept.Namespace {
				t.Fatalf("mutes = %+v, want the original mute retained", held)
			}
		})
	}
}

func TestCappedRBACIndexUsesAnEmptyArrayForNoSubjects(t *testing.T) {
	got := cappedIndex(api.RBACIndex{})

	if got.Subjects == nil {
		t.Fatal("subjects are nil, want an empty array in the API response")
	}
	if len(got.Subjects) != 0 || got.Dropped != 0 {
		t.Fatalf("index = %+v, want an empty uncapped response", got)
	}
}

func TestIncompleteActionWithoutAMessageUsesAStableFailure(t *testing.T) {
	for _, outcome := range []string{api.OutcomeBlocked, api.OutcomeFailed} {
		t.Run(outcome, func(t *testing.T) {
			err := incompleteAction(api.ActionResult{Pods: []api.PodOutcome{{Outcome: outcome}}})

			if !errors.Is(err, api.ErrInternal) {
				t.Fatalf("error = %v, want ErrInternal", err)
			}
			if !strings.Contains(err.Error(), "action did not finish") {
				t.Fatalf("error = %q, want the stable fallback", err)
			}
		})
	}
}

func TestUnknownActionsReachActionValidationWithoutAProtectionPrompt(t *testing.T) {
	if guarded(actions.Request{Action: actions.Action("not-an-action")}) {
		t.Fatal("an unknown action was treated as a confirmed cluster mutation")
	}
}

func TestFleetWideReadPassesWhenEveryOpenClusterIsReadable(t *testing.T) {
	held := &fleet{
		held: []api.OpenCluster{
			{ID: mk1, Context: "p-mk1", Active: true},
			{ID: mk2, Context: "p-mk2"},
		},
		active: mk1,
		backends: map[string]Backend{
			mk1: everyNamespace(),
			mk2: everyNamespace(),
		},
	}
	srv := New(held, testAssets(), "")
	req := httptest.NewRequest(http.MethodGet, "/api/search/fleet?q=api", http.NoBody)

	status, why := srv.wholeFleetRefusal(req)

	if status != 0 || why != "" {
		t.Fatalf("status = %d, refusal = %q, want the fleet read allowed", status, why)
	}
}

func TestWholeClusterScopeGateLeavesMissingClusterHandlingToReachability(t *testing.T) {
	srv := New(&stubBackendCluster{}, testAssets(), "")
	req := httptest.NewRequest(http.MethodGet, "/api/search?q=api", http.NoBody)

	status, why := srv.wholeClusterRefusal(req)

	if status != 0 || why != "" {
		t.Fatalf("status = %d, refusal = %q, want no duplicate refusal", status, why)
	}
}
