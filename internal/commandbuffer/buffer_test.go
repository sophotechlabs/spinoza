package commandbuffer

import (
	"sync"
	"testing"
)

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

func TestFillingABufferExactlyDoesNotClaimTruncation(t *testing.T) {
	for _, buffer := range []*Buffer{Head(3), Tail(3)} {
		_, _ = buffer.Write([]byte("abc"))

		if buffer.Exceeded() {
			t.Fatalf("buffer %q claimed an exact fit was truncated", buffer.String())
		}
	}
}

func TestAnExactTailWriteReplacesOlderBytesAndClaimsTruncation(t *testing.T) {
	buffer := Tail(3)
	_, _ = buffer.Write([]byte("ab"))
	_, _ = buffer.Write([]byte("cde"))

	if buffer.String() != "cde" {
		t.Fatalf("body = %q", buffer.String())
	}
	if !buffer.Exceeded() {
		t.Fatal("replacing older bytes was not recorded as truncation")
	}
}

func TestEmptyWritesDoNotClaimTruncation(t *testing.T) {
	for _, buffer := range []*Buffer{Head(3), Tail(3)} {
		written, err := buffer.Write(nil)

		if err != nil || written != 0 {
			t.Fatalf("write = %d, %v", written, err)
		}
		if buffer.Exceeded() {
			t.Fatal("an empty write claimed truncation")
		}
	}
}

func TestBytesHandsOutACopy(t *testing.T) {
	buffer := Head(3)
	_, _ = buffer.Write([]byte("abc"))

	buffer.Bytes()[0] = 'x'

	if buffer.String() != "abc" {
		t.Fatalf("body = %q, want the buffer isolated from its caller", buffer.String())
	}
}

func TestConcurrentWritesStayBounded(t *testing.T) {
	for _, buffer := range []*Buffer{Head(64), Tail(64)} {
		var group sync.WaitGroup
		for range 128 {
			group.Go(func() {
				_, _ = buffer.Write([]byte("abcdefgh"))
			})
		}
		group.Wait()

		if len(buffer.Bytes()) != 64 {
			t.Fatalf("buffer length = %d, want its limit", len(buffer.Bytes()))
		}
		if !buffer.Exceeded() {
			t.Fatal("concurrent overflow was not recorded")
		}
	}
}
