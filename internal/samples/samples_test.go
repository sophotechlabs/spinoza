package samples

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

var start = time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

func reading(cpuMilli, memoryMi int64) map[string]api.ResourceUsage {
	return map[string]api.ResourceUsage{
		"flux-system/apps": {CPUMilli: cpuMilli, MemoryMi: memoryMi},
	}
}

func apps(store *Store, at time.Time) api.MetricHistory {
	return store.History("flux-system", "apps", time.Hour, at)
}

func TestReadingsComeBackInTheUnitsAChartDraws(t *testing.T) {
	store := New()

	store.Record(start, reading(250, 512))

	got := apps(store, start)
	if len(got.CPU) != 1 {
		t.Fatalf("cpu points = %d, want one", len(got.CPU))
	}
	if got.CPU[0].Value != 0.25 {
		t.Fatalf("cpu = %v cores, want 250 millicores as 0.25", got.CPU[0].Value)
	}
	if got.Memory[0].Value != 512*1024*1024 {
		t.Fatalf("memory = %v bytes, want 512Mi", got.Memory[0].Value)
	}
}

func TestAnAnswerSaysItWasMeasuredHere(t *testing.T) {
	store := New()

	store.Record(start, reading(1, 1))

	got := apps(store, start)
	if !got.Sampled {
		t.Fatal("an answer from the store did not say it was sampled")
	}
	if got.Namespace != "flux-system" || got.Pod != "apps" {
		t.Fatalf("answer names %s/%s, want the pod asked about", got.Namespace, got.Pod)
	}
}

func TestAPodNobodyHasMeasuredIsEmptyRatherThanNothing(t *testing.T) {
	store := New()

	got := store.History("default", "never-seen", time.Hour, start)

	if got.CPU == nil || got.Memory == nil {
		t.Fatal("an unmeasured pod came back with no lists at all")
	}
	if len(got.CPU) != 0 {
		t.Fatalf("cpu points = %d, want none", len(got.CPU))
	}
	if got.Since != 0 {
		t.Fatalf("since = %d, want nothing to have been collected", got.Since)
	}
}

func TestTwoReadsInsideOneIntervalKeepOneReading(t *testing.T) {
	store := New()

	store.Record(start, reading(100, 100))
	store.Record(start.Add(5*time.Second), reading(200, 200))
	store.Record(start.Add(9*time.Second), reading(300, 300))

	got := apps(store, start.Add(9*time.Second))
	if len(got.CPU) != 1 {
		t.Fatalf("cpu points = %d, want the readings inside one interval collapsed", len(got.CPU))
	}
	if got.CPU[0].Value != 0.1 {
		t.Fatalf("cpu = %v, want the first reading kept", got.CPU[0].Value)
	}
}

func TestAReadingAfterTheIntervalIsKept(t *testing.T) {
	store := New()

	store.Record(start, reading(100, 100))
	store.Record(start.Add(Every), reading(200, 200))

	got := apps(store, start.Add(Every))
	if len(got.CPU) != 2 {
		t.Fatalf("cpu points = %d, want both readings", len(got.CPU))
	}
}

func TestReadingsOlderThanTheWindowFallOff(t *testing.T) {
	store := New()
	at := start

	for range 5 {
		store.Record(at, reading(100, 100))
		at = at.Add(30 * time.Minute)
	}

	got := store.History("flux-system", "apps", 24*time.Hour, at)
	if len(got.CPU) != 3 {
		t.Fatalf("cpu points = %d, want only the last hour of them", len(got.CPU))
	}
}

func TestOnlyTheSpanAskedForComesBack(t *testing.T) {
	store := New()
	at := start
	for range 4 {
		store.Record(at, reading(100, 100))
		at = at.Add(10 * time.Minute)
	}
	last := at.Add(-10 * time.Minute)

	got := store.History("flux-system", "apps", 15*time.Minute, last)

	if len(got.CPU) != 2 {
		t.Fatalf("cpu points = %d, want the two inside fifteen minutes", len(got.CPU))
	}
}

func TestSinceIsTheOldestReadingInTheAnswer(t *testing.T) {
	store := New()
	at := start
	for range 4 {
		store.Record(at, reading(100, 100))
		at = at.Add(10 * time.Minute)
	}
	last := at.Add(-10 * time.Minute)

	got := store.History("flux-system", "apps", 15*time.Minute, last)

	want := start.Add(20 * time.Minute).UnixMilli()
	if got.Since != want {
		t.Fatalf("since = %d, want the oldest reading inside the span (%d)", got.Since, want)
	}
}

func TestAPodThatHasGoneIsForgotten(t *testing.T) {
	store := New()
	store.Record(start, map[string]api.ResourceUsage{
		"flux-system/apps":  {CPUMilli: 100},
		"flux-system/infra": {CPUMilli: 200},
	})

	store.Record(start.Add(Every), map[string]api.ResourceUsage{
		"flux-system/apps": {CPUMilli: 100},
	})

	gone := store.History("flux-system", "infra", time.Hour, start.Add(Every))
	if len(gone.CPU) != 0 {
		t.Fatalf("a pod that left kept %d readings", len(gone.CPU))
	}
	stayed := apps(store, start.Add(Every))
	if len(stayed.CPU) != 2 {
		t.Fatalf("the pod that stayed has %d readings, want both", len(stayed.CPU))
	}
}

func TestAReadingOfNothingIsNotTakenAsEveryPodLeaving(t *testing.T) {
	store := New()
	store.Record(start, reading(100, 100))

	store.Record(start.Add(Every), map[string]api.ResourceUsage{})

	if got := apps(store, start.Add(Every)); len(got.CPU) != 1 {
		t.Fatalf("cpu points = %d, want the reading kept through an empty read", len(got.CPU))
	}
}

func TestOnlySoManyPodsAreRemembered(t *testing.T) {
	store := New()
	store.limit = 3
	crowd := map[string]api.ResourceUsage{}
	for i := range 10 {
		crowd[fmt.Sprintf("default/pod-%d", i)] = api.ResourceUsage{CPUMilli: 100}
	}

	store.Record(start, crowd)

	if len(store.pods) != 3 {
		t.Fatalf("remembered %d pods, want the limit of 3", len(store.pods))
	}
}

func TestAPodAlreadyRememberedKeepsItsPlaceAtTheLimit(t *testing.T) {
	store := New()
	store.limit = 2
	store.Record(start, map[string]api.ResourceUsage{"flux-system/apps": {CPUMilli: 100}})

	crowd := map[string]api.ResourceUsage{"flux-system/apps": {CPUMilli: 100}}
	for i := range 10 {
		crowd[fmt.Sprintf("default/pod-%d", i)] = api.ResourceUsage{CPUMilli: 100}
	}
	store.Record(start.Add(Every), crowd)

	if got := apps(store, start.Add(Every)); len(got.CPU) != 2 {
		t.Fatalf("the pod being watched has %d readings, want both", len(got.CPU))
	}
}

func TestRememberedPodsNeverPushTheStorePastItsLimit(t *testing.T) {
	for attempt := range 8 {
		store := New()
		store.limit = 2
		store.Record(start, map[string]api.ResourceUsage{
			"flux-system/apps":  {CPUMilli: 100},
			"flux-system/infra": {CPUMilli: 100},
		})
		crowd := map[string]api.ResourceUsage{
			"flux-system/apps":  {CPUMilli: 100},
			"flux-system/infra": {CPUMilli: 100},
		}
		for i := range 64 {
			crowd[fmt.Sprintf("default/pod-%d", i)] = api.ResourceUsage{CPUMilli: 100}
		}

		store.Record(start.Add(Every), crowd)

		if len(store.pods) > store.limit {
			t.Fatalf("attempt %d remembered %d pods, limit %d", attempt, len(store.pods), store.limit)
		}
	}
}

func TestTheStoreIsWrittenAndReadAtOnce(t *testing.T) {
	store := New()
	var group sync.WaitGroup

	group.Go(func() {
		at := start
		for range 500 {
			store.Record(at, reading(100, 100))
			at = at.Add(Every)
		}
	})
	group.Go(func() {
		for range 500 {
			apps(store, start)
		}
	})
	group.Wait()
}
