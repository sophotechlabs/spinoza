package compare

import (
	"fmt"
	"slices"
	"strings"

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
	clean, _ := Strip(item)
	return clean
}

func Strip(item *unstructured.Unstructured) (*unstructured.Unstructured, []string) {
	clean := authored(item)
	return clean, dropAllocations(clean)
}

func authored(item *unstructured.Unstructured) *unstructured.Unstructured {
	clean := item.DeepCopy()
	unstructured.RemoveNestedField(clean.Object, "status")
	for _, field := range assignedByTheServer {
		unstructured.RemoveNestedField(clean.Object, "metadata", field)
	}
	dropLastApplied(clean)
	dropOwnerUIDs(clean)
	return clean
}

func dropAllocations(clean *unstructured.Unstructured) []string {
	kind := clean.GetKind()
	stripped := []string{}
	for _, path := range allocatedByTheServer[kind] {
		_, found, _ := unstructured.NestedFieldNoCopy(clean.Object, path...)
		if found {
			stripped = append(stripped, strings.Join(path, "."))
		}
		unstructured.RemoveNestedField(clean.Object, path...)
	}
	if dropClusterIPs(clean, kind) {
		stripped = append(stripped, "spec.clusterIP")
	}
	if dropNodePorts(clean, kind) {
		stripped = append(stripped, "spec.ports[].nodePort")
	}
	if dropCABundles(clean, kind) {
		stripped = append(stripped, "webhooks[].clientConfig.caBundle")
	}
	slices.Sort(stripped)
	return stripped
}

func dropClusterIPs(clean *unstructured.Unstructured, kind string) bool {
	if kind != serviceKind {
		return false
	}
	dropped := false
	one, found, err := unstructured.NestedString(clean.Object, specField, "clusterIP")
	if found && err == nil && one != headless {
		unstructured.RemoveNestedField(clean.Object, specField, "clusterIP")
		dropped = true
	}
	many, found, err := unstructured.NestedStringSlice(clean.Object, specField, "clusterIPs")
	if !found || err != nil {
		return dropped
	}
	if slices.Contains(many, headless) {
		return dropped
	}
	unstructured.RemoveNestedField(clean.Object, specField, "clusterIPs")
	return true
}

func dropNodePorts(clean *unstructured.Unstructured, kind string) bool {
	if kind != serviceKind {
		return false
	}
	ports, found, err := unstructured.NestedSlice(clean.Object, specField, "ports")
	if !found || err != nil {
		return false
	}
	dropped := false
	for _, entry := range ports {
		port, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if _, carried := port["nodePort"]; carried {
			dropped = true
		}
		delete(port, "nodePort")
	}
	setErr := unstructured.SetNestedSlice(clean.Object, ports, specField, "ports")
	if setErr != nil {
		unstructured.RemoveNestedField(clean.Object, specField, "ports")
	}
	return dropped
}

func dropCABundles(clean *unstructured.Unstructured, kind string) bool {
	if kind != validatingKind && kind != mutatingKind {
		return false
	}
	hooks, found, err := unstructured.NestedSlice(clean.Object, "webhooks")
	if !found || err != nil {
		return false
	}
	dropped := false
	for _, entry := range hooks {
		hook, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		config, ok := hook["clientConfig"].(map[string]any)
		if !ok {
			continue
		}
		if _, carried := config["caBundle"]; carried {
			dropped = true
		}
		delete(config, "caBundle")
	}
	setErr := unstructured.SetNestedSlice(clean.Object, hooks, "webhooks")
	if setErr != nil {
		unstructured.RemoveNestedField(clean.Object, "webhooks")
	}
	return dropped
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

type Rendering struct {
	Text     string
	Authored string
	Stripped []string
}

func Render(raw string, keep bool) (Rendering, error) {
	if keep {
		return Rendering{Text: raw, Authored: raw}, nil
	}
	parsed, err := Parse(raw)
	if err != nil {
		return Rendering{}, err
	}
	clean, stripped := Strip(parsed)
	text, textErr := YAML(clean)
	if textErr != nil {
		return Rendering{}, textErr
	}
	wrote, wroteErr := YAML(authored(parsed))
	if wroteErr != nil {
		return Rendering{}, wroteErr
	}
	return Rendering{Text: text, Authored: wrote, Stripped: stripped}, nil
}

func Rendered(raw string, keep bool) (string, error) {
	rendering, err := Render(raw, keep)
	if err != nil {
		return "", err
	}
	return rendering.Text, nil
}

func Union(left, right []string) []string {
	out := slices.Clone(left)
	for _, one := range right {
		if !slices.Contains(out, one) {
			out = append(out, one)
		}
	}
	slices.Sort(out)
	return out
}
