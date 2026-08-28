package checks

import (
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestMissingRequestsAndLimitsNameWhatIsAbsent(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", nil))))

	if onlyFinding(t, found, "requests-missing").Detail != "no cpu or memory request" {
		t.Fatal("a container with no requests was not reported")
	}
	if onlyFinding(t, found, "limits-missing").Detail != "no cpu or memory limit" {
		t.Fatal("a container with no limits was not reported")
	}
}

func TestOnlyTheAbsentResourceIsNamed(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", resources("requests", map[string]any{
		"cpu": "100m",
	})))))

	if onlyFinding(t, found, "requests-missing").Detail != "no memory request" {
		t.Fatal("a half-set request block was not reported precisely")
	}
}

func TestBothRequestsSetIsClean(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", resources("requests", map[string]any{
		"cpu":    "100m",
		"memory": "128Mi",
	})))))

	if findingCount(t, found, "requests-missing") != 0 {
		t.Fatal("a container with both requests was reported")
	}
}

func TestAQuantityMayArriveAsANumber(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", resources("requests", map[string]any{
		"cpu":    int64(1),
		"memory": float64(134217728),
	})))))

	if findingCount(t, found, "requests-missing") != 0 {
		t.Fatal("numeric quantities were read as missing")
	}
}

func TestAnUnparseableQuantityCountsAsMissing(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", resources("requests", map[string]any{
		"cpu":    "a hundred",
		"memory": true,
	})))))

	if onlyFinding(t, found, "requests-missing").Detail != "no cpu or memory request" {
		t.Fatal("an unreadable quantity was not treated as missing")
	}
}

func TestALimitFarAboveItsRequestIsFlagged(t *testing.T) {
	cpu := report(t, deployment("api", podSpec(container("app", requestsAndLimits(
		map[string]any{"cpu": "100m", "memory": "128Mi"},
		map[string]any{"cpu": "2", "memory": "256Mi"},
	)))))
	if onlyFinding(t, cpu, "limits-far-above-requests").Detail != "cpu limit 2 is 20x the 100m request" {
		t.Fatalf("detail was %q", onlyFinding(t, cpu, "limits-far-above-requests").Detail)
	}

	memory := report(t, deployment("api", podSpec(container("app", requestsAndLimits(
		map[string]any{"cpu": "100m", "memory": "128Mi"},
		map[string]any{"cpu": "200m", "memory": "2Gi"},
	)))))
	detail := onlyFinding(t, memory, "limits-far-above-requests").Detail
	if !strings.HasPrefix(detail, "memory limit 2Gi is 16x") {
		t.Fatalf("detail was %q", detail)
	}
}

func TestALimitCloseToItsRequestIsNotFlagged(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", requestsAndLimits(
		map[string]any{"cpu": "100m", "memory": "128Mi"},
		map[string]any{"cpu": "200m", "memory": "256Mi"},
	)))))

	if findingCount(t, found, "limits-far-above-requests") != 0 {
		t.Fatal("a limit twice its request was reported")
	}
}

func TestARatioNeedsBothSides(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", resources("limits", map[string]any{
		"cpu":    "2",
		"memory": "2Gi",
	})))))

	if findingCount(t, found, "limits-far-above-requests") != 0 {
		t.Fatal("a limit with no request produced a ratio")
	}
}

func TestAZeroRequestProducesNoRatio(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", requestsAndLimits(
		map[string]any{"cpu": "0", "memory": "0"},
		map[string]any{"cpu": "2", "memory": "2Gi"},
	)))))

	if findingCount(t, found, "limits-far-above-requests") != 0 {
		t.Fatal("a zero request produced a ratio")
	}
}

func usageFor(names []string, cpuMilli, memoryMi int64) map[string]api.ResourceUsage {
	out := map[string]api.ResourceUsage{}
	for _, name := range names {
		out[testNamespace+"/"+name] = api.ResourceUsage{CPUMilli: cpuMilli, MemoryMi: memoryMi}
	}
	return out
}

func TestRequestsFarAboveUsageAreFlagged(t *testing.T) {
	owner := deployment("api", podSpec(container("app", resources("requests", map[string]any{
		"cpu":    "2",
		"memory": "128Mi",
	}))))
	running := ownedBy(pod("api-a", podSpec(container("app", nil))), "Deployment", "api")

	found := reportWithUsage(t, usageFor([]string{"api-a"}, 40, 90), owner, running)

	if onlyFinding(t, found, "requests-far-above-usage").Detail != "pods request 2000m cpu and use 40m" {
		t.Fatalf("detail was %q", onlyFinding(t, found, "requests-far-above-usage").Detail)
	}
}

func TestMemoryOverprovisioningIsReportedWhenCpuIsFine(t *testing.T) {
	owner := deployment("api", podSpec(container("app", resources("requests", map[string]any{
		"cpu":    "100m",
		"memory": "4Gi",
	}))))
	running := ownedBy(pod("api-a", podSpec(container("app", nil))), "Deployment", "api")

	found := reportWithUsage(t, usageFor([]string{"api-a"}, 80, 100), owner, running)

	if onlyFinding(t, found, "requests-far-above-usage").Detail != "pods request 4096Mi memory and use 100Mi" {
		t.Fatalf("detail was %q", onlyFinding(t, found, "requests-far-above-usage").Detail)
	}
}

func TestUsageIsAveragedOverTheWorkloadsPods(t *testing.T) {
	owner := replicas(deployment("api", podSpec(container("app", resources("requests", map[string]any{
		"cpu":    "1",
		"memory": "128Mi",
	})))), 2)
	first := ownedBy(pod("api-a", podSpec(container("app", nil))), "Deployment", "api")
	second := ownedBy(pod("api-b", podSpec(container("app", nil))), "Deployment", "api")
	usage := map[string]api.ResourceUsage{
		testNamespace + "/api-a": {CPUMilli: 400, MemoryMi: 40},
		testNamespace + "/api-b": {CPUMilli: 200, MemoryMi: 40},
	}

	found := reportWithUsage(t, usage, owner, first, second)

	if findingCount(t, found, "requests-far-above-usage") != 0 {
		t.Fatal("a 1000m request against a 300m mean was reported as overprovisioned")
	}
}

func TestTheUsageCheckIsSkippedWithoutMetrics(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", nil))))
	group := groupNamed(t, found, "requests-far-above-usage")

	if group.Skipped == "" {
		t.Fatal("the usage check ran without metrics")
	}
	if len(group.Findings) != 0 {
		t.Fatal("a skipped check produced findings")
	}
}

func TestAWorkloadWithNoMeasuredPodsIsNotFlagged(t *testing.T) {
	owner := deployment("api", podSpec(container("app", resources("requests", map[string]any{
		"cpu":    "2",
		"memory": "128Mi",
	}))))

	found := reportWithUsage(t, usageFor([]string{"other-a"}, 10, 10), owner)

	if findingCount(t, found, "requests-far-above-usage") != 0 {
		t.Fatal("a workload with no measured pods was reported")
	}
}

func TestInitContainersDoNotCountTowardsTheRequestTotal(t *testing.T) {
	spec := podSpec(container("app", resources("requests", map[string]any{
		"cpu":    "100m",
		"memory": "128Mi",
	})))
	spec["initContainers"] = []any{container("setup", resources("requests", map[string]any{
		"cpu":    "4",
		"memory": "128Mi",
	}))}
	owner := deployment("api", spec)
	running := ownedBy(pod("api-a", podSpec(container("app", nil))), "Deployment", "api")

	found := reportWithUsage(t, usageFor([]string{"api-a"}, 80, 100), owner, running)

	if findingCount(t, found, "requests-far-above-usage") != 0 {
		t.Fatal("an init container's request counted towards the running total")
	}
}

func TestAWorkloadUsingNothingIsNotOverprovisioned(t *testing.T) {
	owner := deployment("api", podSpec(container("app", resources("requests", map[string]any{
		"cpu":    "2",
		"memory": "128Mi",
	}))))
	running := ownedBy(pod("api-a", podSpec(container("app", nil))), "Deployment", "api")

	found := reportWithUsage(t, usageFor([]string{"api-a"}, 0, 0), owner, running)

	if findingCount(t, found, "requests-far-above-usage") != 0 {
		t.Fatal("a workload reporting no usage was called overprovisioned")
	}
}

func TestAnUnreadableRequestDoesNotCountTowardsTheTotal(t *testing.T) {
	owner := deployment("api", podSpec(container("app", resources("requests", map[string]any{
		"cpu":    "not a quantity",
		"memory": "not a quantity",
	}))))
	running := ownedBy(pod("api-a", podSpec(container("app", nil))), "Deployment", "api")

	found := reportWithUsage(t, usageFor([]string{"api-a"}, 40, 90), owner, running)

	if findingCount(t, found, "requests-far-above-usage") != 0 {
		t.Fatal("an unreadable request produced an overprovisioning finding")
	}
}
