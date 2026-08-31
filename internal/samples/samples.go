package samples

import (
	"sync"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	Window = time.Hour
	Every  = 15 * time.Second
	Pods   = 2000
)

const (
	milliPerCore = 1000
	bytesPerMi   = 1024 * 1024
)

type sample struct {
	at       int64
	cpuMilli int64
	memoryMi int64
}

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
