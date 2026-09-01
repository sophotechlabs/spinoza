package main

import (
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/mcp"
)

type stubClusters struct {
	ref       api.ContextRef
	id        string
	protected map[string]bool
}

func (s *stubClusters) Current() api.ContextRef {
	return s.ref
}

func (s *stubClusters) ID() string {
	return s.id
}

func (s *stubClusters) Protected(cluster string) bool {
	return s.protected[cluster]
}

func TestTheServerReadsCurrentProtectionEveryTime(t *testing.T) {
	clusters := &stubClusters{
		ref:       api.ContextRef{Name: "p-mk2"},
		id:        "mk2",
		protected: map[string]bool{},
	}
	opts := optionsFor(clusters, mcp.Settings{AllowWrite: true}, nil)

	if opts.Protected() {
		t.Fatal("an unprotected cluster reported as protected")
	}
	clusters.protected["mk2"] = true
	if !opts.Protected() {
		t.Fatal("protection changed after startup was not seen")
	}
}

func TestTheProtectionIsReadForTheCurrentCluster(t *testing.T) {
	clusters := &stubClusters{
		ref:       api.ContextRef{Name: "p-mk1"},
		id:        "mk1",
		protected: map[string]bool{"mk2": true},
	}

	if optionsFor(clusters, mcp.Settings{AllowWrite: true}, nil).Protected() {
		t.Fatal("another cluster's protection was read as this cluster's")
	}
}

func TestTheMCPOptionsCarryAllSettings(t *testing.T) {
	clusters := &stubClusters{
		ref:       api.ContextRef{Name: "p-mk1"},
		id:        "mk1",
		protected: map[string]bool{},
	}
	wantBudget := 7 * time.Second
	opts := optionsFor(clusters, mcp.Settings{
		AllowWrite: true,
		LogLines:   42,
		CallBudget: wantBudget,
	}, nil)

	if opts.Context != "p-mk1" {
		t.Fatalf("context = %q, want p-mk1", opts.Context)
	}
	if !opts.AllowWrite {
		t.Fatal("the write flag was not carried")
	}
	if opts.LogLines != 42 {
		t.Fatalf("log lines = %d, want 42", opts.LogLines)
	}
	if opts.CallBudget != wantBudget {
		t.Fatalf("call budget = %s, want %s", opts.CallBudget, wantBudget)
	}
	if opts.Version == "" {
		t.Fatal("the server reports no version")
	}
}
