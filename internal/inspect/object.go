package inspect

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/argocd"
	"github.com/sophotechlabs/spinoza/internal/flux"
	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const lastAppliedAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

const argoApplication = "Application"

const argoApplications = "applications"

const argoTrackingAnnotation = "argocd.argoproj.io/tracking-id"

var fluxAppliers = map[string]string{
	"kustomize.toolkit.fluxcd.io": "Kustomization",
	"helm.toolkit.fluxcd.io":      "HelmRelease",
}

var ErrNoResourceVersion = errors.New(
	"this document names no resourceVersion, so applying it would overwrite whatever is on the server now; " +
		"Revert to load the current object, then make the change again",
)

var readTimeout = 15 * time.Second

var writeTimeout = 30 * time.Second

func Get(ctx context.Context, dyn dynamic.Interface, ref api.ObjectRef) (api.ObjectDetail, error) {
	bounded, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	u, err := resourceFor(dyn, ref).Get(bounded, ref.Name, metav1.GetOptions{})
	if err != nil {
		return api.ObjectDetail{}, err
	}
	return detailOf(u)
}

func Apply(ctx context.Context, dyn dynamic.Interface, ref api.ObjectRef, kind string, doc []byte) (api.ObjectDetail, error) {
	obj := map[string]any{}
	unmarshalErr := yaml.Unmarshal(doc, &obj)
	if unmarshalErr != nil {
		return api.ObjectDetail{}, fmt.Errorf("parse yaml: %w", unmarshalErr)
	}
	desired := &unstructured.Unstructured{Object: obj}
	matchErr := matchesRef(desired, ref, kind)
	if matchErr != nil {
		return api.ObjectDetail{}, matchErr
	}
	if desired.GetResourceVersion() == "" {
		return api.ObjectDetail{}, ErrNoResourceVersion
	}
	bounded, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	updated, err := resourceFor(dyn, ref).Update(bounded, desired, metav1.UpdateOptions{FieldManager: fieldManager})
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

func matchesRef(doc *unstructured.Unstructured, ref api.ObjectRef, kind string) error {
	if doc.GetName() != ref.Name {
		return fmt.Errorf("document name %q does not match %q", doc.GetName(), ref.Name)
	}
	if doc.GetNamespace() != ref.Namespace {
		return fmt.Errorf("document namespace %q does not match %q", doc.GetNamespace(), ref.Namespace)
	}
	want := apiVersionOf(ref)
	if doc.GetAPIVersion() != want {
		return fmt.Errorf("document apiVersion %q does not match %q", doc.GetAPIVersion(), want)
	}
	if kind == "" {
		return nil
	}
	if doc.GetKind() != kind {
		return fmt.Errorf("document kind %q does not match %q", doc.GetKind(), kind)
	}
	return nil
}

func apiVersionOf(ref api.ObjectRef) string {
	if ref.Group == "" {
		return ref.Version
	}
	return ref.Group + "/" + ref.Version
}

func detailOf(source *unstructured.Unstructured) (api.ObjectDetail, error) {
	clean := sanitize(source)
	raw, err := yaml.Marshal(clean.Object)
	if err != nil {
		return api.ObjectDetail{}, fmt.Errorf("%w: could not render the object as yaml: %w", api.ErrInternal, err)
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
		Terminating: source.GetDeletionTimestamp() != nil,
		Finalizers:  source.GetFinalizers(),
		ManagedBy:   managedBy(source),
		Source:      sourceLabelOf(source),
		Replicas:    replicasOf(clean),
		Schedulable: schedulableOf(clean),
		HandledAt:   unstr.String(clean, "status", "lastHandledReconcileAt"),
		Ports:       portsOf(clean),
		Event:       eventFactsOf(clean),
		Data:        dataOf(clean),
		YAML:        string(raw),
	}, nil
}

func dataOf(item *unstructured.Unstructured) []api.DataEntry {
	switch item.GetKind() {
	case "Secret":
		return sorted(encodedEntries(item, "data"))
	case "ConfigMap":
		return sorted(append(plainEntries(item), encodedEntries(item, "binaryData")...))
	}
	return nil
}

func sorted(out []api.DataEntry) []api.DataEntry {
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, func(left, right api.DataEntry) int {
		return strings.Compare(left.Key, right.Key)
	})
	return out
}

func plainEntries(item *unstructured.Unstructured) []api.DataEntry {
	held, found, err := unstructured.NestedMap(item.Object, "data")
	if !found || err != nil {
		return nil
	}
	out := make([]api.DataEntry, 0, len(held))
	for key, raw := range held {
		text, ok := raw.(string)
		if !ok {
			continue
		}
		out = append(out, api.DataEntry{Key: key, Value: text, Bytes: len(text)})
	}
	return out
}

func encodedEntries(item *unstructured.Unstructured, field string) []api.DataEntry {
	held, found, err := unstructured.NestedMap(item.Object, field)
	if !found || err != nil {
		return nil
	}
	out := make([]api.DataEntry, 0, len(held))
	for key, raw := range held {
		encoded, ok := raw.(string)
		if !ok {
			continue
		}
		out = append(out, entryOf(key, encoded))
	}
	return out
}

func entryOf(key, encoded string) api.DataEntry {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return api.DataEntry{Key: key, Value: encoded, Bytes: len(encoded), Binary: true}
	}
	if !utf8.Valid(decoded) {
		return api.DataEntry{Key: key, Value: encoded, Bytes: len(decoded), Binary: true}
	}
	return api.DataEntry{Key: key, Value: string(decoded), Bytes: len(decoded)}
}

func eventFactsOf(item *unstructured.Unstructured) *api.ObjectEvent {
	if item.GetKind() != "Event" {
		return nil
	}
	out := api.ObjectEvent{
		Type:      unstr.String(item, "type"),
		Reason:    unstr.String(item, "reason"),
		Message:   firstOf(item, []string{"message"}, []string{"note"}),
		Object:    eventObjectOf(item),
		Source:    eventSourceOf(item),
		Count:     eventCountOf(item),
		FirstSeen: firstOf(item, []string{"firstTimestamp"}, []string{"eventTime"}),
		LastSeen: firstOf(
			item,
			[]string{"lastTimestamp"},
			[]string{"series", "lastObservedTime"},
			[]string{"deprecatedLastTimestamp"},
			[]string{"eventTime"},
		),
	}
	return &out
}

func firstOf(item *unstructured.Unstructured, paths ...[]string) string {
	for _, path := range paths {
		found := unstr.String(item, path...)
		if found != "" {
			return found
		}
	}
	return ""
}

func eventObjectOf(item *unstructured.Unstructured) string {
	for _, field := range []string{"involvedObject", "regarding"} {
		kind := unstr.String(item, field, "kind")
		name := unstr.String(item, field, "name")
		namespace := unstr.String(item, field, "namespace")
		if kind == "" && name == "" {
			continue
		}
		if namespace == "" {
			return kind + "/" + name
		}
		return kind + " " + namespace + "/" + name
	}
	return ""
}

func eventSourceOf(item *unstructured.Unstructured) string {
	component := firstOf(item, []string{"source", "component"}, []string{"reportingComponent"})
	host := firstOf(item, []string{"source", "host"}, []string{"reportingInstance"})
	if host == "" {
		return component
	}
	if component == "" {
		return host
	}
	return component + " on " + host
}

func eventCountOf(item *unstructured.Unstructured) int64 {
	for _, path := range [][]string{{"count"}, {"deprecatedCount"}, {"series", "count"}} {
		found := unstr.Int(item, path...)
		if found > 0 {
			return found
		}
	}
	return 0
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
	for _, c := range unstr.Slice(u, "status", "conditions") {
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

func sourceLabelOf(item *unstructured.Unstructured) string {
	if !flux.IsFluxGroup(item.GroupVersionKind().Group) {
		return ""
	}
	kind, name, _ := flux.SourceRef(item)
	if kind == "" || name == "" {
		return ""
	}
	return kind + "/" + name
}

func managedBy(item *unstructured.Unstructured) *api.GitopsOwner {
	labels := item.GetLabels()
	for group, kind := range fluxAppliers {
		name := labels[group+"/name"]
		namespace := labels[group+"/namespace"]
		if name == "" || namespace == "" {
			continue
		}
		return &api.GitopsOwner{
			Controller: api.ControllerFlux,
			Kind:       kind,
			Ref: api.ObjectRef{
				Group:     group,
				Namespace: namespace,
				Name:      name,
			},
		}
	}
	return argoOwner(item)
}

func argoOwner(item *unstructured.Unstructured) *api.GitopsOwner {
	tracked := item.GetAnnotations()[argoTrackingAnnotation]
	if tracked == "" {
		return nil
	}
	name, _, found := strings.Cut(tracked, ":")
	if !found || name == "" {
		return nil
	}
	return &api.GitopsOwner{
		Controller: api.ControllerArgo,
		Kind:       argoApplication,
		Ref: api.ObjectRef{
			Group:    argocd.Group,
			Resource: argoApplications,
			Name:     name,
		},
	}
}

func suspendedOf(u *unstructured.Unstructured) *bool {
	if argocd.IsArgoGroup(u.GroupVersionKind().Group) && u.GetKind() == argoApplication {
		paused := !argocd.AutoSyncing(u)
		return &paused
	}
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
	for _, container := range unstr.Slice(u, "spec", "containers") {
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
		if protocol != "" && protocol != "TCP" {
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
	for _, c := range unstr.Slice(u, "spec", field) {
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
