package mcp

import (
	"bufio"
	"context"
	"errors"
	"io"
	"sync"
)

const (
	maxMessage = 8 * 1024 * 1024
	atOnce     = 8
)

type writer struct {
	mu     sync.Mutex
	out    *bufio.Writer
	failed error
}

func (w *writer) send(body []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.failed != nil {
		return
	}
	if _, err := w.out.Write(append(body, '\n')); err != nil {
		w.failed = err
		return
	}
	w.failed = w.out.Flush()
}

func (w *writer) err() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.failed
}

func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	reader := bufio.NewReaderSize(in, 64*1024)
	sink := &writer{out: bufio.NewWriter(out)}
	slots := make(chan struct{}, atOnce)
	var running sync.WaitGroup

	for {
		if ctx.Err() != nil {
			running.Wait()
			return ctx.Err()
		}
		line, err := readMessage(reader)
		if len(line) > 0 {
			running.Add(1)
			slots <- struct{}{}
			go func() {
				defer running.Done()
				defer func() { <-slots }()
				reply := s.answer(ctx, line)
				if reply == nil {
					return
				}
				sink.send(reply)
			}()
		}
		if err != nil {
			running.Wait()
			if errors.Is(err, io.EOF) {
				return sink.err()
			}
			return err
		}
		if sink.err() != nil {
			running.Wait()
			return sink.err()
		}
	}
}

func (s *Server) answer(ctx context.Context, line []byte) []byte {
	if len(line) > maxMessage {
		return encode(refuse(nil, codeInvalidRequest, "the message is larger than this server accepts"))
	}
	return s.Handle(ctx, line)
}

// A message is one line. Reading it in parts keeps an oversized one from ending
// the session: it is answered with an error like any other bad message.
func readMessage(reader *bufio.Reader) ([]byte, error) {
	var whole []byte
	for {
		part, err := reader.ReadSlice('\n')
		whole = append(whole, part...)
		if err == nil {
			return trimEnd(whole), nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return trimEnd(whole), err
	}
}

func trimEnd(line []byte) []byte {
	for len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r') {
		line = line[:len(line)-1]
	}
	return line
}
