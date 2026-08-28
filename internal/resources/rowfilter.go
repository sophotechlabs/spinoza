package resources

import (
	"regexp"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	nameField      = "name"
	namespaceField = "namespace"
	namespaceAlias = "ns"
)

var notAlphanumeric = regexp.MustCompile(`[^a-z0-9]`)

// Matches the frontend's key, and filters the cache rather than the rows on
// screen: the only way to find anything past the newest few.
func fieldKey(label string) string {
	return notAlphanumeric.ReplaceAllString(strings.ToLower(label), "")
}

type rowMatcher struct {
	filters []api.RowFilter
	cells   map[string]int
}

func matcherFor(columns []api.Column, filters []api.RowFilter) rowMatcher {
	cells := map[string]int{}
	for index, column := range columns {
		key := fieldKey(column.Name)
		if key == "" {
			continue
		}
		_, taken := cells[key]
		if taken {
			continue
		}
		cells[key] = index
	}
	return rowMatcher{filters: filters, cells: cells}
}

func (m rowMatcher) wanted() bool {
	return len(m.filters) > 0
}

func (m rowMatcher) matches(row api.Row) bool {
	for _, filter := range m.filters {
		if !m.matchesOne(row, filter) {
			return false
		}
	}
	return true
}

func (m rowMatcher) matchesOne(row api.Row, filter api.RowFilter) bool {
	found, known := m.valueOf(row, fieldKey(filter.Field))
	if !known {
		return true
	}
	return strings.Contains(strings.ToLower(found), strings.ToLower(filter.Value))
}

func (m rowMatcher) valueOf(row api.Row, key string) (string, bool) {
	if key == nameField {
		return row.Name, true
	}
	if key == namespaceField || key == namespaceAlias {
		return row.Namespace, true
	}
	index, ok := m.cells[key]
	if !ok {
		return "", false
	}
	if index >= len(row.Cells) {
		return "", true
	}
	return row.Cells[index], true
}
