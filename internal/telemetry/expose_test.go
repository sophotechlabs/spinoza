package telemetry

import (
	"math"
	"strings"
	"testing"
)

func rendered(t *testing.T, reg *Registry) string {
	t.Helper()
	page := &strings.Builder{}
	err := reg.Write(page)
	if err != nil {
		t.Fatalf("write the registry: %v", err)
	}
	return page.String()
}

func smallRegistry(t *testing.T) *Registry {
	t.Helper()
	reg := New()
	build := reg.Gauge("spinoza_build_info", "the build of spinoza that is running", "version")
	build.Set(1, "v1.2.3")
	calls := reg.Counter("spinoza_apiserver_calls_total", "calls spinoza has made to an apiserver", "outcome")
	calls.Add(3, "ok")
	calls.Inc("refused")
	took := reg.Histogram("spinoza_check_run_duration_seconds", "how long a run of the checks took", []float64{0.5, 1, 2})
	took.Observe(0.25)
	took.Observe(1.5)
	took.Observe(4)
	return reg
}

func TestASmallRegistryRendersExactlyThisPage(t *testing.T) {
	want := `# HELP spinoza_apiserver_calls_total calls spinoza has made to an apiserver
# TYPE spinoza_apiserver_calls_total counter
spinoza_apiserver_calls_total{outcome="ok"} 3
spinoza_apiserver_calls_total{outcome="refused"} 1
# HELP spinoza_build_info the build of spinoza that is running
# TYPE spinoza_build_info gauge
spinoza_build_info{version="v1.2.3"} 1
# HELP spinoza_check_run_duration_seconds how long a run of the checks took
# TYPE spinoza_check_run_duration_seconds histogram
spinoza_check_run_duration_seconds_bucket{le="0.5"} 1
spinoza_check_run_duration_seconds_bucket{le="1"} 1
spinoza_check_run_duration_seconds_bucket{le="2"} 2
spinoza_check_run_duration_seconds_bucket{le="+Inf"} 3
spinoza_check_run_duration_seconds_sum 5.75
spinoza_check_run_duration_seconds_count 3
`

	got := rendered(t, smallRegistry(t))

	if got != want {
		t.Fatalf("the exposition page reads\n%s\nand should read\n%s", got, want)
	}
}

func TestTheSameRegistryRendersTheSamePageEveryTime(t *testing.T) {
	reg := New()
	calls := reg.Counter("spinoza_apiserver_calls_total", "calls spinoza has made to an apiserver", "outcome")
	sessions := reg.Gauge("spinoza_sessions", "signed-in sessions right now", "role")
	for _, outcome := range []string{"ok", "refused", "timeout", "unreachable", "conflict"} {
		calls.Inc(outcome)
	}
	for _, role := range []string{"admin", "editor", "viewer", "auditor"} {
		sessions.Set(2, role)
	}

	first := rendered(t, reg)
	for range 40 {
		if rendered(t, reg) != first {
			t.Fatal("two renders of one registry disagreed, so the order is not settled")
		}
	}
}

func TestAMetricNamePrecedesTheOneAfterItAlphabetically(t *testing.T) {
	reg := New()
	reg.Counter("spinoza_zulu_total", "last", "kind")
	reg.Counter("spinoza_alpha_total", "first", "kind")
	reg.Counter("spinoza_mike_total", "middle", "kind")
	for _, name := range []string{"spinoza_zulu_total", "spinoza_alpha_total", "spinoza_mike_total"} {
		reg.vectors[name].at([]string{"one"})
	}

	page := rendered(t, reg)

	order := []int{
		strings.Index(page, "spinoza_alpha_total{"),
		strings.Index(page, "spinoza_mike_total{"),
		strings.Index(page, "spinoza_zulu_total{"),
	}
	for at := range len(order) - 1 {
		if order[at] < 0 || order[at] > order[at+1] {
			t.Fatalf("the metrics came out in the order %v, which is not alphabetical", order)
		}
	}
}

func TestALabelValueThatWouldBreakTheFormatIsEscaped(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{name: "a double quote", value: `say "no"`, want: `spinoza_auth_failures_total{reason="say \"no\""} 1`},
		{name: "a backslash", value: `back\slash`, want: `spinoza_auth_failures_total{reason="back\\slash"} 1`},
		{name: "a newline", value: "two\nlines", want: `spinoza_auth_failures_total{reason="two\nlines"} 1`},
		{name: "all three at once", value: "a\"b\\c\nd", want: `spinoza_auth_failures_total{reason="a\"b\\c\nd"} 1`},
		{name: "nothing that needs it", value: "expired", want: `spinoza_auth_failures_total{reason="expired"} 1`},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			reg := New()
			failures := reg.Counter("spinoza_auth_failures_total", "sign-ins spinoza refused", "reason")
			failures.Inc(one.value)

			page := rendered(t, reg)

			if !strings.Contains(page, one.want) {
				t.Fatalf("the page reads\n%s\nand should carry the line\n%s", page, one.want)
			}
		})
	}
}

func TestHelpTextThatWouldBreakTheFormatIsEscaped(t *testing.T) {
	reg := New()
	reg.Counter("spinoza_odd_help_total", "one\ntwo\\three")

	page := rendered(t, reg)

	if !strings.Contains(page, `# HELP spinoza_odd_help_total one\ntwo\\three`+"\n") {
		t.Fatalf("the help line was not escaped:\n%s", page)
	}
}

func TestAMetricWithNoSeriesYetIsLeftOffThePage(t *testing.T) {
	reg := New()
	reg.Counter("spinoza_auth_failures_total", "sign-ins spinoza refused", "reason")

	page := rendered(t, reg)

	if strings.Contains(page, "spinoza_auth_failures_total") {
		t.Fatalf("a metric nothing has written to was announced anyway:\n%s", page)
	}
}

func TestAMetricWithNoLabelsReportsZeroBeforeAnythingWritesToIt(t *testing.T) {
	reg := New()
	reg.Counter("spinoza_audit_write_failures_total", "audit rows spinoza could not write")

	page := rendered(t, reg)

	if !strings.Contains(page, "spinoza_audit_write_failures_total 0\n") {
		t.Fatalf("an unwritten counter did not report zero:\n%s", page)
	}
}

func TestAValueTooLargeForAnIntegerStillRendersAsANumber(t *testing.T) {
	cases := []struct {
		name  string
		value float64
		want  string
	}{
		{name: "a whole number", value: 42, want: "spinoza_sessions{role=\"admin\"} 42"},
		{name: "a fraction", value: 0.5, want: "spinoza_sessions{role=\"admin\"} 0.5"},
		{name: "a million", value: 1e6, want: "spinoza_sessions{role=\"admin\"} 1e+06"},
		{name: "a tiny fraction", value: 1e-9, want: "spinoza_sessions{role=\"admin\"} 1e-09"},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			reg := New()
			sessions := reg.Gauge("spinoza_sessions", "signed-in sessions right now", "role")
			sessions.Set(one.value, "admin")

			page := rendered(t, reg)

			if !strings.Contains(page, one.want) {
				t.Fatalf("the page reads\n%s\nand should carry\n%s", page, one.want)
			}
		})
	}
}

func TestTheContentTypeNamesTheExpositionVersionTheBodyIsWrittenIn(t *testing.T) {
	if ContentType() != "text/plain; version=0.0.4; charset=utf-8" {
		t.Fatalf("ContentType() = %q", ContentType())
	}
}

func TestAGaugeThatIsNotAFiniteNumberStillRendersAsTheFormatSpellsIt(t *testing.T) {
	cases := []struct {
		name  string
		value float64
		want  string
	}{
		{name: "no reading at all", value: math.NaN(), want: "NaN"},
		{name: "past anything measurable", value: math.Inf(1), want: "+Inf"},
		{name: "below anything measurable", value: math.Inf(-1), want: "-Inf"},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			reg := New()
			sessions := reg.Gauge("spinoza_sessions", "signed-in sessions right now", "role")
			sessions.Set(one.value, "admin")

			page := rendered(t, reg)

			if !strings.Contains(page, `spinoza_sessions{role="admin"} `+one.want+"\n") {
				t.Fatalf("the page reads\n%s\nand should carry %s", page, one.want)
			}
		})
	}
}
