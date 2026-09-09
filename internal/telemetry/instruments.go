package telemetry

import "github.com/sophotechlabs/spinoza/internal/version"

var (
	requestBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
	syncBuckets    = []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120}
	runBuckets     = []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60}
)

type Instruments struct {
	Registry *Registry

	BuildInfo              *Gauge
	HTTPRequests           *Counter
	HTTPRequestSeconds     *Histogram
	WebsocketSessions      *Gauge
	WebsocketSessionsTotal *Counter
	Informers              *Gauge
	InformerSyncSeconds    *Histogram
	APIServerCalls         *Counter
	TerminalSessions       *Counter
	AuditWriteFailures     *Counter
	CheckRunSeconds        *Histogram
	SignIns                *Counter
	AuthFailures           *Counter
}

var declared = Declare(New(), version.String())

func Default() *Instruments {
	return declared
}

func Declare(reg *Registry, release string) *Instruments {
	made := &Instruments{
		Registry: reg,

		BuildInfo: reg.Gauge(
			"spinoza_build_info",
			"the build of spinoza that is running",
			"version",
		),
		HTTPRequests: reg.Counter(
			"spinoza_http_requests_total",
			"requests spinoza has answered",
			"route", "method", "status",
		),
		HTTPRequestSeconds: reg.Histogram(
			"spinoza_http_request_duration_seconds",
			"how long spinoza took to answer a request",
			requestBuckets,
			"route",
		),
		WebsocketSessions: reg.Gauge(
			"spinoza_websocket_sessions",
			"websocket sessions open right now",
		),
		WebsocketSessionsTotal: reg.Counter(
			"spinoza_websocket_sessions_total",
			"websocket sessions spinoza has opened",
		),
		Informers: reg.Gauge(
			"spinoza_informers",
			"informers running right now",
		),
		InformerSyncSeconds: reg.Histogram(
			"spinoza_informer_sync_seconds",
			"how long an informer took to sync",
			syncBuckets,
		),
		APIServerCalls: reg.Counter(
			"spinoza_apiserver_calls_total",
			"calls spinoza has made to an apiserver",
			"outcome",
		),
		TerminalSessions: reg.Counter(
			"spinoza_terminal_sessions_total",
			"terminal sessions spinoza has started",
			"kind",
		),
		AuditWriteFailures: reg.Counter(
			"spinoza_audit_write_failures_total",
			"audit rows spinoza could not write",
		),
		CheckRunSeconds: reg.Histogram(
			"spinoza_check_run_duration_seconds",
			"how long a run of the checks took",
			runBuckets,
		),
		SignIns: reg.Counter(
			"spinoza_sign_ins_total",
			"people spinoza has signed in",
			"role",
		),
		AuthFailures: reg.Counter(
			"spinoza_auth_failures_total",
			"sign-ins spinoza refused",
			"reason",
		),
	}
	made.BuildInfo.Set(1, release)
	return made
}
