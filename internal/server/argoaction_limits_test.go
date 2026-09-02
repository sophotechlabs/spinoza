package server

import (
	"net/http"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestArgoActionRefusesAnOversizedOptionsBodyBeforeChangingTheApplication(t *testing.T) {
	ts, dyn := argoActionServer(t, false, newArgoApplication())

	resp, body := postArgoAction(t, ts, "sync", strings.Repeat(" ", maxDocBytes+1))

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413: %s", resp.StatusCode, body)
	}
	if _, found, err := unstructured.NestedMap(storedApplication(t, dyn).Object, "operation"); err != nil || found {
		t.Fatalf("operation found = %v, error = %v, want the oversized action left out", found, err)
	}
}
