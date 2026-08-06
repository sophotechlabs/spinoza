package api_test

import (
	"encoding/json"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
)

func encoded(t *testing.T, frame any) string {
	t.Helper()
	out, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(out)
}

func TestEveryFeedFrameKeepsItsWireShape(t *testing.T) {
	row := api.Row{UID: "u-1", Name: "web", Namespace: "default", CreatedAt: "2026-08-06T09:00:00Z", Cells: []string{"1/1"}}
	cases := []struct {
		name  string
		frame any
		want  string
	}{
		{
			name:  "snapshot",
			frame: api.Snapshot{Type: "snapshot", SubID: "main", Columns: []api.Column{{Name: "Name"}}, Namespaced: true, Rows: []api.Row{row}},
			want:  `{"type":"snapshot","subId":"main","columns":[{"name":"Name"}],"namespaced":true,"rows":[{"uid":"u-1","name":"web","namespace":"default","createdAt":"2026-08-06T09:00:00Z","cells":["1/1"]}]}`,
		},
		{
			name:  "added",
			frame: api.RowChanged{Type: "added", SubID: "main", Row: row},
			want:  `{"type":"added","subId":"main","row":{"uid":"u-1","name":"web","namespace":"default","createdAt":"2026-08-06T09:00:00Z","cells":["1/1"]}}`,
		},
		{
			name:  "modified",
			frame: api.RowChanged{Type: "modified", SubID: "main", Row: row},
			want:  `{"type":"modified","subId":"main","row":{"uid":"u-1","name":"web","namespace":"default","createdAt":"2026-08-06T09:00:00Z","cells":["1/1"]}}`,
		},
		{
			name:  "deleted",
			frame: api.RowDeleted{Type: "deleted", SubID: "main", UID: "u-1"},
			want:  `{"type":"deleted","subId":"main","uid":"u-1"}`,
		},
		{
			name:  "log",
			frame: api.LogLines{Type: "log", SubID: "logs", Lines: []string{"first", "second"}},
			want:  `{"type":"log","subId":"logs","lines":["first","second"]}`,
		},
		{
			name:  "log-end",
			frame: api.LogEnd{Type: "log-end", SubID: "logs"},
			want:  `{"type":"log-end","subId":"logs"}`,
		},
		{
			name:  "error",
			frame: api.FeedError{Type: "error", SubID: "main", Message: "deployments is forbidden"},
			want:  `{"type":"error","subId":"main","message":"deployments is forbidden"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := encoded(t, tc.frame); got != tc.want {
				t.Fatalf("frame = %s\nwant   %s", got, tc.want)
			}
		})
	}
}

func TestEveryFeedFrameStillDecodesAsAServerMsg(t *testing.T) {
	frames := []any{
		api.RowChanged{Type: "added", SubID: "main", Row: api.Row{UID: "u-1", Name: "web"}},
		api.RowDeleted{Type: "deleted", SubID: "main", UID: "u-1"},
		api.LogLines{Type: "log", SubID: "logs", Lines: []string{"a"}},
		api.LogEnd{Type: "log-end", SubID: "logs"},
		api.FeedError{Type: "error", SubID: "main", Message: "boom"},
	}
	for _, frame := range frames {
		var decoded api.ServerMsg
		if err := json.Unmarshal([]byte(encoded(t, frame)), &decoded); err != nil {
			t.Fatalf("decode %T: %v", frame, err)
		}
		if decoded.Type == "" {
			t.Fatalf("%T decoded without a type", frame)
		}
		if decoded.SubID == "" {
			t.Fatalf("%T decoded without a subId", frame)
		}
	}
}
