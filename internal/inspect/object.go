package inspect

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const lastAppliedAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

func Get(ctx context.Context, dyn dynamic.Interface, ref api.ObjectRef) (api.ObjectDetail, error) {
	u, err := resourceFor(dyn, ref).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return api.ObjectDetail{}, err
	}
	return detailOf(u)
}

func Apply(ctx context.Context, dyn dynamic.Interface, ref api.ObjectRef, doc []byte) (api.ObjectDetail, error) {
	obj := map[string]any{}
	unmarshalErr := yaml.Unmarshal(doc, &obj)
	if unmarshalErr != nil {
		return api.ObjectDetail{}, fmt.Errorf("parse yaml: %w", unmarshalErr)
	}
	u := &unstructured.Unstructured{Object: obj}
	matchErr := matchesRef(u, ref)
	if matchErr != nil {
		return api.ObjectDetail{}, matchErr
	}
	updated, err := resourceFor(dyn, ref).Update(ctx, u, metav1.UpdateOptions{FieldManager: fieldManager})
	if err != nil {
		return api.ObjectDetail{}, err
	}
	return detailOf(updated)
}

func Delete(ctx context.Context, dyn dynamic.Interface, ref api.ObjectRef) error {
	return resourceFor(dyn, ref).Delete(ctx, ref.Name, metav1.DeleteOptions{})
}

const fieldManager = "spinoza"

func resourceFor(dyn dynamic.Interface, ref api.ObjectRef) dynamic.ResourceInterface {
	gvr := schema.GroupVersionResource{Group: ref.Group, Version: ref.Version, Resource: ref.Resource}
	if ref.Namespace == "" {
		return dyn.Resource(gvr)
	}
	return dyn.Resource(gvr).Namespace(ref.Namespace)
}

func matchesRef(u *unstructured.Unstructured, ref api.ObjectRef) error {
	if u.GetName() != ref.Name {
		return fmt.Errorf("document name %q does not match %q", u.GetName(), ref.Name)
	}
	if u.GetNamespace() != ref.Namespace {
		return fmt.Errorf("document namespace %q does not match %q", u.GetNamespace(), ref.Namespace)
	}
	return nil
}

func detailOf(u *unstructured.Unstructured) (api.ObjectDetail, error) {
	clean := sanitize(u)
	raw, err := yaml.Marshal(clean.Object)
	if err != nil {
		return api.ObjectDetail{}, fmt.Errorf("marshal yaml: %w", err)
	}
	return api.ObjectDetail{
		APIVersion:  clean.GetAPIVersion(),
		Kind:        clean.GetKind(),
		Name:        clean.GetName(),
		Namespace:   clean.GetNamespace(),
		UID:         string(clean.GetUID()),
		CreatedAt:   creationOf(clean),
		Labels:      clean.GetLabels(),
		Annotations: clean.GetAnnotations(),
		Owners:      ownersOf(clean),
		Conditions:  conditionsOf(clean),
		Containers:  containerNames(clean),
		Suspended:   suspendedOf(clean),
		Replicas:    replicasOf(clean),
		Schedulable: schedulableOf(clean),
		HandledAt:   nestedString(clean, "status", "lastHandledReconcileAt"),
		Ports:       portsOf(clean),
		YAML:        string(raw),
	}, nil
}

func sanitize(u *unstructured.Unstructured) *unstructured.Unstructured {
	clean := u.DeepCopy()
	clean.SetManagedFields(nil)
	annotations := clean.GetAnnotations()
	if annotations == nil {
		return clean
	}
	delete(annotations, lastAppliedAnnotation)
	if len(annotations) == 0 {
		clean.SetAnnotations(nil)
		return clean
	}
	clean.SetAnnotations(annotations)
	return clean
}

func creationOf(u *unstructured.Unstructured) string {
	ts := u.GetCreationTimestamp()
	if ts.IsZero() {
		return ""
	}
	return ts.Time.UTC().Format(time.RFC3339)
}

func ownersOf(u *unstructured.Unstructured) []api.OwnerRef {
	refs := u.GetOwnerReferences()
	if len(refs) == 0 {
		return nil
	}
	out := make([]api.OwnerRef, 0, len(refs))
	for _, r := range refs {
		out = append(out, api.OwnerRef{Kind: r.Kind, Name: r.Name, UID: string(r.UID)})
	}
	return out
}

func conditionsOf(u *unstructured.Unstructured) []api.Condition {
	out := []api.Condition{}
	for _, c := range nestedSlice(u, "status", "conditions") {
		entry, ok := c.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, api.Condition{
			Type:    stringField(entry, "type"),
			Status:  stringField(entry, "status"),
			Reason:  stringField(entry, "reason"),
			Message: stringField(entry, "message"),
			Updated: transitionOf(entry),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func transitionOf(m map[string]any) string {
	v := stringField(m, "lastTransitionTime")
	if v != "" {
		return v
	}
	return stringField(m, "lastUpdateTime")
}

func suspendedOf(u *unstructured.Unstructured) *bool {
	value, found, err := unstructured.NestedBool(u.Object, "spec", "suspend")
	if !found || err != nil {
		return nil
	}
	return &value
}

func replicasOf(u *unstructured.Unstructured) *int64 {
	value, found, err := unstructured.NestedInt64(u.Object, "spec", "replicas")
	if !found || err != nil {
		return nil
	}
	return &value
}

func schedulableOf(u *unstructured.Unstructured) *bool {
	if u.GetKind() != "Node" {
		return nil
	}
	unschedulable, _, err := unstructured.NestedBool(u.Object, "spec", "unschedulable")
	if err != nil {
		return nil
	}
	schedulable := !unschedulable
	return &schedulable
}

func portsOf(u *unstructured.Unstructured) []api.ObjectPort {
	if u.GetKind() == "Pod" {
		return podPorts(u)
	}
	if u.GetKind() == "Service" {
		return servicePorts(u)
	}
	return nil
}

func podPorts(u *unstructured.Unstructured) []api.ObjectPort {
	out := []api.ObjectPort{}
	for _, container := range nestedSlice(u, "spec", "containers") {
		m, ok := container.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, readPorts(m, "ports", "containerPort")...)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func servicePorts(u *unstructured.Unstructured) []api.ObjectPort {
	out := readPorts(u.Object["spec"], "ports", "port")
	if len(out) == 0 {
		return nil
	}
	return out
}

func readPorts(holder any, field, numberKey string) []api.ObjectPort {
	parent, ok := holder.(map[string]any)
	if !ok {
		return nil
	}
	entries, ok := parent[field].([]any)
	if !ok {
		return nil
	}
	out := []api.ObjectPort{}
	for _, entry := range entries {
		mapped, isMap := entry.(map[string]any)
		if !isMap {
			continue
		}
		protocol := stringField(mapped, "protocol")
		if protocol == "UDP" {
			continue
		}
		number := intField(mapped, numberKey)
		if number == 0 {
			continue
		}
		out = append(out, api.ObjectPort{
			Name:     stringField(mapped, "name"),
			Port:     number,
			Protocol: protocol,
		})
	}
	return out
}

const maxPort = 65535

func intField(m map[string]any, key string) int32 {
	switch value := m[key].(type) {
	case int64:
		return boundedPort(value)
	case float64:
		return boundedPort(int64(value))
	default:
		return 0
	}
}

func boundedPort(value int64) int32 {
	if value < 1 {
		return 0
	}
	if value > maxPort {
		return 0
	}
	return int32(value)
}

func containerNames(obj *unstructured.Unstructured) []string {
	if obj.GetKind() != "Pod" {
		return nil
	}
	names := namesFrom(obj, "initContainers")
	names = append(names, namesFrom(obj, "containers")...)
	names = append(names, namesFrom(obj, "ephemeralContainers")...)
	if len(names) == 0 {
		return nil
	}
	return names
}

func namesFrom(u *unstructured.Unstructured, field string) []string {
	out := []string{}
	for _, c := range nestedSlice(u, "spec", field) {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		name := stringField(m, "name")
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}

func stringField(m map[string]any, key string) string {
	v, ok := m[key].(string)
	if !ok {
		return ""
	}
	return v
}

func nestedSlice(u *unstructured.Unstructured, fields ...string) []any {
	v, found, err := unstructured.NestedSlice(u.Object, fields...)
	if !found || err != nil {
		return nil
	}
	return v
}
