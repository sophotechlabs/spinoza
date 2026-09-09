package telemetry

import (
	"math"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

type vector struct {
	name    string
	help    string
	kind    string
	labels  []string
	buckets []float64
	reg     *Registry
	silent  bool

	mu     sync.Mutex
	series map[string]*series
}

type series struct {
	values  []string
	value   atomic.Uint64
	sum     atomic.Uint64
	count   atomic.Uint64
	buckets []atomic.Uint64
}

func (v *vector) blank(values []string) *series {
	made := &series{values: slices.Clone(values)}
	if v.kind == histogramKind {
		made.buckets = make([]atomic.Uint64, len(v.buckets))
	}
	return made
}

func (v *vector) at(values []string) *series {
	if len(values) != len(v.labels) {
		v.reject()
		return nil
	}
	key := strings.Join(values, keyJoin)
	v.mu.Lock()
	found, known := v.series[key]
	if known {
		v.mu.Unlock()
		return found
	}
	if len(v.series) < seriesLimit {
		made := v.blank(values)
		v.series[key] = made
		v.mu.Unlock()
		return made
	}
	folded := v.otherLocked()
	v.mu.Unlock()
	v.fold()
	return folded
}

func (v *vector) otherLocked() *series {
	values := make([]string, len(v.labels))
	for at := range values {
		values[at] = otherValue
	}
	key := strings.Join(values, keyJoin)
	found, known := v.series[key]
	if known {
		return found
	}
	made := v.blank(values)
	v.series[key] = made
	return made
}

func (v *vector) fold() {
	if v.silent {
		return
	}
	v.reg.folded.Inc(v.name)
}

func (v *vector) reject() {
	if v.silent {
		return
	}
	v.reg.rejected.Inc(v.name)
}

func (v *vector) count() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.series)
}

type Counter struct {
	vec *vector
}

func (c *Counter) Inc(labels ...string) {
	c.Add(1, labels...)
}

func (c *Counter) Add(delta float64, labels ...string) {
	if math.IsNaN(delta) || delta < 0 {
		c.vec.reject()
		return
	}
	found := c.vec.at(labels)
	if found == nil {
		return
	}
	addFloat(&found.value, delta)
}

type Gauge struct {
	vec *vector
}

func (g *Gauge) Set(value float64, labels ...string) {
	found := g.vec.at(labels)
	if found == nil {
		return
	}
	found.value.Store(math.Float64bits(value))
}

func (g *Gauge) Add(delta float64, labels ...string) {
	found := g.vec.at(labels)
	if found == nil {
		return
	}
	addFloat(&found.value, delta)
}

func (g *Gauge) Inc(labels ...string) {
	g.Add(1, labels...)
}

func (g *Gauge) Dec(labels ...string) {
	g.Add(-1, labels...)
}

type Histogram struct {
	vec *vector
}

func (h *Histogram) Observe(value float64, labels ...string) {
	if math.IsNaN(value) {
		h.vec.reject()
		return
	}
	found := h.vec.at(labels)
	if found == nil {
		return
	}
	found.count.Add(1)
	for at, bound := range h.vec.buckets {
		if value <= bound {
			found.buckets[at].Add(1)
			break
		}
	}
	addFloat(&found.sum, value)
}

func addFloat(into *atomic.Uint64, delta float64) {
	for {
		current := into.Load()
		next := math.Float64bits(math.Float64frombits(current) + delta)
		if into.CompareAndSwap(current, next) {
			return
		}
	}
}
