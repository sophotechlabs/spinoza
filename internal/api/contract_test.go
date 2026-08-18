package api_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const goldenPath = "testdata/wire-contract.txt"

func wireContract(t *testing.T) string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "types.go", nil, 0)
	if err != nil {
		t.Fatalf("parse types.go: %v", err)
	}

	lines := []string{}
	ast.Inspect(file, func(node ast.Node) bool {
		spec, ok := node.(*ast.TypeSpec)
		if !ok {
			return true
		}
		structType, ok := spec.Type.(*ast.StructType)
		if !ok {
			return true
		}
		for _, field := range structType.Fields.List {
			lines = append(lines, fieldLines(spec.Name.Name, field)...)
		}
		return true
	})
	slices.Sort(lines)
	return strings.Join(lines, "\n") + "\n"
}

func fieldLines(typeName string, field *ast.Field) []string {
	tag := jsonTag(field)
	if tag == "-" {
		return nil
	}
	kind := types.ExprString(field.Type)
	out := []string{}
	for _, name := range field.Names {
		if !name.IsExported() {
			continue
		}
		out = append(out, fmt.Sprintf("%s.%s %s %s", typeName, name.Name, tag, kind))
	}
	return out
}

func jsonTag(field *ast.Field) string {
	if field.Tag == nil {
		return "(no tag)"
	}
	raw := strings.Trim(field.Tag.Value, "`")
	return reflect.StructTag(raw).Get("json")
}

func TestWireContractIsUnchanged(t *testing.T) {
	got := wireContract(t)
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read %s: %v (run with -update to create it)", goldenPath, err)
	}

	if got == string(want) {
		return
	}
	if os.Getenv("UPDATE_WIRE_CONTRACT") == "1" {
		writeErr := os.WriteFile(goldenPath, []byte(got), 0o600)
		if writeErr != nil {
			t.Fatalf("write golden: %v", writeErr)
		}
		t.Fatal("wire contract updated; re-run to confirm and update frontend/src/lib/types.ts to match")
	}
	t.Fatalf(`the JSON wire contract changed.

frontend/src/lib/types.ts mirrors these names and types by hand, so a renamed tag or a
changed field type compiles on both sides and only fails at runtime. Update types.ts,
then refresh the golden with:

    UPDATE_WIRE_CONTRACT=1 go test ./internal/api/

diff (want on the left, got on the right):
%s`, contractDiff(string(want), got))
}

func contractDiff(want, got string) string {
	wantLines := map[string]bool{}
	for line := range strings.SplitSeq(want, "\n") {
		wantLines[line] = true
	}
	gotLines := map[string]bool{}
	for line := range strings.SplitSeq(got, "\n") {
		gotLines[line] = true
	}
	out := []string{}
	for line := range wantLines {
		if line != "" && !gotLines[line] {
			out = append(out, "  removed: "+line)
		}
	}
	for line := range gotLines {
		if line != "" && !wantLines[line] {
			out = append(out, "  added:   "+line)
		}
	}
	slices.Sort(out)
	return "\n" + strings.Join(out, "\n")
}

func TestTheGoldenFileSitsNextToTheTypes(t *testing.T) {
	_, err := os.Stat(filepath.Join("testdata", "wire-contract.txt"))
	if err != nil {
		t.Fatalf("golden file missing: %v", err)
	}
}
