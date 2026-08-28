package access

import (
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func podRows(names ...string) []api.ObjectRef {
	out := make([]api.ObjectRef, 0, len(names))
	for _, name := range names {
		out = append(out, api.ObjectRef{Version: "v1", Resource: "pods", Namespace: "prod", Name: name})
	}
	return out
}

func nodeRows(names ...string) []api.ObjectRef {
	out := make([]api.ObjectRef, 0, len(names))
	for _, name := range names {
		out = append(out, api.ObjectRef{Version: "v1", Resource: "nodes", Name: name})
	}
	return out
}

func places(result api.BulkAccess) []int {
	out := make([]int, 0, len(result.Refused))
	for _, row := range result.Refused {
		out = append(out, row.At)
	}
	return out
}

func TestASelectionThatIsPermittedRefusesNothing(t *testing.T) {
	service := serviceFor(t, refusing(nil))

	result := service.ReviewEach(t.Context(), Delete, podRows("web-0", "web-1"))

	if len(result.Refused) != 0 {
		t.Fatalf("refused = %v, want nothing held back", result.Refused)
	}
}

func TestOnlyTheRefusedRowsAreNamed(t *testing.T) {
	auth := refusing(nil)
	auth.byName = map[string]string{"web-1": "no deleting web-1"}
	service := serviceFor(t, auth)

	result := service.ReviewEach(t.Context(), Delete, podRows("web-0", "web-1", "web-2"))

	if len(result.Refused) != 1 {
		t.Fatalf("refused = %v, want only the named row", result.Refused)
	}
	if result.Refused[0].At != 1 {
		t.Fatalf("refused row %d, want the second one", result.Refused[0].At)
	}
	if result.Refused[0].Reason != "no deleting web-1" {
		t.Fatalf("reason = %q, want the cluster's own words", result.Refused[0].Reason)
	}
}

func TestEveryRowIsRefusedWhenTheKindIs(t *testing.T) {
	service := serviceFor(t, refusing(map[string]string{"delete  pods ": ""}))

	result := service.ReviewEach(t.Context(), Delete, podRows("web-0", "web-1"))

	if len(places(result)) != 2 {
		t.Fatalf("refused = %v, want both rows", result.Refused)
	}
	if result.Refused[0].Reason != "you may not delete pods here" {
		t.Fatalf("reason = %q", result.Refused[0].Reason)
	}
}

func TestOneQuestionIsPutPerObject(t *testing.T) {
	auth := refusing(nil)
	service := serviceFor(t, auth)

	service.ReviewEach(t.Context(), Delete, podRows("web-0", "web-1", "web-2"))

	if auth.count() != 3 {
		t.Fatalf("asked %d questions about 3 objects: %v", auth.count(), auth.questions())
	}
	for _, one := range auth.questions() {
		if one.Verb != "delete" || one.Resource != "pods" {
			t.Fatalf("asked %+v, want the delete each row needs", one)
		}
	}
}

func TestEachRowIsAskedAboutByName(t *testing.T) {
	auth := refusing(nil)
	service := serviceFor(t, auth)

	service.ReviewEach(t.Context(), Delete, podRows("web-0", "web-1"))

	seen := map[string]bool{}
	for _, one := range auth.questions() {
		seen[one.Name] = true
	}
	if !seen["web-0"] || !seen["web-1"] {
		t.Fatalf("asked about %v, want each row by its own name", seen)
	}
}

func TestARowIsAskedAboutInItsOwnNamespace(t *testing.T) {
	auth := refusing(nil)
	service := serviceFor(t, auth)
	refs := podRows("web-0")
	refs = append(refs, api.ObjectRef{
		Version:   "v1",
		Resource:  "pods",
		Namespace: "kube-system",
		Name:      "coredns",
	})

	service.ReviewEach(t.Context(), Delete, refs)

	seen := map[string]string{}
	for _, one := range auth.questions() {
		seen[one.Name] = one.Namespace
	}
	if seen["web-0"] != "prod" || seen["coredns"] != "kube-system" {
		t.Fatalf("asked %v, want each row where it lives", seen)
	}
}

func TestACapabilityAKindDoesNotHaveIsNeverAskedAbout(t *testing.T) {
	auth := refusing(nil)
	service := serviceFor(t, auth)

	result := service.ReviewEach(t.Context(), Restart, podRows("web-0", "web-1"))

	if auth.count() != 0 {
		t.Fatalf("asked %v about restarting pods, which is not something a pod does", auth.questions())
	}
	if len(result.Refused) != 0 {
		t.Fatalf("refused = %v; a capability a kind lacks is not a refusal", result.Refused)
	}
}

func TestRowsWithoutTheCapabilityDoNotShiftTheOthers(t *testing.T) {
	auth := refusing(nil)
	auth.byName = map[string]string{"web": "no restarting web"}
	service := serviceFor(t, auth)
	refs := podRows("pod-0")
	refs = append(refs, api.ObjectRef{
		Group:     "apps",
		Version:   "v1",
		Resource:  "deployments",
		Namespace: "prod",
		Name:      "web",
	})

	result := service.ReviewEach(t.Context(), Restart, refs)

	if len(result.Refused) != 1 || result.Refused[0].At != 1 {
		t.Fatalf("refused = %v, want the second row alone", result.Refused)
	}
}

func TestAQuestionSharedByTheSelectionIsPutOnce(t *testing.T) {
	auth := refusing(nil)
	service := serviceFor(t, auth)

	service.ReviewEach(t.Context(), Drain, nodeRows("node-1", "node-2", "node-3"))

	listings := 0
	for _, one := range auth.questions() {
		if one.Verb == "list" && one.Resource == "pods" {
			listings++
		}
	}
	if listings != 1 {
		t.Fatalf("asked to list pods %d times, want once for the whole selection", listings)
	}
}

func TestTheFirstRefusedRequirementIsTheReportedOne(t *testing.T) {
	service := serviceFor(t, refusing(map[string]string{"list  pods ": "no listing pods"}))

	result := service.ReviewEach(t.Context(), Drain, nodeRows("node-1", "node-2"))

	if len(result.Refused) != 2 {
		t.Fatalf("refused = %v, want both nodes", result.Refused)
	}
	for _, row := range result.Refused {
		if row.Reason != "no listing pods" {
			t.Fatalf("reason = %q, want the requirement that failed first", row.Reason)
		}
	}
}

func TestAnApiserverThatWillNotAnswerRefusesNoRow(t *testing.T) {
	service := serviceFor(t, &authorizer{broken: true})

	result := service.ReviewEach(t.Context(), Delete, podRows("web-0", "web-1"))

	if len(result.Refused) != 0 {
		t.Fatalf("refused = %v; a failed check must not stop anything", result.Refused)
	}
}

func TestAnEmptySelectionAsksNothing(t *testing.T) {
	auth := refusing(nil)
	service := serviceFor(t, auth)

	result := service.ReviewEach(t.Context(), Delete, nil)

	if auth.count() != 0 {
		t.Fatalf("asked %v about nothing at all", auth.questions())
	}
	if len(result.Refused) != 0 {
		t.Fatalf("refused = %v", result.Refused)
	}
}

func TestARefusalWithoutAReasonStillSaysWhatWasRefused(t *testing.T) {
	auth := refusing(nil)
	auth.byName = map[string]string{"web-0": ""}
	service := serviceFor(t, auth)

	result := service.ReviewEach(t.Context(), Delete, podRows("web-0"))

	if len(result.Refused) != 1 {
		t.Fatalf("refused = %v", result.Refused)
	}
	if !strings.Contains(result.Refused[0].Reason, "you may not delete pods") {
		t.Fatalf("reason = %q, want a sentence when the cluster gave none", result.Refused[0].Reason)
	}
}
