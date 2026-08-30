package checks

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
)

var ErrNoSuchCheck = errors.New("no check goes by that name")

const (
	severityHigh   = "high"
	severityMedium = "medium"
	severityLow    = "low"

	categorySecurity    = "security"
	categoryReliability = "reliability"
	categoryEfficiency  = "efficiency"

	pssBaseline   = "PSS baseline"
	pssRestricted = "PSS restricted"
	nsaCisa       = "NSA/CISA"

	noUsage = "metrics-server did not answer, so usage is unknown"

	notEveryKind = "not audited: this decides what nothing references, " +
		"which is only true once every kind has been read"

	readsCustom = "Every built-in kind and every custom resource was read to decide this."

	findingsShown = 200
)

type scan struct {
	subjects  []Subject
	usage     map[string]api.ResourceUsage
	held      *corpus
	facts     Facts
	everyKind bool
	// custom says every kind outside the Kubernetes API groups was read, which
	// is what "nothing references this" needs before it can be said at all.
	custom bool
}

func (sc scan) hasUsage() bool {
	return len(sc.usage) > 0
}

type found struct {
	subject   Subject
	container string
	detail    string
	patch     string
	// convention is why this finding is not the leftover it looks like: an
	// owner, a manager, or something known to read it without naming it.
	convention string
	severity   string
}

type objects struct {
	index map[string]int
	list  []api.CheckObject
}

func newObjects() *objects {
	return &objects{index: map[string]int{}}
}

func (o *objects) ref(subject Subject) int {
	key := subject.Kind + "/" + subject.Ref.Namespace + "/" + subject.Ref.Name
	at, seen := o.index[key]
	if seen {
		return at
	}
	at = len(o.list)
	o.index[key] = at
	o.list = append(o.list, api.CheckObject{
		Group:     subject.Ref.Group,
		Version:   subject.Ref.Version,
		Resource:  subject.Ref.Resource,
		Namespace: subject.Ref.Namespace,
		Name:      subject.Ref.Name,
		Kind:      subject.Kind,
		Origin:    subject.Origin,
		ManagedBy: subject.ManagedBy,
	})
	return at
}

type finder func(scan) []found

type containerRule func(Subject, Container) (string, string)

type subjectRule func(Subject) (string, string)

type usageRule func(Subject, map[string]api.ResourceUsage) (string, string)

type factRule func(Subject, *corpus) (string, string)

type check struct {
	id         string
	title      string
	category   string
	severity   string
	frameworks []string
	wrong      string
	remedy     string
	needsUsage bool
	needsEvery bool
	// arguable says a reasonable operator could leave this as it is. Such a
	// check ships at low severity and says so in its own words, which the
	// registry test holds it to.
	arguable bool
	needs    []target
	find     finder
}

func overContainers(rule containerRule) finder {
	return func(sc scan) []found {
		out := []found{}
		for _, subject := range sc.subjects {
			for _, container := range subject.Containers {
				detail, patch := rule(subject, container)
				if detail == "" {
					continue
				}
				out = append(out, found{
					subject:   subject,
					container: container.Name,
					detail:    detail,
					patch:     patch,
				})
			}
		}
		return out
	}
}

func overSubjects(rule subjectRule) finder {
	return func(sc scan) []found {
		out := []found{}
		for _, subject := range sc.subjects {
			detail, patch := rule(subject)
			if detail == "" {
				continue
			}
			out = append(out, found{subject: subject, detail: detail, patch: patch})
		}
		return out
	}
}

func overFacts(rule factRule) finder {
	return func(sc scan) []found {
		out := []found{}
		for _, subject := range sc.subjects {
			detail, patch := rule(subject, sc.held)
			if detail == "" {
				continue
			}
			out = append(out, found{subject: subject, detail: detail, patch: patch})
		}
		return out
	}
}

func overUsage(rule usageRule) finder {
	return func(sc scan) []found {
		out := []found{}
		for _, subject := range sc.subjects {
			detail, patch := rule(subject, sc.usage)
			if detail == "" {
				continue
			}
			out = append(out, found{subject: subject, detail: detail, patch: patch})
		}
		return out
	}
}

func findingKey(item found) string {
	return rankOf(item.severity) + "\x00" + subjectKey(item.subject) + "\x00" + item.container
}

func encodeCursor(key string) string {
	if key == "" {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(key))
}

func decodeCursor(cursor string) string {
	if cursor == "" {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return ""
	}
	return string(raw)
}

// marked is a finding with what the filter decided about it: whether a mute
// covers it, and whether the baseline had seen it before.
type marked struct {
	found

	muted  bool
	by     Mute
	fresh  bool
	byRule bool
}

type tally struct {
	// found is everything the check produced in the part of the cluster being
	// looked at, before muting and before only-new. It is the number a baseline
	// count can honestly be compared against.
	found int
	total int
	muted int
	fresh int
	// here is every fingerprint this audit produced, so what the baseline holds
	// and this audit does not can be named.
	here map[string]bool
}

func (c check) matching(sc scan, keep Filter) ([]marked, tally) {
	all := c.ranked(c.find(sc))
	out := make([]marked, 0, len(all))
	count := tally{}
	here := map[string]bool{}
	for _, item := range all {
		if !keep.keeps(item) {
			continue
		}
		count.found++
		// Recorded before anything can drop the finding from the page: a muted
		// finding is still here, and a baseline told otherwise would report it
		// as work somebody did.
		key := c.id + "\x00" + fingerprintOf(identityOf(c.id, item))
		here[key] = true
		by, muted := keep.mutes(c.id, item)
		rule := false
		if !muted {
			by, muted = keep.silenced(c.id, item)
			rule = muted
		}
		if !muted && item.convention != "" {
			by, muted = Mute{Check: c.id, Reason: item.convention}, true
		}
		if muted {
			count.muted++
			if !keep.ShowMuted {
				continue
			}
		}
		fresh := c.comparable() && keep.Base.covers(c.id) && !keep.Base.has(key)
		if fresh {
			count.fresh++
		}
		if keep.OnlyNew && !fresh {
			continue
		}
		out = append(out, marked{found: item, muted: muted, by: by, fresh: fresh, byRule: rule})
	}
	count.total = len(out)
	count.here = here
	return out, count
}

func mutedBy(item marked) string {
	if !item.muted {
		return ""
	}
	if item.byRule {
		return ScopeRule
	}
	if item.convention != "" {
		return ScopeConvention
	}
	return scopeOf(item.by)
}

func page(all []marked, objs *objects, after string, limit int) ([]api.CheckFinding, string) {
	out := make([]api.CheckFinding, 0, min(limit, len(all)))
	last := ""
	for _, item := range all {
		key := findingKey(item.found)
		if key <= after {
			continue
		}
		if len(out) == limit {
			return out, encodeCursor(last)
		}
		out = append(out, api.CheckFinding{
			Ref:       objs.ref(item.subject),
			Container: item.container,
			Detail:    item.detail,
			Patch:     item.patch,
			Severity:  item.severity,
			New:       item.fresh,
			Muted:     item.muted,
			MutedBy:   mutedBy(item),
			Reason:    item.by.Reason,
		})
		last = key
	}
	return out, ""
}

// wantsCorpus says some check in this audit decides what nothing references,
// which is the only reason to read every custom resource in the cluster. With
// the orphan checks turned off or under the severity floor, reading a hundred
// custom kinds each time would buy nothing.
func wantsCorpus(keep Filter) bool {
	for _, entry := range keep.chosen(registryWith(keep.Rules)) {
		if entry.needsEvery {
			return true
		}
	}
	return false
}

// comparable says a finding of this check means the same thing on two different
// days. A check that reads live measurement does not: its findings appear and
// go with load, so calling them new would report the weather as work.
func (c check) comparable() bool {
	return !c.needsUsage
}

// standsDown is why a check reported nothing, which is never the same as
// finding nothing.
func (c check) standsDown(sc scan) string {
	if c.needsUsage && !sc.hasUsage() {
		return noUsage
	}
	if c.needsEvery {
		if !sc.custom {
			return notEveryKind
		}
		if refused := sc.held.refused(); refused != "" {
			return "not audited: the cluster would not let this read " + refused +
				", and what nothing references cannot be decided from part of it"
		}
	}
	if missing := missingResources(c.needs, sc.held); len(missing) > 0 {
		return skippedBecause(missing)
	}
	return ""
}

func (c check) group(sc scan, objs *objects, spread *namespaces, keep Filter, shown int) api.CheckGroup {
	out := api.CheckGroup{
		ID:         c.id,
		Title:      c.title,
		Category:   c.category,
		Severity:   c.severity,
		Frameworks: c.frameworks,
		Wrong:      c.wrong,
		Remedy:     c.remedy,
		Findings:   []api.CheckFinding{},
	}
	if down := c.standsDown(sc); down != "" {
		out.Skipped = down
		return out
	}
	all, count := c.matching(sc, keep)
	out.Findings, out.Next = page(all, objs, "", shown)
	out.Total, out.Muted, out.NewCount = count.total, count.muted, count.fresh
	if c.comparable() {
		out.Fixed = keep.fixedSince(c.id, count.found)
		out.Gone = keep.goneSince(c.id, count.here)
		out.Baselined = keep.Base.covers(c.id)
	}
	out.Measured = !c.comparable()
	out.Truncated = out.Next != ""
	spread.add(c.severity, all)
	return out
}

func joined(parts ...string) string {
	kept := []string{}
	for _, part := range parts {
		if part == "" {
			continue
		}
		kept = append(kept, part)
	}
	return strings.Join(kept, "; ")
}

func registryWith(rules []UserRule) []check {
	return append(registry(), userChecks(rules)...)
}

func registry() []check {
	out := securityChecks()
	out = append(out, hardeningChecks()...)
	out = append(out, reliabilityChecks()...)
	out = append(out, lifecycleChecks()...)
	out = append(out, supplyChecks()...)
	out = append(out, batchChecks()...)
	out = append(out, efficiencyChecks()...)
	out = append(out, factChecks()...)
	out = append(out, referenceChecks()...)
	out = append(out, rbacChecks()...)
	out = append(out, objectChecks()...)
	out = append(out, deprecationChecks()...)
	return out
}

func Page(
	ctx context.Context,
	lister Lister,
	descs map[string]api.ResourceDescriptor,
	usage api.Metrics,
	id string,
	after string,
	keep Filter,
	shown int,
) (api.CheckPage, error) {
	return (*Surveys)(nil).Page(ctx, lister, descs, usage, id, after, keep, shown)
}

func (s *Surveys) Page(
	ctx context.Context,
	lister Lister,
	descs map[string]api.ResourceDescriptor,
	usage api.Metrics,
	id string,
	after string,
	keep Filter,
	shown int,
) (api.CheckPage, error) {
	var wanted check
	for _, entry := range registryWith(keep.Rules) {
		if entry.id == id {
			wanted = entry
		}
	}
	if wanted.id == "" {
		return api.CheckPage{}, ErrNoSuchCheck
	}
	sc, _, _ := s.take(ctx, lister, descs, usage, keep)
	if wanted.standsDown(sc) != "" {
		return api.CheckPage{Findings: []api.CheckFinding{}, Objects: []api.CheckObject{}}, nil
	}
	objs := newObjects()
	all, _ := wanted.matching(sc, keep)
	found, next := page(all, objs, decodeCursor(after), pageSize(shown))
	return api.CheckPage{Findings: found, Objects: objs.list, Next: next}, nil
}

func pageSize(shown int) int {
	if shown <= 0 {
		return findingsShown
	}
	return shown
}

func survey(
	ctx context.Context,
	lister Lister,
	descs map[string]api.ResourceDescriptor,
	usage api.Metrics,
	keep Filter,
) (scan, string, []string) {
	wanted, absent := needed(descs, keep.WholeCluster)
	custom := false
	if keep.EveryKind {
		wanted, absent, custom = everyDiscovered(descs), nil, true
	}
	if keep.WholeCluster && !keep.EveryKind {
		if wantsCorpus(keep) {
			wanted = append(wanted, customKinds(descs)...)
			custom = true
		}
		wanted = append(wanted, alsoWarm(lister, wanted)...)
	}
	items, names, unread, mentions, failure := gather(ctx, lister, wanted)
	return scan{
		subjects:  subjectsOf(items),
		usage:     usage.Pods,
		held:      newCorpus(items, names, absent, targetsFor(keep.WholeCluster), unread, mentions),
		facts:     lister.Facts(),
		everyKind: keep.EveryKind,
		custom:    custom,
	}, failure, absent
}

func Run(
	ctx context.Context,
	lister Lister,
	descs map[string]api.ResourceDescriptor,
	usage api.Metrics,
	keep Filter,
	shown int,
) api.CheckReport {
	return (*Surveys)(nil).Run(ctx, lister, descs, usage, keep, shown)
}

func (s *Surveys) Run(
	ctx context.Context,
	lister Lister,
	descs map[string]api.ResourceDescriptor,
	usage api.Metrics,
	keep Filter,
	shown int,
) api.CheckReport {
	sc, failure, absent := s.take(ctx, lister, descs, usage, keep)
	checks := keep.chosen(registryWith(keep.Rules))
	objs := newObjects()
	spread := newNamespaces()
	groups := make([]api.CheckGroup, 0, len(checks))
	for _, entry := range checks {
		if ctx.Err() != nil {
			return api.CheckReport{
				Groups:  []api.CheckGroup{},
				Objects: []api.CheckObject{},
				Error:   "the audit was stopped before it finished",
			}
		}
		groups = append(groups, entry.group(sc, objs, spread, keep, pageSize(shown)))
	}
	return api.CheckReport{
		Groups:       groups,
		Objects:      objs.list,
		Namespaces:   spread.sorted(),
		Baseline:     keep.takenAt(),
		BaselineFrom: keep.takenFrom(),
		Scanned:      len(sc.subjects),
		Error:        joined(failure, undiscovered(absent)),
	}
}
