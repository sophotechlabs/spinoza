package argocd

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/faults"
	"github.com/sophotechlabs/spinoza/internal/unstr"
)

const (
	outOfSync      = "OutOfSync"
	succeededPhase = "Succeeded"
	degraded       = "Degraded"
	missing        = "Missing"
)

var conditionAdvice = map[string]string{
	"ComparisonError":         "check the repository credentials and the path",
	"InvalidSpecError":        "argo cd will not sync until the spec is fixed",
	"OrphanedResourceWarning": "the destination namespace holds resources this application does not declare",
	"SharedResourceWarning":   "another application declares the same resource",
	"RepeatedResourceWarning": "the same resource is declared twice in this application",
	"ExcludedResourceWarning": "argo cd's settings exclude it, so it is never applied",
	"SyncError":               "the last sync did not finish",
	"DeletionError":           "argo cd could not delete something it had applied",
}

func issuesOfApp(app *unstructured.Unstructured) []api.GitopsIssue {
	out := make([]api.GitopsIssue, 0, 4)
	out = append(out, terminatingIssue(app)...)
	out = append(out, conditionIssues(app)...)
	out = append(out, operationIssue(app)...)
	out = append(out, driftIssues(app)...)
	out = append(out, healthIssues(app)...)
	if len(out) == 0 {
		return nil
	}
	return out
}

func terminatingIssue(app *unstructured.Unstructured) []api.GitopsIssue {
	if app.GetDeletionTimestamp() == nil {
		return nil
	}
	return []api.GitopsIssue{{
		Severity: api.SeverityWarning,
		Title:    "This application is being deleted",
		Detail:   heldBy(app.GetFinalizers()),
	}}
}

func heldBy(finalizers []string) string {
	if len(finalizers) == 0 {
		return "no finalizers left"
	}
	return "held by " + strings.Join(finalizers, ", ")
}

func conditionIssues(app *unstructured.Unstructured) []api.GitopsIssue {
	out := []api.GitopsIssue{}
	for _, raw := range unstr.Slice(app, "status", "conditions") {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		kind := unstr.At(entry, "type")
		if kind == "" {
			continue
		}
		out = append(out, api.GitopsIssue{
			Severity: severityOf(kind),
			Title:    unstr.At(entry, "message"),
			Detail:   adviceFor(kind),
			Subject:  kind,
		})
	}
	return out
}

func severityOf(kind string) string {
	if strings.HasSuffix(kind, "Warning") {
		return api.SeverityWarning
	}
	if strings.HasSuffix(kind, "Error") {
		return api.SeverityDegraded
	}
	return api.SeverityInfo
}

func adviceFor(kind string) string {
	advice, ok := conditionAdvice[kind]
	if !ok {
		return ""
	}
	return advice
}

func operationIssue(app *unstructured.Unstructured) []api.GitopsIssue {
	phase := unstr.String(app, "status", "operationState", "phase")
	if phase == "" || phase == succeededPhase || phase == runningPhase {
		return nil
	}
	message := unstr.String(app, "status", "operationState", "message")
	return []api.GitopsIssue{{
		Severity: api.SeverityDegraded,
		Title:    "The last operation " + strings.ToLower(phase),
		Detail:   detailOf(message),
		Subject:  "operation",
	}}
}

func detailOf(message string) string {
	cause := faults.Cause(message)
	if cause == "" {
		return message
	}
	if message == "" {
		return cause
	}
	return cause + " — " + message
}

func driftIssues(app *unstructured.Unstructured) []api.GitopsIssue {
	if unstr.String(app, "status", "sync", "status") != outOfSync {
		return nil
	}
	if !AutoSyncing(app) {
		return []api.GitopsIssue{{
			Severity: api.SeverityWarning,
			Title:    "Nothing will reconcile this",
			Detail:   "auto-sync is off; nothing changes until someone syncs",
			Subject:  "drift",
		}}
	}
	if unstr.String(app, "status", "operationState", "phase") != succeededPhase {
		return nil
	}
	return []api.GitopsIssue{{
		Severity: api.SeverityWarning,
		Title:    "Synced, and out of sync again",
		Detail: "the last sync succeeded and auto-sync is on, so something rewrites these fields after every apply: " +
			"a mutating webhook, a second controller, or a migration",
		Subject: "drift",
	}}
}

func healthIssues(app *unstructured.Unstructured) []api.GitopsIssue {
	blamed := unstr.String(app, "status", "operationState", "message")
	seen := map[string]bool{}
	out := []api.GitopsIssue{}
	for _, one := range resourcesOfApp(app) {
		if one.Health != degraded && one.Health != missing {
			continue
		}
		subject := one.Kind + "/" + one.Name
		if seen[subject] {
			continue
		}
		seen[subject] = true
		if strings.Contains(blamed, one.Name) {
			continue
		}
		out = append(out, api.GitopsIssue{
			Severity: healthSeverity(one.Health),
			Title:    subject + " is " + strings.ToLower(one.Health),
			Detail:   one.Message,
			Subject:  subject,
		})
	}
	return out
}

func healthSeverity(health string) string {
	if health == degraded {
		return api.SeverityDegraded
	}
	return api.SeverityWarning
}
