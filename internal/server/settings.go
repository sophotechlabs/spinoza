package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const maxSettingsBytes = 1 << 20

type Settings interface {
	All() map[string]string
	Replace(values map[string]string) error
}

func (s *Server) UseSettings(store Settings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = store
}

func (s *Server) stored() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

func SettingsScript(values map[string]string) string {
	body, err := json.Marshal(values)
	if err != nil {
		return ""
	}
	return "<script>window.__SPINOZA_SETTINGS__=" + strconv.Quote(string(body)) + ";</script>"
}

func (s *Server) readSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, api.Settings{Values: s.stored().All()})
}

func (s *Server) writeSettings(w http.ResponseWriter, r *http.Request) {
	var wanted api.Settings
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSettingsBytes)).Decode(&wanted)
	if err != nil {
		writeError(w, http.StatusBadRequest, "settings must be an object of strings")
		return
	}
	saveErr := s.stored().Replace(wanted.Values)
	if saveErr != nil {
		writeError(w, http.StatusInternalServerError, saveErr.Error())
		return
	}
	writeJSON(w, api.Settings{Values: s.stored().All()})
}
