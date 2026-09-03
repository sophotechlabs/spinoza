package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/actions"
	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/argocd"
	"github.com/sophotechlabs/spinoza/internal/flux"
)

const restartCall = `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"manage_workload","arguments":{"resource":"deployments","name":"web","namespace":"prod","action":"restart"}}}`

func writableCluster() *fakeCluster {
	return &fakeCluster{catalog: catalogOf(
		descriptor("apps", "v1", "deployments", "Deployment"),
		descriptor("batch", "v1", "cronjobs", "CronJob"),
		descriptor("kustomize.toolkit.fluxcd.io", "v1", "kustomizations", "Kustomization"),
		descriptor("argoproj.io", "v1alpha1", "applications", "Application"),
	)}
}

func TestNoWriteToolExistsUnlessWritesAreAllowed(t *testing.T) {
	server := serverFor(writableCluster(), Options{})

	for name := range writeToolNames {
		if offered(server, name) {
			t.Fatalf("%s is offered on a read-only server", name)
		}
	}
	if len(server.Tools()) == 0 {
		t.Fatal("a read-only server offers nothing at all")
	}
}

func TestAProtectedClusterWithholdsEveryWriteToolEvenWhenAsked(t *testing.T) {
	server := serverFor(writableCluster(), Options{AllowWrite: true, Protected: always(true)})

	for name := range writeToolNames {
		if offered(server, name) {
			t.Fatalf("%s is offered on a protected cluster", name)
		}
	}
}

func TestAWithheldWriteToolIsRefusedWhenItIsCalledAnyway(t *testing.T) {
	server := serverFor(writableCluster(), Options{AllowWrite: true, Protected: always(true)})

	reply := ask(t, server, restartCall)

	message := as[string](t, errorOf(t, reply)["message"])
	if !strings.Contains(message, "read-only") {
		t.Fatalf("message = %q, want the refusal to say this server may not write", message)
	}
}

func TestAProtectionSetAfterTheServerStartedIsHonoured(t *testing.T) {
	protected := false
	cluster := writableCluster()
	server := serverFor(cluster, Options{
		AllowWrite: true,
		Protected: func() bool {
			return protected
		},
	})

	for name := range writeToolNames {
		if !offered(server, name) {
			t.Fatalf("%s is missing while the cluster is unprotected", name)
		}
	}

	protected = true

	for name := range writeToolNames {
		if offered(server, name) {
			t.Fatalf("%s is still offered after the cluster was marked protected", name)
		}
	}
	if _, refused := ask(t, server, restartCall)["error"]; !refused {
		t.Fatal("a write tool still ran after the cluster was marked protected")
	}
	if len(cluster.acted) != 0 {
		t.Fatal("a write reached the cluster after it was protected")
	}
}

func TestTheCLICallRechecksProtectionBeforeWriting(t *testing.T) {
	protected := false
	server := serverFor(writableCluster(), Options{
		AllowWrite: true,
		Protected: func() bool {
			return protected
		},
	})
	protected = true

	err := server.Call(context.Background(), &strings.Builder{}, "manage_workload", []string{
		"resource=deployments",
		"name=web",
		"namespace=prod",
		"action=restart",
	})

	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("error = %v, want the protected-cluster refusal", err)
	}
}

func TestAskingForAWithheldWriteToolSaysWhy(t *testing.T) {
	server := serverFor(writableCluster(), Options{})

	message := server.unknown("manage_node")

	if !strings.Contains(message, "read-only") {
		t.Fatalf("message = %q, want the reason rather than a bare not-found", message)
	}
	if plain := server.unknown("not_a_tool"); strings.Contains(plain, "read-only") {
		t.Fatalf("message = %q, want a plain not-found for a name nobody serves", plain)
	}
}

func TestEveryWriteToolIsOfferedWhenWritesAreAllowed(t *testing.T) {
	server := serverFor(writableCluster(), Options{AllowWrite: true})

	for name := range writeToolNames {
		if !offered(server, name) {
			t.Fatalf("%s is missing from a writable server", name)
		}
	}
}

func TestARefusedCapabilityStopsTheWriteAndCarriesTheReason(t *testing.T) {
	cluster := writableCluster()
	cluster.refused = api.Access{Refused: []api.Refusal{
		{Capability: "scale", Reason: "you may not scale apps/deployments here"},
	}}
	server := serverFor(cluster, Options{AllowWrite: true})

	err := refuses(t, server, "manage_workload", arguments{
		"resource": "deployments", "name": "web", "namespace": "prod",
		"action": "scale", "replicas": float64(3),
	})

	if err.Error() != "you may not scale apps/deployments here" {
		t.Fatalf("error = %q, want the apiserver's own reason", err)
	}
	if len(cluster.acted) != 0 {
		t.Fatalf("the action ran anyway: %v", cluster.acted)
	}
}

func TestInverseActionsUseTheCapabilityTheyShare(t *testing.T) {
	cases := []struct {
		name       string
		tool       string
		arguments  arguments
		capability string
	}{
		{
			name:       "uncordon uses cordon access",
			tool:       "manage_node",
			arguments:  arguments{"name": "worker-1", "action": "uncordon"},
			capability: "cordon",
		},
		{
			name: "resume uses suspend access",
			tool: "manage_cronjob",
			arguments: arguments{
				"name": "nightly", "namespace": "prod", "action": "resume",
			},
			capability: "suspend",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cluster := writableCluster()
			cluster.refused = api.Access{Refused: []api.Refusal{{
				Capability: tc.capability,
				Reason:     "the access review refused the shared capability",
			}}}
			server := serverFor(cluster, Options{AllowWrite: true})

			err := refuses(t, server, tc.tool, tc.arguments)

			if err.Error() != "the access review refused the shared capability" {
				t.Fatalf("error = %q, want the access review's reason", err)
			}
			if len(cluster.acted) != 0 {
				t.Fatalf("the action ran after its shared capability was refused: %v", cluster.acted)
			}
		})
	}
}

func TestARefusedGitopsCapabilityStopsTheWriteAndCarriesTheReason(t *testing.T) {
	cluster := writableCluster()
	cluster.refused = api.Access{Refused: []api.Refusal{
		{Capability: "reconcile", Reason: "you may not patch GitOps objects here"},
	}}
	server := serverFor(cluster, Options{AllowWrite: true})

	err := refuses(t, server, "manage_gitops", arguments{
		"engine": "flux", "resource": "kustomizations", "name": "apps",
		"namespace": "flux-system", "action": "reconcile",
	})

	if err.Error() != "you may not patch GitOps objects here" {
		t.Fatalf("error = %q, want the apiserver's own reason", err)
	}
	if len(cluster.fluxCalls) != 0 {
		t.Fatalf("the GitOps action ran anyway: %v", cluster.fluxCalls)
	}
}

func TestARefusalOfAnotherCapabilityDoesNotBlockThisOne(t *testing.T) {
	cluster := writableCluster()
	cluster.refused = api.Access{Refused: []api.Refusal{{Capability: "delete", Reason: "no"}}}
	server := serverFor(cluster, Options{AllowWrite: true})

	run(t, server, "manage_workload", arguments{
		"resource": "deployments", "name": "web", "namespace": "prod", "action": "restart",
	})

	if len(cluster.acted) != 1 {
		t.Fatalf("acted = %v, want the restart to have run", cluster.acted)
	}
}

func TestScalingCarriesTheReplicaCount(t *testing.T) {
	cluster := writableCluster()
	server := serverFor(cluster, Options{AllowWrite: true})

	run(t, server, "manage_workload", arguments{
		"resource": "deployments", "name": "web", "namespace": "prod",
		"action": "scale", "replicas": float64(4),
	})

	if len(cluster.acted) != 1 {
		t.Fatalf("acted = %v", cluster.acted)
	}
	req := cluster.acted[0]
	if req.Action != actions.Scale || req.Replicas != 4 {
		t.Fatalf("request = %+v, want a scale to 4", req)
	}
	if req.Ref.Resource != "deployments" || req.Ref.Namespace != "prod" {
		t.Fatalf("ref = %+v", req.Ref)
	}
}

func TestScalingWithNoCountIsRefusedRatherThanScalingToZero(t *testing.T) {
	cluster := writableCluster()
	server := serverFor(cluster, Options{AllowWrite: true})

	err := refuses(t, server, "manage_workload", arguments{
		"resource": "deployments", "name": "web", "namespace": "prod", "action": "scale",
	})

	if !strings.Contains(err.Error(), "replicas is required") {
		t.Fatalf("error = %q", err)
	}
	if len(cluster.acted) != 0 {
		t.Fatalf("a workload was scaled with no count: %v", cluster.acted)
	}
}

func TestScalingToZeroIsAllowedWhenItIsAskedFor(t *testing.T) {
	cluster := writableCluster()
	server := serverFor(cluster, Options{AllowWrite: true})

	run(t, server, "manage_workload", arguments{
		"resource": "deployments", "name": "web", "namespace": "prod",
		"action": "scale", "replicas": float64(0),
	})

	if cluster.acted[0].Replicas != 0 {
		t.Fatalf("replicas = %d, want the zero that was asked for", cluster.acted[0].Replicas)
	}
}

func TestAWorkloadActionOutsideTheListIsRefused(t *testing.T) {
	server := serverFor(writableCluster(), Options{AllowWrite: true})

	err := refuses(t, server, "manage_workload", arguments{
		"resource": "deployments", "name": "web", "namespace": "prod", "action": "delete",
	})

	if !strings.Contains(err.Error(), "scale, restart") {
		t.Fatalf("error = %q", err)
	}
}

func TestNodeActionsNameTheNodeAndCarryForce(t *testing.T) {
	cluster := writableCluster()
	server := serverFor(cluster, Options{AllowWrite: true})

	run(t, server, "manage_node", arguments{"name": "worker-1", "action": "drain", "force": true})

	req := cluster.acted[0]
	if req.Ref.Resource != "nodes" || req.Ref.Name != "worker-1" {
		t.Fatalf("ref = %+v", req.Ref)
	}
	if req.Action != actions.Drain || !req.Force {
		t.Fatalf("request = %+v, want a forced drain", req)
	}
}

func TestDrainingIsAnnotatedAsDestructive(t *testing.T) {
	server := serverFor(writableCluster(), Options{AllowWrite: true})

	if !server.tools["manage_node"].card().Annotations.DestructiveHint {
		t.Fatal("draining a node is not annotated destructive; a client cannot warn about it")
	}
	if server.tools["manage_cronjob"].card().Annotations.DestructiveHint {
		t.Fatal("suspending a cron job is annotated destructive")
	}
}

func TestANodeActionNeedsANameAndAKnownVerb(t *testing.T) {
	server := serverFor(writableCluster(), Options{AllowWrite: true})

	if err := refuses(t, server, "manage_node", arguments{"action": "drain"}); err == nil {
		t.Fatal("a node action with no name was accepted")
	}
	if err := refuses(t, server, "manage_node", arguments{"name": "worker-1", "action": "delete"}); err == nil {
		t.Fatal("an unknown node verb was accepted")
	}
}

func TestACronJobActionBuildsItsOwnReference(t *testing.T) {
	cluster := writableCluster()
	server := serverFor(cluster, Options{AllowWrite: true})

	run(t, server, "manage_cronjob", arguments{"name": "nightly", "namespace": "prod", "action": "trigger"})

	req := cluster.acted[0]
	if req.Ref.Group != "batch" || req.Ref.Resource != "cronjobs" {
		t.Fatalf("ref = %+v", req.Ref)
	}
	if req.Action != actions.Trigger {
		t.Fatalf("action = %q", req.Action)
	}
}

func TestACronJobActionNeedsEverything(t *testing.T) {
	server := serverFor(writableCluster(), Options{AllowWrite: true})

	if err := refuses(t, server, "manage_cronjob", arguments{"namespace": "prod", "action": "trigger"}); err == nil {
		t.Fatal("a cron job action with no name was accepted")
	}
	if err := refuses(t, server, "manage_cronjob", arguments{"name": "nightly", "action": "trigger"}); err == nil {
		t.Fatal("a cron job action with no namespace was accepted")
	}
	if err := refuses(t, server, "manage_cronjob", arguments{"name": "n", "namespace": "p", "action": "delete"}); err == nil {
		t.Fatal("an unknown cron job verb was accepted")
	}
}

func TestFluxAndArgoGoToDifferentControllers(t *testing.T) {
	cluster := writableCluster()
	server := serverFor(cluster, Options{AllowWrite: true})

	run(t, server, "manage_gitops", arguments{
		"engine": "flux", "resource": "kustomizations", "name": "apps",
		"namespace": "flux-system", "action": "reconcile",
	})
	run(t, server, "manage_gitops", arguments{
		"engine": "argo", "resource": "applications", "name": "podinfo",
		"namespace": "argocd", "action": "sync",
	})

	if len(cluster.fluxCalls) != 1 || cluster.fluxCalls[0] != flux.Reconcile {
		t.Fatalf("flux calls = %v", cluster.fluxCalls)
	}
	if len(cluster.argoCalls) != 1 || cluster.argoCalls[0].Action != argocd.Sync {
		t.Fatalf("argo calls = %v", cluster.argoCalls)
	}
}

func TestAGitopsCallNeedsAnEngineItKnows(t *testing.T) {
	server := serverFor(writableCluster(), Options{AllowWrite: true})

	err := refuses(t, server, "manage_gitops", arguments{
		"engine": "spinnaker", "resource": "kustomizations", "name": "apps",
		"namespace": "flux-system", "action": "reconcile",
	})

	if !strings.Contains(err.Error(), "flux, argo") {
		t.Fatalf("error = %q", err)
	}
}

func TestAGitopsFailureComesBack(t *testing.T) {
	cluster := writableCluster()
	cluster.gitopsErr = errRefused
	server := serverFor(cluster, Options{AllowWrite: true})

	if err := refuses(t, server, "manage_gitops", arguments{
		"engine": "flux", "resource": "kustomizations", "name": "apps",
		"namespace": "flux-system", "action": "reconcile",
	}); !errors.Is(err, errRefused) {
		t.Fatalf("error = %v", err)
	}
	if err := refuses(t, server, "manage_gitops", arguments{
		"engine": "argo", "resource": "applications", "name": "podinfo",
		"namespace": "argocd", "action": "sync",
	}); !errors.Is(err, errRefused) {
		t.Fatalf("error = %v", err)
	}
}

func TestAGitopsCallNeedsAnActionAndAResourceItCanFind(t *testing.T) {
	server := serverFor(writableCluster(), Options{AllowWrite: true})

	if err := refuses(t, server, "manage_gitops", arguments{
		"engine": "flux", "resource": "widgets", "name": "apps", "namespace": "x", "action": "reconcile",
	}); err == nil {
		t.Fatal("a gitops call named a kind nobody serves and was accepted")
	}
	if err := refuses(t, server, "manage_gitops", arguments{
		"engine": "flux", "resource": "kustomizations", "name": "apps", "namespace": "x",
	}); err == nil {
		t.Fatal("a gitops call with no action was accepted")
	}
}

func TestApplyingSendsTheDocumentAndNamesWhatChanged(t *testing.T) {
	cluster := writableCluster()
	cluster.detail = api.ObjectDetail{Kind: "Deployment", Name: "web", Namespace: "prod"}
	server := serverFor(cluster, Options{AllowWrite: true})

	result := run(t, server, "apply_resource", arguments{
		"resource": "deployments", "name": "web", "namespace": "prod",
		"yaml": "spec:\n  replicas: 2\n",
	})

	if string(cluster.applied) != "spec:\n  replicas: 2\n" {
		t.Fatalf("applied = %q", cluster.applied)
	}
	if result["applied"] != "Deployment/web" {
		t.Fatalf("applied = %v", result["applied"])
	}
}

func TestApplyingIsRefusedWhenEditingIs(t *testing.T) {
	cluster := writableCluster()
	cluster.refused = api.Access{Refused: []api.Refusal{{Capability: "edit", Reason: "read-only here"}}}
	server := serverFor(cluster, Options{AllowWrite: true})

	err := refuses(t, server, "apply_resource", arguments{
		"resource": "deployments", "name": "web", "namespace": "prod", "yaml": "spec: {}",
	})

	if err.Error() != "read-only here" {
		t.Fatalf("error = %q", err)
	}
	if cluster.applied != nil {
		t.Fatal("the document was applied despite the refusal")
	}
}

func TestApplyingNeedsADocumentAndPassesFailuresBack(t *testing.T) {
	cluster := writableCluster()
	server := serverFor(cluster, Options{AllowWrite: true})

	if err := refuses(t, server, "apply_resource", arguments{
		"resource": "deployments", "name": "web", "namespace": "prod",
	}); !strings.Contains(err.Error(), "yaml is required") {
		t.Fatalf("error = %v", err)
	}
	cluster.applyErr = errRefused
	if err := refuses(t, server, "apply_resource", arguments{
		"resource": "deployments", "name": "web", "namespace": "prod", "yaml": "spec: {}",
	}); !errors.Is(err, errRefused) {
		t.Fatalf("error = %v", err)
	}
}

func TestAWriteThatFailsCarriesTheApiserverMessage(t *testing.T) {
	cluster := writableCluster()
	cluster.actErr = errRefused
	server := serverFor(cluster, Options{AllowWrite: true})

	if err := refuses(t, server, "manage_node", arguments{"name": "worker-1", "action": "cordon"}); !errors.Is(err, errRefused) {
		t.Fatalf("error = %v", err)
	}
}

func TestAReplicaCountMustBeAWholeNumber(t *testing.T) {
	cases := []struct {
		name     string
		replicas any
		fails    string
	}{
		{name: "a number", replicas: float64(3)},
		{name: "zero", replicas: float64(0)},
		{name: "a number in a string", replicas: "3"},
		{name: "a word", replicas: "three", fails: "whole number"},
		{name: "a fraction", replicas: 1.5, fails: "whole number"},
		{name: "a boolean", replicas: true, fails: "whole number"},
		{name: "negative", replicas: float64(-1), fails: "negative"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cluster := writableCluster()
			server := serverFor(cluster, Options{AllowWrite: true})
			args := arguments{
				argResource: "deployments", argName: "web", argNamespace: "prod",
				argAction: "scale", argReplicas: tc.replicas,
			}

			_, err := server.tools["manage_workload"].run(t.Context(), args)

			if tc.fails != "" {
				if err == nil {
					t.Fatalf("%v was accepted and the workload scaled to %d", tc.replicas, cluster.acted[0].Replicas)
				}
				if !strings.Contains(err.Error(), tc.fails) {
					t.Fatalf("error = %q, want it to say %q", err, tc.fails)
				}
				return
			}
			if err != nil {
				t.Fatalf("%v was refused: %v", tc.replicas, err)
			}
			if cluster.acted[0].Replicas != 3 && cluster.acted[0].Replicas != 0 {
				t.Fatalf("replicas = %d", cluster.acted[0].Replicas)
			}
		})
	}
}

func TestEachEngineOnlyTakesItsOwnVerbs(t *testing.T) {
	cases := []struct {
		name   string
		engine string
		action string
		fails  bool
	}{
		{name: "flux reconciles", engine: "flux", action: "reconcile"},
		{name: "flux suspends", engine: "flux", action: "suspend"},
		{name: "flux cannot sync", engine: "flux", action: "sync", fails: true},
		{name: "argo syncs", engine: "argo", action: "sync"},
		{name: "argo terminates", engine: "argo", action: "terminate"},
		{name: "argo cannot reconcile", engine: "argo", action: "reconcile", fails: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cluster := writableCluster()
			server := serverFor(cluster, Options{AllowWrite: true})
			args := arguments{
				argEngine: tc.engine, argResource: "kustomizations", argName: "apps",
				argNamespace: "flux-system", argAction: tc.action,
			}

			_, err := server.tools["manage_gitops"].run(t.Context(), args)

			if tc.fails {
				if err == nil {
					t.Fatalf("%s accepted %q, which belongs to the other controller", tc.engine, tc.action)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s refused its own verb %q: %v", tc.engine, tc.action, err)
			}
		})
	}
}

func TestAWorkloadActionNamesTheNamespaceItActsIn(t *testing.T) {
	cluster := writableCluster()
	server := serverFor(cluster, Options{AllowWrite: true})

	_, err := server.tools["manage_workload"].run(t.Context(), arguments{
		argResource: "deployments", argName: "web", argAction: "restart",
	})

	if err == nil {
		t.Fatalf("a workload was restarted without naming its namespace: %+v", cluster.acted)
	}
	if !strings.Contains(err.Error(), argNamespace) {
		t.Fatalf("error = %q, want it to name the namespace", err)
	}
}

func TestADryRunOnlyAppliesToDraining(t *testing.T) {
	cases := []struct {
		name   string
		action string
		dryRun bool
		want   bool
	}{
		{name: "a dry drain", action: "drain", dryRun: true, want: true},
		{name: "a real drain", action: "drain", dryRun: false, want: false},
		{name: "cordon ignores it", action: "cordon", dryRun: true, want: false},
		{name: "uncordon ignores it", action: "uncordon", dryRun: true, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cluster := writableCluster()
			server := serverFor(cluster, Options{AllowWrite: true})

			run(t, server, "manage_node", arguments{
				argName: "worker-1", argAction: tc.action, "dryRun": tc.dryRun,
			})

			if cluster.acted[0].DryRun != tc.want {
				t.Fatalf("DryRun = %v, want %v: only draining reports a plan without acting",
					cluster.acted[0].DryRun, tc.want)
			}
		})
	}
}
