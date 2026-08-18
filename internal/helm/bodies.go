package helm

import (
	"context"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/safe"
)

const readConcurrency = 8

type decoded struct {
	version string
	release api.HelmRelease
}

type releaseCache struct {
	mu    sync.Mutex
	items map[string]decoded
}

func newReleaseCache() *releaseCache {
	return &releaseCache{items: map[string]decoded{}}
}

func cacheKey(ref storedRef) string {
	return ref.driver + " " + ref.namespace + "/" + ref.object
}

func (c *releaseCache) get(ref storedRef) (api.HelmRelease, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	held, ok := c.items[cacheKey(ref)]
	if !ok || held.version != ref.version {
		return api.HelmRelease{}, false
	}
	return held.release, true
}

func (c *releaseCache) put(ref storedRef, release api.HelmRelease) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[cacheKey(ref)] = decoded{version: ref.version, release: release}
}

func (c *releaseCache) keep(refs []storedRef) {
	live := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		live[cacheKey(ref)] = struct{}{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.items {
		_, wanted := live[key]
		if !wanted {
			delete(c.items, key)
		}
	}
}

func (s *Service) read(ctx context.Context, refs []storedRef) ([]api.HelmRelease, int) {
	out := make([]api.HelmRelease, len(refs))
	undecodable := 0
	var mu sync.Mutex
	var group sync.WaitGroup
	slots := make(chan struct{}, readConcurrency)
	for i, ref := range refs {
		group.Add(1)
		go safe.Run("reading the release "+ref.namespace+"/"+ref.name, func() {
			defer group.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			release, ok := s.releaseFor(ctx, ref)
			mu.Lock()
			out[i] = release
			if !ok {
				undecodable++
			}
			mu.Unlock()
		})
	}
	group.Wait()
	return out, undecodable
}

func (s *Service) releaseFor(ctx context.Context, ref storedRef) (api.HelmRelease, bool) {
	held, ok := s.cache.get(ref)
	if ok {
		return held, true
	}
	body, err := s.body(ctx, ref)
	if err != nil {
		return fallbackOf(ref), false
	}
	release, decodeErr := stored{
		driver:    ref.driver,
		namespace: ref.namespace,
		name:      ref.name,
		revision:  ref.revision,
		status:    ref.status,
		created:   ref.created,
		body:      body,
	}.release()
	if decodeErr != nil {
		return release, false
	}
	s.cache.put(ref, release)
	return release, true
}

func fallbackOf(ref storedRef) api.HelmRelease {
	return stored{
		namespace: ref.namespace,
		name:      ref.name,
		revision:  ref.revision,
		status:    ref.status,
		created:   ref.created,
	}.fallback()
}

func (s *Service) body(ctx context.Context, ref storedRef) ([]byte, error) {
	if ref.driver == DriverConfigMap {
		return configMapBody(ctx, s.cs, ref)
	}
	return secretBody(ctx, s.cs, ref)
}

func secretBody(ctx context.Context, cs kubernetes.Interface, ref storedRef) ([]byte, error) {
	got, err := cs.CoreV1().Secrets(ref.namespace).Get(ctx, ref.object, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if got.Type != storageType {
		return nil, errNotRelease
	}
	return got.Data[releaseKey], nil
}

func configMapBody(ctx context.Context, cs kubernetes.Interface, ref storedRef) ([]byte, error) {
	got, err := cs.CoreV1().ConfigMaps(ref.namespace).Get(ctx, ref.object, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	body, ok := got.Data[releaseKey]
	if !ok {
		return nil, errNotRelease
	}
	return []byte(body), nil
}
