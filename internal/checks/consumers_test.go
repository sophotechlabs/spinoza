package checks

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func ownedSecret(name, owner string) *unstructured.Unstructured {
	obj := simple("Secret", name, testNamespace, nil)
	obj.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "v1", Kind: "Node", Name: owner}})
	return obj
}

func managedSecret(name string, labels map[string]string) *unstructured.Unstructured {
	obj := simple("Secret", name, testNamespace, nil)
	obj.SetLabels(labels)
	return obj
}

func mutedFinding(t *testing.T, id string, objects ...*unstructured.Unstructured) api.CheckFinding {
	t.Helper()
	keep := everyKind()
	keep.ShowMuted = true
	report := Run(t.Context(), newLister(objects...), descriptors(), api.Metrics{}, keep, 0)
	return onlyFinding(t, report, id)
}

// what stops an object looking like a leftover

func TestSomethingWithAnOwnerIsNotALeftover(t *testing.T) {
	finding := mutedFinding(t, "orphaned-secret", ownedSecret("node-password", "worker-1"))

	if finding.MutedBy != ScopeConvention {
		t.Fatalf("an owned secret was reported as %q", finding.MutedBy)
	}
	if !strings.Contains(finding.Reason, "owns it") {
		t.Fatalf("the reason was %q", finding.Reason)
	}
}

func TestSomethingAnOperatorManagesIsNotALeftover(t *testing.T) {
	finding := mutedFinding(t, "orphaned-secret",
		managedSecret("cilium-ca", map[string]string{helmManagedLabel: "Helm"}))

	if finding.MutedBy != ScopeConvention {
		t.Fatalf("a managed secret was reported as %q", finding.MutedBy)
	}
	if !strings.Contains(finding.Reason, "Helm") {
		t.Fatalf("the reason was %q, want the manager named", finding.Reason)
	}
}

func TestFluxCountsAsTheManagerWhateverElseTheLabelsSay(t *testing.T) {
	finding := mutedFinding(t, "orphaned-secret", managedSecret("cilium-ca", map[string]string{
		helmManagedLabel:              "Helm",
		"helm.toolkit.fluxcd.io/name": "cilium",
	}))

	if !strings.Contains(finding.Reason, "Flux") {
		t.Fatalf("the reason was %q, want Flux named over Helm", finding.Reason)
	}
}

func TestWhatIsKnownToBeReadWithoutBeingNamedIsNotALeftover(t *testing.T) {
	cases := []struct {
		name, id string
		object   *unstructured.Unstructured
		who      string
	}{
		{
			"the CA bundle every namespace gets", "orphaned-config-map",
			configMap("kube-root-ca.crt", map[string]any{"ca.crt": "..."}), "controller manager",
		},
		{
			"a Helm release record", "orphaned-secret",
			simple("Secret", "sh.helm.release.v1.beyla.v3", testNamespace, nil), "Helm",
		},
		{
			"the k3s serving certificate", "orphaned-secret",
			inNamespace(simple("Secret", "k3s-serving", testNamespace, nil), "kube-system"), "k3s",
		},
		{
			"a k3s node password", "orphaned-secret",
			inNamespace(simple("Secret", "worker-1.node-password.k3s", testNamespace, nil), "kube-system"), "k3s",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			finding := mutedFinding(t, tc.id, tc.object)

			if finding.MutedBy != ScopeConvention {
				t.Fatalf("it was reported as %q", finding.MutedBy)
			}
			if !strings.Contains(finding.Reason, tc.who) {
				t.Fatalf("the reason was %q, want %s named", finding.Reason, tc.who)
			}
		})
	}
}

func TestTheSameNameInAnotherNamespaceIsStillALeftover(t *testing.T) {
	report := reportEverything(t, simple("Secret", "k3s-serving", testNamespace, nil))

	if findingCount(t, report, "orphaned-secret") != 1 {
		t.Fatal("a secret named after a k3s one, somewhere k3s does not put it, was let through")
	}
}

func TestSomethingNobodyOwnsOrManagesIsStillALeftover(t *testing.T) {
	report := reportEverything(t, simple("Secret", "forgotten", testNamespace, nil))

	finding := onlyFinding(t, report, "orphaned-secret")
	if finding.MutedBy != "" {
		t.Fatalf("a genuine leftover was silenced as %q", finding.MutedBy)
	}
}

func TestSilencedLeftoversAreCountedRatherThanHidden(t *testing.T) {
	report := reportEverything(t,
		simple("Secret", "sh.helm.release.v1.beyla.v3", testNamespace, nil),
		simple("Secret", "forgotten", testNamespace, nil))

	group := groupNamed(t, report, "orphaned-secret")
	if group.Total != 1 {
		t.Fatalf("reported %d findings, want only the genuine one", group.Total)
	}
	if group.Muted != 1 {
		t.Fatalf("counted %d silenced, want the Helm record counted rather than dropped", group.Muted)
	}
}
