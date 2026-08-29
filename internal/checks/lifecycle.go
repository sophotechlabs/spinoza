package checks

import (
	"maps"
	"slices"
	"strconv"
	"strings"
)

const (
	livenessProbe   = "livenessProbe"
	readinessProbe  = "readinessProbe"
	startupProbe    = "startupProbe"
	ephemeralName   = "ephemeral-storage"
	recreateShape   = "Recreate"
	graceField      = "terminationGracePeriodSeconds"
	revisionHistory = "revisionHistoryLimit"
)

const (
	longGraceSeconds  = 600
	keptRevisions     = 10
	judgementSentence = " A judgement call: "
)

var probeHandlers = []string{"httpGet", "tcpSocket", "exec", "grpc"}

var batchKinds = []string{"Job", "CronJob"}

const (
	deploymentKind  = "Deployment"
	statefulSetKind = "StatefulSet"
	daemonSetKind   = "DaemonSet"
	secretField     = "secret"
)

var spreadableKinds = []string{deploymentKind, statefulSetKind, "ReplicaSet", "ReplicationController"}

func lifecycleChecks() []check {
	return []check{
		{
			id:       "probes-identical",
			title:    "Liveness and readiness are the same probe",
			category: categoryReliability,
			severity: severityMedium,
			wrong:    "A dependency that fails readiness then also fails liveness, so a pod that would have recovered is restarted instead.",
			remedy:   "Make readiness answer can it serve, and liveness answer is the process alive.",
			find:     overContainers(probesIdentical),
		},
		{
			id:       "probe-port-not-declared",
			title:    "Probe points at an undeclared port",
			category: categoryReliability,
			severity: severityMedium,
			wrong:    "The kubelet probes a port the container never said it listens on, which usually means the probe never passes.",
			remedy:   "Add the port to the container's ports list, or point the probe at one already there.",
			find:     overContainers(probePortUndeclared),
		},
		{
			id:       "liveness-without-startup-grace",
			title:    "Liveness probe with no start-up grace",
			category: categoryReliability,
			severity: severityMedium,
			wrong:    "A slow start looks like a hang, so the kubelet kills the container before it ever finishes booting.",
			remedy:   "Add a startupProbe, or set initialDelaySeconds to longer than a cold start.",
			find:     overContainers(livenessWithoutGrace),
		},
		{
			id:       "no-prestop-hook",
			title:    "No preStop hook",
			category: categoryReliability,
			severity: severityLow,
			wrong:    "Endpoints are removed and SIGTERM is sent at the same moment, so in-flight requests can land on a closing pod." + judgementSentence + "a process that drains cleanly on SIGTERM needs no hook.",
			remedy:   "Add a lifecycle.preStop sleep long enough for the endpoint removal to propagate.",
			find:     overContainers(noPreStop),
		},
		{
			id:       "grace-period-zero",
			title:    "No time to shut down",
			category: categoryReliability,
			severity: severityMedium,
			wrong:    "The container is killed outright, so nothing is flushed and nothing is finished.",
			remedy:   "Raise terminationGracePeriodSeconds to cover the longest request the process serves.",
			find:     overSubjects(graceZero),
		},
		{
			id:       "grace-period-blocks-drain",
			title:    "Shutdown long enough to hold up a drain",
			category: categoryReliability,
			severity: severityLow,
			wrong:    "A drain waits for every pod's grace period, so one long one holds up a node." + judgementSentence + "a queue worker finishing a long job may want exactly this.",
			remedy:   "Shorten terminationGracePeriodSeconds, or move the long work off the shutdown path.",
			find:     overSubjects(graceTooLong),
		},
		{
			id:       "wrong-restart-policy",
			title:    "Restart policy the kind cannot use",
			category: categoryReliability,
			severity: severityHigh,
			wrong:    "A Job whose pods always restart never finishes, and a Deployment whose pods do not restart loses capacity for good.",
			remedy:   "Use Never or OnFailure for Jobs and CronJobs, and Always everywhere else.",
			find:     overSubjects(wrongRestartPolicy),
		},
		{
			id:       "ephemeral-storage-unset",
			title:    "No ephemeral storage request or limit",
			category: categoryEfficiency,
			severity: severityLow,
			wrong:    "Logs and scratch files fill the node's disk, and the kubelet evicts whatever it likes to get it back.",
			remedy:   "Set resources.requests and resources.limits for ephemeral-storage.",
			find:     overContainers(ephemeralStorageUnset),
		},
		{
			id:       "memory-limit-not-request",
			title:    "Memory limit above its request",
			category: categoryEfficiency,
			severity: severityLow,
			wrong:    "The pod is Burstable, so it is a candidate for eviction under node memory pressure even while inside its own limit." + judgementSentence + "headroom is the point of Burstable for workloads that spike briefly.",
			remedy:   "Set resources.requests.memory equal to resources.limits.memory for a Guaranteed pod.",
			find:     overContainers(memoryNotGuaranteed),
		},
		{
			id:       "init-container-unbounded",
			title:    "Init container with no resources",
			category: categoryEfficiency,
			severity: severityLow,
			wrong:    "An init container's request is what the pod is scheduled on while it runs, so an unset one hides the pod's real peak.",
			remedy:   "Set resources.requests on the init container as well.",
			find:     overContainers(initUnbounded),
		},
		{
			id:       "replicas-zero",
			title:    "Scaled to zero",
			category: categoryReliability,
			severity: severityMedium,
			wrong:    "Nothing is running and nothing will restart it, which is usually a scale-down somebody forgot to undo.",
			remedy:   "Scale it back up, or delete it if it is finished with.",
			find:     overSubjects(replicasZero),
		},
		{
			id:       "no-spread-no-anti-affinity",
			title:    "Replicas free to share a node",
			category: categoryReliability,
			severity: severityMedium,
			wrong:    "Nothing stops the scheduler putting every replica on one node, so one node failure takes the whole workload.",
			remedy:   "Add a topologySpreadConstraint on kubernetes.io/hostname, or a podAntiAffinity.",
			find:     overSubjects(noSpreadRule),
		},
		{
			id:       "recreate-strategy",
			title:    "Recreate rollout",
			category: categoryReliability,
			severity: severityMedium,
			wrong:    "Every pod is stopped before the new ones start, so the workload is down for the length of a rollout.",
			remedy:   "Use RollingUpdate unless two versions genuinely cannot run at once.",
			find:     overSubjects(recreateStrategy),
		},
		{
			id:       "max-unavailable-all",
			title:    "Rollout may take every replica at once",
			category: categoryReliability,
			severity: severityMedium,
			wrong:    "maxUnavailable covers the whole workload, so a rolling update is a full outage in slower clothes.",
			remedy:   "Lower maxUnavailable, and set maxSurge so replacements start before the old pods go.",
			find:     overSubjects(maxUnavailableAll),
		},
		{
			id:       "statefulset-no-service-name",
			title:    "StatefulSet with no governing Service",
			category: categoryReliability,
			severity: severityHigh,
			wrong:    "Without serviceName the pods get no stable DNS, which is most of the reason to use a StatefulSet.",
			remedy:   "Create a headless Service and name it in spec.serviceName.",
			find:     overSubjects(statefulSetNoService),
		},
		{
			id:       "unbounded-revision-history",
			title:    "More old ReplicaSets kept than anyone rolls back to",
			category: categoryEfficiency,
			severity: severityLow,
			wrong:    "Old ReplicaSets accumulate for the life of the workload and cost API server memory and list time.",
			remedy:   "Lower spec.revisionHistoryLimit to the number of rollbacks you would actually make.",
			find:     overSubjects(unboundedHistory),
		},
		{
			id:       "duplicate-env-keys",
			title:    "Environment variable set twice",
			category: categoryReliability,
			severity: severityMedium,
			wrong:    "The last value silently wins, so the container reads something other than what the first entry says.",
			remedy:   "Remove the duplicate entry.",
			find:     overContainers(duplicateEnvKeys),
		},
	}
}

func probeAt(container Container, name string) map[string]any {
	probe, ok := container.Spec[name].(map[string]any)
	if !ok {
		return nil
	}
	return probe
}

func handlerOf(probe map[string]any) (name string, body map[string]any) {
	for _, field := range probeHandlers {
		found, ok := probe[field].(map[string]any)
		if ok {
			return field, found
		}
	}
	return "", nil
}

func probesIdentical(_ Subject, container Container) (string, string) {
	live := probeAt(container, livenessProbe)
	ready := probeAt(container, readinessProbe)
	if live == nil || ready == nil {
		return "", ""
	}
	liveName, liveBody := handlerOf(live)
	readyName, readyBody := handlerOf(ready)
	if liveName == "" || liveName != readyName {
		return "", ""
	}
	if !maps.EqualFunc(liveBody, readyBody, sameValue) {
		return "", ""
	}
	return "livenessProbe and readinessProbe both run the same " + liveName, ""
}

func sameValue(left, right any) bool {
	return valueText(left) == valueText(right)
}

func valueText(value any) string {
	switch typed := value.(type) {
	case string:
		return "s" + typed
	case int64:
		return "i" + strconv.FormatInt(typed, 10)
	case float64:
		return "i" + strconv.FormatInt(int64(typed), 10)
	case bool:
		return "b" + strconv.FormatBool(typed)
	default:
		return ""
	}
}

func probePortUndeclared(_ Subject, container Container) (string, string) {
	declared := declaredPorts(container)
	if len(declared) == 0 {
		return "", ""
	}
	for _, name := range []string{livenessProbe, readinessProbe, startupProbe} {
		probe := probeAt(container, name)
		if probe == nil {
			continue
		}
		wanted := probePort(probe)
		if wanted == "" {
			continue
		}
		if !slices.Contains(declared, wanted) {
			return name + " probes port " + wanted + ", which the container does not declare", ""
		}
	}
	return "", ""
}

func probePort(probe map[string]any) string {
	for _, field := range []string{"httpGet", "tcpSocket"} {
		body, ok := probe[field].(map[string]any)
		if !ok {
			continue
		}
		return portText(body["port"])
	}
	return ""
}

func portText(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	default:
		return ""
	}
}

func declaredPorts(container Container) []string {
	listed, ok := container.Spec["ports"].([]any)
	if !ok {
		return nil
	}
	out := []string{}
	for _, entry := range listed {
		item, isMap := entry.(map[string]any)
		if !isMap {
			continue
		}
		if number := portText(item["containerPort"]); number != "" {
			out = append(out, number)
		}
		if name := stringAt(item, "name"); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func livenessWithoutGrace(_ Subject, container Container) (string, string) {
	probe := probeAt(container, livenessProbe)
	if probe == nil {
		return "", ""
	}
	if probeAt(container, startupProbe) != nil {
		return "", ""
	}
	delay, set := numberAt(probe, "initialDelaySeconds")
	if set && delay > 0 {
		return "", ""
	}
	return "livenessProbe has no initialDelaySeconds and the container has no startupProbe", ""
}

func noPreStop(subject Subject, container Container) (string, string) {
	if !longRunning(subject, container) {
		return "", ""
	}
	hooks, ok := container.Spec["lifecycle"].(map[string]any)
	if ok {
		if _, found := hooks["preStop"]; found {
			return "", ""
		}
	}
	return "no lifecycle.preStop hook", ""
}

func graceOf(subject Subject) (seconds int64, set bool) {
	return numberAt(subject.Pod, graceField)
}

func graceZero(subject Subject) (string, string) {
	seconds, set := graceOf(subject)
	if !set || seconds != 0 {
		return "", ""
	}
	return graceField + " is 0, so the container is killed rather than asked to stop", ""
}

func graceTooLong(subject Subject) (string, string) {
	seconds, set := graceOf(subject)
	if !set || seconds <= longGraceSeconds {
		return "", ""
	}
	return graceField + " is " + strconv.FormatInt(seconds, 10) +
		"s, so a drain waits that long for each pod", ""
}

// A bare Pod that runs to completion is meant to use Never, and a job runner
// creates thousands of them. Only a controller that is supposed to keep its
// pods running is judged on this. Found on GKE production 2026-08-29, where
// the check fired on 497 Airbyte replication pods.
func wrongRestartPolicy(subject Subject) (string, string) {
	policy := stringAt(subject.Pod, "restartPolicy")
	if isBatch(subject.Kind) {
		if policy == "Never" || policy == "OnFailure" {
			return "", ""
		}
		named := policy
		if named == "" {
			named = "unset, which means Always"
		}
		return "restartPolicy is " + named + ", which a " + subject.Kind + " cannot use", ""
	}
	if !keepsPodsRunning(subject.Kind) || policy == "" || policy == alwaysPull {
		return "", ""
	}
	return "restartPolicy is " + policy + ", so a stopped container is never replaced", ""
}

func isBatch(kind string) bool {
	return slices.Contains(batchKinds, kind)
}

func keepsPodsRunning(kind string) bool {
	return isSpreadable(kind) || kind == daemonSetKind
}

func ephemeralStorageUnset(_ Subject, container Container) (string, string) {
	_, hasRequest := quantityAt(container.Spec, requests, ephemeralName)
	_, hasLimit := quantityAt(container.Spec, limits, ephemeralName)
	if hasRequest && hasLimit {
		return "", ""
	}
	if !hasRequest && !hasLimit {
		return "no ephemeral-storage request or limit", ""
	}
	if hasRequest {
		return "no ephemeral-storage limit", ""
	}
	return "no ephemeral-storage request", ""
}

func memoryNotGuaranteed(_ Subject, container Container) (string, string) {
	request, hasRequest := quantityAt(container.Spec, requests, memoryName)
	limit, hasLimit := quantityAt(container.Spec, limits, memoryName)
	if !hasRequest || !hasLimit {
		return "", ""
	}
	if request.Value() == limit.Value() {
		return "", ""
	}
	return "memory request " + request.String() + " is below the " + limit.String() + " limit", ""
}

func initUnbounded(_ Subject, container Container) (string, string) {
	if !container.Init {
		return "", ""
	}
	missing := missingIn(container, requests)
	if len(missing) == 0 {
		return "", ""
	}
	return "init container has no " + strings.Join(missing, " or ") + " request", ""
}

func replicasZero(subject Subject) (string, string) {
	if !isSpreadable(subject.Kind) {
		return "", ""
	}
	count, set := numberAt(specAt(subject.Object, specField), "replicas")
	if !set || count != 0 {
		return "", ""
	}
	return "spec.replicas is 0, so nothing is running", ""
}

func isSpreadable(kind string) bool {
	return slices.Contains(spreadableKinds, kind)
}

func noSpreadRule(subject Subject) (string, string) {
	if !isSpreadable(subject.Kind) || subject.Replicas < 2 {
		return "", ""
	}
	if listedItems(subject.Pod, "topologySpreadConstraints") > 0 {
		return "", ""
	}
	if hasAntiAffinity(subject) {
		return "", ""
	}
	return strconv.FormatInt(subject.Replicas, 10) +
		" replicas with no topologySpreadConstraints and no podAntiAffinity", spreadPatch(subject)
}

func hasAntiAffinity(subject Subject) bool {
	affinity, ok := subject.Pod["affinity"].(map[string]any)
	if !ok {
		return false
	}
	rule, found := affinity["podAntiAffinity"].(map[string]any)
	return found && rule != nil
}

func listedItems(spec map[string]any, field string) int {
	listed, ok := spec[field].([]any)
	if !ok {
		return 0
	}
	return len(listed)
}

func strategyOf(subject Subject) map[string]any {
	spec := specAt(subject.Object, specField)
	strategy, ok := spec["strategy"].(map[string]any)
	if !ok {
		return nil
	}
	return strategy
}

func recreateStrategy(subject Subject) (string, string) {
	if subject.Kind != deploymentKind {
		return "", ""
	}
	if stringAt(strategyOf(subject), "type") != recreateShape {
		return "", ""
	}
	return "spec.strategy.type is Recreate, so every pod stops before the new ones start",
		specPatch([]string{"strategy:", indent + "type: RollingUpdate"})
}

func maxUnavailableAll(subject Subject) (string, string) {
	if subject.Kind != deploymentKind {
		return "", ""
	}
	rolling, ok := strategyOf(subject)["rollingUpdate"].(map[string]any)
	if !ok {
		return "", ""
	}
	raw, present := rolling["maxUnavailable"]
	if !present {
		return "", ""
	}
	if !takesEverything(raw, subject.Replicas) {
		return "", ""
	}
	return "spec.strategy.rollingUpdate.maxUnavailable is " + portText(raw) +
		", which covers every replica", ""
}

func takesEverything(raw any, replicas int64) bool {
	text := portText(raw)
	if text == "" {
		return false
	}
	if strings.HasSuffix(text, "%") {
		return text == "100%"
	}
	count, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return false
	}
	return replicas > 0 && count >= replicas
}

func statefulSetNoService(subject Subject) (string, string) {
	if subject.Kind != statefulSetKind {
		return "", ""
	}
	if stringAt(specAt(subject.Object, specField), "serviceName") != "" {
		return "", ""
	}
	return "spec.serviceName is unset, so the pods get no stable DNS name", ""
}

func unboundedHistory(subject Subject) (string, string) {
	if !isSpreadable(subject.Kind) && subject.Kind != daemonSetKind {
		return "", ""
	}
	kept, set := numberAt(specAt(subject.Object, specField), revisionHistory)
	if !set || kept <= keptRevisions {
		return "", ""
	}
	return "spec." + revisionHistory + " keeps " + strconv.FormatInt(kept, 10) + " old revisions",
		specPatch([]string{revisionHistory + ": 3"})
}

func duplicateEnvKeys(_ Subject, container Container) (string, string) {
	listed, ok := container.Spec["env"].([]any)
	if !ok {
		return "", ""
	}
	seen := map[string]bool{}
	repeated := []string{}
	for _, entry := range listed {
		item, isMap := entry.(map[string]any)
		if !isMap {
			continue
		}
		name := stringAt(item, "name")
		if name == "" {
			continue
		}
		if seen[name] && !slices.Contains(repeated, name) {
			repeated = append(repeated, name)
		}
		seen[name] = true
	}
	if len(repeated) == 0 {
		return "", ""
	}
	return strings.Join(repeated, ", ") + " is set more than once", ""
}
