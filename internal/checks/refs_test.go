package checks

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func simple(kind, name, namespace string, spec map[string]any) *unstructured.Unstructured {
	version := "v1"
	if group := kindGroups[kind]; group != "" {
		version = group + "/v1"
	}
	meta := map[string]any{"name": name}
	if namespace != "" {
		meta["namespace"] = namespace
	}
	obj := map[string]any{"apiVersion": version, "kind": kind, "metadata": meta}
	if spec != nil {
		obj[specField] = spec
	}
	return &unstructured.Unstructured{Object: obj}
}

func configMap(name string, data map[string]any) *unstructured.Unstructured {
	obj := simple("ConfigMap", name, testNamespace, nil)
	obj.Object["data"] = data
	return obj
}

func service(name string, selector map[string]any) *unstructured.Unstructured {
	return simple("Service", name, testNamespace, map[string]any{"selector": selector})
}

func budget(name string, selector map[string]any, floor int64) *unstructured.Unstructured {
	spec := map[string]any{"selector": map[string]any{"matchLabels": selector}}
	if floor > 0 {
		spec["minAvailable"] = floor
	}
	return simple("PodDisruptionBudget", name, testNamespace, spec)
}

func autoscaler(name, kind, workload string) *unstructured.Unstructured {
	return simple("HorizontalPodAutoscaler", name, testNamespace, map[string]any{
		"scaleTargetRef": map[string]any{"kind": kind, "name": workload},
	})
}

func labelledDeployment(name string, pod map[string]any) *unstructured.Unstructured {
	return labelledWorkload("Deployment", name, pod)
}

func TestEveryReferenceCheckFiresOnItsOwnFaultAndOnNothingElse(t *testing.T) {
	pod := func(extra map[string]any) map[string]any {
		return podSpecWith(extra, sourcedContainer(nil))
	}
	cases := []struct {
		id      string
		objects []*unstructured.Unstructured
		clean   []*unstructured.Unstructured
	}{
		{
			id:      "service-account-missing",
			objects: []*unstructured.Unstructured{labelledDeployment("api", pod(map[string]any{"serviceAccountName": "api"}))},
			clean: []*unstructured.Unstructured{
				simple("ServiceAccount", "api", testNamespace, nil),
				labelledDeployment("api", pod(map[string]any{"serviceAccountName": "api"})),
			},
		},
		{
			id: "config-map-missing",
			objects: []*unstructured.Unstructured{labelledDeployment("api", podSpec(sourcedContainer(map[string]any{
				"envFrom": []any{map[string]any{"configMapRef": map[string]any{"name": "settings"}}},
			})))},
			clean: []*unstructured.Unstructured{
				configMap("settings", map[string]any{"MODE": "live"}),
				labelledDeployment("api", podSpec(sourcedContainer(map[string]any{
					"envFrom": []any{map[string]any{"configMapRef": map[string]any{"name": "settings"}}},
				}))),
			},
		},
		{
			id: "config-map-key-missing",
			objects: []*unstructured.Unstructured{
				configMap("settings", map[string]any{"MODE": "live"}),
				labelledDeployment("api", podSpec(sourcedContainer(map[string]any{
					"env": []any{map[string]any{"name": "PORT", "valueFrom": map[string]any{
						"configMapKeyRef": map[string]any{"name": "settings", "key": "PORT"},
					}}},
				}))),
			},
			clean: []*unstructured.Unstructured{
				configMap("settings", map[string]any{"MODE": "live", "PORT": "8080"}),
				labelledDeployment("api", podSpec(sourcedContainer(map[string]any{
					"env": []any{map[string]any{"name": "PORT", "valueFrom": map[string]any{
						"configMapKeyRef": map[string]any{"name": "settings", "key": "PORT"},
					}}},
				}))),
			},
		},
		{
			id: "secret-missing",
			objects: []*unstructured.Unstructured{labelledDeployment("api", podSpec(sourcedContainer(map[string]any{
				"envFrom": []any{map[string]any{"secretRef": map[string]any{"name": "creds"}}},
			})))},
			clean: []*unstructured.Unstructured{
				simple("Secret", "creds", testNamespace, nil),
				labelledDeployment("api", podSpec(sourcedContainer(map[string]any{
					"envFrom": []any{map[string]any{"secretRef": map[string]any{"name": "creds"}}},
				}))),
			},
		},
		{
			id: "pull-secret-missing",
			objects: []*unstructured.Unstructured{labelledDeployment("api", pod(map[string]any{
				"imagePullSecrets": []any{map[string]any{"name": "registry"}},
			}))},
			clean: []*unstructured.Unstructured{
				simple("Secret", "registry", testNamespace, nil),
				labelledDeployment("api", pod(map[string]any{
					"imagePullSecrets": []any{map[string]any{"name": "registry"}},
				})),
			},
		},
		{
			id: "claim-missing",
			objects: []*unstructured.Unstructured{labelledDeployment("api", pod(map[string]any{
				"volumes": []any{map[string]any{"name": "data", "persistentVolumeClaim": map[string]any{"claimName": "data"}}},
			}))},
			clean: []*unstructured.Unstructured{
				simple("PersistentVolumeClaim", "data", testNamespace, map[string]any{}),
				labelledDeployment("api", pod(map[string]any{
					"volumes": []any{map[string]any{"name": "data", "persistentVolumeClaim": map[string]any{"claimName": "data"}}},
				})),
			},
		},
		{
			id:      "priority-class-missing",
			objects: []*unstructured.Unstructured{labelledDeployment("api", pod(map[string]any{"priorityClassName": "urgent"}))},
			clean: []*unstructured.Unstructured{
				simple("PriorityClass", "urgent", "", nil),
				labelledDeployment("api", pod(map[string]any{"priorityClassName": "urgent"})),
			},
		},
		{
			id:      "runtime-class-missing",
			objects: []*unstructured.Unstructured{labelledDeployment("api", pod(map[string]any{"runtimeClassName": "gvisor"}))},
			clean: []*unstructured.Unstructured{
				simple("RuntimeClass", "gvisor", "", nil),
				labelledDeployment("api", pod(map[string]any{"runtimeClassName": "gvisor"})),
			},
		},
		{
			id: "no-service-selects-it",
			objects: []*unstructured.Unstructured{
				service("other", map[string]any{"app": "web"}),
				labelledDeployment("api", podSpec(sourcedContainer(nil))),
			},
			clean: []*unstructured.Unstructured{
				service("api", map[string]any{"app": "api"}),
				labelledDeployment("api", podSpec(sourcedContainer(nil))),
			},
		},
		{
			id: "no-network-policy",
			objects: []*unstructured.Unstructured{
				simple("NetworkPolicy", "web", testNamespace, map[string]any{
					"podSelector": map[string]any{"matchLabels": map[string]any{"app": "web"}},
				}),
				labelledDeployment("api", podSpec(sourcedContainer(nil))),
			},
			clean: []*unstructured.Unstructured{
				simple("NetworkPolicy", "api", testNamespace, map[string]any{
					"podSelector": map[string]any{"matchLabels": map[string]any{"app": "api"}},
				}),
				labelledDeployment("api", podSpec(sourcedContainer(nil))),
			},
		},
		{
			id: "no-disruption-budget",
			objects: []*unstructured.Unstructured{
				budget("other", map[string]any{"app": "web"}, 1),
				replicas(labelledDeployment("api", podSpec(sourcedContainer(nil))), 3),
			},
			clean: []*unstructured.Unstructured{
				budget("api", map[string]any{"app": "api"}, 1),
				replicas(labelledDeployment("api", podSpec(sourcedContainer(nil))), 3),
			},
		},
		{
			id: "budget-blocks-every-eviction",
			objects: []*unstructured.Unstructured{
				budget("api", map[string]any{"app": "api"}, 3),
				replicas(labelledDeployment("api", podSpec(sourcedContainer(nil))), 3),
			},
			clean: []*unstructured.Unstructured{
				budget("api", map[string]any{"app": "api"}, 2),
				replicas(labelledDeployment("api", podSpec(sourcedContainer(nil))), 3),
			},
		},
		{
			id: "scaler-fights-fixed-replicas",
			objects: []*unstructured.Unstructured{
				autoscaler("api", "Deployment", "api"),
				replicas(labelledDeployment("api", podSpec(sourcedContainer(nil))), 3),
			},
			clean: []*unstructured.Unstructured{
				autoscaler("api", "Deployment", "other"),
				replicas(labelledDeployment("api", podSpec(sourcedContainer(nil))), 3),
			},
		},
		{
			id: "ingress-backend-missing",
			objects: []*unstructured.Unstructured{
				simple("Ingress", "api", testNamespace, map[string]any{
					"defaultBackend": map[string]any{"service": map[string]any{"name": "api"}},
				}),
			},
			clean: []*unstructured.Unstructured{
				service("api", map[string]any{"app": "api"}),
				simple("Ingress", "api", testNamespace, map[string]any{
					"defaultBackend": map[string]any{"service": map[string]any{"name": "api"}},
				}),
			},
		},
		{
			id: "ingress-class-missing",
			objects: []*unstructured.Unstructured{
				simple("Ingress", "api", testNamespace, map[string]any{"ingressClassName": "traefik"}),
			},
			clean: []*unstructured.Unstructured{
				simple("IngressClass", "traefik", "", nil),
				simple("Ingress", "api", testNamespace, map[string]any{"ingressClassName": "traefik"}),
			},
		},
		{
			id: "storage-class-missing",
			objects: []*unstructured.Unstructured{
				simple("PersistentVolumeClaim", "data", testNamespace, map[string]any{"storageClassName": "fast"}),
			},
			clean: []*unstructured.Unstructured{
				simple("StorageClass", "fast", "", nil),
				simple("PersistentVolumeClaim", "data", testNamespace, map[string]any{"storageClassName": "fast"}),
			},
		},
		{
			id:      "claim-nothing-mounts",
			objects: []*unstructured.Unstructured{simple("PersistentVolumeClaim", "spare", testNamespace, map[string]any{})},
			clean: []*unstructured.Unstructured{
				simple("PersistentVolumeClaim", "data", testNamespace, map[string]any{}),
				labelledDeployment("api", pod(map[string]any{
					"volumes": []any{map[string]any{"name": "data", "persistentVolumeClaim": map[string]any{"claimName": "data"}}},
				})),
			},
		},
	}

	registered := map[string]bool{}
	for _, entry := range referenceChecks() {
		if entry.needsEvery {
			continue
		}
		registered[entry.id] = true
	}
	if len(cases) != len(registered) {
		t.Fatalf("%d cases cover %d registered reference checks", len(cases), len(registered))
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			if !registered[tc.id] {
				t.Fatalf("%s is not a registered reference check", tc.id)
			}
			if findingCount(t, report(t, tc.objects...), tc.id) == 0 {
				t.Fatalf("%s did not fire on the cluster written to trip it", tc.id)
			}
			if findingCount(t, report(t, tc.clean...), tc.id) != 0 {
				t.Fatalf("%s fired on a cluster that satisfies it", tc.id)
			}
		})
	}
}

func TestAnOptionalReferenceIsNotAMissingOne(t *testing.T) {
	found := report(t, labelledDeployment("api", podSpec(sourcedContainer(map[string]any{
		"envFrom": []any{map[string]any{
			"configMapRef": map[string]any{"name": "settings", "optional": true},
		}},
	}))))

	if findingCount(t, found, "config-map-missing") != 0 {
		t.Fatal("a reference marked optional was reported as missing")
	}
}

func TestAReferenceIsResolvedInsideItsOwnNamespace(t *testing.T) {
	elsewhere := configMap("settings", map[string]any{"a": "b"})
	elsewhere.SetNamespace("other")

	found := report(t, elsewhere, labelledDeployment("api", podSpec(sourcedContainer(map[string]any{
		"envFrom": []any{map[string]any{"configMapRef": map[string]any{"name": "settings"}}},
	}))))

	if findingCount(t, found, "config-map-missing") != 1 {
		t.Fatal("a ConfigMap in another namespace satisfied the reference")
	}
}

func TestTheDefaultServiceAccountIsNotReportedAsMissing(t *testing.T) {
	found := report(t, labelledDeployment("api", podSpecWith(map[string]any{
		"serviceAccountName": defaultNamespace,
	}, sourcedContainer(nil))))

	if findingCount(t, found, "service-account-missing") != 0 {
		t.Fatal("the default account was reported as missing")
	}
}

func TestASystemPriorityClassIsTakenOnTrust(t *testing.T) {
	found := report(t, labelledDeployment("api", podSpecWith(map[string]any{
		"priorityClassName": "system-cluster-critical",
	}, sourcedContainer(nil))))

	if findingCount(t, found, "priority-class-missing") != 0 {
		t.Fatal("a built-in system priority class was reported as missing")
	}
}

func TestAPolicySelectingEverythingCoversEveryWorkload(t *testing.T) {
	blanket := simple("NetworkPolicy", "default-deny", testNamespace, map[string]any{
		"podSelector": map[string]any{},
	})

	found := report(t, blanket, labelledDeployment("api", podSpec(sourcedContainer(nil))))

	if findingCount(t, found, "no-network-policy") != 0 {
		t.Fatal("an empty podSelector did not count as covering everything")
	}
}

func TestABudgetIsOnlyJudgedAgainstTheWorkloadItSelects(t *testing.T) {
	found := report(t,
		budget("web", map[string]any{"app": "web"}, 9),
		replicas(labelledDeployment("api", podSpec(sourcedContainer(nil))), 3))

	if findingCount(t, found, "budget-blocks-every-eviction") != 0 {
		t.Fatal("a budget for another workload was judged against this one")
	}
}

func TestAnUnmountedClaimCarriesTheObjectItNames(t *testing.T) {
	found := report(t, simple("PersistentVolumeClaim", "left-over", testNamespace, map[string]any{}))

	object := onlyObject(t, found, "claim-nothing-mounts")
	if object.Name != "left-over" || object.Kind != "PersistentVolumeClaim" {
		t.Fatalf("the finding named %s/%s, want the claim itself", object.Kind, object.Name)
	}
}

func TestAReferenceCheckIsSkippedWhenItsKindWasNeverDiscovered(t *testing.T) {
	descs := descriptors()
	delete(descs, "/v1/configmaps")

	found := Run(t.Context(), newLister(), descs, api.Metrics{}, wholeCluster(), 0)

	group := groupNamed(t, found, "config-map-missing")
	if !strings.Contains(group.Skipped, "configmaps") {
		t.Fatalf("skipped said %q, want it to name configmaps", group.Skipped)
	}
}

func TestAnIngressRuleBackendIsResolvedAsWellAsTheDefault(t *testing.T) {
	routed := simple("Ingress", "api", testNamespace, map[string]any{
		"rules": []any{map[string]any{"host": "api.example", "http": map[string]any{
			"paths": []any{map[string]any{
				"path":    "/",
				"backend": map[string]any{"service": map[string]any{"name": "api"}},
			}},
		}}},
	})

	if findingCount(t, report(t, routed), "ingress-backend-missing") != 1 {
		t.Fatal("a backend named on a rule path was not resolved")
	}
	if findingCount(t, report(t, service("api", map[string]any{"app": "api"}), routed), "ingress-backend-missing") != 0 {
		t.Fatal("a rule backend that exists was still reported")
	}
}

func TestAnIngressOfTheWrongShapeIsSkipped(t *testing.T) {
	odd := simple("Ingress", "api", testNamespace, map[string]any{
		"rules": []any{
			"not-an-object",
			map[string]any{"http": "not-an-object"},
			map[string]any{"http": map[string]any{"paths": "not-a-list"}},
			map[string]any{"http": map[string]any{"paths": []any{
				"not-an-object",
				map[string]any{"path": "/"},
				map[string]any{"backend": map[string]any{}},
			}}},
		},
	})

	if findingCount(t, report(t, odd), "ingress-backend-missing") != 0 {
		t.Fatal("an Ingress whose rules are the wrong shape produced a finding")
	}
}

func TestABinaryConfigMapKeyCounts(t *testing.T) {
	binary := simple("ConfigMap", "settings", testNamespace, nil)
	binary.Object["binaryData"] = map[string]any{"cert.p12": "..."}

	found := report(t, binary, labelledDeployment("api", podSpec(sourcedContainer(map[string]any{
		"env": []any{map[string]any{"name": "CERT", "valueFrom": map[string]any{
			"configMapKeyRef": map[string]any{"name": "settings", "key": "cert.p12"},
		}}},
	}))))

	if findingCount(t, found, "config-map-key-missing") != 0 {
		t.Fatal("a key held in binaryData was reported as missing")
	}
}

func TestAWorkloadWithNoTemplateLabelsIsNotJudgedOnSelectors(t *testing.T) {
	bare := workload("Deployment", "api", podSpec(sourcedContainer(nil)))

	found := report(t, service("other", map[string]any{"app": "web"}), bare)

	for _, id := range []string{"no-service-selects-it", "no-network-policy", "no-disruption-budget"} {
		if findingCount(t, found, id) != 0 {
			t.Fatalf("%s judged a workload whose template carries no labels", id)
		}
	}
}

func TestAClaimAStatefulSetGeneratedIsMounted(t *testing.T) {
	set := labelledWorkload("StatefulSet", "redis-broker", podSpec(sourcedContainer(nil)))
	specOf(set)["volumeClaimTemplates"] = []any{
		map[string]any{"metadata": map[string]any{"name": "data"}},
	}
	claim := simple("PersistentVolumeClaim", "data-redis-broker-0", testNamespace, map[string]any{})

	if findingCount(t, report(t, set, claim), "claim-nothing-mounts") != 0 {
		t.Fatal("a claim a StatefulSet generated was reported as unmounted")
	}
}

func TestAClaimThatMerelyLooksGeneratedIsStillReported(t *testing.T) {
	set := labelledWorkload("StatefulSet", "redis-broker", podSpec(sourcedContainer(nil)))
	specOf(set)["volumeClaimTemplates"] = []any{
		map[string]any{"metadata": map[string]any{"name": "data"}},
	}
	claim := simple("PersistentVolumeClaim", "data-redis-broker-backup", testNamespace, map[string]any{})

	if findingCount(t, report(t, set, claim), "claim-nothing-mounts") != 1 {
		t.Fatal("a claim whose suffix is not an ordinal was treated as generated")
	}
}

func TestAClusterWithNoNetworkPolicyAtAllIsLeftAlone(t *testing.T) {
	found := report(t, labelledDeployment("api", podSpec(sourcedContainer(nil))))

	if findingCount(t, found, "no-network-policy") != 0 {
		t.Fatal("a cluster that uses no NetworkPolicy at all was told off for every workload")
	}
}

func TestHelmsOwnReleaseStorageIsNotAnOrphan(t *testing.T) {
	found := reportEverything(t, simple("Secret", "sh.helm.release.v1.beyla.v3", testNamespace, nil))

	if findingCount(t, found, "orphaned-secret") != 0 {
		t.Fatal("a Helm release record was reported as an orphaned Secret")
	}
}

func TestTheCaBundleEveryNamespaceGetsIsNotAnOrphan(t *testing.T) {
	found := reportEverything(t, configMap("kube-root-ca.crt", map[string]any{"ca.crt": "..."}))

	if findingCount(t, found, "orphaned-config-map") != 0 {
		t.Fatal("the CA bundle every namespace gets was reported as an orphan")
	}
}

func TestASecretAnythingAtAllNamesIsNotAnOrphan(t *testing.T) {
	namer := simple("Service", "gateway", testNamespace, map[string]any{
		"selector":        map[string]any{"app": "api"},
		"externalName":    "tls-cert",
		"sessionAffinity": "None",
	})

	found := reportEverything(t, simple("Secret", "tls-cert", testNamespace, nil), namer)

	if findingCount(t, found, "orphaned-secret") != 0 {
		t.Fatal("a Secret named by another object was still reported as an orphan")
	}
}

func TestAConfigMapNothingNamesIsAnOrphanOnceEveryKindIsRead(t *testing.T) {
	alone := reportEverything(t, configMap("nobody-names-me", map[string]any{"a": "b"}))
	if findingCount(t, alone, "orphaned-config-map") != 1 {
		t.Fatal("a ConfigMap nothing names was not reported")
	}

	named := reportEverything(t,
		configMap("settings", map[string]any{"a": "b"}),
		labelledDeployment("api", podSpec(sourcedContainer(map[string]any{
			"envFrom": []any{map[string]any{"configMapRef": map[string]any{"name": "settings"}}},
		}))))
	if findingCount(t, named, "orphaned-config-map") != 0 {
		t.Fatal("a ConfigMap a workload reads was reported as an orphan")
	}
}

func TestASecretNothingNamesIsAnOrphanOnceEveryKindIsRead(t *testing.T) {
	alone := reportEverything(t, simple("Secret", "nobody-names-me", testNamespace, nil))
	if findingCount(t, alone, "orphaned-secret") != 1 {
		t.Fatal("a Secret nothing names was not reported")
	}

	named := reportEverything(t,
		simple("Secret", "creds", testNamespace, nil),
		labelledDeployment("api", podSpec(sourcedContainer(map[string]any{
			"envFrom": []any{map[string]any{"secretRef": map[string]any{"name": "creds"}}},
		}))))
	if findingCount(t, named, "orphaned-secret") != 0 {
		t.Fatal("a Secret a workload reads was reported as an orphan")
	}
}
