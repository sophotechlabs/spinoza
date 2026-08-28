package faults

import (
	"strings"
	"testing"
)

func TestNamesTheWebhookThatRefused(t *testing.T) {
	got := Cause(`admission webhook "validate.kyverno.svc-fail" denied the request: policy require-labels`)

	want := "the admission webhook validate.kyverno.svc-fail rejected it"
	if got != want {
		t.Fatalf("cause = %q, want %q", got, want)
	}
}

func TestNamesTheMissingNamespace(t *testing.T) {
	got := Cause(`one or more objects failed to apply, reason: namespaces "shop" not found`)

	want := "the destination namespace shop does not exist; add CreateNamespace=true or create it in git"
	if got != want {
		t.Fatalf("cause = %q, want %q", got, want)
	}
}

func TestNamesTheIdentityThatWasRefused(t *testing.T) {
	got := Cause(`deployments.apps is forbidden: User "system:serviceaccount:argocd:argocd-application-controller" cannot create resource`)

	want := "system:serviceaccount:argocd:argocd-application-controller may not write that resource"
	if got != want {
		t.Fatalf("cause = %q, want %q", got, want)
	}
}

func TestReadsEachKnownShape(t *testing.T) {
	cases := map[string]string{
		"metadata.annotations: Too long: must have at most 262144 bytes": "the manifest is too large for the last-applied annotation; sync with server-side apply",
		`Job.batch "load" is invalid: spec.template: field is immutable`: "an immutable field changed; sync with replace to recreate it",
		"error when creating: something is forbidden":                    "argo cd may not write that resource",
		"Operation terminated": "the operation was stopped",
		"error validating data: ValidationError(Deployment.spec)":               "the manifest does not match the cluster's schema for that kind",
		`unable to recognize "x": no matches for kind "Certificate"`:            "the crd for that kind is missing or not ready",
		"authentication required: https://git.example.test":                     "the repository refused argo cd's credentials",
		"rpc error: code = Unknown desc = repository not found":                 "the repository or revision could not be resolved",
		"Get https://10.0.0.1:6443: context deadline exceeded":                  "argo cd could not reach the cluster or the repository",
		"one or more synchronization tasks completed, hook Job load failed":     "a sync hook failed",
		"another operation is already in progress":                              "another operation is still running",
		"Timed out waiting for the deployment to become ready":                  "the resources were applied but never became healthy",
		"Resource Secret is not permitted in project default":                   "the appproject does not allow that resource",
		"Operation cannot be fulfilled on applications.argoproj.io \"web\"":     "something else changed the resource while argo cd was writing it",
		"Deployment.apps \"web\" is invalid: spec.selector: may not be changed": "an immutable field changed; sync with replace to recreate it",
	}
	for message, want := range cases {
		if got := Cause(message); got != want {
			t.Fatalf("Cause(%q) = %q, want %q", message, got, want)
		}
	}
}

func TestSaysNothingAboutAMessageItDoesNotKnow(t *testing.T) {
	if got := Cause("everything is fine"); got != "" {
		t.Fatalf("cause = %q, want nothing", got)
	}
}

func TestSaysNothingAboutAnEmptyMessage(t *testing.T) {
	if got := Cause(""); got != "" {
		t.Fatalf("cause = %q, want nothing", got)
	}
}

// which rule wins when a message matches more than one

func TestASpecificRuleBeatsTheGenericOneBehindIt(t *testing.T) {
	cases := []struct {
		name    string
		message string
		want    string
	}{
		{
			name:    "a named identity beats the bare forbidden",
			message: `secrets is forbidden: User "system:serviceaccount:argocd:controller" cannot create resource`,
			want:    "system:serviceaccount:argocd:controller may not write that resource",
		},
		{
			name:    "an oversized annotation beats the immutable-field rule that follows it",
			message: "metadata.annotations: Too long: must have at most 262144 bytes; field is immutable",
			want:    "the manifest is too large for the last-applied annotation; sync with server-side apply",
		},
		{
			name:    "a webhook denial beats the hook rule",
			message: `admission webhook "gate.example.test" denied the request: the hook failed`,
			want:    "the admission webhook gate.example.test rejected it",
		},
		{
			name:    "a missing namespace beats the schema rule",
			message: `namespaces "shop" not found: error validating data`,
			want:    "the destination namespace shop does not exist; add CreateNamespace=true or create it in git",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Cause(tc.message); got != tc.want {
				t.Fatalf("Cause(%q) = %q, want %q", tc.message, got, tc.want)
			}
		})
	}
}

func TestEveryRuleAnswersWithNoTemplateLeftInIt(t *testing.T) {
	for _, one := range causes {
		if strings.Contains(one.cause, "$") && !strings.Contains(one.match.String(), "(") {
			t.Fatalf("rule %q expands a capture its pattern never takes", one.cause)
		}
	}
}

func TestAMessageLongerThanAnyClusterSendsStillAnswers(t *testing.T) {
	long := strings.Repeat("something happened. ", 5000) + "Operation terminated"

	if got := Cause(long); got != "the operation was stopped" {
		t.Fatalf("cause = %q, want the one it knows at the end of a long message", got)
	}
}
