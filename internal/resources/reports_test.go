package resources

import (
	"testing"

	"github.com/sophotechlabs/spinoza/internal/checks"
)

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

func TestTheIssueQueueIsBuiltFromTheSameCluster(t *testing.T) {
	mgr, cancel := newManager(t, newClient(t, newDeployment("default", "web")))
	defer cancel()

	queue := mgr.Issues(t.Context())

	if queue.Rows == nil {
		t.Fatal("the queue carried no rows slice at all")
	}
}
