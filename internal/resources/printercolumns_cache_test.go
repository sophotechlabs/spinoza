package resources

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

func definitionReads(t *testing.T, mgr *Manager, failFirst int) *atomic.Int32 {
	t.Helper()
	client, ok := mgr.dyn.(*fake.FakeDynamicClient)
	if !ok {
		t.Fatal("the manager is not holding the fake dynamic client")
	}
	var reads atomic.Int32
	client.PrependReactor("get", "customresourcedefinitions", func(k8stesting.Action) (bool, runtime.Object, error) {
		count := reads.Add(1)
		if int(count) <= failFirst {
			return true, nil, errors.New("etcdserver: request timed out")
		}
		return false, nil, nil
	})
	return &reads
}

func TestADefinitionThatCouldNotBeReadIsAskedForAgain(t *testing.T) {
	crd := crdWith(
		"v1",
		column("Ready", "string", `.status.conditions[?(@.type=="Ready")].status`),
		column("Revision", "string", ".status.lastAppliedRevision"),
	)
	mgr, cancel := crdServing(t, crd)
	defer cancel()
	reads := definitionReads(t, mgr, 1)

	sub := subscribeToKustomizations(t, mgr)

	if got := strings.Join(columnNames(sub.Columns()), ","); got != "Ready,Revision" {
		t.Fatalf("columns = %s, want the definition's once a later read succeeded", got)
	}
	if reads.Load() != 2 {
		t.Fatalf("definition reads = %d, want a second read after the failed one", reads.Load())
	}
}

func TestADefinitionThatKeepsFailingLeavesTheUsualTable(t *testing.T) {
	crd := crdWith("v1", column("Ready", "string", ".status.ready"))
	mgr, cancel := crdServing(t, crd)
	defer cancel()
	reads := definitionReads(t, mgr, 1000)

	sub := subscribeToKustomizations(t, mgr)

	if got := strings.Join(columnNames(sub.Columns()), ","); got != "Status" {
		t.Fatalf("columns = %s, want the usual table while the definition cannot be read", got)
	}
	if reads.Load() < 2 {
		t.Fatalf("definition reads = %d, want each layout question to ask again", reads.Load())
	}
}

func TestADefinitionThatDoesNotExistIsNotAskedForTwice(t *testing.T) {
	mgr, cancel := crdServing(t, nil)
	defer cancel()
	reads := definitionReads(t, mgr, 0)

	first := subscribeToKustomizations(t, mgr)
	first.Close()
	second := subscribeToKustomizations(t, mgr)

	if got := strings.Join(columnNames(second.Columns()), ","); got != "Status" {
		t.Fatalf("columns = %s, want the usual table", got)
	}
	if reads.Load() != 1 {
		t.Fatalf("definition reads = %d, want an absent definition remembered", reads.Load())
	}
}

func TestADefinitionReadFailureIsNotMistakenForNoDefinition(t *testing.T) {
	crd := crdWith("v1", column("Ready", "string", ".status.ready"))
	mgr, cancel := crdServing(t, crd)
	defer cancel()
	definitionReads(t, mgr, 1)

	_, found, err := mgr.crdLayout(t.Context(), kustomizationGVR)

	if err == nil || found {
		t.Fatalf("found = %v, err = %v; want the read failure reported", found, err)
	}
}

func TestAMissingDefinitionIsAnAnswerNotAFailure(t *testing.T) {
	mgr, cancel := crdServing(t, nil)
	defer cancel()

	_, found, err := mgr.crdLayout(t.Context(), kustomizationGVR)

	if err != nil || found {
		t.Fatalf("found = %v, err = %v; want no definition and no error", found, err)
	}
}
