package store

import (
	"slices"
	"testing"
)

func FuzzTimelineCellsRoundTrip(f *testing.F) {
	f.Add("Ready", "3/3", "hello")
	f.Add("", "\x00", "line\nbreak")
	f.Add("日本語", `"quoted"`, `[]{}\\`)

	f.Fuzz(func(t *testing.T, first, second, third string) {
		cells := []string{first, second, third}
		if got := cellsOf(cellsText(cells)); !slices.Equal(got, cells) {
			t.Fatalf("cells changed: %q != %q", got, cells)
		}
	})
}

func FuzzHistoryLimit(f *testing.F) {
	for _, seed := range []int{-1, 0, 1, defaultLimit, maxLimit, maxLimit + 1} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, asked int) {
		got := Limit(asked)
		if got <= 0 {
			t.Fatalf("Limit(%d) = %d", asked, got)
		}
		if got > maxLimit {
			t.Fatalf("Limit(%d) = %d, above %d", asked, got, maxLimit)
		}
		if asked > 0 && asked <= maxLimit && got != asked {
			t.Fatalf("Limit(%d) = %d", asked, got)
		}
	})
}
