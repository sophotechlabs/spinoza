package issues

import (
	"testing"
	"time"
)

func TestDurationLabelsMatchTheUserInterfaceUnits(t *testing.T) {
	tests := []struct {
		name string
		span time.Duration
		want string
	}{
		{name: "negative", span: -time.Second, want: "0s"},
		{name: "seconds", span: 59 * time.Second, want: "59s"},
		{name: "minute", span: time.Minute, want: "1m"},
		{name: "minutes", span: 59*time.Minute + 59*time.Second, want: "59m"},
		{name: "hour", span: time.Hour, want: "1h"},
		{name: "hours", span: 23*time.Hour + 59*time.Minute, want: "23h"},
		{name: "day", span: 24 * time.Hour, want: "1d"},
		{name: "days", span: 65*time.Hour + 15*time.Minute, want: "2d"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := durationLabel(test.span); got != test.want {
				t.Fatalf("label = %q, want %q", got, test.want)
			}
		})
	}
}
