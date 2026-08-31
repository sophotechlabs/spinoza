package checks

import (
	"context"
	"slices"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type Baseline struct {
	TakenAt string
	Cluster string
	Checks  []string
	Counts  map[string]int
	Scanned int
	Keys    map[string]string
}

func (b *Baseline) counted(id string) (int, bool) {
	if !b.covers(id) {
		return 0, false
	}
	return b.Counts[id], true
}

func (b *Baseline) covers(id string) bool {
	if b == nil {
		return false
	}
	return slices.Contains(b.Checks, id)
}

func (b *Baseline) has(key string) bool {
	_, found := b.Keys[key]
	return found
}

func (b *Baseline) gone(id string, here map[string]bool) []string {
	if !b.covers(id) {
		return nil
	}
	out := []string{}
	for key, label := range b.Keys {
		if !strings.HasPrefix(key, id+"\x00") {
			continue
		}
		if here[key] {
			continue
		}
		out = append(out, label)
	}
	slices.Sort(out)
	return out
}

func (b *Baseline) fixed(id string, now int) int {
	if !b.covers(id) {
		return 0
	}
	if b.Counts[id] <= now {
		return 0
	}
	return b.Counts[id] - now
}

func Fingerprint(
	ctx context.Context,
	lister Lister,
	descs map[string]api.ResourceDescriptor,
	usage api.Metrics,
	keep Filter,
) Baseline {
	wide := Filter{
		Rules:        keep.Rules,
		WholeCluster: keep.WholeCluster,
		EveryKind:    keep.EveryKind,
	}
	sc, _, _ := survey(ctx, lister, descs, usage, wide)
	out := Baseline{
		Checks:  []string{},
		Counts:  map[string]int{},
		Keys:    map[string]string{},
		Scanned: len(sc.subjects),
	}
	for _, entry := range registryWith(wide.Rules) {
		if ctx.Err() != nil {
			return Baseline{Checks: []string{}, Counts: map[string]int{}, Keys: map[string]string{}}
		}
		if entry.standsDown(sc) != "" || !entry.comparable() {
			continue
		}
		found := entry.find(sc)
		out.Checks = append(out.Checks, entry.id)
		out.Counts[entry.id] = len(found)
		for _, item := range found {
			out.Keys[entry.id+"\x00"+fingerprintOf(identityOf(entry.id, item))] = labelOf(item)
		}
	}
	return out
}
