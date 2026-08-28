package actions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	fieldManager = "spinoza"
	appsGroup    = "apps"
	batchGroup   = "batch"
	specField    = "spec"
)

type Action string

const (
	Scale    Action = "scale"
	Restart  Action = "restart"
	Cordon   Action = "cordon"
	Uncordon Action = "uncordon"
	Drain    Action = "drain"
	Suspend  Action = "suspend"
	Resume   Action = "resume"
	Trigger  Action = "trigger"
)

var ErrUnsupported = errors.New("action is not supported for this resource")

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

var cronJobs = groupResource{group: batchGroup, resource: "cronjobs"}

type Request struct {
	Ref      api.ObjectRef
	Action   Action
	Replicas int64
	Force    bool
	DryRun   bool
}

type Service struct {
	dyn        dynamic.Interface
	cs         kubernetes.Interface
	retryDelay time.Duration
}

const evictRetryDelay = 2 * time.Second

func New(dyn dynamic.Interface, cs kubernetes.Interface) *Service {
	return newWithDelay(dyn, cs, evictRetryDelay)
}

func newWithDelay(dyn dynamic.Interface, cs kubernetes.Interface, retryDelay time.Duration) *Service {
	return &Service{dyn: dyn, cs: cs, retryDelay: retryDelay}
}

func Supported(ref api.ObjectRef, action Action) bool {
	key := groupResource{group: ref.Group, resource: ref.Resource}
	switch action {
	case Scale:
		return scalable[key]
	case Restart:
		return restartable[key]
	case Cordon, Uncordon, Drain:
		return key == groupResource{group: "", resource: "nodes"}
	case Suspend, Resume, Trigger:
		return key == cronJobs
	default:
		return false
	}
}

func known(action Action) bool {
	switch action {
	case Scale, Restart, Cordon, Uncordon, Drain, Suspend, Resume, Trigger:
		return true
	default:
		return false
	}
}

func (s *Service) Do(ctx context.Context, req Request, now time.Time) (api.ActionResult, error) {
	if req.Ref.Name == "" {
		return api.ActionResult{}, errors.New("name is required")
	}
	if !known(req.Action) {
		return api.ActionResult{}, fmt.Errorf("unknown action %q", req.Action)
	}
	if !Supported(req.Ref, req.Action) {
		return api.ActionResult{}, fmt.Errorf("%w: %s on %s", ErrUnsupported, req.Action, describe(req.Ref))
	}
	switch req.Action {
	case Scale:
		return s.scale(ctx, req)
	case Restart:
		return s.restart(ctx, req, now)
	case Cordon:
		return s.setSchedulable(ctx, req.Ref, false)
	case Uncordon:
		return s.setSchedulable(ctx, req.Ref, true)
	case Drain:
		return s.drain(ctx, req)
	case Suspend:
		return s.setSuspended(ctx, req.Ref, true)
	case Resume:
		return s.setSuspended(ctx, req.Ref, false)
	case Trigger:
		return s.trigger(ctx, req.Ref, now)
	default:
		return api.ActionResult{}, fmt.Errorf("unknown action %q", req.Action)
	}
}

func describe(ref api.ObjectRef) string {
	if ref.Group == "" {
		return ref.Resource
	}
	return ref.Group + "/" + ref.Resource
}
