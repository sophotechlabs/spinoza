//go:build integration

package integration

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/metadata"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/exec"
	"github.com/sophotechlabs/spinoza/internal/helm"
	"github.com/sophotechlabs/spinoza/internal/inspect"
	"github.com/sophotechlabs/spinoza/internal/kube"
	"github.com/sophotechlabs/spinoza/internal/portforward"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

const namespace = "spinoza-smoke"

const (
	namespaceTimeout = 2 * time.Minute
	podTimeout       = 2 * time.Minute
	pollInterval     = time.Second
)

var (
	loadedBundle *kube.Bundle
	errCluster   error
)

func TestMain(m *testing.M) {
	loadedBundle, errCluster = openCluster()
	m.Run()
	if loadedBundle != nil {
		closeNamespace(loadedBundle)
	}
}

func openCluster() (*kube.Bundle, error) {
	loaded, err := kube.LoadContext(api.ContextRef{Name: os.Getenv("SPINOZA_TEST_CONTEXT")}, kube.Options{})
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	err = openNamespace(loaded)
	if err != nil {
		return loaded, fmt.Errorf("prepare namespace %s: %w", namespace, err)
	}
	return loaded, nil
}

func openNamespace(loaded *kube.Bundle) error {
	ctx := context.Background()
	err := awaitNamespaceGone(ctx, loaded)
	if err != nil {
		return err
	}
	_, err = loaded.Clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	return awaitNamespaceActive(ctx, loaded)
}

func closeNamespace(loaded *kube.Bundle) {
	_ = loaded.Clientset.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
}

func awaitNamespaceGone(ctx context.Context, loaded *kube.Bundle) error {
	deadline := time.Now().Add(namespaceTimeout)
	for {
		_, err := loaded.Clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		closeNamespace(loaded)
		if time.Now().After(deadline) {
			return fmt.Errorf("namespace %s did not go away within %s", namespace, namespaceTimeout)
		}
		time.Sleep(pollInterval)
	}
}

func awaitNamespaceActive(ctx context.Context, loaded *kube.Bundle) error {
	deadline := time.Now().Add(namespaceTimeout)
	for time.Now().Before(deadline) {
		found, err := loaded.Clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if found.Status.Phase == corev1.NamespaceActive {
			return nil
		}
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("namespace %s never became active within %s", namespace, namespaceTimeout)
}

func bundle(t *testing.T) *kube.Bundle {
	t.Helper()
	if errCluster != nil {
		t.Fatalf("the cluster was never ready: %v", errCluster)
	}
	return loadedBundle
}

func metaFor(t *testing.T, loaded *kube.Bundle) metadata.Interface {
	t.Helper()
	client, err := metadata.NewForConfig(loaded.Config)
	if err != nil {
		t.Fatalf("metadata client: %v", err)
	}
	return client
}

func manager(t *testing.T, loaded *kube.Bundle) *resources.Manager {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cats, descs, err := discovery.List(loaded.Discovery)
	if err != nil {
		t.Logf("discovery came back partial: %v", err)
	}
	if len(descs) == 0 {
		t.Fatal("the cluster listed no resource types")
	}
	mgr := resources.NewManager(ctx, resources.Deps{
		Dynamic:    loaded.Dynamic,
		Clientset:  loaded.Clientset,
		Metadata:   metaFor(t, loaded),
		Categories: cats,
		Helm: helm.NewService(
			loaded.Clientset,
			metaFor(t, loaded),
			helm.NewRunner(""),
			nil,
			nil,
			api.ContextRef{Name: os.Getenv("SPINOZA_TEST_CONTEXT")},
		),
		Descriptors: descs,
	})
	mgr.UseDiscovery(loaded.Discovery, err)
	return mgr
}

func runningPod(t *testing.T, loaded *kube.Bundle, name string) *corev1.Pod {
	t.Helper()
	ctx := context.Background()
	_, err := loaded.Clientset.CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"app": name}},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:    "web",
				Image:   "busybox:1.36",
				Command: []string{"sh", "-c", "sleep 3600"},
				Ports:   []corev1.ContainerPort{{ContainerPort: 80}},
			}},
		},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create pod: %v", err)
	}
	return waitRunning(t, loaded, name)
}

func waitRunning(t *testing.T, loaded *kube.Bundle, name string) *corev1.Pod {
	t.Helper()
	deadline := time.Now().Add(podTimeout)
	var last string
	for time.Now().Before(deadline) {
		pod, err := loaded.Clientset.CoreV1().Pods(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			last = err.Error()
		}
		if err == nil {
			last = string(pod.Status.Phase)
			if pod.Status.Phase == corev1.PodRunning {
				return pod
			}
		}
		time.Sleep(pollInterval)
	}
	t.Fatalf("pod %s never reached Running (last seen: %s)", name, last)
	return nil
}

func TestSubscribeSeesARealInformerFill(t *testing.T) {
	loaded := bundle(t)
	mgr := manager(t, loaded)
	runningPod(t, loaded, "smoke-subscribe")

	sub, err := mgr.Subscribe(context.Background(), "", "v1", "pods", namespace, 0, nil)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(sub.Close)

	rows, _, snapErr := sub.Snapshot()
	if snapErr != nil {
		t.Fatalf("snapshot: %v", snapErr)
	}
	if len(rows) == 0 {
		t.Fatal("a real informer synced with no rows")
	}
	if len(sub.Columns()) == 0 {
		t.Fatal("the subscription carried no columns")
	}
}

func TestApplyReportsARealConflict(t *testing.T) {
	loaded := bundle(t)
	runningPod(t, loaded, "smoke-conflict")
	ref := api.ObjectRef{Version: "v1", Resource: "pods", Namespace: namespace, Name: "smoke-conflict"}

	detail, err := inspect.Get(context.Background(), loaded.Dynamic, ref)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	stale := detail.YAML
	if _, err := loaded.Clientset.CoreV1().Pods(namespace).Patch(context.Background(), "smoke-conflict",
		"application/merge-patch+json", []byte(`{"metadata":{"labels":{"touched":"yes"}}}`), metav1.PatchOptions{}); err != nil {
		t.Fatalf("patch: %v", err)
	}

	_, applyErr := inspect.Apply(context.Background(), loaded.Dynamic, ref, "Pod", []byte(stale))

	if applyErr == nil {
		t.Fatal("applying a stale resourceVersion reported success; a fake clientset never produces this")
	}
	if !apierrors.IsConflict(applyErr) {
		t.Fatalf("err = %v, want a 409 conflict", applyErr)
	}
}

func TestCountsReachEveryDiscoveredType(t *testing.T) {
	loaded := bundle(t)
	mgr := manager(t, loaded)

	counts := mgr.Counts(context.Background())

	if len(counts.Counts) == 0 {
		t.Fatal("counting produced nothing against a real apiserver")
	}
	pods, ok := counts.Counts["/v1/pods"]
	if !ok {
		t.Fatalf("pods were not counted: %v", counts.Errors)
	}
	if pods < 0 {
		t.Fatalf("pods = %d (%s)", pods, counts.Errors["/v1/pods"])
	}
}

func TestPortForwardReachesARealPod(t *testing.T) {
	loaded := bundle(t)
	runningPod(t, loaded, "smoke-forward")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	registry := portforward.NewRegistry(
		ctx,
		portforward.NewRunner(loaded.Clientset, loaded.Config),
		portforward.NewResolver(loaded.Clientset),
		portforward.NewProber(loaded.Clientset),
	)
	t.Cleanup(registry.StopAll)

	forward, err := registry.Start(context.Background(),
		portforward.Target{Kind: portforward.KindPod, Namespace: namespace, Name: "smoke-forward"}, 80)
	if err != nil {
		t.Fatalf("start forward: %v", err)
	}

	if forward.LocalPort == 0 {
		t.Fatal("the forward reported no local port")
	}
	if forward.State != portforward.StateRunning {
		t.Fatalf("state = %q (%s)", forward.State, forward.Error)
	}
}

type collected struct {
	mu    sync.Mutex
	bytes []byte
}

func (c *collected) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bytes = append(c.bytes, p...)
	return len(p), nil
}

func (c *collected) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return string(c.bytes)
}

func TestExecRunsAShellInARealPod(t *testing.T) {
	loaded := bundle(t)
	runningPod(t, loaded, "smoke-exec")
	service := exec.NewService(
		exec.NewStreamer(loaded.Clientset, loaded.Config),
		exec.NewImages(loaded.Clientset),
	)
	output := &collected{}

	session, err := service.Start(context.Background(),
		exec.Request{Namespace: namespace, Pod: "smoke-exec", Container: "web"}, output)
	if err != nil {
		t.Fatalf("start exec: %v", err)
	}
	t.Cleanup(session.Close)

	if _, err := session.Write([]byte("echo spinoza-was-here\n")); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(output.String(), "spinoza-was-here") {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("the shell never echoed back: %q", output.String())
}

func TestListWarmsAPinnedInformerAgainstARealCluster(t *testing.T) {
	loaded := bundle(t)
	mgr := manager(t, loaded)
	desc := api.ResourceDescriptor{Version: "v1", Resource: "namespaces", Kind: "Namespace"}

	items, err := mgr.List(context.Background(), desc)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(items) == 0 {
		t.Fatal("a real cluster listed no namespaces")
	}
	if _, ok := items[0].Object["metadata"]; !ok {
		t.Fatalf("item carried no metadata: %v", items[0])
	}
	mgr.Unpin([]api.ResourceDescriptor{desc})
}

func installRelease(t *testing.T, loaded *kube.Bundle) {
	t.Helper()
	_, err := osexec.LookPath("helm")
	if err != nil {
		t.Skip("helm is not on PATH, so a real release cannot be installed")
	}
	chart := writeChart(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := osexec.CommandContext(ctx, "helm", "install", "smoke-release", chart,
		"--namespace", namespace,
		"--kube-context", os.Getenv("SPINOZA_TEST_CONTEXT"),
		"--wait")
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		t.Fatalf("helm install: %v\n%s", runErr, out)
	}
	t.Cleanup(func() {
		done, cancelDone := context.WithTimeout(context.Background(), time.Minute)
		defer cancelDone()
		_ = osexec.CommandContext(done, "helm", "uninstall", "smoke-release",
			"--namespace", namespace,
			"--kube-context", os.Getenv("SPINOZA_TEST_CONTEXT")).Run()
	})
	_ = loaded
}

func writeChart(t *testing.T) string {
	t.Helper()
	return writeChartVersion(t, "0.1.0")
}

func writeChartVersion(t *testing.T, version string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"Chart.yaml": "apiVersion: v2\nname: spinoza-smoke\nversion: " + version + "\nappVersion: \"1.2.3\"\n",
		"templates/configmap.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n" +
			"  name: {{ .Release.Name }}\ndata:\n  hello: world\n",
	}
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return dir
}

func packageChart(t *testing.T, chartDir string) string {
	t.Helper()
	repoDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pack := osexec.CommandContext(ctx, "helm", "package", chartDir, "-d", repoDir)
	out, err := pack.CombinedOutput()
	if err != nil {
		t.Fatalf("helm package: %v\n%s", err, out)
	}
	index := osexec.CommandContext(ctx, "helm", "repo", "index", repoDir)
	out, err = index.CombinedOutput()
	if err != nil {
		t.Fatalf("helm repo index: %v\n%s", err, out)
	}
	return repoDir
}

func TestOverviewReadsARealCluster(t *testing.T) {
	loaded := bundle(t)
	mgr := manager(t, loaded)
	runningPod(t, loaded, "smoke-overview")

	got := mgr.Overview(context.Background())

	if got.Version == "" {
		t.Fatalf("the overview reported no server version (%s)", got.Error)
	}
	if got.Nodes.Total == 0 {
		t.Fatalf("nodes = 0 against a real cluster (%s)", got.Error)
	}
	if got.Nodes.Ready == 0 {
		t.Fatalf("ready nodes = 0 (%s)", got.Error)
	}
	if got.Nodes.CPUAllocatableMilli == 0 {
		t.Fatalf("cpu allocatable = 0 (%s)", got.Error)
	}
	if !got.Pods.Known {
		t.Fatalf("the pod tally could not be taken: %s", got.Error)
	}
	if got.Pods.Total < got.Pods.Running {
		t.Fatalf("total %d is below running %d", got.Pods.Total, got.Pods.Running)
	}
	if got.Pods.Running == 0 {
		t.Fatalf("running pods = 0 with a pod we just started (%s)", got.Error)
	}
}

func TestHelmDetailAndActionsAgainstRealHelm(t *testing.T) {
	loaded := bundle(t)
	mgr := manager(t, loaded)
	installRelease(t, loaded)
	upgradeRelease(t)

	detail, err := mgr.HelmRelease(context.Background(), namespace, "smoke-release")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Release.Revision != 2 {
		t.Fatalf("revision = %d, want the upgrade", detail.Release.Revision)
	}
	if len(detail.History) != 2 {
		t.Fatalf("history = %d, want both revisions", len(detail.History))
	}
	if detail.History[0].Revision != 2 {
		t.Fatalf("history = %v, want newest first", detail.History)
	}
	if !strings.Contains(detail.Manifest, "kind: ConfigMap") {
		t.Fatalf("manifest = %q, want the rendered configmap", detail.Manifest)
	}
	if len(detail.Resources) != 1 {
		t.Fatalf("resources = %v, want the one configmap", detail.Resources)
	}
	if detail.Resources[0].Resource != "configmaps" {
		t.Fatalf("resource = %q, want discovery to resolve it", detail.Resources[0].Resource)
	}
	if !strings.Contains(detail.Values, "extra") {
		t.Fatalf("values = %q, want the upgrade's supplied values", detail.Values)
	}
	if detail.Driver != "secret" {
		t.Fatalf("driver = %q, want secret", detail.Driver)
	}

	support := mgr.HelmSupport()
	if !support.Available {
		t.Fatalf("helm reported unavailable: %s", support.Reason)
	}

	rolled, rollErr := mgr.HelmRollback(context.Background(), namespace, "smoke-release", 1)
	if rollErr != nil {
		t.Fatalf("rollback: %v", rollErr)
	}
	if rolled.Revision != 1 {
		t.Fatalf("rollback revision = %d, want 1", rolled.Revision)
	}
	after, afterErr := mgr.HelmRelease(context.Background(), namespace, "smoke-release")
	if afterErr != nil {
		t.Fatalf("detail after rollback: %v", afterErr)
	}
	if after.Release.Revision != 3 {
		t.Fatalf("revision = %d, want the rollback to have made revision 3", after.Release.Revision)
	}
	if strings.Contains(after.Values, "extra") {
		t.Fatalf("values = %q, want revision 1's values back", after.Values)
	}

	_, removeErr := mgr.HelmUninstall(context.Background(), namespace, "smoke-release")
	if removeErr != nil {
		t.Fatalf("uninstall: %v", removeErr)
	}
	_, goneErr := mgr.HelmRelease(context.Background(), namespace, "smoke-release")
	if goneErr == nil {
		t.Fatal("the release still reads back after an uninstall")
	}
}

const chartHostVar = "SPINOZA_CHART_HOST"

func namedRepo(t *testing.T, raw string) string {
	t.Helper()
	host := os.Getenv(chartHostVar)
	if host == "" {
		t.Skipf("set %s to a name mapped to 127.0.0.1 to run this test", chartHostVar)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	//nolint:gosec // the host is this test's own SPINOZA_CHART_HOST, and the lookup only proves it resolves
	if _, lookupErr := net.LookupHost(host); lookupErr != nil {
		t.Fatalf("%s is %q, which does not resolve: %v", chartHostVar, host, lookupErr)
	}
	return "http://" + net.JoinHostPort(host, parsed.Port())
}

func TestHelmUpgradeRefusesARepoOnThisMachine(t *testing.T) {
	loaded := bundle(t)
	mgr := manager(t, loaded)
	installRelease(t, loaded)

	server := httptest.NewServer(http.FileServer(http.Dir(packageChart(t, writeChartVersion(t, "0.2.0")))))
	t.Cleanup(server.Close)

	_, err := mgr.HelmUpgrade(context.Background(), helm.UpgradeRequest{
		Namespace: namespace,
		Name:      "smoke-release",
		Chart:     "spinoza-smoke",
		Version:   "0.2.0",
		RepoURL:   server.URL,
	})

	if err == nil {
		t.Fatal("a chart repo on this machine was accepted")
	}
	if !strings.Contains(err.Error(), "is not a public address") {
		t.Fatalf("error = %v, want it to name the private address", err)
	}
}

func TestHelmUpgradeThroughAChartRepo(t *testing.T) {
	loaded := bundle(t)
	mgr := manager(t, loaded)
	installRelease(t, loaded)

	repoDir := packageChart(t, writeChartVersion(t, "0.2.0"))
	server := httptest.NewServer(http.FileServer(http.Dir(repoDir)))
	t.Cleanup(server.Close)

	req := helm.UpgradeRequest{
		Namespace: namespace,
		Name:      "smoke-release",
		Chart:     "spinoza-smoke",
		Version:   "0.2.0",
		RepoURL:   namedRepo(t, server.URL),
		Values:    "extra: upgraded\n",
	}

	dry := req
	dry.DryRun = true
	rendered, dryErr := mgr.HelmUpgrade(context.Background(), dry)
	if dryErr != nil {
		t.Fatalf("dry run: %v", dryErr)
	}
	if !rendered.DryRun {
		t.Fatal("the dry run was not marked as one")
	}
	if !strings.Contains(rendered.Manifest, "kind: ConfigMap") {
		t.Fatalf("manifest = %q, want the rendered configmap", rendered.Manifest)
	}
	before, beforeErr := mgr.HelmRelease(context.Background(), namespace, "smoke-release")
	if beforeErr != nil {
		t.Fatalf("detail after the dry run: %v", beforeErr)
	}
	if before.Release.Revision != 1 {
		t.Fatalf("revision = %d, want the dry run to have changed nothing", before.Release.Revision)
	}

	result, upErr := mgr.HelmUpgrade(context.Background(), req)
	if upErr != nil {
		t.Fatalf("upgrade: %v", upErr)
	}
	if result.Action != helm.ActionUpgrade {
		t.Fatalf("action = %q, want upgrade", result.Action)
	}
	after, afterErr := mgr.HelmRelease(context.Background(), namespace, "smoke-release")
	if afterErr != nil {
		t.Fatalf("detail after the upgrade: %v", afterErr)
	}
	if after.Release.Revision != 2 {
		t.Fatalf("revision = %d, want 2", after.Release.Revision)
	}
	if after.Release.ChartVersion != "0.2.0" {
		t.Fatalf("chart version = %q, want 0.2.0", after.Release.ChartVersion)
	}
	if !strings.Contains(after.Values, "upgraded") {
		t.Fatalf("values = %q, want the supplied values applied", after.Values)
	}

	rolled, rollErr := mgr.HelmRollback(context.Background(), namespace, "smoke-release", 1)
	if rollErr != nil {
		t.Fatalf("rollback: %v", rollErr)
	}
	if rolled.Revision != 1 {
		t.Fatalf("rollback revision = %d, want 1", rolled.Revision)
	}
	undone, undoneErr := mgr.HelmRelease(context.Background(), namespace, "smoke-release")
	if undoneErr != nil {
		t.Fatalf("detail after the rollback: %v", undoneErr)
	}
	if undone.Release.Revision != 3 {
		t.Fatalf("revision = %d, want the rollback to have made revision 3", undone.Release.Revision)
	}
	if undone.Release.ChartVersion != "0.1.0" {
		t.Fatalf("chart version = %q, want the rollback back on 0.1.0", undone.Release.ChartVersion)
	}
}

func upgradeRelease(t *testing.T) {
	t.Helper()
	chart := writeChart(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := osexec.CommandContext(ctx, "helm", "upgrade", "smoke-release", chart,
		"--namespace", namespace,
		"--kube-context", os.Getenv("SPINOZA_TEST_CONTEXT"),
		"--set", "extra=value",
		"--wait")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helm upgrade: %v\n%s", err, out)
	}
}

func TestHelmReleasesReadsRealStorageSecrets(t *testing.T) {
	loaded := bundle(t)
	mgr := manager(t, loaded)
	installRelease(t, loaded)

	got, err := mgr.HelmReleases(context.Background())
	if err != nil {
		t.Fatalf("helm releases: %v", err)
	}

	found := api.HelmRelease{}
	for _, release := range got.Releases {
		if release.Name == "smoke-release" {
			found = release
		}
	}
	if found.Name == "" {
		t.Fatalf("the release we installed is missing from %v (%s)", got.Releases, got.Error)
	}
	if found.Namespace != namespace {
		t.Fatalf("namespace = %q, want %s", found.Namespace, namespace)
	}
	if found.Chart == "" {
		t.Fatalf("chart = %q, want the chart name out of the payload", found.Chart)
	}
	if found.Revision != 1 {
		t.Fatalf("revision = %d, want 1", found.Revision)
	}
	if found.Status != "deployed" {
		t.Fatalf("status = %q, want deployed", found.Status)
	}
	if found.Updated == "" {
		t.Fatal("the release carried no last-deployed time")
	}
}
