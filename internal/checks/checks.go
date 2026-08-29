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

	findingsShown = 200
)

type scan struct {
	subjects []Subject
	usage    map[string]api.ResourceUsage
	held     *corpus
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

func (c check) slice(sc scan, objs *objects, after string, limit int) (
	[]api.CheckFinding, string, int,
) {
	all := c.find(sc)
	out := make([]api.CheckFinding, 0, min(limit, len(all)))
	last := ""
	for _, item := range all {
		key := findingKey(item)
		if key <= after {
			continue
		}
		if len(out) == limit {
			return out, encodeCursor(last), len(all)
		}
		out = append(out, api.CheckFinding{
			Ref:       objs.ref(item.subject),
			Container: item.container,
			Detail:    item.detail,
			Patch:     item.patch,
		})
		last = key
	}
	return out, "", len(all)
}

func (c check) group(sc scan, objs *objects) api.CheckGroup {
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
	if c.needsUsage && !sc.hasUsage() {
		out.Skipped = noUsage
		return out
	}
	if missing := missingResources(c.needs, sc.held); len(missing) > 0 {
		out.Skipped = skippedBecause(missing)
		return out
	}
	out.Findings, out.Next, out.Total = c.slice(sc, objs, "", findingsShown)
	out.Truncated = out.Next != ""
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
	return out
}

func Page(
	ctx context.Context,
	lister Lister,
	descs map[string]api.ResourceDescriptor,
	usage api.Metrics,
	id string,
	after string,
) (api.CheckPage, error) {
	var wanted check
	for _, entry := range registry() {
		if entry.id == id {
			wanted = entry
		}
	}
	if wanted.id == "" {
		return api.CheckPage{}, ErrNoSuchCheck
	}
	sc, _, _ := survey(ctx, lister, descs, usage)
	if wanted.needsUsage && !sc.hasUsage() {
		return api.CheckPage{Findings: []api.CheckFinding{}, Objects: []api.CheckObject{}}, nil
	}
	if len(missingResources(wanted.needs, sc.held)) > 0 {
		return api.CheckPage{Findings: []api.CheckFinding{}, Objects: []api.CheckObject{}}, nil
	}
	objs := newObjects()
	found, next, _ := wanted.slice(sc, objs, decodeCursor(after), findingsShown)
	return api.CheckPage{Findings: found, Objects: objs.list, Next: next}, nil
}

func survey(
	ctx context.Context,
	lister Lister,
	descs map[string]api.ResourceDescriptor,
	usage api.Metrics,
) (scan, string, []string) {
	wanted, absent := needed(descs)
	items, names, failure := gather(ctx, lister, wanted)
	return scan{
		subjects: subjectsOf(items),
		usage:    usage.Pods,
		held:     newCorpus(items, names, absent),
	}, failure, absent
}

func Run(
	ctx context.Context,
	lister Lister,
	descs map[string]api.ResourceDescriptor,
	usage api.Metrics,
) api.CheckReport {
	sc, failure, absent := survey(ctx, lister, descs, usage)
	checks := registry()
	objs := newObjects()
	groups := make([]api.CheckGroup, 0, len(checks))
	for _, entry := range checks {
		groups = append(groups, entry.group(sc, objs))
	}
	return api.CheckReport{
		Groups:  groups,
		Objects: objs.list,
		Scanned: len(sc.subjects),
		Error:   joined(failure, undiscovered(absent)),
	}
}
