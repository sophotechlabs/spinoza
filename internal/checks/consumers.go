package checks

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type Named struct {
	Ref     api.ObjectRef
	Owned   bool
	Manager string
}

var fluxLabels = []string{"helm.toolkit.fluxcd.io/name", "kustomize.toolkit.fluxcd.io/name"}

func ManagerOf(labels map[string]string) string {
	for _, key := range fluxLabels {
		if labels[key] != "" {
			return "Flux"
		}
	}
	return labels[helmManagedLabel]
}

func namedOf(ref api.ObjectRef, obj *unstructured.Unstructured) Named {
	return Named{
		Ref:     ref,
		Owned:   len(obj.GetOwnerReferences()) > 0,
		Manager: ManagerOf(obj.GetLabels()),
	}
}

type consumer struct {
	who       string
	resource  string
	namespace string
	name      string
	prefix    string
	suffix    string
}

const (
	secretsResource    = "secrets"
	configMapsResource = "configmaps"
)

var knownConsumers = []consumer{
	{who: "the Kubernetes controller manager", resource: configMapsResource, name: "kube-root-ca.crt"},
	{who: "Helm, which keeps release history here", resource: secretsResource, prefix: "sh.helm.release.v1."},
	{who: "k3s, which serves the apiserver with it", resource: secretsResource, namespace: "kube-system", name: "k3s-serving"},
	{who: "k3s, which authenticates a node with it", resource: secretsResource, namespace: "kube-system", suffix: ".node-password.k3s"},
	{who: "k3s, which reads the cluster DNS address and domain from it", resource: configMapsResource, namespace: "kube-system", name: "cluster-dns"},
}

func (c consumer) covers(resource string, ref api.ObjectRef) bool {
	if c.resource != resource {
		return false
	}
	if c.namespace != "" && c.namespace != ref.Namespace {
		return false
	}
	if c.name != "" {
		return c.name == ref.Name
	}
	if c.prefix != "" {
		return strings.HasPrefix(ref.Name, c.prefix)
	}
	return c.suffix != "" && strings.HasSuffix(ref.Name, c.suffix)
}

func consumedBy(resource string, held Named) string {
	if held.Owned {
		return "something in this cluster owns it, so it is not loose"
	}
	for _, one := range knownConsumers {
		if one.covers(resource, held.Ref) {
			return "it is read by " + one.who
		}
	}
	if held.Manager != "" {
		return "it is managed by " + held.Manager + ", which will put it back"
	}
	return ""
}
