package checks

import (
	"fmt"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func namespaceCount(t *testing.T, report api.CheckReport, namespace string) api.NamespaceCount {
	t.Helper()
	for _, entry := range report.Namespaces {
		if entry.Namespace == namespace {
			return entry
		}
	}
	t.Fatalf("no summary for namespace %q in %v", namespace, report.Namespaces)
	return api.NamespaceCount{}
}

func TestTheSummaryCountsFindingsWhereTheyLive(t *testing.T) {
	report := report(t, privilegedDeployment("api"), inNamespace(privilegedDeployment("agent"), "kube-system"))

	if count := namespaceCount(t, report, testNamespace); count.High == 0 {
		t.Fatalf("%s carried no high findings: %+v", testNamespace, count)
	}
	if count := namespaceCount(t, report, "kube-system"); count.Total == 0 {
		t.Fatalf("kube-system carried no findings: %+v", count)
	}
}

func TestTheSummaryCountsTheSeverityEachRowShows(t *testing.T) {
	report := report(t, inNamespace(privilegedDeployment("agent"), "kube-system"))

	count := namespaceCount(t, report, "kube-system")
	shown := map[string]int{}
	for _, group := range report.Groups {
		for _, finding := range group.Findings {
			if report.Objects[finding.Ref].Namespace != "kube-system" {
				continue
			}
			shown[finding.Severity]++
		}
	}
	if count.High != shown[severityHigh] {
		t.Fatalf("the panel counted %d high where %d rows read high: %+v", count.High, shown[severityHigh], count)
	}
	if count.Medium != shown[severityMedium] {
		t.Fatalf("the panel counted %d medium where %d rows read medium: %+v", count.Medium, shown[severityMedium], count)
	}
	if count.Low != shown[severityLow] {
		t.Fatalf("the panel counted %d low where %d rows read low: %+v", count.Low, shown[severityLow], count)
	}
}

func TestTheNamespaceCarryingTheMostComesFirst(t *testing.T) {
	report := report(t,
		privilegedDeployment("api"), privilegedDeployment("web"), privilegedDeployment("worker"),
		inNamespace(privilegedDeployment("agent"), "quiet"))

	if report.Namespaces[0].Namespace != testNamespace {
		t.Fatalf("the summary put %q first", report.Namespaces[0].Namespace)
	}
}

func TestWhatIsMutedIsLeftOutOfTheSummary(t *testing.T) {
	loud := report(t, inNamespace(privilegedDeployment("agent"), "kube-system"))
	quiet := withMutes(t, []Mute{{Check: privilegedCheck, Namespace: "kube-system"}},
		inNamespace(privilegedDeployment("agent"), "kube-system"))

	before := namespaceCount(t, loud, "kube-system").Total
	after := namespaceCount(t, quiet, "kube-system").Total
	if after != before-1 {
		t.Fatalf("the summary counted %d before the mute and %d after", before, after)
	}
}

func TestWhatBelongsToNoNamespaceIsLeftOutOfTheSummary(t *testing.T) {
	report := report(t, privilegedDeployment("api"))

	for _, entry := range report.Namespaces {
		if entry.Namespace == "" {
			t.Fatal("the summary carried a row for no namespace at all")
		}
	}
}

func TestOneNamespaceCanBeAskedForOnItsOwn(t *testing.T) {
	keep := wholeCluster()
	keep.Namespace = "kube-system"

	report := Run(t.Context(), newLister(
		privilegedDeployment("api"), inNamespace(privilegedDeployment("agent"), "kube-system"),
	), descriptors(), api.Metrics{}, keep, 0)

	finding := onlyFinding(t, report, privilegedCheck)
	if objectFor(t, report, finding).Namespace != "kube-system" {
		t.Fatalf("asking for one namespace returned %s", objectFor(t, report, finding).Namespace)
	}
}

func TestTheNamespaceBreakdownAndHeadlineCountTheSameFindings(t *testing.T) {
	for _, showMuted := range []bool{false, true} {
		t.Run(fmt.Sprintf("show-muted-%t", showMuted), func(t *testing.T) {
			keep := wholeCluster()
			keep.ShowMuted = showMuted
			keep.Mutes = []Mute{{Check: privilegedCheck, Namespace: "kube-system"}}
			found := Run(t.Context(), newLister(
				privilegedDeployment("api"),
				inNamespace(privilegedDeployment("agent"), "kube-system"),
			), descriptors(), api.Metrics{}, keep, 0)

			headline := 0
			for _, group := range found.Groups {
				headline += group.Total
			}
			breakdown := 0
			for _, namespace := range found.Namespaces {
				breakdown += namespace.Total
			}
			if headline != breakdown {
				t.Fatalf("headline counted %d findings while namespaces counted %d", headline, breakdown)
			}
		})
	}
}
