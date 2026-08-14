package helm

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
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
