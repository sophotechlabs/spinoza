package helm

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestHistoryIsLoadedInBoundedRevisionPages(t *testing.T) {
	objects := make([]runtime.Object, 0, historyPageSize+3)
	for revision := int64(1); revision <= historyPageSize+3; revision++ {
		spec := sampleRelease()
		spec.revision = revision
		spec.version = "6.9." + strconv.FormatInt(revision, 10)
		objects = append(objects, detailSecret(spec))
	}
	client := k8sfake.NewClientset(objects...)
	service := newService(client, nil, nil)
	client.ClearActions()

	first, err := service.History(context.Background(), "demo", "podinfo", historyPageSize+3)
	if err != nil {
		t.Fatalf("first history page: %v", err)
	}
	if len(first.Revisions) != historyPageSize {
		t.Fatalf("first page = %d revisions, want %d", len(first.Revisions), historyPageSize)
	}
	if first.Revisions[0].Revision != historyPageSize+3 {
		t.Fatalf("first revision = %d, want %d", first.Revisions[0].Revision, historyPageSize+3)
	}
	if first.Revisions[historyPageSize-1].Revision != 4 {
		t.Fatalf("last revision = %d, want 4", first.Revisions[historyPageSize-1].Revision)
	}
	if first.Next != 3 {
		t.Fatalf("next = %d, want 3", first.Next)
	}
	payloadReads := 0
	for _, action := range client.Actions() {
		_, ok := action.(k8stesting.GetAction)
		if ok {
			payloadReads++
		}
	}
	if payloadReads != historyPageSize {
		t.Fatalf("payload reads = %d, want only the %d visible revisions", payloadReads, historyPageSize)
	}

	second, secondErr := service.History(context.Background(), "demo", "podinfo", first.Next)
	if secondErr != nil {
		t.Fatalf("second history page: %v", secondErr)
	}
	if len(second.Revisions) != 3 {
		t.Fatalf("second page = %d revisions, want 3", len(second.Revisions))
	}
	if second.Revisions[0].Revision != 3 || second.Revisions[2].Revision != 1 {
		t.Fatalf("second page = %+v, want revisions 3 through 1", second.Revisions)
	}
	if second.Next != 0 {
		t.Fatalf("next = %d, want the end of history", second.Next)
	}
}

func TestDetailFetchesOnlyTheRequestedRevisionPayload(t *testing.T) {
	objects := make([]runtime.Object, 0, 3)
	for revision := int64(1); revision <= 3; revision++ {
		spec := sampleRelease()
		spec.revision = revision
		spec.version = "6.9." + strconv.FormatInt(revision, 10)
		objects = append(objects, detailSecret(spec))
	}
	client := k8sfake.NewClientset(objects...)
	service := newService(client, nil, nil)
	client.ClearActions()

	got, err := service.Detail(context.Background(), "demo", "podinfo", 2, resolver)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if got.Release.Revision != 2 || got.Release.ChartVersion != "6.9.2" {
		t.Fatalf("release = %+v, want revision 2", got.Release)
	}
	if len(got.History) != 0 {
		t.Fatalf("history = %d, want it loaded only on request", len(got.History))
	}

	gets := []k8stesting.GetAction{}
	for _, action := range client.Actions() {
		get, ok := action.(k8stesting.GetAction)
		if ok {
			gets = append(gets, get)
		}
	}
	if len(gets) != 1 {
		t.Fatalf("payload gets = %d, want exactly one", len(gets))
	}
	if gets[0].GetName() != "sh.helm.release.v1.podinfo.v2" {
		t.Fatalf("payload = %q, want only revision 2", gets[0].GetName())
	}
}

func TestHistorySkipsMissingRevisionNumbers(t *testing.T) {
	newest := sampleRelease()
	newest.revision = 15
	older := sampleRelease()
	older.revision = 7
	service := newService(k8sfake.NewClientset(detailSecret(newest), detailSecret(older)), nil, nil)

	got, err := service.History(context.Background(), "demo", "podinfo", 15)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(got.Revisions) != 2 {
		t.Fatalf("history = %+v, want the two stored revisions", got.Revisions)
	}
	if got.Revisions[0].Revision != 15 || got.Revisions[1].Revision != 7 {
		t.Fatalf("history = %+v, want revisions 15 and 7", got.Revisions)
	}
	if got.Next != 0 {
		t.Fatalf("next = %d, want the end of stored history", got.Next)
	}
}

func TestHistoryCursorJumpsAcrossLargeRevisionGaps(t *testing.T) {
	revisions := []int64{1000, 900, 800, 700, 600, 500, 400, 300, 200, 100, 10, 1}
	objects := make([]runtime.Object, 0, len(revisions))
	for _, revision := range revisions {
		spec := sampleRelease()
		spec.revision = revision
		objects = append(objects, detailSecret(spec))
	}
	service := newService(k8sfake.NewClientset(objects...), nil, nil)

	first, err := service.History(context.Background(), "demo", "podinfo", 1000)
	if err != nil {
		t.Fatalf("first history page: %v", err)
	}
	if len(first.Revisions) != historyPageSize || first.Next != 10 {
		t.Fatalf("first page = %+v, next %d; want ten revisions and cursor 10", first.Revisions, first.Next)
	}

	second, secondErr := service.History(context.Background(), "demo", "podinfo", first.Next)
	if secondErr != nil {
		t.Fatalf("second history page: %v", secondErr)
	}
	if len(second.Revisions) != 2 || second.Revisions[0].Revision != 10 {
		t.Fatalf("second page = %+v, want revisions 10 and 1", second.Revisions)
	}
}

func TestHistoryRejectsARevisionStoredByTwoDrivers(t *testing.T) {
	spec := sampleRelease()
	secret := detailSecret(spec)
	entry := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret.Name,
			Namespace: secret.Namespace,
			Labels:    secret.Labels,
		},
		Data: map[string]string{releaseKey: string(secret.Data[releaseKey])},
	}
	service := newService(k8sfake.NewClientset(secret, entry), nil, nil)

	_, err := service.History(context.Background(), "demo", "podinfo", spec.revision)

	if !errors.Is(err, errAmbiguousRevision) {
		t.Fatalf("history error = %v, want the duplicate revision rejected", err)
	}
}

func TestStoredRevisionMetadataHasADefensiveCap(t *testing.T) {
	objects := make([]runtime.Object, 0, 3)
	for revision := int64(1); revision <= 3; revision++ {
		spec := sampleRelease()
		spec.revision = revision
		objects = append(objects, detailSecret(spec))
	}
	service := newService(k8sfake.NewClientset(objects...), nil, nil)

	_, err := service.storedRefs(context.Background(), "demo", releaseSelector("podinfo"), 2)

	if err == nil || !strings.Contains(err.Error(), "more than 2") {
		t.Fatalf("metadata error = %v, want the cap reported", err)
	}
}
