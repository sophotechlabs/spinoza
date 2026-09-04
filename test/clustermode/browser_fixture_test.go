//go:build clustermode

package clustermode

import (
	"os"
	"testing"
)

func TestTheBrowserFixtureIsReady(t *testing.T) {
	mode := os.Getenv("SPINOZA_CM_BROWSER_FIXTURE")
	if mode == "" {
		t.Skip("SPINOZA_CM_BROWSER_FIXTURE is not set")
	}
	if mode == "oidc" {
		values := oidcValues()
		values["persistence.enabled"] = "true"
		deploy(t, values)
		session := whoami(t, signIn(t, "alice"))
		if session.User != "alice" || session.Role != "admin" {
			t.Fatalf("oidc fixture session = %+v, want alice as admin", session)
		}
		return
	}
	if mode == "proxy" {
		deployRealProxy(t)
		session := whoami(t, signIn(t, "alice"))
		if session.User != "alice" || session.Role != "admin" {
			t.Fatalf("proxy fixture session = %+v, want alice as admin", session)
		}
		return
	}
	t.Fatalf("SPINOZA_CM_BROWSER_FIXTURE = %q, want oidc or proxy", mode)
}
