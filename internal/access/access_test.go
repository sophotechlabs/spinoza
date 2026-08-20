package access

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
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

// authorizer answers access reviews the way an apiserver does, recording what it
// was asked so a test can check the question and not only the answer.
type authorizer struct {
	mu     sync.Mutex
	asked  []authv1.ResourceAttributes
	refuse map[string]string
	broken bool
	unsure bool
}

func (a *authorizer) answer(action k8stesting.Action) (bool, runtime.Object, error) {
	create, ok := action.(k8stesting.CreateAction)
	if !ok {
		return false, nil, nil
	}
	review, ok := create.GetObject().(*authv1.SelfSubjectAccessReview)
	if !ok {
		return false, nil, nil
	}
	attributes := *review.Spec.ResourceAttributes
	a.mu.Lock()
	a.asked = append(a.asked, attributes)
	reason, refused := a.refuse[key(attributes)]
	a.mu.Unlock()

	if a.broken {
		return true, nil, errors.New("the apiserver would not answer")
	}
	if a.unsure {
		review.Status = authv1.SubjectAccessReviewStatus{
			Allowed:         false,
			EvaluationError: "the webhook authorizer did not answer",
		}
		return true, review, nil
	}
	review.Status = authv1.SubjectAccessReviewStatus{Allowed: !refused, Reason: reason}
	return true, review, nil
}

func (a *authorizer) questions() []authv1.ResourceAttributes {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]authv1.ResourceAttributes{}, a.asked...)
}

func (a *authorizer) count() int {
	return len(a.questions())
}

// key names a question the way the tests talk about one: verb, group, resource,
// subresource.
func key(attributes authv1.ResourceAttributes) string {
	parts := []string{attributes.Verb, attributes.Group, attributes.Resource, attributes.Subresource}
	return strings.Join(parts, " ")
}

func serviceFor(t *testing.T, auth *authorizer) *Service {
	t.Helper()
	cs := k8sfake.NewClientset()
	cs.PrependReactor("create", "selfsubjectaccessreviews", auth.answer)
	return New(cs)
}

func refusing(pairs map[string]string) *authorizer {
	return &authorizer{refuse: pairs}
}

func podRef() api.ObjectRef {
	return api.ObjectRef{Version: "v1", Resource: "pods", Namespace: "prod", Name: "web"}
}

func deploymentRef() api.ObjectRef {
	return api.ObjectRef{
		Group:     "apps",
		Version:   "v1",
		Resource:  "deployments",
		Namespace: "prod",
		Name:      "web",
	}
}

func nodeRef() api.ObjectRef {
	return api.ObjectRef{Version: "v1", Resource: "nodes", Name: "node-1"}
}

func reasons(result api.Access) map[string]string {
	out := map[string]string{}
	for _, refusal := range result.Refused {
		out[refusal.Capability] = refusal.Reason
	}
	return out
}

func asked(auth *authorizer) map[string]authv1.ResourceAttributes {
	out := map[string]authv1.ResourceAttributes{}
	for _, one := range auth.questions() {
		out[key(one)] = one
	}
	return out
}

func TestAPermittedObjectRefusesNothing(t *testing.T) {
	service := serviceFor(t, refusing(nil))

	result := service.Review(t.Context(), podRef())

	if len(result.Refused) != 0 {
		t.Fatalf("refused = %v, want nothing held back", result.Refused)
	}
}

func TestARefusalCarriesTheClustersOwnReason(t *testing.T) {
	service := serviceFor(t, refusing(map[string]string{
		"get  pods log": `requires one of ["container.pods.getLogs"] permission(s) in Cloud IAM`,
	}))

	got := reasons(service.Review(t.Context(), podRef()))

	if !strings.Contains(got[Logs], "container.pods.getLogs") {
		t.Fatalf("logs reason = %q, want the cluster's own words", got[Logs])
	}
	if _, refused := got[Exec]; refused {
		t.Fatalf("exec was withheld too: %v", got)
	}
}

func TestARefusalWithoutAReasonStillSaysWhat(t *testing.T) {
	service := serviceFor(t, refusing(map[string]string{"delete  pods ": ""}))

	got := reasons(service.Review(t.Context(), podRef()))

	if got[Delete] != "you may not delete pods here" {
		t.Fatalf("delete reason = %q", got[Delete])
	}
}

func TestARefusedSubresourceIsNamedInFull(t *testing.T) {
	service := serviceFor(t, refusing(map[string]string{"create  pods portforward": ""}))

	got := reasons(service.Review(t.Context(), podRef()))

	if got[PortForward] != "you may not create pods/portforward here" {
		t.Fatalf("port forward reason = %q", got[PortForward])
	}
}

func TestARefusedGroupedResourceIsNamedInFull(t *testing.T) {
	service := serviceFor(t, refusing(map[string]string{"delete apps deployments ": ""}))

	got := reasons(service.Review(t.Context(), deploymentRef()))

	if got[Delete] != "you may not delete apps/deployments here" {
		t.Fatalf("delete reason = %q", got[Delete])
	}
}

func TestAPodIsAskedAboutItsOwnPanels(t *testing.T) {
	auth := refusing(nil)
	service := serviceFor(t, auth)

	service.Review(t.Context(), podRef())

	want := map[string]bool{
		"get  pods log":            true,
		"create  pods exec":        true,
		"create  pods portforward": true,
		"update  pods ":            true,
		"delete  pods ":            true,
	}
	for _, one := range auth.questions() {
		delete(want, key(one))
		if one.Name != "web" || one.Namespace != "prod" {
			t.Fatalf("asked about %+v, want the selected pod", one)
		}
	}
	if len(want) != 0 {
		t.Fatalf("never asked about %v", want)
	}
}

func TestAWorkloadIsAskedAboutScalingAndItsPodsLogs(t *testing.T) {
	auth := refusing(nil)
	service := serviceFor(t, auth)

	service.Review(t.Context(), deploymentRef())

	questions := asked(auth)
	scale, ok := questions["patch apps deployments scale"]
	if !ok {
		t.Fatalf("scaling was never asked about: %v", questions)
	}
	if scale.Name != "web" {
		t.Fatalf("scale asked about %q, want the workload", scale.Name)
	}
	logs, ok := questions["get  pods log"]
	if !ok {
		t.Fatalf("the pods' logs were never asked about: %v", questions)
	}
	// The workload's own name is not a pod name, so asking with it would be a
	// different question from the one the log stream will put.
	if logs.Name != "" {
		t.Fatalf("logs asked about pod %q, want any pod in the namespace", logs.Name)
	}
	if logs.Namespace != "prod" {
		t.Fatalf("logs asked in %q", logs.Namespace)
	}
}

func TestAWorkloadIsNotAskedAboutExec(t *testing.T) {
	auth := refusing(nil)
	service := serviceFor(t, auth)

	service.Review(t.Context(), deploymentRef())

	if _, ok := asked(auth)["create  pods exec"]; ok {
		t.Fatal("a deployment was asked about exec, which is a question about a pod")
	}
}

func TestANodeIsAskedAboutCordonAndTheReadsADrainNeeds(t *testing.T) {
	auth := refusing(nil)
	service := serviceFor(t, auth)

	service.Review(t.Context(), nodeRef())

	questions := asked(auth)
	if _, ok := questions["patch  nodes "]; !ok {
		t.Fatalf("cordoning was never asked about: %v", questions)
	}
	// A drain reads every pod on the node before it cordons, and that read is
	// cluster wide because the pods can be anywhere.
	listing, ok := questions["list  pods "]
	if !ok {
		t.Fatalf("the pod list a drain needs was never asked about: %v", questions)
	}
	if listing.Namespace != "" {
		t.Fatalf("the pod list was asked in %q, want every namespace", listing.Namespace)
	}
}

// Eviction is per pod, and a drain reports each pod separately, so a user who
// may evict in some namespaces and not others must keep the button.
func TestANodeIsNotAskedAboutEvictingEverywhere(t *testing.T) {
	auth := refusing(nil)
	service := serviceFor(t, auth)

	service.Review(t.Context(), nodeRef())

	if _, ok := asked(auth)["create  pods eviction"]; ok {
		t.Fatal("draining was gated on being able to evict in every namespace")
	}
}

func TestADrainIsRefusedWhenItsPodListIs(t *testing.T) {
	service := serviceFor(t, refusing(map[string]string{"list  pods ": "no listing pods"}))

	got := reasons(service.Review(t.Context(), nodeRef()))

	if got[Drain] != "no listing pods" {
		t.Fatalf("drain reason = %q, want the refused pod list", got[Drain])
	}
	if _, refused := got[Cordon]; refused {
		t.Fatalf("cordon was withheld over the pod list: %v", got)
	}
}

func TestADrainIsRefusedWhenTheCordonIs(t *testing.T) {
	service := serviceFor(t, refusing(map[string]string{"patch  nodes ": "no patching nodes"}))

	got := reasons(service.Review(t.Context(), nodeRef()))

	if got[Drain] != "no patching nodes" {
		t.Fatalf("drain reason = %q, want the refused cordon", got[Drain])
	}
	if got[Cordon] != "no patching nodes" {
		t.Fatalf("cordon reason = %q", got[Cordon])
	}
}

func TestTheFirstRefusalIsTheOneReported(t *testing.T) {
	service := serviceFor(t, refusing(map[string]string{
		"list  pods ":   "no listing pods",
		"patch  nodes ": "no patching nodes",
	}))

	got := reasons(service.Review(t.Context(), nodeRef()))

	// The list comes first, so that is what stopped it.
	if got[Drain] != "no listing pods" {
		t.Fatalf("drain reason = %q, want the first requirement that failed", got[Drain])
	}
}

func TestAnUnscalableKindIsNotAskedAboutScaling(t *testing.T) {
	auth := refusing(nil)
	service := serviceFor(t, auth)

	service.Review(t.Context(), api.ObjectRef{
		Version:   "v1",
		Resource:  "configmaps",
		Namespace: "prod",
		Name:      "settings",
	})

	for _, one := range auth.questions() {
		if one.Subresource == "scale" {
			t.Fatalf("a configmap was asked about scaling: %+v", one)
		}
	}
	if auth.count() != 2 {
		t.Fatalf("asked %d questions, want only edit and delete", auth.count())
	}
}

func TestAnApiserverThatWillNotAnswerTakesNothingAway(t *testing.T) {
	service := serviceFor(t, &authorizer{broken: true})

	result := service.Review(t.Context(), podRef())

	if len(result.Refused) != 0 {
		t.Fatalf("refused = %v; a failed check must not disable anything", result.Refused)
	}
}

func TestAnAuthorizerWithNoOpinionTakesNothingAway(t *testing.T) {
	service := serviceFor(t, &authorizer{unsure: true})

	result := service.Review(t.Context(), podRef())

	if len(result.Refused) != 0 {
		t.Fatalf("refused = %v; an evaluation error is not a refusal", result.Refused)
	}
}

func TestTheSameQuestionIsNotAskedTwice(t *testing.T) {
	auth := refusing(nil)
	service := serviceFor(t, auth)

	service.Review(t.Context(), podRef())
	first := auth.count()
	service.Review(t.Context(), podRef())

	if auth.count() != first {
		t.Fatalf("asked %d then %d, want the second review answered from memory", first, auth.count())
	}
}

func TestAStaleAnswerIsAskedAgain(t *testing.T) {
	auth := refusing(nil)
	service := serviceFor(t, auth)
	moment := time.Now()
	service.now = func() time.Time {
		return moment
	}

	service.Review(t.Context(), podRef())
	first := auth.count()
	moment = moment.Add(2 * remembered)
	service.Review(t.Context(), podRef())

	if auth.count() != first*2 {
		t.Fatalf("asked %d then %d in total, want the stale answers re-asked", first, auth.count())
	}
}

func TestADifferentObjectIsItsOwnQuestion(t *testing.T) {
	auth := refusing(nil)
	service := serviceFor(t, auth)

	service.Review(t.Context(), podRef())
	first := auth.count()
	other := podRef()
	other.Name = "api"
	service.Review(t.Context(), other)

	if auth.count() == first {
		t.Fatal("a second pod reused the first pod's answers")
	}
}

func TestAnAnswerThatCouldNotBeGivenIsNotRemembered(t *testing.T) {
	auth := &authorizer{broken: true}
	service := serviceFor(t, auth)

	service.Review(t.Context(), podRef())
	first := auth.count()
	service.Review(t.Context(), podRef())

	if auth.count() != first*2 {
		t.Fatalf("asked %d then %d, want a failed check asked again", first, auth.count())
	}
}

func TestWithoutAClusterNothingIsRefused(t *testing.T) {
	service := New(nil)

	result := service.Review(context.Background(), podRef())

	if len(result.Refused) != 0 {
		t.Fatalf("refused = %v", result.Refused)
	}
}

func TestEveryCheckIsAnswered(t *testing.T) {
	service := serviceFor(t, refusing(map[string]string{"delete  pods ": "no"}))

	decisions := service.review(t.Context(), []Check{
		{Verb: "get", Resource: "pods", Subresource: "log", Namespace: "prod"},
		{Verb: "delete", Resource: "pods", Namespace: "prod"},
	})

	if len(decisions) != 2 {
		t.Fatalf("decisions = %d, want one per check", len(decisions))
	}
	if !decisions[0].Allowed {
		t.Fatalf("first decision = %+v, want allowed", decisions[0])
	}
	if decisions[1].Allowed {
		t.Fatalf("second decision = %+v, want refused", decisions[1])
	}
}

func TestAnAllowedAnswerCarriesNoReason(t *testing.T) {
	auth := refusing(nil)
	auth.refuse = map[string]string{}
	service := serviceFor(t, auth)

	decisions := service.review(t.Context(), []Check{
		{Verb: "get", Resource: "pods", Namespace: "prod"},
	})

	if decisions[0].Reason != "" {
		t.Fatalf("reason = %q on an allowed check", decisions[0].Reason)
	}
}

func TestManyChecksAreAnsweredAtOnce(t *testing.T) {
	auth := refusing(nil)
	service := serviceFor(t, auth)
	checks := make([]Check, 0, atOnce*3)
	for i := range atOnce * 3 {
		checks = append(checks, Check{Verb: "get", Resource: "pods", Name: string(rune('a' + i))})
	}

	decisions := service.review(t.Context(), checks)

	if len(decisions) != len(checks) {
		t.Fatalf("decisions = %d, want %d", len(decisions), len(checks))
	}
	for i, decision := range decisions {
		if !decision.Allowed {
			t.Fatalf("decision %d = %+v", i, decision)
		}
	}
}

func TestAServiceIsAskedAboutForwardingToItsPods(t *testing.T) {
	auth := refusing(nil)
	service := serviceFor(t, auth)

	service.Review(t.Context(), api.ObjectRef{
		Version:   "v1",
		Resource:  "services",
		Namespace: "prod",
		Name:      "web",
	})

	forward, ok := asked(auth)["create  pods portforward"]
	if !ok {
		t.Fatalf("forwarding was never asked about: %v", asked(auth))
	}
	// The forward goes through whichever pod is behind the service.
	if forward.Name != "" {
		t.Fatalf("asked about pod %q, want any pod behind the service", forward.Name)
	}
	if forward.Namespace != "prod" {
		t.Fatalf("asked in %q", forward.Namespace)
	}
}

func TestAFluxResourceIsAskedAboutPatchingItself(t *testing.T) {
	auth := refusing(nil)
	service := serviceFor(t, auth)

	service.Review(t.Context(), api.ObjectRef{
		Group:     "kustomize.toolkit.fluxcd.io",
		Version:   "v1",
		Resource:  "kustomizations",
		Namespace: "flux-system",
		Name:      "apps",
	})

	// Reconcile, Suspend and Resume are all this one patch.
	if _, ok := asked(auth)["patch kustomize.toolkit.fluxcd.io kustomizations "]; !ok {
		t.Fatalf("the gitops buttons were never asked about: %v", asked(auth))
	}
}

func TestAnArgoApplicationIsAskedAboutPatchingItself(t *testing.T) {
	auth := refusing(nil)
	service := serviceFor(t, auth)

	service.Review(t.Context(), api.ObjectRef{
		Group:     "argoproj.io",
		Version:   "v1alpha1",
		Resource:  "applications",
		Namespace: "argocd",
		Name:      "web",
	})

	if _, ok := asked(auth)["patch argoproj.io applications "]; !ok {
		t.Fatalf("Sync and Refresh were never asked about: %v", asked(auth))
	}
}

func TestAnOrdinaryKindIsNotAskedAboutReconciling(t *testing.T) {
	auth := refusing(nil)
	service := serviceFor(t, auth)

	service.Review(t.Context(), deploymentRef())

	for _, one := range auth.questions() {
		if one.Verb == "patch" && one.Subresource == "" && one.Resource == "deployments" {
			// This is the restart question, which a deployment does have.
			continue
		}
		if one.Verb == "patch" && one.Group == "argoproj.io" {
			t.Fatalf("a deployment was asked a gitops question: %+v", one)
		}
	}
}

func TestARefusedGitopsPatchIsReported(t *testing.T) {
	service := serviceFor(t, refusing(map[string]string{
		"patch kustomize.toolkit.fluxcd.io kustomizations ": "no reconciling for you",
	}))

	got := reasons(service.Review(t.Context(), api.ObjectRef{
		Group:     "kustomize.toolkit.fluxcd.io",
		Version:   "v1",
		Resource:  "kustomizations",
		Namespace: "flux-system",
		Name:      "apps",
	}))

	if got[Reconcile] != "no reconciling for you" {
		t.Fatalf("reconcile reason = %q", got[Reconcile])
	}
}
