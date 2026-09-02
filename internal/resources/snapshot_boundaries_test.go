package resources

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestSnapshotSkipsObjectsThatDoNotBelongInTheInformerCache(t *testing.T) {
	shown := builtinLayout("Deployment")
	st := &stream{
		kind:    "Deployment",
		lister:  stubCacheLister{objects: []runtime.Object{newDeployment("default", "web"), &corev1.Pod{}}},
		columns: shown.columns,
		cells:   shown.cells,
	}

	rows, total, err := st.snapshot("", 0, nil, everything())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("rows = %d of %d, want only the deployment", len(rows), total)
	}
	if rows[0].Name != "web" {
		t.Fatalf("row = %+v, want the deployment", rows[0])
	}
}
