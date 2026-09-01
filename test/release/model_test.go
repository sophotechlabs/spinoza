package release_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type releaseConfig struct {
	LastReleaseSHA string                    `json:"last-release-sha"`
	Packages       map[string]releasePackage `json:"packages"`
}

type releasePackage struct {
	ForceTag   bool        `json:"force-tag-creation"`
	ExtraFiles []extraFile `json:"extra-files"`
}

type extraFile struct {
	Type     string `json:"type"`
	Path     string `json:"path"`
	JSONPath string `json:"jsonpath"`
}

type chart struct {
	Version    string `yaml:"version"`
	AppVersion string `yaml:"appVersion"`
}

type wails struct {
	Info struct {
		ProductVersion string `json:"productVersion"`
	} `json:"info"`
}

type workflow struct {
	On struct {
		Push struct {
			Paths []string `yaml:"paths"`
		} `yaml:"push"`
	} `yaml:"on"`
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	If          string            `yaml:"if"`
	Needs       stringList        `yaml:"needs"`
	Outputs     map[string]string `yaml:"outputs"`
	Permissions map[string]string `yaml:"permissions"`
	Steps       []workflowStep    `yaml:"steps"`
}

type workflowStep struct {
	ID   string            `yaml:"id"`
	Name string            `yaml:"name"`
	If   string            `yaml:"if"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	Env  map[string]string `yaml:"env"`
	With map[string]any    `yaml:"with"`
}

type stringList []string

func (values *stringList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*values = []string{node.Value}
		return nil
	case yaml.SequenceNode:
		for _, item := range node.Content {
			if item.Kind != yaml.ScalarNode {
				return fmt.Errorf("need is not a string")
			}
			*values = append(*values, item.Value)
		}
		return nil
	default:
		return fmt.Errorf("needs is neither a string nor a list")
	}
}

func readJSON[T any](t *testing.T, name string) T {
	t.Helper()
	var value T
	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return value
}

func readYAML[T any](t *testing.T, name string) T {
	t.Helper()
	var value T
	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), name))
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, &value); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return value
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func requirePackage(t *testing.T, config releaseConfig) releasePackage {
	t.Helper()
	pkg, ok := config.Packages["."]
	if !ok {
		t.Fatal("release-please config has no root package")
	}
	return pkg
}

func requireJob(t *testing.T, workflow workflow, name string) workflowJob {
	t.Helper()
	job, ok := workflow.Jobs[name]
	if !ok {
		t.Fatalf("release artifact workflow has no %s job", name)
	}
	return job
}

func requireStep(t *testing.T, job workflowJob, id string) workflowStep {
	t.Helper()
	for _, step := range job.Steps {
		if step.ID == id {
			return step
		}
	}
	t.Fatalf("job has no %s step", id)
	return workflowStep{}
}

func requireNamedStep(t *testing.T, job workflowJob, name string) workflowStep {
	t.Helper()
	for _, step := range job.Steps {
		if step.Name == name {
			return step
		}
	}
	t.Fatalf("job has no %q step", name)
	return workflowStep{}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsRun(steps []workflowStep, want string) bool {
	for _, step := range steps {
		if strings.Contains(step.Run, want) {
			return true
		}
	}
	return false
}
