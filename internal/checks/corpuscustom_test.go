package checks

import (
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func clusterIssuer(name, secret string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "cert-manager.io/v1",
		"kind":       "ClusterIssuer",
		"metadata":   map[string]any{"name": name},
		"spec": map[string]any{
			"acme": map[string]any{
				"privateKeySecretRef": map[string]any{"name": secret},
			},
		},
	}}
}

// what a custom resource naming something is allowed to prevent

func TestASecretACustomResourceNamesIsNotOrphaned(t *testing.T) {
	report := report(t,
		simple("Secret", "letsencrypt-account", testNamespace, nil),
		clusterIssuer("letsencrypt", "letsencrypt-account"))

	if findingCount(t, report, "orphaned-secret") != 0 {
		t.Fatal("a secret a ClusterIssuer names was reported as referenced by nothing")
	}
}

func TestASecretNothingNamesIsStillOrphaned(t *testing.T) {
	report := report(t,
		simple("Secret", "forgotten", testNamespace, nil),
		clusterIssuer("letsencrypt", "letsencrypt-account"))

	if findingCount(t, report, "orphaned-secret") != 1 {
		t.Fatal("a secret nothing names was not reported once the custom resources were read")
	}
}

func TestTheCustomResourcesAreReadWithoutBeingAskedFor(t *testing.T) {
	found := customKinds(descriptors())

	if len(found) != 1 || found[0].Resource != "clusterissuers" {
		t.Fatalf("the custom kinds were %v", found)
	}
}

func TestAnAuditOfTheWorkloadsAloneLeavesTheCustomResourcesUnread(t *testing.T) {
	report := Run(t.Context(), newLister(
		simple("Secret", "forgotten", testNamespace, nil),
		clusterIssuer("letsencrypt", "letsencrypt-account"),
	), descriptors(), api.Metrics{}, Filter{}, 0)

	if groupNamed(t, report, "orphaned-secret").Skipped == "" {
		t.Fatal("an orphan check answered from an audit that never read the custom resources")
	}
}

func TestACustomResourceIsReadWithoutACacheBehindIt(t *testing.T) {
	lister := newLister(
		simple("Secret", "letsencrypt-account", testNamespace, nil),
		clusterIssuer("letsencrypt", "letsencrypt-account"),
	)

	Run(t.Context(), lister, descriptors(), api.Metrics{}, wholeCluster(), 0)

	if lister.scanCount() == 0 {
		t.Fatal("the custom resources were read through a cache the window keeps")
	}
}

func TestWhatACustomResourceNamesIsKeptWithoutKeepingTheResource(t *testing.T) {
	lister := newLister(
		simple("Secret", "letsencrypt-account", testNamespace, nil),
		clusterIssuer("letsencrypt", "letsencrypt-account"),
	)

	report := Run(t.Context(), lister, descriptors(), api.Metrics{}, wholeCluster(), 0)

	if findingCount(t, report, "orphaned-secret") != 0 {
		t.Fatal("the name a custom resource mentions was lost with the resource")
	}
	for _, object := range report.Objects {
		if object.Kind == "ClusterIssuer" {
			t.Fatal("a custom resource was kept in the report it was only read to inform")
		}
	}
}

func TestNothingIsReadForACheckNobodyAskedFor(t *testing.T) {
	lister := newLister(
		simple("Secret", "letsencrypt-account", testNamespace, nil),
		clusterIssuer("letsencrypt", "letsencrypt-account"),
	)
	keep := wholeCluster()
	keep.Disabled = []string{"orphaned-secret", "orphaned-config-map"}

	Run(t.Context(), lister, descriptors(), api.Metrics{}, keep, 0)

	if lister.scanCount() != 0 {
		t.Fatal("the custom resources were read although nothing in the audit needed them")
	}
}

func TestTheSeverityFloorAloneCanStopTheCustomResourcesBeingRead(t *testing.T) {
	lister := newLister(
		simple("Secret", "letsencrypt-account", testNamespace, nil),
		clusterIssuer("letsencrypt", "letsencrypt-account"),
	)
	keep := wholeCluster()
	keep.MinSeverity = severityHigh

	Run(t.Context(), lister, descriptors(), api.Metrics{}, keep, 0)

	if lister.scanCount() != 0 {
		t.Fatal("a high-only audit still read every custom resource for a low check it dropped")
	}
}

func TestACustomKindThatCannotBeListedStopsTheOrphanChecks(t *testing.T) {
	lister := newLister(
		simple("Secret", "forgotten", testNamespace, nil),
		clusterIssuer("letsencrypt", "letsencrypt-account"),
	)
	lister.errs["clusterissuers"] = errors.New("clusterissuers is forbidden")

	report := Run(t.Context(), lister, descriptors(), api.Metrics{}, wholeCluster(), 0)

	if findingCount(t, report, "orphaned-secret") != 0 {
		t.Fatal("a refused custom resource listing became a secret nothing references")
	}
}
