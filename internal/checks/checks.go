package checks

import (
	"context"
	"strings"

	"github.com/sophotechlabs/spinoza/internal/api"
)

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
	})
	return at
}

type finder func(scan) []found

type containerRule func(Subject, Container) (string, string)

type subjectRule func(Subject) (string, string)

type usageRule func(Subject, map[string]api.ResourceUsage) (string, string)

type check struct {
	id         string
	title      string
	category   string
	severity   string
	frameworks []string
	wrong      string
	remedy     string
	needsUsage bool
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
	all := c.find(sc)
	out.Total = len(all)
	if len(all) > findingsShown {
		all = all[:findingsShown]
		out.Truncated = true
	}
	out.Findings = make([]api.CheckFinding, 0, len(all))
	for _, item := range all {
		out.Findings = append(out.Findings, api.CheckFinding{
			Ref:       objs.ref(item.subject),
			Container: item.container,
			Detail:    item.detail,
			Patch:     item.patch,
		})
	}
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
	out = append(out, reliabilityChecks()...)
	out = append(out, efficiencyChecks()...)
	return out
}

func Run(
	ctx context.Context,
	lister Lister,
	descs map[string]api.ResourceDescriptor,
	usage api.Metrics,
) api.CheckReport {
	wanted, absent := needed(descs)
	items, failure := gather(ctx, lister, wanted)
	sc := scan{subjects: subjectsOf(items), usage: usage.Pods}
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
