package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/transcript"
)

func transcribingServer(t *testing.T) (*Server, *transcript.Store) {
	t.Helper()
	srv := New(&stubBackendCluster{backend: &stubCatalog{}}, testAssets(), testToken)
	held := transcript.Open(t.TempDir())
	srv.UseTranscripts(held)
	return srv, held
}

func TestSessionsAreNotRecordedUnlessThisDeploymentSaysSo(t *testing.T) {
	srv := New(&stubBackendCluster{backend: &stubCatalog{}}, testAssets(), testToken)
	srv.UseTranscripts(transcript.Open(""))

	rec := httptest.NewRecorder()
	srv.listTranscripts(rec, httptest.NewRequest(http.MethodGet, "/api/transcripts", http.NoBody))

	var page api.Transcripts
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("the page did not parse: %v", err)
	}
	if page.Recording {
		t.Fatal("a store with no directory reported that it was recording")
	}
	if page.Reason == "" {
		t.Fatal("nothing said why there are no sessions")
	}
	if len(page.Sessions) != 0 {
		t.Fatalf("sessions = %d, want none", len(page.Sessions))
	}
}

func TestARecordedSessionIsListedAndCanBeRead(t *testing.T) {
	srv, held := transcribingServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/exec", http.NoBody)

	tape := srv.startTranscript(req, "exec", "prod/web")
	if tape == nil {
		t.Fatal("no transcript was started")
	}
	tape.Typed([]byte("id\n"))
	tape.Shown([]byte("uid=0(root)\n"))
	tape.Close()

	rec := httptest.NewRecorder()
	srv.listTranscripts(rec, httptest.NewRequest(http.MethodGet, "/api/transcripts", http.NoBody))
	var page api.Transcripts
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("the page did not parse: %v", err)
	}
	if !page.Recording || len(page.Sessions) != 1 {
		t.Fatalf("page = %+v", page)
	}
	one := page.Sessions[0]
	if one.Target != "prod/web" || one.Kind != "exec" || one.Actor == "" {
		t.Fatalf("session = %+v", one)
	}

	text := httptest.NewRecorder()
	srv.readTranscript(text, httptest.NewRequest(http.MethodGet, "/api/transcripts/text?id="+one.ID, http.NoBody))
	if text.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", text.Code, text.Body.String())
	}
	if !strings.Contains(text.Body.String(), "uid=0(root)") {
		t.Fatalf("body = %q", text.Body.String())
	}
	if text.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("content type = %q", text.Header().Get("Content-Type"))
	}
	if !held.On() {
		t.Fatal("the store stopped recording")
	}
}

func TestReadingASessionThatIsNotThereSaysSo(t *testing.T) {
	srv, _ := transcribingServer(t)

	rec := httptest.NewRecorder()
	srv.readTranscript(rec, httptest.NewRequest(http.MethodGet, "/api/transcripts/text?id=missing", http.NoBody))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestStartingATranscriptWithNoStoreGivesNothingToWriteTo(t *testing.T) {
	srv := New(&stubBackendCluster{backend: &stubCatalog{}}, testAssets(), testToken)
	srv.UseTranscripts(transcript.Open(""))

	tape := srv.startTranscript(httptest.NewRequest(http.MethodGet, "/api/exec", http.NoBody), "exec", "prod/web")

	if tape != nil {
		t.Fatal("a transcript was started with nowhere to write it")
	}
	tape.Typed([]byte("still safe"))
	tape.Close()
}
