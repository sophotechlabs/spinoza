package resources

import (
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	metadatafake "k8s.io/client-go/metadata/fake"
	k8stesting "k8s.io/client-go/testing"
)

type namespaceLister struct {
	mu     sync.Mutex
	listed int
}

func (nl *namespaceLister) count() int {
	nl.mu.Lock()
	defer nl.mu.Unlock()
	return nl.listed
}

func refusingNamespaces(t *testing.T, names ...string) (*metadatafake.FakeMetadataClient, *namespaceLister) {
	t.Helper()
	client := namespacesNamed(names...)
	calls := &namespaceLister{}
	client.PrependReactor("list", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		calls.mu.Lock()
		calls.listed++
		calls.mu.Unlock()
		return true, nil, errors.New(`namespaces is forbidden: User "editor" cannot list resource "namespaces"`)
	})
	return client, calls
}

func countingNamespaces(names ...string) (*metadatafake.FakeMetadataClient, *namespaceLister) {
	client := namespacesNamed(names...)
	calls := &namespaceLister{}
	client.PrependReactor("list", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		calls.mu.Lock()
		calls.listed++
		calls.mu.Unlock()
		return false, nil, nil
	})
	return client, calls
}

func namedAs(names ...string) func() []string {
	return func() []string {
		return names
	}
}

func TestAnAccountThatCannotListNamespacesSeesTheOnesItNamedAndCanRead(t *testing.T) {
	client, _ := refusingNamespaces(t)
	manager := NewManager(t.Context(), Deps{
		Metadata:   client,
		Perms:      boundTo("payments"),
		Namespace:  "default",
		Namespaces: namedAs("payments", "storefront"),
	})

	got := manager.Namespaces(t.Context())

	if !slices.Equal(got.Names, []string{"payments"}) {
		t.Fatalf("names = %v, want payments", got.Names)
	}
	if !got.Narrowed {
		t.Fatal("a list narrowed to what the account can read was not marked narrowed")
	}
	if got.Error != "" {
		t.Fatalf("error = %q, want none once a readable namespace was found", got.Error)
	}
}

func TestTheKubeconfigNamespaceIsTriedWithoutAnySetting(t *testing.T) {
	client, _ := refusingNamespaces(t)
	manager := NewManager(t.Context(), Deps{
		Metadata:  client,
		Perms:     boundTo("team-a"),
		Namespace: "team-a",
	})

	got := manager.Namespaces(t.Context())

	if !slices.Equal(got.Names, []string{"team-a"}) {
		t.Fatalf("names = %v, want the kubeconfig's own namespace", got.Names)
	}
}

func TestNamespacesTheAccountCanListAreTriedToo(t *testing.T) {
	manager := NewManager(t.Context(), Deps{
		Metadata: namespacesNamed("payments", "kube-system", "storefront"),
		Perms:    boundTo("storefront"),
	})

	got := manager.Namespaces(t.Context())

	if !slices.Equal(got.Names, []string{"storefront"}) {
		t.Fatalf("names = %v, want only storefront", got.Names)
	}
	if !got.Narrowed {
		t.Fatal("a listed cluster the account cannot read everywhere was not marked narrowed")
	}
}

func TestAnAccountThatReadsNoNamespaceIsToldWhereToNameThem(t *testing.T) {
	client, _ := refusingNamespaces(t)
	manager := NewManager(t.Context(), Deps{
		Metadata:   client,
		Perms:      boundTo("payments"),
		Namespace:  "default",
		Namespaces: namedAs("storefront"),
	})

	got := manager.Namespaces(t.Context())

	if got.Error != ErrNoReadableNamespace.Error() {
		t.Fatalf("error = %q, want the pointer to the setting", got.Error)
	}
	if !got.Narrowed {
		t.Fatal("an account that reads nothing was not marked narrowed")
	}
	if got.Names == nil || len(got.Names) != 0 {
		t.Fatalf("names = %#v, want an empty list", got.Names)
	}
}

func TestAnAccountThatReadsTheWholeClusterGetsEveryNamespaceUnmarked(t *testing.T) {
	manager := NewManager(t.Context(), Deps{
		Metadata:   namespacesNamed("payments", "kube-system"),
		Perms:      boundTo(""),
		Namespaces: namedAs("elsewhere"),
	})

	got := manager.Namespaces(t.Context())

	if !slices.Equal(got.Names, []string{"kube-system", "payments"}) {
		t.Fatalf("names = %v, want the listed namespaces only", got.Names)
	}
	if got.Narrowed {
		t.Fatal("a cluster-wide reader was marked narrowed")
	}
}

func TestANamespaceNamedInSettingsCountsWithoutARestart(t *testing.T) {
	client, _ := refusingNamespaces(t)
	var mu sync.Mutex
	named := []string{}
	manager := NewManager(t.Context(), Deps{
		Metadata: client,
		Perms:    boundTo("payments"),
		Namespaces: func() []string {
			mu.Lock()
			defer mu.Unlock()
			return slices.Clone(named)
		},
	})
	before := manager.Namespaces(t.Context())
	mu.Lock()
	named = []string{"payments"}
	mu.Unlock()

	after := manager.Namespaces(t.Context())

	if len(before.Names) != 0 {
		t.Fatalf("before = %v, want nothing", before.Names)
	}
	if !slices.Equal(after.Names, []string{"payments"}) {
		t.Fatalf("after = %v, want payments as soon as it was named", after.Names)
	}
}

func TestCandidatesDropBlanksAndRepeats(t *testing.T) {
	client, _ := refusingNamespaces(t)
	manager := NewManager(t.Context(), Deps{
		Metadata:   client,
		Namespace:  "payments",
		Namespaces: namedAs("", "storefront", "payments", "storefront"),
	})

	got := manager.candidates(t.Context())

	if !slices.Equal(got, []string{"payments", "storefront"}) {
		t.Fatalf("candidates = %v, want each name once and no blank", got)
	}
}

func TestTheNamespaceListIsReadOnceForSeveralScopeQuestions(t *testing.T) {
	client, calls := refusingNamespaces(t)
	manager := NewManager(t.Context(), Deps{
		Metadata: client,
		Perms:    boundTo("payments"),
	})

	for range 3 {
		manager.Scope(t.Context())
	}

	if calls.count() != 1 {
		t.Fatalf("listed %d times, want once within the cache window", calls.count())
	}
}

func TestTheNamespaceListIsReadAgainOnceItIsStale(t *testing.T) {
	client, calls := countingNamespaces("payments")
	manager := NewManager(t.Context(), Deps{
		Metadata: client,
		Perms:    boundTo("payments"),
	})
	now := time.Now()
	manager.now = func() time.Time {
		return now
	}
	manager.Scope(t.Context())
	now = now.Add(namespaceListTTL + time.Second)

	manager.Scope(t.Context())

	if calls.count() != 2 {
		t.Fatalf("listed %d times, want a second read after the cache went stale", calls.count())
	}
}

func TestTheNamespaceCacheWindowIsShortEnoughToNoticeANewNamespace(t *testing.T) {
	if namespaceListTTL < 5*time.Second || namespaceListTTL > time.Minute {
		t.Fatalf("namespaceListTTL = %s, want between 5s and a minute", namespaceListTTL)
	}
}

func TestYourOwnTablesAreNotFilteredByTheNarrowedList(t *testing.T) {
	client, _ := refusingNamespaces(t)
	manager := NewManager(t.Context(), Deps{
		Metadata: client,
		Perms:    boundTo("payments"),
	})

	seen := manager.filter(t.Context())

	if !seen.all {
		t.Fatal("tables for your own account were filtered by spinoza rather than left to the cluster")
	}
}

func TestASignedInAccountIsStillFilteredToWhatItCanRead(t *testing.T) {
	manager := NewManager(t.Context(), Deps{
		Metadata: namespacesNamed("payments", "storefront"),
		Perms:    boundTo("payments"),
	})

	seen := manager.filter(alice(t))

	if seen.all {
		t.Fatal("a signed-in account bound in one namespace was let through everywhere")
	}
	if !seen.allows("payments") || seen.allows("storefront") {
		t.Fatalf("filter = %+v, want payments only", seen)
	}
}
