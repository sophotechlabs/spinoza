package api_test

import (
	"testing"
)

func TestEveryGoTypeOnTheWireHasAShape(t *testing.T) {
	cases := map[string]string{
		"string":                     jsonString,
		"bool":                       jsonBoolean,
		"int":                        jsonNumber,
		"int32":                      jsonNumber,
		"int64":                      jsonNumber,
		"uint16":                     jsonNumber,
		"float64":                    jsonNumber,
		"[]string":                   jsonArray,
		"[]Row":                      jsonArray,
		"map[string]string":          jsonObject,
		"map[string]ResourceUsage":   jsonObject,
		"Row":                        jsonObject,
		"ContextRef":                 jsonObject,
		"*bool":                      jsonBoolean,
		"*int64":                     jsonNumber,
		"*Row":                       jsonObject,
		"[]byte":                     jsonString,
		"time.Time":                  jsonString,
		"time.Duration":              jsonNumber,
		"json.RawMessage":            jsonAny,
		"any":                        jsonAny,
		"map[string][]HelmRepoValue": jsonObject,
	}
	for kind, want := range cases {
		got, ok := goShape(kind)
		if !ok {
			t.Fatalf("goShape(%q) could not place it on the wire", kind)
		}
		if got != want {
			t.Fatalf("goShape(%q) = %q, want %q", kind, got, want)
		}
	}
}

func TestAGoTypeNobodyTaughtItIsRefused(t *testing.T) {
	for _, kind := range []string{"chan int", "func()", "sql.NullString", ""} {
		if _, ok := goShape(kind); ok {
			t.Fatalf("goShape(%q) placed a type it should have refused", kind)
		}
	}
}

func tsTable() (map[string]string, map[string][]property) {
	aliases := map[string]string{
		"Protection":    "'protected' | 'open' | 'unknown'",
		"ReadyState":    "'True', 'False', 'Unknown', ''",
		"PanelId":       "'overview' | 'yaml'",
		"NestedAlias":   "Protection",
		"LoopingAlias":  "LoopingAlias",
		"ObjectAlias":   "Row",
		"UnknownTarget": "SomethingNobodyDeclared",
	}
	interfaces := map[string][]property{
		"Row":     {},
		"Column":  {},
		"FluxSet": {},
	}
	return aliases, interfaces
}

func TestEveryTypeScriptDeclarationHasAShape(t *testing.T) {
	aliases, interfaces := tsTable()
	cases := map[string]string{
		"string":                    jsonString,
		"number":                    jsonNumber,
		"boolean":                   jsonBoolean,
		"Row[]":                     jsonArray,
		"Array<Row>":                jsonArray,
		"Record<string, string>":    jsonObject,
		"Partial<Record<int, int>>": jsonObject,
		"{ name: string }":          jsonObject,
		"Row":                       jsonObject,
		"Column":                    jsonObject,
		"Protection":                jsonString,
		"ReadyState":                jsonString,
		"NestedAlias":               jsonString,
		"ObjectAlias":               jsonObject,
		"'a' | 'b'":                 jsonString,
		"string | null":             jsonString,
		"Row | undefined":           jsonObject,
		"unknown":                   jsonAny,
		"any":                       jsonAny,
	}
	for kind, want := range cases {
		got, ok := tsShape(kind, aliases, interfaces, 0)
		if !ok {
			t.Fatalf("tsShape(%q) could not place it on the wire", kind)
		}
		if got != want {
			t.Fatalf("tsShape(%q) = %q, want %q", kind, got, want)
		}
	}
}

func TestATypeScriptDeclarationNobodyDeclaredIsRefused(t *testing.T) {
	aliases, interfaces := tsTable()
	for _, kind := range []string{"SomethingNobodyDeclared", "UnknownTarget", "LoopingAlias"} {
		if _, ok := tsShape(kind, aliases, interfaces, 0); ok {
			t.Fatalf("tsShape(%q) placed a type it should have refused", kind)
		}
	}
}

func TestADriftedFieldIsCaught(t *testing.T) {
	aliases, interfaces := tsTable()
	cases := []struct {
		goKind string
		tsKind string
	}{
		{goKind: "int64", tsKind: "string"},
		{goKind: "string", tsKind: "number"},
		{goKind: "bool", tsKind: "string"},
		{goKind: "[]string", tsKind: "Record<string, string>"},
		{goKind: "map[string]string", tsKind: "string[]"},
		{goKind: "Row", tsKind: "string"},
	}
	for _, tc := range cases {
		want, okGo := goShape(tc.goKind)
		got, okTS := tsShape(tc.tsKind, aliases, interfaces, 0)
		if !okGo || !okTS {
			t.Fatalf("go %q / ts %q could not both be placed", tc.goKind, tc.tsKind)
		}
		if want == got {
			t.Fatalf("go %q and ts %q both read as %q, so the drift would pass unnoticed", tc.goKind, tc.tsKind, want)
		}
	}
}

func TestAliasesResolveThroughTheirConstants(t *testing.T) {
	source := `export const READY_STATES = ['True', 'False', 'Unknown', ''] as const;
export type ReadyState = (typeof READY_STATES)[number];
export type Protection = 'protected' | 'open' | 'unknown';
export type Nothing = SomeInterface;
`
	aliases := mirrorAliases(source)

	if aliases["ReadyState"] != "'True', 'False', 'Unknown', ''" {
		t.Fatalf("ReadyState resolved to %q", aliases["ReadyState"])
	}
	if aliases["Protection"] != "'protected' | 'open' | 'unknown'" {
		t.Fatalf("Protection resolved to %q", aliases["Protection"])
	}
	if aliases["Nothing"] != "SomeInterface" {
		t.Fatalf("Nothing resolved to %q", aliases["Nothing"])
	}
}

func TestAStringUnionIsToldFromTheRest(t *testing.T) {
	if !literals("'a' | 'b'") {
		t.Fatal("a union of string literals was not recognized")
	}
	if !literals("'True', 'False', ''") {
		t.Fatal("a const array of string literals was not recognized")
	}
	if literals("Row | null") {
		t.Fatal("a union naming an interface was read as string literals")
	}
	if literals("") {
		t.Fatal("an empty declaration was read as string literals")
	}
}
