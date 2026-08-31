package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/checks"
)

const maxMuteBytes = 1 << 16

const maxMutes = 2000

var errTooManyMutes = errors.New("that is more mutes than one cluster holds; turn the check off or skip the namespace instead")

func (s *Server) muteFinding(w http.ResponseWriter, r *http.Request) {
	wanted, ok := decodeMute(w, r)
	if !ok {
		return
	}
	wanted.At = s.now().UTC().Format(time.DateOnly)
	s.changeMutes(w, r, func(held []checks.Mute) ([]checks.Mute, error) {
		out := without(held, wanted)
		if len(out) >= maxMutes {
			return nil, errTooManyMutes
		}
		return append(out, wanted), nil
	})
}

func (s *Server) unmuteFinding(w http.ResponseWriter, r *http.Request) {
	wanted, ok := decodeMute(w, r)
	if !ok {
		return
	}
	s.changeMutes(w, r, func(held []checks.Mute) ([]checks.Mute, error) {
		return without(held, wanted), nil
	})
}

func decodeMute(w http.ResponseWriter, r *http.Request) (checks.Mute, bool) {
	var wanted checks.Mute
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxMuteBytes)).Decode(&wanted)
	if err != nil {
		writeError(w, http.StatusBadRequest, "a mute must be an object")
		return checks.Mute{}, false
	}
	if wanted.Check == "" {
		writeError(w, http.StatusBadRequest, "a mute must name the check it silences")
		return checks.Mute{}, false
	}
	return wanted, true
}

func (s *Server) changeMutes(
	w http.ResponseWriter, r *http.Request, change func([]checks.Mute) ([]checks.Mute, error),
) {
	cluster := s.clusterKey(r)
	all := checks.AllMutes(s.stored().All()[checks.MutesKey])
	next, err := change(all[cluster])
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if len(next) == 0 {
		delete(all, cluster)
	} else {
		all[cluster] = next
	}
	saveErr := s.stored().Merge(map[string]string{checks.MutesKey: checks.EncodeMutes(all)})
	if saveErr != nil {
		writeError(w, http.StatusInternalServerError, saveErr.Error())
		return
	}
	writeJSON(w, api.Mutes{Mutes: next})
}

func (s *Server) readMutes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, api.Mutes{Mutes: checks.ParseMutes(s.stored().All()[checks.MutesKey], s.clusterKey(r))})
}

func without(held []checks.Mute, wanted checks.Mute) []checks.Mute {
	out := slices.Clone(held)
	return slices.DeleteFunc(out, func(one checks.Mute) bool {
		return checks.SameMute(one, wanted)
	})
}
