package resources

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const maxCustomColumns = 8

type Columns func() map[string][]api.CustomColumn

func (m *Manager) customFor(desc api.ResourceDescriptor) []*declaredColumn {
	if m.columns == nil {
		return nil
	}
	held := m.columns()
	if len(held) == 0 {
		return nil
	}
	wanted := held[keyOf(desc)]
	out := make([]*declaredColumn, 0, len(wanted))
	for _, one := range wanted {
		column, made := customColumnOf(one)
		if !made {
			continue
		}
		out = append(out, column)
		if len(out) == maxCustomColumns {
			break
		}
	}
	return out
}

func customColumnOf(one api.CustomColumn) (*declaredColumn, bool) {
	name := strings.TrimSpace(one.Name)
	path := strings.TrimSpace(one.Path)
	if name == "" || path == "" {
		return nil, false
	}
	parsed, err := parsePath(name, path)
	if err != nil {
		return nil, false
	}
	column := &declaredColumn{name: name, template: path}
	if !ranges(name, path) {
		column.kept = parsed
	}
	return column, true
}

func withCustom(base layout, custom []*declaredColumn) layout {
	if len(custom) == 0 {
		return base
	}
	columns := make([]api.Column, 0, len(base.columns)+len(custom))
	columns = append(columns, base.columns...)
	for _, one := range custom {
		columns = append(columns, api.Column{Name: one.name})
	}
	return layout{
		columns: columns,
		cells: func(obj *unstructured.Unstructured) []string {
			out := base.cells(obj)
			for at := range custom {
				out = append(out, custom[at].read(obj))
			}
			return out
		},
	}
}
