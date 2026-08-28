package topology

import (
	"errors"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
)

var (
	podGVR         = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	serviceGVR     = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}
	deploymentGVR  = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	replicaSetGVR  = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}
	statefulSetGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
	daemonSetGVR   = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}
	jobGVR         = schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}
	cronJobGVR     = schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}
	ingressGVR     = schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"}
	hpaGVR         = schema.GroupVersionResource{Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"}
	rcGVR          = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "replicationcontrollers"}
)

func listKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		podGVR:         "PodList",
		serviceGVR:     "ServiceList",
		deploymentGVR:  "DeploymentList",
		replicaSetGVR:  "ReplicaSetList",
		statefulSetGVR: "StatefulSetList",
		daemonSetGVR:   "DaemonSetList",
		jobGVR:         "JobList",
		cronJobGVR:     "CronJobList",
		ingressGVR:     "IngressList",
		hpaGVR:         "HorizontalPodAutoscalerList",
		rcGVR:          "ReplicationControllerList",
	}
}

func desc(group, version, resource, kind string) api.ResourceDescriptor {
	return api.ResourceDescriptor{
		Group:      group,
		Version:    version,
		Resource:   resource,
		Kind:       kind,
		Namespaced: true,
		Category:   "Workloads",
	}
}

func clusterDesc(group, version, resource, kind string) api.ResourceDescriptor {
	out := desc(group, version, resource, kind)
	out.Namespaced = false
	return out
}

func descs() map[string]api.ResourceDescriptor {
	return map[string]api.ResourceDescriptor{
		discovery.Key("", "v1", "pods"):                                desc("", "v1", "pods", "Pod"),
		discovery.Key("", "v1", "services"):                            desc("", "v1", "services", "Service"),
		discovery.Key("apps", "v1", "deployments"):                     desc("apps", "v1", "deployments", "Deployment"),
		discovery.Key("apps", "v1", "replicasets"):                     desc("apps", "v1", "replicasets", "ReplicaSet"),
		discovery.Key("apps", "v1", "statefulsets"):                    desc("apps", "v1", "statefulsets", "StatefulSet"),
		discovery.Key("apps", "v1", "daemonsets"):                      desc("apps", "v1", "daemonsets", "DaemonSet"),
		discovery.Key("batch", "v1", "jobs"):                           desc("batch", "v1", "jobs", "Job"),
		discovery.Key("batch", "v1", "cronjobs"):                       desc("batch", "v1", "cronjobs", "CronJob"),
		discovery.Key("networking.k8s.io", "v1", "ingresses"):          desc("networking.k8s.io", "v1", "ingresses", "Ingress"),
		discovery.Key("autoscaling", "v2", "horizontalpodautoscalers"): desc("autoscaling", "v2", "horizontalpodautoscalers", "HorizontalPodAutoscaler"),
		discovery.Key("argoproj.io", "v1alpha1", "rollouts"):           desc("argoproj.io", "v1alpha1", "rollouts", "Rollout"),
		discovery.Key("", "v1", "configmaps"):                          desc("", "v1", "configmaps", "ConfigMap"),
		discovery.Key("", "v1", "replicationcontrollers"):              desc("", "v1", "replicationcontrollers", "ReplicationController"),
		discovery.Key("example.com", "v1", "fleets"):                   clusterDesc("example.com", "v1", "fleets", "Fleet"),
	}
}

func meta(name, namespace, uid string) map[string]any {
	return map[string]any{"name": name, "namespace": namespace, "uid": uid}
}

func ownedBy(name, namespace, uid, ownerKind, ownerName, ownerUID, ownerAPI string) map[string]any {
	out := meta(name, namespace, uid)
	out["ownerReferences"] = []any{map[string]any{
		"apiVersion": ownerAPI,
		"kind":       ownerKind,
		"name":       ownerName,
		"uid":        ownerUID,
		"controller": true,
	}}
	return out
}

func deployment() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   meta("api", "prod", "dep-api"),
		"spec": map[string]any{
			"replicas": int64(2),
			"template": map[string]any{"spec": map[string]any{
				"volumes": []any{
					map[string]any{"name": "tls", "secret": map[string]any{"secretName": "api-tls"}},
					map[string]any{"name": "nothing"},
					"not-a-map",
				},
				"imagePullSecrets": []any{map[string]any{"name": "registry"}},
				"containers": []any{map[string]any{
					"name": "api",
					"envFrom": []any{
						map[string]any{"configMapRef": map[string]any{"name": "api-config"}},
						"not-a-map",
					},
					"env": []any{map[string]any{
						"name":      "TOKEN",
						"valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": "api-tls"}},
					}},
				}},
			}},
		},
		"status": map[string]any{"readyReplicas": int64(2)},
	}}
}

func replicaSet() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "ReplicaSet",
		"metadata":   ownedBy("api-abc", "prod", "rs-api", "Deployment", "api", "dep-api", "apps/v1"),
		"spec":       map[string]any{"replicas": int64(2)},
		"status":     map[string]any{"readyReplicas": int64(2)},
	}}
}

type owner struct {
	kind       string
	name       string
	uid        string
	apiVersion string
}

func pod(name, uid, phase, ready string, held owner, app string) *unstructured.Unstructured {
	holder := meta(name, "prod", uid)
	if held.uid != "" {
		holder = ownedBy(name, "prod", uid, held.kind, held.name, held.uid, held.apiVersion)
	}
	if app != "" {
		holder["labels"] = map[string]any{"app": app}
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   holder,
		"status": map[string]any{
			"phase":      phase,
			"conditions": []any{map[string]any{"type": "Ready", "status": ready}},
		},
	}}
}

func service() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   meta("api", "prod", "svc-api"),
		"spec":       map[string]any{"selector": map[string]any{"app": "api"}},
	}}
}

func headlessService() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   meta("external", "prod", "svc-external"),
		"spec":       map[string]any{"type": "ExternalName", "selector": map[string]any{"port": int64(80)}},
	}}
}

func ingress() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "networking.k8s.io/v1",
		"kind":       "Ingress",
		"metadata":   meta("web", "prod", "ing-web"),
		"spec": map[string]any{
			"defaultBackend": map[string]any{"service": map[string]any{"name": "gone"}},
			"rules": []any{
				map[string]any{"http": map[string]any{"paths": []any{
					map[string]any{"backend": map[string]any{"service": map[string]any{"name": "api"}}},
					map[string]any{"backend": map[string]any{"resource": map[string]any{"name": "bucket"}}},
					"not-a-map",
				}}},
				"not-a-map",
			},
		},
	}}
}

func autoscaler() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "autoscaling/v2",
		"kind":       "HorizontalPodAutoscaler",
		"metadata":   meta("api", "prod", "hpa-api"),
		"spec": map[string]any{"scaleTargetRef": map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       "api",
		}},
	}}
}

func cronJob() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "CronJob",
		"metadata":   meta("nightly", "prod", "cj-nightly"),
		"spec": map[string]any{"jobTemplate": map[string]any{"spec": map[string]any{
			"template": map[string]any{"spec": map[string]any{
				"containers": []any{map[string]any{"name": "run"}},
			}},
		}}},
	}}
}

func job() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata":   ownedBy("nightly-1", "prod", "job-1", "CronJob", "nightly", "cj-nightly", "batch/v1"),
	}}
}

func otherDeployment() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   meta("web", "other", "dep-web"),
		"spec":       map[string]any{"replicas": int64(1)},
		"status":     map[string]any{"readyReplicas": int64(1)},
	}}
}

func failedJob() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata":   meta("import", "prod", "job-2"),
		"status": map[string]any{"conditions": []any{
			map[string]any{"type": "Complete", "status": "False"},
			map[string]any{"type": "Failed", "status": "True"},
			"not-a-map",
		}},
	}}
}

func daemonSet() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "DaemonSet",
		"metadata":   meta("agent", "prod", "ds-agent"),
		"spec": map[string]any{"template": map[string]any{"spec": map[string]any{
			"volumes": []any{
				map[string]any{
					"name": "bundle",
					"projected": map[string]any{"sources": []any{
						map[string]any{"configMap": map[string]any{"name": "agent-config"}},
						map[string]any{"secret": map[string]any{"name": "agent-token"}},
						"not-a-map",
					}},
				},
				map[string]any{
					"name": "kube-api-access-x9k2p",
					"projected": map[string]any{"sources": []any{
						map[string]any{"serviceAccountToken": map[string]any{"path": "token"}},
						map[string]any{"configMap": map[string]any{"name": "kube-root-ca.crt"}},
						"not-a-map",
					}},
				},
			},
		}}},
		"status": map[string]any{"numberReady": int64(1), "desiredNumberScheduled": int64(3)},
	}}
}

func controller() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ReplicationController",
		"metadata":   meta("legacy", "prod", "rc-legacy"),
		"spec":       map[string]any{"replicas": int64(1)},
		"status":     map[string]any{"readyReplicas": int64(1)},
	}}
}

func adopted() *unstructured.Unstructured {
	holder := meta("adopted", "prod", "pod-9")
	holder["ownerReferences"] = []any{map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "ReplicaSet",
		"name":       "api-abc",
		"uid":        "rs-api",
	}}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   holder,
		"status": map[string]any{
			"phase":      "Running",
			"conditions": []any{map[string]any{"type": "Ready", "status": "True"}},
		},
	}}
}

func plainIngress() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "networking.k8s.io/v1",
		"kind":       "Ingress",
		"metadata":   meta("plain", "prod", "ing-plain"),
		"spec":       map[string]any{"rules": []any{}},
	}}
}

var (
	fleetOwner      = owner{kind: "Fleet", name: "west", uid: "fleet-west", apiVersion: "example.com/v1"}
	controllerOwner = owner{kind: "ReplicationController", name: "legacy", uid: "rc-legacy", apiVersion: "v1"}
	replicaSetOwner = owner{kind: "ReplicaSet", name: "api-abc", uid: "rs-api", apiVersion: "apps/v1"}
	jobOwner        = owner{kind: "Job", name: "nightly-1", uid: "job-1", apiVersion: "batch/v1"}
	rolloutOwner    = owner{kind: "Rollout", name: "canary", uid: "rollout-1", apiVersion: "argoproj.io/v1alpha1"}
)

func newClient() *fake.FakeDynamicClient {
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		listKinds(),
		deployment(),
		replicaSet(),
		pod("api-abc-1", "pod-1", "Running", "True", replicaSetOwner, "api"),
		pod("api-abc-2", "pod-2", "Running", "False", replicaSetOwner, "api"),
		pod("nightly-1-x", "pod-3", "Succeeded", "False", jobOwner, ""),
		pod("debug", "pod-4", "Running", "True", owner{}, ""),
		pod("canary-1", "pod-5", "Running", "True", rolloutOwner, ""),
		service(),
		headlessService(),
		ingress(),
		autoscaler(),
		cronJob(),
		job(),
		failedJob(),
		daemonSet(),
		controller(),
		pod("legacy-1", "pod-6", "Running", "True", controllerOwner, ""),
		pod("knot", "pod-7", "Running", "True", owner{
			kind:       "Pod",
			name:       "knot",
			uid:        "pod-7",
			apiVersion: "v1",
		}, ""),
		adopted(),
		plainIngress(),
		pod("fleet-1", "pod-8", "Running", "True", fleetOwner, ""),
		otherDeployment(),
	)
	dyn.PrependReactor("list", "statefulsets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("list statefulsets failed")
	})
	return dyn
}
