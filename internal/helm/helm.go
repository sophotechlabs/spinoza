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
	"strings"
	"time"

	"k8s.io/client-go/kubernetes"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/charts"
)

const (
	storageType  = "helm.sh/release.v1"
	ownerLabel   = "owner=helm"
	releaseKey   = "release"
	listTimeout  = 15 * time.Second
	pageSize     = 200
	maxObjects   = 5000
	maxPayload   = 32 << 20
	nameLabel    = "name"
	versionLabel = "version"
	statusLabel  = "status"
)

var errNotGzip = errors.New("release payload is not gzipped json")

type Charts interface {
	Latest(repo charts.Repo, chart string) string
	Warm(repo charts.Repo, chart string)
}

type Service struct {
	cs      kubernetes.Interface
	runner  Runner
	index   Charts
	repos   []charts.Repo
	kubeRef api.ContextRef
}

func NewService(cs kubernetes.Interface, runner Runner, index Charts, repos []charts.Repo, kubeRef api.ContextRef) *Service {
	return &Service{cs: cs, runner: runner, index: index, repos: repos, kubeRef: kubeRef}
}

func (s *Service) List(ctx context.Context) (api.HelmReleases, error) {
	bounded, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()

	found, err := loadAll(bounded, s.cs)
	if err != nil {
		return api.HelmReleases{Releases: []api.HelmRelease{}}, err
	}

	latest := map[string]api.HelmRelease{}
	undecodable := 0
	for _, item := range found.items {
		release, decodeErr := item.release()
		if decodeErr != nil {
			undecodable++
		}
		if release.Name == "" {
			continue
		}
		key := release.Namespace + "/" + release.Name
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
	slices.SortFunc(out, byNamespaceThenName)
	s.addLatest(out)
	return api.HelmReleases{Releases: out, Error: partialMessage(undecodable, found.truncated, found.denied)}, nil
}

func byNamespaceThenName(left, right api.HelmRelease) int {
	if left.Namespace != right.Namespace {
		return strings.Compare(left.Namespace, right.Namespace)
	}
	return strings.Compare(left.Name, right.Name)
}

func (s *Service) addLatest(releases []api.HelmRelease) {
	if s.index == nil {
		return
	}
	for i := range releases {
		release := &releases[i]
		if release.Chart == "" {
			continue
		}
		newest := ""
		for _, repo := range s.repos {
			s.index.Warm(repo, release.Chart)
			newest = pick(newest, s.index.Latest(repo, release.Chart))
		}
		release.Latest = newest
		release.Outdated = charts.Newer(release.ChartVersion, newest)
	}
}

func pick(current, found string) string {
	if found == "" {
		return current
	}
	if current == "" {
		return found
	}
	if charts.Newer(current, found) {
		return found
	}
	return current
}

func partialMessage(undecodable int, truncated bool, denied string) string {
	notes := []string{}
	if denied != "" {
		notes = append(notes, denied)
	}
	if undecodable > 0 {
		notes = append(notes, fmt.Sprintf(
			"%d release payloads could not be read; their name and status come from the storage object's labels",
			undecodable,
		))
	}
	if truncated {
		notes = append(notes, fmt.Sprintf(
			"only the first %d helm storage objects were read, so some releases may be missing",
			maxObjects,
		))
	}
	return strings.Join(notes, "; ")
}

type payload struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Version   int64  `json:"version"`
	Info      struct {
		Status        string `json:"status"`
		FirstDeployed string `json:"first_deployed"`
		LastDeployed  string `json:"last_deployed"`
		Description   string `json:"description"`
		Notes         string `json:"notes"`
	} `json:"info"`
	Chart struct {
		Metadata struct {
			Name       string `json:"name"`
			Version    string `json:"version"`
			AppVersion string `json:"appVersion"`
		} `json:"metadata"`
	} `json:"chart"`
	Config   map[string]any `json:"config"`
	Manifest string         `json:"manifest"`
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
