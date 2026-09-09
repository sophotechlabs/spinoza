package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/safe"
	"github.com/sophotechlabs/spinoza/internal/store"
)

const (
	auditPostTimeout = 10 * time.Second
	auditRunTimeout  = 10 * time.Minute
	auditRunsShown   = 50
	auditRetryAfter  = 5 * time.Second
)

type AuditSchedule struct {
	Every   time.Duration
	Webhook string
}

type auditRunner struct {
	server     *Server
	every      time.Duration
	post       string
	client     *http.Client
	retryAfter time.Duration
}

func (s *Server) UseAuditSchedule(ctx context.Context, plan AuditSchedule) {
	if plan.Every <= 0 {
		return
	}
	runner := &auditRunner{
		server:     s,
		every:      plan.Every,
		post:       plan.Webhook,
		client:     &http.Client{Timeout: auditPostTimeout},
		retryAfter: auditRetryAfter,
	}
	safe.Go("running the checks on a timer", func() {
		runner.loop(ctx)
	})
}

func (r *auditRunner) loop(ctx context.Context) {
	ticker := time.NewTicker(r.every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.once(ctx)
		}
	}
}

func (r *auditRunner) once(ctx context.Context) {
	for _, open := range r.server.cluster.Opened() {
		bounded, cancel := context.WithTimeout(ctx, auditRunTimeout)
		r.runOn(bounded, open.ID)
		cancel()
	}
}

func (r *auditRunner) runOn(ctx context.Context, cluster string) {
	backend := r.server.managerOf(cluster)
	if backend == nil {
		return
	}
	report := backend.CheckExport(ctx, r.server.scheduledCheckFilter(cluster))
	run := runOf(report, r.server.instant())
	past := r.server.recorder()
	before, known := r.previous(ctx, past, cluster)
	if past != nil {
		writeErr := past.RecordRun(ctx, cluster, run)
		if writeErr != nil {
			slog.Warn("a scheduled audit run was not recorded", "cluster", cluster, "error", writeErr)
		}
	}
	if known && !moved(before, run) {
		return
	}
	if !known && run.Findings == 0 {
		return
	}
	r.announce(ctx, cluster, run, report)
}

func (r *auditRunner) previous(ctx context.Context, past History, cluster string) (store.Run, bool) {
	if past == nil {
		return store.Run{}, false
	}
	held, err := past.Runs(ctx, cluster, 1)
	if err != nil || len(held) == 0 {
		return store.Run{}, false
	}
	return held[0], true
}

func moved(before, now store.Run) bool {
	if before.Findings != now.Findings {
		return true
	}
	if before.Scanned != now.Scanned {
		return true
	}
	return before.Fresh != now.Fresh || before.Cleared != now.Cleared
}

func runOf(report api.CheckReport, at time.Time) store.Run {
	run := store.Run{At: at, Scanned: report.Scanned}
	for _, group := range report.Groups {
		run.Findings += group.Total
		run.Fresh += group.NewCount
		run.Cleared += group.Fixed
	}
	return run
}

type auditNotice struct {
	Cluster  string         `json:"cluster"`
	At       string         `json:"at"`
	Findings int            `json:"findings"`
	New      int            `json:"new"`
	Cleared  int            `json:"cleared"`
	Scanned  int            `json:"scanned"`
	Severity map[string]int `json:"severity,omitempty"`
	Changed  []string       `json:"changed,omitempty"`
}

func (r *auditRunner) announce(ctx context.Context, cluster string, run store.Run, report api.CheckReport) {
	if r.post == "" {
		return
	}
	body, err := json.Marshal(noticeOf(cluster, run, report))
	if err != nil {
		slog.Warn("the audit notice could not be encoded", "cluster", cluster, "error", err)
		return
	}
	if r.send(ctx, body) {
		return
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(r.retryAfter):
	}
	if r.send(ctx, body) {
		return
	}
	slog.Warn("the audit notice was not delivered", "cluster", cluster, "url", r.post)
}

func (r *auditRunner) send(ctx context.Context, body []byte) bool {
	asked, err := http.NewRequestWithContext(ctx, http.MethodPost, r.post, bytes.NewReader(body))
	if err != nil {
		return false
	}
	asked.Header.Set("Content-Type", "application/json")
	answer, sendErr := r.client.Do(asked)
	if sendErr != nil {
		return false
	}
	defer func() { _ = answer.Body.Close() }()
	return answer.StatusCode >= 200 && answer.StatusCode < 300
}

func noticeOf(cluster string, run store.Run, report api.CheckReport) auditNotice {
	notice := auditNotice{
		Cluster:  cluster,
		At:       run.At.UTC().Format(time.RFC3339),
		Findings: run.Findings,
		New:      run.Fresh,
		Cleared:  run.Cleared,
		Scanned:  run.Scanned,
		Severity: map[string]int{},
		Changed:  []string{},
	}
	for _, group := range report.Groups {
		if group.Total > 0 {
			notice.Severity[group.Severity] += group.Total
		}
		if group.NewCount > 0 || group.Fixed > 0 {
			notice.Changed = append(notice.Changed, group.ID)
		}
	}
	return notice
}

func (s *Server) handleAuditRuns(w http.ResponseWriter, r *http.Request) {
	past := s.recorder()
	if past == nil {
		writeJSON(w, api.AuditRuns{Runs: []api.AuditRun{}, Reason: api.HistoryOff})
		return
	}
	held, err := past.Runs(r.Context(), s.clusterKey(r), auditRunsShown)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := api.AuditRuns{Runs: make([]api.AuditRun, 0, len(held)), Interval: s.auditInterval()}
	for _, one := range held {
		out.Runs = append(out.Runs, api.AuditRun{
			ID:       strconv.FormatInt(one.ID, 10),
			At:       one.At.UTC().Format(time.RFC3339),
			Findings: one.Findings,
			NewCount: one.Fresh,
			Cleared:  one.Cleared,
			Scanned:  one.Scanned,
		})
	}
	writeJSON(w, out)
}
