package checks

import (
	"slices"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/unstr"
)

var (
	serviceTarget   = target{group: "", resource: "services"}
	accountTarget   = target{group: "", resource: "serviceaccounts"}
	configMapTarget = target{group: "", resource: "configmaps"}
	secretTarget    = target{group: "", resource: "secrets"}
	claimTarget     = target{group: "", resource: "persistentvolumeclaims"}
	ingressTarget   = target{group: networkGroup, resource: "ingresses"}
	classTarget     = target{group: networkGroup, resource: "ingressclasses"}
	storageTarget   = target{group: "storage.k8s.io", resource: "storageclasses"}
	policyTarget    = target{group: networkGroup, resource: "networkpolicies"}
	budgetTarget    = target{group: "policy", resource: "poddisruptionbudgets"}
	scalerTarget    = target{group: "autoscaling", resource: "horizontalpodautoscalers"}
	priorityTarget  = target{group: "scheduling.k8s.io", resource: "priorityclasses"}
	runtimeTarget   = target{group: "node.k8s.io", resource: "runtimeclasses"}
)

const networkGroup = "networking.k8s.io"

func referenceChecks() []check {
	out := workloadRefChecks()
	out = append(out, networkChecks()...)
	out = append(out, orphanChecks()...)
	return out
}

func workloadRefChecks() []check {
	return []check{
		{
			id:       "service-account-missing",
			title:    "Names a ServiceAccount that does not exist",
			category: categoryReliability,
			severity: severityHigh,
			needs:    []target{accountTarget},
			wrong:    "The pod cannot be admitted until the account exists, so the controller creates it and gets nothing back.",
			remedy:   "Create the ServiceAccount, or point serviceAccountName at one that is there.",
			find:     overFacts(serviceAccountMissing),
		},
		{
			id:       "config-map-missing",
			title:    "Reads a ConfigMap that does not exist",
			category: categoryReliability,
			severity: severityHigh,
			needs:    []target{configMapTarget},
			wrong:    "The pod stays in CreateContainerConfigError until the ConfigMap appears.",
			remedy:   "Create the ConfigMap, or mark the reference optional.",
			find:     overFacts(configMapMissing),
		},
		{
			id:       "config-map-key-missing",
			title:    "Reads a key the ConfigMap does not have",
			category: categoryReliability,
			severity: severityHigh,
			needs:    []target{configMapTarget},
			wrong:    "The ConfigMap is there and the key is not, which fails the container in the same way a missing map would.",
			remedy:   "Add the key, or point configMapKeyRef at one that exists.",
			find:     overFacts(configMapKeyMissing),
		},
		{
			id:       "secret-missing",
			title:    "Reads a Secret that does not exist",
			category: categoryReliability,
			severity: severityHigh,
			needs:    []target{secretTarget},
			wrong:    "The pod stays in CreateContainerConfigError until the Secret appears.",
			remedy:   "Create the Secret, or mark the reference optional.",
			find:     overFacts(secretMissing),
		},
		{
			id:       "pull-secret-missing",
			title:    "Names an imagePullSecret that does not exist",
			category: categoryReliability,
			severity: severityMedium,
			needs:    []target{secretTarget},
			wrong:    "The pull falls back to anonymous, so a private image fails to pull with no obvious cause.",
			remedy:   "Create the pull secret, or take the reference out.",
			find:     overFacts(pullSecretMissing),
		},
		{
			id:       "claim-missing",
			title:    "Mounts a PersistentVolumeClaim that does not exist",
			category: categoryReliability,
			severity: severityHigh,
			needs:    []target{claimTarget},
			wrong:    "The pod stays Pending on a volume that will never bind.",
			remedy:   "Create the claim, or take the volume out.",
			find:     overFacts(claimMissing),
		},
		{
			id:       "priority-class-missing",
			title:    "Names a PriorityClass that does not exist",
			category: categoryReliability,
			severity: severityHigh,
			needs:    []target{priorityTarget},
			wrong:    "The pod is rejected at admission, so the controller never gets one.",
			remedy:   "Create the PriorityClass, or take the name out.",
			find:     overFacts(priorityClassMissing),
		},
		{
			id:       "runtime-class-missing",
			title:    "Names a RuntimeClass that does not exist",
			category: categoryReliability,
			severity: severityHigh,
			needs:    []target{runtimeTarget},
			wrong:    "The pod is rejected at admission until the RuntimeClass exists.",
			remedy:   "Create the RuntimeClass, or take the name out.",
			find:     overFacts(runtimeClassMissing),
		},
		{
			id:       "no-service-selects-it",
			title:    "Nothing routes to this workload",
			category: categoryReliability,
			severity: severityLow,
			needs:    []target{serviceTarget},
			wrong:    "No Service selects these pods, so nothing in the cluster can reach them by name. A judgement call: a worker that only makes outbound calls needs none.",
			remedy:   "Add a Service whose selector matches the pod labels, or leave it if nothing calls in.",
			find:     overFacts(noServiceSelectsIt),
		},
		{
			id:       "no-network-policy",
			title:    "No NetworkPolicy covers this workload",
			category: categorySecurity,
			severity: severityLow,
			needs:    []target{policyTarget},
			frameworks: []string{
				nsaCisa,
			},
			wrong:  "Every pod in the cluster can open a connection to it, while the pods beside it are behind a policy. A cluster with no NetworkPolicy at all is left alone; this only fires once you have started using them.",
			remedy: "Add a NetworkPolicy selecting these pods and naming what may reach them.",
			find:   overFacts(noNetworkPolicy),
		},
		{
			id:       "no-disruption-budget",
			title:    "No PodDisruptionBudget",
			category: categoryReliability,
			severity: severityLow,
			needs:    []target{budgetTarget},
			wrong:    "A drain evicts every replica at once, so a node upgrade is an outage.",
			remedy:   "Add a PodDisruptionBudget with minAvailable set below the replica count.",
			find:     overFacts(noDisruptionBudget),
		},
		{
			id:       "budget-blocks-every-eviction",
			title:    "Disruption budget a drain can never satisfy",
			category: categoryReliability,
			severity: severityHigh,
			needs:    []target{budgetTarget},
			wrong:    "minAvailable is at or above the replica count, so no pod may ever be evicted and a drain hangs for ever.",
			remedy:   "Lower minAvailable below spec.replicas, or raise the replica count above it.",
			find:     overFacts(budgetBlocksEviction),
		},
		{
			id:       "scaler-fights-fixed-replicas",
			title:    "An autoscaler and a fixed replica count on one workload",
			category: categoryReliability,
			severity: severityMedium,
			needs:    []target{scalerTarget},
			wrong:    "The controller writes the replica count and the autoscaler writes it back, so the workload oscillates.",
			remedy:   "Remove spec.replicas from the workload and let the HorizontalPodAutoscaler own it.",
			find:     overFacts(scalerFightsReplicas),
		},
	}
}

func orphanChecks() []check {
	return []check{
		{
			id:       "claim-nothing-mounts",
			title:    "PersistentVolumeClaim nothing mounts",
			category: categoryEfficiency,
			severity: severityLow,
			needs:    []target{claimTarget},
			wrong:    "The volume is provisioned and billed and no pod uses it. A judgement call: a claim between deployments is normal for a short while.",
			remedy:   "Delete the claim, or attach it to the workload that is meant to use it.",
			find:     overCorpus(unmountedClaims),
		},
	}
}

func serviceAccountMissing(subject Subject, held *corpus) (string, string) {
	name := stringAt(subject.Pod, "serviceAccountName")
	if name == "" || name == defaultNamespace {
		return "", ""
	}
	if held.named("", "serviceaccounts", subject.Ref.Namespace, name) {
		return "", ""
	}
	return "serviceAccountName is " + name + ", which this namespace does not have", ""
}

type reference struct {
	kind     string
	name     string
	key      string
	optional bool
}

func referencedMaps(subject Subject, field string) []reference {
	out := make([]reference, 0, len(subject.Containers))
	for _, container := range subject.Containers {
		out = append(out, envReferences(container, field)...)
	}
	out = append(out, volumeReferences(subject, field)...)
	return out
}

func envReferences(container Container, field string) []reference {
	entries := envEntries(container)
	out := make([]reference, 0, len(entries))
	for _, entry := range entries {
		from, ok := entry["valueFrom"].(map[string]any)
		if !ok {
			continue
		}
		ref, isRef := from[field+"KeyRef"].(map[string]any)
		if !isRef {
			continue
		}
		out = append(out, reference{
			kind:     field,
			name:     stringAt(ref, "name"),
			key:      stringAt(ref, "key"),
			optional: optionalAt(ref),
		})
	}
	out = append(out, envFromReferences(container, field)...)
	return out
}

func envFromReferences(container Container, field string) []reference {
	listed, ok := container.Spec["envFrom"].([]any)
	if !ok {
		return nil
	}
	out := make([]reference, 0, len(listed))
	for _, raw := range listed {
		entry, isMap := raw.(map[string]any)
		if !isMap {
			continue
		}
		ref, isRef := entry[field+"Ref"].(map[string]any)
		if !isRef {
			continue
		}
		out = append(out, reference{kind: field, name: stringAt(ref, "name"), optional: optionalAt(ref)})
	}
	return out
}

func volumeReferences(subject Subject, field string) []reference {
	volumes, ok := subject.Pod["volumes"].([]any)
	if !ok {
		return nil
	}
	out := make([]reference, 0, len(volumes))
	for _, raw := range volumes {
		volume, isMap := raw.(map[string]any)
		if !isMap {
			continue
		}
		out = append(out, volumeSourceReference(volume, field)...)
	}
	return out
}

func volumeSourceReference(volume map[string]any, field string) []reference {
	source, ok := volume[volumeField(field)].(map[string]any)
	if !ok {
		return nil
	}
	return []reference{{kind: field, name: volumeName(source, field), optional: optionalAt(source)}}
}

func volumeField(field string) string {
	if field == secretField {
		return "secret"
	}
	return "configMap"
}

func volumeName(source map[string]any, field string) string {
	if field == secretField {
		return stringAt(source, "secretName")
	}
	return stringAt(source, "name")
}

func optionalAt(entry map[string]any) bool {
	value, set := boolAt(entry, "optional")
	return set && value
}

func configMapMissing(subject Subject, held *corpus) (string, string) {
	return missingReference(subject, held, "configMap", "configmaps", "ConfigMap")
}

func secretMissing(subject Subject, held *corpus) (string, string) {
	return missingReference(subject, held, secretField, "secrets", "Secret")
}

func missingReference(subject Subject, held *corpus, field, resource, label string) (string, string) {
	for _, ref := range referencedMaps(subject, field) {
		if ref.name == "" || ref.optional {
			continue
		}
		if held.named("", resource, subject.Ref.Namespace, ref.name) {
			continue
		}
		return "references the " + label + " " + ref.name + ", which this namespace does not have", ""
	}
	return "", ""
}

func configMapKeyMissing(subject Subject, held *corpus) (string, string) {
	for _, ref := range referencedMaps(subject, "configMap") {
		if ref.name == "" || ref.key == "" || ref.optional {
			continue
		}
		data, found := configMapKeys(held, subject.Ref.Namespace, ref.name)
		if !found || slices.Contains(data, ref.key) {
			continue
		}
		return "reads " + ref.key + " from the ConfigMap " + ref.name + ", which does not have it", ""
	}
	return "", ""
}

func configMapKeys(held *corpus, namespace, name string) (keys []string, found bool) {
	for _, obj := range held.inNamespace("", "configmaps", namespace) {
		if obj.GetName() != name {
			continue
		}
		return append(mapKeys(obj, "data"), mapKeys(obj, "binaryData")...), true
	}
	return nil, false
}

func mapKeys(obj *unstructured.Unstructured, field string) []string {
	raw, ok, err := unstructured.NestedFieldNoCopy(obj.Object, field)
	if !ok || err != nil {
		return nil
	}
	entries, isMap := raw.(map[string]any)
	if !isMap {
		return nil
	}
	out := make([]string, 0, len(entries))
	for key := range entries {
		out = append(out, key)
	}
	return out
}

func pullSecretMissing(subject Subject, held *corpus) (string, string) {
	listed, ok := subject.Pod["imagePullSecrets"].([]any)
	if !ok {
		return "", ""
	}
	for _, raw := range listed {
		entry, isMap := raw.(map[string]any)
		if !isMap {
			continue
		}
		name := stringAt(entry, "name")
		if name == "" || held.named("", "secrets", subject.Ref.Namespace, name) {
			continue
		}
		return "imagePullSecrets names " + name + ", which this namespace does not have", ""
	}
	return "", ""
}

func claimNames(subject Subject) []string {
	volumes, ok := subject.Pod["volumes"].([]any)
	if !ok {
		return nil
	}
	out := []string{}
	for _, raw := range volumes {
		volume, isMap := raw.(map[string]any)
		if !isMap {
			continue
		}
		claim, isClaim := volume["persistentVolumeClaim"].(map[string]any)
		if !isClaim {
			continue
		}
		if name := stringAt(claim, "claimName"); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func claimMissing(subject Subject, held *corpus) (string, string) {
	for _, name := range claimNames(subject) {
		if held.named("", "persistentvolumeclaims", subject.Ref.Namespace, name) {
			continue
		}
		return "mounts the claim " + name + ", which this namespace does not have", ""
	}
	return "", ""
}

func priorityClassMissing(subject Subject, held *corpus) (string, string) {
	name := stringAt(subject.Pod, "priorityClassName")
	if name == "" || strings.HasPrefix(name, "system-") {
		return "", ""
	}
	if held.named("scheduling.k8s.io", "priorityclasses", "", name) {
		return "", ""
	}
	return "priorityClassName is " + name + ", which the cluster does not have", ""
}

func runtimeClassMissing(subject Subject, held *corpus) (string, string) {
	name := stringAt(subject.Pod, "runtimeClassName")
	if name == "" {
		return "", ""
	}
	if held.named("node.k8s.io", "runtimeclasses", "", name) {
		return "", ""
	}
	return "runtimeClassName is " + name + ", which the cluster does not have", ""
}

func podLabels(subject Subject) map[string]string {
	labels := templateLabels(subject)
	out := map[string]string{}
	for key, value := range labels {
		text, isString := value.(string)
		if !isString {
			continue
		}
		out[key] = text
	}
	return out
}

func selectorPicks(selector, labels map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for key, value := range selector {
		if labels[key] != value {
			return false
		}
	}
	return true
}

func stringMapAt(obj *unstructured.Unstructured, fields ...string) map[string]string {
	raw, ok, err := unstructured.NestedFieldNoCopy(obj.Object, fields...)
	if !ok || err != nil {
		return nil
	}
	entries, isMap := raw.(map[string]any)
	if !isMap {
		return nil
	}
	out := map[string]string{}
	for key, value := range entries {
		text, isString := value.(string)
		if !isString {
			continue
		}
		out[key] = text
	}
	return out
}

func servesWorkloads(subject Subject) bool {
	return isSpreadable(subject.Kind) || subject.Kind == daemonSetKind
}

func noServiceSelectsIt(subject Subject, held *corpus) (string, string) {
	if !servesWorkloads(subject) {
		return "", ""
	}
	labels := podLabels(subject)
	if len(labels) == 0 {
		return "", ""
	}
	for _, service := range held.inNamespace("", "services", subject.Ref.Namespace) {
		if selectorPicks(stringMapAt(service, specField, "selector"), labels) {
			return "", ""
		}
	}
	return "no Service in " + subject.Ref.Namespace + " selects these pods", ""
}

func noNetworkPolicy(subject Subject, held *corpus) (string, string) {
	if !servesWorkloads(subject) || len(held.of(networkGroup, "networkpolicies")) == 0 {
		return "", ""
	}
	labels := podLabels(subject)
	if len(labels) == 0 {
		return "", ""
	}
	for _, policy := range held.inNamespace(networkGroup, "networkpolicies", subject.Ref.Namespace) {
		selector := stringMapAt(policy, specField, "podSelector", "matchLabels")
		if len(selector) == 0 || selectorPicks(selector, labels) {
			return "", ""
		}
	}
	return "no NetworkPolicy in " + subject.Ref.Namespace + " covers these pods", ""
}

func budgetsFor(subject Subject, held *corpus) []*unstructured.Unstructured {
	labels := podLabels(subject)
	out := []*unstructured.Unstructured{}
	if len(labels) == 0 {
		return out
	}
	for _, budget := range held.inNamespace("policy", "poddisruptionbudgets", subject.Ref.Namespace) {
		if selectorPicks(stringMapAt(budget, specField, "selector", "matchLabels"), labels) {
			out = append(out, budget)
		}
	}
	return out
}

func noDisruptionBudget(subject Subject, held *corpus) (string, string) {
	if !servesWorkloads(subject) || subject.Replicas < 2 {
		return "", ""
	}
	if len(budgetsFor(subject, held)) > 0 {
		return "", ""
	}
	return strconv.FormatInt(subject.Replicas, 10) + " replicas and no PodDisruptionBudget", ""
}

func budgetBlocksEviction(subject Subject, held *corpus) (string, string) {
	if !servesWorkloads(subject) {
		return "", ""
	}
	for _, budget := range budgetsFor(subject, held) {
		floor, set := numberAt(specAt(budget, specField), "minAvailable")
		if !set || floor < subject.Replicas {
			continue
		}
		return budget.GetName() + " keeps " + strconv.FormatInt(floor, 10) +
			" of " + strconv.FormatInt(subject.Replicas, 10) + " replicas available, so nothing may be evicted", ""
	}
	return "", ""
}

func scalerFightsReplicas(subject Subject, held *corpus) (string, string) {
	if !isSpreadable(subject.Kind) {
		return "", ""
	}
	if _, set := numberAt(specAt(subject.Object, specField), "replicas"); !set {
		return "", ""
	}
	for _, scaler := range held.inNamespace("autoscaling", "horizontalpodautoscalers", subject.Ref.Namespace) {
		targetRef := specAt(scaler, specField, "scaleTargetRef")
		if stringAt(targetRef, "kind") != subject.Kind || stringAt(targetRef, "name") != subject.Ref.Name {
			continue
		}
		return scaler.GetName() + " scales this workload and spec.replicas is set as well", ""
	}
	return "", ""
}

func overCorpus(rule func(scan) []found) finder {
	return rule
}

func unmountedClaims(sc scan) []found {
	mounted := map[string]bool{}
	generated := make([]string, 0, len(sc.subjects))
	for _, subject := range sc.subjects {
		for _, name := range claimNames(subject) {
			mounted[subject.Ref.Namespace+"/"+name] = true
		}
		generated = append(generated, claimTemplatePrefixes(subject)...)
	}
	out := []found{}
	for _, ref := range sc.held.every("persistentvolumeclaims") {
		if mounted[ref.Namespace+"/"+ref.Name] || fromClaimTemplate(ref.Namespace+"/"+ref.Name, generated) {
			continue
		}
		out = append(out, found{
			subject: Subject{Ref: ref, Kind: "PersistentVolumeClaim", Object: emptyObject(ref, "PersistentVolumeClaim")},
			detail:  "no pod in this cluster mounts this claim",
		})
	}
	return out
}

func claimTemplatePrefixes(subject Subject) []string {
	if subject.Kind != statefulSetKind {
		return nil
	}
	out := []string{}
	for _, raw := range unstr.Slice(subject.Object, specField, "volumeClaimTemplates") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		meta, hasMeta := entry["metadata"].(map[string]any)
		if !hasMeta {
			continue
		}
		name := stringAt(meta, "name")
		if name == "" {
			continue
		}
		out = append(out, subject.Ref.Namespace+"/"+name+"-"+subject.Ref.Name+"-")
	}
	return out
}

func fromClaimTemplate(key string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		if ordinal := key[len(prefix):]; ordinal != "" && isDigits(ordinal) {
			return true
		}
	}
	return false
}

func isDigits(text string) bool {
	for _, one := range text {
		if one < '0' || one > '9' {
			return false
		}
	}
	return true
}

func emptyObject(ref api.ObjectRef, kind string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       kind,
		"metadata":   map[string]any{"name": ref.Name, "namespace": ref.Namespace},
	}}
}

func networkChecks() []check {
	return []check{
		{
			id:       "ingress-backend-missing",
			title:    "Ingress routes to a Service that does not exist",
			category: categoryReliability,
			severity: severityHigh,
			needs:    []target{ingressTarget, serviceTarget},
			wrong:    "The controller has nowhere to send the traffic, so the route answers 503 with nothing in the logs to explain it.",
			remedy:   "Create the Service, or point the backend at one that is there.",
			find:     overCorpus(ingressBackendMissing),
		},
		{
			id:       "ingress-class-missing",
			title:    "Ingress names an IngressClass that does not exist",
			category: categoryReliability,
			severity: severityHigh,
			needs:    []target{ingressTarget, classTarget},
			wrong:    "No controller claims the Ingress, so it is never programmed and the host simply does not resolve to anything.",
			remedy:   "Create the IngressClass, or name one the cluster has.",
			find:     overCorpus(ingressClassMissing),
		},
		{
			id:       "storage-class-missing",
			title:    "Claim names a StorageClass that does not exist",
			category: categoryReliability,
			severity: severityHigh,
			needs:    []target{claimTarget, storageTarget},
			wrong:    "Nothing provisions the volume, so the claim stays Pending and every pod that wants it stays Pending too.",
			remedy:   "Create the StorageClass, or name one the cluster has.",
			find:     overCorpus(storageClassMissing),
		},
	}
}

func corpusFinding(obj *unstructured.Unstructured, key target, kind, detail string) found {
	ref := api.ObjectRef{
		Group:     key.group,
		Version:   "v1",
		Resource:  key.resource,
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
	origin, managedBy := originOf(obj)
	return found{
		subject: Subject{Ref: ref, Kind: kind, Object: obj, Origin: origin, ManagedBy: managedBy},
		detail:  detail,
	}
}

func ingressServices(obj *unstructured.Unstructured) []string {
	out := []string{}
	if name := stringAt(specAt(obj, specField, "defaultBackend", "service"), "name"); name != "" {
		out = append(out, name)
	}
	for _, raw := range unstr.Slice(obj, specField, "rules") {
		rule, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, ruleServices(rule)...)
	}
	return out
}

func ruleServices(rule map[string]any) []string {
	http, isHTTP := rule["http"].(map[string]any)
	if !isHTTP {
		return nil
	}
	paths, isList := http["paths"].([]any)
	if !isList {
		return nil
	}
	out := make([]string, 0, len(paths))
	for _, entry := range paths {
		path, isMap := entry.(map[string]any)
		if !isMap {
			continue
		}
		if name := backendService(path); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func backendService(path map[string]any) string {
	backend, hasBackend := path["backend"].(map[string]any)
	if !hasBackend {
		return ""
	}
	service, hasService := backend["service"].(map[string]any)
	if !hasService {
		return ""
	}
	return stringAt(service, "name")
}

func ingressBackendMissing(sc scan) []found {
	out := []found{}
	for _, obj := range sc.held.of(networkGroup, "ingresses") {
		for _, name := range ingressServices(obj) {
			if sc.held.named("", "services", obj.GetNamespace(), name) {
				continue
			}
			out = append(out, corpusFinding(obj, ingressTarget, "Ingress",
				"routes to the Service "+name+", which this namespace does not have"))
			break
		}
	}
	return out
}

func ingressClassMissing(sc scan) []found {
	out := []found{}
	for _, obj := range sc.held.of(networkGroup, "ingresses") {
		name := stringAt(specAt(obj, specField), "ingressClassName")
		if name == "" || sc.held.named(networkGroup, "ingressclasses", "", name) {
			continue
		}
		out = append(out, corpusFinding(obj, ingressTarget, "Ingress",
			"names the IngressClass "+name+", which the cluster does not have"))
	}
	return out
}

func storageClassMissing(sc scan) []found {
	out := []found{}
	for _, obj := range sc.held.of("", "persistentvolumeclaims") {
		name := stringAt(specAt(obj, specField), "storageClassName")
		if name == "" || sc.held.named("storage.k8s.io", "storageclasses", "", name) {
			continue
		}
		out = append(out, corpusFinding(obj, claimTarget, "PersistentVolumeClaim",
			"names the StorageClass "+name+", which the cluster does not have"))
	}
	return out
}
