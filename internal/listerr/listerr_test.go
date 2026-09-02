package listerr

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func TestNoFailuresLeavesNoMessage(t *testing.T) {
	collector := New()
	collector.Record("pods", nil)
	collector.Record("deployments", nil)

	if got := collector.Message(); got != "" {
		t.Fatalf("message = %q, want empty", got)
	}
}

func TestOneFailureNamesTheResourceAndTheReason(t *testing.T) {
	collector := New()
	collector.Record("pods", nil)
	collector.Record("kustomizations", errors.New("is forbidden"))

	got := collector.Message()

	if !strings.HasPrefix(got, "1 of 2 resource types could not be listed") {
		t.Fatalf("message = %q", got)
	}
	if !strings.Contains(got, "kustomizations (is forbidden)") {
		t.Fatalf("message = %q, want the reason", got)
	}
}

func TestFailuresAreListedInAStableOrder(t *testing.T) {
	collector := New()
	collector.Record("zebras", errors.New("late"))
	collector.Record("alpacas", errors.New("early"))

	got := collector.Message()

	if strings.Index(got, "alpacas") > strings.Index(got, "zebras") {
		t.Fatalf("message = %q, want a stable alphabetical order", got)
	}
}

func TestTheSameResourceIsNotCountedTwice(t *testing.T) {
	collector := New()
	collector.Record("pods", errors.New("boom"))
	collector.Record("pods", errors.New("boom"))

	if !strings.HasPrefix(collector.Message(), "1 of 1 resource types could not be listed") {
		t.Fatalf("message = %q", collector.Message())
	}
}

func TestAListingFailureCannotMakeTheBannerUnbounded(t *testing.T) {
	collector := New()
	collector.Record("pods", errors.New(strings.Repeat("x", 1<<20)))

	if got := collector.Message(); len(got) > 512 {
		t.Fatalf("message is %d bytes, want a bounded banner", len(got))
	}
}

func TestConcurrentRecordsAreSafe(t *testing.T) {
	collector := New()
	var group sync.WaitGroup
	for i := range 50 {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			if index%2 == 0 {
				collector.Record("even", errors.New("boom"))
				return
			}
			collector.Record("odd", nil)
		}(i)
	}
	group.Wait()

	if !strings.Contains(collector.Message(), "even") {
		t.Fatalf("message = %q", collector.Message())
	}
}

func TestALongListOfFailuresIsTrimmed(t *testing.T) {
	collector := New()
	for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
		collector.Record(name, errors.New("is forbidden"))
	}

	got := collector.Message()

	if !strings.HasPrefix(got, "6 of 6 resource types could not be listed") {
		t.Fatalf("message = %q", got)
	}
	if !strings.HasSuffix(got, "and 3 more") {
		t.Fatalf("message = %q, want the tail shortened", got)
	}
	if strings.Contains(got, "f (") {
		t.Fatalf("message = %q, want only the first few named", got)
	}
	if len(got) > 240 {
		t.Fatalf("message is %d chars; it has to fit in a banner", len(got))
	}
}

func TestExactlyThreeFailuresAreAllNamed(t *testing.T) {
	collector := New()
	for _, name := range []string{"a", "b", "c"} {
		collector.Record(name, errors.New("nope"))
	}

	got := collector.Message()

	if strings.Contains(got, "more") {
		t.Fatalf("message = %q, want no tail when everything fits", got)
	}
}

func TestTheZeroValueCanRecordAFailure(t *testing.T) {
	var collector Collector

	collector.Record("pods", errors.New("is forbidden"))

	if got := collector.Message(); !strings.Contains(got, "pods (is forbidden)") {
		t.Fatalf("message = %q, want the recorded failure", got)
	}
}

func TestANilPanicLeavesNoFailure(t *testing.T) {
	collector := New()

	collector.RecordPanic("pods", "reading pods", nil)

	if got := collector.Message(); got != "" {
		t.Fatalf("message = %q, want no failure", got)
	}
}

func TestAPanicIsRecordedAsAListingFailure(t *testing.T) {
	collector := New()

	collector.RecordPanic("pods", "reading pods", "boom")

	if got := collector.Message(); !strings.Contains(got, "pods") {
		t.Fatalf("message = %q, want the panicking resource", got)
	}
}

func TestALongFailureIsShortenedAtACharacterBoundary(t *testing.T) {
	message := strings.Repeat("a", maxFailureSize-4) + "€" + "tail"

	got := shorten(message)

	if !utf8.ValidString(got) {
		t.Fatalf("shortened message is not valid UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("shortened message = %q, want an ellipsis", got)
	}
}
