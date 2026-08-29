package cluster

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

const resolvedRoot = "/resolved"

type stubProtection struct {
	verdicts map[string]string
	set      map[string]bool
	setErr   error
}

func newStubProtection() *stubProtection {
	return &stubProtection{verdicts: map[string]string{}, set: map[string]bool{}}
}

func (s *stubProtection) Verdict(server string) string {
	verdict, found := s.verdicts[server]
	if !found {
		return api.ProtectionUnknown
	}
	return verdict
}

func (s *stubProtection) Set(server string, protected bool) error {
	if s.setErr != nil {
		return s.setErr
	}
	s.set[server] = protected
	return nil
}

type stubSources struct {
	entries   []api.Kubeconfig
	added     []string
	removed   []string
	addErr    error
	removeErr error
}

func newStubSources() *stubSources {
	return &stubSources{entries: []api.Kubeconfig{{
		Label: "default",
		Contexts: []api.KubeContext{
			{Name: "alpha", Cluster: "c1"},
			{Name: "beta", Cluster: "c1"},
		},
	}}}
}

func (s *stubSources) List() []api.Kubeconfig {
	return s.entries
}

func (s *stubSources) Add(path string) error {
	if s.addErr != nil {
		return s.addErr
	}
	s.added = append(s.added, path)
	s.entries = append(s.entries, api.Kubeconfig{Label: path, Path: path, Removable: true})
	return nil
}

func (s *stubSources) Remove(path string) error {
	if s.removeErr != nil {
		return s.removeErr
	}
	s.removed = append(s.removed, path)
	return nil
}

func (s *stubSources) Resolve(path string) (string, error) {
	if path == "" {
		return "", errors.New("a kubeconfig path is required")
	}
	if filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Join(resolvedRoot, path), nil
}

type recorder struct {
	refs    []api.ContextRef
	live    []context.Context
	failOn  string
	failErr error
}

func (r *recorder) build(ctx context.Context, ref api.ContextRef) (*connection, error) {
	if r.failErr != nil && ref.Name == r.failOn {
		return nil, r.failErr
	}
	r.refs = append(r.refs, ref)
	r.live = append(r.live, ctx)
	resolved := ref
	if resolved.Name == "" {
		resolved.Name = "default-context"
	}
	return &connection{
		manager: resources.NewManager(ctx, resources.Deps{}),
		ref:     resolved,
		host:    "https://" + resolved.Name + ":6443",
	}, nil
}

func newTestCluster(t *testing.T, rec *recorder) *Cluster {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return newCluster(ctx, rec.build, newStubSources(), newStubProtection())
}

func TestNewBuildsTheDefaultContext(t *testing.T) {
	rec := &recorder{}

	cluster := newTestCluster(t, rec)

	if len(rec.refs) != 1 || rec.refs[0] != (api.ContextRef{}) {
		t.Fatalf("built %v, want one build of the kubeconfig default", rec.refs)
	}
	if cluster.Current().Name != "default-context" {
		t.Fatalf("current = %q", cluster.Current().Name)
	}
	if cluster.Manager("") == nil {
		t.Fatal("no manager after construction")
	}
}

func TestUseSwapsTheManager(t *testing.T) {
	rec := &recorder{}
	cluster := newTestCluster(t, rec)
	first := cluster.Manager("")

	err := cluster.Use(api.ContextRef{Name: "p-mk1"})
	if err != nil {
		t.Fatalf("use: %v", err)
	}

	if cluster.Manager("") == first {
		t.Fatal("the manager was not replaced")
	}
	if cluster.Current().Name != "p-mk1" {
		t.Fatalf("current = %q", cluster.Current().Name)
	}
}

func TestUseCarriesTheKubeconfigTheContextCameFrom(t *testing.T) {
	rec := &recorder{}
	cluster := newTestCluster(t, rec)

	err := cluster.Use(api.ContextRef{Kubeconfig: "/tmp/other.yaml", Name: "beta"})
	if err != nil {
		t.Fatalf("use: %v", err)
	}

	if rec.refs[1].Kubeconfig != "/tmp/other.yaml" {
		t.Fatalf("built %v, want the file the context was picked from", rec.refs[1])
	}
	if cluster.Current().Kubeconfig != "/tmp/other.yaml" {
		t.Fatalf("current = %v, want the kubeconfig kept with the context", cluster.Current())
	}
}

func TestUseCancelsThePreviousManager(t *testing.T) {
	rec := &recorder{}
	cluster := newTestCluster(t, rec)

	err := cluster.Use(api.ContextRef{Name: "p-mk1"})
	if err != nil {
		t.Fatalf("use: %v", err)
	}

	select {
	case <-rec.live[0].Done():
	default:
		t.Fatal("the old context stayed live; its informers and forwards would keep running")
	}
	select {
	case <-rec.live[1].Done():
		t.Fatal("the new context was canceled")
	default:
	}
}

func TestAFailedUseKeepsTheWorkingManager(t *testing.T) {
	rec := &recorder{failOn: "gone", failErr: errors.New("context \"gone\" does not exist")}
	cluster := newTestCluster(t, rec)
	before := cluster.Manager("")

	err := cluster.Use(api.ContextRef{Name: "gone"})

	if err == nil {
		t.Fatal("expected the failure to surface")
	}
	if cluster.Manager("") != before {
		t.Fatal("a failed switch replaced the working manager")
	}
	if cluster.Current().Name != "default-context" {
		t.Fatalf("current = %q, want the old context kept", cluster.Current().Name)
	}
	select {
	case <-rec.live[0].Done():
		t.Fatal("a failed switch canceled the working manager")
	default:
	}
}

func TestAnUnreachableClusterStillStarts(t *testing.T) {
	rec := &recorder{failOn: "", failErr: errors.New("kubeconfig is unreadable")}

	cluster := newCluster(context.Background(), rec.build, newStubSources(), newStubProtection())

	if cluster == nil {
		t.Fatal("spinoza refused to start without a cluster, so the context picker is unreachable")
	}
	if cluster.Manager("") != nil {
		t.Fatal("a cluster that never connected handed out a manager")
	}
	list := cluster.Contexts()
	if !strings.Contains(list.Error, "kubeconfig is unreadable") {
		t.Fatalf("error = %q, want the startup failure carried to the picker", list.Error)
	}
	if len(list.Kubeconfigs[0].Contexts) == 0 {
		t.Fatal("the picker has no contexts to offer")
	}
}

func TestPickingAWorkingContextClearsTheStartupFailure(t *testing.T) {
	rec := &recorder{failOn: "", failErr: errors.New("kubeconfig is unreadable")}
	cluster := newCluster(context.Background(), rec.build, newStubSources(), newStubProtection())
	rec.failErr = nil

	if err := cluster.Use(api.ContextRef{Name: "p-mk2"}); err != nil {
		t.Fatalf("use: %v", err)
	}

	if cluster.Manager("") == nil {
		t.Fatal("the recovered context handed out no manager")
	}
	if cluster.Contexts().Error != "" {
		t.Fatalf("error = %q, want it cleared once a context answered", cluster.Contexts().Error)
	}
}

func TestContextsReportsTheCurrentSelection(t *testing.T) {
	rec := &recorder{}
	cluster := newTestCluster(t, rec)

	list := cluster.Contexts()

	if list.Current.Name != "default-context" {
		t.Fatalf("current = %q, want the context spinoza actually connected to", list.Current.Name)
	}
	if len(list.Kubeconfigs) != 1 {
		t.Fatalf("kubeconfigs = %v, want the ones spinoza reads", list.Kubeconfigs)
	}
}

func TestAddingAKubeconfigReachesTheSources(t *testing.T) {
	sources := newStubSources()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cluster := newCluster(ctx, (&recorder{}).build, sources, newStubProtection())

	err := cluster.AddKubeconfig("/tmp/other.yaml")
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	if !slices.Equal(sources.added, []string{"/tmp/other.yaml"}) {
		t.Fatalf("added = %v", sources.added)
	}
	if len(cluster.Contexts().Kubeconfigs) != 2 {
		t.Fatalf("kubeconfigs = %v, want the added one listed", cluster.Contexts().Kubeconfigs)
	}
}

func TestAKubeconfigThatIsRefusedIsNotAdded(t *testing.T) {
	sources := newStubSources()
	sources.addErr = errors.New("that file is not a kubeconfig")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cluster := newCluster(ctx, (&recorder{}).build, sources, newStubProtection())

	err := cluster.AddKubeconfig("/tmp/notes.txt")

	if err == nil {
		t.Fatal("a file that is not a kubeconfig was accepted")
	}
	if len(cluster.Contexts().Kubeconfigs) != 1 {
		t.Fatalf("kubeconfigs = %v", cluster.Contexts().Kubeconfigs)
	}
}

func TestRemovingAKubeconfigResolvesThePathFirst(t *testing.T) {
	sources := newStubSources()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cluster := newCluster(ctx, (&recorder{}).build, sources, newStubProtection())

	err := cluster.RemoveKubeconfig("other.yaml")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}

	if !slices.Equal(sources.removed, []string{filepath.Join(resolvedRoot, "other.yaml")}) {
		t.Fatalf("removed = %v, want the resolved path", sources.removed)
	}
}

func TestAPathThatCannotBeResolvedIsNotRemoved(t *testing.T) {
	sources := newStubSources()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cluster := newCluster(ctx, (&recorder{}).build, sources, newStubProtection())

	err := cluster.RemoveKubeconfig("")

	if err == nil {
		t.Fatal("an empty path was accepted as something to remove")
	}
	if len(sources.removed) != 0 {
		t.Fatalf("removed = %v", sources.removed)
	}
}

func TestTheKubeconfigInUseIsNotRemoved(t *testing.T) {
	sources := newStubSources()
	sources.entries = append(sources.entries, api.Kubeconfig{
		Label:     "/tmp/other.yaml",
		Path:      "/tmp/other.yaml",
		Removable: true,
		Contexts:  []api.KubeContext{{Name: "beta", Cluster: "c2"}},
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cluster := newCluster(ctx, (&recorder{}).build, sources, newStubProtection())
	if err := cluster.Use(api.ContextRef{Kubeconfig: "/tmp/other.yaml", Name: "beta"}); err != nil {
		t.Fatalf("use: %v", err)
	}

	err := cluster.RemoveKubeconfig("/tmp/other.yaml")

	if err == nil {
		t.Fatal("the kubeconfig spinoza is connected through was removed under it")
	}
	if len(sources.removed) != 0 {
		t.Fatalf("removed = %v", sources.removed)
	}
}

func TestAKubeconfigThatWentMissingCanBeRemovedEvenWhileInUse(t *testing.T) {
	sources := newStubSources()
	sources.entries = append(sources.entries, api.Kubeconfig{
		Label:     "/tmp/other.yaml",
		Path:      "/tmp/other.yaml",
		Removable: true,
		Error:     "kubeconfig: stat /tmp/other.yaml: no such file or directory",
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cluster := newCluster(ctx, (&recorder{}).build, sources, newStubProtection())
	if err := cluster.Use(api.ContextRef{Kubeconfig: "/tmp/other.yaml", Name: "beta"}); err != nil {
		t.Fatalf("use: %v", err)
	}

	err := cluster.RemoveKubeconfig("/tmp/other.yaml")
	if err != nil {
		t.Fatalf("remove: %v; a file that no longer reads cannot be switched away from either", err)
	}
	if !slices.Equal(sources.removed, []string{"/tmp/other.yaml"}) {
		t.Fatalf("removed = %v", sources.removed)
	}
}

func TestAKubeconfigSpinozaNeverHeardOfIsNotProtected(t *testing.T) {
	sources := newStubSources()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cluster := newCluster(ctx, (&recorder{}).build, sources, newStubProtection())
	if err := cluster.Use(api.ContextRef{Kubeconfig: "/tmp/gone.yaml", Name: "beta"}); err != nil {
		t.Fatalf("use: %v", err)
	}

	err := cluster.RemoveKubeconfig("/tmp/gone.yaml")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
}

func TestContextsSurfacesAKubeconfigFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	sources := newStubSources()
	sources.entries = []api.Kubeconfig{{Label: "default", Error: "kubeconfig is unreadable"}}
	cluster := newCluster(ctx, (&recorder{}).build, sources, newStubProtection())

	list := cluster.Contexts()

	if list.Kubeconfigs[0].Error == "" {
		t.Fatal("an unreadable kubeconfig was reported as an empty context list")
	}
	if list.Current.Name != "default-context" {
		t.Fatalf("current = %q, want the connected context even when listing fails", list.Current.Name)
	}
}

type gatedBuilder struct {
	gates   map[string]chan struct{}
	entered chan string
}

func newGatedBuilder(slow string) *gatedBuilder {
	return &gatedBuilder{
		gates:   map[string]chan struct{}{slow: make(chan struct{})},
		entered: make(chan string, 4),
	}
}

func (g *gatedBuilder) build(ctx context.Context, ref api.ContextRef) (*connection, error) {
	g.entered <- ref.Name
	gate, ok := g.gates[ref.Name]
	if ok {
		<-gate
	}
	return &connection{manager: resources.NewManager(ctx, resources.Deps{}), ref: ref}, nil
}

func (g *gatedBuilder) waitFor(t *testing.T, name string) {
	t.Helper()
	for {
		select {
		case entered := <-g.entered:
			if entered == name {
				return
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("the builder was never asked for %q", name)
		}
	}
}

func TestTheLastRequestedContextWinsEvenIfItBuildsFirst(t *testing.T) {
	gated := newGatedBuilder("slow")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cluster := newCluster(ctx, gated.build, newStubSources(), newStubProtection())

	done := make(chan error, 1)
	go func() {
		done <- cluster.Use(api.ContextRef{Name: "slow"})
	}()
	gated.waitFor(t, "slow")

	fastErr := cluster.Use(api.ContextRef{Name: "fast"})
	if fastErr != nil {
		t.Fatalf("use fast: %v", fastErr)
	}

	close(gated.gates["slow"])
	slowErr := <-done
	if slowErr != nil {
		t.Fatalf("use slow: %v", slowErr)
	}

	if cluster.Current().Name != "fast" {
		t.Fatalf("current = %q, want the context requested last", cluster.Current().Name)
	}
}

func TestASupersededSwitchDoesNotStrandItsManager(t *testing.T) {
	gated := newGatedBuilder("slow")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cluster := newCluster(ctx, gated.build, newStubSources(), newStubProtection())

	done := make(chan error, 1)
	go func() {
		done <- cluster.Use(api.ContextRef{Name: "slow"})
	}()
	gated.waitFor(t, "slow")

	useErr := cluster.Use(api.ContextRef{Name: "fast"})
	if useErr != nil {
		t.Fatalf("use fast: %v", useErr)
	}
	winner := cluster.Manager("")

	close(gated.gates["slow"])
	if slowErr := <-done; slowErr != nil {
		t.Fatalf("use slow: %v", slowErr)
	}

	if cluster.Manager("") != winner {
		t.Fatal("a superseded switch replaced the installed manager")
	}
}

func TestAFailedSwitchKeepsTheWorkingCluster(t *testing.T) {
	rec := &recorder{failOn: "broken", failErr: errors.New("context \"broken\" lists no resource types")}
	cluster := newTestCluster(t, rec)
	working := cluster.Manager("")

	err := cluster.Use(api.ContextRef{Name: "broken"})

	if err == nil {
		t.Fatal("switching to an unusable context reported success")
	}
	if cluster.Manager("") != working {
		t.Fatal("the working cluster's manager was replaced by the unusable one")
	}
	if cluster.Current().Name != "default-context" {
		t.Fatalf("current = %q, want the working context to still be in force", cluster.Current().Name)
	}
}

func TestAFailedSwitchLeavesTheWorkingInformersRunning(t *testing.T) {
	rec := &recorder{failOn: "broken", failErr: errors.New("unreachable")}
	cluster := newTestCluster(t, rec)
	live := rec.live[0]

	useErr := cluster.Use(api.ContextRef{Name: "broken"})
	if useErr == nil {
		t.Fatal("expected the switch to fail")
	}

	select {
	case <-live.Done():
		t.Fatal("the working cluster's context was canceled by a switch that never took effect")
	default:
	}
}

func TestTheProtectionOfTheClusterInUseIsReported(t *testing.T) {
	protection := newStubProtection()
	protection.verdicts["https://default-context:6443"] = api.ProtectionProtected
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cluster := newCluster(ctx, (&recorder{}).build, newStubSources(), protection)

	if cluster.Contexts().Protection != api.ProtectionProtected {
		t.Fatalf("protection = %q, want the verdict for the cluster spinoza connected to", cluster.Contexts().Protection)
	}
	if !cluster.Protected() {
		t.Fatal("the gate on destructive actions would be open")
	}
}

func TestProtectionFollowsTheClusterNotTheContextName(t *testing.T) {
	protection := newStubProtection()
	protection.verdicts["https://p-mk1:6443"] = api.ProtectionProtected
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cluster := newCluster(ctx, (&recorder{}).build, newStubSources(), protection)

	if cluster.Protected() {
		t.Fatal("the default context was protected by another cluster's answer")
	}
	if err := cluster.Use(api.ContextRef{Kubeconfig: "/tmp/other.yaml", Name: "p-mk1"}); err != nil {
		t.Fatalf("use: %v", err)
	}

	if !cluster.Protected() {
		t.Fatal("the same cluster reached through another kubeconfig lost its protection")
	}
}

func TestProtectingRemembersTheClusterInUse(t *testing.T) {
	protection := newStubProtection()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cluster := newCluster(ctx, (&recorder{}).build, newStubSources(), protection)

	err := cluster.Protect(true)
	if err != nil {
		t.Fatalf("protect: %v", err)
	}

	if !protection.set["https://default-context:6443"] {
		t.Fatalf("set = %v, want the server spinoza is connected to", protection.set)
	}
}

func TestProtectionThatCannotBeSavedSurfaces(t *testing.T) {
	protection := newStubProtection()
	protection.setErr = errors.New("read-only file system")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cluster := newCluster(ctx, (&recorder{}).build, newStubSources(), protection)

	err := cluster.Protect(true)

	if err == nil {
		t.Fatal("a protection that was never written reported success")
	}
}

func TestAClusterThatNeverConnectedIsNotProtected(t *testing.T) {
	rec := &recorder{failOn: "", failErr: errors.New("kubeconfig is unreadable")}
	cluster := newCluster(context.Background(), rec.build, newStubSources(), newStubProtection())

	if cluster.Protected() {
		t.Fatal("an unconnected cluster reported itself protected")
	}
	if cluster.Contexts().Protection != api.ProtectionUnknown {
		t.Fatalf("protection = %q", cluster.Contexts().Protection)
	}
}

func TestTheConnectedHostIsNormalisedBeforeAnythingKeysOnIt(t *testing.T) {
	protection := newStubProtection()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	build := func(buildCtx context.Context, ref api.ContextRef) (*connection, error) {
		return &connection{
			manager: resources.NewManager(buildCtx, resources.Deps{}),
			ref:     api.ContextRef{Name: "prod"},
			host:    "HTTPS://Prod.Example.COM:6443/",
		}, nil
	}
	cluster := newCluster(ctx, build, newStubSources(), protection)

	if err := cluster.Protect(true); err != nil {
		t.Fatalf("protect: %v", err)
	}

	if !protection.set["https://prod.example.com:6443"] {
		t.Fatalf("protection keyed on %v, want the normalised api server url", protection.set)
	}
}

func TestAskingByTheActiveIdGivesTheSameManager(t *testing.T) {
	cluster := newTestCluster(t, &recorder{})

	if cluster.Manager(cluster.ID()) != cluster.Manager("") {
		t.Fatal("naming the connected cluster gave a different manager than asking for the active one")
	}
}

func TestAskingForAClusterNobodyOpenedGivesNothing(t *testing.T) {
	cluster := newTestCluster(t, &recorder{})

	if cluster.Manager("https://nobody-opened-this:6443") != nil {
		t.Fatal("a cluster that was never opened answered with a manager")
	}
}

func TestAnIdIsNormalisedOnTheWayIn(t *testing.T) {
	cluster := newTestCluster(t, &recorder{})

	if cluster.Manager("HTTPS://Default-Context:6443/") != cluster.Manager("") {
		t.Fatal("a differently spelt id missed the connection it names")
	}
}

func TestOnlyOneClusterStaysOpenAcrossASwitch(t *testing.T) {
	cluster := newTestCluster(t, &recorder{})
	if err := cluster.Use(api.ContextRef{Name: "beta"}); err != nil {
		t.Fatalf("use: %v", err)
	}

	cluster.mu.Lock()
	open := len(cluster.open)
	active := cluster.active
	cluster.mu.Unlock()

	if open != 1 {
		t.Fatalf("%d connections open after a switch, want 1 until tabs exist", open)
	}
	if active != "https://beta:6443" {
		t.Fatalf("active = %q, want the cluster just switched to", active)
	}
}

func TestSwitchingBackToTheSameClusterKeepsItOpen(t *testing.T) {
	rec := &recorder{}
	cluster := newTestCluster(t, rec)
	first := cluster.Manager("")

	if err := cluster.Use(api.ContextRef{}); err != nil {
		t.Fatalf("use: %v", err)
	}

	if cluster.Manager("") == first {
		t.Fatal("reconnecting to the same cluster reused the old manager; Use rebuilds")
	}
	cluster.mu.Lock()
	open := len(cluster.open)
	cluster.mu.Unlock()
	if open != 1 {
		t.Fatalf("%d connections open, want the replacement to have retired the original", open)
	}
}
