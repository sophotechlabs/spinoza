package checks

import (
	"maps"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func node(name string, labels, spec, allocatable map[string]any) *unstructured.Unstructured {
	meta := map[string]any{"name": name}
	if labels != nil {
		meta["labels"] = labels
	}
	if spec == nil {
		spec = map[string]any{}
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata":   meta,
		"spec":       spec,
		"status":     map[string]any{"allocatable": allocatable},
	}}
}

func plainNode(name string, labels map[string]any) *unstructured.Unstructured {
	return node(name, labels, nil, map[string]any{"cpu": "4", "memory": "8Gi"})
}

func namespaceObj(name string, labels map[string]any) *unstructured.Unstructured {
	meta := map[string]any{"name": name}
	if labels != nil {
		meta["labels"] = labels
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata":   meta,
	}}
}

func limitRange(namespace string, bounds map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "LimitRange",
		"metadata":   map[string]any{"name": "limits", "namespace": namespace},
		"spec": map[string]any{
			"limits": []any{mergeInto(map[string]any{"type": containerLimits}, bounds)},
		},
	}}
}

func mergeInto(base, extra map[string]any) map[string]any {
	maps.Copy(base, extra)
	return base
}

func resourceQuota(namespace string, hard, used map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ResourceQuota",
		"metadata":   map[string]any{"name": "team", "namespace": namespace},
		"status":     map[string]any{"hard": hard, "used": used},
	}}
}

func onNodeSelector(spec, selector map[string]any) map[string]any {
	spec["nodeSelector"] = selector
	return spec
}

func TestEveryFactCheckFiresOnItsOwnFaultAndOnNothingElse(t *testing.T) {
	cases := []struct {
		id      string
		objects []*unstructured.Unstructured
		clean   []*unstructured.Unstructured
	}{
		{
			id: "node-selector-matches-nothing",
			objects: []*unstructured.Unstructured{
				plainNode("worker", map[string]any{hostnameKey: "worker"}),
				deployment("api", onNodeSelector(podSpec(container("app", nil)), map[string]any{"disk": "nvme"})),
			},
			clean: []*unstructured.Unstructured{
				plainNode("worker", map[string]any{hostnameKey: "worker", "disk": "nvme"}),
				deployment("api", onNodeSelector(podSpec(container("app", nil)), map[string]any{"disk": "nvme"})),
			},
		},
		{
			id: "tolerations-miss-the-taints",
			objects: []*unstructured.Unstructured{
				node("worker", map[string]any{hostnameKey: "worker"}, map[string]any{
					"taints": []any{map[string]any{"key": "gpu", "value": "yes", "effect": "NoSchedule"}},
				}, map[string]any{"cpu": "4", "memory": "8Gi"}),
				deployment("api", podSpec(container("app", nil))),
			},
			clean: []*unstructured.Unstructured{
				plainNode("worker", map[string]any{hostnameKey: "worker"}),
				deployment("api", podSpec(container("app", nil))),
			},
		},
		{
			id: "request-exceeds-largest-node",
			objects: []*unstructured.Unstructured{
				plainNode("worker", map[string]any{hostnameKey: "worker"}),
				deployment("api", podSpec(container("app", resources(requests, map[string]any{"cpu": "16"})))),
			},
			clean: []*unstructured.Unstructured{
				plainNode("worker", map[string]any{hostnameKey: "worker"}),
				deployment("api", podSpec(container("app", resources(requests, map[string]any{"cpu": "1"})))),
			},
		},
		{
			id: "spread-needs-more-domains",
			objects: []*unstructured.Unstructured{
				plainNode("worker", map[string]any{hostnameKey: "worker", "zone": "a"}),
				replicas(deployment("api", podSpecWith(map[string]any{
					"topologySpreadConstraints": []any{map[string]any{
						"topologyKey": "zone", "whenUnsatisfiable": doNotSchedule, "maxSkew": int64(1),
					}},
				}, container("app", nil))), 4),
			},
			clean: []*unstructured.Unstructured{
				plainNode("worker", map[string]any{hostnameKey: "worker", "zone": "a"}),
				replicas(deployment("api", podSpecWith(map[string]any{
					"topologySpreadConstraints": []any{map[string]any{
						"topologyKey": "zone", "whenUnsatisfiable": "ScheduleAnyway", "maxSkew": int64(1),
					}},
				}, container("app", nil))), 4),
			},
		},
		{
			id: "anti-affinity-exceeds-nodes",
			objects: []*unstructured.Unstructured{
				plainNode("worker", map[string]any{hostnameKey: "worker"}),
				replicas(deployment("api", podSpecWith(map[string]any{
					"affinity": map[string]any{"podAntiAffinity": map[string]any{
						"requiredDuringSchedulingIgnoredDuringExecution": []any{
							map[string]any{"topologyKey": hostnameKey},
						},
					}},
				}, container("app", nil))), 3),
			},
			clean: []*unstructured.Unstructured{
				plainNode("worker", map[string]any{hostnameKey: "worker"}),
				replicas(deployment("api", podSpecWith(map[string]any{
					"affinity": map[string]any{"podAntiAffinity": map[string]any{
						"preferredDuringSchedulingIgnoredDuringExecution": []any{
							map[string]any{"topologyKey": hostnameKey},
						},
					}},
				}, container("app", nil))), 3),
			},
		},
		{
			id: "outside-limit-range",
			objects: []*unstructured.Unstructured{
				limitRange(testNamespace, map[string]any{"min": map[string]any{"memory": "128Mi"}}),
				deployment("api", podSpec(container("app", resources(requests, map[string]any{"memory": "16Mi"})))),
			},
			clean: []*unstructured.Unstructured{
				limitRange(testNamespace, map[string]any{"min": map[string]any{"memory": "128Mi"}}),
				deployment("api", podSpec(container("app", resources(requests, map[string]any{"memory": "256Mi"})))),
			},
		},
		{
			id: "quota-nearly-exhausted",
			objects: []*unstructured.Unstructured{
				resourceQuota(testNamespace,
					map[string]any{"requests.memory": "10Gi"},
					map[string]any{"requests.memory": "10Gi"}),
				deployment("api", podSpec(container("app", nil))),
			},
			clean: []*unstructured.Unstructured{
				resourceQuota(testNamespace,
					map[string]any{"requests.memory": "10Gi"},
					map[string]any{"requests.memory": "1Gi"}),
				deployment("api", podSpec(container("app", nil))),
			},
		},
		{
			id: "pod-security-would-reject",
			objects: []*unstructured.Unstructured{
				namespaceObj(testNamespace, map[string]any{enforceLabel: profileBaseline}),
				deployment("api", podSpecWith(map[string]any{"hostPID": true}, container("app", nil))),
			},
			clean: []*unstructured.Unstructured{
				namespaceObj(testNamespace, map[string]any{enforceLabel: profileBaseline}),
				deployment("api", podSpec(container("app", nil))),
			},
		},
	}

	registered := map[string]bool{}
	for _, entry := range factChecks() {
		registered[entry.id] = true
	}
	if len(cases) != len(registered) {
		t.Fatalf("%d cases cover %d registered fact checks", len(cases), len(registered))
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			if findingCount(t, report(t, tc.objects...), tc.id) == 0 {
				t.Fatalf("%s did not fire on the cluster written to trip it", tc.id)
			}
			if findingCount(t, report(t, tc.clean...), tc.id) != 0 {
				t.Fatalf("%s fired on a cluster that satisfies it", tc.id)
			}
		})
	}
}

func TestACheckSaysSoWhenTheClusterNeverReportedWhatItNeeds(t *testing.T) {
	descs := descriptors()
	delete(descs, "/v1/nodes")

	found := Run(t.Context(), newLister(), descs, api.Metrics{}, wholeCluster(), 0)

	group := groupNamed(t, found, "node-selector-matches-nothing")
	if group.Skipped == "" {
		t.Fatal("a check needing nodes ran on a cluster that reported none")
	}
	if !strings.Contains(group.Skipped, "nodes") {
		t.Fatalf("skipped said %q, want it to name nodes", group.Skipped)
	}
}

func TestAChecksNeedsAreNamedForEveryFactCheck(t *testing.T) {
	for _, entry := range factChecks() {
		if len(entry.needs) == 0 {
			t.Fatalf("%s reads the cluster's shape but declares no needs", entry.id)
		}
	}
}

func TestATolerationCoversTheTaintItNames(t *testing.T) {
	tainted := node("worker", map[string]any{hostnameKey: "worker"}, map[string]any{
		"taints": []any{map[string]any{"key": "gpu", "value": "yes", "effect": "NoSchedule"}},
	}, map[string]any{"cpu": "4", "memory": "8Gi"})

	tolerated := deployment("api", podSpecWith(map[string]any{
		"tolerations": []any{map[string]any{
			"key": "gpu", "value": "yes", "effect": "NoSchedule", "operator": "Equal",
		}},
	}, container("app", nil)))
	if findingCount(t, report(t, tainted, tolerated), "tolerations-miss-the-taints") != 0 {
		t.Fatal("a matching toleration was not accepted")
	}

	exists := deployment("api", podSpecWith(map[string]any{
		"tolerations": []any{map[string]any{"operator": "Exists"}},
	}, container("app", nil)))
	if findingCount(t, report(t, tainted, exists), "tolerations-miss-the-taints") != 0 {
		t.Fatal("a blanket Exists toleration was not accepted")
	}
}

func TestAMalformedTolerationDoesNotHideALaterMatch(t *testing.T) {
	tainted := node("worker", map[string]any{hostnameKey: "worker"}, map[string]any{
		"taints": []any{map[string]any{"key": "gpu", "value": "yes", "effect": "NoSchedule"}},
	}, map[string]any{"cpu": "4", "memory": "8Gi"})
	tolerated := deployment("api", podSpecWith(map[string]any{
		"tolerations": []any{
			"not-an-object",
			map[string]any{"key": "gpu", "value": "yes", "effect": "NoSchedule", "operator": "Equal"},
		},
	}, container("app", nil)))

	if findingCount(t, report(t, tainted, tolerated), "tolerations-miss-the-taints") != 0 {
		t.Fatal("a malformed toleration hid a later matching one")
	}
}

func TestAPreferNoScheduleTaintDoesNotBlockAnything(t *testing.T) {
	soft := node("worker", map[string]any{hostnameKey: "worker"}, map[string]any{
		"taints": []any{map[string]any{"key": "spot", "effect": "PreferNoSchedule"}},
	}, map[string]any{"cpu": "4", "memory": "8Gi"})

	found := report(t, soft, deployment("api", podSpec(container("app", nil))))

	if findingCount(t, found, "tolerations-miss-the-taints") != 0 {
		t.Fatal("a PreferNoSchedule taint was treated as blocking")
	}
}

func TestOneUntaintedNodeIsEnough(t *testing.T) {
	tainted := node("gpu", map[string]any{hostnameKey: "gpu"}, map[string]any{
		"taints": []any{map[string]any{"key": "gpu", "effect": "NoSchedule"}},
	}, map[string]any{"cpu": "4", "memory": "8Gi"})
	free := plainNode("worker", map[string]any{hostnameKey: "worker"})

	found := report(t, tainted, free, deployment("api", podSpec(container("app", nil))))

	if findingCount(t, found, "tolerations-miss-the-taints") != 0 {
		t.Fatal("one free node did not satisfy the check")
	}
}

func TestTheLargestNodeIsTheOneCompared(t *testing.T) {
	small := node("small", map[string]any{hostnameKey: "small"}, nil, map[string]any{"cpu": "2", "memory": "4Gi"})
	big := node("big", map[string]any{hostnameKey: "big"}, nil, map[string]any{"cpu": "32", "memory": "128Gi"})
	greedy := deployment("api", podSpec(container("app", resources(requests, map[string]any{"cpu": "8"}))))

	if findingCount(t, report(t, small, big, greedy), "request-exceeds-largest-node") != 0 {
		t.Fatal("a request the big node can hold was compared against the small one")
	}
	if findingCount(t, report(t, small, greedy), "request-exceeds-largest-node") != 1 {
		t.Fatal("a request no node can hold was not reported")
	}
}

func TestTheSpreadCheckCountsDomainsNotNodes(t *testing.T) {
	spread := func(count int64) *unstructured.Unstructured {
		return replicas(deployment("api", podSpecWith(map[string]any{
			"topologySpreadConstraints": []any{map[string]any{
				"topologyKey": "zone", "whenUnsatisfiable": doNotSchedule, "maxSkew": int64(1),
			}},
		}, container("app", nil))), count)
	}
	one := plainNode("a1", map[string]any{hostnameKey: "a1", "zone": "a"})
	two := plainNode("a2", map[string]any{hostnameKey: "a2", "zone": "a"})
	three := plainNode("b1", map[string]any{hostnameKey: "b1", "zone": "b"})
	joining := plainNode("joining", map[string]any{hostnameKey: "joining"})

	if findingCount(t, report(t, joining, one, two, spread(2)), "spread-needs-more-domains") != 1 {
		t.Fatal("two nodes in one zone were counted as two domains")
	}
	if findingCount(t, report(t, one, two, three, spread(2)), "spread-needs-more-domains") != 0 {
		t.Fatal("two zones were not enough for two replicas")
	}
}

func TestMalformedSpreadConstraintsDoNotHideALaterValidOne(t *testing.T) {
	workload := replicas(deployment("api", podSpecWith(map[string]any{
		"topologySpreadConstraints": []any{
			"not-an-object",
			map[string]any{"whenUnsatisfiable": doNotSchedule},
			map[string]any{
				"topologyKey": "zone", "whenUnsatisfiable": doNotSchedule, "maxSkew": int64(1),
			},
		},
	}, container("app", nil))), 2)
	one := plainNode("a1", map[string]any{hostnameKey: "a1", "zone": "a"})
	two := plainNode("a2", map[string]any{hostnameKey: "a2", "zone": "a"})

	if findingCount(t, report(t, one, two, workload), "spread-needs-more-domains") != 1 {
		t.Fatal("a malformed constraint hid a later valid one")
	}
}

func TestTheQuotaCheckNamesTheFullestEntry(t *testing.T) {
	quota := resourceQuota(testNamespace,
		map[string]any{"requests.cpu": "10", "requests.memory": "10Gi"},
		map[string]any{"requests.cpu": "1", "requests.memory": "10Gi"})

	found := report(t, quota, deployment("api", podSpec(container("app", nil))))

	if detail := onlyFinding(t, found, "quota-nearly-exhausted").Detail; !strings.Contains(detail, "requests.memory") {
		t.Fatalf("detail was %q, want the fullest entry named", detail)
	}
}

func TestAQuotaWithNoStatusIsSkipped(t *testing.T) {
	bare := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ResourceQuota",
		"metadata":   map[string]any{"name": "team", "namespace": testNamespace},
	}}

	found := report(t, bare, deployment("api", podSpec(container("app", nil))))

	if findingCount(t, found, "quota-nearly-exhausted") != 0 {
		t.Fatal("a quota the apiserver had not filled in yet was reported")
	}
}

func TestPodSecurityOnlyJudgesTheLevelTheNamespaceEnforces(t *testing.T) {
	escalating := deployment("api", podSpec(container("app", nil)))

	baseline := report(t, namespaceObj(testNamespace, map[string]any{enforceLabel: profileBaseline}), escalating)
	if findingCount(t, baseline, "pod-security-would-reject") != 0 {
		t.Fatal("baseline rejected a pod that only breaks restricted")
	}

	strict := report(t, namespaceObj(testNamespace, map[string]any{enforceLabel: profileStrict}), escalating)
	if detail := onlyFinding(t, strict, "pod-security-would-reject").Detail; !strings.Contains(detail, "privilege escalation") {
		t.Fatalf("detail was %q, want the restricted control named", detail)
	}
}

func TestANamespaceEnforcingNothingIsNotJudged(t *testing.T) {
	found := report(t,
		namespaceObj(testNamespace, map[string]any{enforceLabel: "privileged"}),
		deployment("api", podSpecWith(map[string]any{"hostPID": true}, container("app", nil))))

	if findingCount(t, found, "pod-security-would-reject") != 0 {
		t.Fatal("a namespace at the privileged level rejected something")
	}
}

func TestANamespaceObjectIsNotAuditedAsAWorkload(t *testing.T) {
	found := report(t, namespaceObj(testNamespace, nil), plainNode("worker", nil))

	if found.Scanned != 0 {
		t.Fatalf("scanned %d subjects, want 0 from a cluster holding only context objects", found.Scanned)
	}
}

func TestClusterObjectsOfTheWrongShapeAreSkipped(t *testing.T) {
	odd := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata":   map[string]any{"name": "worker"},
		"spec":       map[string]any{"taints": []any{"not-an-object"}},
		"status":     map[string]any{"allocatable": map[string]any{"cpu": true}},
	}}
	workload := deployment("api", podSpecWith(map[string]any{
		"nodeSelector":              map[string]any{"disk": int64(7)},
		"tolerations":               []any{"not-an-object"},
		"topologySpreadConstraints": []any{"not-an-object"},
	}, container("app", resources(requests, map[string]any{"cpu": "1"}))))

	found := report(t, odd, workload)

	for _, id := range []string{
		"node-selector-matches-nothing",
		"tolerations-miss-the-taints",
		"request-exceeds-largest-node",
		"spread-needs-more-domains",
	} {
		if findingCount(t, found, id) != 0 {
			t.Fatalf("%s reported something from a field of the wrong shape", id)
		}
	}
}

func TestATolerationWhoseEffectDiffersDoesNotCover(t *testing.T) {
	tainted := node("worker", map[string]any{hostnameKey: "worker"}, map[string]any{
		"taints": []any{map[string]any{"key": "gpu", "effect": "NoExecute"}},
	}, map[string]any{"cpu": "4", "memory": "8Gi"})
	wrongEffect := deployment("api", podSpecWith(map[string]any{
		"tolerations": []any{map[string]any{"key": "gpu", "operator": "Exists", "effect": "NoSchedule"}},
	}, container("app", nil)))

	if findingCount(t, report(t, tainted, wrongEffect), "tolerations-miss-the-taints") != 1 {
		t.Fatal("a toleration for a different effect was accepted")
	}
}

func TestANodeWithNoAllocatableIsNotTheLargest(t *testing.T) {
	bare := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "Node",
		"metadata": map[string]any{"name": "joining"},
	}}
	small := node("small", nil, nil, map[string]any{"cpu": "2", "memory": "4Gi"})
	greedy := deployment("api", podSpec(container("app", resources(requests, map[string]any{"cpu": "8"}))))

	if findingCount(t, report(t, bare, small, greedy), "request-exceeds-largest-node") != 1 {
		t.Fatal("a node that reports no allocatable hid a request nothing can hold")
	}
}

func TestALimitRangeForSomethingOtherThanAContainerIsIgnored(t *testing.T) {
	podRange := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "LimitRange",
		"metadata": map[string]any{"name": "limits", "namespace": testNamespace},
		"spec": map[string]any{"limits": []any{
			map[string]any{"type": "Pod", "min": map[string]any{"memory": "128Mi"}},
		}},
	}}
	small := deployment("api", podSpec(container("app", resources(requests, map[string]any{"memory": "16Mi"}))))

	if findingCount(t, report(t, podRange, small), "outside-limit-range") != 0 {
		t.Fatal("a Pod-scoped LimitRange was applied to a container")
	}
}

func TestARequestAboveTheLimitRangeMaximumIsReported(t *testing.T) {
	capped := limitRange(testNamespace, map[string]any{"max": map[string]any{"cpu": "1"}})
	greedy := deployment("api", podSpec(container("app", resources(requests, map[string]any{"cpu": "4"}))))

	if detail := onlyFinding(t, report(t, capped, greedy), "outside-limit-range").Detail; !strings.Contains(detail, "maximum") {
		t.Fatalf("detail was %q, want the maximum named", detail)
	}
}
