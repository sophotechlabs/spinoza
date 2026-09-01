package checks

import (
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type clock struct {
	at time.Time
}

func (c *clock) now() time.Time {
	return c.at
}

func cachedRun(t *testing.T) (*Surveys, *fakeLister, *clock) {
	t.Helper()
	held := &clock{at: time.Unix(1700000000, 0)}
	lister := newLister(
		deployment("api", hostileWorkload("api")),
		pod("standalone", podSpec(container("app", nil))),
	)
	return NewSurveys(held.now), lister, held
}

func TestASecondReadInsideTheWindowDoesNotWalkTheCacheAgain(t *testing.T) {
	surveys, lister, _ := cachedRun(t)

	surveys.Run(t.Context(), lister, descriptors(), api.Metrics{}, wholeCluster(), 0)
	after := lister.listCount()
	if after == 0 {
		t.Fatal("the first audit read nothing, so this proves nothing")
	}

	surveys.Run(t.Context(), lister, descriptors(), api.Metrics{}, wholeCluster(), 0)

	if lister.listCount() != after {
		t.Fatalf("the second audit read %d more kinds", lister.listCount()-after)
	}
}

func TestPagingAFindingReusesTheSurveyTheReportBuilt(t *testing.T) {
	surveys, lister, _ := cachedRun(t)
	report := surveys.Run(t.Context(), lister, descriptors(), api.Metrics{}, wholeCluster(), 0)
	if len(report.Groups) == 0 {
		t.Fatal("the audit found no group to page")
	}
	after := lister.listCount()

	_, err := surveys.Page(
		t.Context(), lister, descriptors(), api.Metrics{},
		report.Groups[0].ID, "", wholeCluster(), 0,
	)
	if err != nil {
		t.Fatalf("page: %v", err)
	}

	if lister.listCount() != after {
		t.Fatalf("paging read %d more kinds", lister.listCount()-after)
	}
}

func TestOnceTheWindowPassesTheClusterIsReadAgain(t *testing.T) {
	surveys, lister, held := cachedRun(t)
	surveys.Run(t.Context(), lister, descriptors(), api.Metrics{}, wholeCluster(), 0)
	after := lister.listCount()

	held.at = held.at.Add(surveyTTL)
	surveys.Run(t.Context(), lister, descriptors(), api.Metrics{}, wholeCluster(), 0)

	if lister.listCount() == after {
		t.Fatal("the audit went on answering from a survey older than its window")
	}
}

func TestADifferentFilterGetsItsOwnSurvey(t *testing.T) {
	surveys, lister, _ := cachedRun(t)
	surveys.Run(t.Context(), lister, descriptors(), api.Metrics{}, wholeCluster(), 0)
	after := lister.listCount()

	surveys.Run(t.Context(), lister, descriptors(), api.Metrics{}, everyKind(), 0)

	if lister.listCount() == after {
		t.Fatal("a different filter answered from the survey built for another one")
	}
}

func TestNoCacheAtAllStillWorks(t *testing.T) {
	_, lister, _ := cachedRun(t)

	report := Run(t.Context(), lister, descriptors(), api.Metrics{}, wholeCluster(), 0)

	if len(report.Groups) == 0 {
		t.Fatal("the uncached path found nothing")
	}
}

func TestASurveyOlderThanTheWindowIsDroppedRatherThanHeld(t *testing.T) {
	surveys, lister, held := cachedRun(t)
	surveys.Run(t.Context(), lister, descriptors(), api.Metrics{}, wholeCluster(), 0)

	held.at = held.at.Add(surveyTTL)
	surveys.Run(t.Context(), lister, descriptors(), api.Metrics{}, everyKind(), 0)

	surveys.mu.Lock()
	kept := len(surveys.held)
	surveys.mu.Unlock()
	if kept != 1 {
		t.Fatalf("the cache holds %d surveys, want the stale one dropped", kept)
	}
}

func TestASurveyWithoutMetricsIsNotReusedOnceMetricsArrive(t *testing.T) {
	surveys, lister, _ := cachedRun(t)
	surveys.Run(t.Context(), lister, descriptors(), api.Metrics{}, wholeCluster(), 0)
	after := lister.listCount()

	withUsage := api.Metrics{Pods: map[string]api.ResourceUsage{
		testNamespace + "/api": {CPUMilli: 5, MemoryMi: 20},
	}}
	surveys.Run(t.Context(), lister, descriptors(), withUsage, wholeCluster(), 0)

	if lister.listCount() == after {
		t.Fatal("the survey without metrics was reused after metrics arrived")
	}
}

func TestASurveyWithMetricsIsReusedForTheSameMetrics(t *testing.T) {
	surveys, lister, _ := cachedRun(t)
	withUsage := api.Metrics{Pods: map[string]api.ResourceUsage{
		testNamespace + "/api": {CPUMilli: 5, MemoryMi: 20},
	}}
	surveys.Run(t.Context(), lister, descriptors(), withUsage, wholeCluster(), 0)
	after := lister.listCount()

	surveys.Run(t.Context(), lister, descriptors(), withUsage, wholeCluster(), 0)

	if lister.listCount() != after {
		t.Fatalf("the same metrics caused %d more kinds to be read", lister.listCount()-after)
	}
}

func TestASurveyWithFailedMetricsIsNotReusedOnceTheyRead(t *testing.T) {
	surveys, lister, _ := cachedRun(t)
	surveys.Run(t.Context(), lister, descriptors(), api.Metrics{Error: "metrics unavailable"}, wholeCluster(), 0)
	after := lister.listCount()

	surveys.Run(t.Context(), lister, descriptors(), api.Metrics{}, wholeCluster(), 0)

	if lister.listCount() == after {
		t.Fatal("the survey with failed metrics was reused after metrics recovered")
	}
}
