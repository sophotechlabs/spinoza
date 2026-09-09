package telemetry

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
)

func caught(t *testing.T, declare func()) string {
	t.Helper()
	message := ""
	func() {
		defer func() {
			raised := recover()
			if raised != nil {
				message = fmt.Sprint(raised)
			}
		}()
		declare()
	}()
	return message
}

func TestASeriesBeyondTheCapIsFoldedIntoOtherRatherThanGrowing(t *testing.T) {
	reg := New()
	calls := reg.Counter("spinoza_apiserver_calls_total", "calls spinoza has made to an apiserver", "outcome")
	const written = seriesLimit + 300
	for at := range written {
		calls.Inc("outcome" + strconv.Itoa(at))
	}

	if calls.vec.count() > seriesLimit+1 {
		t.Fatalf("the metric holds %d series and the cap is %d", calls.vec.count(), seriesLimit)
	}

	page := rendered(t, reg)

	folded := countAfter(t, page, `spinoza_apiserver_calls_total{outcome="other"} `)
	if folded != written-seriesLimit {
		t.Fatalf("the other series reads %d and %d writes ran past the cap", folded, written-seriesLimit)
	}
	reported := countAfter(t, page, `spinoza_telemetry_series_folded_total{metric="spinoza_apiserver_calls_total"} `)
	if reported != written-seriesLimit {
		t.Fatalf("the fold was counted %d times and happened %d times", reported, written-seriesLimit)
	}
}

func TestASeriesInsideTheCapIsNotFolded(t *testing.T) {
	reg := New()
	calls := reg.Counter("spinoza_apiserver_calls_total", "calls spinoza has made to an apiserver", "outcome")
	for at := range seriesLimit {
		calls.Inc("outcome" + strconv.Itoa(at))
	}

	page := rendered(t, reg)

	if strings.Contains(page, `spinoza_apiserver_calls_total{outcome="other"}`) {
		t.Fatal("a metric that stayed inside the cap folded a series anyway")
	}
	if strings.Contains(page, "spinoza_telemetry_series_folded_total") {
		t.Fatalf("a fold was reported and none happened:\n%s", page)
	}
}

func TestAHistogramBeyondTheCapFoldsIntoOtherAsWell(t *testing.T) {
	reg := New()
	took := reg.Histogram("spinoza_http_request_duration_seconds", "how long spinoza took to answer a request", []float64{1}, "route")
	for at := range seriesLimit + 10 {
		took.Observe(0.5, "/api/route"+strconv.Itoa(at))
	}

	page := rendered(t, reg)

	if countAfter(t, page, `spinoza_http_request_duration_seconds_count{route="other"} `) != 10 {
		t.Fatalf("the folded histogram did not take the ten writes past the cap:\n%s", page)
	}
}

func TestAWriteWithTheWrongNumberOfLabelValuesIsCountedRatherThanStored(t *testing.T) {
	cases := []struct {
		name   string
		values []string
	}{
		{name: "no values at all", values: nil},
		{name: "one value short", values: []string{"/api/x", "GET"}},
		{name: "one value too many", values: []string{"/api/x", "GET", "200", "extra"}},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			reg := New()
			requests := reg.Counter("spinoza_http_requests_total", "requests spinoza has answered", "route", "method", "status")
			requests.Inc(one.values...)

			page := rendered(t, reg)

			if strings.Contains(page, "spinoza_http_requests_total{") {
				t.Fatalf("a write with the wrong labels was stored anyway:\n%s", page)
			}
			reported := countAfter(t, page, `spinoza_telemetry_writes_rejected_total{metric="spinoza_http_requests_total"} `)
			if reported != 1 {
				t.Fatalf("the refusal was counted %d times", reported)
			}
		})
	}
}

func TestAWriteThatMakesNoSenseForItsKindIsCountedRatherThanStored(t *testing.T) {
	reg := New()
	failures := reg.Counter("spinoza_audit_write_failures_total", "audit rows spinoza could not write")
	failures.Add(-3)
	took := reg.Histogram("spinoza_check_run_duration_seconds", "how long a run of the checks took", []float64{1})
	took.Observe(math.NaN())

	page := rendered(t, reg)

	for _, name := range []string{"spinoza_audit_write_failures_total", "spinoza_check_run_duration_seconds"} {
		reported := countAfter(t, page, `spinoza_telemetry_writes_rejected_total{metric="`+name+`"} `)
		if reported != 1 {
			t.Fatalf("%s counted %d refusals and one was made", name, reported)
		}
	}
	if countAfter(t, page, "spinoza_check_run_duration_seconds_count ") != 0 {
		t.Fatalf("a refused observation was still counted:\n%s", page)
	}
}

func TestTheRegistrysOwnBookkeepingDoesNotReportOnItself(t *testing.T) {
	reg := New()

	for at := range seriesLimit + 5 {
		reg.folded.Inc("metric" + strconv.Itoa(at))
		reg.rejected.Inc("metric" + strconv.Itoa(at))
	}
	reg.folded.Inc()
	reg.rejected.Inc()

	page := rendered(t, reg)

	if strings.Contains(page, `spinoza_telemetry_series_folded_total{metric="spinoza_telemetry_`) {
		t.Fatalf("the bookkeeping counters reported on themselves:\n%s", page)
	}
	if reg.folded.vec.count() > seriesLimit+1 {
		t.Fatalf("the fold counter holds %d series and its own cap is %d", reg.folded.vec.count(), seriesLimit)
	}
	if countAfter(t, page, `spinoza_telemetry_series_folded_total{metric="other"} `) != 5 {
		t.Fatalf("the fold counter did not keep the five writes past its own cap:\n%s", page)
	}
}

func TestAMetricDeclaredWithoutTheHouseNameIsRefused(t *testing.T) {
	cases := []struct {
		name    string
		declare func(reg *Registry)
		want    string
	}{
		{
			name:    "no spinoza prefix",
			declare: func(reg *Registry) { reg.Counter("http_requests_total", "requests") },
			want:    "does not start with spinoza_",
		},
		{
			name:    "the prefix and nothing else",
			declare: func(reg *Registry) { reg.Counter("spinoza_", "requests") },
			want:    "does not start with spinoza_",
		},
		{
			name:    "no name at all",
			declare: func(reg *Registry) { reg.Counter("", "requests") },
			want:    "is not a usable metric name",
		},
		{
			name:    "a name the format cannot carry",
			declare: func(reg *Registry) { reg.Counter("spinoza-requests", "requests") },
			want:    "is not a usable metric name",
		},
		{
			name:    "a label the format cannot carry",
			declare: func(reg *Registry) { reg.Counter("spinoza_requests_total", "requests", "the route") },
			want:    "is not a usable label name",
		},
		{
			name:    "a label that starts with a digit",
			declare: func(reg *Registry) { reg.Counter("spinoza_requests_total", "requests", "9route") },
			want:    "is not a usable label name",
		},
		{
			name:    "a label named le",
			declare: func(reg *Registry) { reg.Counter("spinoza_requests_total", "requests", "le") },
			want:    "le is reserved",
		},
		{
			name:    "one label twice",
			declare: func(reg *Registry) { reg.Counter("spinoza_requests_total", "requests", "route", "route") },
			want:    "is declared twice on one metric",
		},
		{
			name: "one name twice",
			declare: func(reg *Registry) {
				reg.Counter("spinoza_requests_total", "requests")
				reg.Gauge("spinoza_requests_total", "requests")
			},
			want: "is declared twice",
		},
		{
			name:    "a histogram with no buckets",
			declare: func(reg *Registry) { reg.Histogram("spinoza_requests_seconds", "how long", nil) },
			want:    "needs at least one bucket",
		},
		{
			name: "a bucket bound that is not a number",
			declare: func(reg *Registry) {
				reg.Histogram("spinoza_requests_seconds", "how long", []float64{math.NaN()})
			},
			want: "has to be a finite number",
		},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			reg := New()

			message := caught(t, func() { one.declare(reg) })

			if !strings.Contains(message, one.want) {
				t.Fatalf("the refusal said %q and should have said %q", message, one.want)
			}
		})
	}
}

func TestAMetricDeclaredProperlyIsAccepted(t *testing.T) {
	reg := New()

	message := caught(t, func() {
		reg.Counter("spinoza_http_requests_total", "requests spinoza has answered", "route", "method", "status")
		reg.Gauge("spinoza_sessions", "signed-in sessions right now", "role")
		reg.Histogram("spinoza_http_request_duration_seconds", "how long", []float64{0.5, 1}, "route")
		reg.Counter("spinoza_Odd_Name_9_total", "a name the format still carries", "Route9")
	})

	if message != "" {
		t.Fatalf("a well-formed declaration was refused: %s", message)
	}
}

func TestOnlyAHistogramCarriesBuckets(t *testing.T) {
	reg := New()

	message := caught(t, func() {
		reg.declare("spinoza_sessions", "signed-in sessions right now", gaugeKind, []float64{1}, nil)
	})

	if !strings.Contains(message, "only a histogram has buckets") {
		t.Fatalf("the refusal said %q", message)
	}
}

func TestAHistogramObservedWithTheWrongLabelsKeepsNoSeries(t *testing.T) {
	reg := New()
	took := reg.Histogram("spinoza_http_request_duration_seconds", "how long spinoza took to answer a request", []float64{0.5, 1}, "route")

	took.Observe(0.25)
	took.Observe(0.25, "/api/version", "extra")

	page := rendered(t, reg)

	if strings.Contains(page, "spinoza_http_request_duration_seconds_bucket{") {
		t.Fatalf("an observation with the wrong labels was stored anyway:\n%s", page)
	}
	if countAfter(t, page, `spinoza_telemetry_writes_rejected_total{metric="spinoza_http_request_duration_seconds"} `) != 2 {
		t.Fatalf("the two refusals were not both counted:\n%s", page)
	}
}

func TestAGaugeWrittenWithTheWrongLabelsKeepsNoSeries(t *testing.T) {
	cases := []struct {
		name  string
		write func(gauge *Gauge)
	}{
		{name: "set with no label value", write: func(gauge *Gauge) { gauge.Set(3) }},
		{name: "set with two label values", write: func(gauge *Gauge) { gauge.Set(3, "admin", "extra") }},
		{name: "add with no label value", write: func(gauge *Gauge) { gauge.Add(3) }},
		{name: "increment with no label value", write: func(gauge *Gauge) { gauge.Inc() }},
		{name: "decrement with no label value", write: func(gauge *Gauge) { gauge.Dec() }},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			reg := New()
			sessions := reg.Gauge("spinoza_sessions", "signed-in sessions right now", "role")

			one.write(sessions)

			page := rendered(t, reg)

			if strings.Contains(page, "spinoza_sessions{") {
				t.Fatalf("a write with the wrong labels was stored anyway:\n%s", page)
			}
			if countAfter(t, page, `spinoza_telemetry_writes_rejected_total{metric="spinoza_sessions"} `) != 1 {
				t.Fatalf("the refusal was not counted:\n%s", page)
			}
		})
	}
}
