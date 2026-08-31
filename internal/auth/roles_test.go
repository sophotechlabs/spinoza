package auth

import "testing"

func TestWithNoGroupListsEverybodyGetsTheDefaultRole(t *testing.T) {
	held := newRoleMap(Config{DefaultRole: RoleEditor})

	if got := held.forGroups([]string{"anything"}); got != RoleEditor {
		t.Fatalf("role = %q, want the default %q", got, RoleEditor)
	}
}

func TestTheStrongestMatchingGroupWins(t *testing.T) {
	held := newRoleMap(Config{
		DefaultRole:  RoleViewer,
		AdminGroups:  []string{"platform-admins"},
		EditorGroups: []string{"platform"},
		ViewerGroups: []string{"everyone"},
	})

	cases := []struct {
		name   string
		groups []string
		want   string
	}{
		{name: "an admin who is also an editor", groups: []string{"platform", "platform-admins"}, want: RoleAdmin},
		{name: "an editor", groups: []string{"platform", "everyone"}, want: RoleEditor},
		{name: "a viewer", groups: []string{"everyone"}, want: RoleViewer},
		{name: "somebody in none of them", groups: []string{"guests"}, want: RoleViewer},
		{name: "somebody in no group at all", groups: nil, want: RoleViewer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := held.forGroups(tc.groups); got != tc.want {
				t.Fatalf("role = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOnlyAViewerListStillCountsAsNamingGroups(t *testing.T) {
	held := newRoleMap(Config{DefaultRole: RoleAdmin, ViewerGroups: []string{"everyone"}})

	if got := held.forGroups([]string{"everyone"}); got != RoleViewer {
		t.Fatalf("role = %q, want %q from the viewer list", got, RoleViewer)
	}
	if got := held.forGroups([]string{"guests"}); got != RoleAdmin {
		t.Fatalf("role = %q, want the default %q", got, RoleAdmin)
	}
}
