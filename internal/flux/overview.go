package flux

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/discovery"
	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const (
	partOfFlux      = "app.kubernetes.io/part-of=flux"
	operatorName    = "app.kubernetes.io/name=flux-operator"
	componentLabel  = "app.kubernetes.io/component"
	versionLabel    = "app.kubernetes.io/version"
	defaultSyncName = "flux-system"
	instanceGroup   = "fluxcd.controlplane.io"
	instanceKind    = "fluxinstances"
)

type Cluster struct {
	Kubernetes string
	Nodes      int
	Usage      map[string]api.ResourceUsage
}

func Overview(
	ctx context.Context,
	cs kubernetes.Interface,
	lister Lister,
	descs map[string]api.ResourceDescriptor,
	cluster Cluster,
) api.FluxOverview {
	out := api.FluxOverview{
		Kubernetes:  cluster.Kubernetes,
		Nodes:       cluster.Nodes,
		Controllers: []api.FluxController{},
	}
	deployments, err := controllers(ctx, cs)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	if len(deployments) == 0 {
		return out
	}
	out.Namespace = deployments[0].Namespace
	out.Controllers = controllersOf(deployments)
	out.Distribution = sharedVersion(out.Controllers)
	out.Operator = operatorVersion(ctx, cs, out.Namespace)
	instance := fluxInstance(ctx, lister, descs)
	if instance != nil {
		out.Distribution = orKeep(unstr.String(instance, "spec", "distribution", "version"), out.Distribution)
	}
	out.Sync = syncOf(ctx, lister, descs, out.Namespace, syncName(instance))
	out.Usage = usageOf(ctx, cs, out.Namespace, cluster.Usage)
	out.Ready, out.Summary = verdict(out)
	return out
}

func orKeep(found, fallback string) string {
	if found == "" {
		return fallback
	}
	return found
}

func controllers(ctx context.Context, cs kubernetes.Interface) ([]appsDeployment, error) {
	listed, err := cs.AppsV1().Deployments(metav1.NamespaceAll).
		List(ctx, metav1.ListOptions{LabelSelector: partOfFlux})
	if err != nil {
		return nil, err
	}
	out := make([]appsDeployment, 0, len(listed.Items))
	for i := range listed.Items {
		item := &listed.Items[i]
		out = append(out, appsDeployment{
			Namespace: item.Namespace,
			Name:      item.Name,
			Component: item.Labels[componentLabel],
			Version:   item.Labels[versionLabel],
			Ready:     item.Status.ReadyReplicas > 0,
			Wanted:    wantedReplicas(item.Spec.Replicas),
			Available: item.Status.ReadyReplicas,
		})
	}
	return out, nil
}

func wantedReplicas(replicas *int32) int32 {
	if replicas == nil {
		return 1
	}
	return *replicas
}

type appsDeployment struct {
	Namespace string
	Name      string
	Component string
	Version   string
	Ready     bool
	Wanted    int32
	Available int32
}

func controllersOf(deployments []appsDeployment) []api.FluxController {
	out := make([]api.FluxController, 0, len(deployments))
	for _, item := range deployments {
		out = append(out, api.FluxController{
			Name:      orKeep(item.Component, item.Name),
			Version:   item.Version,
			Ready:     item.Ready,
			Replicas:  int(item.Available),
			Wanted:    int(item.Wanted),
			Namespace: item.Namespace,
		})
	}
	return out
}

func sharedVersion(controllers []api.FluxController) string {
	version := ""
	for _, one := range controllers {
		if one.Version == "" {
			continue
		}
		if version == "" {
			version = one.Version
			continue
		}
		if version != one.Version {
			return ""
		}
	}
	return version
}

func operatorVersion(ctx context.Context, cs kubernetes.Interface, namespace string) string {
	listed, err := cs.AppsV1().Deployments(namespace).
		List(ctx, metav1.ListOptions{LabelSelector: operatorName})
	if err != nil || len(listed.Items) == 0 {
		return ""
	}
	return listed.Items[0].Labels[versionLabel]
}

func fluxInstance(
	ctx context.Context,
	lister Lister,
	descs map[string]api.ResourceDescriptor,
) *unstructured.Unstructured {
	for _, desc := range descs {
		if desc.Group != instanceGroup || desc.Resource != instanceKind {
			continue
		}
		found, err := lister.List(ctx, desc)
		if err != nil || len(found) == 0 {
			return nil
		}
		return found[0]
	}
	return nil
}

func syncName(instance *unstructured.Unstructured) string {
	if instance == nil {
		return defaultSyncName
	}
	return orKeep(unstr.String(instance, "spec", "sync", "name"), defaultSyncName)
}

func syncOf(
	ctx context.Context,
	lister Lister,
	descs map[string]api.ResourceDescriptor,
	namespace, name string,
) api.FluxSync {
	out := api.FluxSync{Namespace: namespace, Name: name}
	kustomization, ok := descs[discovery.Key("kustomize.toolkit.fluxcd.io", "v1", "kustomizations")]
	if !ok {
		return out
	}
	found, err := lister.List(ctx, kustomization)
	if err != nil {
		return out
	}
	entry := pick(found, namespace, name)
	if entry == nil {
		return out
	}
	out.Kind = "Kustomization"
	out.Path = unstr.String(entry, "spec", "path")
	out.Revision = unstr.String(entry, "status", "lastAppliedRevision")
	out.Ready = readyCondition(entry)
	out.Source, out.URL, out.Ref = syncSource(ctx, lister, descs, entry, namespace)
	return out
}

func pick(items []*unstructured.Unstructured, namespace, name string) *unstructured.Unstructured {
	for _, item := range items {
		if item.GetNamespace() == namespace && item.GetName() == name {
			return item
		}
	}
	return nil
}

func syncSource(
	ctx context.Context,
	lister Lister,
	descs map[string]api.ResourceDescriptor,
	entry *unstructured.Unstructured,
	namespace string,
) (kind, url, ref string) {
	kind = unstr.String(entry, "spec", "sourceRef", "kind")
	name := unstr.String(entry, "spec", "sourceRef", "name")
	if kind == "" || name == "" {
		return "", "", ""
	}
	desc, ok := descs[discovery.Key("source.toolkit.fluxcd.io", "v1", sourceResourceOf(kind))]
	if !ok {
		return kind, "", ""
	}
	found, err := lister.List(ctx, desc)
	if err != nil {
		return kind, "", ""
	}
	source := pick(found, namespace, name)
	if source == nil {
		return kind, "", ""
	}
	return kind, unstr.String(source, "spec", "url"), refOf(source)
}

var sourceKinds = map[string]string{
	"GitRepository":  "gitrepositories",
	"OCIRepository":  "ocirepositories",
	"HelmRepository": "helmrepositories",
	"HelmChart":      "helmcharts",
	"Bucket":         "buckets",
}

func sourceResourceOf(kind string) string {
	return sourceKinds[kind]
}

func refOf(source *unstructured.Unstructured) string {
	for _, field := range []string{"branch", "tag", "semver", "name", "commit"} {
		found := unstr.String(source, "spec", "ref", field)
		if found != "" {
			return found
		}
	}
	return ""
}

func readyCondition(item *unstructured.Unstructured) bool {
	for _, raw := range unstr.Slice(item, "status", "conditions") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if entry["type"] != "Ready" {
			continue
		}
		return entry["status"] == "True"
	}
	return false
}

func usageOf(
	ctx context.Context,
	cs kubernetes.Interface,
	namespace string,
	usage map[string]api.ResourceUsage,
) api.FluxUsage {
	out := api.FluxUsage{}
	if len(usage) == 0 {
		return out
	}
	listed, err := cs.CoreV1().Pods(namespace).
		List(ctx, metav1.ListOptions{LabelSelector: partOfFlux})
	if err != nil || len(listed.Items) == 0 {
		return out
	}
	for i := range listed.Items {
		pod := &listed.Items[i]
		used, known := usage[pod.Namespace+"/"+pod.Name]
		if known {
			out.CPUMilli += used.CPUMilli
			out.MemoryMi += used.MemoryMi
			out.Known = true
		}
		addRequests(&out, pod)
	}
	return out
}

func addRequests(out *api.FluxUsage, pod *corev1.Pod) {
	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		out.CPURequestMilli += container.Resources.Requests.Cpu().MilliValue()
		out.MemRequestMi += container.Resources.Requests.Memory().Value() / (1024 * 1024)
		out.CPULimitMilli += container.Resources.Limits.Cpu().MilliValue()
		out.MemLimitMi += container.Resources.Limits.Memory().Value() / (1024 * 1024)
	}
}

func verdict(out api.FluxOverview) (bool, string) {
	short := []string{}
	for _, one := range out.Controllers {
		if !one.Ready {
			short = append(short, one.Name)
		}
	}
	if len(short) > 0 {
		return false, strings.Join(short, ", ") + " is not ready"
	}
	if out.Sync.Kind != "" && !out.Sync.Ready {
		return false, "the cluster sync is not ready"
	}
	if out.Sync.Kind == "" {
		return true, "the controllers are ready; no cluster sync was found"
	}
	return true, "the cluster is in sync with what the repository asks for"
}
