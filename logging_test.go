package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"k8s.io/klog/v2"
)

func TestKlogRoutesThroughSlog(t *testing.T) {
	var buf bytes.Buffer
	klog.SetSlogLogger(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(klog.ClearLogger)

	klog.Warning("spinoza-klog-probe")
	klog.Flush()

	if !strings.Contains(buf.String(), "spinoza-klog-probe") {
		t.Fatalf("klog output did not reach the slog handler: %q", buf.String())
	}
}

func TestTheHandlerEscapesWhatItIsGiven(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(logHandler(&buf, slog.LevelInfo, logText))

	logger.Warn("a websocket upgrade was refused", "path", "/api\nlevel=ERROR msg=\"forged\"")

	written := strings.TrimSuffix(buf.String(), "\n")
	if strings.Contains(written, "\n") {
		t.Fatalf("one call wrote more than one line:\n%s", written)
	}
	if !strings.Contains(written, `\nlevel=ERROR`) {
		t.Fatalf("the newline was not escaped: %s", written)
	}
}

func TestAPanicIsLoggedOnOneLine(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(logHandler(&buf, slog.LevelInfo, logText))

	logger.Error("recovered from a panic", "panic", "boom\nlevel=INFO msg=\"all is well\"")

	if lines := strings.Count(strings.TrimSuffix(buf.String(), "\n"), "\n"); lines != 0 {
		t.Fatalf("a panic wrote %d extra lines:\n%s", lines, buf.String())
	}
}

func TestTheJSONHandlerKeepsOneLineAndParses(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(logHandler(&buf, slog.LevelInfo, logJSON))

	logger.Warn("a websocket upgrade was refused", "path", "/api\nforged")

	written := strings.TrimSuffix(buf.String(), "\n")
	if strings.Count(written, "\n") != 0 {
		t.Fatalf("one call wrote more than one line:\n%s", written)
	}
	var read map[string]any
	err := json.Unmarshal([]byte(written), &read)
	if err != nil {
		t.Fatalf("the line did not parse as json: %v\n%s", err, written)
	}
	if read["path"] != "/api\nforged" {
		t.Fatalf("path came back as %q", read["path"])
	}
}

func TestTheFormatDefaultsToTextHereAndJSONWhenServingACluster(t *testing.T) {
	cases := []struct {
		name    string
		asked   string
		serving bool
		want    string
	}{
		{name: "nothing asked on your own machine", want: logText},
		{name: "nothing asked while serving", serving: true, want: logJSON},
		{name: "text asked while serving", asked: logText, serving: true, want: logText},
		{name: "json asked on your own machine", asked: logJSON, want: logJSON},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			if got := logFormatFor(one.asked, one.serving); got != one.want {
				t.Fatalf("format = %q, want %q", got, one.want)
			}
		})
	}
}

func TestOnlyJSONAndTextAreFormats(t *testing.T) {
	for _, good := range []string{"", logText, logJSON} {
		if _, err := parseLogFormat(good); err != nil {
			t.Fatalf("%q was refused: %v", good, err)
		}
	}
	if _, err := parseLogFormat("logfmt"); err == nil {
		t.Fatal("an unknown format was accepted")
	}
}
