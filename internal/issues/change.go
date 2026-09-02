package issues

import (
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const revisionAnnotation = "deployment.kubernetes.io/revision"

type change struct {
	what string
	at   time.Time
}

func rolloutOf(snap *snapshot, item object) change {
	owner, ok := snap.controllerOf(item)
	if !ok {
		return change{}
	}
	return changeOf(snap, owner)
}

func changeOf(snap *snapshot, item object) change {
	switch item.desc.Kind {
	case kindReplicaSet:
		return replicaSetChange(snap, item)
	case kindDeployment:
		return deploymentChange(snap, item)
	case kindStatefulSet:
		return revisionChange(item.obj, unstr.String(item.obj, "status", "updateRevision"))
	case kindDaemonSet:
		return generationChange(item.obj)
	case kindJob:
		return change{what: "job created", at: item.obj.GetCreationTimestamp().Time}
	}
	return change{}
}

func replicaSetChange(snap *snapshot, item object) change {
	revision := item.obj.GetAnnotations()[revisionAnnotation]
	at := item.obj.GetCreationTimestamp().Time
	if revision != "" {
		return change{what: "revision " + revision, at: at}
	}
	owner, ok := snap.controllerOf(item)
	if !ok {
		return change{what: "replica set created", at: at}
	}
	return change{what: revisionText(owner.obj), at: at}
}

func deploymentChange(snap *snapshot, item object) change {
	newest := newestReplicaSet(snap, item)
	if newest.obj == nil {
		return generationChange(item.obj)
	}
	return replicaSetChange(snap, newest)
}

func newestReplicaSet(snap *snapshot, owner object) object {
	best := object{}
	for _, candidate := range snap.children(owner.uid()) {
		if candidate.desc.Kind != kindReplicaSet {
			continue
		}
		if best.obj == nil || candidate.obj.GetCreationTimestamp().After(best.obj.GetCreationTimestamp().Time) {
			best = candidate
		}
	}
	return best
}

func revisionText(obj *unstructured.Unstructured) string {
	revision := obj.GetAnnotations()[revisionAnnotation]
	if revision == "" {
		return "rolled out"
	}
	return "revision " + revision
}

func revisionChange(obj *unstructured.Unstructured, revision string) change {
	if revision == "" {
		return generationChange(obj)
	}
	return change{what: "revision " + shortened(revision), at: transitionOf(obj)}
}

func generationChange(obj *unstructured.Unstructured) change {
	generation := obj.GetGeneration()
	if generation == 0 {
		return change{}
	}
	return change{what: "generation " + strconv.FormatInt(generation, 10), at: transitionOf(obj)}
}

func transitionOf(obj *unstructured.Unstructured) time.Time {
	newest := newestCondition(obj)
	if newest.IsZero() {
		return obj.GetCreationTimestamp().Time
	}
	return newest
}

func newestCondition(obj *unstructured.Unstructured) time.Time {
	newest := time.Time{}
	for _, raw := range unstr.Slice(obj, "status", "conditions") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		at, err := time.Parse(time.RFC3339, unstr.At(entry, "lastTransitionTime"))
		if err != nil {
			continue
		}
		if at.After(newest) {
			newest = at
		}
	}
	return newest
}

func shortened(revision string) string {
	const shortSHA = 7
	trimmed := revision
	if cut := strings.LastIndexAny(revision, "@:"); cut >= 0 {
		trimmed = revision[cut+1:]
	}
	runes := []rune(trimmed)
	if len(runes) > shortSHA*2 {
		return string(runes[:shortSHA])
	}
	return trimmed
}
