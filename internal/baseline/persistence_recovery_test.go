package baseline

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/checks"
)

func TestARejectedStoredReplacementPreservesBothClustersAndCanRecover(t *testing.T) {
	for _, reason := range []string{"checks", "counts", "encoded bytes"} {
		t.Run(reason, func(t *testing.T) {
			held := store(t)
			original := taken()
			other := taken()
			other.TakenAt = "2026-09-24T12:00:00Z"
			if err := held.Save(cluster, original); err != nil {
				t.Fatalf("seed original: %v", err)
			}
			if err := held.Save("other", other); err != nil {
				t.Fatalf("seed other cluster: %v", err)
			}
			before, err := os.ReadFile(held.fileFor(cluster))
			if err != nil {
				t.Fatalf("read original bytes: %v", err)
			}
			rejected := taken()
			switch reason {
			case "checks":
				rejected.Checks = numberedStrings(maxChecks + 1)
			case "counts":
				rejected.Counts = numberedCounts(maxCounts + 1)
			case "encoded bytes":
				rejected.TakenAt = strings.Repeat("x", maxBytes)
			default:
				t.Fatalf("unknown refusal scenario: %s", reason)
			}
			if saveErr := held.Save(cluster, rejected); saveErr == nil || !strings.Contains(saveErr.Error(), "more than one baseline holds") {
				t.Fatalf("save error = %v, want the %s size refusal", saveErr, reason)
			}
			after, err := os.ReadFile(held.fileFor(cluster))
			if err != nil || !bytes.Equal(after, before) {
				t.Fatalf("persisted bytes/error = %q/%v, want original bytes retained", after, err)
			}
			for name, expected := range map[string]checks.Baseline{cluster: original, "other": other} {
				loaded, ok := Open(held.dir).Load(name)
				if !ok || !reflect.DeepEqual(loaded, expected) {
					t.Fatalf("reopened %s = %+v/%v, want %+v", name, loaded, ok, expected)
				}
			}
			entries, err := os.ReadDir(held.dir)
			if err != nil || len(entries) != 2 {
				t.Fatalf("files/error = %v/%v, want only the two persisted baselines", entries, err)
			}
			if err := held.Save(cluster, other); err != nil {
				t.Fatalf("replacement after refusal: %v", err)
			}
			loaded, ok := Open(held.dir).Load(cluster)
			if !ok || !reflect.DeepEqual(loaded, other) {
				t.Fatalf("recovered baseline = %+v/%v, want the corrected replacement", loaded, ok)
			}
		})
	}
}
