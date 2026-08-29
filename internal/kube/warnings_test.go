package kube

import (
	"bytes"
	"log/slog"
	"strconv"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func recorded(t *testing.T) (*WarningSink, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	handler := slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})
	return newWarningLogger(slog.New(handler)), &out
}

const deprecated = "v1 Endpoints is deprecated in v1.33+; use discovery.k8s.io/v1 EndpointSlice"

func TestAWarningIsLoggedAtWarnLevel(t *testing.T) {
	logger, out := recorded(t)

	logger.HandleWarningHeader(deprecationCode, "", deprecated)

	if !strings.Contains(out.String(), "level=WARN") {
		t.Fatalf("log = %q, want it at warn level; klog's bridge reports these as info", out.String())
	}
	if !strings.Contains(out.String(), "EndpointSlice") {
		t.Fatalf("log = %q, want the apiserver's own words", out.String())
	}
}

func TestTheSameWarningIsLoggedOnce(t *testing.T) {
	logger, out := recorded(t)

	for range 20 {
		logger.HandleWarningHeader(deprecationCode, "", deprecated)
	}

	if strings.Count(out.String(), "EndpointSlice") != 1 {
		t.Fatalf("log = %q, want one line; every list of a deprecated type carries this header", out.String())
	}
}

func TestADifferentWarningStillSpeaks(t *testing.T) {
	logger, out := recorded(t)

	logger.HandleWarningHeader(deprecationCode, "", deprecated)
	logger.HandleWarningHeader(deprecationCode, "", "batch/v1beta1 CronJob is deprecated")

	if strings.Count(out.String(), "level=WARN") != 2 {
		t.Fatalf("log = %q, want both warnings", out.String())
	}
}

func TestOnlyDeprecationWarningsAreLogged(t *testing.T) {
	logger, out := recorded(t)

	logger.HandleWarningHeader(199, "", "some other warning")
	logger.HandleWarningHeader(deprecationCode, "", "")

	if out.String() != "" {
		t.Fatalf("log = %q, want nothing for a non-deprecation code or an empty text", out.String())
	}
}

func TestTheRememberedWarningsAreBounded(t *testing.T) {
	logger, _ := recorded(t)

	for i := range warningsRemembered * 2 {
		logger.HandleWarningHeader(deprecationCode, "", "warning "+strconv.Itoa(i))
	}

	if len(logger.seen) > warningsRemembered {
		t.Fatalf("remembered %d warnings, want at most %d", len(logger.seen), warningsRemembered)
	}
}

func TestEveryClientCarriesTheWarningLogger(t *testing.T) {
	t.Setenv("KUBECONFIG", writeKubeconfig(t, validKubeconfig))

	bundle, err := LoadContext(api.ContextRef{}, Options{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if bundle.Config.WarningHandler == nil {
		t.Fatal("no warning handler, so the apiserver's deprecation notices go through klog as info")
	}
}

func TestTheSinkKeepsWhatItSawInOrder(t *testing.T) {
	sink := newWarningLogger(slog.New(slog.DiscardHandler))

	sink.HandleWarningHeader(deprecationCode, "", "v1 Endpoints is deprecated")
	sink.HandleWarningHeader(deprecationCode, "", "batch/v1beta1 CronJob is deprecated")
	sink.HandleWarningHeader(deprecationCode, "", "v1 Endpoints is deprecated")

	seen := sink.Seen()
	if len(seen) != 2 {
		t.Fatalf("kept %d warnings, want the two distinct ones", len(seen))
	}
	if seen[0] != "batch/v1beta1 CronJob is deprecated" {
		t.Fatalf("first was %q, want them sorted", seen[0])
	}
}

func TestTheSinkKeepsNothingItWasNotToldAbout(t *testing.T) {
	sink := newWarningLogger(slog.New(slog.DiscardHandler))

	sink.HandleWarningHeader(200, "", "not a deprecation")
	sink.HandleWarningHeader(deprecationCode, "", "")

	if seen := sink.Seen(); len(seen) != 0 {
		t.Fatalf("kept %v, want nothing", seen)
	}
}
