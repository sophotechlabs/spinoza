package resources

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/auth"
)

func TestSearchReadsOnlyTheNamespacesGrantedToTheCurrentIdentity(t *testing.T) {
	for _, tc := range []struct {
		name    string
		allowed []string
		want    []string
	}{
		{name: "one namespace", allowed: []string{"payments"}, want: []string{"web-payments"}},
		{name: "two namespaces", allowed: []string{"payments", "storefront"}, want: []string{"web-payments", "web-storefront"}},
		{name: "no namespaces", allowed: []string{}, want: []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fakeMeta(
				t,
				meta("", "v1", "Namespace", "", "payments"),
				meta("", "v1", "Namespace", "", "storefront"),
				meta("", "v1", "Namespace", "", "private"),
				meta("", "v1", "Pod", "payments", "web-payments"),
				meta("", "v1", "Pod", "storefront", "web-storefront"),
				meta("", "v1", "Pod", "private", "web-private"),
			)
			manager := NewManager(t.Context(), Deps{
				Metadata: client, Perms: boundTo(tc.allowed...),
				Descriptors: map[string]api.ResourceDescriptor{"/v1/pods": descriptorsFor("/v1/pods")[0]},
			})
			unscoped := manager.Search(t.Context(), "web")
			if len(unscoped.Hits) != 3 {
				t.Fatalf("local search = %+v, want all three pods", unscoped)
			}
			client.ClearActions()
			found := manager.Search(alice(t), "web")
			if !reflect.DeepEqual(names(found), tc.want) || len(found.Errors) != 0 || found.Truncated {
				t.Fatalf("scoped search = %+v, want %v without errors or truncation", found, tc.want)
			}
			requested := []string{}
			for _, action := range client.Actions() {
				if action.GetResource().Resource == "pods" {
					requested = append(requested, action.GetNamespace())
				}
			}
			slices.Sort(requested)
			if !reflect.DeepEqual(requested, tc.allowed) {
				t.Fatalf("pod reads = %v, want exactly the admitted namespaces %v", requested, tc.allowed)
			}
		})
	}
}

func TestAScopedSearchKeepsReadableResultsAndNamesTheNamespaceThatRefused(t *testing.T) {
	client := fakeMeta(
		t,
		meta("", "v1", "Namespace", "", "payments"),
		meta("", "v1", "Namespace", "", "storefront"),
		meta("", "v1", "Pod", "payments", "web-payments"),
	)
	refused := errors.New("storefront pod listing was forbidden")
	client.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetNamespace() == "storefront" {
			return true, nil, refused
		}
		return false, nil, nil
	})
	manager := NewManager(t.Context(), Deps{
		Metadata: client, Perms: boundTo("payments", "storefront"),
		Descriptors: map[string]api.ResourceDescriptor{"/v1/pods": descriptorsFor("/v1/pods")[0]},
	})
	found := manager.Search(alice(t), "web")
	if !reflect.DeepEqual(names(found), []string{"web-payments"}) {
		t.Fatalf("hits = %+v, want the readable namespace preserved", found.Hits)
	}
	if !reflect.DeepEqual(found.Errors, map[string]string{"/v1/pods/storefront": refused.Error()}) {
		t.Fatalf("errors = %v, want only the namespace refusal", found.Errors)
	}
}

func TestIdentityCountsDoNotReuseOrOverwriteTheLocalCountCache(t *testing.T) {
	client := fakeMeta(t, meta("", "v1", "ConfigMap", "payments", "first"))
	manager := NewManager(t.Context(), Deps{
		Metadata:    client,
		Descriptors: map[string]api.ResourceDescriptor{"/v1/configmaps": descriptorsFor("/v1/configmaps")[0]},
	})
	if got := manager.Counts(t.Context()).Counts["/v1/configmaps"]; got != 1 {
		t.Fatalf("local count = %d, want 1", got)
	}
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	if err := client.Tracker().Create(gvr, meta("", "v1", "ConfigMap", "payments", "second"), "payments"); err != nil {
		t.Fatalf("add second map: %v", err)
	}
	if got := manager.Counts(alice(t)).Counts["/v1/configmaps"]; got != 2 {
		t.Fatalf("alice count = %d, want a fresh read of 2", got)
	}
	if err := client.Tracker().Create(gvr, meta("", "v1", "ConfigMap", "payments", "third"), "payments"); err != nil {
		t.Fatalf("add third map: %v", err)
	}
	bob := auth.WithIdentity(t.Context(), auth.Identity{User: "bob"})
	if got := manager.Counts(bob).Counts["/v1/configmaps"]; got != 3 {
		t.Fatalf("bob count = %d, want 3 rather than alice's or the local result", got)
	}
	client.ClearActions()
	if got := manager.Counts(t.Context()).Counts["/v1/configmaps"]; got != 1 {
		t.Fatalf("local count = %d, want the original local cache", got)
	}
	if len(client.Actions()) != 0 {
		t.Fatalf("local cache was replaced or invalidated: %v", client.Actions())
	}
}

func TestARefusedIdentityCountCannotFallBackToTheLocalCount(t *testing.T) {
	client := fakeMeta(t, meta("", "v1", "ConfigMap", "payments", "private"))
	manager := NewManager(t.Context(), Deps{
		Metadata:    client,
		Descriptors: map[string]api.ResourceDescriptor{"/v1/configmaps": descriptorsFor("/v1/configmaps")[0]},
	})
	if got := manager.Counts(t.Context()).Counts["/v1/configmaps"]; got != 1 {
		t.Fatalf("seed count = %d, want 1", got)
	}
	refused := errors.New("this identity cannot list configmaps")
	client.PrependReactor("list", "configmaps", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, refused
	})
	found := manager.Counts(alice(t))
	if !reflect.DeepEqual(found.Counts, map[string]int{"/v1/configmaps": -1}) {
		t.Fatalf("counts = %v, want unknown rather than the privileged count", found.Counts)
	}
	if !reflect.DeepEqual(found.Errors, map[string]string{"/v1/configmaps": refused.Error()}) {
		t.Fatalf("errors = %v, want the refusal", found.Errors)
	}
}
