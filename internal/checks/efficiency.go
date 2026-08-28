package checks

import (
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	overprovisionRatio = 4
	bytesPerMi         = 1024 * 1024
	requests           = "requests"
	limits             = "limits"
	cpuName            = "cpu"
	memoryName         = "memory"
)

func efficiencyChecks() []check {
	return []check{
		{
			id:       "requests-missing",
			title:    "No resource requests",
			category: categoryEfficiency,
			severity: severityMedium,
			wrong:    "The scheduler places it as if it needed nothing, so the node fills past what it can serve.",
			remedy:   "Set resources.requests.cpu and resources.requests.memory to what it needs at rest.",
			find:     overContainers(requestsMissing),
		},
		{
			id:       "limits-missing",
			title:    "No resource limits",
			category: categoryEfficiency,
			severity: severityLow,
			wrong:    "One runaway process can take the node and evict everything on it.",
			remedy:   "Set resources.limits.memory at minimum; that is what stops a leak.",
			find:     overContainers(limitsMissing),
		},
		{
			id:       "limits-far-above-requests",
			title:    "Limits far above requests",
			category: categoryEfficiency,
			severity: severityLow,
			wrong:    "The node was scheduled for the request and can be asked for the limit.",
			remedy:   "Raise the request to what it uses, or lower the limit.",
			find:     overContainers(limitsAboveRequests),
		},
		{
			id:         "requests-far-above-usage",
			title:      "Requests far above measured usage",
			category:   categoryEfficiency,
			severity:   severityLow,
			needsUsage: true,
			wrong:      "Reserved capacity nothing uses: the cluster looks full while nodes sit idle.",
			remedy:     "Lower the requests towards measured usage, leaving headroom for peaks.",
			find:       overUsage(requestsAboveUsage),
		},
	}
}

func quantityAt(spec map[string]any, section, name string) (resource.Quantity, bool) {
	resources, ok := spec["resources"].(map[string]any)
	if !ok {
		return resource.Quantity{}, false
	}
	part, ok := resources[section].(map[string]any)
	if !ok {
		return resource.Quantity{}, false
	}
	return quantityFrom(part[name])
}

func quantityFrom(raw any) (resource.Quantity, bool) {
	switch value := raw.(type) {
	case string:
		return parsed(value)
	case int64:
		return parsed(strconv.FormatInt(value, 10))
	case float64:
		return parsed(strconv.FormatFloat(value, 'f', -1, 64))
	default:
		return resource.Quantity{}, false
	}
}

func parsed(raw string) (resource.Quantity, bool) {
	value, err := resource.ParseQuantity(raw)
	if err != nil {
		return resource.Quantity{}, false
	}
	return value, true
}

func missingIn(container Container, section string) []string {
	out := []string{}
	for _, name := range []string{cpuName, memoryName} {
		_, ok := quantityAt(container.Spec, section, name)
		if !ok {
			out = append(out, name)
		}
	}
	return out
}

func requestsMissing(_ Subject, container Container) (string, string) {
	missing := missingIn(container, requests)
	if len(missing) == 0 {
		return "", ""
	}
	return "no " + strings.Join(missing, " or ") + " request", ""
}

func limitsMissing(_ Subject, container Container) (string, string) {
	missing := missingIn(container, limits)
	if len(missing) == 0 {
		return "", ""
	}
	return "no " + strings.Join(missing, " or ") + " limit", ""
}

func limitsAboveRequests(_ Subject, container Container) (string, string) {
	cpu := ratioDetail(container, cpuName, milliOf)
	if cpu != "" {
		return cpu, ""
	}
	return ratioDetail(container, memoryName, plainOf), ""
}

func milliOf(value resource.Quantity) int64 {
	return value.MilliValue()
}

func plainOf(value resource.Quantity) int64 {
	return value.Value()
}

func ratioDetail(container Container, name string, scale func(resource.Quantity) int64) string {
	request, hasRequest := quantityAt(container.Spec, requests, name)
	limit, hasLimit := quantityAt(container.Spec, limits, name)
	if !hasRequest || !hasLimit {
		return ""
	}
	asked := scale(request)
	allowed := scale(limit)
	if asked <= 0 {
		return ""
	}
	ratio := allowed / asked
	if ratio < overprovisionRatio {
		return ""
	}
	return name + " limit " + limit.String() + " is " + strconv.FormatInt(ratio, 10) +
		"x the " + request.String() + " request"
}

func requestsAboveUsage(subject Subject, usage map[string]api.ResourceUsage) (string, string) {
	measured, ok := meanUsage(subject, usage)
	if !ok {
		return "", ""
	}
	cpu := askedFor(subject, cpuName, milliOf)
	if overprovisioned(cpu, measured.CPUMilli) {
		return usageDetail("cpu", cpu, measured.CPUMilli, "m"), ""
	}
	memory := askedFor(subject, memoryName, plainOf) / bytesPerMi
	if overprovisioned(memory, measured.MemoryMi) {
		return usageDetail(memoryName, memory, measured.MemoryMi, "Mi"), ""
	}
	return "", ""
}

func overprovisioned(asked, used int64) bool {
	if used <= 0 {
		return false
	}
	return asked/used >= overprovisionRatio
}

func usageDetail(name string, asked, used int64, unit string) string {
	return "pods request " + strconv.FormatInt(asked, 10) + unit + " " + name +
		" and use " + strconv.FormatInt(used, 10) + unit
}

func askedFor(subject Subject, name string, scale func(resource.Quantity) int64) int64 {
	var total int64
	for _, container := range subject.Containers {
		if container.Init {
			continue
		}
		value, ok := quantityAt(container.Spec, requests, name)
		if !ok {
			continue
		}
		total += scale(value)
	}
	return total
}

func meanUsage(subject Subject, usage map[string]api.ResourceUsage) (api.ResourceUsage, bool) {
	var total api.ResourceUsage
	var seen int64
	for _, pod := range subject.Pods {
		found, ok := usage[subject.Ref.Namespace+"/"+pod.Name]
		if !ok {
			continue
		}
		total.CPUMilli += found.CPUMilli
		total.MemoryMi += found.MemoryMi
		seen++
	}
	if seen == 0 {
		return api.ResourceUsage{}, false
	}
	return api.ResourceUsage{CPUMilli: total.CPUMilli / seen, MemoryMi: total.MemoryMi / seen}, true
}
