package issues

import (
	"errors"
	"testing"
)

func FuzzCursor(f *testing.F) {
	for _, seed := range []struct {
		key   string
		order byte
	}{
		{key: "cluster\x00issue", order: 0},
		{key: "000000001\x00alpha\x00pod/a", order: 1},
		{key: "\x00", order: 2},
		{key: "", order: 3},
	} {
		f.Add(seed.key, seed.order)
	}

	orders := []string{ByWorst, ByNewest, ByOldest}
	f.Fuzz(func(t *testing.T, key string, selector byte) {
		order := orders[int(selector)%len(orders)]
		cursor := EncodeCursor(key, order)
		decoded, err := DecodeCursor(cursor, order)
		if err != nil {
			t.Fatalf("decode encoded cursor: %v", err)
		}
		if decoded != key {
			t.Fatalf("decoded key %q, want %q", decoded, key)
		}
		if cursor == "" {
			return
		}
		for _, other := range orders {
			if other == order {
				continue
			}
			if _, err := DecodeCursor(cursor, other); !errors.Is(err, ErrCursorOrder) {
				t.Fatalf("cursor for %q decoded under %q with %v", order, other, err)
			}
		}
	})
}
