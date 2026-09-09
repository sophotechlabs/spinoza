package telemetry

import (
	"math"
	"slices"
	"sync"
)

const (
	namePrefix  = "spinoza_"
	seriesLimit = 200
	otherValue  = "other"
	keyJoin     = "\x00"
)

const (
	counterKind   = "counter"
	gaugeKind     = "gauge"
	histogramKind = "histogram"
)

type Registry struct {
	mu       sync.Mutex
	vectors  map[string]*vector
	folded   *Counter
	rejected *Counter
}

func New() *Registry {
	reg := &Registry{vectors: map[string]*vector{}}
	reg.folded = reg.Counter(
		"spinoza_telemetry_series_folded_total",
		"writes folded into the other series because a metric ran out of room",
		"metric",
	)
	reg.rejected = reg.Counter(
		"spinoza_telemetry_writes_rejected_total",
		"writes refused because they did not match what the metric declared",
		"metric",
	)
	reg.folded.vec.silent = true
	reg.rejected.vec.silent = true
	return reg
}

func (r *Registry) Counter(name, help string, labels ...string) *Counter {
	return &Counter{vec: r.declare(name, help, counterKind, nil, labels)}
}

func (r *Registry) Gauge(name, help string, labels ...string) *Gauge {
	return &Gauge{vec: r.declare(name, help, gaugeKind, nil, labels)}
}

func (r *Registry) Histogram(name, help string, buckets []float64, labels ...string) *Histogram {
	return &Histogram{vec: r.declare(name, help, histogramKind, buckets, labels)}
}

func (r *Registry) declare(name, help, kind string, buckets []float64, labels []string) *vector {
	checkName(name)
	checkLabels(labels)
	made := &vector{
		name:    name,
		help:    help,
		kind:    kind,
		labels:  slices.Clone(labels),
		buckets: bounds(kind, buckets),
		series:  map[string]*series{},
		reg:     r,
	}
	if len(made.labels) == 0 {
		made.series[""] = made.blank(nil)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, taken := r.vectors[name]
	if taken {
		panic("telemetry: " + name + " is declared twice")
	}
	r.vectors[name] = made
	return made
}

func checkName(name string) {
	if !validName(name) {
		panic("telemetry: " + name + " is not a usable metric name")
	}
	if len(name) <= len(namePrefix) || name[:len(namePrefix)] != namePrefix {
		panic("telemetry: " + name + " does not start with " + namePrefix)
	}
}

func checkLabels(labels []string) {
	seen := map[string]bool{}
	for _, label := range labels {
		if !validName(label) {
			panic("telemetry: " + label + " is not a usable label name")
		}
		if label == "le" {
			panic("telemetry: le is reserved for histogram buckets")
		}
		if seen[label] {
			panic("telemetry: " + label + " is declared twice on one metric")
		}
		seen[label] = true
	}
}

func validName(name string) bool {
	if name == "" {
		return false
	}
	for at, letter := range name {
		if letter >= 'a' && letter <= 'z' {
			continue
		}
		if letter >= 'A' && letter <= 'Z' {
			continue
		}
		if letter == '_' {
			continue
		}
		if at > 0 && letter >= '0' && letter <= '9' {
			continue
		}
		return false
	}
	return true
}

func bounds(kind string, given []float64) []float64 {
	if kind != histogramKind {
		if len(given) > 0 {
			panic("telemetry: only a histogram has buckets")
		}
		return nil
	}
	if len(given) == 0 {
		panic("telemetry: a histogram needs at least one bucket")
	}
	out := slices.Compact(slices.Sorted(slices.Values(given)))
	for _, bound := range out {
		if math.IsNaN(bound) || math.IsInf(bound, 0) {
			panic("telemetry: a bucket bound has to be a finite number")
		}
	}
	return out
}
