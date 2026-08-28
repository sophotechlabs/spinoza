package checks

import (
	"context"

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

	noUsage = "metrics-server did not answer, so nothing is known about what these workloads actually use"
)

type scan struct {
	subjects []Subject
	usage    map[string]api.ResourceUsage
}

func (sc scan) hasUsage() bool {
	return len(sc.usage) > 0
}

type finder func(scan) []api.CheckFinding

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

func finding(subject Subject, container Container, detail, patch string) api.CheckFinding {
	return api.CheckFinding{
		Object:    subject.Ref,
		Kind:      subject.Kind,
		Container: container.Name,
		Detail:    detail,
		Patch:     patch,
	}
}

func overContainers(rule containerRule) finder {
	return func(sc scan) []api.CheckFinding {
		out := []api.CheckFinding{}
		for _, subject := range sc.subjects {
			for _, container := range subject.Containers {
				detail, patch := rule(subject, container)
				if detail == "" {
					continue
				}
				out = append(out, finding(subject, container, detail, patch))
			}
		}
		return out
	}
}

func overSubjects(rule subjectRule) finder {
	return func(sc scan) []api.CheckFinding {
		out := []api.CheckFinding{}
		for _, subject := range sc.subjects {
			detail, patch := rule(subject)
			if detail == "" {
				continue
			}
			out = append(out, finding(subject, Container{}, detail, patch))
		}
		return out
	}
}

func overUsage(rule usageRule) finder {
	return func(sc scan) []api.CheckFinding {
		out := []api.CheckFinding{}
		for _, subject := range sc.subjects {
			detail, patch := rule(subject, sc.usage)
			if detail == "" {
				continue
			}
			out = append(out, finding(subject, Container{}, detail, patch))
		}
		return out
	}
}

func (c check) group(sc scan) api.CheckGroup {
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
	out.Findings = c.find(sc)
	return out
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
	items, failure := gather(ctx, lister, needed(descs))
	sc := scan{subjects: subjectsOf(items), usage: usage.Pods}
	checks := registry()
	groups := make([]api.CheckGroup, 0, len(checks))
	for _, entry := range checks {
		groups = append(groups, entry.group(sc))
	}
	return api.CheckReport{Groups: groups, Scanned: len(sc.subjects), Error: failure}
}
