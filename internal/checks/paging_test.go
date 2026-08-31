package checks

import (
	"errors"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const paged = "requests-missing"

func pageOf(t *testing.T, lister *fakeLister, after string) api.CheckPage {
	t.Helper()
	got, err := Page(t.Context(), lister, descriptors(), api.Metrics{}, paged, after, wholeCluster(), 0)
	if err != nil {
		t.Fatalf("page after %q: %v", after, err)
	}
	return got
}

func namesIn(t *testing.T, page api.CheckPage) []string {
	t.Helper()
	out := make([]string, 0, len(page.Findings))
	for _, finding := range page.Findings {
		if finding.Ref < 0 || finding.Ref >= len(page.Objects) {
			t.Fatalf("ref %d falls outside the %d objects the page carries", finding.Ref, len(page.Objects))
		}
		out = append(out, page.Objects[finding.Ref].Name)
	}
	return out
}

func walkEveryPage(t *testing.T, lister *fakeLister) []string {
	t.Helper()
	seen := []string{}
	after := ""
	for range 40 {
		page := pageOf(t, lister, after)
		seen = append(seen, namesIn(t, page)...)
		if page.Next == "" {
			return seen
		}
		after = page.Next
	}
	t.Fatal("paging never reached the end within 40 pages")
	return nil
}

func TestAPageStopsAtTheLimitAndOffersACursor(t *testing.T) {
	lister := newLister(manyDeployments(findingsShown + 40)...)

	first := pageOf(t, lister, "")

	if len(first.Findings) != findingsShown {
		t.Fatalf("first page carried %d findings, want %d", len(first.Findings), findingsShown)
	}
	if first.Next == "" {
		t.Fatal("a page that filled its limit offered no cursor")
	}
}

func TestTheLastPageOffersNoCursor(t *testing.T) {
	lister := newLister(manyDeployments(findingsShown + 40)...)

	second := pageOf(t, lister, pageOf(t, lister, "").Next)

	if len(second.Findings) != 40 {
		t.Fatalf("second page carried %d findings, want the remaining 40", len(second.Findings))
	}
	if second.Next != "" {
		t.Fatalf("the last page offered a cursor: %q", second.Next)
	}
}

func TestPagingReachesEveryFindingExactlyOnce(t *testing.T) {
	lister := newLister(manyDeployments(findingsShown*2 + 7)...)

	seen := walkEveryPage(t, lister)

	if len(seen) != findingsShown*2+7 {
		t.Fatalf("paging surfaced %d findings, want %d", len(seen), findingsShown*2+7)
	}
	once := map[string]bool{}
	for _, name := range seen {
		if once[name] {
			t.Fatalf("%s came back on more than one page", name)
		}
		once[name] = true
	}
}

func TestPagingKeepsTheOrderTheFirstPageStarted(t *testing.T) {
	lister := newLister(manyDeployments(findingsShown + 5)...)

	seen := walkEveryPage(t, lister)

	sorted := make([]string, len(seen))
	copy(sorted, seen)
	for at := 1; at < len(sorted); at++ {
		if sorted[at-1] >= sorted[at] {
			t.Fatalf("paging returned %s before %s", sorted[at-1], sorted[at])
		}
	}
}

func TestAFindingFixedMidWalkDoesNotShiftThePagesAfterIt(t *testing.T) {
	lister := newLister(manyDeployments(findingsShown + 10)...)
	first := pageOf(t, lister, "")
	names := namesIn(t, first)
	last := names[len(names)-1]

	held := lister.objects["deployments"]
	lister.objects["deployments"] = append(held[:0:0], held[3:]...)

	second := pageOf(t, lister, first.Next)

	for _, name := range namesIn(t, second) {
		if name <= last {
			t.Fatalf("%s came back after three findings before it were fixed", name)
		}
	}
	if len(second.Findings) != 10 {
		t.Fatalf("second page carried %d findings, want all 10 that follow the cursor; "+
			"offset paging would have skipped three here", len(second.Findings))
	}
}

func TestACursorForAFindingThatIsGoneStillPagesForward(t *testing.T) {
	lister := newLister(manyDeployments(findingsShown + 10)...)
	first := pageOf(t, lister, "")
	last := namesIn(t, first)[len(first.Findings)-1]

	lister.objects["deployments"] = lister.objects["deployments"][findingsShown:]

	second := pageOf(t, lister, first.Next)

	for _, name := range namesIn(t, second) {
		if name <= last {
			t.Fatalf("%s came back although the cursor had already passed it", name)
		}
	}
}

func TestAnUnknownCheckIsRefusedByName(t *testing.T) {
	_, err := Page(t.Context(), newLister(), descriptors(), api.Metrics{}, "not-a-check", "", wholeCluster(), 0)

	if !errors.Is(err, ErrNoSuchCheck) {
		t.Fatalf("error = %v, want ErrNoSuchCheck", err)
	}
	if err.Error() != "no check goes by that name" {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestACursorNobodyIssuedStartsFromTheBeginning(t *testing.T) {
	lister := newLister(manyDeployments(5)...)

	page := pageOf(t, lister, "!!!not base64!!!")

	if len(page.Findings) != 5 {
		t.Fatalf("an unreadable cursor returned %d findings, want all 5", len(page.Findings))
	}
}

func TestACheckWithNoUsageDataPagesToNothing(t *testing.T) {
	got, err := Page(
		t.Context(), newLister(manyDeployments(3)...), descriptors(), api.Metrics{},
		"requests-far-above-usage", "", wholeCluster(), 0,
	)
	if err != nil {
		t.Fatalf("page: %v", err)
	}

	if len(got.Findings) != 0 {
		t.Fatalf("a check with no metrics returned %d findings", len(got.Findings))
	}
	if got.Next != "" {
		t.Fatal("a check with no metrics offered a cursor")
	}
}

func TestTheReportCarriesTheCursorForACappedGroup(t *testing.T) {
	found := report(t, manyDeployments(findingsShown+40)...)
	group := groupNamed(t, found, paged)

	if group.Next == "" {
		t.Fatal("a capped group carried no cursor to continue from")
	}
	if !group.Truncated {
		t.Fatal("a capped group did not say it was truncated")
	}
	if group.Total != findingsShown+40 {
		t.Fatalf("total = %d, want %d", group.Total, findingsShown+40)
	}
}

func TestAGroupThatFitsCarriesNoCursor(t *testing.T) {
	found := report(t, manyDeployments(3)...)
	group := groupNamed(t, found, paged)

	if group.Next != "" {
		t.Fatalf("a complete group offered a cursor: %q", group.Next)
	}
	if group.Truncated {
		t.Fatal("a complete group claimed truncation")
	}
}

func TestTheReportsFirstPageAndTheEndpointsFirstPageAgree(t *testing.T) {
	objects := manyDeployments(findingsShown + 12)
	lister := newLister(objects...)
	fromReport := Run(t.Context(), lister, descriptors(), api.Metrics{}, wholeCluster(), 0)
	group := api.CheckGroup{}
	for _, one := range fromReport.Groups {
		if one.ID == paged {
			group = one
		}
	}

	fromEndpoint := pageOf(t, lister, "")

	if group.Next != fromEndpoint.Next {
		t.Fatalf("report cursor %q, endpoint cursor %q", group.Next, fromEndpoint.Next)
	}
	if len(group.Findings) != len(fromEndpoint.Findings) {
		t.Fatalf("report sent %d findings, endpoint sent %d", len(group.Findings), len(fromEndpoint.Findings))
	}
}

func TestTheFindingKeyOrdersBySubjectThenContainer(t *testing.T) {
	subject := Subject{Kind: "Deployment", Ref: api.ObjectRef{Namespace: "apps", Name: "api"}}
	cases := []struct {
		name  string
		left  found
		right found
	}{
		{
			name:  "a container sorts after the workload itself",
			left:  found{subject: subject},
			right: found{subject: subject, container: "app"},
		},
		{
			name:  "containers sort by name",
			left:  found{subject: subject, container: "app"},
			right: found{subject: subject, container: "sidecar"},
		},
		{
			name:  "namespaces sort before containers",
			left:  found{subject: Subject{Kind: "Deployment", Ref: api.ObjectRef{Namespace: "aaa", Name: "z"}}, container: "z"},
			right: found{subject: subject, container: "a"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if findingKey(tc.left) >= findingKey(tc.right) {
				t.Fatalf("%q did not sort before %q", findingKey(tc.left), findingKey(tc.right))
			}
		})
	}
}

func TestACursorRoundTripsThroughItsEncoding(t *testing.T) {
	cases := []string{"", "apps/Deployment/api\x00app", "ns/Pod/x"}
	for _, key := range cases {
		t.Run(key, func(t *testing.T) {
			if got := decodeCursor(encodeCursor(key)); got != key {
				t.Fatalf("round-tripped %q as %q", key, got)
			}
		})
	}
}

func TestACursorCarriesNothingAUrlWouldMangle(t *testing.T) {
	lister := newLister(manyDeployments(findingsShown + 1)...)

	cursor := pageOf(t, lister, "").Next

	if strings.ContainsAny(cursor, "/+=&? \x00") {
		t.Fatalf("cursor %q carries a character a query string would mangle", cursor)
	}
}
