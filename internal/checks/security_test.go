package checks

import (
	"strings"
	"testing"
)

func TestPrivilegedContainerIsFlaggedWithAPatchThatTurnsItOff(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"privileged": true,
	})))))

	finding := onlyFinding(t, found, "privileged-containers")
	if finding.Detail != "securityContext.privileged is true" {
		t.Fatalf("detail was %q", finding.Detail)
	}
	if finding.Container != "app" {
		t.Fatalf("container was %q", finding.Container)
	}
	want := "spec:\n  template:\n    spec:\n      containers:\n        - name: app\n          securityContext:\n            privileged: false\n"
	if finding.Patch != want {
		t.Fatalf("patch was:\n%s", finding.Patch)
	}
}

func TestPrivilegedFalseIsNotAFinding(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"privileged": false,
	})))))

	if findingCount(t, found, "privileged-containers") != 0 {
		t.Fatal("privileged: false was reported as privileged")
	}
}

func TestPrivilegeEscalationIsFlaggedWhenUnsetAndWhenTrue(t *testing.T) {
	unset := report(t, deployment("api", podSpec(container("app", nil))))
	if !strings.Contains(onlyFinding(t, unset, "privilege-escalation").Detail, "unset") {
		t.Fatal("an unset allowPrivilegeEscalation was not reported as unset")
	}

	explicit := report(t, deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"allowPrivilegeEscalation": true,
	})))))
	if !strings.Contains(onlyFinding(t, explicit, "privilege-escalation").Detail, "is true") {
		t.Fatal("an explicit allowPrivilegeEscalation was not reported as true")
	}

	off := report(t, deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"allowPrivilegeEscalation": false,
	})))))
	if findingCount(t, off, "privilege-escalation") != 0 {
		t.Fatal("allowPrivilegeEscalation: false was reported")
	}
}

func TestHostNamespacesNamesEveryOneShared(t *testing.T) {
	spec := podSpec(container("app", nil))
	spec["hostPID"] = true
	spec["hostNetwork"] = true
	spec["hostIPC"] = false

	finding := onlyFinding(t, report(t, deployment("api", spec)), "host-namespaces")

	if finding.Detail != "shares the host namespaces: hostPID, hostNetwork" {
		t.Fatalf("detail was %q", finding.Detail)
	}
	if !strings.Contains(finding.Patch, "hostPID: false") {
		t.Fatalf("patch was:\n%s", finding.Patch)
	}
	if strings.Contains(finding.Patch, "hostIPC") {
		t.Fatalf("a namespace that was already false reached the patch:\n%s", finding.Patch)
	}
}

func TestAPodSharingNoHostNamespaceIsClean(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", nil))))

	if findingCount(t, found, "host-namespaces") != 0 {
		t.Fatal("a workload sharing nothing was reported")
	}
}

func TestAMountedRuntimeSocketIsFlagged(t *testing.T) {
	spec := podSpec(container("app", nil))
	spec["volumes"] = []any{
		map[string]any{"name": "config", "configMap": map[string]any{"name": "settings"}},
		map[string]any{"name": "sock", "hostPath": map[string]any{"path": "/var/run/docker.sock"}},
	}

	finding := onlyFinding(t, report(t, deployment("api", spec)), "runtime-socket-mounted")

	if finding.Detail != "volume sock mounts /var/run/docker.sock from the node" {
		t.Fatalf("detail was %q", finding.Detail)
	}
	if !strings.Contains(finding.Patch, "$patch: delete") {
		t.Fatalf("patch was:\n%s", finding.Patch)
	}
}

func TestAnUnrelatedHostPathIsNotARuntimeSocket(t *testing.T) {
	spec := podSpec(container("app", nil))
	spec["volumes"] = []any{
		map[string]any{"name": "data", "hostPath": map[string]any{"path": "/srv/data"}},
		map[string]any{"name": "broken", "hostPath": map[string]any{"path": int64(7)}},
		"not a volume",
	}

	if findingCount(t, report(t, deployment("api", spec)), "runtime-socket-mounted") != 0 {
		t.Fatal("an ordinary hostPath was read as a runtime socket")
	}
}

func TestRunAsRootReadsTheContainerThenThePod(t *testing.T) {
	spec := podSpec(container("app", nil))
	spec["securityContext"] = map[string]any{"runAsNonRoot": true}
	if findingCount(t, report(t, deployment("api", spec)), "run-as-root") != 0 {
		t.Fatal("runAsNonRoot on the pod did not reach the container")
	}

	override := podSpec(container("app", withSecurity(map[string]any{"runAsNonRoot": false})))
	override["securityContext"] = map[string]any{"runAsNonRoot": true}
	finding := onlyFinding(t, report(t, deployment("api", override)), "run-as-root")
	if !strings.Contains(finding.Detail, "is false") {
		t.Fatalf("detail was %q", finding.Detail)
	}
}

func TestRunAsUserZeroIsFlaggedAndANonZeroUserIsNot(t *testing.T) {
	root := report(t, deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"runAsUser": int64(0),
	})))))
	if onlyFinding(t, root, "run-as-root").Detail != "securityContext.runAsUser is 0" {
		t.Fatal("runAsUser 0 was not reported")
	}

	other := report(t, deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"runAsUser": int64(1000),
	})))))
	if findingCount(t, other, "run-as-root") != 0 {
		t.Fatal("a non-zero runAsUser was reported")
	}
}

func TestAnUnsetUserIsReportedAsUnset(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", nil))))

	if !strings.Contains(onlyFinding(t, found, "run-as-root").Detail, "unset") {
		t.Fatal("a container with no user was not reported as unset")
	}
}

func TestAFloatUserIsRead(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"runAsUser": float64(0),
	})))))

	if onlyFinding(t, found, "run-as-root").Detail != "securityContext.runAsUser is 0" {
		t.Fatal("a float runAsUser was not read")
	}
}

func TestAWritableRootFilesystemIsFlaggedUnsetAndFalse(t *testing.T) {
	unset := report(t, deployment("api", podSpec(container("app", nil))))
	if !strings.Contains(onlyFinding(t, unset, "writable-root-filesystem").Detail, "unset") {
		t.Fatal("an unset readOnlyRootFilesystem was not reported as unset")
	}

	off := report(t, deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"readOnlyRootFilesystem": false,
	})))))
	if !strings.Contains(onlyFinding(t, off, "writable-root-filesystem").Detail, "is false") {
		t.Fatal("readOnlyRootFilesystem: false was not reported")
	}

	on := report(t, deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"readOnlyRootFilesystem": true,
	})))))
	if findingCount(t, on, "writable-root-filesystem") != 0 {
		t.Fatal("a read-only root filesystem was reported")
	}
}

func TestAddedCapabilitiesAreListedExceptTheBaselineOne(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"capabilities": map[string]any{"add": []any{"SYS_ADMIN", "NET_BIND_SERVICE", "NET_ADMIN", 7}},
	})))))

	finding := onlyFinding(t, found, "dangerous-capabilities")
	if finding.Detail != "securityContext.capabilities.add carries NET_ADMIN, SYS_ADMIN" {
		t.Fatalf("detail was %q", finding.Detail)
	}
	if !strings.Contains(finding.Patch, "- ALL") {
		t.Fatalf("patch was:\n%s", finding.Patch)
	}
}

func TestOnlyTheBaselineCapabilityIsNotAFinding(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"capabilities": map[string]any{"add": []any{"NET_BIND_SERVICE"}, "drop": []any{"ALL"}},
	})))))

	if findingCount(t, found, "dangerous-capabilities") != 0 {
		t.Fatal("NET_BIND_SERVICE alone was reported as dangerous")
	}
}

func TestACapabilitiesBlockWithoutAnAddListIsIgnored(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"capabilities": map[string]any{"drop": []any{"ALL"}},
	})))))

	if findingCount(t, found, "dangerous-capabilities") != 0 {
		t.Fatal("a drop-only capabilities block was reported")
	}
}

func TestAContainerWithNoSecurityContextIsStillRead(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", map[string]any{
		"securityContext": "not a map",
	}))))

	if findingCount(t, found, "privileged-containers") != 0 {
		t.Fatal("an unreadable securityContext was treated as privileged")
	}
	if findingCount(t, found, "dangerous-capabilities") != 0 {
		t.Fatal("an unreadable securityContext produced capabilities")
	}
}
