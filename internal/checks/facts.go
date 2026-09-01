package checks

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const (
	hostnameKey     = "kubernetes.io/hostname"
	enforceLabel    = "pod-security.kubernetes.io/enforce"
	profileBaseline = "baseline"
	profileStrict   = "restricted"
	containerLimits = "Container"
	doNotSchedule   = "DoNotSchedule"
)

var nodeTarget = target{group: "", resource: "nodes"}

var namespaceTarget = target{group: "", resource: "namespaces"}

var quotaTarget = target{group: "", resource: "resourcequotas"}

var limitRangeTarget = target{group: "", resource: "limitranges"}

func factChecks() []check {
	return []check{
		{
			id:       "node-selector-matches-nothing",
			title:    "Scheduled onto a node that does not exist",
			category: categoryReliability,
			severity: severityHigh,
			needs:    []target{nodeTarget},
			wrong:    "No node in the cluster carries the labels the pod asks for, so it can never be scheduled.",
			remedy:   "Fix the label in nodeSelector, or label the nodes it is meant to land on.",
			find:     overFacts(nodeSelectorMatchesNothing),
		},
		{
			id:       "tolerations-miss-the-taints",
			title:    "Every matching node repels the pod",
			category: categoryReliability,
			severity: severityHigh,
			needs:    []target{nodeTarget},
			wrong:    "The nodes whose labels match all carry a taint the pod does not tolerate, so nothing will accept it.",
			remedy:   "Add the toleration the taint calls for, or take the taint off a node that should run this.",
			find:     overFacts(tolerationsMissTaints),
		},
		{
			id:       "request-exceeds-largest-node",
			title:    "Asks for more than any node has",
			category: categoryReliability,
			severity: severityHigh,
			needs:    []target{nodeTarget},
			wrong:    "The request is larger than the allocatable capacity of the biggest node, so no amount of free space will let it schedule.",
			remedy:   "Lower the request, or add a node large enough to hold it.",
			find:     overFacts(requestExceedsLargestNode),
		},
		{
			id:       "spread-needs-more-domains",
			title:    "Spread asks for more failure domains than exist",
			category: categoryReliability,
			severity: severityMedium,
			needs:    []target{nodeTarget},
			wrong:    "A DoNotSchedule constraint cannot be satisfied with fewer domains than replicas, so the surplus replicas stay Pending.",
			remedy:   "Use ScheduleAnyway, lower the replica count, or add nodes in more domains.",
			find:     overFacts(spreadNeedsMoreDomains),
		},
		{
			id:       "anti-affinity-exceeds-nodes",
			title:    "Anti-affinity asks for more nodes than exist",
			category: categoryReliability,
			severity: severityMedium,
			needs:    []target{nodeTarget},
			wrong:    "A required anti-affinity on hostname allows one replica per node, and there are fewer nodes than replicas.",
			remedy:   "Make the rule preferred, lower the replica count, or add nodes.",
			find:     overFacts(antiAffinityExceedsNodes),
		},
		{
			id:       "outside-limit-range",
			title:    "Resources outside the namespace's LimitRange",
			category: categoryReliability,
			severity: severityHigh,
			needs:    []target{limitRangeTarget},
			wrong:    "The namespace's LimitRange rejects the pod at admission, so the controller creates it and never gets one.",
			remedy:   "Bring the request and limit inside the range, or widen the LimitRange.",
			find:     overFacts(outsideLimitRange),
		},
		{
			id:       "quota-nearly-exhausted",
			title:    "Namespace quota almost spent",
			category: categoryReliability,
			severity: severityMedium,
			needs:    []target{quotaTarget},
			wrong:    "Once a ResourceQuota is full the next pod is refused at admission, which shows up as a controller that cannot create its pods.",
			remedy:   "Raise the quota, or lower what the workloads in this namespace reserve.",
			find:     overFacts(quotaNearlyExhausted),
		},
		{
			id:         "pod-security-would-reject",
			title:      "The namespace's Pod Security level would reject this",
			category:   categorySecurity,
			severity:   severityHigh,
			frameworks: []string{pssBaseline, pssRestricted},
			needs:      []target{namespaceTarget},
			wrong:      "The namespace enforces a Pod Security level this pod does not meet, so the pods are refused at admission.",
			remedy:     "Fix what the level forbids, or move the workload to a namespace at a lower level.",
			find:       overFacts(podSecurityWouldReject),
		},
	}
}

func nodeLabels(node *unstructured.Unstructured) map[string]string {
	return node.GetLabels()
}

func selectorOf(subject Subject) map[string]string {
	raw, ok := subject.Pod["nodeSelector"].(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]string{}
	for key, value := range raw {
		text, isString := value.(string)
		if !isString {
			continue
		}
		out[key] = text
	}
	return out
}

func nodesMatching(subject Subject, held *corpus) []*unstructured.Unstructured {
	wanted := selectorOf(subject)
	out := []*unstructured.Unstructured{}
	for _, node := range held.of("", "nodes") {
		labels := nodeLabels(node)
		fits := true
		for key, value := range wanted {
			if labels[key] != value {
				fits = false
				break
			}
		}
		if fits {
			out = append(out, node)
		}
	}
	return out
}

func nodeSelectorMatchesNothing(subject Subject, held *corpus) (string, string) {
	wanted := selectorOf(subject)
	if len(wanted) == 0 || len(held.of("", "nodes")) == 0 {
		return "", ""
	}
	if len(nodesMatching(subject, held)) > 0 {
		return "", ""
	}
	pairs := make([]string, 0, len(wanted))
	for key, value := range wanted {
		pairs = append(pairs, key+"="+value)
	}
	slices.Sort(pairs)
	return "no node carries " + strings.Join(pairs, ", "), ""
}

type taint struct {
	key    string
	value  string
	effect string
}

func taintsOf(node *unstructured.Unstructured) []taint {
	out := []taint{}
	for _, raw := range unstr.Slice(node, specField, "taints") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		effect := stringAt(entry, "effect")
		if effect == "PreferNoSchedule" {
			continue
		}
		out = append(out, taint{
			key:    stringAt(entry, "key"),
			value:  stringAt(entry, "value"),
			effect: effect,
		})
	}
	return out
}

func tolerates(subject Subject, one taint) bool {
	listed, ok := subject.Pod["tolerations"].([]any)
	if !ok {
		return false
	}
	for _, raw := range listed {
		entry, isMap := raw.(map[string]any)
		if !isMap {
			continue
		}
		if !tolerationCovers(entry, one) {
			continue
		}
		return true
	}
	return false
}

func tolerationCovers(entry map[string]any, one taint) bool {
	effect := stringAt(entry, "effect")
	if effect != "" && effect != one.effect {
		return false
	}
	operator := stringAt(entry, "operator")
	key := stringAt(entry, "key")
	if operator == "Exists" {
		return key == "" || key == one.key
	}
	if key != one.key {
		return false
	}
	return stringAt(entry, "value") == one.value
}

func tolerationsMissTaints(subject Subject, held *corpus) (string, string) {
	nodes := nodesMatching(subject, held)
	if len(nodes) == 0 {
		return "", ""
	}
	blocking := ""
	for _, node := range nodes {
		repelled := false
		for _, one := range taintsOf(node) {
			if tolerates(subject, one) {
				continue
			}
			repelled = true
			blocking = one.key
			break
		}
		if !repelled {
			return "", ""
		}
	}
	return "every node it could use is tainted " + blocking + ", which it does not tolerate", ""
}

func allocatableOf(node *unstructured.Unstructured, name string) (resource.Quantity, bool) {
	raw, found, err := unstructured.NestedFieldNoCopy(node.Object, "status", "allocatable", name)
	if !found || err != nil {
		return resource.Quantity{}, false
	}
	return quantityFrom(raw)
}

func largestNode(held *corpus, name string, scale func(resource.Quantity) int64) int64 {
	var biggest int64
	for _, node := range held.of("", "nodes") {
		value, ok := allocatableOf(node, name)
		if !ok {
			continue
		}
		if scaled := scale(value); scaled > biggest {
			biggest = scaled
		}
	}
	return biggest
}

func requestExceedsLargestNode(subject Subject, held *corpus) (string, string) {
	for _, entry := range []struct {
		name  string
		unit  string
		scale func(resource.Quantity) int64
	}{
		{name: cpuName, unit: "m", scale: milliOf},
		{name: memoryName, unit: " bytes", scale: plainOf},
	} {
		biggest := largestNode(held, entry.name, entry.scale)
		if biggest == 0 {
			continue
		}
		for _, container := range subject.Containers {
			asked, ok := quantityAt(container.Spec, requests, entry.name)
			if !ok {
				continue
			}
			if entry.scale(asked) <= biggest {
				continue
			}
			return "container " + container.Name + " requests " + asked.String() + " " + entry.name +
				" and the largest node has " + strconv.FormatInt(biggest, 10) + entry.unit, ""
		}
	}
	return "", ""
}

func spreadConstraints(subject Subject) []map[string]any {
	out := []map[string]any{}
	listed, ok := subject.Pod["topologySpreadConstraints"].([]any)
	if !ok {
		return out
	}
	for _, raw := range listed {
		entry, isMap := raw.(map[string]any)
		if !isMap {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func domainsOf(held *corpus, key string) int {
	seen := map[string]bool{}
	for _, node := range held.of("", "nodes") {
		value, ok := nodeLabels(node)[key]
		if !ok {
			continue
		}
		seen[value] = true
	}
	return len(seen)
}

func spreadNeedsMoreDomains(subject Subject, held *corpus) (string, string) {
	if subject.Replicas < 2 || len(held.of("", "nodes")) == 0 {
		return "", ""
	}
	for _, entry := range spreadConstraints(subject) {
		if stringAt(entry, "whenUnsatisfiable") != doNotSchedule {
			continue
		}
		key := stringAt(entry, "topologyKey")
		if key == "" {
			continue
		}
		skew, hasSkew := numberAt(entry, "maxSkew")
		if !hasSkew {
			skew = 1
		}
		domains := int64(domainsOf(held, key))
		if domains == 0 || subject.Replicas <= domains*skew {
			continue
		}
		return strconv.FormatInt(subject.Replicas, 10) + " replicas spread over " + key +
			", and the cluster has " + strconv.FormatInt(domains, 10) + " of those", ""
	}
	return "", ""
}

func requiredAntiAffinity(subject Subject) map[string]any {
	affinity, ok := subject.Pod["affinity"].(map[string]any)
	if !ok {
		return nil
	}
	anti, ok := affinity["podAntiAffinity"].(map[string]any)
	if !ok {
		return nil
	}
	listed, ok := anti["requiredDuringSchedulingIgnoredDuringExecution"].([]any)
	if !ok || len(listed) == 0 {
		return nil
	}
	entry, isMap := listed[0].(map[string]any)
	if !isMap {
		return nil
	}
	return entry
}

func antiAffinityExceedsNodes(subject Subject, held *corpus) (string, string) {
	rule := requiredAntiAffinity(subject)
	if rule == nil || subject.Replicas < 2 {
		return "", ""
	}
	if stringAt(rule, "topologyKey") != hostnameKey {
		return "", ""
	}
	nodes := int64(len(nodesMatching(subject, held)))
	if nodes == 0 || subject.Replicas <= nodes {
		return "", ""
	}
	return strconv.FormatInt(subject.Replicas, 10) + " replicas, one node each, and " +
		strconv.FormatInt(nodes, 10) + " nodes to put them on", ""
}

func containerRanges(held *corpus, namespace string) []map[string]any {
	out := []map[string]any{}
	for _, obj := range held.inNamespace("", "limitranges", namespace) {
		for _, raw := range unstr.Slice(obj, specField, "limits") {
			entry, ok := raw.(map[string]any)
			if !ok || stringAt(entry, "type") != containerLimits {
				continue
			}
			out = append(out, entry)
		}
	}
	return out
}

func outsideLimitRange(subject Subject, held *corpus) (string, string) {
	for _, entry := range containerRanges(held, subject.Ref.Namespace) {
		for _, container := range subject.Containers {
			if detail := breaksRange(container, entry); detail != "" {
				return "container " + container.Name + " " + detail, ""
			}
		}
	}
	return "", ""
}

func breaksRange(container Container, entry map[string]any) string {
	for _, name := range []string{cpuName, memoryName} {
		asked, ok := quantityAt(container.Spec, requests, name)
		if !ok {
			continue
		}
		if floor, has := boundOf(entry, "min", name); has && asked.Cmp(floor) < 0 {
			return "requests " + asked.String() + " " + name + ", below the LimitRange minimum " + floor.String()
		}
		if ceiling, has := boundOf(entry, "max", name); has && asked.Cmp(ceiling) > 0 {
			return "requests " + asked.String() + " " + name + ", above the LimitRange maximum " + ceiling.String()
		}
	}
	return ""
}

func boundOf(entry map[string]any, section, name string) (resource.Quantity, bool) {
	part, ok := entry[section].(map[string]any)
	if !ok {
		return resource.Quantity{}, false
	}
	return quantityFrom(part[name])
}

func quotaNearlyExhausted(subject Subject, held *corpus) (string, string) {
	const nearly = 90
	for _, quota := range held.inNamespace("", "resourcequotas", subject.Ref.Namespace) {
		hard, okHard, hardErr := unstructured.NestedFieldNoCopy(quota.Object, "status", "hard")
		used, okUsed, usedErr := unstructured.NestedFieldNoCopy(quota.Object, "status", "used")
		if !okHard || !okUsed || hardErr != nil || usedErr != nil {
			continue
		}
		name, share := fullestEntry(hard, used)
		if share < nearly {
			continue
		}
		return quota.GetName() + " is " + strconv.FormatInt(share, 10) + "% spent on " + name, ""
	}
	return "", ""
}

func fullestEntry(hard, used any) (name string, share int64) {
	hardMap, okHard := hard.(map[string]any)
	usedMap, okUsed := used.(map[string]any)
	if !okHard || !okUsed {
		return "", 0
	}
	for _, key := range slices.Sorted(maps.Keys(hardMap)) {
		raw := hardMap[key]
		ceiling, ok := quantityFrom(raw)
		if !ok || ceiling.Value() == 0 {
			continue
		}
		spent, ok := quantityFrom(usedMap[key])
		if !ok {
			continue
		}
		percent := spent.MilliValue() * 100 / ceiling.MilliValue()
		if percent > share {
			name, share = key, percent
		}
	}
	return name, share
}

func podSecurityWouldReject(subject Subject, held *corpus) (string, string) {
	space := held.namespace(subject.Ref.Namespace)
	if space == nil {
		return "", ""
	}
	level := space.GetLabels()[enforceLabel]
	if level != profileBaseline && level != profileStrict {
		return "", ""
	}
	broken := brokenControls(subject, level)
	if len(broken) == 0 {
		return "", ""
	}
	return "the namespace enforces " + level + " and this breaks " + strings.Join(broken, ", "), ""
}

func brokenControls(subject Subject, level string) []string {
	out := []string{}
	if detail, _ := hostNamespaces(subject); detail != "" {
		out = append(out, "host namespaces")
	}
	for _, container := range subject.Containers {
		if detail, _ := privileged(subject, container); detail != "" {
			out = append(out, "privileged")
			break
		}
	}
	if _, path := firstHostPath(subject, false); path != "" {
		out = append(out, "hostPath volumes")
	}
	if level != profileStrict {
		return slices.Compact(out)
	}
	for _, container := range subject.Containers {
		if detail, _ := escalation(subject, container); detail != "" {
			out = append(out, "privilege escalation")
			break
		}
	}
	for _, container := range subject.Containers {
		if detail, _ := capabilitiesNotDropped(subject, container); detail != "" {
			out = append(out, "capabilities not dropped")
			break
		}
	}
	return slices.Compact(out)
}
