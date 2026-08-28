package issues

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func ownedCrashLoop(name, ownerKind, ownerUID string) *unstructured.Unstructured {
	return newPod(
		name,
		withOwner(ownerKind, "web-abc", ownerUID),
		withStartTime(testNow.Add(-20*time.Minute)),
		withContainer("app", map[string]any{
			"waiting": map[string]any{"reason": "CrashLoopBackOff"},
		}),
	)
}

func replicaSet(name, uid, ownerUID, revision string) *unstructured.Unstructured {
	obj := newWorkload(kindReplicaSet, name, uid, map[string]any{"readyReplicas": int64(0)}, map[string]any{"replicas": int64(0)})
	obj.SetAnnotations(map[string]string{revisionAnnotation: revision})
	controller := true
	obj.SetOwnerReferences(ownerReference(kindDeployment, "web", ownerUID, &controller))
	return obj
}

func TestPodsFoldUnderTheirDeployment(t *testing.T) {
	pods := make([]*unstructured.Unstructured, 0, 200)
	for index := range 200 {
		pods = append(pods, ownedCrashLoop("web-abc-"+strconv.Itoa(index), kindReplicaSet, "uid-rs"))
	}
	deployment := deploymentWith("web", map[string]any{
		"readyReplicas": int64(0),
		"conditions":    []any{condition("Available", "True", nil)},
	}, map[string]any{"replicas": int64(200)})
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{
		"pods":        pods,
		"replicasets": {replicaSet("web-abc", "uid-rs", "uid-web", "4")},
		"deployments": {deployment},
	}}

	queue := build(t, lister, catalog(podDescriptor(), replicaSetDescriptor(), deploymentDescriptor()))

	if len(queue.Rows) != 1 {
		t.Fatalf("rows = %d, want the whole crashloop as one row", len(queue.Rows))
	}
	row := queue.Rows[0]
	if row.Kind != kindDeployment || row.Object.Name != "web" {
		t.Fatalf("row names %s/%s, want the deployment", row.Kind, row.Object.Name)
	}
	if row.Folded != 200 {
		t.Fatalf("folded = %d, want 200", row.Folded)
	}
	if len(row.Children) != defaultChildren {
		t.Fatalf("children = %d, want the list capped at %d", len(row.Children), defaultChildren)
	}
	if !contains(row.Detail, "200 pods affected") {
		t.Fatalf("detail = %q, want the count", row.Detail)
	}
	if row.Change != "revision 4" {
		t.Fatalf("change = %q, want the replica set revision", row.Change)
	}
}

func TestAFoldNeverLowersSeverity(t *testing.T) {
	crashing := ownedCrashLoop("web-abc-1", kindReplicaSet, "uid-rs")
	deployment := deploymentWith("web", map[string]any{
		"readyReplicas": int64(1),
		"conditions":    []any{condition("Available", "True", nil)},
	}, map[string]any{"replicas": int64(2)})
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{
		"pods":        {crashing},
		"replicasets": {replicaSet("web-abc", "uid-rs", "uid-web", "4")},
		"deployments": {deployment},
	}}

	queue := build(t, lister, catalog(podDescriptor(), replicaSetDescriptor(), deploymentDescriptor()))

	if len(queue.Rows) != 1 {
		t.Fatalf("rows = %d, want one folded row", len(queue.Rows))
	}
	if queue.Rows[0].Severity != api.SeverityFatal {
		t.Fatalf("severity = %q, want the worst of what it folded", queue.Rows[0].Severity)
	}
}

func TestARowWithoutAnOwnerStandsAlone(t *testing.T) {
	pod := newPod(
		"bare",
		withStartTime(testNow.Add(-20*time.Minute)),
		withContainer("app", map[string]any{"waiting": map[string]any{"reason": "CrashLoopBackOff"}}),
	)
	lister := &stubLister{items: itemsOf("pods", pod)}

	queue := build(t, lister, catalog(podDescriptor()))

	row := queue.Rows[0]
	if row.Kind != kindPod || row.Folded != 0 || len(row.Children) != 0 {
		t.Fatalf("row = %+v, want the pod as its own row", row)
	}
}

func TestAnOwnerOutsideTheSnapshotStopsTheWalk(t *testing.T) {
	pod := ownedCrashLoop("web-abc-1", kindReplicaSet, "uid-missing")
	lister := &stubLister{items: itemsOf("pods", pod)}

	queue := build(t, lister, catalog(podDescriptor()))

	if queue.Rows[0].Kind != kindPod {
		t.Fatalf("row = %+v, want the pod when its owner is not loaded", queue.Rows[0])
	}
}

func TestAnOwnerReferenceThatIsNotTheControllerIsIgnored(t *testing.T) {
	pod := newPod(
		"web-1",
		withStartTime(testNow.Add(-20*time.Minute)),
		withContainer("app", map[string]any{"waiting": map[string]any{"reason": "CrashLoopBackOff"}}),
	)
	pod.SetOwnerReferences(ownerReference(kindReplicaSet, "web-abc", "uid-rs", nil))
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{
		"pods":        {pod},
		"replicasets": {replicaSet("web-abc", "uid-rs", "uid-web", "4")},
	}}

	queue := build(t, lister, catalog(podDescriptor(), replicaSetDescriptor()))

	row, ok := rowNamed(queue, "web-1")
	if !ok {
		t.Fatalf("rows = %+v, want the pod on its own", queue.Rows)
	}
	if row.Kind != kindPod {
		t.Fatalf("kind = %q, want the pod", row.Kind)
	}
}

func TestFatalRowsSortAboveDegradedOnes(t *testing.T) {
	rows := []api.Issue{
		{ID: "one", Severity: api.SeverityWarning, Since: stamp(testNow)},
		{ID: "two", Severity: api.SeverityFatal, Since: stamp(testNow.Add(-time.Hour))},
		{ID: "three", Severity: api.SeverityDegraded, Since: stamp(testNow)},
	}

	rank(rows)

	if rows[0].ID != "two" || rows[1].ID != "three" || rows[2].ID != "one" {
		t.Fatalf("order = %q, %q, %q; want fatal, degraded, warning", rows[0].ID, rows[1].ID, rows[2].ID)
	}
}

func TestTheWiderBlastRadiusSortsFirstWithinASeverity(t *testing.T) {
	rows := []api.Issue{
		{ID: "small", Severity: api.SeverityFatal, Folded: 2, Since: stamp(testNow)},
		{ID: "wide", Severity: api.SeverityFatal, Folded: 30, Since: stamp(testNow.Add(-time.Hour))},
	}

	rank(rows)

	if rows[0].ID != "wide" {
		t.Fatalf("first = %q, want the row explaining the most", rows[0].ID)
	}
}

func TestTheNewestRowSortsFirstWhenNothingElseSeparatesThem(t *testing.T) {
	rows := []api.Issue{
		{ID: "old", Severity: api.SeverityFatal, Since: stamp(testNow.Add(-time.Hour))},
		{ID: "new", Severity: api.SeverityFatal, Since: stamp(testNow)},
	}

	rank(rows)

	if rows[0].ID != "new" {
		t.Fatalf("first = %q, want the newest", rows[0].ID)
	}
}

func TestIdenticalRowsSortByTheirIdentity(t *testing.T) {
	rows := []api.Issue{
		{ID: "b", Severity: api.SeverityFatal, Since: stamp(testNow)},
		{ID: "a", Severity: api.SeverityFatal, Since: stamp(testNow)},
	}

	rank(rows)

	if rows[0].ID != "a" {
		t.Fatalf("first = %q, want a stable order", rows[0].ID)
	}
}

func TestTheQueueIsCappedAndSaysWhatItDropped(t *testing.T) {
	const room = 3
	names := []string{"bare-1", "bare-2", "bare-3", "bare-4", "bare-5"}
	pods := make([]*unstructured.Unstructured, 0, len(names))
	for _, name := range names {
		pods = append(pods, newPod(
			name,
			withStartTime(testNow.Add(-20*time.Minute)),
			withContainer("app", map[string]any{"waiting": map[string]any{"reason": "CrashLoopBackOff"}}),
		))
	}
	lister := &stubLister{items: itemsOf("pods", pods...)}
	limits := testLimits()
	limits.Rows = room

	queue := buildLimited(t, lister, &stubEvents{}, catalog(podDescriptor()), limits)

	if len(queue.Rows) != room {
		t.Fatalf("rows = %d, want the cap at %d", len(queue.Rows), room)
	}
	if queue.Dropped != len(names)-room {
		t.Fatalf("dropped = %d, want %d", queue.Dropped, len(names)-room)
	}
}

func TestTheChildListIsCappedWhileTheCountStaysHonest(t *testing.T) {
	const shown = 2
	pods := make([]*unstructured.Unstructured, 0, 6)
	for index := range 6 {
		pods = append(pods, ownedCrashLoop("web-abc-"+strconv.Itoa(index), kindReplicaSet, "uid-rs"))
	}
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{
		"pods":        pods,
		"replicasets": {replicaSet("web-abc", "uid-rs", "uid-web", "4")},
	}}
	limits := testLimits()
	limits.Children = shown

	queue := buildLimited(t, lister, &stubEvents{}, catalog(podDescriptor(), replicaSetDescriptor()), limits)

	row := queue.Rows[0]
	if len(row.Children) != shown {
		t.Fatalf("children = %d, want the list capped at %d", len(row.Children), shown)
	}
	if row.Folded != 6 {
		t.Fatalf("folded = %d, want the true count kept while the list is trimmed", row.Folded)
	}
}

func TestASingleFoldedChildDoesNotGetACountSuffix(t *testing.T) {
	pod := ownedCrashLoop("web-abc-1", kindReplicaSet, "uid-rs")
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{
		"pods":        {pod},
		"replicasets": {replicaSet("web-abc", "uid-rs", "uid-web", "4")},
	}}

	queue := build(t, lister, catalog(podDescriptor(), replicaSetDescriptor()))

	if contains(queue.Rows[0].Detail, "affected") {
		t.Fatalf("detail = %q, want no count for a single pod", queue.Rows[0].Detail)
	}
}

func TestAnEmptyClusterProducesAnEmptyQueue(t *testing.T) {
	lister := &stubLister{}

	queue := build(t, lister, catalog(podDescriptor(), deploymentDescriptor()))

	if len(queue.Rows) != 0 || queue.Dropped != 0 || queue.Error != "" {
		t.Fatalf("queue = %+v, want it empty and quiet", queue)
	}
}

func TestStampIsEmptyForAZeroTime(t *testing.T) {
	if got := stamp(time.Time{}); got != "" {
		t.Fatalf("stamp = %q, want empty", got)
	}
}

func TestPluralFallsBackWhenTheKindIsUnknown(t *testing.T) {
	if got := plural([]api.IssueChild{{}, {}}); got != "objects" {
		t.Fatalf("plural = %q, want objects", got)
	}
}

func TestPluralLowercasesTheKind(t *testing.T) {
	children := []api.IssueChild{{Kind: kindPod}, {Kind: kindPod}}

	if got := plural(children); got != "pods" {
		t.Fatalf("plural = %q, want pods", got)
	}
}

func TestPluralSaysObjectsWhenTheFoldMixesKinds(t *testing.T) {
	children := []api.IssueChild{{Kind: kindPod}, {Kind: kindReplicaSet}}

	if got := plural(children); got != "objects" {
		t.Fatalf("plural = %q, want objects", got)
	}
}

func TestALowercaseKindIsLeftAlone(t *testing.T) {
	if got := lowerFirst("pod"); got != "pod" {
		t.Fatalf("lowerFirst = %q, want pod", got)
	}
}

func TestAnEmptyStringLowercasesToNothing(t *testing.T) {
	if got := lowerFirst(""); got != "" {
		t.Fatalf("lowerFirst = %q, want empty", got)
	}
}

func TestAnUnreadableTimestampSortsAsZero(t *testing.T) {
	if got := seenAt("not a time"); !got.IsZero() {
		t.Fatalf("seenAt = %v, want the zero time", got)
	}
}

func TestAReplicaShortfallIsDroppedWhenItsPodsExplainIt(t *testing.T) {
	pods := []*unstructured.Unstructured{
		ownedCrashLoop("web-abc-1", kindReplicaSet, "uid-rs"),
		ownedCrashLoop("web-abc-2", kindReplicaSet, "uid-rs"),
	}
	replica := replicaSet("web-abc", "uid-rs", "uid-web", "4")
	setNested(replica, int64(0), "status", "readyReplicas")
	setNested(replica, int64(2), "spec", "replicas")
	deployment := deploymentWith("web", map[string]any{
		"readyReplicas": int64(0),
		"conditions":    []any{condition("Available", "True", nil)},
	}, map[string]any{"replicas": int64(2)})
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{
		"pods":        pods,
		"replicasets": {replica},
		"deployments": {deployment},
	}}

	queue := build(t, lister, catalog(podDescriptor(), replicaSetDescriptor(), deploymentDescriptor()))

	if len(queue.Rows) != 1 {
		t.Fatalf("rows = %+v, want one row", queue.Rows)
	}
	row := queue.Rows[0]
	if row.Title != "CrashLoopBackOff" {
		t.Fatalf("title = %q, want the cause rather than its effect", row.Title)
	}
	if row.Folded != 2 || len(row.Children) != 2 {
		t.Fatalf("row = %+v, want only the two pods folded", row)
	}
	if !contains(row.Detail, "2 pods affected") {
		t.Fatalf("detail = %q, want the pods counted as pods", row.Detail)
	}
}

func TestAReplicaShortfallSurvivesWhenNoPodExplainsIt(t *testing.T) {
	deployment := deploymentWith("web", map[string]any{
		"readyReplicas": int64(0),
		"conditions":    []any{condition("Available", "True", nil)},
	}, map[string]any{"replicas": int64(2)})
	lister := &stubLister{items: deploymentItems(deployment)}

	queue := build(t, lister, catalog(deploymentDescriptor()))

	if len(queue.Rows) != 1 || queue.Rows[0].Title != titleShortOfReplicas {
		t.Fatalf("rows = %+v, want the shortfall reported when nothing else explains it", queue.Rows)
	}
}

func shuffledCrashLoops(t *testing.T, names []string) api.IssueQueue {
	t.Helper()
	pods := make([]*unstructured.Unstructured, 0, len(names))
	for _, name := range names {
		pods = append(pods, ownedCrashLoop(name, kindReplicaSet, "uid-rs"))
	}
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{
		"pods":        pods,
		"replicasets": {replicaSet("web-abc", "uid-rs", "uid-web", "4")},
	}}
	return build(t, lister, catalog(podDescriptor(), replicaSetDescriptor()))
}

func TestChildrenComeBackInTheSameOrderWhateverTheCacheHandsOver(t *testing.T) {
	forward := shuffledCrashLoops(t, []string{"web-abc-1", "web-abc-2", "web-abc-3"})
	backward := shuffledCrashLoops(t, []string{"web-abc-3", "web-abc-1", "web-abc-2"})

	if len(forward.Rows) != 1 || len(backward.Rows) != 1 {
		t.Fatalf("rows = %d and %d, want one each", len(forward.Rows), len(backward.Rows))
	}
	for index, child := range forward.Rows[0].Children {
		if backward.Rows[0].Children[index].Object.Name != child.Object.Name {
			t.Fatalf("children = %+v then %+v, want the same order both times",
				forward.Rows[0].Children, backward.Rows[0].Children)
		}
	}
	if forward.Rows[0].Children[0].Object.Name != "web-abc-1" {
		t.Fatalf("first child = %q, want the name order to settle ties", forward.Rows[0].Children[0].Object.Name)
	}
}

func tiedFaults(t *testing.T, crashingFirst bool) api.IssueQueue {
	t.Helper()
	crashing := ownedCrashLoop("web-abc-1", kindReplicaSet, "uid-rs")
	pulling := newPod(
		"web-abc-2",
		withOwner(kindReplicaSet, "web-abc", "uid-rs"),
		withStartTime(testNow.Add(-20*time.Minute)),
		withContainer("app", map[string]any{
			"waiting": map[string]any{"reason": "ImagePullBackOff"},
		}),
	)
	pods := []*unstructured.Unstructured{crashing, pulling}
	if !crashingFirst {
		pods = []*unstructured.Unstructured{pulling, crashing}
	}
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{
		"pods":        pods,
		"replicasets": {replicaSet("web-abc", "uid-rs", "uid-web", "4")},
	}}
	return build(t, lister, catalog(podDescriptor(), replicaSetDescriptor()))
}

func TestTheLeadIsTheSameWhateverTheCacheHandsOver(t *testing.T) {
	forward := tiedFaults(t, true)
	backward := tiedFaults(t, false)

	if forward.Rows[0].Title != backward.Rows[0].Title {
		t.Fatalf("title = %q then %q, want two equally bad pods to settle on one lead",
			forward.Rows[0].Title, backward.Rows[0].Title)
	}
	if forward.Rows[0].Title != "CrashLoopBackOff" {
		t.Fatalf("title = %q, want the lead settled by name", forward.Rows[0].Title)
	}
}

func TestWhichFindingLeadsARow(t *testing.T) {
	at := testNow.Add(-time.Hour)
	base := finding{severity: severityDegraded, since: at, subject: object{obj: newPod("web-2"), desc: podDescriptor()}}
	cases := []struct {
		name string
		item finding
		want bool
	}{
		{
			name: "the worse severity leads",
			item: finding{severity: severityFatal, since: at.Add(-time.Hour), subject: base.subject},
			want: true,
		},
		{
			name: "the milder severity does not",
			item: finding{severity: severityWarning, since: at.Add(time.Hour), subject: base.subject},
			want: false,
		},
		{
			name: "at the same severity the newer leads",
			item: finding{severity: severityDegraded, since: at.Add(time.Minute), subject: base.subject},
			want: true,
		},
		{
			name: "at the same severity the older does not",
			item: finding{severity: severityDegraded, since: at.Add(-time.Minute), subject: base.subject},
			want: false,
		},
		{
			name: "an exact tie settles on the name",
			item: finding{
				severity: severityDegraded,
				since:    at,
				subject:  object{obj: newPod("web-1"), desc: podDescriptor()},
			},
			want: true,
		},
		{
			name: "and the later name loses",
			item: finding{
				severity: severityDegraded,
				since:    at,
				subject:  object{obj: newPod("web-3"), desc: podDescriptor()},
			},
			want: false,
		},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if got := leads(item.item, base); got != item.want {
				t.Fatalf("leads = %v, want %v", got, item.want)
			}
		})
	}
}

func TestTheOldestStallCandidatesSettleTiesByName(t *testing.T) {
	names := []string{"web-3", "web-1", "web-2"}
	pods := make([]*unstructured.Unstructured, 0, len(names))
	for _, name := range names {
		pods = append(pods, newPod(
			name,
			withPhase(phasePending),
			withStartTime(testNow.Add(-time.Hour)),
		))
	}
	lister := &stubLister{items: itemsOf("pods", pods...)}
	events := &stubEvents{}

	buildWith(t, lister, events, catalog(podDescriptor()))

	want := []string{"uid-web-1", "uid-web-2", "uid-web-3"}
	if !slices.Equal(events.askedAbout(), want) {
		t.Fatalf("asked = %v, want the tied candidates settled by name", events.askedAbout())
	}
}

func TestAFoldNeverLowersSeverityWhenAMilderFindingExplainsIt(t *testing.T) {
	stalled := newPod(
		"web-abc-1",
		withOwner(kindReplicaSet, "web-abc", "uid-rs"),
		withPhase(phasePending),
		withStartTime(testNow.Add(-time.Hour)),
	)
	deployment := deploymentWith("web", map[string]any{
		"readyReplicas": int64(0),
		"conditions":    []any{condition("Available", "True", nil)},
	}, map[string]any{"replicas": int64(3)})
	lister := &stubLister{items: map[string][]*unstructured.Unstructured{
		"pods":        {stalled},
		"replicasets": {replicaSet("web-abc", "uid-rs", "uid-web", "4")},
		"deployments": {deployment},
	}}

	queue := buildWith(t, lister, &stubEvents{}, catalog(podDescriptor(), replicaSetDescriptor(), deploymentDescriptor()))

	if len(queue.Rows) != 1 {
		t.Fatalf("rows = %+v, want one folded row", queue.Rows)
	}
	if queue.Rows[0].Severity != api.SeverityFatal {
		t.Fatalf("severity = %q, want fatal; a warning-level guess must not erase the fatal shortfall it sits under",
			queue.Rows[0].Severity)
	}
}

func TestAFoldTakesTheWorstOfWhateverItHolds(t *testing.T) {
	cases := []struct {
		name  string
		group []finding
		want  string
	}{
		{
			name:  "a fatal among warnings",
			group: []finding{{severity: severityWarning}, {severity: severityFatal}, {severity: severityWarning}},
			want:  api.SeverityFatal,
		},
		{
			name:  "a degraded among warnings",
			group: []finding{{severity: severityWarning}, {severity: severityDegraded}},
			want:  api.SeverityDegraded,
		},
		{
			name:  "warnings alone",
			group: []finding{{severity: severityWarning}, {severity: severityWarning}},
			want:  api.SeverityWarning,
		},
		{
			name:  "the worst arriving first",
			group: []finding{{severity: severityFatal}, {severity: severityWarning}},
			want:  api.SeverityFatal,
		},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if got := severityName(worst(item.group)); got != item.want {
				t.Fatalf("severity = %q, want %q", got, item.want)
			}
		})
	}
}
