package mcp

import (
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func TestReadingTextFromWhateverTypeArrived(t *testing.T) {
	args := arguments{"name": "web", "count": float64(3), "on": true, "list": []any{"a"}}

	cases := []struct {
		name string
		key  string
		want string
	}{
		{name: "a string", key: "name", want: "web"},
		{name: "a number", key: "count", want: "3"},
		{name: "a boolean", key: "on", want: "true"},
		{name: "something else", key: "list", want: ""},
		{name: "a key nobody sent", key: "missing", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := args.text(tc.key); got != tc.want {
				t.Fatalf("text(%q) = %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}

func TestARequiredArgumentSaysWhichOneIsMissing(t *testing.T) {
	args := arguments{"name": "   "}

	if _, err := args.required("namespace"); err == nil || err.Error() != "namespace is required" {
		t.Fatalf("error = %v, want it to name the argument", err)
	}
	if _, err := args.required("name"); err == nil {
		t.Fatal("whitespace was accepted as a value")
	}
}

func TestReadingNumbersFallsBackRatherThanFailing(t *testing.T) {
	args := arguments{"a": float64(7), "b": "12", "c": "twelve", "d": []any{}}

	cases := []struct {
		name string
		key  string
		want int
	}{
		{name: "a number", key: "a", want: 7},
		{name: "a number in a string", key: "b", want: 12},
		{name: "a word", key: "c", want: 5},
		{name: "another type", key: "d", want: 5},
		{name: "nothing", key: "e", want: 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := args.number(tc.key, 5); got != tc.want {
				t.Fatalf("number(%q) = %d, want %d", tc.key, got, tc.want)
			}
		})
	}
}

func TestLimitsStayPositiveAndBounded(t *testing.T) {
	args := arguments{"negative": float64(-1), "huge": float64(maxRows + 1), "small": float64(7)}

	if got := args.limit("negative", 5); got != 5 {
		t.Fatalf("negative limit = %d, want fallback 5", got)
	}
	if got := args.limit("huge", 5); got != maxRows {
		t.Fatalf("huge limit = %d, want cap %d", got, maxRows)
	}
	if got := args.limit("small", 5); got != 7 {
		t.Fatalf("small limit = %d, want 7", got)
	}
}

func TestReadingFlags(t *testing.T) {
	args := arguments{"a": true, "b": "true", "c": "yes", "d": float64(1)}

	cases := []struct {
		name string
		key  string
		want bool
	}{
		{name: "a boolean", key: "a", want: true},
		{name: "the word true", key: "b", want: true},
		{name: "another word", key: "c", want: false},
		{name: "a number", key: "d", want: false},
		{name: "nothing", key: "e", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := args.flag(tc.key); got != tc.want {
				t.Fatalf("flag(%q) = %v, want %v", tc.key, got, tc.want)
			}
		})
	}
}

func TestAChoiceListsWhatItWouldHaveAccepted(t *testing.T) {
	if _, err := (arguments{"action": "delete"}).oneOf("action", "scale", "restart"); err == nil {
		t.Fatal("an action outside the list was accepted")
	} else if !strings.Contains(err.Error(), "scale, restart") {
		t.Fatalf("error = %q, want it to list what is allowed", err)
	}
	if got, err := (arguments{"action": "scale"}).oneOf("action", "scale", "restart"); err != nil || got != "scale" {
		t.Fatalf("scale = %q, %v", got, err)
	}
	if _, err := (arguments{}).oneOf("action", "scale"); err == nil {
		t.Fatal("a missing action was accepted")
	}
}

func TestResolvingAResourceName(t *testing.T) {
	catalog := catalogOf(
		descriptor("apps", "v1", "deployments", "Deployment"),
		descriptor("", "v1", "pods", "Pod"),
	)

	cases := []struct {
		name     string
		asked    string
		group    string
		resource string
		fails    string
	}{
		{name: "by plural", asked: "deployments", resource: "deployments"},
		{name: "by kind", asked: "Deployment", resource: "deployments"},
		{name: "by kind in any case", asked: "deployment", resource: "deployments"},
		{name: "a core resource", asked: "pods", resource: "pods"},
		{name: "with the group named", asked: "deployments", group: "apps", resource: "deployments"},
		{name: "with the wrong group", asked: "deployments", group: "batch", fails: "no resource type"},
		{name: "one nobody serves", asked: "widgets", fails: "no resource type called \"widgets\""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			found, err := resolve(catalog, tc.asked, tc.group)
			if tc.fails != "" {
				if err == nil {
					t.Fatalf("resolve accepted %q", tc.asked)
				}
				if !strings.Contains(err.Error(), tc.fails) {
					t.Fatalf("error = %q, want it to contain %q", err, tc.fails)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolve(%q) failed: %v", tc.asked, err)
			}
			if found.Resource != tc.resource {
				t.Fatalf("resource = %q, want %q", found.Resource, tc.resource)
			}
		})
	}
}

func TestANameTwoGroupsServeAsksWhichOne(t *testing.T) {
	catalog := catalogOf(
		descriptor("apps", "v1", "deployments", "Deployment"),
		descriptor("extensions", "v1beta1", "deployments", "Deployment"),
	)

	_, err := resolve(catalog, "deployments", "")

	if err == nil {
		t.Fatal("an ambiguous name was resolved to one of them")
	}
	if !strings.Contains(err.Error(), "apps and extensions") {
		t.Fatalf("error = %q, want both groups named", err)
	}
}

func TestTheCoreGroupIsNamedInWords(t *testing.T) {
	catalog := catalogOf(
		descriptor("", "v1", "services", "Service"),
		descriptor("serving.knative.dev", "v1", "services", "Service"),
	)

	_, err := resolve(catalog, "services", "")

	if err == nil || !strings.Contains(err.Error(), "the core group") {
		t.Fatalf("error = %v, want the empty group described in words", err)
	}
}

func TestBuildingAnObjectReference(t *testing.T) {
	catalog := catalogOf(descriptor("apps", "v1", "deployments", "Deployment"))
	args := arguments{"resource": "deployments", "name": "web", "namespace": "prod"}

	ref, err := args.ref(catalog)
	if err != nil {
		t.Fatalf("ref failed: %v", err)
	}
	expected := api.ObjectRef{Group: "apps", Version: "v1", Resource: "deployments", Namespace: "prod", Name: "web"}
	if ref != expected {
		t.Fatalf("ref = %+v, want %+v", ref, expected)
	}
}

func TestAReferenceNeedsBothAResourceAndAName(t *testing.T) {
	catalog := catalogOf(descriptor("apps", "v1", "deployments", "Deployment"))

	if _, err := (arguments{"name": "web"}).ref(catalog); err == nil {
		t.Fatal("a reference with no resource was built")
	}
	if _, err := (arguments{"resource": "deployments"}).ref(catalog); err == nil {
		t.Fatal("a reference with no name was built")
	}
	if _, err := (arguments{"resource": "widgets", "name": "web"}).ref(catalog); err == nil {
		t.Fatal("a reference to a kind nobody serves was built")
	}
}

func TestBuildingAKindReferenceNeedsNoName(t *testing.T) {
	catalog := catalogOf(descriptor("apps", "v1", "deployments", "Deployment"))

	ref, err := (arguments{"resource": "deployments", "namespace": "prod"}).kind(catalog)
	if err != nil {
		t.Fatalf("kind failed: %v", err)
	}
	if ref.Name != "" || ref.Resource != "deployments" || ref.Namespace != "prod" {
		t.Fatalf("ref = %+v", ref)
	}
	if _, err := (arguments{}).kind(catalog); err == nil {
		t.Fatal("a kind reference with no resource was built")
	}
	if _, err := (arguments{"resource": "widgets"}).kind(catalog); err == nil {
		t.Fatal("a kind reference to something nobody serves was built")
	}
}
