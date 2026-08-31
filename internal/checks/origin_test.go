package checks

import (
	"slices"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func refFor(name string) api.ObjectRef {
	return api.ObjectRef{Namespace: testNamespace, Name: name}
}

func labeled(namespace string, labels, annotations map[string]any) *unstructured.Unstructured {
	meta := map[string]any{"name": "app", "namespace": namespace}
	if labels != nil {
		meta["labels"] = labels
	}
	if annotations != nil {
		meta["annotations"] = annotations
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   meta,
	}}
}

func TestOriginNamesWhatDeployedTheObject(t *testing.T) {
	cases := []struct {
		name        string
		obj         *unstructured.Unstructured
		wantOrigin  string
		wantManager string
	}{
		{
			name:       "a workload you applied yourself",
			obj:        labeled(testNamespace, nil, nil),
			wantOrigin: "",
		},
		{
			name:        "a helm release",
			obj:         labeled(testNamespace, nil, map[string]any{helmReleaseAnnotation: "podinfo"}),
			wantOrigin:  originPackaged,
			wantManager: "Helm: podinfo",
		},
		{
			name:        "something that only claims helm managed it",
			obj:         labeled(testNamespace, map[string]any{helmManagedLabel: helmManagerName}, nil),
			wantOrigin:  originPackaged,
			wantManager: "Helm",
		},
		{
			name:        "a flux kustomization",
			obj:         labeled(testNamespace, map[string]any{fluxKustomizeLabel: "apps"}, nil),
			wantOrigin:  originPackaged,
			wantManager: "Flux: apps",
		},
		{
			name:        "an argo application",
			obj:         labeled(testNamespace, map[string]any{argoInstanceLabel: "store"}, nil),
			wantOrigin:  originPackaged,
			wantManager: "Argo: store",
		},
		{
			name:       "the distribution's own workload",
			obj:        labeled("kube-system", nil, nil),
			wantOrigin: originSystem,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			origin, manager := originOf(tc.obj)
			if origin != tc.wantOrigin {
				t.Fatalf("origin = %q, want %q", origin, tc.wantOrigin)
			}
			if manager != tc.wantManager {
				t.Fatalf("managed by = %q, want %q", manager, tc.wantManager)
			}
		})
	}
}

func TestAHelmReleaseFluxInstalledIsNamedAfterItsHelmRelease(t *testing.T) {
	obj := labeled(testNamespace,
		map[string]any{fluxHelmLabel: "ingress-nginx", helmManagedLabel: helmManagerName},
		map[string]any{helmReleaseAnnotation: "ingress-nginx"})

	origin, manager := originOf(obj)

	if origin != originPackaged {
		t.Fatalf("origin = %q, want %q", origin, originPackaged)
	}
	if manager != "Flux: ingress-nginx" {
		t.Fatalf("managed by = %q, want the HelmRelease that owns it, not the chart", manager)
	}
}

func TestAPackagedWorkloadInASystemNamespaceIsStillPackaged(t *testing.T) {
	obj := labeled("kube-system", map[string]any{fluxKustomizeLabel: "infra"}, nil)

	origin, manager := originOf(obj)

	if origin != originPackaged {
		t.Fatalf("origin = %q, want %q because you can still change it", origin, originPackaged)
	}
	if manager != "Flux: infra" {
		t.Fatalf("managed by = %q, want %q", manager, "Flux: infra")
	}
}

func TestYoursSortsAheadOfPackagedAndPackagedAheadOfTheDistribution(t *testing.T) {
	mine := Subject{Ref: refFor("z-last-alphabetically"), Kind: "Deployment"}
	packaged := Subject{Ref: refFor("a-first-alphabetically"), Kind: "Deployment", Origin: originPackaged}
	system := Subject{Ref: refFor("a-first-alphabetically"), Kind: "Deployment", Origin: originSystem}

	keys := []string{subjectKey(system), subjectKey(mine), subjectKey(packaged)}
	slices.Sort(keys)

	if keys[0] != subjectKey(mine) {
		t.Fatalf("your own workload did not sort first, even named last alphabetically")
	}
	if keys[1] != subjectKey(packaged) {
		t.Fatalf("a packaged workload did not sort ahead of the distribution's")
	}
}

func TestOrderingWithinOneOriginStaysAlphabetical(t *testing.T) {
	first := Subject{Ref: refFor("alpha"), Kind: "Deployment", Origin: originPackaged}
	second := Subject{Ref: refFor("beta"), Kind: "Deployment", Origin: originPackaged}

	if subjectKey(first) >= subjectKey(second) {
		t.Fatalf("%q did not sort before %q", subjectKey(first), subjectKey(second))
	}
}

func TestAFindingCursorKeepsOriginAheadOfTheName(t *testing.T) {
	mine := found{subject: Subject{Ref: refFor("zulu"), Kind: "Deployment"}, container: "app"}
	theirs := found{subject: Subject{Ref: refFor("alpha"), Kind: "Deployment", Origin: originSystem}, container: "app"}

	if findingKey(mine) >= findingKey(theirs) {
		t.Fatalf("the cursor would page the distribution's findings before yours")
	}
}
