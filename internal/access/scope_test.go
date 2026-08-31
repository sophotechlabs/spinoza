package access

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	authv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/auth"
)

type namespaceRules struct {
	mu      sync.Mutex
	allowed map[string]bool
	mute    map[string]bool
	asked   int
	silent  bool
}

func (nr *namespaceRules) answer(action k8stesting.Action) (bool, runtime.Object, error) {
	create, ok := action.(k8stesting.CreateAction)
	if !ok {
		return false, nil, nil
	}
	review, ok := create.GetObject().(*authv1.SelfSubjectAccessReview)
	if !ok {
		return false, nil, nil
	}
	attributes := *review.Spec.ResourceAttributes
	nr.mu.Lock()
	nr.asked++
	allowed := nr.allowed[attributes.Namespace]
	nr.mu.Unlock()
	nr.mu.Lock()
	quiet := nr.silent || nr.mute[attributes.Namespace]
	nr.mu.Unlock()
	if quiet {
		review.Status = authv1.SubjectAccessReviewStatus{
			Allowed:         false,
			EvaluationError: "the webhook authorizer did not answer",
		}
		return true, review, nil
	}
	review.Status = authv1.SubjectAccessReviewStatus{Allowed: allowed}
	return true, review, nil
}

func (nr *namespaceRules) count() int {
	nr.mu.Lock()
	defer nr.mu.Unlock()
	return nr.asked
}

func scopeService(t *testing.T, rules *namespaceRules) *Service {
	t.Helper()
	cs := k8sfake.NewClientset()
	cs.PrependReactor("create", "selfsubjectaccessreviews", rules.answer)
	return New(cs)
}

func asAlice(t *testing.T) context.Context {
	t.Helper()
	return auth.WithIdentity(t.Context(), auth.Identity{User: "alice", Groups: []string{"platform"}})
}

func names() []string {
	return []string{"payments", "storefront", "kube-system"}
}

func TestNobodySignedInReadsTheWholeCluster(t *testing.T) {
	rules := &namespaceRules{allowed: map[string]bool{}}

	got := scopeService(t, rules).Scope(t.Context(), names)

	if !got.Everywhere {
		t.Fatal("a window with nobody signed in was scoped, so a local spinoza would show nothing")
	}
	if rules.count() != 0 {
		t.Fatalf("asked %d times, want none", rules.count())
	}
}

func TestAServiceThatWasNeverWiredUpReadsEverything(t *testing.T) {
	var missing *Service

	if !missing.Scope(asAlice(t), names).Everywhere {
		t.Fatal("a manager with no permission service refused to show anything")
	}
}

func TestSomebodyWhoCanListPodsAnywhereReadsTheWholeCluster(t *testing.T) {
	rules := &namespaceRules{allowed: map[string]bool{"": true}}

	got := scopeService(t, rules).Scope(asAlice(t), names)

	if !got.Everywhere {
		t.Fatal("a cluster-wide reader was scoped to namespaces")
	}
	if rules.count() > 2 {
		t.Fatalf("asked %d times, want the cluster-wide question only", rules.count())
	}
}

func TestSomebodyBoundInTwoNamespacesReadsOnlyThose(t *testing.T) {
	rules := &namespaceRules{allowed: map[string]bool{"payments": true, "storefront": true}}

	got := scopeService(t, rules).Scope(asAlice(t), names)

	if got.Everywhere {
		t.Fatal("an account bound in two namespaces was given the whole cluster")
	}
	if strings.Join(got.Namespaces, ",") != "payments,storefront" {
		t.Fatalf("namespaces = %v, want the two they are bound in", got.Namespaces)
	}
}

func TestAnAnswerNobodyGaveIsNotAYes(t *testing.T) {
	rules := &namespaceRules{allowed: map[string]bool{"payments": true}, silent: true}

	got := scopeService(t, rules).Scope(asAlice(t), names)

	if got.Everywhere || len(got.Namespaces) != 0 {
		t.Fatalf("scope = %+v, want nothing when the cluster would not answer", got)
	}
	every := names()
	slices.Sort(every)
	if !slices.Equal(got.Undecided, every) {
		t.Fatalf("undecided = %v, want every namespace the cluster would not answer about", got.Undecided)
	}
}

func TestANamespaceTheClusterWouldNotDecideIsNotReportedAsRefused(t *testing.T) {
	rules := &namespaceRules{
		allowed: map[string]bool{"payments": true, "storefront": true},
		mute:    map[string]bool{"storefront": true},
	}

	got := scopeService(t, rules).Scope(asAlice(t), names)

	if !slices.Equal(got.Namespaces, []string{"payments"}) {
		t.Fatalf("namespaces = %v, want only the one the cluster allowed", got.Namespaces)
	}
	if !slices.Equal(got.Undecided, []string{"storefront"}) {
		t.Fatalf("undecided = %v, want the one the cluster would not answer about", got.Undecided)
	}
}

func TestANamespaceTheClusterRefusedIsNotCalledUndecided(t *testing.T) {
	rules := &namespaceRules{allowed: map[string]bool{"payments": true}}

	got := scopeService(t, rules).Scope(asAlice(t), names)

	if !slices.Equal(got.Namespaces, []string{"payments"}) {
		t.Fatalf("namespaces = %v, want the one namespace alice is bound in", got.Namespaces)
	}
	if len(got.Undecided) != 0 {
		t.Fatalf("undecided = %v, want none when the cluster answered every check", got.Undecided)
	}
}

func TestTheScopeHeldForARequestIsNotHandedToAnotherAccount(t *testing.T) {
	rules := &namespaceRules{allowed: map[string]bool{"payments": true}}
	held := scopeService(t, rules)
	slot := WithScopeSlot(t.Context())
	alice := auth.WithIdentity(slot, auth.Identity{User: "alice", Groups: []string{"platform"}})

	first := held.Scope(alice, names)
	rules.mu.Lock()
	rules.allowed = map[string]bool{"storefront": true}
	rules.mu.Unlock()
	bob := auth.WithIdentity(slot, auth.Identity{User: "bob"})
	second := held.Scope(bob, names)

	if !slices.Equal(first.Namespaces, []string{"payments"}) {
		t.Fatalf("alice read %v, want payments", first.Namespaces)
	}
	if !slices.Equal(second.Namespaces, []string{"storefront"}) {
		t.Fatalf("bob read %v, want the answer the cluster gave about bob", second.Namespaces)
	}
}

func TestTheScopeIsWorkedOutOncePerRequest(t *testing.T) {
	rules := &namespaceRules{allowed: map[string]bool{"payments": true}}
	held := scopeService(t, rules)
	ctx := WithScopeSlot(asAlice(t))

	first := held.Scope(ctx, names)
	round := rules.count()
	second := held.Scope(ctx, names)

	if !slices.Equal(first.Namespaces, second.Namespaces) {
		t.Fatalf("scope changed within one request: %v then %v", first.Namespaces, second.Namespaces)
	}
	if rules.count() != round {
		t.Fatalf("asked %d times then %d, want the answer kept for the request", round, rules.count())
	}
}

func TestASocketOpenAllDayAsksAgainOnceTheAnswerIsStale(t *testing.T) {
	rules := &namespaceRules{allowed: map[string]bool{"payments": true}}
	held := scopeService(t, rules)
	ctx := WithScopeSlot(asAlice(t))
	held.Scope(ctx, names)
	round := rules.count()

	held.now = func() time.Time { return time.Now().Add(time.Hour) }
	held.Scope(ctx, names)

	if rules.count() <= round {
		t.Fatal("a long-lived connection kept an answer the cluster may have changed")
	}
}

func TestOneUsersVerdictIsNeverHandedToAnother(t *testing.T) {
	rules := &namespaceRules{allowed: map[string]bool{"": true}}
	held := scopeService(t, rules)
	alice := auth.WithIdentity(t.Context(), auth.Identity{User: "alice"})
	bob := auth.WithIdentity(t.Context(), auth.Identity{User: "bob"})

	if !held.Ask(alice, Check{Verb: listVerb, Resource: pods}).Allowed {
		t.Fatal("alice was refused a check the cluster allows")
	}
	rules.mu.Lock()
	rules.allowed = map[string]bool{}
	rules.mu.Unlock()

	if held.Ask(bob, Check{Verb: listVerb, Resource: pods}).Allowed {
		t.Fatal("bob read the verdict the cluster gave about alice")
	}
}

func TestAManagerWithNoPermissionServiceReadsEverythingEvenMidRequest(t *testing.T) {
	var missing *Service

	got := missing.Scope(WithScopeSlot(asAlice(t)), names)

	if !got.Everywhere {
		t.Fatalf("scope = %+v, want the whole cluster", got)
	}
}
