package helm

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

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

func revisionsIn(
	ctx context.Context,
	cs kubernetes.Interface,
	namespace, name string,
) ([]stored, error) {
	selector := ownerLabel + "," + nameLabel + "=" + name
	return revisionsMatching(ctx, cs, namespace, selector, maxObjects)
}

func revisionsMatching(
	ctx context.Context,
	cs kubernetes.Interface,
	namespace, selector string,
	limit int,
) ([]stored, error) {
	if limit < 1 {
		return nil, errors.New("revision limit must be positive")
	}
	out := []stored{}
	secrets, err := revisionSecrets(ctx, cs, namespace, selector, limit+1)
	if err != nil {
		return nil, err
	}
	out = append(out, secrets...)
	maps, mapErr := revisionConfigMaps(ctx, cs, namespace, selector, limit+1)
	if mapErr != nil {
		return nil, mapErr
	}
	out = append(out, maps...)
	if len(out) > limit {
		return nil, fmt.Errorf("more than %d stored revisions matched", limit)
	}
	return out, nil
}

func revisionSecrets(
	ctx context.Context,
	cs kubernetes.Interface,
	namespace, selector string,
	limit int,
) ([]stored, error) {
	out := []stored{}
	opts := metav1.ListOptions{LabelSelector: selector, Limit: pageSize}
	seen := map[string]bool{}
	scanned := 0
	for {
		listed, err := cs.CoreV1().Secrets(namespace).List(ctx, opts)
		if err != nil {
			return nil, err
		}
		for i := range listed.Items {
			scanned++
			if scanned > limit {
				return nil, fmt.Errorf("more than %d release secrets matched", limit)
			}
			secret := &listed.Items[i]
			if secret.Type != storageType {
				continue
			}
			out = append(out, storedOf(DriverSecret, &secret.ObjectMeta, secret.Data[releaseKey]))
		}
		more, nextErr := advancePage(&opts, listed.Continue, seen)
		if nextErr != nil {
			return nil, nextErr
		}
		if !more {
			return out, nil
		}
	}
}

func revisionConfigMaps(
	ctx context.Context,
	cs kubernetes.Interface,
	namespace, selector string,
	limit int,
) ([]stored, error) {
	out := []stored{}
	opts := metav1.ListOptions{LabelSelector: selector, Limit: pageSize}
	seen := map[string]bool{}
	scanned := 0
	for {
		listed, err := cs.CoreV1().ConfigMaps(namespace).List(ctx, opts)
		if err != nil {
			return nil, err
		}
		for i := range listed.Items {
			scanned++
			if scanned > limit {
				return nil, fmt.Errorf("more than %d release config maps matched", limit)
			}
			entry := &listed.Items[i]
			body, ok := entry.Data[releaseKey]
			if !ok {
				continue
			}
			out = append(out, storedOf(DriverConfigMap, &entry.ObjectMeta, []byte(body)))
		}
		more, nextErr := advancePage(&opts, listed.Continue, seen)
		if nextErr != nil {
			return nil, nextErr
		}
		if !more {
			return out, nil
		}
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

func (s stored) fallback() api.HelmRelease {
	return api.HelmRelease{
		Name:      s.name,
		Namespace: s.namespace,
		Revision:  s.revision,
		Status:    s.status,
		Updated:   s.created.Format(time.RFC3339),
	}
}

func (s stored) release() (api.HelmRelease, error) {
	fallback := s.fallback()
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
