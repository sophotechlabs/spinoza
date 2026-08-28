package resources

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/jsonpath"

	"github.com/sophotechlabs/spinoza/internal/api"
)

var crdGVR = schema.GroupVersionResource{
	Group:    "apiextensions.k8s.io",
	Version:  "v1",
	Resource: "customresourcedefinitions",
}

const (
	crdReadTimeout = 5 * time.Second
	layoutTTL      = time.Minute
)

type layout struct {
	columns []api.Column
	cells   func(obj *unstructured.Unstructured) []string
}

func builtinLayout(kind string) layout {
	return layout{
		columns: columnsFor(kind),
		cells: func(obj *unstructured.Unstructured) []string {
			return cellsFor(obj, kind)
		},
	}
}

func (m *Manager) layoutFor(
	ctx context.Context,
	desc api.ResourceDescriptor,
	gvr schema.GroupVersionResource,
) layout {
	_, ours := builtinColumns(desc.Kind)
	if ours {
		return builtinLayout(desc.Kind)
	}
	built, ok := shared(ctx, m.layoutStore(gvr), m.now, layoutTTL, func(ctx context.Context) (layout, bool) {
		declared, found := m.crdLayout(ctx, gvr)
		if !found {
			return builtinLayout(desc.Kind), true
		}
		return declared, true
	})
	if !ok {
		return builtinLayout(desc.Kind)
	}
	return built
}

func (m *Manager) layoutStore(gvr schema.GroupVersionResource) *recent[layout] {
	m.layoutMu.Lock()
	defer m.layoutMu.Unlock()
	store, ok := m.layouts[gvr]
	if ok {
		return store
	}
	store = &recent[layout]{}
	m.layouts[gvr] = store
	return store
}

func (m *Manager) forgetLayouts() {
	m.layoutMu.Lock()
	defer m.layoutMu.Unlock()
	m.layouts = map[schema.GroupVersionResource]*recent[layout]{}
}

// Failure is not fatal: many users cannot read CRDs.
func (m *Manager) crdLayout(ctx context.Context, gvr schema.GroupVersionResource) (layout, bool) {
	if m.dyn == nil {
		return layout{}, false
	}
	// Only CRDs declare them; core kinds are built into the apiserver.
	if gvr.Group == "" {
		return layout{}, false
	}
	bounded, cancel := context.WithTimeout(ctx, crdReadTimeout)
	defer cancel()
	definition, err := m.dyn.Resource(crdGVR).
		Get(bounded, gvr.Resource+"."+gvr.Group, metav1.GetOptions{})
	if err != nil {
		return layout{}, false
	}
	return layoutOf(definition, gvr.Version)
}

func layoutOf(definition *unstructured.Unstructured, version string) (layout, bool) {
	declared := printerColumnsOf(definition, version)
	if len(declared) == 0 {
		return layout{}, false
	}
	columns := make([]api.Column, 0, len(declared))
	for _, one := range declared {
		columns = append(columns, api.Column{Name: one.name, Render: one.render})
	}
	return layout{columns: columns, cells: cellsFromDeclared(declared)}, true
}

func cellsFromDeclared(declared []*declaredColumn) func(*unstructured.Unstructured) []string {
	return func(obj *unstructured.Unstructured) []string {
		out := make([]string, 0, len(declared))
		for i := range declared {
			out = append(out, declared[i].read(obj))
		}
		return out
	}
}

type declaredColumn struct {
	name     string
	render   string
	template string
	// A ranging template rewrites its own parse tree and is spent after one
	// object; the rest write nothing while reading.
	kept *jsonpath.JSONPath
}

func (c *declaredColumn) read(obj *unstructured.Unstructured) string {
	if c.kept != nil {
		return valuesFrom(c.kept, obj)
	}
	parsed, err := parsePath(c.name, c.template)
	if err != nil {
		return ""
	}
	return valuesFrom(parsed, obj)
}

func valuesFrom(path *jsonpath.JSONPath, obj *unstructured.Unstructured) string {
	found, err := path.FindResults(obj.Object)
	if err != nil {
		return ""
	}
	parts := []string{}
	for _, group := range found {
		for _, value := range group {
			parts = append(parts, fmt.Sprintf("%v", value.Interface()))
		}
	}
	return strings.Join(parts, ", ")
}

func printerColumnsOf(definition *unstructured.Unstructured, version string) []*declaredColumn {
	entries, ok := entriesFor(definition, version)
	if !ok {
		return nil
	}
	out := make([]*declaredColumn, 0, len(entries))
	for _, entry := range entries {
		column, made := declaredColumnOf(entry)
		if !made {
			continue
		}
		out = append(out, column)
	}
	return out
}

func entriesFor(definition *unstructured.Unstructured, version string) ([]any, bool) {
	versions, found, err := unstructured.NestedSlice(definition.Object, "spec", "versions")
	if err != nil {
		return nil, false
	}
	if !found {
		return nil, false
	}
	for _, entry := range versions {
		fields, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if stringAt(fields, "name") != version {
			continue
		}
		columns, ok := fields["additionalPrinterColumns"].([]any)
		if !ok {
			return nil, false
		}
		return columns, true
	}
	return nil, false
}

func declaredColumnOf(entry any) (*declaredColumn, bool) {
	fields, ok := entry.(map[string]any)
	if !ok {
		return nil, false
	}
	name := stringAt(fields, "name")
	if name == "" {
		return nil, false
	}
	path := stringAt(fields, "jsonPath")
	if path == "" {
		return nil, false
	}
	if alreadyShown(path) {
		return nil, false
	}
	if numberAt(fields, "priority") > 0 {
		return nil, false
	}
	parsed, err := parsePath(name, path)
	if err != nil {
		return nil, false
	}
	column := &declaredColumn{
		name:     name,
		render:   renderFor(name, stringAt(fields, "type")),
		template: path,
	}
	if !ranges(name, path) {
		column.kept = parsed
	}
	return column, true
}

func ranges(name, path string) bool {
	tree, err := jsonpath.Parse(name, braced(path))
	if err != nil {
		return false
	}
	return listRanges(tree.Root)
}

// A range sits one level down, inside a {...} group. No deeper: the parser
// rejects a brace in a filter or a union.
func listRanges(list *jsonpath.ListNode) bool {
	return slices.ContainsFunc(list.Nodes, nodeRanges)
}

func nodeRanges(node jsonpath.Node) bool {
	inner, nested := node.(*jsonpath.ListNode)
	if nested {
		return listRanges(inner)
	}
	identifier, named := node.(*jsonpath.IdentifierNode)
	if !named {
		return false
	}
	return identifier.Name == "range"
}

func alreadyShown(path string) bool {
	trimmed := strings.TrimSpace(path)
	if trimmed == ".metadata.name" {
		return true
	}
	return trimmed == ".metadata.creationTimestamp"
}

func renderFor(name, declared string) string {
	if declared == "date" {
		return "age"
	}
	if working(name) {
		return "condition"
	}
	return ""
}

// working names columns whose True means healthy; Suspended and Paused are not.
func working(name string) bool {
	for _, known := range []string{"Ready", "Healthy", "Available", "Synced", "Established", "Reconciled"} {
		if strings.EqualFold(name, known) {
			return true
		}
	}
	return false
}

func parsePath(name, path string) (*jsonpath.JSONPath, error) {
	parser := jsonpath.New(name)
	// Definitions write the path without braces, the way kubectl takes it.
	parser.AllowMissingKeys(true)
	err := parser.Parse(braced(path))
	if err != nil {
		return nil, err
	}
	return parser, nil
}

func braced(path string) string {
	trimmed := strings.TrimSpace(path)
	if strings.HasPrefix(trimmed, "{") {
		return trimmed
	}
	return "{" + trimmed + "}"
}

func stringAt(fields map[string]any, key string) string {
	value, ok := fields[key].(string)
	if !ok {
		return ""
	}
	return value
}

func numberAt(fields map[string]any, key string) int64 {
	value, ok := fields[key].(int64)
	if ok {
		return value
	}
	asFloat, ok := fields[key].(float64)
	if ok {
		return int64(asFloat)
	}
	return 0
}
