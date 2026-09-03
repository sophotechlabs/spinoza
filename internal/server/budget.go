package server

import "sync"

type workBudget struct {
	mu          sync.Mutex
	global      int
	perIdentity int
	used        int
	byIdentity  map[string]int
}

func newWorkBudget(global, perIdentity int) *workBudget {
	return &workBudget{
		global:      global,
		perIdentity: perIdentity,
		byIdentity:  map[string]int{},
	}
}

func (b *workBudget) claim(identity string, units int) (func(), bool) {
	if b == nil {
		return func() {}, true
	}
	if units <= 0 {
		return func() {}, true
	}
	b.mu.Lock()
	globalFull := b.used+units > b.global
	identityFull := b.byIdentity[identity]+units > b.perIdentity
	if globalFull || identityFull {
		b.mu.Unlock()
		return nil, false
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
			b.mu.Unlock()
		})
	}, true
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
