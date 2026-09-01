package checks

import (
	"maps"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const pinnedImage = "ghcr.io/sophotechlabs/api:1.4.2@sha256:" +
	"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func sourcedContainer(fields map[string]any) map[string]any {
	base := map[string]any{"image": pinnedImage}
	maps.Copy(base, fields)
	return container("app", base)
}

func labelledWorkload(kind, name string, spec map[string]any) *unstructured.Unstructured {
	obj := workload(kind, name, spec)
	meta, ok := obj.Object["metadata"].(map[string]any)
	if ok {
		meta["labels"] = map[string]any{nameLabel: name}
	}
	template, ok := specOf(obj)["template"].(map[string]any)
	if ok {
		template["metadata"] = map[string]any{"labels": map[string]any{"app": name}}
	}
	return obj
}

func sourced(spec map[string]any) *unstructured.Unstructured {
	return labelledWorkload("Deployment", "api", spec)
}

func sourcedClean() *unstructured.Unstructured {
	return sourced(podSpec(sourcedContainer(nil)))
}

func namespaced(obj *unstructured.Unstructured, namespace string) *unstructured.Unstructured {
	meta, ok := obj.Object["metadata"].(map[string]any)
	if ok {
		meta["namespace"] = namespace
	}
	return obj
}

func TestEverySupplyCheckFiresOnItsOwnFaultAndOnNothingElse(t *testing.T) {
	cases := []struct {
		id    string
		trips *unstructured.Unstructured
	}{
		{
			id:    "image-not-digest-pinned",
			trips: sourced(podSpec(sourcedContainer(map[string]any{"image": "ghcr.io/x/api:1.4.2"}))),
		},
		{
			id:    "image-from-docker-hub",
			trips: sourced(podSpec(sourcedContainer(map[string]any{"image": "nginx:1.27"}))),
		},
		{
			id: "pull-policy-not-always",
			trips: sourced(podSpec(sourcedContainer(map[string]any{
				"image":           "ghcr.io/x/api:1.4.2",
				"imagePullPolicy": "IfNotPresent",
			}))),
		},
		{
			id:    "private-registry-no-pull-secret",
			trips: sourced(podSpec(sourcedContainer(map[string]any{"image": "registry.sopho.tech/api:1.4.2"}))),
		},
		{
			id: "secret-in-env-literal",
			trips: sourced(podSpec(sourcedContainer(map[string]any{
				"env": []any{map[string]any{"name": "DB_PASSWORD", "value": "hunter2"}},
			}))),
		},
		{
			id: "env-from-secret-wholesale",
			trips: sourced(podSpec(sourcedContainer(map[string]any{
				"envFrom": []any{map[string]any{"secretRef": map[string]any{"name": "api-secrets"}}},
			}))),
		},
		{
			id: "secret-volume-world-readable",
			trips: sourced(podSpecWith(map[string]any{
				"volumes": []any{map[string]any{
					"name":   "creds",
					"secret": map[string]any{"secretName": "api", "defaultMode": int64(0o644)},
				}},
			}, sourcedContainer(nil))),
		},
		{
			id:    "default-namespace",
			trips: namespaced(sourcedClean(), defaultNamespace),
		},
		{
			id:    "missing-recommended-labels",
			trips: workload("Deployment", "api", podSpec(sourcedContainer(nil))),
		},
		{
			id:    "selector-template-mismatch",
			trips: workload("Deployment", "api", podSpec(sourcedContainer(nil))),
		},
		{
			id: "cpu-limit-set",
			trips: sourced(podSpec(sourcedContainer(map[string]any{
				"resources": map[string]any{"limits": map[string]any{cpuName: "500m"}},
			}))),
		},
	}

	registered := map[string]bool{}
	for _, entry := range supplyChecks() {
		registered[entry.id] = true
	}
	if len(cases) != len(registered) {
		t.Fatalf("%d cases cover %d registered supply checks", len(cases), len(registered))
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			if !registered[tc.id] {
				t.Fatalf("%s is not a registered supply check", tc.id)
			}
			if findingCount(t, report(t, tc.trips), tc.id) == 0 {
				t.Fatalf("%s did not fire on the workload written to trip it", tc.id)
			}
			if findingCount(t, report(t, sourcedClean()), tc.id) != 0 {
				t.Fatalf("%s fired on a workload with nothing wrong with it", tc.id)
			}
		})
	}
}

func TestABareImageNameIsReadAsDockerHub(t *testing.T) {
	found := report(t, sourced(podSpec(sourcedContainer(map[string]any{"image": "busybox:1.36"}))))

	if findingCount(t, found, "image-from-docker-hub") != 1 {
		t.Fatal("an image with no registry host was not read as Docker Hub")
	}
}

func TestAHostWithNoDotIsAnOrganisationNotARegistry(t *testing.T) {
	found := report(t, sourced(podSpec(sourcedContainer(map[string]any{"image": "library/nginx:1.27"}))))

	if findingCount(t, found, "image-from-docker-hub") != 1 {
		t.Fatal("an organization-qualified Docker Hub image was read as a private registry")
	}
}

func TestALocalhostRegistryIsARegistry(t *testing.T) {
	found := report(t, sourced(podSpec(sourcedContainer(map[string]any{"image": "localhost:5000/api:1.0"}))))

	if findingCount(t, found, "image-from-docker-hub") != 0 {
		t.Fatal("a localhost registry was read as Docker Hub")
	}
	if findingCount(t, found, "private-registry-no-pull-secret") != 1 {
		t.Fatal("a localhost registry was treated as public")
	}
}

func TestADigestPinnedImageIsNotAskedToPullAlways(t *testing.T) {
	found := report(t, sourced(podSpec(sourcedContainer(map[string]any{
		"imagePullPolicy": "IfNotPresent",
	}))))

	if findingCount(t, found, "pull-policy-not-always") != 0 {
		t.Fatal("a digest-pinned image was asked to pull always")
	}
}

func TestTheLatestTagIsLeftToItsOwnCheck(t *testing.T) {
	found := report(t, sourced(podSpec(sourcedContainer(map[string]any{
		"image":           "ghcr.io/x/api:latest",
		"imagePullPolicy": "IfNotPresent",
	}))))

	if findingCount(t, found, "pull-policy-not-always") != 0 {
		t.Fatal(":latest was reported by the pull-policy check as well as its own")
	}
	if findingCount(t, found, "image-latest") != 1 {
		t.Fatal("the latest-tag check stopped reporting it")
	}
}

func TestAPullSecretSatisfiesThePrivateRegistryCheck(t *testing.T) {
	found := report(t, sourced(podSpecWith(map[string]any{
		"imagePullSecrets": []any{map[string]any{"name": "registry"}},
	}, sourcedContainer(map[string]any{"image": "registry.sopho.tech/api:1.4.2"}))))

	if findingCount(t, found, "private-registry-no-pull-secret") != 0 {
		t.Fatal("a workload with imagePullSecrets was still reported")
	}
}

func TestOnlyCredentialShapedNamesWithLiteralValuesAreReported(t *testing.T) {
	fromSecret := report(t, sourced(podSpec(sourcedContainer(map[string]any{
		"env": []any{map[string]any{
			"name":      "DB_PASSWORD",
			"valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": "db", "key": "password"}},
		}},
	}))))
	if findingCount(t, fromSecret, "secret-in-env-literal") != 0 {
		t.Fatal("a password read from a Secret was reported as a literal")
	}

	ordinary := report(t, sourced(podSpec(sourcedContainer(map[string]any{
		"env": []any{map[string]any{"name": "LOG_LEVEL", "value": "debug"}},
	}))))
	if findingCount(t, ordinary, "secret-in-env-literal") != 0 {
		t.Fatal("an ordinary environment variable was read as a credential")
	}

	named := report(t, sourced(podSpec(sourcedContainer(map[string]any{
		"env": []any{map[string]any{"name": "STRIPE_API_KEY", "value": "sk_live_abcdef"}},
	}))))
	if !strings.Contains(onlyFinding(t, named, "secret-in-env-literal").Detail, "STRIPE_API_KEY") {
		t.Fatal("a credential-shaped name with a literal value was not reported")
	}
}

func TestAnEmptyCredentialValueIsNotReported(t *testing.T) {
	found := report(t, sourced(podSpec(sourcedContainer(map[string]any{
		"env": []any{map[string]any{"name": "API_TOKEN", "value": ""}},
	}))))

	if findingCount(t, found, "secret-in-env-literal") != 0 {
		t.Fatal("an empty placeholder was reported as a credential")
	}
}

func TestAConfigMapLoadedWholesaleIsNotASecretFinding(t *testing.T) {
	found := report(t, sourced(podSpec(sourcedContainer(map[string]any{
		"envFrom": []any{map[string]any{"configMapRef": map[string]any{"name": "settings"}}},
	}))))

	if findingCount(t, found, "env-from-secret-wholesale") != 0 {
		t.Fatal("a ConfigMap loaded wholesale was reported as a Secret")
	}
}

func TestATightSecretModeIsAccepted(t *testing.T) {
	found := report(t, sourced(podSpecWith(map[string]any{
		"volumes": []any{map[string]any{
			"name":   "creds",
			"secret": map[string]any{"secretName": "api", "defaultMode": int64(0o400)},
		}},
	}, sourcedContainer(nil))))

	if findingCount(t, found, "secret-volume-world-readable") != 0 {
		t.Fatal("a secret volume at 0400 was reported")
	}
}

func TestASecretVolumeThatSetsNoModeIsLeftAlone(t *testing.T) {
	found := report(t, sourced(podSpecWith(map[string]any{
		"volumes": []any{map[string]any{"name": "creds", "secret": map[string]any{"secretName": "api"}}},
	}, sourcedContainer(nil))))

	if findingCount(t, found, "secret-volume-world-readable") != 0 {
		t.Fatal("a secret volume with no explicit mode was reported")
	}
}

func TestASelectorLabelCarriedWithADifferentValueIsAMismatch(t *testing.T) {
	obj := sourcedClean()
	template, ok := specOf(obj)["template"].(map[string]any)
	if !ok {
		t.Fatal("the fixture has no pod template")
	}
	template["metadata"] = map[string]any{"labels": map[string]any{"app": "something-else"}}

	if !strings.Contains(onlyFinding(t, report(t, obj), "selector-template-mismatch").Detail, "app") {
		t.Fatal("a selector label carried with the wrong value was not reported")
	}
}

func TestAWorkloadWithNoSelectorIsNotJudgedOnOne(t *testing.T) {
	obj := sourcedClean()
	delete(specOf(obj), "selector")

	if findingCount(t, report(t, obj), "selector-template-mismatch") != 0 {
		t.Fatal("a workload with no selector was reported as mismatched")
	}
}

func TestAPodIsNotAskedAboutItsSelector(t *testing.T) {
	found := report(t, pod("api", podSpec(sourcedContainer(nil))))

	if findingCount(t, found, "selector-template-mismatch") != 0 {
		t.Fatal("a bare pod was asked about a selector it cannot have")
	}
}

func TestAlwaysPullSatisfiesAMutableTag(t *testing.T) {
	obj := sourced(podSpec(sourcedContainer(map[string]any{
		"image":           "ghcr.io/acme/api:stable",
		"imagePullPolicy": "Always",
	})))

	if findingCount(t, report(t, obj), "pull-policy-not-always") != 0 {
		t.Fatal("Always was reported as an unsafe pull policy")
	}
}

func TestASelectorWithMalformedMatchLabelsIsIgnored(t *testing.T) {
	obj := sourcedClean()
	specOf(obj)["selector"] = map[string]any{"matchLabels": "not-an-object"}

	if findingCount(t, report(t, obj), "selector-template-mismatch") != 0 {
		t.Fatal("malformed matchLabels were treated as a selector")
	}
}

func TestListsAndFieldsOfTheWrongShapeAreSkippedByTheSupplyChecks(t *testing.T) {
	obj := sourced(podSpecWith(map[string]any{
		"volumes":          []any{"not-an-object"},
		"imagePullSecrets": []any{},
	}, sourcedContainer(map[string]any{
		"env":     []any{"not-an-object"},
		"envFrom": []any{"not-an-object"},
	})))
	specOf(obj)["selector"] = map[string]any{"matchLabels": map[string]any{"app": int64(7)}}

	found := report(t, obj)

	for _, id := range []string{
		"secret-in-env-literal",
		"env-from-secret-wholesale",
		"secret-volume-world-readable",
		"selector-template-mismatch",
	} {
		if findingCount(t, found, id) != 0 {
			t.Fatalf("%s reported something from a field of the wrong shape", id)
		}
	}
}

func TestEveryBatchCheckFiresOnItsOwnFaultAndOnNothingElse(t *testing.T) {
	settledJob := func() *unstructured.Unstructured {
		obj := labelledWorkload("Job", "import", podSpecWith(map[string]any{
			"restartPolicy": "Never",
		}, sourcedContainer(nil)))
		spec := specOf(obj)
		spec[ttlField] = int64(3600)
		spec[activeDeadline] = int64(600)
		spec[backoffField] = int64(4)
		return obj
	}
	settledCron := func() *unstructured.Unstructured {
		obj := cronJob("nightly", podSpecWith(map[string]any{
			"restartPolicy": "OnFailure",
		}, sourcedContainer(nil)))
		spec := specOf(obj)
		spec[startingDeadline] = int64(120)
		spec[concurrencyField] = "Forbid"
		spec[successHistory] = int64(3)
		spec[failedHistory] = int64(1)
		template, hasTemplate := spec["jobTemplate"].(map[string]any)
		if !hasTemplate {
			t.Fatal("the fixture has no job template")
		}
		jobSpec, hasSpec := template[specField].(map[string]any)
		if !hasSpec {
			t.Fatal("the job template has no spec")
		}
		jobSpec[ttlField] = int64(3600)
		jobSpec[activeDeadline] = int64(600)
		return obj
	}

	cases := []struct {
		id     string
		trips  *unstructured.Unstructured
		settle *unstructured.Unstructured
	}{
		{
			id: "job-no-ttl",
			trips: func() *unstructured.Unstructured {
				obj := settledJob()
				delete(specOf(obj), ttlField)
				return obj
			}(),
			settle: settledJob(),
		},
		{
			id: "job-no-active-deadline",
			trips: func() *unstructured.Unstructured {
				obj := settledJob()
				delete(specOf(obj), activeDeadline)
				return obj
			}(),
			settle: settledJob(),
		},
		{
			id: "job-backoff-limit",
			trips: func() *unstructured.Unstructured {
				obj := settledJob()
				specOf(obj)[backoffField] = int64(0)
				return obj
			}(),
			settle: settledJob(),
		},
		{
			id: "cronjob-no-starting-deadline",
			trips: func() *unstructured.Unstructured {
				obj := settledCron()
				delete(specOf(obj), startingDeadline)
				return obj
			}(),
			settle: settledCron(),
		},
		{
			id: "cronjob-concurrency-allow",
			trips: func() *unstructured.Unstructured {
				obj := settledCron()
				specOf(obj)[concurrencyField] = allowConcurrent
				return obj
			}(),
			settle: settledCron(),
		},
		{
			id: "cronjob-unbounded-history",
			trips: func() *unstructured.Unstructured {
				obj := settledCron()
				delete(specOf(obj), successHistory)
				return obj
			}(),
			settle: settledCron(),
		},
	}

	registered := map[string]bool{}
	for _, entry := range batchChecks() {
		registered[entry.id] = true
	}
	if len(cases) != len(registered) {
		t.Fatalf("%d cases cover %d registered batch checks", len(cases), len(registered))
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			if findingCount(t, report(t, tc.trips), tc.id) == 0 {
				t.Fatalf("%s did not fire on the object written to trip it", tc.id)
			}
			if findingCount(t, report(t, tc.settle), tc.id) != 0 {
				t.Fatalf("%s fired on a settled %s", tc.id, tc.settle.GetKind())
			}
		})
	}
}

func TestACronJobIsJudgedOnItsJobTemplate(t *testing.T) {
	obj := cronJob("nightly", podSpec(sourcedContainer(nil)))

	found := report(t, obj)

	if findingCount(t, found, "job-no-ttl") != 1 {
		t.Fatal("a CronJob's job template was not read for its ttl")
	}
	patch := onlyFinding(t, found, "job-no-ttl").Patch
	if !strings.Contains(patch, "jobTemplate") {
		t.Fatalf("the patch was rooted at the wrong place:\n%s", patch)
	}
}

func TestADeploymentIsNotAskedAboutJobFields(t *testing.T) {
	found := report(t, sourcedClean())

	for _, id := range []string{"job-no-ttl", "job-no-active-deadline", "cronjob-concurrency-allow"} {
		if findingCount(t, found, id) != 0 {
			t.Fatalf("%s was asked of a Deployment", id)
		}
	}
}

func TestAHighBackoffLimitIsReportedAndAModerateOneIsNot(t *testing.T) {
	high := labelledWorkload("Job", "import", podSpecWith(map[string]any{"restartPolicy": "Never"},
		sourcedContainer(nil)))
	specOf(high)[backoffField] = int64(500)
	if !strings.Contains(onlyFinding(t, report(t, high), "job-backoff-limit").Detail, "500") {
		t.Fatal("a high backoffLimit was not reported")
	}

	moderate := labelledWorkload("Job", "import", podSpecWith(map[string]any{"restartPolicy": "Never"},
		sourcedContainer(nil)))
	specOf(moderate)[backoffField] = int64(6)
	if findingCount(t, report(t, moderate), "job-backoff-limit") != 0 {
		t.Fatal("a backoffLimit of 6 was reported")
	}
}

func TestTheCronHistoryCheckNamesWhichLimitIsWrong(t *testing.T) {
	obj := cronJob("nightly", podSpec(sourcedContainer(nil)))
	specOf(obj)[successHistory] = int64(50)

	if !strings.Contains(onlyFinding(t, report(t, obj), "cronjob-unbounded-history").Detail, successHistory) {
		t.Fatal("the history finding did not name the limit that is wrong")
	}
}
