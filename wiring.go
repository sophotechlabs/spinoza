//go:build !desktop

package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/sophotechlabs/spinoza/internal/server"
	"github.com/sophotechlabs/spinoza/internal/store"
	"github.com/sophotechlabs/spinoza/internal/telemetry"
	"github.com/sophotechlabs/spinoza/internal/transcript"
)

const (
	metricsReadHeaderTimeout = 5 * time.Second
	drainGrace               = 20 * time.Second
	hoursADay                = 24
)

type mirroredHistory struct {
	server.History

	out *slog.Logger
}

func (m mirroredHistory) For(cluster string) store.Recorder {
	return store.Mirror(m.History.For(cluster), m.out)
}

func auditedHistory(past server.History, serving bool, format string) server.History {
	if !serving {
		return past
	}
	return mirroredHistory{History: past, out: auditLogger(format)}
}

func auditLogger(format string) *slog.Logger {
	return auditLoggerTo(os.Stderr, format)
}

func auditLoggerTo(out io.Writer, format string) *slog.Logger {
	return slog.New(logHandler(out, slog.LevelInfo, format))
}

func auditRetention(keep time.Duration) store.Retention {
	if keep <= 0 {
		return store.Retention{}
	}
	days := max(int(keep/(hoursADay*time.Hour)), 1)
	return store.Retention{Days: days}
}

func transcriptStore(on bool) *transcript.Store {
	if !on {
		return transcript.Open("")
	}
	dir, err := transcript.DefaultDir()
	if err != nil {
		slog.Warn("terminal sessions will not be recorded", "error", err)
		return transcript.Open("")
	}
	return transcript.Open(dir)
}

func metricsServer(addr string) *http.Server {
	if addr == "" {
		return nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", telemetry.ContentType())
		writeErr := telemetry.Default().Registry.Write(w)
		if writeErr != nil {
			slog.Warn("the metrics page could not be written", "error", writeErr)
		}
	})
	return &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: metricsReadHeaderTimeout}
}

func serveMetrics(metrics *http.Server) {
	if metrics == nil {
		return
	}
	slog.Info("spinoza is also reporting on itself", "addr", metrics.Addr, "path", "/metrics")
	err := metrics.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Warn("the metrics listener stopped", "error", err)
	}
}

func stopMetrics(ctx context.Context, metrics *http.Server) {
	if metrics == nil {
		return
	}
	_ = metrics.Shutdown(ctx)
}
