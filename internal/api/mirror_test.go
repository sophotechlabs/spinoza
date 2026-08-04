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
	"ClientMsg": "ClientMsg",
	"ServerMsg": "ServerMsg",
	"Snapshot":  "ServerMsg",
}

var neverReachesTheBrowser = map[string]bool{
	"Build":  true,
	"Health": true,
}

var regroupedIntoNestedViewTypes = map[string]bool{
	"ObjectDetail": true,
}

type property struct {
	name     string
	optional bool
}

var (
	interfaceHead = regexp.MustCompile(`^export interface (\w+) \{$`)
	unionHead     = regexp.MustCompile(`^export type (\w+) =$`)
	propertyLine  = regexp.MustCompile(`^\s+(\w+)(\??):`)
	unionProperty = regexp.MustCompile(`(\w+)(\??):`)
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
			out[current] = append(out[current], property{name: field[1], optional: field[2] == "?"})
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
		if len(parts) != 2 {
			t.Fatalf("unreadable contract line %q", line)
		}
		typeName, _, _ := strings.Cut(parts[0], ".")
		tag, options, _ := strings.Cut(parts[1], ",")
		if tag == "" {
			continue
		}
		out[typeName] = append(out[typeName], property{name: tag, optional: strings.Contains(options, "omitempty")})
	}
	return out
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
