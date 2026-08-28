//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/kube"
)

const auditWorkload = "audit-target"

func failingDeployment(t *testing.T, loaded *kube.Bundle) {
	t.Helper()
	yes := true
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: auditWorkload},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": auditWorkload}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": auditWorkload}},
				Spec: corev1.PodSpec{
					HostPID: true,
					Containers: []corev1.Container{{
						Name:    "app",
						Image:   "busybox",
						Command: []string{"sh", "-c", "sleep 3600"},
						SecurityContext: &corev1.SecurityContext{
							Privileged:   &yes,
							Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"SYS_ADMIN"}},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "runtime-sock",
							MountPath: "/var/run/docker.sock",
						}},
					}},
					Volumes: []corev1.Volume{{
						Name: "runtime-sock",
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{Path: "/var/run/docker.sock"},
						},
					}},
				},
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
			context.Background(), auditWorkload, metav1.DeleteOptions{},
		)
	})
}

func gvrOf(ref api.ObjectRef) schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: ref.Group, Version: ref.Version, Resource: ref.Resource}
}

func findingsFor(report api.CheckReport, id string) []api.CheckFinding {
	for _, group := range report.Groups {
		if group.ID == id {
			return group.Findings
		}
	}
	return nil
}

func auditedHere(findings []api.CheckFinding) bool {
	for _, finding := range findings {
		if finding.Object.Namespace == namespace && finding.Object.Name == auditWorkload {
			return true
		}
	}
	return false
}

func TestChecksAuditARealCluster(t *testing.T) {
	loaded := bundle(t)
	mgr := manager(t, loaded)
	failingDeployment(t, loaded)

	report := mgr.Checks(context.Background())

	if report.Scanned == 0 {
		t.Fatalf("the audit read nothing (%s)", report.Error)
	}
	if strings.Contains(report.Error, "not discovered yet") {
		t.Fatalf("a real cluster left a type undiscovered: %s", report.Error)
	}
	for _, id := range []string{
		"privileged-containers",
		"host-namespaces",
		"dangerous-capabilities",
		"runtime-socket-mounted",
		"requests-missing",
		"image-latest",
	} {
		if !auditedHere(findingsFor(report, id)) {
			t.Errorf("%s did not fire on a workload built to trip it", id)
		}
	}
}

func TestEveryCheckIsRegisteredOnce(t *testing.T) {
	report := manager(t, bundle(t)).Checks(context.Background())

	seen := map[string]bool{}
	for _, group := range report.Groups {
		if seen[group.ID] {
			t.Fatalf("%s appears twice in one report", group.ID)
		}
		seen[group.ID] = true
	}
	if len(seen) == 0 {
		t.Fatal("the report carried no checks")
	}
}

func TestEveryPatchTheAuditOffersIsAcceptedByTheApiServer(t *testing.T) {
	loaded := bundle(t)
	mgr := manager(t, loaded)
	failingDeployment(t, loaded)

	report := mgr.Checks(context.Background())

	tried := 0
	for _, group := range report.Groups {
		for _, finding := range group.Findings {
			if finding.Patch == "" || finding.Object.Namespace != namespace {
				continue
			}
			tried++
			assertPatchApplies(t, loaded, group.ID, finding)
		}
	}
	if tried == 0 {
		t.Fatal("no finding offered a patch, so nothing was checked against the api server")
	}
	t.Logf("%d patches accepted by the api server", tried)
}

func assertPatchApplies(t *testing.T, loaded *kube.Bundle, id string, finding api.CheckFinding) {
	t.Helper()
	merged, err := yaml.YAMLToJSON([]byte(finding.Patch))
	if err != nil {
		t.Errorf("%s on %s: the patch is not valid yaml: %v", id, finding.Object.Name, err)
		return
	}
	_, err = loaded.Dynamic.
		Resource(gvrOf(finding.Object)).
		Namespace(finding.Object.Namespace).
		Patch(
			context.Background(),
			finding.Object.Name,
			types.StrategicMergePatchType,
			merged,
			metav1.PatchOptions{DryRun: []string{metav1.DryRunAll}},
		)
	if err != nil {
		t.Errorf("%s on %s/%s: the api server refused the patch: %v\n%s",
			id, finding.Object.Namespace, finding.Object.Name, err, finding.Patch)
	}
}
