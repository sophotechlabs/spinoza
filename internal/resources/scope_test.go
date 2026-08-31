package resources

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	authv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	metadatafake "k8s.io/client-go/metadata/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/access"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/auth"
	"github.com/sophotechlabs/spinoza/internal/discovery"
)

func boundTo(namespaces ...string) *access.Service {
	allowed := map[string]bool{}
	for _, one := range namespaces {
		allowed[one] = true
	}
	cs := k8sfake.NewClientset()
	cs.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		create, ok := action.(k8stesting.CreateAction)
		if !ok {
			return false, nil, nil
		}
		review, ok := create.GetObject().(*authv1.SelfSubjectAccessReview)
		if !ok {
			return false, nil, nil
		}
		review.Status = authv1.SubjectAccessReviewStatus{
			Allowed: allowed[review.Spec.ResourceAttributes.Namespace],
		}
		return true, review, nil
	})
	return access.New(cs)
}

func shiftingBinding(namespaces ...string) (*access.Service, func(...string)) {
	var mu sync.Mutex
	allowed := map[string]bool{}
	rebind := func(names ...string) {
		mu.Lock()
		defer mu.Unlock()
		allowed = map[string]bool{}
		for _, one := range names {
			allowed[one] = true
		}
	}
	rebind(namespaces...)
	cs := k8sfake.NewClientset()
	cs.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		create, ok := action.(k8stesting.CreateAction)
		if !ok {
			return false, nil, nil
		}
		review, ok := create.GetObject().(*authv1.SelfSubjectAccessReview)
		if !ok {
			return false, nil, nil
		}
		mu.Lock()
		defer mu.Unlock()
		review.Status = authv1.SubjectAccessReviewStatus{
			Allowed: allowed[review.Spec.ResourceAttributes.Namespace],
		}
		return true, review, nil
	})
	return access.New(cs), rebind
}

func namespacesNamed(names ...string) *metadatafake.FakeMetadataClient {
	objs := make([]runtime.Object, 0, len(names))
	for _, name := range names {
		objs = append(objs, meta("", "v1", "Namespace", "", name))
	}
	return metadatafake.NewSimpleMetadataClient(searchScheme(), objs...)
}

func alice(t *testing.T) context.Context {
	t.Helper()
	return auth.WithIdentity(t.Context(), auth.Identity{User: "alice"})
}

func TestAFilterOverEveryNamespaceLetsEverythingThrough(t *testing.T) {
	seen := filterFor(api.Scope{Everywhere: true})

	if !seen.allows("payments") || !seen.allows("") {
		t.Fatal("a cluster-wide reader was held back")
	}
	if seen.only() != "" {
		t.Fatalf("only = %q, want none, so the informer is not narrowed", seen.only())
	}
}

func TestAFilterOverNamedNamespacesLetsOnlyThoseThrough(t *testing.T) {
	seen := filterFor(api.Scope{Namespaces: []string{"payments", "storefront"}})

	if !seen.allows("payments") {
		t.Fatal("a namespace the account reads was held back")
	}
	if seen.allows("kube-system") {
		t.Fatal("a namespace the account cannot read was let through")
	}
	if seen.allows("") {
		t.Fatal("an object in no namespace was let through to a scoped account")
	}
	if seen.only() != "" {
		t.Fatalf("only = %q, want none while there is more than one", seen.only())
	}
}

func TestOneNamespaceIsPassedStraightToTheCacheLookup(t *testing.T) {
	if got := filterFor(api.Scope{Namespaces: []string{"payments"}}).only(); got != "payments" {
		t.Fatalf("only = %q, want payments", got)
	}
	if got := filterFor(api.Scope{}).only(); got != "" {
		t.Fatalf("only = %q, want none when the account reads nothing", got)
	}
}

func TestWhatAScopedAccountMaySubscribeTo(t *testing.T) {
	mgr := &Manager{}
	deployments := testDescs()[discovery.Key("apps", "v1", "deployments")]
	nodes := testDescs()[discovery.Key("", "v1", "nodes")]
	scoped := filterFor(api.Scope{Namespaces: []string{"payments"}})

	cases := []struct {
		name      string
		seen      nsFilter
		desc      api.ResourceDescriptor
		namespace string
		want      error
	}{
		{name: "everything, anywhere", seen: everything(), desc: nodes, namespace: "", want: nil},
		{name: "a namespace they read", seen: scoped, desc: deployments, namespace: "payments", want: nil},
		{name: "every namespace they read", seen: scoped, desc: deployments, namespace: "", want: nil},
		{name: "a namespace they do not", seen: scoped, desc: deployments, namespace: "kube-system", want: ErrOutOfScope},
		{name: "a kind in no namespace", seen: scoped, desc: nodes, namespace: "", want: ErrClusterWide},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := mgr.admits(tc.seen, tc.desc, tc.namespace)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("refused with %v, want it allowed", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestSearchHitsOutsideTheAccountsNamespacesAreDropped(t *testing.T) {
	mgr := &Manager{}
	found := api.SearchResults{Hits: []api.SearchHit{
		{Name: "web", Namespace: "payments"},
		{Name: "till", Namespace: "storefront"},
		{Name: "node-1", Namespace: ""},
	}}

	kept := mgr.scopedHits(filterFor(api.Scope{Namespaces: []string{"payments"}}), found)

	if len(kept.Hits) != 1 || kept.Hits[0].Name != "web" {
		t.Fatalf("hits = %+v, want only the one in payments", kept.Hits)
	}
	if len(mgr.scopedHits(everything(), found).Hits) != 3 {
		t.Fatal("a cluster-wide reader lost hits")
	}
}

func TestAWindowWithNobodySignedInReadsTheWholeCluster(t *testing.T) {
	mgr := &Manager{}

	if !mgr.Scope(context.Background()).Everywhere {
		t.Fatal("a local window was scoped")
	}
}

func TestASubscriptionShowsOnlyTheRowsTheAccountMayRead(t *testing.T) {
	dyn := newClient(t, newDeployment("payments", "web"), newDeployment("kube-system", "coredns"))
	mgr := NewManager(t.Context(), Deps{
		Dynamic:     dyn,
		Clientset:   k8sfake.NewClientset(),
		Metadata:    namespacesNamed("payments", "kube-system"),
		Perms:       boundTo("payments"),
		Categories:  []api.Category{{Name: "Workloads"}},
		Descriptors: testDescs(),
		Limits:      Limits{IdleGrace: time.Millisecond, SyncTimeout: 2 * time.Second},
	})

	sub, err := mgr.Subscribe(alice(t), "apps", "v1", "deployments", "", 0, nil)
	if err != nil {
		t.Fatalf("subscribing: %v", err)
	}
	defer sub.Close()

	names := make([]string, 0, len(sub.Rows))
	for _, row := range sub.Rows {
		names = append(names, row.Name)
	}
	if strings.Join(names, ",") != "web" {
		t.Fatalf("rows = %v, want only the one in payments", names)
	}
}

func TestASubscriptionToAKindInNoNamespaceIsRefusedForAScopedAccount(t *testing.T) {
	dyn := newClient(t, newNode("node-1"))
	mgr := NewManager(t.Context(), Deps{
		Dynamic:     dyn,
		Clientset:   k8sfake.NewClientset(),
		Metadata:    namespacesNamed("payments", "kube-system"),
		Perms:       boundTo("payments"),
		Categories:  []api.Category{{Name: "Cluster"}},
		Descriptors: testDescs(),
		Limits:      Limits{IdleGrace: time.Millisecond, SyncTimeout: 2 * time.Second},
	})

	_, err := mgr.Subscribe(alice(t), "", "v1", "nodes", "", 0, nil)
	if !errors.Is(err, ErrClusterWide) {
		t.Fatalf("error = %v, want %v", err, ErrClusterWide)
	}
}

func TestAScopedAccountCannotLeaseTheWholeCache(t *testing.T) {
	dyn := newClient(t, newDeployment("payments", "web"))
	mgr := NewManager(t.Context(), Deps{
		Dynamic:     dyn,
		Clientset:   k8sfake.NewClientset(),
		Metadata:    namespacesNamed("payments", "kube-system"),
		Perms:       boundTo("payments"),
		Categories:  []api.Category{{Name: "Workloads"}},
		Descriptors: testDescs(),
		Limits:      Limits{IdleGrace: time.Millisecond, SyncTimeout: 2 * time.Second},
	})

	_, err := mgr.Lease(alice(t), testDescs()[discovery.Key("apps", "v1", "deployments")])
	if !errors.Is(err, ErrClusterWide) {
		t.Fatalf("error = %v, want %v", err, ErrClusterWide)
	}
}

func TestADeltaForANamespaceTheAccountCannotReadIsNotSent(t *testing.T) {
	scoped := newSubscriber("", 0, filterFor(api.Scope{Namespaces: []string{"payments"}}))

	if !scoped.wants(Event{Kind: "added", Row: api.Row{Namespace: "payments"}}) {
		t.Fatal("a row in the account's own namespace was dropped")
	}
	if scoped.wants(Event{Kind: "added", Row: api.Row{Namespace: "kube-system"}}) {
		t.Fatal("a row from another team reached a scoped account")
	}
	if !scoped.wants(Event{Kind: "deleted"}) {
		t.Fatal("a deletion was dropped, so the row would never leave the table")
	}
}

func TestATableAlreadyOpenReadsTheScopeAgainInsteadOfKeepingTheOneItOpenedWith(t *testing.T) {
	perms, rebind := shiftingBinding("payments")
	dyn := newClient(t, newDeployment("payments", "web"))
	mgr := NewManager(t.Context(), Deps{
		Dynamic:     dyn,
		Clientset:   k8sfake.NewClientset(),
		Metadata:    namespacesNamed("payments", "kube-system"),
		Perms:       perms,
		Categories:  []api.Category{{Name: "Workloads"}},
		Descriptors: testDescs(),
		Limits:      Limits{IdleGrace: time.Millisecond, SyncTimeout: 2 * time.Second},
	})

	sub, err := mgr.Subscribe(alice(t), "apps", "v1", "deployments", "", 0, nil)
	if err != nil {
		t.Fatalf("subscribing: %v", err)
	}
	defer sub.Close()
	if len(sub.Rows) != 1 {
		t.Fatalf("rows = %d, want the one row in the namespace alice was bound to", len(sub.Rows))
	}

	rebind()
	sub.Refresh(auth.WithIdentity(t.Context(), auth.Identity{User: "bob"}))

	rows, _, snapErr := sub.Snapshot()
	if snapErr != nil {
		t.Fatalf("snapshot: %v", snapErr)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %d, want none once the account lost the namespace", len(rows))
	}
	if sub.entry.wants(Event{Kind: "added", Row: api.Row{Namespace: "payments"}}) {
		t.Fatal("a delta still reached a table whose account no longer reads that namespace")
	}
}
