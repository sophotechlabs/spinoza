package issues

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const crashLoopPodJSON = `{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
    "name": "web-7d9f6c5b4d-hkq2p",
    "namespace": "shop",
    "uid": "3f2a1b0c-1111-2222-3333-444455556666",
    "creationTimestamp": "2026-08-18T09:14:02Z",
    "ownerReferences": [
      {
        "apiVersion": "apps/v1",
        "kind": "ReplicaSet",
        "name": "web-7d9f6c5b4d",
        "uid": "aaaabbbb-cccc-dddd-eeee-ffff00001111",
        "controller": true,
        "blockOwnerDeletion": true
      }
    ]
  },
  "spec": { "nodeName": "node-2" },
  "status": {
    "phase": "Running",
    "startTime": "2026-08-18T09:14:02Z",
    "conditions": [
      { "type": "PodScheduled", "status": "True", "lastTransitionTime": "2026-08-18T09:14:02Z" },
      { "type": "Ready", "status": "False", "lastTransitionTime": "2026-08-28T11:58:30Z" }
    ],
    "containerStatuses": [
      {
        "name": "app",
        "ready": false,
        "restartCount": 137,
        "started": false,
        "state": {
          "waiting": { "reason": "CrashLoopBackOff", "message": "back-off 5m0s restarting failed container" }
        },
        "lastState": {
          "terminated": {
            "exitCode": 137,
            "reason": "OOMKilled",
            "startedAt": "2026-08-28T11:58:00Z",
            "finishedAt": "2026-08-28T11:58:30Z"
          }
        }
      }
    ]
  }
}`

const replicaSetJSON = `{
  "apiVersion": "apps/v1",
  "kind": "ReplicaSet",
  "metadata": {
    "name": "web-7d9f6c5b4d",
    "namespace": "shop",
    "uid": "aaaabbbb-cccc-dddd-eeee-ffff00001111",
    "annotations": { "deployment.kubernetes.io/revision": "12" },
    "creationTimestamp": "2026-08-18T09:14:00Z"
  },
  "spec": { "replicas": 1 },
  "status": { "replicas": 1, "readyReplicas": 0 }
}`

func decoded(t *testing.T, doc string) *unstructured.Unstructured {
	t.Helper()
	obj := &unstructured.Unstructured{}
	if err := obj.UnmarshalJSON([]byte(doc)); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return obj
}

func TestAPodDecodedTheWayTheInformerDecodesItReadsRight(t *testing.T) {
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{
		"pods":        {decoded(t, crashLoopPodJSON)},
		"replicasets": {decoded(t, replicaSetJSON)},
	}}

	queue := build(t, lister, catalog(podDescriptor(), replicaSetDescriptor()))

	if len(queue.Rows) != 1 {
		t.Fatalf("rows = %+v, want the crashlooping pod under its replica set", queue.Rows)
	}
	row := queue.Rows[0]
	if row.Severity != api.SeverityFatal || row.Title != "CrashLoopBackOff" {
		t.Fatalf("row = %+v, want a fatal crashloop", row)
	}
	if !contains(row.Detail, "exit code 137") {
		t.Fatalf("detail = %q, want the exit code off a real status", row.Detail)
	}
	if !contains(row.Detail, "restarted 137 times") {
		t.Fatalf("detail = %q, want the restart count off a real status", row.Detail)
	}
	if !contains(row.Detail, "(OOMKilled)") {
		t.Fatalf("detail = %q, want the termination reason", row.Detail)
	}
	if row.Change != "revision 12" {
		t.Fatalf("change = %q, want the revision off a real annotation", row.Change)
	}
	if row.Since != "2026-08-28T11:58:30Z" {
		t.Fatalf("since = %q, want the moment it last died", row.Since)
	}
}
