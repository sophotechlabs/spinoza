package issues

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const detectorCondition = "condition"

var healthyWhenTrue = []string{"Ready", "Available", "Healthy", "Synced"}

var brokenWhenTrue = []string{"Stalled", "Degraded", "Failed"}

var ownedKinds = map[kindRef]bool{
	{group: "", kind: kindPod}:                      true,
	{group: "", kind: kindReplicationController}:    true,
	{group: appsGroup, kind: kindDeployment}:        true,
	{group: appsGroup, kind: kindReplicaSet}:        true,
	{group: appsGroup, kind: kindStatefulSet}:       true,
	{group: appsGroup, kind: kindDaemonSet}:         true,
	{group: batchGroup, kind: kindJob}:              true,
	{group: batchGroup, kind: kindCronJob}:          true,
	{group: autoscalingGroup, kind: autoscalerKind}: true,
	{group: argoGroup, kind: applicationsKind}:      true,
}

func conditionFindings(snap *snapshot) []finding {
	out := []finding{}
	for _, items := range snap.byKind {
		for _, item := range items {
			found, ok := conditionFinding(item)
			if !ok {
				continue
			}
			out = append(out, found)
		}
	}
	return out
}

func conditionFinding(item object) (finding, bool) {
	if ownedKinds[kindRef{group: item.desc.Group, kind: item.desc.Kind}] || isFluxGroup(item.desc.Group) {
		return finding{}, false
	}
	if item.obj.GetDeletionTimestamp() != nil {
		return finding{}, false
	}
	entry, ok := badCondition(item.obj)
	if !ok {
		return finding{}, false
	}
	name := unstr.At(entry, "type")
	return finding{
		detector:  detectorCondition,
		severity:  severityDegraded,
		title:     reasonOr(entry, name+"="+unstr.At(entry, "status")),
		detail:    messageOr(entry, item.desc.Kind+" reports "+name+"="+unstr.At(entry, "status")),
		action:    "open the object and read its conditions",
		uncertain: unstr.At(entry, "status") == "Unknown",
		kind:      item.desc.Kind,
		subject:   item,
		since:     conditionSince(item.obj, name),
	}, true
}

func badCondition(obj *unstructured.Unstructured) (map[string]any, bool) {
	for _, name := range healthyWhenTrue {
		entry, ok := conditionOf(obj, name)
		if !ok {
			continue
		}
		if unstr.At(entry, "status") == conditionTrue {
			return nil, false
		}
		return entry, true
	}
	for _, name := range brokenWhenTrue {
		entry, ok := conditionOf(obj, name)
		if !ok {
			continue
		}
		if unstr.At(entry, "status") != conditionTrue {
			continue
		}
		return entry, true
	}
	return nil, false
}
