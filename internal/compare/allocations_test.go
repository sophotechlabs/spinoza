package compare

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func service(clusterIP string, nodePort int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       serviceKind,
		"metadata":   map[string]any{"name": "web", "namespace": "prod"},
		"spec": map[string]any{
			"type":                  "NodePort",
			"clusterIP":             clusterIP,
			"clusterIPs":            []any{clusterIP},
			"healthCheckNodePort":   nodePort + 1,
			"externalTrafficPolicy": "Cluster",
			"selector":              map[string]any{"app": "web"},
			"ports": []any{map[string]any{
				"name":       "http",
				"port":       int64(80),
				"targetPort": int64(8080),
				"nodePort":   nodePort,
			}},
		},
	}}
}

func onlyPort(t *testing.T, clean *unstructured.Unstructured) map[string]any {
	t.Helper()
	ports, found, err := unstructured.NestedSlice(clean.Object, "spec", "ports")
	if !found || err != nil || len(ports) != 1 {
		t.Fatalf("spec.ports = %v found=%v err=%v", ports, found, err)
	}
	one, ok := ports[0].(map[string]any)
	if !ok {
		t.Fatalf("port entry = %#v", ports[0])
	}
	return one
}

func TestTwoClustersServicesMatchOnceTheAllocationsAreDropped(t *testing.T) {
	here := rendered(t, Normalise(service("10.43.7.19", 31877)))
	there := rendered(t, Normalise(service("10.96.204.3", 30022)))

	if here != there {
		t.Fatalf("the same Service differed across clusters:\n%s\n---\n%s", here, there)
	}
}

func TestWhatTheAuthorWroteSurvivesTheService(t *testing.T) {
	clean := Normalise(service("10.43.7.19", 31877))

	kind, found, err := unstructured.NestedString(clean.Object, "spec", "type")
	if !found || err != nil || kind != "NodePort" {
		t.Fatalf("spec.type = %q found=%v err=%v", kind, found, err)
	}
	one := onlyPort(t, clean)
	if one["port"] != int64(80) || one["targetPort"] != int64(8080) || one["name"] != "http" {
		t.Fatalf("the authored port fields were lost: %#v", one)
	}
	if _, still := one["nodePort"]; still {
		t.Fatalf("nodePort survived: %#v", one)
	}
}

func TestAHeadlessServiceKeepsItsClusterIP(t *testing.T) {
	clean := Normalise(service("None", 0))

	value, found, err := unstructured.NestedString(clean.Object, "spec", "clusterIP")
	if !found || err != nil || value != "None" {
		t.Fatalf("clusterIP = %q found=%v err=%v, want None kept so a headless Service still "+
			"differs from an allocated one", value, found, err)
	}
	list, found, err := unstructured.NestedStringSlice(clean.Object, "spec", "clusterIPs")
	if !found || err != nil || len(list) != 1 || list[0] != "None" {
		t.Fatalf("clusterIPs = %v found=%v err=%v", list, found, err)
	}
}

func claim(volume string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       claimKind,
		"metadata":   map[string]any{"name": "data", "namespace": "prod"},
		"spec": map[string]any{
			"accessModes": []any{"ReadWriteOnce"},
			"volumeName":  volume,
			"resources":   map[string]any{"requests": map[string]any{"storage": "10Gi"}},
		},
	}}
}

func TestABoundClaimLosesTheVolumeTheServerPickedForIt(t *testing.T) {
	here := rendered(t, Normalise(claim("pvc-2a1f")))
	there := rendered(t, Normalise(claim("pvc-9c40")))

	if here != there {
		t.Fatalf("the same claim differed across clusters:\n%s\n---\n%s", here, there)
	}
	if !strings.Contains(here, "10Gi") {
		t.Fatalf("the authored request was lost:\n%s", here)
	}
}

func webhook(bundle string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "admissionregistration.k8s.io/v1",
		"kind":       validatingKind,
		"metadata":   map[string]any{"name": "guard"},
		"webhooks": []any{map[string]any{
			"name": "guard.example.com",
			"clientConfig": map[string]any{
				"caBundle": bundle,
				"service":  map[string]any{"name": "guard", "namespace": "prod"},
			},
		}},
	}}
}

func TestAWebhookLosesTheCABundleEachClusterInjects(t *testing.T) {
	here := rendered(t, Normalise(webhook("LS0tLXdoYXRldmVy")))
	there := rendered(t, Normalise(webhook("LS0tLXNvbWV0aGluZw==")))

	if here != there {
		t.Fatalf("the same webhook differed across clusters:\n%s\n---\n%s", here, there)
	}
	if !strings.Contains(here, "guard.example.com") {
		t.Fatalf("the authored webhook was lost:\n%s", here)
	}
}

func TestAKindWithNoAllocationsKeepsItsSpec(t *testing.T) {
	clean := rendered(t, Normalise(deployment()))

	if !strings.Contains(clean, "replicas") {
		t.Fatalf("the Deployment lost its spec:\n%s", clean)
	}
}
