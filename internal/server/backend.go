package server

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/sophotechlabs/spinoza/internal/actions"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/debugcontainer"
	"github.com/sophotechlabs/spinoza/internal/exec"
	"github.com/sophotechlabs/spinoza/internal/flux"
	"github.com/sophotechlabs/spinoza/internal/jsonschema"
	"github.com/sophotechlabs/spinoza/internal/logs"
	"github.com/sophotechlabs/spinoza/internal/portforward"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

type Catalog interface {
	Resources() api.ResourceCatalog
	RefreshResources() api.ResourceCatalog
	Counts(ctx context.Context) api.ResourceCounts
}

type Objects interface {
	Object(ctx context.Context, ref api.ObjectRef) (api.ObjectDetail, error)
	ApplyObject(ctx context.Context, ref api.ObjectRef, doc []byte) (api.ObjectDetail, error)
	DeleteObject(ctx context.Context, ref api.ObjectRef) error
	Events(ctx context.Context, namespace, uid string) ([]api.Event, error)
	Schema(ctx context.Context, gvk jsonschema.GVK) (json.RawMessage, error)
}

type Feeds interface {
	Subscribe(ctx context.Context, group, version, resource, namespace string) (*resources.Subscription, error)
	Logs(ctx context.Context, req logs.Request) (*logs.Stream, error)
}

type Views interface {
	Graph(ctx context.Context) api.Graph
	Flux(ctx context.Context) api.FluxDashboard
	Metrics(ctx context.Context) api.Metrics
	MetricHistory(ctx context.Context, namespace, pod string, span time.Duration) (api.MetricHistory, error)
}

type Changes interface {
	Action(ctx context.Context, req actions.Request) (api.ActionResult, error)
	FluxAction(ctx context.Context, ref api.ObjectRef, action flux.Action) (api.FluxActionResult, error)
}

type Forwarding interface {
	Forwards() []api.PortForward
	StartForward(ctx context.Context, target portforward.Target, port int32) (api.PortForward, error)
	StopForward(id string) error
}

type Terminals interface {
	ExecSupport(ctx context.Context, req exec.Request) (api.ExecSupport, error)
	StartExec(ctx context.Context, req exec.Request, stdout io.Writer) (*exec.Session, error)
	DebugSupport(ctx context.Context, namespace, pod string) api.DebugSupport
	StartDebug(ctx context.Context, req debugcontainer.Request) (api.DebugSession, error)
}

type Backend interface {
	Catalog
	Objects
	Feeds
	Views
	Changes
	Forwarding
	Terminals
}
