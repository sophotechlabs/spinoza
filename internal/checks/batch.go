package checks

import (
	"strconv"
)

const (
	ttlField          = "ttlSecondsAfterFinished"
	activeDeadline    = "activeDeadlineSeconds"
	backoffField      = "backoffLimit"
	startingDeadline  = "startingDeadlineSeconds"
	concurrencyField  = "concurrencyPolicy"
	successHistory    = "successfulJobsHistoryLimit"
	failedHistory     = "failedJobsHistoryLimit"
	allowConcurrent   = "Allow"
	jobKind           = "Job"
	cronKind          = "CronJob"
	highBackoffLimit  = 10
	keptJobsCeiling   = 10
	defaultKeptJobs   = 3
	defaultTTLSeconds = 3600
)

func batchChecks() []check {
	return []check{
		{
			id:       "job-no-ttl",
			title:    "Finished Job kept for ever",
			category: categoryEfficiency,
			severity: severityMedium,
			wrong:    "The Job and its pods stay until somebody deletes them, so completed work accumulates in the API server.",
			remedy:   "Set spec.ttlSecondsAfterFinished so the Job cleans itself up.",
			find:     overSubjects(jobNoTTL),
		},
		{
			id:       "job-no-active-deadline",
			title:    "Job with no deadline",
			category: categoryReliability,
			severity: severityMedium,
			wrong:    "A job that hangs holds its pod and its resources until somebody notices.",
			remedy:   "Set spec.activeDeadlineSeconds to longer than a healthy run and shorter than a hang.",
			find:     overSubjects(jobNoDeadline),
		},
		{
			id:       "job-backoff-limit",
			title:    "Job retry count worth a second look",
			category: categoryReliability,
			severity: severityLow,
			wrong:    "A backoffLimit of 0 gives up on the first transient failure; a high one retries a permanent failure for hours.",
			remedy:   "Set spec.backoffLimit to the number of retries the work actually deserves.",
			find:     overSubjects(jobBackoffLimit),
		},
		{
			id:       "cronjob-no-starting-deadline",
			title:    "CronJob with no starting deadline",
			category: categoryReliability,
			severity: severityMedium,
			wrong:    "After enough missed schedules the controller stops scheduling the CronJob entirely and says so only in an event.",
			remedy:   "Set spec.startingDeadlineSeconds so a missed run is skipped rather than counted against it.",
			find:     overSubjects(cronNoStartingDeadline),
		},
		{
			id:       "cronjob-concurrency-allow",
			title:    "CronJob runs may overlap",
			category: categoryReliability,
			severity: severityMedium,
			wrong:    "A run that outlasts its interval is joined by the next one, so two copies work on the same thing at once.",
			remedy:   "Set spec.concurrencyPolicy to Forbid, or to Replace if the newest run should win.",
			find:     overSubjects(cronConcurrencyAllow),
		},
		{
			id:       "cronjob-unbounded-history",
			title:    "Every CronJob run kept",
			category: categoryEfficiency,
			severity: severityLow,
			wrong:    "Finished Jobs and their pods accumulate on every schedule for the life of the CronJob.",
			remedy:   "Lower spec.successfulJobsHistoryLimit and spec.failedJobsHistoryLimit.",
			find:     overSubjects(cronUnboundedHistory),
		},
	}
}

func jobSpecOf(subject Subject) map[string]any {
	if subject.Kind == cronKind {
		return specAt(subject.Object, specField, "jobTemplate", specField)
	}
	return specAt(subject.Object, specField)
}

func isJobShaped(kind string) bool {
	return kind == jobKind || kind == cronKind
}

func jobNoTTL(subject Subject) (string, string) {
	if !isJobShaped(subject.Kind) {
		return "", ""
	}
	if _, set := numberAt(jobSpecOf(subject), ttlField); set {
		return "", ""
	}
	return "spec." + ttlField + " is unset, so the finished Job stays until it is deleted",
		jobPatch(subject, []string{ttlField + ": " + strconv.Itoa(defaultTTLSeconds)})
}

func jobNoDeadline(subject Subject) (string, string) {
	if !isJobShaped(subject.Kind) {
		return "", ""
	}
	if _, set := numberAt(jobSpecOf(subject), activeDeadline); set {
		return "", ""
	}
	return "spec." + activeDeadline + " is unset, so a hung run is never stopped", ""
}

func jobBackoffLimit(subject Subject) (string, string) {
	if !isJobShaped(subject.Kind) {
		return "", ""
	}
	limit, set := numberAt(jobSpecOf(subject), backoffField)
	if !set {
		return "", ""
	}
	if limit == 0 {
		return "spec." + backoffField + " is 0, so one transient failure fails the Job", ""
	}
	if limit > highBackoffLimit {
		return "spec." + backoffField + " is " + strconv.FormatInt(limit, 10) +
			", so a permanent failure is retried that many times", ""
	}
	return "", ""
}

func cronNoStartingDeadline(subject Subject) (string, string) {
	if subject.Kind != cronKind {
		return "", ""
	}
	if _, set := numberAt(specAt(subject.Object, specField), startingDeadline); set {
		return "", ""
	}
	return "spec." + startingDeadline + " is unset, so missed schedules count towards the hundred that stop it",
		specPatch([]string{startingDeadline + ": 120"})
}

func cronConcurrencyAllow(subject Subject) (string, string) {
	if subject.Kind != cronKind {
		return "", ""
	}
	policy := stringAt(specAt(subject.Object, specField), concurrencyField)
	if policy != "" && policy != allowConcurrent {
		return "", ""
	}
	named := policy
	if named == "" {
		named = "unset, which allows overlap"
	}
	return "spec." + concurrencyField + " is " + named,
		specPatch([]string{concurrencyField + ": Forbid"})
}

func cronUnboundedHistory(subject Subject) (string, string) {
	if subject.Kind != cronKind {
		return "", ""
	}
	spec := specAt(subject.Object, specField)
	for _, field := range []string{successHistory, failedHistory} {
		kept, set := numberAt(spec, field)
		if set && kept <= keptJobsCeiling {
			continue
		}
		if !set {
			return "spec." + field + " is unset",
				specPatch([]string{field + ": " + strconv.Itoa(defaultKeptJobs)})
		}
		return "spec." + field + " keeps " + strconv.FormatInt(kept, 10) + " finished jobs",
			specPatch([]string{field + ": " + strconv.Itoa(defaultKeptJobs)})
	}
	return "", ""
}

func jobPatch(subject Subject, body []string) string {
	if subject.Kind == cronKind {
		return nest([]string{specField, "jobTemplate", specField}, body)
	}
	return specPatch(body)
}
