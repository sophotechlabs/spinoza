package waste

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type fakeSampler struct {
	metrics api.Metrics
}

func (fs fakeSampler) Metrics(context.Context) api.Metrics {
	return fs.metrics
}

type fakeHistorian struct {
	mu    sync.Mutex
	byPod map[string]api.MetricHistory
	err   error
	spans []time.Duration
}

func (fh *fakeHistorian) asked() []time.Duration {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	return append([]time.Duration{}, fh.spans...)
}

func (fh *fakeHistorian) MetricHistory(
	_ context.Context,
	namespace, pod string,
	span time.Duration,
) (api.MetricHistory, error) {
	fh.mu.Lock()
	fh.spans = append(fh.spans, span)
	fh.mu.Unlock()
	if fh.err != nil {
		return api.MetricHistory{}, fh.err
	}
	history, ok := fh.byPod[namespace+"/"+pod]
	if !ok {
		return api.MetricHistory{}, errors.New("no history for " + namespace + "/" + pod)
	}
	return history, nil
}

func pointsOf(values ...float64) []api.MetricPoint {
	out := make([]api.MetricPoint, 0, len(values))
	for at, value := range values {
		out = append(out, api.MetricPoint{At: int64(at), Value: value})
	}
	return out
}

func TestTheLiveMeterNamesMetricsServerAndTheMomentItRead(t *testing.T) {
	meter := RightNow(fakeSampler{metrics: api.Metrics{Pods: map[string]api.ResourceUsage{
		"shop/web-1": {CPUMilli: 120, MemoryMi: 256},
	}}})

	reading, err := meter.Usage(t.Context(), []Ref{{Namespace: "shop", Name: "web-1"}})
	if err != nil {
		t.Fatalf("usage: %v", err)
	}

	if reading.Source != liveSource || reading.Window != liveWindow {
		t.Errorf("source = %q, window = %q, want the live reading named", reading.Source, reading.Window)
	}
	if reading.Pods["shop/web-1"].CPUMilli != 120 {
		t.Errorf("usage = %+v, want what metrics-server said", reading.Pods["shop/web-1"])
	}
}

func TestTheLiveMeterRefusesRatherThanAnswerFromAFailedRead(t *testing.T) {
	for _, one := range []struct {
		name    string
		metrics api.Metrics
		want    string
	}{
		{
			name:    "metrics-server reported an error",
			metrics: api.Metrics{Error: "pods.metrics.k8s.io: the server could not find the requested resource"},
			want:    "could not find",
		},
		{
			name:    "metrics-server knew none of the pods",
			metrics: api.Metrics{Pods: map[string]api.ResourceUsage{"other/thing": {CPUMilli: 1}}},
			want:    "named none of the pods",
		},
	} {
		t.Run(one.name, func(t *testing.T) {
			meter := RightNow(fakeSampler{metrics: one.metrics})

			_, err := meter.Usage(t.Context(), []Ref{{Namespace: "shop", Name: "web-1"}})

			if err == nil {
				t.Fatal("err = nil, want the read refused so another source is tried")
			}
			if !strings.Contains(err.Error(), one.want) {
				t.Errorf("err = %q, want it to contain %q", err, one.want)
			}
		})
	}
}

func TestTheWindowMeterAveragesTheHistoryOfEachPod(t *testing.T) {
	historian := &fakeHistorian{byPod: map[string]api.MetricHistory{
		"shop/web-1": {
			Source: "monitoring/prometheus:9090 (https)",
			CPU:    pointsOf(0.1, 0.3),
			Memory: pointsOf(100*bytesPerMi, 300*bytesPerMi),
		},
	}}
	meter := OverAWindow(historian, 30*time.Minute)

	reading, err := meter.Usage(t.Context(), []Ref{{Namespace: "shop", Name: "web-1"}})
	if err != nil {
		t.Fatalf("usage: %v", err)
	}

	use := reading.Pods["shop/web-1"]
	if use.CPUMilli != 200 {
		t.Errorf("cpu = %d millicores, want the mean of the window in millicores", use.CPUMilli)
	}
	if use.MemoryMi != 200 {
		t.Errorf("memory = %d mebibytes, want the mean of the window in mebibytes", use.MemoryMi)
	}
	if reading.Window != "the last 30 minutes" {
		t.Errorf("window = %q, want it said in words", reading.Window)
	}
	if reading.Source != "monitoring/prometheus:9090 (https)" {
		t.Errorf("source = %q, want where the history came from", reading.Source)
	}
	asked := historian.asked()
	if len(asked) != 1 || asked[0] != 30*time.Minute {
		t.Errorf("spans = %v, want the span it was built with", asked)
	}
}

func TestTheWindowMeterNamesWhereTheHistoryCameFrom(t *testing.T) {
	for _, one := range []struct {
		name    string
		history api.MetricHistory
		want    string
	}{
		{name: "spinoza recorded it", history: api.MetricHistory{Sampled: true}, want: sampledSource},
		{name: "prometheus named a target", history: api.MetricHistory{Source: "monitoring/prom:9090"}, want: "monitoring/prom:9090"},
		{name: "nothing named itself", history: api.MetricHistory{}, want: promSource},
	} {
		t.Run(one.name, func(t *testing.T) {
			one.history.CPU = pointsOf(0.5)

			historian := &fakeHistorian{byPod: map[string]api.MetricHistory{"shop/web-1": one.history}}
			reading, err := OverAWindow(historian, time.Hour).Usage(t.Context(), []Ref{{Namespace: "shop", Name: "web-1"}})
			if err != nil {
				t.Fatalf("usage: %v", err)
			}

			if reading.Source != one.want {
				t.Errorf("source = %q, want %q", reading.Source, one.want)
			}
		})
	}
}

func TestTheWindowMeterDeclinesMorePodsThanItWillRead(t *testing.T) {
	historian := &fakeHistorian{byPod: map[string]api.MetricHistory{}}
	pods := make([]Ref, 0, windowPodBudget+1)
	for at := range windowPodBudget + 1 {
		pods = append(pods, Ref{Namespace: "shop", Name: "web-" + string(rune('a'+at%26))})
	}

	_, err := OverAWindow(historian, time.Hour).Usage(t.Context(), pods)

	if err == nil {
		t.Fatal("err = nil, want the window to decline rather than read one pod at a time")
	}
	if reads := len(historian.asked()); reads != 0 {
		t.Errorf("reads = %d, want none attempted", reads)
	}
	if !strings.Contains(err.Error(), "ask about one namespace") {
		t.Errorf("err = %q, want it to say what to do instead", err)
	}
}

func TestAPodWithNoHistoryIsLeftUnmeasuredRatherThanReadAsZero(t *testing.T) {
	historian := &fakeHistorian{byPod: map[string]api.MetricHistory{
		"shop/web-1": {CPU: pointsOf(0.5)},
		"shop/web-2": {},
	}}

	reading, err := OverAWindow(historian, time.Hour).Usage(t.Context(), []Ref{
		{Namespace: "shop", Name: "web-1"},
		{Namespace: "shop", Name: "web-2"},
	})
	if err != nil {
		t.Fatalf("usage: %v", err)
	}

	if _, seen := reading.Pods["shop/web-2"]; seen {
		t.Errorf("pods = %+v, want the pod with no history left out rather than read as zero", reading.Pods)
	}
	if reading.Pods["shop/web-1"].CPUMilli != 500 {
		t.Errorf("web-1 = %+v, want the pod that had a history measured", reading.Pods["shop/web-1"])
	}
}

func TestAWindowNoPodAnsweredIsRefusedSoAnotherSourceIsTried(t *testing.T) {
	historian := &fakeHistorian{err: errors.New("prometheus is unavailable")}

	_, err := OverAWindow(historian, time.Hour).Usage(t.Context(), []Ref{{Namespace: "shop", Name: "web-1"}})

	if err == nil {
		t.Fatal("err = nil, want the window refused so another source is tried")
	}
	if !strings.Contains(err.Error(), "prometheus is unavailable") {
		t.Errorf("err = %q, want the reason prometheus gave", err)
	}
}

func TestTheWindowIsSaidInWords(t *testing.T) {
	for _, one := range []struct {
		span time.Duration
		want string
	}{
		{span: 0, want: "right now"},
		{span: 45 * time.Second, want: "the last 45 seconds"},
		{span: time.Minute, want: "the last minute"},
		{span: 30 * time.Minute, want: "the last 30 minutes"},
		{span: time.Hour, want: "the last hour"},
		{span: 6 * time.Hour, want: "the last 6 hours"},
		{span: 24 * time.Hour, want: "the last day"},
		{span: 72 * time.Hour, want: "the last 3 days"},
	} {
		t.Run(one.want, func(t *testing.T) {
			got := windowWords(one.span)
			if got != one.want {
				t.Errorf("windowWords(%s) = %q, want %q", one.span, got, one.want)
			}
		})
	}
}
