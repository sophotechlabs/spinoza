package helm

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/charts"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "repositories.yaml")
	err := os.WriteFile(path, []byte(body), 0o600)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestRepositoriesReadsTheHelmConfig(t *testing.T) {
	path := writeConfig(t, `apiVersion: ""
repositories:
  - name: bitnami
    url: https://charts.bitnami.com/bitnami
  - name: private
    url: oci://ghcr.io/acme/charts
`)

	got := Repositories(path)

	if len(got) != 2 {
		t.Fatalf("repositories = %d, want 2", len(got))
	}
	if got[0].Repo.URL != "https://charts.bitnami.com/bitnami" {
		t.Fatalf("url = %q, want the bitnami index", got[0].Repo.URL)
	}
	if got[0].Name != "bitnami" {
		t.Fatalf("name = %q, want the alias helm keeps", got[0].Name)
	}
	if got[0].Repo.OCI {
		t.Fatal("an https repo was marked as oci")
	}
	if !got[1].Repo.OCI {
		t.Fatal("an oci:// repo was not marked as oci")
	}
}

func TestRepositoriesSkipsAnEntryWithNoUrl(t *testing.T) {
	path := writeConfig(t, "repositories:\n  - name: broken\n  - name: fine\n    url: https://one.example.com\n")

	got := Repositories(path)

	if len(got) != 1 {
		t.Fatalf("repositories = %d, want the one with a url", len(got))
	}
}

func TestRepositoriesIsEmptyWhenHelmWasNeverUsed(t *testing.T) {
	if len(Repositories(filepath.Join(t.TempDir(), "missing.yaml"))) != 0 {
		t.Fatal("a missing config produced repositories")
	}
	if len(Repositories("")) != 0 {
		t.Fatal("an empty path produced repositories")
	}
}

func TestRepositoriesIgnoresAConfigItCannotParse(t *testing.T) {
	path := writeConfig(t, "repositories: [this is not: valid: yaml\n")

	if len(Repositories(path)) != 0 {
		t.Fatal("a broken config produced repositories")
	}
}

func TestTheConfigPathFollowsHelmsOwnEnvironment(t *testing.T) {
	t.Setenv(repoConfigEnv, "/somewhere/repositories.yaml")

	if RepositoryConfig() != "/somewhere/repositories.yaml" {
		t.Fatalf("path = %q, want the environment's", RepositoryConfig())
	}
}

func TestTheConfigPathFollowsXdgWhenItIsSet(t *testing.T) {
	t.Setenv(repoConfigEnv, "")
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	if RepositoryConfig() != filepath.Join(xdg, "helm", "repositories.yaml") {
		t.Fatalf("path = %q, want it under XDG_CONFIG_HOME", RepositoryConfig())
	}
}

func TestTheConfigPathIsWhereHelmKeepsItOnThisPlatform(t *testing.T) {
	t.Setenv(repoConfigEnv, "")
	t.Setenv("XDG_CONFIG_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := RepositoryConfig()

	want := filepath.Join(home, ".config", "helm", "repositories.yaml")
	if runtime.GOOS == "darwin" {
		want = filepath.Join(home, "Library", "Preferences", "helm", "repositories.yaml")
	}
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestTheConfigPathIsEmptyWithoutAHome(t *testing.T) {
	t.Setenv(repoConfigEnv, "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")

	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		if RepositoryConfig() != "" {
			t.Fatalf("path = %q, want it empty with no home to look in", RepositoryConfig())
		}
	}
}

func TestTheRepositoryCachePathFollowsHelmsEnvironment(t *testing.T) {
	t.Setenv(repoCacheEnv, "/somewhere/repository")

	if RepositoryCache() != "/somewhere/repository" {
		t.Fatalf("path = %q, want the environment's", RepositoryCache())
	}
}

func TestTheRepositoryCachePathIsWhereHelmKeepsIt(t *testing.T) {
	t.Setenv(repoCacheEnv, "")
	t.Setenv("XDG_CACHE_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := RepositoryCache()

	want := filepath.Join(home, ".cache", "helm", "repository")
	if runtime.GOOS == "darwin" {
		want = filepath.Join(home, "Library", "Caches", "helm", "repository")
	}
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestTheRepositoryCachePathIsEmptyWithoutACacheHome(t *testing.T) {
	t.Setenv(repoCacheEnv, "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")

	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		if RepositoryCache() != "" {
			t.Fatalf("path = %q, want it empty with no cache home", RepositoryCache())
		}
	}
}

func TestAConfiguredRepositoryUsesHelmsCachedIndex(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "platform-index.yaml")
	body := "entries:\n  podinfo:\n    - version: 6.15.1\n    - version: 6.14.0\n"
	writeErr := os.WriteFile(path, []byte(body), 0o600)
	if writeErr != nil {
		t.Fatalf("write cache: %v", writeErr)
	}
	index := charts.New(t.Context(), nil, time.Hour)
	repo := charts.Repo{URL: "https://charts.example.com"}

	err := SeedRepositoryCache(index, []RepoEntry{{Name: "platform", Repo: repo}}, dir)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	versions, versionsErr := index.Versions(t.Context(), repo, "podinfo")
	if versionsErr != nil {
		t.Fatalf("versions: %v", versionsErr)
	}
	if strings.Join(versions, ",") != "6.15.1,6.14.0" {
		t.Fatalf("versions = %v, want Helm's cached versions", versions)
	}
}

func TestMissingAndOCIRepositoryCachesNeedNoIndexFile(t *testing.T) {
	index := charts.New(t.Context(), nil, time.Hour)
	repos := []RepoEntry{
		{Name: "missing", Repo: charts.Repo{URL: "https://charts.example.com"}},
		{Name: "oci", Repo: charts.Repo{URL: "oci://registry.example.com/charts", OCI: true}},
	}

	if err := SeedRepositoryCache(index, repos, t.TempDir()); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestSeedingNeedsAnIndexAndACacheDirectory(t *testing.T) {
	index := charts.New(t.Context(), nil, time.Hour)
	repos := []RepoEntry{{Name: "platform", Repo: charts.Repo{URL: "https://charts.example.com"}}}

	if err := SeedRepositoryCache(nil, repos, t.TempDir()); err != nil {
		t.Fatalf("nil index: %v", err)
	}
	if err := SeedRepositoryCache(index, repos, ""); err != nil {
		t.Fatalf("empty directory: %v", err)
	}
}

func TestARepositoryNameCannotEscapeTheCacheDirectory(t *testing.T) {
	index := charts.New(t.Context(), nil, time.Hour)
	repos := []RepoEntry{{Name: "../outside", Repo: charts.Repo{URL: "https://charts.example.com"}}}

	err := SeedRepositoryCache(index, repos, t.TempDir())
	if err == nil {
		t.Fatal("a repository name escaped the cache directory")
	}
	if !strings.Contains(err.Error(), "not a cache filename") {
		t.Fatalf("error = %q, want the unsafe name explained", err.Error())
	}
}

func TestABrokenRepositoryCacheIsReported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken-index.yaml")
	writeErr := os.WriteFile(path, []byte("entries: [not closed"), 0o600)
	if writeErr != nil {
		t.Fatalf("write cache: %v", writeErr)
	}
	index := charts.New(t.Context(), nil, time.Hour)
	repos := []RepoEntry{{Name: "broken", Repo: charts.Repo{URL: "https://charts.example.com"}}}

	err := SeedRepositoryCache(index, repos, dir)
	if err == nil {
		t.Fatal("a malformed cached index was ignored")
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("error = %q, want it to name %q", err.Error(), path)
	}
}
