package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/store"
)

type boundTabs struct {
	*heldTabs
}

func (b boundTabs) Forget(ctx context.Context, id string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return b.heldTabs.Forget(ctx, id)
}

func TestClosingAClusterForgetsItEvenIfTheRequestIsAbandoned(t *testing.T) {
	held := &fleet{
		held:   []api.OpenCluster{{ID: mk1, Context: "p-mk1", Active: true}},
		active: mk1,
	}
	srv := New(held, testAssets(), testToken)
	kept := &heldTabs{tabs: []store.Tab{{ID: mk1, Context: "p-mk1", Reopen: true}}}
	srv.UseTabs(boundTabs{heldTabs: kept})

	gone, walkAway := context.WithCancel(context.Background())
	req, reqErr := http.NewRequestWithContext(
		gone,
		http.MethodDelete,
		"http://spinoza.test/api/clusters?cluster="+url.QueryEscape(mk1),
		http.NoBody,
	)
	if reqErr != nil {
		t.Fatalf("request: %v", reqErr)
	}
	walkAway()
	srv.closeCluster(httptest.NewRecorder(), req)

	if remembered := kept.remembered(); len(remembered) != 0 {
		t.Fatalf("remembered %+v after the close, so it comes back next launch", remembered)
	}
}
