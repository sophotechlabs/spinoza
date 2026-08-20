package resources

import (
	"context"
	"errors"
	"testing"
	"time"

	authv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/exec"
	"github.com/sophotechlabs/spinoza/internal/jsonschema"
	"github.com/sophotechlabs/spinoza/internal/portforward"
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

func TestAccessHoldsNothingBackWhenEverythingIsAllowed(t *testing.T) {
	mgr, cancel := managerWithClientset(t, decidingClientset(true, ""))
	defer cancel()

	result := mgr.Access(t.Context(), deployAt("prod", "web"))

	if len(result.Refused) != 0 {
		t.Fatalf("refused = %v, want nothing", result.Refused)
	}
}

// A manager with nothing wired up is what spinoza has when it was started
// without helm, without prometheus, or before a cluster answered. Every one of
// these has to say so rather than panic.
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
		_, err := mgr.HelmRelease(t.Context(), "prod", "web")
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
}
