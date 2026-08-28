package mcp

import (
	"bufio"
	"context"
	"io"
)

const maxMessage = 8 * 1024 * 1024

func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	reader := bufio.NewScanner(in)
	reader.Buffer(make([]byte, 0, 64*1024), maxMessage)
	writer := bufio.NewWriter(out)
	for reader.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := reader.Bytes()
		if len(line) == 0 {
			continue
		}
		reply := s.Handle(ctx, line)
		if reply == nil {
			continue
		}
		if _, err := writer.Write(append(reply, '\n')); err != nil {
			return err
		}
		if err := writer.Flush(); err != nil {
			return err
		}
	}
	return reader.Err()
}
