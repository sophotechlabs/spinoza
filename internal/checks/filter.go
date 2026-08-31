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

func (f Filter) narrows() bool {
	if f.MinSeverity != "" {
		return true
	}
	return f.Namespace != "" || len(f.SkipNamespaces) > 0 || f.OnlyNew || !f.WholeCluster
}

func (f Filter) fixedSince(id string, found int) int {
	if f.narrows() || f.foreign() {
		return 0
	}
	return f.Base.fixed(id, found)
}

func (f Filter) goneSince(id string, here map[string]bool) []string {
	if f.narrows() || f.foreign() {
		return nil
	}
	return f.Base.gone(id, here)
}

func (f Filter) countedBefore(id string) (int, bool) {
	if f.narrows() {
		return 0, false
	}
	if f.Base == nil {
		return 0, false
	}
	return f.Base.counted(id)
}

func (f Filter) scannedBefore() int {
	if f.Base == nil {
		return 0
	}
	return f.Base.Scanned
}

func (f Filter) takenAt() string {
	if f.Base == nil {
		return ""
	}
	return f.Base.TakenAt
}

func (f Filter) foreign() bool {
	return f.takenFrom() != ""
}

func (f Filter) takenFrom() string {
	if f.Base == nil {
		return ""
	}
	return f.Base.Cluster
}

func asked(raw string) bool {
	return raw == "1" || strings.EqualFold(raw, "true")
}

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
	return f.couldReach(entry.severity)
}

func (f Filter) couldReach(severity string) bool {
	if f.MinSeverity == "" {
		return true
	}
	return highestWeight(severity) >= baseWeight(f.MinSeverity)
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
	if !f.severeEnough(item.severity) {
		return false
	}
	if f.Namespace == "" {
		return true
	}
	return f.Namespace == item.subject.Ref.Namespace
}

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
