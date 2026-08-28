//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

const issueTimeout = 3 * time.Minute

func deploymentOf(name, image string, command []string) *appsv1.Deployment {
	replicas := int32(2)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:    "app",
						Image:   image,
						Command: command,
					}},
				},
			},
		},
	}
}

func applyDeployment(t *testing.T, deployment *appsv1.Deployment) {
	t.Helper()
	loaded := bundle(t)
	_, err := loaded.Clientset.AppsV1().Deployments(namespace).
		Create(context.Background(), deployment, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create deployment %s: %v", deployment.Name, err)
	}
	t.Cleanup(func() {
		_ = loaded.Clientset.AppsV1().Deployments(namespace).
			Delete(context.Background(), deployment.Name, metav1.DeleteOptions{})
	})
}

func awaitRow(t *testing.T, mgr *resources.Manager, name string, matches func(api.Issue) bool) api.Issue {
	t.Helper()
	deadline := time.Now().Add(issueTimeout)
	var last api.IssueQueue
	for time.Now().Before(deadline) {
		last = mgr.Issues(context.Background())
		for _, row := range last.Rows {
			if row.Object.Name != name {
				continue
			}
			if matches(row) {
				return row
			}
		}
		time.Sleep(pollInterval)
	}
	t.Fatalf("no row for %s within %s; the queue held %+v", name, issueTimeout, last.Rows)
	return api.Issue{}
}

func TestACrashLoopFoldsUnderItsDeployment(t *testing.T) {
	mgr := manager(t, bundle(t))
	applyDeployment(t, deploymentOf("smoke-crash", "busybox:1.36", []string{"sh", "-c", "exit 1"}))

	row := awaitRow(t, mgr, "smoke-crash", func(row api.Issue) bool {
		return row.Title == "CrashLoopBackOff"
	})

	if row.Kind != "Deployment" {
		t.Fatalf("kind = %q, want the row to name the deployment, not a pod", row.Kind)
	}
	if row.Severity != api.SeverityFatal {
		t.Fatalf("severity = %q, want fatal", row.Severity)
	}
	if row.Folded == 0 || len(row.Children) == 0 {
		t.Fatalf("row = %+v, want the pods folded underneath", row)
	}
	if row.Children[0].Object.Resource != "pods" {
		t.Fatalf("child = %+v, want a pod", row.Children[0])
	}
	if row.Action == "" {
		t.Fatalf("row = %+v, want it to state the next action", row)
	}
}

func TestAnImageThatCannotBePulledIsReported(t *testing.T) {
	mgr := manager(t, bundle(t))
	applyDeployment(t, deploymentOf("smoke-image", "example.invalid/nothing:v0", nil))

	row := awaitRow(t, mgr, "smoke-image", func(row api.Issue) bool {
		return row.Title == "ImagePullBackOff" || row.Title == "ErrImagePull"
	})

	if row.Severity != api.SeverityFatal {
		t.Fatalf("severity = %q, want fatal", row.Severity)
	}
}

func TestAPodNoNodeCanTakeIsReported(t *testing.T) {
	loaded := bundle(t)
	mgr := manager(t, loaded)
	huge := resource.MustParse("1000")
	_, err := loaded.Clientset.CoreV1().Pods(namespace).Create(context.Background(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "smoke-unschedulable"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "app",
				Image: "busybox:1.36",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: huge},
				},
			}},
		},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create pod: %v", err)
	}
	t.Cleanup(func() {
		_ = loaded.Clientset.CoreV1().Pods(namespace).
			Delete(context.Background(), "smoke-unschedulable", metav1.DeleteOptions{})
	})

	row := awaitRow(t, mgr, "smoke-unschedulable", func(row api.Issue) bool {
		return row.Title == "Unschedulable"
	})

	if row.Detail == "" {
		t.Fatalf("row = %+v, want the scheduler's own message", row)
	}
}

func TestAJobThatGivesUpIsReported(t *testing.T) {
	loaded := bundle(t)
	mgr := manager(t, loaded)
	limit := int32(0)
	_, err := loaded.Clientset.BatchV1().Jobs(namespace).Create(context.Background(), &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "smoke-job"},
		Spec: batchv1.JobSpec{
			BackoffLimit: &limit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:    "app",
						Image:   "busybox:1.36",
						Command: []string{"sh", "-c", "exit 2"},
					}},
				},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create job: %v", err)
	}
	t.Cleanup(func() {
		policy := metav1.DeletePropagationBackground
		_ = loaded.Clientset.BatchV1().Jobs(namespace).Delete(
			context.Background(), "smoke-job", metav1.DeleteOptions{PropagationPolicy: &policy},
		)
	})

	row := awaitRow(t, mgr, "smoke-job", func(row api.Issue) bool {
		return row.Title == "JobFailed"
	})

	if row.Severity != api.SeverityFatal {
		t.Fatalf("severity = %q, want fatal", row.Severity)
	}
}

func TestAHealthyClusterHasAnEmptyQueue(t *testing.T) {
	mgr := manager(t, bundle(t))
	runningPod(t, bundle(t), "smoke-healthy")

	queue := mgr.Issues(context.Background())

	for _, row := range queue.Rows {
		if row.Object.Namespace == namespace && row.Object.Name == "smoke-healthy" {
			t.Fatalf("row = %+v, want a healthy pod left out of the queue", row)
		}
	}
}
