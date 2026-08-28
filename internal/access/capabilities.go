package access

import (
	"context"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/argocd"
	"github.com/sophotechlabs/spinoza/internal/flux"
)

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
	// Reconcile covers suspend and resume too: the same patch, one answer.
	Reconcile = "reconcile"
)

const (
	appsGroup = "apps"
	pods      = "pods"
	nodes     = "nodes"
)

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

// Logs come from the pods these select, so the question is about pods.
var ownsPods = map[groupResource]bool{
	{group: appsGroup, resource: "deployments"}:     true,
	{group: appsGroup, resource: "statefulsets"}:    true,
	{group: appsGroup, resource: "daemonsets"}:      true,
	{group: appsGroup, resource: "replicasets"}:     true,
	{group: "batch", resource: "jobs"}:              true,
	{group: "", resource: "replicationcontrollers"}: true,
}

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
	// A service forwards through one of its pods, so ask about pods.
	if here == (groupResource{resource: "services"}) {
		held = append(held, needs(PortForward, podCheck("create", ref.Namespace, "", "portforward")))
	}
	return held
}

func gitops(group string) bool {
	return flux.IsFluxGroup(group) || argocd.IsArgoGroup(group)
}

func nodeCapabilities(object Check) []capability {
	cordon := with(object, "patch")
	return []capability{
		needs(Cordon, cordon),
		// Reading pods and cordoning are all or nothing; eviction is per pod.
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

func (s *Service) Review(ctx context.Context, ref api.ObjectRef) api.Access {
	return s.answer(ctx, capabilitiesFor(ref))
}

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

// ReviewEach asks one capability of many objects. Not applicable is not
// refused.
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
