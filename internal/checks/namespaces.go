package checks

import (
	"slices"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
)

type namespaces struct {
	index map[string]int
	list  []api.NamespaceCount
}

func newNamespaces() *namespaces {
	return &namespaces{index: map[string]int{}}
}

func (n *namespaces) add(all []marked) {
	for _, item := range all {
		space := item.subject.Ref.Namespace
		if space == "" {
			continue
		}
		n.count(space, item.severity)
	}
}

func (n *namespaces) count(space, severity string) {
	at, seen := n.index[space]
	if !seen {
		at = len(n.list)
		n.index[space] = at
		n.list = append(n.list, api.NamespaceCount{Namespace: space})
	}
	entry := &n.list[at]
	entry.Total++
	switch severity {
	case severityHigh:
		entry.High++
	case severityMedium:
		entry.Medium++
	default:
		entry.Low++
	}
}

func (n *namespaces) sorted() []api.NamespaceCount {
	out := slices.Clone(n.list)
	slices.SortFunc(out, func(left, right api.NamespaceCount) int {
		if left.High != right.High {
			return right.High - left.High
		}
		if left.Total != right.Total {
			return right.Total - left.Total
		}
		return strings.Compare(left.Namespace, right.Namespace)
	})
	return out
}
