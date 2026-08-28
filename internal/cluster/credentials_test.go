package cluster

import (
	"errors"
	"testing"
)

func TestTheGkePluginIsNamed(t *testing.T) {
	err := errors.New(`Get "https://35.204.10.181/api?timeout=30s": getting credentials: ` +
		`exec: executable /opt/homebrew/share/google-cloud-sdk/bin/gke-gcloud-auth-plugin failed with exit code 1`)

	if got := credentialPlugin(err); got != "gke-gcloud-auth-plugin" {
		t.Fatalf("plugin = %q, want the binary that failed", got)
	}
}

func TestAPluginOnThePathIsNamedToo(t *testing.T) {
	err := errors.New("getting credentials: exec: executable aws-iam-authenticator failed with exit code 2")

	if got := credentialPlugin(err); got != "aws-iam-authenticator" {
		t.Fatalf("plugin = %q", got)
	}
}

func TestAnUnrelatedFailureNamesNoPlugin(t *testing.T) {
	err := errors.New(`Get "https://10.0.0.1/api": dial tcp 10.0.0.1:443: connect: connection refused`)

	if got := credentialPlugin(err); got != "" {
		t.Fatalf("plugin = %q, want none", got)
	}
}

func TestNoErrorNamesNoPlugin(t *testing.T) {
	if got := credentialPlugin(nil); got != "" {
		t.Fatalf("plugin = %q, want none", got)
	}
}

func TestAMarkerWithNothingAfterItNamesNoPlugin(t *testing.T) {
	err := errors.New("getting credentials: exec: executable  failed")

	if got := credentialPlugin(err); got != "" {
		t.Fatalf("plugin = %q, want none", got)
	}
}

func TestACredentialFailureIsSaidInOneLine(t *testing.T) {
	err := unreachable("gke_prod", errors.New(
		"getting credentials: exec: executable /opt/homebrew/bin/gke-gcloud-auth-plugin failed with exit code 1",
	))

	want := `context "gke_prod" could not get credentials: gke-gcloud-auth-plugin failed. Check that it runs in your shell`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestAnyOtherFailureKeepsTheWholeReason(t *testing.T) {
	err := unreachable("p-mk1", errors.New("connection refused"))

	want := `context "p-mk1" lists no resource types: connection refused`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestNoReasonAtAllStillSaysWhichContext(t *testing.T) {
	err := unreachable("p-mk1", nil)

	want := `context "p-mk1" lists no resource types`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}
