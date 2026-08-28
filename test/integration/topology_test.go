//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/resources"
	"github.com/sophotechlabs/spinoza/internal/topology"
)

const foldTimeout = 3 * time.Minute

func applyService(t *testing.T, name string) {
	t.Helper()
	loaded := bundle(t)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": name},
			Ports:    []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromInt32(80)}},
		},
	}
	_, err := loaded.Clientset.CoreV1().Services(namespace).
		Create(context.Background(), service, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create service %s: %v", name, err)
	}
	t.Cleanup(func() {
		_ = loaded.Clientset.CoreV1().Services(namespace).
			Delete(context.Background(), name, metav1.DeleteOptions{})
	})
}

func here(req topology.Request) topology.Request {
	req.Namespace = namespace
	return req
}

func nodeNamed(graph api.Graph, kind, name string) (api.GraphNode, bool) {
	for _, node := range graph.Nodes {
		if node.Kind == kind && node.Name == name {
			return node, true
		}
	}
	return api.GraphNode{}, false
}

func awaitFold(t *testing.T, mgr *resources.Manager, name string, matches func(api.GraphNode) bool) api.GraphNode {
	t.Helper()
	deadline := time.Now().Add(foldTimeout)
	var last api.GraphNode
	for time.Now().Before(deadline) {
		graph := mgr.Topology(context.Background(), here(topology.Request{}))
		node, found := nodeNamed(graph, "Deployment", name)
		if found {
			last = node
			if matches(node) {
				return node
			}
		}
		time.Sleep(pollInterval)
	}
	t.Fatalf("Deployment %s never settled within %s; last read %+v", name, foldTimeout, last)
	return api.GraphNode{}
}

func TestADeploymentFoldsToOneNodeOnARealCluster(t *testing.T) {
	mgr := manager(t, bundle(t))
	applyDeployment(t, deploymentOf("smoke-fold", "registry.k8s.io/pause:3.10", nil))
	applyService(t, "smoke-fold")

	folded := awaitFold(t, mgr, "smoke-fold", func(node api.GraphNode) bool {
		return node.Contains == 3 && node.Unhealthy == 0 && node.Ready == "True"
	})

	if folded.Status != "2/2" {
		t.Fatalf("status = %q, want 2/2", folded.Status)
	}
	graph := mgr.Topology(context.Background(), here(topology.Request{}))
	service, found := nodeNamed(graph, "Service", "smoke-fold")
	if !found {
		t.Fatal("the Service is missing from the graph")
	}
	if !reaches(graph, service.ID, folded.ID, "routes") {
		t.Fatal("no routes edge from the Service to the workload its pods fold into")
	}
	for _, node := range graph.Nodes {
		if !strings.HasPrefix(node.Name, "smoke-fold") {
			continue
		}
		if node.Kind == "ReplicaSet" || node.Kind == "Pod" {
			t.Fatalf("%s %s was drawn instead of folding away", node.Kind, node.Name)
		}
	}
}

func TestOneClickOpensOneLevelOnARealCluster(t *testing.T) {
	mgr := manager(t, bundle(t))
	applyDeployment(t, deploymentOf("smoke-open", "registry.k8s.io/pause:3.10", nil))

	folded := awaitFold(t, mgr, "smoke-open", func(node api.GraphNode) bool {
		return node.Contains == 3
	})

	opened := mgr.Topology(context.Background(), here(topology.Request{Expanded: []string{folded.ID}}))
	replicas, found := replicaSetOf(opened, "smoke-open")
	if !found {
		t.Fatal("the ReplicaSet stayed folded after its Deployment was expanded")
	}
	if replicas.Contains != 2 {
		t.Fatalf("the ReplicaSet folds %d pods, want the 2 replicas", replicas.Contains)
	}
	if !reaches(opened, folded.ID, replicas.ID, "owns") {
		t.Fatal("no owns edge between the Deployment and the ReplicaSet it owns")
	}
}

func replicaSetOf(graph api.Graph, workload string) (api.GraphNode, bool) {
	for _, node := range graph.Nodes {
		if node.Kind != "ReplicaSet" {
			continue
		}
		if len(node.Name) > len(workload) && node.Name[:len(workload)] == workload {
			return node, true
		}
	}
	return api.GraphNode{}, false
}

func reaches(graph api.Graph, from, to, kind string) bool {
	for _, edge := range graph.Edges {
		if edge.From == from && edge.To == to && edge.Kind == kind {
			return true
		}
	}
	return false
}
