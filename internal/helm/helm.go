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
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"

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
	maxPayload   = 8 << 20
	nameLabel    = "name"
	versionLabel = "version"
	statusLabel  = "status"
)

var errNotGzip = errors.New("release payload is not gzipped json")

var errNotRelease = errors.New("the object does not hold a helm release")

var errNoMetadata = errors.New("this cluster connection has no metadata client, so releases cannot be listed")

var releaseDecodes = newDecodeBudget(2)

type decodeBudget struct {
	slots chan struct{}
}

func newDecodeBudget(limit int) *decodeBudget {
	return &decodeBudget{slots: make(chan struct{}, limit)}
}

func (b *decodeBudget) claim() func() {
	b.slots <- struct{}{}
	return func() {
		<-b.slots
	}
}

type Charts interface {
	Latest(repo charts.Repo, chart string) string
	Warm(repo charts.Repo, chart string)
	Versions(ctx context.Context, repo charts.Repo, chart string) ([]string, error)
	Search(ctx context.Context, repo charts.Repo, query string, limit int) ([]charts.Chart, error)
}

type Service struct {
	cs             kubernetes.Interface
	meta           metadata.Interface
	runner         Runner
	index          Charts
	repos          []RepoEntry
	kubeRef        api.ContextRef
	cache          *releaseCache
	valuesMu       sync.Mutex
	values         map[ValuesRequest]cachedValues
	valuesInflight map[ValuesRequest]*valuesFlight
	valuesNow      func() time.Time
}

type cachedValues struct {
	value   api.HelmChartValues
	fetched time.Time
}

type valuesFlight struct {
	done  chan struct{}
	value api.HelmChartValues
	err   error
}

func NewService(
	cs kubernetes.Interface,
	meta metadata.Interface,
	runner Runner,
	index Charts,
	repos []RepoEntry,
	kubeRef api.ContextRef,
) *Service {
	return &Service{
		cs:             cs,
		meta:           meta,
		runner:         runner,
		index:          index,
		repos:          repos,
		kubeRef:        kubeRef,
		cache:          newReleaseCache(),
		values:         map[ValuesRequest]cachedValues{},
		valuesInflight: map[ValuesRequest]*valuesFlight{},
		valuesNow:      time.Now,
	}
}

func (s *Service) List(ctx context.Context) (api.HelmReleases, error) {
	bounded, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()

	if s.meta == nil {
		return api.HelmReleases{Releases: []api.HelmRelease{}}, errNoMetadata
	}
	found, err := allRefs(bounded, s.meta)
	if err != nil {
		return api.HelmReleases{Releases: []api.HelmRelease{}}, err
	}

	current := newestPerRelease(found.items)
	s.cache.keep(current)
	out, undecodable := s.read(bounded, current)
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
		for _, entry := range s.repos {
			s.index.Warm(entry.Repo, release.Chart)
			newest = pick(newest, s.index.Latest(entry.Repo, release.Chart))
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
	release := releaseDecodes.claim()
	defer release()
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
		if len(body) > maxPayload {
			return payload{}, fmt.Errorf("release payload is larger than %d bytes", maxPayload)
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
	return gunzipLimit(body, maxPayload)
}

func gunzipLimit(body []byte, limit int) ([]byte, error) {
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
	plain, readErr := io.ReadAll(io.LimitReader(reader, int64(limit)+1))
	if readErr != nil {
		return nil, readErr
	}
	if len(plain) > limit {
		return nil, fmt.Errorf("release payload is larger than %d bytes", limit)
	}
	return plain, nil
}
