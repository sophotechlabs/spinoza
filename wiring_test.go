//go:build !desktop

package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/server"
	"github.com/sophotechlabs/spinoza/internal/store"
)

func TestAuditRetentionTurnsADurationIntoWholeDays(t *testing.T) {
	cases := []struct {
		name string
		keep time.Duration
		want int
	}{
		{name: "nothing asked for", keep: 0},
		{name: "a negative window", keep: -time.Hour},
		{name: "under a day still keeps a day", keep: time.Hour, want: 1},
		{name: "thirty days", keep: 720 * time.Hour, want: 30},
		{name: "a day and a half rounds down to one", keep: 36 * time.Hour, want: 1},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			got := auditRetention(one.keep)
			if got.Days != one.want {
				t.Fatalf("days = %d, want %d", got.Days, one.want)
			}
		})
	}
}

func TestOnYourOwnMachineTheAuditIsNotMirrored(t *testing.T) {
	past := &countingHistory{}

	held := auditedHistory(past, false, logJSON)

	if held == nil {
		t.Fatal("the history was dropped")
	}
	if _, wrapped := held.(mirroredHistory); wrapped {
		t.Fatal("a local run mirrored its audit to the log")
	}
}

func TestServingAClusterMirrorsTheAuditAsJSON(t *testing.T) {
	past := &countingHistory{}

	held := auditedHistory(past, true, logJSON)

	if _, wrapped := held.(mirroredHistory); !wrapped {
		t.Fatal("a served cluster did not mirror its audit")
	}
}

func TestTheMetricsServerIsOnlyBuiltWhenAnAddressIsGiven(t *testing.T) {
	if metricsServer("") != nil {
		t.Fatal("a metrics listener was built with no address")
	}
	built := metricsServer("127.0.0.1:0")
	if built == nil {
		t.Fatal("no metrics listener was built")
	}
	if built.ReadHeaderTimeout == 0 {
		t.Fatal("the metrics listener has no read header timeout")
	}
	stopMetrics(t.Context(), nil)
	stopMetrics(t.Context(), built)
}

type countingHistory struct {
	server.History

	recorded int
}

func (c *countingHistory) For(string) store.Recorder {
	return countingRecorder{into: c}
}

type countingRecorder struct {
	into *countingHistory
}

func (c countingRecorder) Record(context.Context, store.Entry) error {
	c.into.recorded++
	return nil
}

func TestTheAuditLineParsesWhateverTheLogLevelIs(t *testing.T) {
	var page strings.Builder
	logger := auditLoggerTo(&page, logJSON)

	logger.Info("what spinoza did", "event", "audit", "actor", "alice@example.com")

	line := strings.TrimSpace(page.String())
	if line == "" {
		t.Fatal("nothing was written")
	}
	var read map[string]any
	if err := json.Unmarshal([]byte(line), &read); err != nil {
		t.Fatalf("the line did not parse: %v (%s)", err, line)
	}
	if read["actor"] != "alice@example.com" {
		t.Fatalf("actor = %v", read["actor"])
	}
}

func TestTheMirroredHistoryWrapsEveryClusterRecorder(t *testing.T) {
	var page strings.Builder
	past := &countingHistory{}
	held := mirroredHistory{History: past, out: auditLoggerTo(&page, logJSON)}

	err := held.For("kind-spinoza").Record(t.Context(), store.Entry{
		Verb:  "delete",
		Actor: "alice@example.com",
		Name:  "web",
		At:    time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if past.recorded != 1 {
		t.Fatalf("the store was handed %d entries", past.recorded)
	}
	line := strings.TrimSpace(page.String())
	var read map[string]any
	if unmarshalErr := json.Unmarshal([]byte(line), &read); unmarshalErr != nil {
		t.Fatalf("the audit line did not parse: %v (%s)", unmarshalErr, line)
	}
	if read["event"] != "audit" || read["actor"] != "alice@example.com" {
		t.Fatalf("line = %v", read)
	}
}

func TestNoTranscriptStoreUnlessThisDeploymentAsksForOne(t *testing.T) {
	if transcriptStore(false).On() {
		t.Fatal("sessions are being recorded without being asked for")
	}
	if !transcriptStore(true).On() {
		t.Fatal("sessions were asked for and are not being recorded")
	}
}

func TestTheMetricsListenerServesTheRegistryAndStops(t *testing.T) {
	built := metricsServer("127.0.0.1:0")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	built.Addr = listener.Addr().String()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = built.Serve(listener)
	}()

	answer, getErr := http.Get("http://" + built.Addr + "/metrics")
	if getErr != nil {
		t.Fatalf("get: %v", getErr)
	}
	defer func() { _ = answer.Body.Close() }()
	body, readErr := io.ReadAll(answer.Body)
	if readErr != nil {
		t.Fatalf("read: %v", readErr)
	}
	if answer.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", answer.StatusCode)
	}
	if !strings.Contains(string(body), "spinoza_build_info") {
		t.Fatalf("the metrics page carried nothing: %q", string(body))
	}
	if answer.Header.Get("Content-Type") == "" {
		t.Fatal("the metrics page named no content type")
	}

	stopMetrics(t.Context(), built)
	<-done
}

func TestServingMetricsOnAnAddressNobodyCanTakeSaysSo(t *testing.T) {
	serveMetrics(nil)
	serveMetrics(metricsServer("127.0.0.1:1"))
}
