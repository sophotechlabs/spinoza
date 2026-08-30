package checks

import (
	"slices"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
)

// namespaces counts findings per namespace as the audit produces them. The
// browser cannot work this out for itself: a group stops at the findings it
// shows, so anything counted there would be wrong for every capped check.
type namespaces struct {
	index map[string]int
	list  []api.NamespaceCount
}

func newNamespaces() *namespaces {
	return &namespaces{index: map[string]int{}}
}

func (n *namespaces) add(severity string, all []marked) {
	for _, item := range all {
		space := item.subject.Ref.Namespace
		if space == "" {
			continue
		}
		if item.muted {
			continue
		}
		n.count(space, severity)
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

// sorted puts the namespace carrying the most weight first, so the list reads
// as where to start.
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
