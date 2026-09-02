package server

import (
	"strings"
	"testing"
)

func TestFleetCursorRejectsBytesThatAreNotBase64(t *testing.T) {
	cursor, err := decodeFleetCheckCursor("%")

	if err == nil {
		t.Fatal("a cursor that is not base64 was accepted")
	}
	if !strings.Contains(err.Error(), "cursor is invalid") {
		t.Fatalf("error = %v, want the invalid cursor reason", err)
	}
	if cursor.Check != "" || len(cursor.After) != 0 {
		t.Fatalf("cursor = %+v, want no partial state from invalid bytes", cursor)
	}
}
