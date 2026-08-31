package auth

import (
	"strings"
	"testing"
)

func TestTheFirstUsernameClaimPresentWins(t *testing.T) {
	keys := strings.Split(DefaultUsernameKeys, ",")

	cases := []struct {
		name   string
		claims claimSet
		want   string
	}{
		{name: "keycloak", claims: claimSet{"preferred_username": "alice", "email": "a@b", "sub": "1"}, want: "alice"},
		{name: "no preferred name", claims: claimSet{"email": "a@b", "sub": "1"}, want: "a@b"},
		{name: "only a subject", claims: claimSet{"sub": "1"}, want: "1"},
		{name: "an empty claim is skipped", claims: claimSet{"preferred_username": "", "email": "a@b"}, want: "a@b"},
		{name: "a claim of the wrong shape is skipped", claims: claimSet{"preferred_username": 7, "email": "a@b"}, want: "a@b"},
		{name: "nothing usable", claims: claimSet{"name": "alice"}, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := usernameFrom(tc.claims, keys); got != tc.want {
				t.Fatalf("username = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGroupsAreReadFromEveryShapeAProviderSends(t *testing.T) {
	cases := []struct {
		name   string
		claims claimSet
		want   string
	}{
		{name: "a json array", claims: claimSet{"groups": []any{"platform", "sre"}}, want: "platform,sre"},
		{name: "a go slice", claims: claimSet{"groups": []string{"platform"}}, want: "platform"},
		{name: "a comma separated string", claims: claimSet{"groups": "platform, sre"}, want: "platform,sre"},
		{name: "entries that are not strings", claims: claimSet{"groups": []any{"platform", 7, ""}}, want: "platform"},
		{name: "a shape nobody expected", claims: claimSet{"groups": 7}, want: ""},
		{name: "no claim at all", claims: claimSet{}, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := strings.Join(groupsFrom(tc.claims, "groups"), ","); got != tc.want {
				t.Fatalf("groups = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAPrefixGoesInFrontOfEveryGroup(t *testing.T) {
	got := prefixed([]string{"platform", "sre"}, "oidc:")

	if strings.Join(got, ",") != "oidc:platform,oidc:sre" {
		t.Fatalf("groups = %v, want both prefixed", got)
	}
	if strings.Join(prefixed([]string{"platform"}, ""), ",") != "platform" {
		t.Fatal("an empty prefix changed the groups")
	}
}

func TestASessionIDComesFromTheTokenWhenItHasOne(t *testing.T) {
	if got := sessionIDFrom(claimSet{"sid": "session-7"}); got != "session-7" {
		t.Fatalf("session = %q, want the one in the token", got)
	}
	made := sessionIDFrom(claimSet{})
	if made == "" {
		t.Fatal("a token with no sid produced no session id, so nothing could be revoked")
	}
	if made == sessionIDFrom(claimSet{"sid": ""}) {
		t.Fatal("two invented session ids were the same")
	}
}
