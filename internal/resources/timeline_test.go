package resources

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type kept struct {
	mu    sync.Mutex
	notes []Note
}

func (k *kept) Note(note Note) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.notes = append(k.notes, note)
}

func (k *kept) all() []Note {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]Note{}, k.notes...)
}

func (k *kept) await(t *testing.T, want int) []Note {
	t.Helper()
	for range 200 {
		found := k.all()
		if len(found) >= want {
			return found
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("wanted %d notes, got %d", want, len(k.all()))
	return nil
}

var deployments = Kind{Group: "apps", Resource: "deployments"}

func scaledTo(t *testing.T, replicas int64) *unstructured.Unstructured {
	t.Helper()
	obj := newDeployment("default", "web")
	err := unstructured.SetNestedField(obj.Object, replicas, "spec", "replicas")
	if err != nil {
		t.Fatalf("set replicas: %v", err)
	}
	return obj
}

func deploymentStream(t *testing.T, mgr *Manager) *stream {
	t.Helper()
	st, ok := mgr.lookupStream(streamKey{
		gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	})
	if !ok {
		t.Fatal("the deployment stream was never built")
	}
	return st
}

func startRecording(t *testing.T, mgr *Manager) (*kept, *stream) {
	t.Helper()
	held := &kept{}
	mgr.Record(context.Background(), held, []Kind{deployments})
	t.Cleanup(mgr.StopRecording)
	waitForStreams(t, mgr, 1)
	return held, deploymentStream(t, mgr)
}

func TestRecordingWatchesTheKindsItWasGivenRatherThanWhatIsOnScreen(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()

	held, st := startRecording(t, mgr)

	st.publish("added", newDeployment("default", "api"))
	found := held.await(t, 1)
	if found[0].Verb != Added || found[0].Name != "api" {
		t.Fatalf("the first note was %+v", found[0])
	}
	if found[0].Kind != "Deployment" || found[0].Resource != "deployments" {
		t.Fatalf("the note did not say what it was about: %+v", found[0])
	}
}

func TestWhatWasAlreadyThereIsNotSomethingThatHappened(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()

	held, _ := startRecording(t, mgr)

	for _, note := range held.all() {
		if note.Name == "web" {
			t.Fatalf("the initial listing was recorded as history: %+v", note)
		}
	}
}

func TestTheInitialListingStillCountsAsWhatTheRowLookedLike(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()

	held, st := startRecording(t, mgr)

	st.publish("modified", newDeployment("default", "web"))

	if len(held.all()) != 0 {
		t.Fatalf("an unchanged row was recorded after the listing: %+v", held.all())
	}
}

func TestAKindTheClusterDoesNotHaveIsSkipped(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()
	held := &kept{}

	mgr.Record(context.Background(), held, []Kind{
		deployments,
		{Group: "gremlin.io", Resource: "gremlins"},
	})
	defer mgr.StopRecording()

	waitForStreams(t, mgr, 1)
}

func TestARowThatDidNotChangeIsNotRecordedTwice(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()
	held, st := startRecording(t, mgr)

	st.publish("modified", scaledTo(t, 9))
	held.await(t, 1)
	st.publish("modified", scaledTo(t, 9))

	if len(held.all()) != 1 {
		t.Fatalf("wanted the repeat rows ignored, got %d notes", len(held.all()))
	}
}

func TestARowThatChangedIsRecorded(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()
	held, st := startRecording(t, mgr)

	st.publish("modified", scaledTo(t, 9))

	found := held.await(t, 1)
	if found[0].Verb != Changed {
		t.Fatalf("the note was %+v", found[0])
	}
}

func TestADeletionIsRecordedAndForgotten(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()
	held, st := startRecording(t, mgr)

	st.publishDelete(newDeployment("default", "web"))
	st.publish("modified", newDeployment("default", "web"))

	found := held.await(t, 2)
	if found[0].Verb != Removed || found[0].Name != "web" {
		t.Fatalf("the deletion came out as %+v", found[0])
	}
	if found[1].Verb != Changed {
		t.Fatalf("the object coming back was recorded as %+v", found[1])
	}
}

func TestADeletionNotSeenBeforeIsStillRecorded(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()
	held, st := startRecording(t, mgr)

	st.publishDelete(newDeployment("default", "api"))

	found := held.await(t, 1)
	if found[0].Verb != Removed || found[0].Name != "api" {
		t.Fatalf("deletion = %+v, want the unseen object recorded", found[0])
	}
	if len(found[0].Was) != 0 {
		t.Fatalf("previous cells = %v, want none for an unseen object", found[0].Was)
	}
}

func TestNothingIsRecordedForAKindThatWasNotAskedFor(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()
	held := &kept{}
	mgr.Record(context.Background(), held, []Kind{{Group: "", Resource: "nodes"}})
	defer mgr.StopRecording()

	sub, err := mgr.Subscribe(context.Background(), "apps", "v1", "deployments", "default", 0, nil)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Close()
	deploymentStream(t, mgr).publish("modified", scaledTo(t, 9))

	for _, note := range held.all() {
		if note.Resource == "deployments" {
			t.Fatalf("a kind nobody asked for was recorded: %+v", note)
		}
	}
}

func TestNothingIsRecordedOnceRecordingStops(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()
	held, st := startRecording(t, mgr)

	mgr.StopRecording()
	st.publish("modified", scaledTo(t, 5))

	if len(held.all()) != 0 {
		t.Fatalf("wanted nothing after stopping, got %d notes", len(held.all()))
	}
}

func TestStoppingRecordingLetsTheWarmedKindsGo(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()
	mgr.Record(context.Background(), &kept{}, []Kind{deployments})
	waitForStreams(t, mgr, 1)

	mgr.StopRecording()

	waitForStreams(t, mgr, 0)
}

func TestTwoRowsThatLookAlikeAreToldApartByTheirObject(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()
	held, st := startRecording(t, mgr)

	st.publish("added", newDeployment("default", "api"))

	found := held.await(t, 1)
	if found[0].Name != "api" {
		t.Fatalf("the second object was recorded as %+v", found[0])
	}
}

func TestTheShapeOfARowFollowsItsContainers(t *testing.T) {
	steady := api.Row{UID: "one", Cells: []string{"1/1"}, Containers: []api.ContainerState{
		{Name: "app", State: "running", Ready: true, Restarts: 0},
	}}
	restarted := api.Row{UID: "one", Cells: []string{"1/1"}, Containers: []api.ContainerState{
		{Name: "app", State: "running", Ready: true, Restarts: 1},
	}}

	if shapeOf(steady) == shapeOf(restarted) {
		t.Fatal("a restart went unnoticed")
	}
}

func TestAChangeSaysWhatTheRowMovedFrom(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()

	held, st := startRecording(t, mgr)

	st.publish("modified", scaledTo(t, 9))

	found := held.await(t, 1)
	if len(found[0].Was) == 0 {
		t.Fatal("a change came back with nothing to compare against")
	}
	if found[0].Was[0] == found[0].Cells[0] {
		t.Fatalf("it moved from %v to %v", found[0].Was, found[0].Cells)
	}
}

func TestSomethingSeenForTheFirstTimeHasNothingToCompareAgainst(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()

	held, st := startRecording(t, mgr)

	st.publish("added", newDeployment("default", "api"))

	found := held.await(t, 1)
	if len(found[0].Was) != 0 {
		t.Fatalf("a new object claimed to have moved from %v", found[0].Was)
	}
}

func TestADeletionSaysWhatWasThereBeforeIt(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()

	held, st := startRecording(t, mgr)

	st.publishDelete(newDeployment("default", "web"))

	found := held.await(t, 1)
	if len(found[0].Was) == 0 {
		t.Fatal("a deletion said nothing about what was deleted")
	}
}
