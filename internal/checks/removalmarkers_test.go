package checks

import (
	"strings"
	"testing"
)

func rowsFor(t *testing.T, groupVersion string) map[int]string {
	t.Helper()
	out := map[int]string{}
	for _, one := range removals() {
		if one.groupVersion != groupVersion {
			continue
		}
		out[one.minor] = one.kinds
	}
	return out
}

func TestAGroupVersionCarriesEveryMinorItsKindsWereRemovedAt(t *testing.T) {
	cases := []struct {
		groupVersion string
		minor        int
		kinds        string
	}{
		{groupVersion: "networking.k8s.io/v1beta1", minor: 22, kinds: "Ingress, IngressClass"},
		{groupVersion: "networking.k8s.io/v1beta1", minor: 37, kinds: "IPAddress, ServiceCIDR"},
		{groupVersion: "coordination.k8s.io/v1beta1", minor: 22, kinds: "Lease"},
		{groupVersion: "coordination.k8s.io/v1beta1", minor: 39, kinds: "LeaseCandidate"},
		{groupVersion: "authentication.k8s.io/v1beta1", minor: 22, kinds: "TokenReview"},
		{groupVersion: "authentication.k8s.io/v1beta1", minor: 33, kinds: "SelfSubjectReview"},
	}

	for _, one := range cases {
		rows := rowsFor(t, one.groupVersion)
		if rows[one.minor] != one.kinds {
			t.Errorf("%s at 1.%d = %q, want %q", one.groupVersion, one.minor, rows[one.minor], one.kinds)
		}
	}
}

func TestTheMinorsTheHandWrittenTableGotWrong(t *testing.T) {
	coordination := rowsFor(t, "coordination.k8s.io/v1beta1")
	if _, wrong := coordination[25]; wrong {
		t.Error("coordination.k8s.io/v1beta1 still claims a removal at 1.25")
	}
	storage := rowsFor(t, "storage.k8s.io/v1beta1")
	if _, wrong := storage[25]; wrong {
		t.Error("storage.k8s.io/v1beta1 still claims a removal at 1.25")
	}
	if !strings.Contains(storage[22], "CSIDriver") {
		t.Errorf("storage.k8s.io/v1beta1 at 1.22 = %q, want the 1.22 kinds", storage[22])
	}
	if storage[27] != "CSIStorageCapacity" {
		t.Errorf("storage.k8s.io/v1beta1 at 1.27 = %q", storage[27])
	}
}

func TestTheGroupVersionsTheHandWrittenTableNeverHad(t *testing.T) {
	for _, groupVersion := range []string{"resource.k8s.io/v1beta1", "resource.k8s.io/v1beta2"} {
		if len(rowsFor(t, groupVersion)) == 0 {
			t.Errorf("%s carries no removals", groupVersion)
		}
	}
}

func TestALaterRemovalInAGroupVersionWithAnEarlierOneIsStillWarnedAbout(t *testing.T) {
	report := withFacts(t, Facts{
		ServerVersion:  "v1.36.0",
		ServedVersions: []string{"v1", "networking.k8s.io/v1beta1"},
	})

	finding := onlyFinding(t, report, "serves-a-removed-api")
	if !strings.Contains(finding.Detail, "IPAddress") {
		t.Fatalf("detail was %q, want the kinds removed at 1.37", finding.Detail)
	}
	if !strings.Contains(finding.Detail, "removed in 1.37") {
		t.Fatalf("detail was %q, want the coming release named", finding.Detail)
	}
	if strings.Contains(finding.Detail, "Ingress") {
		t.Fatalf("detail was %q, want the 1.22 removal left out on a 1.36 cluster", finding.Detail)
	}
}

func TestEveryRowNamesAtLeastOneKind(t *testing.T) {
	for _, one := range removals() {
		if one.kinds == "" {
			t.Errorf("%s at 1.%d names no kind", one.groupVersion, one.minor)
		}
		if one.minor == 0 {
			t.Errorf("%s carries no removal minor", one.groupVersion)
		}
	}
}
