package checks

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	originPackaged = "packaged"
	originSystem   = "system"
)

const (
	helmReleaseAnnotation = "meta.helm.sh/release-name"
	helmManagedLabel      = "app.kubernetes.io/managed-by"
	helmManagerName       = "Helm"
	fluxKustomizeLabel    = "kustomize.toolkit.fluxcd.io/name"
	fluxHelmLabel         = "helm.toolkit.fluxcd.io/name"
	argoInstanceLabel     = "argocd.argoproj.io/instance"
)

var systemNamespaces = map[string]bool{
	"kube-system":     true,
	"kube-public":     true,
	"kube-node-lease": true,
}

func originOf(obj *unstructured.Unstructured) (origin, managedBy string) {
	if named := managerOf(obj); named != "" {
		return originPackaged, named
	}
	if systemNamespaces[obj.GetNamespace()] {
		return originSystem, ""
	}
	return "", ""
}

func managerOf(obj *unstructured.Unstructured) string {
	labels := obj.GetLabels()
	if release := labels[fluxHelmLabel]; release != "" {
		return "Flux: " + release
	}
	if release := labels[fluxKustomizeLabel]; release != "" {
		return "Flux: " + release
	}
	if release := obj.GetAnnotations()[helmReleaseAnnotation]; release != "" {
		return "Helm: " + release
	}
	if release := labels[argoInstanceLabel]; release != "" {
		return "Argo: " + release
	}
	if labels[helmManagedLabel] == helmManagerName {
		return helmManagerName
	}
	return ""
}

func originRank(origin string) string {
	switch origin {
	case originPackaged:
		return "1"
	case originSystem:
		return "2"
	default:
		return "0"
	}
}
