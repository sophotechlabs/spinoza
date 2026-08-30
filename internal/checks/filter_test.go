package checks

import (
	"errors"
	"net/url"
	"slices"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func filtered(t *testing.T, keep Filter, objects ...*unstructured.Unstructured) api.CheckReport {
	t.Helper()
	keep.WholeCluster = true
	return Run(t.Context(), newLister(objects...), descriptors(), api.Metrics{}, keep, 0)
}

func groupIDs(report api.CheckReport) []string {
	out := make([]string, 0, len(report.Groups))
	for _, group := range report.Groups {
		out = append(out, group.ID)
	}
	return out
}

func holds(names []string, wanted string) bool {
	return slices.Contains(names, wanted)
}

// what the filter reads off a query string

func TestAFilterIsReadFromWhatTheBrowserSends(t *testing.T) {
	keep := ParseFilter(url.Values{
		"disabled":       {"cpu-limit-set, image-latest"},
		"skipNamespaces": {"kube-system,flux-system"},
		"namespace":      {"prod"},
		"minSeverity":    {"medium"},
		"everyKind":      {"1"},
		"onlyNew":        {"1"},
		"showMuted":      {"true"},
	})

	if len(keep.Disabled) != 2 || keep.Disabled[0] != "cpu-limit-set" {
		t.Fatalf("disabled was %v", keep.Disabled)
	}
	if len(keep.SkipNamespaces) != 2 {
		t.Fatalf("namespaces was %v", keep.SkipNamespaces)
	}
	if keep.MinSeverity != severityMedium {
		t.Fatalf("severity was %q", keep.MinSeverity)
	}
	if !keep.WholeCluster {
		t.Fatal("wholeCluster was not read")
	}
	if !keep.EveryKind {
		t.Fatal("everyKind was not read")
	}
	if keep.Namespace != "prod" {
		t.Fatalf("namespace was %q", keep.Namespace)
	}
	if !keep.OnlyNew || !keep.ShowMuted {
		t.Fatalf("onlyNew was %v and showMuted was %v", keep.OnlyNew, keep.ShowMuted)
	}
	if ParseFilter(url.Values{"wholeCluster": {"0"}}).WholeCluster {
		t.Fatal("a caller asking for workloads only was still given the whole cluster")
	}
}

func TestASeverityNobodyDefinedIsIgnoredRatherThanObeyed(t *testing.T) {
	keep := ParseFilter(url.Values{"minSeverity": {"catastrophic"}})

	if keep.MinSeverity != "" {
		t.Fatalf("severity was %q, want it dropped", keep.MinSeverity)
	}
	if !keep.WholeCluster {
		t.Fatal("a caller that said nothing was narrowed to workloads")
	}
	if keep.EveryKind {
		t.Fatal("a caller that said nothing was given the expensive read")
	}
}

func TestAnEmptyFilterChangesNothing(t *testing.T) {
	full := filtered(t, Filter{}, deployment("api", podSpec(container("app", nil))))

	if len(full.Groups) != len(registry()) {
		t.Fatalf("an empty filter reported %d of %d checks", len(full.Groups), len(registry()))
	}
}

// what it takes away

func TestADisabledCheckIsNotReportedAtAll(t *testing.T) {
	report := filtered(t, Filter{Disabled: []string{"image-latest"}},
		deployment("api", podSpec(container("app", map[string]any{"image": "busybox"}))))

	if holds(groupIDs(report), "image-latest") {
		t.Fatal("a disabled check was still reported")
	}
	if !holds(groupIDs(report), "privileged-containers") {
		t.Fatal("disabling one check took another with it")
	}
}

func TestASeverityFloorDropsEverythingBelowIt(t *testing.T) {
	report := filtered(t, Filter{MinSeverity: severityHigh},
		deployment("api", podSpec(container("app", nil))))

	for _, group := range report.Groups {
		if group.Severity != severityHigh {
			t.Fatalf("%s is %s and survived a high floor", group.ID, group.Severity)
		}
	}
	if len(report.Groups) == 0 {
		t.Fatal("a high floor left nothing at all")
	}
}

func TestASkippedNamespaceLosesItsFindingsAndItsCount(t *testing.T) {
	loud := deployment("api", podSpec(container("app", map[string]any{"image": "busybox"})))

	kept := filtered(t, Filter{}, loud)
	if groupNamed(t, kept, "image-latest").Total != 1 {
		t.Fatal("the fixture did not trip image-latest")
	}

	skipped := filtered(t, Filter{SkipNamespaces: []string{testNamespace}}, loud)
	group := groupNamed(t, skipped, "image-latest")
	if group.Total != 0 || len(group.Findings) != 0 {
		t.Fatalf("a skipped namespace left %d findings and a total of %d", len(group.Findings), group.Total)
	}
}

func TestSkippingOneNamespaceLeavesTheOthers(t *testing.T) {
	here := deployment("api", podSpec(container("app", map[string]any{"image": "busybox"})))
	elsewhere := deployment("web", podSpec(container("app", map[string]any{"image": "busybox"})))
	elsewhere.SetNamespace("other")

	report := filtered(t, Filter{SkipNamespaces: []string{"other"}}, here, elsewhere)

	if groupNamed(t, report, "image-latest").Total != 1 {
		t.Fatal("skipping one namespace did not leave the other")
	}
}

// what the whole-cluster switch gathers

func TestTheWorkloadOnlyAuditAsksForFewerKinds(t *testing.T) {
	narrow, _ := needed(descriptors(), false)
	wide, _ := needed(descriptors(), true)

	if len(narrow) >= len(wide) {
		t.Fatalf("workload-only asked for %d types and whole-cluster for %d", len(narrow), len(wide))
	}
	for _, desc := range narrow {
		if desc.Resource == "secrets" || desc.Resource == "configmaps" {
			t.Fatalf("the workload-only audit asked for %s", desc.Resource)
		}
	}
}

func TestACheckNeedingTheWiderCorpusIsSkippedWithoutIt(t *testing.T) {
	report := Run(t.Context(), newLister(), descriptors(), api.Metrics{}, Filter{}, 0)

	group := groupNamed(t, report, "config-map-missing")
	if group.Skipped == "" {
		t.Fatal("a reference check ran on a workload-only audit")
	}
}

func TestTheFactChecksSurviveAWorkloadOnlyAudit(t *testing.T) {
	report := Run(t.Context(), newLister(), descriptors(), api.Metrics{}, Filter{}, 0)

	if group := groupNamed(t, report, "node-selector-matches-nothing"); group.Skipped != "" {
		t.Fatalf("a cluster-fact check was skipped without the wider corpus: %q", group.Skipped)
	}
}

// a listing the apiserver refused

func TestACheckIsSkippedWhenItsKindCouldNotBeListed(t *testing.T) {
	lister := newLister(labelledDeployment("api", podSpec(sourcedContainer(map[string]any{
		"envFrom": []any{map[string]any{"secretRef": map[string]any{"name": "creds"}}},
	}))))
	lister.errs["secrets"] = errors.New("secrets is forbidden")

	report := Run(t.Context(), lister, descriptors(), api.Metrics{}, wholeCluster(), 0)

	group := groupNamed(t, report, "secret-missing")
	if group.Skipped == "" {
		t.Fatal("a check ran against a kind the apiserver refused to list")
	}
	if group.Total != 0 {
		t.Fatalf("a refused listing produced %d findings", group.Total)
	}
	if !strings.Contains(report.Error, "secrets") {
		t.Fatalf("the report did not name the listing that failed: %q", report.Error)
	}
}

func TestARefusedListingDoesNotSilenceUnrelatedChecks(t *testing.T) {
	lister := newLister(deployment("api", podSpec(container("app", map[string]any{"image": "busybox"}))))
	lister.errs["secrets"] = errors.New("secrets is forbidden")

	report := Run(t.Context(), lister, descriptors(), api.Metrics{}, wholeCluster(), 0)

	if groupNamed(t, report, "image-latest").Total != 1 {
		t.Fatal("a refused Secret listing silenced a check that never needed one")
	}
}

// what nothing-references-this needs before it may say so

func TestTheOrphanChecksWaitUntilTheCustomResourcesAreRead(t *testing.T) {
	lister := newLister(configMap("nobody-names-me", map[string]any{"a": "b"}))

	narrow := Run(t.Context(), lister, descriptors(), api.Metrics{}, Filter{}, 0)
	group := groupNamed(t, narrow, "orphaned-config-map")
	if group.Skipped == "" {
		t.Fatal("an orphan check answered from an audit that only read the workloads")
	}
	if !strings.Contains(group.Skipped, "every kind") {
		t.Fatalf("skipped said %q, want it to name what it is waiting for", group.Skipped)
	}
	if group.Total != 0 {
		t.Fatalf("a skipped orphan check still reported %d findings", group.Total)
	}

	whole := Run(t.Context(), lister, descriptors(), api.Metrics{}, Filter{WholeCluster: true}, 0)
	if groupNamed(t, whole, "orphaned-config-map").Total != 1 {
		t.Fatal("the orphan check stayed quiet on an audit of the whole cluster")
	}
}

func TestTheOrphanChecksSayTheyReadTheCustomResources(t *testing.T) {
	report := Run(t.Context(), newLister(configMap("nobody-names-me", map[string]any{"a": "b"})),
		descriptors(), api.Metrics{}, Filter{WholeCluster: true}, 0)

	if wrong := groupNamed(t, report, "orphaned-config-map").Wrong; !strings.Contains(wrong, "custom resource") {
		t.Fatalf("the check does not say what it read: %q", wrong)
	}
}

func TestAnExhaustiveReadAsksForEveryKindDiscoveryReported(t *testing.T) {
	if len(everyDiscovered(descriptors())) != len(descriptors()) {
		t.Fatal("the exhaustive read left a discovered kind out")
	}
}

func TestAPageOfASkippedOrphanCheckIsEmpty(t *testing.T) {
	lister := newLister(configMap("nobody-names-me", map[string]any{"a": "b"}))

	page, err := Page(t.Context(), lister, descriptors(), api.Metrics{},
		"orphaned-config-map", "", Filter{}, 0)
	if err != nil {
		t.Fatalf("page: %v", err)
	}
	if len(page.Findings) != 0 {
		t.Fatalf("a skipped orphan check paged %d findings", len(page.Findings))
	}
}
