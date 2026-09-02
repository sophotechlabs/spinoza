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

const testOpenTimeout = 2 * time.Second

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

type pausedSources struct {
	*stubSources

	listed chan struct{}
	resume chan struct{}
}

func (s *pausedSources) List() []api.Kubeconfig {
	close(s.listed)
	<-s.resume
	return s.stubSources.List()
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
	return newCluster(ctx, rec.build, newStubSources(), newStubProtection(), testOpenTimeout, api.ContextRef{})
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

func TestCancelingTheRootStopsWaitingForAStalledOpen(t *testing.T) {
	root, cancel := context.WithCancel(context.Background())
	entered := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	held := &Cluster{
		openWithin: testOpenTimeout,
		build: func(context.Context, api.ContextRef) (*connection, error) {
			close(entered)
			<-release
			return &connection{host: "https://p-mk2:6443"}, nil
		},
	}
	done := make(chan error, 1)
	go func() {
		_, err := held.dial(root, api.ContextRef{Name: "p-mk2"})
		done <- err
	}()
	<-entered
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want the root cancellation", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("a stalled open kept waiting after the cluster root was canceled")
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

	cluster := newCluster(context.Background(), rec.build, newStubSources(), newStubProtection(), testOpenTimeout, api.ContextRef{})

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
	cluster := newCluster(context.Background(), rec.build, newStubSources(), newStubProtection(), testOpenTimeout, api.ContextRef{})
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
	cluster := newCluster(ctx, (&recorder{}).build, sources, newStubProtection(), testOpenTimeout, api.ContextRef{})

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
	cluster := newCluster(ctx, (&recorder{}).build, sources, newStubProtection(), testOpenTimeout, api.ContextRef{})

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
	cluster := newCluster(ctx, (&recorder{}).build, sources, newStubProtection(), testOpenTimeout, api.ContextRef{})

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
	cluster := newCluster(ctx, (&recorder{}).build, sources, newStubProtection(), testOpenTimeout, api.ContextRef{})

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
	cluster := newCluster(ctx, (&recorder{}).build, sources, newStubProtection(), testOpenTimeout, api.ContextRef{})
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
	cluster := newCluster(ctx, (&recorder{}).build, sources, newStubProtection(), testOpenTimeout, api.ContextRef{})
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
	cluster := newCluster(ctx, (&recorder{}).build, sources, newStubProtection(), testOpenTimeout, api.ContextRef{})
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
	cluster := newCluster(ctx, (&recorder{}).build, sources, newStubProtection(), testOpenTimeout, api.ContextRef{})

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
	cluster := newCluster(ctx, gated.build, newStubSources(), newStubProtection(), testOpenTimeout, api.ContextRef{})

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
	cluster := newCluster(ctx, gated.build, newStubSources(), newStubProtection(), testOpenTimeout, api.ContextRef{})

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
	cluster := newCluster(ctx, (&recorder{}).build, newStubSources(), protection, testOpenTimeout, api.ContextRef{})

	if cluster.Contexts().Protection != api.ProtectionProtected {
		t.Fatalf("protection = %q, want the verdict for the cluster spinoza connected to", cluster.Contexts().Protection)
	}
	if !cluster.Protected(cluster.ID()) {
		t.Fatal("the gate on destructive actions would be open")
	}
}

func TestContextsKeepsTheCurrentContextAndProtectionTogether(t *testing.T) {
	protection := newStubProtection()
	sources := &pausedSources{
		stubSources: newStubSources(),
		listed:      make(chan struct{}),
		resume:      make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cluster := newCluster(ctx, (&recorder{}).build, sources, protection, testOpenTimeout, api.ContextRef{})
	first := cluster.ID()
	second, err := cluster.Open(api.ContextRef{Name: "p-mk2"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := cluster.Activate(first); err != nil {
		t.Fatalf("activate first: %v", err)
	}
	protection.verdicts[first] = api.ProtectionProtected
	protection.verdicts[second] = api.ProtectionOpen

	got := make(chan api.ContextList, 1)
	go func() {
		got <- cluster.Contexts()
	}()
	<-sources.listed
	if err := cluster.Activate(second); err != nil {
		t.Fatalf("activate second: %v", err)
	}
	close(sources.resume)
	list := <-got

	if list.Current.Name != "default-context" {
		t.Fatalf("current = %q, want the context selected when the snapshot began", list.Current.Name)
	}
	if list.Protection != api.ProtectionProtected {
		t.Fatalf("protection = %q, want the verdict for %q", list.Protection, first)
	}
}

func TestProtectionFollowsTheClusterNotTheContextName(t *testing.T) {
	protection := newStubProtection()
	protection.verdicts["https://p-mk1:6443"] = api.ProtectionProtected
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cluster := newCluster(ctx, (&recorder{}).build, newStubSources(), protection, testOpenTimeout, api.ContextRef{})

	if cluster.Protected(cluster.ID()) {
		t.Fatal("the default context was protected by another cluster's answer")
	}
	if err := cluster.Use(api.ContextRef{Kubeconfig: "/tmp/other.yaml", Name: "p-mk1"}); err != nil {
		t.Fatalf("use: %v", err)
	}

	if !cluster.Protected(cluster.ID()) {
		t.Fatal("the same cluster reached through another kubeconfig lost its protection")
	}
}

func TestProtectingRemembersTheClusterInUse(t *testing.T) {
	protection := newStubProtection()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cluster := newCluster(ctx, (&recorder{}).build, newStubSources(), protection, testOpenTimeout, api.ContextRef{})

	err := cluster.Protect(cluster.ID(), true)
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
	cluster := newCluster(ctx, (&recorder{}).build, newStubSources(), protection, testOpenTimeout, api.ContextRef{})

	err := cluster.Protect(cluster.ID(), true)

	if err == nil {
		t.Fatal("a protection that was never written reported success")
	}
}

func TestAClusterThatNeverConnectedIsNotProtected(t *testing.T) {
	rec := &recorder{failOn: "", failErr: errors.New("kubeconfig is unreadable")}
	cluster := newCluster(context.Background(), rec.build, newStubSources(), newStubProtection(), testOpenTimeout, api.ContextRef{})

	if cluster.Protected(cluster.ID()) {
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
	cluster := newCluster(ctx, build, newStubSources(), protection, testOpenTimeout, api.ContextRef{})

	if err := cluster.Protect(cluster.ID(), true); err != nil {
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

func TestOpeningASecondClusterLeavesTheFirstOpen(t *testing.T) {
	cluster := newTestCluster(t, &recorder{})
	first := cluster.ID()

	second, err := cluster.Open(api.ContextRef{Name: "beta"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if cluster.Manager(first) == nil {
		t.Fatal("opening a second cluster closed the first")
	}
	if cluster.Manager(second) == nil {
		t.Fatal("the cluster just opened has no manager")
	}
	if cluster.ID() != second {
		t.Fatalf("active = %q, want the cluster just opened", cluster.ID())
	}
}

func TestOpeningAClusterThatIsAlreadyOpenFocusesIt(t *testing.T) {
	rec := &recorder{}
	cluster := newTestCluster(t, rec)
	first, err := cluster.Open(api.ContextRef{Name: "beta"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	held := cluster.Manager(first)

	again, againErr := cluster.Open(api.ContextRef{Name: "beta"})

	if againErr != nil {
		t.Fatalf("open: %v", againErr)
	}
	if again != first {
		t.Fatalf("id = %q, want the id already open", again)
	}
	if cluster.Manager(first) != held {
		t.Fatal("opening the same cluster twice replaced its manager; the tab should just be focused")
	}
	if len(cluster.Opened()) != 2 {
		t.Fatalf("%d open, want the default and beta; not a duplicate", len(cluster.Opened()))
	}
}

func TestOpeningAClusterThatRefusesLeavesNothingBehind(t *testing.T) {
	rec := &recorder{failOn: "beta", failErr: errors.New("no route to host")}
	cluster := newTestCluster(t, rec)
	before := len(cluster.Opened())

	_, err := cluster.Open(api.ContextRef{Name: "beta"})

	if err == nil {
		t.Fatal("opening a cluster that refused reported success")
	}
	if len(cluster.Opened()) != before {
		t.Fatalf("%d open after a failure, want %d", len(cluster.Opened()), before)
	}
}

func TestAnApiserverThatNeverAnswersIsAnErrorNotAHang(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	blocked := make(chan struct{})
	t.Cleanup(func() { close(blocked) })
	build := func(buildCtx context.Context, _ api.ContextRef) (*connection, error) {
		select {
		case <-buildCtx.Done():
			return nil, buildCtx.Err()
		case <-blocked:
			return nil, errors.New("gone")
		}
	}
	cluster := newCluster(ctx, build, newStubSources(), newStubProtection(), 20*time.Millisecond, api.ContextRef{})

	_, err := cluster.Open(api.ContextRef{Name: "wedged"})

	if err == nil {
		t.Fatal("an apiserver that never answered opened successfully")
	}
	if !strings.Contains(err.Error(), "did not answer") {
		t.Fatalf("error = %q, want it to say the cluster never answered", err.Error())
	}
}

func TestActivatingSwitchesWhichClusterIsCurrent(t *testing.T) {
	cluster := newTestCluster(t, &recorder{})
	first := cluster.ID()
	if _, err := cluster.Open(api.ContextRef{Name: "beta"}); err != nil {
		t.Fatalf("open: %v", err)
	}

	if err := cluster.Activate(first); err != nil {
		t.Fatalf("activate: %v", err)
	}

	if cluster.ID() != first {
		t.Fatalf("active = %q, want %q", cluster.ID(), first)
	}
	if cluster.Manager("") != cluster.Manager(first) {
		t.Fatal("the active manager is not the cluster that was activated")
	}
}

func TestActivatingAClusterNobodyOpenedSaysSo(t *testing.T) {
	cluster := newTestCluster(t, &recorder{})

	err := cluster.Activate("https://ghost:6443")

	if !errors.Is(err, api.ErrNotOpen) {
		t.Fatalf("error = %v, want it to report the cluster is not open", err)
	}
}

func TestActivatingAcceptsANonCanonicalSpelling(t *testing.T) {
	cluster := newTestCluster(t, &recorder{})

	if err := cluster.Activate("HTTPS://Default-Context:6443/"); err != nil {
		t.Fatalf("activate: %v, want the id normalised on the way in", err)
	}
}

func TestWhatIsOpenComesBackSortedAndFlagged(t *testing.T) {
	protection := newStubProtection()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cluster := newCluster(ctx, (&recorder{}).build, newStubSources(), protection, testOpenTimeout, api.ContextRef{})
	if _, err := cluster.Open(api.ContextRef{Name: "alpha"}); err != nil {
		t.Fatalf("open: %v", err)
	}
	protection.verdicts["https://alpha:6443"] = api.ProtectionProtected

	opened := cluster.Opened()

	if len(opened) != 2 {
		t.Fatalf("%d open, want the default and alpha", len(opened))
	}
	if opened[0].ID > opened[1].ID {
		t.Fatalf("order = %q then %q, want them sorted so the strip does not shuffle", opened[0].ID, opened[1].ID)
	}
	var alpha api.OpenCluster
	for _, one := range opened {
		if one.Context == "alpha" {
			alpha = one
		}
	}
	if !alpha.Active {
		t.Fatal("the cluster just opened is not marked active")
	}
	if alpha.Protection != api.ProtectionProtected {
		t.Fatalf("protection = %q, want it carried so the strip can mark it", alpha.Protection)
	}
	if alpha.Kubeconfig != "" || alpha.Context != "alpha" {
		t.Fatalf("entry = %+v, want the context it was opened from", alpha)
	}
}

func TestClosingAClusterDropsItsConnection(t *testing.T) {
	cluster := newTestCluster(t, &recorder{})
	beta, err := cluster.Open(api.ContextRef{Name: "beta"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if closeErr := cluster.Close(beta); closeErr != nil {
		t.Fatalf("close: %v", closeErr)
	}

	if cluster.Manager(beta) != nil {
		t.Fatal("a closed cluster still answers with a manager")
	}
	if len(cluster.Opened()) != 1 {
		t.Fatalf("%d open, want only the one that was not closed", len(cluster.Opened()))
	}
}

func TestClosingTheActiveClusterPromotesAnother(t *testing.T) {
	cluster := newTestCluster(t, &recorder{})
	first := cluster.ID()
	beta, err := cluster.Open(api.ContextRef{Name: "beta"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if closeErr := cluster.Close(beta); closeErr != nil {
		t.Fatalf("close: %v", closeErr)
	}

	if cluster.ID() != first {
		t.Fatalf("active = %q, want the cluster still open", cluster.ID())
	}
	if cluster.Manager("") == nil {
		t.Fatal("closing the active tab left nothing serving requests")
	}
}

func TestClosingAClusterThatIsNotActiveLeavesTheActiveAlone(t *testing.T) {
	cluster := newTestCluster(t, &recorder{})
	first := cluster.ID()
	beta, err := cluster.Open(api.ContextRef{Name: "beta"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if closeErr := cluster.Close(first); closeErr != nil {
		t.Fatalf("close: %v", closeErr)
	}

	if cluster.ID() != beta {
		t.Fatalf("active = %q, want the tab that was already focused", cluster.ID())
	}
}

func TestClosingTheLastClusterLeavesNothingActive(t *testing.T) {
	cluster := newTestCluster(t, &recorder{})

	if err := cluster.Close(cluster.ID()); err != nil {
		t.Fatalf("close: %v", err)
	}

	if cluster.ID() != "" {
		t.Fatalf("active = %q, want nothing", cluster.ID())
	}
	if cluster.Manager("") != nil {
		t.Fatal("nothing is open but a manager came back anyway")
	}
	if cluster.Current() != (api.ContextRef{}) {
		t.Fatalf("current = %+v, want nothing", cluster.Current())
	}
}

func TestClosingAClusterNobodyOpenedSaysSo(t *testing.T) {
	cluster := newTestCluster(t, &recorder{})

	err := cluster.Close("https://ghost:6443")

	if !errors.Is(err, api.ErrNotOpen) {
		t.Fatalf("error = %v, want it to report the cluster is not open", err)
	}
}

func TestOnlyTheRequestedContextIsDialledAtStart(t *testing.T) {
	rec := &recorder{}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cluster := newCluster(ctx, rec.build, newStubSources(), newStubProtection(), testOpenTimeout, api.ContextRef{Name: "p-mk2"})

	if len(rec.refs) != 1 {
		t.Fatalf("dialed %d contexts at start, want only the one that was asked for", len(rec.refs))
	}
	if rec.refs[0].Name != "p-mk2" {
		t.Fatalf("dialed %q, want the context the flag named", rec.refs[0].Name)
	}
	if cluster.Current().Name != "p-mk2" {
		t.Fatalf("current = %q, want the context the flag named", cluster.Current().Name)
	}
}

func TestSwitchingToTheSameClusterCancelsTheConnectionItReplaced(t *testing.T) {
	rec := &recorder{}
	cluster := newTestCluster(t, rec)

	if err := cluster.Use(api.ContextRef{}); err != nil {
		t.Fatalf("use: %v", err)
	}

	if len(rec.live) != 2 {
		t.Fatalf("built %d connections, want the switch to have built one", len(rec.live))
	}
	select {
	case <-rec.live[0].Done():
	case <-time.After(2 * time.Second):
		t.Fatal("the connection that was replaced was left running")
	}
}
