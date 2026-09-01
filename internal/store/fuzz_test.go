package store

import (
	"slices"
	"testing"
)

func FuzzTimelineCellsRoundTrip(f *testing.F) {
	f.Add("Ready", "3/3", "hello")
	f.Add("", "\x00", "line\nbreak")
	f.Add("\u65e5\u672c\u8a9e", `"quoted"`, `[]{}\\`)
	f.Add("\xff", "\xfe\xff", "valid")

	f.Fuzz(func(t *testing.T, first, second, third string) {
		cells := []string{first, second, third}
		want := []string{string([]rune(first)), string([]rune(second)), string([]rune(third))}
		if got := cellsOf(cellsText(cells)); !slices.Equal(got, want) {
			t.Fatalf("cells changed: %q != %q", got, want)
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
