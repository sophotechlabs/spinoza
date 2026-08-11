package helm

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	storageType  = "helm.sh/release.v1"
	ownerLabel   = "owner=helm"
	releaseKey   = "release"
	listTimeout  = 15 * time.Second
	pageSize     = 200
	maxSecrets   = 5000
	maxPayload   = 32 << 20
	nameLabel    = "name"
	versionLabel = "version"
	statusLabel  = "status"
)

var errNotGzip = errors.New("release payload is not gzipped json")

func List(ctx context.Context, cs kubernetes.Interface) (api.HelmReleases, error) {
	bounded, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()

	secrets, truncated, err := allSecrets(bounded, cs)
	if err != nil {
		return api.HelmReleases{Releases: []api.HelmRelease{}}, err
	}

	latest := map[string]api.HelmRelease{}
	undecodable := 0
	for i := range secrets {
		secret := &secrets[i]
		if secret.Type != storageType {
			continue
		}
		release, decodeErr := releaseOf(secret)
		if decodeErr != nil {
			undecodable++
		}
		key := release.Namespace + "/" + release.Name
		if release.Name == "" {
			continue
		}
		previous, seen := latest[key]
		if seen && previous.Revision >= release.Revision {
			continue
		}
		latest[key] = release
	}

	out := make([]api.HelmRelease, 0, len(latest))
	for _, release := range latest {
		out = append(out, release)
	}
	slices.SortFunc(out, func(left, right api.HelmRelease) int {
		if left.Namespace != right.Namespace {
			return strings.Compare(left.Namespace, right.Namespace)
		}
		return strings.Compare(left.Name, right.Name)
	})
	return api.HelmReleases{Releases: out, Error: partialMessage(undecodable, truncated)}, nil
}

func partialMessage(undecodable int, truncated bool) string {
	notes := []string{}
	if undecodable > 0 {
		notes = append(notes, fmt.Sprintf(
			"%d release payloads could not be read; their name and status come from the storage secret's labels",
			undecodable,
		))
	}
	if truncated {
		notes = append(notes, fmt.Sprintf(
			"only the first %d helm storage secrets were read, so some releases may be missing",
			maxSecrets,
		))
	}
	return strings.Join(notes, "; ")
}

func allSecrets(ctx context.Context, cs kubernetes.Interface) (secrets []corev1.Secret, truncated bool, err error) {
	out := []corev1.Secret{}
	opts := metav1.ListOptions{LabelSelector: ownerLabel, Limit: pageSize}
	for {
		page, listErr := cs.CoreV1().Secrets("").List(ctx, opts)
		if listErr != nil {
			return nil, false, listErr
		}
		out = append(out, page.Items...)
		if page.Continue == "" {
			return out, false, nil
		}
		if len(out) >= maxSecrets {
			return out, true, nil
		}
		opts.Continue = page.Continue
	}
}

type payload struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Version   int64  `json:"version"`
	Info      struct {
		Status       string `json:"status"`
		LastDeployed string `json:"last_deployed"`
		Description  string `json:"description"`
	} `json:"info"`
	Chart struct {
		Metadata struct {
			Name       string `json:"name"`
			Version    string `json:"version"`
			AppVersion string `json:"appVersion"`
		} `json:"metadata"`
	} `json:"chart"`
}

func releaseOf(secret *corev1.Secret) (api.HelmRelease, error) {
	fallback := fromLabels(secret)
	decoded, err := decode(secret.Data[releaseKey])
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
	return release, nil
}

func fromLabels(secret *corev1.Secret) api.HelmRelease {
	revision, err := strconv.ParseInt(secret.Labels[versionLabel], 10, 64)
	if err != nil {
		revision = 0
	}
	return api.HelmRelease{
		Name:      secret.Labels[nameLabel],
		Namespace: secret.Namespace,
		Revision:  revision,
		Status:    secret.Labels[statusLabel],
		Updated:   secret.CreationTimestamp.UTC().Format(time.RFC3339),
	}
}

func decode(raw []byte) (payload, error) {
	if len(raw) == 0 {
		return payload{}, errors.New("release payload is empty")
	}
	body, err := base64.StdEncoding.DecodeString(string(raw))
	if err != nil {
		body = raw
	}
	plain, err := gunzip(body)
	if err != nil {
		if !errors.Is(err, errNotGzip) {
			return payload{}, err
		}
		plain = body
	}
	var decoded payload
	unmarshalErr := json.Unmarshal(plain, &decoded)
	if unmarshalErr != nil {
		return payload{}, unmarshalErr
	}
	return decoded, nil
}

func gunzip(body []byte) ([]byte, error) {
	if len(body) < 2 || body[0] != 0x1f || body[1] != 0x8b {
		return nil, errNotGzip
	}
	reader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = reader.Close()
	}()
	return io.ReadAll(io.LimitReader(reader, maxPayload))
}
