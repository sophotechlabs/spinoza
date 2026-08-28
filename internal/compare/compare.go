package compare

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const lastApplied = "kubectl.kubernetes.io/last-applied-configuration"

// Fields every cluster writes for itself.
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
	return clean
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
