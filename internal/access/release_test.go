package access

import (
	"strings"
	"testing"

	authv1 "k8s.io/api/authorization/v1"

	"github.com/sophotechlabs/spinoza/internal/helm"
)

func askedAbout(auth *authorizer, verb, resource string) (authv1.ResourceAttributes, bool) {
	for _, one := range auth.questions() {
		if one.Verb != verb {
			continue
		}
		if one.Resource != resource {
			continue
		}
		return one, true
	}
	return authv1.ResourceAttributes{}, false
}

func TestAReleaseNothingIsRefusedForHoldsNothingBack(t *testing.T) {
	service := serviceFor(t, refusing(nil))

	result := service.ReviewRelease(t.Context(), "prod", helm.DriverSecret)

	if len(result.Refused) != 0 {
		t.Fatalf("refused = %v, want nothing held back", result.Refused)
	}
}

// Measured against helm, one refused verb at a time: an install writes the
// first release object and nothing else.
func TestInstallingNeedsToCreateTheReleaseObject(t *testing.T) {
	service := serviceFor(t, refusingVerb("create", "secrets", "no creating secrets"))

	got := reasons(service.ReviewRelease(t.Context(), "prod", helm.DriverSecret))

	if got[Install] != "no creating secrets" {
		t.Fatalf("install reason = %q, want the refused create", got[Install])
	}
}

func TestInstallingIsNotRefusedOverAnUpdate(t *testing.T) {
	service := serviceFor(t, refusingVerb("update", "secrets", "no updating secrets"))

	got := reasons(service.ReviewRelease(t.Context(), "prod", helm.DriverSecret))

	// An install has nothing to supersede, so it never updates anything.
	if _, refused := got[Install]; refused {
		t.Fatalf("install was withheld over an update it does not make: %v", got)
	}
}

func TestInstallingIsNotRefusedOverADelete(t *testing.T) {
	service := serviceFor(t, refusingVerb("delete", "secrets", "no deleting secrets"))

	got := reasons(service.ReviewRelease(t.Context(), "prod", helm.DriverSecret))

	if _, refused := got[Install]; refused {
		t.Fatalf("install was withheld over a delete it does not make: %v", got)
	}
}

// An upgrade writes the new revision and marks the one before it superseded, so
// either verb refused stops it.
func TestUpgradingNeedsBothTheCreateAndTheUpdate(t *testing.T) {
	create := serviceFor(t, refusingVerb("create", "secrets", "no creating secrets"))
	update := serviceFor(t, refusingVerb("update", "secrets", "no updating secrets"))

	fromCreate := reasons(create.ReviewRelease(t.Context(), "prod", helm.DriverSecret))
	fromUpdate := reasons(update.ReviewRelease(t.Context(), "prod", helm.DriverSecret))

	if fromCreate[Upgrade] != "no creating secrets" {
		t.Fatalf("upgrade reason = %q, want the refused create", fromCreate[Upgrade])
	}
	if fromUpdate[Upgrade] != "no updating secrets" {
		t.Fatalf("upgrade reason = %q, want the refused update", fromUpdate[Upgrade])
	}
}

func TestRollingBackNeedsBothTheCreateAndTheUpdate(t *testing.T) {
	create := serviceFor(t, refusingVerb("create", "secrets", "no creating secrets"))
	update := serviceFor(t, refusingVerb("update", "secrets", "no updating secrets"))

	fromCreate := reasons(create.ReviewRelease(t.Context(), "prod", helm.DriverSecret))
	fromUpdate := reasons(update.ReviewRelease(t.Context(), "prod", helm.DriverSecret))

	if fromCreate[Rollback] != "no creating secrets" {
		t.Fatalf("rollback reason = %q, want the refused create", fromCreate[Rollback])
	}
	if fromUpdate[Rollback] != "no updating secrets" {
		t.Fatalf("rollback reason = %q, want the refused update", fromUpdate[Rollback])
	}
}

// The create comes first, and a user who may do neither is told about the step
// that stops them first rather than about all of them.
func TestAnUpgradeIsRefusedByTheFirstVerbItNeeds(t *testing.T) {
	both := refusing(map[string]string{})
	both.refuse["create  secrets "] = "no creating secrets"
	both.refuse["update  secrets "] = "no updating secrets"
	service := serviceFor(t, both)

	got := reasons(service.ReviewRelease(t.Context(), "prod", helm.DriverSecret))

	if got[Upgrade] != "no creating secrets" {
		t.Fatalf("upgrade reason = %q, want the first requirement that failed", got[Upgrade])
	}
}

// An uninstall purges the release objects. It writes nothing first, so only the
// delete matters.
func TestUninstallingNeedsToDeleteTheReleaseObject(t *testing.T) {
	service := serviceFor(t, refusingVerb("delete", "secrets", "no deleting secrets"))

	got := reasons(service.ReviewRelease(t.Context(), "prod", helm.DriverSecret))

	if got[Uninstall] != "no deleting secrets" {
		t.Fatalf("uninstall reason = %q, want the refused delete", got[Uninstall])
	}
	for _, name := range []string{Install, Upgrade, Rollback} {
		if _, refused := got[name]; refused {
			t.Fatalf("%s was withheld over a delete it does not make: %v", name, got)
		}
	}
}

func TestUninstallingIsNotRefusedOverACreate(t *testing.T) {
	service := serviceFor(t, refusingVerb("create", "secrets", "no creating secrets"))

	got := reasons(service.ReviewRelease(t.Context(), "prod", helm.DriverSecret))

	if _, refused := got[Uninstall]; refused {
		t.Fatalf("uninstall was withheld over a create it does not make: %v", got)
	}
}

// A release kept in configmaps is the same questions about a different kind.
func TestAReleaseKeptInConfigMapsIsAskedAboutConfigMaps(t *testing.T) {
	auth := refusing(map[string]string{"create  configmaps ": "no creating configmaps"})
	service := serviceFor(t, auth)

	got := reasons(service.ReviewRelease(t.Context(), "prod", helm.DriverConfigMap))

	if got[Install] != "no creating configmaps" {
		t.Fatalf("install reason = %q, want the refused configmap create", got[Install])
	}
	if _, asked := askedAbout(auth, "create", "secrets"); asked {
		t.Fatal("a configmap release was asked about secrets")
	}
}

// A driver nobody knows is treated as the one helm falls back to, which is
// better than asking about nothing at all.
func TestAnUnknownDriverIsAskedAboutAsASecret(t *testing.T) {
	auth := refusing(nil)
	service := serviceFor(t, auth)

	service.ReviewRelease(t.Context(), "prod", "sql")

	if _, asked := askedAbout(auth, "create", "secrets"); !asked {
		t.Fatalf("asked %v, want the questions helm's own default would need", auth.questions())
	}
}

// The question is about the namespace, not about one object in it: the revision
// an action is about to write does not exist yet and has no name to ask about.
func TestAReleaseIsAskedAboutByNamespaceAlone(t *testing.T) {
	auth := refusing(nil)
	service := serviceFor(t, auth)

	service.ReviewRelease(t.Context(), "prod", helm.DriverSecret)

	for _, one := range auth.questions() {
		if one.Namespace != "prod" {
			t.Fatalf("asked in %q, want the release's namespace", one.Namespace)
		}
		if one.Name != "" {
			t.Fatalf("asked about %q, want the kind in that namespace", one.Name)
		}
	}
}

// Three verbs, four buttons: the create an install needs is the same question
// as the create an upgrade needs.
func TestAReleaseIsThreeQuestions(t *testing.T) {
	auth := refusing(nil)
	service := serviceFor(t, auth)

	service.ReviewRelease(t.Context(), "prod", helm.DriverSecret)

	if auth.count() != 3 {
		t.Fatalf("asked %d questions: %v", auth.count(), auth.questions())
	}
}

func TestAnApiserverThatWillNotAnswerLeavesEveryHelmButtonAlone(t *testing.T) {
	service := serviceFor(t, &authorizer{broken: true})

	result := service.ReviewRelease(t.Context(), "prod", helm.DriverSecret)

	if len(result.Refused) != 0 {
		t.Fatalf("refused = %v; a failed check must not disable anything", result.Refused)
	}
}

func TestAHelmRefusalWithoutAReasonStillSaysWhat(t *testing.T) {
	service := serviceFor(t, refusingVerb("delete", "secrets", ""))

	got := reasons(service.ReviewRelease(t.Context(), "prod", helm.DriverSecret))

	if !strings.Contains(got[Uninstall], "you may not delete secrets") {
		t.Fatalf("uninstall reason = %q, want a sentence when the cluster gave none", got[Uninstall])
	}
}
