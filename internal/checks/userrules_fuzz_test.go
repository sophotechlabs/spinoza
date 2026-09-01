package checks

import (
	"encoding/json"
	"slices"
	"testing"
)

func FuzzParseRules(f *testing.F) {
	for _, seed := range []string{
		`[]`,
		`[{"id":"always","expr":"true"}]`,
		`[{"id":"metadata","expr":"object.metadata.name == 'api'"}]`,
		`[{"id":"broken","expr":"object.spec["}]`,
		`not json`,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		rules := ParseRules(raw)
		for _, rule := range rules {
			if rule.ID == "" {
				t.Fatal("ParseRules kept a rule without an id")
			}
			if rule.Expr == "" {
				t.Fatal("ParseRules kept a rule without an expression")
			}
		}
		encoded, err := json.Marshal(rules)
		if err != nil {
			t.Fatalf("marshal parsed rules: %v", err)
		}
		roundTrip := ParseRules(string(encoded))
		if !slices.Equal(roundTrip, rules) {
			t.Fatalf("round trip changed rules: %#v != %#v", roundTrip, rules)
		}
		_ = Faults(raw)
	})
}
