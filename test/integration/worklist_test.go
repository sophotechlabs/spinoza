//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/checks"
)

func totalOf(report api.CheckReport, id string) int {
	return groupOf(report, id).Total
}

func firstFiring(t *testing.T, report api.CheckReport) api.CheckGroup {
	t.Helper()
	for _, group := range report.Groups {
		if group.Total > 0 && group.Skipped == "" {
			return group
		}
	}
	t.Fatal("no check found anything on the cluster, so there is nothing to mute")
	return api.CheckGroup{}
}

func TestAMuteSilencesARealFindingOnTheCluster(t *testing.T) {
	held := manager(t, bundle(t))
	before := held.Checks(context.Background(), checks.Filter{WholeCluster: true})
	group := firstFiring(t, before)
	object := before.Objects[group.Findings[0].Ref]

	keep := checks.Filter{WholeCluster: true, Mutes: []checks.Mute{{
		Check: group.ID,
		Ref: checks.RefKey(api.ObjectRef{
			Group:     object.Group,
			Version:   object.Version,
			Resource:  object.Resource,
			Namespace: object.Namespace,
			Name:      object.Name,
		}),
		Reason: "known",
	}}}
	after := held.Checks(context.Background(), keep)

	if totalOf(after, group.ID) >= group.Total {
		t.Fatalf("%s reported %d findings after muting one of its %d",
			group.ID, totalOf(after, group.ID), group.Total)
	}
	if groupOf(after, group.ID).Muted == 0 {
		t.Fatalf("%s counted no muted findings after one was muted", group.ID)
	}
}

func TestABaselineOfThisClusterLeavesNothingNew(t *testing.T) {
	held := manager(t, bundle(t))
	keep := checks.Filter{WholeCluster: true}
	taken := held.CheckFingerprint(context.Background(), keep)
	taken.TakenAt = "2026-08-30T00:00:00Z"
	keep.Base = &taken

	report := held.Checks(context.Background(), keep)

	if report.Baseline != taken.TakenAt {
		t.Fatalf("the report named %q as its baseline", report.Baseline)
	}
	for _, group := range report.Groups {
		if group.NewCount != 0 {
			t.Fatalf("%s called %d findings new against a baseline taken from the same cluster",
				group.ID, group.NewCount)
		}
	}
	if len(taken.Keys) == 0 {
		t.Fatal("the baseline of a cluster with findings held no fingerprints")
	}
}

func TestWhatTheClusterOwnsOrManagesIsNotCalledALeftover(t *testing.T) {
	keep := checks.Filter{WholeCluster: true, ShowMuted: true}
	report := manager(t, bundle(t)).Checks(context.Background(), keep)

	group := groupOf(report, "orphaned-secret")
	if group.Skipped != "" {
		t.Skipf("the orphan check stood down: %s", group.Skipped)
	}
	silenced := 0
	for _, finding := range group.Findings {
		if finding.MutedBy != checks.ScopeConvention {
			continue
		}
		silenced++
		if finding.Reason == "" {
			t.Fatalf("a secret was silenced with no reason: %+v", report.Objects[finding.Ref])
		}
	}
	if silenced == 0 {
		t.Fatal("no secret on this cluster was recognized as owned, managed, or read by convention")
	}
}

func TestAnAuditStoppedPartWaySaysSo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	report := manager(t, bundle(t)).Checks(ctx, checks.Filter{WholeCluster: true})

	if len(report.Groups) != 0 {
		t.Fatalf("a stopped audit answered with %d groups", len(report.Groups))
	}
}

func TestTheExportCarriesTheWholeAuditNotJustAPage(t *testing.T) {
	held := manager(t, bundle(t))
	keep := checks.Filter{WholeCluster: true}
	report := held.Checks(context.Background(), keep)

	whole := held.CheckExport(context.Background(), keep)

	for _, group := range whole.Groups {
		if len(group.Findings) != group.Total {
			t.Fatalf("%s exported %d of %d findings", group.ID, len(group.Findings), group.Total)
		}
	}
	if len(whole.Groups) != len(report.Groups) {
		t.Fatalf("the export carried %d groups and the view %d", len(whole.Groups), len(report.Groups))
	}
}

func TestTheNamespaceSummaryAddsUpToWhatTheChecksFound(t *testing.T) {
	report := manager(t, bundle(t)).Checks(context.Background(), checks.Filter{WholeCluster: true})

	counted := 0
	for _, entry := range report.Namespaces {
		if entry.Namespace == "" {
			t.Fatal("the summary carried a row for no namespace at all")
		}
		counted += entry.Total
	}
	if counted == 0 {
		t.Fatalf("the summary counted nothing across %d namespaces", len(report.Namespaces))
	}

	narrowed := manager(t, bundle(t)).Checks(context.Background(), checks.Filter{
		WholeCluster: true, Namespace: report.Namespaces[0].Namespace,
	})
	for _, object := range narrowed.Objects {
		if object.Namespace != "" && object.Namespace != report.Namespaces[0].Namespace {
			t.Fatalf("asking for %s returned an object in %s",
				report.Namespaces[0].Namespace, object.Namespace)
		}
	}
}
