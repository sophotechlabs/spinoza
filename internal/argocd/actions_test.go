package argocd

import (
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

var applicationGVR = schema.GroupVersionResource{
	Group:    "argoproj.io",
	Version:  "v1alpha1",
	Resource: "applications",
}

func actionClient(objs ...runtime.Object) *fake.FakeDynamicClient {
	kinds := map[schema.GroupVersionResource]string{
		applicationGVR: "ApplicationList",
	}
	return fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), kinds, objs...)
}

func newApplication() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata": map[string]any{
			"name":      "podinfo",
			"namespace": "argocd",
		},
		"spec": map[string]any{
			"project": "default",
			"source":  map[string]any{"repoURL": "https://example.test/apps", "path": "podinfo"},
		},
	}}
}

func automating(app *unstructured.Unstructured, policy map[string]any) *unstructured.Unstructured {
	spec, _, _ := unstructured.NestedMap(app.Object, "spec")
	spec["syncPolicy"] = map[string]any{"automated": policy}
	_ = unstructured.SetNestedMap(app.Object, spec, "spec")
	return app
}

func operating(app *unstructured.Unstructured, phase string) *unstructured.Unstructured {
	_ = unstructured.SetNestedField(app.Object, phase, "status", "operationState", "phase")
	return app
}

func withHistory(app *unstructured.Unstructured, entries ...any) *unstructured.Unstructured {
	_ = unstructured.SetNestedSlice(app.Object, entries, "status", "history")
	return app
}

func historyOf(id int64, revision string) map[string]any {
	return map[string]any{
		"id":       id,
		"revision": revision,
		"source":   map[string]any{"repoURL": "https://example.test/apps", "path": "podinfo"},
	}
}

func applicationRef() api.ObjectRef {
	return api.ObjectRef{
		Group:     "argoproj.io",
		Version:   "v1alpha1",
		Resource:  "applications",
		Namespace: "argocd",
		Name:      "podinfo",
	}
}

func ask(action Action) Request {
	return Request{Action: action}
}

func readBack(t *testing.T, client *fake.FakeDynamicClient) *unstructured.Unstructured {
	t.Helper()
	got, err := client.Resource(applicationGVR).
		Namespace("argocd").
		Get(t.Context(), "podinfo", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	return got
}

func syncOperationOf(t *testing.T, got *unstructured.Unstructured) map[string]any {
	t.Helper()
	sync, found, err := unstructured.NestedMap(got.Object, "operation", "sync")
	if !found || err != nil {
		t.Fatalf("no operation.sync on the application: found=%v err=%v", found, err)
	}
	return sync
}

func boolAt(t *testing.T, entry map[string]any, key string) bool {
	t.Helper()
	value, ok := entry[key].(bool)
	if !ok {
		t.Fatalf("%s = %v, want a boolean", key, entry[key])
	}
	return value
}

func mapAt(t *testing.T, entry map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := entry[key].(map[string]any)
	if !ok {
		t.Fatalf("%s = %v, want a map", key, entry[key])
	}
	return value
}

func listAt(t *testing.T, entry map[string]any, key string) []any {
	t.Helper()
	value, ok := entry[key].([]any)
	if !ok {
		t.Fatalf("%s = %v, want a list", key, entry[key])
	}
	return value
}

func itemAt(t *testing.T, list []any, at int) map[string]any {
	t.Helper()
	value, ok := list[at].(map[string]any)
	if !ok {
		t.Fatalf("entry %d = %v, want a map", at, list[at])
	}
	return value
}

func TestSyncAsksTheControllerForAnOperation(t *testing.T) {
	client := actionClient(newApplication())

	result, err := Do(t.Context(), client, applicationRef(), ask(Sync))
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	if result.Action != "sync" {
		t.Fatalf("action = %q, want sync", result.Action)
	}
	got := readBack(t, client)
	operation, found, _ := unstructured.NestedMap(got.Object, "operation")
	if !found {
		t.Fatal("sync wrote no operation for the controller to pick up")
	}
	if _, ok := operation["sync"]; !ok {
		t.Fatalf("operation = %v, want a sync in it", operation)
	}
	who, _, _ := unstructured.NestedString(got.Object, "operation", "initiatedBy", "username")
	if who != fieldManager {
		t.Fatalf("initiatedBy.username = %q, want %q", who, fieldManager)
	}
}

func TestSyncLeavesTheSpecAlone(t *testing.T) {
	client := actionClient(newApplication())

	_, err := Do(t.Context(), client, applicationRef(), ask(Sync))
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	got := readBack(t, client)
	project, _, _ := unstructured.NestedString(got.Object, "spec", "project")
	if project != "default" {
		t.Fatalf("spec.project = %q, want it untouched", project)
	}
	if got.GetAnnotations()[refreshAnnotation] != "" {
		t.Fatal("sync also asked for a refresh")
	}
}

func TestAPlainSyncCarriesNoOptions(t *testing.T) {
	client := actionClient(newApplication())

	_, err := Do(t.Context(), client, applicationRef(), ask(Sync))
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	sync := syncOperationOf(t, readBack(t, client))
	if len(sync) != 0 {
		t.Fatalf("operation.sync = %v, want it empty when nothing was asked for", sync)
	}
}

func TestSyncCarriesPruneAndDryRun(t *testing.T) {
	client := actionClient(newApplication())

	_, err := Do(t.Context(), client, applicationRef(), Request{Action: Sync, Prune: true, DryRun: true})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	sync := syncOperationOf(t, readBack(t, client))
	if !boolAt(t, sync, "prune") {
		t.Fatal("prune was not carried into the operation")
	}
	if !boolAt(t, sync, "dryRun") {
		t.Fatal("dryRun was not carried into the operation")
	}
}

func TestReplaceAndServerSideApplyBecomeSyncOptions(t *testing.T) {
	client := actionClient(newApplication())

	_, err := Do(t.Context(), client, applicationRef(), Request{Action: Sync, Replace: true, ServerSide: true})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	sync := syncOperationOf(t, readBack(t, client))
	options := listAt(t, sync, "syncOptions")
	if len(options) != 2 || options[0] != "Replace=true" || options[1] != "ServerSideApply=true" {
		t.Fatalf("syncOptions = %v, want Replace and ServerSideApply", options)
	}
}

func TestForceKeepsTheHookStrategySoHooksStillRun(t *testing.T) {
	client := actionClient(newApplication())

	_, err := Do(t.Context(), client, applicationRef(), Request{Action: Sync, Force: true})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	sync := syncOperationOf(t, readBack(t, client))
	strategy := mapAt(t, sync, "syncStrategy")
	if _, apply := strategy["apply"]; apply {
		t.Fatal("force alone chose the apply strategy, which skips the hooks")
	}
	if !boolAt(t, mapAt(t, strategy, "hook"), "force") {
		t.Fatalf("syncStrategy.hook = %v, want force true", strategy["hook"])
	}
}

func TestApplyOnlyChoosesTheApplyStrategy(t *testing.T) {
	client := actionClient(newApplication())

	_, err := Do(t.Context(), client, applicationRef(), Request{Action: Sync, ApplyOnly: true, Force: true})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	sync := syncOperationOf(t, readBack(t, client))
	strategy := mapAt(t, sync, "syncStrategy")
	if !boolAt(t, mapAt(t, strategy, "apply"), "force") {
		t.Fatalf("syncStrategy.apply = %v, want force true", strategy["apply"])
	}
	if _, hook := strategy["hook"]; hook {
		t.Fatal("apply-only also asked for the hook strategy")
	}
}

func TestApplyOnlyWithoutForceSaysSo(t *testing.T) {
	client := actionClient(newApplication())

	_, err := Do(t.Context(), client, applicationRef(), Request{Action: Sync, ApplyOnly: true})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	sync := syncOperationOf(t, readBack(t, client))
	strategy := mapAt(t, sync, "syncStrategy")
	if boolAt(t, mapAt(t, strategy, "apply"), "force") {
		t.Fatalf("syncStrategy.apply = %v, want force false", strategy["apply"])
	}
}

func TestSelectiveSyncNamesOnlyTheMarkedResources(t *testing.T) {
	client := actionClient(newApplication())
	req := Request{Action: Sync, Resources: []Resource{
		{Group: "apps", Kind: "Deployment", Name: "web", Namespace: "shop"},
		{Kind: "Service", Name: "web"},
	}}

	_, err := Do(t.Context(), client, applicationRef(), req)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	sync := syncOperationOf(t, readBack(t, client))
	listed := listAt(t, sync, "resources")
	if len(listed) != 2 {
		t.Fatalf("resources = %v, want the two that were marked", sync["resources"])
	}
	first := itemAt(t, listed, 0)
	if first["group"] != "apps" || first["kind"] != "Deployment" || first["namespace"] != "shop" {
		t.Fatalf("first resource = %v, want the deployment with its group and namespace", first)
	}
	second := itemAt(t, listed, 1)
	if _, carried := second["group"]; carried {
		t.Fatalf("second resource = %v, want no empty group on a core kind", second)
	}
	if _, carried := second["namespace"]; carried {
		t.Fatalf("second resource = %v, want no empty namespace", second)
	}
}

func TestRefreshSetsTheAnnotationTheControllerWatches(t *testing.T) {
	client := actionClient(newApplication())

	result, err := Do(t.Context(), client, applicationRef(), ask(Refresh))
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	if result.Action != "refresh" {
		t.Fatalf("action = %q, want refresh", result.Action)
	}
	got := readBack(t, client)
	if got.GetAnnotations()[refreshAnnotation] != normalRefresh {
		t.Fatalf("annotation = %q, want %q", got.GetAnnotations()[refreshAnnotation], normalRefresh)
	}
}

func TestHardRefreshAsksForTheDeeperOne(t *testing.T) {
	client := actionClient(newApplication())

	result, err := Do(t.Context(), client, applicationRef(), ask(HardRefresh))
	if err != nil {
		t.Fatalf("hard refresh: %v", err)
	}

	if result.Action != "hard-refresh" {
		t.Fatalf("action = %q, want hard-refresh", result.Action)
	}
	got := readBack(t, client)
	if got.GetAnnotations()[refreshAnnotation] != hardRefresh {
		t.Fatalf("annotation = %q, want %q", got.GetAnnotations()[refreshAnnotation], hardRefresh)
	}
}

func TestRefreshStartsNoSync(t *testing.T) {
	client := actionClient(newApplication())

	_, err := Do(t.Context(), client, applicationRef(), ask(Refresh))
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	if _, found, _ := unstructured.NestedMap(readBack(t, client).Object, "operation"); found {
		t.Fatal("refresh queued a sync operation")
	}
}

func TestSuspendTurnsAutoSyncOffAndKeepsThePolicy(t *testing.T) {
	app := automating(newApplication(), map[string]any{"prune": true, "selfHeal": true})
	client := actionClient(app)

	_, err := Do(t.Context(), client, applicationRef(), ask(Suspend))
	if err != nil {
		t.Fatalf("suspend: %v", err)
	}

	automated, found, _ := unstructured.NestedMap(readBack(t, client).Object, "spec", "syncPolicy", "automated")
	if !found {
		t.Fatal("suspend removed the automated block instead of switching it off")
	}
	if boolAt(t, automated, "enabled") {
		t.Fatalf("enabled = %v, want false", automated["enabled"])
	}
	if !boolAt(t, automated, "prune") || !boolAt(t, automated, "selfHeal") {
		t.Fatalf("automated = %v, want prune and selfHeal kept", automated)
	}
}

func TestResumeTurnsAutoSyncBackOn(t *testing.T) {
	app := automating(newApplication(), map[string]any{"enabled": false, "prune": true})
	client := actionClient(app)

	_, err := Do(t.Context(), client, applicationRef(), ask(Resume))
	if err != nil {
		t.Fatalf("resume: %v", err)
	}

	automated, _, _ := unstructured.NestedMap(readBack(t, client).Object, "spec", "syncPolicy", "automated")
	if !boolAt(t, automated, "enabled") {
		t.Fatalf("enabled = %v, want true", automated["enabled"])
	}
	if !boolAt(t, automated, "prune") {
		t.Fatalf("automated = %v, want the prune setting kept", automated)
	}
}

func TestResumeOnAnApplicationThatNeverSyncedItselfUsesArgoDefaults(t *testing.T) {
	client := actionClient(newApplication())

	_, err := Do(t.Context(), client, applicationRef(), ask(Resume))
	if err != nil {
		t.Fatalf("resume: %v", err)
	}

	automated, found, _ := unstructured.NestedMap(readBack(t, client).Object, "spec", "syncPolicy", "automated")
	if !found {
		t.Fatal("resume wrote no automated block")
	}
	if !boolAt(t, automated, "enabled") {
		t.Fatalf("enabled = %v, want true", automated["enabled"])
	}
	if _, carried := automated["prune"]; carried {
		t.Fatalf("automated = %v, want prune left at the argo default", automated)
	}
	if _, carried := automated["selfHeal"]; carried {
		t.Fatalf("automated = %v, want selfHeal left at the argo default", automated)
	}
}

func TestSuspendRefusesWhenAutoSyncIsAlreadyOff(t *testing.T) {
	client := actionClient(newApplication())

	_, err := Do(t.Context(), client, applicationRef(), ask(Suspend))

	if !errors.Is(err, ErrRefused) {
		t.Fatalf("error = %v, want a refusal", err)
	}
	if _, found, _ := unstructured.NestedMap(readBack(t, client).Object, "spec", "syncPolicy"); found {
		t.Fatal("the refused suspend still wrote a sync policy")
	}
}

func TestResumeRefusesWhenAutoSyncIsAlreadyOn(t *testing.T) {
	client := actionClient(automating(newApplication(), map[string]any{}))

	_, err := Do(t.Context(), client, applicationRef(), ask(Resume))

	if !errors.Is(err, ErrRefused) {
		t.Fatalf("error = %v, want a refusal", err)
	}
}

func TestAnAutomatedBlockWithNoFlagCountsAsOn(t *testing.T) {
	app := automating(newApplication(), map[string]any{"prune": true})

	if !AutoSyncing(app) {
		t.Fatal("an automated block without an enabled flag should read as syncing itself")
	}
}

func TestSuspendSaysSoWhenTheControllerIgnoresTheFlag(t *testing.T) {
	client := actionClient(automating(newApplication(), map[string]any{"prune": true}))
	client.PrependReactor("patch", "applications", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, automating(newApplication(), map[string]any{"prune": true}), nil
	})

	_, err := Do(t.Context(), client, applicationRef(), ask(Suspend))

	if !errors.Is(err, ErrRefused) {
		t.Fatalf("error = %v, want a refusal naming the old argo cd", err)
	}
}

func TestTerminateStopsAnOperationInFlight(t *testing.T) {
	client := actionClient(operating(newApplication(), runningPhase))

	result, err := Do(t.Context(), client, applicationRef(), ask(Terminate))
	if err != nil {
		t.Fatalf("terminate: %v", err)
	}

	if result.Action != "terminate" {
		t.Fatalf("action = %q, want terminate", result.Action)
	}
	phase, _, _ := unstructured.NestedString(readBack(t, client).Object, "status", "operationState", "phase")
	if phase != terminatingPhase {
		t.Fatalf("phase = %q, want %q", phase, terminatingPhase)
	}
}

func TestTerminateRefusesWhenNothingIsRunning(t *testing.T) {
	client := actionClient(operating(newApplication(), "Succeeded"))

	_, err := Do(t.Context(), client, applicationRef(), ask(Terminate))

	if !errors.Is(err, ErrRefused) {
		t.Fatalf("error = %v, want a refusal", err)
	}
	phase, _, _ := unstructured.NestedString(readBack(t, client).Object, "status", "operationState", "phase")
	if phase != "Succeeded" {
		t.Fatalf("phase = %q, want the finished operation left alone", phase)
	}
}

func TestTerminateRefusesWhenThereWasNoOperationAtAll(t *testing.T) {
	client := actionClient(newApplication())

	_, err := Do(t.Context(), client, applicationRef(), ask(Terminate))

	if !errors.Is(err, ErrRefused) {
		t.Fatalf("error = %v, want a refusal", err)
	}
}

func TestRollbackSyncsTheRevisionThatWasDeployed(t *testing.T) {
	app := withHistory(newApplication(), historyOf(0, "aaaa"), historyOf(1, "bbbb"))
	client := actionClient(app)

	result, err := Do(t.Context(), client, applicationRef(), Request{Action: Rollback, Revision: 0})
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}

	if result.Action != "rollback" {
		t.Fatalf("action = %q, want rollback", result.Action)
	}
	sync := syncOperationOf(t, readBack(t, client))
	if sync["revision"] != "aaaa" {
		t.Fatalf("revision = %v, want the one from history entry 0", sync["revision"])
	}
	if mapAt(t, sync, "source")["path"] != "podinfo" {
		t.Fatalf("source = %v, want the source that entry recorded", sync["source"])
	}
}

func TestRollbackCarriesTheOptionsItWasGiven(t *testing.T) {
	client := actionClient(withHistory(newApplication(), historyOf(2, "cccc")))

	_, err := Do(t.Context(), client, applicationRef(), Request{Action: Rollback, Revision: 2, Prune: true, DryRun: true})
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}

	sync := syncOperationOf(t, readBack(t, client))
	if !boolAt(t, sync, "prune") || !boolAt(t, sync, "dryRun") {
		t.Fatalf("operation.sync = %v, want prune and dryRun carried", sync)
	}
}

func TestRollbackRefusesWhileTheApplicationSyncsItself(t *testing.T) {
	app := automating(withHistory(newApplication(), historyOf(0, "aaaa")), map[string]any{"selfHeal": true})
	client := actionClient(app)

	_, err := Do(t.Context(), client, applicationRef(), Request{Action: Rollback, Revision: 0})

	if !errors.Is(err, ErrRefused) {
		t.Fatalf("error = %v, want a refusal", err)
	}
	if _, found, _ := unstructured.NestedMap(readBack(t, client).Object, "operation"); found {
		t.Fatal("the refused rollback still queued an operation")
	}
}

func TestRollbackRefusesAnIDThatIsNotInTheHistory(t *testing.T) {
	client := actionClient(withHistory(newApplication(), historyOf(0, "aaaa")))

	_, err := Do(t.Context(), client, applicationRef(), Request{Action: Rollback, Revision: 7})

	if !errors.Is(err, ErrRefused) {
		t.Fatalf("error = %v, want a refusal", err)
	}
}

func TestRollbackRefusesAnApplicationWithNoHistory(t *testing.T) {
	client := actionClient(newApplication())

	_, err := Do(t.Context(), client, applicationRef(), Request{Action: Rollback, Revision: 0})

	if !errors.Is(err, ErrRefused) {
		t.Fatalf("error = %v, want a refusal", err)
	}
}

func TestActionsThatReadFirstReportAMissingApplication(t *testing.T) {
	for _, action := range []Action{Suspend, Resume, Terminate, Rollback} {
		client := actionClient()

		_, err := Do(t.Context(), client, applicationRef(), ask(action))

		if err == nil {
			t.Fatalf("%s: expected an error for an application that is not there", action)
		}
	}
}

func TestRejectsANonArgoGroup(t *testing.T) {
	client := actionClient()
	ref := api.ObjectRef{Group: "apps", Version: "v1", Resource: "deployments", Namespace: "d", Name: "web"}

	_, err := Do(t.Context(), client, ref, ask(Sync))

	want := `"apps" is not an argo cd resource group`
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestRejectsKindsTheControllerCannotSync(t *testing.T) {
	client := actionClient()
	ref := api.ObjectRef{Group: Group, Version: "v1alpha1", Resource: appProjects, Namespace: "argocd", Name: "default"}

	_, err := Do(t.Context(), client, ref, ask(Sync))

	want := `only applications can be operated on, not "appprojects"`
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestRejectsAnUnknownAction(t *testing.T) {
	client := actionClient(newApplication())

	_, err := Do(t.Context(), client, applicationRef(), ask(Action("explode")))

	want := `unknown action "explode"`
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestReportsWhatTheAPIServerSaid(t *testing.T) {
	client := actionClient()

	result, err := Do(t.Context(), client, applicationRef(), ask(Sync))

	if err == nil {
		t.Fatal("expected an error patching an application that is not there")
	}
	if result.Action != "sync" {
		t.Fatalf("action = %q, want the attempted action back", result.Action)
	}
}

func TestClusterScopedApplicationsPatchWithoutANamespace(t *testing.T) {
	app := newApplication()
	app.SetNamespace("")
	client := actionClient(app)
	ref := applicationRef()
	ref.Namespace = ""

	_, err := Do(t.Context(), client, ref, ask(Refresh))
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	got, getErr := client.Resource(applicationGVR).Get(t.Context(), "podinfo", metav1.GetOptions{})
	if getErr != nil {
		t.Fatalf("read back: %v", getErr)
	}
	if got.GetAnnotations()[refreshAnnotation] != normalRefresh {
		t.Fatal("the cluster-scoped patch did not land")
	}
}

func TestIsArgoGroup(t *testing.T) {
	cases := map[string]bool{
		"argoproj.io":             true,
		"apps":                    false,
		"":                        false,
		"argoproj.io.example.com": false,
	}
	for group, want := range cases {
		if got := IsArgoGroup(group); got != want {
			t.Fatalf("IsArgoGroup(%q) = %v, want %v", group, got, want)
		}
	}
}

func TestRollbackSkipsHistoryEntriesThatAreNotUsable(t *testing.T) {
	app := withHistory(newApplication(), "not a map", map[string]any{"revision": "no id"}, historyOf(3, "dddd"))
	client := actionClient(app)

	_, err := Do(t.Context(), client, applicationRef(), Request{Action: Rollback, Revision: 3})
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}

	if syncOperationOf(t, readBack(t, client))["revision"] != "dddd" {
		t.Fatal("the rollback did not reach the entry past the unusable ones")
	}
}
