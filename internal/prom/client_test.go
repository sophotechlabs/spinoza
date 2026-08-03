package prom

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func service(namespace, name string, labels map[string]string, ports ...corev1.ServicePort) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: labels},
		Spec:       corev1.ServiceSpec{Ports: ports},
	}
}

func TestParseTarget(t *testing.T) {
	cases := []struct {
		spec string
		want Target
		bad  bool
	}{
		{spec: "", want: Target{}},
		{spec: "monitoring/prom:9090", want: Target{Namespace: "monitoring", Service: "prom", Port: "9090"}},
		{spec: "monitoring/prom", want: Target{Namespace: "monitoring", Service: "prom", Port: "9090"}},
		{spec: "noslash", bad: true},
		{spec: "/prom:9090", bad: true},
		{spec: "monitoring/:9090", bad: true},
		{spec: "monitoring/prom:", bad: true},
		{spec: "monitoring/prom:9090:extra", bad: true},
		{spec: "monitoring/prom:web", bad: true},
		{spec: "monitoring/prom:0", bad: true},
		{spec: "monitoring/prom:65536", bad: true},
		{spec: "monitoring/prom:65535", want: Target{Namespace: "monitoring", Service: "prom", Port: "65535"}},
	}
	for _, tc := range cases {
		t.Run(tc.spec, func(t *testing.T) {
			got, err := ParseTarget(tc.spec)
			if tc.bad {
				if err == nil {
					t.Fatalf("expected %q to be refused", tc.spec)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestWebPortPrefers9090(t *testing.T) {
	ports := []corev1.ServicePort{{Name: "reloader-web", Port: 8080}, {Name: "http-web", Port: 9090}}
	if got := webPort(ports); got != "9090" {
		t.Fatalf("webPort = %q, want 9090", got)
	}
}

func TestWebPortFallsBackToANamedWebPort(t *testing.T) {
	ports := []corev1.ServicePort{{Name: "grpc", Port: 1234}, {Name: "http-web", Port: 9999}}
	if got := webPort(ports); got != "9999" {
		t.Fatalf("webPort = %q, want 9999", got)
	}
}

func TestWebPortRejectsAServiceWithNoWebPort(t *testing.T) {
	if got := webPort([]corev1.ServicePort{{Name: "grpc", Port: 1234}}); got != "" {
		t.Fatalf("webPort = %q, want empty", got)
	}
}

func TestDiscoverPrefersTheOperatorLabel(t *testing.T) {
	cs := k8sfake.NewClientset(
		service("other", "some-prometheus", map[string]string{"app.kubernetes.io/name": "prometheus"},
			corev1.ServicePort{Name: "http-web", Port: 9090}),
		service("monitoring", "prometheus-operated", map[string]string{"operated-prometheus": "true"},
			corev1.ServicePort{Name: "http-web", Port: 9090}),
	)
	client := NewClient(cs, Target{})

	got, err := client.discover(context.Background())
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if got.Service != "prometheus-operated" {
		t.Fatalf("service = %q, want the operator-managed one", got.Service)
	}
}

func TestDiscoverFallsBackToTheChartLabel(t *testing.T) {
	cs := k8sfake.NewClientset(
		service("obs", "prom", map[string]string{"app.kubernetes.io/name": "prometheus"},
			corev1.ServicePort{Name: "http-web", Port: 9090}),
	)
	client := NewClient(cs, Target{})

	got, err := client.discover(context.Background())
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if got.Namespace != "obs" || got.Service != "prom" {
		t.Fatalf("got %+v", got)
	}
}

func TestDiscoverSkipsAServiceWithoutAWebPort(t *testing.T) {
	cs := k8sfake.NewClientset(
		service("monitoring", "prometheus-operated", map[string]string{"operated-prometheus": "true"},
			corev1.ServicePort{Name: "grpc", Port: 1234}),
	)
	client := NewClient(cs, Target{})

	_, err := client.discover(context.Background())
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !strings.Contains(err.Error(), "--prometheus") {
		t.Fatalf("message = %q, want it to name the flag", err.Error())
	}
}

func TestDiscoverHonoursTheOverride(t *testing.T) {
	client := NewClient(k8sfake.NewClientset(), Target{Namespace: "x", Service: "y", Port: "1"})

	got, err := client.discover(context.Background())
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if got.Service != "y" {
		t.Fatalf("override ignored: %+v", got)
	}
}

func TestTargetString(t *testing.T) {
	target := Target{Namespace: "monitoring", Service: "prom", Port: "9090", Scheme: "https"}
	if got := target.String(); got != "monitoring/prom:9090 (https)" {
		t.Fatalf("String = %q", got)
	}
}

const sample = `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[[1785434552,"0.028"],[1785434612,"0.031"]]}]}}`

func TestDecodeRange(t *testing.T) {
	points, err := decodeRange([]byte(sample))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("points = %d", len(points))
	}
	if points[0].At != 1785434552 || points[0].Value != 0.028 {
		t.Fatalf("first = %+v", points[0])
	}
}

func TestDecodeRangeEmptyResult(t *testing.T) {
	points, err := decodeRange([]byte(`{"status":"success","data":{"result":[]}}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(points) != 0 {
		t.Fatalf("points = %d, want none", len(points))
	}
}

func TestDecodeRangeRejectsAnError(t *testing.T) {
	_, err := decodeRange([]byte(`{"status":"error","error":"parse error"}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "parse error") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestDecodeRangeRejectsGarbage(t *testing.T) {
	_, err := decodeRange([]byte("not json"))
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestDecodeRangeSkipsMalformedPoints(t *testing.T) {
	raw := `{"status":"success","data":{"result":[{"values":[
	  [1,"1.0"], [2], ["nan","1.0"], [3,4], [5,"notanumber"], [6,"2.0"]
	]}]}}`
	points, err := decodeRange([]byte(raw))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("points = %+v, want only the two well-formed pairs", points)
	}
}

func TestParseSpan(t *testing.T) {
	got, err := ParseSpan("")
	if err != nil || got != DefaultSpan {
		t.Fatalf("empty = %v, %v", got, err)
	}
	got, err = ParseSpan("15m")
	if err != nil || got != 15*time.Minute {
		t.Fatalf("15m = %v, %v", got, err)
	}
	for _, bad := range []string{"banana", "-1h", "0s", "48h"} {
		if _, err := ParseSpan(bad); err == nil {
			t.Fatalf("%q should be refused", bad)
		}
	}
}

func TestStepForKeepsThePointCountBounded(t *testing.T) {
	if got := StepFor(time.Hour); got != 15*time.Second {
		t.Fatalf("1h step = %v, want the floor", got)
	}
	if got := StepFor(24 * time.Hour); got != 6*time.Minute {
		t.Fatalf("24h step = %v", got)
	}
	if points := int(24 * time.Hour / StepFor(24*time.Hour)); points > maxPoints {
		t.Fatalf("24h yields %d points, over the cap", points)
	}
}

type stubProxy struct {
	calls    []Target
	paths    []string
	params   []map[string]string
	failFor  map[string]bool
	body     string
	rangeErr error
}

func (s *stubProxy) Get(_ context.Context, target Target, path string, params map[string]string) ([]byte, error) {
	s.calls = append(s.calls, target)
	s.paths = append(s.paths, path)
	s.params = append(s.params, params)
	if s.failFor[target.Scheme] {
		return nil, errors.New("connection refused over " + target.Scheme)
	}
	if path == rangePath && s.rangeErr != nil {
		return nil, s.rangeErr
	}
	if path == rangePath {
		return []byte(s.body), nil
	}
	return []byte(`{"status":"success"}`), nil
}

func operatedClient(t *testing.T, proxy Proxy) *Client {
	t.Helper()
	cs := k8sfake.NewClientset(
		service("monitoring", "prometheus-operated", map[string]string{"operated-prometheus": "true"},
			corev1.ServicePort{Name: "http-web", Port: 9090}),
	)
	return NewClientWithProxy(cs, proxy, Target{})
}

func TestTargetProbesHTTPSFirst(t *testing.T) {
	proxy := &stubProxy{}
	client := operatedClient(t, proxy)

	target, err := client.Target(context.Background())
	if err != nil {
		t.Fatalf("target: %v", err)
	}
	if target.Scheme != "https" {
		t.Fatalf("scheme = %q, want https tried first", target.Scheme)
	}
	if len(proxy.calls) != 1 {
		t.Fatalf("probed %d times, want 1", len(proxy.calls))
	}
}

func TestTargetUsesTheOverrideWithoutListingServices(t *testing.T) {
	proxy := &stubProxy{}
	client := NewClientWithProxy(
		k8sfake.NewClientset(),
		proxy,
		Target{Namespace: "obs", Service: "prom", Port: "9091"},
	)

	target, err := client.Target(context.Background())
	if err != nil {
		t.Fatalf("target: %v", err)
	}
	if target.String() != "obs/prom:9091 (https)" {
		t.Fatalf("target = %q, want the override probed", target.String())
	}
}

func TestTargetFallsBackToHTTP(t *testing.T) {
	proxy := &stubProxy{failFor: map[string]bool{"https": true}}
	client := operatedClient(t, proxy)

	target, err := client.Target(context.Background())
	if err != nil {
		t.Fatalf("target: %v", err)
	}
	if target.Scheme != "http" {
		t.Fatalf("scheme = %q, want the http fallback", target.Scheme)
	}
}

func TestTargetReportsWhenNeitherSchemeAnswers(t *testing.T) {
	proxy := &stubProxy{failFor: map[string]bool{"https": true, "http": true}}
	client := operatedClient(t, proxy)

	_, err := client.Target(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestTargetIsResolvedOnce(t *testing.T) {
	proxy := &stubProxy{}
	client := operatedClient(t, proxy)

	first, _ := client.Target(context.Background())
	second, err := client.Target(context.Background())
	if err != nil {
		t.Fatalf("target: %v", err)
	}
	if first != second {
		t.Fatalf("%+v != %+v", first, second)
	}
	if len(proxy.calls) != 1 {
		t.Fatalf("probed %d times, want the result cached", len(proxy.calls))
	}
}

func TestTargetReportsADiscoveryFailure(t *testing.T) {
	client := NewClientWithProxy(k8sfake.NewClientset(), &stubProxy{}, Target{})

	_, err := client.Target(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestRangeSendsTheQueryWindow(t *testing.T) {
	proxy := &stubProxy{body: sample}
	client := operatedClient(t, proxy)
	start := time.Unix(1000, 0)
	end := time.Unix(4600, 0)

	points, err := client.Range(context.Background(), "up", start, end, 30*time.Second)
	if err != nil {
		t.Fatalf("range: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("points = %d", len(points))
	}
	sent := proxy.params[len(proxy.params)-1]
	if sent["query"] != "up" || sent["start"] != "1000" || sent["end"] != "4600" || sent["step"] != "30" {
		t.Fatalf("params = %+v", sent)
	}
}

func TestRangeSurfacesAProxyFailure(t *testing.T) {
	proxy := &stubProxy{rangeErr: errors.New("service unavailable")}
	client := operatedClient(t, proxy)

	_, err := client.Range(context.Background(), "up", time.Unix(0, 0), time.Unix(60, 0), time.Second)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestRangeReportsAnUnresolvableTarget(t *testing.T) {
	client := NewClientWithProxy(k8sfake.NewClientset(), &stubProxy{}, Target{})

	_, err := client.Range(context.Background(), "up", time.Unix(0, 0), time.Unix(60, 0), time.Second)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestPodHistoryQueriesCPUAndMemory(t *testing.T) {
	proxy := &stubProxy{body: sample}
	client := operatedClient(t, proxy)

	history, err := client.PodHistory(context.Background(), "monitoring", "loki-0", time.Hour, time.Unix(10000, 0))
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if history.Namespace != "monitoring" || history.Pod != "loki-0" {
		t.Fatalf("history = %+v", history)
	}
	if len(history.CPU) != 2 || len(history.Memory) != 2 {
		t.Fatalf("cpu %d memory %d", len(history.CPU), len(history.Memory))
	}
	if history.Source != "monitoring/prometheus-operated:9090 (https)" {
		t.Fatalf("source = %q", history.Source)
	}
	queries := []string{}
	for _, sent := range proxy.params {
		if sent != nil {
			queries = append(queries, sent["query"])
		}
	}
	if len(queries) != 2 {
		t.Fatalf("queries = %v", queries)
	}
	if !strings.Contains(queries[0], "container_cpu_usage_seconds_total") {
		t.Fatalf("first query = %q", queries[0])
	}
	if !strings.Contains(queries[1], "container_memory_working_set_bytes") {
		t.Fatalf("second query = %q", queries[1])
	}
	for _, q := range queries {
		if !strings.Contains(q, `container!=""`) {
			t.Fatalf("query %q must exclude the pod-level cgroup or it double counts", q)
		}
	}
}

func TestPodHistoryReportsAnUnresolvableTarget(t *testing.T) {
	client := NewClientWithProxy(k8sfake.NewClientset(), &stubProxy{}, Target{})

	_, err := client.PodHistory(context.Background(), "ns", "pod", time.Hour, time.Unix(0, 0))
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestPodHistorySurfacesAMemoryQueryFailure(t *testing.T) {
	proxy := &stubProxy{rangeErr: errors.New("boom")}
	client := operatedClient(t, proxy)

	_, err := client.PodHistory(context.Background(), "ns", "pod", time.Hour, time.Unix(0, 0))
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestRangeForgetsATargetThatStopsAnswering(t *testing.T) {
	proxy := &stubProxy{}
	client := operatedClient(t, proxy)
	_, err := client.Target(context.Background())
	if err != nil {
		t.Fatalf("target: %v", err)
	}

	proxy.rangeErr = errors.New("connection refused")
	_, rangeErr := client.Range(context.Background(), "up", time.Unix(0, 0), time.Unix(60, 0), time.Minute)
	if rangeErr == nil {
		t.Fatal("expected the range query to fail")
	}

	client.mu.Lock()
	cached := client.resolved
	client.mu.Unlock()
	if cached != nil {
		t.Fatalf("target %+v stayed cached after the endpoint stopped answering", cached)
	}
}

func TestRangeKeepsATargetThatAnswers(t *testing.T) {
	proxy := &stubProxy{body: `{"status":"success","data":{"result":[]}}`}
	client := operatedClient(t, proxy)
	_, err := client.Target(context.Background())
	if err != nil {
		t.Fatalf("target: %v", err)
	}

	_, rangeErr := client.Range(context.Background(), "up", time.Unix(0, 0), time.Unix(60, 0), time.Minute)
	if rangeErr != nil {
		t.Fatalf("range: %v", rangeErr)
	}

	client.mu.Lock()
	cached := client.resolved
	client.mu.Unlock()
	if cached == nil {
		t.Fatal("a working target was dropped from the cache")
	}
}

func TestForgetKeepsATargetThatWasAlreadyReplaced(t *testing.T) {
	client := &Client{resolved: &Target{Namespace: "monitoring", Service: "new", Port: "9090"}}

	client.forget(Target{Namespace: "monitoring", Service: "old", Port: "9090"})

	if client.resolved == nil {
		t.Fatal("forget dropped a target it was not asked about")
	}
}

func TestForgetIsSafeWithNothingCached(t *testing.T) {
	client := &Client{}

	client.forget(Target{Namespace: "monitoring", Service: "prom", Port: "9090"})

	if client.resolved != nil {
		t.Fatal("forget invented a target")
	}
}
