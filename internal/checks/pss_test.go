package checks

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	psapi "k8s.io/pod-security-admission/api"
	"k8s.io/pod-security-admission/policy"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func reportAt(t *testing.T, serverVersion string, objects ...*unstructured.Unstructured) api.CheckReport {
	t.Helper()
	lister := newLister(objects...)
	lister.facts = Facts{ServerVersion: serverVersion}
	return Run(t.Context(), lister, descriptors(), api.Metrics{}, wholeCluster(), 0)
}

func probing() *unstructured.Unstructured {
	return deployment("api", podSpec(container("app", map[string]any{"livenessProbe": probeAimedAt("10.0.0.9")})))
}

// which upstream controls the registry answers for

func TestEveryUpstreamControlIsAnsweredByExactlyOneRule(t *testing.T) {
	answered := map[policy.CheckID][]string{}
	for _, entry := range registry() {
		if entry.upstream == "" {
			continue
		}
		answered[entry.upstream] = append(answered[entry.upstream], entry.id)
	}
	for _, control := range policy.DefaultChecks() {
		if rules := answered[control.ID]; len(rules) != 1 {
			t.Fatalf("upstream control %s is answered by %v, want exactly one rule", control.ID, rules)
		}
		delete(answered, control.ID)
	}
	for id, rules := range answered {
		t.Fatalf("%v name an upstream control %s that upstream does not ship", rules, id)
	}
}

func TestARuleDecidedUpstreamCarriesTheLabelOfItsLevelFirst(t *testing.T) {
	loaded, err := upstream()
	if err != nil {
		t.Fatal(err)
	}
	found := report(t)
	for _, entry := range registry() {
		if entry.upstream == "" {
			continue
		}
		want := pssLabelOf(loaded.controls[entry.upstream].level)
		group := groupNamed(t, found, entry.id)
		if len(group.Frameworks) == 0 || group.Frameworks[0] != want {
			t.Fatalf("%s carries %v, want %s first from upstream's %s", entry.id, group.Frameworks, want, entry.upstream)
		}
		for _, label := range group.Frameworks[1:] {
			if label == pssBaseline || label == pssRestricted {
				t.Fatalf("%s still carries a hand-typed PSS label %q", entry.id, label)
			}
		}
	}
}

func TestARuleUpstreamAloneKnowsSpeaksInUpstreamsWords(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"runAsUser": int64(0),
	})))))

	detail := onlyFinding(t, found, "run-as-user-zero").Detail
	want := `runAsUser=0 (container "app" must not set runAsUser=0)`
	if detail != want {
		t.Fatalf("detail = %q, want %q", detail, want)
	}
}

// what the cluster's version changes

func TestASysctlUpstreamAllowedLaterIsJudgedByTheClusterVersion(t *testing.T) {
	workload := deployment("api", podSpecWith(map[string]any{
		"securityContext": map[string]any{
			"sysctls": []any{map[string]any{"name": "net.ipv4.tcp_rmem", "value": "4096 87380 6291456"}},
		},
	}, container("app", nil)))

	if findingCount(t, reportAt(t, "v1.31.0", workload), "unsafe-sysctls") != 1 {
		t.Fatal("net.ipv4.tcp_rmem was not reported on a 1.31 cluster, where baseline forbids it")
	}
	if findingCount(t, reportAt(t, "v1.32.0", workload), "unsafe-sysctls") != 0 {
		t.Fatal("net.ipv4.tcp_rmem was reported on a 1.32 cluster, where baseline allows it")
	}
}

func TestAnUnmaskedProcMountInsideAUserNamespaceIsAllowedFrom135(t *testing.T) {
	workload := deployment("api", podSpecWith(map[string]any{"hostUsers": false},
		container("app", withSecurity(map[string]any{"procMount": procMountUnmask}))))

	if findingCount(t, reportAt(t, "v1.34.0", workload), "proc-mount-unmasked") != 1 {
		t.Fatal("an unmasked /proc was not reported on 1.34, before upstream relaxed it for user namespaces")
	}
	if findingCount(t, reportAt(t, "v1.35.0", workload), "proc-mount-unmasked") != 0 {
		t.Fatal("an unmasked /proc inside a user namespace was reported on 1.35, where upstream allows it")
	}
	if findingCount(t, reportAt(t, "v1.35.0", workload), "proc-mount-in-user-namespace") != 1 {
		t.Fatal("on 1.35 the restricted rule did not pick up what baseline stopped reporting")
	}
	if findingCount(t, reportAt(t, "v1.34.0", workload), "proc-mount-in-user-namespace") != 0 {
		t.Fatal("the restricted rule fired on 1.34, before upstream ships it")
	}

	plain := deployment("api", podSpec(container("app", withSecurity(map[string]any{"procMount": procMountUnmask}))))
	if findingCount(t, reportAt(t, "v1.35.0", plain), "proc-mount-unmasked") != 1 {
		t.Fatal("outside a user namespace the baseline rule stopped reporting an unmasked /proc")
	}
	if findingCount(t, reportAt(t, "v1.35.0", plain), "proc-mount-in-user-namespace") != 0 {
		t.Fatal("outside a user namespace the restricted rule doubled the baseline finding")
	}
}

func TestAnSELinuxTypeUpstreamAllowedFrom131IsJudgedByTheClusterVersion(t *testing.T) {
	workload := deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"seLinuxOptions": map[string]any{"type": "container_engine_t"},
	}))))

	if findingCount(t, reportAt(t, "v1.30.0", workload), "selinux-options-set") != 1 {
		t.Fatal("container_engine_t was not reported on 1.30, where upstream did not allow it yet")
	}
	if findingCount(t, reportAt(t, "v1.31.0", workload), "selinux-options-set") != 0 {
		t.Fatal("container_engine_t was reported on 1.31, where upstream allows it")
	}
}

func TestAControlUpstreamAppliesLaterStandsDownWithoutItsLabel(t *testing.T) {
	early := groupNamed(t, reportAt(t, "v1.33.0", probing()), "probe-host-set")
	if early.Skipped != "not audited: upstream applies this from v1.34 and this cluster reports v1.33" {
		t.Fatalf("skipped = %q", early.Skipped)
	}
	if len(early.Frameworks) != 0 {
		t.Fatalf("a control that does not apply yet still carries %v", early.Frameworks)
	}

	late := groupNamed(t, reportAt(t, "v1.34.0", probing()), "probe-host-set")
	if late.Skipped != "" || len(late.Findings) != 1 {
		t.Fatalf("on 1.34 skipped = %q with %d findings, want the probe host reported", late.Skipped, len(late.Findings))
	}
	if len(late.Frameworks) != 1 || late.Frameworks[0] != pssBaseline {
		t.Fatalf("on 1.34 the rule carries %v, want %s", late.Frameworks, pssBaseline)
	}
	if !strings.Contains(late.Findings[0].Detail, `"10.0.0.9"`) {
		t.Fatalf("detail = %q, want the host named", late.Findings[0].Detail)
	}
}

func TestAServerVersionNobodyCanReadIsJudgedAtUpstreamsLatest(t *testing.T) {
	if findingCount(t, reportAt(t, "unknown", probing()), "probe-host-set") != 1 {
		t.Fatal("an unreadable server version did not fall back to upstream's latest policy")
	}
}

// what upstream cannot decide

func TestAPodTemplateUpstreamCannotReadIsJudgedByHand(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"privileged": true,
		"runAsUser":  "zero",
	})))))

	if onlyFinding(t, found, "privileged-containers").Detail != "securityContext.privileged is true" {
		t.Fatal("a template upstream could not decode lost the hand-written finding")
	}
}

func TestAnUpstreamPolicyThatWillNotLoadStandsTheBackedRulesDown(t *testing.T) {
	broken := policy.DefaultChecks()
	broken = append(broken, broken[0])
	_, err := upstreamFrom(broken)
	if err == nil {
		t.Fatal("a policy with a duplicated control loaded")
	}
	sc := scan{upstream: &upstreamScan{version: psapi.LatestVersion(), err: err, pods: map[string]upstreamPod{}}}

	backed := check{upstream: "privileged"}
	if down := backed.standsDown(sc); !strings.HasPrefix(down, upstreamUnloaded) {
		t.Fatalf("a backed rule stood down with %q", down)
	}
	enforced := check{enforced: true, find: enforcedLevelRejects}
	if down := enforced.standsDown(sc); !strings.HasPrefix(down, upstreamUnloaded) {
		t.Fatalf("the enforced-level rule stood down with %q", down)
	}
	if verdicts := enforced.findings(sc); len(verdicts) != 0 {
		t.Fatalf("an unloaded policy still produced %d findings", len(verdicts))
	}
}

func TestAControlUpstreamDoesNotShipStandsDown(t *testing.T) {
	sc := scan{upstream: newUpstreamScan("v1.36.0")}
	want := "not audited: upstream has no control named no-such-control"
	if down := (check{upstream: "no-such-control"}).standsDown(sc); down != want {
		t.Fatalf("stood down with %q, want %q", down, want)
	}
}

// the level a namespace enforces

func TestTheEnforcedLevelHonoursTheVersionLabel(t *testing.T) {
	pinnedEarly := namespaceObj(testNamespace, map[string]any{
		enforceLabel: profileBaseline, psapi.EnforceVersionLabel: "v1.33",
	})
	if findingCount(t, report(t, pinnedEarly, probing()), "pod-security-would-reject") != 0 {
		t.Fatal("a namespace pinned to baseline v1.33 rejected a probe host that v1.33 allows")
	}

	current := namespaceObj(testNamespace, map[string]any{enforceLabel: profileBaseline})
	detail := onlyFinding(t, report(t, current, probing()), "pod-security-would-reject").Detail
	if detail != "the namespace enforces baseline and this breaks probe or lifecycle host" {
		t.Fatalf("detail = %q", detail)
	}

	pinnedLate := namespaceObj(testNamespace, map[string]any{
		enforceLabel: profileBaseline, psapi.EnforceVersionLabel: "v1.34",
	})
	detail = onlyFinding(t, report(t, pinnedLate, probing()), "pod-security-would-reject").Detail
	if detail != "the namespace enforces baseline as of v1.34 and this breaks probe or lifecycle host" {
		t.Fatalf("detail = %q", detail)
	}
}

func TestALevelLabelUpstreamCannotReadIsEnforcedAsRestricted(t *testing.T) {
	found := report(t,
		namespaceObj(testNamespace, map[string]any{enforceLabel: "strict"}),
		deployment("api", podSpec(container("app", nil))))

	detail := onlyFinding(t, found, "pod-security-would-reject").Detail
	want := "the namespace enforces restricted, which is what upstream falls back to for a label it cannot read, and this breaks "
	if !strings.HasPrefix(detail, want) {
		t.Fatalf("detail = %q, want it to start with %q", detail, want)
	}
}

func TestTheEnforcedLevelNamesEveryControlThePodBreaks(t *testing.T) {
	found := report(t,
		namespaceObj(testNamespace, map[string]any{enforceLabel: profileStrict}),
		deployment("api", podSpec(container("app", nil))))

	detail := onlyFinding(t, found, "pod-security-would-reject").Detail
	want := "the namespace enforces restricted and this breaks allowPrivilegeEscalation != false, " +
		"unrestricted capabilities, runAsNonRoot != true, seccompProfile"
	if detail != want {
		t.Fatalf("detail = %q, want %q", detail, want)
	}
}

// what upstream is more lenient about than the hand-written rule was

func TestABaselineCapabilityAddedIsNotDangerous(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"capabilities": map[string]any{"add": []any{"CHOWN"}},
	})))))

	if findingCount(t, found, "dangerous-capabilities") != 0 {
		t.Fatal("CHOWN, which baseline allows, was reported as dangerous")
	}
}

func TestAHostPathLeftToASiblingRuleIsNotReportedTwice(t *testing.T) {
	found := report(t, deployment("api", podSpecWith(map[string]any{
		"volumes": []any{hostPathVolumeNamed("sock", "/var/run/docker.sock")},
	}, container("app", nil))))

	if findingCount(t, found, "host-path-volume") != 0 {
		t.Fatal("upstream's hostPath verdict was added on top of the socket rule's own finding")
	}
	if findingCount(t, found, "runtime-socket-mounted") != 1 {
		t.Fatal("the socket rule stopped reporting the socket")
	}
}
