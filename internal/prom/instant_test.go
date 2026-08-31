package prom

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

type instantProxy struct {
	body     string
	err      error
	paths    []string
	params   []map[string]string
	askedFor int
}

func (p *instantProxy) Get(_ context.Context, _ Target, path string, params map[string]string) ([]byte, error) {
	p.paths = append(p.paths, path)
	p.params = append(p.params, params)
	if path != instantPath {
		return []byte(`{"status":"success"}`), nil
	}
	p.askedFor++
	if p.err != nil {
		return nil, p.err
	}
	return []byte(p.body), nil
}

func instantClient(t *testing.T, proxy *instantProxy) *Client {
	t.Helper()
	return operatedClient(t, proxy)
}

func TestInstantKeepsLabels(t *testing.T) {
	proxy := &instantProxy{body: `{"status":"success","data":{"resultType":"vector","result":[
		{"metric":{"source_workload":"web","verdict":"FORWARDED"},"value":[1787933018.510,"12.5"]},
		{"metric":{"source_workload":"beat","verdict":"DROPPED"},"value":[1787933018.510,"0.5"]}
	]}}`}
	client := instantClient(t, proxy)

	samples, err := client.Instant(context.Background(), `up`, time.Unix(1787933018, 0))
	if err != nil {
		t.Fatalf("instant: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("got %d samples, want 2", len(samples))
	}
	if samples[0].Labels["source_workload"] != "web" {
		t.Fatalf("first sample lost its labels: %+v", samples[0])
	}
	if samples[0].Value != 12.5 {
		t.Fatalf("first sample value is %v, want 12.5", samples[0].Value)
	}
	if samples[1].Labels["verdict"] != "DROPPED" {
		t.Fatalf("second sample lost its verdict: %+v", samples[1])
	}
}

func TestInstantSendsQueryAndTime(t *testing.T) {
	proxy := &instantProxy{body: `{"status":"success","data":{"resultType":"vector","result":[]}}`}
	client := instantClient(t, proxy)

	_, err := client.Instant(context.Background(), `count(up)`, time.Unix(1787933018, 0))
	if err != nil {
		t.Fatalf("instant: %v", err)
	}
	last := proxy.params[len(proxy.params)-1]
	if last["query"] != "count(up)" {
		t.Fatalf("query sent as %q", last["query"])
	}
	if last["time"] != "1787933018" {
		t.Fatalf("time sent as %q", last["time"])
	}
}

func TestInstantEmptyVector(t *testing.T) {
	proxy := &instantProxy{body: `{"status":"success","data":{"resultType":"vector","result":[]}}`}
	client := instantClient(t, proxy)

	samples, err := client.Instant(context.Background(), `up`, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("instant: %v", err)
	}
	if len(samples) != 0 {
		t.Fatalf("got %d samples, want none", len(samples))
	}
}

func TestInstantSkipsUnreadableValues(t *testing.T) {
	proxy := &instantProxy{body: `{"status":"success","data":{"resultType":"vector","result":[
		{"metric":{"a":"1"},"value":[1787933018.510]},
		{"metric":{"a":"2"},"value":["not a number","1"]},
		{"metric":{"a":"3"},"value":[1787933018.510,7]},
		{"metric":{"a":"4"},"value":[1787933018.510,"nope"]},
		{"metric":{"a":"5"},"value":[1787933018.510,"3"]}
	]}}`}
	client := instantClient(t, proxy)

	samples, err := client.Instant(context.Background(), `up`, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("instant: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("got %d samples, want only the readable one", len(samples))
	}
	if samples[0].Labels["a"] != "5" {
		t.Fatalf("kept the wrong sample: %+v", samples[0])
	}
}

func TestInstantRefusesNonVector(t *testing.T) {
	proxy := &instantProxy{body: `{"status":"success","data":{"resultType":"matrix","result":[]}}`}
	client := instantClient(t, proxy)

	_, err := client.Instant(context.Background(), `up[5m]`, time.Unix(1, 0))
	if err == nil {
		t.Fatal("a matrix answer was accepted")
	}
}

func TestInstantReportsPrometheusError(t *testing.T) {
	proxy := &instantProxy{body: `{"status":"error","error":"parse error"}`}
	client := instantClient(t, proxy)

	_, err := client.Instant(context.Background(), `sum(`, time.Unix(1, 0))
	if err == nil {
		t.Fatal("a rejected query was accepted")
	}
}

func TestInstantReportsUnreadableBody(t *testing.T) {
	proxy := &instantProxy{body: `not json`}
	client := instantClient(t, proxy)

	_, err := client.Instant(context.Background(), `up`, time.Unix(1, 0))
	if err == nil {
		t.Fatal("an unreadable body was accepted")
	}
}

func TestInstantForgetsTargetAfterAFailure(t *testing.T) {
	proxy := &instantProxy{err: errors.New("connection refused")}
	client := instantClient(t, proxy)

	_, err := client.Instant(context.Background(), `up`, time.Unix(1, 0))
	if err == nil {
		t.Fatal("a proxy failure was swallowed")
	}
	client.mu.Lock()
	resolved := client.resolved
	client.mu.Unlock()
	if resolved != nil {
		t.Fatalf("the failed target stayed cached as %+v", resolved)
	}
}

func TestInstantReportsDiscoveryFailure(t *testing.T) {
	client := NewClientWithProxy(k8sfake.NewClientset(), &instantProxy{}, Target{})

	_, err := client.Instant(context.Background(), `up`, time.Unix(1, 0))
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("got %v, want ErrUnavailable", err)
	}
}

func TestInstantSendsTheQueryOverTheServiceProxy(t *testing.T) {
	var asked string
	apiserver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[
			{"metric":{"source_workload":"web"},"value":[1787933018.510,"12.5"]}
		]}}`))
	}))
	t.Cleanup(apiserver.Close)
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: apiserver.URL})
	if err != nil {
		t.Fatalf("clientset: %v", err)
	}
	client := NewClient(cs, Target{Namespace: "monitoring", Service: "prometheus", Port: "9090"})

	samples, instantErr := client.Instant(t.Context(), `sum(rate(x[5m]))`, time.Unix(1787933018, 0))

	if instantErr != nil {
		t.Fatalf("instant: %v", instantErr)
	}
	if len(samples) != 1 {
		t.Fatalf("samples = %d, want the one the server sent", len(samples))
	}
	if samples[0].Labels["source_workload"] != "web" {
		t.Fatalf("sample = %+v, want its labels kept", samples[0])
	}
	if !strings.Contains(asked, "/services/https:prometheus:9090/proxy/api/v1/query") {
		t.Fatalf("path = %q, want the instant query through the service proxy", asked)
	}
	if !strings.Contains(asked, "query=sum%28rate%28x%5B5m%5D%29%29") {
		t.Fatalf("path = %q, want the PromQL escaped into the query string", asked)
	}
	if !strings.Contains(asked, "time=1787933018") {
		t.Fatalf("path = %q, want the instant passed through", asked)
	}
}

func TestInstantReportsAnApiserverThatRefusesToProxy(t *testing.T) {
	apiserver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"kind":"Status","status":"Failure","reason":"Forbidden","code":403}`))
	}))
	t.Cleanup(apiserver.Close)
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: apiserver.URL})
	if err != nil {
		t.Fatalf("clientset: %v", err)
	}
	client := NewClient(cs, Target{Namespace: "monitoring", Service: "prometheus", Port: "9090"})

	_, instantErr := client.Instant(t.Context(), `up`, time.Unix(1, 0))

	if !errors.Is(instantErr, ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", instantErr)
	}
	if !strings.Contains(instantErr.Error(), "may not proxy services") {
		t.Fatalf("error = %q, want the proxy refusal spelled out", instantErr.Error())
	}
}
