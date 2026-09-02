package server

import (
	"errors"
	"testing"
)

func TestKeepingThePreviousClusterActiveSkipsNoOpRestorations(t *testing.T) {
	for name, pair := range map[string][2]string{
		"there was no previous cluster":         {"", mk2},
		"the opened cluster was already active": {mk1, mk1},
	} {
		t.Run(name, func(t *testing.T) {
			held := &fleet{}
			srv := New(held, testAssets(), "")

			srv.keepActive(pair[0], pair[1])

			if len(held.activated) != 0 {
				t.Fatalf("activated = %v, want no restoration", held.activated)
			}
		})
	}
}

func TestKeepingThePreviousClusterActiveToleratesItsDisappearance(t *testing.T) {
	held := &fleet{activateErr: errors.New("the previous cluster closed")}
	srv := New(held, testAssets(), "")

	srv.keepActive(mk1, mk2)

	if len(held.activated) != 1 || held.activated[0] != mk1 {
		t.Fatalf("activated = %v, want the previous cluster attempted once", held.activated)
	}
}
