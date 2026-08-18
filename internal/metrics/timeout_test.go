package metrics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

func hungAPIServer(t *testing.T) dynamic.Interface {
	t.Helper()
	stop := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-stop:
		}
	}))
	t.Cleanup(func() {
		close(stop)
		server.Close()
	})
	client, err := dynamic.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatalf("dynamic client: %v", err)
	}
	return client
}

func TestBuildGivesUpOnAMetricsApiThatNeverAnswers(t *testing.T) {
	previous := buildTimeout
	buildTimeout = 150 * time.Millisecond
	t.Cleanup(func() {
		buildTimeout = previous
	})
	dyn := hungAPIServer(t)

	started := time.Now()
	built := Build(context.Background(), dyn, FromCluster(dyn))
	elapsed := time.Since(started)

	if elapsed > time.Second {
		t.Fatalf("Build took %s, so it was not the cap that ended it", elapsed)
	}
	if built.Error == "" {
		t.Fatal("a metrics API that never answers produced no error for the client")
	}
	if !strings.Contains(built.Error, "deadline") {
		t.Fatalf("error = %q, want it to name the deadline", built.Error)
	}
	if len(built.Pods) != 0 {
		t.Fatalf("pods = %v, want nothing usable", built.Pods)
	}
}
