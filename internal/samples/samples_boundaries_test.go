package samples

import (
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestDisablingSamplesDropsPodsAlreadyRemembered(t *testing.T) {
	store := New()
	store.Record(start, reading(100, 100))
	store.limit = 0

	store.Record(start.Add(Every), reading(200, 200))

	if len(store.pods) != 0 {
		t.Fatalf("remembered pods = %d, want none while sampling is disabled", len(store.pods))
	}
	if got := apps(store, start.Add(Every)); len(got.CPU) != 0 {
		t.Fatalf("cpu points = %d, want the old samples dropped", len(got.CPU))
	}
}

func TestLoweringThePodLimitTrimsPodsAlreadyRemembered(t *testing.T) {
	store := New()
	store.limit = 3
	pods := map[string]api.ResourceUsage{
		"default/api":    {CPUMilli: 100},
		"default/web":    {CPUMilli: 200},
		"default/worker": {CPUMilli: 300},
	}
	store.Record(start, pods)
	store.limit = 1

	store.Record(start.Add(Every), pods)

	if len(store.pods) != 1 {
		t.Fatalf("remembered pods = %d, want the lowered limit of 1", len(store.pods))
	}
}
