package helm

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/charts"
)

const deployedAt = "2026-08-11T09:30:00Z"

func newService(cs kubernetes.Interface, index Charts, repos []charts.Repo) *Service {
	return NewService(cs, nil, index, repos, api.ContextRef{Name: "kind-spinoza"})
}

type release struct {
	name       string
	namespace  string
	revision   int64
	status     string
	chart      string
	version    string
	appVersion string
}

func payloadJSON(spec release) string {
	return fmt.Sprintf(`{
	  "name": %q,
	  "namespace": %q,
	  "version": %d,
	  "info": {"status": %q, "last_deployed": %q, "description": "Upgrade complete"},
	  "chart": {"metadata": {"name": %q, "version": %q, "appVersion": %q}}
	}`, spec.name, spec.namespace, spec.revision, spec.status, deployedAt,
		spec.chart, spec.version, spec.appVersion)
}

func gzipped(body string) []byte {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	_, err := writer.Write([]byte(body))
	if err != nil {
		panic(err)
	}
	err = writer.Close()
	if err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func storedSecret(spec release, data []byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:              fmt.Sprintf("sh.helm.release.v1.%s.v%d", spec.name, spec.revision),
			Namespace:         spec.namespace,
			CreationTimestamp: metav1.NewTime(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)),
			Labels: map[string]string{
				"owner":   "helm",
				"name":    spec.name,
				"version": strconv.FormatInt(spec.revision, 10),
				"status":  spec.status,
			},
		},
		Type: storageType,
		Data: map[string][]byte{releaseKey: data},
	}
}

func helmSecret(spec release) *corev1.Secret {
	body := []byte(base64.StdEncoding.EncodeToString(gzipped(payloadJSON(spec))))
	return storedSecret(spec, body)
}

func sampleRelease() release {
	return release{
		name:       "podinfo",
		namespace:  "demo",
		revision:   3,
		status:     "deployed",
		chart:      "podinfo",
		version:    "6.9.2",
		appVersion: "6.9.2",
	}
}

func TestListReadsAGzippedBase64Release(t *testing.T) {
	cs := k8sfake.NewClientset(helmSecret(sampleRelease()))

	got, err := newService(cs, nil, nil).List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(got.Releases) != 1 {
		t.Fatalf("releases = %d, want 1", len(got.Releases))
	}
	release := got.Releases[0]
	if release.Name != "podinfo" {
		t.Fatalf("name = %q, want podinfo", release.Name)
	}
	if release.Namespace != "demo" {
		t.Fatalf("namespace = %q, want demo", release.Namespace)
	}
	if release.Chart != "podinfo" {
		t.Fatalf("chart = %q, want podinfo", release.Chart)
	}
	if release.ChartVersion != "6.9.2" {
		t.Fatalf("chart version = %q, want 6.9.2", release.ChartVersion)
	}
	if release.AppVersion != "6.9.2" {
		t.Fatalf("app version = %q, want 6.9.2", release.AppVersion)
	}
	if release.Revision != 3 {
		t.Fatalf("revision = %d, want 3", release.Revision)
	}
	if release.Status != "deployed" {
		t.Fatalf("status = %q, want deployed", release.Status)
	}
	if release.Updated != deployedAt {
		t.Fatalf("updated = %q, want %s", release.Updated, deployedAt)
	}
	if release.Description != "Upgrade complete" {
		t.Fatalf("description = %q, want the upgrade note", release.Description)
	}
	if got.Error != "" {
		t.Fatalf("error = %q, want none", got.Error)
	}
}

func TestListReadsAReleaseThatWasNeverGzipped(t *testing.T) {
	spec := sampleRelease()
	body := []byte(base64.StdEncoding.EncodeToString([]byte(payloadJSON(spec))))
	cs := k8sfake.NewClientset(storedSecret(spec, body))

	got, err := newService(cs, nil, nil).List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(got.Releases) != 1 {
		t.Fatalf("releases = %d, want 1", len(got.Releases))
	}
	if got.Releases[0].ChartVersion != "6.9.2" {
		t.Fatalf("chart version = %q, want 6.9.2", got.Releases[0].ChartVersion)
	}
}

func TestListReadsAReleaseThatWasNeverBase64Encoded(t *testing.T) {
	spec := sampleRelease()
	cs := k8sfake.NewClientset(storedSecret(spec, gzipped(payloadJSON(spec))))

	got, err := newService(cs, nil, nil).List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(got.Releases) != 1 {
		t.Fatalf("releases = %d, want 1", len(got.Releases))
	}
	if got.Releases[0].Chart != "podinfo" {
		t.Fatalf("chart = %q, want podinfo", got.Releases[0].Chart)
	}
}

func TestListKeepsOnlyTheNewestRevision(t *testing.T) {
	first := sampleRelease()
	first.revision = 1
	first.status = "superseded"
	first.version = "6.9.0"
	second := sampleRelease()
	second.revision = 2
	second.status = "superseded"
	second.version = "6.9.1"
	third := sampleRelease()
	cs := k8sfake.NewClientset(helmSecret(third), helmSecret(first), helmSecret(second))

	got, err := newService(cs, nil, nil).List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(got.Releases) != 1 {
		t.Fatalf("releases = %d, want the newest revision only", len(got.Releases))
	}
	if got.Releases[0].Revision != 3 {
		t.Fatalf("revision = %d, want 3", got.Releases[0].Revision)
	}
	if got.Releases[0].ChartVersion != "6.9.2" {
		t.Fatalf("chart version = %q, want the newest", got.Releases[0].ChartVersion)
	}
}

func TestListSortsByNamespaceThenName(t *testing.T) {
	specs := []release{
		{name: "zoo", namespace: "apps", revision: 1, status: "deployed"},
		{name: "alpha", namespace: "apps", revision: 1, status: "deployed"},
		{name: "beta", namespace: "aaa", revision: 1, status: "deployed"},
	}
	objs := make([]runtime.Object, 0, len(specs))
	for _, spec := range specs {
		objs = append(objs, helmSecret(spec))
	}
	cs := k8sfake.NewClientset(objs...)

	got, err := newService(cs, nil, nil).List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	order := make([]string, 0, len(got.Releases))
	for _, release := range got.Releases {
		order = append(order, release.Namespace+"/"+release.Name)
	}
	want := []string{"aaa/beta", "apps/alpha", "apps/zoo"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestListIgnoresSecretsThatAreNotHelmStorage(t *testing.T) {
	other := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tls",
			Namespace: "demo",
			Labels:    map[string]string{"owner": "helm"},
		},
		Type: corev1.SecretTypeTLS,
	}
	cs := k8sfake.NewClientset(helmSecret(sampleRelease()), other)

	got, err := newService(cs, nil, nil).List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(got.Releases) != 1 {
		t.Fatalf("releases = %d, want only the helm one", len(got.Releases))
	}
}

func TestListFallsBackToTheLabelsWhenThePayloadIsUnreadable(t *testing.T) {
	spec := sampleRelease()
	cs := k8sfake.NewClientset(storedSecret(spec, []byte("not a release at all")))

	got, err := newService(cs, nil, nil).List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(got.Releases) != 1 {
		t.Fatalf("releases = %d, want the fallback from labels", len(got.Releases))
	}
	release := got.Releases[0]
	if release.Name != "podinfo" {
		t.Fatalf("name = %q, want the label", release.Name)
	}
	if release.Revision != 3 {
		t.Fatalf("revision = %d, want the label", release.Revision)
	}
	if release.Status != "deployed" {
		t.Fatalf("status = %q, want the label", release.Status)
	}
	if release.Chart != "" {
		t.Fatalf("chart = %q, want it unknown", release.Chart)
	}
	if release.Updated != "2026-08-01T00:00:00Z" {
		t.Fatalf("updated = %q, want the secret's creation time", release.Updated)
	}
	if !strings.Contains(got.Error, "1 release payloads could not be read") {
		t.Fatalf("error = %q, want it to say one payload was unreadable", got.Error)
	}
}

func TestListSkipsASecretWithNothingToIdentifyIt(t *testing.T) {
	nameless := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sh.helm.release.v1.mystery.v1",
			Namespace: "demo",
			Labels:    map[string]string{"owner": "helm"},
		},
		Type: storageType,
		Data: map[string][]byte{releaseKey: []byte("broken")},
	}
	cs := k8sfake.NewClientset(nameless)

	got, err := newService(cs, nil, nil).List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(got.Releases) != 0 {
		t.Fatalf("releases = %v, want none", got.Releases)
	}
}

func TestListReportsARefusedSecretList(t *testing.T) {
	cs := k8sfake.NewClientset()
	cs.PrependReactor("list", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("secrets is forbidden")
	})

	got, err := newService(cs, nil, nil).List(context.Background())

	if err == nil {
		t.Fatal("a refused list reported success")
	}
	if !strings.Contains(err.Error(), "secrets is forbidden") {
		t.Fatalf("err = %v, want the refusal", err)
	}
	if got.Releases == nil {
		t.Fatal("the empty result should still carry a list, not nil")
	}
}

func TestListAsksOnlyForHelmOwnedSecrets(t *testing.T) {
	cs := k8sfake.NewClientset()
	selectors := []string{}
	cs.PrependReactor("list", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		list, ok := action.(k8stesting.ListAction)
		if ok {
			selectors = append(selectors, list.GetListRestrictions().Labels.String())
		}
		return false, nil, nil
	})

	_, err := newService(cs, nil, nil).List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(selectors) != 1 {
		t.Fatalf("secrets were listed %d times, want once", len(selectors))
	}
	if selectors[0] != "owner=helm" {
		t.Fatalf("selector = %q, want owner=helm", selectors[0])
	}
}

func TestListFollowsThePagesTheApiserverHandsBack(t *testing.T) {
	first := sampleRelease()
	second := sampleRelease()
	second.name = "other"
	cs := k8sfake.NewClientset()
	page := 0
	cs.PrependReactor("list", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		page++
		if page == 1 {
			return true, &corev1.SecretList{
				ListMeta: metav1.ListMeta{Continue: "next-page"},
				Items:    []corev1.Secret{*helmSecret(first)},
			}, nil
		}
		return true, &corev1.SecretList{Items: []corev1.Secret{*helmSecret(second)}}, nil
	})

	got, err := newService(cs, nil, nil).List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if page != 2 {
		t.Fatalf("pages read = %d, want 2", page)
	}
	if len(got.Releases) != 2 {
		t.Fatalf("releases = %d, want both pages", len(got.Releases))
	}
}

func TestListSaysWhenItStoppedReadingPages(t *testing.T) {
	cs := k8sfake.NewClientset()
	page := 0
	cs.PrependReactor("list", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		page++
		items := make([]corev1.Secret, 0, maxObjects)
		for i := range maxObjects {
			spec := sampleRelease()
			spec.name = fmt.Sprintf("release-%d-%d", page, i)
			items = append(items, *helmSecret(spec))
		}
		return true, &corev1.SecretList{
			ListMeta: metav1.ListMeta{Continue: "next-page"},
			Items:    items,
		}, nil
	})

	got, err := newService(cs, nil, nil).List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if page != 1 {
		t.Fatalf("pages read = %d, want it to stop after the cap", page)
	}
	if !strings.Contains(got.Error, "some releases may be missing") {
		t.Fatalf("error = %q, want the truncation named", got.Error)
	}
}

func TestListReportsNothingForAClusterWithoutHelm(t *testing.T) {
	got, err := newService(k8sfake.NewClientset(), nil, nil).List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(got.Releases) != 0 {
		t.Fatalf("releases = %v, want none", got.Releases)
	}
	if got.Error != "" {
		t.Fatalf("error = %q, want none", got.Error)
	}
}

func TestAnEmptyPayloadIsRefused(t *testing.T) {
	_, err := decode(nil)

	if err == nil {
		t.Fatal("an empty payload decoded successfully")
	}
}

func TestAGzipHeaderWithRubbishBehindItIsRefused(t *testing.T) {
	_, err := decode([]byte{0x1f, 0x8b, 0x00, 0x01})

	if err == nil {
		t.Fatal("a broken gzip stream decoded successfully")
	}
	if errors.Is(err, errNotGzip) {
		t.Fatalf("err = %v, want the gzip failure itself", err)
	}
}

func TestALabelWithANonNumericRevisionReadsAsZero(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "demo",
			Labels:    map[string]string{"name": "podinfo", "version": "latest"},
		},
	}

	got := storedOf(DriverSecret, &secret.ObjectMeta, nil)

	if got.revision != 0 {
		t.Fatalf("revision = %d, want 0", got.revision)
	}
}

func TestAPartialPayloadIsCompletedFromTheLabels(t *testing.T) {
	spec := sampleRelease()
	body := []byte(base64.StdEncoding.EncodeToString(gzipped(`{"chart":{"metadata":{"name":"podinfo","version":"6.9.2"}}}`)))
	cs := k8sfake.NewClientset(storedSecret(spec, body))

	got, err := newService(cs, nil, nil).List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(got.Releases) != 1 {
		t.Fatalf("releases = %d, want 1", len(got.Releases))
	}
	release := got.Releases[0]
	if release.Name != "podinfo" {
		t.Fatalf("name = %q, want the label", release.Name)
	}
	if release.Namespace != "demo" {
		t.Fatalf("namespace = %q, want the secret's", release.Namespace)
	}
	if release.Revision != 3 {
		t.Fatalf("revision = %d, want the label", release.Revision)
	}
	if release.Status != "deployed" {
		t.Fatalf("status = %q, want the label", release.Status)
	}
	if release.Updated != "2026-08-01T00:00:00Z" {
		t.Fatalf("updated = %q, want the secret's creation time", release.Updated)
	}
	if release.ChartVersion != "6.9.2" {
		t.Fatalf("chart version = %q, want what the payload did carry", release.ChartVersion)
	}
	if got.Error != "" {
		t.Fatalf("error = %q, want none — the payload parsed fine", got.Error)
	}
}

type stubCharts struct {
	versions map[string]string
	warmed   []string
}

func (s *stubCharts) Latest(repo charts.Repo, chart string) string {
	return s.versions[repo.URL+"|"+chart]
}

func (s *stubCharts) Warm(repo charts.Repo, chart string) {
	s.warmed = append(s.warmed, repo.URL+"|"+chart)
}

func TestListMarksAReleaseThatHasANewerChart(t *testing.T) {
	cs := k8sfake.NewClientset(helmSecret(sampleRelease()))
	index := &stubCharts{versions: map[string]string{
		"https://charts.example.com|podinfo": "6.9.5",
	}}
	repos := []charts.Repo{{URL: "https://charts.example.com"}}

	got, err := newService(cs, index, repos).List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if got.Releases[0].Latest != "6.9.5" {
		t.Fatalf("latest = %q, want 6.9.5", got.Releases[0].Latest)
	}
	if !got.Releases[0].Outdated {
		t.Fatal("a release behind its repo was not marked outdated")
	}
	if len(index.warmed) != 1 {
		t.Fatalf("warmed %v, want the one repo asked once", index.warmed)
	}
}

func TestListLeavesAnUpToDateReleaseAlone(t *testing.T) {
	cs := k8sfake.NewClientset(helmSecret(sampleRelease()))
	index := &stubCharts{versions: map[string]string{
		"https://charts.example.com|podinfo": "6.9.2",
	}}

	got, err := newService(cs, index, []charts.Repo{{URL: "https://charts.example.com"}}).List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if got.Releases[0].Latest != "6.9.2" {
		t.Fatalf("latest = %q, want the same version", got.Releases[0].Latest)
	}
	if got.Releases[0].Outdated {
		t.Fatal("a current release was marked outdated")
	}
}

func TestListTakesTheHighestVersionAcrossRepositories(t *testing.T) {
	cs := k8sfake.NewClientset(helmSecret(sampleRelease()))
	index := &stubCharts{versions: map[string]string{
		"https://one.example.com|podinfo":   "6.9.3",
		"https://two.example.com|podinfo":   "7.1.0",
		"https://three.example.com|podinfo": "6.0.0",
	}}
	repos := []charts.Repo{
		{URL: "https://one.example.com"},
		{URL: "https://two.example.com"},
		{URL: "https://three.example.com"},
	}

	got, err := newService(cs, index, repos).List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if got.Releases[0].Latest != "7.1.0" {
		t.Fatalf("latest = %q, want the highest across repos", got.Releases[0].Latest)
	}
}

func TestListSaysNothingAboutLatestWithoutAnIndex(t *testing.T) {
	cs := k8sfake.NewClientset(helmSecret(sampleRelease()))

	got, err := newService(cs, nil, []charts.Repo{{URL: "https://one.example.com"}}).List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if got.Releases[0].Latest != "" {
		t.Fatalf("latest = %q, want it unknown", got.Releases[0].Latest)
	}
	if got.Releases[0].Outdated {
		t.Fatal("a release with no index to compare against was marked outdated")
	}
}

func TestListSkipsTheLookupForAChartItCouldNotName(t *testing.T) {
	spec := sampleRelease()
	cs := k8sfake.NewClientset(storedSecret(spec, []byte("unreadable")))
	index := &stubCharts{versions: map[string]string{}}

	got, err := newService(cs, index, []charts.Repo{{URL: "https://one.example.com"}}).List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(index.warmed) != 0 {
		t.Fatalf("warmed %v, want no lookup for a nameless chart", index.warmed)
	}
	if got.Releases[0].Latest != "" {
		t.Fatalf("latest = %q, want it unknown", got.Releases[0].Latest)
	}
}

func TestListIgnoresARepositoryWithNothingForThatChart(t *testing.T) {
	cs := k8sfake.NewClientset(helmSecret(sampleRelease()))
	index := &stubCharts{versions: map[string]string{
		"https://two.example.com|podinfo": "7.0.0",
	}}
	repos := []charts.Repo{{URL: "https://one.example.com"}, {URL: "https://two.example.com"}}

	got, err := newService(cs, index, repos).List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if got.Releases[0].Latest != "7.0.0" {
		t.Fatalf("latest = %q, want the repo that has it", got.Releases[0].Latest)
	}
}
