package rollout

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func diffWorld() []runtime.Object {
	return append(deploymentWorld(), replicaSet("web-6", "6", "nginx:5.0", "web"))
}

func TestADiffBetweenTwoRevisionsWithTheSameTemplateSaysTheyMatch(t *testing.T) {
	diff, err := Diff(t.Context(), FromDynamic(clusterOf(diffWorld()...)), deploymentRef(), 5, 6)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}

	if !diff.Same {
		t.Fatalf("same = false, want two revisions with the same pod template to match")
	}
	if diff.Lines != 0 {
		t.Fatalf("lines = %d, want none", diff.Lines)
	}
	if diff.Left == "" || diff.Left != diff.Right {
		t.Fatalf("left = %q right = %q, want both sides rendered and equal", diff.Left, diff.Right)
	}
}

func TestADiffCountsTheChangedImageOnBothSides(t *testing.T) {
	diff, err := Diff(t.Context(), FromDynamic(clusterOf(deploymentWorld()...)), deploymentRef(), 4, 5)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}

	if diff.Same {
		t.Fatal("same = true, want a changed image to differ")
	}
	if diff.Lines != 2 {
		t.Fatalf("lines = %d, want the image line on each side", diff.Lines)
	}
	if diff.From != 4 || diff.To != 5 {
		t.Fatalf("from = %d to = %d, want the revisions that were asked for", diff.From, diff.To)
	}
	if !strings.Contains(diff.Left, "nginx:4.0") {
		t.Fatalf("left = %q, want the older image", diff.Left)
	}
	if !strings.Contains(diff.Right, "nginx:5.0") {
		t.Fatalf("right = %q, want the newer image", diff.Right)
	}
}

func TestADiffLeavesOutTheServerAssignedFields(t *testing.T) {
	diff, err := Diff(t.Context(), FromDynamic(clusterOf(deploymentWorld()...)), deploymentRef(), 4, 5)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}

	for _, field := range []string{"creationTimestamp", "managedFields", "resourceVersion", "uid", "status"} {
		if strings.Contains(diff.Left, field) {
			t.Fatalf("left = %q, want %s stripped", diff.Left, field)
		}
	}
	if strings.Contains(diff.Left, templateHashLabel) {
		t.Fatalf("left = %q, want the pod template hash left out", diff.Left)
	}
}

func TestADiffNamesARevisionItCannotFind(t *testing.T) {
	cases := []struct {
		name string
		from int64
		to   int64
		want string
	}{
		{name: "the left side is missing", from: 42, to: 5, want: "web has no revision 42"},
		{name: "the right side is missing", from: 5, to: 43, want: "web has no revision 43"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Diff(t.Context(), FromDynamic(clusterOf(deploymentWorld()...)), deploymentRef(), tc.from, tc.to)

			if err == nil {
				t.Fatal("expected a missing revision to be refused")
			}
			if err.Error() != tc.want {
				t.Fatalf("error = %q, want %q", err.Error(), tc.want)
			}
		})
	}
}

func TestADiffOfAKindWithNoHistorySaysSo(t *testing.T) {
	ref := api.ObjectRef{Version: "v1", Resource: "configmaps", Namespace: "shop", Name: "settings"}

	_, err := Diff(t.Context(), FromDynamic(clusterOf()), ref, 1, 2)

	if err == nil || err.Error() != "configmaps keep no rollout revisions" {
		t.Fatalf("error = %v, want the sentence the list gives", err)
	}
}

func TestADiffOfAWorkloadThatIsNotThereIsReported(t *testing.T) {
	_, err := Diff(t.Context(), FromDynamic(clusterOf()), deploymentRef(), 1, 2)

	if err == nil {
		t.Fatal("expected a missing deployment to fail")
	}
}
