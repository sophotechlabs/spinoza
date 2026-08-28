package main

import (
	"context"

	"github.com/sophotechlabs/spinoza/internal/logs"
	"github.com/sophotechlabs/spinoza/internal/server"
)

type logReader struct {
	server.Backend
}

func (l logReader) LogLines(ctx context.Context, req logs.Request) ([]string, error) {
	stream, err := l.Logs(ctx, req)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	out := []string{}
	for line := range stream.Lines {
		out = append(out, line.Text)
	}
	return out, stream.Err()
}
