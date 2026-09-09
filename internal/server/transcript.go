package server

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/store"
	"github.com/sophotechlabs/spinoza/internal/transcript"
)

const transcriptsShown = 200

func (s *Server) UseTranscripts(held *transcript.Store) {
	s.mu.Lock()
	s.transcripts = held
	s.mu.Unlock()
}

func (s *Server) transcriptStore() *transcript.Store {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.transcripts
}

func (s *Server) startTranscript(r *http.Request, kind, target string) *transcript.Session {
	held := s.transcriptStore()
	if !held.On() {
		return nil
	}
	started, err := held.Start(transcript.Header{
		ID:     newRequestID(),
		At:     s.instant().UTC().Format(time.RFC3339),
		Actor:  actorOf(r),
		Target: target,
		Kind:   kind,
	})
	if err != nil {
		slog.Warn("this session will not be recorded", "kind", kind, "target", target, "error", err)
		return nil
	}
	return started
}

func (s *Server) listTranscripts(w http.ResponseWriter, r *http.Request) {
	held := s.transcriptStore()
	if !held.On() {
		writeJSON(w, api.Transcripts{Sessions: []api.Transcript{}, Recording: false, Reason: notRecording})
		return
	}
	headers, err := held.List(transcriptsShown)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := api.Transcripts{Sessions: make([]api.Transcript, 0, len(headers)), Recording: true}
	for _, header := range headers {
		out.Sessions = append(out.Sessions, api.Transcript{
			ID:     header.ID,
			At:     header.At,
			Actor:  header.Actor,
			Target: header.Target,
			Kind:   header.Kind,
			Bytes:  header.Bytes,
		})
	}
	writeJSON(w, out)
}

func (s *Server) readTranscript(w http.ResponseWriter, r *http.Request) {
	held := s.transcriptStore()
	if !held.On() {
		writeError(w, http.StatusNotFound, notRecording)
		return
	}
	_, text, err := held.Text(r.URL.Query().Get("id"))
	if errors.Is(err, transcript.ErrUnknown()) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", `attachment; filename="spinoza-session.txt"`)
	//nolint:gosec // a recorded terminal is raw bytes by nature; it goes out as plain text, nosniff, as an attachment, to admins only
	_, _ = io.WriteString(w, text)
}

func (s *Server) trimTranscripts(keep store.Retention) {
	held := s.transcriptStore()
	if !held.On() || keep.Days <= 0 {
		return
	}
	_, err := held.Trim(time.Duration(keep.Days)*hoursADay*time.Hour, s.instant())
	if err != nil {
		slog.Warn("the recorded sessions could not be trimmed", "error", err)
	}
}

const hoursADay = 24

const notRecording = "this deployment does not record terminal sessions"
