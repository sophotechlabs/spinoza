//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/checks"
	"github.com/sophotechlabs/spinoza/internal/kube"
	"github.com/sophotechlabs/spinoza/internal/resources"
)

const auditWorkload = "audit-target"

const (
	usageWorkload = "usage-target"
	usageCheck    = "requests-far-above-usage"
	usageRequest  = "512Mi"
	usageTimeout  = 3 * time.Minute
	usagePoll     = 5 * time.Second
)

func failingDeployment(t *testing.T, loaded *kube.Bundle) {
	t.Helper()
	yes := true
	var noGrace int64
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: auditWorkload},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": auditWorkload}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": auditWorkload}},
				Spec: corev1.PodSpec{
					HostPID:                       true,
					TerminationGracePeriodSeconds: &noGrace,
					Containers: []corev1.Container{{
						Name:    "app",
						Image:   "busybox",
						Command: []string{"sh", "-c", "sleep 3600"},
						Ports: []corev1.ContainerPort{{
							ContainerPort: 8080,
							HostPort:      8080,
						}},
						Env: []corev1.EnvVar{
							{Name: "DB_PASSWORD", Value: "hunter2"},
							{Name: "MODE", Value: "a"},
							{Name: "MODE", Value: "b"},
						},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
						},
						SecurityContext: &corev1.SecurityContext{
							Privileged:   &yes,
							Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"SYS_ADMIN"}},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "runtime-sock",
							MountPath: "/var/run/docker.sock",
						}, {
							Name:      "host-etc",
							MountPath: "/host-etc",
						}},
					}},
					Volumes: []corev1.Volume{{
						Name: "runtime-sock",
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{Path: "/var/run/docker.sock"},
						},
					}, {
						Name: "host-etc",
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{Path: "/etc"},
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

func gvrOf(object api.CheckObject) schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    object.Group,
		Version:  object.Version,
		Resource: object.Resource,
	}
}

func objectOf(report api.CheckReport, finding api.CheckFinding) api.CheckObject {
	if finding.Ref < 0 || finding.Ref >= len(report.Objects) {
		return api.CheckObject{}
	}
	return report.Objects[finding.Ref]
}

func groupOf(report api.CheckReport, id string) api.CheckGroup {
	for _, group := range report.Groups {
		if group.ID == id {
			return group
		}
	}
	return api.CheckGroup{}
}

func findingsFor(report api.CheckReport, id string) []api.CheckFinding {
	return groupOf(report, id).Findings
}

func auditedHere(report api.CheckReport, id string) bool {
	for _, finding := range findingsFor(report, id) {
		object := objectOf(report, finding)
		if object.Namespace == namespace && object.Name == auditWorkload {
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
		"seccomp-unset",
		"capabilities-not-dropped",
		"net-raw-kept",
		"sensitive-host-path",
		"writable-host-mount",
		"host-ports",
		"automount-token",
		"default-service-account",
		"grace-period-zero",
		"duplicate-env-keys",
		"secret-in-env-literal",
		"cpu-limit-set",
		"image-not-digest-pinned",
		"image-from-docker-hub",
		"no-prestop-hook",
		"no-progress-deadline",
		"unbounded-revision-history",
		"missing-recommended-labels",
		"ephemeral-storage-unset",
	} {
		if !auditedHere(report, id) {
			t.Errorf("%s did not fire on a workload built to trip it", id)
		}
	}
}

const packagedWorkload = "aaa-packaged-target"

func packagedDeployment(t *testing.T, loaded *kube.Bundle) {
	t.Helper()
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        packagedWorkload,
			Annotations: map[string]string{"meta.helm.sh/release-name": "audit-chart"},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": packagedWorkload}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": packagedWorkload}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:    "app",
						Image:   "busybox",
						Command: []string{"sh", "-c", "sleep 3600"},
					}},
				},
			},
		},
	}
	_, err := loaded.Clientset.AppsV1().Deployments(namespace).Create(
		context.Background(), deployment, metav1.CreateOptions{},
	)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create packaged deployment: %v", err)
	}
	t.Cleanup(func() {
		_ = loaded.Clientset.AppsV1().Deployments(namespace).Delete(
			context.Background(), packagedWorkload, metav1.DeleteOptions{},
		)
	})
}

func positionOf(report api.CheckReport, id, name string) int {
	for at, finding := range findingsFor(report, id) {
		if objectOf(report, finding).Name == name {
			return at
		}
	}
	return -1
}

func TestYourOwnWorkloadIsListedBeforeOneAPackageInstalled(t *testing.T) {
	loaded := bundle(t)
	mgr := manager(t, loaded)
	failingDeployment(t, loaded)
	packagedDeployment(t, loaded)

	report := mgr.Checks(context.Background())

	mine := positionOf(report, "requests-missing", auditWorkload)
	packaged := positionOf(report, "requests-missing", packagedWorkload)
	if mine < 0 || packaged < 0 {
		t.Fatalf("requests-missing found %s at %d and %s at %d (%s)",
			auditWorkload, mine, packagedWorkload, packaged, report.Error)
	}
	if mine > packaged {
		t.Fatalf("%s listed at %d, after the Helm-installed %s at %d, "+
			"even though it sorts later alphabetically",
			auditWorkload, mine, packagedWorkload, packaged)
	}
	if objectOf(report, findingsFor(report, "requests-missing")[packaged]).ManagedBy != "Helm: audit-chart" {
		t.Fatal("the packaged workload did not carry the release that installed it")
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

func TestPagingAChecksFindingsAgainstARealCluster(t *testing.T) {
	loaded := bundle(t)
	mgr := manager(t, loaded)
	failingDeployment(t, loaded)

	report := mgr.Checks(context.Background())
	group := api.CheckGroup{}
	for _, one := range report.Groups {
		if one.ID == "requests-missing" {
			group = one
		}
	}
	if group.Total == 0 {
		t.Fatalf("nothing on this cluster is missing requests (%s)", report.Error)
	}

	page, err := mgr.CheckPage(context.Background(), "requests-missing", "")
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(page.Findings) != len(group.Findings) {
		t.Fatalf("the endpoint sent %d findings, the report sent %d",
			len(page.Findings), len(group.Findings))
	}
	if page.Next != group.Next {
		t.Fatalf("endpoint cursor %q, report cursor %q", page.Next, group.Next)
	}

	seen := len(page.Findings)
	for page.Next != "" {
		page, err = mgr.CheckPage(context.Background(), "requests-missing", page.Next)
		if err != nil {
			t.Fatalf("next page: %v", err)
		}
		if len(page.Findings) == 0 {
			t.Fatal("a cursor pointed at a page with nothing on it")
		}
		seen += len(page.Findings)
	}
	if seen != group.Total {
		t.Fatalf("paging surfaced %d findings, the report counted %d", seen, group.Total)
	}
}

func TestPagingRefusesACheckThatDoesNotExist(t *testing.T) {
	_, err := manager(t, bundle(t)).CheckPage(context.Background(), "not-a-real-check", "")

	if !errors.Is(err, checks.ErrNoSuchCheck) {
		t.Fatalf("error = %v, want ErrNoSuchCheck", err)
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
			object := objectOf(report, finding)
			if finding.Patch == "" || object.Namespace != namespace {
				continue
			}
			tried++
			assertPatchApplies(t, loaded, group.ID, object, finding.Patch)
		}
	}
	if tried == 0 {
		t.Fatal("no finding offered a patch, so nothing was checked against the api server")
	}
	t.Logf("%d patches accepted by the api server", tried)
}

func assertPatchApplies(
	t *testing.T,
	loaded *kube.Bundle,
	id string,
	object api.CheckObject,
	patch string,
) {
	t.Helper()
	merged, err := yaml.YAMLToJSON([]byte(patch))
	if err != nil {
		t.Errorf("%s on %s: the patch is not valid yaml: %v", id, object.Name, err)
		return
	}
	_, err = loaded.Dynamic.
		Resource(gvrOf(object)).
		Namespace(object.Namespace).
		Patch(
			context.Background(),
			object.Name,
			types.StrategicMergePatchType,
			merged,
			metav1.PatchOptions{DryRun: []string{metav1.DryRunAll}},
		)
	if err != nil {
		t.Errorf("%s on %s/%s: the api server refused the patch: %v\n%s",
			id, object.Namespace, object.Name, err, patch)
	}
}

func overprovisionedDeployment(t *testing.T, loaded *kube.Bundle) {
	t.Helper()
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: usageWorkload},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": usageWorkload}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": usageWorkload}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:    "app",
						Image:   "busybox:1.36",
						Command: []string{"sh", "-c", "dd if=/dev/zero of=/scratch/blob bs=1M count=8 && sleep 3600"},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse(usageRequest)},
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "scratch", MountPath: "/scratch"}},
					}},
					Volumes: []corev1.Volume{{
						Name: "scratch",
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory},
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
			context.Background(), usageWorkload, metav1.DeleteOptions{},
		)
	})
}

func awaitMetrics(t *testing.T, mgr *resources.Manager) {
	t.Helper()
	deadline := time.Now().Add(usageTimeout)
	var last api.Metrics
	for time.Now().Before(deadline) {
		last = mgr.Metrics(context.Background())
		if len(last.Pods) > 0 {
			return
		}
		time.Sleep(usagePoll)
	}
	t.Fatalf("metrics-server reported usage for no pod within %s (%s)", usageTimeout, last.Error)
}

func awaitUsageFinding(t *testing.T, mgr *resources.Manager) api.CheckFinding {
	t.Helper()
	deadline := time.Now().Add(usageTimeout)
	var last api.CheckGroup
	for time.Now().Before(deadline) {
		report := mgr.Checks(context.Background())
		last = groupOf(report, usageCheck)
		for _, finding := range last.Findings {
			object := objectOf(report, finding)
			if object.Namespace == namespace && object.Name == usageWorkload {
				return finding
			}
		}
		time.Sleep(usagePoll)
	}
	t.Fatalf("%s never fired on %s within %s; it was skipped with %q and found %d elsewhere",
		usageCheck, usageWorkload, usageTimeout, last.Skipped, last.Total)
	return api.CheckFinding{}
}

func TestTheUsageCheckRunsOnceMetricsServerAnswers(t *testing.T) {
	mgr := manager(t, bundle(t))
	awaitMetrics(t, mgr)

	group := groupOf(mgr.Checks(context.Background()), usageCheck)

	if group.ID != usageCheck {
		t.Fatalf("the report carries no %s group at all", usageCheck)
	}
	if group.Skipped != "" {
		t.Fatalf("skipped = %q, want the check to have run against a real metrics-server", group.Skipped)
	}
}

func TestRequestsFarAboveUsageFiresOnAWorkloadBuiltToTripIt(t *testing.T) {
	loaded := bundle(t)
	mgr := manager(t, loaded)
	overprovisionedDeployment(t, loaded)

	finding := awaitUsageFinding(t, mgr)

	want := "pods request " + usageRequest + " memory and use "
	if !strings.HasPrefix(finding.Detail, want) {
		t.Fatalf("detail = %q, want it to open with %q", finding.Detail, want)
	}
	if finding.Container != "" {
		t.Fatalf("container = %q, want the finding on the workload rather than one container", finding.Container)
	}
}
