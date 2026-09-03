package resources

import (
	"errors"
	"slices"
	"sync"
	"testing"

	authv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/access"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/auth"
)

type resourceAuthorizer struct {
	mu      sync.Mutex
	asked   []authv1.ResourceAttributes
	refused map[string]bool
}

func (a *resourceAuthorizer) answer(action k8stesting.Action) (bool, runtime.Object, error) {
	create, ok := action.(k8stesting.CreateAction)
	if !ok {
		return true, nil, errors.New("action is not a create")
	}
	review, ok := create.GetObject().(*authv1.SelfSubjectAccessReview)
	if !ok {
		return true, nil, errors.New("created object is not an access review")
	}
	attributes := *review.Spec.ResourceAttributes
	a.mu.Lock()
	a.asked = append(a.asked, attributes)
	denied := a.refused[attributes.Verb+" "+attributes.Namespace]
	a.mu.Unlock()
	review.Status.Allowed = !denied
	if denied {
		review.Status.Reason = "revoked"
	}
	return true, review, nil
}

func (a *resourceAuthorizer) questions() []authv1.ResourceAttributes {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.asked)
}

func managerWithAuthorizer(rules *resourceAuthorizer) *Manager {
	cs := k8sfake.NewClientset()
	cs.PrependReactor("create", "selfsubjectaccessreviews", rules.answer)
	return &Manager{perms: access.New(cs)}
}

func TestManagerAuthorizationUsesCachedAndFreshReviews(t *testing.T) {
	rules := &resourceAuthorizer{refused: map[string]bool{}}
	mgr := managerWithAuthorizer(rules)
	ctx := auth.WithIdentity(t.Context(), auth.Identity{User: "alice"})
	check := access.Check{Verb: "get", Resource: "pods", Namespace: "prod"}

	if err := mgr.Authorize(ctx, check); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if err := mgr.Authorize(ctx, check); err != nil {
		t.Fatalf("cached authorize: %v", err)
	}
	if questions := rules.questions(); len(questions) != 1 {
		t.Fatalf("cached questions = %d, want 1", len(questions))
	}
	if err := mgr.Reauthorize(ctx, check); err != nil {
		t.Fatalf("reauthorize: %v", err)
	}
	if questions := rules.questions(); len(questions) != 2 {
		t.Fatalf("fresh questions = %d, want 2", len(questions))
	}
}

func TestManagerAuthorizationNeedsNoReviewerOutsideClusterMode(t *testing.T) {
	mgr := &Manager{}
	check := access.Check{Verb: "get", Resource: "pods", Namespace: "prod"}

	if err := mgr.Authorize(t.Context(), check); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if err := mgr.Reauthorize(t.Context(), check); err != nil {
		t.Fatalf("reauthorize: %v", err)
	}
}

func TestImpersonatedCountsNeedAMetadataClient(t *testing.T) {
	mgr := &Manager{}
	ctx := auth.WithIdentity(t.Context(), auth.Identity{User: "alice"})

	counts := mgr.Counts(ctx)

	if counts.Counts == nil || len(counts.Counts) != 0 {
		t.Fatalf("counts = %v, want a known empty result without a metadata client", counts.Counts)
	}
}

func securedDeploymentDescriptor() api.ResourceDescriptor {
	return api.ResourceDescriptor{
		Group:      "apps",
		Resource:   "deployments",
		Kind:       "Deployment",
		Namespaced: true,
	}
}

func TestSharedFeedAdmissionChecksExactListAndWatchAccess(t *testing.T) {
	rules := &resourceAuthorizer{refused: map[string]bool{}}
	mgr := managerWithAuthorizer(rules)
	ctx := auth.WithIdentity(t.Context(), auth.Identity{User: "alice"})
	checks, err := mgr.requireRead(ctx, everything(), securedDeploymentDescriptor(), "prod")
	if err != nil {
		t.Fatalf("require read: %v", err)
	}
	if len(checks) != 2 {
		t.Fatalf("checks = %+v", checks)
	}
	questions := rules.questions()
	if len(questions) != 2 {
		t.Fatalf("questions = %+v", questions)
	}
	for _, question := range questions {
		if question.Group != "apps" || question.Resource != "deployments" || question.Namespace != "prod" {
			t.Fatalf("question = %+v", question)
		}
	}
	verbs := []string{questions[0].Verb, questions[1].Verb}
	slices.Sort(verbs)
	if !slices.Equal(verbs, []string{"list", "watch"}) {
		t.Fatalf("verbs = %q", verbs)
	}
}

func TestNamespaceScopedFeedChecksEveryAdmittedNamespace(t *testing.T) {
	rules := &resourceAuthorizer{refused: map[string]bool{}}
	mgr := managerWithAuthorizer(rules)
	ctx := auth.WithIdentity(t.Context(), auth.Identity{User: "alice"})
	seen := filterFor(api.Scope{Namespaces: []string{"storefront", "payments"}})
	checks, err := mgr.requireRead(ctx, seen, securedDeploymentDescriptor(), "")
	if err != nil {
		t.Fatalf("require read: %v", err)
	}
	want := []access.Check{
		{Verb: "list", Group: "apps", Resource: "deployments", Namespace: "payments"},
		{Verb: "watch", Group: "apps", Resource: "deployments", Namespace: "payments"},
		{Verb: "list", Group: "apps", Resource: "deployments", Namespace: "storefront"},
		{Verb: "watch", Group: "apps", Resource: "deployments", Namespace: "storefront"},
	}
	if !slices.Equal(checks, want) {
		t.Fatalf("checks = %+v, want %+v", checks, want)
	}
}

func TestSharedFeedAdmissionFailsOnOneDeniedPermission(t *testing.T) {
	rules := &resourceAuthorizer{refused: map[string]bool{"watch prod": true}}
	mgr := managerWithAuthorizer(rules)
	ctx := auth.WithIdentity(t.Context(), auth.Identity{User: "alice"})
	_, err := mgr.requireRead(ctx, everything(), securedDeploymentDescriptor(), "prod")
	if !errors.Is(err, access.ErrDenied) {
		t.Fatalf("error = %v", err)
	}
	if err.Error() != "kubernetes authorization denied: revoked" {
		t.Fatalf("error = %q", err)
	}
}

func TestSubscriptionReauthorizationBypassesTheAdmissionCache(t *testing.T) {
	rules := &resourceAuthorizer{refused: map[string]bool{}}
	mgr := managerWithAuthorizer(rules)
	ctx := auth.WithIdentity(t.Context(), auth.Identity{User: "alice"})
	checks, err := mgr.requireRead(ctx, everything(), securedDeploymentDescriptor(), "prod")
	if err != nil {
		t.Fatalf("admission: %v", err)
	}
	rules.mu.Lock()
	rules.refused["watch prod"] = true
	rules.mu.Unlock()
	sub := &Subscription{owner: mgr, checks: checks}
	err = sub.Reauthorize(ctx)
	if !errors.Is(err, access.ErrDenied) {
		t.Fatalf("reauthorize error = %v", err)
	}
	if len(rules.questions()) != 4 {
		t.Fatalf("questions = %d, want admission and fresh reauthorization", len(rules.questions()))
	}
}
