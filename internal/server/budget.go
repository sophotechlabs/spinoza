package server

import (
	"context"
	"sync"
)

type workBudget struct {
	mu          sync.Mutex
	global      int
	perIdentity int
	used        int
	byIdentity  map[string]int
	freed       chan struct{}
}

func newWorkBudget(global, perIdentity int) *workBudget {
	return &workBudget{
		global:      global,
		perIdentity: perIdentity,
		byIdentity:  map[string]int{},
		freed:       make(chan struct{}),
	}
}

func (b *workBudget) claim(identity string, units int) (func(), bool) {
	release, ok, _ := b.attempt(identity, units)
	return release, ok
}

func (b *workBudget) await(ctx context.Context, identity string) (func(), bool) {
	for {
		release, ok, freed := b.attempt(identity, 1)
		if ok {
			return release, true
		}
		select {
		case <-freed:
		case <-ctx.Done():
			return nil, false
		}
	}
}

func (b *workBudget) attempt(identity string, units int) (func(), bool, <-chan struct{}) {
	if b == nil {
		return func() {}, true, nil
	}
	if units <= 0 {
		return func() {}, true, nil
	}
	b.mu.Lock()
	if b.freed == nil {
		b.freed = make(chan struct{})
	}
	globalFull := b.used+units > b.global
	identityFull := b.byIdentity[identity]+units > b.perIdentity
	if globalFull || identityFull {
		freed := b.freed
		b.mu.Unlock()
		return nil, false, freed
	}
	b.used += units
	b.byIdentity[identity] += units
	b.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			b.used -= units
			b.byIdentity[identity] -= units
			if b.byIdentity[identity] == 0 {
				delete(b.byIdentity, identity)
			}
			close(b.freed)
			b.freed = make(chan struct{})
			b.mu.Unlock()
		})
	}, true, nil
}

type reservedResource struct {
	stoppable

	release func()
	once    sync.Once
}

func (r *reservedResource) Close() {
	r.once.Do(func() {
		r.stoppable.Close()
		r.release()
	})
}
