package commandbuffer

import "testing"

func TestAHeadBufferKeepsOnlyItsLimit(t *testing.T) {
	buffer := Head(5)
	first, firstErr := buffer.Write([]byte("abc"))
	second, secondErr := buffer.Write([]byte("defg"))

	if firstErr != nil || secondErr != nil {
		t.Fatalf("write errors = %v, %v", firstErr, secondErr)
	}
	if first != 3 || second != 4 {
		t.Fatalf("writes = %d and %d", first, second)
	}
	if buffer.String() != "abcde" {
		t.Fatalf("body = %q", buffer.String())
	}
	if !buffer.Exceeded() {
		t.Fatal("overflow was not recorded")
	}
}

func TestATailBufferKeepsTheNewestBytes(t *testing.T) {
	buffer := Tail(5)
	_, _ = buffer.Write([]byte("abc"))
	_, _ = buffer.Write([]byte("defg"))

	if buffer.String() != "cdefg" {
		t.Fatalf("body = %q", buffer.String())
	}
	if !buffer.Exceeded() {
		t.Fatal("overflow was not recorded")
	}
}

func TestATailWriteLargerThanTheLimitKeepsItsEnd(t *testing.T) {
	buffer := Tail(3)
	_, _ = buffer.Write([]byte("abcdef"))

	if buffer.String() != "def" {
		t.Fatalf("body = %q", buffer.String())
	}
}

func TestAZeroLimitStillAcceptsWrites(t *testing.T) {
	for _, buffer := range []*Buffer{Head(0), Tail(-1)} {
		written, err := buffer.Write([]byte("abc"))
		if err != nil || written != 3 {
			t.Fatalf("write = %d, %v", written, err)
		}
		if len(buffer.Bytes()) != 0 || !buffer.Exceeded() {
			t.Fatalf("buffer = %q, exceeded = %t", buffer.String(), buffer.Exceeded())
		}
	}
}
