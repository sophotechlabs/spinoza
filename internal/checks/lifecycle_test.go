package checks

import (
	"maps"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func httpProbe(port any, delay int64) map[string]any {
	return map[string]any{
		"httpGet":             map[string]any{"path": "/healthz", "port": port},
		"initialDelaySeconds": delay,
	}
}

func settledContainer(fields map[string]any) map[string]any {
	base := map[string]any{
		"ports":          []any{map[string]any{"name": "http", "containerPort": int64(8080)}},
		"livenessProbe":  httpProbe(int64(8080), int64(10)),
		"readinessProbe": map[string]any{"tcpSocket": map[string]any{"port": int64(8080)}},
		"lifecycle":      map[string]any{"preStop": map[string]any{"exec": map[string]any{"command": []any{"sleep", "5"}}}},
		"resources": map[string]any{
			"requests": map[string]any{"memory": "256Mi", ephemeralName: "1Gi"},
			"limits":   map[string]any{"memory": "256Mi", ephemeralName: "1Gi"},
		},
	}
	maps.Copy(base, fields)
	return container("app", base)
}

func settledPod(fields map[string]any, containers ...map[string]any) map[string]any {
	base := map[string]any{
		graceField:                  int64(30),
		"topologySpreadConstraints": []any{map[string]any{"maxSkew": int64(1)}},
	}
	maps.Copy(base, fields)
	return podSpecWith(base, containers...)
}

func settledDeployment(pod map[string]any) *unstructured.Unstructured {
	obj := replicas(deployment("api", pod), 2)
	spec := specOf(obj)
	spec["strategy"] = map[string]any{
		"type":          "RollingUpdate",
		"rollingUpdate": map[string]any{"maxUnavailable": int64(1)},
	}
	spec[revisionHistory] = int64(3)
	return obj
}

func settled() *unstructured.Unstructured {
	return settledDeployment(settledPod(nil, settledContainer(nil)))
}

func withSpec(obj *unstructured.Unstructured, key string, value any) *unstructured.Unstructured {
	specOf(obj)[key] = value
	return obj
}

// what each lifecycle check refuses, and what it lets through

func TestEveryLifecycleCheckFiresOnItsOwnFaultAndOnNothingElse(t *testing.T) {
	cases := []struct {
		id    string
		trips *unstructured.Unstructured
	}{
		{
			id: "probes-identical",
			trips: settledDeployment(settledPod(nil, settledContainer(map[string]any{
				"readinessProbe": httpProbe(int64(8080), int64(10)),
			}))),
		},
		{
			id: "probe-port-not-declared",
			trips: settledDeployment(settledPod(nil, settledContainer(map[string]any{
				"livenessProbe": httpProbe(int64(9090), int64(10)),
			}))),
		},
		{
			id: "liveness-without-startup-grace",
			trips: settledDeployment(settledPod(nil, settledContainer(map[string]any{
				"livenessProbe": httpProbe(int64(8080), int64(0)),
			}))),
		},
		{
			id: "no-prestop-hook",
			trips: settledDeployment(settledPod(nil, settledContainer(map[string]any{
				"lifecycle": nil,
			}))),
		},
		{
			id:    "grace-period-zero",
			trips: settledDeployment(settledPod(map[string]any{graceField: int64(0)}, settledContainer(nil))),
		},
		{
			id:    "grace-period-blocks-drain",
			trips: settledDeployment(settledPod(map[string]any{graceField: int64(1800)}, settledContainer(nil))),
		},
		{
			id:    "wrong-restart-policy",
			trips: settledDeployment(settledPod(map[string]any{"restartPolicy": "OnFailure"}, settledContainer(nil))),
		},
		{
			id: "ephemeral-storage-unset",
			trips: settledDeployment(settledPod(nil, settledContainer(map[string]any{
				"resources": map[string]any{
					"requests": map[string]any{"memory": "256Mi"},
					"limits":   map[string]any{"memory": "256Mi"},
				},
			}))),
		},
		{
			id: "memory-limit-not-request",
			trips: settledDeployment(settledPod(nil, settledContainer(map[string]any{
				"resources": map[string]any{
					"requests": map[string]any{"memory": "128Mi", ephemeralName: "1Gi"},
					"limits":   map[string]any{"memory": "256Mi", ephemeralName: "1Gi"},
				},
			}))),
		},
		{
			id: "init-container-unbounded",
			trips: settledDeployment(settledPod(map[string]any{
				"initContainers": []any{map[string]any{"name": "setup", "image": "busybox:1.36"}},
			}, settledContainer(nil))),
		},
		{
			id:    "replicas-zero",
			trips: withSpec(settled(), "replicas", int64(0)),
		},
		{
			id: "no-spread-no-anti-affinity",
			trips: settledDeployment(settledPod(map[string]any{
				"topologySpreadConstraints": nil,
			}, settledContainer(nil))),
		},
		{
			id:    "recreate-strategy",
			trips: withSpec(settled(), "strategy", map[string]any{"type": recreateShape}),
		},
		{
			id: "max-unavailable-all",
			trips: withSpec(settled(), "strategy", map[string]any{
				"type":          "RollingUpdate",
				"rollingUpdate": map[string]any{"maxUnavailable": "100%"},
			}),
		},
		{
			id:    "statefulset-no-service-name",
			trips: workload("StatefulSet", "db", settledPod(nil, settledContainer(nil))),
		},
		{
			id:    "unbounded-revision-history",
			trips: withSpec(settled(), revisionHistory, int64(50)),
		},
		{
			id: "duplicate-env-keys",
			trips: settledDeployment(settledPod(nil, settledContainer(map[string]any{
				"env": []any{
					map[string]any{"name": "MODE", "value": "a"},
					map[string]any{"name": "MODE", "value": "b"},
				},
			}))),
		},
	}

	registered := map[string]bool{}
	for _, entry := range lifecycleChecks() {
		registered[entry.id] = true
	}
	if len(cases) != len(registered) {
		t.Fatalf("%d cases cover %d registered lifecycle checks", len(cases), len(registered))
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			if !registered[tc.id] {
				t.Fatalf("%s is not a registered lifecycle check", tc.id)
			}
			if findingCount(t, report(t, tc.trips), tc.id) == 0 {
				t.Fatalf("%s did not fire on the workload written to trip it", tc.id)
			}
			if findingCount(t, report(t, settled()), tc.id) != 0 {
				t.Fatalf("%s fired on a settled deployment", tc.id)
			}
		})
	}
}

// the parts that are easy to get subtly wrong

func TestProbesThatShareAHandlerKindButNotItsFieldsAreNotIdentical(t *testing.T) {
	found := report(t, settledDeployment(settledPod(nil, settledContainer(map[string]any{
		"readinessProbe": map[string]any{
			"httpGet": map[string]any{"path": "/ready", "port": int64(8080)},
		},
	}))))

	if findingCount(t, found, "probes-identical") != 0 {
		t.Fatal("two http probes on different paths were called identical")
	}
}

func TestProbesOnDifferentHandlerKindsAreNotIdentical(t *testing.T) {
	found := report(t, settled())

	if findingCount(t, found, "probes-identical") != 0 {
		t.Fatal("an http liveness and a tcp readiness were called identical")
	}
}

func TestAProbeNamedPortIsMatchedAgainstTheDeclaredNames(t *testing.T) {
	named := report(t, settledDeployment(settledPod(nil, settledContainer(map[string]any{
		"livenessProbe": httpProbe("http", int64(10)),
	}))))
	if findingCount(t, named, "probe-port-not-declared") != 0 {
		t.Fatal("a probe on the declared port name was reported as undeclared")
	}

	wrong := report(t, settledDeployment(settledPod(nil, settledContainer(map[string]any{
		"livenessProbe": httpProbe("metrics", int64(10)),
	}))))
	if !strings.Contains(onlyFinding(t, wrong, "probe-port-not-declared").Detail, "metrics") {
		t.Fatal("a probe on an undeclared port name was not reported")
	}
}

func TestAContainerThatDeclaresNoPortsIsNotProbeChecked(t *testing.T) {
	found := report(t, settledDeployment(settledPod(nil, settledContainer(map[string]any{
		"ports":         nil,
		"livenessProbe": httpProbe(int64(9090), int64(10)),
	}))))

	if findingCount(t, found, "probe-port-not-declared") != 0 {
		t.Fatal("a container that declares no ports at all was reported")
	}
}

func TestAStartupProbeSatisfiesTheLivenessGraceCheck(t *testing.T) {
	found := report(t, settledDeployment(settledPod(nil, settledContainer(map[string]any{
		"livenessProbe": httpProbe(int64(8080), int64(0)),
		"startupProbe":  map[string]any{"httpGet": map[string]any{"port": int64(8080)}},
	}))))

	if findingCount(t, found, "liveness-without-startup-grace") != 0 {
		t.Fatal("a startupProbe did not satisfy the grace check")
	}
}

func TestAJobIsToldApartFromADeploymentOnRestartPolicy(t *testing.T) {
	job := workload("Job", "import", podSpecWith(map[string]any{
		"restartPolicy": "OnFailure",
	}, container("app", nil)))
	if findingCount(t, report(t, job), "wrong-restart-policy") != 0 {
		t.Fatal("OnFailure was reported as wrong for a Job")
	}

	always := workload("Job", "import", podSpecWith(map[string]any{
		"restartPolicy": alwaysPull,
	}, container("app", nil)))
	if !strings.Contains(onlyFinding(t, report(t, always), "wrong-restart-policy").Detail, "Job cannot use") {
		t.Fatal("Always was not reported as wrong for a Job")
	}

	unset := workload("Job", "import", podSpec(container("app", nil)))
	if !strings.Contains(onlyFinding(t, report(t, unset), "wrong-restart-policy").Detail, "unset") {
		t.Fatal("an unset restartPolicy on a Job was not reported")
	}
}

func TestNoPreStopIsNotAskedOfAJob(t *testing.T) {
	found := report(t, workload("Job", "import", podSpecWith(map[string]any{
		"restartPolicy": "Never",
	}, container("app", nil))))

	if findingCount(t, found, "no-prestop-hook") != 0 {
		t.Fatal("a Job was asked for a preStop hook")
	}
}

func TestEphemeralStorageSaysWhichHalfIsMissing(t *testing.T) {
	requestOnly := report(t, settledDeployment(settledPod(nil, settledContainer(map[string]any{
		"resources": map[string]any{"requests": map[string]any{ephemeralName: "1Gi"}},
	}))))
	if detail := onlyFinding(t, requestOnly, "ephemeral-storage-unset").Detail; detail != "no ephemeral-storage limit" {
		t.Fatalf("detail was %q, want the limit named", detail)
	}

	limitOnly := report(t, settledDeployment(settledPod(nil, settledContainer(map[string]any{
		"resources": map[string]any{"limits": map[string]any{ephemeralName: "1Gi"}},
	}))))
	if detail := onlyFinding(t, limitOnly, "ephemeral-storage-unset").Detail; detail != "no ephemeral-storage request" {
		t.Fatalf("detail was %q, want the request named", detail)
	}
}

func TestMemoryIsOnlyJudgedWhenBothNumbersExist(t *testing.T) {
	found := report(t, settledDeployment(settledPod(nil, settledContainer(map[string]any{
		"resources": map[string]any{
			"requests": map[string]any{"memory": "128Mi", ephemeralName: "1Gi"},
			"limits":   map[string]any{ephemeralName: "1Gi"},
		},
	}))))

	if findingCount(t, found, "memory-limit-not-request") != 0 {
		t.Fatal("a container with no memory limit was judged on its QoS class")
	}
}

func TestAPodAntiAffinitySatisfiesTheSpreadCheck(t *testing.T) {
	found := report(t, settledDeployment(settledPod(map[string]any{
		"topologySpreadConstraints": nil,
		"affinity":                  map[string]any{"podAntiAffinity": map[string]any{}},
	}, settledContainer(nil))))

	if findingCount(t, found, "no-spread-no-anti-affinity") != 0 {
		t.Fatal("a podAntiAffinity did not satisfy the spread check")
	}
}

func TestASingleReplicaIsNotAskedToSpread(t *testing.T) {
	obj := settledDeployment(settledPod(map[string]any{"topologySpreadConstraints": nil}, settledContainer(nil)))
	found := report(t, replicas(obj, 1))

	if findingCount(t, found, "no-spread-no-anti-affinity") != 0 {
		t.Fatal("a single-replica deployment was asked to spread")
	}
}

func TestMaxUnavailableIsReadAsBothACountAndAPercentage(t *testing.T) {
	counted := withSpec(settled(), "strategy", map[string]any{
		"rollingUpdate": map[string]any{"maxUnavailable": int64(2)},
	})
	if findingCount(t, report(t, counted), "max-unavailable-all") != 1 {
		t.Fatal("maxUnavailable equal to the replica count was not reported")
	}

	partial := withSpec(settled(), "strategy", map[string]any{
		"rollingUpdate": map[string]any{"maxUnavailable": "50%"},
	})
	if findingCount(t, report(t, partial), "max-unavailable-all") != 0 {
		t.Fatal("maxUnavailable of half the workload was reported as all of it")
	}
}

func TestAStatefulSetThatNamesItsServiceIsAccepted(t *testing.T) {
	obj := workload("StatefulSet", "db", settledPod(nil, settledContainer(nil)))
	found := report(t, withSpec(obj, "serviceName", "db"))

	if findingCount(t, found, "statefulset-no-service-name") != 0 {
		t.Fatal("a StatefulSet that names its service was reported")
	}
}

func TestTheRevisionHistoryCheckNamesTheCountAndLetsTheDefaultAlone(t *testing.T) {
	high := report(t, withSpec(settled(), revisionHistory, int64(50)))
	if !strings.Contains(onlyFinding(t, high, "unbounded-revision-history").Detail, "50 old revisions") {
		t.Fatal("a high revisionHistoryLimit did not name the count")
	}

	unset := report(t, withSpec(settled(), revisionHistory, nil))
	if findingCount(t, unset, "unbounded-revision-history") != 0 {
		t.Fatal("an unset revisionHistoryLimit was reported, but a live apiserver always defaults it to ten")
	}
}

func TestOnlyTheRepeatedEnvKeyIsNamedAndOnlyOnce(t *testing.T) {
	found := report(t, settledDeployment(settledPod(nil, settledContainer(map[string]any{
		"env": []any{
			map[string]any{"name": "MODE", "value": "a"},
			map[string]any{"name": "MODE", "value": "b"},
			map[string]any{"name": "MODE", "value": "c"},
			map[string]any{"name": "PORT", "value": "8080"},
		},
	}))))

	if detail := onlyFinding(t, found, "duplicate-env-keys").Detail; detail != "MODE is set more than once" {
		t.Fatalf("detail was %q, want MODE named once", detail)
	}
}

func TestListsAndFieldsOfTheWrongShapeAreSkippedByTheLifecycleChecks(t *testing.T) {
	found := report(t, settledDeployment(settledPod(map[string]any{
		"topologySpreadConstraints": nil,
		"affinity":                  "not-an-object",
	}, settledContainer(map[string]any{
		"ports":         []any{"not-an-object"},
		"env":           []any{"not-an-object", map[string]any{"value": "no name"}},
		"lifecycle":     "not-an-object",
		"livenessProbe": map[string]any{"exec": map[string]any{"command": []any{"true"}}},
	}))))

	if findingCount(t, found, "duplicate-env-keys") != 0 {
		t.Fatal("an env list of non-objects produced a duplicate")
	}
	if findingCount(t, found, "probe-port-not-declared") != 0 {
		t.Fatal("a ports list of non-objects was read as declaring ports")
	}
	if findingCount(t, found, "no-prestop-hook") != 1 {
		t.Fatal("a lifecycle field of the wrong shape was read as holding a preStop hook")
	}
	if findingCount(t, found, "no-spread-no-anti-affinity") != 1 {
		t.Fatal("an affinity field of the wrong shape was read as anti-affinity")
	}
}

func TestEveryJudgementCallShipsAtLowSeverity(t *testing.T) {
	for _, entry := range registry() {
		if !strings.Contains(entry.wrong, "judgement call") {
			continue
		}
		if entry.severity != severityLow {
			t.Fatalf("%s calls itself a judgement call but ships at %s", entry.id, entry.severity)
		}
	}
}

func TestEveryArguableCheckSaysSoInItsOwnText(t *testing.T) {
	arguable := []string{
		"automount-token",
		"default-service-account",
		"no-prestop-hook",
		"grace-period-blocks-drain",
		"memory-limit-not-request",
		"pull-policy-not-always",
		"private-registry-no-pull-secret",
		"missing-recommended-labels",
		"cpu-limit-set",
		"secret-volume-world-readable",
	}
	byID := map[string]check{}
	for _, entry := range registry() {
		byID[entry.id] = entry
	}
	for _, id := range arguable {
		entry, ok := byID[id]
		if !ok {
			t.Fatalf("%s is not registered", id)
		}
		if !strings.Contains(entry.wrong, "judgement call") {
			t.Fatalf("%s does not tell the reader it is a judgement call", id)
		}
	}
}

// numbers that arrive as floats, and probes of every handler kind

func TestPortsAndProbeValuesDecodedAsFloatsAreStillRead(t *testing.T) {
	found := report(t, settledDeployment(settledPod(nil, settledContainer(map[string]any{
		"ports":          []any{map[string]any{"containerPort": float64(8080)}},
		"livenessProbe":  map[string]any{"tcpSocket": map[string]any{"port": float64(9090)}},
		"readinessProbe": map[string]any{"tcpSocket": map[string]any{"port": float64(8080)}},
	}))))

	if !strings.Contains(onlyFinding(t, found, "probe-port-not-declared").Detail, "9090") {
		t.Fatal("a float-decoded probe port was not compared against the declared ports")
	}
}

func TestTwoIdenticalTcpProbesAreCaught(t *testing.T) {
	same := map[string]any{"tcpSocket": map[string]any{"port": int64(8080), "host": "localhost"}}
	found := report(t, settledDeployment(settledPod(nil, settledContainer(map[string]any{
		"livenessProbe":  same,
		"readinessProbe": map[string]any{"tcpSocket": map[string]any{"port": int64(8080), "host": "localhost"}},
	}))))

	if !strings.Contains(onlyFinding(t, found, "probes-identical").Detail, "tcpSocket") {
		t.Fatal("two identical tcp probes were not caught")
	}
}

func TestProbesHoldingValuesOfDifferentTypesAreNotIdentical(t *testing.T) {
	found := report(t, settledDeployment(settledPod(nil, settledContainer(map[string]any{
		"livenessProbe":  map[string]any{"httpGet": map[string]any{"port": int64(8080), "scheme": "HTTP"}},
		"readinessProbe": map[string]any{"httpGet": map[string]any{"port": int64(8080), "scheme": true}},
	}))))

	if findingCount(t, found, "probes-identical") != 0 {
		t.Fatal("probes whose fields hold different types were called identical")
	}
}

func TestAProbeWithNoRecognisedHandlerIsIgnored(t *testing.T) {
	found := report(t, settledDeployment(settledPod(nil, settledContainer(map[string]any{
		"livenessProbe":  map[string]any{"initialDelaySeconds": int64(5)},
		"readinessProbe": map[string]any{"initialDelaySeconds": int64(5)},
	}))))

	if findingCount(t, found, "probes-identical") != 0 {
		t.Fatal("two probes with no handler at all were called identical")
	}
	if findingCount(t, found, "probe-port-not-declared") != 0 {
		t.Fatal("a probe with no handler was read as naming a port")
	}
}

func TestMaxUnavailableOfAShapeNobodyWritesIsIgnored(t *testing.T) {
	odd := withSpec(settled(), "strategy", map[string]any{
		"rollingUpdate": map[string]any{"maxUnavailable": true},
	})
	if findingCount(t, report(t, odd), "max-unavailable-all") != 0 {
		t.Fatal("a boolean maxUnavailable was read as a count")
	}

	unparsable := withSpec(settled(), "strategy", map[string]any{
		"rollingUpdate": map[string]any{"maxUnavailable": "half"},
	})
	if findingCount(t, report(t, unparsable), "max-unavailable-all") != 0 {
		t.Fatal("a maxUnavailable that is not a number was read as one")
	}
}

func TestAStrategyWithNoRollingUpdateBlockIsIgnored(t *testing.T) {
	found := report(t, withSpec(settled(), "strategy", map[string]any{"type": "RollingUpdate"}))

	if findingCount(t, found, "max-unavailable-all") != 0 {
		t.Fatal("a strategy with no rollingUpdate block was reported")
	}
}
