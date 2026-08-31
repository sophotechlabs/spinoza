package checks

import (
	"maps"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func podSpecWith(fields map[string]any, containers ...map[string]any) map[string]any {
	spec := podSpec(containers...)
	maps.Copy(spec, fields)
	return spec
}

func hostPathVolumeNamed(name, path string) map[string]any {
	return map[string]any{"name": name, "hostPath": map[string]any{"path": path}}
}

func mount(name, path string, readOnly bool) map[string]any {
	return map[string]any{"name": name, "mountPath": path, "readOnly": readOnly}
}

func annotated(obj *unstructured.Unstructured, key, value string) *unstructured.Unstructured {
	meta, ok := obj.Object["metadata"].(map[string]any)
	if !ok {
		return obj
	}
	meta["annotations"] = map[string]any{key: value}
	return obj
}

func hardened(fields map[string]any) map[string]any {
	base := map[string]any{
		"seccompProfile":           map[string]any{"type": runtimeDefault},
		"capabilities":             map[string]any{"drop": []any{dropAll}},
		"allowPrivilegeEscalation": false,
		"runAsNonRoot":             true,
		"readOnlyRootFilesystem":   true,
	}
	maps.Copy(base, fields)
	return base
}

func cleanPod() map[string]any {
	return podSpecWith(
		map[string]any{
			"automountServiceAccountToken": false,
			"serviceAccountName":           "api",
		},
		container("app", withSecurity(hardened(nil))),
	)
}

func TestEveryHardeningCheckFiresOnItsOwnFaultAndOnNothingElse(t *testing.T) {
	cases := []struct {
		id    string
		trips map[string]any
	}{
		{
			id: "seccomp-unset",
			trips: podSpecWith(nil, container("app", withSecurity(hardened(map[string]any{
				"seccompProfile": nil,
			})))),
		},
		{
			id: "seccomp-unconfined",
			trips: podSpecWith(nil, container("app", withSecurity(hardened(map[string]any{
				"seccompProfile": map[string]any{"type": unconfined},
			})))),
		},
		{
			id: "apparmor-unconfined",
			trips: podSpecWith(nil, container("app", withSecurity(hardened(map[string]any{
				"appArmorProfile": map[string]any{"type": unconfined},
			})))),
		},
		{
			id: "selinux-options-set",
			trips: podSpecWith(nil, container("app", withSecurity(hardened(map[string]any{
				"seLinuxOptions": map[string]any{"type": "spc_t"},
			})))),
		},
		{
			id: "proc-mount-unmasked",
			trips: podSpecWith(nil, container("app", withSecurity(hardened(map[string]any{
				"procMount": procMountUnmask,
			})))),
		},
		{
			id: "unsafe-sysctls",
			trips: podSpecWith(map[string]any{
				"securityContext": map[string]any{
					"sysctls": []any{map[string]any{"name": "net.core.somaxconn", "value": "1024"}},
				},
			}, container("app", withSecurity(hardened(nil)))),
		},
		{
			id: "host-ports",
			trips: podSpecWith(nil, container("app", map[string]any{
				"securityContext": hardened(nil),
				"ports":           []any{map[string]any{"containerPort": int64(8080), "hostPort": int64(8080)}},
			})),
		},
		{
			id: "host-path-volume",
			trips: podSpecWith(map[string]any{
				"volumes": []any{hostPathVolumeNamed("data", "/opt/data")},
			}, container("app", withSecurity(hardened(nil)))),
		},
		{
			id: "sensitive-host-path",
			trips: podSpecWith(map[string]any{
				"volumes": []any{hostPathVolumeNamed("etc", "/etc")},
			}, container("app", withSecurity(hardened(nil)))),
		},
		{
			id: "writable-host-mount",
			trips: podSpecWith(map[string]any{
				"volumes": []any{hostPathVolumeNamed("data", "/opt/data")},
			}, container("app", map[string]any{
				"securityContext": hardened(nil),
				"volumeMounts":    []any{mount("data", "/data", false)},
			})),
		},
		{
			id: "capabilities-not-dropped",
			trips: podSpecWith(nil, container("app", withSecurity(hardened(map[string]any{
				"capabilities": map[string]any{"drop": []any{"NET_ADMIN"}},
			})))),
		},
		{
			id: "net-raw-kept",
			trips: podSpecWith(nil, container("app", withSecurity(hardened(map[string]any{
				"capabilities": map[string]any{"drop": []any{"SYS_ADMIN"}},
			})))),
		},
		{
			id: "restricted-volume-types",
			trips: podSpecWith(map[string]any{
				"volumes": []any{map[string]any{"name": "src", "gitRepo": map[string]any{"repository": "git://x"}}},
			}, container("app", withSecurity(hardened(nil)))),
		},
		{
			id: "root-group",
			trips: podSpecWith(nil, container("app", withSecurity(hardened(map[string]any{
				"runAsGroup": int64(0),
			})))),
		},
		{
			id: "host-process",
			trips: podSpecWith(nil, container("app", withSecurity(hardened(map[string]any{
				"windowsOptions": map[string]any{"hostProcess": true},
			})))),
		},
		{
			id:    "automount-token",
			trips: podSpecWith(map[string]any{"serviceAccountName": "api"}, container("app", withSecurity(hardened(nil)))),
		},
		{
			id: "default-service-account",
			trips: podSpecWith(map[string]any{
				"automountServiceAccountToken": false,
				"serviceAccountName":           defaultNamespace,
			}, container("app", withSecurity(hardened(nil)))),
		},
	}

	registered := map[string]bool{}
	for _, entry := range hardeningChecks() {
		registered[entry.id] = true
	}
	if len(cases) != len(registered) {
		t.Fatalf("%d cases cover %d registered hardening checks", len(cases), len(registered))
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			if !registered[tc.id] {
				t.Fatalf("%s is not a registered hardening check", tc.id)
			}
			tripped := report(t, deployment("api", tc.trips))
			if findingCount(t, tripped, tc.id) == 0 {
				t.Fatalf("%s did not fire on the spec written to trip it", tc.id)
			}
			clean := report(t, deployment("api", cleanPod()))
			if findingCount(t, clean, tc.id) != 0 {
				t.Fatalf("%s fired on a hardened pod", tc.id)
			}
		})
	}
}

func TestSeccompSaysWhetherTheProfileIsMissingOrTurnedOff(t *testing.T) {
	missing := report(t, deployment("api", podSpec(container("app", nil))))
	if !strings.Contains(onlyFinding(t, missing, "seccomp-unset").Detail, "no securityContext.seccompProfile") {
		t.Fatal("an absent seccomp profile was not reported as absent")
	}

	off := report(t, deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"seccompProfile": map[string]any{"type": unconfined},
	})))))
	if !strings.Contains(onlyFinding(t, off, "seccomp-unconfined").Detail, "on the container") {
		t.Fatal("an Unconfined profile did not say it was set on the container")
	}
}

func TestAPodLevelSeccompProfileSatisfiesItsContainers(t *testing.T) {
	found := report(t, deployment("api", podSpecWith(map[string]any{
		"securityContext": map[string]any{"seccompProfile": map[string]any{"type": runtimeDefault}},
	}, container("app", nil))))

	if findingCount(t, found, "seccomp-unset") != 0 {
		t.Fatal("a profile set on the pod did not satisfy the container")
	}
}

func TestAContainerProfileOverridesAnUnconfinedPod(t *testing.T) {
	found := report(t, deployment("api", podSpecWith(map[string]any{
		"securityContext": map[string]any{"seccompProfile": map[string]any{"type": unconfined}},
	}, container("app", withSecurity(map[string]any{
		"seccompProfile": map[string]any{"type": runtimeDefault},
	})))))

	if findingCount(t, found, "seccomp-unconfined") != 0 {
		t.Fatal("the container's own RuntimeDefault did not override the pod's Unconfined")
	}
}

func TestTheLegacyAppArmorAnnotationIsRead(t *testing.T) {
	obj := annotated(deployment("api", podSpec(container("app", nil))), apparmorPrefix+"app", "unconfined")

	found := report(t, obj)

	if !strings.Contains(onlyFinding(t, found, "apparmor-unconfined").Detail, apparmorPrefix+"app") {
		t.Fatal("the legacy apparmor annotation was not read")
	}
}

func TestOnlyTheNarrowingSELinuxFieldsAreFlagged(t *testing.T) {
	level := report(t, deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"seLinuxOptions": map[string]any{"level": "s0:c1,c2"},
	})))))
	if findingCount(t, level, "selinux-options-set") != 0 {
		t.Fatal("setting only the SELinux level was reported")
	}

	widened := report(t, deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"seLinuxOptions": map[string]any{"role": "system_r", "type": "spc_t"},
	})))))
	if detail := onlyFinding(t, widened, "selinux-options-set").Detail; !strings.Contains(detail, "role, type") {
		t.Fatalf("detail was %q, want the fields it names", detail)
	}
}

func TestASafeSysctlIsNotReported(t *testing.T) {
	found := report(t, deployment("api", podSpecWith(map[string]any{
		"securityContext": map[string]any{
			"sysctls": []any{map[string]any{"name": "net.ipv4.ping_group_range", "value": "0 0"}},
		},
	}, container("app", nil))))

	if findingCount(t, found, "unsafe-sysctls") != 0 {
		t.Fatal("a sysctl on the kubelet's safe list was reported as unsafe")
	}
}

func TestSeveralUnsafeSysctlsAreNamedTogetherAndSorted(t *testing.T) {
	found := report(t, deployment("api", podSpecWith(map[string]any{
		"securityContext": map[string]any{
			"sysctls": []any{
				map[string]any{"name": "vm.max_map_count"},
				map[string]any{"name": "net.core.somaxconn"},
			},
		},
	}, container("app", nil))))

	detail := onlyFinding(t, found, "unsafe-sysctls").Detail
	if !strings.Contains(detail, "sysctls net.core.somaxconn, vm.max_map_count") {
		t.Fatalf("detail was %q, want both names in order", detail)
	}
}

func TestARuntimeSocketIsLeftToItsOwnCheck(t *testing.T) {
	found := report(t, deployment("api", podSpecWith(map[string]any{
		"volumes": []any{hostPathVolumeNamed("sock", "/var/run/docker.sock")},
	}, container("app", nil))))

	if findingCount(t, found, "runtime-socket-mounted") != 1 {
		t.Fatal("the runtime socket check did not report the socket")
	}
	if findingCount(t, found, "host-path-volume") != 0 {
		t.Fatal("the socket was reported twice, once as a plain host path")
	}
	if findingCount(t, found, "sensitive-host-path") != 0 {
		t.Fatal("the socket was reported twice, once as a sensitive path")
	}
}

func TestASensitivePathIsNotAlsoReportedAsAPlainHostPath(t *testing.T) {
	found := report(t, deployment("api", podSpecWith(map[string]any{
		"volumes": []any{hostPathVolumeNamed("etc", "/etc")},
	}, container("app", nil))))

	if findingCount(t, found, "sensitive-host-path") != 1 {
		t.Fatal("/etc was not reported as sensitive")
	}
	if findingCount(t, found, "host-path-volume") != 0 {
		t.Fatal("/etc was reported as a plain host path as well")
	}
}

func TestAReadOnlyHostMountIsNotReportedAsWritable(t *testing.T) {
	found := report(t, deployment("api", podSpecWith(map[string]any{
		"volumes": []any{hostPathVolumeNamed("data", "/opt/data")},
	}, container("app", map[string]any{
		"volumeMounts": []any{mount("data", "/data", true)},
	}))))

	if findingCount(t, found, "writable-host-mount") != 0 {
		t.Fatal("a readOnly mount was reported as writable")
	}
}

func TestDroppingAllSatisfiesBothCapabilityChecks(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"capabilities": map[string]any{"drop": []any{"all"}},
	})))))

	if findingCount(t, found, "capabilities-not-dropped") != 0 {
		t.Fatal("a lowercase drop of all was not accepted")
	}
	if findingCount(t, found, "net-raw-kept") != 0 {
		t.Fatal("dropping ALL did not satisfy the NET_RAW check")
	}
}

func TestDroppingOnlyNetRawLeavesTheDropAllCheckFiring(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"capabilities": map[string]any{"drop": []any{netRaw}},
	})))))

	if findingCount(t, found, "net-raw-kept") != 0 {
		t.Fatal("dropping NET_RAW did not satisfy its own check")
	}
	if findingCount(t, found, "capabilities-not-dropped") != 1 {
		t.Fatal("dropping only NET_RAW was accepted as dropping ALL")
	}
}

func TestAllowedVolumeTypesAreNotReported(t *testing.T) {
	found := report(t, deployment("api", podSpecWith(map[string]any{
		"volumes": []any{
			map[string]any{"name": "conf", "configMap": map[string]any{"name": "conf"}},
			map[string]any{"name": "tmp", "emptyDir": map[string]any{}},
		},
	}, container("app", nil))))

	if findingCount(t, found, "restricted-volume-types") != 0 {
		t.Fatal("configMap and emptyDir were reported as outside the restricted set")
	}
}

func TestARootGroupIsFoundOnThePodAsWellAsTheContainer(t *testing.T) {
	inherited := report(t, deployment("api", podSpecWith(map[string]any{
		"securityContext": map[string]any{"fsGroup": int64(0)},
	}, container("app", nil))))
	if !strings.Contains(onlyFinding(t, inherited, "root-group").Detail, "fsGroup") {
		t.Fatal("fsGroup 0 on the pod was not reported")
	}

	supplemental := report(t, deployment("api", podSpecWith(map[string]any{
		"securityContext": map[string]any{"supplementalGroups": []any{int64(0), int64(1000)}},
	}, container("app", nil))))
	if !strings.Contains(onlyFinding(t, supplemental, "root-group").Detail, "supplementalGroups") {
		t.Fatal("a root supplementalGroup was not reported")
	}
}

func TestANonRootGroupIsAccepted(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"runAsGroup": int64(1000),
	})))))

	if findingCount(t, found, "root-group") != 0 {
		t.Fatal("a non-zero runAsGroup was reported")
	}
}

func TestTheTokenCheckSaysWhetherItWasAskedForOrDefaulted(t *testing.T) {
	unset := report(t, deployment("api", podSpec(container("app", nil))))
	if !strings.Contains(onlyFinding(t, unset, "automount-token").Detail, "unset") {
		t.Fatal("an unset automount was not reported as unset")
	}

	asked := report(t, deployment("api", podSpecWith(map[string]any{
		"automountServiceAccountToken": true,
	}, container("app", nil))))
	if !strings.Contains(onlyFinding(t, asked, "automount-token").Detail, "is true") {
		t.Fatal("an explicit automount was not reported as explicit")
	}
}

func TestNamingAnAccountThatIsNotTheDefaultIsAccepted(t *testing.T) {
	found := report(t, deployment("api", podSpecWith(map[string]any{
		"serviceAccountName": "api",
	}, container("app", nil))))

	if findingCount(t, found, "default-service-account") != 0 {
		t.Fatal("a named service account was reported as the default")
	}
}

func TestListsHoldingSomethingOtherThanObjectsAreSkipped(t *testing.T) {
	found := report(t, deployment("api", podSpecWith(map[string]any{
		"securityContext": map[string]any{"sysctls": []any{"not-an-object"}},
		"volumes":         []any{"not-an-object"},
	}, container("app", map[string]any{
		"ports":        []any{"not-an-object"},
		"volumeMounts": []any{"not-an-object"},
		"securityContext": map[string]any{
			"capabilities": map[string]any{"drop": []any{int64(7)}},
		},
	}))))

	for _, id := range []string{"unsafe-sysctls", "host-ports", "host-path-volume", "writable-host-mount"} {
		if findingCount(t, found, id) != 0 {
			t.Fatalf("%s reported something from a list of non-objects", id)
		}
	}
	if findingCount(t, found, "capabilities-not-dropped") != 1 {
		t.Fatal("a drop list holding a number was read as dropping ALL")
	}
}

func TestAPortWithNoHostPortIsNotReported(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", map[string]any{
		"ports": []any{map[string]any{"containerPort": int64(8080)}},
	}))))

	if findingCount(t, found, "host-ports") != 0 {
		t.Fatal("a container port with no hostPort was reported")
	}
}

func TestASupplementalGroupDecodedAsAFloatIsStillRead(t *testing.T) {
	found := report(t, deployment("api", podSpecWith(map[string]any{
		"securityContext": map[string]any{"supplementalGroups": []any{float64(0)}},
	}, container("app", nil))))

	if findingCount(t, found, "root-group") != 1 {
		t.Fatal("a root supplementalGroup decoded as a float was not read")
	}
}

func TestAProfileFieldHoldingSomethingElseIsTreatedAsUnset(t *testing.T) {
	found := report(t, deployment("api", podSpec(container("app", withSecurity(map[string]any{
		"seccompProfile": "RuntimeDefault",
	})))))

	if findingCount(t, found, "seccomp-unset") != 1 {
		t.Fatal("a seccompProfile that is a string, not an object, was accepted as set")
	}
}
