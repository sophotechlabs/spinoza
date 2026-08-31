package checks

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func csiVolume(name, secret, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "PersistentVolume",
		"metadata":   map[string]any{"name": name},
		"spec": map[string]any{
			"capacity": map[string]any{"storage": "1Gi"},
			"csi": map[string]any{
				"driver":       "example.csi.storage",
				"volumeHandle": name,
				"nodePublishSecretRef": map[string]any{
					"name":      secret,
					"namespace": namespace,
				},
			},
		},
	}}
}

func TestASecretAPersistentVolumeMountsIsNotOrphaned(t *testing.T) {
	report := report(t,
		simple("Secret", "csi-creds", testNamespace, nil),
		csiVolume("data", "csi-creds", testNamespace))

	if findingCount(t, report, "orphaned-secret") != 0 {
		t.Fatalf("a secret a PersistentVolume mounts was offered for deletion: %+v",
			groupNamed(t, report, "orphaned-secret").Findings)
	}
}

func TestASecretNoPersistentVolumeMountsIsStillOrphaned(t *testing.T) {
	report := report(t,
		simple("Secret", "forgotten", testNamespace, nil),
		csiVolume("data", "csi-creds", testNamespace))

	if findingCount(t, report, "orphaned-secret") != 1 {
		t.Fatalf("a secret nothing names was not reported: %+v",
			groupNamed(t, report, "orphaned-secret").Findings)
	}
}

func TestThePersistentVolumesAreReadWithoutBeingAskedFor(t *testing.T) {
	found := mentionKinds(descriptors())

	if len(found) != 1 || found[0].Resource != "persistentvolumes" {
		t.Fatalf("the mention kinds were %v", found)
	}
}

func TestTheOrphanCopyOnlyClaimsEveryKindWhenEveryKindWasRead(t *testing.T) {
	partial := groupNamed(t, report(t, simple("Secret", "forgotten", testNamespace, nil)), "orphaned-secret")
	whole := groupNamed(t, reportEverything(t, simple("Secret", "forgotten", testNamespace, nil)), "orphaned-secret")

	if partial.Wrong == whole.Wrong {
		t.Fatalf("the same sentence shipped on both paths: %q", partial.Wrong)
	}
	if partial.Wrong != readsPart {
		t.Fatalf("the default path said %q", partial.Wrong)
	}
	if whole.Wrong != readsEvery {
		t.Fatalf("the everyKind path said %q", whole.Wrong)
	}
}
