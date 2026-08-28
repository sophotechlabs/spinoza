package issues

import (
	"context"
	"slices"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

// what the engine defaults to, and what it does when it runs out of time

func TestEveryLimitHasADefault(t *testing.T) {
	got := Limits{}.orDefaults()

	want := Limits{
		Budget:      defaultBudget,
		StallBudget: defaultStallBudget,
		StallGrace:  defaultStallGrace,
		ReadyGrace:  defaultReadyGrace,
		Rows:        defaultRows,
		Children:    defaultChildren,
		Candidates:  defaultCandidates,
		Fallback:    defaultFallback,
		Readers:     defaultReaders,
		StallReader: defaultStallReaders,
	}
	if got != want {
		t.Fatalf("limits = %+v, want %+v", got, want)
	}
}

func TestALimitThatIsSetIsLeftAlone(t *testing.T) {
	asked := Limits{
		Budget:      time.Second,
		StallBudget: 2 * time.Second,
		StallGrace:  3 * time.Second,
		ReadyGrace:  4 * time.Second,
		Rows:        1,
		Children:    2,
		Candidates:  3,
		Fallback:    4,
		Readers:     5,
		StallReader: 6,
	}

	if got := asked.orDefaults(); got != asked {
		t.Fatalf("limits = %+v, want them untouched", got)
	}
}

type blockingLister struct {
	reached chan struct{}
	once    bool
}

func (b *blockingLister) Lease(ctx context.Context, _ api.ResourceDescriptor) ([]*unstructured.Unstructured, error) {
	if !b.once {
		b.once = true
		close(b.reached)
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (b *blockingLister) Cached() []api.ResourceDescriptor {
	return nil
}

func TestABuildThatRunsOutOfTimeAnswersInsteadOfHanging(t *testing.T) {
	lister := &blockingLister{reached: make(chan struct{})}
	limits := testLimits()
	limits.Budget = 40 * time.Millisecond

	started := time.Now()
	queue := buildLimited(t, lister, &stubEvents{}, catalog(podDescriptor()), limits)
	took := time.Since(started)

	select {
	case <-lister.reached:
	default:
		t.Fatal("the lister was never asked; the test proved nothing")
	}
	if took > time.Second {
		t.Fatalf("build took %s, want it cut off near its %s budget", took, limits.Budget)
	}
	if queue.Error == "" {
		t.Fatalf("queue = %+v, want the timeout reported rather than passed off as a healthy cluster", queue)
	}
	if len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want none when nothing could be read", queue.Rows)
	}
}

type blockingEvents struct {
	reached chan struct{}
	once    bool
}

func (b *blockingEvents) Events(ctx context.Context, _, _ string) ([]api.Event, error) {
	if !b.once {
		b.once = true
		close(b.reached)
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestAStallProbeThatRunsOutOfTimeReportsNoStall(t *testing.T) {
	pod := newPod("web-1", withPhase(phasePending), withStartTime(testNow.Add(-time.Hour)))
	lister := &stubLister{items: itemsOf("pods", pod)}
	events := &blockingEvents{reached: make(chan struct{})}
	limits := testLimits()
	limits.StallBudget = 40 * time.Millisecond

	started := time.Now()
	queue := buildLimited(t, lister, events, catalog(podDescriptor()), limits)
	took := time.Since(started)

	select {
	case <-events.reached:
	default:
		t.Fatal("the event lookup was never made; the test proved nothing")
	}
	if took > time.Second {
		t.Fatalf("build took %s, want the stall probe cut off near its %s budget", took, limits.StallBudget)
	}
	if len(queue.Rows) != 0 {
		t.Fatalf("rows = %+v, want silence rather than a guess we could not check", queue.Rows)
	}
}

func TestTheStallProbeAsksAboutEveryCandidateAtOnce(t *testing.T) {
	pods := make([]*unstructured.Unstructured, 0, 4)
	for _, name := range []string{"web-1", "web-2", "web-3", "web-4"} {
		pods = append(pods, newPod(name, withPhase(phasePending), withStartTime(testNow.Add(-time.Hour))))
	}
	lister := &stubLister{items: itemsOf("pods", pods...)}
	events := &countingEvents{}
	limits := testLimits()
	limits.StallReader = 4

	buildLimited(t, lister, events, catalog(podDescriptor()), limits)

	if got := events.highWater(); got < 2 {
		t.Fatalf("peak concurrent lookups = %d, want the probe asking in parallel rather than one at a time", got)
	}
}

func TestTheFallbackTypeListIsCapped(t *testing.T) {
	cached := []api.ResourceDescriptor{
		descriptor("a.example.com", "v1", "alphas", "Alpha"),
		descriptor("b.example.com", "v1", "betas", "Beta"),
		descriptor("c.example.com", "v1", "gammas", "Gamma"),
	}
	lister := &stubLister{cached: cached}
	limits := testLimits()
	limits.Fallback = 2

	buildLimited(t, lister, &stubEvents{}, catalog(), limits)

	leased := lister.leasedResources()
	slices.Sort(leased)
	if !slices.Equal(leased, []string{"alphas", "betas"}) {
		t.Fatalf("leased = %v, want the cap to take the first two of a stable order and drop the rest", leased)
	}
}
