package resources

import (
	"context"
	"fmt"
	"strings"
	"sync"
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

const crdReadTimeout = 5 * time.Second

// layout is how one kind is shown in a table: the columns, and how to fill a
// row from an object.
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

// layoutFor is how this kind should be shown. Spinoza's own tables come first,
// then whatever the resource's own definition asks for, and a single status
// when there is neither.
func (m *Manager) layoutFor(
	ctx context.Context,
	desc api.ResourceDescriptor,
	gvr schema.GroupVersionResource,
) layout {
	_, ours := builtinColumns(desc.Kind)
	if ours {
		return builtinLayout(desc.Kind)
	}
	declared, ok := m.crdLayout(ctx, gvr)
	if !ok {
		return builtinLayout(desc.Kind)
	}
	return declared
}

// crdLayout reads the columns a custom resource asks to be shown by. Every CRD
// may publish them, and they are what kubectl prints — the author of the kind
// knows better than a table that can only say whether something is ready.
//
// Nothing here may stop a table opening: plenty of users cannot read custom
// resource definitions at all, and a kind spinoza cannot ask about is simply
// shown the way it always was.
func (m *Manager) crdLayout(ctx context.Context, gvr schema.GroupVersionResource) (layout, bool) {
	if m.dyn == nil {
		return layout{}, false
	}
	// Only what a CRD defines has one. Core kinds are built into the apiserver.
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

// declaredColumn is one column a resource definition asks for, ready to read.
type declaredColumn struct {
	name   string
	render string
	// A parsed path keeps counters on itself while it walks a range expression,
	// so it is not something two goroutines may read through at once. Rows are
	// built both by the informer and by whoever is taking a snapshot, which is
	// exactly two.
	mu   sync.Mutex
	path *jsonpath.JSONPath
}

func (c *declaredColumn) read(obj *unstructured.Unstructured) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	found, err := c.path.FindResults(obj.Object)
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

// entriesFor finds the printer columns for the version being listed. A
// definition serves several versions and they need not agree on their columns.
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
	// A column the table already has its own place for, and one kubectl only
	// shows when asked for a wide table.
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
	return &declaredColumn{
		name:   name,
		render: renderFor(stringAt(fields, "type")),
		path:   parsed,
	}, true
}

// alreadyShown names the two fields every table already has a column for.
func alreadyShown(path string) bool {
	trimmed := strings.TrimSpace(path)
	if trimmed == ".metadata.name" {
		return true
	}
	return trimmed == ".metadata.creationTimestamp"
}

// renderFor turns a declared type into the way spinoza draws that cell. Only a
// date is worth knowing about: it is the difference between a timestamp nobody
// reads and the ages shown everywhere else.
func renderFor(declared string) string {
	if declared == "date" {
		return "age"
	}
	return ""
}

func parsePath(name, path string) (*jsonpath.JSONPath, error) {
	parser := jsonpath.New(name)
	// A definition writes the path the way kubectl takes it on the command line,
	// without the braces the parser wants.
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
