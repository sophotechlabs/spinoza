package resources

import (
	"context"
	"sync"
	"time"

	"github.com/sophotechlabs/spinoza/internal/auth"
)

type recent[T any] struct {
	mu       sync.Mutex
	at       time.Time
	value    T
	building chan struct{}
}

type turn struct {
	done chan struct{}
	mine bool
}

func (r *recent[T]) fresh(now time.Time, ttl time.Duration) (T, *turn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.at.IsZero() && now.Sub(r.at) < ttl {
		return r.value, nil
	}
	if r.building != nil {
		var zero T
		return zero, &turn{done: r.building}
	}
	r.building = make(chan struct{})
	var zero T
	return zero, &turn{done: r.building, mine: true}
}

func (r *recent[T]) store(value T, at time.Time, keep bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if keep {
		r.value = value
		r.at = at
	}
	done := r.building
	r.building = nil
	if done != nil {
		close(done)
	}
}

func shared[T any](
	ctx context.Context,
	store *recent[T],
	now func() time.Time,
	ttl time.Duration,
	build func(context.Context) (T, bool),
) (T, bool) {
	for {
		value, waiting := store.fresh(now(), ttl)
		if waiting == nil {
			return value, true
		}
		if waiting.mine {
			built, keep := build(auth.AsServer(ctx))
			store.store(built, now(), keep)
			return built, true
		}
		select {
		case <-waiting.done:
		case <-ctx.Done():
			var zero T
			return zero, false
		}
	}
}
