//go:build clustermode

package clustermode

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestMetricsHistoryIsTheReadersOwnPermission(t *testing.T) {
	deploy(t, oidcValues())
	pod := podIn(t, "payments", "app=web")
	path := "/api/metrics/history?namespace=payments&pod=" + url.QueryEscape(pod) + "&range=1h"

	status, message := read(t, signIn(t, "alice"), path)
	if status != http.StatusOK {
		t.Fatalf("an admin reading history gave %d: %s", status, message)
	}

	status, message = read(t, signIn(t, "bob"), path)
	if status != http.StatusForbidden {
		t.Fatalf("a viewer without metrics.k8s.io reading history gave %d: %s", status, message)
	}
	if !strings.Contains(messageOf(t, message), "authorization denied") {
		t.Fatalf("the refusal %q does not say it is the reader's own permission", message)
	}
}
