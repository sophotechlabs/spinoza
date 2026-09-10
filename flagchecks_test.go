//go:build !desktop

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/cluster"
)

func TestAMetricsAddressHasToBeAPortNobodyElseIsOn(t *testing.T) {
	cases := []struct {
		name    string
		metrics string
		main    string
		says    string
	}{
		{name: "no metrics port at all", metrics: "", main: "0.0.0.0:8080"},
		{name: "its own port", metrics: "0.0.0.0:9090", main: "0.0.0.0:8080"},
		{name: "not a host and port", metrics: "9090", main: "0.0.0.0:8080", says: "host:port"},
		{name: "the port the app is on", metrics: "0.0.0.0:8080", main: "0.0.0.0:8080", says: "differ from addr"},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			err := checkMetricsAddr(one.metrics, one.main)
			if one.says == "" {
				if err != nil {
					t.Fatalf("refused a good address: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("accepted an address it should not have")
			}
			if !strings.Contains(err.Error(), one.says) {
				t.Fatalf("error = %q, want it to say %q", err, one.says)
			}
		})
	}
}

func TestAnAuditWebhookHasToBeSomewhereToPostTo(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		says string
	}{
		{name: "no webhook at all", raw: ""},
		{name: "an https url", raw: "https://hooks.example.com/spinoza"},
		{name: "an http url", raw: "http://hooks.example.com/spinoza"},
		{name: "not a url at all", raw: "://", says: "is not a url"},
		{name: "a scheme nothing posts over", raw: "ftp://hooks.example.com", says: "http or https"},
		{name: "a url naming no host", raw: "https:///spinoza", says: "names no host"},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			err := checkAuditWebhook(one.raw)
			if one.says == "" {
				if err != nil {
					t.Fatalf("refused a good webhook: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("accepted a webhook it should not have")
			}
			if !strings.Contains(err.Error(), one.says) {
				t.Fatalf("error = %q, want it to say %q", err, one.says)
			}
		})
	}
}

func runnable() runnableSettings {
	return runnableSettings{
		addr: "0.0.0.0:8080",
		cluster: cluster.Options{
			ClientQPS:        defaultQPS,
			ClientBurst:      defaultBurst,
			SyncTimeout:      defaultSync,
			WarmConcurrency:  defaultWarm,
			CountBudget:      defaultCountBudget,
			CountPerType:     defaultCountPerType,
			CountConcurrency: defaultCountConcurrency,
		},
	}
}

func TestSettingsAreCheckedBeforeSpinozaStarts(t *testing.T) {
	cases := []struct {
		name  string
		build func(runnableSettings) runnableSettings
		says  string
	}{
		{
			name:  "nothing unusual asked for",
			build: func(rs runnableSettings) runnableSettings { return rs },
		},
		{
			name: "a metrics port that is not one",
			build: func(rs runnableSettings) runnableSettings {
				rs.metrics = "nine thousand"
				return rs
			},
			says: "host:port",
		},
		{
			name: "a webhook that is not a url",
			build: func(rs runnableSettings) runnableSettings {
				rs.webhook = "ftp://hooks.example.com"
				return rs
			},
			says: "http or https",
		},
		{
			name: "an audit interval too short to run in",
			build: func(rs runnableSettings) runnableSettings {
				rs.interval = time.Second
				return rs
			},
			says: "at least",
		},
		{
			name: "an audit interval long enough",
			build: func(rs runnableSettings) runnableSettings {
				rs.interval = time.Hour
				return rs
			},
		},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			err := one.build(runnable()).check()
			if one.says == "" {
				if err != nil {
					t.Fatalf("refused settings it should have accepted: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("accepted settings it should have refused")
			}
			if !strings.Contains(err.Error(), one.says) {
				t.Fatalf("error = %q, want it to say %q", err, one.says)
			}
		})
	}
}

func TestANodeShellImageHasToBePinned(t *testing.T) {
	rs := runnable()
	rs.nodeShell = true
	rs.cluster.NodeShellImage = "busybox:latest"

	err := rs.check()

	if err == nil {
		t.Fatal("an unpinned node shell image was accepted")
	}
	if !strings.Contains(err.Error(), "sha256 digest") {
		t.Fatalf("error = %q", err)
	}
}
