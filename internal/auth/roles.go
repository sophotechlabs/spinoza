package auth

import "slices"

type roleMap struct {
	admin    []string
	editor   []string
	viewer   []string
	fallback string
}

func newRoleMap(cfg Config) roleMap {
	return roleMap{
		admin:    slices.Clone(cfg.AdminGroups),
		editor:   slices.Clone(cfg.EditorGroups),
		viewer:   slices.Clone(cfg.ViewerGroups),
		fallback: cfg.DefaultRole,
	}
}

func (rm roleMap) named() bool {
	if len(rm.admin) > 0 {
		return true
	}
	if len(rm.editor) > 0 {
		return true
	}
	return len(rm.viewer) > 0
}

func (rm roleMap) forGroups(groups []string) string {
	if !rm.named() {
		return rm.fallback
	}
	if anyOf(groups, rm.admin) {
		return RoleAdmin
	}
	if anyOf(groups, rm.editor) {
		return RoleEditor
	}
	if anyOf(groups, rm.viewer) {
		return RoleViewer
	}
	return rm.fallback
}

func anyOf(held, wanted []string) bool {
	for _, one := range wanted {
		if slices.Contains(held, one) {
			return true
		}
	}
	return false
}
