package server

import (
	"context"
	"io/fs"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/settings"
	"github.com/sophotechlabs/spinoza/internal/version"
)

const (
	maxDocBytes = 4 << 20
	queryTrue   = "true"
	ociScheme   = "oci://"
)

type Elsewhere interface {
	Read(ctx context.Context, ref api.ContextRef, target api.ObjectRef) (string, error)
	List(ctx context.Context, ref api.ContextRef, target api.ObjectRef) ([]*unstructured.Unstructured, error)
}

type Kubeconfigs interface {
	AddKubeconfig(path string) error
	RemoveKubeconfig(path string) error
}

type Guarded interface {
	Protect(protected bool) error
	Protected() bool
}

type Cluster interface {
	Elsewhere
	Kubeconfigs
	Guarded
	Manager() Backend
	Contexts() api.ContextList
	Use(ref api.ContextRef) error
	ID() string
}

type FilePicker func(ctx context.Context) (string, error)

const noFilePicker = "only the desktop window can open a file dialog; type the path instead"

type Server struct {
	cluster    Cluster
	assets     fs.FS
	files      http.Handler
	token      string
	mu         sync.Mutex
	picker     FilePicker
	localShell LocalShellOpener
	settings   Settings
	window     Window
	browser    BrowserOpener
	views      views
	sessions   map[*wsSession]struct{}
	terminals  map[*websocket.Conn]struct{}
	profiler   bool
	health     api.ClusterHealth
	watching   bool
	updates    Updates
	installer  Installs
	past       History
	now        func() time.Time
	pingEvery  time.Duration
}

func New(cluster Cluster, assets fs.FS, token string) *Server {
	return &Server{
		cluster:   cluster,
		assets:    assets,
		files:     http.FileServerFS(assets),
		token:     token,
		settings:  settings.Memory(),
		sessions:  map[*wsSession]struct{}{},
		terminals: map[*websocket.Conn]struct{}{},
		health:    assumedHealth(),
		now:       time.Now,
		pingEvery: defaultPingInterval,
		views:     views{grace: defaultIdleGrace, await: defaultBrowserAwait},
	}
}

func (s *Server) manager() Backend {
	return s.cluster.Manager()
}

func (s *Server) UseFilePicker(picker FilePicker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.picker = picker
}

func (s *Server) UseProfiler(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.profiler = enabled
}

func (s *Server) wantsProfiler() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.profiler
}

func (s *Server) filePicker() FilePicker {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.picker
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, api.Health{
		Status:  "ok",
		Version: version.String(),
		Context: s.cluster.Contexts().Current.Name,
	})
}

func handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, api.Build{Version: version.String()})
}
