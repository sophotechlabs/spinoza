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
	"github.com/sophotechlabs/spinoza/internal/auth"
	"github.com/sophotechlabs/spinoza/internal/settings"
	"github.com/sophotechlabs/spinoza/internal/version"
)

const (
	maxDocBytes  = 4 << 20
	queryTrue    = "true"
	ociScheme    = "oci://"
	clusterParam = "cluster"
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
	Protect(cluster string, protected bool) error
	Protected(cluster string) bool
}

type Connections interface {
	Manager(id string) Backend
	Open(ref api.ContextRef) (string, error)
	Close(id string) error
	Activate(id string) error
	Opened() []api.OpenCluster
	ID() string
}

type Cluster interface {
	Elsewhere
	Kubeconfigs
	Guarded
	Connections
	Contexts() api.ContextList
	Use(ref api.ContextRef) error
}

type FilePicker func(ctx context.Context) (string, error)

const noFilePicker = "only the desktop window can open a file dialog; type the path instead"

type Server struct {
	cluster       Cluster
	assets        fs.FS
	files         http.Handler
	token         string
	mu            sync.Mutex
	picker        FilePicker
	localShell    LocalShellOpener
	settings      Settings
	baseline      Baselines
	window        Window
	browser       BrowserOpener
	views         views
	sessions      map[*wsSession]struct{}
	terminals     map[*websocket.Conn]string
	profiler      bool
	health        map[string]api.ClusterHealth
	misses        map[string]int
	start         startRoute
	watching      bool
	updates       Updates
	installer     Installs
	past          History
	open          Tabs
	taping        map[string]*recording
	now           func() time.Time
	pingEvery     time.Duration
	feedPingEvery time.Duration
	feedPingWait  time.Duration
	authn         *auth.Authenticator
	publicOrigin  string
	served        bool
}

func New(cluster Cluster, assets fs.FS, token string) *Server {
	return &Server{
		cluster:       cluster,
		assets:        assets,
		files:         http.FileServerFS(assets),
		token:         token,
		settings:      settings.Memory(),
		baseline:      noBaselines{},
		sessions:      map[*wsSession]struct{}{},
		terminals:     map[*websocket.Conn]string{},
		health:        map[string]api.ClusterHealth{},
		misses:        map[string]int{},
		taping:        map[string]*recording{},
		now:           time.Now,
		pingEvery:     defaultPingInterval,
		feedPingEvery: defaultFeedPingInterval,
		feedPingWait:  defaultFeedPingTimeout,
		views:         views{grace: defaultIdleGrace, await: defaultBrowserAwait},
	}
}

func clusterOf(r *http.Request) string {
	return r.URL.Query().Get(clusterParam)
}

func (s *Server) managerFor(r *http.Request) Reader {
	backend, _ := s.lookup(clusterOf(r))
	return backend
}

func (s *Server) managerOf(id string) Backend {
	return s.cluster.Manager(id)
}

func (s *Server) writerOf(id string) Writer {
	return s.cluster.Manager(id)
}

type clusterLookup func(id string) (Reader, string)

func (s *Server) clusterKey(r *http.Request) string {
	_, on := s.lookup(clusterOf(r))
	return on
}

func (s *Server) lookup(id string) (Reader, string) {
	on := id
	if on == "" {
		on = s.cluster.ID()
	}
	return s.managerOf(on), on
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
	writeJSON(w, api.Health{Status: "ok"})
}

func handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, api.Build{Version: version.String()})
}
