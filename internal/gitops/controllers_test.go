package gitops

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func deployment(namespace, name, partOf string, ready, wanted int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Labels:    map[string]string{partOfLabel: partOf},
		},
		Spec:   appsv1.DeploymentSpec{Replicas: &wanted},
		Status: appsv1.DeploymentStatus{ReadyReplicas: ready},
	}
}

func statefulSet(namespace, name, partOf string, ready, wanted int32) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Labels:    map[string]string{partOfLabel: partOf},
		},
		Spec:   appsv1.StatefulSetSpec{Replicas: &wanted},
		Status: appsv1.StatefulSetStatus{ReadyReplicas: ready},
	}
}

func TestControllersFindBothEcosystems(t *testing.T) {
	cs := k8sfake.NewClientset(
		deployment("flux-system", "source-controller", "flux", 1, 1),
		deployment("argocd", "argocd-repo-server", "argocd", 1, 1),
		statefulSet("argocd", "argocd-application-controller", "argocd", 1, 1),
	)

	found := Controllers(t.Context(), cs)

	if len(found) != 3 {
		t.Fatalf("controllers = %+v, want three", found)
	}
	if found[0].Controller != api.ControllerArgo || found[0].Name != "argocd-application-controller" {
		t.Fatalf("first = %+v, want argo sorted first", found[0])
	}
	if found[2].Controller != api.ControllerFlux {
		t.Fatalf("last = %+v, want the flux one", found[2])
	}
}

func TestControllersReportWhatIsNotRunning(t *testing.T) {
	cs := k8sfake.NewClientset(deployment("argocd", "argocd-server", "argocd", 0, 2))

	found := Controllers(t.Context(), cs)

	if found[0].Ready != 0 || found[0].Wanted != 2 {
		t.Fatalf("controller = %+v", found[0])
	}
}

func TestControllersTakeOneReplicaAsTheDefault(t *testing.T) {
	pod := deployment("argocd", "argocd-server", "argocd", 1, 1)
	pod.Spec.Replicas = nil
	cs := k8sfake.NewClientset(pod)

	found := Controllers(t.Context(), cs)

	if found[0].Wanted != 1 {
		t.Fatalf("wanted = %d, want the default of one", found[0].Wanted)
	}
}

func TestControllersIgnoreEverythingElse(t *testing.T) {
	cs := k8sfake.NewClientset(deployment("shop", "web", "shop", 1, 1))

	if found := Controllers(t.Context(), cs); found != nil {
		t.Fatalf("controllers = %+v, want none on a cluster with no gitops", found)
	}
}

func TestControllersAnswerNothingWithoutAClient(t *testing.T) {
	if found := Controllers(t.Context(), nil); found != nil {
		t.Fatalf("controllers = %+v, want none", found)
	}
}
