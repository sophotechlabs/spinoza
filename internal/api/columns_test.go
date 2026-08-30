package api

import "testing"

func TestColumnsComeBackFromTheirJSON(t *testing.T) {
	held := ParseColumns(`{"/v1/pods":[{"name":"Node name","path":".spec.nodeName"}]}`)

	if len(held) != 1 || len(held["/v1/pods"]) != 1 {
		t.Fatalf("read %v", held)
	}
	one := held["/v1/pods"][0]
	if one.Name != "Node name" || one.Path != ".spec.nodeName" {
		t.Fatalf("read %+v", one)
	}
}

func TestAnythingThatIsNotColumnsReadsAsNone(t *testing.T) {
	for _, raw := range []string{"", "not json", "[]", "null", "42"} {
		if got := ParseColumns(raw); len(got) != 0 {
			t.Fatalf("%q read as %v", raw, got)
		}
	}
}
