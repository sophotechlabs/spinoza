package api_test

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const mirrorPath = "../../frontend/src/lib/types.ts"

var namedDifferentlyInTypeScript = map[string]string{
	"Event": "K8sEvent",
}

var sentAsADiscriminatedUnion = map[string]string{
	"ClientMsg":  "ClientMsg",
	"ServerMsg":  "ServerMsg",
	"Snapshot":   "ServerMsg",
	"RowChanged": "ServerMsg",
	"RowDeleted": "ServerMsg",
	"RowChange":  "ServerMsg",
	"RowBatch":   "ServerMsg",
	"LogLines":   "ServerMsg",
	"LogEnd":     "ServerMsg",
	"FeedError":  "ServerMsg",
}

var neverReachesTheBrowser = map[string]bool{
	"Build":            true,
	"Health":           true,
	"NodeShellSession": true,
}

var regroupedIntoNestedViewTypes = map[string]bool{
	"ObjectDetail": true,
}

// fields whose two sides are deliberately typed differently, with the reason.
var typedDifferentlyInTypeScript = map[string]string{}

type property struct {
	name     string
	optional bool
	kind     string
}

var (
	interfaceHead = regexp.MustCompile(`^export interface (\w+) \{$`)
	unionHead     = regexp.MustCompile(`^export type (\w+) =$`)
	propertyLine  = regexp.MustCompile(`^\s+(\w+)(\??): (.+?);?$`)
	unionProperty = regexp.MustCompile(`(\w+)(\??):`)
	aliasLine     = regexp.MustCompile(`^export type (\w+) = (.+?);$`)
	constLine     = regexp.MustCompile(`^export const (\w+) = \[(.*?)\] as const;$`)
	fromConstants = regexp.MustCompile(`^\(typeof (\w+)\)\[number\]$`)
)

func readMirror(t *testing.T) string {
	t.Helper()
	source, err := os.ReadFile(mirrorPath)
	if err != nil {
		t.Fatalf("read %s: %v", mirrorPath, err)
	}
	return string(source)
}

func mirrorInterfaces(source string) map[string][]property {
	out := map[string][]property{}
	current := ""
	for line := range strings.SplitSeq(source, "\n") {
		if head := interfaceHead.FindStringSubmatch(line); head != nil {
			current = head[1]
			out[current] = []property{}
			continue
		}
		if line == "}" {
			current = ""
			continue
		}
		if current == "" {
			continue
		}
		if field := propertyLine.FindStringSubmatch(line); field != nil {
			out[current] = append(out[current], property{name: field[1], optional: field[2] == "?", kind: field[3]})
		}
	}
	return out
}

func mirrorUnions(source string) map[string][]string {
	out := map[string][]string{}
	current := ""
	for line := range strings.SplitSeq(source, "\n") {
		if head := unionHead.FindStringSubmatch(line); head != nil {
			current = head[1]
			out[current] = []string{}
			continue
		}
		if current == "" {
			continue
		}
		if line == "" {
			current = ""
			continue
		}
		for _, match := range unionProperty.FindAllStringSubmatch(line, -1) {
			out[current] = append(out[current], match[1])
		}
	}
	return out
}

func goProperties(t *testing.T) map[string][]property {
	t.Helper()
	out := map[string][]property{}
	for line := range strings.SplitSeq(wireContract(t), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 3 {
			t.Fatalf("unreadable contract line %q", line)
		}
		typeName, _, _ := strings.Cut(parts[0], ".")
		tag, options, _ := strings.Cut(parts[1], ",")
		if tag == "" {
			continue
		}
		out[typeName] = append(out[typeName], property{
			name:     tag,
			optional: strings.Contains(options, "omitempty"),
			kind:     parts[2],
		})
	}
	return out
}

// the shapes a JSON value can take, which is all the two sides have to agree on.
const (
	jsonString  = "string"
	jsonNumber  = "number"
	jsonBoolean = "boolean"
	jsonArray   = "array"
	jsonObject  = "object"
	jsonAny     = "any"
)

var goNumbers = regexp.MustCompile(`^(u?int(8|16|32|64)?|float(32|64)|byte|rune)$`)

func goShape(kind string) (string, bool) {
	bare := strings.TrimPrefix(kind, "*")
	switch {
	case bare == "[]byte":
		return jsonString, true
	case strings.HasPrefix(bare, "[]"):
		return jsonArray, true
	case strings.HasPrefix(bare, "map["):
		return jsonObject, true
	case bare == "string":
		return jsonString, true
	case bare == "bool":
		return jsonBoolean, true
	case goNumbers.MatchString(bare):
		return jsonNumber, true
	case bare == "json.RawMessage" || bare == "any":
		return jsonAny, true
	case bare == "time.Time":
		return jsonString, true
	case bare == "time.Duration":
		return jsonNumber, true
	case strings.Contains(bare, "."):
		return "", false
	case bare != "" && strings.ToUpper(bare[:1]) == bare[:1]:
		return jsonObject, true
	}
	return "", false
}

// aliases resolve a named TypeScript type to the text it stands for.
func mirrorAliases(source string) map[string]string {
	constants := map[string]string{}
	for line := range strings.SplitSeq(source, "\n") {
		if found := constLine.FindStringSubmatch(line); found != nil {
			constants[found[1]] = found[2]
		}
	}
	out := map[string]string{}
	for line := range strings.SplitSeq(source, "\n") {
		found := aliasLine.FindStringSubmatch(line)
		if found == nil {
			continue
		}
		text := found[2]
		if members := fromConstants.FindStringSubmatch(text); members != nil {
			text = constants[members[1]]
		}
		out[found[1]] = text
	}
	return out
}

func tsShape(kind string, aliases map[string]string, interfaces map[string][]property, depth int) (string, bool) {
	if depth > 4 {
		return "", false
	}
	bare := kind
	for _, nullable := range []string{"| null", "| undefined"} {
		bare = strings.ReplaceAll(bare, nullable, "")
	}
	bare = strings.TrimSpace(bare)
	switch {
	case strings.HasSuffix(bare, "[]"), strings.HasPrefix(bare, "Array<"):
		return jsonArray, true
	case strings.HasPrefix(bare, "Record<"), strings.HasPrefix(bare, "{"), strings.HasPrefix(bare, "Partial<"):
		return jsonObject, true
	case bare == "string":
		return jsonString, true
	case bare == "number":
		return jsonNumber, true
	case bare == "boolean":
		return jsonBoolean, true
	case bare == "unknown", bare == "any":
		return jsonAny, true
	}
	if literals(bare) {
		return jsonString, true
	}
	if resolved, ok := aliases[bare]; ok {
		return tsShape(resolved, aliases, interfaces, depth+1)
	}
	if _, ok := interfaces[bare]; ok {
		return jsonObject, true
	}
	return "", false
}

// a union of string literals is a string on the wire.
func literals(kind string) bool {
	arms := strings.Split(kind, "|")
	for _, arm := range arms {
		arm = strings.TrimSpace(arm)
		if !strings.HasPrefix(arm, "'") || !strings.HasSuffix(arm, "'") {
			return false
		}
	}
	return len(arms) > 0
}

func mirroredName(typeName string) string {
	renamed, ok := namedDifferentlyInTypeScript[typeName]
	if ok {
		return renamed
	}
	return typeName
}

func compared(typeName string) bool {
	if neverReachesTheBrowser[typeName] {
		return false
	}
	_, union := sentAsADiscriminatedUnion[typeName]
	return !union
}

func names(properties []property) []string {
	out := make([]string, 0, len(properties))
	for _, entry := range properties {
		out = append(out, entry.name)
	}
	slices.Sort(out)
	return out
}

func optionals(properties []property) []string {
	out := []string{}
	for _, entry := range properties {
		if entry.optional {
			out = append(out, entry.name)
		}
	}
	slices.Sort(out)
	return out
}

func TestEveryWireTypeReachesTheFrontend(t *testing.T) {
	source := readMirror(t)
	interfaces := mirrorInterfaces(source)
	unions := mirrorUnions(source)

	for typeName := range goProperties(t) {
		if !compared(typeName) {
			continue
		}
		mirrored := mirroredName(typeName)
		if _, ok := interfaces[mirrored]; ok {
			continue
		}
		if _, ok := unions[mirrored]; ok {
			continue
		}
		t.Errorf("%s has no counterpart in %s; mirror it or list it as one the browser never reads", typeName, mirrorPath)
	}
}

func TestTheFrontendInterfacesCarryTheSameFields(t *testing.T) {
	interfaces := mirrorInterfaces(readMirror(t))

	for typeName, fields := range goProperties(t) {
		if !compared(typeName) || regroupedIntoNestedViewTypes[typeName] {
			continue
		}
		found, ok := interfaces[mirroredName(typeName)]
		if !ok {
			continue
		}
		t.Run(typeName, func(t *testing.T) {
			if !slices.Equal(names(fields), names(found)) {
				t.Fatalf("fields differ:\n  go: %v\n  ts: %v", names(fields), names(found))
			}
			if !slices.Equal(optionals(fields), optionals(found)) {
				t.Fatalf("optional fields differ:\n  go: %v\n  ts: %v", optionals(fields), optionals(found))
			}
		})
	}
}

func shapeOf(t *testing.T, entry property) string {
	t.Helper()
	shape, ok := goShape(entry.kind)
	if !ok {
		t.Fatalf("%s is a Go type this test cannot place on the wire; teach goShape about it", entry.kind)
	}
	return shape
}

func TestTheFrontendInterfacesCarryTheSameTypes(t *testing.T) {
	source := readMirror(t)
	interfaces := mirrorInterfaces(source)
	aliases := mirrorAliases(source)

	for typeName, fields := range goProperties(t) {
		if !compared(typeName) || regroupedIntoNestedViewTypes[typeName] {
			continue
		}
		found, ok := interfaces[mirroredName(typeName)]
		if !ok {
			continue
		}
		mirrored := map[string]property{}
		for _, entry := range found {
			mirrored[entry.name] = entry
		}
		t.Run(typeName, func(t *testing.T) {
			for _, entry := range fields {
				other, present := mirrored[entry.name]
				if !present {
					continue
				}
				if reason, allowed := typedDifferentlyInTypeScript[typeName+"."+entry.name]; allowed {
					t.Logf("%s.%s is typed differently on purpose: %s", typeName, entry.name, reason)
					continue
				}
				want := shapeOf(t, entry)
				got, readable := tsShape(other.kind, aliases, interfaces, 0)
				if !readable {
					t.Fatalf("%s.%s is %q in %s, which this test cannot place on the wire",
						typeName, entry.name, other.kind, mirrorPath)
				}
				if want == jsonAny || got == jsonAny {
					continue
				}
				if want != got {
					t.Fatalf("%s.%s is %s on the wire but %s in the browser:\n  go: %s\n  ts: %s",
						typeName, entry.name, want, got, entry.kind, other.kind)
				}
			}
		})
	}
}

func TestTheMessageUnionsCoverEveryWireField(t *testing.T) {
	unions := mirrorUnions(readMirror(t))
	fields := goProperties(t)

	for typeName, unionName := range sentAsADiscriminatedUnion {
		t.Run(typeName, func(t *testing.T) {
			members, ok := unions[unionName]
			if !ok {
				t.Fatalf("%s is not a union in %s", unionName, mirrorPath)
			}
			for _, field := range fields[typeName] {
				if !slices.Contains(members, field.name) {
					t.Fatalf("%s.%s reaches no arm of the %s union in %s", typeName, field.name, unionName, mirrorPath)
				}
			}
		})
	}
}

func TestTheMessageUnionsInventNoFields(t *testing.T) {
	unions := mirrorUnions(readMirror(t))
	fields := goProperties(t)

	for typeName, unionName := range sentAsADiscriminatedUnion {
		if typeName != unionName {
			continue
		}
		t.Run(unionName, func(t *testing.T) {
			known := names(fields[typeName])
			for _, member := range unions[unionName] {
				if !slices.Contains(known, member) {
					t.Fatalf("the %s union carries %q, which no longer exists on the Go side", unionName, member)
				}
			}
		})
	}
}

func TestTheReshapedTypesAreStillDeclaredOnBothSides(t *testing.T) {
	interfaces := mirrorInterfaces(readMirror(t))
	fields := goProperties(t)

	for typeName := range regroupedIntoNestedViewTypes {
		if len(fields[typeName]) == 0 {
			t.Fatalf("%s is listed as regrouped but no longer exists on the Go side", typeName)
		}
		if _, ok := interfaces[mirroredName(typeName)]; !ok {
			t.Fatalf("%s is listed as regrouped but no longer exists in %s", typeName, mirrorPath)
		}
	}
}
