package inspect

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

const testTimeout = 150 * time.Millisecond

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

func shorten(t *testing.T, limit *time.Duration) {
	t.Helper()
	previous := *limit
	*limit = testTimeout
	t.Cleanup(func() {
		*limit = previous
	})
}

func wantsDeadline(t *testing.T, err error, elapsed time.Duration) {
	t.Helper()
	if err == nil {
		t.Fatal("a request against an apiserver that never answers returned no error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the deadline to have expired", err)
	}
	if elapsed > time.Second {
		t.Fatalf("the call took %s, so it was not the cap that ended it", elapsed)
	}
}

func TestGetGivesUpOnAnApiserverThatNeverAnswers(t *testing.T) {
	shorten(t, &readTimeout)
	dyn := hungAPIServer(t)

	started := time.Now()
	_, err := Get(context.Background(), dyn, podRef())

	wantsDeadline(t, err, time.Since(started))
}

func TestApplyGivesUpOnAnApiserverThatNeverAnswers(t *testing.T) {
	shorten(t, &writeTimeout)
	dyn := hungAPIServer(t)
	doc := []byte("apiVersion: v1\nkind: Pod\nmetadata:\n  name: web\n  namespace: flux-system\n")

	started := time.Now()
	_, err := Apply(context.Background(), dyn, podRef(), "Pod", doc)

	wantsDeadline(t, err, time.Since(started))
}

func TestEventsGiveUpOnAnApiserverThatNeverAnswers(t *testing.T) {
	shorten(t, &listTimeout)
	dyn := hungAPIServer(t)

	started := time.Now()
	_, err := Events(context.Background(), dyn, "flux-system", "11111111-2222-3333-4444-555555555555")

	wantsDeadline(t, err, time.Since(started))
}

func TestTheCallerKeepsAShorterDeadlineOfItsOwn(t *testing.T) {
	dyn := hungAPIServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	started := time.Now()
	_, err := Get(ctx, dyn, podRef())

	wantsDeadline(t, err, time.Since(started))
}
