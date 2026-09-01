package commandbuffer

import "sync"

type Buffer struct {
	mu       sync.Mutex
	body     []byte
	limit    int
	tail     bool
	exceeded bool
}

func Head(limit int) *Buffer {
	return &Buffer{limit: positive(limit)}
}

func Tail(limit int) *Buffer {
	return &Buffer{limit: positive(limit), tail: true}
}

func positive(limit int) int {
	if limit < 0 {
		return 0
	}
	return limit
}

func (b *Buffer) Write(chunk []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	written := len(chunk)
	if b.tail {
		b.writeTail(chunk)
		return written, nil
	}
	b.writeHead(chunk)
	return written, nil
}

func (b *Buffer) writeHead(chunk []byte) {
	remaining := b.limit - len(b.body)
	if remaining <= 0 {
		if len(chunk) > 0 {
			b.exceeded = true
		}
		return
	}
	if len(chunk) <= remaining {
		b.body = append(b.body, chunk...)
		return
	}
	b.body = append(b.body, chunk[:remaining]...)
	b.exceeded = true
}

func (b *Buffer) writeTail(chunk []byte) {
	if len(chunk) == 0 {
		return
	}
	if b.limit == 0 {
		b.exceeded = true
		return
	}
	if len(chunk) >= b.limit {
		b.body = append(b.body[:0], chunk[len(chunk)-b.limit:]...)
		b.exceeded = true
		return
	}
	overflow := len(b.body) + len(chunk) - b.limit
	if overflow > 0 {
		copy(b.body, b.body[overflow:])
		b.body = b.body[:len(b.body)-overflow]
		b.exceeded = true
	}
	b.body = append(b.body, chunk...)
}

func (b *Buffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte{}, b.body...)
}

func (b *Buffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.body)
}

func (b *Buffer) Exceeded() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.exceeded
}
