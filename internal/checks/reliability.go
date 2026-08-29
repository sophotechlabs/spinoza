package checks

import (
	"slices"
	"strconv"
	"strings"
)

var noSpread = []string{daemonSetKind, "CronJob", "Pod"}

func reliabilityChecks() []check {
	return []check{
		{
			id:       "probes-missing",
			title:    "No liveness or readiness probe",
			category: categoryReliability,
			severity: severityMedium,
			wrong:    "Without readiness a pod takes traffic before it can serve; without liveness a wedged process is never restarted.",
			remedy:   "Add a readinessProbe for when it can serve and a livenessProbe for when it is alive.",
			find:     overContainers(probesMissing),
		},
		{
			id:       "image-latest",
			title:    "Image tagged :latest",
			category: categoryReliability,
			severity: severityMedium,
			wrong:    "Two pods of the same workload can run different code, and a rollback has no target.",
			remedy:   "Pin to a release tag, or to a digest when the tag is republished.",
			find:     overContainers(latestImage),
		},
		{
			id:       "single-replica",
			title:    "Single-replica Deployment",
			category: categoryReliability,
			severity: severityLow,
			wrong:    "Every drain, eviction and rollout is downtime.",
			remedy:   "Raise spec.replicas to at least 2 and add a PodDisruptionBudget.",
			find:     overSubjects(singleReplica),
		},
		{
			id:       "replicas-one-node",
			title:    "Every replica on one node",
			category: categoryReliability,
			severity: severityMedium,
			wrong:    "The replicas share one failure domain; losing that node takes all of them.",
			remedy:   "Add a topologySpreadConstraint on kubernetes.io/hostname.",
			find:     overSubjects(oneNode),
		},
	}
}

func probesMissing(subject Subject, container Container) (string, string) {
	if !longRunning(subject, container) {
		return "", ""
	}
	_, live := container.Spec["livenessProbe"]
	_, ready := container.Spec["readinessProbe"]
	if live || ready {
		return "", ""
	}
	return "no livenessProbe and no readinessProbe", ""
}

func longRunning(subject Subject, container Container) bool {
	if container.Init {
		return false
	}
	if subject.Kind == "Job" || subject.Kind == "CronJob" {
		return false
	}
	policy := stringAt(subject.Pod, "restartPolicy")
	return policy == "" || policy == "Always"
}

func latestImage(_ Subject, container Container) (string, string) {
	image := stringAt(container.Spec, "image")
	if image == "" {
		return "", ""
	}
	if strings.Contains(image, "@") {
		return "", ""
	}
	tag := tagOf(image)
	if tag == "" {
		return image + " carries no tag, so :latest is pulled", ""
	}
	if tag == "latest" {
		return image + " is tagged :latest", ""
	}
	return "", ""
}

func tagOf(image string) string {
	colon := strings.LastIndex(image, ":")
	if colon < 0 {
		return ""
	}
	if colon < strings.LastIndex(image, "/") {
		return ""
	}
	return image[colon+1:]
}

func singleReplica(subject Subject) (string, string) {
	if subject.Kind != "Deployment" {
		return "", ""
	}
	if subject.Replicas != 1 {
		return "", ""
	}
	return "spec.replicas is 1", specPatch([]string{"replicas: 2"})
}

func oneNode(subject Subject) (string, string) {
	if slices.Contains(noSpread, subject.Kind) {
		return "", ""
	}
	node := sharedNode(subject.Pods)
	if node == "" {
		return "", ""
	}
	count := strconv.Itoa(len(subject.Pods))
	return "all " + count + " pods run on node " + node, spreadPatch(subject)
}

func sharedNode(pods []Placed) string {
	if len(pods) < 2 {
		return ""
	}
	node := pods[0].Node
	if node == "" {
		return ""
	}
	for _, pod := range pods[1:] {
		if pod.Node != node {
			return ""
		}
	}
	return node
}
