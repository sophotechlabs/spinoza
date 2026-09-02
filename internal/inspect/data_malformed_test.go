package inspect

import "testing"

func TestConfigMapTextSkipsValuesThatAreNotStrings(t *testing.T) {
	got := dataOf(configmap(map[string]any{
		"fine":   "plain text",
		"number": int64(3),
		"nested": map[string]any{"value": "not configmap text"},
	}, nil))

	if len(got) != 1 {
		t.Fatalf("entries = %+v, want only the string value", got)
	}
	if got[0].Key != "fine" || got[0].Value != "plain text" {
		t.Fatalf("entry = %+v, want the valid text left intact", got[0])
	}
}
