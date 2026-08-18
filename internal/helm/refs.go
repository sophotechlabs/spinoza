package helm

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/metadata"
)

var (
	secretsGVR    = schema.GroupVersionResource{Version: "v1", Resource: "secrets"}
	configMapsGVR = schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	namespacesGVR = schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}
)

type storedRef struct {
	driver    string
	namespace string
	object    string
	name      string
	revision  int64
	status    string
	created   time.Time
	version   string
}

type refPage struct {
	items     []storedRef
	truncated bool
	denied    string
}

func refOf(driver string, item *metav1.PartialObjectMetadata) storedRef {
	revision, err := strconv.ParseInt(item.Labels[versionLabel], 10, 64)
	if err != nil {
		revision = 0
	}
	return storedRef{
		driver:    driver,
		namespace: item.Namespace,
		object:    item.Name,
		name:      item.Labels[nameLabel],
		revision:  revision,
		status:    item.Labels[statusLabel],
		created:   item.CreationTimestamp.UTC(),
		version:   item.ResourceVersion,
	}
}

func allRefs(ctx context.Context, client metadata.Interface) (refPage, error) {
	secrets, err := refsWithFallback(ctx, client, DriverSecret, secretsGVR, "secrets")
	if err != nil {
		return refPage{}, err
	}
	maps, mapErr := refsWithFallback(ctx, client, DriverConfigMap, configMapsGVR, "config maps")
	if mapErr != nil {
		return refPage{}, mapErr
	}
	return refPage{
		items:     append(secrets.items, maps.items...),
		truncated: secrets.truncated || maps.truncated,
		denied:    joinNotes(secrets.denied, maps.denied),
	}, nil
}

func refsWithFallback(
	ctx context.Context,
	client metadata.Interface,
	driver string,
	gvr schema.GroupVersionResource,
	resource string,
) (refPage, error) {
	direct, err := listRefs(ctx, client, driver, gvr, metav1.NamespaceAll)
	if err == nil {
		return direct, nil
	}
	if !apierrors.IsForbidden(err) {
		return refPage{}, err
	}
	names, nsErr := namespaceRefs(ctx, client)
	if nsErr != nil {
		return refPage{}, err
	}
	out := refPage{}
	allowed := []string{}
	for _, name := range names {
		one, oneErr := listRefs(ctx, client, driver, gvr, name)
		if apierrors.IsForbidden(oneErr) {
			continue
		}
		if oneErr != nil {
			return refPage{}, oneErr
		}
		allowed = append(allowed, name)
		out.items = append(out.items, one.items...)
		if one.truncated {
			out.truncated = true
		}
		if len(out.items) >= maxObjects {
			out.truncated = true
			break
		}
	}
	out.denied = deniedNote(resource, len(names), allowed)
	return out, nil
}

func listRefs(
	ctx context.Context,
	client metadata.Interface,
	driver string,
	gvr schema.GroupVersionResource,
	namespace string,
) (refPage, error) {
	out := refPage{}
	opts := metav1.ListOptions{LabelSelector: ownerLabel, Limit: pageSize}
	for {
		listed, err := client.Resource(gvr).Namespace(namespace).List(ctx, opts)
		if err != nil {
			return refPage{}, err
		}
		for i := range listed.Items {
			out.items = append(out.items, refOf(driver, &listed.Items[i]))
		}
		if listed.Continue == "" {
			return out, nil
		}
		if len(out.items) >= maxObjects {
			out.truncated = true
			return out, nil
		}
		opts.Continue = listed.Continue
	}
}

func namespaceRefs(ctx context.Context, client metadata.Interface) ([]string, error) {
	names := []string{}
	opts := metav1.ListOptions{Limit: pageSize}
	for {
		listed, err := client.Resource(namespacesGVR).List(ctx, opts)
		if err != nil {
			return nil, err
		}
		for i := range listed.Items {
			names = append(names, listed.Items[i].Name)
		}
		if listed.Continue == "" {
			return names, nil
		}
		opts.Continue = listed.Continue
	}
}

func deniedNote(resource string, namespaces int, allowed []string) string {
	head := fmt.Sprintf(
		"%s could not be listed cluster-wide; %d of %d namespaces allowed it",
		resource, len(allowed), namespaces,
	)
	if len(allowed) == 0 {
		return head
	}
	return head + ": " + strings.Join(allowed, ", ")
}

func joinNotes(left, right string) string {
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}
	return left + "; " + right
}

func newestPerRelease(refs []storedRef) []storedRef {
	newest := map[string]storedRef{}
	for _, ref := range refs {
		if ref.name == "" {
			continue
		}
		key := ref.namespace + "/" + ref.name
		previous, seen := newest[key]
		if seen && previous.revision >= ref.revision {
			continue
		}
		newest[key] = ref
	}
	out := make([]storedRef, 0, len(newest))
	for _, ref := range newest {
		out = append(out, ref)
	}
	return out
}
