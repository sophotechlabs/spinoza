package mcp

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func answeringCluster() *fakeCluster {
	return &fakeCluster{
		catalog: catalogOf(
			descriptor("apps", "v1", "deployments", "Deployment"),
			descriptor("", "v1", "pods", "Pod"),
			descriptor("batch", "v1", "cronjobs", "CronJob"),
			descriptor("kustomize.toolkit.fluxcd.io", "v1", "kustomizations", "Kustomization"),
			descriptor("argoproj.io", "v1alpha1", "applications", "Application"),
		),
		spaces:   api.Namespaces{Names: []string{"prod"}},
		detail:   api.ObjectDetail{Kind: "Deployment", Name: "web", Namespace: "prod"},
		selector: "app=web",
		lines:    []string{"ready"},
		usage:    api.Metrics{Pods: map[string]api.ResourceUsage{"prod/web-0": {CPUMilli: 1}}},
	}
}

var verbFor = map[string]string{
	"manage_workload": "restart",
	"manage_node":     "drain",
	"manage_cronjob":  "trigger",
	"manage_gitops":   "reconcile",
}

func everyArgument(tool string) arguments {
	args := arguments{
		argResource:  "deployments",
		argName:      "web",
		argNamespace: "prod",
		argQuery:     "up",
		argYAML:      "spec: {}",
		"uid":        "uid-1",
		argEngine:    "flux",
		argReplicas:  float64(1),
	}
	if verb, held := verbFor[tool]; held {
		args[argAction] = verb
	}
	if tool == "manage_cronjob" {
		args[argResource] = "cronjobs"
	}
	if tool == "manage_gitops" {
		args[argResource] = "kustomizations"
	}
	return args
}

func TestNoReadToolEverTouchesAWritePath(t *testing.T) {
	for _, card := range serverFor(answeringCluster(), Options{AllowWrite: true}).cards() {
		if !card.Annotations.ReadOnlyHint {
			continue
		}
		t.Run(card.Name, func(t *testing.T) {
			cluster := answeringCluster()
			server := serverFor(cluster, Options{AllowWrite: true, Prometheus: &fakeProm{}})
			_, _ = server.tools[card.Name].run(context.Background(), everyArgument(card.Name))

			if len(cluster.acted) != 0 {
				t.Fatalf("%s ran %d cluster actions", card.Name, len(cluster.acted))
			}
			if cluster.applied != nil {
				t.Fatalf("%s applied a document", card.Name)
			}
			if len(cluster.fluxCalls) != 0 || len(cluster.argoCalls) != 0 {
				t.Fatalf("%s drove a GitOps controller", card.Name)
			}
		})
	}
}

func TestEveryRequiredArgumentIsEnforced(t *testing.T) {
	server := serverFor(answeringCluster(), Options{AllowWrite: true, Prometheus: &fakeProm{}})

	for _, card := range server.cards() {
		for _, needed := range card.InputSchema.Required {
			t.Run(card.Name+"/"+needed, func(t *testing.T) {
				args := everyArgument(card.Name)
				delete(args, needed)
				_, err := server.tools[card.Name].run(context.Background(), args)
				if err == nil {
					t.Fatalf("%s ran without %s, which its schema calls required", card.Name, needed)
				}
				if !strings.Contains(err.Error(), needed) {
					t.Fatalf("%s failed without naming %s: %v", card.Name, needed, err)
				}
			})
		}
	}
}

func TestEveryToolIsNamedTheWayClientsExpect(t *testing.T) {
	shape := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

	for _, card := range serverFor(answeringCluster(), Options{AllowWrite: true}).cards() {
		if !shape.MatchString(card.Name) {
			t.Fatalf("%q is not lower snake case", card.Name)
		}
		if card.Title == "" {
			t.Fatalf("%s has no title for a client to show", card.Name)
		}
		if !strings.HasSuffix(card.Description, ".") {
			t.Fatalf("%s description does not read as a sentence: %q", card.Name, card.Description)
		}
		for name, prop := range card.InputSchema.Properties {
			if prop.Description == "" {
				t.Fatalf("%s.%s is undescribed", card.Name, name)
			}
			if prop.Type != "string" && prop.Type != "integer" && prop.Type != "boolean" {
				t.Fatalf("%s.%s is type %q, which no client schema expects", card.Name, name, prop.Type)
			}
		}
	}
}

func TestEveryToolReturnsSomethingOrSaysWhyNot(t *testing.T) {
	server := serverFor(answeringCluster(), Options{AllowWrite: true, Prometheus: &fakeProm{}})

	for _, card := range server.cards() {
		t.Run(card.Name, func(t *testing.T) {
			result, err := server.tools[card.Name].run(context.Background(), everyArgument(card.Name))
			if err != nil {
				t.Fatalf("%s failed on arguments that name everything: %v", card.Name, err)
			}
			if result == nil {
				t.Fatalf("%s returned nothing at all", card.Name)
			}
		})
	}
}

type slowCluster struct {
	*fakeCluster
}

func (s slowCluster) Namespaces(ctx context.Context) api.Namespaces {
	<-ctx.Done()
	return api.Namespaces{}
}

func TestAToolThatOverrunsItsBudgetIsCutOff(t *testing.T) {
	server := serverFor(slowCluster{fakeCluster: &fakeCluster{}}, Options{CallBudget: 20 * time.Millisecond})

	_, err := server.runBounded(context.Background(), server.tools["list_namespaces"], arguments{})

	if err == nil {
		t.Fatal("a tool that never finished came back as a success")
	}
	if !strings.Contains(err.Error(), "list_namespaces") {
		t.Fatalf("error = %q, want the tool named", err)
	}
}

func TestTheBudgetHasADefaultSoNoCallIsUnbounded(t *testing.T) {
	if serverFor(&fakeCluster{}, Options{}).budget != defaultCallBudget {
		t.Fatal("a server built with no budget would let a tool run forever")
	}
	if got := serverFor(&fakeCluster{}, Options{CallBudget: time.Second}).budget; got != time.Second {
		t.Fatalf("budget = %s, want the one that was asked for", got)
	}
}

func TestInvalidServerLimitsFallBackToSafeDefaults(t *testing.T) {
	server := serverFor(&fakeCluster{}, Options{LogLines: -1, CallBudget: -time.Second})

	if server.logLines != defaultLogLines {
		t.Fatalf("log lines = %d, want %d", server.logLines, defaultLogLines)
	}
	if server.budget != defaultCallBudget {
		t.Fatalf("budget = %s, want %s", server.budget, defaultCallBudget)
	}
}

func TestEveryWriteToolAsksTheAccessCheckFirst(t *testing.T) {
	for name := range writeToolNames {
		t.Run(name, func(t *testing.T) {
			cluster := answeringCluster()
			cluster.refused = api.Access{Refused: []api.Refusal{
				{Capability: "restart", Reason: "no"},
				{Capability: "drain", Reason: "no"},
				{Capability: "trigger", Reason: "no"},
				{Capability: "reconcile", Reason: "no"},
				{Capability: "edit", Reason: "no"},
			}}
			server := serverFor(cluster, Options{AllowWrite: true})

			_, err := server.tools[name].run(context.Background(), everyArgument(name))

			if err == nil {
				t.Fatalf("%s ran despite the refusal", name)
			}
			if !errors.Is(err, err) || err.Error() != "no" {
				t.Fatalf("%s returned %q, want the refusal reason verbatim", name, err)
			}
		})
	}
}
