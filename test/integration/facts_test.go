//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/checks"
	"github.com/sophotechlabs/spinoza/internal/kube"
)

const factsNamespace = "spinoza-facts"

const (
	spentQuota   = "spent"
	quotaCounted = "count/configmaps"
	quotaTimeout = 2 * time.Minute
)

func factsNamespaceExists(t *testing.T, loaded *kube.Bundle) {
	t.Helper()
	space := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   factsNamespace,
			Labels: map[string]string{"pod-security.kubernetes.io/enforce": "baseline"},
		},
	}
	_, err := loaded.Clientset.CoreV1().Namespaces().Create(context.Background(), space, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create namespace: %v", err)
	}
	t.Cleanup(func() {
		_ = loaded.Clientset.CoreV1().Namespaces().Delete(
			context.Background(), factsNamespace, metav1.DeleteOptions{},
		)
	})
}

func factsWorkload(t *testing.T, loaded *kube.Bundle, name string, count int32, shape func(*corev1.PodSpec)) {
	t.Helper()
	pod := corev1.PodSpec{
		Containers: []corev1.Container{{
			Name:    "app",
			Image:   "busybox:1.36",
			Command: []string{"sh", "-c", "sleep 3600"},
		}},
	}
	shape(&pod)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: factsNamespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: &count,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
				Spec:       pod,
			},
		},
	}
	_, err := loaded.Clientset.AppsV1().Deployments(factsNamespace).Create(
		context.Background(), deployment, metav1.CreateOptions{},
	)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create %s: %v", name, err)
	}
	t.Cleanup(func() {
		_ = loaded.Clientset.AppsV1().Deployments(factsNamespace).Delete(
			context.Background(), name, metav1.DeleteOptions{},
		)
	})
}

func taintOneNode(t *testing.T, loaded *kube.Bundle) string {
	t.Helper()
	nodes, err := loaded.Clientset.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil || len(nodes.Items) == 0 {
		t.Fatalf("list nodes: %v", err)
	}
	name := nodes.Items[0].Name
	patch := []byte(`{"spec":{"taints":[{"key":"spinoza-audit","value":"yes","effect":"NoSchedule"}]}}`)
	_, err = loaded.Clientset.CoreV1().Nodes().Patch(
		context.Background(), name, types.StrategicMergePatchType, patch, metav1.PatchOptions{},
	)
	if err != nil {
		t.Fatalf("taint %s: %v", name, err)
	}
	t.Cleanup(func() {
		clear := []byte(`{"spec":{"taints":null}}`)
		_, _ = loaded.Clientset.CoreV1().Nodes().Patch(
			context.Background(), name, types.StrategicMergePatchType, clear, metav1.PatchOptions{},
		)
	})
	return name
}

func limitRangeExists(t *testing.T, loaded *kube.Bundle) {
	t.Helper()
	limits := &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{Name: "floor", Namespace: factsNamespace},
		Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{
			Type: corev1.LimitTypeContainer,
			Min:  corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("128Mi")},
		}}},
	}
	_, err := loaded.Clientset.CoreV1().LimitRanges(factsNamespace).Create(
		context.Background(), limits, metav1.CreateOptions{},
	)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create limit range: %v", err)
	}
}

func spentQuotaExists(t *testing.T, loaded *kube.Bundle) {
	t.Helper()
	quota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: spentQuota, Namespace: factsNamespace},
		Spec: corev1.ResourceQuotaSpec{Hard: corev1.ResourceList{
			quotaCounted: resource.MustParse("1"),
		}},
	}
	_, err := loaded.Clientset.CoreV1().ResourceQuotas(factsNamespace).Create(
		context.Background(), quota, metav1.CreateOptions{},
	)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create quota: %v", err)
	}
	awaitQuotaSpent(t, loaded)
}

func awaitQuotaSpent(t *testing.T, loaded *kube.Bundle) {
	t.Helper()
	deadline := time.Now().Add(quotaTimeout)
	for {
		found, err := loaded.Clientset.CoreV1().ResourceQuotas(factsNamespace).Get(
			context.Background(), spentQuota, metav1.GetOptions{},
		)
		if err != nil {
			t.Fatalf("read quota: %v", err)
		}
		hard, okHard := found.Status.Hard[quotaCounted]
		used, okUsed := found.Status.Used[quotaCounted]
		if okHard && okUsed && used.Cmp(hard) >= 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf(
				"the quota never reached its ceiling within %s: hard %v, used %v",
				quotaTimeout, found.Status.Hard, found.Status.Used,
			)
		}
		time.Sleep(pollInterval)
	}
}

func factsFiredOn(report api.CheckReport, id, name string) bool {
	for _, finding := range findingsFor(report, id) {
		object := objectOf(report, finding)
		if object.Namespace == factsNamespace && object.Name == name {
			return true
		}
	}
	return false
}

func TestTheClusterFactChecksFireOnARealCluster(t *testing.T) {
	loaded := bundle(t)
	mgr := manager(t, loaded)
	factsNamespaceExists(t, loaded)
	limitRangeExists(t, loaded)
	spentQuotaExists(t, loaded)
	tainted := taintOneNode(t, loaded)

	factsWorkload(t, loaded, "nowhere-to-land", 1, func(pod *corev1.PodSpec) {
		pod.NodeSelector = map[string]string{"spinoza.tech/disk": "nvme"}
	})
	factsWorkload(t, loaded, "repelled", 1, func(pod *corev1.PodSpec) {
		pod.NodeSelector = map[string]string{"kubernetes.io/hostname": tainted}
	})
	factsWorkload(t, loaded, "too-big", 1, func(pod *corev1.PodSpec) {
		pod.Containers[0].Resources.Requests = corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("500"),
		}
	})
	factsWorkload(t, loaded, "over-spread", 6, func(pod *corev1.PodSpec) {
		pod.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{{
			MaxSkew:           1,
			TopologyKey:       "kubernetes.io/os",
			WhenUnsatisfiable: corev1.DoNotSchedule,
		}}
	})
	factsWorkload(t, loaded, "one-per-node", 9, func(pod *corev1.PodSpec) {
		pod.Affinity = &corev1.Affinity{PodAntiAffinity: &corev1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
				TopologyKey: "kubernetes.io/hostname",
			}},
		}}
	})
	factsWorkload(t, loaded, "below-the-floor", 1, func(pod *corev1.PodSpec) {
		pod.Containers[0].Resources.Requests = corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("16Mi"),
		}
	})
	factsWorkload(t, loaded, "shares-the-host", 1, func(pod *corev1.PodSpec) {
		pod.HostPID = true
	})

	report := mgr.Checks(context.Background(), checks.Filter{WholeCluster: true})
	if report.Scanned == 0 {
		t.Fatalf("the audit read nothing (%s)", report.Error)
	}

	cases := []struct{ id, workload string }{
		{"node-selector-matches-nothing", "nowhere-to-land"},
		{"tolerations-miss-the-taints", "repelled"},
		{"request-exceeds-largest-node", "too-big"},
		{"spread-needs-more-domains", "over-spread"},
		{"anti-affinity-exceeds-nodes", "one-per-node"},
		{"outside-limit-range", "below-the-floor"},
		{"pod-security-would-reject", "shares-the-host"},
	}
	for _, tc := range cases {
		if !factsFiredOn(report, tc.id, tc.workload) {
			t.Errorf("%s did not fire on %s, which was built to trip it", tc.id, tc.workload)
		}
	}

	if group := groupOf(report, "quota-nearly-exhausted"); group.Total == 0 {
		t.Errorf("quota-nearly-exhausted found nothing in a namespace whose quota is spent (%s)", report.Error)
	}
}

func TestACheckSaysWhyItWasSkippedWhenTheClusterHidesWhatItNeeds(t *testing.T) {
	report := manager(t, bundle(t)).Checks(context.Background(), checks.Filter{WholeCluster: true})

	reasons := []string{"metrics-server", "did not report", "every kind has been read"}
	for _, group := range report.Groups {
		if group.Skipped == "" {
			continue
		}
		if !containsAny(group.Skipped, reasons) {
			t.Fatalf("%s was skipped with an unhelpful reason: %q", group.ID, group.Skipped)
		}
	}
}

func TestAnAuditOfTheWholeClusterRunsTheOrphanChecks(t *testing.T) {
	held := manager(t, bundle(t))

	narrow := held.Checks(context.Background(), checks.Filter{})
	if groupOf(narrow, "orphaned-config-map").Skipped == "" {
		t.Fatal("orphaned-config-map ran from an audit that only read the workloads")
	}

	whole := groupOf(held.Checks(context.Background(), checks.Filter{WholeCluster: true}), "orphaned-config-map")
	if whole.ID == "" {
		t.Fatal("orphaned-config-map is not in the report at all")
	}
	if whole.Skipped != "" {
		t.Fatalf("orphaned-config-map was skipped on an audit of the whole cluster: %q", whole.Skipped)
	}
}

func TestTheCustomResourcesAreReadStraightFromTheApiserver(t *testing.T) {
	held := manager(t, bundle(t))

	report := held.Checks(context.Background(), checks.Filter{WholeCluster: true})

	for _, object := range report.Objects {
		if object.Group == "apiextensions.k8s.io" {
			t.Fatalf("a custom resource definition was kept in the report: %s", object.Name)
		}
	}
	if groupOf(report, "orphaned-secret").Skipped != "" {
		t.Fatalf("the orphan check stood down on a cluster it can read: %q",
			groupOf(report, "orphaned-secret").Skipped)
	}
}

func containsAny(text string, wanted []string) bool {
	for _, one := range wanted {
		if strings.Contains(text, one) {
			return true
		}
	}
	return false
}
