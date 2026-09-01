package resources

import (
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func eventColumns() []api.Column {
	return columnsFor(eventKind)
}

func eventRow(name, namespace, eventType, reason string) api.Row {
	return api.Row{
		Name:      name,
		Namespace: namespace,
		Cells:     []string{eventType, reason, "Pod/web", "1", "2026-08-20T12:00:00Z", "Readiness probe failed"},
	}
}

func TestAFilterMatchesTheColumnItNames(t *testing.T) {
	matcher := matcherFor(eventColumns(), []api.RowFilter{{Field: "type", Value: "Warning"}})

	if !matcher.matches(eventRow("a", "demo", "Warning", "Unhealthy")) {
		t.Fatal("a warning did not match type:Warning")
	}
	if matcher.matches(eventRow("b", "demo", "Normal", "Pulled")) {
		t.Fatal("a normal event matched type:Warning")
	}
}

func TestAFilterIgnoresCase(t *testing.T) {
	matcher := matcherFor(eventColumns(), []api.RowFilter{{Field: "TYPE", Value: "warning"}})

	if !matcher.matches(eventRow("a", "demo", "Warning", "Unhealthy")) {
		t.Fatal("a filter written in another case did not match")
	}
}

func TestAFilterMatchesPartOfACell(t *testing.T) {
	matcher := matcherFor(eventColumns(), []api.RowFilter{{Field: "reason", Value: "health"}})

	if !matcher.matches(eventRow("a", "demo", "Warning", "Unhealthy")) {
		t.Fatal("a filter did not match the middle of the cell")
	}
}

func TestAFilterReadsTheNameAndNamespaceThatAreNotCells(t *testing.T) {
	byName := matcherFor(eventColumns(), []api.RowFilter{{Field: "name", Value: "web"}})
	byNamespace := matcherFor(eventColumns(), []api.RowFilter{{Field: "namespace", Value: "demo"}})
	byAlias := matcherFor(eventColumns(), []api.RowFilter{{Field: "ns", Value: "demo"}})
	row := eventRow("web-0.18cc", "demo", "Warning", "Unhealthy")

	if !byName.matches(row) || !byNamespace.matches(row) || !byAlias.matches(row) {
		t.Fatal("name, namespace or the ns alias did not match")
	}
	if byNamespace.matches(eventRow("web-0.18cc", "other", "Warning", "Unhealthy")) {
		t.Fatal("a row in another namespace matched")
	}
}

func TestEveryFilterHasToMatch(t *testing.T) {
	matcher := matcherFor(eventColumns(), []api.RowFilter{
		{Field: "type", Value: "Warning"},
		{Field: "reason", Value: "BackOff"},
	})

	if matcher.matches(eventRow("a", "demo", "Warning", "Unhealthy")) {
		t.Fatal("a row matching only one of two filters was kept")
	}
	if !matcher.matches(eventRow("b", "demo", "Warning", "BackoffLimitExceeded")) {
		t.Fatal("a row matching both filters was dropped")
	}
}

func TestAFilterOnAColumnThatIsNotThereIsIgnored(t *testing.T) {
	matcher := matcherFor(eventColumns(), []api.RowFilter{{Field: "phase", Value: "Running"}})

	if !matcher.matches(eventRow("a", "demo", "Normal", "Pulled")) {
		t.Fatal("a filter naming an unknown column threw the row away; the browser ignores it")
	}
}

func TestARowShorterThanItsColumnsDoesNotPanic(t *testing.T) {
	matcher := matcherFor(eventColumns(), []api.RowFilter{{Field: "message", Value: "probe"}})

	if matcher.matches(api.Row{Name: "a", Cells: []string{"Warning"}}) {
		t.Fatal("a row with missing cells matched on a cell it does not have")
	}
}

func TestNoFiltersMeansNoFiltering(t *testing.T) {
	matcher := matcherFor(eventColumns(), nil)

	if matcher.wanted() {
		t.Fatal("an empty filter list asked for filtering")
	}
	if !matcher.matches(eventRow("a", "demo", "Normal", "Pulled")) {
		t.Fatal("a row was dropped with no filters at all")
	}
}

func TestBlankAndDuplicateColumnNamesDoNotDisplaceTheFirstColumn(t *testing.T) {
	columns := []api.Column{
		{Name: "---"},
		{Name: "Status"},
		{Name: "status"},
	}
	matcher := matcherFor(columns, []api.RowFilter{{Field: "status", Value: "running"}})

	if len(matcher.cells) != 1 || matcher.cells["status"] != 1 {
		t.Fatalf("column index = %v, want the first named Status column", matcher.cells)
	}
	if !matcher.matches(api.Row{Cells: []string{"ignored", "Running", "Stopped"}}) {
		t.Fatal("a duplicate label displaced the first matching column")
	}
}

func TestTheFieldKeyMatchesTheOneTheBrowserBuilds(t *testing.T) {
	cases := map[string]string{
		"Last seen":  "lastseen",
		"Type":       "type",
		"Cluster-IP": "clusterip",
		"Up-to-date": "uptodate",
	}
	for label, want := range cases {
		if fieldKey(label) != want {
			t.Fatalf("fieldKey(%q) = %q, want %q", label, fieldKey(label), want)
		}
	}
}
