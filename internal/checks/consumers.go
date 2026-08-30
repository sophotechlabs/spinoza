package checks

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

// Named is one object the corpus knows of by name, with what the cluster says
// about who put it there. A secret is only ever read this far: the audit needs
// to know it exists and who manages it, never what is in it.
type Named struct {
	Ref     api.ObjectRef
	Owned   bool
	Manager string
}

var fluxLabels = []string{"helm.toolkit.fluxcd.io/name", "kustomize.toolkit.fluxcd.io/name"}

// ManagerOf reads who the cluster says looks after this object. A manager is
// not a reference, but it does mean deleting the object is somebody else's
// decision: whatever put it there will put it back.
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

// A consumer is something that reads an object without ever naming it in the
// cluster. The list is written by hand from what each of these is known to do,
// so it only covers what is on it; anything else stays a finding. Nothing here
// is hidden — the audit reports these as silenced, with the reason.
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

// consumedBy says why this object is not the leftover it looks like, or nothing
// when it is one.
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
