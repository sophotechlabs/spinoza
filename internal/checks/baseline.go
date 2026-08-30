package checks

import (
	"context"
	"slices"

	"github.com/sophotechlabs/spinoza/internal/api"
)

// Baseline is a past audit, kept so this one can say what is new. It holds
// fingerprints rather than findings, the count each check produced, and the
// ids it covered, so a check added since is reported as absent from the
// baseline instead of as thousands of new findings.
type Baseline struct {
	TakenAt string
	Checks  []string
	Counts  map[string]int
	Keys    map[string]bool
}

func (b *Baseline) covers(id string) bool {
	if b == nil {
		return false
	}
	return slices.Contains(b.Checks, id)
}

// has is only ever asked after covers said yes, so there is no nil to guard.
func (b *Baseline) has(key string) bool {
	return b.Keys[key]
}

// fixed is what the baseline counted for a check that this audit no longer
// finds. A check the baseline never ran reports nothing rather than guessing.
func (b *Baseline) fixed(id string, now int) int {
	if !b.covers(id) {
		return 0
	}
	if b.Counts[id] <= now {
		return 0
	}
	return b.Counts[id] - now
}

// Fingerprint is the audit taken to be compared against later. It deliberately
// ignores the mutes, the severity floor and the namespace the caller is looking
// at: a baseline narrower than the audits compared to it would report the
// difference between two filters as work someone did.
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
	out := Baseline{Checks: []string{}, Counts: map[string]int{}, Keys: map[string]bool{}}
	for _, entry := range registryWith(wide.Rules) {
		if entry.standsDown(sc) != "" {
			continue
		}
		found := entry.find(sc)
		out.Checks = append(out.Checks, entry.id)
		out.Counts[entry.id] = len(found)
		for _, item := range found {
			out.Keys[fingerprintOf(identityOf(entry.id, item))] = true
		}
	}
	return out
}
