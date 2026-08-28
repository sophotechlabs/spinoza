package checks

import (
	"maps"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const testNamespace = "apps"

func metadata(name string) map[string]any {
	return map[string]any{"name": name, "namespace": testNamespace}
}

func container(name string, fields map[string]any) map[string]any {
	out := map[string]any{"name": name, "image": "registry.example/app:1.4.2"}
	maps.Copy(out, fields)
	return out
}

func podSpec(containers ...map[string]any) map[string]any {
	listed := make([]any, 0, len(containers))
	for _, entry := range containers {
		listed = append(listed, entry)
	}
	return map[string]any{"containers": listed}
}

func pod(name string, spec map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   metadata(name),
		"spec":       spec,
	}}
}

func workload(kind, name string, spec map[string]any) *unstructured.Unstructured {
	version := "apps/v1"
	if kindGroups[kind] == "batch" {
		version = "batch/v1"
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": version,
		"kind":       kind,
		"metadata":   metadata(name),
		"spec": map[string]any{
			"selector": map[string]any{"matchLabels": map[string]any{"app": name}},
			"template": map[string]any{"spec": spec},
		},
	}}
}

func deployment(name string, spec map[string]any) *unstructured.Unstructured {
	return workload("Deployment", name, spec)
}

func cronJob(name string, spec map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "CronJob",
		"metadata":   metadata(name),
		"spec": map[string]any{
			"jobTemplate": map[string]any{
				"spec": map[string]any{"template": map[string]any{"spec": spec}},
			},
		},
	}}
}

func specOf(obj *unstructured.Unstructured) map[string]any {
	spec, ok := obj.Object["spec"].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return spec
}

func replicas(obj *unstructured.Unstructured, count int64) *unstructured.Unstructured {
	specOf(obj)["replicas"] = count
	return obj
}

func ownedBy(obj *unstructured.Unstructured, kind, name string) *unstructured.Unstructured {
	obj.SetOwnerReferences([]metav1.OwnerReference{{Kind: kind, Name: name, APIVersion: "apps/v1"}})
	return obj
}

func onNode(obj *unstructured.Unstructured, node string) *unstructured.Unstructured {
	specOf(obj)["nodeName"] = node
	return obj
}

func withSecurity(fields map[string]any) map[string]any {
	return map[string]any{"securityContext": fields}
}

func resources(section string, values map[string]any) map[string]any {
	return map[string]any{"resources": map[string]any{section: values}}
}

func requestsAndLimits(request, limit map[string]any) map[string]any {
	return map[string]any{"resources": map[string]any{"requests": request, "limits": limit}}
}
