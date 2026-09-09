package rollout

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const (
	appsGroup           = "apps"
	replicaSets         = "replicasets"
	controllerRevisions = "controllerrevisions"
	deploymentKind      = "Deployment"
	statefulSetKind     = "StatefulSet"
	daemonSetKind       = "DaemonSet"
	revisionAnnotation  = "deployment.kubernetes.io/revision"
	causeAnnotation     = "kubernetes.io/change-cause"
	templateHashLabel   = "pod-template-hash"
	revisionHashLabel   = "controller-revision-hash"
	specField           = "spec"
	templateField       = "template"
)

type Source interface {
	Get(ctx context.Context, ref api.ObjectRef) (*unstructured.Unstructured, error)
	List(ctx context.Context, ref api.ObjectRef) ([]*unstructured.Unstructured, error)
}

func FromDynamic(dyn dynamic.Interface) Source {
	return clusterSource{dyn: dyn}
}

type clusterSource struct {
	dyn dynamic.Interface
}

func (source clusterSource) Get(ctx context.Context, ref api.ObjectRef) (*unstructured.Unstructured, error) {
	return source.objects(ref).Get(ctx, ref.Name, metav1.GetOptions{})
}

func (source clusterSource) List(ctx context.Context, ref api.ObjectRef) ([]*unstructured.Unstructured, error) {
	found, err := source.objects(ref).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]*unstructured.Unstructured, 0, len(found.Items))
	for i := range found.Items {
		out = append(out, &found.Items[i])
	}
	return out, nil
}

func (source clusterSource) objects(ref api.ObjectRef) dynamic.ResourceInterface {
	gvr := schema.GroupVersionResource{Group: ref.Group, Version: ref.Version, Resource: ref.Resource}
	if ref.Namespace == "" {
		return source.dyn.Resource(gvr)
	}
	return source.dyn.Resource(gvr).Namespace(ref.Namespace)
}

type entry struct {
	api.Revision

	template map[string]any
}

func List(ctx context.Context, src Source, ref api.ObjectRef) (api.Revisions, error) {
	found, reason, err := history(ctx, src, ref)
	if err != nil {
		return api.Revisions{}, err
	}
	if reason != "" {
		return api.Revisions{Supported: false, Reason: reason}, nil
	}
	out := make([]api.Revision, 0, len(found))
	for _, one := range found {
		out = append(out, one.Revision)
	}
	return api.Revisions{Revisions: out, Supported: true}, nil
}

func Template(ctx context.Context, src Source, ref api.ObjectRef, number int64) (map[string]any, error) {
	found, reason, err := history(ctx, src, ref)
	if err != nil {
		return nil, err
	}
	if reason != "" {
		return nil, errors.New(reason)
	}
	return pick(found, ref, number)
}

func pick(found []entry, ref api.ObjectRef, number int64) (map[string]any, error) {
	for _, one := range found {
		if one.Number != number {
			continue
		}
		if one.template == nil {
			return nil, fmt.Errorf("revision %d of %s holds no pod template", number, ref.Name)
		}
		return one.template, nil
	}
	return nil, fmt.Errorf("%s has no revision %d", ref.Name, number)
}

func history(ctx context.Context, src Source, ref api.ObjectRef) ([]entry, string, error) {
	kind := kindOf(ref)
	switch kind {
	case deploymentKind:
		found, err := replicaSetHistory(ctx, src, ref)
		return found, "", err
	case statefulSetKind, daemonSetKind:
		found, err := controllerRevisionHistory(ctx, src, ref, kind)
		return found, "", err
	default:
		return nil, describe(ref) + " keep no rollout revisions", nil
	}
}

var kinds = map[string]string{
	"deployments":  deploymentKind,
	"statefulsets": statefulSetKind,
	"daemonsets":   daemonSetKind,
}

func kindOf(ref api.ObjectRef) string {
	if ref.Group != appsGroup {
		return ""
	}
	return kinds[ref.Resource]
}

func describe(ref api.ObjectRef) string {
	if ref.Group == "" {
		return ref.Resource
	}
	return ref.Group + "/" + ref.Resource
}

func replicaSetHistory(ctx context.Context, src Source, ref api.ObjectRef) ([]entry, error) {
	if _, err := src.Get(ctx, ref); err != nil {
		return nil, err
	}
	sets, err := src.List(ctx, beside(ref, replicaSets))
	if err != nil {
		return nil, err
	}
	found := make([]entry, 0, len(sets))
	for _, set := range sets {
		if !ownedBy(set, ref.Name, deploymentKind) {
			continue
		}
		number, numbered := annotatedRevision(set)
		if !numbered {
			continue
		}
		one := entryOf(set, number, templateOf(set, specField, templateField))
		one.Replicas = int(unstr.Int(set, specField, "replicas"))
		one.Ready = int(unstr.Int(set, "status", "readyReplicas"))
		found = append(found, one)
	}
	return ordered(found), nil
}

func controllerRevisionHistory(ctx context.Context, src Source, ref api.ObjectRef, kind string) ([]entry, error) {
	if _, err := src.Get(ctx, ref); err != nil {
		return nil, err
	}
	held, err := src.List(ctx, beside(ref, controllerRevisions))
	if err != nil {
		return nil, err
	}
	found := make([]entry, 0, len(held))
	for _, revision := range held {
		if !ownedBy(revision, ref.Name, kind) {
			continue
		}
		number := unstr.Int(revision, "revision")
		if number == 0 {
			continue
		}
		one := entryOf(revision, number, templateOf(revision, "data", specField, templateField))
		one.Replicas = int(unstr.Int(revision, "data", specField, "replicas"))
		found = append(found, one)
	}
	return ordered(found), nil
}

func entryOf(item *unstructured.Unstructured, number int64, template map[string]any) entry {
	return entry{
		Revision: api.Revision{
			Number:    number,
			Name:      item.GetName(),
			CreatedAt: createdAt(item),
			Cause:     item.GetAnnotations()[causeAnnotation],
			Images:    imagesOf(template),
		},
		template: template,
	}
}

func beside(ref api.ObjectRef, resource string) api.ObjectRef {
	return api.ObjectRef{
		Group:     appsGroup,
		Version:   ref.Version,
		Resource:  resource,
		Namespace: ref.Namespace,
	}
}

func ownedBy(item *unstructured.Unstructured, name, kind string) bool {
	for _, owner := range item.GetOwnerReferences() {
		if owner.Kind == kind && owner.Name == name {
			return true
		}
	}
	return false
}

func annotatedRevision(item *unstructured.Unstructured) (int64, bool) {
	raw := item.GetAnnotations()[revisionAnnotation]
	if raw == "" {
		return 0, false
	}
	number, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return number, true
}

func templateOf(item *unstructured.Unstructured, fields ...string) map[string]any {
	template, found := unstr.Map(item, fields...)
	if !found {
		return nil
	}
	return withoutHashes(template)
}

func withoutHashes(template map[string]any) map[string]any {
	labels, found, err := unstructured.NestedStringMap(template, "metadata", "labels")
	if !found || err != nil {
		return template
	}
	delete(labels, templateHashLabel)
	delete(labels, revisionHashLabel)
	if len(labels) == 0 {
		unstructured.RemoveNestedField(template, "metadata", "labels")
		return template
	}
	setErr := unstructured.SetNestedStringMap(template, labels, "metadata", "labels")
	if setErr != nil {
		unstructured.RemoveNestedField(template, "metadata", "labels")
	}
	return template
}

func createdAt(item *unstructured.Unstructured) string {
	stamp := item.GetCreationTimestamp()
	if stamp.IsZero() {
		return ""
	}
	return stamp.Time.UTC().Format(time.RFC3339)
}

func imagesOf(template map[string]any) []string {
	containers, found, err := unstructured.NestedSlice(template, specField, "containers")
	if !found || err != nil {
		return nil
	}
	var out []string
	for _, raw := range containers {
		container, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		image := unstr.At(container, "image")
		if image == "" {
			continue
		}
		out = append(out, image)
	}
	return out
}

func ordered(found []entry) []entry {
	slices.SortFunc(found, func(left, right entry) int {
		if left.Number != right.Number {
			return cmp.Compare(right.Number, left.Number)
		}
		return strings.Compare(left.Name, right.Name)
	})
	return onlyOneCurrent(found)
}

func onlyOneCurrent(found []entry) []entry {
	for at := range found {
		found[at].Current = at == 0
	}
	return found
}
