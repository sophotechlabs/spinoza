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

type capability struct {
	name  string
	check Check
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
		{name: Edit, check: with(object, "update")},
		{name: Delete, check: with(object, "delete")},
	}
	if scalable[here] {
		scale := with(object, "patch")
		scale.Subresource = "scale"
		held = append(held, capability{name: Scale, check: scale})
	}
	if restartable[here] {
		held = append(held, capability{name: Restart, check: with(object, "patch")})
	}
	if gitops(ref.Group) {
		held = append(held, capability{name: Reconcile, check: with(object, "patch")})
	}
	if here == (groupResource{resource: nodes}) {
		held = append(held, nodeCapabilities(object)...)
	}
	if here == (groupResource{resource: pods}) {
		return append(held, podCapabilities(ref)...)
	}
	if ownsPods[here] {
		held = append(held, capability{name: Logs, check: podCheck("get", ref.Namespace, "", "log")})
	}
	// A service is forwarded through one of the pods behind it, so the question
	// is about pods here rather than about the service.
	if here == (groupResource{resource: "services"}) {
		held = append(held, capability{
			name:  PortForward,
			check: podCheck("create", ref.Namespace, "", "portforward"),
		})
	}
	return held
}

// gitops is true for the kinds whose Reconcile, Suspend, Resume, Sync and
// Refresh buttons all patch the resource itself.
func gitops(group string) bool {
	return flux.IsFluxGroup(group) || argocd.IsArgoGroup(group)
}

func nodeCapabilities(object Check) []capability {
	return []capability{
		{name: Cordon, check: with(object, "patch")},
		// A drain cordons the node and then evicts what runs on it, wherever that
		// happens to be, so the eviction question is not tied to one namespace.
		{name: Drain, check: podCheck("create", "", "", "eviction")},
	}
}

func podCapabilities(ref api.ObjectRef) []capability {
	return []capability{
		{name: Logs, check: podCheck("get", ref.Namespace, ref.Name, "log")},
		{name: Exec, check: podCheck("create", ref.Namespace, ref.Name, "exec")},
		{name: PortForward, check: podCheck("create", ref.Namespace, ref.Name, "portforward")},
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
	held := capabilitiesFor(ref)
	checks := make([]Check, 0, len(held))
	for _, one := range held {
		checks = append(checks, one.check)
	}
	decisions := s.review(ctx, checks)
	refused := make([]api.Refusal, 0, len(held))
	for i, one := range held {
		if decisions[i].Allowed {
			continue
		}
		refused = append(refused, api.Refusal{
			Capability: one.name,
			Reason:     because(decisions[i].Reason, one.check),
		})
	}
	return api.Access{Refused: refused}
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
