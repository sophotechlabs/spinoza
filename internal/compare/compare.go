package compare

import (
	"fmt"
	"slices"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const lastApplied = "kubectl.kubernetes.io/last-applied-configuration"

const headless = "None"

const specField = "spec"

const (
	serviceKind    = "Service"
	claimKind      = "PersistentVolumeClaim"
	volumeKind     = "PersistentVolume"
	crdKind        = "CustomResourceDefinition"
	validatingKind = "ValidatingWebhookConfiguration"
	mutatingKind   = "MutatingWebhookConfiguration"
)

var allocatedByTheServer = map[string][][]string{
	claimKind:  {{specField, "volumeName"}},
	volumeKind: {{specField, "claimRef", "uid"}, {specField, "claimRef", "resourceVersion"}},
	serviceKind: {
		{specField, "healthCheckNodePort"},
	},
	crdKind: {{specField, "conversion", "webhook", "clientConfig", "caBundle"}},
}

var assignedByTheServer = []string{
	"uid",
	"resourceVersion",
	"generation",
	"creationTimestamp",
	"managedFields",
	"selfLink",
}

func Normalise(item *unstructured.Unstructured) *unstructured.Unstructured {
	clean := item.DeepCopy()
	unstructured.RemoveNestedField(clean.Object, "status")
	for _, field := range assignedByTheServer {
		unstructured.RemoveNestedField(clean.Object, "metadata", field)
	}
	dropLastApplied(clean)
	dropOwnerUIDs(clean)
	dropAllocations(clean)
	return clean
}

func dropAllocations(clean *unstructured.Unstructured) {
	kind := clean.GetKind()
	for _, path := range allocatedByTheServer[kind] {
		unstructured.RemoveNestedField(clean.Object, path...)
	}
	dropClusterIPs(clean, kind)
	dropNodePorts(clean, kind)
	dropCABundles(clean, kind)
}

func dropClusterIPs(clean *unstructured.Unstructured, kind string) {
	if kind != serviceKind {
		return
	}
	one, found, err := unstructured.NestedString(clean.Object, specField, "clusterIP")
	if found && err == nil && one != headless {
		unstructured.RemoveNestedField(clean.Object, specField, "clusterIP")
	}
	many, found, err := unstructured.NestedStringSlice(clean.Object, specField, "clusterIPs")
	if !found || err != nil {
		return
	}
	if slices.Contains(many, headless) {
		return
	}
	unstructured.RemoveNestedField(clean.Object, specField, "clusterIPs")
}

func dropNodePorts(clean *unstructured.Unstructured, kind string) {
	if kind != serviceKind {
		return
	}
	ports, found, err := unstructured.NestedSlice(clean.Object, specField, "ports")
	if !found || err != nil {
		return
	}
	for _, entry := range ports {
		port, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		delete(port, "nodePort")
	}
	setErr := unstructured.SetNestedSlice(clean.Object, ports, specField, "ports")
	if setErr != nil {
		unstructured.RemoveNestedField(clean.Object, specField, "ports")
	}
}

func dropCABundles(clean *unstructured.Unstructured, kind string) {
	if kind != validatingKind && kind != mutatingKind {
		return
	}
	hooks, found, err := unstructured.NestedSlice(clean.Object, "webhooks")
	if !found || err != nil {
		return
	}
	for _, entry := range hooks {
		hook, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		config, ok := hook["clientConfig"].(map[string]any)
		if !ok {
			continue
		}
		delete(config, "caBundle")
	}
	setErr := unstructured.SetNestedSlice(clean.Object, hooks, "webhooks")
	if setErr != nil {
		unstructured.RemoveNestedField(clean.Object, "webhooks")
	}
}

func dropLastApplied(clean *unstructured.Unstructured) {
	annotations := clean.GetAnnotations()
	if annotations == nil {
		return
	}
	_, carried := annotations[lastApplied]
	if !carried {
		return
	}
	delete(annotations, lastApplied)
	if len(annotations) == 0 {
		clean.SetAnnotations(nil)
		return
	}
	clean.SetAnnotations(annotations)
}

func dropOwnerUIDs(clean *unstructured.Unstructured) {
	owners, found, err := unstructured.NestedSlice(clean.Object, "metadata", "ownerReferences")
	if !found || err != nil {
		return
	}
	for _, entry := range owners {
		owner, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		delete(owner, "uid")
	}
	setErr := unstructured.SetNestedSlice(clean.Object, owners, "metadata", "ownerReferences")
	if setErr != nil {
		unstructured.RemoveNestedField(clean.Object, "metadata", "ownerReferences")
	}
}

func Parse(raw string) (*unstructured.Unstructured, error) {
	object := map[string]any{}
	err := yaml.Unmarshal([]byte(raw), &object)
	if err != nil {
		return nil, fmt.Errorf("%w: that object could not be read as yaml: %w", api.ErrInternal, err)
	}
	return &unstructured.Unstructured{Object: object}, nil
}

func YAML(item *unstructured.Unstructured) (string, error) {
	raw, err := yaml.Marshal(item.Object)
	if err != nil {
		return "", fmt.Errorf("%w: that object could not be written as yaml: %w", api.ErrInternal, err)
	}
	return string(raw), nil
}

func Rendered(raw string, keep bool) (string, error) {
	if keep {
		return raw, nil
	}
	parsed, err := Parse(raw)
	if err != nil {
		return "", err
	}
	return YAML(Normalise(parsed))
}
