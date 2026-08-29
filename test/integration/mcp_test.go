//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sophotechlabs/spinoza/internal/kube"
	"github.com/sophotechlabs/spinoza/internal/logs"
	"github.com/sophotechlabs/spinoza/internal/mcp"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

type mcpBackend struct {
	*resources.Manager
}

func (b mcpBackend) LogLines(ctx context.Context, req logs.Request) ([]string, error) {
	stream, err := b.Logs(ctx, req)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	out := []string{}
	for line := range stream.Lines {
		out = append(out, line.Text)
	}
	return out, stream.Err()
}

const (
	mcpWorkload = "mcp-target"
	mcpCronJob  = "mcp-cron"
	restartedAt = "kubectl.kubernetes.io/restartedAt"
)

func mcpServer(t *testing.T, loaded *kube.Bundle, allowWrite bool) *mcp.Server {
	t.Helper()
	return mcp.New(mcpBackend{Manager: manager(t, loaded)}, mcp.Options{
		Version:    "integration",
		Context:    os.Getenv("SPINOZA_TEST_CONTEXT"),
		AllowWrite: allowWrite,
		CallBudget: 60 * time.Second,
	})
}

func mcpCall(t *testing.T, server *mcp.Server, name string, args map[string]any) (string, bool) {
	t.Helper()
	frame, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  map[string]any{"name": name, "arguments": args},
	})
	if err != nil {
		t.Fatalf("marshal call: %v", err)
	}
	raw := server.Handle(context.Background(), frame)
	var reply struct {
		Error  *struct{ Message string } `json:"error"`
		Result *struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	err = json.Unmarshal(raw, &reply)
	if err != nil {
		t.Fatalf("decode reply %s: %v", raw, err)
	}
	if reply.Error != nil {
		return reply.Error.Message, true
	}
	if reply.Result == nil {
		t.Fatalf("neither result nor error in %s", raw)
	}
	parts := make([]string, 0, len(reply.Result.Content))
	for _, item := range reply.Result.Content {
		parts = append(parts, item.Text)
	}
	return strings.Join(parts, "\n"), reply.Result.IsError
}

func mcpDeployment(t *testing.T, loaded *kube.Bundle, name string) {
	t.Helper()
	labels := map[string]string{"app": name}
	one := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name:  "pause",
					Image: "registry.k8s.io/pause:3.10",
				}}},
			},
		},
	}
	_, err := loaded.Clientset.AppsV1().Deployments(namespace).Create(
		context.Background(), deployment, metav1.CreateOptions{},
	)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create deployment: %v", err)
	}
	t.Cleanup(func() {
		_ = loaded.Clientset.AppsV1().Deployments(namespace).Delete(
			context.Background(), name, metav1.DeleteOptions{},
		)
	})
}

func mcpReadDeployment(t *testing.T, loaded *kube.Bundle, name string) *appsv1.Deployment {
	t.Helper()
	found, err := loaded.Clientset.AppsV1().Deployments(namespace).Get(
		context.Background(), name, metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("read deployment: %v", err)
	}
	return found
}

func TestTheMCPServerOffersItsWriteToolsAgainstARealCluster(t *testing.T) {
	server := mcpServer(t, bundle(t), true)

	frame := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	var listing struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	err := json.Unmarshal(server.Handle(context.Background(), frame), &listing)
	if err != nil {
		t.Fatalf("decode listing: %v", err)
	}
	offered := map[string]bool{}
	for _, card := range listing.Result.Tools {
		offered[card.Name] = true
	}
	for _, want := range []string{
		"manage_workload", "manage_node", "manage_cronjob", "manage_gitops", "apply_resource",
	} {
		if !offered[want] {
			t.Errorf("%s is missing from tools/list", want)
		}
	}
}

func TestScalingThroughMCPMovesTheRealApiserver(t *testing.T) {
	loaded := bundle(t)
	mcpDeployment(t, loaded, mcpWorkload)
	server := mcpServer(t, loaded, true)

	text, failed := mcpCall(t, server, "manage_workload", map[string]any{
		"resource": "deployments", "name": mcpWorkload,
		"namespace": namespace, "action": "scale", "replicas": 3,
	})
	if failed {
		t.Fatalf("scale refused: %s", text)
	}

	found := mcpReadDeployment(t, loaded, mcpWorkload)
	if found.Spec.Replicas == nil || *found.Spec.Replicas != 3 {
		t.Fatalf("replicas = %v, want 3", found.Spec.Replicas)
	}
}

func TestRestartingThroughMCPStampsThePodTemplate(t *testing.T) {
	loaded := bundle(t)
	mcpDeployment(t, loaded, mcpWorkload)
	server := mcpServer(t, loaded, true)
	before := mcpReadDeployment(t, loaded, mcpWorkload)
	if _, stamped := before.Spec.Template.Annotations[restartedAt]; stamped {
		t.Fatalf("the deployment was already stamped before the call")
	}

	text, failed := mcpCall(t, server, "manage_workload", map[string]any{
		"resource": "deployments", "name": mcpWorkload,
		"namespace": namespace, "action": "restart",
	})
	if failed {
		t.Fatalf("restart refused: %s", text)
	}

	found := mcpReadDeployment(t, loaded, mcpWorkload)
	if found.Spec.Template.Annotations[restartedAt] == "" {
		t.Fatalf("%s is not set on the pod template", restartedAt)
	}
}

func TestAReadOnlyMCPServerRefusesAWriteAndChangesNothing(t *testing.T) {
	loaded := bundle(t)
	mcpDeployment(t, loaded, mcpWorkload)
	server := mcpServer(t, loaded, false)

	text, failed := mcpCall(t, server, "manage_workload", map[string]any{
		"resource": "deployments", "name": mcpWorkload,
		"namespace": namespace, "action": "scale", "replicas": 5,
	})
	if !failed {
		t.Fatalf("a read-only server carried out the scale: %s", text)
	}
	if !strings.Contains(text, "read-only") {
		t.Errorf("refusal = %q, want it to say the server is read-only", text)
	}

	found := mcpReadDeployment(t, loaded, mcpWorkload)
	if found.Spec.Replicas == nil || *found.Spec.Replicas != 1 {
		t.Fatalf("replicas = %v, want the untouched 1", found.Spec.Replicas)
	}
}

func TestSuspendingACronJobThroughMCPReachesTheApiserver(t *testing.T) {
	loaded := bundle(t)
	job := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: mcpCronJob, Namespace: namespace},
		Spec: batchv1.CronJobSpec{
			Schedule: "0 0 31 2 *",
			JobTemplate: batchv1.JobTemplateSpec{Spec: batchv1.JobSpec{
				Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:  "pause",
						Image: "registry.k8s.io/pause:3.10",
					}},
				}},
			}},
		},
	}
	_, err := loaded.Clientset.BatchV1().CronJobs(namespace).Create(
		context.Background(), job, metav1.CreateOptions{},
	)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create cronjob: %v", err)
	}
	t.Cleanup(func() {
		_ = loaded.Clientset.BatchV1().CronJobs(namespace).Delete(
			context.Background(), mcpCronJob, metav1.DeleteOptions{},
		)
	})
	server := mcpServer(t, loaded, true)

	text, failed := mcpCall(t, server, "manage_cronjob", map[string]any{
		"name": mcpCronJob, "namespace": namespace, "action": "suspend",
	})
	if failed {
		t.Fatalf("suspend refused: %s", text)
	}

	found, err := loaded.Clientset.BatchV1().CronJobs(namespace).Get(
		context.Background(), mcpCronJob, metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("read cronjob: %v", err)
	}
	if found.Spec.Suspend == nil || !*found.Spec.Suspend {
		t.Fatalf("suspend = %v, want true", found.Spec.Suspend)
	}
}

func TestCordoningANodeThroughMCPAndPuttingItBack(t *testing.T) {
	loaded := bundle(t)
	nodes, err := loaded.Clientset.CoreV1().Nodes().List(
		context.Background(), metav1.ListOptions{},
	)
	if err != nil || len(nodes.Items) == 0 {
		t.Fatalf("list nodes: %v", err)
	}
	name := nodes.Items[len(nodes.Items)-1].Name
	server := mcpServer(t, loaded, true)
	t.Cleanup(func() {
		_, _ = mcpCall(t, server, "manage_node", map[string]any{
			"name": name, "action": "uncordon",
		})
	})

	text, failed := mcpCall(t, server, "manage_node", map[string]any{
		"name": name, "action": "cordon",
	})
	if failed {
		t.Fatalf("cordon refused: %s", text)
	}
	found, err := loaded.Clientset.CoreV1().Nodes().Get(
		context.Background(), name, metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("read node: %v", err)
	}
	if !found.Spec.Unschedulable {
		t.Fatalf("%s is still schedulable after cordon", name)
	}

	text, failed = mcpCall(t, server, "manage_node", map[string]any{
		"name": name, "action": "uncordon",
	})
	if failed {
		t.Fatalf("uncordon refused: %s", text)
	}
	found, err = loaded.Clientset.CoreV1().Nodes().Get(
		context.Background(), name, metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("read node: %v", err)
	}
	if found.Spec.Unschedulable {
		t.Fatalf("%s is still cordoned after uncordon", name)
	}
}

func mcpStableVersion(t *testing.T, loaded *kube.Bundle, name string) *appsv1.Deployment {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	last := mcpReadDeployment(t, loaded, name)
	for time.Now().Before(deadline) {
		time.Sleep(pollInterval)
		next := mcpReadDeployment(t, loaded, name)
		if next.ResourceVersion == last.ResourceVersion {
			return next
		}
		last = next
	}
	t.Fatalf("%s never settled on a resourceVersion", name)
	return nil
}

func mcpDocument(name, version string, replicas int) string {
	return fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
  resourceVersion: "%s"
  labels:
    applied-by: mcp
spec:
  replicas: %d
  selector:
    matchLabels:
      app: %s
  template:
    metadata:
      labels:
        app: %s
    spec:
      containers:
        - name: pause
          image: registry.k8s.io/pause:3.10
`, name, namespace, version, replicas, name, name)
}

func TestApplyingADocumentThroughMCPCarriesTheResourceVersion(t *testing.T) {
	loaded := bundle(t)
	mcpDeployment(t, loaded, mcpWorkload)
	settled := mcpStableVersion(t, loaded, mcpWorkload)
	server := mcpServer(t, loaded, true)

	text, failed := mcpCall(t, server, "apply_resource", map[string]any{
		"resource": "deployments", "name": mcpWorkload,
		"namespace": namespace, "yaml": mcpDocument(mcpWorkload, settled.ResourceVersion, 2),
	})
	if failed {
		t.Fatalf("apply refused: %s", text)
	}

	found := mcpReadDeployment(t, loaded, mcpWorkload)
	if found.Labels["applied-by"] != "mcp" {
		t.Fatalf("labels = %v, want applied-by=mcp", found.Labels)
	}
	if found.Spec.Replicas == nil || *found.Spec.Replicas != 2 {
		t.Fatalf("replicas = %v, want 2", found.Spec.Replicas)
	}
}

func TestApplyingAStaleDocumentThroughMCPIsRefused(t *testing.T) {
	loaded := bundle(t)
	mcpDeployment(t, loaded, mcpWorkload)
	settled := mcpStableVersion(t, loaded, mcpWorkload)
	server := mcpServer(t, loaded, true)

	text, failed := mcpCall(t, server, "apply_resource", map[string]any{
		"resource": "deployments", "name": mcpWorkload,
		"namespace": namespace, "yaml": mcpDocument(mcpWorkload, "1", 9),
	})
	if !failed {
		t.Fatalf("a document carrying a stale resourceVersion was applied: %s", text)
	}
	if !strings.Contains(text, "has been modified") {
		t.Errorf("refusal = %q, want it to name the conflict", text)
	}

	found := mcpReadDeployment(t, loaded, mcpWorkload)
	if found.Spec.Replicas == nil || *found.Spec.Replicas != *settled.Spec.Replicas {
		t.Fatalf("replicas = %v, want the untouched %v", found.Spec.Replicas, settled.Spec.Replicas)
	}
}

func TestAGitopsCallFailsCleanlyWhenNoControllerIsInstalled(t *testing.T) {
	server := mcpServer(t, bundle(t), true)

	text, failed := mcpCall(t, server, "manage_gitops", map[string]any{
		"engine": "flux", "resource": "kustomizations", "name": "nothing",
		"namespace": namespace, "action": "reconcile",
	})
	if !failed {
		t.Fatalf("reconcile against a cluster with no Flux reported success: %s", text)
	}
	if text == "" {
		t.Fatal("the refusal carried no message")
	}
}
