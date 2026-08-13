package helm

import (
	"context"
	"fmt"
	"strconv"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	DriverSecret    = "secret"
	DriverConfigMap = "configmap"
)

type stored struct {
	driver    string
	namespace string
	name      string
	revision  int64
	status    string
	created   time.Time
	body      []byte
}

type page struct {
	items     []stored
	truncated bool
	denied    string
}

type loader func(ctx context.Context, cs kubernetes.Interface, namespace string) (page, error)

func loadAll(ctx context.Context, cs kubernetes.Interface) (page, error) {
	secrets, err := withFallback(ctx, cs, "secrets", loadSecrets)
	if err != nil {
		return page{}, err
	}
	maps, mapErr := withFallback(ctx, cs, "config maps", loadConfigMaps)
	if mapErr != nil {
		return page{}, mapErr
	}
	return page{
		items:     append(secrets.items, maps.items...),
		truncated: secrets.truncated || maps.truncated,
		denied:    joinNotes(secrets.denied, maps.denied),
	}, nil
}

func withFallback(ctx context.Context, cs kubernetes.Interface, resource string, load loader) (page, error) {
	direct, err := load(ctx, cs, "")
	if err == nil {
		return direct, nil
	}
	if !apierrors.IsForbidden(err) {
		return page{}, err
	}
	names, nsErr := namespaceNames(ctx, cs)
	if nsErr != nil {
		return page{}, err
	}
	out := page{}
	refused := 0
	for _, name := range names {
		one, oneErr := load(ctx, cs, name)
		if apierrors.IsForbidden(oneErr) {
			refused++
			continue
		}
		if oneErr != nil {
			return page{}, oneErr
		}
		out.items = append(out.items, one.items...)
		if one.truncated {
			out.truncated = true
		}
		if len(out.items) >= maxObjects {
			out.truncated = true
			break
		}
	}
	out.denied = deniedNote(resource, len(names), refused)
	return out, nil
}

func namespaceNames(ctx context.Context, cs kubernetes.Interface) ([]string, error) {
	names := []string{}
	opts := metav1.ListOptions{Limit: pageSize}
	for {
		listed, err := cs.CoreV1().Namespaces().List(ctx, opts)
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

func deniedNote(resource string, namespaces, refused int) string {
	return fmt.Sprintf(
		"%s could not be listed cluster-wide; %d of %d namespaces allowed it",
		resource, namespaces-refused, namespaces,
	)
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

func loadSecrets(ctx context.Context, cs kubernetes.Interface, namespace string) (page, error) {
	out := page{}
	opts := metav1.ListOptions{LabelSelector: ownerLabel, Limit: pageSize}
	for {
		listed, err := cs.CoreV1().Secrets(namespace).List(ctx, opts)
		if err != nil {
			return page{}, err
		}
		for i := range listed.Items {
			secret := &listed.Items[i]
			if secret.Type != storageType {
				continue
			}
			out.items = append(out.items, storedOf(DriverSecret, &secret.ObjectMeta, secret.Data[releaseKey]))
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

func loadConfigMaps(ctx context.Context, cs kubernetes.Interface, namespace string) (page, error) {
	out := page{}
	opts := metav1.ListOptions{LabelSelector: ownerLabel, Limit: pageSize}
	for {
		listed, err := cs.CoreV1().ConfigMaps(namespace).List(ctx, opts)
		if err != nil {
			return page{}, err
		}
		for i := range listed.Items {
			entry := &listed.Items[i]
			body, ok := entry.Data[releaseKey]
			if !ok {
				continue
			}
			out.items = append(out.items, storedOf(DriverConfigMap, &entry.ObjectMeta, []byte(body)))
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

func storedOf(driver string, meta *metav1.ObjectMeta, body []byte) stored {
	revision, err := strconv.ParseInt(meta.Labels[versionLabel], 10, 64)
	if err != nil {
		revision = 0
	}
	return stored{
		driver:    driver,
		namespace: meta.Namespace,
		name:      meta.Labels[nameLabel],
		revision:  revision,
		status:    meta.Labels[statusLabel],
		created:   meta.CreationTimestamp.UTC(),
		body:      body,
	}
}

func (s stored) release() (api.HelmRelease, error) {
	fallback := api.HelmRelease{
		Name:      s.name,
		Namespace: s.namespace,
		Revision:  s.revision,
		Status:    s.status,
		Updated:   s.created.Format(time.RFC3339),
	}
	decoded, err := decode(s.body)
	if err != nil {
		return fallback, err
	}
	release := api.HelmRelease{
		Name:         decoded.Name,
		Namespace:    decoded.Namespace,
		Chart:        decoded.Chart.Metadata.Name,
		ChartVersion: decoded.Chart.Metadata.Version,
		AppVersion:   decoded.Chart.Metadata.AppVersion,
		Revision:     decoded.Version,
		Status:       decoded.Info.Status,
		Updated:      decoded.Info.LastDeployed,
		Description:  decoded.Info.Description,
	}
	return completed(release, fallback), nil
}

func completed(release, fallback api.HelmRelease) api.HelmRelease {
	if release.Name == "" {
		release.Name = fallback.Name
	}
	if release.Namespace == "" {
		release.Namespace = fallback.Namespace
	}
	if release.Revision == 0 {
		release.Revision = fallback.Revision
	}
	if release.Status == "" {
		release.Status = fallback.Status
	}
	if release.Updated == "" {
		release.Updated = fallback.Updated
	}
	return release
}
