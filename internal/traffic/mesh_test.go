package traffic

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goldenPath = "testdata/queries.txt"

func rendered() string {
	var out strings.Builder
	for _, entry := range meshes {
		fmt.Fprintf(&out, "# %s\n", entry.name)
		fmt.Fprintf(&out, "present  %s\n", entry.present)
		fmt.Fprintf(&out, "labeled  %s\n", entry.labeled)
		fmt.Fprintf(&out, "flows    %s\n", entry.flows)
		fmt.Fprintf(&out, "from     %s, %s\n", entry.from.namespace, entry.from.workload)
		fmt.Fprintf(&out, "to       %s, %s\n", entry.to.namespace, entry.to.workload)
		fmt.Fprintf(&out, "verdict  %s\n", entry.verdict)
	}
	return out.String()
}

func TestTheQueriesMatchTheGolden(t *testing.T) {
	got := rendered()
	if os.Getenv("UPDATE_TRAFFIC_QUERIES") == "1" {
		err := os.WriteFile(filepath.FromSlash(goldenPath), []byte(got), 0o600)
		if err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Fatalf("golden refreshed; run the test again without UPDATE_TRAFFIC_QUERIES")
	}
	want, err := os.ReadFile(filepath.FromSlash(goldenPath))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(want) {
		t.Fatalf(
			"the queries changed.\n\ngot:\n%s\nwant:\n%s\nrefresh with:\n\n    UPDATE_TRAFFIC_QUERIES=1 go test ./internal/traffic/\n",
			got, string(want),
		)
	}
}

func TestEveryMeshGroupsByTheLabelsItReads(t *testing.T) {
	for _, entry := range meshes {
		t.Run(entry.name, func(t *testing.T) {
			labels := []string{
				entry.from.namespace,
				entry.from.workload,
				entry.to.namespace,
				entry.to.workload,
				entry.verdict,
			}
			for _, label := range labels {
				if !strings.Contains(entry.flows, label) {
					t.Fatalf("the flow query never mentions %q, so grouping cannot produce it", label)
				}
			}
		})
	}
}

func TestEveryMeshProvesItsLabelsBeforeTrustingThem(t *testing.T) {
	for _, entry := range meshes {
		t.Run(entry.name, func(t *testing.T) {
			if !strings.Contains(entry.labeled, entry.from.workload) {
				t.Fatalf("the labeled probe never checks %q", entry.from.workload)
			}
			if !strings.Contains(entry.labeled, entry.to.workload) {
				t.Fatalf("the labeled probe never checks %q", entry.to.workload)
			}
		})
	}
}

func TestEveryMeshNamesItselfAndSaysWhatToConfigure(t *testing.T) {
	for _, entry := range meshes {
		t.Run(entry.name, func(t *testing.T) {
			if entry.name == "" {
				t.Fatal("a mesh with no name cannot be reported as the source")
			}
			if entry.hint == "" {
				t.Fatal("a mesh with no hint leaves the unlabeled state unexplained")
			}
			if !strings.Contains(entry.hint, entry.from.workload) {
				t.Fatalf("the hint never names %q, so it does not say what to add", entry.from.workload)
			}
		})
	}
}

func TestMeshNamesListsEveryMesh(t *testing.T) {
	names := meshNames()
	for _, entry := range meshes {
		if !strings.Contains(names, entry.name) {
			t.Fatalf("%q is missing from %q", entry.name, names)
		}
	}
}
