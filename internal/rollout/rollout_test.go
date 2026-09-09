package rollout

import (
	"errors"
	"slices"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

var (
	deploymentGVR         = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	replicaSetGVR         = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: replicaSets}
	statefulSetGVR        = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
	daemonSetGVR          = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}
	controllerRevisionGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: controllerRevisions}
)

func clusterOf(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
	kinds := map[schema.GroupVersionResource]string{
		deploymentGVR:         "DeploymentList",
		replicaSetGVR:         "ReplicaSetList",
		statefulSetGVR:        "StatefulSetList",
		daemonSetGVR:          "DaemonSetList",
		controllerRevisionGVR: "ControllerRevisionList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, objs...)
}

func podTemplate(image, hash string) map[string]any {
	labels := map[string]any{"app": "web"}
	if hash != "" {
		labels[templateHashLabel] = hash
	}
	return map[string]any{
		"metadata": map[string]any{"labels": labels},
		specField: map[string]any{
			"containers": []any{
				map[string]any{"name": "app", "image": image},
			},
		},
	}
}

func workload(kind, name, image string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       kind,
		"metadata": map[string]any{
			"name":      name,
			"namespace": "shop",
		},
		specField: map[string]any{
			"replicas":    int64(3),
			templateField: podTemplate(image, ""),
		},
	}}
}

func replicaSet(name, revision, image, owner string) *unstructured.Unstructured {
	annotations := map[string]any{"kubernetes.io/change-cause": "kubectl set image"}
	if revision != "" {
		annotations[revisionAnnotation] = revision
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "ReplicaSet",
		"metadata": map[string]any{
			"name":              name,
			"namespace":         "shop",
			"annotations":       annotations,
			"creationTimestamp": "2026-07-31T12:00:00Z",
			"ownerReferences": []any{
				map[string]any{"apiVersion": "apps/v1", "kind": deploymentKind, "name": owner},
			},
		},
		specField: map[string]any{
			"replicas":    int64(3),
			templateField: podTemplate(image, name),
		},
		"status": map[string]any{"readyReplicas": int64(2)},
	}}
}

func controllerRevision(name string, number int64, image, kind, owner string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "ControllerRevision",
		"metadata": map[string]any{
			"name":              name,
			"namespace":         "shop",
			"creationTimestamp": "2026-07-31T12:00:00Z",
			"ownerReferences": []any{
				map[string]any{"apiVersion": "apps/v1", "kind": kind, "name": owner},
			},
		},
		"revision": number,
		"data": map[string]any{
			specField: map[string]any{templateField: podTemplate(image, name)},
		},
	}}
}

func deploymentRef() api.ObjectRef {
	return api.ObjectRef{Group: appsGroup, Version: "v1", Resource: "deployments", Namespace: "shop", Name: "web"}
}

func statefulSetRef() api.ObjectRef {
	return api.ObjectRef{Group: appsGroup, Version: "v1", Resource: "statefulsets", Namespace: "shop", Name: "db"}
}

func daemonSetRef() api.ObjectRef {
	return api.ObjectRef{Group: appsGroup, Version: "v1", Resource: "daemonsets", Namespace: "shop", Name: "agent"}
}

func deploymentWorld() []runtime.Object {
	return []runtime.Object{
		workload(deploymentKind, "web", "nginx:5.0"),
		replicaSet("web-1", "1", "nginx:1.0", "web"),
		replicaSet("web-2", "2", "nginx:2.0", "web"),
		replicaSet("web-3", "3", "nginx:3.0", "web"),
		replicaSet("web-4", "4", "nginx:4.0", "web"),
		replicaSet("web-5", "5", "nginx:5.0", "web"),
		replicaSet("web-elsewhere", "9", "nginx:9.0", "shop-api"),
		replicaSet("web-unnumbered", "", "nginx:0.9", "web"),
	}
}

func statefulSetWorld() []runtime.Object {
	return []runtime.Object{
		workload(statefulSetKind, "db", "postgres:16"),
		controllerRevision("db-1", 1, "postgres:15", statefulSetKind, "db"),
		controllerRevision("db-2", 2, "postgres:16", statefulSetKind, "db"),
		controllerRevision("cache-1", 1, "redis:7", statefulSetKind, "cache"),
	}
}

func daemonSetWorld() []runtime.Object {
	return []runtime.Object{
		workload(daemonSetKind, "agent", "agent:2.0"),
		controllerRevision("agent-1", 1, "agent:1.0", daemonSetKind, "agent"),
		controllerRevision("agent-2", 2, "agent:2.0", daemonSetKind, "agent"),
	}
}

func numbersOf(found api.Revisions) []int64 {
	out := make([]int64, 0, len(found.Revisions))
	for _, one := range found.Revisions {
		out = append(out, one.Number)
	}
	return out
}

func namesOf(found api.Revisions) []string {
	out := make([]string, 0, len(found.Revisions))
	for _, one := range found.Revisions {
		out = append(out, one.Name)
	}
	return out
}

func currentOf(t *testing.T, found api.Revisions) string {
	t.Helper()
	current := ""
	for _, one := range found.Revisions {
		if !one.Current {
			continue
		}
		if current != "" {
			t.Fatalf("%s and %s are both marked current", current, one.Name)
		}
		current = one.Name
	}
	return current
}

func TestTheRevisionListIsNewestFirstAndHoldsOnlyWhatTheWorkloadOwns(t *testing.T) {
	cases := []struct {
		name      string
		objects   []runtime.Object
		ref       api.ObjectRef
		supported bool
		reason    string
		numbers   []int64
		current   string
	}{
		{
			name:      "a deployment orders its own replica sets",
			objects:   deploymentWorld(),
			ref:       deploymentRef(),
			supported: true,
			numbers:   []int64{5, 4, 3, 2, 1},
			current:   "web-5",
		},
		{
			name:      "a stateful set orders its controller revisions",
			objects:   statefulSetWorld(),
			ref:       statefulSetRef(),
			supported: true,
			numbers:   []int64{2, 1},
			current:   "db-2",
		},
		{
			name:      "a daemon set orders its controller revisions",
			objects:   daemonSetWorld(),
			ref:       daemonSetRef(),
			supported: true,
			numbers:   []int64{2, 1},
			current:   "agent-2",
		},
		{
			name:      "a kind with no rollout history says so instead of failing",
			objects:   nil,
			ref:       api.ObjectRef{Version: "v1", Resource: "configmaps", Namespace: "shop", Name: "settings"},
			supported: false,
			reason:    "configmaps keep no rollout revisions",
			numbers:   []int64{},
		},
		{
			name:      "a kind outside the apps group is named with its group",
			objects:   nil,
			ref:       api.ObjectRef{Group: "batch", Version: "v1", Resource: "cronjobs", Namespace: "shop", Name: "nightly"},
			supported: false,
			reason:    "batch/cronjobs keep no rollout revisions",
			numbers:   []int64{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			found, err := List(t.Context(), FromDynamic(clusterOf(tc.objects...)), tc.ref)
			if err != nil {
				t.Fatalf("list revisions: %v", err)
			}

			if found.Supported != tc.supported {
				t.Fatalf("supported = %v, want %v", found.Supported, tc.supported)
			}
			if found.Reason != tc.reason {
				t.Fatalf("reason = %q, want %q", found.Reason, tc.reason)
			}
			if !slices.Equal(numbersOf(found), tc.numbers) {
				t.Fatalf("revisions = %v, want %v", numbersOf(found), tc.numbers)
			}
			if current := currentOf(t, found); current != tc.current {
				t.Fatalf("current = %q, want %q", current, tc.current)
			}
		})
	}
}

func TestAReplicaSetOwnedByAnotherDeploymentIsLeftOut(t *testing.T) {
	found, err := List(t.Context(), FromDynamic(clusterOf(deploymentWorld()...)), deploymentRef())
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}

	names := namesOf(found)
	if slices.Contains(names, "web-elsewhere") {
		t.Fatalf("revisions = %v, want the replica set another deployment owns to be left out", names)
	}
	if slices.Contains(names, "web-unnumbered") {
		t.Fatalf("revisions = %v, want the replica set with no revision annotation to be left out", names)
	}
}

func TestARevisionCarriesItsImagesCauseAndReplicaCounts(t *testing.T) {
	found, err := List(t.Context(), FromDynamic(clusterOf(deploymentWorld()...)), deploymentRef())
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}

	newest := found.Revisions[0]
	if !slices.Equal(newest.Images, []string{"nginx:5.0"}) {
		t.Fatalf("images = %v, want the images of that revision", newest.Images)
	}
	if newest.Cause != "kubectl set image" {
		t.Fatalf("cause = %q, want the change-cause annotation", newest.Cause)
	}
	if newest.CreatedAt != "2026-07-31T12:00:00Z" {
		t.Fatalf("createdAt = %q, want the creation timestamp", newest.CreatedAt)
	}
	if newest.Replicas != 3 || newest.Ready != 2 {
		t.Fatalf("replicas = %d ready = %d, want 3 and 2", newest.Replicas, newest.Ready)
	}
}

func TestTheTemplateOfARevisionLeavesOutThePodTemplateHash(t *testing.T) {
	template, err := Template(t.Context(), FromDynamic(clusterOf(deploymentWorld()...)), deploymentRef(), 2)
	if err != nil {
		t.Fatalf("read the template: %v", err)
	}

	labels, found, nestedErr := unstructured.NestedStringMap(template, "metadata", "labels")
	if !found || nestedErr != nil {
		t.Fatalf("labels: found = %v err = %v", found, nestedErr)
	}
	if _, carried := labels[templateHashLabel]; carried {
		t.Fatalf("labels = %v, want the pod template hash left out", labels)
	}
	images, _, _ := unstructured.NestedSlice(template, specField, "containers")
	if len(images) != 1 {
		t.Fatalf("containers = %v, want the containers of revision 2", images)
	}
}

func TestATemplateOfAKindWithNoHistoryReadsBackTheReason(t *testing.T) {
	ref := api.ObjectRef{Version: "v1", Resource: "configmaps", Namespace: "shop", Name: "settings"}

	_, err := Template(t.Context(), FromDynamic(clusterOf()), ref, 1)

	if err == nil || err.Error() != "configmaps keep no rollout revisions" {
		t.Fatalf("error = %v, want the sentence the list gives", err)
	}
}

func TestARevisionThatIsNotThereIsNamedInTheError(t *testing.T) {
	_, err := Template(t.Context(), FromDynamic(clusterOf(deploymentWorld()...)), deploymentRef(), 12)

	if err == nil {
		t.Fatal("expected a missing revision to be refused")
	}
	if err.Error() != "web has no revision 12" {
		t.Fatalf("error = %q, want it to name the revision", err.Error())
	}
}

func TestAWorkloadThatIsNotThereIsReportedRatherThanEmpty(t *testing.T) {
	cases := []struct {
		name string
		ref  api.ObjectRef
	}{
		{name: "a deployment that was deleted", ref: deploymentRef()},
		{name: "a stateful set that was deleted", ref: statefulSetRef()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := List(t.Context(), FromDynamic(clusterOf()), tc.ref)

			if err == nil {
				t.Fatal("expected a missing workload to fail")
			}
		})
	}
}

func TestAListThatTheApiserverRefusesIsReported(t *testing.T) {
	cases := []struct {
		name     string
		resource string
		ref      api.ObjectRef
		objects  []runtime.Object
	}{
		{
			name:     "replica sets a deployment cannot list",
			resource: replicaSets,
			ref:      deploymentRef(),
			objects:  deploymentWorld(),
		},
		{
			name:     "controller revisions a stateful set cannot list",
			resource: controllerRevisions,
			ref:      statefulSetRef(),
			objects:  statefulSetWorld(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := clusterOf(tc.objects...)
			client.PrependReactor("list", tc.resource, func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, errors.New("no")
			})

			_, err := List(t.Context(), FromDynamic(client), tc.ref)

			if err == nil {
				t.Fatal("expected a refused list to be reported")
			}
		})
	}
}

func plainReplicaSet(name, revision string, template map[string]any) *unstructured.Unstructured {
	set := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "ReplicaSet",
		"metadata": map[string]any{
			"name":        name,
			"namespace":   "shop",
			"annotations": map[string]any{revisionAnnotation: revision},
			"ownerReferences": []any{
				map[string]any{"apiVersion": "apps/v1", "kind": deploymentKind, "name": "web"},
			},
		},
		specField: map[string]any{},
	}}
	if template != nil {
		_ = unstructured.SetNestedMap(set.Object, template, specField, templateField)
	}
	return set
}

func plainWorkload(kind, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       kind,
		"metadata":   map[string]any{"name": name, "namespace": "shop"},
		specField:    map[string]any{"replicas": int64(1)},
	}}
}

func plainControllerRevision(name, kind, owner string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "ControllerRevision",
		"metadata": map[string]any{
			"name":      name,
			"namespace": "shop",
			"ownerReferences": []any{
				map[string]any{"apiVersion": "apps/v1", "kind": kind, "name": owner},
			},
		},
		"data": map[string]any{specField: map[string]any{templateField: podTemplate("postgres:14", name)}},
	}}
}

func TestARevisionListLeavesOutWhatItCannotRead(t *testing.T) {
	cases := []struct {
		name    string
		objects []runtime.Object
		ref     api.ObjectRef
		names   []string
		current string
	}{
		{
			name: "a revision annotation that is not a number",
			objects: []runtime.Object{
				workload(deploymentKind, "web", "nginx:5.0"),
				replicaSet("web-1", "1", "nginx:1.0", "web"),
				replicaSet("web-later", "later", "nginx:2.0", "web"),
			},
			ref:     deploymentRef(),
			names:   []string{"web-1"},
			current: "web-1",
		},
		{
			name: "a workload with no pod template of its own",
			objects: []runtime.Object{
				plainWorkload(deploymentKind, "web"),
				replicaSet("web-1", "1", "nginx:1.0", "web"),
			},
			ref:     deploymentRef(),
			names:   []string{"web-1"},
			current: "web-1",
		},
		{
			name: "a controller revision with no revision number",
			objects: []runtime.Object{
				workload(statefulSetKind, "db", "postgres:16"),
				controllerRevision("db-1", 1, "postgres:15", statefulSetKind, "db"),
				plainControllerRevision("db-unnumbered", statefulSetKind, "db"),
			},
			ref:     statefulSetRef(),
			names:   []string{"db-1"},
			current: "db-1",
		},
		{
			name: "two replica sets that claim the same revision are ordered by name",
			objects: []runtime.Object{
				workload(deploymentKind, "web", "nginx:5.0"),
				replicaSet("web-b", "1", "nginx:1.0", "web"),
				replicaSet("web-a", "1", "nginx:1.1", "web"),
			},
			ref:     deploymentRef(),
			names:   []string{"web-a", "web-b"},
			current: "web-a",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			found, err := List(t.Context(), FromDynamic(clusterOf(tc.objects...)), tc.ref)
			if err != nil {
				t.Fatalf("list revisions: %v", err)
			}

			if !slices.Equal(namesOf(found), tc.names) {
				t.Fatalf("revisions = %v, want %v", namesOf(found), tc.names)
			}
			if current := currentOf(t, found); current != tc.current {
				t.Fatalf("current = %q, want %q", current, tc.current)
			}
		})
	}
}

func TestARevisionWithNothingToSayLeavesItsFieldsEmpty(t *testing.T) {
	objects := []runtime.Object{
		plainWorkload(deploymentKind, "web"),
		plainReplicaSet("web-1", "1", map[string]any{
			"metadata": map[string]any{"labels": map[string]any{templateHashLabel: "web-1"}},
		}),
	}

	found, err := List(t.Context(), FromDynamic(clusterOf(objects...)), deploymentRef())
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}

	one := found.Revisions[0]
	if one.Images != nil {
		t.Fatalf("images = %v, want none", one.Images)
	}
	if one.CreatedAt != "" {
		t.Fatalf("createdAt = %q, want it empty", one.CreatedAt)
	}
	if one.Cause != "" {
		t.Fatalf("cause = %q, want it empty", one.Cause)
	}
	if one.Replicas != 0 || one.Ready != 0 {
		t.Fatalf("replicas = %d ready = %d, want no counts", one.Replicas, one.Ready)
	}
}

func TestARevisionOnlyListsContainersThatNameAnImage(t *testing.T) {
	objects := []runtime.Object{
		plainWorkload(deploymentKind, "web"),
		plainReplicaSet("web-1", "1", map[string]any{
			specField: map[string]any{
				"containers": []any{
					map[string]any{"name": "app", "image": "nginx:1.0"},
					map[string]any{"name": "waiting"},
					"not a container at all",
				},
			},
		}),
	}

	found, err := List(t.Context(), FromDynamic(clusterOf(objects...)), deploymentRef())
	if err != nil {
		t.Fatalf("list revisions: %v", err)
	}

	if !slices.Equal(found.Revisions[0].Images, []string{"nginx:1.0"}) {
		t.Fatalf("images = %v, want only the container that names one", found.Revisions[0].Images)
	}
}

func TestARevisionThatHoldsNoPodTemplateCannotBeReplayed(t *testing.T) {
	objects := []runtime.Object{
		plainWorkload(deploymentKind, "web"),
		plainReplicaSet("web-1", "1", nil),
	}

	_, err := Template(t.Context(), FromDynamic(clusterOf(objects...)), deploymentRef(), 1)

	if err == nil {
		t.Fatal("expected a revision with no pod template to be refused")
	}
	if err.Error() != "revision 1 of web holds no pod template" {
		t.Fatalf("error = %q, want it to name the revision", err.Error())
	}
}

func TestATemplateOfAWorkloadThatIsNotThereIsReported(t *testing.T) {
	_, err := Template(t.Context(), FromDynamic(clusterOf()), deploymentRef(), 1)

	if err == nil {
		t.Fatal("expected a missing deployment to fail")
	}
}

func TestASourceWithoutANamespaceReadsAcrossTheCluster(t *testing.T) {
	source := FromDynamic(clusterOf(deploymentWorld()...))
	ref := api.ObjectRef{Group: appsGroup, Version: "v1", Resource: replicaSets}

	found, err := source.List(t.Context(), ref)
	if err != nil {
		t.Fatalf("list replica sets: %v", err)
	}

	if len(found) != 7 {
		t.Fatalf("read %d replica sets, want every one in the cluster", len(found))
	}
}

func TestAfterARollbackOnlyOneRevisionIsCurrent(t *testing.T) {
	same := []entry{
		{Revision: api.Revision{Number: 3, Name: "web-c", Current: true}},
		{Revision: api.Revision{Number: 2, Name: "web-b"}},
		{Revision: api.Revision{Number: 1, Name: "web-a", Current: true}},
	}

	got := ordered(same)

	current := []int64{}
	for _, one := range got {
		if one.Current {
			current = append(current, one.Number)
		}
	}
	if len(current) != 1 {
		t.Fatalf("revisions %v say they are current, want exactly one", current)
	}
	if current[0] != 3 {
		t.Fatalf("revision %d is current, want the newest that matches", current[0])
	}
}

func TestTheNewestRevisionIsTheOneRunning(t *testing.T) {
	got := ordered([]entry{
		{Revision: api.Revision{Number: 1, Name: "web-a"}},
		{Revision: api.Revision{Number: 3, Name: "web-c"}},
		{Revision: api.Revision{Number: 2, Name: "web-b"}},
	})

	if len(got) != 3 {
		t.Fatalf("ordered %d revisions", len(got))
	}
	if !got[0].Current || got[0].Number != 3 {
		t.Fatalf("current = %+v, want revision 3", got[0].Revision)
	}
	for _, one := range got[1:] {
		if one.Current {
			t.Fatalf("revision %d also says it is current", one.Number)
		}
	}
}

func TestNothingIsCurrentWhenThereAreNoRevisions(t *testing.T) {
	if got := ordered(nil); len(got) != 0 {
		t.Fatalf("ordered %d revisions from none", len(got))
	}
}
