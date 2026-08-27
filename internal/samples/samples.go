// Package samples remembers what the cluster's own metrics said, so that a pod
// has a chart on a cluster with no Prometheus in it.
//
// Everything here is what spinoza saw while it was running. There is no history
// before the process started and none after it stops, which is the whole
// difference between this and a metrics database — the caller is told which of
// the two answered so that it can say so.
package samples

import (
	"sync"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	// Window is how far back a pod is remembered.
	Window = time.Hour
	// Every is the closest two samples of the same pod may be. Metrics are read
	// whenever a page asks for them, which is more often than the cluster
	// measures anything, so the ones in between would only be the same number
	// again.
	Every = 15 * time.Second
	// Pods is how many pods are remembered at once. A cluster bigger than this
	// is one that has a metrics database of its own; what it costs to keep more
	// is not worth spending on the few who do not.
	Pods = 2000
)

const (
	milliPerCore = 1000
	bytesPerMi   = 1024 * 1024
)

// A sample is one reading of one pod, in the units the cluster reports.
type sample struct {
	at       int64
	cpuMilli int64
	memoryMi int64
}

// Store holds the readings. It is written by whoever refreshed the metrics and
// read by whoever opened a chart, so every method takes the lock.
type Store struct {
	mu     sync.Mutex
	window time.Duration
	every  time.Duration
	limit  int
	pods   map[string][]sample
}

func New() *Store {
	return &Store{
		window: Window,
		every:  Every,
		limit:  Pods,
		pods:   map[string][]sample{},
	}
}

// Record keeps what a fresh read of the cluster's metrics said. Pods missing
// from it have gone, and are forgotten with it: the reading lists every pod the
// cluster measures, so absence is an answer rather than a gap.
func (s *Store) Record(at time.Time, pods map[string]api.ResourceUsage) {
	if len(pods) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := make(map[string][]sample, len(pods))
	for key, use := range pods {
		held, seen := s.pods[key]
		if !seen && len(kept) >= s.limit {
			continue
		}
		kept[key] = s.append(held, at, use)
	}
	s.pods = kept
}

func (s *Store) append(held []sample, at time.Time, use api.ResourceUsage) []sample {
	stamp := at.UnixMilli()
	if len(held) > 0 && stamp-held[len(held)-1].at < s.every.Milliseconds() {
		return held
	}
	held = append(held, sample{at: stamp, cpuMilli: use.CPUMilli, memoryMi: use.MemoryMi})
	return trim(held, stamp-s.window.Milliseconds())
}

func trim(held []sample, oldest int64) []sample {
	cut := 0
	for cut < len(held) && held[cut].at < oldest {
		cut++
	}
	if cut == 0 {
		return held
	}
	return append([]sample{}, held[cut:]...)
}

// History is what is remembered about one pod over the span asked for, in the
// units a chart draws: cores and bytes, the same as a metrics database reports.
func (s *Store) History(namespace, pod string, span time.Duration, now time.Time) api.MetricHistory {
	out := api.MetricHistory{
		Namespace: namespace,
		Pod:       pod,
		Sampled:   true,
		CPU:       []api.MetricPoint{},
		Memory:    []api.MetricPoint{},
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	held := s.pods[namespace+"/"+pod]
	oldest := now.Add(-span).UnixMilli()
	for _, one := range held {
		if one.at < oldest {
			continue
		}
		if out.Since == 0 {
			out.Since = one.at
		}
		at := one.at / 1000
		out.CPU = append(out.CPU, api.MetricPoint{At: at, Value: cores(one.cpuMilli)})
		out.Memory = append(out.Memory, api.MetricPoint{At: at, Value: bytes(one.memoryMi)})
	}
	return out
}

func cores(milli int64) float64 {
	return float64(milli) / milliPerCore
}

func bytes(mebi int64) float64 {
	return float64(mebi) * bytesPerMi
}
