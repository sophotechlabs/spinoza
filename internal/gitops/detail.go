package gitops

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/argocd"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/flux"
	"github.com/sophotechlabs/spinoza/internal/inspect"
	"github.com/sophotechlabs/spinoza/internal/unstr"
)

var ErrNotAnApplier = errors.New("that object is not an argo cd application, a flux kustomization or a flux helmrelease")

const outOfSync = "OutOfSync"

const (
	maxLiveReads    = 25
	maxEventsPer    = 5
	maxNamespaces   = 5
	eventPageSize   = 500
	readTimeout     = 15 * time.Second
	eventsGroupless = ""
)

var eventsGVR = schema.GroupVersionResource{Group: eventsGroupless, Version: "v1", Resource: "events"}

func Detail(ctx context.Context, dyn dynamic.Interface, descs map[string]api.ResourceDescriptor, ref api.ObjectRef) (api.GitopsApp, error) {
	desc, known := descs[discovery.Key(ref.Group, ref.Version, ref.Resource)]
	if !known {
		return api.GitopsApp{}, fmt.Errorf("%w: %s is not a kind this cluster serves", ErrNotAnApplier, ref.Resource)
	}
	if !applied(desc) {
		return api.GitopsApp{}, fmt.Errorf("%w: %s/%s is a %s", ErrNotAnApplier, ref.Namespace, ref.Name, desc.Kind)
	}
	obj, err := read(ctx, dyn, ref)
	if err != nil {
		return api.GitopsApp{}, err
	}
	app := build(ctx, dyn, descs, obj, desc)
	app.Ref = ref
	enrich(ctx, dyn, descs, &app)
	return app, nil
}

func applied(desc api.ResourceDescriptor) bool {
	if argocd.IsArgoGroup(desc.Group) && desc.Kind == argocd.IsApplication {
		return true
	}
	return flux.Applies(desc)
}

func build(ctx context.Context, dyn dynamic.Interface, descs map[string]api.ResourceDescriptor, obj *unstructured.Unstructured, desc api.ResourceDescriptor) api.GitopsApp {
	if argocd.IsArgoGroup(desc.Group) && desc.Kind == argocd.IsApplication {
		return argocd.Detail(obj)
	}
	app := flux.Detail(obj, desc)
	resolveSource(ctx, dyn, descs, obj, &app)
	return app
}

func read(ctx context.Context, dyn dynamic.Interface, ref api.ObjectRef) (*unstructured.Unstructured, error) {
	gvr := schema.GroupVersionResource{Group: ref.Group, Version: ref.Version, Resource: ref.Resource}
	bounded, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	if ref.Namespace == "" {
		return dyn.Resource(gvr).Get(bounded, ref.Name, metav1.GetOptions{})
	}
	return dyn.Resource(gvr).Namespace(ref.Namespace).Get(bounded, ref.Name, metav1.GetOptions{})
}

func resolveSource(ctx context.Context, dyn dynamic.Interface, descs map[string]api.ResourceDescriptor, obj *unstructured.Unstructured, app *api.GitopsApp) {
	kind, name, found := strings.Cut(app.Source.Repo, "/")
	if !found {
		return
	}
	desc, known := byKind(descs, kind)
	if !known {
		return
	}
	namespace := unstr.String(obj, "spec", "sourceRef", "namespace")
	if namespace == "" {
		namespace = obj.GetNamespace()
	}
	source, err := read(ctx, dyn, api.ObjectRef{
		Group:     desc.Group,
		Version:   desc.Version,
		Resource:  desc.Resource,
		Namespace: namespace,
		Name:      name,
	})
	if err != nil {
		return
	}
	app.Source.Repo = unstr.String(source, "spec", "url")
	app.Source.Target = branchOf(source)
}

func branchOf(source *unstructured.Unstructured) string {
	for _, field := range []string{"branch", "tag", "semver", "commit", "name"} {
		value := unstr.String(source, "spec", "ref", field)
		if value != "" {
			return value
		}
	}
	return ""
}

func byKind(descs map[string]api.ResourceDescriptor, kind string) (api.ResourceDescriptor, bool) {
	for _, desc := range descs {
		if desc.Kind == kind {
			return desc, true
		}
	}
	return api.ResourceDescriptor{}, false
}

func enrich(ctx context.Context, dyn dynamic.Interface, descs map[string]api.ResourceDescriptor, app *api.GitopsApp) {
	identify(descs, app.Resources)
	order := readingOrder(app.Resources)
	read := 0
	for _, at := range order {
		if read == maxLiveReads {
			break
		}
		read++
		live := liveResource(ctx, dyn, descs, app.Resources[at])
		if live == nil {
			continue
		}
		apply(&app.Resources[at], live)
	}
	if len(order) > read {
		app.Issues = append(app.Issues, api.GitopsIssue{
			Severity: api.SeverityInfo,
			Title:    fmt.Sprintf("%d of %d managed resources were read from the cluster", read, len(order)),
			Detail:   "the rest show what the controller reported",
		})
	}
	attachEvents(ctx, dyn, app)
}

func identify(descs map[string]api.ResourceDescriptor, resources []api.GitopsResource) {
	for at := range resources {
		desc, known := descriptorFor(descs, resources[at])
		if !known {
			continue
		}
		resources[at].Resource = desc.Resource
		if resources[at].Version == "" {
			resources[at].Version = desc.Version
		}
	}
}

func readingOrder(resources []api.GitopsResource) []int {
	order := make([]int, 0, len(resources))
	for at := range resources {
		order = append(order, at)
	}
	slices.SortStableFunc(order, func(left, right int) int {
		return interest(resources[right]) - interest(resources[left])
	})
	return order
}

func interest(resource api.GitopsResource) int {
	score := 0
	if resource.Sync != "" && resource.Sync != "Synced" {
		score += 2
	}
	if resource.Health != "" && resource.Health != "Healthy" {
		score++
	}
	return score
}

func liveResource(ctx context.Context, dyn dynamic.Interface, descs map[string]api.ResourceDescriptor, resource api.GitopsResource) *unstructured.Unstructured {
	desc, known := descriptorFor(descs, resource)
	if !known {
		return nil
	}
	live, err := read(ctx, dyn, api.ObjectRef{
		Group:     desc.Group,
		Version:   desc.Version,
		Resource:  desc.Resource,
		Namespace: resource.Namespace,
		Name:      resource.Name,
	})
	if err != nil {
		return nil
	}
	return live
}

func descriptorFor(descs map[string]api.ResourceDescriptor, resource api.GitopsResource) (api.ResourceDescriptor, bool) {
	for _, desc := range descs {
		if desc.Group != resource.Group {
			continue
		}
		if desc.Kind != resource.Kind {
			continue
		}
		return desc, true
	}
	return api.ResourceDescriptor{}, false
}

func apply(resource *api.GitopsResource, live *unstructured.Unstructured) {
	desc := live.GroupVersionKind()
	resource.Version = desc.Version
	if live.GetDeletionTimestamp() != nil {
		resource.Terminating = true
		resource.Finalizers = live.GetFinalizers()
	}
	drift, note := Drift(live)
	resource.Drift = drift
	resource.DriftNote = note
	if len(drift) == 0 && note == "" && resource.Sync == outOfSync {
		resource.DriftNote = "no declared field differs; git may no longer declare this resource"
	}
}

func attachEvents(ctx context.Context, dyn dynamic.Interface, app *api.GitopsApp) {
	for _, namespace := range namespacesOf(app.Resources) {
		found := eventsIn(ctx, dyn, namespace)
		for at := range app.Resources {
			resource := &app.Resources[at]
			if resource.Namespace != namespace {
				continue
			}
			resource.Events = found[resource.Kind+"/"+resource.Name]
		}
	}
}

func namespacesOf(resources []api.GitopsResource) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, one := range resources {
		if one.Namespace == "" || seen[one.Namespace] {
			continue
		}
		seen[one.Namespace] = true
		out = append(out, one.Namespace)
		if len(out) == maxNamespaces {
			return out
		}
	}
	return out
}

func eventsIn(ctx context.Context, dyn dynamic.Interface, namespace string) map[string][]api.Event {
	bounded, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	list, err := dyn.Resource(eventsGVR).Namespace(namespace).List(bounded, metav1.ListOptions{Limit: eventPageSize})
	if err != nil {
		return nil
	}
	out := map[string][]api.Event{}
	for i := range list.Items {
		item := &list.Items[i]
		key := unstr.String(item, "involvedObject", "kind") + "/" + unstr.String(item, "involvedObject", "name")
		out[key] = append(out[key], inspect.EventOf(item))
	}
	for key, events := range out {
		slices.SortStableFunc(events, func(left, right api.Event) int {
			return strings.Compare(right.LastSeen, left.LastSeen)
		})
		if len(events) > maxEventsPer {
			events = events[:maxEventsPer]
		}
		out[key] = events
	}
	return out
}
