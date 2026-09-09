package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/auth"
	settingsstore "github.com/sophotechlabs/spinoza/internal/settings"
)

func savedViewServer(t *testing.T) *Server {
	t.Helper()
	srv := New(&stubBackendCluster{backend: &stubCatalog{}}, testAssets(), testToken)
	srv.UseSettings(settingsstore.Memory())
	return srv
}

func saveOne(t *testing.T, srv *Server, view api.SavedView, who *auth.Identity) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/views", strings.NewReader(string(body)))
	if who != nil {
		req = req.WithContext(auth.WithIdentity(req.Context(), *who))
	}
	rec := httptest.NewRecorder()
	srv.saveView(rec, req)
	return rec
}

func listViews(t *testing.T, srv *Server, who *auth.Identity) api.SavedViews {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/views", http.NoBody)
	if who != nil {
		req = req.WithContext(auth.WithIdentity(req.Context(), *who))
	}
	rec := httptest.NewRecorder()
	srv.listSavedViews(rec, req)
	var page api.SavedViews
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("the page did not parse: %v (%s)", err, rec.Body.String())
	}
	return page
}

func TestASavedViewComesBackWithWhatWasSaved(t *testing.T) {
	srv := savedViewServer(t)

	rec := saveOne(t, srv, api.SavedView{
		Name:      "crashing pods",
		View:      "resources",
		Resource:  "pods",
		Namespace: "prod",
		Filter:    "status=CrashLoopBackOff",
		Columns:   []string{"name", "status"},
		Sort:      "name",
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	page := listViews(t, srv, nil)
	if len(page.Views) != 1 {
		t.Fatalf("views = %d, want 1", len(page.Views))
	}
	one := page.Views[0]
	if one.Name != "crashing pods" || one.Filter != "status=CrashLoopBackOff" || one.Namespace != "prod" {
		t.Fatalf("view = %+v", one)
	}
	if len(one.Columns) != 2 || one.ID == "" || one.At == "" {
		t.Fatalf("view = %+v", one)
	}
}

func TestSavingTheSameViewAgainReplacesItRatherThanAddingOne(t *testing.T) {
	srv := savedViewServer(t)
	saveOne(t, srv, api.SavedView{Name: "one", View: "resources"}, nil)
	held := listViews(t, srv, nil).Views[0]

	held.Name = "renamed"
	saveOne(t, srv, held, nil)

	page := listViews(t, srv, nil)
	if len(page.Views) != 1 {
		t.Fatalf("views = %d, want the same one back", len(page.Views))
	}
	if page.Views[0].Name != "renamed" {
		t.Fatalf("name = %q", page.Views[0].Name)
	}
}

func TestAViewWithNoNameIsRefused(t *testing.T) {
	srv := savedViewServer(t)

	rec := saveOne(t, srv, api.SavedView{Name: "   ", View: "resources"}, nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "needs a name") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestForgettingAViewLeavesTheOthers(t *testing.T) {
	srv := savedViewServer(t)
	saveOne(t, srv, api.SavedView{Name: "one", View: "resources"}, nil)
	saveOne(t, srv, api.SavedView{Name: "two", View: "checks"}, nil)
	first := listViews(t, srv, nil).Views[0]

	rec := httptest.NewRecorder()
	srv.forgetView(rec, httptest.NewRequest(http.MethodDelete, "/api/views?id="+first.ID, http.NoBody))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	page := listViews(t, srv, nil)
	if len(page.Views) != 1 {
		t.Fatalf("views = %d, want 1 left", len(page.Views))
	}
	if page.Views[0].ID == first.ID {
		t.Fatal("the wrong view was forgotten")
	}
}

func TestForgettingWithNoIdIsRefused(t *testing.T) {
	srv := savedViewServer(t)

	rec := httptest.NewRecorder()
	srv.forgetView(rec, httptest.NewRequest(http.MethodDelete, "/api/views", http.NoBody))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func servedViewServer(t *testing.T) *Server {
	t.Helper()
	srv := savedViewServer(t)
	srv.UseClusterAuth(ClusterAuth{})
	return srv
}

func TestOnePersonsViewsAreNotAnothersWhenServingACluster(t *testing.T) {
	srv := servedViewServer(t)
	alice := auth.Identity{User: "alice@example.com", Role: auth.RoleEditor}
	bob := auth.Identity{User: "bob@example.com", Role: auth.RoleEditor}

	saveOne(t, srv, api.SavedView{Name: "alice's", View: "resources"}, &alice)

	if got := listViews(t, srv, &alice); len(got.Views) != 1 {
		t.Fatalf("alice sees %d of her own views", len(got.Views))
	}
	if got := listViews(t, srv, &bob); len(got.Views) != 0 {
		t.Fatalf("bob sees %d of alice's views", len(got.Views))
	}
}

func TestOnlyAnAdminPublishesAViewForEverybody(t *testing.T) {
	srv := servedViewServer(t)
	editor := auth.Identity{User: "editor@example.com", Role: auth.RoleEditor}
	admin := auth.Identity{User: "admin@example.com", Role: auth.RoleAdmin}

	refused := saveOne(t, srv, api.SavedView{Name: "for everybody", View: "checks", Shared: true}, &editor)
	if refused.Code != http.StatusForbidden {
		t.Fatalf("an editor published a shared view: %d %s", refused.Code, refused.Body.String())
	}

	allowed := saveOne(t, srv, api.SavedView{Name: "for everybody", View: "checks", Shared: true}, &admin)
	if allowed.Code != http.StatusOK {
		t.Fatalf("an admin could not publish: %d %s", allowed.Code, allowed.Body.String())
	}

	seen := listViews(t, srv, &editor)
	if len(seen.Views) != 1 || !seen.Views[0].Shared {
		t.Fatalf("the editor does not see the published view: %+v", seen.Views)
	}
	if seen.MayShare {
		t.Fatal("the editor was told they may publish")
	}
}

func TestABrokenStoredViewListReadsAsNoneRatherThanBreaking(t *testing.T) {
	srv := savedViewServer(t)
	if err := srv.stored().Merge(map[string]string{savedViewsKey: "not json"}); err != nil {
		t.Fatalf("merge: %v", err)
	}

	page := listViews(t, srv, nil)

	if len(page.Views) != 0 {
		t.Fatalf("views = %d, want none", len(page.Views))
	}
}
