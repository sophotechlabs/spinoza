package checks

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const privilegedCheck = "privileged-containers"

func privilegedDeployment(name string) *unstructured.Unstructured {
	return deployment(name, podSpec(container("app", map[string]any{
		"securityContext": map[string]any{"privileged": true},
	})))
}

func inNamespace(obj *unstructured.Unstructured, namespace string) *unstructured.Unstructured {
	obj.SetNamespace(namespace)
	return obj
}

func withMutes(t *testing.T, mutes []Mute, objects ...*unstructured.Unstructured) api.CheckReport {
	t.Helper()
	keep := wholeCluster()
	keep.Mutes = mutes
	return Run(t.Context(), newLister(objects...), descriptors(), api.Metrics{}, keep, 0)
}

func deploymentRef(name, namespace string) string {
	return RefKey(api.ObjectRef{
		Group: "apps", Version: "v1", Resource: "deployments", Namespace: namespace, Name: name,
	})
}

// what a mute silences

func TestAMuteOnOneObjectLeavesTheOthersAlone(t *testing.T) {
	report := withMutes(t,
		[]Mute{{Check: privilegedCheck, Ref: deploymentRef("api", testNamespace), Reason: "it is the node agent"}},
		privilegedDeployment("api"), privilegedDeployment("web"))

	group := groupNamed(t, report, privilegedCheck)
	if group.Total != 1 {
		t.Fatalf("reported %d findings, want the one that was not muted", group.Total)
	}
	if group.Muted != 1 {
		t.Fatalf("counted %d muted, want the one that was", group.Muted)
	}
}

func TestAMuteOnANamespaceSilencesEverythingInIt(t *testing.T) {
	report := withMutes(t,
		[]Mute{{Check: privilegedCheck, Namespace: "kube-system"}},
		inNamespace(privilegedDeployment("agent"), "kube-system"),
		inNamespace(privilegedDeployment("csi"), "kube-system"),
		privilegedDeployment("api"))

	group := groupNamed(t, report, privilegedCheck)
	if group.Total != 1 || group.Muted != 2 {
		t.Fatalf("reported %d findings and %d muted", group.Total, group.Muted)
	}
}

func TestAMuteNamingNeitherSilencesTheCheckEverywhere(t *testing.T) {
	report := withMutes(t,
		[]Mute{{Check: privilegedCheck}},
		privilegedDeployment("api"), inNamespace(privilegedDeployment("agent"), "kube-system"))

	group := groupNamed(t, report, privilegedCheck)
	if group.Total != 0 || group.Muted != 2 {
		t.Fatalf("reported %d findings and %d muted", group.Total, group.Muted)
	}
}

func TestAMuteOnOneCheckLeavesTheOthersSaying(t *testing.T) {
	report := withMutes(t,
		[]Mute{{Check: privilegedCheck, Ref: deploymentRef("api", testNamespace)}},
		privilegedDeployment("api"))

	if findingCount(t, report, "limits-missing") == 0 {
		t.Fatal("muting one check silenced another")
	}
}

// what a mute is never allowed to do

func TestAMutedFindingIsCountedRatherThanForgotten(t *testing.T) {
	report := withMutes(t,
		[]Mute{{Check: privilegedCheck, Ref: deploymentRef("api", testNamespace)}},
		privilegedDeployment("api"))

	group := groupNamed(t, report, privilegedCheck)
	if group.Muted != 1 {
		t.Fatal("a muted finding disappeared without being counted, so nobody can find it again")
	}
}

func TestAskingForTheMutedOnesBringsThemBackWithTheirReason(t *testing.T) {
	keep := wholeCluster()
	keep.Mutes = []Mute{{
		Check: privilegedCheck, Ref: deploymentRef("api", testNamespace), Reason: "the node agent needs it",
	}}
	keep.ShowMuted = true

	report := Run(t.Context(), newLister(privilegedDeployment("api")), descriptors(), api.Metrics{}, keep, 0)

	finding := onlyFinding(t, report, privilegedCheck)
	if !finding.Muted {
		t.Fatal("a muted finding came back without saying it was muted")
	}
	if !strings.Contains(finding.Reason, "node agent") {
		t.Fatalf("reason was %q, want what was said at the time", finding.Reason)
	}
}

func TestAMutedFindingSaysWhichMuteSilencedIt(t *testing.T) {
	cases := []struct {
		name  string
		mute  Mute
		scope string
	}{
		{"one object", Mute{Check: privilegedCheck, Ref: deploymentRef("api", testNamespace)}, ScopeObject},
		{"a namespace", Mute{Check: privilegedCheck, Namespace: testNamespace}, ScopeNamespace},
		{"the check", Mute{Check: privilegedCheck}, ScopeCheck},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			keep := wholeCluster()
			keep.Mutes = []Mute{tc.mute}
			keep.ShowMuted = true

			report := Run(t.Context(), newLister(privilegedDeployment("api")),
				descriptors(), api.Metrics{}, keep, 0)

			if got := onlyFinding(t, report, privilegedCheck).MutedBy; got != tc.scope {
				t.Fatalf("the finding said it was muted by %q, want %q", got, tc.scope)
			}
		})
	}
}

func TestAFindingNobodyMutedNamesNoMute(t *testing.T) {
	report := report(t, privilegedDeployment("api"))

	if got := onlyFinding(t, report, privilegedCheck).MutedBy; got != "" {
		t.Fatalf("an unmuted finding said it was muted by %q", got)
	}
}

// what the settings store is allowed to hold

func TestMutesAreReadForOneClusterOnly(t *testing.T) {
	raw := `{"https://one":[{"check":"a"}],"https://two":[{"check":"b"}]}`

	mine := ParseMutes(raw, "https://one")

	if len(mine) != 1 || mine[0].Check != "a" {
		t.Fatalf("read %v, want only the first cluster's", mine)
	}
}

func TestAStoreHoldingNothingUsefulYieldsNoMutes(t *testing.T) {
	for _, raw := range []string{"", "   ", "not json", `["a list"]`, `{"c":[{"reason":"no check"}]}`} {
		if all := AllMutes(raw); len(all) != 0 {
			t.Fatalf("%q yielded %v", raw, all)
		}
	}
}

func TestMutesSurviveBeingWrittenAndReadBack(t *testing.T) {
	all := map[string][]Mute{"https://one": {{Check: "a", Namespace: "kube-system", Reason: "known"}}}

	back := AllMutes(EncodeMutes(all))

	if len(back["https://one"]) != 1 || back["https://one"][0].Reason != "known" {
		t.Fatalf("read back %v", back)
	}
}

func TestTwoMutesAreTheSameWhenTheySilenceTheSameThing(t *testing.T) {
	one := Mute{Check: "a", Namespace: "kube-system", Reason: "written on Monday"}
	other := Mute{Check: "a", Namespace: "kube-system", Reason: "written on Friday"}

	if !SameMute(one, other) {
		t.Fatal("the same mute written twice was read as two")
	}
	if SameMute(one, Mute{Check: "a", Namespace: "flux-system"}) {
		t.Fatal("two namespaces were read as one mute")
	}
}
