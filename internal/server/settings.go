package server

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/auth"
	"github.com/sophotechlabs/spinoza/internal/checks"
)

const maxSettingsBytes = 128 << 10

const maxPersonalSettingsBytes = 64 << 10

const defaultPersonalSettingBytes = 16 << 10

const userSettingsPrefix = "spinoza.user.v1."

var personalSettingKeys = map[string]bool{
	"spinoza.theme.v1":        true,
	"spinoza.themes.v1":       true,
	"spinoza.panels.v1":       true,
	"spinoza.layout.v1":       true,
	"spinoza.tables.v1":       true,
	"spinoza.sidebar.v1":      true,
	"spinoza.settings.v1":     true,
	"spinoza.painted.v1":      true,
	"spinoza.nodeshell.v1":    true,
	"spinoza.columns.v1":      true,
	"spinoza.update.check.v1": true,
	"spinoza.checks.rules.v1": true,
}

var personalSettingLimits = map[string]int{
	"spinoza.themes.v1":       32 << 10,
	"spinoza.tables.v1":       32 << 10,
	"spinoza.checks.rules.v1": 32 << 10,
}

type Settings interface {
	All() map[string]string
	Off(key string) bool
	Merge(values map[string]string) error
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
	return "<script>window.__SPINOZA_SETTINGS__=" + scriptValue(string(body)) + ";</script>"
}

func (s *Server) readSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, api.Settings{Values: s.settingsFor(r)})
}

func (s *Server) writeSettings(w http.ResponseWriter, r *http.Request) {
	var wanted api.Settings
	err := decodeJSONBody(w, r, maxSettingsBytes, &wanted)
	if err != nil {
		writeError(w, http.StatusBadRequest, "settings must be an object of strings")
		return
	}
	s.settingsWrite.Lock()
	defer s.settingsWrite.Unlock()
	values, valueErr := s.valuesToStore(r, wanted.Values)
	if valueErr != nil {
		writeError(w, http.StatusBadRequest, valueErr.Error())
		return
	}
	saveErr := s.stored().Merge(values)
	if saveErr != nil {
		writeError(w, http.StatusInternalServerError, saveErr.Error())
		return
	}
	writeJSON(w, api.Settings{Values: s.settingsFor(r)})
}

func servedSettings(values map[string]string) map[string]string {
	out := maps.Clone(values)
	delete(out, checks.MutesKey)
	delete(out, timelineDaysKey)
	for key := range out {
		if strings.HasPrefix(key, userSettingsPrefix) {
			delete(out, key)
		}
	}
	return out
}

func (s *Server) settingsFor(r *http.Request) map[string]string {
	values := s.stored().All()
	if !s.inCluster() {
		return servedSettings(values)
	}
	prefix, ok := settingsPrefix(r)
	if !ok {
		return map[string]string{}
	}
	out := map[string]string{}
	for key := range personalSettingKeys {
		value, found := values[prefix+key]
		if found {
			out[key] = value
		}
	}
	return out
}

func (s *Server) valuesToStore(r *http.Request, values map[string]string) (map[string]string, error) {
	for key := range values {
		if key == checks.MutesKey {
			return nil, errors.New("check mutes must be changed through the mutes endpoint")
		}
		if key == timelineDaysKey || strings.HasPrefix(key, userSettingsPrefix) {
			return nil, errors.New("deployment settings cannot be changed through the personal settings endpoint")
		}
	}
	if !s.inCluster() {
		return values, nil
	}
	prefix, ok := settingsPrefix(r)
	if !ok {
		return nil, errors.New("personal settings need a signed-in user")
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		if !personalSettingKeys[key] {
			return nil, fmt.Errorf("%q is not a personal setting", key)
		}
		limit := defaultPersonalSettingBytes
		if configured, found := personalSettingLimits[key]; found {
			limit = configured
		}
		if len(value) > limit {
			return nil, fmt.Errorf("%q is larger than %d bytes", key, limit)
		}
		out[prefix+key] = value
	}
	merged := map[string]string{}
	for key, value := range s.stored().All() {
		personalKey, found := strings.CutPrefix(key, prefix)
		if found {
			merged[personalKey] = value
		}
	}
	maps.Copy(merged, values)
	if personalSettingsSize(merged) > maxPersonalSettingsBytes {
		return nil, fmt.Errorf("personal settings are larger than %d bytes", maxPersonalSettingsBytes)
	}
	return out, nil
}

func personalSettingsSize(values map[string]string) int {
	total := 0
	for key, value := range values {
		total += len(key) + len(value)
	}
	return total
}

func settingsPrefix(r *http.Request) (string, bool) {
	who, ok := auth.IdentityFrom(r.Context())
	if !ok || who.User == "" {
		return "", false
	}
	sum := sha256.Sum256([]byte(who.User))
	return fmt.Sprintf("%s%x.", userSettingsPrefix, sum), true
}

func (s *Server) settingFor(r *http.Request, values map[string]string, key string) string {
	if !s.inCluster() {
		return values[key]
	}
	prefix, ok := settingsPrefix(r)
	if !ok {
		return ""
	}
	return values[prefix+key]
}
