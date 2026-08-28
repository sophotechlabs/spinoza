package checks

import (
	"reflect"
	"strings"
	"sync"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func hostileWorkload(name string) map[string]any {
	spec := podSpec(
		container(`app`, map[string]any{
			"image":        "registry.example/app:latest",
			"volumeMounts": []any{map[string]any{"name": "sock", "mountPath": `/mnt/a #b "c"/docker.sock`}},
			"securityContext": map[string]any{
				"privileged":               true,
				"allowPrivilegeEscalation": true,
				"readOnlyRootFilesystem":   false,
				"runAsUser":                int64(0),
				"capabilities":             map[string]any{"add": []any{"SYS_ADMIN", "NET_RAW"}},
			},
			"resources": map[string]any{
				"requests": map[string]any{"cpu": "10m", "memory": "16Mi"},
				"limits":   map[string]any{"cpu": "8", "memory": "8Gi"},
			},
		}),
	)
	spec["hostPID"] = true
	spec["hostNetwork"] = true
	spec["volumes"] = []any{
		map[string]any{"name": "sock", "hostPath": map[string]any{"path": "/var/run/docker.sock"}},
	}
	_ = name
	return spec
}

func TestEveryPatchAnyCheckEmitsIsValidYaml(t *testing.T) {
	owner := deployment("api", hostileWorkload("api"))
	first := ownedBy(onNode(pod("api-a", podSpec(container("app", nil))), "worker-1"), "Deployment", "api")
	second := ownedBy(onNode(pod("api-b", podSpec(container("app", nil))), "worker-1"), "Deployment", "api")
	naked := pod("standalone", hostileWorkload("standalone"))
	scheduled := cronJob("nightly", hostileWorkload("nightly"))

	found := report(t, owner, first, second, naked, scheduled)

	patched := 0
	for _, group := range found.Groups {
		for _, finding := range group.Findings {
			if finding.Patch == "" {
				continue
			}
			patched++
			var parsed map[string]any
			err := yaml.Unmarshal([]byte(finding.Patch), &parsed)
			if err != nil {
				t.Errorf("%s emitted a patch that is not yaml: %v\n%s", group.ID, err, finding.Patch)
				continue
			}
			if len(parsed) == 0 {
				t.Errorf("%s emitted a patch that parses to nothing:\n%s", group.ID, finding.Patch)
			}
		}
	}
	if patched < 6 {
		t.Fatalf("only %d patches were checked; the fixture stopped tripping the patching checks", patched)
	}
	t.Logf("%d patches parsed as yaml", patched)
}

func TestEveryPatchIsRootedAtSpec(t *testing.T) {
	owner := deployment("api", hostileWorkload("api"))
	naked := pod("standalone", hostileWorkload("standalone"))

	found := report(t, owner, naked)

	for _, group := range found.Groups {
		for _, finding := range group.Findings {
			if finding.Patch == "" {
				continue
			}
			if !strings.HasPrefix(finding.Patch, "spec:\n") {
				t.Errorf("%s emitted a patch that is not rooted at spec:\n%s", group.ID, finding.Patch)
			}
			if !strings.HasSuffix(finding.Patch, "\n") {
				t.Errorf("%s emitted a patch with no trailing newline:\n%q", group.ID, finding.Patch)
			}
		}
	}
}

func TestAQuotedScalarEscapesWhatYamlWouldEat(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{name: "plain", value: "/var/run/docker.sock", want: `"/var/run/docker.sock"`},
		{name: "a quote", value: `say "hi"`, want: `"say \"hi\""`},
		{name: "a backslash", value: `back\slash`, want: `"back\\slash"`},
		{name: "a newline", value: "two\nlines", want: `"two\nlines"`},
		{name: "a tab", value: "a\tb", want: `"a\tb"`},
		{name: "empty", value: "", want: `""`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := quoted(tc.value)
			if got != tc.want {
				t.Fatalf("quoted(%q) = %s, want %s", tc.value, got, tc.want)
			}
			holder := struct {
				Value string `json:"value"`
			}{}
			err := yaml.Unmarshal([]byte("value: "+got+"\n"), &holder)
			if err != nil {
				t.Fatalf("quoted(%q) does not parse: %v", tc.value, err)
			}
			if holder.Value != tc.value {
				t.Fatalf("quoted(%q) round-tripped as %q", tc.value, holder.Value)
			}
		})
	}
}

func TestRunLeavesTheCachedObjectsUntouched(t *testing.T) {
	owner := deployment("api", hostileWorkload("api"))
	running := ownedBy(onNode(pod("api-a", podSpec(container("app", nil))), "worker-1"), "Deployment", "api")
	before := []*unstructured.Unstructured{owner.DeepCopy(), running.DeepCopy()}
	lister := newLister(owner, running)

	Run(t.Context(), lister, descriptors(), api.Metrics{})
	Run(t.Context(), lister, descriptors(), api.Metrics{})

	after := []*unstructured.Unstructured{owner, running}
	for at := range before {
		if !reflect.DeepEqual(before[at].Object, after[at].Object) {
			t.Fatalf("%s was modified by the audit; specAt hands out the cache without copying",
				before[at].GetName())
		}
	}
}

func TestTwoAuditsAtOnceDoNotShareState(t *testing.T) {
	lister := newLister(
		deployment("api", hostileWorkload("api")),
		pod("standalone", hostileWorkload("standalone")),
	)
	first := Run(t.Context(), lister, descriptors(), api.Metrics{})

	var wg sync.WaitGroup
	reports := make([]api.CheckReport, 8)
	for at := range reports {
		wg.Go(func() {
			reports[at] = Run(t.Context(), lister, descriptors(), api.Metrics{})
		})
	}
	wg.Wait()

	for at, got := range reports {
		if got.Scanned != first.Scanned {
			t.Fatalf("run %d scanned %d, want %d", at, got.Scanned, first.Scanned)
		}
		if len(got.Objects) != len(first.Objects) {
			t.Fatalf("run %d listed %d objects, want %d", at, len(got.Objects), len(first.Objects))
		}
	}
}
