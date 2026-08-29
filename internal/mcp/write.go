package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/sophotechlabs/spinoza/internal/actions"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/argocd"
	"github.com/sophotechlabs/spinoza/internal/flux"
)

var (
	fluxVerbs = []string{"reconcile", "reconcile-with-source", "suspend", "resume"}
	argoVerbs = []string{"sync", "refresh", "hard-refresh", "terminate", "suspend", "resume", "rollback"}
)

var writeToolNames = map[string]bool{
	"manage_workload": true,
	"manage_node":     true,
	"manage_cronjob":  true,
	"manage_gitops":   true,
	"apply_resource":  true,
}

func (s *Server) registerWrites() {
	s.register(tool{
		name:        "manage_workload",
		title:       "Scale or restart a workload",
		description: "Scale a workload to a replica count, or roll its pods by stamping a restart annotation.",
		properties: map[string]propOf{
			argResource:  text("Workload resource or kind, for example deployments."),
			argName:      text("Workload name."),
			argNamespace: text("Namespace."),
			argGroup:     text("API group, when ambiguous."),
			argAction:    choice("What to do.", "scale", "restart"),
			argReplicas:  number("Replica count, for scale."),
		},
		required:   []string{argResource, argName, argNamespace, argAction},
		writes:     true,
		idempotent: true,
		run:        s.manageWorkload,
	})
	s.register(tool{
		name:        "manage_node",
		title:       "Cordon, uncordon or drain a node",
		description: "Mark a node unschedulable, schedulable again, or evict its pods. Draining moves running workloads; pass dryRun to see what it would evict first.",
		properties: map[string]propOf{
			argName:   text("Node name."),
			argAction: choice("What to do.", "cordon", "uncordon", "drain"),
			"force":   toggle("Evict pods nothing owns, when draining."),
			"dryRun":  toggle("Draining only: report what would be evicted and change nothing."),
		},
		required:    []string{argName, argAction},
		writes:      true,
		destructive: true,
		run:         s.manageNode,
	})
	s.register(tool{
		name:        "manage_cronjob",
		title:       "Suspend, resume or trigger a cron job",
		description: "Pause a schedule, start it again, or run one job now from the cron job's template.",
		properties: map[string]propOf{
			argName:      text("Cron job name."),
			argNamespace: text("Namespace."),
			argAction:    choice("What to do.", "suspend", "resume", "trigger"),
		},
		required:   []string{argName, argNamespace, argAction},
		writes:     true,
		idempotent: true,
		run:        s.manageCronJob,
	})
	s.register(tool{
		name:        "manage_gitops",
		title:       "Reconcile, suspend or sync a GitOps object",
		description: "Drive Flux or Argo CD: reconcile, suspend, resume, sync or refresh the object that owns a workload.",
		properties: map[string]propOf{
			"engine":     choice("Which controller owns it.", "flux", "argo"),
			argResource:  text("Resource or kind, for example kustomizations or applications."),
			argName:      text("Object name."),
			argNamespace: text("Namespace."),
			argGroup:     text("API group, when ambiguous."),
			argAction: choice("What to do. Flux takes the first four, Argo the rest.",
				"reconcile", "reconcile-with-source", "suspend", "resume",
				"sync", "refresh", "hard-refresh", "terminate", "rollback"),
		},
		required:   []string{"engine", argResource, argName, argNamespace, argAction},
		writes:     true,
		idempotent: true,
		run:        s.manageGitops,
	})
	s.register(tool{
		name:        "apply_resource",
		title:       "Apply a document",
		description: "Server-side apply one YAML document over an object that already exists. Read the object with get_resource first and carry its resourceVersion in the document, or the apply is refused rather than overwriting a change someone else made.",
		properties: map[string]propOf{
			argResource:  text("Resource or kind the document describes."),
			argName:      text("Object name."),
			argNamespace: text("Namespace."),
			argGroup:     text("API group, when ambiguous."),
			argYAML:      text("The whole document to apply, carrying the resourceVersion get_resource returned."),
		},
		required:    []string{argResource, argName, argYAML},
		writes:      true,
		destructive: true,
		run:         s.applyResource,
	})
}

func (s *Server) permitted(ctx context.Context, ref api.ObjectRef, capability string) error {
	for _, refusal := range s.cluster.Access(ctx, ref).Refused {
		if refusal.Capability == capability {
			return errors.New(refusal.Reason)
		}
	}
	return nil
}

func (s *Server) act(ctx context.Context, ref api.ObjectRef, req actions.Request, capability string) (any, error) {
	if err := s.permitted(ctx, ref, capability); err != nil {
		return nil, err
	}
	result, err := s.cluster.Action(ctx, req)
	if err != nil {
		return nil, err
	}
	return map[string]any{"applied": result}, nil
}

func (s *Server) manageWorkload(ctx context.Context, args arguments) (any, error) {
	ref, err := args.refIn(s.cluster.Resources())
	if err != nil {
		return nil, err
	}
	verb, err := args.oneOf(argAction, "scale", "restart")
	if err != nil {
		return nil, err
	}
	req := actions.Request{Ref: ref, Action: actions.Action(verb)}
	if verb == "scale" {
		replicas, err := args.count(argReplicas)
		if err != nil {
			return nil, err
		}
		if replicas < 0 {
			return nil, errors.New("replicas cannot be negative")
		}
		req.Replicas = replicas
	}
	return s.act(ctx, ref, req, verb)
}

func (s *Server) manageNode(ctx context.Context, args arguments) (any, error) {
	name, err := args.required(argName)
	if err != nil {
		return nil, err
	}
	verb, err := args.oneOf(argAction, "cordon", "uncordon", "drain")
	if err != nil {
		return nil, err
	}
	ref := api.ObjectRef{Version: "v1", Resource: "nodes", Name: name}
	req := actions.Request{
		Ref:    ref,
		Action: actions.Action(verb),
		Force:  args.flag("force"),
		DryRun: verb == "drain" && args.flag("dryRun"),
	}
	return s.act(ctx, ref, req, verb)
}

func (s *Server) manageCronJob(ctx context.Context, args arguments) (any, error) {
	name, err := args.required(argName)
	if err != nil {
		return nil, err
	}
	namespace, err := args.required(argNamespace)
	if err != nil {
		return nil, err
	}
	verb, err := args.oneOf(argAction, "suspend", "resume", "trigger")
	if err != nil {
		return nil, err
	}
	ref := api.ObjectRef{
		Group:     "batch",
		Version:   "v1",
		Resource:  "cronjobs",
		Namespace: namespace,
		Name:      name,
	}
	req := actions.Request{Ref: ref, Action: actions.Action(verb)}
	return s.act(ctx, ref, req, verb)
}

func (s *Server) manageGitops(ctx context.Context, args arguments) (any, error) {
	engine, err := args.oneOf(argEngine, "flux", "argo")
	if err != nil {
		return nil, err
	}
	ref, err := args.refIn(s.cluster.Resources())
	if err != nil {
		return nil, err
	}
	if engine == "flux" {
		verb, wrong := args.oneOf(argAction, fluxVerbs...)
		if wrong != nil {
			return nil, wrong
		}
		result, actErr := s.cluster.FluxAction(ctx, ref, flux.Action(verb))
		if actErr != nil {
			return nil, actErr
		}
		return map[string]any{"applied": result}, nil
	}
	verb, err := args.oneOf(argAction, argoVerbs...)
	if err != nil {
		return nil, err
	}
	result, actErr := s.cluster.ArgoAction(ctx, ref, argocd.Request{Action: argocd.Action(verb)})
	if actErr != nil {
		return nil, actErr
	}
	return map[string]any{"applied": result}, nil
}

func (s *Server) applyResource(ctx context.Context, args arguments) (any, error) {
	ref, err := args.ref(s.cluster.Resources())
	if err != nil {
		return nil, err
	}
	document, err := args.required(argYAML)
	if err != nil {
		return nil, err
	}
	if refused := s.permitted(ctx, ref, "edit"); refused != nil {
		return nil, refused
	}
	detail, err := s.cluster.ApplyObject(ctx, ref, []byte(document))
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"applied":    fmt.Sprintf("%s/%s", detail.Kind, detail.Name),
		argNamespace: detail.Namespace,
	}, nil
}
