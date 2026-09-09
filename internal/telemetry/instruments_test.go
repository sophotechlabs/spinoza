package telemetry

import (
	"slices"
	"strings"
	"testing"
)

func TestTheAppDeclaresEveryInstrumentItReportsOn(t *testing.T) {
	cases := []struct {
		name   string
		kind   string
		labels []string
	}{
		{name: "spinoza_build_info", kind: gaugeKind, labels: []string{"version"}},
		{name: "spinoza_http_requests_total", kind: counterKind, labels: []string{"route", "method", "status"}},
		{name: "spinoza_http_request_duration_seconds", kind: histogramKind, labels: []string{"route"}},
		{name: "spinoza_websocket_sessions", kind: gaugeKind, labels: nil},
		{name: "spinoza_websocket_sessions_total", kind: counterKind, labels: nil},
		{name: "spinoza_informers", kind: gaugeKind, labels: nil},
		{name: "spinoza_informer_sync_seconds", kind: histogramKind, labels: nil},
		{name: "spinoza_apiserver_calls_total", kind: counterKind, labels: []string{"outcome"}},
		{name: "spinoza_terminal_sessions_total", kind: counterKind, labels: []string{"kind"}},
		{name: "spinoza_audit_write_failures_total", kind: counterKind, labels: nil},
		{name: "spinoza_check_run_duration_seconds", kind: histogramKind, labels: nil},
		{name: "spinoza_sign_ins_total", kind: counterKind, labels: []string{"role"}},
		{name: "spinoza_auth_failures_total", kind: counterKind, labels: []string{"reason"}},
	}
	reg := New()
	Declare(reg, "v9.9.9")

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			found, declared := reg.vectors[one.name]
			if !declared {
				t.Fatalf("%s is not declared", one.name)
			}
			if found.kind != one.kind {
				t.Fatalf("%s is a %s and should be a %s", one.name, found.kind, one.kind)
			}
			if !slices.Equal(found.labels, one.labels) {
				t.Fatalf("%s carries the labels %v and should carry %v", one.name, found.labels, one.labels)
			}
			if found.kind == histogramKind && len(found.buckets) == 0 {
				t.Fatalf("%s is a histogram with no buckets", one.name)
			}
		})
	}
}

func TestTheBuildIsReportedAsOneAgainstTheVersionThatIsRunning(t *testing.T) {
	reg := New()
	Declare(reg, "v9.9.9")

	page := rendered(t, reg)

	if !strings.Contains(page, "# TYPE spinoza_build_info gauge\nspinoza_build_info{version=\"v9.9.9\"} 1\n") {
		t.Fatalf("the build line reads\n%s", page)
	}
}

func TestEverySecondsHistogramCarriesBucketsAPersonWouldWaitFor(t *testing.T) {
	reg := New()
	Declare(reg, "v9.9.9")

	for name, one := range reg.vectors {
		if one.kind != histogramKind {
			continue
		}
		if !strings.HasSuffix(name, "_seconds") {
			t.Errorf("%s measures time and is not named in seconds", name)
		}
		if !slices.IsSorted(one.buckets) {
			t.Errorf("%s has buckets out of order: %v", name, one.buckets)
		}
		if one.buckets[0] <= 0 {
			t.Errorf("%s starts at %v, which no observation can fall below", name, one.buckets[0])
		}
		if one.buckets[len(one.buckets)-1] > 600 {
			t.Errorf("%s runs to %v seconds, which is past anything a person waits for", name, one.buckets[len(one.buckets)-1])
		}
	}
}

func TestTheDefaultInstrumentsAreDeclaredOnceAndShared(t *testing.T) {
	first := Default()
	second := Default()

	if first != second {
		t.Fatal("two calls to Default gave two different sets of instruments")
	}
	if first.Registry == nil {
		t.Fatal("the default instruments carry no registry to write")
	}
	if !strings.Contains(rendered(t, first.Registry), "# TYPE spinoza_build_info gauge\n") {
		t.Fatal("the default registry does not report the build")
	}
}

func TestTheDefaultRegistryTakesWritesFromEveryInstrument(t *testing.T) {
	reg := New()
	app := Declare(reg, "v9.9.9")

	app.HTTPRequests.Inc("/api/version", "GET", "200")
	app.HTTPRequestSeconds.Observe(0.02, "/api/version")
	app.WebsocketSessions.Inc()
	app.WebsocketSessionsTotal.Inc()
	app.Informers.Set(4)
	app.InformerSyncSeconds.Observe(1.5)
	app.APIServerCalls.Inc("ok")
	app.TerminalSessions.Inc("exec")
	app.AuditWriteFailures.Inc()
	app.CheckRunSeconds.Observe(3)
	app.SignIns.Inc("admin")
	app.AuthFailures.Inc("expired")

	page := rendered(t, reg)

	want := []string{
		`spinoza_http_requests_total{route="/api/version",method="GET",status="200"} 1`,
		`spinoza_http_request_duration_seconds_count{route="/api/version"} 1`,
		"spinoza_websocket_sessions 1",
		"spinoza_websocket_sessions_total 1",
		"spinoza_informers 4",
		"spinoza_informer_sync_seconds_count 1",
		`spinoza_apiserver_calls_total{outcome="ok"} 1`,
		`spinoza_terminal_sessions_total{kind="exec"} 1`,
		"spinoza_audit_write_failures_total 1",
		"spinoza_check_run_duration_seconds_count 1",
		`spinoza_sign_ins_total{role="admin"} 1`,
		`spinoza_auth_failures_total{reason="expired"} 1`,
	}
	for _, line := range want {
		if !strings.Contains(page, line+"\n") {
			t.Errorf("the page does not carry %q", line)
		}
	}
	if strings.Contains(page, "spinoza_telemetry_writes_rejected_total") {
		t.Errorf("a write to a declared instrument was refused:\n%s", page)
	}
}
