package server

import (
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestWorkBudgetEnforcesIdentityAndGlobalCapacity(t *testing.T) {
	budget := newWorkBudget(3, 2)
	releaseA, ok := budget.claim("alice", 2)
	if !ok {
		t.Fatal("alice was refused at the identity boundary")
	}
	if _, aliceClaimedAgain := budget.claim("alice", 1); aliceClaimedAgain {
		t.Fatal("alice exceeded the identity boundary")
	}
	releaseB, ok := budget.claim("bob", 1)
	if !ok {
		t.Fatal("bob was refused at the global boundary")
	}
	if _, carolClaimed := budget.claim("carol", 1); carolClaimed {
		t.Fatal("the global boundary was exceeded")
	}
	releaseA()
	releaseA()
	if _, ok := budget.claim("alice", 2); !ok {
		t.Fatal("released capacity was not reusable")
	}
	releaseB()
}

func TestRowFilterShapeIsBounded(t *testing.T) {
	accepted := make([]api.RowFilter, maxRowFilters, maxRowFilters+1)
	for index := range accepted {
		accepted[index] = api.RowFilter{
			Field: strings.Repeat("f", maxFilterFieldBytes),
			Value: strings.Repeat("v", maxFilterValueBytes),
		}
	}
	if err := validFilters(accepted); err != nil {
		t.Fatalf("boundary filters: %v", err)
	}

	cases := []struct {
		name    string
		filters []api.RowFilter
		want    string
	}{
		{
			name:    "count",
			filters: append(accepted, api.RowFilter{}),
			want:    "a subscription cannot contain more than 8 row filters",
		},
		{
			name:    "field",
			filters: []api.RowFilter{{Field: strings.Repeat("f", maxFilterFieldBytes+1)}},
			want:    "a row filter field cannot exceed 64 bytes",
		},
		{
			name:    "value",
			filters: []api.RowFilter{{Value: strings.Repeat("v", maxFilterValueBytes+1)}},
			want:    "a row filter value cannot exceed 256 bytes",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validFilters(tc.filters)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestLogRequestAndReservationsAreBounded(t *testing.T) {
	for _, tail := range []int64{0, maxLogTailLines} {
		if err := validLogRequest(api.ClientMsg{TailLines: tail}); err != nil {
			t.Fatalf("tail %d: %v", tail, err)
		}
	}
	for _, tail := range []int64{-1, maxLogTailLines + 1} {
		err := validLogRequest(api.ClientMsg{TailLines: tail})
		if err == nil || err.Error() != "log tail lines must be between 0 and 5000" {
			t.Fatalf("tail %d error = %v", tail, err)
		}
	}
	if got := logStreamUnits(api.ClientMsg{Resource: "pods"}); got != 1 {
		t.Fatalf("pod reservation = %d, want 1", got)
	}
	if got := logStreamUnits(api.ClientMsg{Resource: "deployments"}); got != maxWorkloadLogStreams {
		t.Fatalf("workload reservation = %d, want %d", got, maxWorkloadLogStreams)
	}
}
