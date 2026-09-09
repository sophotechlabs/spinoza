package telemetry

import (
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func countAfter(t *testing.T, page, prefix string) int {
	t.Helper()
	for line := range strings.SplitSeq(page, "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		value := strings.TrimPrefix(line, prefix)
		count, err := strconv.Atoi(value)
		if err != nil {
			t.Fatalf("the line %q does not end in a count: %v", line, err)
		}
		return count
	}
	t.Fatalf("no line starting %q in\n%s", prefix, page)
	return 0
}

func TestAHistogramCountsEveryObservationIntoTheBucketsAboveIt(t *testing.T) {
	cases := []struct {
		name    string
		seen    []float64
		buckets []string
		sum     string
		count   string
	}{
		{
			name:    "nothing observed yet",
			seen:    nil,
			buckets: []string{"0", "0", "0", "0"},
			sum:     "0",
			count:   "0",
		},
		{
			name:    "one observation in the first bucket",
			seen:    []float64{0.1},
			buckets: []string{"1", "1", "1", "1"},
			sum:     "0.1",
			count:   "1",
		},
		{
			name:    "one observation past the last bound",
			seen:    []float64{9},
			buckets: []string{"0", "0", "0", "1"},
			sum:     "9",
			count:   "1",
		},
		{
			name:    "an observation exactly on a bound belongs to it",
			seen:    []float64{1},
			buckets: []string{"0", "1", "1", "1"},
			sum:     "1",
			count:   "1",
		},
		{
			name:    "one in each bucket",
			seen:    []float64{0.25, 0.75, 3, 40},
			buckets: []string{"1", "2", "3", "4"},
			sum:     "44",
			count:   "4",
		},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			reg := New()
			took := reg.Histogram("spinoza_check_run_duration_seconds", "how long a run of the checks took", []float64{0.5, 1, 5})
			for _, value := range one.seen {
				took.Observe(value)
			}

			page := rendered(t, reg)

			want := []string{
				`spinoza_check_run_duration_seconds_bucket{le="0.5"} ` + one.buckets[0],
				`spinoza_check_run_duration_seconds_bucket{le="1"} ` + one.buckets[1],
				`spinoza_check_run_duration_seconds_bucket{le="5"} ` + one.buckets[2],
				`spinoza_check_run_duration_seconds_bucket{le="+Inf"} ` + one.buckets[3],
				"spinoza_check_run_duration_seconds_sum " + one.sum,
				"spinoza_check_run_duration_seconds_count " + one.count,
			}
			for _, line := range want {
				if !strings.Contains(page, line+"\n") {
					t.Fatalf("the page reads\n%s\nand should carry\n%s", page, line)
				}
			}
		})
	}
}

func TestAHistogramBucketNeverReadsLowerThanTheOneBelowIt(t *testing.T) {
	reg := New()
	took := reg.Histogram("spinoza_check_run_duration_seconds", "how long a run of the checks took", []float64{0.5, 1, 5})
	for _, value := range []float64{0.1, 0.2, 0.7, 2, 3, 40, 41} {
		took.Observe(value)
	}

	page := rendered(t, reg)

	running := 0
	for _, bound := range []string{"0.5", "1", "5", "+Inf"} {
		count := countAfter(t, page, `spinoza_check_run_duration_seconds_bucket{le="`+bound+`"} `)
		if count < running {
			t.Fatalf("bucket %s reads %d after %d, so the buckets are not cumulative", bound, count, running)
		}
		running = count
	}
	if running != 7 {
		t.Fatalf("the +Inf bucket reads %d and seven observations were made", running)
	}
	if countAfter(t, page, "spinoza_check_run_duration_seconds_count ") != 7 {
		t.Fatalf("the count disagrees with the +Inf bucket:\n%s", page)
	}
}

func TestTheBucketsOfAHistogramComeOutInAscendingOrderEndingAtInf(t *testing.T) {
	reg := New()
	reg.Histogram("spinoza_informer_sync_seconds", "how long an informer took to sync", []float64{5, 0.5, 1, 5})

	page := rendered(t, reg)

	want := []string{`le="0.5"`, `le="1"`, `le="5"`, `le="+Inf"`}
	at := 0
	for _, bound := range want {
		found := strings.Index(page[at:], bound)
		if found < 0 {
			t.Fatalf("the buckets came out as\n%s\nand %s is missing or out of order", page, bound)
		}
		at += found
	}
	if strings.Count(page, "_bucket{") != len(want) {
		t.Fatalf("a repeated bound was declared twice:\n%s", page)
	}
}

func TestManyGoroutinesIncrementingOneCounterLoseNothing(t *testing.T) {
	reg := New()
	calls := reg.Counter("spinoza_apiserver_calls_total", "calls spinoza has made to an apiserver", "outcome")
	const writers = 64
	const each = 200

	running := sync.WaitGroup{}
	running.Add(writers)
	for range writers {
		go func() {
			defer running.Done()
			for range each {
				calls.Inc("ok")
			}
		}()
	}
	running.Wait()

	want := "spinoza_apiserver_calls_total{outcome=\"ok\"} " + strconv.Itoa(writers*each)
	if !strings.Contains(rendered(t, reg), want+"\n") {
		t.Fatalf("the counter should read %s and reads\n%s", want, rendered(t, reg))
	}
}

func TestTheRegistryCanBeReadWhileItIsBeingWrittenTo(t *testing.T) {
	reg := New()
	calls := reg.Counter("spinoza_apiserver_calls_total", "calls spinoza has made to an apiserver", "outcome")
	took := reg.Histogram("spinoza_check_run_duration_seconds", "how long a run of the checks took", []float64{0.5, 1, 5})
	sessions := reg.Gauge("spinoza_sessions", "signed-in sessions right now", "role")

	running := sync.WaitGroup{}
	running.Add(3)
	go func() {
		defer running.Done()
		for at := range 500 {
			calls.Inc("outcome" + strconv.Itoa(at%7))
		}
	}()
	go func() {
		defer running.Done()
		for at := range 500 {
			took.Observe(float64(at) / 100)
			sessions.Set(float64(at), "admin")
		}
	}()
	go func() {
		defer running.Done()
		for range 200 {
			err := reg.Write(io.Discard)
			if err != nil {
				t.Errorf("write the registry: %v", err)
				return
			}
		}
	}()
	running.Wait()

	if !strings.Contains(rendered(t, reg), "spinoza_check_run_duration_seconds_count 500\n") {
		t.Fatal("the histogram lost an observation made while the page was being read")
	}
}

func TestACounterAskedToGoBackwardsKeepsItsValueAndSaysSo(t *testing.T) {
	cases := []struct {
		name  string
		delta float64
		want  string
	}{
		{name: "a negative step", delta: -1, want: "1"},
		{name: "a positive step", delta: 2, want: "3"},
		{name: "no step at all", delta: 0, want: "1"},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			reg := New()
			failures := reg.Counter("spinoza_audit_write_failures_total", "audit rows spinoza could not write")
			failures.Inc()
			failures.Add(one.delta)

			page := rendered(t, reg)

			if !strings.Contains(page, "spinoza_audit_write_failures_total "+one.want+"\n") {
				t.Fatalf("the counter should read %s and the page reads\n%s", one.want, page)
			}
		})
	}
}

func TestAGaugeCarriesTheLastValueItWasGiven(t *testing.T) {
	reg := New()
	open := reg.Gauge("spinoza_websocket_sessions", "websocket sessions open right now")
	open.Inc()
	open.Inc()
	open.Dec()
	open.Add(4)

	if !strings.Contains(rendered(t, reg), "spinoza_websocket_sessions 5\n") {
		t.Fatalf("the gauge reads\n%s", rendered(t, reg))
	}

	open.Set(0)

	if !strings.Contains(rendered(t, reg), "spinoza_websocket_sessions 0\n") {
		t.Fatalf("a gauge set back to zero reads\n%s", rendered(t, reg))
	}
}
