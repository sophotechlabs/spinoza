package checks

import (
	"context"
	"hash/fnv"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const surveyTTL = 2 * time.Second

type surveyed struct {
	at      time.Time
	scan    scan
	failure string
	absent  []string
}

type Surveys struct {
	mu   sync.Mutex
	ttl  time.Duration
	now  func() time.Time
	held map[string]surveyed
}

func NewSurveys(now func() time.Time) *Surveys {
	return &Surveys{ttl: surveyTTL, now: now, held: map[string]surveyed{}}
}

func (s *Surveys) take(
	ctx context.Context,
	lister Lister,
	descs map[string]api.ResourceDescriptor,
	usage api.Metrics,
	keep Filter,
) (scan, string, []string) {
	if s == nil {
		return survey(ctx, lister, descs, usage, keep)
	}
	key := surveyKey(descs, keep)
	held, fresh := s.lookup(key)
	if fresh {
		return held.scan, held.failure, held.absent
	}
	sc, failure, absent := survey(ctx, lister, descs, usage, keep)
	s.keep(key, surveyed{at: s.now(), scan: sc, failure: failure, absent: absent})
	return sc, failure, absent
}

func (s *Surveys) lookup(key string) (surveyed, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	held, ok := s.held[key]
	if !ok {
		return surveyed{}, false
	}
	if s.now().Sub(held.at) >= s.ttl {
		return surveyed{}, false
	}
	return held, true
}

func (s *Surveys) keep(key string, one surveyed) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for held, was := range s.held {
		if s.now().Sub(was.at) >= s.ttl {
			delete(s.held, held)
		}
	}
	s.held[key] = one
}

func surveyKey(descs map[string]api.ResourceDescriptor, keep Filter) string {
	ids := make([]string, 0, 32)
	for _, entry := range keep.chosen(registryWith(keep.Rules)) {
		ids = append(ids, entry.id)
	}
	slices.Sort(ids)
	return strconv.FormatBool(keep.WholeCluster) +
		"|" + strconv.FormatBool(keep.EveryKind) +
		"|" + strconv.FormatUint(fingerprint(descs), 10) +
		"|" + strings.Join(ids, ",")
}

func fingerprint(descs map[string]api.ResourceDescriptor) uint64 {
	keys := make([]string, 0, len(descs))
	for key := range descs {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	sum := fnv.New64a()
	for _, key := range keys {
		_, _ = sum.Write([]byte(key))
		_, _ = sum.Write([]byte{0})
	}
	return sum.Sum64()
}
