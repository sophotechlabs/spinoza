package topology

import (
	"os"
	"regexp"
	"strconv"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func replicated(ready int64, wanted any) *unstructured.Unstructured {
	spec := map[string]any{}
	if wanted != nil {
		spec["replicas"] = wanted
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"spec":   spec,
		"status": map[string]any{"readyReplicas": ready},
	}}
}

func scheduled(ready, wanted int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"status": map[string]any{"numberReady": ready, "desiredNumberScheduled": wanted},
	}}
}

func phased(phase, ready string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"status": map[string]any{
			"phase":      phase,
			"conditions": []any{map[string]any{"type": "Ready", "status": ready}},
		},
	}}
}

func conditioned(entries ...any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"status": map[string]any{"conditions": entries},
	}}
}

func condition(kind, status string) map[string]any {
	return map[string]any{"type": kind, "status": status}
}

func TestTheNodeBudgetIsTheCanvasCap(t *testing.T) {
	source, err := os.ReadFile("../../frontend/src/lib/graphState.ts")
	if err != nil {
		t.Fatalf("read the graph state: %v", err)
	}
	found := regexp.MustCompile(`MAX_NODES = (\d+)`).FindSubmatch(source)
	if found == nil {
		t.Fatal("the graph state no longer declares MAX_NODES; the two limits cannot be compared")
	}
	refuses, err := strconv.Atoi(string(found[1]))
	if err != nil {
		t.Fatalf("MAX_NODES is not a number: %v", err)
	}
	if refuses != nodeBudget {
		t.Fatalf("the canvas refuses above %d but the builder aims under %d; they move together or not at all", refuses, nodeBudget)
	}
}

func TestOnlyThreeKindsEverFold(t *testing.T) {
	for _, kind := range []string{kindPod, kindReplicaSet, kindJob} {
		if !foldableKinds[kind] {
			t.Fatalf("%s no longer folds", kind)
		}
	}
	if len(foldableKinds) != 3 {
		t.Fatalf("%d kinds fold, want the three the rule names", len(foldableKinds))
	}
	for _, kind := range []string{kindDeployment, kindStatefulSet, kindDaemonSet, kindCronJob} {
		if foldableKinds[kind] {
			t.Fatalf("%s folds into its owner; only pods, replica sets and jobs do", kind)
		}
	}
}

func TestEveryListedResourceHasACategory(t *testing.T) {
	cases := []struct {
		name     string
		group    string
		resource string
		want     string
	}{
		{name: "pods", resource: "pods", want: categoryPod},
		{name: "services", resource: "services", want: categoryService},
		{name: "replication controllers", resource: "replicationcontrollers", want: categoryWorkload},
		{name: "deployments", group: "apps", resource: "deployments", want: categoryWorkload},
		{name: "replica sets", group: "apps", resource: "replicasets", want: categoryWorkload},
		{name: "stateful sets", group: "apps", resource: "statefulsets", want: categoryWorkload},
		{name: "daemon sets", group: "apps", resource: "daemonsets", want: categoryWorkload},
		{name: "jobs", group: "batch", resource: "jobs", want: categoryWorkload},
		{name: "cron jobs", group: "batch", resource: "cronjobs", want: categoryWorkload},
		{name: "ingresses", group: "networking.k8s.io", resource: "ingresses", want: categoryIngress},
		{name: "autoscalers", group: "autoscaling", resource: "horizontalpodautoscalers", want: categoryAutoscaler},
		{name: "config maps are never listed", resource: "configmaps", want: ""},
		{name: "secrets are never listed", resource: "secrets", want: ""},
		{name: "nodes are never listed", resource: "nodes", want: ""},
		{name: "an unknown custom resource", group: "argoproj.io", resource: "rollouts", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			desc := api.ResourceDescriptor{Group: tc.group, Resource: tc.resource}
			if got := categoryFor(desc); got != tc.want {
				t.Fatalf("category = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAnAPIVersionSplitsIntoGroupAndVersion(t *testing.T) {
	cases := []struct {
		name        string
		apiVersion  string
		wantGroup   string
		wantVersion string
	}{
		{name: "a grouped kind", apiVersion: "apps/v1", wantGroup: "apps", wantVersion: "v1"},
		{name: "the core group", apiVersion: "v1", wantGroup: "", wantVersion: "v1"},
		{name: "a dotted group", apiVersion: "argoproj.io/v1alpha1", wantGroup: "argoproj.io", wantVersion: "v1alpha1"},
		{name: "nothing at all", apiVersion: "", wantGroup: "", wantVersion: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := groupOf(tc.apiVersion); got != tc.wantGroup {
				t.Fatalf("group = %q, want %q", got, tc.wantGroup)
			}
			if got := versionOf(tc.apiVersion); got != tc.wantVersion {
				t.Fatalf("version = %q, want %q", got, tc.wantVersion)
			}
		})
	}
}

func TestAnAbsentReplicaCountMeansOne(t *testing.T) {
	cases := []struct {
		name    string
		wanted  any
		replica int64
	}{
		{name: "a count it states", wanted: int64(3), replica: 3},
		{name: "deliberately scaled to zero", wanted: int64(0), replica: 0},
		{name: "no count at all", wanted: nil, replica: 1},
		{name: "a count of the wrong type", wanted: "two", replica: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := wantedReplicas(replicated(0, tc.wanted)); got != tc.replica {
				t.Fatalf("wanted = %d, want %d", got, tc.replica)
			}
		})
	}
}

func TestAReplicaSummaryReadsReadyOverWanted(t *testing.T) {
	if got := replicaSummary(2, 3); got != "2/3" {
		t.Fatalf("summary = %q, want 2/3", got)
	}
}

func TestReadinessPerKind(t *testing.T) {
	cases := []struct {
		name  string
		kind  string
		obj   *unstructured.Unstructured
		ready string
	}{
		{name: "a full deployment", kind: kindDeployment, obj: replicated(3, int64(3)), ready: readyTrue},
		{name: "a deployment short of replicas", kind: kindDeployment, obj: replicated(1, int64(3)), ready: readyFalse},
		{name: "a deployment scaled to zero", kind: kindDeployment, obj: replicated(0, int64(0)), ready: readyTrue},
		{name: "a deployment that states no count", kind: kindDeployment, obj: replicated(0, nil), ready: readyFalse},
		{name: "a stateful set short of replicas", kind: kindStatefulSet, obj: replicated(0, int64(1)), ready: readyFalse},
		{name: "a full daemon set", kind: kindDaemonSet, obj: scheduled(2, 2), ready: readyTrue},
		{name: "a daemon set missing a node", kind: kindDaemonSet, obj: scheduled(1, 2), ready: readyFalse},
		{name: "a running pod", kind: kindPod, obj: phased("Running", readyTrue), ready: readyTrue},
		{name: "a pod that is not ready", kind: kindPod, obj: phased("Running", readyFalse), ready: readyFalse},
		{name: "a pod that finished", kind: kindPod, obj: phased("Succeeded", readyFalse), ready: readyTrue},
		{name: "a pod that failed", kind: kindPod, obj: phased("Failed", readyTrue), ready: readyFalse},
		{name: "a pending pod", kind: kindPod, obj: phased("Pending", ""), ready: readyFalse},
		{name: "a job still running", kind: kindJob, obj: conditioned(), ready: readyTrue},
		{name: "a job that failed", kind: kindJob, obj: conditioned(condition("Failed", readyTrue)), ready: readyFalse},
		{name: "a cron job says nothing", kind: kindCronJob, obj: conditioned(), ready: readyUnknown},
		{name: "a custom resource that is ready", kind: "Rollout", obj: conditioned(condition("Ready", readyTrue)), ready: readyTrue},
		{name: "a custom resource that is not", kind: "Rollout", obj: conditioned(condition("Ready", readyFalse)), ready: readyFalse},
		{name: "a custom resource with no conditions", kind: "Rollout", obj: conditioned(), ready: readyUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := readyOf(tc.obj, tc.kind); got != tc.ready {
				t.Fatalf("ready = %q, want %q", got, tc.ready)
			}
		})
	}
}

func TestTheStatusLinePerKind(t *testing.T) {
	cases := []struct {
		name   string
		kind   string
		obj    *unstructured.Unstructured
		status string
	}{
		{name: "a deployment counts replicas", kind: kindDeployment, obj: replicated(1, int64(3)), status: "1/3"},
		{name: "a deployment with no count wants one", kind: kindDeployment, obj: replicated(0, nil), status: "0/1"},
		{name: "a daemon set counts nodes", kind: kindDaemonSet, obj: scheduled(1, 4), status: "1/4"},
		{name: "a pod states its phase", kind: kindPod, obj: phased("Running", readyTrue), status: "Running"},
		{name: "a custom resource summarizes its condition", kind: "Rollout", obj: conditioned(condition("Ready", readyTrue)), status: "Ready"},
		{name: "a custom resource with nothing to say", kind: "Rollout", obj: conditioned(), status: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusOf(tc.obj, tc.kind); got != tc.status {
				t.Fatalf("status = %q, want %q", got, tc.status)
			}
		})
	}
}

func TestAConditionIsOnlyTrueWhenItSaysSo(t *testing.T) {
	cases := []struct {
		name string
		obj  *unstructured.Unstructured
		want bool
	}{
		{name: "the condition is true", obj: conditioned(condition("Failed", readyTrue)), want: true},
		{name: "the condition is false", obj: conditioned(condition("Failed", readyFalse)), want: false},
		{name: "another condition is true", obj: conditioned(condition("Complete", readyTrue)), want: false},
		{name: "no conditions at all", obj: conditioned(), want: false},
		{name: "a condition that is not a map", obj: conditioned("not-a-map"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := conditionTrue(tc.obj, "Failed"); got != tc.want {
				t.Fatalf("failed = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAPlaceholderIsBrokenOnlyWhenItIsMissing(t *testing.T) {
	if got := readyForPlaceholder(statusMissing); got != readyFalse {
		t.Fatalf("ready = %q, want False for something an object names but nothing provides", got)
	}
	if got := readyForPlaceholder(""); got != readyUnknown {
		t.Fatalf("ready = %q, want Unknown for something that was simply never listed", got)
	}
}

func TestASelectorPicksOnlyWhatMatchesEveryPair(t *testing.T) {
	cases := []struct {
		name     string
		selector map[string]string
		labels   map[string]string
		want     bool
	}{
		{name: "one pair that matches", selector: map[string]string{"app": "web"}, labels: map[string]string{"app": "web"}, want: true},
		{name: "one pair that does not", selector: map[string]string{"app": "web"}, labels: map[string]string{"app": "api"}, want: false},
		{name: "every pair matches", selector: map[string]string{"app": "web", "tier": "front"}, labels: map[string]string{"app": "web", "tier": "front", "extra": "ok"}, want: true},
		{name: "one pair of several does not", selector: map[string]string{"app": "web", "tier": "front"}, labels: map[string]string{"app": "web"}, want: false},
		{name: "the pod carries no labels", selector: map[string]string{"app": "web"}, labels: nil, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := selects(tc.selector, tc.labels); got != tc.want {
				t.Fatalf("selects = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestASelectorKeepsOnlyItsStringPairs(t *testing.T) {
	raw := map[string]any{"app": "web", "port": int64(80), "on": true}

	result := stringsOf(raw, true)

	expected := map[string]string{"app": "web"}
	if len(result) != len(expected) || result["app"] != expected["app"] {
		t.Fatalf("selector = %v, want %v: a non-string value cannot match a label", result, expected)
	}
	if stringsOf(nil, false) != nil {
		t.Fatal("a spec with no selector at all should read as no selector")
	}
}

func TestAnIngressNamesEveryBackendOnce(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{"spec": map[string]any{
		"defaultBackend": map[string]any{"service": map[string]any{"name": "fallback"}},
		"rules": []any{
			map[string]any{"http": map[string]any{"paths": []any{
				map[string]any{"backend": map[string]any{"service": map[string]any{"name": "web"}}},
				map[string]any{"backend": map[string]any{"service": map[string]any{"name": "web"}}},
				map[string]any{"backend": map[string]any{"resource": map[string]any{"name": "bucket"}}},
				map[string]any{"backend": map[string]any{}},
				"not-a-map",
			}}},
			map[string]any{"http": map[string]any{}},
			"not-a-map",
		},
	}}}

	result := backendServices(obj)

	expected := []string{"fallback", "web"}
	if len(result) != len(expected) {
		t.Fatalf("backends = %v, want %v", result, expected)
	}
	for i, want := range expected {
		if result[i] != want {
			t.Fatalf("backend %d = %q, want %q", i, result[i], want)
		}
	}
}

func TestAnIngressWithNoBackendsNamesNothing(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{"spec": map[string]any{}}}

	if got := backendServices(obj); len(got) != 0 {
		t.Fatalf("backends = %v, want none", got)
	}
}

func TestAPodTemplateNamesEveryConfigItMounts(t *testing.T) {
	spec := map[string]any{
		"volumes": []any{
			map[string]any{"configMap": map[string]any{"name": "settings"}},
			map[string]any{"secret": map[string]any{"secretName": "tls"}},
			map[string]any{"projected": map[string]any{"sources": []any{
				map[string]any{"configMap": map[string]any{"name": "bundle"}},
				map[string]any{"secret": map[string]any{"name": "signing"}},
			}}},
			map[string]any{"name": "identity", "projected": map[string]any{"sources": []any{
				map[string]any{"serviceAccountToken": map[string]any{"path": "token"}},
				map[string]any{"configMap": map[string]any{"name": "identity-policy"}},
			}}},
			map[string]any{"emptyDir": map[string]any{}},
			"not-a-map",
		},
		"imagePullSecrets": []any{map[string]any{"name": "registry"}, "not-a-map"},
		"initContainers": []any{map[string]any{
			"envFrom": []any{map[string]any{"configMapRef": map[string]any{"name": "shared"}}},
		}},
		"containers": []any{"not-a-map", map[string]any{
			"envFrom": []any{
				map[string]any{"secretRef": map[string]any{"name": "tls"}},
				map[string]any{},
				"not-a-map",
			},
			"env": []any{
				map[string]any{"valueFrom": map[string]any{"configMapKeyRef": map[string]any{"name": "settings"}}},
				map[string]any{"valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": "token"}}},
				map[string]any{"value": "literal"},
				"not-a-map",
			},
		}},
		"ephemeralContainers": []any{map[string]any{
			"envFrom": []any{map[string]any{"configMapRef": map[string]any{"name": "debug"}}},
		}},
	}

	result := configRefs(spec)

	expected := []configRef{
		{kind: kindConfigMap, name: "bundle"},
		{kind: kindConfigMap, name: "debug"},
		{kind: kindConfigMap, name: "identity-policy"},
		{kind: kindConfigMap, name: "settings"},
		{kind: kindConfigMap, name: "shared"},
		{kind: kindSecret, name: "registry"},
		{kind: kindSecret, name: "signing"},
		{kind: kindSecret, name: "tls"},
		{kind: kindSecret, name: "token"},
	}
	if len(result) != len(expected) {
		t.Fatalf("refs = %v, want %v", result, expected)
	}
	for i, want := range expected {
		if result[i] != want {
			t.Fatalf("ref %d = %v, want %v", i, result[i], want)
		}
	}
}

func TestTheVolumeTheKubeletInjectsIsRecognised(t *testing.T) {
	cases := []struct {
		name   string
		volume map[string]any
		want   bool
	}{
		{
			name: "the injected token volume",
			volume: map[string]any{
				"name": "kube-api-access-x9k2p",
				"projected": map[string]any{"sources": []any{
					map[string]any{"serviceAccountToken": map[string]any{"path": "token"}},
					map[string]any{"configMap": map[string]any{"name": "kube-root-ca.crt"}},
				}},
			},
			want: true,
		},
		{
			name: "a projection the author wrote",
			volume: map[string]any{
				"name": "bundle",
				"projected": map[string]any{"sources": []any{
					map[string]any{"configMap": map[string]any{"name": "bundle"}},
				}},
			},
			want: false,
		},
		{name: "no sources at all", volume: map[string]any{"name": "kube-api-access-empty"}, want: false},
		{
			name: "a source that is not a map",
			volume: map[string]any{
				"name":      "kube-api-access-broken",
				"projected": map[string]any{"sources": []any{"not-a-map"}},
			},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := injectedByKubelet(tc.volume); got != tc.want {
				t.Fatalf("injected = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReachingIntoAShapeThatIsNotThere(t *testing.T) {
	object := map[string]any{"spec": map[string]any{"list": []any{"one"}, "leaf": "text"}}

	cases := []struct {
		name string
		keys []string
		want bool
	}{
		{name: "a map that is there", keys: []string{"spec"}, want: true},
		{name: "a key that is missing", keys: []string{"status"}, want: false},
		{name: "a key that holds a string", keys: []string{"spec", "leaf"}, want: false},
		{name: "a path through a missing key", keys: []string{"status", "conditions"}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if found := mapAt(object, tc.keys...) != nil; found != tc.want {
				t.Fatalf("found = %v, want %v", found, tc.want)
			}
		})
	}

	if got := sliceAt(object, "spec", "list"); len(got) != 1 {
		t.Fatalf("list = %v, want the one entry", got)
	}
	if got := sliceAt(object, "spec", "missing"); got != nil {
		t.Fatalf("list = %v, want nothing", got)
	}
	if got := sliceAt(object, "status", "conditions"); got != nil {
		t.Fatalf("list = %v, want nothing when the path does not exist", got)
	}
}

func TestAnOpenSetIgnoresWhatIsNotAnID(t *testing.T) {
	result := setOf([]string{"one", "", "two"})

	if len(result) != 2 {
		t.Fatalf("open = %v, want the two real ids", result)
	}
	if result[""] {
		t.Fatal("an empty entry became an id")
	}
}

func TestWhatIsVisibleDependsOnEveryOwnerAboveIt(t *testing.T) {
	parents := map[string]string{"pod": "replicas", "replicas": "deployment"}

	cases := []struct {
		name     string
		expanded map[string]bool
		want     bool
	}{
		{name: "nothing open", expanded: map[string]bool{}, want: false},
		{name: "only the deployment open", expanded: map[string]bool{"deployment": true}, want: false},
		{name: "only the replica set open", expanded: map[string]bool{"replicas": true}, want: false},
		{name: "both open", expanded: map[string]bool{"replicas": true, "deployment": true}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shown("pod", parents, tc.expanded); got != tc.want {
				t.Fatalf("shown = %v, want %v", got, tc.want)
			}
		})
	}
	if !shown("deployment", parents, map[string]bool{}) {
		t.Fatal("a node nobody owns is always drawn")
	}
}

func TestAHiddenNodeAnchorsToTheNearestOwnerThatIsDrawn(t *testing.T) {
	parents := map[string]string{"pod": "replicas", "replicas": "deployment"}

	cases := []struct {
		name    string
		visible map[string]bool
		want    string
	}{
		{name: "the deployment is drawn", visible: map[string]bool{"deployment": true}, want: "deployment"},
		{name: "the replica set is drawn", visible: map[string]bool{"replicas": true}, want: "replicas"},
		{name: "nothing above it is drawn", visible: map[string]bool{}, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := anchorOf("pod", parents, tc.visible); got != tc.want {
				t.Fatalf("anchor = %q, want %q", got, tc.want)
			}
		})
	}
	if got := anchorOf("deployment", parents, map[string]bool{}); got != "" {
		t.Fatalf("anchor = %q, want nothing for a node nobody owns", got)
	}
}

func TestAFoldDeepEnoughToBeACycleStopsWalking(t *testing.T) {
	parents := map[string]string{"left": "right", "right": "left"}
	open := map[string]bool{"left": true, "right": true}

	if shown("left", parents, open) {
		t.Fatal("a cycle where every owner is open must still stop rather than walk forever")
	}
	if got := anchorOf("left", parents, map[string]bool{}); got != "" {
		t.Fatalf("anchor = %q, want nothing: a cycle has no owner that is drawn", got)
	}
}
