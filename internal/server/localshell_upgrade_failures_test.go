package server

import (
	"errors"
	"net/http/httptest"
	"testing"
)

func TestARefusedLocalShellUpgradeReleasesItsConnectionSlot(t *testing.T) {
	srv := New(nil, nil, "")
	srv.liveLimit = 1
	srv.identityLimit = 1
	opened := false
	srv.UseLocalShell(func(uint16, uint16) (LocalShell, error) {
		opened = true
		return nil, errors.New("local shell must not open")
	})
	req := liveRequest(t, "alice")
	recorded := httptest.NewRecorder()

	srv.handleLocalShell(recorded, req)

	if opened {
		t.Fatal("the shell opened after the websocket upgrade was refused")
	}
	release, ok := srv.claimLiveConnection(req)
	if !ok {
		t.Fatal("the refused upgrade leaked its live connection slot")
	}
	release()
}
