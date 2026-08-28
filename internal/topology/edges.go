package topology

import (
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/unstr"
)

var workloadTemplate = []string{fieldSpec, fieldTemplate, fieldSpec}

var podTemplatePaths = map[string][]string{
	kindPod:                   {fieldSpec},
	kindDeployment:            workloadTemplate,
	kindReplicaSet:            workloadTemplate,
	kindReplicationController: workloadTemplate,
	kindStatefulSet:           workloadTemplate,
	kindDaemonSet:             workloadTemplate,
	kindJob:                   workloadTemplate,
	kindCronJob:               {fieldSpec, "jobTemplate", fieldSpec, fieldTemplate, fieldSpec},
}

func (b *builder) link() {
	listed := b.listed()
	for _, entry := range listed {
		b.ownerEdges(entry)
		b.configEdges(entry)
	}
	for _, entry := range listed {
		if entry.node.Category == categoryService {
			b.serviceEdges(entry)
		}
		if entry.node.Category == categoryIngress {
			b.ingressEdges(entry)
		}
		if entry.node.Category == categoryAutoscaler {
			b.scaleEdges(entry)
		}
	}
}

func (b *builder) listed() []*object {
	ids := make([]string, 0, len(b.objects))
	for id, entry := range b.objects {
		if entry.raw == nil {
			continue
		}
		ids = append(ids, id)
	}
	slices.Sort(ids)
	out := make([]*object, 0, len(ids))
	for _, id := range ids {
		out = append(out, b.objects[id])
	}
	return out
}

func (b *builder) ownerEdges(entry *object) {
	for _, owner := range entry.raw.GetOwnerReferences() {
		if owner.UID == "" {
			continue
		}
		id := b.ensureOwner(owner, entry.node.Namespace)
		b.addEdge(id, entry.node.ID, edgeOwns)
		if owner.Controller != nil && *owner.Controller {
			b.controller[entry.node.ID] = id
		}
	}
}

func (b *builder) serviceEdges(entry *object) {
	selector := stringsOf(unstr.Map(entry.raw, fieldSpec, "selector"))
	if len(selector) == 0 {
		return
	}
	for _, id := range b.podsByNs[entry.node.Namespace] {
		pod := b.objects[id]
		if !selects(selector, pod.raw.GetLabels()) {
			continue
		}
		b.addEdge(entry.node.ID, id, edgeRoutes)
	}
}

func selects(selector, labels map[string]string) bool {
	for key, want := range selector {
		if labels[key] != want {
			return false
		}
	}
	return true
}

func stringsOf(raw map[string]any, present bool) map[string]string {
	if !present {
		return nil
	}
	out := make(map[string]string, len(raw))
	for key, value := range raw {
		text, ok := value.(string)
		if !ok {
			continue
		}
		out[key] = text
	}
	return out
}

func (b *builder) ingressEdges(entry *object) {
	for _, name := range backendServices(entry.raw) {
		id := b.ensure("", kindService, entry.node.Namespace, name, categoryService, statusMissing)
		b.addEdge(entry.node.ID, id, edgeRoutes)
	}
}

func backendServices(obj *unstructured.Unstructured) []string {
	names := []string{}
	names = appendService(names, mapAt(obj.Object, fieldSpec, "defaultBackend"))
	for _, rule := range unstr.Slice(obj, fieldSpec, "rules") {
		entry, ok := rule.(map[string]any)
		if !ok {
			continue
		}
		for _, path := range sliceAt(entry, "http", "paths") {
			step, ok := path.(map[string]any)
			if !ok {
				continue
			}
			names = appendService(names, mapAt(step, "backend"))
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

func appendService(names []string, backend map[string]any) []string {
	service := mapAt(backend, "service")
	name := unstr.At(service, "name")
	if name == "" {
		return names
	}
	return append(names, name)
}

func (b *builder) scaleEdges(entry *object) {
	target := mapAt(entry.raw.Object, fieldSpec, "scaleTargetRef")
	kind := unstr.At(target, "kind")
	name := unstr.At(target, "name")
	if kind == "" || name == "" {
		return
	}
	group := groupOf(unstr.At(target, "apiVersion"))
	id := b.ensure(group, kind, entry.node.Namespace, name, categoryWorkload, statusMissing)
	b.addEdge(entry.node.ID, id, edgeScales)
}

func (b *builder) configEdges(entry *object) {
	path, ok := podTemplatePaths[entry.node.Kind]
	if !ok {
		return
	}
	spec := mapAt(entry.raw.Object, path...)
	for _, ref := range configRefs(spec) {
		id := b.ensure("", ref.kind, entry.node.Namespace, ref.name, categoryConfig, "")
		b.addEdge(id, entry.node.ID, edgeConfigures)
	}
}

type configRef struct {
	kind string
	name string
}

func configRefs(spec map[string]any) []configRef {
	refs := []configRef{}
	refs = append(refs, volumeRefs(spec)...)
	refs = append(refs, pullSecretRefs(spec)...)
	for _, key := range []string{"initContainers", "containers", "ephemeralContainers"} {
		for _, raw := range sliceAt(spec, key) {
			container, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			refs = append(refs, containerRefs(container)...)
		}
	}
	return dedupe(refs)
}

func volumeRefs(spec map[string]any) []configRef {
	refs := []configRef{}
	for _, raw := range sliceAt(spec, "volumes") {
		volume, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		refs = appendRef(refs, kindConfigMap, unstr.At(mapAt(volume, "configMap"), "name"))
		refs = appendRef(refs, kindSecret, unstr.At(mapAt(volume, "secret"), "secretName"))
		refs = append(refs, projectedRefs(volume)...)
	}
	return refs
}

func projectedRefs(volume map[string]any) []configRef {
	sources := sliceAt(volume, "projected", "sources")
	if injectedByKubelet(sources) {
		return nil
	}
	refs := []configRef{}
	for _, raw := range sources {
		source, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		refs = appendRef(refs, kindConfigMap, unstr.At(mapAt(source, "configMap"), "name"))
		refs = appendRef(refs, kindSecret, unstr.At(mapAt(source, "secret"), "name"))
	}
	return refs
}

func injectedByKubelet(sources []any) bool {
	for _, raw := range sources {
		source, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if mapAt(source, "serviceAccountToken") != nil {
			return true
		}
	}
	return false
}

func pullSecretRefs(spec map[string]any) []configRef {
	refs := []configRef{}
	for _, raw := range sliceAt(spec, "imagePullSecrets") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		refs = appendRef(refs, kindSecret, unstr.At(entry, "name"))
	}
	return refs
}

func containerRefs(container map[string]any) []configRef {
	refs := []configRef{}
	for _, raw := range sliceAt(container, "envFrom") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		refs = appendRef(refs, kindConfigMap, unstr.At(mapAt(entry, "configMapRef"), "name"))
		refs = appendRef(refs, kindSecret, unstr.At(mapAt(entry, "secretRef"), "name"))
	}
	for _, raw := range sliceAt(container, "env") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		source := mapAt(entry, "valueFrom")
		refs = appendRef(refs, kindConfigMap, unstr.At(mapAt(source, "configMapKeyRef"), "name"))
		refs = appendRef(refs, kindSecret, unstr.At(mapAt(source, "secretKeyRef"), "name"))
	}
	return refs
}

func appendRef(refs []configRef, kind, name string) []configRef {
	if name == "" {
		return refs
	}
	return append(refs, configRef{kind: kind, name: name})
}

func dedupe(refs []configRef) []configRef {
	slices.SortFunc(refs, func(left, right configRef) int {
		if left.kind != right.kind {
			return strings.Compare(left.kind, right.kind)
		}
		return strings.Compare(left.name, right.name)
	})
	return slices.Compact(refs)
}

func mapAt(entry map[string]any, keys ...string) map[string]any {
	current := entry
	for _, key := range keys {
		if current == nil {
			return nil
		}
		next, ok := current[key].(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}
	return current
}

func sliceAt(entry map[string]any, keys ...string) []any {
	last := len(keys) - 1
	holder := mapAt(entry, keys[:last]...)
	if holder == nil {
		return nil
	}
	out, ok := holder[keys[last]].([]any)
	if !ok {
		return nil
	}
	return out
}
