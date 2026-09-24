package baseline

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/checks"
)

type interruptedBaseline struct {
	body string
	err  error
}

func (r *interruptedBaseline) Read(target []byte) (int, error) {
	if r.body == "" {
		return 0, r.err
	}
	count := copy(target, r.body)
	r.body = r.body[count:]
	return count, nil
}

func TestAnInterruptedBaselinePreservesTheReadFailure(t *testing.T) {
	failure := errors.New("baseline upload disconnected")
	cases := map[string]string{
		"before the document": "",
		"inside checks":       `{"takenAt":"2026-09-23","checks":["pod-security"`,
		"after valid JSON":    `{"takenAt":"2026-09-23","checks":["pod-security"],"counts":{},"keys":{}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			found, err := DecodeReader(&interruptedBaseline{body: body, err: failure})
			if !errors.Is(err, ErrRead) || !errors.Is(err, failure) {
				t.Fatalf("error = %v, want both the read sentinel and transport cause", err)
			}
			if err.Error() != "baselines: could not be read: baseline upload disconnected" {
				t.Fatalf("error = %q, want the exact read failure", err)
			}
			if !reflect.DeepEqual(found, checks.Baseline{}) {
				t.Fatalf("baseline = %+v, want no partial document", found)
			}
		})
	}
}

func TestABaselineReadInSmallChunksMatchesTheRealStoredDocument(t *testing.T) {
	taken := checks.Baseline{
		Cluster: "example-cluster", TakenAt: "2026-09-23T12:00:00Z",
		Checks: []string{"pod-security"}, Counts: map[string]int{"pod-security": 1},
		Keys: map[string]string{"pod/default/web": "fingerprint"}, Scanned: 7,
	}
	body, err := Encode(taken)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	reader := io.MultiReader(strings.NewReader(string(body[:7])), strings.NewReader(string(body[7:])))
	found, err := DecodeReader(reader)
	if err != nil || !reflect.DeepEqual(found, taken) {
		t.Fatalf("baseline/error = %+v/%v, want %+v", found, err, taken)
	}
}
