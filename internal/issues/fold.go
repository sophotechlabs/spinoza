package issues

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	maxRows     = 200
	maxChildren = 50
)

const (
	severityWarning = iota + 1
	severityDegraded
	severityFatal
)

type finding struct {
	detector  string
	severity  int
	title     string
	detail    string
	action    string
	change    string
	changedAt time.Time
	uncertain bool
	kind      string
	subject   object
	since     time.Time
}

func severityName(level int) string {
	switch level {
	case severityFatal:
		return api.SeverityFatal
	case severityDegraded:
		return api.SeverityDegraded
	default:
		return api.SeverityWarning
	}
}

func stamp(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.UTC().Format(time.RFC3339)
}

func fold(found []finding, snap *snapshot) api.IssueQueue {
	groups := map[string][]finding{}
	owners := map[string]object{}
	order := []string{}
	for _, item := range found {
		owner := snap.owner(item.subject)
		key := owner.uid()
		if _, seen := groups[key]; !seen {
			order = append(order, key)
			owners[key] = owner
		}
		groups[key] = append(groups[key], item)
	}
	rows := make([]api.Issue, 0, len(order))
	for _, key := range order {
		rows = append(rows, rowOf(owners[key], prune(groups[key])))
	}
	rank(rows)
	dropped := 0
	if len(rows) > maxRows {
		dropped = len(rows) - maxRows
		rows = rows[:maxRows]
	}
	return api.IssueQueue{Rows: rows, Dropped: dropped}
}

func prune(group []finding) []finding {
	explained := slices.ContainsFunc(group, func(item finding) bool {
		return item.kind == kindPod
	})
	if !explained {
		return group
	}
	return slices.DeleteFunc(slices.Clone(group), func(item finding) bool {
		return item.title == titleShortOfReplicas
	})
}

func rowOf(owner object, group []finding) api.Issue {
	lead := leadOf(group)
	children := childrenOf(owner, group)
	row := api.Issue{
		ID:        owner.uid(),
		Severity:  severityName(worst(group)),
		Detector:  lead.detector,
		Title:     lead.title,
		Detail:    detailOf(lead, children),
		Action:    lead.action,
		Change:    lead.change,
		ChangedAt: stamp(lead.changedAt),
		Uncertain: lead.uncertain,
		Object:    owner.ref(),
		Kind:      owner.desc.Kind,
		Since:     stamp(oldest(group)),
		Folded:    len(children),
	}
	if len(children) > maxChildren {
		row.Children = children[:maxChildren]
		return row
	}
	if len(children) > 0 {
		row.Children = children
	}
	return row
}

func detailOf(lead finding, children []api.IssueChild) string {
	if len(children) < 2 {
		return lead.detail
	}
	return lead.detail + " · " + strconv.Itoa(len(children)) + " " + plural(children) + " affected"
}

func plural(children []api.IssueChild) string {
	kind := children[0].Kind
	for _, child := range children[1:] {
		if child.Kind != kind {
			return "objects"
		}
	}
	if kind == "" {
		return "objects"
	}
	return lowerFirst(kind) + "s"
}

func lowerFirst(text string) string {
	if text == "" {
		return text
	}
	head := text[:1]
	lowered := []byte(head)[0]
	if lowered >= 'A' && lowered <= 'Z' {
		lowered += 'a' - 'A'
	}
	return string(lowered) + text[1:]
}

func leadOf(group []finding) finding {
	lead := group[0]
	for _, item := range group[1:] {
		if leads(item, lead) {
			lead = item
		}
	}
	return lead
}

func leads(item, lead finding) bool {
	if item.severity != lead.severity {
		return item.severity > lead.severity
	}
	if !item.since.Equal(lead.since) {
		return item.since.After(lead.since)
	}
	return whereOf(item.subject) < whereOf(lead.subject)
}

func whereOf(item object) string {
	return item.obj.GetNamespace() + "/" + item.obj.GetName()
}

func worst(group []finding) int {
	level := group[0].severity
	for _, item := range group[1:] {
		if item.severity > level {
			level = item.severity
		}
	}
	return level
}

func oldest(group []finding) time.Time {
	at := group[0].since
	for _, item := range group[1:] {
		if at.IsZero() || (!item.since.IsZero() && item.since.Before(at)) {
			at = item.since
		}
	}
	return at
}

func childrenOf(owner object, group []finding) []api.IssueChild {
	out := []api.IssueChild{}
	for _, item := range group {
		if item.subject.uid() == owner.uid() {
			continue
		}
		out = append(out, api.IssueChild{
			Object:   item.subject.ref(),
			Kind:     item.kind,
			Severity: severityName(item.severity),
			Detail:   item.detail,
			Since:    stamp(item.since),
		})
	}
	slices.SortStableFunc(out, func(left, right api.IssueChild) int {
		if !seenAt(left.Since).Equal(seenAt(right.Since)) {
			return seenAt(right.Since).Compare(seenAt(left.Since))
		}
		return strings.Compare(whereOfRef(left.Object), whereOfRef(right.Object))
	})
	return out
}

func whereOfRef(ref api.ObjectRef) string {
	return ref.Namespace + "/" + ref.Name
}

func rank(rows []api.Issue) {
	slices.SortStableFunc(rows, func(left, right api.Issue) int {
		if severityRank(left.Severity) != severityRank(right.Severity) {
			return severityRank(right.Severity) - severityRank(left.Severity)
		}
		if left.Folded != right.Folded {
			return right.Folded - left.Folded
		}
		if !seenAt(left.Since).Equal(seenAt(right.Since)) {
			return seenAt(right.Since).Compare(seenAt(left.Since))
		}
		return strings.Compare(left.ID, right.ID)
	})
}

func severityRank(name string) int {
	switch name {
	case api.SeverityFatal:
		return severityFatal
	case api.SeverityDegraded:
		return severityDegraded
	default:
		return severityWarning
	}
}

func seenAt(text string) time.Time {
	at, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return time.Time{}
	}
	return at
}
