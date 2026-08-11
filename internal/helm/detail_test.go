package helm

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const manifest = `---
# Source: live-check/templates/cm.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: live-check
  namespace: demo
data:
  hello: world
---
# Source: live-check/templates/deploy.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: live-check
  namespace: demo
spec:
  replicas: 1
`

func detailPayload(spec release) string {
	return `{
	  "name": "` + spec.name + `",
	  "namespace": "` + spec.namespace + `",
	  "version": ` + strconv.FormatInt(spec.revision, 10) + `,
	  "info": {
	    "status": "` + spec.status + `",
	    "first_deployed": "2026-08-01T09:00:00Z",
	    "last_deployed": "` + deployedAt + `",
	    "description": "Upgrade complete",
	    "notes": "Thank you for installing live-check."
	  },
	  "chart": {"metadata": {"name": "` + spec.chart + `", "version": "` + spec.version + `", "appVersion": "` + spec.appVersion + `"}},
	  "config": {"replicaCount": 2, "image": {"tag": "1.2.3"}},
	  "manifest": ` + quote(manifest) + `
	}`
}

func quote(body string) string {
	replaced := strings.ReplaceAll(body, "\\", "\\\\")
	replaced = strings.ReplaceAll(replaced, "\"", "\\\"")
	replaced = strings.ReplaceAll(replaced, "\n", "\\n")
	return "\"" + replaced + "\""
}

func detailSecret(spec release) *corev1.Secret {
	body := []byte(base64.StdEncoding.EncodeToString(gzipped(detailPayload(spec))))
	return storedSecret(spec, body)
}

func resolver(apiVersion, kind string) (Kind, bool) {
	switch {
	case apiVersion == "v1" && kind == "ConfigMap":
		return Kind{Version: "v1", Resource: "configmaps", Namespaced: true}, true
	case apiVersion == "apps/v1" && kind == "Deployment":
		return Kind{Group: "apps", Version: "v1", Resource: "deployments", Namespaced: true}, true
	case apiVersion == "v1" && kind == "Namespace":
		return Kind{Version: "v1", Resource: "namespaces"}, true
	}
	return Kind{}, false
}

func TestDetailCarriesValuesNotesAndManifest(t *testing.T) {
	spec := sampleRelease()
	service := newService(k8sfake.NewClientset(detailSecret(spec)), nil, nil)

	got, err := service.Detail(context.Background(), "demo", "podinfo", resolver)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}

	if got.Release.ChartVersion != "6.9.2" {
		t.Fatalf("chart version = %q, want 6.9.2", got.Release.ChartVersion)
	}
	if !strings.Contains(got.Values, "replicaCount: 2") {
		t.Fatalf("values = %q, want the supplied values as yaml", got.Values)
	}
	if !strings.Contains(got.Values, "tag: 1.2.3") {
		t.Fatalf("values = %q, want nested values kept", got.Values)
	}
	if got.Notes != "Thank you for installing live-check." {
		t.Fatalf("notes = %q, want the release notes", got.Notes)
	}
	if !strings.Contains(got.Manifest, "kind: Deployment") {
		t.Fatalf("manifest = %q, want the rendered manifest", got.Manifest)
	}
	if got.FirstDeployed != "2026-08-01T09:00:00Z" {
		t.Fatalf("first deployed = %q, want the install time", got.FirstDeployed)
	}
	if got.Driver != DriverSecret {
		t.Fatalf("driver = %q, want secret", got.Driver)
	}
}

func TestDetailListsTheResourcesTheManifestRenders(t *testing.T) {
	service := newService(k8sfake.NewClientset(detailSecret(sampleRelease())), nil, nil)

	got, err := service.Detail(context.Background(), "demo", "podinfo", resolver)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}

	if len(got.Resources) != 2 {
		t.Fatalf("resources = %d, want 2", len(got.Resources))
	}
	first := got.Resources[0]
	if first.Kind != "ConfigMap" || first.Name != "live-check" {
		t.Fatalf("first = %+v, want the ConfigMap", first)
	}
	if first.Resource != "configmaps" {
		t.Fatalf("resource = %q, want discovery to have resolved the plural", first.Resource)
	}
	second := got.Resources[1]
	if second.Group != "apps" || second.Resource != "deployments" {
		t.Fatalf("second = %+v, want the apps/v1 deployment", second)
	}
	if second.Namespace != "demo" {
		t.Fatalf("namespace = %q, want demo", second.Namespace)
	}
}

func TestDetailLeavesAnUnknownKindUnresolved(t *testing.T) {
	service := newService(k8sfake.NewClientset(detailSecret(sampleRelease())), nil, nil)

	got, err := service.Detail(context.Background(), "demo", "podinfo",
		func(string, string) (Kind, bool) {
			return Kind{}, false
		})
	if err != nil {
		t.Fatalf("detail: %v", err)
	}

	if got.Resources[0].Resource != "" {
		t.Fatalf("resource = %q, want it unresolved", got.Resources[0].Resource)
	}
	if got.Resources[0].Kind != "ConfigMap" {
		t.Fatalf("kind = %q, want it still named", got.Resources[0].Kind)
	}
}

func TestDetailWorksWithNoResolverAtAll(t *testing.T) {
	service := newService(k8sfake.NewClientset(detailSecret(sampleRelease())), nil, nil)

	got, err := service.Detail(context.Background(), "demo", "podinfo", nil)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}

	if len(got.Resources) != 2 {
		t.Fatalf("resources = %d, want them listed even unresolved", len(got.Resources))
	}
}

func TestDetailOrdersTheHistoryNewestFirst(t *testing.T) {
	first := sampleRelease()
	first.revision = 1
	first.status = "superseded"
	first.version = "6.9.0"
	second := sampleRelease()
	second.revision = 2
	second.status = "superseded"
	second.version = "6.9.1"
	third := sampleRelease()
	service := newService(k8sfake.NewClientset(
		detailSecret(second), detailSecret(third), detailSecret(first),
	), nil, nil)

	got, err := service.Detail(context.Background(), "demo", "podinfo", resolver)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}

	if len(got.History) != 3 {
		t.Fatalf("history = %d, want every revision", len(got.History))
	}
	if got.History[0].Revision != 3 || got.History[2].Revision != 1 {
		t.Fatalf("history = %v, want newest first", got.History)
	}
	if got.History[1].ChartVersion != "6.9.1" {
		t.Fatalf("chart version = %q, want each revision's own", got.History[1].ChartVersion)
	}
	if got.Release.Revision != 3 {
		t.Fatalf("release = revision %d, want the newest", got.Release.Revision)
	}
}

func TestDetailRefusesAReleaseThatIsNotThere(t *testing.T) {
	service := newService(k8sfake.NewClientset(), nil, nil)

	_, err := service.Detail(context.Background(), "demo", "missing", resolver)

	if !errors.Is(err, ErrNoRelease) {
		t.Fatalf("err = %v, want it to say there is no such release", err)
	}
}

func TestDetailRefusesNamesThatCouldBeFlags(t *testing.T) {
	service := newService(k8sfake.NewClientset(), nil, nil)

	_, err := service.Detail(context.Background(), "demo", "--kubeconfig=/etc/shadow", resolver)

	if err == nil {
		t.Fatal("a flag-shaped release name was accepted")
	}
}

func TestDetailStillAnswersWhenTheNewestPayloadIsUnreadable(t *testing.T) {
	spec := sampleRelease()
	service := newService(k8sfake.NewClientset(storedSecret(spec, []byte("rubbish"))), nil, nil)

	got, err := service.Detail(context.Background(), "demo", "podinfo", resolver)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}

	if got.Release.Name != "podinfo" {
		t.Fatalf("name = %q, want the label's", got.Release.Name)
	}
	if !strings.Contains(got.Error, "could not be read") {
		t.Fatalf("error = %q, want it to say the payload is unreadable", got.Error)
	}
	if len(got.History) != 1 {
		t.Fatalf("history = %d, want the revision still listed", len(got.History))
	}
	if got.History[0].Description != "this revision's payload could not be read" {
		t.Fatalf("description = %q, want the unreadable note", got.History[0].Description)
	}
}

func TestDetailReportsARefusedList(t *testing.T) {
	cs := k8sfake.NewClientset()
	cs.PrependReactor("list", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("secrets is forbidden")
	})
	service := newService(cs, nil, nil)

	_, err := service.Detail(context.Background(), "demo", "podinfo", resolver)

	if err == nil {
		t.Fatal("a refused list reported success")
	}
}

func TestAReleaseWithNoSuppliedValuesReadsEmpty(t *testing.T) {
	spec := sampleRelease()
	body := []byte(base64.StdEncoding.EncodeToString(gzipped(`{"name":"podinfo","namespace":"demo","version":3,"info":{"status":"deployed"}}`)))
	service := newService(k8sfake.NewClientset(storedSecret(spec, body)), nil, nil)

	got, err := service.Detail(context.Background(), "demo", "podinfo", resolver)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}

	if got.Values != "" {
		t.Fatalf("values = %q, want nothing", got.Values)
	}
	if len(got.Resources) != 0 {
		t.Fatalf("resources = %v, want none", got.Resources)
	}
}

func TestAManifestDocumentThatIsNotAnObjectIsSkipped(t *testing.T) {
	got := resourcesOf("---\njust a string\n---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: real\n", "demo", nil)

	if len(got) != 1 {
		t.Fatalf("resources = %v, want only the real object", got)
	}
	if got[0].Name != "real" {
		t.Fatalf("name = %q, want real", got[0].Name)
	}
}

func TestAManifestDocumentWithNoNameIsSkipped(t *testing.T) {
	got := resourcesOf("apiVersion: v1\nkind: ConfigMap\nmetadata: {}\n", "demo", nil)

	if len(got) != 0 {
		t.Fatalf("resources = %v, want none", got)
	}
}

func TestDetailReadsAReleaseStoredInAConfigMap(t *testing.T) {
	spec := sampleRelease()
	entry := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sh.helm.release.v1.podinfo.v3",
			Namespace: "demo",
			Labels: map[string]string{
				"owner": "helm", "name": "podinfo", "version": "3", "status": "deployed",
			},
		},
		Data: map[string]string{releaseKey: base64.StdEncoding.EncodeToString(gzipped(detailPayload(spec)))},
	}
	service := newService(k8sfake.NewClientset(entry), nil, nil)

	got, err := service.Detail(context.Background(), "demo", "podinfo", resolver)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}

	if got.Driver != DriverConfigMap {
		t.Fatalf("driver = %q, want configmap", got.Driver)
	}
	if got.Release.ChartVersion != "6.9.2" {
		t.Fatalf("chart version = %q, want the configmap payload decoded", got.Release.ChartVersion)
	}
}

func TestANamespacedResourceInheritsTheReleaseNamespace(t *testing.T) {
	manifest := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: live-check\n"

	got := resourcesOf(manifest, "helm-live", resolver)

	if len(got) != 1 {
		t.Fatalf("resources = %v, want one", got)
	}
	if got[0].Namespace != "helm-live" {
		t.Fatalf("namespace = %q, want the release's, since helm renders none", got[0].Namespace)
	}
}

func TestAResourceKeepsTheNamespaceTheManifestGave(t *testing.T) {
	manifest := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm\n  namespace: elsewhere\n"

	got := resourcesOf(manifest, "helm-live", resolver)

	if got[0].Namespace != "elsewhere" {
		t.Fatalf("namespace = %q, want the manifest's own", got[0].Namespace)
	}
}

func TestAClusterScopedResourceGetsNoNamespace(t *testing.T) {
	manifest := "apiVersion: v1\nkind: Namespace\nmetadata:\n  name: made-by-chart\n"

	got := resourcesOf(manifest, "helm-live", resolver)

	if got[0].Namespace != "" {
		t.Fatalf("namespace = %q, want none for a cluster-scoped kind", got[0].Namespace)
	}
}

func TestAnUnresolvedKindIsLeftWithoutANamespace(t *testing.T) {
	manifest := "apiVersion: acme.io/v1\nkind: Widget\nmetadata:\n  name: thing\n"

	got := resourcesOf(manifest, "helm-live", resolver)

	if got[0].Namespace != "" {
		t.Fatalf("namespace = %q, want none when the kind is unknown", got[0].Namespace)
	}
}
