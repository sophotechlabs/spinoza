package helm

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/sophotechlabs/spinoza/internal/charts"
)

const repoConfigEnv = "HELM_REPOSITORY_CONFIG"

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

func Repositories(path string) []charts.Repo {
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
	out := make([]charts.Repo, 0, len(parsed.Repositories))
	for _, entry := range parsed.Repositories {
		if entry.URL == "" {
			continue
		}
		out = append(out, charts.Repo{URL: entry.URL, OCI: strings.HasPrefix(entry.URL, "oci://")})
	}
	return out
}
