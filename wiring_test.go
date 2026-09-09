//go:build !desktop

package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/server"
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
