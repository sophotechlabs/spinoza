package checks

import (
	"net/url"
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
	Rules          []UserRule
	Silencers      []UserRule
	Mutes          []Mute
	Base           *Baseline
	Disabled       []string
	SkipNamespaces []string
	Namespace      string
	MinSeverity    string
	WholeCluster   bool
	EveryKind      bool
	OnlyNew        bool
	ShowMuted      bool
}

func ParseFilter(query url.Values) Filter {
	return Filter{
		Disabled:       splitList(query.Get("disabled")),
		SkipNamespaces: splitList(query.Get("skipNamespaces")),
		Namespace:      strings.TrimSpace(query.Get("namespace")),
		MinSeverity:    knownSeverity(query.Get("minSeverity")),
		WholeCluster:   !narrowed(query.Get("wholeCluster")),
		EveryKind:      asked(query.Get("everyKind")),
		OnlyNew:        asked(query.Get("onlyNew")),
		ShowMuted:      asked(query.Get("showMuted")),
	}
}

// narrows says the audit is looking at part of the cluster. A count taken from
// part of it cannot be subtracted from a baseline taken over all of it: the
// difference is the filter, not work anybody did.
func (f Filter) narrows() bool {
	return f.Namespace != "" || len(f.SkipNamespaces) > 0 || f.OnlyNew || !f.WholeCluster
}

func (f Filter) fixedSince(id string, found int) int {
	if f.narrows() {
		return 0
	}
	return f.Base.fixed(id, found)
}

func (f Filter) goneSince(id string, here map[string]bool) []string {
	if f.narrows() {
		return nil
	}
	return f.Base.gone(id, here)
}

func (f Filter) takenAt() string {
	if f.Base == nil {
		return ""
	}
	return f.Base.TakenAt
}

// takenFrom names the cluster the baseline came from, which the server leaves
// set only when it is not the one being audited.
func (f Filter) takenFrom() string {
	if f.Base == nil {
		return ""
	}
	return f.Base.Cluster
}

func asked(raw string) bool {
	return raw == "1" || strings.EqualFold(raw, "true")
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
	if slices.Contains(f.SkipNamespaces, item.subject.Ref.Namespace) {
		return false
	}
	if f.Namespace == "" {
		return true
	}
	return f.Namespace == item.subject.Ref.Namespace
}

// silenced answers whether a rule of your own quietens this finding, and what
// the rule said. A rule that errors on one object leaves it alone rather than
// taking the audit down.
func (f Filter) silenced(id string, item found) (Mute, bool) {
	for _, one := range f.Silencers {
		if one.Silences != id {
			continue
		}
		if !one.matches(item.subject) {
			continue
		}
		if !one.holds(item.subject) {
			continue
		}
		return Mute{Check: id, Reason: one.Reason}, true
	}
	return Mute{}, false
}

// mutes answers whether you have already decided about this finding, and what
// you said at the time.
func (f Filter) mutes(id string, item found) (Mute, bool) {
	for _, one := range f.Mutes {
		if silences(one, id, item) {
			return one, true
		}
	}
	return Mute{}, false
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
