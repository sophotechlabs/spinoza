package server

import (
	"bytes"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/telemetry"
	"github.com/sophotechlabs/spinoza/internal/version"
)

const redacted = "[redacted]"

var secretish = []string{"secret", "token", "password", "credential", "key"}

type supportBundle struct {
	Taken     string            `json:"taken"`
	Version   string            `json:"version"`
	Go        string            `json:"go"`
	Platform  string            `json:"platform"`
	Serving   bool              `json:"servingACluster"`
	Arguments []string          `json:"arguments"`
	Ready     api.Ready         `json:"ready"`
	Clusters  []supportCluster  `json:"clusters"`
	Memory    api.Memory        `json:"memory"`
	Counts    map[string]int    `json:"counts"`
	Recording map[string]bool   `json:"recording"`
	Metrics   string            `json:"metrics"`
	Leftout   []string          `json:"whatThisDoesNotCarry"`
	Settings  map[string]string `json:"settingKeys"`
}

type supportCluster struct {
	Name      string `json:"name"`
	Reachable bool   `json:"reachable"`
	Kinds     int    `json:"kinds"`
	Syncing   bool   `json:"informersStillSyncing"`
}

func (s *Server) handleSupport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Disposition", `attachment; filename="spinoza-support.json"`)
	writeJSON(w, s.supportBundle())
}

func (s *Server) supportBundle() supportBundle {
	var held runtime.MemStats
	runtime.ReadMemStats(&held)
	bundle := supportBundle{
		Taken:     s.instant().UTC().Format(time.RFC3339),
		Version:   version.String(),
		Go:        runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
		Serving:   s.inCluster(),
		Arguments: redactArguments(os.Args),
		Ready:     s.Ready(),
		Clusters:  s.supportClusters(),
		Memory:    api.Memory{HeapMi: int64(held.HeapAlloc / bytesPerMi), SysMi: int64(held.Sys / bytesPerMi)},
		Counts:    s.supportCounts(),
		Recording: map[string]bool{"sessions": s.transcriptStore().On()},
		Metrics:   supportMetrics(),
		Settings:  supportSettingKeys(s.stored().All()),
		Leftout: []string{
			"no cluster object, log line or terminal transcript",
			"no kubeconfig, token, session cookie or client secret",
			"the value of every setting; only which settings are set",
		},
	}
	return bundle
}

func (s *Server) supportClusters() []supportCluster {
	opened := s.cluster.Opened()
	out := make([]supportCluster, 0, len(opened))
	for _, open := range opened {
		one := supportCluster{Name: nameOf(open), Reachable: false}
		backend := s.managerOf(open.ID)
		if backend != nil {
			one.Reachable = true
			one.Kinds = len(backend.Resources().Categories)
			one.Syncing = backend.Syncing()
		}
		out = append(out, one)
	}
	return out
}

func (s *Server) supportCounts() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]int{
		"openFeeds":     len(s.sessions),
		"openTerminals": len(s.terminals),
	}
}

func supportMetrics() string {
	var page bytes.Buffer
	err := telemetry.Default().Registry.Write(&page)
	if err != nil {
		return ""
	}
	return page.String()
}

func supportSettingKeys(values map[string]string) map[string]string {
	out := map[string]string{}
	people := map[string]bool{}
	for key, value := range values {
		if !strings.HasPrefix(key, userSettingsPrefix) {
			out[key] = supportShape(value)
			continue
		}
		who, setting, split := strings.Cut(strings.TrimPrefix(key, userSettingsPrefix), ".")
		if !split {
			out[userSettingsPrefix+"<somebody>"] = supportShape(value)
			continue
		}
		people[who] = true
		out["<somebody>."+setting] = supportShape(value)
	}
	if len(people) > 0 {
		out["<people with settings of their own>"] = strconv.Itoa(len(people))
	}
	return out
}

func supportShape(value string) string {
	if value == "" {
		return "empty"
	}
	return "set, " + lengthWord(len(value))
}

func lengthWord(size int) string {
	if size < 1024 {
		return "under a kilobyte"
	}
	return "over a kilobyte"
}

func redactArguments(args []string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		out = append(out, redactArgument(arg))
	}
	return out
}

func redactArgument(arg string) string {
	name, value, split := strings.Cut(arg, "=")
	if !split {
		return arg
	}
	if looksSecret(name) {
		return name + "=" + redacted
	}
	return name + "=" + redactURL(value)
}

func looksSecret(name string) bool {
	lowered := strings.ToLower(name)
	for _, word := range secretish {
		if strings.Contains(lowered, word) {
			return true
		}
	}
	return false
}

func redactURL(value string) string {
	if !strings.Contains(value, "://") {
		return value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return redacted
	}
	if parsed.User != nil {
		parsed.User = url.User(redacted)
	}
	if parsed.RawQuery != "" {
		parsed.RawQuery = redacted
	}
	return parsed.String()
}
