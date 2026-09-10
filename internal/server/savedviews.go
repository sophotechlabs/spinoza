package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/auth"
)

const (
	savedViewsKey    = "savedViews"
	sharedViewsKey   = "sharedViews"
	maxSavedViews    = 100
	maxSavedViewName = 80
	maxViewBytes     = 64 << 10
)

var errTooManyViews = errors.New("that is as many saved views as spinoza keeps; forget one first")

var errNoViewName = errors.New("a saved view needs a name")

var errNotYoursToShare = errors.New("your role here cannot publish a view for everybody")

func (s *Server) listSavedViews(w http.ResponseWriter, r *http.Request) {
	held := s.stored().All()
	out := api.SavedViews{Views: []api.SavedView{}, MayShare: s.mayShareViews(r)}
	out.Views = append(out.Views, decodeViews(held[sharedViewsKey], true)...)
	out.Views = append(out.Views, decodeViews(s.settingFor(r, held, savedViewsKey), false)...)
	writeJSON(w, out)
}

func (s *Server) saveView(w http.ResponseWriter, r *http.Request) {
	var wanted api.SavedView
	if err := decodeJSONBody(w, r, maxViewBytes, &wanted); err != nil {
		writeError(w, http.StatusBadRequest, "a saved view must be an object")
		return
	}
	wanted.Name = strings.TrimSpace(wanted.Name)
	if wanted.Name == "" {
		writeError(w, http.StatusBadRequest, errNoViewName.Error())
		return
	}
	if len(wanted.Name) > maxSavedViewName {
		writeError(w, http.StatusBadRequest, "that name is longer than spinoza keeps")
		return
	}
	if wanted.Shared && !s.mayShareViews(r) {
		writeError(w, http.StatusForbidden, errNotYoursToShare.Error())
		return
	}
	if wanted.ID == "" {
		wanted.ID = newRequestID()
	}
	wanted.At = s.instant().UTC().Format(time.RFC3339)
	s.settingsWrite.Lock()
	defer s.settingsWrite.Unlock()
	held, key, err := s.viewsFor(r, wanted.Shared)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	kept, replaced := replaceView(held, wanted)
	if !replaced && len(kept) > maxSavedViews {
		writeError(w, http.StatusConflict, errTooManyViews.Error())
		return
	}
	if saveErr := s.writeViews(key, kept); saveErr != nil {
		writeError(w, http.StatusInternalServerError, saveErr.Error())
		return
	}
	writeJSON(w, wanted)
}

func (s *Server) forgetView(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "which saved view?")
		return
	}
	shared := r.URL.Query().Get("shared") == queryTrue
	if shared && !s.mayShareViews(r) {
		writeError(w, http.StatusForbidden, errNotYoursToShare.Error())
		return
	}
	s.settingsWrite.Lock()
	defer s.settingsWrite.Unlock()
	held, key, err := s.viewsFor(r, shared)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	kept := slices.DeleteFunc(held, func(one api.SavedView) bool { return one.ID == id })
	if saveErr := s.writeViews(key, kept); saveErr != nil {
		writeError(w, http.StatusInternalServerError, saveErr.Error())
		return
	}
	writeJSON(w, api.SavedViews{Views: kept, MayShare: s.mayShareViews(r)})
}

func (s *Server) viewsFor(r *http.Request, shared bool) ([]api.SavedView, string, error) {
	values := s.stored().All()
	if shared {
		return decodeViews(values[sharedViewsKey], true), sharedViewsKey, nil
	}
	if !s.inCluster() {
		return decodeViews(values[savedViewsKey], false), savedViewsKey, nil
	}
	prefix, ok := settingsPrefix(r)
	if !ok {
		return nil, "", errors.New("spinoza does not know who you are, so it cannot keep a view for you")
	}
	return decodeViews(values[prefix+savedViewsKey], false), prefix + savedViewsKey, nil
}

func (s *Server) writeViews(key string, held []api.SavedView) error {
	//nolint:errchkjson // a saved view is strings and string slices; encoding it cannot fail
	body, _ := json.Marshal(held)
	return s.stored().Merge(map[string]string{key: string(body)})
}

func (s *Server) mayShareViews(r *http.Request) bool {
	if !s.inCluster() {
		return true
	}
	return s.holdsRole(r, auth.RoleAdmin)
}

func decodeViews(raw string, shared bool) []api.SavedView {
	if raw == "" {
		return []api.SavedView{}
	}
	var held []api.SavedView
	if err := json.Unmarshal([]byte(raw), &held); err != nil {
		return []api.SavedView{}
	}
	out := make([]api.SavedView, 0, len(held))
	for _, one := range held {
		if one.ID == "" || one.Name == "" {
			continue
		}
		one.Shared = shared
		out = append(out, one)
	}
	return out
}

func replaceView(held []api.SavedView, wanted api.SavedView) ([]api.SavedView, bool) {
	for at, one := range held {
		if one.ID == wanted.ID {
			held[at] = wanted
			return held, true
		}
	}
	return append(held, wanted), false
}
