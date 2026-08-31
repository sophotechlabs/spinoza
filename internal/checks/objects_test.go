package checks

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func annotatedObject(obj *unstructured.Unstructured, key, value string) *unstructured.Unstructured {
	meta, ok := obj.Object["metadata"].(map[string]any)
	if ok {
		meta["annotations"] = map[string]any{key: value}
	}
	return obj
}

func ingressOn(host string, tls bool) *unstructured.Unstructured {
	spec := map[string]any{
		"rules": []any{map[string]any{"host": host}},
	}
	if tls {
		spec["tls"] = []any{map[string]any{"hosts": []any{host}, "secretName": "cert"}}
	}
	return simple("Ingress", "api", testNamespace, spec)
}

func policyWith(spec map[string]any) *unstructured.Unstructured {
	return simple("NetworkPolicy", "guard", testNamespace, spec)
}

func budgetWith(spec map[string]any) *unstructured.Unstructured {
	return simple("PodDisruptionBudget", "api", testNamespace, spec)
}

func scalerWith(spec map[string]any) *unstructured.Unstructured {
	return simple("HorizontalPodAutoscaler", "api", testNamespace, spec)
}

func TestEveryObjectCheckFiresOnItsOwnFaultAndOnNothingElse(t *testing.T) {
	settledPolicy := policyWith(map[string]any{
		"podSelector": map[string]any{"matchLabels": map[string]any{"app": "api"}},
		"ingress":     []any{map[string]any{"from": []any{map[string]any{"ipBlock": map[string]any{"cidr": "10.0.0.0/8"}}}}},
	})
	settledBudget := budgetWith(map[string]any{
		"minAvailable": int64(1),
		unhealthyField: "AlwaysAllow",
		"selector":     map[string]any{"matchLabels": map[string]any{"app": "api"}},
	})
	settledScaler := scalerWith(map[string]any{
		"minReplicas": int64(2),
		"maxReplicas": int64(6),
		"metrics":     []any{map[string]any{"type": "Resource"}},
	})
	settledClaim := simple("PersistentVolumeClaim", "data", testNamespace,
		map[string]any{"storageClassName": "fast"})
	settledClass := simple("StorageClass", "fast", "", nil)
	settledClass.Object["reclaimPolicy"] = "Retain"

	cases := []struct {
		id      string
		objects []*unstructured.Unstructured
		clean   []*unstructured.Unstructured
	}{
		{
			id:      "service-load-balancer",
			objects: []*unstructured.Unstructured{simple("Service", "api", testNamespace, map[string]any{"type": loadBalancer})},
			clean:   []*unstructured.Unstructured{service("api", map[string]any{"app": "api"})},
		},
		{
			id:      "service-node-port",
			objects: []*unstructured.Unstructured{simple("Service", "api", testNamespace, map[string]any{"type": nodePort})},
			clean:   []*unstructured.Unstructured{service("api", map[string]any{"app": "api"})},
		},
		{
			id: "service-external-ips",
			objects: []*unstructured.Unstructured{simple("Service", "api", testNamespace, map[string]any{
				"externalIPs": []any{"203.0.113.5"},
			})},
			clean: []*unstructured.Unstructured{service("api", map[string]any{"app": "api"})},
		},
		{
			id:      "ingress-no-tls",
			objects: []*unstructured.Unstructured{ingressOn("api.example", false)},
			clean:   []*unstructured.Unstructured{ingressOn("api.example", true)},
		},
		{
			id:      "ingress-wildcard-host",
			objects: []*unstructured.Unstructured{ingressOn("*.example", true)},
			clean:   []*unstructured.Unstructured{ingressOn("api.example", true)},
		},
		{
			id: "ingress-snippet-annotation",
			objects: []*unstructured.Unstructured{annotatedObject(ingressOn("api.example", true),
				"nginx.ingress.kubernetes.io/configuration-snippet", "more_set_headers \"X: y\";")},
			clean: []*unstructured.Unstructured{ingressOn("api.example", true)},
		},
		{
			id: "policy-allows-everything",
			objects: []*unstructured.Unstructured{policyWith(map[string]any{
				"podSelector": map[string]any{},
				"ingress":     []any{map[string]any{}},
			})},
			clean: []*unstructured.Unstructured{settledPolicy},
		},
		{
			id: "policy-open-ip-block",
			objects: []*unstructured.Unstructured{policyWith(map[string]any{
				"podSelector": map[string]any{"matchLabels": map[string]any{"app": "api"}},
				"egress":      []any{map[string]any{"to": []any{map[string]any{"ipBlock": map[string]any{"cidr": openCIDR}}}}},
			})},
			clean: []*unstructured.Unstructured{settledPolicy},
		},
		{
			id:      "pdb-no-policy",
			objects: []*unstructured.Unstructured{budgetWith(map[string]any{unhealthyField: "AlwaysAllow"})},
			clean:   []*unstructured.Unstructured{settledBudget},
		},
		{
			id: "pdb-blocks-all",
			objects: []*unstructured.Unstructured{budgetWith(map[string]any{
				"maxUnavailable": int64(0), unhealthyField: "AlwaysAllow",
			})},
			clean: []*unstructured.Unstructured{settledBudget},
		},
		{
			id:      "pdb-unhealthy-eviction",
			objects: []*unstructured.Unstructured{budgetWith(map[string]any{"minAvailable": int64(1)})},
			clean:   []*unstructured.Unstructured{settledBudget},
		},
		{
			id:      "claim-no-storage-class",
			objects: []*unstructured.Unstructured{simple("PersistentVolumeClaim", "data", testNamespace, map[string]any{})},
			clean:   []*unstructured.Unstructured{settledClaim},
		},
		{
			id: "storage-class-deletes-data",
			objects: []*unstructured.Unstructured{func() *unstructured.Unstructured {
				class := simple("StorageClass", "fast", "", nil)
				class.Object["reclaimPolicy"] = deleteReclaim
				return class
			}()},
			clean: []*unstructured.Unstructured{settledClass},
		},
		{
			id: "hpa-floor-of-one",
			objects: []*unstructured.Unstructured{scalerWith(map[string]any{
				"minReplicas": int64(1), "maxReplicas": int64(6),
				"metrics": []any{map[string]any{"type": "Resource"}},
			})},
			clean: []*unstructured.Unstructured{settledScaler},
		},
		{
			id: "hpa-no-metrics",
			objects: []*unstructured.Unstructured{scalerWith(map[string]any{
				"minReplicas": int64(2), "maxReplicas": int64(6),
			})},
			clean: []*unstructured.Unstructured{settledScaler},
		},
	}

	registered := map[string]bool{}
	for _, entry := range objectChecks() {
		registered[entry.id] = true
	}
	if len(cases) != len(registered) {
		t.Fatalf("%d cases cover %d registered object checks", len(cases), len(registered))
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			if !registered[tc.id] {
				t.Fatalf("%s is not a registered object check", tc.id)
			}
			if findingCount(t, report(t, tc.objects...), tc.id) == 0 {
				t.Fatalf("%s did not fire on the object written to trip it", tc.id)
			}
			if findingCount(t, report(t, tc.clean...), tc.id) != 0 {
				t.Fatalf("%s fired on an object that satisfies it", tc.id)
			}
		})
	}
}

func TestAClusterIPServiceIsNotPublished(t *testing.T) {
	found := report(t, simple("Service", "api", testNamespace, map[string]any{"type": "ClusterIP"}))

	for _, id := range []string{"service-load-balancer", "service-node-port", "service-external-ips"} {
		if findingCount(t, found, id) != 0 {
			t.Fatalf("%s fired on a ClusterIP service", id)
		}
	}
}

func TestAnIngressWithNoRulesStillSaysItServesPlaintext(t *testing.T) {
	bare := simple("Ingress", "api", testNamespace, map[string]any{})

	if findingCount(t, report(t, bare), "ingress-no-tls") != 1 {
		t.Fatal("an Ingress with no rules and no TLS was not reported")
	}
}

func TestAPolicyThatNamesItsPodsIsNotOpen(t *testing.T) {
	narrow := policyWith(map[string]any{
		"podSelector": map[string]any{"matchLabels": map[string]any{"app": "api"}},
		"ingress":     []any{map[string]any{}},
	})

	if findingCount(t, report(t, narrow), "policy-allows-everything") != 0 {
		t.Fatal("a policy naming its pods was reported as allowing everything")
	}
}

func TestAPolicyWithARuleThatNamesSomethingIsNotOpen(t *testing.T) {
	narrow := policyWith(map[string]any{
		"podSelector": map[string]any{},
		"ingress": []any{map[string]any{
			"from": []any{map[string]any{"podSelector": map[string]any{}}},
		}},
	})

	if findingCount(t, report(t, narrow), "policy-allows-everything") != 0 {
		t.Fatal("a rule that names a source was reported as empty")
	}
}

func TestAPinnedAutoscalerIsNotToldToRaiseItsFloor(t *testing.T) {
	pinned := scalerWith(map[string]any{
		"minReplicas": int64(1), "maxReplicas": int64(1),
		"metrics": []any{map[string]any{"type": "Resource"}},
	})

	if findingCount(t, report(t, pinned), "hpa-floor-of-one") != 0 {
		t.Fatal("an autoscaler pinned at one was told to raise its floor")
	}
}

func TestTheLegacyTargetCountsAsAMetric(t *testing.T) {
	legacy := scalerWith(map[string]any{
		"minReplicas": int64(2), "maxReplicas": int64(6),
		"targetCPUUtilizationPercentage": int64(80),
	})

	if findingCount(t, report(t, legacy), "hpa-no-metrics") != 0 {
		t.Fatal("the autoscaling/v1 CPU target was not read as a metric")
	}
}

func TestObjectsOfTheWrongShapeAreSkipped(t *testing.T) {
	oddPolicy := policyWith(map[string]any{
		"podSelector": map[string]any{},
		"ingress":     []any{"not-an-object", map[string]any{"from": "not-a-list"}},
		"egress":      []any{map[string]any{"to": []any{"not-an-object", map[string]any{"ipBlock": "not-an-object"}}}},
	})
	oddIngress := simple("Ingress", "api", testNamespace, map[string]any{
		"rules": []any{"not-an-object"},
		"tls":   []any{map[string]any{}},
	})
	oddClass := simple("StorageClass", "fast", "", nil)
	oddClass.Object["reclaimPolicy"] = int64(7)

	found := report(t, oddPolicy, oddIngress, oddClass)

	for _, id := range []string{
		"policy-allows-everything",
		"policy-open-ip-block",
		"ingress-wildcard-host",
		"ingress-no-tls",
		"storage-class-deletes-data",
	} {
		if findingCount(t, found, id) != 0 {
			t.Fatalf("%s reported something from an object of the wrong shape", id)
		}
	}
}

func TestTheObjectChecksAreSkippedWithoutTheWiderCorpus(t *testing.T) {
	report := Run(t.Context(), newLister(), descriptors(), api.Metrics{}, Filter{}, 0)

	for _, id := range []string{"service-load-balancer", "pdb-no-policy", "hpa-no-metrics"} {
		if group := groupNamed(t, report, id); group.Skipped == "" {
			t.Fatalf("%s ran on a workload-only audit", id)
		}
	}
}
