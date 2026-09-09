package server

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/sophotechlabs/spinoza/internal/telemetry"
)

const otherRoute = "other"

var knownRoutes sync.Map

func rememberRoute(path string) {
	knownRoutes.Store(path, struct{}{})
}

func routeLabel(path string) string {
	_, known := knownRoutes.Load(path)
	if known {
		return path
	}
	return otherRoute
}

func measureRequest(r *http.Request, status int, took time.Duration) {
	route := routeLabel(r.URL.Path)
	meters := telemetry.Default()
	meters.HTTPRequests.Add(1, route, r.Method, strconv.Itoa(status))
	meters.HTTPRequestSeconds.Observe(took.Seconds(), route)
}

func measureSocketOpened() {
	meters := telemetry.Default()
	meters.WebsocketSessions.Add(1)
	meters.WebsocketSessionsTotal.Add(1)
}

func measureSocketClosed() {
	telemetry.Default().WebsocketSessions.Add(-1)
}

func measureTerminal(kind string) {
	telemetry.Default().TerminalSessions.Add(1, kind)
}

func measureAuditFailure() {
	telemetry.Default().AuditWriteFailures.Add(1)
}
