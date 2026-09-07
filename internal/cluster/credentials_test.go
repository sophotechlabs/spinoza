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
	err := unreachable("gke_prod", "/home/me/.kube/config", "https://35.204.10.181", errors.New(
		"getting credentials: exec: executable /opt/homebrew/bin/gke-gcloud-auth-plugin failed with exit code 1",
	))

	want := `context "gke_prod" could not get credentials: gke-gcloud-auth-plugin failed. Check that it runs in your shell`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestAnyOtherFailureKeepsTheWholeReason(t *testing.T) {
	err := unreachable("p-mk1", "/home/me/.kube/config", "https://10.0.0.1:6443", errors.New("connection refused"))

	want := `context "p-mk1" lists no resource types: connection refused`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestNoReasonAtAllStillSaysWhichContext(t *testing.T) {
	err := unreachable("p-mk1", "/home/me/.kube/config", "https://10.0.0.1:6443", nil)

	want := `context "p-mk1" lists no resource types`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestAnUntrustedCertificateNamesTheKubeconfigThatWasRead(t *testing.T) {
	err := unreachable("niio-prod", "/Users/me/.kube/config", "https://96102852A7BB2F1369A62D1695713FC6.gr7.us-east-1.eks.amazonaws.com", errors.New(
		`Get "https://96102852A7BB2F1369A62D1695713FC6.gr7.us-east-1.eks.amazonaws.com/api?timeout=30s": `+
			`tls: failed to verify certificate: x509: certificate signed by unknown authority`,
	))

	want := `context "niio-prod" in /Users/me/.kube/config does not trust the certificate ` +
		`https://96102852A7BB2F1369A62D1695713FC6.gr7.us-east-1.eks.amazonaws.com presented ` +
		`(x509: certificate signed by unknown authority). Either this is not the kubeconfig kubectl reads, ` +
		`or a TLS-inspecting proxy on this machine re-signs the connection and spinoza needs the exemption kubectl has`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestACertificateTheSystemDoesNotTrustIsSaidTheSameWay(t *testing.T) {
	reason := untrustedCertificate(errors.New(
		`Get "https://127.0.0.1:6443/api?timeout=30s": tls: failed to verify certificate: x509: “kube-apiserver” certificate is not trusted`,
	))

	if reason != `x509: “kube-apiserver” certificate is not trusted` {
		t.Fatalf("reason = %q", reason)
	}
}

func TestAnExpiredCertificateKeepsTheWholeReason(t *testing.T) {
	err := unreachable("old", "/home/me/.kube/config", "https://10.0.0.1", errors.New(
		`Get "https://10.0.0.1/api": tls: failed to verify certificate: x509: certificate has expired or is not yet valid`,
	))

	want := `context "old" lists no resource types: Get "https://10.0.0.1/api": tls: failed to verify certificate: x509: certificate has expired or is not yet valid`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestAFailureWithoutATrustProblemNamesNoReason(t *testing.T) {
	if got := untrustedCertificate(errors.New("connection refused")); got != "" {
		t.Fatalf("reason = %q, want none", got)
	}
	if got := untrustedCertificate(nil); got != "" {
		t.Fatalf("reason = %q, want none", got)
	}
}
