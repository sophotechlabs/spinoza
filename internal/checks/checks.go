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

	findingsShown = 200
)

type scan struct {
	subjects  []Subject
	usage     map[string]api.ResourceUsage
	held      *corpus
	facts     Facts
	everyKind bool
}

func (sc scan) hasUsage() bool {
	return len(sc.usage) > 0
}

type found struct {
	subject   Subject
	container string
	detail    string
	patch     string
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
	needs      []target
	find       finder
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
	return subjectKey(item.subject) + "\x00" + item.container
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

	muted bool
	by    Mute
	fresh bool
}

type tally struct {
	// found is everything the check produced in the part of the cluster being
	// looked at, before muting and before only-new. It is the number a baseline
	// count can honestly be compared against.
	found int
	total int
	muted int
	fresh int
}

func (c check) matching(sc scan, keep Filter) ([]marked, tally) {
	all := c.find(sc)
	out := make([]marked, 0, len(all))
	count := tally{}
	for _, item := range all {
		if !keep.keeps(item) {
			continue
		}
		count.found++
		by, muted := keep.mutes(c.id, item)
		if muted {
			count.muted++
			if !keep.ShowMuted {
				continue
			}
		}
		fresh := keep.Base.covers(c.id) && !keep.Base.has(fingerprintOf(identityOf(c.id, item)))
		if fresh {
			count.fresh++
		}
		if keep.OnlyNew && !fresh {
			continue
		}
		out = append(out, marked{found: item, muted: muted, by: by, fresh: fresh})
	}
	count.total = len(out)
	return out, count
}

func mutedBy(item marked) string {
	if !item.muted {
		return ""
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
			New:       item.fresh,
			Muted:     item.muted,
			MutedBy:   mutedBy(item),
			Reason:    item.by.Reason,
		})
		last = key
	}
	return out, ""
}

// standsDown is why a check reported nothing, which is never the same as
// finding nothing.
func (c check) standsDown(sc scan) string {
	if c.needsUsage && !sc.hasUsage() {
		return noUsage
	}
	if c.needsEvery && !sc.everyKind {
		return notEveryKind
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
	out.Fixed = keep.fixedSince(c.id, count.found)
	out.Baselined = keep.Base.covers(c.id)
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
	var wanted check
	for _, entry := range registryWith(keep.Rules) {
		if entry.id == id {
			wanted = entry
		}
	}
	if wanted.id == "" {
		return api.CheckPage{}, ErrNoSuchCheck
	}
	sc, _, _ := survey(ctx, lister, descs, usage, keep)
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
	if keep.EveryKind {
		wanted, absent = everyDiscovered(descs), nil
	}
	if keep.WholeCluster && !keep.EveryKind {
		wanted = append(wanted, alsoWarm(lister, wanted)...)
	}
	items, names, unread, failure := gather(ctx, lister, wanted)
	return scan{
		subjects:  subjectsOf(items),
		usage:     usage.Pods,
		held:      newCorpus(items, names, absent, targetsFor(keep.WholeCluster), unread),
		facts:     lister.Facts(),
		everyKind: keep.EveryKind,
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
	sc, failure, absent := survey(ctx, lister, descs, usage, keep)
	checks := keep.chosen(registryWith(keep.Rules))
	objs := newObjects()
	spread := newNamespaces()
	groups := make([]api.CheckGroup, 0, len(checks))
	for _, entry := range checks {
		groups = append(groups, entry.group(sc, objs, spread, keep, pageSize(shown)))
	}
	return api.CheckReport{
		Groups:     groups,
		Objects:    objs.list,
		Namespaces: spread.sorted(),
		Baseline:   keep.takenAt(),
		Scanned:    len(sc.subjects),
		Error:      joined(failure, undiscovered(absent)),
	}
}
