package helm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/sophotechlabs/spinoza/internal/charts"
)

const (
	repoConfigEnv = "HELM_REPOSITORY_CONFIG"
	repoCacheEnv  = "HELM_REPOSITORY_CACHE"
)

type repoFile struct {
	Repositories []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"repositories"`
}

func RepositoryConfig() string {
	fromEnv := os.Getenv(repoConfigEnv)
	if fromEnv != "" {
		return fromEnv
	}
	dir := configHome()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "helm", "repositories.yaml")
}

func RepositoryCache() string {
	fromEnv := os.Getenv(repoCacheEnv)
	if fromEnv != "" {
		return fromEnv
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "helm", "repository")
}

func configHome() string {
	fromXDG := os.Getenv("XDG_CONFIG_HOME")
	if fromXDG != "" {
		return fromXDG
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Preferences")
	}
	return filepath.Join(home, ".config")
}

type RepoEntry struct {
	Name string
	Repo charts.Repo
}

func (s *Service) admitsRepository(raw string, oci bool) error {
	err := charts.CheckRepoURL(raw)
	if err != nil {
		return err
	}
	for _, entry := range s.repos {
		configured := strings.TrimSuffix(entry.Repo.URL, "/")
		requested := strings.TrimSuffix(raw, "/")
		if configured != requested {
			continue
		}
		if entry.Repo.OCI != oci {
			return fmt.Errorf("repository %q does not match the requested protocol", raw)
		}
		return nil
	}
	return fmt.Errorf("repository %q is not configured", raw)
}

func Repositories(path string) []RepoEntry {
	if path == "" {
		return nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var parsed repoFile
	unmarshalErr := yaml.Unmarshal(body, &parsed)
	if unmarshalErr != nil {
		return nil
	}
	out := make([]RepoEntry, 0, len(parsed.Repositories))
	for _, entry := range parsed.Repositories {
		if entry.URL == "" {
			continue
		}
		repo := charts.Repo{URL: entry.URL, OCI: strings.HasPrefix(entry.URL, "oci://")}
		out = append(out, RepoEntry{Name: entry.Name, Repo: repo})
	}
	return out
}

func SeedRepositoryCache(index *charts.Cache, repos []RepoEntry, dir string) error {
	if index == nil || dir == "" {
		return nil
	}
	failures := []error{}
	for _, entry := range repos {
		if entry.Name == "" || entry.Repo.OCI {
			continue
		}
		if filepath.Base(entry.Name) != entry.Name {
			failures = append(failures, fmt.Errorf("repository name %q is not a cache filename", entry.Name))
			continue
		}
		path := filepath.Join(dir, entry.Name+"-index.yaml")
		err := seedRepository(index, entry.Repo, path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func seedRepository(index *charts.Cache, repo charts.Repo, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat repository cache %s: %w", path, err)
	}
	seedErr := index.Seed(repo, file, info.ModTime())
	if seedErr != nil {
		return fmt.Errorf("read repository cache %s: %w", path, seedErr)
	}
	return nil
}
