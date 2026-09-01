package resources

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	kubediscovery "k8s.io/client-go/discovery"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/checks"
	"github.com/sophotechlabs/spinoza/internal/kube"
)

type factsDiscovery struct {
	kubediscovery.CachedDiscoveryInterface

	info       *version.Info
	versionErr error
	groups     *metav1.APIGroupList
	groupsErr  error
}

func (f factsDiscovery) ServerVersion() (*version.Info, error) {
	return f.info, f.versionErr
}

func (f factsDiscovery) ServerGroups() (*metav1.APIGroupList, error) {
	return f.groups, f.groupsErr
}

func factsWarningSink(t *testing.T) *kube.WarningSink {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kubeconfig")
	body := `apiVersion: v1
kind: Config
current-context: test
clusters:
- name: test
  cluster:
    server: https://127.0.0.1:1
    insecure-skip-tls-verify: true
contexts:
- name: test
  context:
    cluster: test
    user: test
users:
- name: test
  user: {}
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	bundle, err := kube.LoadContext(api.ContextRef{Name: "test"}, kube.Options{Kubeconfig: path})
	if err != nil {
		t.Fatalf("load context: %v", err)
	}
	return bundle.Warnings
}

func TestTheAuditRunsOverWhatTheClusterHolds(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()

	report := mgr.Checks(t.Context(), checks.Filter{WholeCluster: true})

	if len(report.Groups) == 0 {
		t.Fatal("the audit came back with no groups at all")
	}
}

func TestAnExportedAuditCarriesEveryFinding(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()

	report := mgr.CheckExport(t.Context(), checks.Filter{WholeCluster: true})

	if len(report.Groups) == 0 {
		t.Fatal("the export came back with no groups at all")
	}
}

func TestAPageOfFindingsComesBackForACheckThatRan(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()
	keep := checks.Filter{WholeCluster: true}
	report := mgr.Checks(t.Context(), keep)
	if len(report.Groups) == 0 {
		t.Fatal("nothing to page")
	}

	page, err := mgr.CheckPage(t.Context(), report.Groups[0].ID, "", keep)
	if err != nil {
		t.Fatalf("page: %v", err)
	}
	if page.Findings == nil {
		t.Fatal("the page carried no findings slice at all")
	}
}

func TestAPageForACheckThatDoesNotExistSaysSo(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	_, err := mgr.CheckPage(t.Context(), "no-such-check", "", checks.Filter{WholeCluster: true})

	if err == nil {
		t.Fatal("a check nobody registered was paged without complaint")
	}
}

func TestAFingerprintCountsWhatTheAuditFound(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()

	base := mgr.CheckFingerprint(t.Context(), checks.Filter{WholeCluster: true})

	if base.Counts == nil {
		t.Fatal("the baseline counted nothing at all")
	}
}

func TestTheFactsComeFromDiscovery(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t))
	defer cancel()

	facts := mgr.Facts()

	if facts.ServerVersion != "" {
		t.Fatalf("a manager with no discovery reported version %q", facts.ServerVersion)
	}
}

func TestFactsCarryVersionServedAPIsAndWarnings(t *testing.T) {
	warnings := factsWarningSink(t)
	warnings.HandleWarningHeader(299, "kubernetes", "extensions/v1beta1 is deprecated")
	discovery := factsDiscovery{
		info: &version.Info{GitVersion: "v1.34.2"},
		groups: &metav1.APIGroupList{Groups: []metav1.APIGroup{
			{
				Name: "apps",
				Versions: []metav1.GroupVersionForDiscovery{
					{GroupVersion: "apps/v1", Version: "v1"},
					{GroupVersion: "apps/v1beta1", Version: "v1beta1"},
				},
			},
		}},
	}
	manager := NewManager(t.Context(), Deps{Warnings: warnings})
	manager.disco = discovery

	facts := manager.Facts()

	if facts.ServerVersion != "v1.34.2" {
		t.Fatalf("version = %q", facts.ServerVersion)
	}
	if len(facts.ServedVersions) != 2 {
		t.Fatalf("served versions = %v", facts.ServedVersions)
	}
	if facts.ServedVersions[0] != "apps/v1" || facts.ServedVersions[1] != "apps/v1beta1" {
		t.Fatalf("served versions = %v", facts.ServedVersions)
	}
	if len(facts.Warnings) != 1 || facts.Warnings[0] != "extensions/v1beta1 is deprecated" {
		t.Fatalf("warnings = %v", facts.Warnings)
	}
}

func TestFactsIgnoreDiscoveryCallsThatFail(t *testing.T) {
	manager := NewManager(t.Context(), Deps{})
	manager.disco = factsDiscovery{
		versionErr: errors.New("version is forbidden"),
		groupsErr:  errors.New("groups are forbidden"),
	}

	facts := manager.Facts()

	if facts.ServerVersion != "" {
		t.Fatalf("version = %q, want none", facts.ServerVersion)
	}
	if len(facts.ServedVersions) != 0 {
		t.Fatalf("served versions = %v, want none", facts.ServedVersions)
	}
}

func TestTheIssueQueueIsBuiltFromTheSameCluster(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()

	queue := mgr.Issues(t.Context())

	if queue.Rows == nil {
		t.Fatal("the queue carried no rows slice at all")
	}
}
