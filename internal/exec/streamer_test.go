package exec

import (
	"context"
	"net/url"
	"testing"
	"time"

	restclient "k8s.io/client-go/rest"
)

func TestFallbackExecutorIsBuiltForBothProtocols(t *testing.T) {
	endpoint, err := url.Parse("https://api.example:6443/api/v1/namespaces/monitoring/pods/loki-0/exec")
	if err != nil {
		t.Fatalf("url: %v", err)
	}
	executor, err := fallbackExecutor(&restclient.Config{Host: endpoint.Host}, endpoint)
	if err != nil {
		t.Fatalf("executor: %v", err)
	}
	if executor == nil {
		t.Fatal("expected an executor")
	}
}

func TestFallbackExecutorReportsABrokenConfig(t *testing.T) {
	endpoint, _ := url.Parse("https://api.example:6443/exec")
	config := &restclient.Config{
		Host: endpoint.Host,
		TLSClientConfig: restclient.TLSClientConfig{
			CAFile: "/nonexistent/ca.crt",
		},
	}
	_, err := fallbackExecutor(config, endpoint)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestNewStreamerIsConstructed(t *testing.T) {
	streamer := NewStreamer(nil, &restclient.Config{Host: "api.example:6443"})
	if streamer == nil {
		t.Fatal("expected a streamer")
	}
}

func TestSizeQueueReturnsTheRequestedSize(t *testing.T) {
	sizes := make(chan Size, 1)
	sizes <- Size{Cols: 120, Rows: 40}
	queue := &sizeQueue{ctx: context.Background(), resize: sizes}

	next := queue.Next()
	if next == nil {
		t.Fatal("expected a size")
	}
	if next.Width != 120 {
		t.Fatalf("width = %d", next.Width)
	}
	if next.Height != 40 {
		t.Fatalf("height = %d", next.Height)
	}
}

func TestSizeQueueEndsWithTheContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	queue := &sizeQueue{ctx: ctx, resize: make(chan Size)}

	done := make(chan *struct{}, 1)
	go func() {
		next := queue.Next()
		if next == nil {
			done <- nil
			return
		}
		done <- &struct{}{}
	}()

	select {
	case got := <-done:
		if got != nil {
			t.Fatal("expected a nil size after cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Next did not return")
	}
}

func TestSizeQueueEndsWhenTheChannelCloses(t *testing.T) {
	sizes := make(chan Size)
	close(sizes)
	queue := &sizeQueue{ctx: context.Background(), resize: sizes}

	if queue.Next() != nil {
		t.Fatal("expected a nil size after close")
	}
}

func TestShouldFallbackIgnoresOrdinaryErrors(t *testing.T) {
	if shouldFallback(context.Canceled) {
		t.Fatal("a cancelled context is not an upgrade failure")
	}
}
