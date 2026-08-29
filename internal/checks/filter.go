package checks

import (
	"slices"
	"strings"
)

const (
	SeverityLow    = severityLow
	SeverityMedium = severityMedium
	SeverityHigh   = severityHigh
)

var severityOrder = []string{severityLow, severityMedium, severityHigh}

type Filter struct {
	Disabled       []string
	SkipNamespaces []string
	MinSeverity    string
	WholeCluster   bool
}

func ParseFilter(disabled, namespaces, minSeverity, wholeCluster string) Filter {
	return Filter{
		Disabled:       splitList(disabled),
		SkipNamespaces: splitList(namespaces),
		MinSeverity:    knownSeverity(minSeverity),
		WholeCluster:   !narrowed(wholeCluster),
	}
}

// Absence means the whole cluster: a caller that says nothing gets every check.
// Narrowing to workloads is the thing you have to ask for.
func narrowed(raw string) bool {
	return raw == "0" || strings.EqualFold(raw, "false")
}

func splitList(raw string) []string {
	out := []string{}
	for part := range strings.SplitSeq(raw, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func knownSeverity(name string) string {
	if slices.Contains(severityOrder, name) {
		return name
	}
	return ""
}

func (f Filter) wants(entry check) bool {
	if slices.Contains(f.Disabled, entry.id) {
		return false
	}
	return f.severeEnough(entry.severity)
}

func (f Filter) severeEnough(severity string) bool {
	if f.MinSeverity == "" {
		return true
	}
	return slices.Index(severityOrder, severity) >= slices.Index(severityOrder, f.MinSeverity)
}

func (f Filter) keeps(item found) bool {
	return !slices.Contains(f.SkipNamespaces, item.subject.Ref.Namespace)
}

func (f Filter) chosen(all []check) []check {
	out := make([]check, 0, len(all))
	for _, entry := range all {
		if !f.wants(entry) {
			continue
		}
		out = append(out, entry)
	}
	return out
}
