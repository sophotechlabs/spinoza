package resources

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	authv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/openapi"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/access"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/exec"
	"github.com/sophotechlabs/spinoza/internal/helm"
	"github.com/sophotechlabs/spinoza/internal/jsonschema"
	"github.com/sophotechlabs/spinoza/internal/portforward"
	"github.com/sophotechlabs/spinoza/internal/prom"
)

func deployAt(namespace, name string) api.ObjectRef {
	return api.ObjectRef{
		Group:     "apps",
		Version:   "v1",
		Resource:  "deployments",
		Namespace: namespace,
		Name:      name,
	}
}

func namesOf(found []*unstructured.Unstructured) []string {
	out := make([]string, 0, len(found))
	for _, one := range found {
		out = append(out, one.GetName())
	}
	return out
}

func TestListKindReadsOneNamespace(t *testing.T) {
	dyn := newClient(t, newDeployment("prod", "web"), newDeployment("staging", "web"))
	mgr, cancel := newManager(t, dyn)
	defer cancel()

	found, err := mgr.ListKind(t.Context(), deployAt("prod", ""))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("found %v, want only the one in prod", namesOf(found))
	}
	if found[0].GetNamespace() != "prod" {
		t.Fatalf("namespace = %q", found[0].GetNamespace())
	}
}

func TestListKindReadsEveryNamespaceWhenNoneIsNamed(t *testing.T) {
	dyn := newClient(t, newDeployment("prod", "web"), newDeployment("staging", "api"))
	mgr, cancel := newManager(t, dyn)
	defer cancel()

	found, err := mgr.ListKind(t.Context(), deployAt("", ""))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("found %v, want both", namesOf(found))
	}
}

func TestListKindOfNothingIsNotAnError(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	found, err := mgr.ListKind(t.Context(), deployAt("prod", ""))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("found %v, want none", namesOf(found))
	}
}

func TestListKindSurfacesWhatTheApiserverSaid(t *testing.T) {
	dyn := newClient(t)
	dyn.PrependReactor("list", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("deployments is forbidden")
	})
	mgr, cancel := newManager(t, dyn)
	defer cancel()

	_, err := mgr.ListKind(t.Context(), deployAt("prod", ""))

	if err == nil {
		t.Fatal("a refused list was reported as an empty kind")
	}
}

func TestListKindWithoutAClusterSaysSo(t *testing.T) {
	mgr := &Manager{}

	_, err := mgr.ListKind(t.Context(), deployAt("prod", ""))

	if !errors.Is(err, api.ErrInternal) {
		t.Fatalf("error = %v, want an internal one", err)
	}
}

func selectingDeployment(namespace, name string, labels map[string]any) *unstructured.Unstructured {
	object := newDeployment(namespace, name)
	spec, ok := object.Object["spec"].(map[string]any)
	if !ok {
		panic("the deployment fixture lost its spec")
	}
	spec["selector"] = map[string]any{"matchLabels": labels}
	return object
}

func TestPodSelectorReadsTheLabelsAWorkloadPutsOnItsPods(t *testing.T) {
	dyn := newClient(t, selectingDeployment("prod", "web", map[string]any{"app": "web"}))
	mgr, cancel := newManager(t, dyn)
	defer cancel()

	selector, err := mgr.PodSelector(t.Context(), deployAt("prod", "web"))
	if err != nil {
		t.Fatalf("selector: %v", err)
	}
	if selector != "app=web" {
		t.Fatalf("selector = %q", selector)
	}
}

func TestPodSelectorJoinsEveryLabel(t *testing.T) {
	dyn := newClient(t, selectingDeployment("prod", "web", map[string]any{
		"app":  "web",
		"tier": "front",
	}))
	mgr, cancel := newManager(t, dyn)
	defer cancel()

	selector, err := mgr.PodSelector(t.Context(), deployAt("prod", "web"))
	if err != nil {
		t.Fatalf("selector: %v", err)
	}
	if selector != "app=web,tier=front" {
		t.Fatalf("selector = %q, want every label in it", selector)
	}
}

func TestAWorkloadThatSelectsNothingSaysSo(t *testing.T) {
	dyn := newClient(t, newDeployment("prod", "web"))
	mgr, cancel := newManager(t, dyn)
	defer cancel()

	_, err := mgr.PodSelector(t.Context(), deployAt("prod", "web"))

	if !errors.Is(err, api.ErrInternal) {
		t.Fatalf("error = %v, want it to say the workload selects no pods", err)
	}
}

func TestPodSelectorOfSomethingThatIsNotThere(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	_, err := mgr.PodSelector(t.Context(), deployAt("prod", "gone"))

	if err == nil {
		t.Fatal("a missing workload handed back a selector")
	}
}

func TestPodSelectorWithoutAClusterSaysSo(t *testing.T) {
	mgr := &Manager{}

	_, err := mgr.PodSelector(t.Context(), deployAt("prod", "web"))

	if !errors.Is(err, api.ErrInternal) {
		t.Fatalf("error = %v, want an internal one", err)
	}
}

func decidingClientset(allowed bool, reason string) *k8sfake.Clientset {
	cs := k8sfake.NewClientset()
	cs.PrependReactor(
		"create",
		"selfsubjectaccessreviews",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			create, ok := action.(k8stesting.CreateAction)
			if !ok {
				return false, nil, nil
			}
			review, ok := create.GetObject().(*authv1.SelfSubjectAccessReview)
			if !ok {
				return false, nil, nil
			}
			review.Status = authv1.SubjectAccessReviewStatus{Allowed: allowed, Reason: reason}
			return true, review, nil
		},
	)
	return cs
}

func managerWithClientset(t *testing.T, cs *k8sfake.Clientset) (*Manager, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	mgr := NewManager(ctx, Deps{
		Dynamic:     newClient(t),
		Clientset:   cs,
		Descriptors: testDescs(),
	})
	return mgr, cancel
}

func TestAccessReportsWhatTheClusterRefuses(t *testing.T) {
	mgr, cancel := managerWithClientset(t, decidingClientset(false, "not for you"))
	defer cancel()

	result := mgr.Access(t.Context(), deployAt("prod", "web"))

	if len(result.Refused) == 0 {
		t.Fatal("a cluster that refuses everything held nothing back")
	}
	for _, refusal := range result.Refused {
		if refusal.Reason != "not for you" {
			t.Fatalf("%s reason = %q", refusal.Capability, refusal.Reason)
		}
	}
}

func TestHelmAccessReportsWhatTheClusterRefuses(t *testing.T) {
	mgr, cancel := managerWithClientset(t, decidingClientset(false, "not for you"))
	defer cancel()

	result := mgr.HelmAccess(t.Context(), "prod", "podinfo")

	if len(result.Refused) != 4 {
		t.Fatalf("refused = %v, want every helm button held back", result.Refused)
	}
	for _, refusal := range result.Refused {
		if refusal.Reason != "not for you" {
			t.Fatalf("%s reason = %q", refusal.Capability, refusal.Reason)
		}
	}
}

func TestHelmAccessHoldsNothingBackWhenEverythingIsAllowed(t *testing.T) {
	mgr, cancel := managerWithClientset(t, decidingClientset(true, ""))
	defer cancel()

	result := mgr.HelmAccess(t.Context(), "prod", "podinfo")

	if len(result.Refused) != 0 {
		t.Fatalf("refused = %v, want nothing", result.Refused)
	}
}

func TestAccessHoldsNothingBackWhenEverythingIsAllowed(t *testing.T) {
	mgr, cancel := managerWithClientset(t, decidingClientset(true, ""))
	defer cancel()

	result := mgr.Access(t.Context(), deployAt("prod", "web"))

	if len(result.Refused) != 0 {
		t.Fatalf("refused = %v, want nothing", result.Refused)
	}
}

func TestWhatAManagerWithNothingWiredUpSays(t *testing.T) {
	mgr := &Manager{}
	ref := deployAt("prod", "web")

	t.Run("metric history", func(t *testing.T) {
		_, err := mgr.MetricHistory(t.Context(), "prod", "web", time.Hour)
		if err == nil {
			t.Fatal("history without prometheus was reported as empty")
		}
	})
	t.Run("schema", func(t *testing.T) {
		_, err := mgr.Schema(t.Context(), jsonschema.GVK{Kind: "Deployment"})
		if !errors.Is(err, api.ErrInternal) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("helm releases", func(t *testing.T) {
		_, err := mgr.HelmReleases(t.Context())
		if !errors.Is(err, api.ErrInternal) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("helm release", func(t *testing.T) {
		_, err := mgr.HelmRelease(t.Context(), "prod", "web", 0)
		if !errors.Is(err, api.ErrInternal) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("helm support", func(t *testing.T) {
		if mgr.HelmSupport().Available {
			t.Fatal("helm was reported as available")
		}
	})
	t.Run("helm rollback", func(t *testing.T) {
		_, err := mgr.HelmRollback(t.Context(), "prod", "web", 1)
		if !errors.Is(err, api.ErrInternal) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("helm uninstall", func(t *testing.T) {
		_, err := mgr.HelmUninstall(t.Context(), "prod", "web")
		if !errors.Is(err, api.ErrInternal) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("port forward", func(t *testing.T) {
		_, err := mgr.StartForward(t.Context(), portforward.Target{Kind: "Pod"}, 80)
		if !errors.Is(err, api.ErrInternal) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("stop forward", func(t *testing.T) {
		if err := mgr.StopForward("pf-1"); !errors.Is(err, api.ErrInternal) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("exec support", func(t *testing.T) {
		_, err := mgr.ExecSupport(t.Context(), exec.Request{Namespace: "prod", Pod: "web"})
		if !errors.Is(err, api.ErrInternal) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("node shell support", func(t *testing.T) {
		if mgr.NodeShellSupport(t.Context(), "node-1").Allowed {
			t.Fatal("a node shell was offered with nothing wired up")
		}
	})
	t.Run("node shell start", func(t *testing.T) {
		_, err := mgr.StartNodeShell(t.Context(), "node-1")
		if !errors.Is(err, api.ErrInternal) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("access", func(t *testing.T) {
		if len(mgr.Access(t.Context(), ref).Refused) != 0 {
			t.Fatal("a manager with no cluster refused something")
		}
	})
	t.Run("helm access", func(t *testing.T) {
		if len(mgr.HelmAccess(t.Context(), "prod", "web").Refused) != 0 {
			t.Fatal("a manager with no cluster refused a helm action")
		}
	})
}

func versionSays(err error) *k8sfake.Clientset {
	cs := k8sfake.NewClientset()
	cs.PrependReactor("get", "version", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, err
	})
	return cs
}

func TestAClusterThatAnswersIsReachable(t *testing.T) {
	mgr, cancel := managerWithClientset(t, versionSays(nil))
	defer cancel()

	if err := mgr.Ping(t.Context()); err != nil {
		t.Fatalf("ping: %v, want the cluster counted as answering", err)
	}
}

func TestAClusterThatRefusesTheQuestionIsStillReachable(t *testing.T) {
	refused := apierrors.NewForbidden(
		schema.GroupResource{Resource: "version"},
		"",
		errors.New("requires container.clusters.get"),
	)
	mgr, cancel := managerWithClientset(t, versionSays(refused))
	defer cancel()

	if err := mgr.Ping(t.Context()); err != nil {
		t.Fatalf("ping: %v, want a refusal to count as an answer", err)
	}
}

func TestAClusterThatDoesNotAnswerAtAllIsUnreachable(t *testing.T) {
	mgr, cancel := managerWithClientset(t, versionSays(
		&net.OpError{Op: "dial", Err: errors.New("connect: connection refused")},
	))
	defer cancel()

	err := mgr.Ping(t.Context())

	if err == nil {
		t.Fatal("a cluster that could not be dialed was reported as answering")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("error = %v, want what went wrong", err)
	}
}

func TestAnUnauthorizedClusterIsStillReachable(t *testing.T) {
	mgr, cancel := managerWithClientset(t, versionSays(apierrors.NewUnauthorized("token expired")))
	defer cancel()

	if err := mgr.Ping(t.Context()); err != nil {
		t.Fatalf("ping: %v, want the apiserver's refusal counted as an answer", err)
	}
}

func TestAPingWithNoClusterWiredUpSaysSo(t *testing.T) {
	mgr := &Manager{}

	if err := mgr.Ping(t.Context()); !errors.Is(err, api.ErrInternal) {
		t.Fatalf("error = %v, want an internal one", err)
	}
}

func TestAPingGivesUpWhenTheCallerDoes(t *testing.T) {
	held := make(chan struct{})
	cs := k8sfake.NewClientset()
	cs.PrependReactor("get", "version", func(k8stesting.Action) (bool, runtime.Object, error) {
		<-held
		return true, nil, nil
	})
	mgr, cancel := managerWithClientset(t, cs)
	defer cancel()
	defer close(held)

	ctx, giveUp := context.WithCancel(t.Context())
	giveUp()
	err := mgr.Ping(ctx)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want it to stop when the caller did", err)
	}
}

func TestAPingCancelsTheApiserverRequestWithItsCaller(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	var startOnce sync.Once
	var stopOnce sync.Once
	apiserver := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		startOnce.Do(func() {
			close(started)
		})
		<-r.Context().Done()
		stopOnce.Do(func() {
			close(stopped)
		})
	}))
	t.Cleanup(func() {
		apiserver.CloseClientConnections()
		apiserver.Close()
	})
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: apiserver.URL})
	if err != nil {
		t.Fatal(err)
	}
	mgr := NewManager(t.Context(), Deps{Clientset: cs})
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	pinged := make(chan error, 1)
	go func() {
		pinged <- mgr.Ping(ctx)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("the apiserver received no ping")
	}
	cancel()
	select {
	case pingErr := <-pinged:
		if !errors.Is(pingErr, context.Canceled) {
			t.Fatalf("error = %v, want cancellation", pingErr)
		}
	case <-time.After(time.Second):
		t.Fatal("the ping did not return after cancellation")
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("the apiserver request survived cancellation")
	}
}

type openapiFor struct {
	reads *atomic.Int64
}

func (o openapiFor) Paths() (map[string]openapi.GroupVersion, error) {
	return map[string]openapi.GroupVersion{"api/v1": schemaDoc(o)}, nil
}

type schemaDoc struct {
	reads *atomic.Int64
}

func (d schemaDoc) Schema(string) ([]byte, error) {
	d.reads.Add(1)
	return []byte(`{"components":{"schemas":{"io.k8s.api.core.v1.Pod":{"type":"object",` +
		`"x-kubernetes-group-version-kind":[{"group":"","kind":"Pod","version":"v1"}]}}}}`), nil
}

func (schemaDoc) ServerRelativeURL() string {
	return ""
}

func TestRefreshingResourcesAlsoDropsTheSchemasItHeld(t *testing.T) {
	ctx := t.Context()
	reads := &atomic.Int64{}
	mgr := NewManager(ctx, Deps{
		Dynamic:     newClient(t),
		Clientset:   k8sfake.NewClientset(),
		Schemas:     jsonschema.NewClient(func() openapi.Client { return openapiFor{reads: reads} }),
		Descriptors: testDescs(),
	})
	mgr.UseDiscovery(&stubDiscovery{results: []discoveryResult{{lists: podList()}}}, nil)
	pod := jsonschema.GVK{Version: "v1", Kind: "Pod"}
	if _, err := mgr.Schema(ctx, pod); err != nil {
		t.Fatalf("first schema read: %v", err)
	}
	if _, err := mgr.Schema(ctx, pod); err != nil {
		t.Fatalf("cached schema read: %v", err)
	}
	if reads.Load() != 1 {
		t.Fatalf("document read %d times, want the second answered from cache", reads.Load())
	}

	mgr.RefreshResources()

	if _, err := mgr.Schema(ctx, pod); err != nil {
		t.Fatalf("schema read after refresh: %v", err)
	}
	if reads.Load() != 2 {
		t.Fatalf("document read %d times, want it fetched again after the refresh", reads.Load())
	}
}

func TestMetricHistoryComesBackFromPrometheus(t *testing.T) {
	ctx := t.Context()
	cs := k8sfake.NewClientset(&corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "monitoring",
			Name:      "prometheus-operated",
			Labels:    map[string]string{"operated-prometheus": "true"},
		},
		Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "http-web", Port: 9090}}},
	})
	mgr := NewManager(ctx, Deps{
		Dynamic:     newClient(t),
		Clientset:   cs,
		Prometheus:  prom.NewClientWithProxy(cs, &rangeProxy{}, prom.Target{}),
		Descriptors: testDescs(),
	})

	history, err := mgr.MetricHistory(ctx, "prod", "web", time.Hour)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history.CPU) == 0 {
		t.Fatalf("history = %+v, want the points prometheus returned", history)
	}
}

type rangeProxy struct{}

func (*rangeProxy) Get(context.Context, prom.Target, string, map[string]string) ([]byte, error) {
	return []byte(`{"status":"success","data":{"resultType":"matrix","result":` +
		`[{"metric":{},"values":[[1785434552,"0.028"],[1785434612,"0.031"]]}]}}`), nil
}

func refusingResource(resource string) *k8sfake.Clientset {
	cs := k8sfake.NewClientset()
	cs.PrependReactor(
		"create",
		"selfsubjectaccessreviews",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			create, ok := action.(k8stesting.CreateAction)
			if !ok {
				return false, nil, nil
			}
			review, ok := create.GetObject().(*authv1.SelfSubjectAccessReview)
			if !ok {
				return false, nil, nil
			}
			refused := review.Spec.ResourceAttributes.Resource == resource
			review.Status = authv1.SubjectAccessReviewStatus{Allowed: !refused, Reason: "not that kind"}
			return true, review, nil
		},
	)
	return cs
}

func helmManager(t *testing.T, cs *k8sfake.Clientset, releases *helm.Service) (*Manager, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	mgr := NewManager(ctx, Deps{
		Dynamic:     newClient(t),
		Clientset:   cs,
		Helm:        releases,
		Descriptors: testDescs(),
	})
	return mgr, cancel
}

func releaseConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "prod",
			Name:      "sh.helm.release.v1.podinfo.v1",
			Labels:    map[string]string{"owner": "helm", "name": "podinfo", "version": "1"},
		},
		Data: map[string]string{"release": "body"},
	}
}

func TestHelmAccessAsksAboutWhereTheReleaseIsKept(t *testing.T) {
	cs := refusingResource("configmaps")
	store := k8sfake.NewClientset(releaseConfigMap())
	mgr, cancel := helmManager(t, cs, helm.NewService(store, nil, nil, nil, nil, api.ContextRef{}))
	defer cancel()

	result := mgr.HelmAccess(t.Context(), "prod", "podinfo")

	if len(result.Refused) != 4 {
		t.Fatalf("refused = %v, want the configmap release held back", result.Refused)
	}
}

func TestHelmAccessDoesNotAskAboutTheKindTheReleaseIsNotIn(t *testing.T) {
	cs := refusingResource("secrets")
	store := k8sfake.NewClientset(releaseConfigMap())
	mgr, cancel := helmManager(t, cs, helm.NewService(store, nil, nil, nil, nil, api.ContextRef{}))
	defer cancel()

	result := mgr.HelmAccess(t.Context(), "prod", "podinfo")

	if len(result.Refused) != 0 {
		t.Fatalf("refused = %v; a configmap release was refused over secrets", result.Refused)
	}
}

func TestAManagerUsesThePermissionsItWasHanded(t *testing.T) {
	handed := access.New(decidingClientset(false, "not for you"))
	mgr := NewManager(t.Context(), Deps{
		Dynamic:     newClient(t),
		Clientset:   decidingClientset(true, ""),
		Perms:       handed,
		Descriptors: testDescs(),
	})

	result := mgr.Access(t.Context(), deployAt("prod", "web"))

	if len(result.Refused) == 0 {
		t.Fatal("the manager answered from a service of its own rather than the one it was handed")
	}
}

func TestAManagerWithoutOneKeepsItsOwn(t *testing.T) {
	mgr := NewManager(t.Context(), Deps{
		Dynamic:     newClient(t),
		Clientset:   decidingClientset(false, "not for you"),
		Descriptors: testDescs(),
	})

	result := mgr.Access(t.Context(), deployAt("prod", "web"))

	if len(result.Refused) == 0 {
		t.Fatal("a manager with no service handed to it asked nobody")
	}
}
