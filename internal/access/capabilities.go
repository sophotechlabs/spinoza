package access

import (
	"context"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/argocd"
	"github.com/sophotechlabs/spinoza/internal/flux"
)

// The names the browser knows. Each one stands for a button or a tab, and maps
// to the request that button would actually make.
const (
	Edit        = "edit"
	Delete      = "delete"
	Scale       = "scale"
	Restart     = "restart"
	Cordon      = "cordon"
	Drain       = "drain"
	Logs        = "logs"
	Exec        = "exec"
	PortForward = "portForward"
	// Reconcile covers every gitops button on one resource. Suspending, resuming
	// and reconciling are all the same patch on the same object, so one answer
	// speaks for all of them.
	Reconcile = "reconcile"
)

const (
	appsGroup = "apps"
	pods      = "pods"
	nodes     = "nodes"
)

// capability is a button and everything that has to be permitted for it to work.
// A button with more than one requirement is refused by the first one that is
// refused.
type capability struct {
	name   string
	checks []Check
}

func needs(name string, checks ...Check) capability {
	return capability{name: name, checks: checks}
}

type groupResource struct {
	group    string
	resource string
}

var scalable = map[groupResource]bool{
	{group: appsGroup, resource: "deployments"}:     true,
	{group: appsGroup, resource: "statefulsets"}:    true,
	{group: appsGroup, resource: "replicasets"}:     true,
	{group: "", resource: "replicationcontrollers"}: true,
}

var restartable = map[groupResource]bool{
	{group: appsGroup, resource: "deployments"}:  true,
	{group: appsGroup, resource: "statefulsets"}: true,
	{group: appsGroup, resource: "daemonsets"}:   true,
}

// ownsPods lists the kinds whose logs are read through the pods they select, so
// the question is about pods in that namespace rather than about the workload.
var ownsPods = map[groupResource]bool{
	{group: appsGroup, resource: "deployments"}:     true,
	{group: appsGroup, resource: "statefulsets"}:    true,
	{group: appsGroup, resource: "daemonsets"}:      true,
	{group: appsGroup, resource: "replicasets"}:     true,
	{group: "batch", resource: "jobs"}:              true,
	{group: "", resource: "replicationcontrollers"}: true,
}

// capabilitiesFor lists what is worth asking about for this object. The verbs
// mirror what each action does: scaling patches the scale subresource, a rollout
// restart patches the workload, draining evicts pods, and so on.
func capabilitiesFor(ref api.ObjectRef) []capability {
	here := groupResource{group: ref.Group, resource: ref.Resource}
	object := Check{Group: ref.Group, Resource: ref.Resource, Namespace: ref.Namespace, Name: ref.Name}
	held := []capability{
		needs(Edit, with(object, "update")),
		needs(Delete, with(object, "delete")),
	}
	if scalable[here] {
		scale := with(object, "patch")
		scale.Subresource = "scale"
		held = append(held, needs(Scale, scale))
	}
	if restartable[here] {
		held = append(held, needs(Restart, with(object, "patch")))
	}
	if gitops(ref.Group) {
		held = append(held, needs(Reconcile, with(object, "patch")))
	}
	if here == (groupResource{resource: nodes}) {
		held = append(held, nodeCapabilities(object)...)
	}
	if here == (groupResource{resource: pods}) {
		return append(held, podCapabilities(ref)...)
	}
	if ownsPods[here] {
		held = append(held, needs(Logs, podCheck("get", ref.Namespace, "", "log")))
	}
	// A service is forwarded through one of the pods behind it, so the question
	// is about pods here rather than about the service.
	if here == (groupResource{resource: "services"}) {
		held = append(held, needs(PortForward, podCheck("create", ref.Namespace, "", "portforward")))
	}
	return held
}

// gitops is true for the kinds whose Reconcile, Suspend, Resume, Sync and
// Refresh buttons all patch the resource itself.
func gitops(group string) bool {
	return flux.IsFluxGroup(group) || argocd.IsArgoGroup(group)
}

func nodeCapabilities(object Check) []capability {
	cordon := with(object, "patch")
	return []capability{
		needs(Cordon, cordon),
		// A drain reads the pods on the node, cordons it, and then evicts them one
		// at a time. Only the first two are all or nothing. Eviction is per pod and
		// a partial drain is a real outcome, so a user who may evict in some
		// namespaces and not others keeps the button and reads the result per pod.
		needs(Drain, podCheck("list", "", "", ""), cordon),
	}
}

func podCapabilities(ref api.ObjectRef) []capability {
	return []capability{
		needs(Logs, podCheck("get", ref.Namespace, ref.Name, "log")),
		needs(Exec, podCheck("create", ref.Namespace, ref.Name, "exec")),
		needs(PortForward, podCheck("create", ref.Namespace, ref.Name, "portforward")),
	}
}

func podCheck(verb, namespace, name, subresource string) Check {
	return Check{
		Verb:        verb,
		Resource:    pods,
		Subresource: subresource,
		Namespace:   namespace,
		Name:        name,
	}
}

func with(check Check, verb string) Check {
	check.Verb = verb
	return check
}

// Review answers what this object's buttons would need, and reports only the
// refusals: a capability the cluster did not object to is simply absent.
func (s *Service) Review(ctx context.Context, ref api.ObjectRef) api.Access {
	return s.answer(ctx, capabilitiesFor(ref))
}

// answer puts every capability's questions in one pass, so the cache and the
// concurrency limit hold across the whole set.
func (s *Service) answer(ctx context.Context, held []capability) api.Access {
	checks := make([]Check, 0, len(held))
	for _, one := range held {
		checks = append(checks, one.checks...)
	}
	decisions := s.review(ctx, checks)
	refused := make([]api.Refusal, 0, len(held))
	at := 0
	for _, one := range held {
		answers := decisions[at : at+len(one.checks)]
		at += len(one.checks)
		stopped, why := firstRefusal(one.checks, answers)
		if stopped == nil {
			continue
		}
		refused = append(refused, api.Refusal{Capability: one.name, Reason: because(why, *stopped)})
	}
	return api.Access{Refused: refused}
}

// ReviewEach answers one capability across many objects in a single pass, so
// that the cache and the concurrency limit hold across the whole selection
// rather than across each object on its own. A capability that means nothing
// for a kind is not a refusal: it is simply never asked about.
func (s *Service) ReviewEach(ctx context.Context, name string, refs []api.ObjectRef) api.BulkAccess {
	wanted := make([][]Check, len(refs))
	checks := []Check{}
	for i, ref := range refs {
		found, ok := capabilityNamed(name, ref)
		if !ok {
			continue
		}
		wanted[i] = found.checks
		checks = append(checks, found.checks...)
	}
	decisions := s.review(ctx, checks)
	refused := make([]api.RowRefusal, 0, len(refs))
	at := 0
	for i, mine := range wanted {
		answers := decisions[at : at+len(mine)]
		at += len(mine)
		stopped, why := firstRefusal(mine, answers)
		if stopped == nil {
			continue
		}
		refused = append(refused, api.RowRefusal{At: i, Reason: because(why, *stopped)})
	}
	return api.BulkAccess{Refused: refused}
}

func capabilityNamed(name string, ref api.ObjectRef) (capability, bool) {
	for _, one := range capabilitiesFor(ref) {
		if one.name == name {
			return one, true
		}
	}
	return capability{}, false
}

func firstRefusal(checks []Check, decisions []Decision) (*Check, string) {
	for i, decision := range decisions {
		if decision.Allowed {
			continue
		}
		return &checks[i], decision.Reason
	}
	return nil, ""
}

// because falls back to a plain sentence when the cluster refused without
// saying why, which is what plain RBAC does.
func because(reason string, check Check) string {
	if reason != "" {
		return reason
	}
	return "you may not " + check.Verb + " " + describe(check) + " here"
}

func describe(check Check) string {
	name := check.Resource
	if check.Group != "" {
		name = check.Group + "/" + check.Resource
	}
	if check.Subresource != "" {
		return name + "/" + check.Subresource
	}
	return name
}
